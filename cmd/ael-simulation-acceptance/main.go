package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/containerlab"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func main() {
	workspace := flag.String("workspace", ".", "workspace")
	revision := flag.String("revision", "", "source Git revision; defaults to HEAD")
	rebuild := flag.Bool("rebuild", false, "rebuild existing firmware")
	parallel := flag.Int("parallel-builds", 2, "parallel firmware builds")
	determinismRepeats := flag.Int("determinism-repeats", 20, "fixed-variant repetitions for every benchmark")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	fatal(err)
	if *revision == "" {
		*revision = gitRevision(root)
	}
	if *revision == "" || *revision == "working-tree" {
		fatal(errors.New("a concrete Git source revision is required"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	lab := containerlab.Lab{Workspace: root, WorkerBinary: filepath.Join(root, ".ael", "container-bin", "ael-backend"), RuntimeRoot: filepath.Join(root, ".ael", "container-runs"), Images: containerlab.DefaultImages()}
	cases := []int{}
	for id := 4; id <= 17; id++ {
		cases = append(cases, id)
	}
	cases = append(cases, 19, 21, 23, 24)
	jobs := make(chan buildJob)
	failures := make(chan error, len(cases)*2)
	var wait sync.WaitGroup
	for worker := 0; worker < max(1, *parallel); worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for job := range jobs {
				elf := filepath.Join(root, ".ael", "firmware-builds", fmt.Sprintf("build-case%02d-%s", job.id, job.variant), "zephyr", "zephyr.elf")
				if job.id == 17 {
					elf = filepath.Join(root, ".ael", "firmware-builds", fmt.Sprintf("build-case%02d-%s", job.id, job.variant), "merged.hex")
				}
				if !*rebuild {
					if _, err := os.Stat(elf); err == nil {
						continue
					}
				}
				if err := lab.BuildFirmware(ctx, job.id, job.variant); err != nil {
					failures <- err
				}
			}
		}()
	}
	for _, id := range cases {
		for _, variant := range []string{"faulty", "fixed"} {
			jobs <- buildJob{id, variant}
		}
	}
	close(jobs)
	wait.Wait()
	close(failures)
	var buildErrors []error
	for err := range failures {
		buildErrors = append(buildErrors, err)
	}
	if len(buildErrors) > 0 {
		fatal(errors.Join(buildErrors...))
	}
	riscvELF := filepath.Join(root, ".ael", "firmware-builds", "build-hifive1", "zephyr", "zephyr.elf")
	if *rebuild {
		fatal(lab.BuildRISCvFirmware(ctx))
	} else if _, err := os.Stat(riscvELF); err != nil {
		fatal(lab.BuildRISCvFirmware(ctx))
	}
	catalog, err := benchmark.Load(root, "benchmarks/v2/catalog.yaml")
	fatal(err)
	runner := benchmark.BenchmarkRunner{Workspace: root, ProjectID: "containerlab", Execute: lab.Run}
	manifest, err := runner.Run(ctx, catalog, nil, *revision)
	fatal(err)
	if *determinismRepeats < 2 {
		fatal(errors.New("determinism-repeats must be at least 2"))
	}
	matrix := map[string]any{}
	deterministic := true
	for _, item := range catalog.Cases {
		prefix := fmt.Sprintf("%02d-%s", item.ID, item.Slug)
		experimentPath := "benchmarks/v2/experiments/" + prefix + "-fixed.yaml"
		experiment, err := ael.LoadExperiment(root, experimentPath)
		fatal(err)
		systemPath, err := findSystem(root, experiment.SystemID)
		fatal(err)
		hashes := make([]string, 0, *determinismRepeats)
		runs := make([]string, 0, *determinismRepeats)
		for repeat := 0; repeat < *determinismRepeats; repeat++ {
			bundle, evidence, err := lab.Run(ctx, experimentPath, systemPath, *revision)
			fatal(err)
			hashes = append(hashes, bundle.TraceSHA256)
			relative, err := filepath.Rel(root, evidence)
			fatal(err)
			runs = append(runs, filepath.ToSlash(relative))
			if repeat > 0 && hashes[repeat] != hashes[0] {
				deterministic = false
			}
		}
		matrix[prefix] = map[string]any{"seed": experiment.Seed, "trace_hashes": hashes, "run_paths": runs, "all_equal": allEqual(hashes)}
	}
	determinismPath := filepath.Join(root, "acceptance", "v2", "evidence", "determinism-trace-20.json")
	determinismPayload := map[string]any{"api_version": ael.APIVersion, "source_revision": *revision, "benchmark_count": len(catalog.Cases), "repeats_per_benchmark": *determinismRepeats, "matrix": matrix, "all_equal": deterministic, "hardware_validated": false}
	data, _ := json.MarshalIndent(determinismPayload, "", "  ")
	fatal(os.MkdirAll(filepath.Dir(determinismPath), 0o700))
	fatal(os.WriteFile(determinismPath, append(data, '\n'), 0o600))
	hash, err := benchmark.FileSHA256(determinismPath)
	fatal(err)
	relative, _ := filepath.Rel(root, determinismPath)
	status := "failed"
	if deterministic {
		status = "passed"
	}
	manifest.Entries = append(manifest.Entries, benchmark.AcceptanceEntry{Name: "determinism:trace-20", Status: status, EvidencePath: filepath.ToSlash(relative), EvidenceSHA256: hash, Limitations: []string{"Quantized simulation trace determinism only; no hardware timing equivalence."}})
	if entry, err := currentFMIEntry(root, *revision); err == nil {
		manifest.Entries = append(manifest.Entries, entry)
	} else {
		fatal(err)
	}
	manifestPath := filepath.Join(root, "acceptance", "v2", "simulation.json")
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	fatal(os.WriteFile(manifestPath, append(manifestData, '\n'), 0o600))
	passed := true
	for _, entry := range manifest.Entries {
		passed = passed && entry.Status == "passed"
	}
	fmt.Printf("simulation entries=%d passed=%t\n", len(manifest.Entries), passed)
	if !passed {
		os.Exit(2)
	}
}

func findSystem(root, id string) (string, error) {
	base := filepath.Join(root, "benchmarks", "v2", "systems")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		relative := filepath.ToSlash(filepath.Join("benchmarks", "v2", "systems", entry.Name()))
		system, err := ael.LoadSystem(root, relative)
		if err == nil && system.ID == id {
			return relative, nil
		}
	}
	return "", fmt.Errorf("system %s is missing", id)
}

func allEqual(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index] != values[0] {
			return false
		}
	}
	return len(values) > 0
}

func currentFMIEntry(root, revision string) (benchmark.AcceptanceEntry, error) {
	path := filepath.Join(root, "acceptance", "v2", "evidence", "fmi-five-domain.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return benchmark.AcceptanceEntry{}, errors.New("FMI evidence is missing; run cmd/ael-fmi-acceptance")
	}
	var evidence struct {
		Status         string `json:"status"`
		SourceRevision string `json:"source_revision"`
	}
	if json.Unmarshal(data, &evidence) != nil || evidence.Status != "passed" || evidence.SourceRevision != revision {
		return benchmark.AcceptanceEntry{}, errors.New("FMI evidence is stale for the requested source revision")
	}
	hash, err := benchmark.FileSHA256(path)
	return benchmark.AcceptanceEntry{Name: "fmi:five-domain", Status: "passed", EvidencePath: "acceptance/v2/evidence/fmi-five-domain.json", EvidenceSHA256: hash, Limitations: []string{"FMI 2.0 functional exchange only; no calibrated hardware equivalence."}}, err
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

type buildJob struct {
	id      int
	variant string
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
