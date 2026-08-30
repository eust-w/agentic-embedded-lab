package ael

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type StepResult struct {
	Outputs   map[string]float64
	Metrics   map[string]float64
	Events    []Event
	Artifacts map[string]string
}

type metricSample struct {
	AtUS  int64
	Value float64
}

type Adapter interface {
	Prepare(context.Context, Component, int64) error
	Inject(context.Context, string, any, int64) ([]Event, error)
	Step(context.Context, int64, int64) (StepResult, error)
	Snapshot(context.Context, int64) (string, error)
	Shutdown(context.Context) error
}
type Restorer interface {
	Restore(context.Context, string) error
}

type AdapterFactory func(Component) (Adapter, error)

type Scheduler struct {
	Factories map[Backend]AdapterFactory
}

func (s Scheduler) Run(ctx context.Context, experiment Experiment, system System, sourceRevision string) (bundle EvidenceBundle, runErr error) {
	if err := Validate(experiment, system); err != nil {
		return EvidenceBundle{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, experiment.Timeout)
	defer cancel()
	components, err := orderedComponents(system)
	if err != nil {
		return EvidenceBundle{}, err
	}
	started := time.Now().UTC()
	bundle = EvidenceBundle{APIVersion: APIVersion, RunID: uuid.NewString(), Experiment: experiment, System: system, Artifacts: make(map[string]string), SourceRevision: sourceRevision, StartedAt: started, Fidelity: aggregateFidelity(components)}
	defer func() {
		bundle.FinishedAt = time.Now().UTC()
		if runErr != nil {
			bundle.Failure = &RunFailure{Code: classifyRunError(runErr), Message: runErr.Error(), Retryable: errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, context.Canceled)}
		}
		payload, _ := json.Marshal(bundle.Events)
		digest := sha256.Sum256(payload)
		bundle.TraceSHA256 = hex.EncodeToString(digest[:])
	}()
	adapters := make(map[string]Adapter, len(components))
	defer func() {
		for _, adapter := range adapters {
			_ = adapter.Shutdown(context.Background())
		}
	}()
	for _, component := range components {
		factory := s.Factories[component.Backend]
		if factory == nil {
			return bundle, fmt.Errorf("backend %s is unavailable", component.Backend)
		}
		adapter, err := factory(component)
		if err != nil {
			return bundle, err
		}
		if err := adapter.Prepare(ctx, component, experiment.Seed); err != nil {
			return bundle, fmt.Errorf("prepare %s: %w", component.ID, err)
		}
		adapters[component.ID] = adapter
		if component.Rollback {
			if _, ok := adapter.(Restorer); !ok {
				return bundle, fmt.Errorf("component %s declares rollback but adapter cannot restore", component.ID)
			}
		}
	}
	metrics := make(map[string]float64)
	samples := make(map[string][]metricSample)
	connections := connectionsBySource(system.Connections)
	communicationStepUS := communicationStep(experiment, components)
	dirty := make(map[string]bool, len(components))
	for _, component := range components {
		dirty[component.ID] = true
	}
	var sequence int64
	for virtualTime := int64(0); virtualTime < experiment.DurationUS; virtualTime += communicationStepUS {
		select {
		case <-ctx.Done():
			return bundle, ctx.Err()
		default:
		}
		for _, stimulus := range experiment.Stimuli {
			if stimulus.AtUS == virtualTime {
				componentID, target := splitTarget(stimulus.Target)
				if adapters[componentID] == nil {
					return bundle, fmt.Errorf("stimulus target %s references unknown component", stimulus.Target)
				}
				injected, err := adapters[componentID].Inject(ctx, target, stimulus.Value, virtualTime)
				if err != nil {
					return bundle, err
				}
				sequence = appendEvents(&bundle.Events, injected, sequence, virtualTime, componentID)
				dirty[componentID] = true
			}
		}
		for _, fault := range experiment.Faults {
			if fault.AtUS == virtualTime {
				componentID, target := splitTarget(fault.Target)
				if adapters[componentID] == nil {
					return bundle, fmt.Errorf("fault target %s references unknown component", fault.Target)
				}
				injected, err := adapters[componentID].Inject(ctx, target, map[string]any{"kind": fault.Kind, "parameters": fault.Parameters}, virtualTime)
				if err != nil {
					return bundle, err
				}
				sequence = appendEvents(&bundle.Events, injected, sequence, virtualTime, componentID)
				dirty[componentID] = true
			}
		}
		maxIterations := 1
		if system.AlgebraicSolver != nil {
			maxIterations = system.AlgebraicSolver.MaxIterations
		}
		previousOutputs := map[string]float64{}
		converged := system.AlgebraicSolver == nil
		for iteration := 0; iteration < maxIterations; iteration++ {
			if iteration > 0 {
				for _, component := range components {
					checkpoint := bundle.Artifacts[fmt.Sprintf("checkpoint.%s.%d", component.ID, virtualTime)]
					if err := adapters[component.ID].(Restorer).Restore(ctx, checkpoint); err != nil {
						return bundle, err
					}
					dirty[component.ID] = true
				}
				for _, connection := range system.Connections {
					if value, ok := previousOutputs[connection.SourceComponent+"."+connection.SourcePort]; ok {
						if _, err := adapters[connection.TargetComponent].Inject(ctx, connection.TargetPort, value, virtualTime); err != nil {
							return bundle, err
						}
					}
				}
			}
			currentOutputs := map[string]float64{}
			maxDelta := 0.0
			for _, component := range components {
				if !componentDue(component, virtualTime, dirty[component.ID], experiment.MacroStepUS) {
					continue
				}
				stepUS := componentStep(component, experiment.MacroStepUS, experiment.DurationUS-virtualTime)
				if component.Rollback {
					checkpoint, err := adapters[component.ID].Snapshot(ctx, virtualTime)
					if err != nil {
						return bundle, fmt.Errorf("checkpoint %s at %dus: %w", component.ID, virtualTime, err)
					}
					if checkpoint == "" {
						return bundle, fmt.Errorf("checkpoint %s at %dus returned no artifact", component.ID, virtualTime)
					}
					bundle.Artifacts[fmt.Sprintf("checkpoint.%s.%d", component.ID, virtualTime)] = checkpoint
				}
				result, err := adapters[component.ID].Step(ctx, virtualTime, stepUS)
				if err != nil && component.Rollback {
					restorer := adapters[component.ID].(Restorer)
					checkpoint := bundle.Artifacts[fmt.Sprintf("checkpoint.%s.%d", component.ID, virtualTime)]
					if restoreErr := restorer.Restore(ctx, checkpoint); restoreErr != nil {
						return bundle, fmt.Errorf("restore %s at %dus: %w", component.ID, virtualTime, restoreErr)
					}
					sequence = appendEvents(&bundle.Events, []Event{{Type: "scheduler.rollback_retry", Payload: map[string]any{"checkpoint": checkpoint}, FidelityRef: component.ID + ":checkpoint"}}, sequence, virtualTime, component.ID)
					result, err = adapters[component.ID].Step(ctx, virtualTime, stepUS)
				}
				if err != nil {
					return bundle, fmt.Errorf("step %s at %dus: %w", component.ID, virtualTime, err)
				}
				dirty[component.ID] = false
				for _, name := range sortedFloatKeys(result.Metrics) {
					value := result.Metrics[name]
					if math.IsNaN(value) || math.IsInf(value, 0) {
						return bundle, fmt.Errorf("component %s produced invalid metric %s", component.ID, name)
					}
					key := component.ID + "." + name
					metrics[key] = value
					currentOutputs[key] = value
					if previous, ok := previousOutputs[key]; ok {
						maxDelta = math.Max(maxDelta, math.Abs(value-previous))
					} else if system.AlgebraicSolver != nil {
						maxDelta = math.Inf(1)
					}
					samples[key] = append(samples[key], metricSample{AtUS: virtualTime + stepUS, Value: value})
				}
				for _, name := range sortedFloatKeys(result.Outputs) {
					value := result.Outputs[name]
					if math.IsNaN(value) || math.IsInf(value, 0) {
						return bundle, fmt.Errorf("component %s produced invalid output %s", component.ID, name)
					}
					key := component.ID + "." + name
					metrics[key] = value
					currentOutputs[key] = value
					if previous, ok := previousOutputs[key]; ok {
						maxDelta = math.Max(maxDelta, math.Abs(value-previous))
					} else if system.AlgebraicSolver != nil {
						maxDelta = math.Inf(1)
					}
					if _, alreadySampled := result.Metrics[name]; !alreadySampled {
						samples[key] = append(samples[key], metricSample{AtUS: virtualTime + stepUS, Value: value})
					}
					for _, connection := range connections[component.ID+"."+name] {
						injected, err := adapters[connection.TargetComponent].Inject(ctx, connection.TargetPort, value, virtualTime)
						if err != nil {
							return bundle, fmt.Errorf("propagate %s.%s to %s.%s: %w", component.ID, name, connection.TargetComponent, connection.TargetPort, err)
						}
						sequence = appendEvents(&bundle.Events, []Event{{Type: "connection.propagated", Payload: map[string]any{"source": component.ID + "." + name, "target": connection.TargetComponent + "." + connection.TargetPort, "value": value, "unit": connection.Unit}, FidelityRef: component.ID + ":port"}}, sequence, virtualTime, component.ID)
						sequence = appendEvents(&bundle.Events, injected, sequence, virtualTime, connection.TargetComponent)
						dirty[connection.TargetComponent] = true
					}
				}
				for _, name := range sortedStringKeys(result.Artifacts) {
					hash := result.Artifacts[name]
					bundle.Artifacts[component.ID+"."+name] = hash
				}
				sequence = appendEvents(&bundle.Events, result.Events, sequence, virtualTime, component.ID)
			}
			if system.AlgebraicSolver != nil {
				sequence = appendEvents(&bundle.Events, []Event{{Type: "scheduler.algebraic_iteration", Payload: map[string]any{"iteration": iteration + 1, "max_delta": maxDelta}, FidelityRef: "scheduler:fixed-point"}}, sequence, virtualTime, "scheduler")
				if iteration > 0 && maxDelta <= system.AlgebraicSolver.Tolerance {
					converged = true
					break
				}
				previousOutputs = currentOutputs
			}
		}
		if !converged {
			return bundle, fmt.Errorf("algebraic loop did not converge within %d iterations", maxIterations)
		}
	}
	bundle.Assertions = evaluateAssertions(experiment.Assertions, metrics, samples, bundle.Events)
	return bundle, nil
}

func sortedFloatKeys(values map[string]float64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func classifyRunError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "nan") || strings.Contains(message, "inf") || strings.Contains(message, "invalid metric") || strings.Contains(message, "invalid output") {
		return "non_finite_value"
	}
	if strings.Contains(message, "backend") || strings.Contains(message, "step ") || strings.Contains(message, "prepare ") {
		return "backend_failure"
	}
	return "experiment_failure"
}

func Validate(experiment Experiment, system System) error {
	if experiment.APIVersion != APIVersion || system.APIVersion != APIVersion {
		return errors.New("unsupported AEL api version")
	}
	if experiment.SystemID == "" || experiment.SystemID != system.ID {
		return fmt.Errorf("experiment system_id %s does not match system %s", experiment.SystemID, system.ID)
	}
	if experiment.DurationUS <= 0 || experiment.MacroStepUS <= 0 || experiment.Timeout <= 0 {
		return errors.New("duration, macro step, and timeout must be positive")
	}
	for _, assertion := range experiment.Assertions {
		aggregation := assertion.Aggregation
		if aggregation == "" {
			aggregation = "final"
		}
		if aggregation != "final" && aggregation != "min" && aggregation != "max" && aggregation != "p95" && aggregation != "p99" && aggregation != "event_count" && aggregation != "event_before" && aggregation != "event_deadline" && aggregation != "event_duration" {
			return fmt.Errorf("assertion %s has unsupported aggregation %s", assertion.ID, aggregation)
		}
		if assertion.FromUS != nil && *assertion.FromUS < 0 || assertion.ToUS != nil && *assertion.ToUS > experiment.DurationUS || assertion.FromUS != nil && assertion.ToUS != nil && *assertion.FromUS > *assertion.ToUS {
			return fmt.Errorf("assertion %s has an invalid observation window", assertion.ID)
		}
	}
	ids := make(map[string]bool)
	ports := make(map[string]Port)
	for _, component := range system.Components {
		if component.ID == "" || component.Backend == "" || ids[component.ID] {
			return errors.New("component ids and backends must be unique and non-empty")
		}
		ids[component.ID] = true
		if component.Properties != nil {
			if _, declared := component.Properties["communication_step_us"]; declared {
				step := componentCommunicationStep(component)
				if step <= 0 {
					return fmt.Errorf("component %s communication_step_us must be positive", component.ID)
				}
			}
		}
		for _, port := range component.Ports {
			key := component.ID + "." + port.Name
			if port.Name == "" || ports[key].Name != "" {
				return fmt.Errorf("component %s has an empty or duplicate port", component.ID)
			}
			if port.Direction != "input" && port.Direction != "output" {
				return fmt.Errorf("port %s has unsupported direction %s", key, port.Direction)
			}
			ports[key] = port
		}
	}
	if system.AlgebraicSolver != nil {
		for _, component := range system.Components {
			if !component.Rollback {
				return fmt.Errorf("algebraic component %s must declare rollback", component.ID)
			}
		}
	}
	for _, connection := range system.Connections {
		if !ids[connection.SourceComponent] || !ids[connection.TargetComponent] {
			return errors.New("connection references unknown component")
		}
		source, sourceOK := ports[connection.SourceComponent+"."+connection.SourcePort]
		target, targetOK := ports[connection.TargetComponent+"."+connection.TargetPort]
		if !sourceOK || !targetOK {
			return fmt.Errorf("connection references unknown port %s.%s -> %s.%s", connection.SourceComponent, connection.SourcePort, connection.TargetComponent, connection.TargetPort)
		}
		if source.Direction != "output" || target.Direction != "input" {
			return fmt.Errorf("connection direction must be output to input for %s.%s", connection.SourceComponent, connection.SourcePort)
		}
		if source.Type != target.Type {
			return fmt.Errorf("connection type mismatch for %s.%s", connection.SourceComponent, connection.SourcePort)
		}
		if !compatiblePortUnits(system, connection) {
			return fmt.Errorf("connection unit mismatch for %s.%s", connection.SourceComponent, connection.SourcePort)
		}
	}
	if _, err := orderedComponents(system); err != nil {
		return err
	}
	return nil
}

func orderedComponents(system System) ([]Component, error) {
	byID := make(map[string]Component, len(system.Components))
	indegree := make(map[string]int, len(system.Components))
	edges := make(map[string][]string)
	for _, component := range system.Components {
		byID[component.ID] = component
		indegree[component.ID] = 0
	}
	seenEdges := make(map[string]bool)
	for _, connection := range system.Connections {
		key := connection.SourceComponent + "\x00" + connection.TargetComponent
		if connection.SourceComponent == connection.TargetComponent || seenEdges[key] {
			continue
		}
		seenEdges[key] = true
		edges[connection.SourceComponent] = append(edges[connection.SourceComponent], connection.TargetComponent)
		indegree[connection.TargetComponent]++
	}
	ready := make([]string, 0, len(indegree))
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	ordered := make([]Component, 0, len(system.Components))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, byID[id])
		sort.Strings(edges[id])
		for _, target := range edges[id] {
			indegree[target]--
			if indegree[target] == 0 {
				ready = append(ready, target)
				sort.Strings(ready)
			}
		}
	}
	if len(ordered) != len(system.Components) {
		if system.AlgebraicSolver == nil {
			return nil, errors.New("algebraic loop detected; configure a rollback-capable fixed-point solver or add an explicit delay")
		}
		if system.AlgebraicSolver.Method != "fixed_point" || system.AlgebraicSolver.Tolerance <= 0 || system.AlgebraicSolver.MaxIterations < 2 {
			return nil, errors.New("algebraic solver requires method=fixed_point, positive tolerance, and max_iterations >= 2")
		}
		ordered = ordered[:0]
		for _, component := range system.Components {
			if !component.Rollback {
				return nil, fmt.Errorf("algebraic component %s is not rollback-capable", component.ID)
			}
			ordered = append(ordered, component)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	}
	return ordered, nil
}

func connectionsBySource(connections []Connection) map[string][]Connection {
	result := make(map[string][]Connection)
	for _, connection := range connections {
		key := connection.SourceComponent + "." + connection.SourcePort
		result[key] = append(result[key], connection)
	}
	for key := range result {
		sort.Slice(result[key], func(i, j int) bool {
			left := result[key][i].TargetComponent + "." + result[key][i].TargetPort
			right := result[key][j].TargetComponent + "." + result[key][j].TargetPort
			return left < right
		})
	}
	return result
}

func communicationStep(experiment Experiment, components []Component) int64 {
	step := experiment.MacroStepUS
	for _, component := range components {
		if communication := componentCommunicationStep(component); communication > 0 {
			step = greatestCommonDivisor(step, communication)
		}
	}
	return step
}

func greatestCommonDivisor(left, right int64) int64 {
	for right != 0 {
		left, right = right, left%right
	}
	if left < 0 {
		return -left
	}
	return left
}

func componentDue(component Component, virtualTime int64, dirty bool, macroStepUS int64) bool {
	if component.EventDriven {
		return dirty
	}
	step := componentCommunicationStep(component)
	if step <= 0 {
		step = macroStepUS
	}
	return virtualTime%step == 0
}

func componentStep(component Component, macroStepUS, remainingUS int64) int64 {
	step := componentCommunicationStep(component)
	if step <= 0 {
		step = macroStepUS
	}
	return min(step, remainingUS)
}

func componentCommunicationStep(component Component) int64 {
	if component.Properties == nil {
		return 0
	}
	switch value := component.Properties["communication_step_us"].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}

func appendEvents(destination *[]Event, source []Event, sequence, virtualTime int64, componentID string) int64 {
	for _, event := range source {
		sequence++
		event.APIVersion = APIVersion
		event.Sequence = sequence
		if event.VirtualTimeUS == 0 {
			event.VirtualTimeUS = virtualTime
		}
		if event.Source == "" {
			event.Source = componentID
		}
		*destination = append(*destination, event)
	}
	return sequence
}

func evaluateAssertions(assertions []Assertion, metrics map[string]float64, samples map[string][]metricSample, events []Event) []AssertionResult {
	results := make([]AssertionResult, 0, len(assertions))
	for _, assertion := range assertions {
		aggregation := assertion.Aggregation
		if aggregation == "" {
			aggregation = "final"
		}
		observed, observedAt, ok := aggregateMetric(aggregation, assertion, metrics, samples)
		if strings.HasPrefix(aggregation, "event_") {
			observed, observedAt, ok = aggregateEvent(aggregation, assertion, events)
		}
		passed := ok
		switch assertion.Operator {
		case "<":
			passed = passed && observed < assertion.Expected
		case "<=":
			passed = passed && observed <= assertion.Expected
		case ">":
			passed = passed && observed > assertion.Expected
		case ">=":
			passed = passed && observed >= assertion.Expected
		case "==":
			passed = passed && observed == assertion.Expected
		default:
			passed = false
		}
		message := "passed"
		if !ok {
			message = "metric was not observed"
		} else if !passed {
			message = "assertion failed"
		}
		results = append(results, AssertionResult{ID: assertion.ID, Passed: passed, Observed: observed, Expected: assertion.Expected, Aggregation: aggregation, ObservedAtUS: observedAt, Message: message})
	}
	return results
}

func aggregateEvent(aggregation string, assertion Assertion, events []Event) (float64, int64, bool) {
	var matching, related []Event
	for _, event := range events {
		if event.Type == assertion.EventType {
			matching = append(matching, event)
		}
		if event.Type == assertion.RelatedEventType {
			related = append(related, event)
		}
	}
	switch aggregation {
	case "event_count":
		return float64(len(matching)), 0, true
	case "event_before":
		if len(matching) == 0 || len(related) == 0 {
			return 0, 0, false
		}
		value := 0.0
		if matching[0].VirtualTimeUS < related[0].VirtualTimeUS || matching[0].Sequence < related[0].Sequence {
			value = 1
		}
		return value, matching[0].VirtualTimeUS, true
	case "event_deadline":
		if len(matching) == 0 || assertion.DeadlineUS == nil {
			return 0, 0, false
		}
		value := 0.0
		if matching[0].VirtualTimeUS <= *assertion.DeadlineUS {
			value = 1
		}
		return value, matching[0].VirtualTimeUS, true
	case "event_duration":
		if len(matching) < 2 || assertion.MinimumDurationUS == nil {
			return 0, 0, false
		}
		duration := matching[len(matching)-1].VirtualTimeUS - matching[0].VirtualTimeUS
		return float64(duration), matching[len(matching)-1].VirtualTimeUS, true
	default:
		return 0, 0, false
	}
}

func aggregateMetric(aggregation string, assertion Assertion, metrics map[string]float64, samples map[string][]metricSample) (float64, int64, bool) {
	if aggregation == "final" {
		value, ok := metrics[assertion.Metric]
		values := samples[assertion.Metric]
		if len(values) > 0 {
			return value, values[len(values)-1].AtUS, ok
		}
		return value, 0, ok
	}
	filtered := make([]metricSample, 0, len(samples[assertion.Metric]))
	for _, sample := range samples[assertion.Metric] {
		if assertion.FromUS != nil && sample.AtUS < *assertion.FromUS {
			continue
		}
		if assertion.ToUS != nil && sample.AtUS > *assertion.ToUS {
			continue
		}
		filtered = append(filtered, sample)
	}
	if len(filtered) == 0 {
		return 0, 0, false
	}
	switch aggregation {
	case "min", "max":
		selected := filtered[0]
		for _, sample := range filtered[1:] {
			if aggregation == "min" && sample.Value < selected.Value || aggregation == "max" && sample.Value > selected.Value {
				selected = sample
			}
		}
		return selected.Value, selected.AtUS, true
	case "p95", "p99":
		ordered := append([]metricSample(nil), filtered...)
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Value < ordered[j].Value })
		percentile := 0.95
		if aggregation == "p99" {
			percentile = 0.99
		}
		index := int(math.Ceil(percentile*float64(len(ordered)))) - 1
		selected := ordered[max(0, index)]
		return selected.Value, selected.AtUS, true
	default:
		return 0, 0, false
	}
}

func splitTarget(target string) (string, string) {
	component, port, _ := strings.Cut(target, ".")
	return component, port
}

func aggregateFidelity(components []Component) Fidelity {
	result := Fidelity{Firmware: FidelityPhysical, Register: FidelityPhysical, Protocol: FidelityPhysical, Timing: FidelityPhysical, Physical: FidelityPhysical, HardwareValidated: true}
	for _, component := range components {
		result.Firmware = minimumFidelity(result.Firmware, component.Fidelity.Firmware)
		result.Register = minimumFidelity(result.Register, component.Fidelity.Register)
		result.Protocol = minimumFidelity(result.Protocol, component.Fidelity.Protocol)
		result.Timing = minimumFidelity(result.Timing, component.Fidelity.Timing)
		result.Physical = minimumFidelity(result.Physical, component.Fidelity.Physical)
		result.HardwareValidated = result.HardwareValidated && component.Fidelity.HardwareValidated
		result.Limitations = append(result.Limitations, component.Fidelity.Limitations...)
	}
	return result
}

func minimumFidelity(left, right FidelityLevel) FidelityLevel {
	rank := map[FidelityLevel]int{FidelityUnsupported: 0, FidelitySynthetic: 1, FidelityFunctional: 2, FidelityRegister: 3, FidelityTiming: 4, FidelityPhysical: 5}
	if rank[right] < rank[left] {
		return right
	}
	return left
}

func compatiblePortUnits(system System, connection Connection) bool {
	var source, target string
	for _, component := range system.Components {
		for _, port := range component.Ports {
			if component.ID == connection.SourceComponent && port.Name == connection.SourcePort {
				source = port.Unit
			}
			if component.ID == connection.TargetComponent && port.Name == connection.TargetPort {
				target = port.Unit
			}
		}
	}
	return source == target && (connection.Unit == "" || connection.Unit == source)
}
