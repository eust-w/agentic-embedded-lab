package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

func TestMemoryRedactsSecretsAndScopesSearch(t *testing.T) {
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	repository := New(state)
	if enabled, err := repository.Enabled(context.Background(), ScopeProject, "p"); err != nil || enabled {
		t.Fatalf("memory must be disabled by default: %v %v", enabled, err)
	}
	if _, err := repository.Save(context.Background(), Memory{Scope: ScopeProject, ProjectID: "p", Content: "must fail"}); err == nil {
		t.Fatal("memory saved without explicit opt-in")
	}
	if err := repository.SetEnabled(context.Background(), ScopeProject, "p", true); err != nil {
		t.Fatal(err)
	}
	saved, err := repository.Save(context.Background(), Memory{Scope: ScopeProject, ProjectID: "p", Content: "Use api_key=test-secret-value for UART", SourceThreadID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(saved.Content, "test-secret") || !strings.Contains(saved.Content, "[REDACTED]") {
		t.Fatalf("secret was not redacted: %s", saved.Content)
	}
	items, err := repository.Search(context.Background(), ScopeProject, "p", "UART", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("unexpected memories: %#v %v", items, err)
	}
	if err := repository.Delete(context.Background(), saved.ID); err != nil {
		t.Fatal(err)
	}
}
