package approval

import (
	"path/filepath"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

func TestWorkspaceProfileBlocksEscapesAndAsksForNetwork(t *testing.T) {
	engine := New()
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "src", "main.go")
	if result := engine.Evaluate(protocol.PermissionWorkspace, workspace, Operation{
		Action: "write", Resource: inside, Risk: protocol.RiskMedium,
	}); result.Decision != DecisionAllow {
		t.Fatalf("expected allow, got %#v", result)
	}
	outside := filepath.Join(filepath.Dir(workspace), "secret")
	if result := engine.Evaluate(protocol.PermissionWorkspace, workspace, Operation{
		Action: "write", Resource: outside, Risk: protocol.RiskMedium,
	}); result.Decision != DecisionDeny {
		t.Fatalf("expected deny, got %#v", result)
	}
	if result := engine.Evaluate(protocol.PermissionWorkspace, workspace, Operation{
		Tool: "http", Action: "request", Resource: "https://example.com", Network: true,
	}); result.Decision != DecisionAsk {
		t.Fatalf("expected approval, got %#v", result)
	}
}

func TestDestructiveAlwaysAsks(t *testing.T) {
	result := New().Evaluate(protocol.PermissionFullAccess, t.TempDir(), Operation{
		Tool: "git", Action: "push", Resource: "origin", Destructive: true,
	})
	if result.Decision != DecisionAsk {
		t.Fatalf("expected approval, got %#v", result)
	}
}
