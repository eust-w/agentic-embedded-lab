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

func (s Scheduler) Run(ctx context.Context, experiment Experiment, system System, sourceRevision string) (EvidenceBundle, error) {
	if err := Validate(experiment, system); err != nil {
		return EvidenceBundle{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, experiment.Timeout)
	defer cancel()
	components := append([]Component(nil), system.Components...)
	sort.Slice(components, func(i, j int) bool { return components[i].ID < components[j].ID })
	adapters := make(map[string]Adapter, len(components))
	defer func() {
		for _, adapter := range adapters {
			_ = adapter.Shutdown(context.Background())
		}
	}()
	for _, component := range components {
		factory := s.Factories[component.Backend]
		if factory == nil {
			return EvidenceBundle{}, fmt.Errorf("backend %s is unavailable", component.Backend)
		}
		adapter, err := factory(component)
		if err != nil {
			return EvidenceBundle{}, err
		}
		if err := adapter.Prepare(ctx, component, experiment.Seed); err != nil {
			return EvidenceBundle{}, fmt.Errorf("prepare %s: %w", component.ID, err)
		}
		adapters[component.ID] = adapter
	}
	started := time.Now().UTC()
	bundle := EvidenceBundle{APIVersion: APIVersion, RunID: uuid.NewString(), Experiment: experiment, System: system, Artifacts: make(map[string]string), SourceRevision: sourceRevision, StartedAt: started, Fidelity: aggregateFidelity(components)}
	metrics := make(map[string]float64)
	var sequence int64
	for virtualTime := int64(0); virtualTime <= experiment.DurationUS; virtualTime += experiment.MacroStepUS {
		select {
		case <-ctx.Done():
			return bundle, ctx.Err()
		default:
		}
		for _, stimulus := range experiment.Stimuli {
			if stimulus.AtUS == virtualTime {
				componentID, target := splitTarget(stimulus.Target)
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
				injected, err := adapters[componentID].Inject(ctx, target, map[string]any{"kind": fault.Kind, "parameters": fault.Parameters}, virtualTime)
				if err != nil {
					return bundle, err
				}
				sequence = appendEvents(&bundle.Events, injected, sequence, virtualTime, componentID)
			}
		}
		for _, component := range components {
			result, err := adapters[component.ID].Step(ctx, virtualTime, experiment.MacroStepUS)
			if err != nil {
				return bundle, fmt.Errorf("step %s at %dus: %w", component.ID, virtualTime, err)
			}
			for name, value := range result.Metrics {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return bundle, fmt.Errorf("component %s produced invalid metric %s", component.ID, name)
				}
				metrics[component.ID+"."+name] = value
			}
			for name, hash := range result.Artifacts {
				bundle.Artifacts[component.ID+"."+name] = hash
			}
			sequence = appendEvents(&bundle.Events, result.Events, sequence, virtualTime, component.ID)
		}
	}
	bundle.Assertions = evaluateAssertions(experiment.Assertions, metrics)
	bundle.FinishedAt = time.Now().UTC()
	payload, _ := json.Marshal(bundle.Events)
	digest := sha256.Sum256(payload)
	bundle.TraceSHA256 = hex.EncodeToString(digest[:])
	return bundle, nil
}

func Validate(experiment Experiment, system System) error {
	if experiment.APIVersion != APIVersion || system.APIVersion != APIVersion {
		return errors.New("unsupported AEL api version")
	}
	if experiment.DurationUS <= 0 || experiment.MacroStepUS <= 0 || experiment.Timeout <= 0 {
		return errors.New("duration, macro step, and timeout must be positive")
	}
	ids := make(map[string]bool)
	for _, component := range system.Components {
		if component.ID == "" || component.Backend == "" || ids[component.ID] {
			return errors.New("component ids and backends must be unique and non-empty")
		}
		ids[component.ID] = true
	}
	for _, connection := range system.Connections {
		if !ids[connection.SourceComponent] || !ids[connection.TargetComponent] {
			return errors.New("connection references unknown component")
		}
		if connection.Unit != "" && !compatiblePortUnits(system, connection) {
			return fmt.Errorf("connection unit mismatch for %s.%s", connection.SourceComponent, connection.SourcePort)
		}
	}
	return nil
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
