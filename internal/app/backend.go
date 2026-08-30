package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/modeling"
	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/daemon"
	gitrepo "github.com/eust-w/agentic-embedded-lab/internal/git"
	"github.com/eust-w/agentic-embedded-lab/internal/launchagent"
	aethermemory "github.com/eust-w/agentic-embedded-lab/internal/memory"
	"github.com/eust-w/agentic-embedded-lab/internal/multiagent"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/release"
	"github.com/eust-w/agentic-embedded-lab/internal/secret"
	"github.com/eust-w/agentic-embedded-lab/internal/terminal"
	"github.com/eust-w/agentic-embedded-lab/internal/updater"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const daemonTokenAccount = "daemon-capability-token"

type Backend struct {
	ctx            context.Context
	client         *daemon.Client
	service        launchagent.Service
	currentProject string
	projectID      string
	updaterStarted bool
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
	b.updaterStarted = updater.Start()
}

func (b *Backend) UpdateStatus() map[string]bool {
	return map[string]bool{"available": updater.Available(), "started": b.updaterStarted}
}

func (b *Backend) Shutdown(context.Context) {}

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
	b.currentProject = root
	b.projectID = projectID
	tools := make([]protocol.ToolDefinition, 0)
	if value, ok := result["tools"]; ok {
		payload, _ := json.Marshal(value)
		_ = json.Unmarshal(payload, &tools)
	}
	return ProjectInfo{ID: projectID, Root: root, Permission: permission, Tools: tools}, nil
}

func (b *Backend) CheckRelease(profile release.Profile) (release.Result, error) {
	if b.currentProject == "" {
		return release.Result{}, errors.New("请先选择项目工作区")
	}
	return release.Check(b.currentProject, profile)
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
	return b.RunTurnWithAttachments(thread, input, nil)
}

func (b *Backend) RunTurnWithAttachments(thread protocol.Thread, input string, attachments []protocol.AttachmentRef) (protocol.Turn, error) {
	var turn protocol.Turn
	params, _ := json.Marshal(map[string]any{"thread": thread, "input": input, "attachments": attachments})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "turn.run", Params: params}, &turn)
	return turn, err
}

func (b *Backend) PickImageAttachments() ([]protocol.AttachmentRef, error) {
	paths, err := runtime.OpenMultipleFilesDialog(b.ctx, runtime.OpenDialogOptions{Title: "选择图片附件（最多5张，每张≤10 MiB）", Filters: []runtime.FileFilter{{DisplayName: "图片", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp"}}})
	if err != nil {
		return nil, err
	}
	if len(paths) > 5 {
		return nil, errors.New("最多选择5张图片")
	}
	var result []protocol.AttachmentRef
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 || len(data) > 10*1024*1024 {
			return nil, errors.New("图片必须介于1字节和10 MiB之间")
		}
		mimeType := http.DetectContentType(data)
		if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/gif" && mimeType != "image/webp" {
			return nil, errors.New("附件内容不是受支持的图片格式")
		}
		var attachment protocol.AttachmentRef
		params, _ := json.Marshal(map[string]string{"name": filepath.Base(path), "mime_type": mimeType, "data_base64": base64.StdEncoding.EncodeToString(data)})
		if err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "attachment.put", Params: params}, &attachment); err != nil {
			return nil, err
		}
		result = append(result, attachment)
	}
	return result, nil
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

func (b *Backend) BrowserStatus() (browser.Status, error) {
	var result browser.Status
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.status"}, &result)
	return result, err
}

func (b *Backend) StartBrowser() error {
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.start"}, nil)
}

func (b *Backend) StopBrowser() error {
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.stop"}, nil)
}

func (b *Backend) SetSitePermission(rawURL string, allow bool) error {
	decision := "deny"
	if allow {
		decision = "allow"
	}
	params, _ := json.Marshal(map[string]string{"url": rawURL, "decision": decision})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.site_permission", Params: params}, nil)
}

func (b *Backend) RevokeSitePermission(rawURL string) error {
	params, _ := json.Marshal(map[string]string{"url": rawURL, "decision": "revoke"})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.site_permission", Params: params}, nil)
}

func (b *Backend) NavigateBrowser(target string) error {
	params, _ := json.Marshal(map[string]string{"url": target})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.navigate", Params: params}, nil)
}

func (b *Backend) BrowserDOM() (string, error) {
	var result string
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.dom"}, &result)
	return result, err
}

func (b *Backend) BrowserScreenshot() (string, error) {
	var payload []byte
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.screenshot"}, &payload)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload), nil
}

func (b *Backend) BrowserConsole(after int) ([]browser.ConsoleEntry, error) {
	var result []browser.ConsoleEntry
	params, _ := json.Marshal(map[string]int{"after": after})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.console", Params: params}, &result)
	return result, err
}

func (b *Backend) BrowserNetwork(after int) ([]browser.NetworkEntry, error) {
	var result []browser.NetworkEntry
	params, _ := json.Marshal(map[string]int{"after": after})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.network", Params: params}, &result)
	return result, err
}

func (b *Backend) LatestChromeSnapshot() (map[string]any, error) {
	if err := b.refreshClient(); err != nil {
		return nil, errors.New("Aether 后台尚未配置")
	}
	var result map[string]any
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.chrome_latest"}, &result)
	return result, err
}

func (b *Backend) StartTerminal(columns, rows uint16) (terminal.Info, error) {
	if b.currentProject == "" {
		return terminal.Info{}, errors.New("请先选择项目工作区")
	}
	var result terminal.Info
	params, _ := json.Marshal(map[string]any{"workspace": b.currentProject, "columns": columns, "rows": rows})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "terminal.start", Params: params}, &result)
	return result, err
}

func (b *Backend) ListTerminals() ([]terminal.Info, error) {
	var result []terminal.Info
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "terminal.list"}, &result)
	return result, err
}

func (b *Backend) ReadTerminal(id string, after int64) (terminal.Snapshot, error) {
	var result terminal.Snapshot
	params, _ := json.Marshal(map[string]any{"id": id, "after": after, "limit": 64 * 1024})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "terminal.read", Params: params}, &result)
	return result, err
}

func (b *Backend) WriteTerminal(id, dataBase64 string) error {
	params, _ := json.Marshal(map[string]string{"id": id, "data_base64": dataBase64})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "terminal.write", Params: params}, nil)
}

func (b *Backend) ResizeTerminal(id string, columns, rows uint16) error {
	params, _ := json.Marshal(map[string]any{"id": id, "columns": columns, "rows": rows})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "terminal.resize", Params: params}, nil)
}

func (b *Backend) StopTerminal(id string) error {
	params, _ := json.Marshal(map[string]string{"id": id})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "terminal.stop", Params: params}, nil)
}

func (b *Backend) GitChanges(scope, base string) ([]gitrepo.Change, error) {
	repository, err := b.gitRepository()
	if err != nil {
		return nil, err
	}
	return repository.Changes(b.ctx, scope, base)
}

func (b *Backend) GitFileContent(path, scope, base string) (gitrepo.FileContent, error) {
	repository, err := b.gitRepository()
	if err != nil {
		return gitrepo.FileContent{}, err
	}
	return repository.FileContent(b.ctx, path, scope, base)
}

func (b *Backend) GitStage(paths []string) error {
	repository, err := b.gitRepository()
	if err != nil {
		return err
	}
	return repository.Stage(b.ctx, paths)
}

func (b *Backend) GitUnstage(paths []string) error {
	repository, err := b.gitRepository()
	if err != nil {
		return err
	}
	return repository.Unstage(b.ctx, paths)
}

func (b *Backend) GitRestore(paths []string) error {
	repository, err := b.gitRepository()
	if err != nil {
		return err
	}
	return repository.Restore(b.ctx, paths)
}

func (b *Backend) GitCommit(message string) (string, error) {
	repository, err := b.gitRepository()
	if err != nil {
		return "", err
	}
	return repository.Commit(b.ctx, message)
}

func (b *Backend) GitPush(remote, branch string) error {
	repository, err := b.gitRepository()
	if err != nil {
		return err
	}
	return repository.Push(b.ctx, remote, branch)
}

func (b *Backend) GitCreatePullRequest(title, body, base, head string, draft bool) (gitrepo.PullRequest, error) {
	repository, err := b.gitRepository()
	if err != nil {
		return gitrepo.PullRequest{}, err
	}
	return repository.CreatePullRequest(b.ctx, title, body, base, head, draft)
}

func (b *Backend) gitRepository() (*gitrepo.Repository, error) {
	if b.currentProject == "" {
		return nil, errors.New("请先选择 Git 项目工作区")
	}
	return gitrepo.Discover(b.ctx, b.currentProject)
}

func (b *Backend) StartCodeReview(scope, base string) (protocol.Thread, error) {
	if b.projectID == "" {
		return protocol.Thread{}, errors.New("请先选择Git项目")
	}
	thread, err := b.CreateThread(b.projectID, "代码审查："+scope, protocol.PermissionReadOnly)
	if err != nil {
		return protocol.Thread{}, err
	}
	prompt := "对当前Git变更执行只读语义代码审查。使用git_read工具，范围=" + scope
	if base != "" {
		prompt += "，base=" + base
	}
	prompt += "。按严重级别输出findings；每条必须包含文件路径、1-based行号、问题机制和最小修复建议。不要修改任何文件；如果没有问题，明确说明未发现finding。"
	if _, err := b.RunTurn(thread, prompt); err != nil {
		return protocol.Thread{}, err
	}
	return thread, nil
}

func (b *Backend) BrowserClick(selector string, confirmed bool) error {
	params, _ := json.Marshal(map[string]any{"selector": selector, "confirmed": confirmed})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.click", Params: params}, nil)
}

func (b *Backend) BrowserType(selector, text string) error {
	params, _ := json.Marshal(map[string]string{"selector": selector, "text": text})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.type", Params: params}, nil)
}

func (b *Backend) ComputerStatus(prompt bool) (map[string]bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]bool{"prompt": prompt})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "computer.status", Params: params}, &result)
	return result, err
}

func (b *Backend) ComputerDecision(bundleID string) (map[string]any, error) {
	var result map[string]any
	params, _ := json.Marshal(map[string]string{"bundle_id": bundleID})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "computer.decision", Params: params}, &result)
	return result, err
}

func (b *Backend) SetComputerPermission(bundleID string, allow bool, scope string) error {
	decision := "deny"
	if allow {
		decision = "allow"
	}
	params, _ := json.Marshal(map[string]string{"bundle_id": bundleID, "decision": decision, "scope": scope})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "computer.permission", Params: params}, nil)
}

func (b *Backend) SaveAutomation(spec protocol.AutomationSpec) (protocol.AutomationSpec, error) {
	var result protocol.AutomationSpec
	params, _ := json.Marshal(spec)
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "automation.save", Params: params}, &result)
	return result, err
}

func (b *Backend) ListAutomations() ([]protocol.AutomationSpec, error) {
	var result []protocol.AutomationSpec
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "automation.list"}, &result)
	return result, err
}

func (b *Backend) RunAutomation(id string) (string, error) {
	var result map[string]string
	params, _ := json.Marshal(map[string]string{"id": id})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "automation.run", Params: params}, &result)
	return result["job_id"], err
}

func (b *Backend) CancelAutomation(jobID string) (bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]string{"job_id": jobID})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "automation.cancel", Params: params}, &result)
	return result["cancelled"], err
}

func (b *Backend) ListPlugins() ([]plugins.Installed, error) {
	var result []plugins.Installed
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "plugin.list"}, &result)
	return result, err
}

func (b *Backend) SelectAndInstallPlugin(approvePermissions bool) (plugins.Installed, error) {
	path, err := runtime.OpenDirectoryDialog(b.ctx, runtime.OpenDialogOptions{Title: "选择Aether插件目录"})
	if err != nil {
		return plugins.Installed{}, err
	}
	if path == "" {
		return plugins.Installed{}, errors.New("未选择插件目录")
	}
	var result plugins.Installed
	params, _ := json.Marshal(map[string]any{"source": path, "approve_permissions": approvePermissions})
	err = b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "plugin.install", Params: params}, &result)
	return result, err
}

func (b *Backend) RevokePlugin(id, reason string) error {
	params, _ := json.Marshal(map[string]string{"id": id, "reason": reason})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "plugin.revoke", Params: params}, nil)
}

func (b *Backend) MemoryStatus() (map[string]bool, error) {
	var result map[string]bool
	params, _ := json.Marshal(map[string]string{"project_id": b.currentProjectID()})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "memory.status", Params: params}, &result)
	return result, err
}

func (b *Backend) SetMemoryEnabled(scope aethermemory.Scope, enabled bool) error {
	params, _ := json.Marshal(map[string]any{"scope": scope, "project_id": b.currentProjectID(), "enabled": enabled})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "memory.enable", Params: params}, nil)
}

func (b *Backend) ListMemories(scope aethermemory.Scope) ([]aethermemory.Memory, error) {
	var result []aethermemory.Memory
	params, _ := json.Marshal(map[string]any{"scope": scope, "project_id": b.currentProjectID()})
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "memory.list", Params: params}, &result)
	return result, err
}

func (b *Backend) SaveMemory(scope aethermemory.Scope, content, sourceThreadID string) (aethermemory.Memory, error) {
	var result aethermemory.Memory
	value := aethermemory.Memory{Scope: scope, ProjectID: b.currentProjectID(), Content: content, SourceThreadID: sourceThreadID}
	if scope == aethermemory.ScopeGlobal {
		value.ProjectID = ""
	}
	params, _ := json.Marshal(value)
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "memory.save", Params: params}, &result)
	return result, err
}

func (b *Backend) DeleteMemory(id string) error {
	params, _ := json.Marshal(map[string]string{"id": id})
	return b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "memory.delete", Params: params}, nil)
}

func (b *Backend) currentProjectID() string {
	return b.projectID
}

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
