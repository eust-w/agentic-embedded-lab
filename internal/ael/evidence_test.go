package ael

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvidenceWriterCreatesAtomicTruthfulBundle(t *testing.T) {
	root := t.TempDir()
	bundle := EvidenceBundle{
		APIVersion: APIVersion,
		RunID:      "run-001",
		Experiment: Experiment{APIVersion: APIVersion, ID: "experiment"},
		System:     System{APIVersion: APIVersion, ID: "system"},
		Events:     []Event{{APIVersion: APIVersion, Sequence: 1, Type: "step"}},
		Assertions: []AssertionResult{{ID: "A01", Passed: true, Message: "passed"}},
		Artifacts:  map[string]string{"trace": "sha256:abc"},
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
		Fidelity:   Fidelity{Firmware: FidelityFunctional, Physical: FidelityUnsupported},
	}
	path, err := (EvidenceWriter{Workspace: root}).Write(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"system.resolved.json", "experiment.resolved.json", "provenance.json", "events.jsonl", "assertions.json", "artifacts.json", "junit.xml", "summary.md"} {
		if _, err := os.Stat(filepath.Join(path, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	summary, _ := os.ReadFile(filepath.Join(path, "summary.md"))
	if !strings.Contains(string(summary), "未证明真实硬件行为") {
		t.Fatal("summary omitted the hardware boundary")
	}
	if _, err := (EvidenceWriter{Workspace: root}).Write(bundle); err == nil {
		t.Fatal("writer silently overwrote an evidence bundle")
	}
}
