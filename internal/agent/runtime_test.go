package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
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
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for streamed items")
}
