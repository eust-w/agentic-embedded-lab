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

type Adapter interface {
	Prepare(context.Context, Component, int64) error
	Inject(context.Context, string, any, int64) ([]Event, error)
	Step(context.Context, int64, int64) (StepResult, error)
	Snapshot(context.Context, int64) (string, error)
	Shutdown(context.Context) error
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
	}
	metrics := make(map[string]float64)
	connections := connectionsBySource(system.Connections)
	var sequence int64
	for virtualTime := int64(0); virtualTime < experiment.DurationUS; virtualTime += experiment.MacroStepUS {
		stepUS := min(experiment.MacroStepUS, experiment.DurationUS-virtualTime)
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
			}
		}
		for _, component := range components {
			result, err := adapters[component.ID].Step(ctx, virtualTime, stepUS)
			if err != nil {
				return bundle, fmt.Errorf("step %s at %dus: %w", component.ID, virtualTime, err)
			}
			for name, value := range result.Metrics {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return bundle, fmt.Errorf("component %s produced invalid metric %s", component.ID, name)
				}
				metrics[component.ID+"."+name] = value
			}
			for name, value := range result.Outputs {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return bundle, fmt.Errorf("component %s produced invalid output %s", component.ID, name)
				}
				metrics[component.ID+"."+name] = value
				for _, connection := range connections[component.ID+"."+name] {
					injected, err := adapters[connection.TargetComponent].Inject(ctx, connection.TargetPort, value, virtualTime)
					if err != nil {
						return bundle, fmt.Errorf("propagate %s.%s to %s.%s: %w", component.ID, name, connection.TargetComponent, connection.TargetPort, err)
					}
					sequence = appendEvents(&bundle.Events, []Event{{Type: "connection.propagated", Payload: map[string]any{"source": component.ID + "." + name, "target": connection.TargetComponent + "." + connection.TargetPort, "value": value, "unit": connection.Unit}, FidelityRef: component.ID + ":port"}}, sequence, virtualTime, component.ID)
					sequence = appendEvents(&bundle.Events, injected, sequence, virtualTime, connection.TargetComponent)
				}
			}
			for name, hash := range result.Artifacts {
				bundle.Artifacts[component.ID+"."+name] = hash
			}
			sequence = appendEvents(&bundle.Events, result.Events, sequence, virtualTime, component.ID)
		}
	}
	bundle.Assertions = evaluateAssertions(experiment.Assertions, metrics)
	return bundle, nil
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
	ids := make(map[string]bool)
	ports := make(map[string]Port)
	for _, component := range system.Components {
		if component.ID == "" || component.Backend == "" || ids[component.ID] {
			return errors.New("component ids and backends must be unique and non-empty")
		}
		ids[component.ID] = true
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
		return nil, errors.New("algebraic loop detected; v2 requires an explicit delay or rollback-capable coordinator")
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

func evaluateAssertions(assertions []Assertion, metrics map[string]float64) []AssertionResult {
	results := make([]AssertionResult, 0, len(assertions))
	for _, assertion := range assertions {
		observed, ok := metrics[assertion.Metric]
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
		results = append(results, AssertionResult{ID: assertion.ID, Passed: passed, Observed: observed, Expected: assertion.Expected, Message: message})
	}
	return results
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
