package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/mcp"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type MCPProxyTool struct {
	Client *mcp.Client
	Remote mcp.Tool
}

func (m MCPProxyTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: publicToolName(m.Remote.Name), Description: m.Remote.Description, Parameters: m.Remote.InputSchema}
}

func (m MCPProxyTool) Operation(map[string]any) approval.Operation {
	return approval.Operation{Tool: publicToolName(m.Remote.Name), Action: "mcp_call", Resource: m.Remote.Name, Risk: protocol.RiskHigh, Network: true}
}

var invalidFunctionName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func publicToolName(value string) string {
	digest := sha256.Sum256([]byte(value))
	suffix := "_" + hex.EncodeToString(digest[:4])
	name := invalidFunctionName.ReplaceAllString(value, "_")
	limit := 64 - len(suffix)
	if len(name) > limit {
		name = name[:limit]
	}
	return name + suffix
}

func (m MCPProxyTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	value, err := m.Client.CallTool(ctx, m.Remote.Name, arguments)
	return Result{Output: value}, err
}
