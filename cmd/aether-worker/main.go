package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eust-w/agentic-embedded-lab/internal/server"
)

func main() {
	config := server.WorkerConfig{}
	capabilities := flag.String("capabilities", "health_probe", "comma-separated capabilities")
	flag.StringVar(&config.ControlPlane, "control-plane", "", "control plane URL")
	flag.StringVar(&config.Cert, "cert", "", "worker certificate")
	flag.StringVar(&config.Key, "key", "", "worker key")
	flag.StringVar(&config.CA, "ca", "", "CA certificate")
	flag.StringVar(&config.ID, "worker-id", "", "worker id")
	flag.StringVar(&config.Workspace, "workspace", envOr("AEL_WORKSPACE", "/workspace"), "workspace")
	flag.StringVar(&config.BackendExecutable, "ael-backend", "/usr/local/bin/ael-backend", "AEL backend executable")
	flag.Parse()
	for _, item := range strings.Split(*capabilities, ",") {
		if value := strings.TrimSpace(item); value != "" {
			config.Capabilities = append(config.Capabilities, value)
		}
	}
	worker, err := server.NewWorker(config)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
