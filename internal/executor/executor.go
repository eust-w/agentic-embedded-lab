package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/protocol"
)

type CommandSpec struct {
	Executable string                     `json:"executable"`
	Arguments  []string                   `json:"arguments"`
	Directory  string                     `json:"directory"`
	Workspace  string                     `json:"workspace"`
	Profile    protocol.PermissionProfile `json:"profile"`
	Network    bool                       `json:"network"`
	Timeout    time.Duration              `json:"timeout"`
	Env        map[string]string          `json:"env,omitempty"`
}

type Result struct {
	ExitCode  int           `json:"exit_code"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	Duration  time.Duration `json:"duration"`
	TimedOut  bool          `json:"timed_out"`
	Cancelled bool          `json:"cancelled"`
}

type Executor struct {
	SandboxExecutable string
}

func New() *Executor { return &Executor{SandboxExecutable: "/usr/bin/sandbox-exec"} }

func (e *Executor) Run(ctx context.Context, spec CommandSpec) (Result, error) {
	prepared, err := e.Prepare(spec)
	if err != nil {
		return Result{}, err
	}
	if spec.Timeout <= 0 || spec.Timeout > 24*time.Hour {
		spec.Timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	command := exec.CommandContext(ctx, prepared[0], prepared[1:]...)
	command.Dir = spec.Directory
	command.Env = safeEnvironment(spec.Env)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	err = command.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(started)}
	if command.ProcessState != nil {
		result.ExitCode = command.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		result.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		return result, ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

func (e *Executor) Prepare(spec CommandSpec) ([]string, error) {
	if spec.Executable == "" || spec.Workspace == "" || spec.Directory == "" {
		return nil, errors.New("executable, workspace, and directory are required")
	}
	workspace, err := filepath.Abs(spec.Workspace)
	if err != nil {
		return nil, err
	}
	directory, err := filepath.Abs(spec.Directory)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(workspace, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("command directory is outside workspace")
	}
	executable, err := exec.LookPath(spec.Executable)
	if err != nil {
		return nil, err
	}
	command := append([]string{executable}, spec.Arguments...)
	if runtime.GOOS != "darwin" || spec.Profile == protocol.PermissionFullAccess {
		return command, nil
	}
	if _, err := os.Stat(e.SandboxExecutable); err != nil {
		return nil, errors.New("macOS sandbox-exec is unavailable")
	}
	profile := SeatbeltProfile(workspace, spec.Profile, spec.Network)
	return append([]string{e.SandboxExecutable, "-p", profile, "--"}, command...), nil
}

func SeatbeltProfile(workspace string, permission protocol.PermissionProfile, network bool) string {
	quoted := strings.ReplaceAll(workspace, "\"", "\\\"")
	lines := []string{
		"(version 1)",
		"(deny default)",
		"(allow process*)",
		"(allow sysctl-read)",
		"(allow file-read* (subpath \"/System\") (subpath \"/usr\") (subpath \"/bin\") (subpath \"/opt/homebrew\"))",
		fmt.Sprintf("(allow file-read* (subpath \"%s\"))", quoted),
		"(allow file-write* (literal \"/dev/null\") (literal \"/dev/tty\"))",
	}
	if permission == protocol.PermissionWorkspace {
		lines = append(lines, fmt.Sprintf("(allow file-write* (subpath \"%s\"))", quoted))
	}
	if network {
		lines = append(lines, "(allow network-outbound)")
	}
	return strings.Join(lines, "\n")
}

func safeEnvironment(values map[string]string) []string {
	result := []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
	}
	for key, value := range values {
		if key == "PATH" || key == "HOME" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			continue
		}
		result = append(result, key+"="+value)
	}
	return result
}
