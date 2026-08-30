package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

func TestProductionEvidenceRequiresTrustedValidSignature(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "lab"), 0o700)
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	keys, _ := json.Marshal(map[string]string{"reviewer": base64.StdEncoding.EncodeToString(public)})
	_ = os.WriteFile(filepath.Join(root, "lab", "trusted-reviewers.json"), keys, 0o600)
	envelope := ael.ValidationEnvelope{ID: "e", ModelID: "m", HardwareRevision: "r", EvidenceRunIDs: []string{"run"}, SignedBy: "reviewer", Conditions: map[string]string{}, Tolerances: map[string]float64{}}
	payload, _ := json.Marshal(envelope)
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))
	evidence, _ := json.Marshal(map[string]any{"hardware_validated": true, "human_approved": true, "envelope": envelope})
	path := filepath.Join(root, "evidence.json")
	_ = os.WriteFile(path, evidence, 0o600)
	if failures := validateProductionEvidence(root, path); len(failures) != 0 {
		t.Fatal(failures)
	}
	envelope.Signature = base64.StdEncoding.EncodeToString([]byte("invalid"))
	evidence, _ = json.Marshal(map[string]any{"hardware_validated": true, "human_approved": true, "envelope": envelope})
	_ = os.WriteFile(path, evidence, 0o600)
	if failures := validateProductionEvidence(root, path); len(failures) == 0 {
		t.Fatal("invalid signature passed")
	}
}
