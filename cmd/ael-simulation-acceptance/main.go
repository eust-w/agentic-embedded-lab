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
	parallelRuns := flag.Int("parallel-runs", 4, "parallel deterministic experiment runs")
	determinismRepeats := flag.Int("determinism-repeats", 2, "fixed-variant repetitions for every benchmark")
	stressRepeats := flag.Int("determinism-stress-repeats", 20, "repetitions for representative backend stress cases")
	finalizeOnly := flag.Bool("finalize-only", false, "assemble completed simulation, FMI, and determinism evidence without rerunning")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	fatal(err)
	if *revision == "" {
		*revision = gitRevision(root)
	}
	if *revision == "" || *revision == "working-tree" {
		fatal(errors.New("a concrete Git source revision is required"))
	}
	if *finalizeOnly {
		fatal(finalizeExisting(root, *revision))
		fmt.Println("simulation evidence finalized passed=true")
		return
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
	if *determinismRepeats < 2 || *stressRepeats < *determinismRepeats {
		fatal(errors.New("determinism-repeats must be at least 2"))
	}
	stressCases := map[int]bool{4: true, 18: true, 21: true, 22: true, 24: true}
	type traceSet struct {
		seed   int64
		hashes []string
		runs   []string
	}
	sets := map[string]*traceSet{}
	type deterministicJob struct {
		prefix, experimentPath, systemPath string
		repeat                             int
	}
	determinismJobs := make(chan deterministicJob)
	determinismErrors := make(chan error, len(catalog.Cases)**stressRepeats)
	var determinismWait sync.WaitGroup
	var determinismMu sync.Mutex
	for worker := 0; worker < max(1, *parallelRuns); worker++ {
		determinismWait.Add(1)
		go func() {
			defer determinismWait.Done()
			for job := range determinismJobs {
				bundle, evidence, err := lab.Run(ctx, job.experimentPath, job.systemPath, *revision)
				if err != nil {
					determinismErrors <- fmt.Errorf("determinism %s repeat %d: %w", job.prefix, job.repeat, err)
					continue
				}
				relative, err := filepath.Rel(root, evidence)
				if err != nil {
					determinismErrors <- err
					continue
				}
				determinismMu.Lock()
				sets[job.prefix].hashes[job.repeat] = bundle.TraceSHA256
				sets[job.prefix].runs[job.repeat] = filepath.ToSlash(relative)
				determinismMu.Unlock()
			}
		}()
	}
	for _, item := range catalog.Cases {
		prefix := fmt.Sprintf("%02d-%s", item.ID, item.Slug)
		experimentPath := "benchmarks/v2/experiments/" + prefix + "-fixed.yaml"
		experiment, err := ael.LoadExperiment(root, experimentPath)
		fatal(err)
		systemPath, err := findSystem(root, experiment.SystemID)
		fatal(err)
		repeatCount := *determinismRepeats
		if stressCases[item.ID] {
			repeatCount = *stressRepeats
		}
		sets[prefix] = &traceSet{seed: experiment.Seed, hashes: make([]string, repeatCount), runs: make([]string, repeatCount)}
		for repeat := 0; repeat < repeatCount; repeat++ {
			determinismJobs <- deterministicJob{prefix: prefix, experimentPath: experimentPath, systemPath: systemPath, repeat: repeat}
		}
	}
	close(determinismJobs)
	determinismWait.Wait()
	close(determinismErrors)
	var runErrors []error
	for err := range determinismErrors {
		runErrors = append(runErrors, err)
	}
	if len(runErrors) > 0 {
		fatal(errors.Join(runErrors...))
	}
	matrix := map[string]any{}
	deterministic := true
	for prefix, set := range sets {
		equal := allEqual(set.hashes)
		deterministic = deterministic && equal
		matrix[prefix] = map[string]any{"seed": set.seed, "trace_hashes": set.hashes, "run_paths": set.runs, "all_equal": equal}
	}
	determinismPath := filepath.Join(root, "acceptance", "v2", "evidence", "determinism-trace-matrix.json")
	determinismPayload := map[string]any{"api_version": ael.APIVersion, "source_revision": *revision, "benchmark_count": len(catalog.Cases), "base_repeats": *determinismRepeats, "stress_repeats": *stressRepeats, "stress_case_ids": []int{4, 18, 21, 22, 24}, "matrix": matrix, "all_equal": deterministic, "hardware_validated": false}
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
	manifest.Entries = append(manifest.Entries, benchmark.AcceptanceEntry{Name: "determinism:trace-matrix", Status: status, EvidencePath: filepath.ToSlash(relative), EvidenceSHA256: hash, Limitations: []string{"All 24 fixed cases are repeated; representative five-backend cases run 20 times. Quantized simulation determinism only."}})
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
	if json.Unmarshal(data, &evidence) != nil || evidence.Status != "passed" || !sameRevision(evidence.SourceRevision, revision) {
		return benchmark.AcceptanceEntry{}, errors.New("FMI evidence is stale for the requested source revision")
	}
	hash, err := benchmark.FileSHA256(path)
	return benchmark.AcceptanceEntry{Name: "fmi:five-domain", Status: "passed", EvidencePath: "acceptance/v2/evidence/fmi-five-domain.json", EvidenceSHA256: hash, Limitations: []string{"FMI 2.0 functional exchange only; no calibrated hardware equivalence."}}, err
}

func finalizeExisting(root, revision string) error {
	manifestPath := filepath.Join(root, "acceptance", "v2", "simulation.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest benchmark.AcceptanceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if !sameRevision(manifest.SourceRevision, revision) {
		return errors.New("simulation manifest source revision does not match")
	}
	determinismPath := filepath.Join(root, "acceptance", "v2", "evidence", "determinism-trace-matrix.json")
	data, err = os.ReadFile(determinismPath)
	if err != nil {
		return err
	}
	var matrix struct {
		SourceRevision string `json:"source_revision"`
		AllEqual       bool   `json:"all_equal"`
		CaseCount      int    `json:"benchmark_count"`
	}
	if json.Unmarshal(data, &matrix) != nil || !matrix.AllEqual || matrix.CaseCount != 24 || !sameRevision(matrix.SourceRevision, revision) {
		return errors.New("determinism matrix is incomplete, unequal, or stale")
	}
	determinismHash, err := benchmark.FileSHA256(determinismPath)
	if err != nil {
		return err
	}
	fmiEntry, err := currentFMIEntry(root, revision)
	if err != nil {
		return err
	}
	entries := manifest.Entries[:0]
	for _, entry := range manifest.Entries {
		if entry.Name != "fmi:five-domain" && entry.Name != "determinism:trace-matrix" && entry.Name != "determinism:trace-20" {
			entries = append(entries, entry)
		}
	}
	manifest.Entries = append(entries, benchmark.AcceptanceEntry{Name: "determinism:trace-matrix", Status: "passed", EvidencePath: "acceptance/v2/evidence/determinism-trace-matrix.json", EvidenceSHA256: determinismHash, Limitations: []string{"All 24 fixed cases are repeated; representative five-backend cases run 20 times. Quantized simulation determinism only."}}, fmiEntry)
	manifest.SourceRevision = revision
	manifest.CreatedAt = time.Now().UTC()
	data, _ = json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(manifestPath, append(data, '\n'), 0o600)
}

func sameRevision(left, right string) bool {
	return left == right || len(left) >= 7 && len(right) >= 7 && (strings.HasPrefix(left, right) || strings.HasPrefix(right, left))
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
