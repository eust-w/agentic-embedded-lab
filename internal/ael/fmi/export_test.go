package fmi

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

func TestExportSSPPackagesTypedTopologyDeterministically(t *testing.T) {
	root := t.TempDir()
	fmu := filepath.Join(root, "source.fmu")
	if err := os.WriteFile(fmu, []byte("fmu"), 0o600); err != nil {
		t.Fatal(err)
	}
	system := ael.System{ID: "five-domain", Components: []ael.Component{{ID: "source", Backend: ael.BackendNgspice, StepUS: 500, Ports: []ael.Port{{Name: "voltage", Direction: "output", Type: "real", Unit: "V"}}}}}
	first, second := filepath.Join(root, "first.ssp"), filepath.Join(root, "second.ssp")
	for _, path := range []string{first, second} {
		if err := ExportSSP(system, map[string]string{"source": fmu}, path); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSSP(path); err != nil {
			t.Fatal(err)
		}
	}
	left, _ := os.ReadFile(first)
	right, _ := os.ReadFile(second)
	if string(left) != string(right) {
		t.Fatal("SSP export is not deterministic")
	}
	archive, err := zip.OpenReader(first)
	if err != nil || len(archive.File) != 2 {
		t.Fatalf("unexpected SSP archive: %v %v", archive, err)
	}
	archive.Close()
}
