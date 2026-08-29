package ael

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictLoaderRejectsUnknownFieldsAndPathEscape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "experiment.yaml")
	if err := os.WriteFile(path, []byte("api_version: ael.dev/v2\nid: e\nunknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var experiment Experiment
	err := loadStrictWorkspaceYAML(root, "experiment.yaml", &experiment)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
	if _, err := resolveWorkspacePath(root, "../escape"); err == nil {
		t.Fatal("path escape was accepted")
	}
}

func TestValidateRequiresMatchingSystemID(t *testing.T) {
	system := System{APIVersion: APIVersion, ID: "system"}
	experiment := Experiment{APIVersion: APIVersion, ID: "experiment", SystemID: "other", DurationUS: 1, MacroStepUS: 1}
	if err := Validate(experiment, system); err == nil {
		t.Fatal("mismatched system id was accepted")
	}
}
