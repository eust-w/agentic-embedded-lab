package computeruse

import (
	"context"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

type fakeNative struct {
	clicked, typed, secure bool
	frontmost              string
}

func (f *fakeNative) AccessibilityTrusted(bool) bool     { return true }
func (f *fakeNative) ScreenRecordingTrusted(bool) bool   { return true }
func (f *fakeNative) FrontmostBundleID() (string, error) { return f.frontmost, nil }
func (f *fakeNative) FocusedElementSecure() bool         { return f.secure }
func (f *fakeNative) ElementTree(int) ([]byte, error)    { return []byte(`[]`), nil }
func (f *fakeNative) Screenshot() ([]byte, error)        { return []byte("png"), nil }
func (f *fakeNative) Click(float64, float64) error       { f.clicked = true; return nil }
func (f *fakeNative) Type(string) error                  { f.typed = true; return nil }

func TestComputerUseRequiresPerApplicationPermission(t *testing.T) {
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	native := &fakeNative{frontmost: "com.apple.TextEdit"}
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

func TestComputerUseRejectsFrontmostMismatchSecureFieldsAndSystemSettings(t *testing.T) {
	state, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	native := &fakeNative{frontmost: "com.example.Other"}
	controller := New(state, native)
	if err := controller.SetApplicationPermission(context.Background(), "com.apple.TextEdit", DecisionAllow, "persistent"); err != nil {
		t.Fatal(err)
	}
	if err := controller.Click(context.Background(), "com.apple.TextEdit", 1, 1); err == nil {
		t.Fatal("frontmost mismatch was accepted")
	}
	native.frontmost, native.secure = "com.apple.TextEdit", true
	if err := controller.Type(context.Background(), "com.apple.TextEdit", "secret"); err == nil {
		t.Fatal("secure-field typing was accepted")
	}
	if err := controller.SetApplicationPermission(context.Background(), "com.apple.SystemSettings", DecisionAllow, "persistent"); err == nil {
		t.Fatal("System Settings control was accepted")
	}
}
