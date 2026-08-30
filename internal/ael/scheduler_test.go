package ael

import (
	"context"
	"testing"
	"time"
)

type fakeAdapter struct {
	component Component
	inputs    map[string]any
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
func (f *fakeAdapter) Snapshot(context.Context, int64) (string, error) { return "snapshot", nil }
func (f *fakeAdapter) Shutdown(context.Context) error                  { return nil }

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
