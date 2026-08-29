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
