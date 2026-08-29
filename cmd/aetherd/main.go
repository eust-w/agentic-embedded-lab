package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/eust-w/agentic-embedded-lab/internal/daemon"
	"github.com/eust-w/agentic-embedded-lab/internal/events"
	"github.com/eust-w/agentic-embedded-lab/internal/multiagent"
	"github.com/eust-w/agentic-embedded-lab/internal/secret"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

const daemonTokenAccount = "daemon-capability-token"

func main() {
	dataDirectory := flag.String("data", defaultDataDirectory(), "Aether application data directory")
	socketPath := flag.String("socket", defaultSocketPath(), "Aether daemon Unix socket")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	keychain := secret.NewKeychainStore()
	token, err := loadOrCreateToken(keychain)
	if err != nil {
		log.Fatalf("load daemon capability token: %v", err)
	}
	state, err := store.Open(ctx, *dataDirectory)
	if err != nil {
		log.Fatalf("open state store: %v", err)
	}
	defer state.Close()
	client := agent.NewResponsesClient(agent.KeychainAPIKey{Store: keychain})
	bus := events.New()
	runtime := agent.NewRuntime(state, client, bus)
	agents := multiagent.New(state, runtime, bus, 4)
	defer agents.Close()
	server := daemon.Server{SocketPath: *socketPath, Token: token, Runtime: runtime, Agents: agents}
	if err := server.Listen(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run daemon: %v", err)
	}
}

func loadOrCreateToken(keychain secret.Store) (string, error) {
	value, err := keychain.Get(agent.KeychainService, daemonTokenAccount)
	if err == nil && len(value) >= 32 {
		return string(value), nil
	}
	if err != nil && !errors.Is(err, secret.ErrNotFound) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := keychain.Set(agent.KeychainService, daemonTokenAccount, []byte(token)); err != nil {
		return "", err
	}
	return token, nil
}

func defaultDataDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "Aether")
	}
	return filepath.Join(home, "Library", "Application Support", "Aether")
}

func defaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aetherd.sock")
	}
	return filepath.Join(home, "Library", "Application Support", "Aether", "run", "aetherd.sock")
}

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.SetPrefix("aetherd ")
	_ = fmt.Sprintf
}
