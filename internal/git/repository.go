package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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

type FileContent struct {
	Path     string `json:"path"`
	Original string `json:"original"`
	Modified string `json:"modified"`
	Language string `json:"language"`
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

func (r *Repository) Changes(ctx context.Context, scope, base string) ([]Change, error) {
	if scope == "unstaged" || scope == "staged" {
		status, err := r.Status(ctx)
		if err != nil {
			return nil, err
		}
		filtered := make([]Change, 0, len(status))
		for _, change := range status {
			if scope == "unstaged" && change.Worktree != " " || scope == "staged" && change.Index != " " && change.Index != "?" {
				filtered = append(filtered, change)
			}
		}
		return filtered, nil
	}
	if !safeRef(base) {
		return nil, errors.New("valid base branch or commit is required")
	}
	var output string
	var err error
	switch scope {
	case "branch":
		output, err = run(ctx, r.Root, "diff", "--name-only", "-z", base+"...HEAD")
	case "commit":
		output, err = run(ctx, r.Root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", base)
	default:
		return nil, fmt.Errorf("unsupported diff scope %q", scope)
	}
	if err != nil {
		return nil, err
	}
	var result []Change
	for _, path := range strings.Split(output, "\x00") {
		if path != "" {
			result = append(result, Change{Path: path})
		}
	}
	return result, nil
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

func (r *Repository) FileContent(ctx context.Context, path, scope, base string) (FileContent, error) {
	resolved, err := r.safePaths([]string{path})
	if err != nil {
		return FileContent{}, err
	}
	relative := resolved[0]
	var original, modified string
	switch scope {
	case "unstaged":
		original, _ = run(ctx, r.Root, "show", ":"+relative)
		modified, err = readTextFile(filepath.Join(r.Root, relative))
	case "staged":
		original, _ = run(ctx, r.Root, "show", "HEAD:"+relative)
		modified, err = run(ctx, r.Root, "show", ":"+relative)
	case "branch":
		if !safeRef(base) {
			return FileContent{}, errors.New("valid base branch is required")
		}
		original, _ = run(ctx, r.Root, "show", base+":"+relative)
		modified, err = run(ctx, r.Root, "show", "HEAD:"+relative)
	case "commit":
		if !safeRef(base) {
			return FileContent{}, errors.New("valid commit is required")
		}
		original, _ = run(ctx, r.Root, "show", base+"^:"+relative)
		modified, err = run(ctx, r.Root, "show", base+":"+relative)
	default:
		return FileContent{}, fmt.Errorf("unsupported diff scope %q", scope)
	}
	if err != nil && original == "" && modified == "" {
		return FileContent{}, err
	}
	return FileContent{Path: relative, Original: original, Modified: modified, Language: languageForPath(relative)}, nil
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

func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if len(data) > 5*1024*1024 {
		return "", errors.New("file is too large for the diff editor")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", errors.New("binary files are not supported by the text diff editor")
	}
	return string(data), nil
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp":
		return "cpp"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs":
		return "javascript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".sh":
		return "shell"
	default:
		return "plaintext"
	}
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
