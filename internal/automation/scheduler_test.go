package automation

import (
	"context"
	"testing"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

func TestAutomationPersistsAndRunNowCreatesDurableJob(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	finished := make(chan string, 1)
	scheduler := New(state, func(ctx context.Context, spec protocol.AutomationSpec, jobID string) error {
		finished <- jobID
		return nil
	})
	spec := protocol.AutomationSpec{APIVersion: protocol.APIVersion, ID: "nightly", Name: "Nightly simulation", Prompt: "run", RRULE: "FREQ=DAILY;COUNT=2", ProjectID: "p", UseWorktree: true, Permission: protocol.PermissionWorkspace, Enabled: true}
	if err := scheduler.Save(ctx, spec); err != nil {
		t.Fatal(err)
	}
	listed, err := scheduler.List(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != spec.ID {
		t.Fatalf("unexpected automations: %#v %v", listed, err)
	}
	jobID, err := scheduler.RunNow(ctx, spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-finished:
		if completed != jobID {
			t.Fatalf("unexpected job %s", completed)
		}
	case <-time.After(time.Second):
		t.Fatal("automation handler did not run")
	}
}
