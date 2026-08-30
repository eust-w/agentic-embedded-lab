package ael

import (
	"context"
	"testing"
	"time"
)

type fakeAdapter struct {
	component Component
	inputs    map[string]any
	steps     []int64
	snapshots int
	restores  int
	failOnce  bool
}

func (f *fakeAdapter) Prepare(context.Context, Component, int64) error { return nil }
func (f *fakeAdapter) Inject(ctx context.Context, target string, value any, at int64) ([]Event, error) {
	if f.inputs == nil {
		f.inputs = make(map[string]any)
	}
	f.inputs[target] = value
	return []Event{{Type: "stimulus.injected", Payload: map[string]any{"target": target, "value": value}}}, nil
}
func (f *fakeAdapter) Step(ctx context.Context, at, step int64) (StepResult, error) {
	if f.failOnce {
		f.failOnce = false
		return StepResult{}, context.DeadlineExceeded
	}
	f.steps = append(f.steps, step)
	result := StepResult{Metrics: map[string]float64{"failure": 0}, Events: []Event{{Type: "component.stepped", FidelityRef: "functional", Payload: map[string]any{"step_us": step}}}}
	if f.component.ID == "source" {
		result.Outputs = map[string]float64{"voltage": 3.3}
	}
	if f.component.ID == "sink" {
		if value, ok := f.inputs["supply"].(float64); ok {
			result.Metrics["received"] = value
		}
	}
	return result, nil
}
func (f *fakeAdapter) Restore(context.Context, string) error { f.restores++; return nil }
func (f *fakeAdapter) Snapshot(context.Context, int64) (string, error) {
	f.snapshots++
	return "snapshot", nil
}
func (f *fakeAdapter) Shutdown(context.Context) error { return nil }

func TestSchedulerProducesDeterministicTraceAndExplicitFidelity(t *testing.T) {
	system := System{APIVersion: APIVersion, ID: "system", Components: []Component{{ID: "mcu", Backend: BackendRenode, StepUS: 1000, Fidelity: Fidelity{Firmware: FidelityFunctional, Register: FidelitySynthetic, Protocol: FidelityFunctional, Timing: FidelityFunctional, Physical: FidelityUnsupported, Limitations: []string{"no hardware calibration"}}}}}
	experiment := Experiment{APIVersion: APIVersion, ID: "experiment", SystemID: system.ID, DurationUS: 2000, MacroStepUS: 1000, Seed: 7, Timeout: time.Second, Stimuli: []Stimulus{{AtUS: 1000, Target: "mcu.temperature", Value: 85}}, Assertions: []Assertion{{ID: "A01", Metric: "mcu.failure", Operator: "==", Expected: 0}}}
	scheduler := Scheduler{Factories: map[Backend]AdapterFactory{BackendRenode: func(component Component) (Adapter, error) { return &fakeAdapter{component: component}, nil }}}
	first, err := scheduler.Run(context.Background(), experiment, system, "revision")
	if err != nil {
		t.Fatal(err)
	}
	second, err := scheduler.Run(context.Background(), experiment, system, "revision")
	if err != nil {
		t.Fatal(err)
	}
	if first.TraceSHA256 != second.TraceSHA256 || len(first.Events) != 3 {
		t.Fatalf("trace is not deterministic: %s %s %#v", first.TraceSHA256, second.TraceSHA256, first.Events)
	}
	if !first.Assertions[0].Passed || first.Fidelity.HardwareValidated || first.Fidelity.Physical != FidelityUnsupported {
		t.Fatalf("unexpected evidence boundary: %#v", first)
	}
}

func TestValidateRejectsUnitMismatch(t *testing.T) {
	system := System{APIVersion: APIVersion, ID: "s", Components: []Component{
		{ID: "a", Backend: BackendNgspice, Ports: []Port{{Name: "out", Direction: "output", Type: "real", Unit: "V"}}},
		{ID: "b", Backend: BackendRenode, Ports: []Port{{Name: "in", Direction: "input", Type: "real", Unit: "A"}}},
	}, Connections: []Connection{{SourceComponent: "a", SourcePort: "out", TargetComponent: "b", TargetPort: "in", Unit: "V"}}}
	experiment := Experiment{APIVersion: APIVersion, ID: "e", SystemID: system.ID, DurationUS: 1000, MacroStepUS: 1000, Timeout: time.Second}
	if err := Validate(experiment, system); err == nil {
		t.Fatal("expected unit mismatch")
	}
}

func TestSchedulerPropagatesPortsBeforeDownstreamStep(t *testing.T) {
	source := &fakeAdapter{component: Component{ID: "source"}}
	sink := &fakeAdapter{component: Component{ID: "sink"}}
	system := System{APIVersion: APIVersion, ID: "connected", Components: []Component{
		{ID: "sink", Backend: BackendModelica, Ports: []Port{{Name: "supply", Direction: "input", Type: "real", Unit: "V"}}},
		{ID: "source", Backend: BackendNgspice, Ports: []Port{{Name: "voltage", Direction: "output", Type: "real", Unit: "V"}}},
	}, Connections: []Connection{{SourceComponent: "source", SourcePort: "voltage", TargetComponent: "sink", TargetPort: "supply", Unit: "V"}}}
	experiment := Experiment{APIVersion: APIVersion, ID: "propagation", SystemID: system.ID, DurationUS: 1000, MacroStepUS: 1000, Timeout: time.Second, Assertions: []Assertion{{ID: "A01", Metric: "sink.received", Operator: "==", Expected: 3.3}}}
	scheduler := Scheduler{Factories: map[Backend]AdapterFactory{
		BackendNgspice:  func(Component) (Adapter, error) { return source, nil },
		BackendModelica: func(Component) (Adapter, error) { return sink, nil },
	}}
	bundle, err := scheduler.Run(context.Background(), experiment, system, "revision")
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Assertions[0].Passed || sink.inputs["supply"] != 3.3 {
		t.Fatalf("port value was not propagated: %#v %#v", bundle.Assertions, sink.inputs)
	}
}

func TestValidateRejectsAlgebraicLoop(t *testing.T) {
	system := System{APIVersion: APIVersion, ID: "loop", Components: []Component{
		{ID: "a", Backend: BackendNgspice, Ports: []Port{{Name: "in", Direction: "input", Type: "real", Unit: "V"}, {Name: "out", Direction: "output", Type: "real", Unit: "V"}}},
		{ID: "b", Backend: BackendModelica, Ports: []Port{{Name: "in", Direction: "input", Type: "real", Unit: "V"}, {Name: "out", Direction: "output", Type: "real", Unit: "V"}}},
	}, Connections: []Connection{
		{SourceComponent: "a", SourcePort: "out", TargetComponent: "b", TargetPort: "in", Unit: "V"},
		{SourceComponent: "b", SourcePort: "out", TargetComponent: "a", TargetPort: "in", Unit: "V"},
	}}
	experiment := Experiment{APIVersion: APIVersion, ID: "e", SystemID: system.ID, DurationUS: 1000, MacroStepUS: 1000, Timeout: time.Second}
	if err := Validate(experiment, system); err == nil {
		t.Fatal("expected algebraic loop rejection")
	}
}

func TestSchedulerHonorsMultiRateEventDrivenAndCheckpoints(t *testing.T) {
	fast := &fakeAdapter{component: Component{ID: "fast"}}
	slow := &fakeAdapter{component: Component{ID: "slow"}}
	event := &fakeAdapter{component: Component{ID: "event"}}
	system := System{APIVersion: APIVersion, ID: "multi-rate", Components: []Component{
		{ID: "fast", Backend: BackendRenode, StepUS: 500, Properties: map[string]any{"communication_step_us": float64(500)}},
		{ID: "slow", Backend: BackendModelica, StepUS: 2000, Rollback: true, Properties: map[string]any{"communication_step_us": float64(2000)}},
		{ID: "event", Backend: BackendOpenEMS, EventDriven: true},
	}}
	experiment := Experiment{APIVersion: APIVersion, ID: "multi", SystemID: system.ID, DurationUS: 4000, MacroStepUS: 1000, Timeout: time.Second, Stimuli: []Stimulus{{AtUS: 2000, Target: "event.detune", Value: 1}}}
	scheduler := Scheduler{Factories: map[Backend]AdapterFactory{
		BackendRenode:   func(Component) (Adapter, error) { return fast, nil },
		BackendModelica: func(Component) (Adapter, error) { return slow, nil },
		BackendOpenEMS:  func(Component) (Adapter, error) { return event, nil },
	}}
	bundle, err := scheduler.Run(context.Background(), experiment, system, "revision")
	if err != nil {
		t.Fatal(err)
	}
	if len(fast.steps) != 8 || len(slow.steps) != 2 || len(event.steps) != 2 {
		t.Fatalf("unexpected multi-rate steps fast=%v slow=%v event=%v", fast.steps, slow.steps, event.steps)
	}
	if slow.snapshots != 2 || bundle.Artifacts["checkpoint.slow.0"] == "" || bundle.Artifacts["checkpoint.slow.2000"] == "" {
		t.Fatalf("missing synchronized checkpoints: %#v", bundle.Artifacts)
	}
}

func TestTemporalAssertionsUseWindowedMaxAndPercentile(t *testing.T) {
	from, to := int64(1000), int64(3000)
	metrics := map[string]float64{"m.current": 1}
	samples := map[string][]metricSample{"m.current": {{AtUS: 500, Value: 100}, {AtUS: 1000, Value: 2}, {AtUS: 2000, Value: 4}, {AtUS: 3000, Value: 3}}}
	results := evaluateAssertions([]Assertion{
		{ID: "max", Metric: "m.current", Operator: "==", Expected: 4, Aggregation: "max", FromUS: &from, ToUS: &to},
		{ID: "p95", Metric: "m.current", Operator: "==", Expected: 4, Aggregation: "p95", FromUS: &from, ToUS: &to},
	}, metrics, samples, nil)
	if !results[0].Passed || !results[1].Passed || results[0].ObservedAtUS != 2000 {
		t.Fatalf("unexpected temporal assertions: %#v", results)
	}
}

func TestInternalStepHintDoesNotCreateExternalCommunicationPoints(t *testing.T) {
	adapter := &fakeAdapter{component: Component{ID: "mcu"}}
	system := System{APIVersion: APIVersion, ID: "internal-step", Components: []Component{{ID: "mcu", Backend: BackendRenode, StepUS: 1000}}}
	experiment := Experiment{APIVersion: APIVersion, ID: "long", SystemID: system.ID, DurationUS: 120_000_000, MacroStepUS: 120_000_000, Timeout: time.Second}
	bundle, err := (Scheduler{Factories: map[Backend]AdapterFactory{BackendRenode: func(Component) (Adapter, error) { return adapter, nil }}}).Run(context.Background(), experiment, system, "revision")
	if err != nil {
		t.Fatal(err)
	}
	if len(adapter.steps) != 1 || adapter.steps[0] != experiment.DurationUS || len(bundle.Events) != 1 {
		t.Fatalf("internal step hint escaped into coordinator schedule: steps=%v events=%d", adapter.steps, len(bundle.Events))
	}
}

type unorderedOutputAdapter struct{}

func (*unorderedOutputAdapter) Prepare(context.Context, Component, int64) error { return nil }
func (*unorderedOutputAdapter) Inject(context.Context, string, any, int64) ([]Event, error) {
	return nil, nil
}
func (*unorderedOutputAdapter) Step(context.Context, int64, int64) (StepResult, error) {
	return StepResult{Outputs: map[string]float64{"z": 3, "a": 1, "m": 2}}, nil
}
func (*unorderedOutputAdapter) Snapshot(context.Context, int64) (string, error) {
	return "snapshot", nil
}
func (*unorderedOutputAdapter) Shutdown(context.Context) error { return nil }

func TestTraceOrderingDoesNotDependOnGoMapIteration(t *testing.T) {
	system := System{APIVersion: APIVersion, ID: "ordered", Components: []Component{
		{ID: "source", Backend: BackendNgspice, Ports: []Port{{Name: "a", Direction: "output", Type: "real", Unit: "V"}, {Name: "m", Direction: "output", Type: "real", Unit: "V"}, {Name: "z", Direction: "output", Type: "real", Unit: "V"}}},
		{ID: "sink", Backend: BackendModelica, Ports: []Port{{Name: "a", Direction: "input", Type: "real", Unit: "V"}, {Name: "m", Direction: "input", Type: "real", Unit: "V"}, {Name: "z", Direction: "input", Type: "real", Unit: "V"}}},
	}, Connections: []Connection{{SourceComponent: "source", SourcePort: "z", TargetComponent: "sink", TargetPort: "z", Unit: "V"}, {SourceComponent: "source", SourcePort: "a", TargetComponent: "sink", TargetPort: "a", Unit: "V"}, {SourceComponent: "source", SourcePort: "m", TargetComponent: "sink", TargetPort: "m", Unit: "V"}}}
	experiment := Experiment{APIVersion: APIVersion, ID: "ordered", SystemID: system.ID, DurationUS: 1000, MacroStepUS: 1000, Timeout: time.Second}
	scheduler := Scheduler{Factories: map[Backend]AdapterFactory{BackendNgspice: func(Component) (Adapter, error) { return &unorderedOutputAdapter{}, nil }, BackendModelica: func(component Component) (Adapter, error) { return &fakeAdapter{component: component}, nil }}}
	var expected string
	for repeat := 0; repeat < 50; repeat++ {
		bundle, err := scheduler.Run(context.Background(), experiment, system, "revision")
		if err != nil {
			t.Fatal(err)
		}
		if repeat == 0 {
			expected = bundle.TraceSHA256
		} else if bundle.TraceSHA256 != expected {
			t.Fatalf("trace order changed: %s != %s", bundle.TraceSHA256, expected)
		}
	}
}

func TestRollbackRestoresAndRetriesExactlyOnce(t *testing.T) {
	adapter := &fakeAdapter{component: Component{ID: "plant"}, failOnce: true}
	system := System{APIVersion: APIVersion, ID: "rollback", Components: []Component{{ID: "plant", Backend: BackendModelica, Rollback: true}}}
	experiment := Experiment{APIVersion: APIVersion, ID: "rollback", SystemID: system.ID, DurationUS: 1000, MacroStepUS: 1000, Timeout: time.Second}
	bundle, err := (Scheduler{Factories: map[Backend]AdapterFactory{BackendModelica: func(Component) (Adapter, error) { return adapter, nil }}}).Run(context.Background(), experiment, system, "revision")
	if err != nil || adapter.restores != 1 || len(adapter.steps) != 1 {
		t.Fatalf("rollback bundle=%#v restores=%d steps=%v err=%v", bundle, adapter.restores, adapter.steps, err)
	}
	found := false
	for _, event := range bundle.Events {
		found = found || event.Type == "scheduler.rollback_retry"
	}
	if !found {
		t.Fatal("rollback event missing")
	}
}

func TestEventAssertionsCoverCountOrderDeadlineAndDuration(t *testing.T) {
	deadline, duration := int64(1500), int64(1000)
	events := []Event{{Sequence: 1, VirtualTimeUS: 500, Type: "start"}, {Sequence: 2, VirtualTimeUS: 1000, Type: "tick"}, {Sequence: 3, VirtualTimeUS: 2000, Type: "tick"}, {Sequence: 4, VirtualTimeUS: 2500, Type: "stop"}}
	results := evaluateAssertions([]Assertion{
		{ID: "count", Aggregation: "event_count", EventType: "tick", Operator: "==", Expected: 2},
		{ID: "order", Aggregation: "event_before", EventType: "start", RelatedEventType: "stop", Operator: "==", Expected: 1},
		{ID: "deadline", Aggregation: "event_deadline", EventType: "start", DeadlineUS: &deadline, Operator: "==", Expected: 1},
		{ID: "duration", Aggregation: "event_duration", EventType: "tick", MinimumDurationUS: &duration, Operator: ">=", Expected: 1000},
	}, nil, nil, events)
	for _, result := range results {
		if !result.Passed {
			t.Fatalf("event assertion failed: %#v", results)
		}
	}
}

type algebraicAdapter struct{ input float64 }

func (*algebraicAdapter) Prepare(context.Context, Component, int64) error { return nil }
func (a *algebraicAdapter) Inject(_ context.Context, _ string, value any, _ int64) ([]Event, error) {
	a.input = value.(float64)
	return nil, nil
}
func (a *algebraicAdapter) Step(context.Context, int64, int64) (StepResult, error) {
	return StepResult{Outputs: map[string]float64{"out": (a.input + 1) / 2}}, nil
}
func (*algebraicAdapter) Snapshot(context.Context, int64) (string, error) { return "checkpoint", nil }
func (*algebraicAdapter) Restore(context.Context, string) error           { return nil }
func (*algebraicAdapter) Shutdown(context.Context) error                  { return nil }

func TestRollbackFixedPointAlgebraicLoopConverges(t *testing.T) {
	a, b := &algebraicAdapter{}, &algebraicAdapter{}
	system := System{APIVersion: APIVersion, ID: "loop", AlgebraicSolver: &AlgebraicSolver{Method: "fixed_point", Tolerance: 0.001, MaxIterations: 20}, Components: []Component{
		{ID: "a", Backend: BackendNgspice, Rollback: true, Ports: []Port{{Name: "in", Direction: "input", Type: "real", Unit: "1"}, {Name: "out", Direction: "output", Type: "real", Unit: "1"}}},
		{ID: "b", Backend: BackendModelica, Rollback: true, Ports: []Port{{Name: "in", Direction: "input", Type: "real", Unit: "1"}, {Name: "out", Direction: "output", Type: "real", Unit: "1"}}},
	}, Connections: []Connection{{SourceComponent: "a", SourcePort: "out", TargetComponent: "b", TargetPort: "in", Unit: "1"}, {SourceComponent: "b", SourcePort: "out", TargetComponent: "a", TargetPort: "in", Unit: "1"}}}
	experiment := Experiment{APIVersion: APIVersion, ID: "loop", SystemID: system.ID, DurationUS: 1000, MacroStepUS: 1000, Timeout: time.Second, Assertions: []Assertion{{ID: "fixed", Metric: "a.out", Operator: ">", Expected: 0.99}}}
	bundle, err := (Scheduler{Factories: map[Backend]AdapterFactory{BackendNgspice: func(Component) (Adapter, error) { return a, nil }, BackendModelica: func(Component) (Adapter, error) { return b, nil }}}).Run(context.Background(), experiment, system, "revision")
	if err != nil || !bundle.Assertions[0].Passed {
		t.Fatalf("algebraic result=%#v err=%v", bundle.Assertions, err)
	}
}
