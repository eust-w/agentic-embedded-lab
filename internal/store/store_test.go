package store

import (
	"context"
	"strings"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

func TestThreadTurnAndItemsSurviveReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(ctx, "project-1", "Fix UART", "gpt-5.6", protocol.PermissionWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.StartTurn(ctx, thread.ID, "Investigate UART overrun")
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.AppendItem(ctx, protocol.Item{
		ThreadID: thread.ID,
		TurnID:   turn.ID,
		Type:     protocol.ItemUserMessage,
		Payload:  map[string]any{"text": "Investigate UART overrun"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", item.Sequence)
	}
	if _, _, err := store.PutArtifact(strings.NewReader("trace")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	threads, err := store.ListThreads(ctx, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].ID != thread.ID {
		t.Fatalf("unexpected threads: %#v", threads)
	}
	items, err := store.Items(ctx, thread.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Payload["text"] != "Investigate UART overrun" {
		t.Fatalf("unexpected items: %#v", items)
	}
}
