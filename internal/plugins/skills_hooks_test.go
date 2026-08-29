package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsDiscoverMetadataThenLoadInstructions(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "review")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: review\ndescription: Review code safely\n---\nRun tests."), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := DiscoverSkills([]string{root})
	if err != nil || len(metadata) != 1 || metadata[0].Name != "review" {
		t.Fatalf("unexpected metadata: %#v %v", metadata, err)
	}
	skill, err := LoadSkill(metadata[0].Path)
	if err != nil || skill.Instructions != "Run tests." {
		t.Fatalf("unexpected skill: %#v %v", skill, err)
	}
}

func TestHooksCanBlockToolUse(t *testing.T) {
	dispatcher := NewHookDispatcher()
	dispatcher.Register(HookPreToolUse, func(ctx context.Context, payload HookPayload) (HookResult, error) {
		return HookResult{Block: true, Reason: "secret detected"}, nil
	})
	results, err := dispatcher.Dispatch(context.Background(), HookPayload{Event: HookPreToolUse, ThreadID: "t"})
	if err != nil || len(results) != 1 || !results[0].Block {
		t.Fatalf("unexpected hook result: %#v %v", results, err)
	}
}
