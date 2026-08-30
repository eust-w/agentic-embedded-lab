package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

type fakeImplementation struct{}

func (fakeImplementation) Backend() ael.Backend                  { return ael.BackendRenode }
func (fakeImplementation) ExpectedVersion() string               { return "test" }
func (fakeImplementation) Commands() []string                    { return []string{"true"} }
func (fakeImplementation) VersionArguments() []string            { return []string{"--version"} }
func (fakeImplementation) Prepare(context.Context, *State) error { return nil }
func (fakeImplementation) Step(context.Context, *State, int64) (ael.StepResult, error) {
	return ael.StepResult{Metrics: map[string]float64{"failure": 0}, Events: []ael.Event{{Type: "fake.step"}}}, nil
}

func TestRenodeInitialisationAppliesValidatedPerformanceMIPS(t *testing.T) {
	state := &State{Component: ael.Component{Properties: map[string]any{"performance_mips": 320.0}}}
	lines, err := renodeInitialisation(state, "platform.repl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "cpu PerformanceInMips 320") {
		t.Fatalf("missing performance command: %v", lines)
	}
	state.Component.Properties["performance_mips"] = -1
	if _, err := renodeInitialisation(state, "platform.repl"); err == nil {
		t.Fatal("negative performance_mips must fail closed")
	}
}

func TestRenodeInitialisationCanPreserveFlashAcrossReset(t *testing.T) {
	root := t.TempDir()
	firmware := filepath.Join(root, "ota.hex")
	if err := os.WriteFile(firmware, []byte(":00000001FF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := &State{Workspace: root, Component: ael.Component{Properties: map[string]any{
		"firmware":                   "ota.hex",
		"preserve_firmware_on_reset": true,
	}}}
	lines, err := renodeInitialisation(state, "platform.repl")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Join(lines, "\n")
	if strings.Count(script, "sysbus LoadHEX") != 1 {
		t.Fatalf("persistent flash must only be loaded initially: %s", script)
	}
	if strings.Count(script, "cpu VectorTableOffset 0x08000000") != 2 {
		t.Fatalf("reset must retain vector table setup: %s", script)
	}
}

func TestRenodeInitialisationAddsTypedStopWatchpoint(t *testing.T) {
	state := &State{Component: ael.Component{Properties: map[string]any{
		"stop_register": float64(0x2001fc04),
		"stop_value":    float64(0xa17ead17),
	}}}
	lines, err := renodeInitialisation(state, "platform.repl")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Join(lines, "\n")
	if !strings.Contains(script, "AddWatchpointHook 0x2001fc04 DoubleWord Write") ||
		!strings.Contains(script, "value == 0xa17ead17") {
		t.Fatalf("missing typed stop watchpoint: %s", script)
	}
}

func TestServerRejectsVersionMismatchWithoutSilentFallback(t *testing.T) {
	server, err := NewServer(t.TempDir(), fakeImplementation{})
	if err != nil {
		t.Fatal(err)
	}
	server.state.Tool = "/usr/bin/true"
	server.state.Version = "wrong"
	component := ael.Component{ID: "mcu", Backend: ael.BackendRenode}
	componentPayload, _ := json.Marshal(component)
	var componentValue any
	_ = json.Unmarshal(componentPayload, &componentValue)
	requests := []ael.BackendRequest{
		{APIVersion: ael.BackendProtocolVersion, ID: "prepare", Operation: "prepare", Payload: map[string]any{"component": componentValue, "seed": float64(1)}},
		{APIVersion: ael.BackendProtocolVersion, ID: "shutdown", Operation: "shutdown"},
	}
	var input bytes.Buffer
	for _, request := range requests {
		_ = json.NewEncoder(&input).Encode(request)
	}
	var output bytes.Buffer
	if err := server.Serve(context.Background(), &input, &output); err != nil {
		t.Fatal(err)
	}
	var response ael.BackendResponse
	if err := json.NewDecoder(&output).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == "" {
		t.Fatalf("version mismatch was not explicit: %#v", response)
	}
}

func TestNgspiceMeasurementParserExtractsToolResults(t *testing.T) {
	metrics := parseNgspiceMeasures("ael_supply_voltage = 3.12e+00\nael_power = 1.8e-01\n")
	if metrics["supply_voltage"] != 3.12 || metrics["power"] != 0.18 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}
