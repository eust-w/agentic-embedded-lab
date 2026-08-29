package fmi

import (
	"context"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

type fakeAdapter struct{ input float64 }

func (f *fakeAdapter) Prepare(context.Context, ael.Component, int64) error { return nil }
func (f *fakeAdapter) Inject(_ context.Context, name string, value any, _ int64) ([]ael.Event, error) {
	if name == "input" {
		f.input = value.(float64)
	}
	return nil, nil
}
func (f *fakeAdapter) Step(context.Context, int64, int64) (ael.StepResult, error) {
	return ael.StepResult{Outputs: map[string]float64{"output": f.input * 2}}, nil
}
func (f *fakeAdapter) Snapshot(context.Context, int64) (string, error) { return "", nil }
func (f *fakeAdapter) Shutdown(context.Context) error                  { return nil }
func TestBridgeExchangesTypedValuesAndRejectsTimeRollback(t *testing.T) {
	adapter := &fakeAdapter{}
	bridge := Bridge{Instances: map[string]*Instance{"plant": {Adapter: adapter, Variables: map[uint32]Variable{1: {Reference: 1, Name: "input", Type: Real, Direction: "input"}, 2: {Reference: 2, Name: "output", Type: Real, Direction: "output"}}}}}
	response, err := bridge.Exchange(context.Background(), "STEP plant 0 0.001 r1=1.25")
	if err != nil || response != "OK r2=2.5" {
		t.Fatalf("unexpected response %q %v", response, err)
	}
	if _, err := bridge.Exchange(context.Background(), "STEP plant 0 0.001 r1=1"); err == nil {
		t.Fatal("time rollback was accepted")
	}
	if _, err := bridge.Exchange(context.Background(), "STEP plant 0.001 0.001 r99=1"); err == nil {
		t.Fatal("unknown value reference was accepted")
	}
}
