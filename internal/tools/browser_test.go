package tools

import (
	"context"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

func TestBrowserAndComputerToolsRequireApprovalForMutation(t *testing.T) {
	browserTool := BrowserTool{}
	if operation := browserTool.Operation(map[string]any{"action": "click", "selector": "button.submit"}); operation.Risk != protocol.RiskHigh {
		t.Fatalf("browser click risk: %#v", operation)
	}
	if operation := browserTool.Operation(map[string]any{"action": "navigate", "url": "https://example.com"}); !operation.Network {
		t.Fatalf("browser navigation did not declare network: %#v", operation)
	}
	computerTool := ComputerTool{}
	if operation := computerTool.Operation(map[string]any{"action": "type", "bundle_id": "com.example.App"}); operation.Risk != protocol.RiskHigh {
		t.Fatalf("computer typing risk: %#v", operation)
	}
	if _, err := browserTool.Execute(context.Background(), map[string]any{"action": "dom"}); err == nil {
		t.Fatal("missing browser controller did not fail closed")
	}
	if _, err := computerTool.Execute(context.Background(), map[string]any{"action": "status", "bundle_id": "com.example.App"}); err == nil {
		t.Fatal("missing Computer Use controller did not fail closed")
	}
}

func TestRemoteToolNamesAreModelSafeAndCollisionResistant(t *testing.T) {
	left := publicToolName("server.tool")
	right := publicToolName("server__tool")
	if left == right || !functionName.MatchString(left) || !functionName.MatchString(right) {
		t.Fatalf("unsafe remote names: %q %q", left, right)
	}
}
