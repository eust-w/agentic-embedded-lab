package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/server"
)

func main() {
	mode := flag.String("mode", "control-plane", "control-plane or dev-oidc")
	address := flag.String("listen", ":8765", "listen address")
	probe := flag.String("probe", "", "HTTP health URL to probe and exit")
	flag.Parse()
	if *probe != "" {
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get(*probe)
		if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			if err != nil {
				log.Print(err)
			}
			os.Exit(1)
		}
		response.Body.Close()
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var handler http.Handler
	var closeFn func() error
	switch *mode {
	case "control-plane":
		control, err := server.Open(ctx, server.ConfigFromEnv())
		if err != nil {
			log.Fatal(err)
		}
		handler = control.Handler()
		closeFn = control.Close
	case "dev-oidc":
		issuer := os.Getenv("AEL_DEV_OIDC_ISSUER")
		audience := os.Getenv("AEL_DEV_OIDC_AUDIENCE")
		provider, err := server.NewDevOIDC(issuer, audience)
		if err != nil {
			log.Fatal(err)
		}
		handler = provider.Handler()
	default:
		log.Fatalf("unsupported mode %s", *mode)
	}
	httpServer := &http.Server{Addr: *address, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	if closeFn != nil {
		defer closeFn()
	}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
