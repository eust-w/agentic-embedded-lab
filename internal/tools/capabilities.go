package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	capability "github.com/eust-w/agentic-embedded-lab/internal/acceptance"
	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	gitrepo "github.com/eust-w/agentic-embedded-lab/internal/git"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type CapabilityTool struct {
	Workspace string
	Mode      string
}

func (c CapabilityTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: c.Mode, Description: "Read the immutable acceptance state for frozen Aether/AEL capabilities", Parameters: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": func() []string {
		if c.Mode == "inspect_capabilities" {
			return nil
		}
		return []string{"id"}
	}()}}
}
func (c CapabilityTool) Operation(map[string]any) approval.Operation {
	return approval.Operation{Tool: c.Mode, Action: "inspect", Resource: c.Workspace, Risk: protocol.RiskLow}
}
func (c CapabilityTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	repository, err := gitrepo.Discover(ctx, c.Workspace)
	if err != nil {
		return Result{}, err
	}
	revision, err := repository.Head(ctx)
	if err != nil {
		return Result{}, err
	}
	id, _ := arguments["id"].(string)
	switch c.Mode {
	case "inspect_capabilities":
		if id == "" {
			values, err := capability.List(c.Workspace, revision)
			return Result{Output: map[string]any{"capabilities": values}}, err
		}
		value, err := capability.Inspect(c.Workspace, revision, id)
		return Result{Output: map[string]any{"capability": value}}, err
	case "get_acceptance_status":
		value, err := capability.Inspect(c.Workspace, revision, id)
		return Result{Output: map[string]any{"capability": value}}, err
	case "get_acceptance_evidence":
		path, err := capability.EvidencePath(id)
		if err != nil {
			return Result{}, err
		}
		absolute := filepath.Join(c.Workspace, filepath.FromSlash(path))
		data, err := os.ReadFile(absolute)
		if err != nil {
			return Result{}, err
		}
		if len(data) > 256*1024 {
			return Result{}, errors.New("acceptance evidence exceeds inline limit")
		}
		value, err := capability.Inspect(c.Workspace, revision, id)
		return Result{Output: map[string]any{"capability": value, "evidence_path": path, "evidence_json": string(data)}}, err
	default:
		return Result{}, errors.New("unsupported capability tool mode")
	}
}
