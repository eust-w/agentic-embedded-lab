package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/containerlab"
)

type extension struct{ ID, Slug, System string }
type runEvidence struct {
	Variant         string `json:"variant"`
	RunID           string `json:"run_id"`
	RunPath         string `json:"run_path"`
	TraceSHA256     string `json:"trace_sha256"`
	AssertionSHA256 string `json:"assertion_sha256"`
	ExpectedFailure bool   `json:"expected_failure"`
	Passed          bool   `json:"passed"`
	AssertionCount  int    `json:"assertion_count"`
	ArtifactCount   int    `json:"artifact_count"`
}

func main() {
	workspace := flag.String("workspace", ".", "workspace")
	determinismRepeats := flag.Int("determinism-repeats", 20, "fixed variant deterministic repetitions")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	fatal(err)
	revision := gitRevision(root)
	if revision == "" {
		fatal(errors.New("Git revision is required"))
	}
	if *determinismRepeats < 2 {
		fatal(errors.New("determinism-repeats must be at least 2"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	worker := filepath.Join(root, ".ael", "container-bin", "ael-backend")
	fatal(buildWorker(ctx, root, worker))
	lab := containerlab.Lab{Workspace: root, WorkerBinary: worker, RuntimeRoot: filepath.Join(root, ".ael", "container-runs"), Images: containerlab.DefaultImages()}
	items := []extension{{"rtl", "25-rtl-timer", "verilator-rtl"}, {"motor", "26-motor", "modelica-motor-extension"}, {"battery", "27-battery", "modelica-battery-extension"}, {"sensor", "28-sensor", "modelica-sensor-extension"}, {"pcb", "29-pcb", "ngspice-pcb-extension"}, {"emc", "30-emc", "ngspice-emc-extension"}}
	manifest := benchmark.AcceptanceManifest{APIVersion: ael.APIVersion, Profile: "simulation-extensions", SourceRevision: revision, CreatedAt: time.Now().UTC()}
	for _, item := range items {
		var checks []runEvidence
		fixedHashes := make([]string, 0, *determinismRepeats)
		fixedAssertionHashes := make([]string, 0, *determinismRepeats)
		fixedRuns := make([]string, 0, *determinismRepeats)
		for _, variant := range []string{"faulty", "fixed"} {
			system := item.System
			if item.ID == "rtl" {
				system += "-" + variant
			}
			experimentPath := fmt.Sprintf("benchmarks/v2/experiments/%s-%s.yaml", item.Slug, variant)
			systemPath := fmt.Sprintf("benchmarks/v2/systems/%s.yaml", system)
			bundle, path, runErr := lab.Run(ctx, experimentPath, systemPath, revision)
			failed := runErr != nil
			for _, assertion := range bundle.Assertions {
				failed = failed || !assertion.Passed
			}
			expectedFailure := variant == "faulty"
			relative, relErr := filepath.Rel(root, path)
			fatal(relErr)
			assertionHash, hashErr := benchmark.FileSHA256(filepath.Join(path, "assertions.json"))
			fatal(hashErr)
			checks = append(checks, runEvidence{Variant: variant, RunID: bundle.RunID, RunPath: filepath.ToSlash(relative), TraceSHA256: bundle.TraceSHA256, AssertionSHA256: assertionHash, ExpectedFailure: expectedFailure, Passed: failed == expectedFailure, AssertionCount: len(bundle.Assertions), ArtifactCount: len(bundle.Artifacts)})
			if variant == "fixed" {
				fixedHashes = append(fixedHashes, bundle.TraceSHA256)
				fixedAssertionHashes = append(fixedAssertionHashes, assertionHash)
				fixedRuns = append(fixedRuns, filepath.ToSlash(relative))
			}
		}
		for repeat := 1; repeat < *determinismRepeats; repeat++ {
			system := item.System
			if item.ID == "rtl" {
				system += "-fixed"
			}
			experimentPath := fmt.Sprintf("benchmarks/v2/experiments/%s-fixed.yaml", item.Slug)
			systemPath := fmt.Sprintf("benchmarks/v2/systems/%s.yaml", system)
			bundle, runPath, runErr := lab.Run(ctx, experimentPath, systemPath, revision)
			fatal(runErr)
			for _, assertion := range bundle.Assertions {
				if !assertion.Passed {
					fatal(fmt.Errorf("extension %s deterministic repeat %d assertion %s failed", item.ID, repeat, assertion.ID))
				}
			}
			relative, relErr := filepath.Rel(root, runPath)
			fatal(relErr)
			assertionHash, hashErr := benchmark.FileSHA256(filepath.Join(runPath, "assertions.json"))
			fatal(hashErr)
			fixedHashes = append(fixedHashes, bundle.TraceSHA256)
			fixedAssertionHashes = append(fixedAssertionHashes, assertionHash)
			fixedRuns = append(fixedRuns, filepath.ToSlash(relative))
		}
		deterministic := allEqual(fixedHashes) && allEqual(fixedAssertionHashes)
		path := filepath.Join(root, "acceptance", "v2", "evidence", "extension-"+item.ID+".json")
		payload := map[string]any{"api_version": ael.APIVersion, "id": item.ID, "source_revision": revision, "checks": checks, "determinism": map[string]any{"repeats": *determinismRepeats, "trace_hashes": fixedHashes, "assertion_hashes": fixedAssertionHashes, "run_paths": fixedRuns, "all_equal": deterministic}, "hardware_validated": false}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fatal(os.MkdirAll(filepath.Dir(path), 0o700))
		fatal(os.WriteFile(path, append(data, '\n'), 0o600))
		hash, err := benchmark.FileSHA256(path)
		fatal(err)
		passed := true
		for _, check := range checks {
			passed = passed && check.Passed
		}
		passed = passed && deterministic
		status := "failed"
		if passed {
			status = "passed"
		}
		manifest.Entries = append(manifest.Entries, benchmark.AcceptanceEntry{Name: "extension:" + item.ID, Status: status, EvidencePath: "acceptance/v2/evidence/extension-" + item.ID + ".json", EvidenceSHA256: hash, Limitations: []string{"Functional simulation evidence only; hardware calibration is not implied."}})
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	output := filepath.Join(root, "acceptance", "v2", "extensions.json")
	fatal(os.WriteFile(output, append(data, '\n'), 0o600))
	for _, entry := range manifest.Entries {
		if entry.Status != "passed" {
			os.Exit(2)
		}
	}
	fmt.Printf("extensions=%d passed=true revision=%s\n", len(manifest.Entries), revision[:12])
}

func allEqual(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values[1:] {
		if value != values[0] {
			return false
		}
	}
	return true
}

func buildWorker(ctx context.Context, root, output string) error {
	_ = os.MkdirAll(filepath.Dir(output), 0o700)
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", output, "./cmd/ael-backend")
	command.Dir = root
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	data, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build worker: %w: %s", err, string(data))
	}
	return nil
}
func gitRevision(root string) string {
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
