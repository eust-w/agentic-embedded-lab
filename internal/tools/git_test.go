package tools

import (
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

func TestGitWriteOperationsAlwaysRequestSensitiveApproval(t *testing.T) {
	tool := GitWriteTool{Workspace: "/tmp/project"}
	push := tool.Operation(map[string]any{"action": "push"})
	if push.Risk != protocol.RiskHigh || !push.Network || !push.External {
		t.Fatalf("push operation is not guarded: %#v", push)
	}
	restore := tool.Operation(map[string]any{"action": "restore"})
	if !restore.Destructive {
		t.Fatalf("restore operation is not destructive: %#v", restore)
	}
	pr := tool.Operation(map[string]any{"action": "pr_create"})
	if !pr.External || !pr.Network {
		t.Fatalf("pull request operation is not external: %#v", pr)
	}
}

func TestStringArgumentsRejectsMissingAndMalformedPaths(t *testing.T) {
	if _, err := stringArguments(nil); err == nil {
		t.Fatal("missing paths were accepted")
	}
	if _, err := stringArguments([]any{"ok", 4}); err == nil {
		t.Fatal("non-string path was accepted")
	}
	paths, err := stringArguments([]any{"a.c", "b.c"})
	if err != nil || len(paths) != 2 {
		t.Fatalf("valid paths: %#v %v", paths, err)
	}
}
