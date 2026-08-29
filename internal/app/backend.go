package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/modeling"
	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/daemon"
	"github.com/eust-w/agentic-embedded-lab/internal/launchagent"
	"github.com/eust-w/agentic-embedded-lab/internal/multiagent"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/secret"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const daemonTokenAccount = "daemon-capability-token"

type Backend struct {
	ctx     context.Context
	client  *daemon.Client
	service launchagent.Service
}

type ProjectInfo struct {
	ID         string                     `json:"id"`
	Root       string                     `json:"root"`
	Permission protocol.PermissionProfile `json:"permission"`
	Tools      []protocol.ToolDefinition  `json:"tools"`
}

func NewBackend() *Backend { return &Backend{service: launchagent.New()} }

func (b *Backend) Startup(ctx context.Context) {
	b.ctx = ctx
	_ = b.refreshClient()
}

func (b *Backend) Health() (map[string]any, error) {
	if err := b.refreshClient(); err != nil {
		return nil, errors.New("Aether 后台尚未配置")
	}
	var result map[string]any
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "health"}, &result)
	if err != nil {
		if refreshErr := b.refreshClient(); refreshErr == nil {
			err = b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "health"}, &result)
		}
	}
	return result, err
}

func (b *Backend) OpenProject(projectID, root string, permission protocol.PermissionProfile) (map[string]any, error) {
	var result map[string]any
	params, _ := json.Marshal(map[string]any{"project_id": projectID, "root": root, "permission": permission})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "project.open", Params: params}, &result)
	return result, err
}

func (b *Backend) SelectProject(permission protocol.PermissionProfile) (ProjectInfo, error) {
	if _, err := b.Health(); err != nil {
		return ProjectInfo{}, err
	}
	if permission == "" {
		permission = protocol.PermissionWorkspace
	}
	root, err := runtime.OpenDirectoryDialog(b.ctx, runtime.OpenDialogOptions{Title: "选择嵌入式项目工作区"})
	if err != nil {
		return ProjectInfo{}, err
	}
	if root == "" {
		return ProjectInfo{}, errors.New("未选择项目")
	}
	digest := sha256.Sum256([]byte(root))
	projectID := "project-" + hex.EncodeToString(digest[:8])
	result, err := b.OpenProject(projectID, root, permission)
	if err != nil {
		return ProjectInfo{}, err
	}
	tools := make([]protocol.ToolDefinition, 0)
	if value, ok := result["tools"]; ok {
		payload, _ := json.Marshal(value)
		_ = json.Unmarshal(payload, &tools)
	}
	return ProjectInfo{ID: projectID, Root: root, Permission: permission, Tools: tools}, nil
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

func (b *Backend) SpawnAgent(parent protocol.Thread, prompt string, spec protocol.AgentSpec) (multiagent.Handle, error) {
	var handle multiagent.Handle
	params, _ := json.Marshal(map[string]any{"parent": parent, "prompt": prompt, "spec": spec})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "agent.spawn", Params: params}, &handle)
	return handle, err
}

func (b *Backend) ListAgents() ([]multiagent.Handle, error) {
	var handles []multiagent.Handle
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "agent.list"}, &handles)
	return handles, err
}

func (b *Backend) MessageAgent(id, message string) (protocol.Turn, error) {
	var turn protocol.Turn
	params, _ := json.Marshal(map[string]string{"id": id, "message": message})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "agent.message", Params: params}, &turn)
	return turn, err
}

func (b *Backend) InterruptAgent(id string) (bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]string{"id": id})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "agent.interrupt", Params: params}, &result)
	return result["interrupted"], err
}

func (b *Backend) AgentResult(id string) (multiagent.Result, error) {
	var result multiagent.Result
	params, _ := json.Marshal(map[string]string{"id": id})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "agent.result", Params: params}, &result)
	return result, err
}

func (b *Backend) CloseAgent(id string) (bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]string{"id": id})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "agent.close", Params: params}, &result)
	return result["closed"], err
}

func (b *Backend) StartExperiment(request ael.RunRequest) (ael.RunRecord, error) {
	var record ael.RunRecord
	params, _ := json.Marshal(request)
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "ael.start", Params: params}, &record)
	return record, err
}

func (b *Backend) GetExperiment(id string) (ael.RunRecord, error) {
	var record ael.RunRecord
	params, _ := json.Marshal(map[string]string{"id": id})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "ael.get", Params: params}, &record)
	return record, err
}

func (b *Backend) CancelExperiment(id string) (bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]string{"id": id})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "ael.cancel", Params: params}, &result)
	return result["cancelled"], err
}

func (b *Backend) ReplayExperiment(id string) (ael.RunRecord, error) {
	var record ael.RunRecord
	params, _ := json.Marshal(map[string]string{"id": id})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "ael.replay", Params: params}, &record)
	return record, err
}

func (b *Backend) CompareExperiments(left, right string) (ael.Comparison, error) {
	var result ael.Comparison
	params, _ := json.Marshal(map[string]string{"left": left, "right": right})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "ael.compare", Params: params}, &result)
	return result, err
}

func (b *Backend) GenerateModel(projectID string, request modeling.GenerationRequest) (modeling.Package, error) {
	var result modeling.Package
	params, _ := json.Marshal(map[string]any{"project_id": projectID, "request": request})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "model.generate", Params: params}, &result)
	return result, err
}

func (b *Backend) GenerateGroundedModel(projectID string, request modeling.GroundedRequest) (modeling.Package, error) {
	var result modeling.Package
	params, _ := json.Marshal(map[string]any{"project_id": projectID, "request": request})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "model.generate_grounded", Params: params}, &result)
	return result, err
}

func (b *Backend) PromoteModel(projectID, id, version string, target modeling.ModelState, actor string, evidence *modeling.ConformanceEvidence) (modeling.Package, error) {
	var result modeling.Package
	params, _ := json.Marshal(map[string]any{"project_id": projectID, "id": id, "version": version, "target": target, "actor": actor, "evidence": evidence})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "model.promote", Params: params}, &result)
	return result, err
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

func (b *Backend) refreshClient() error {
	keychain := secret.NewKeychainStore()
	token, err := keychain.Get(agent.KeychainService, daemonTokenAccount)
	if err != nil || len(token) < 32 {
		return errors.New("daemon capability token is unavailable")
	}
	b.client = &daemon.Client{SocketPath: defaultSocketPath(), Token: string(token)}
	return nil
}
