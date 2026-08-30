package plugins

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

const ProcessProtocolVersion = "aether.plugin.process/v1"

type ProcessRuntime struct {
	mu      sync.Mutex
	plugin  Installed
	runtime string
	socket  string
	command *exec.Cmd
	cancel  context.CancelFunc
	conn    *grpc.ClientConn
}

type ProcessServer interface {
	Handshake(context.Context, map[string]any) (map[string]any, error)
	Invoke(context.Context, string, map[string]any) (map[string]any, error)
}

func RegisterProcessServer(server *grpc.Server, implementation ProcessServer) {
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "aether.plugin.v1.Plugin",
		HandlerType: (*ProcessServer)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "Handshake", Handler: processHandshakeHandler},
			{MethodName: "Invoke", Handler: processInvokeHandler},
		},
	}, implementation)
}

func StartProcess(ctx context.Context, plugin Installed, runtimeRoot string) (*ProcessRuntime, error) {
	if plugin.Revoked || !plugin.Active || plugin.Manifest.Process == nil {
		return nil, errors.New("active process plugin is required")
	}
	root, err := filepath.Abs(plugin.Path)
	if err != nil {
		return nil, err
	}
	executable, err := containedRegularFile(root, plugin.Manifest.Process.Executable)
	if err != nil {
		return nil, err
	}
	runtimeRoot, err = filepath.Abs(runtimeRoot)
	if err != nil {
		return nil, err
	}
	runtimeDir, err := os.MkdirTemp(runtimeRoot, plugin.Manifest.ID+"-")
	if err != nil {
		return nil, err
	}
	socket := filepath.Join(runtimeDir, "plugin.sock")
	if len(socket) >= 100 {
		_ = os.RemoveAll(runtimeDir)
		return nil, errors.New("plugin runtime path is too long for a Unix socket")
	}
	processCtx, cancel := context.WithCancel(ctx)
	arguments := append([]string(nil), plugin.Manifest.Process.Arguments...)
	commandName := executable
	if runtime.GOOS == "darwin" {
		profile := processSeatbeltProfile(root, runtimeDir, executable, containsPermission(plugin.Manifest.Permissions, PermissionNetwork))
		arguments = append([]string{"-p", profile, "--", executable}, arguments...)
		commandName = "/usr/bin/sandbox-exec"
	}
	command := exec.CommandContext(processCtx, commandName, arguments...)
	command.Dir = root
	command.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"TMPDIR=" + runtimeDir,
		"AETHER_PLUGIN_SOCKET=" + socket,
		"AETHER_PLUGIN_ID=" + plugin.Manifest.ID,
		"AETHER_PLUGIN_PROTOCOL=" + ProcessProtocolVersion,
	}
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		cancel()
		_ = os.RemoveAll(runtimeDir)
		return nil, err
	}
	runtimeClient := &ProcessRuntime{plugin: plugin, runtime: runtimeDir, socket: socket, command: command, cancel: cancel}
	connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
	defer connectCancel()
	conn, err := grpc.NewClient("passthrough:///"+socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}),
	)
	if err != nil {
		runtimeClient.Close()
		return nil, err
	}
	runtimeClient.conn = conn
	conn.Connect()
	if err := waitForGRPC(connectCtx, conn); err != nil {
		runtimeClient.Close()
		return nil, fmt.Errorf("plugin process did not become ready: %w", err)
	}
	response, err := runtimeClient.call(connectCtx, "/aether.plugin.v1.Plugin/Handshake", map[string]any{
		"protocol":  ProcessProtocolVersion,
		"plugin_id": plugin.Manifest.ID,
		"version":   plugin.Manifest.Version,
	})
	if err != nil {
		runtimeClient.Close()
		return nil, fmt.Errorf("plugin handshake: %w", err)
	}
	if protocol, _ := response["protocol"].(string); protocol != ProcessProtocolVersion {
		runtimeClient.Close()
		return nil, errors.New("plugin process protocol mismatch")
	}
	return runtimeClient, nil
}

func processHandshakeHandler(server any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	request := &structpb.Struct{}
	if err := decode(request); err != nil {
		return nil, err
	}
	handler := func(ctx context.Context, value any) (any, error) {
		result, err := server.(ProcessServer).Handshake(ctx, value.(*structpb.Struct).AsMap())
		if err != nil {
			return nil, err
		}
		return structpb.NewStruct(result)
	}
	if interceptor == nil {
		return handler(ctx, request)
	}
	return interceptor(ctx, request, &grpc.UnaryServerInfo{Server: server, FullMethod: "/aether.plugin.v1.Plugin/Handshake"}, handler)
}

func processInvokeHandler(server any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	request := &structpb.Struct{}
	if err := decode(request); err != nil {
		return nil, err
	}
	handler := func(ctx context.Context, value any) (any, error) {
		values := value.(*structpb.Struct).AsMap()
		capability, _ := values["capability"].(string)
		input, _ := values["input"].(map[string]any)
		result, err := server.(ProcessServer).Invoke(ctx, capability, input)
		if err != nil {
			return nil, err
		}
		return structpb.NewStruct(result)
	}
	if interceptor == nil {
		return handler(ctx, request)
	}
	return interceptor(ctx, request, &grpc.UnaryServerInfo{Server: server, FullMethod: "/aether.plugin.v1.Plugin/Invoke"}, handler)
}

func (p *ProcessRuntime) Invoke(ctx context.Context, capability string, input map[string]any) (map[string]any, error) {
	if strings.TrimSpace(capability) == "" {
		return nil, errors.New("plugin capability is required")
	}
	return p.call(ctx, "/aether.plugin.v1.Plugin/Invoke", map[string]any{
		"capability": capability,
		"input":      input,
	})
}

func (p *ProcessRuntime) call(ctx context.Context, method string, values map[string]any) (map[string]any, error) {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn == nil {
		return nil, errors.New("plugin process is closed")
	}
	request, err := structpb.NewStruct(values)
	if err != nil {
		return nil, err
	}
	response := &structpb.Struct{}
	if err := conn.Invoke(ctx, method, request, response); err != nil {
		return nil, err
	}
	return response.AsMap(), nil
}

func (p *ProcessRuntime) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.command != nil && p.command.Process != nil {
		_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGTERM)
		wait := make(chan struct{})
		go func() { _ = p.command.Wait(); close(wait) }()
		select {
		case <-wait:
		case <-time.After(2 * time.Second):
			_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
			<-wait
		}
	}
	p.command = nil
	return os.RemoveAll(p.runtime)
}

func waitForGRPC(ctx context.Context, conn *grpc.ClientConn) error {
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if !conn.WaitForStateChange(ctx, state) {
			return ctx.Err()
		}
	}
}

func containedRegularFile(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || relative == "" {
		return "", errors.New("plugin executable must be package-relative")
	}
	path, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("plugin executable escapes package")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		return "", errors.New("plugin executable must be a non-symlink executable file")
	}
	return path, nil
}

func containsPermission(values []Permission, expected Permission) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func processSeatbeltProfile(pluginRoot, runtimeRoot, executable string, network bool) string {
	quote := func(value string) string { return strings.ReplaceAll(value, "\"", "\\\"") }
	lines := []string{
		"(version 1)",
		"(deny default)",
		"(allow process*)",
		"(allow sysctl-read)",
		"(allow file-read* (subpath \"/System\") (subpath \"/usr\") (subpath \"/bin\") (subpath \"/opt/homebrew\"))",
		fmt.Sprintf("(allow file-read* (subpath \"%s\"))", quote(pluginRoot)),
		fmt.Sprintf("(allow file-read* (literal \"%s\"))", quote(executable)),
		fmt.Sprintf("(allow file-read* file-write* (subpath \"%s\"))", quote(runtimeRoot)),
	}
	if network {
		lines = append(lines, "(allow network-outbound)")
	} else {
		lines = append(lines, fmt.Sprintf("(allow network-outbound (path \"%s\"))", quote(filepath.Join(runtimeRoot, "plugin.sock"))))
	}
	return strings.Join(lines, "\n")
}
