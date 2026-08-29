package plugins

import (
	"context"
	"errors"
	"sync"
)

type HookEvent string

const (
	HookSessionStart      HookEvent = "SessionStart"
	HookSessionEnd        HookEvent = "SessionEnd"
	HookTurnStart         HookEvent = "TurnStart"
	HookPreToolUse        HookEvent = "PreToolUse"
	HookPermissionRequest HookEvent = "PermissionRequest"
	HookPostToolUse       HookEvent = "PostToolUse"
	HookPreCompact        HookEvent = "PreCompact"
	HookPostCompact       HookEvent = "PostCompact"
	HookSubagentStart     HookEvent = "SubagentStart"
	HookSubagentStop      HookEvent = "SubagentStop"
	HookStop              HookEvent = "Stop"
)

type HookPayload struct {
	Event    HookEvent      `json:"event"`
	ThreadID string         `json:"thread_id"`
	TurnID   string         `json:"turn_id,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type HookResult struct {
	Block   bool           `json:"block"`
	Reason  string         `json:"reason,omitempty"`
	Updates map[string]any `json:"updates,omitempty"`
}

type HookHandler func(context.Context, HookPayload) (HookResult, error)

type HookDispatcher struct {
	mu       sync.RWMutex
	handlers map[HookEvent][]HookHandler
}

func NewHookDispatcher() *HookDispatcher {
	return &HookDispatcher{handlers: make(map[HookEvent][]HookHandler)}
}

func (d *HookDispatcher) Register(event HookEvent, handler HookHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[event] = append(d.handlers[event], handler)
}

func (d *HookDispatcher) Dispatch(ctx context.Context, payload HookPayload) ([]HookResult, error) {
	d.mu.RLock()
	handlers := append([]HookHandler(nil), d.handlers[payload.Event]...)
	d.mu.RUnlock()
	results := make([]HookResult, len(handlers))
	errorsByIndex := make([]error, len(handlers))
	var wait sync.WaitGroup
	for index, handler := range handlers {
		wait.Add(1)
		go func(index int, handler HookHandler) {
			defer wait.Done()
			results[index], errorsByIndex[index] = handler(ctx, payload)
		}(index, handler)
	}
	wait.Wait()
	return results, errors.Join(errorsByIndex...)
}
