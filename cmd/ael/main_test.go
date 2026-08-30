package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFoundationReleaseCheck(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", ".", "release", "check", "--profile", "foundation", "--workspace", root)
	command.Dir = filepath.Join(root, "cmd", "ael")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("release check: %v\n%s", err, output)
	}
}
