package benchmark

import (
	"path/filepath"
	"testing"
)

func TestV2CatalogRejectsNoResultSelectorsAndHasAllMechanisms(t *testing.T) {
	workspace, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := Load(workspace, "benchmarks/v2/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if failures := catalog.Validate(workspace); len(failures) > 0 {
		for _, failure := range failures {
			t.Log(failure)
		}
		t.Fatalf("catalog has %d validation failures", len(failures))
	}
}
