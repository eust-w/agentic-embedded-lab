package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Change struct {
	Path     string `json:"path"`
	Index    string `json:"index"`
	Worktree string `json:"worktree"`
}

type Repository struct {
	Root string
}

func Discover(ctx context.Context, path string) (*Repository, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	result, err := run(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("discover git repository: %w", err)
	}
	root := strings.TrimSpace(result)
	if root == "" {
		return nil, errors.New("git returned an empty repository root")
	}
	return &Repository{Root: root}, nil
}

func (r *Repository) Branch(ctx context.Context) (string, error) {
	result, err := run(ctx, r.Root, "branch", "--show-current")
	return strings.TrimSpace(result), err
}

func (r *Repository) Head(ctx context.Context) (string, error) {
	result, err := run(ctx, r.Root, "rev-parse", "HEAD")
	return strings.TrimSpace(result), err
}

func (r *Repository) Status(ctx context.Context) ([]Change, error) {
	result, err := run(ctx, r.Root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var changes []Change
	for _, record := range strings.Split(result, "\x00") {
		if len(record) < 4 {
			continue
		}
		changes = append(changes, Change{Index: record[:1], Worktree: record[1:2], Path: record[3:]})
	}
	return changes, nil
}

func (r *Repository) Diff(ctx context.Context, scope string, base string) (string, error) {
	switch scope {
	case "unstaged":
		return run(ctx, r.Root, "diff", "--no-ext-diff")
	case "staged":
		return run(ctx, r.Root, "diff", "--cached", "--no-ext-diff")
	case "branch":
		if base == "" {
			return "", errors.New("base branch is required")
		}
		return run(ctx, r.Root, "diff", "--no-ext-diff", base+"...HEAD")
	case "commit":
		if base == "" {
			return "", errors.New("commit is required")
		}
		return run(ctx, r.Root, "show", "--format=", "--no-ext-diff", base)
	default:
		return "", fmt.Errorf("unsupported diff scope %q", scope)
	}
}

func (r *Repository) Stage(ctx context.Context, paths []string) error {
	resolved, err := r.safePaths(paths)
	if err != nil {
		return err
	}
	_, err = run(ctx, r.Root, append([]string{"add", "--"}, resolved...)...)
	return err
}

func (r *Repository) Unstage(ctx context.Context, paths []string) error {
	resolved, err := r.safePaths(paths)
	if err != nil {
		return err
	}
	_, err = run(ctx, r.Root, append([]string{"restore", "--staged", "--"}, resolved...)...)
	return err
}

func (r *Repository) Restore(ctx context.Context, paths []string) error {
	resolved, err := r.safePaths(paths)
	if err != nil {
		return err
	}
	_, err = run(ctx, r.Root, append([]string{"restore", "--worktree", "--"}, resolved...)...)
	return err
}

func (r *Repository) Commit(ctx context.Context, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" || strings.ContainsRune(message, '\x00') {
		return "", errors.New("commit message is required")
	}
	if _, err := run(ctx, r.Root, "commit", "-m", message); err != nil {
		return "", err
	}
	return r.Head(ctx)
}

func (r *Repository) Push(ctx context.Context, remote, branch string) error {
	if !safeRef(remote) || !safeRef(branch) {
		return errors.New("remote and branch must be simple git identifiers")
	}
	_, err := run(ctx, r.Root, "push", remote, "HEAD:"+branch)
	return err
}

func (r *Repository) safePaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("at least one path is required")
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		absolute, err := filepath.Abs(filepath.Join(r.Root, path))
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(r.Root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("path escapes repository: %s", path)
		}
		result = append(result, relative)
	}
	return result, nil
}

func safeRef(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, " \t\r\n~^:?*[\\") {
		return false
	}
	return !strings.Contains(value, "..") && !strings.Contains(value, "@{")
}

func run(ctx context.Context, directory string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = directory
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin", "LANG=C.UTF-8"}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}
