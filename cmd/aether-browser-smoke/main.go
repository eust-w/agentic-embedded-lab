package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/browser"
	"github.com/eust-w/agentic-embedded-lab/internal/store"
)

func main() {
	executable := flag.String("chromium", "", "absolute bundled Chromium executable")
	target := flag.String("url", "http://127.0.0.1:8877/", "local test URL")
	flag.Parse()
	if !filepath.IsAbs(*executable) {
		fatal(fmt.Errorf("absolute --chromium path is required"))
	}
	root, err := os.MkdirTemp("", "aether-browser-smoke-")
	fatal(err)
	defer os.RemoveAll(root)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	state, err := store.Open(ctx, filepath.Join(root, "state"))
	fatal(err)
	defer state.Close()
	controller := &browser.Controller{Executable: *executable, ProfilePath: filepath.Join(root, "profile"), Permissions: browser.NewPermissionStore(state)}
	fatal(controller.Start(ctx))
	defer controller.Stop()
	fatal(controller.Navigate(ctx, *target))
	dom, err := controller.DOM(ctx)
	fatal(err)
	screenshot, err := controller.Screenshot(ctx)
	fatal(err)
	report := map[string]any{
		"status":           controller.Status(),
		"dom_bytes":        len(dom),
		"screenshot_bytes": len(screenshot),
		"console":          controller.Console(0),
		"network":          controller.Network(0),
	}
	payload, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(payload))
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
