package executor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

func TestPrepareUsesArgumentVectorAndWorkspaceSandbox(t *testing.T) {
	workspace := t.TempDir()
	executor := New()
	command, err := executor.Prepare(CommandSpec{
		Executable: "git",
		Arguments:  []string{"status", "--short", "; rm -rf /"},
		Directory:  workspace,
		Workspace:  workspace,
		Profile:    protocol.PermissionWorkspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command, " ")
	if !strings.Contains(joined, "sandbox-exec") || !strings.Contains(joined, "; rm -rf /") {
		t.Fatalf("unexpected prepared command: %#v", command)
	}
	if command[len(command)-1] != "; rm -rf /" {
		t.Fatal("argument was not preserved as a non-shell argument")
	}
}

func TestPrepareRejectsDirectoryEscape(t *testing.T) {
	workspace := t.TempDir()
	_, err := New().Prepare(CommandSpec{Executable: "git", Directory: filepath.Dir(workspace), Workspace: workspace, Profile: protocol.PermissionWorkspace})
	if err == nil {
		t.Fatal("expected directory escape rejection")
	}
}

func TestSeatbeltProfileDoesNotGrantWritesInReadOnlyMode(t *testing.T) {
	profile := SeatbeltProfile("/tmp/project", protocol.PermissionReadOnly, false)
	if strings.Contains(profile, "file-write* (subpath \"/tmp/project\")") || strings.Contains(profile, "network-outbound") {
		t.Fatalf("read-only profile is too broad: %s", profile)
	}
}
