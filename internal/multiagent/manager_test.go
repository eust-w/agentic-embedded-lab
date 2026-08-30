package multiagent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/tools"
)

type transport func(*http.Request) (*http.Response, error)

func (fn transport) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestSpawnRunsIndependentChildThreadAndWaits(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	bus := events.New()
	client := agent.NewResponsesClient(agent.StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"done\"}\n" + "data: {\"type\":\"response.completed\"}\n"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	runtime := agent.NewRuntime(state, client, bus)
	hooks := plugins.NewHookDispatcher()
	var starts, stops atomic.Int32
	hooks.Register(plugins.HookSubagentStart, func(context.Context, plugins.HookPayload) (plugins.HookResult, error) {
		starts.Add(1)
		return plugins.HookResult{}, nil
	})
	hooks.Register(plugins.HookSubagentStop, func(context.Context, plugins.HookPayload) (plugins.HookResult, error) {
		stops.Add(1)
		return plugins.HookResult{}, nil
	})
	runtime.ConfigureProjectTools("p", tools.NewRegistry(), approval.New(), hooks)
	parent, err := runtime.CreateThread(ctx, "p", "parent", agent.DefaultModel, protocol.PermissionWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	manager := New(state, runtime, bus, 2)
	defer manager.Close()
	handle, err := manager.Spawn(ctx, parent, "p", "review", protocol.AgentSpec{Name: "reviewer", Role: "review", Model: agent.DefaultModel, Permission: protocol.PermissionReadOnly, MaxConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	finished, err := manager.Wait(waitCtx, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != StatusDone || finished.Thread.ParentID != parent.ID {
		t.Fatalf("unexpected child: %#v", finished)
	}
	deadline := time.Now().Add(time.Second)
	for stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if starts.Load() != 1 || stops.Load() != 1 {
		t.Fatalf("subagent hooks were not dispatched: starts=%d stops=%d", starts.Load(), stops.Load())
	}
}
