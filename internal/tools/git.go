package tools

import (
	"context"
	"errors"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	gitrepo "github.com/eust-w/agentic-embedded-lab/internal/git"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type GitReadTool struct{ Workspace string }

func (g GitReadTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "git_read", Description: "Read Git status, unified diff, or one file version pair without mutating the repository", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"status", "diff", "file_content"}},
			"scope":  map[string]any{"type": "string", "enum": []string{"unstaged", "staged", "branch", "commit"}},
			"base":   map[string]any{"type": "string"},
			"path":   map[string]any{"type": "string"},
		},
		"required": []string{"action"},
	}}
}

func (g GitReadTool) Operation(arguments map[string]any) approval.Operation {
	action, _ := arguments["action"].(string)
	return approval.Operation{Tool: "git_read", Action: action, Resource: g.Workspace, Risk: protocol.RiskLow}
}

func (g GitReadTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	repository, err := gitrepo.Discover(ctx, g.Workspace)
	if err != nil {
		return Result{}, err
	}
	action, _ := arguments["action"].(string)
	scope, _ := arguments["scope"].(string)
	base, _ := arguments["base"].(string)
	switch action {
	case "status":
		changes, err := repository.Changes(ctx, defaultScope(scope), base)
		return Result{Output: map[string]any{"changes": changes}}, err
	case "diff":
		value, err := repository.Diff(ctx, defaultScope(scope), base)
		if len(value) > 2*1024*1024 {
			return Result{}, errors.New("Git diff exceeds the 2 MiB model context limit")
		}
		return Result{Output: map[string]any{"diff": value}}, err
	case "file_content":
		path, _ := arguments["path"].(string)
		value, err := repository.FileContent(ctx, path, defaultScope(scope), base)
		return Result{Output: map[string]any{"file": value}}, err
	default:
		return Result{}, errors.New("unsupported Git read action")
	}
}

func defaultScope(scope string) string {
	if scope == "" {
		return "unstaged"
	}
	return scope
}
