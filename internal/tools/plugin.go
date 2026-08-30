package tools

import (
	"context"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type PluginProcessTool struct {
	PluginID   string
	Runtime    *plugins.ProcessRuntime
	Capability plugins.ProcessCapability
}

func (p PluginProcessTool) Definition() protocol.ToolDefinition {
	schema := p.Capability.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object", "additionalProperties": true}
	}
	return protocol.ToolDefinition{Type: "function", Name: publicToolName(p.PluginID + "." + p.Capability.Name), Description: p.Capability.Description, Parameters: schema}
}

func (p PluginProcessTool) Operation(map[string]any) approval.Operation {
	name := publicToolName(p.PluginID + "." + p.Capability.Name)
	return approval.Operation{Tool: name, Action: "plugin_call", Resource: name, Risk: protocol.RiskHigh}
}

func (p PluginProcessTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	value, err := p.Runtime.Invoke(ctx, p.Capability.Name, arguments)
	return Result{Output: value}, err
}
