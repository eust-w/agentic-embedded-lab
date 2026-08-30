package multiagent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/executor"
	aethergit "github.com/eust-w/agentic-embedded-lab/internal/git"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/tools"
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
	ID        string                `json:"id"`
	ParentID  string                `json:"parent_id"`
	Thread    protocol.Thread       `json:"thread"`
	TurnID    string                `json:"turn_id"`
	Spec      protocol.AgentSpec    `json:"spec"`
	Status    Status                `json:"status"`
	StartedAt time.Time             `json:"started_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	Worktree  *protocol.WorktreeRef `json:"worktree,omitempty"`
}

type Result struct {
	Handle  Handle          `json:"handle"`
	Items   []protocol.Item `json:"items"`
	Summary string          `json:"summary"`
}

type Manager struct {
	store        *store.Store
	runtime      *agent.Runtime
	bus          *events.Bus
	mu           sync.RWMutex
	handles      map[string]*Handle
	turnAgents   map[string]string
	limit        int
	cancel       func()
	projectRoots map[string]string
	worktrees    aethergit.WorktreeManager
}

func New(state *store.Store, runtime *agent.Runtime, bus *events.Bus, limit int) *Manager {
	if limit < 1 {
		limit = 4
	}
	manager := &Manager{store: state, runtime: runtime, bus: bus, handles: make(map[string]*Handle), turnAgents: make(map[string]string), projectRoots: make(map[string]string), limit: limit}
	channel, cancel := bus.Subscribe(128)
	manager.cancel = cancel
	go manager.observe(channel)
	return manager
}

func (m *Manager) Close() { m.cancel() }

func (m *Manager) ConfigureProject(projectID, root, worktreeRoot string) error {
	root, err := filepath.Abs(root)
	if err != nil || projectID == "" {
		return errors.New("project id and root are required")
	}
	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(filepath.Dir(root), ".aether-worktrees")
	}
	m.mu.Lock()
	m.projectRoots[projectID] = root
	m.worktrees = aethergit.WorktreeManager{Root: worktreeRoot}
	m.mu.Unlock()
	return nil
}

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
	hooks := m.runtime.ProjectHooks(projectID)
	if hooks != nil {
		results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookSubagentStart, ThreadID: parent.ID, Data: map[string]any{"prompt": prompt, "spec": spec}})
		if err != nil {
			return Handle{}, err
		}
		for _, result := range results {
			if result.Block {
				return Handle{}, fmt.Errorf("subagent blocked by hook: %s", result.Reason)
			}
		}
	}
	childProjectID := projectID
	var worktree *protocol.WorktreeRef
	if spec.Permission != protocol.PermissionReadOnly {
		m.mu.RLock()
		root := m.projectRoots[projectID]
		manager := m.worktrees
		m.mu.RUnlock()
		if root == "" || manager.Root == "" {
			return Handle{}, errors.New("write-capable subagents require a configured isolated worktree")
		}
		repository, err := aethergit.Discover(ctx, root)
		if err != nil {
			return Handle{}, err
		}
		reference, err := manager.Create(ctx, repository, "HEAD", true)
		if err != nil {
			return Handle{}, err
		}
		worktree = &reference
		childProjectID = projectID + ":subagent:" + reference.ID
		m.runtime.RegisterProject(childProjectID, reference.Path)
		registry := tools.NewRegistry()
		_ = registry.Register(tools.FileTool{Workspace: reference.Path})
		_ = registry.Register(tools.SearchTool{Workspace: reference.Path, MaxResults: 200})
		_ = registry.Register(tools.GitReadTool{Workspace: reference.Path})
		_ = registry.Register(tools.CommandTool{Workspace: reference.Path, Executor: executor.New(), Profile: spec.Permission})
		if hooks == nil {
			hooks = plugins.NewHookDispatcher()
		}
		m.runtime.ConfigureProjectTools(childProjectID, registry, approval.New(), hooks)
		m.runtime.CopyProjectInstructions(projectID, childProjectID)
	}
	thread, err := m.store.CreateChildThread(ctx, parent.ID, childProjectID, spec.Name, spec.Model, spec.Permission)
	if err != nil {
		if worktree != nil {
			_ = m.worktrees.Remove(context.Background(), *worktree)
		}
		return Handle{}, err
	}
	turn, err := m.runtime.RunTurn(ctx, thread, prompt)
	if err != nil {
		if worktree != nil {
			_ = m.worktrees.Remove(context.Background(), *worktree)
		}
		return Handle{}, err
	}
	now := time.Now().UTC()
	handle := Handle{ID: uuid.NewString(), ParentID: parent.ID, Thread: thread, TurnID: turn.ID, Spec: spec, Status: StatusActive, StartedAt: now, UpdatedAt: now, Worktree: worktree}
	m.mu.Lock()
	m.handles[handle.ID] = &handle
	m.turnAgents[turn.ID] = handle.ID
	m.mu.Unlock()
	return handle, nil
}

func (m *Manager) Message(ctx context.Context, id, message string) (protocol.Turn, error) {
	return m.Steer(ctx, id, message)
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
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.Before(result[j].StartedAt) })
	return result
}

func (m *Manager) Result(ctx context.Context, id string) (Result, error) {
	m.mu.RLock()
	handle, ok := m.handles[id]
	m.mu.RUnlock()
	if !ok {
		return Result{}, errors.New("subagent not found")
	}
	items, err := m.store.Items(ctx, handle.Thread.ID, 0, 500)
	if err != nil {
		return Result{}, err
	}
	var parts []string
	for _, item := range items {
		if item.Type != protocol.ItemAgentMessage {
			continue
		}
		if delta, ok := item.Payload["delta"].(string); ok && delta != "" {
			parts = append(parts, delta)
		}
	}
	return Result{Handle: *handle, Items: items, Summary: strings.TrimSpace(strings.Join(parts, ""))}, nil
}

func (m *Manager) CloseAgent(ctx context.Context, id string) error {
	m.mu.Lock()
	handle, ok := m.handles[id]
	if ok {
		delete(m.handles, id)
		delete(m.turnAgents, handle.TurnID)
	}
	m.mu.Unlock()
	if !ok {
		return errors.New("subagent not found")
	}
	if handle.Status == StatusActive {
		m.runtime.CancelTurn(handle.TurnID)
	}
	if handle.Worktree != nil {
		return m.worktrees.Remove(ctx, *handle.Worktree)
	}
	return nil
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
		var completed *Handle
		if handle := m.handles[id]; handle != nil {
			if event.Topic == "turn.completed" {
				handle.Status = StatusDone
			} else {
				handle.Status = StatusFailed
			}
			handle.UpdatedAt = time.Now().UTC()
			copy := *handle
			completed = &copy
		}
		delete(m.turnAgents, turnID)
		m.mu.Unlock()
		if completed != nil {
			if hooks := m.runtime.ProjectHooks(completed.Thread.ProjectID); hooks != nil {
				_, _ = hooks.Dispatch(context.Background(), plugins.HookPayload{Event: plugins.HookSubagentStop, ThreadID: completed.Thread.ID, TurnID: completed.TurnID, Data: map[string]any{"status": completed.Status}})
			}
		}
	}
}
