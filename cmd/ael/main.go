package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eust-w/agentic-embedded-lab/internal/release"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "release" || os.Args[2] != "check" {
		usage()
		os.Exit(2)
	}
	flags := flag.NewFlagSet("ael release check", flag.ContinueOnError)
	profileName := flags.String("profile", string(release.Foundation), "foundation|simulation|software|production")
	workspace := flags.String("workspace", ".", "项目工作区")
	output := flags.String("output", "", "可选 JSON 报告路径")
	if err := flags.Parse(os.Args[3:]); err != nil {
		os.Exit(2)
	}
	profile := release.Profile(*profileName)
	switch profile {
	case release.Foundation, release.Simulation, release.Software, release.Production:
	default:
		fatal(fmt.Errorf("未知发布门 profile %q", *profileName))
	}
	result, err := release.Check(*workspace, profile)
	if err != nil {
		fatal(err)
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err)
	}
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
	if !result.Passed {
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: ael release check --profile foundation|simulation|software|production [--workspace PATH] [--output FILE]")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
