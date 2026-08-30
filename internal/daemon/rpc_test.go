package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

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

func TestDaemonAcceptsOnlyTypedChromeSnapshots(t *testing.T) {
	ctx := context.Background()
	server := &Server{Token: "capability", Chrome: &browser.ChromeSessionStore{}}
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
