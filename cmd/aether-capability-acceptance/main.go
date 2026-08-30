package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
	"github.com/eust-w/agentic-embedded-lab/internal/packaging"
)

type check struct {
	ID       string
	Commands [][]string
}

func main() {
	root, _ := filepath.Abs(".")
	revision := gitRevision(root)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	checks := []check{
		{"desktop.shell", [][]string{{"go", "test", "./internal/app", "./internal/daemon", "./internal/store"}, {"npm", "--prefix", "frontend", "run", "lint"}, {"npm", "--prefix", "frontend", "run", "test"}, {"npm", "--prefix", "frontend", "run", "build"}}},
		{"agent.runtime", [][]string{{"go", "test", "./internal/agent", "./internal/memory", "./internal/instructions"}}},
		{"engineering.git-terminal", [][]string{{"go", "test", "./internal/git", "./internal/terminal", "./internal/executor"}}},
		{"security.approval-sandbox", [][]string{{"go", "test", "./internal/approval", "./internal/executor", "./internal/secret"}}},
		{"customization.plugins-mcp", [][]string{{"go", "test", "./internal/plugins", "./internal/mcp", "./internal/tools"}}},
		{"multiagent.automation", [][]string{{"go", "test", "./internal/multiagent", "./internal/automation"}}},
		{"browser.computer-use", [][]string{{"go", "test", "./internal/browser", "./internal/computeruse"}}},
		{"modeling.behavior-ir", [][]string{{"go", "test", "./internal/ael/modeling"}}},
		{"server.worker-storage", [][]string{{"go", "test", "./internal/server", "./internal/store"}}},
	}
	manifest := benchmark.AcceptanceManifest{APIVersion: ael.APIVersion, Profile: "capabilities", SourceRevision: revision, CreatedAt: time.Now().UTC()}
	for _, item := range checks {
		passed, output := runAll(ctx, root, item.Commands)
		path := filepath.Join(root, "acceptance", "v2", "evidence", "capability-"+strings.ReplaceAll(item.ID, ".", "-")+".json")
		payload := map[string]any{"api_version": ael.APIVersion, "id": item.ID, "status": status(passed), "source_revision": revision, "commands": item.Commands, "output": output}
		data, _ := json.MarshalIndent(payload, "", "  ")
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		_ = os.WriteFile(path, append(data, '\n'), 0o600)
		hash, _ := benchmark.FileSHA256(path)
		manifest.Entries = append(manifest.Entries, benchmark.AcceptanceEntry{Name: item.ID, Status: status(passed), EvidencePath: relative(root, path), EvidenceSHA256: hash})
	}
	path := filepath.Join(root, "acceptance", "v2", "capabilities.json")
	data, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(path, append(data, '\n'), 0o600)
	packageReport := packaging.CheckBundle(filepath.Join(root, "build/bin/Aether Desktop.app"), false)
	packageData, _ := json.MarshalIndent(packageReport, "", "  ")
	_ = os.WriteFile(filepath.Join(root, "acceptance", "v2", "desktop-development.json"), append(packageData, '\n'), 0o600)
	for _, entry := range manifest.Entries {
		if entry.Status != "passed" {
			os.Exit(2)
		}
	}
	if !packageReport.Passed {
		os.Exit(2)
	}
	fmt.Printf("capabilities=%d package=true passed=true\n", len(manifest.Entries))
}
func runAll(ctx context.Context, root string, commands [][]string) (bool, []string) {
	passed := true
	outputs := []string{}
	for _, args := range commands {
		command := exec.CommandContext(ctx, args[0], args[1:]...)
		command.Dir = root
		command.Env = append(os.Environ(), "GOCACHE=/private/tmp/ael-go-cache", "GOTOOLCHAIN=local")
		data, err := command.CombinedOutput()
		outputs = append(outputs, strings.Join(args, " ")+"\n"+string(data))
		passed = passed && err == nil
	}
	return passed, outputs
}
func status(value bool) string {
	if value {
		return "passed"
	}
	return "failed"
}
func relative(root, path string) string {
	value, _ := filepath.Rel(root, path)
	return filepath.ToSlash(value)
}
func gitRevision(root string) string {
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	data, _ := command.Output()
	return strings.TrimSpace(string(data))
}
