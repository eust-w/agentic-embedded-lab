package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type FileTool struct{ Workspace string }

func (f FileTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "file", Description: "Read or atomically write one workspace file", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"action": map[string]any{"type": "string", "enum": []string{"read", "write"}}, "path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}},
		"required":   []string{"action", "path"},
	}}
}

func (f FileTool) Operation(arguments map[string]any) approval.Operation {
	action, _ := arguments["action"].(string)
	path, _ := arguments["path"].(string)
	risk := approvalRisk(action == "write")
	return approval.Operation{Tool: "file", Action: action, Resource: filepath.Join(f.Workspace, path), Risk: risk}
}

func (f FileTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	default:
	}
	action, _ := arguments["action"].(string)
	relative, _ := arguments["path"].(string)
	target, err := safePath(f.Workspace, relative)
	if err != nil {
		return Result{}, err
	}
	switch action {
	case "read":
		data, err := os.ReadFile(target)
		if err != nil {
			return Result{}, err
		}
		if len(data) > 1<<20 {
			return Result{}, errors.New("file exceeds 1 MiB read limit")
		}
		return Result{Output: map[string]any{"path": relative, "content": string(data)}}, nil
	case "write":
		content, ok := arguments["content"].(string)
		if !ok {
			return Result{}, errors.New("content is required for write")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return Result{}, err
		}
		temporary, err := os.CreateTemp(filepath.Dir(target), ".aether-write-*")
		if err != nil {
			return Result{}, err
		}
		name := temporary.Name()
		defer os.Remove(name)
		if err := temporary.Chmod(0o600); err != nil {
			_ = temporary.Close()
			return Result{}, err
		}
		if _, err := temporary.WriteString(content); err != nil {
			_ = temporary.Close()
			return Result{}, err
		}
		if err := temporary.Sync(); err != nil {
			_ = temporary.Close()
			return Result{}, err
		}
		if err := temporary.Close(); err != nil {
			return Result{}, err
		}
		if err := os.Rename(name, target); err != nil {
			return Result{}, err
		}
		return Result{Output: map[string]any{"path": relative, "bytes": len(content)}}, nil
	default:
		return Result{}, errors.New("unsupported file action")
	}
}

type SearchTool struct {
	Workspace  string
	MaxResults int
}

func (s SearchTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "search", Description: "Search UTF-8 workspace files without executing a shell", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"query": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}},
		"required":   []string{"query"},
	}}
}

func (s SearchTool) Operation(arguments map[string]any) approval.Operation {
	path, _ := arguments["path"].(string)
	return approval.Operation{Tool: "search", Action: "read", Resource: filepath.Join(s.Workspace, path)}
}

func (s SearchTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	query, _ := arguments["query"].(string)
	relative, _ := arguments["path"].(string)
	if query == "" {
		return Result{}, errors.New("search query is required")
	}
	root, err := safePath(s.Workspace, relative)
	if err != nil {
		return Result{}, err
	}
	limit := s.MaxResults
	if limit <= 0 {
		limit = 200
	}
	type match struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	matches := make([]match, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || len(matches) >= limit {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" || strings.HasPrefix(entry.Name(), ".ael") {
				return filepath.SkipDir
			}
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() && len(matches) < limit {
			line++
			text := scanner.Text()
			if strings.Contains(text, query) {
				rel, _ := filepath.Rel(s.Workspace, path)
				matches = append(matches, match{Path: rel, Line: line, Text: text})
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("search workspace: %w", err)
	}
	return Result{Output: map[string]any{"matches": matches, "truncated": len(matches) == limit}}, nil
}

func safePath(workspace, relative string) (string, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes workspace")
	}
	return target, nil
}

func approvalRisk(write bool) protocol.ApprovalRisk {
	if write {
		return protocol.RiskHigh
	}
	return protocol.RiskLow
}
