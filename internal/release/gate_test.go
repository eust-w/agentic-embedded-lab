package release

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentTreePassesFoundationButRejectsStaleSimulationEvidence(t *testing.T) {
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
	if simulation.Passed {
		t.Fatal("stale or absent v2 simulation evidence was accepted")
	}
	joined := strings.Join(simulation.Failures, "\n")
	if !strings.Contains(joined, "simulation evidence") {
		t.Fatalf("unexpected failures: %s", joined)
	}
}
