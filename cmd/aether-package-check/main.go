package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eust-w/agentic-embedded-lab/internal/packaging"
)

func main() {
	app := flag.String("app", "build/bin/Aether Desktop.app", "Aether Desktop app bundle")
	release := flag.Bool("release", false, "require release signing, Chromium, Sparkle and notarization prerequisites")
	output := flag.String("output", "", "optional JSON report path")
	flag.Parse()
	absolute, err := filepath.Abs(*app)
	if err != nil {
		fatal(err)
	}
	report := packaging.CheckBundle(absolute, *release)
	payload, _ := json.MarshalIndent(report, "", "  ")
	if *output != "" {
		path, err := filepath.Abs(*output)
		if err != nil {
			fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
			fatal(err)
		}
	}
	fmt.Println(string(payload))
	if err := report.Error(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
