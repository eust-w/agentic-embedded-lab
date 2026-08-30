package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReportBlocksMissingExtensionAndPreservesExternalBoundary(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"acceptance/v2/software.json", "acceptance/v2/simulation.json", "acceptance/v2/evidence/fmi-five-domain.json", "acceptance/v2/desktop-development.json"} {
		full := filepath.Join(root, path)
		_ = os.MkdirAll(filepath.Dir(full), 0o700)
		_ = os.WriteFile(full, []byte(`{"status":"passed"}`), 0o600)
	}
	report, err := Report(root, "abc")
	if err != nil || report["software_accepted"] != false || report["external_boundaries_preserved"] != true {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}
