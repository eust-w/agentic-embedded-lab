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

func TestDeclarativeHookBlocksOnlyMatchingTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hook.json")
	if err := os.WriteFile(path, []byte(`{"event":"PreToolUse","tool":"command","block":true,"reason":"command disabled"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadHookConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := config.Handler()(context.Background(), HookPayload{Event: HookPreToolUse, Data: map[string]any{"tool": "file"}})
	if err != nil || allowed.Block {
		t.Fatalf("unmatched tool was blocked: %#v %v", allowed, err)
	}
	blocked, err := config.Handler()(context.Background(), HookPayload{Event: HookPreToolUse, Data: map[string]any{"tool": "command"}})
	if err != nil || !blocked.Block || blocked.Reason != "command disabled" {
		t.Fatalf("matching tool was not blocked: %#v %v", blocked, err)
	}
}
