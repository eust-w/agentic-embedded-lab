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
	aethermemory "github.com/eust-w/agentic-embedded-lab/internal/memory"
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

func TestRuntimeInjectsDiscoveredProjectInstructions(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("PROJECT_RULE_SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	requestBody := make(chan string, 1)
	client := NewResponsesClient(StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(request.Body)
		requestBody <- string(payload)
		body := "data: {\"type\":\"response.completed\"}\n"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	runtime := NewRuntime(state, client, events.New())
	runtime.RegisterProject("project", workspace)
	thread, err := runtime.CreateThread(context.Background(), "project", "instructions", DefaultModel, protocol.PermissionReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunTurn(context.Background(), thread, "inspect"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-requestBody:
		if !strings.Contains(body, "PROJECT_RULE_SENTINEL") || !strings.Contains(body, `"role":"developer"`) {
			t.Fatalf("project instructions missing from request: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("model request was not sent")
	}
}

func TestRuntimeRestoresConversationAcrossTurns(t *testing.T) {
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var requests atomic.Int32
	secondBody := make(chan string, 1)
	client := NewResponsesClient(StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(request.Body)
		count := requests.Add(1)
		if count == 2 {
			secondBody <- string(payload)
		}
		delta := "first-answer"
		if count == 2 {
			delta = "second-answer"
		}
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + delta + "\"}\n" + "data: {\"type\":\"response.completed\"}\n"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	runtime := NewRuntime(state, client, events.New())
	thread, err := runtime.CreateThread(context.Background(), "project", "history", DefaultModel, protocol.PermissionReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunTurn(context.Background(), thread, "first-question"); err != nil {
		t.Fatal(err)
	}
	waitForThreadStatus(t, runtime, "project", protocol.ThreadReady)
	threads, _ := runtime.ListThreads(context.Background(), "project")
	if _, err := runtime.RunTurn(context.Background(), threads[0], "second-question"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-secondBody:
		for _, expected := range []string{"first-question", "first-answer", "second-question"} {
			if !strings.Contains(body, expected) {
				t.Fatalf("conversation history missing %q: %s", expected, body)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second model request was not sent")
	}
}

func TestRuntimeLoadsOnlyOptedInRedactedMemory(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	repository := aethermemory.New(state)
	if err := repository.SetEnabled(ctx, aethermemory.ScopeProject, "project", true); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Save(ctx, aethermemory.Memory{Scope: aethermemory.ScopeProject, ProjectID: "project", Content: "Use DMA; api_key=do-not-leak"}); err != nil {
		t.Fatal(err)
	}
	bodyChannel := make(chan string, 1)
	client := NewResponsesClient(StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(request.Body)
		bodyChannel <- string(payload)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n")), Header: make(http.Header)}, nil
	})}
	runtime := NewRuntime(state, client, events.New())
	runtime.ConfigureMemory(repository)
	runtime.RegisterProject("project", t.TempDir())
	thread, err := runtime.CreateThread(ctx, "project", "memory", DefaultModel, protocol.PermissionReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunTurn(ctx, thread, "continue"); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-bodyChannel:
		if !strings.Contains(body, "Use DMA") || strings.Contains(body, "do-not-leak") || !strings.Contains(body, "[REDACTED]") {
			t.Fatalf("memory boundary failed: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("model request was not sent")
	}
}

func TestRuntimeLoadsImageAttachmentFromCAS(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	digest, _, err := state.PutArtifact(strings.NewReader("fake-png"))
	if err != nil {
		t.Fatal(err)
	}
	bodyChannel := make(chan string, 1)
	client := NewResponsesClient(StaticAPIKey("test"))
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(request.Body)
		bodyChannel <- string(payload)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n")), Header: make(http.Header)}, nil
	})}
	runtime := NewRuntime(state, client, events.New())
	thread, err := runtime.CreateThread(ctx, "project", "image", DefaultModel, protocol.PermissionReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	attachment := protocol.AttachmentRef{SHA256: digest, Name: "board.png", MimeType: "image/png", Bytes: 8}
	if _, err := runtime.RunTurnWithAttachments(ctx, thread, "inspect board", []protocol.AttachmentRef{attachment}); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-bodyChannel:
		if !strings.Contains(body, `"type":"input_image"`) || !strings.Contains(body, "data:image/png;base64,ZmFrZS1wbmc=") {
			t.Fatalf("image attachment missing: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("model request was not sent")
	}
}

func waitForThreadStatus(t *testing.T, runtime *Runtime, projectID string, expected protocol.ThreadStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		threads, err := runtime.ListThreads(context.Background(), projectID)
		if err != nil {
			t.Fatal(err)
		}
		if len(threads) > 0 && threads[0].Status == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("thread did not reach %s", expected)
}
