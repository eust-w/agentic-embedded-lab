package tools

import (
	"context"
	"errors"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/executor"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type CommandTool struct {
	Workspace string
	Executor  *executor.Executor
	Profile   protocol.PermissionProfile
}

func (c CommandTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "command", Description: "Run an argument-vector command in the macOS sandbox", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"executable":      map[string]any{"type": "string"},
			"arguments":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"directory":       map[string]any{"type": "string"},
			"network":         map[string]any{"type": "boolean"},
			"timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 86400},
		},
		"required": []string{"executable", "arguments", "directory"},
	}}
}

func (c CommandTool) Operation(arguments map[string]any) approval.Operation {
	executable, _ := arguments["executable"].(string)
	network, _ := arguments["network"].(bool)
	return approval.Operation{Tool: "command", Action: "execute", Resource: executable, Risk: protocol.RiskHigh, Network: network}
}

func (c CommandTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	if c.Executor == nil {
		return Result{}, errors.New("executor is not configured")
	}
	executable, _ := arguments["executable"].(string)
	directory, _ := arguments["directory"].(string)
	argumentValues, _ := arguments["arguments"].([]any)
	argv := make([]string, 0, len(argumentValues))
	for _, value := range argumentValues {
		text, ok := value.(string)
		if !ok {
			return Result{}, errors.New("command arguments must be strings")
		}
		argv = append(argv, text)
	}
	network, _ := arguments["network"].(bool)
	timeout := 1800 * time.Second
	if seconds, ok := arguments["timeout_seconds"].(float64); ok {
		timeout = time.Duration(seconds) * time.Second
	}
	result, err := c.Executor.Run(ctx, executor.CommandSpec{Executable: executable, Arguments: argv, Directory: directory, Workspace: c.Workspace, Profile: c.Profile, Network: network, Timeout: timeout})
	output := map[string]any{"exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr, "duration_ms": result.Duration.Milliseconds(), "timed_out": result.TimedOut, "cancelled": result.Cancelled}
	return Result{Output: output}, err
}
