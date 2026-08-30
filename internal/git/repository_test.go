package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryStagesSpecificPathsAndRejectsEscape(t *testing.T) {
	ctx := context.Background()
	root := initialiseRepository(t)
	repository, err := Discover(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unstaged, err := repository.FileContent(ctx, "file.txt", "unstaged", "")
	if err != nil || unstaged.Original != "base\n" || unstaged.Modified != "changed\n" {
		t.Fatalf("unexpected unstaged content: %#v %v", unstaged, err)
	}
	if err := repository.Stage(ctx, []string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	diff, err := repository.Diff(ctx, "staged", "")
	if err != nil || !strings.Contains(diff, "changed") {
		t.Fatalf("unexpected staged diff: %q %v", diff, err)
	}
	staged, err := repository.FileContent(ctx, "file.txt", "staged", "")
	if err != nil || staged.Original != "base\n" || staged.Modified != "changed\n" {
		t.Fatalf("unexpected staged content: %#v %v", staged, err)
	}
	if err := repository.Stage(ctx, []string{"../escape"}); err == nil {
		t.Fatal("expected path escape to fail")
	}
}

func TestManagedWorktreeCarriesDirtyTrackedPatch(t *testing.T) {
	ctx := context.Background()
	root := initialiseRepository(t)
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository := &Repository{Root: root}
	manager := WorktreeManager{Root: filepath.Join(t.TempDir(), "worktrees")}
	reference, err := manager.Create(ctx, repository, "HEAD", true)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Remove(ctx, reference)
	content, err := os.ReadFile(filepath.Join(reference.Path, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "dirty\n" || reference.PatchSHA256 == "" {
		t.Fatalf("dirty state was not transferred: %q %#v", content, reference)
	}
}

func initialiseRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	if _, err := run(ctx, root, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(ctx, root, "config", "user.email", "test@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(ctx, root, "config", "user.name", "Aether Test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(ctx, root, "add", "file.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(ctx, root, "commit", "-m", "initial"); err != nil {
		t.Fatal(err)
	}
	return root
}
