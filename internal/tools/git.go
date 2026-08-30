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

type GitWriteTool struct{ Workspace string }

func (g GitWriteTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "git_write", Description: "Stage, unstage, restore, commit, push, or create a GitHub pull request after explicit approval", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": []string{"stage", "unstage", "restore", "commit", "push", "pr_create"}},
			"paths":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 200},
			"message": map[string]any{"type": "string"},
			"remote":  map[string]any{"type": "string"},
			"branch":  map[string]any{"type": "string"},
			"title":   map[string]any{"type": "string"},
			"body":    map[string]any{"type": "string"},
			"base":    map[string]any{"type": "string"},
			"head":    map[string]any{"type": "string"},
			"draft":   map[string]any{"type": "boolean"},
		},
		"required": []string{"action"},
	}}
}

func (g GitWriteTool) Operation(arguments map[string]any) approval.Operation {
	action, _ := arguments["action"].(string)
	operation := approval.Operation{Tool: "git_write", Action: action, Resource: g.Workspace, Risk: protocol.RiskHigh}
	if action == "push" || action == "pr_create" {
		operation.Network = true
		operation.External = true
	}
	if action == "restore" {
		operation.Destructive = true
	}
	return operation
}

func (g GitWriteTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	repository, err := gitrepo.Discover(ctx, g.Workspace)
	if err != nil {
		return Result{}, err
	}
	action, _ := arguments["action"].(string)
	switch action {
	case "stage":
		paths, err := stringArguments(arguments["paths"])
		if err != nil {
			return Result{}, err
		}
		err = repository.Stage(ctx, paths)
	case "unstage":
		paths, err := stringArguments(arguments["paths"])
		if err != nil {
			return Result{}, err
		}
		err = repository.Unstage(ctx, paths)
	case "restore":
		paths, err := stringArguments(arguments["paths"])
		if err != nil {
			return Result{}, err
		}
		err = repository.Restore(ctx, paths)
	case "commit":
		message, _ := arguments["message"].(string)
		var head string
		head, err = repository.Commit(ctx, message)
		return Result{Output: map[string]any{"head": head}}, err
	case "push":
		remote, _ := arguments["remote"].(string)
		branch, _ := arguments["branch"].(string)
		err = repository.Push(ctx, remote, branch)
	case "pr_create":
		title, _ := arguments["title"].(string)
		body, _ := arguments["body"].(string)
		base, _ := arguments["base"].(string)
		head, _ := arguments["head"].(string)
		draft, _ := arguments["draft"].(bool)
		var pullRequest gitrepo.PullRequest
		pullRequest, err = repository.CreatePullRequest(ctx, title, body, base, head, draft)
		return Result{Output: map[string]any{"pull_request": pullRequest}}, err
	default:
		return Result{}, errors.New("unsupported Git write action")
	}
	return Result{Output: map[string]any{"action": action, "completed": err == nil}}, err
}

func stringArguments(value any) ([]string, error) {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		if direct, ok := value.([]string); ok && len(direct) > 0 {
			return direct, nil
		}
		return nil, errors.New("one or more paths are required")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok || item == "" {
			return nil, errors.New("paths must contain non-empty strings")
		}
		result = append(result, item)
	}
	return result, nil
}
