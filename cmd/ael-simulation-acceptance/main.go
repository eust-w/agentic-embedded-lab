package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/containerlab"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func main() {
	workspace := flag.String("workspace", ".", "workspace")
	revision := flag.String("revision", "working-tree", "revision")
	rebuild := flag.Bool("rebuild", false, "rebuild existing firmware")
	parallel := flag.Int("parallel-builds", 2, "parallel firmware builds")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	fatal(err)
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
	hashes := make([]string, 0, 20)
	evidencePaths := make([]string, 0, 20)
	deterministic := true
	for repeat := 0; repeat < 20; repeat++ {
		bundle, evidence, err := lab.Run(ctx, "benchmarks/v2/experiments/18-ldo-dropout-fixed.yaml", "benchmarks/v2/systems/ngspice-power.yaml", *revision)
		if err != nil {
			fatal(err)
		}
		hashes = append(hashes, bundle.TraceSHA256)
		evidencePaths = append(evidencePaths, evidence)
		if repeat > 0 && hashes[repeat] != hashes[0] {
			deterministic = false
		}
	}
	determinismPath := filepath.Join(root, "acceptance", "v2", "evidence", "determinism-trace-20.json")
	determinismPayload := map[string]any{"api_version": "ael.dev/v2", "experiment": "18-ldo-dropout-fixed", "seed": 1018, "repeats": 20, "trace_hashes": hashes, "evidence_paths": evidencePaths, "all_equal": deterministic, "hardware_validated": false}
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
