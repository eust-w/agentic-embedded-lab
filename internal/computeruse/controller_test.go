package computeruse

import (
	"context"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

type fakeNative struct{ clicked, typed bool }

func (f *fakeNative) AccessibilityTrusted(bool) bool   { return true }
func (f *fakeNative) ScreenRecordingTrusted(bool) bool { return true }
func (f *fakeNative) Click(float64, float64) error     { f.clicked = true; return nil }
func (f *fakeNative) Type(string) error                { f.typed = true; return nil }

func TestComputerUseRequiresPerApplicationPermission(t *testing.T) {
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	native := &fakeNative{}
	controller := New(state, native)
	if err := controller.Click(context.Background(), "com.apple.TextEdit", 10, 10); err == nil {
		t.Fatal("expected permission denial")
	}
	if err := controller.SetApplicationPermission(context.Background(), "com.apple.TextEdit", DecisionAllow, "persistent"); err != nil {
		t.Fatal(err)
	}
	if err := controller.Click(context.Background(), "com.apple.TextEdit", 10, 10); err != nil || !native.clicked {
		t.Fatalf("approved click failed: %v", err)
	}
}
