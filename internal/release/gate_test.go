package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeterminismEvidenceRequiresAssertionHashesAndStressRuns(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "matrix.json")
	payload, _ := json.Marshal(map[string]any{"source_revision": "rev", "benchmark_count": 24, "base_repeats": 2, "stress_repeats": 20, "all_equal": true, "matrix": map[string]any{}})
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	failures := validateDeterminismRuns(root, path, "rev")
	if len(failures) == 0 || !strings.Contains(failures[0], "incomplete") {
		t.Fatalf("incomplete determinism evidence was accepted: %#v", failures)
	}
}

func TestExtensionEvidenceRequiresTwentyResultStableRuns(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "extension.json")
	payload, _ := json.Marshal(map[string]any{"source_revision": "rev", "checks": []any{}, "determinism": map[string]any{"repeats": 2, "trace_hashes": []string{"a", "a"}, "assertion_hashes": []string{"b", "b"}, "run_paths": []string{"runs/1", "runs/2"}, "all_equal": true}})
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	failures := validateExtensionRuns(root, path, "rev")
	if len(failures) == 0 || !strings.Contains(failures[0], "incomplete") {
		t.Fatalf("short extension determinism evidence was accepted: %#v", failures)
	}
}

func TestCurrentTreePassesFoundationAndValidatesAvailableSimulationEvidence(t *testing.T) {
	workspace, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	foundation, err := Check(workspace, Foundation)
	if err != nil || !foundation.Passed {
		t.Fatalf("foundation: %#v %v", foundation, err)
	}
	simulation, err := Check(workspace, Simulation)
	if err != nil {
		t.Fatal(err)
	}
	_, statErr := os.Stat(filepath.Join(workspace, "acceptance", "v2", "simulation.json"))
	if statErr == nil {
		if !simulation.Passed {
			t.Fatalf("fresh simulation evidence was rejected: %s", strings.Join(simulation.Failures, "\n"))
		}
	} else {
		if simulation.Passed || !strings.Contains(strings.Join(simulation.Failures, "\n"), "simulation evidence") {
			t.Fatalf("absent simulation evidence was not rejected: %#v", simulation)
		}
	}
}

func TestCurrentTreeValidatesSoftwareEvidenceAndRejectsMissingProductionEvidence(t *testing.T) {
	workspace, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "acceptance", "v2", "software.json")); err == nil {
		software, err := Check(workspace, Software)
		if err != nil || !software.Passed {
			t.Fatalf("software gate: %#v %v", software, err)
		}
	}
	production, err := Check(workspace, Production)
	if err != nil {
		t.Fatal(err)
	}
	if production.Passed {
		t.Fatal("production gate passed without hardware and calibration evidence")
	}
	joined := strings.Join(production.Failures, "\n")
	if !strings.Contains(joined, "production evidence") && !strings.Contains(joined, "hardware:") {
		t.Fatalf("production failure did not preserve the hardware boundary: %s", joined)
	}
}
