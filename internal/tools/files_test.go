package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileToolWritesAtomicallyAndRejectsEscape(t *testing.T) {
	workspace := t.TempDir()
	tool := FileTool{Workspace: workspace}
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "write", "path": "src/main.go", "content": "package main\n"}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "src", "main.go"))
	if err != nil || string(content) != "package main\n" {
		t.Fatalf("unexpected file: %q %v", content, err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "read", "path": "../secret"}); err == nil {
		t.Fatal("expected path escape rejection")
	}
}

func TestRegistryRejectsDuplicateTools(t *testing.T) {
	registry := NewRegistry()
	tool := FileTool{Workspace: t.TempDir()}
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(tool); err == nil {
		t.Fatal("expected duplicate rejection")
	}
	if len(registry.Definitions()) != 1 {
		t.Fatal("definition was not registered")
	}
}

func TestFileReadReturnsNestedDirectoryInstructions(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "firmware"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("root rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "firmware", "AGENTS.override.md"), []byte("firmware rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "firmware", "main.c"), []byte("int main(void) {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (FileTool{Workspace: workspace}).Execute(context.Background(), map[string]any{"action": "read", "path": "firmware/main.c"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output["directory_instructions"] != "root rule\n\nfirmware rule" {
		t.Fatalf("nested instructions missing: %#v", result.Output)
	}
}
