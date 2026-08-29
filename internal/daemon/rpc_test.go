package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
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

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
