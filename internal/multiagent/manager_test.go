package multiagent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
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
}
