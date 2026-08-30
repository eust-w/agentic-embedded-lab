package plugins

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type testProcessServer struct{}

func (testProcessServer) Handshake(context.Context, map[string]any) (map[string]any, error) {
	return map[string]any{"protocol": ProcessProtocolVersion}, nil
}

func (testProcessServer) Invoke(_ context.Context, capability string, input map[string]any) (map[string]any, error) {
	return map[string]any{"capability": capability, "input": input}, nil
}

func TestProcessProtocolInvokesTypedGRPCService(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	RegisterProcessServer(server, testProcessServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///plugin",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}))
	if err != nil {
		t.Fatal(err)
	}
	conn.Connect()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForGRPC(ctx, conn); err != nil {
		t.Fatal(err)
	}
	runtime := &ProcessRuntime{conn: conn}
	result, err := runtime.Invoke(ctx, "analyze", map[string]any{"sample": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if result["capability"] != "analyze" {
		t.Fatalf("unexpected response: %#v", result)
	}
}

func TestProcessExecutableAndSeatbeltFailClosed(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "plugin")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := containedRegularFile(root, "plugin"); err != nil {
		t.Fatal(err)
	}
	if _, err := containedRegularFile(root, "../plugin"); err == nil {
		t.Fatal("path escape was accepted")
	}
	profile := processSeatbeltProfile(root, filepath.Join(root, "runtime"), executable, false)
	if strings.Contains(profile, "(allow network-outbound)\n") || !strings.Contains(profile, "plugin.sock") {
		t.Fatalf("network sandbox is too broad: %s", profile)
	}
}
