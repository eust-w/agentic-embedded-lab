package agent

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/approval"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/plugins"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
	"github.com/eust-w/agentic-embedded-lab/internal/tools"
)

func TestRuntimePersistsStreamedTurn(t *testing.T) {
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	client := NewResponsesClient(StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"fixed\"}\n" +
			"data: {\"type\":\"response.completed\"}\n"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	runtime := NewRuntime(state, client, events.New())
	thread, err := runtime.CreateThread(context.Background(), "p", "UART", DefaultModel, protocol.PermissionWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := runtime.RunTurn(context.Background(), thread, "fix")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items, err := runtime.Items(context.Background(), thread.ID, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) >= 3 {
			if turn.ID != items[0].TurnID {
				t.Fatalf("unexpected turn linkage")
			}
			threads, err := runtime.ListThreads(context.Background(), "p")
			if err != nil {
				t.Fatal(err)
			}
			if len(threads) == 1 && threads[0].Status == protocol.ThreadReady {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for streamed items")
}

func TestRuntimeExecutesFunctionCallAndContinuesResponse(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var requests atomic.Int32
	client := NewResponsesClient(StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		var body string
		if requests.Add(1) == 1 {
			body = "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"file\",\"arguments\":\"{\\\"action\\\":\\\"read\\\",\\\"path\\\":\\\"hello.txt\\\"}\"}}\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-1\"}}\n"
		} else {
			body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"done\"}\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-2\"}}\n"
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	bus := events.New()
	runtime := NewRuntime(state, client, bus)
	registry := tools.NewRegistry()
	if err := registry.Register(tools.FileTool{Workspace: workspace}); err != nil {
		t.Fatal(err)
	}
	runtime.ConfigureTools(registry, approval.New(), plugins.NewHookDispatcher())
	runtime.RegisterProject("project", workspace)
	thread, err := runtime.CreateThread(context.Background(), "project", "tool", DefaultModel, protocol.PermissionReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunTurn(context.Background(), thread, "read hello"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items, err := runtime.Items(context.Background(), thread.ID, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.Type == protocol.ItemToolResult {
				if requests.Load() != 2 {
					t.Fatalf("expected two model requests, got %d", requests.Load())
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("tool result was not persisted")
}

func TestApprovalBrokerResolvesPendingRequest(t *testing.T) {
	broker := NewApprovalBroker()
	result := make(chan bool, 1)
	go func() {
		allowed, _ := broker.Wait(context.Background(), "approval")
		result <- allowed
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !broker.Resolve("approval", true) {
		time.Sleep(time.Millisecond)
	}
	select {
	case allowed := <-result:
		if !allowed {
			t.Fatal("approval was not granted")
		}
	case <-time.After(time.Second):
		t.Fatal("approval did not resolve")
	}
}
