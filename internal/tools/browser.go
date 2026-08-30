package tools

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/computeruse"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type BrowserTool struct{ Controller *browser.Controller }

func (b BrowserTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "browser", Description: "Operate the pinned controlled browser after explicit site authorization", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"action":   map[string]any{"type": "string", "enum": []string{"start", "navigate", "dom", "screenshot", "console", "network", "download", "click", "type"}},
			"url":      map[string]any{"type": "string"},
			"selector": map[string]any{"type": "string"},
			"text":     map[string]any{"type": "string"},
		},
		"required": []string{"action"},
	}}
}

func (b BrowserTool) Operation(arguments map[string]any) approval.Operation {
	action, _ := arguments["action"].(string)
	resource, _ := arguments["url"].(string)
	if resource == "" {
		resource, _ = arguments["selector"].(string)
	}
	risk := protocol.RiskLow
	if action == "click" || action == "type" {
		risk = protocol.RiskHigh
	}
	return approval.Operation{Tool: "browser", Action: action, Resource: resource, Risk: risk, Network: action == "navigate" || action == "download"}
}

func (b BrowserTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	if b.Controller == nil {
		return Result{}, errors.New("controlled browser is unavailable")
	}
	action, _ := arguments["action"].(string)
	switch action {
	case "start":
		if err := b.Controller.Start(ctx); err != nil {
			return Result{}, err
		}
		return Result{Output: map[string]any{"status": b.Controller.Status()}}, nil
	case "navigate":
		value, _ := arguments["url"].(string)
		if err := b.Controller.Navigate(ctx, value); err != nil {
			return Result{}, err
		}
		return Result{Output: map[string]any{"status": b.Controller.Status()}}, nil
	case "dom":
		value, err := b.Controller.DOM(ctx)
		return Result{Output: map[string]any{"dom": value}}, err
	case "screenshot":
		value, err := b.Controller.Screenshot(ctx)
		return Result{Output: map[string]any{"png_base64": base64.StdEncoding.EncodeToString(value), "bytes": len(value)}}, err
	case "console":
		return Result{Output: map[string]any{"entries": b.Controller.Console(0)}}, nil
	case "network":
		return Result{Output: map[string]any{"entries": b.Controller.Network(0)}}, nil
	case "download":
		value, _ := arguments["url"].(string)
		path, err := b.Controller.Download(ctx, value)
		return Result{Output: map[string]any{"path": path}}, err
	case "click":
		selector, _ := arguments["selector"].(string)
		if err := b.Controller.Click(ctx, selector); err != nil {
			return Result{}, err
		}
		return Result{Output: map[string]any{"clicked": selector}}, nil
	case "type":
		selector, _ := arguments["selector"].(string)
		text, _ := arguments["text"].(string)
		if err := b.Controller.Type(ctx, selector, text); err != nil {
			return Result{}, err
		}
		return Result{Output: map[string]any{"typed": len(text), "selector": selector}}, nil
	default:
		return Result{}, errors.New("unsupported browser action")
	}
}

type ComputerTool struct{ Controller *computeruse.Controller }

func (c ComputerTool) Definition() protocol.ToolDefinition {
	return protocol.ToolDefinition{Type: "function", Name: "computer", Description: "Inspect or operate one explicitly authorized frontmost macOS application", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"action":    map[string]any{"type": "string", "enum": []string{"status", "tree", "screenshot", "click", "type"}},
			"bundle_id": map[string]any{"type": "string"},
			"x":         map[string]any{"type": "number"},
			"y":         map[string]any{"type": "number"},
			"text":      map[string]any{"type": "string"},
		},
		"required": []string{"action", "bundle_id"},
	}}
}

func (c ComputerTool) Operation(arguments map[string]any) approval.Operation {
	action, _ := arguments["action"].(string)
	bundleID, _ := arguments["bundle_id"].(string)
	risk := protocol.RiskLow
	if action == "click" || action == "type" {
		risk = protocol.RiskHigh
	}
	return approval.Operation{Tool: "computer", Action: action, Resource: bundleID, Risk: risk}
}

func (c ComputerTool) Execute(ctx context.Context, arguments map[string]any) (Result, error) {
	if c.Controller == nil {
		return Result{}, errors.New("Computer Use is unavailable")
	}
	action, _ := arguments["action"].(string)
	bundleID, _ := arguments["bundle_id"].(string)
	switch action {
	case "status":
		return Result{Output: map[string]any{"permissions": c.Controller.Status(false), "decision": c.Controller.ApplicationDecision(ctx, bundleID)}}, nil
	case "tree":
		value, err := c.Controller.ElementTree(ctx, bundleID, 200)
		return Result{Output: map[string]any{"tree": string(value)}}, err
	case "screenshot":
		value, err := c.Controller.Screenshot(ctx, bundleID)
		return Result{Output: map[string]any{"png_base64": base64.StdEncoding.EncodeToString(value), "bytes": len(value)}}, err
	case "click":
		x, xOK := numericArgument(arguments["x"])
		y, yOK := numericArgument(arguments["y"])
		if !xOK || !yOK {
			return Result{}, errors.New("click requires numeric x and y")
		}
		if err := c.Controller.Click(ctx, bundleID, x, y); err != nil {
			return Result{}, err
		}
		return Result{Output: map[string]any{"clicked": true}}, nil
	case "type":
		text, _ := arguments["text"].(string)
		if err := c.Controller.Type(ctx, bundleID, text); err != nil {
			return Result{}, err
		}
		return Result{Output: map[string]any{"typed": len(text)}}, nil
	default:
		return Result{}, errors.New("unsupported Computer Use action")
	}
}

func numericArgument(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int:
		return float64(number), true
	default:
		return 0, false
	}
}
