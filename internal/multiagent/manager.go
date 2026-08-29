package multiagent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/google/uuid"
)

type Status string

const (
	StatusActive      Status = "active"
	StatusDone        Status = "done"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted"
)

type Handle struct {
	ID        string             `json:"id"`
	ParentID  string             `json:"parent_id"`
	Thread    protocol.Thread    `json:"thread"`
	TurnID    string             `json:"turn_id"`
	Spec      protocol.AgentSpec `json:"spec"`
	Status    Status             `json:"status"`
	StartedAt time.Time          `json:"started_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type Manager struct {
	store      *store.Store
	runtime    *agent.Runtime
	bus        *events.Bus
	mu         sync.RWMutex
	handles    map[string]*Handle
	turnAgents map[string]string
	limit      int
	cancel     func()
}

func New(state *store.Store, runtime *agent.Runtime, bus *events.Bus, limit int) *Manager {
	if limit < 1 {
		limit = 4
	}
	manager := &Manager{store: state, runtime: runtime, bus: bus, handles: make(map[string]*Handle), turnAgents: make(map[string]string), limit: limit}
	channel, cancel := bus.Subscribe(128)
	manager.cancel = cancel
	go manager.observe(channel)
	return manager
}

func (m *Manager) Close() { m.cancel() }

func (m *Manager) Spawn(ctx context.Context, parent protocol.Thread, projectID, prompt string, spec protocol.AgentSpec) (Handle, error) {
	if prompt == "" || spec.Name == "" || spec.Role == "" {
		return Handle{}, errors.New("prompt, name, and role are required")
	}
	m.mu.Lock()
	active := 0
	for _, handle := range m.handles {
		if handle.Status == StatusActive {
			active++
		}
	}
	if active >= m.limit {
		m.mu.Unlock()
		return Handle{}, errors.New("subagent concurrency limit reached")
	}
	m.mu.Unlock()
	thread, err := m.store.CreateChildThread(ctx, parent.ID, projectID, spec.Name, spec.Model, spec.Permission)
	if err != nil {
		return Handle{}, err
	}
	turn, err := m.runtime.RunTurn(ctx, thread, prompt)
	if err != nil {
		return Handle{}, err
	}
	now := time.Now().UTC()
	handle := Handle{ID: uuid.NewString(), ParentID: parent.ID, Thread: thread, TurnID: turn.ID, Spec: spec, Status: StatusActive, StartedAt: now, UpdatedAt: now}
	m.mu.Lock()
	m.handles[handle.ID] = &handle
	m.turnAgents[turn.ID] = handle.ID
	m.mu.Unlock()
	return handle, nil
}

func (m *Manager) Steer(ctx context.Context, id, message string) (protocol.Turn, error) {
	m.mu.RLock()
	handle, ok := m.handles[id]
	m.mu.RUnlock()
	if !ok {
		return protocol.Turn{}, errors.New("subagent not found")
	}
	turn, err := m.runtime.RunTurn(ctx, handle.Thread, message)
	if err != nil {
		return protocol.Turn{}, err
	}
	m.mu.Lock()
	handle.TurnID = turn.ID
	handle.Status = StatusActive
	handle.UpdatedAt = time.Now().UTC()
	m.turnAgents[turn.ID] = id
	m.mu.Unlock()
	return turn, nil
}

func (m *Manager) Interrupt(id string) bool {
	m.mu.Lock()
	handle, ok := m.handles[id]
	if ok {
		handle.Status = StatusInterrupted
		handle.UpdatedAt = time.Now().UTC()
	}
	m.mu.Unlock()
	return ok && m.runtime.CancelTurn(handle.TurnID)
}

func (m *Manager) List() []Handle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Handle, 0, len(m.handles))
	for _, handle := range m.handles {
		result = append(result, *handle)
	}
	return result
}

func (m *Manager) Wait(ctx context.Context, id string) (Handle, error) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		m.mu.RLock()
		handle, ok := m.handles[id]
		if ok && handle.Status != StatusActive {
			result := *handle
			m.mu.RUnlock()
			return result, nil
		}
		m.mu.RUnlock()
		if !ok {
			return Handle{}, errors.New("subagent not found")
		}
		select {
		case <-ctx.Done():
			return Handle{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *Manager) observe(channel <-chan events.Event) {
	for event := range channel {
		if event.Topic != "turn.completed" && event.Topic != "turn.failed" {
			continue
		}
		data, ok := event.Data.(map[string]any)
		if !ok {
			continue
		}
		turnID, _ := data["turn_id"].(string)
		m.mu.Lock()
		id := m.turnAgents[turnID]
		if handle := m.handles[id]; handle != nil {
			if event.Topic == "turn.completed" {
				handle.Status = StatusDone
			} else {
				handle.Status = StatusFailed
			}
			handle.UpdatedAt = time.Now().UTC()
		}
		delete(m.turnAgents, turnID)
		m.mu.Unlock()
	}
}
