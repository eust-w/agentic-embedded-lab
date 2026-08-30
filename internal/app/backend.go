package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/modeling"
	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/daemon"
	"github.com/eust-w/agentic-embedded-lab/internal/launchagent"
	"github.com/eust-w/agentic-embedded-lab/internal/multiagent"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/release"
	"github.com/eust-w/agentic-embedded-lab/internal/secret"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
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
	browser        *browser.Controller
	browserStore   *store.Store
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
	root := applicationSupportRoot()
	state, err := store.Open(ctx, filepath.Join(root, "browser-state"))
	if err == nil {
		b.browserStore = state
		b.browser = &browser.Controller{
			Executable:  bundledChromiumExecutable(),
			ProfilePath: filepath.Join(root, "ChromiumProfile"),
			Permissions: browser.NewPermissionStore(state),
		}
	}
}

func (b *Backend) UpdateStatus() map[string]bool {
	return map[string]bool{"available": updater.Available(), "started": b.updaterStarted}
}

func (b *Backend) Shutdown(context.Context) {
	if b.browser != nil {
		b.browser.Stop()
	}
	if b.browserStore != nil {
		_ = b.browserStore.Close()
	}
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
	b.currentProject = root
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

func (b *Backend) BrowserStatus() browser.Status {
	if b.browser == nil {
		return browser.Status{Executable: bundledChromiumExecutable()}
	}
	return b.browser.Status()
}

func (b *Backend) StartBrowser() error {
	if b.browser == nil {
		return errors.New("浏览器状态存储不可用")
	}
	return b.browser.Start(b.ctx)
}

func (b *Backend) StopBrowser() {
	if b.browser != nil {
		b.browser.Stop()
	}
}

func (b *Backend) SetSitePermission(rawURL string, allow bool) error {
	if b.browser == nil || b.browser.Permissions == nil {
		return errors.New("浏览器权限存储不可用")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("请输入包含协议和主机名的有效网址")
	}
	decision := browser.DecisionDeny
	if allow {
		decision = browser.DecisionAllow
	}
	return b.browser.Permissions.Set(b.ctx, "site", strings.ToLower(parsed.Hostname()), decision, "persistent")
}

func (b *Backend) RevokeSitePermission(rawURL string) error {
	if b.browser == nil || b.browser.Permissions == nil {
		return errors.New("浏览器权限存储不可用")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("请输入包含协议和主机名的有效网址")
	}
	return b.browser.Permissions.Revoke(b.ctx, "site", strings.ToLower(parsed.Hostname()))
}

func (b *Backend) NavigateBrowser(target string) error {
	if b.browser == nil {
		return errors.New("浏览器不可用")
	}
	return b.browser.Navigate(b.ctx, target)
}

func (b *Backend) BrowserDOM() (string, error) {
	if b.browser == nil {
		return "", errors.New("浏览器不可用")
	}
	return b.browser.DOM(b.ctx)
}

func (b *Backend) BrowserScreenshot() (string, error) {
	if b.browser == nil {
		return "", errors.New("浏览器不可用")
	}
	payload, err := b.browser.Screenshot(b.ctx)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload), nil
}

func (b *Backend) BrowserConsole(after int) []browser.ConsoleEntry {
	if b.browser == nil {
		return nil
	}
	return b.browser.Console(after)
}

func (b *Backend) BrowserNetwork(after int) []browser.NetworkEntry {
	if b.browser == nil {
		return nil
	}
	return b.browser.Network(after)
}

func (b *Backend) LatestChromeSnapshot() (map[string]any, error) {
	if err := b.refreshClient(); err != nil {
		return nil, errors.New("Aether 后台尚未配置")
	}
	var result map[string]any
	err := b.client.Call(b.ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.chrome_latest"}, &result)
	return result, err
}

func (b *Backend) BrowserClick(selector string, confirmed bool) error {
	if b.browser == nil {
		return errors.New("浏览器不可用")
	}
	if sensitiveBrowserAction(selector) && !confirmed {
		return errors.New("该选择器可能触发提交、购买或删除；需要二次确认")
	}
	return b.browser.Click(b.ctx, selector)
}

func (b *Backend) BrowserType(selector, text string) error {
	if b.browser == nil {
		return errors.New("浏览器不可用")
	}
	return b.browser.Type(b.ctx, selector, text)
}

func sensitiveBrowserAction(selector string) bool {
	value := strings.ToLower(selector)
	for _, marker := range []string{"submit", "purchase", "buy", "delete", "remove", "upload", "付款", "购买", "删除", "提交", "上传"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func applicationSupportRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "Aether")
	}
	return filepath.Join(home, "Library", "Application Support", "Aether")
}

func bundledChromiumExecutable() string {
	if override := strings.TrimSpace(os.Getenv("AETHER_CHROMIUM_PATH")); override != "" {
		if absolute, err := filepath.Abs(override); err == nil {
			return absolute
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return filepath.Join(string(filepath.Separator), "Applications", "Aether Desktop.app", "Contents", "Resources", "Chromium.app", "Contents", "MacOS", "Chromium")
	}
	contents := filepath.Dir(filepath.Dir(executable))
	return filepath.Join(contents, "Resources", "Chromium.app", "Contents", "MacOS", "Chromium")
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
