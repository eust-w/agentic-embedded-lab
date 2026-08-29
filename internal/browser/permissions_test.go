package browser

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

func TestSitePermissionsDefaultAskAndLocalhostAllow(t *testing.T) {
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	permissions := NewPermissionStore(state)
	if decision, _ := permissions.Site(context.Background(), "http://localhost:3000"); decision != DecisionAllow {
		t.Fatalf("localhost should be allowed, got %s", decision)
	}
	if decision, _ := permissions.Site(context.Background(), "https://example.com"); decision != DecisionAsk {
		t.Fatalf("unknown site should ask, got %s", decision)
	}
	if err := permissions.Set(context.Background(), "site", "example.com", DecisionDeny, "persistent"); err != nil {
		t.Fatal(err)
	}
	if decision, _ := permissions.Site(context.Background(), "https://example.com/path"); decision != DecisionDeny {
		t.Fatalf("stored decision not applied: %s", decision)
	}
}

func TestControllerRejectsMissingBundledChromium(t *testing.T) {
	controller := &Controller{Executable: filepath.Join(t.TempDir(), "Chromium"), ProfilePath: filepath.Join(t.TempDir(), "profile")}
	if err := controller.Start(context.Background()); err == nil {
		t.Fatal("expected missing Chromium to fail")
	}
}
