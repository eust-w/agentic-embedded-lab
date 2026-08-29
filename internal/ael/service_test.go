package ael

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

func TestRunManagerPersistsFailureEvidenceWithoutBackendFallback(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	state, err := store.Open(ctx, filepath.Join(workspace, ".state"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	if err := os.WriteFile(filepath.Join(workspace, "model.repl"), []byte("using sysbus\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "system.yaml"), []byte(`api_version: ael.dev/v2
id: system
components:
  - id: mcu
    backend: renode
    model: model.repl
    step_us: 1000
    rollback: false
    event_driven: false
    ports: []
    properties: {}
    fidelity:
      firmware: functional
      register: synthetic
      protocol: functional
      timing: functional
      physical: unsupported
      hardware_validated: false
      limitations: [no hardware calibration]
connections: []
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "experiment.yaml"), []byte(`api_version: ael.dev/v2
id: experiment
system_id: system
duration_us: 1000
macro_step_us: 1000
seed: 1
timeout: 1000000000
stimuli: []
faults: []
assertions: []
required_fidelity:
  firmware: functional
  register: synthetic
  protocol: functional
  timing: functional
  physical: unsupported
  hardware_validated: false
  limitations: []
`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewRunManager(state, filepath.Join(workspace, "missing-ael-backend"))
	if err := manager.RegisterProject("project", workspace); err != nil {
		t.Fatal(err)
	}
	record, err := manager.Start(ctx, RunRequest{ProjectID: "project", ExperimentPath: "experiment.yaml", SystemPath: "system.yaml", SourceRevision: "test"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := manager.Get(ctx, record.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == RunFailed {
			if current.Bundle == nil || current.Bundle.Failure == nil || current.EvidencePath == "" {
				t.Fatalf("missing failure evidence: %#v", current)
			}
			if _, err := os.Stat(filepath.Join(current.EvidencePath, "summary.md")); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("run did not reach a terminal state")
}
