package workers

import (
	"bytes"
	"context"
	"encoding/json"
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
