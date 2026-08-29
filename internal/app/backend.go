package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/daemon"
	"github.com/eust-w/agentic-embedded-lab/internal/launchagent"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/secret"
	"github.com/google/uuid"
)

const daemonTokenAccount = "daemon-capability-token"

type Backend struct {
	ctx     context.Context
	client  *daemon.Client
	service launchagent.Service
}

func NewBackend() *Backend { return &Backend{service: launchagent.New()} }

func (b *Backend) Startup(ctx context.Context) {
	b.ctx = ctx
	keychain := secret.NewKeychainStore()
	token, _ := keychain.Get(agent.KeychainService, daemonTokenAccount)
	b.client = &daemon.Client{SocketPath: defaultSocketPath(), Token: string(token)}
}

func (b *Backend) Health() (map[string]any, error) {
	if b.client == nil || b.client.Token == "" {
		return nil, errors.New("Aether daemon is not configured")
	}
	var result map[string]any
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "health"}, &result)
	return result, err
}

func (b *Backend) OpenProject(projectID, root string, permission protocol.PermissionProfile) (map[string]any, error) {
	var result map[string]any
	params, _ := json.Marshal(map[string]any{"project_id": projectID, "root": root, "permission": permission})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "project.open", Params: params}, &result)
	return result, err
}

func (b *Backend) CreateThread(projectID, title string, permission protocol.PermissionProfile) (protocol.Thread, error) {
	var thread protocol.Thread
	params, _ := json.Marshal(map[string]any{
		"project_id": projectID,
		"title":      title,
		"model":      agent.DefaultModel,
		"permission": permission,
	})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "thread.create", Params: params}, &thread)
	return thread, err
}

func (b *Backend) ListThreads(projectID string) ([]protocol.Thread, error) {
	var threads []protocol.Thread
	params, _ := json.Marshal(map[string]string{"project_id": projectID})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "thread.list", Params: params}, &threads)
	return threads, err
}

func (b *Backend) Items(threadID string, after int64) ([]protocol.Item, error) {
	var items []protocol.Item
	params, _ := json.Marshal(map[string]any{"thread_id": threadID, "after": after, "limit": 200})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "thread.items", Params: params}, &items)
	return items, err
}

func (b *Backend) RunTurn(thread protocol.Thread, input string) (protocol.Turn, error) {
	var turn protocol.Turn
	params, _ := json.Marshal(map[string]any{"thread": thread, "input": input})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "turn.run", Params: params}, &turn)
	return turn, err
}

func (b *Backend) CancelTurn(turnID string) (bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]string{"turn_id": turnID})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "turn.cancel", Params: params}, &result)
	return result["cancelled"], err
}

func (b *Backend) ResolveApproval(approvalID string, allow bool) (bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]any{"approval_id": approvalID, "allow": allow})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "approval.resolve", Params: params}, &result)
	return result["resolved"], err
}

func (b *Backend) BackgroundServiceStatus() launchagent.Status { return b.service.Status() }

func (b *Backend) InstallBackgroundService() error { return b.service.Register() }

func (b *Backend) UninstallBackgroundService() error { return b.service.Unregister() }

func defaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aetherd.sock")
	}
	return filepath.Join(home, "Library", "Application Support", "Aether", "run", "aetherd.sock")
}
