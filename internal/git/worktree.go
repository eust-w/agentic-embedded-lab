package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
	"github.com/google/uuid"
)

type WorktreeManager struct {
	Root string
}

type HandoffResult struct {
	PatchSHA256 string   `json:"patch_sha256"`
	Paths       []string `json:"paths"`
	Applied     bool     `json:"applied"`
	CleanedUp   bool     `json:"cleaned_up"`
}

func (m WorktreeManager) Create(ctx context.Context, repository *Repository, base string, includeDirty bool) (protocol.WorktreeRef, error) {
	if repository == nil || repository.Root == "" || m.Root == "" {
		return protocol.WorktreeRef{}, errors.New("repository and worktree root are required")
	}
	if !safeRef(base) {
		return protocol.WorktreeRef{}, errors.New("invalid base ref")
	}
	id := uuid.NewString()
	path := filepath.Join(m.Root, id)
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return protocol.WorktreeRef{}, err
	}
	if _, err := run(ctx, repository.Root, "worktree", "add", "--detach", path, base); err != nil {
		return protocol.WorktreeRef{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_, _ = run(context.Background(), repository.Root, "worktree", "remove", "--force", path)
		}
	}()
	var patchHash string
	if includeDirty {
		patch, err := run(ctx, repository.Root, "diff", "--binary", "HEAD")
		if err != nil {
			return protocol.WorktreeRef{}, err
		}
		if patch != "" {
			patchPath := filepath.Join(path, ".aether-dirty.patch")
			if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
				return protocol.WorktreeRef{}, err
			}
			if _, err := run(ctx, path, "apply", "--index", patchPath); err != nil {
				return protocol.WorktreeRef{}, fmt.Errorf("apply dirty patch: %w", err)
			}
			_ = os.Remove(patchPath)
			digest := sha256.Sum256([]byte(patch))
			patchHash = hex.EncodeToString(digest[:])
		}
		if err := copyIncludedFiles(repository.Root, path); err != nil {
			return protocol.WorktreeRef{}, err
		}
	}
	head, err := (&Repository{Root: path}).Head(ctx)
	if err != nil {
		return protocol.WorktreeRef{}, err
	}
	cleanup = false
	return protocol.WorktreeRef{ID: id, Repository: repository.Root, Path: path, BaseBranch: base, Head: head, PatchSHA256: patchHash, CreatedAt: time.Now().UTC()}, nil
}

func (m WorktreeManager) Remove(ctx context.Context, reference protocol.WorktreeRef) error {
	root, err := filepath.Abs(m.Root)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(reference.Path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("worktree path is outside managed root")
	}
	_, err = run(ctx, reference.Repository, "worktree", "remove", "--force", path)
	return err
}

func (m WorktreeManager) Handoff(ctx context.Context, reference protocol.WorktreeRef, cleanup bool) (HandoffResult, error) {
	if err := m.validateReference(reference); err != nil {
		return HandoffResult{}, err
	}
	worktree := &Repository{Root: reference.Path}
	untracked, err := run(ctx, worktree.Root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return HandoffResult{}, err
	}
	for _, path := range strings.Split(untracked, "\x00") {
		if path == "" {
			continue
		}
		if _, err := run(ctx, worktree.Root, "add", "-N", "--", path); err != nil {
			return HandoffResult{}, err
		}
	}
	patch, err := run(ctx, worktree.Root, "diff", "--binary", "HEAD")
	if err != nil {
		return HandoffResult{}, err
	}
	if patch == "" {
		return HandoffResult{}, errors.New("worktree has no changes to hand off")
	}
	pathsOutput, err := run(ctx, worktree.Root, "diff", "--name-only", "-z", "HEAD")
	if err != nil {
		return HandoffResult{}, err
	}
	paths := make([]string, 0)
	for _, path := range strings.Split(pathsOutput, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	patchPath := filepath.Join(reference.Path, ".aether-handoff.patch")
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		return HandoffResult{}, err
	}
	defer os.Remove(patchPath)
	if _, err := run(ctx, reference.Repository, "apply", "--check", "--binary", patchPath); err != nil {
		return HandoffResult{}, fmt.Errorf("handoff conflicts with the destination worktree: %w", err)
	}
	if _, err := run(ctx, reference.Repository, "apply", "--binary", patchPath); err != nil {
		return HandoffResult{}, fmt.Errorf("apply handoff patch: %w", err)
	}
	digest := sha256.Sum256([]byte(patch))
	result := HandoffResult{PatchSHA256: hex.EncodeToString(digest[:]), Paths: paths, Applied: true}
	if cleanup {
		if err := m.Remove(ctx, reference); err != nil {
			return result, fmt.Errorf("handoff applied but worktree cleanup failed: %w", err)
		}
		result.CleanedUp = true
	}
	return result, nil
}

func (m WorktreeManager) validateReference(reference protocol.WorktreeRef) error {
	root, err := filepath.Abs(m.Root)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(reference.Path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("worktree path is outside managed root")
	}
	if info, err := os.Lstat(path); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed worktree is unavailable or unsafe")
	}
	repository, err := filepath.Abs(reference.Repository)
	if err != nil {
		return err
	}
	if repository == path || reference.ID == "" {
		return errors.New("invalid managed worktree reference")
	}
	return nil
}

func copyIncludedFiles(sourceRoot, destinationRoot string) error {
	manifest := filepath.Join(sourceRoot, ".worktreeinclude")
	content, err := os.ReadFile(manifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.ContainsAny(line, "*?[]") {
			continue
		}
		source := filepath.Join(sourceRoot, line)
		destination := filepath.Join(destinationRoot, line)
		relative, err := filepath.Rel(sourceRoot, source)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid .worktreeinclude path %q", line)
		}
		info, err := os.Stat(source)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("included path must be a regular file: %s", line)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
