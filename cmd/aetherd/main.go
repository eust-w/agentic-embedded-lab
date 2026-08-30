package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/modeling"
	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/automation"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/computeruse"
	"github.com/eust-w/agentic-embedded-lab/internal/daemon"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	aethermemory "github.com/eust-w/agentic-embedded-lab/internal/memory"
	"github.com/eust-w/agentic-embedded-lab/internal/multiagent"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/secret"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/terminal"
)

const daemonTokenAccount = "daemon-capability-token"

func main() {
	dataDirectory := flag.String("data", defaultDataDirectory(), "Aether application data directory")
	socketPath := flag.String("socket", defaultSocketPath(), "Aether daemon Unix socket")
	aelBackend := flag.String("ael-backend", defaultAELBackend(), "AEL backend worker executable")
	chromium := flag.String("chromium", defaultChromium(), "bundled Chromium executable")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	keychain := secret.NewKeychainStore()
	token, err := loadOrCreateToken(keychain)
	if err != nil {
		log.Fatalf("load daemon capability token: %v", err)
	}
	state, err := store.Open(ctx, *dataDirectory)
	if err != nil {
		log.Fatalf("open state store: %v", err)
	}
	defer state.Close()
	client := agent.NewResponsesClient(agent.KeychainAPIKey{Store: keychain})
	bus := events.New()
	runtime := agent.NewRuntime(state, client, bus)
	memoryRepository := aethermemory.New(state)
	runtime.ConfigureMemory(memoryRepository)
	agents := multiagent.New(state, runtime, bus, 4)
	defer agents.Close()
	aelRuns := ael.NewRunManager(state, *aelBackend)
	models := modeling.NewManager(client)
	terminals := terminal.NewManager(ctx)
	browserController := &browser.Controller{Executable: *chromium, ProfilePath: filepath.Join(*dataDirectory, "ChromiumProfile"), Permissions: browser.NewPermissionStore(state)}
	defer browserController.Stop()
	computerController := computeruse.New(state, computeruse.NewNative())
	trust, err := plugins.LoadTrustStore(filepath.Join(*dataDirectory, "trusted-plugin-keys.json"))
	if err != nil {
		log.Fatalf("load plugin trust store: %v", err)
	}
	pluginRegistry := &plugins.Registry{Root: filepath.Join(*dataDirectory, "plugins"), Trust: trust, DevelopmentMode: os.Getenv("AETHER_PLUGIN_DEVELOPMENT") == "1"}
	server := daemon.Server{SocketPath: *socketPath, Token: token, Runtime: runtime, Agents: agents, AEL: aelRuns, Models: models, ChromeSessions: &browser.ChromeSessionStore{}, Browser: browserController, Computer: computerController, Terminals: terminals, State: state, PluginRegistry: pluginRegistry, Memory: memoryRepository}
	defer server.CloseExtensions()
	projects, err := state.ListProjects(ctx)
	if err != nil {
		log.Fatalf("load persisted projects: %v", err)
	}
	for _, project := range projects {
		if _, err := server.ConfigureProject(ctx, project, false); err != nil {
			log.Printf("skip unavailable persisted project %s: %v", project.ID, err)
		}
	}
	automations := automation.New(state, func(jobCtx context.Context, spec protocol.AutomationSpec, jobID string) error {
		return executeAutomation(jobCtx, spec, jobID, state, runtime, agents)
	})
	server.Automations = automations
	go func() {
		if err := automations.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("automation scheduler stopped: %v", err)
		}
	}()
	if err := server.Listen(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run daemon: %v", err)
	}
}

func executeAutomation(ctx context.Context, spec protocol.AutomationSpec, jobID string, state *store.Store, runtime *agent.Runtime, agents *multiagent.Manager) error {
	ctx, cancel := automationContext(ctx, spec.StopPolicy)
	defer cancel()
	title := "自动化：" + spec.Name + " [" + jobID[:8] + "]"
	if spec.UseWorktree && spec.Permission != protocol.PermissionReadOnly {
		parent, err := runtime.CreateThread(ctx, spec.ProjectID, title, agent.DefaultModel, protocol.PermissionReadOnly)
		if err != nil {
			return err
		}
		handle, err := agents.Spawn(ctx, parent, spec.ProjectID, spec.Prompt, protocol.AgentSpec{Name: title, Role: "后台隔离自动化", Model: agent.DefaultModel, ReasoningEffort: "high", Permission: spec.Permission, Tools: []string{"file", "search", "command"}, MaxConcurrency: 1})
		if err != nil {
			return err
		}
		return waitAgent(ctx, agents, handle.ID)
	}
	thread, err := runtime.CreateThread(ctx, spec.ProjectID, title, agent.DefaultModel, spec.Permission)
	if err != nil {
		return err
	}
	if _, err := runtime.RunTurn(ctx, thread, spec.Prompt); err != nil {
		return err
	}
	return waitThread(ctx, state, spec.ProjectID, thread.ID)
}

func automationContext(parent context.Context, policy map[string]any) (context.Context, context.CancelFunc) {
	timeout := 2 * time.Hour
	if raw, ok := policy["timeout_seconds"].(float64); ok && raw >= 1 && raw <= 86400 {
		timeout = time.Duration(raw) * time.Second
	}
	return context.WithTimeout(parent, timeout)
}

func waitThread(ctx context.Context, state *store.Store, projectID, threadID string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		threads, err := state.ListThreads(ctx, projectID)
		if err != nil {
			return err
		}
		for _, thread := range threads {
			if thread.ID != threadID {
				continue
			}
			switch thread.Status {
			case protocol.ThreadReady:
				return nil
			case protocol.ThreadFailed, protocol.ThreadCancelled:
				return fmt.Errorf("automation thread ended with %s", thread.Status)
			case protocol.ThreadWaiting:
				return errors.New("automation requires foreground approval")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitAgent(ctx context.Context, agents *multiagent.Manager, id string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, handle := range agents.List() {
			if handle.ID != id {
				continue
			}
			switch handle.Status {
			case multiagent.StatusDone:
				return nil
			case multiagent.StatusFailed, multiagent.StatusInterrupted:
				return fmt.Errorf("automation agent ended with %s", handle.Status)
			}
		}
		select {
		case <-ctx.Done():
			agents.Interrupt(id)
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func loadOrCreateToken(keychain secret.Store) (string, error) {
	value, err := keychain.Get(agent.KeychainService, daemonTokenAccount)
	if err == nil && len(value) >= 32 {
		return string(value), nil
	}
	if err != nil && !errors.Is(err, secret.ErrNotFound) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := keychain.Set(agent.KeychainService, daemonTokenAccount, []byte(token)); err != nil {
		return "", err
	}
	return token, nil
}

func defaultDataDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "Aether")
	}
	return filepath.Join(home, "Library", "Application Support", "Aether")
}

func defaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aetherd.sock")
	}
	return filepath.Join(home, "Library", "Application Support", "Aether", "run", "aetherd.sock")
}

func defaultAELBackend() string {
	executable, err := os.Executable()
	if err != nil {
		return "ael-backend"
	}
	return filepath.Join(filepath.Dir(executable), "ael-backend")
}

func defaultChromium() string {
	executable, err := os.Executable()
	if err != nil {
		return "Chromium"
	}
	contents := filepath.Dir(filepath.Dir(executable))
	return filepath.Join(contents, "Resources", "Chromium.app", "Contents", "MacOS", "Chromium")
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.SetPrefix("aetherd ")
	_ = fmt.Sprintf
}
