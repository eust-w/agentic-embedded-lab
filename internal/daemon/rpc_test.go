package daemon

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/automation"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/computeruse"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/terminal"
)

type daemonComputerNative struct{ clicked bool }

func (n *daemonComputerNative) AccessibilityTrusted(bool) bool     { return true }
func (n *daemonComputerNative) ScreenRecordingTrusted(bool) bool   { return true }
func (n *daemonComputerNative) FrontmostBundleID() (string, error) { return "com.example.Test", nil }
func (n *daemonComputerNative) FocusedElementSecure() bool         { return false }
func (n *daemonComputerNative) ElementTree(int) ([]byte, error) {
	return []byte(`{"role":"AXApplication"}`), nil
}
func (n *daemonComputerNative) Screenshot() ([]byte, error)  { return []byte("png"), nil }
func (n *daemonComputerNative) Click(float64, float64) error { n.clicked = true; return nil }
func (n *daemonComputerNative) Type(string) error            { return nil }

func TestDaemonRequiresCapabilityToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	runtime := agent.NewRuntime(state, agent.NewResponsesClient(agent.StaticAPIKey("test")), events.New())
	server := &Server{Token: "capability", Runtime: runtime}
	denied := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "1", Token: "wrong", Method: "health"})
	if denied.Error != "unauthorized" {
		t.Fatalf("expected unauthorized, got %#v", denied)
	}
	health := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "2", Token: "capability", Method: "health"})
	if health.Error != "" {
		t.Fatalf("unexpected health error: %s", health.Error)
	}
	created := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "3", Token: "capability", Method: "thread.create", Params: mustJSON(t, map[string]any{
		"project_id": "p", "title": "t", "model": agent.DefaultModel, "permission": protocol.PermissionWorkspace,
	})})
	if created.Error != "" {
		t.Fatal(created.Error)
	}
	thread, ok := created.Result.(protocol.Thread)
	if !ok || thread.ID == "" {
		t.Fatal("thread was not created")
	}
}

func TestDaemonTerminalIsRestrictedToRegisteredProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	terminals := terminal.NewManager(ctx)
	server := &Server{Token: "capability", Runtime: agent.NewRuntime(state, agent.NewResponsesClient(agent.StaticAPIKey("test")), events.New()), Terminals: terminals}
	workspace := t.TempDir()
	opened := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "1", Token: "capability", Method: "project.open", Params: mustJSON(t, map[string]any{"project_id": "p", "root": workspace, "permission": protocol.PermissionWorkspace})})
	if opened.Error != "" {
		t.Fatal(opened.Error)
	}
	started := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "2", Token: "capability", Method: "terminal.start", Params: mustJSON(t, map[string]any{"workspace": workspace, "columns": 80, "rows": 24})})
	info, ok := started.Result.(terminal.Info)
	if started.Error != "" || !ok || info.ID == "" {
		t.Fatalf("terminal start: %#v", started)
	}
	defer terminals.Stop(info.ID)
	denied := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "3", Token: "capability", Method: "terminal.start", Params: mustJSON(t, map[string]any{"workspace": filepath.Dir(workspace), "columns": 80, "rows": 24})})
	if denied.Error == "" {
		t.Fatal("unregistered terminal workspace was accepted")
	}
}

func TestDaemonAutomationContractPersistsRRULE(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	scheduler := automation.New(state, func(context.Context, protocol.AutomationSpec, string) error { return nil })
	server := &Server{Token: "capability", Automations: scheduler}
	spec := protocol.AutomationSpec{APIVersion: protocol.APIVersion, ID: "nightly", Name: "夜间回归", Prompt: "run tests", RRULE: "FREQ=DAILY;BYHOUR=2", ProjectID: "p", Permission: protocol.PermissionWorkspace, Enabled: true}
	saved := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "1", Token: "capability", Method: "automation.save", Params: mustJSON(t, spec)})
	if saved.Error != "" {
		t.Fatal(saved.Error)
	}
	listed := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "2", Token: "capability", Method: "automation.list"})
	values, ok := listed.Result.([]protocol.AutomationSpec)
	if listed.Error != "" || !ok || len(values) != 1 || values[0].ID != "nightly" {
		t.Fatalf("automation list: %#v", listed)
	}
	run := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "3", Token: "capability", Method: "automation.run", Params: mustJSON(t, map[string]string{"id": spec.ID})})
	if run.Error != "" {
		t.Fatal(run.Error)
	}
	jobID := run.Result.(map[string]string)["job_id"]
	deadline := time.Now().Add(time.Second)
	for {
		status := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "4", Token: "capability", Method: "automation.job", Params: mustJSON(t, map[string]string{"job_id": jobID})})
		if status.Error != "" {
			t.Fatal(status.Error)
		}
		if status.Result.(automation.Job).Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("automation job did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	deleted := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "5", Token: "capability", Method: "automation.delete", Params: mustJSON(t, map[string]string{"id": spec.ID})})
	if deleted.Error != "" {
		t.Fatal(deleted.Error)
	}
	eventSpec := spec
	eventSpec.ID, eventSpec.RRULE, eventSpec.EventSource = "plugin-event", "", "plugin.model.updated"
	if saved := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "6", Token: "capability", Method: "automation.save", Params: mustJSON(t, eventSpec)}); saved.Error != "" {
		t.Fatal(saved.Error)
	}
	triggered := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "7", Token: "capability", Method: "automation.trigger", Params: mustJSON(t, map[string]string{"event_source": eventSpec.EventSource})})
	if triggered.Error != "" || len(triggered.Result.(map[string][]string)["job_ids"]) != 1 {
		t.Fatalf("event automation did not trigger: %#v", triggered)
	}
}

func TestConfigureProjectLoadsSignedPluginSkillAndHook(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	publicKey, privateKey, _ := ed25519.GenerateKey(nil)
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "skills", "review"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "skills", "review", "SKILL.md"), []byte("---\nname: review\ndescription: Safe review\n---\nFollow REVIEW_PLUGIN_RULE."), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "hooks", "block.json"), []byte(`{"event":"PreToolUse","tool":"command","block":true,"reason":"plugin policy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := plugins.Sign(plugins.Manifest{APIVersion: plugins.ManifestVersion, ID: "review", Name: "Review", Version: "1.0.0", Permissions: []plugins.Permission{plugins.PermissionFiles}, Skills: []string{"skills"}, Hooks: []string{"hooks/block.json"}}, "official", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(source, "plugin.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	registry := &plugins.Registry{Root: t.TempDir(), Trust: plugins.StaticTrustStore{"official": publicKey}}
	if _, err := registry.Install(source, false); err != nil {
		t.Fatal(err)
	}
	runtime := agent.NewRuntime(state, agent.NewResponsesClient(agent.StaticAPIKey("test")), events.New())
	server := &Server{Runtime: runtime, State: state, PluginRegistry: registry}
	projectRoot := t.TempDir()
	if _, err := server.ConfigureProject(ctx, store.ProjectRecord{ID: "p", Root: projectRoot, Permission: protocol.PermissionReadOnly}, false); err != nil {
		t.Fatal(err)
	}
	hooks := runtime.ProjectHooks("p")
	results, err := hooks.Dispatch(ctx, plugins.HookPayload{Event: plugins.HookPreToolUse, Data: map[string]any{"tool": "command"}})
	if err != nil || len(results) != 1 || !results[0].Block {
		t.Fatalf("plugin hook missing: %#v %v", results, err)
	}
}

func TestDaemonAcceptsOnlyTypedChromeSnapshots(t *testing.T) {
	ctx := context.Background()
	server := &Server{Token: "capability", ChromeSessions: &browser.ChromeSessionStore{}}
	message := browser.NativeMessage{Type: "snapshot", ID: "capture-1", TabID: 8, Payload: map[string]any{"url": "https://example.com", "title": "Example", "dom": "<html></html>"}}
	ingested := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "1", Token: "capability", Method: "browser.chrome_ingest", Params: mustJSON(t, message)})
	if ingested.Error != "" {
		t.Fatal(ingested.Error)
	}
	latest := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "2", Token: "capability", Method: "browser.chrome_latest"})
	if latest.Error != "" {
		t.Fatal(latest.Error)
	}
	result, ok := latest.Result.(map[string]any)
	if !ok || result["available"] != true {
		t.Fatalf("latest Chrome snapshot missing: %#v", latest.Result)
	}
}

func TestDaemonComputerClickRequiresExplicitConfirmation(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	native := &daemonComputerNative{}
	controller := computeruse.New(state, native)
	if err := controller.SetApplicationPermission(ctx, "com.example.Test", computeruse.DecisionAllow, "persistent"); err != nil {
		t.Fatal(err)
	}
	server := &Server{Token: "capability", Computer: controller}
	denied := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "1", Token: "capability", Method: "computer.click", Params: mustJSON(t, map[string]any{"bundle_id": "com.example.Test", "x": 1, "y": 2})})
	if denied.Error == "" || native.clicked {
		t.Fatalf("unconfirmed click was accepted: %#v", denied)
	}
	allowed := server.dispatch(ctx, Request{APIVersion: protocol.APIVersion, ID: "2", Token: "capability", Method: "computer.click", Params: mustJSON(t, map[string]any{"bundle_id": "com.example.Test", "x": 1, "y": 2, "confirmed": true})})
	if allowed.Error != "" || !native.clicked {
		t.Fatalf("confirmed click failed: %#v", allowed)
	}
}

func TestAuthenticatedUnixClientAndServerRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	socketRoot, err := os.MkdirTemp("/tmp", "aetherd-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketRoot) })
	socket := filepath.Join(socketRoot, "daemon.sock")
	server := &Server{SocketPath: socket, Token: "01234567890123456789012345678901", Runtime: agent.NewRuntime(state, agent.NewResponsesClient(agent.StaticAPIKey("test")), events.New())}
	done := make(chan error, 1)
	go func() { done <- server.Listen(ctx) }()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		select {
		case err := <-done:
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
				t.Skip("sandbox blocks Unix socket binding")
			}
			t.Fatalf("daemon failed before socket creation: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon socket did not appear")
		}
		time.Sleep(5 * time.Millisecond)
	}
	client := Client{SocketPath: socket, Token: server.Token}
	var health map[string]any
	if err := client.Call(ctx, Request{ID: "health", Method: "health"}, &health); err != nil {
		t.Fatal(err)
	}
	if health["status"] != "ready" {
		t.Fatalf("unexpected health: %#v", health)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestResponseProofAuthenticatesDaemonAndPayload(t *testing.T) {
	response := Response{APIVersion: protocol.APIVersion, ID: "request-1", Result: map[string]any{"status": "ready"}}
	proof := responseProof("capability", response)
	if proof == "" || proof != responseProof("capability", response) {
		t.Fatal("response proof is not deterministic")
	}
	response.Result = map[string]any{"status": "tampered"}
	if proof == responseProof("capability", response) {
		t.Fatal("tampered response retained a valid proof")
	}
	if proof == responseProof("other-capability", Response{APIVersion: protocol.APIVersion, ID: "request-1", Result: map[string]any{"status": "ready"}}) {
		t.Fatal("different capability produced the same proof")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
