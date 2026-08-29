package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLayersGlobalRootAndNestedInstructions(t *testing.T) {
	global := filepath.Join(t.TempDir(), "global")
	root := filepath.Join(t.TempDir(), "repo")
	nested := filepath.Join(root, "firmware", "drivers")
	for _, directory := range []string{global, nested} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(global, "AGENTS.md"), "global")
	write(t, filepath.Join(root, "AGENTS.md"), "root")
	write(t, filepath.Join(root, "firmware", "AGENTS.override.md"), "firmware override")
	write(t, filepath.Join(nested, "AGENTS.md"), "drivers")
	result, err := Discover(global, root, nested, 32<<10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "global\n\nroot\n\nfirmware override\n\ndrivers" {
		t.Fatalf("unexpected instruction order: %q", result.Content)
	}
	if len(result.Sources) != 4 || result.Limited {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDiscoverRejectsEscapeAndHonorsLimit(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "AGENTS.md"), strings.Repeat("x", 100))
	if _, err := Discover("", root, filepath.Dir(root), 10); err == nil {
		t.Fatal("expected outside directory to fail")
	}
	result, err := Discover("", root, root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Limited || result.Bytes != 10 {
		t.Fatalf("expected limited instructions: %#v", result)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
