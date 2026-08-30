package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/daemon"
	"github.com/eust-w/agentic-embedded-lab/internal/secret"
	"github.com/google/uuid"
)

const daemonTokenAccount = "daemon-capability-token"

func main() {
	socket := flag.String("socket", defaultSocketPath(), "Aether daemon Unix socket")
	flag.Parse()
	keychain := secret.NewKeychainStore()
	token, err := keychain.Get(agent.KeychainService, daemonTokenAccount)
	if err != nil {
		log.Fatalf("读取 Aether 后台凭据失败: %v", err)
	}
	client := &daemon.Client{SocketPath: *socket, Token: string(token)}
	for {
		message, err := browser.ReadNativeMessage(os.Stdin)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			_ = browser.WriteNativeResponse(os.Stdout, browser.NativeResponse{OK: false, Error: err.Error()})
			return
		}
		response := handle(context.Background(), client, message)
		if err := browser.WriteNativeResponse(os.Stdout, response); err != nil {
			return
		}
	}
}

func handle(ctx context.Context, client *daemon.Client, message browser.NativeMessage) browser.NativeResponse {
	response := browser.NativeResponse{ID: message.ID}
	switch message.Type {
	case "ping":
		response.OK = true
		response.Payload = map[string]any{"status": "ready", "host": browser.NativeHostName}
	case "snapshot":
		var snapshot browser.ChromeSnapshot
		payload, _ := json.Marshal(message)
		err := client.Call(ctx, daemon.Request{ID: uuid.NewString(), Method: "browser.chrome_ingest", Params: payload}, &snapshot)
		if err != nil {
			response.Error = err.Error()
			return response
		}
		response.OK = true
		response.Payload = map[string]any{"snapshot_id": snapshot.ID, "captured_at": snapshot.CapturedAt}
	default:
		response.Error = "不支持的 Chrome 桥接消息类型"
	}
	return response
}

func defaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aetherd.sock")
	}
	return filepath.Join(home, "Library", "Application Support", "Aether", "run", "aetherd.sock")
}

func init() {
	log.SetFlags(0)
	log.SetPrefix("aether-chrome-host: ")
}
