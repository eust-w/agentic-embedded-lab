package ael

import (
	"path/filepath"
	"testing"
)

func TestMigratedBenchmarkContractsAreStrictAndConsistent(t *testing.T) {
	workspace, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	systemFiles, err := filepath.Glob(filepath.Join(workspace, "benchmarks", "v2", "systems", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	experimentFiles, err := filepath.Glob(filepath.Join(workspace, "benchmarks", "v2", "experiments", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(systemFiles) != 27 || len(experimentFiles) != 48 {
		t.Fatalf("unexpected migrated asset count: systems=%d experiments=%d", len(systemFiles), len(experimentFiles))
	}
	systems := make(map[string]System)
	for _, path := range systemFiles {
		var system System
		if err := loadStrictWorkspaceYAML(workspace, path, &system); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		systems[system.ID] = system
	}
	for _, path := range experimentFiles {
		var experiment Experiment
		if err := loadStrictWorkspaceYAML(workspace, path, &experiment); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		system, ok := systems[experiment.SystemID]
		if !ok {
			t.Fatalf("%s references missing system %s", path, experiment.SystemID)
		}
		if err := Validate(experiment, system); err != nil {
			t.Fatalf("validate %s: %v", path, err)
		}
	}
}
