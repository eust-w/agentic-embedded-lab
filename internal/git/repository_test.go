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

func TestWorktreeHandoffAppliesTrackedAndUntrackedChanges(t *testing.T) {
	ctx := context.Background()
	root := initialiseRepository(t)
	manager := WorktreeManager{Root: filepath.Join(t.TempDir(), "worktrees")}
	reference, err := manager.Create(ctx, &Repository{Root: root}, "HEAD", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reference.Path, "file.txt"), []byte("from agent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reference.Path, "new.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Handoff(ctx, reference, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || !result.CleanedUp || result.PatchSHA256 == "" || len(result.Paths) != 2 {
		t.Fatalf("unexpected handoff: %#v", result)
	}
	for path, expected := range map[string]string{"file.txt": "from agent\n", "new.txt": "new\n"} {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(content) != expected {
			t.Fatalf("handoff %s: %q %v", path, content, err)
		}
	}
}

func TestWorktreeHandoffRejectsDestinationConflict(t *testing.T) {
	ctx := context.Background()
	root := initialiseRepository(t)
	manager := WorktreeManager{Root: filepath.Join(t.TempDir(), "worktrees")}
	reference, err := manager.Create(ctx, &Repository{Root: root}, "HEAD", false)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Remove(ctx, reference)
	if err := os.WriteFile(filepath.Join(reference.Path, "file.txt"), []byte("from agent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("user change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Handoff(ctx, reference, false); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("destination conflict was not rejected: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "file.txt"))
	if string(content) != "user change\n" {
		t.Fatalf("destination changed after failed handoff: %q", content)
	}
}

func TestCreatePullRequestValidatesBeforeExternalTool(t *testing.T) {
	repository := &Repository{Root: t.TempDir()}
	if _, err := repository.CreatePullRequest(context.Background(), "", "", "main", "feature", true); err == nil {
		t.Fatal("empty title was accepted")
	}
	if _, err := repository.CreatePullRequest(context.Background(), "title", "", "../main", "feature", true); err == nil {
		t.Fatal("unsafe base ref was accepted")
	}
}

func TestRepositoryRejectsSymlinkTraversal(t *testing.T) {
	root := initialiseRepository(t)
	external := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(external, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	repository := &Repository{Root: root}
	if _, err := repository.FileContent(context.Background(), "link.txt", "unstaged", ""); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink traversal was not rejected: %v", err)
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
