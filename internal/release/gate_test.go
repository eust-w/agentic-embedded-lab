package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
