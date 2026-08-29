package modeling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/agent"
)

func TestSVDAndSystemRDLImportGenerateValidatedRenodeModel(t *testing.T) {
	workspace, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		path, name string
		load       func(string, string) (IR, error)
	}{
		{filepath.Join(workspace, "examples", "models", "minimal.svd"), "minimal_svd", ImportSVD},
		{filepath.Join(workspace, "examples", "models", "minimal.rdl"), "minimal_rdl", ImportSystemRDL},
	} {
		ir, err := fixture.load(fixture.path, fixture.name)
		if err != nil {
			t.Fatalf("import %s: %v", fixture.path, err)
		}
		if err := ir.Validate(); err != nil {
			t.Fatal(err)
		}
		source, err := GenerateRenodeCSharp(ir, "Ael.Generated")
		if err != nil || !strings.Contains(source, "BasicDoubleWordPeripheral") || !strings.Contains(source, "WithFlag") {
			t.Fatalf("invalid generated source: %v\n%s", err, source)
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (fn roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestGroundedGenerationProducesAuditableReceipt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("CONTROL at offset zero has ENABLE bit zero read-write."), 0o600); err != nil {
		t.Fatal(err)
	}
	ir := IR{APIVersion: APIVersion, Kind: "HardwareBehaviorIR", Name: "grounded", BusWidth: 32, Size: 4, Registers: []Register{{Name: "CONTROL", Width: 32, Fields: []Field{{Name: "ENABLE", Width: 1, Access: "rw"}}}}, Timing: map[string]UnitValue{}, Grounding: map[string][]string{"/registers/0": {"source.txt#lines:1-1"}}}
	raw, _ := json.Marshal(ir)
	event, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": string(raw)})
	completed, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp-grounded"}})
	client := agent.NewResponsesClient(agent.StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: " + string(event) + "\n\ndata: " + string(completed) + "\n\n"))}, nil
	})}
	registry := Registry{Workspace: root}
	packageValue, err := registry.GenerateGrounded(context.Background(), GroundedRequest{APIVersion: APIVersion, ID: "grounded", Name: "grounded", Version: "1.0.0", Backend: ael.BackendRenode, Sources: []string{"source.txt"}, Model: "gpt-test", MaxAttempts: 1}, client)
	if err != nil {
		t.Fatal(err)
	}
	if packageValue.State != StateGenerated || packageValue.GenerationReceiptPath == "" {
		t.Fatalf("unexpected package: %#v", packageValue)
	}
	receipt, err := os.ReadFile(filepath.Join(root, packageValue.GenerationReceiptPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(receipt), "test") && strings.Contains(string(receipt), "API") {
		t.Fatal("receipt leaked an API key")
	}
}

func TestRegistryRequiresIndependentConformanceEvidence(t *testing.T) {
	root := t.TempDir()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "models", "minimal.svd"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "minimal.svd"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	registry := Registry{Workspace: root}
	packageValue, err := registry.Generate(GenerationRequest{APIVersion: APIVersion, ID: "demo", Name: "demo", Version: "1.0.0", Backend: ael.BackendRenode, SVD: "minimal.svd"})
	if err != nil {
		t.Fatal(err)
	}
	if packageValue.State != StateGenerated {
		t.Fatalf("unexpected state %s", packageValue.State)
	}
	if _, err := registry.Promote("demo", "1.0.0", StateStaticValidated, "agent", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Promote("demo", "1.0.0", StateConformanceValidated, "agent", nil); err == nil {
		t.Fatal("promotion without evidence succeeded")
	}
	digest := sha256.Sum256([]byte("independent"))
	evidence := &ConformanceEvidence{APIVersion: APIVersion, ModelID: "demo", Validator: "ael.conformance/v2", SourceIndependent: true, RegisterLayoutPassed: true, CompilePassed: true, DriverTestsPassed: true, PropertyTestsPassed: true, ReferenceTracePassed: true, SandboxNetwork: "none", SandboxReadOnly: true, ArtifactHashes: map[string]string{"trace": hex.EncodeToString(digest[:])}}
	validated, err := registry.Promote("demo", "1.0.0", StateConformanceValidated, "agent", evidence)
	if err != nil {
		t.Fatal(err)
	}
	if validated.State != StateConformanceValidated {
		t.Fatal("model was not promoted")
	}
	if _, err := registry.Promote("demo", "1.0.0", StateHardwareValidated, "agent", evidence); err == nil {
		t.Fatal("agent promoted hardware validation")
	}
}

func TestIRRejectsOverlappingFieldsAndAgentPromotion(t *testing.T) {
	ir := IR{APIVersion: APIVersion, Kind: "HardwareBehaviorIR", Name: "bad", Size: 4, BusWidth: 32, Registers: []Register{{Name: "CTRL", Width: 32, Fields: []Field{{Name: "A", LSB: 0, Width: 2, Access: "rw"}, {Name: "B", LSB: 1, Width: 1, Access: "rw"}}}}}
	if err := ir.Validate(); err == nil {
		t.Fatal("overlapping fields were accepted")
	}
	if CanTransition(StateConformanceValidated, StateHardwareValidated, "agent") {
		t.Fatal("agent promoted a model to hardware_validated")
	}
	if !CanTransition(StateConformanceValidated, StateHardwareValidated, "human") {
		t.Fatal("human promotion transition was rejected")
	}
}
