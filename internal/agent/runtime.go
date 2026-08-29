package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

type Runtime struct {
	store  *store.Store
	client *ResponsesClient
	bus    *events.Bus
	mu     sync.Mutex
	runs   map[string]context.CancelFunc
}

func NewRuntime(state *store.Store, client *ResponsesClient, bus *events.Bus) *Runtime {
	return &Runtime{store: state, client: client, bus: bus, runs: make(map[string]context.CancelFunc)}
}

func (r *Runtime) CreateThread(ctx context.Context, projectID, title, model string, permission protocol.PermissionProfile) (protocol.Thread, error) {
	if projectID == "" || title == "" {
		return protocol.Thread{}, errors.New("projectID and title are required")
	}
	return r.store.CreateThread(ctx, projectID, title, model, permission)
}

func (r *Runtime) ListThreads(ctx context.Context, projectID string) ([]protocol.Thread, error) {
	return r.store.ListThreads(ctx, projectID)
}

func (r *Runtime) Items(ctx context.Context, threadID string, after int64, limit int) ([]protocol.Item, error) {
	return r.store.Items(ctx, threadID, after, limit)
}

func (r *Runtime) RunTurn(ctx context.Context, thread protocol.Thread, input string) (protocol.Turn, error) {
	turn, err := r.store.StartTurn(ctx, thread.ID, input)
	if err != nil {
		return protocol.Turn{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.runs[turn.ID] = cancel
	r.mu.Unlock()
	go r.execute(ctx, thread, turn)
	return turn, nil
}

func (r *Runtime) CancelTurn(turnID string) bool {
	r.mu.Lock()
	cancel, ok := r.runs[turnID]
	if ok {
		delete(r.runs, turnID)
	}
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (r *Runtime) execute(ctx context.Context, thread protocol.Thread, turn protocol.Turn) {
	defer func() {
		r.mu.Lock()
		delete(r.runs, turn.ID)
		r.mu.Unlock()
	}()
	_, _ = r.store.AppendItem(ctx, protocol.Item{
		ThreadID: thread.ID,
		TurnID:   turn.ID,
		Type:     protocol.ItemUserMessage,
		Payload:  map[string]any{"text": turn.Input},
	})
	request := ResponseRequest{
		Model: thread.Model,
		Input: []map[string]any{{
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": turn.Input}},
		}},
		Reasoning: map[string]any{"effort": "high"},
		Metadata:  map[string]string{"thread_id": thread.ID, "turn_id": turn.ID},
	}
	err := r.client.Stream(ctx, request, turn.ID, func(event ResponseEvent) error {
		itemType := protocol.ItemReasoning
		if stringsContains(event.Type, "output_text") {
			itemType = protocol.ItemAgentMessage
		}
		item, err := r.store.AppendItem(ctx, protocol.Item{
			ThreadID: thread.ID,
			TurnID:   turn.ID,
			Type:     itemType,
			Payload: map[string]any{
				"event_type": event.Type,
				"delta":      event.Delta,
				"raw":        event.Payload,
			},
		})
		if err == nil {
			r.bus.Publish(events.Event{Topic: "thread.item", Data: item})
		}
		return err
	})
	if err != nil {
		_, _ = r.store.AppendItem(context.Background(), protocol.Item{
			ThreadID: thread.ID,
			TurnID:   turn.ID,
			Type:     protocol.ItemAgentMessage,
			Payload:  map[string]any{"error": fmt.Sprintf("%v", err), "at": time.Now().UTC()},
		})
		r.bus.Publish(events.Event{Topic: "turn.failed", Data: map[string]any{"turn_id": turn.ID, "error": err.Error()}})
		return
	}
	r.bus.Publish(events.Event{Topic: "turn.completed", Data: map[string]any{"turn_id": turn.ID}})
}

func stringsContains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
