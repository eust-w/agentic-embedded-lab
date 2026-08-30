package terminal

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestPTYRoundTripAndStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewManager(ctx)
	workspace := t.TempDir()
	if err := manager.RegisterWorkspace(workspace); err != nil {
		t.Fatal(err)
	}
	info, err := manager.Start(workspace, 100, 24)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Write(info.ID, []byte("printf '__AETHER_PTY__\\n'\n")); err != nil {
		t.Fatal(err)
	}
	var output string
	var offset int64
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := manager.Read(info.ID, offset, 64*1024)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := base64.StdEncoding.DecodeString(snapshot.DataBase64)
		if err != nil {
			t.Fatal(err)
		}
		output += string(payload)
		offset = snapshot.NextOffset
		if strings.Contains(output, "__AETHER_PTY__") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(output, "__AETHER_PTY__") {
		t.Fatalf("terminal output missing: %q", output)
	}
	if err := manager.Resize(info.ID, 120, 30); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(info.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPTYRejectsUnsafeWorkspaceAndDimensions(t *testing.T) {
	manager := NewManager(context.Background())
	if _, err := manager.Start("relative", 80, 24); err == nil {
		t.Fatal("relative workspace was accepted")
	}
	workspace := t.TempDir()
	if err := manager.RegisterWorkspace(workspace); err != nil {
		t.Fatal(err)
	}
	info, err := manager.Start(workspace, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(info.ID)
	if err := manager.Resize(info.ID, 1, 1); err == nil {
		t.Fatal("unsafe terminal dimensions were accepted")
	}
}
