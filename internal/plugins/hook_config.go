package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

type HookConfig struct {
	Event   HookEvent      `json:"event"`
	Tool    string         `json:"tool,omitempty"`
	Block   bool           `json:"block"`
	Reason  string         `json:"reason,omitempty"`
	Updates map[string]any `json:"updates,omitempty"`
}

func LoadHookConfig(path string) (HookConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HookConfig{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var config HookConfig
	if err := decoder.Decode(&config); err != nil {
		return HookConfig{}, err
	}
	if !validHookEvent(config.Event) || config.Block && strings.TrimSpace(config.Reason) == "" {
		return HookConfig{}, errors.New("hook event and block reason are required")
	}
	return config, nil
}

func validHookEvent(event HookEvent) bool {
	switch event {
	case HookSessionStart, HookSessionEnd, HookTurnStart, HookPreToolUse, HookPermissionRequest, HookPostToolUse, HookPreCompact, HookPostCompact, HookSubagentStart, HookSubagentStop, HookStop:
		return true
	default:
		return false
	}
}

func (c HookConfig) Handler() HookHandler {
	return func(_ context.Context, payload HookPayload) (HookResult, error) {
		if c.Tool != "" {
			tool, _ := payload.Data["tool"].(string)
			if tool != c.Tool {
				return HookResult{}, nil
			}
		}
		return HookResult{Block: c.Block, Reason: c.Reason, Updates: c.Updates}, nil
	}
}
