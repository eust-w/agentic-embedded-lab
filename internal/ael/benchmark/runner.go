package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

type VariantResult struct {
	Variant           string        `json:"variant"`
	RunID             string        `json:"run_id"`
	Status            ael.RunStatus `json:"status"`
	ExpectedStatus    ael.RunStatus `json:"expected_status"`
	EvidencePath      string        `json:"evidence_path"`
	MechanismEvidence []string      `json:"mechanism_evidence"`
	Passed            bool          `json:"passed"`
	Error             string        `json:"error,omitempty"`
}

type CaseEvidence struct {
	APIVersion        string            `json:"api_version"`
	Benchmark         string            `json:"benchmark"`
	Checks            []VariantResult   `json:"checks"`
	AssetHashes       map[string]string `json:"asset_hashes"`
	CausalChain       []string          `json:"causal_chain"`
	FidelityBoundary  string            `json:"fidelity_boundary"`
	HardwareValidated bool              `json:"hardware_validated"`
	Mechanism         Mechanism         `json:"mechanism"`
}

type AcceptanceEntry struct {
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	EvidencePath   string   `json:"evidence_path"`
	EvidenceSHA256 string   `json:"evidence_sha256"`
	Limitations    []string `json:"limitations"`
}
type AcceptanceManifest struct {
	APIVersion     string            `json:"api_version"`
	Profile        string            `json:"profile"`
	SourceRevision string            `json:"source_revision"`
	Entries        []AcceptanceEntry `json:"entries"`
	CreatedAt      time.Time         `json:"created_at"`
}

type BenchmarkRunner struct {
	Workspace string
	ProjectID string
	Runs      *ael.RunManager
	Execute   func(context.Context, string, string, string) (ael.EvidenceBundle, string, error)
}

func (r BenchmarkRunner) Run(ctx context.Context, catalog Catalog, selected map[int]bool, sourceRevision string) (AcceptanceManifest, error) {
	if r.Runs == nil && r.Execute == nil {
		return AcceptanceManifest{}, errors.New("AEL run manager is required")
	}
	if failures := catalog.Validate(r.Workspace); len(failures) > 0 {
		return AcceptanceManifest{}, errors.Join(failures...)
	}
	systemPaths, err := systemPathIndex(r.Workspace)
	if err != nil {
		return AcceptanceManifest{}, err
	}
	manifest := AcceptanceManifest{APIVersion: ael.APIVersion, Profile: "simulation", SourceRevision: sourceRevision, CreatedAt: time.Now().UTC()}
	for _, item := range catalog.Cases {
		if len(selected) > 0 && !selected[item.ID] {
			continue
		}
		entry, err := r.runCase(ctx, item, systemPaths, sourceRevision)
		if err != nil {
			return manifest, err
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	for _, entry := range manifest.Entries {
		if entry.Name == "benchmark:24-antenna-cross-domain" {
			manifest.Entries = append(manifest.Entries, AcceptanceEntry{Name: "cross-domain:five-backend", Status: entry.Status, EvidencePath: entry.EvidencePath, EvidenceSHA256: entry.EvidenceSHA256, Limitations: []string{"Five-domain tool-executed simulation only; no calibrated hardware equivalence."}})
			break
		}
	}
	if len(selected) == 0 {
		manifest.Entries = append(manifest.Entries, r.backendEntries(catalog, manifest.Entries)...)
		architecture, err := r.architectureEntry(ctx, systemPaths, sourceRevision)
		if err != nil {
			return manifest, err
		}
		manifest.Entries = append(manifest.Entries, architecture)
		qualified := true
		backendHashes := map[string]string{}
		for _, entry := range manifest.Entries {
			if strings.HasPrefix(entry.Name, "backend:") {
				qualified = qualified && entry.Status == "passed"
				backendHashes[entry.Name] = entry.EvidenceSHA256
			}
		}
		environmentPath := filepath.Join(r.Workspace, "acceptance", "v2", "evidence", "environment-simulation.json")
		environment := map[string]any{"api_version": ael.APIVersion, "qualified": qualified, "source_revision": sourceRevision, "backend_evidence": backendHashes, "versions": map[string]string{"zephyr": "4.4.2", "renode": "1.16.1", "ngspice": "46", "openmodelica": "1.27.0", "ns3": "3.47", "openems": "0.0.36"}, "hardware_validated": false}
		if err := writeAcceptance(environmentPath, environment); err != nil {
			return manifest, err
		}
		hash, _ := FileSHA256(environmentPath)
		relative, _ := filepath.Rel(r.Workspace, environmentPath)
		manifest.Entries = append(manifest.Entries, AcceptanceEntry{Name: "environment:qualified", Status: passStatus(qualified), EvidencePath: filepath.ToSlash(relative), EvidenceSHA256: hash, Limitations: []string{"Version-enforced simulation environment only; no hardware equivalence."}})
	}
	path := filepath.Join(r.Workspace, "acceptance", "v2", "simulation.json")
	if err := writeAcceptance(path, manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func (r BenchmarkRunner) backendEntries(catalog Catalog, entries []AcceptanceEntry) []AcceptanceEntry {
	entryMap := map[string]AcceptanceEntry{}
	for _, entry := range entries {
		entryMap[entry.Name] = entry
	}
	backends := []string{"zephyr_build", "renode", "ngspice", "openmodelica", "ns3", "openems"}
	var result []AcceptanceEntry
	for _, backend := range backends {
		cases := []string{}
		hashes := map[string]string{}
		passed := true
		for _, item := range catalog.Cases {
			if !contains(item.Backends, backend) {
				continue
			}
			name := fmt.Sprintf("benchmark:%02d-%s", item.ID, item.Slug)
			entry := entryMap[name]
			cases = append(cases, name)
			hashes[name] = entry.EvidenceSHA256
			passed = passed && entry.Status == "passed"
		}
		path := filepath.Join(r.Workspace, "acceptance", "v2", "evidence", "backend-"+backend+".json")
		payload := map[string]any{"api_version": ael.APIVersion, "backend": backend, "cases": cases, "case_evidence_sha256": hashes, "status": passStatus(passed), "hardware_validated": false}
		_ = writeAcceptance(path, payload)
		hash, _ := FileSHA256(path)
		relative, _ := filepath.Rel(r.Workspace, path)
		result = append(result, AcceptanceEntry{Name: "backend:" + backend, Status: passStatus(passed), EvidencePath: filepath.ToSlash(relative), EvidenceSHA256: hash, Limitations: []string{"Tool-executed backend conformance only; no hardware equivalence."}})
	}
	return result
}

func (r BenchmarkRunner) architectureEntry(ctx context.Context, systems map[string]string, revision string) (AcceptanceEntry, error) {
	if r.Execute == nil {
		return AcceptanceEntry{}, errors.New("architecture acceptance requires a direct executor")
	}
	experimentPath := "benchmarks/v2/experiments/riscv-smoke.yaml"
	experiment, err := ael.LoadExperiment(r.Workspace, experimentPath)
	if err != nil {
		return AcceptanceEntry{}, err
	}
	systemPath := systems[experiment.SystemID]
	bundle, evidence, runErr := r.Execute(ctx, experimentPath, systemPath, revision)
	evidence, err = relativeRunPath(r.Workspace, evidence)
	if err != nil {
		return AcceptanceEntry{}, err
	}
	passed := runErr == nil
	for _, assertion := range bundle.Assertions {
		passed = passed && assertion.Passed
	}
	firmware := map[string]string{}
	for name, path := range map[string]string{"cortex_m": filepath.Join(r.Workspace, ".ael", "firmware-builds", "build-case24-fixed", "zephyr", "zephyr.elf"), "riscv": filepath.Join(r.Workspace, ".ael", "firmware-builds", "build-hifive1", "zephyr", "zephyr.elf")} {
		hash, err := FileSHA256(path)
		if err != nil {
			return AcceptanceEntry{}, err
		}
		firmware[name] = hash
	}
	path := filepath.Join(r.Workspace, "acceptance", "v2", "evidence", "firmware-arm-riscv.json")
	payload := map[string]any{"api_version": ael.APIVersion, "zephyr_version": "4.4.2", "zephyr_sdk_version": "1.0.1", "firmware_sha256": firmware, "riscv_run_id": bundle.RunID, "riscv_trace_sha256": bundle.TraceSHA256, "riscv_evidence_path": evidence, "status": passStatus(passed), "hardware_validated": false}
	if err := writeAcceptance(path, payload); err != nil {
		return AcceptanceEntry{}, err
	}
	hash, _ := FileSHA256(path)
	relative, _ := filepath.Rel(r.Workspace, path)
	return AcceptanceEntry{Name: "firmware:arm-riscv", Status: passStatus(passed), EvidencePath: filepath.ToSlash(relative), EvidenceSHA256: hash, Limitations: []string{"Cross-compilation and virtual boot only; no physical CPU equivalence."}}, nil
}
func passStatus(value bool) string {
	if value {
		return "passed"
	}
	return "failed"
}

func (r BenchmarkRunner) runCase(ctx context.Context, item Case, systemPaths map[string]string, sourceRevision string) (AcceptanceEntry, error) {
	prefix := fmt.Sprintf("%02d-%s", item.ID, item.Slug)
	evidence := CaseEvidence{APIVersion: ael.APIVersion, Benchmark: prefix, AssetHashes: map[string]string{}, CausalChain: item.CausalChain, FidelityBoundary: item.FidelityBoundary, HardwareValidated: false, Mechanism: item.Mechanism}
	for label, assets := range map[string][]string{"faulty": item.Mechanism.FaultyAssets, "fixed": item.Mechanism.FixedAssets} {
		var hashes []string
		for _, asset := range assets {
			path, err := resolveExisting(r.Workspace, asset)
			if err != nil {
				return AcceptanceEntry{}, err
			}
			hash, err := FileSHA256(path)
			if err != nil {
				return AcceptanceEntry{}, err
			}
			hashes = append(hashes, hash)
		}
		sort.Strings(hashes)
		evidence.AssetHashes[label] = strings.Join(hashes, ",")
	}
	for _, variant := range []string{"faulty", "fixed"} {
		experimentPath := filepath.ToSlash(filepath.Join("benchmarks", "v2", "experiments", prefix+"-"+variant+".yaml"))
		experiment, err := ael.LoadExperiment(r.Workspace, experimentPath)
		if err != nil {
			return AcceptanceEntry{}, err
		}
		systemPath := systemPaths[experiment.SystemID]
		if systemPath == "" {
			return AcceptanceEntry{}, fmt.Errorf("system path for %s is missing", experiment.SystemID)
		}
		var record ael.RunRecord
		if r.Execute != nil {
			bundle, evidencePath, runErr := r.Execute(ctx, experimentPath, systemPath, sourceRevision)
			evidencePath, err = relativeRunPath(r.Workspace, evidencePath)
			if err != nil {
				return AcceptanceEntry{}, err
			}
			status := ael.RunCompleted
			for _, assertion := range bundle.Assertions {
				if !assertion.Passed {
					status = ael.RunFailed
					break
				}
			}
			if runErr != nil {
				status = ael.RunFailed
			}
			record = ael.RunRecord{ID: bundle.RunID, Status: status, EvidencePath: evidencePath, Bundle: &bundle}
			if runErr != nil {
				record.Error = runErr.Error()
			}
		} else {
			record, err = r.Runs.Start(ctx, ael.RunRequest{ProjectID: r.ProjectID, ExperimentPath: experimentPath, SystemPath: systemPath, SourceRevision: sourceRevision})
			if err != nil {
				return AcceptanceEntry{}, err
			}
			record, err = waitRun(ctx, r.Runs, record.ID)
			if err != nil {
				return AcceptanceEntry{}, err
			}
		}
		expected := ael.RunCompleted
		if variant == "faulty" {
			expected = ael.RunFailed
		}
		kinds := mechanismEvidenceKinds(record.Bundle)
		required := containsAll(kinds, item.Mechanism.RequiredEvidence)
		evidence.Checks = append(evidence.Checks, VariantResult{Variant: variant, RunID: record.ID, Status: record.Status, ExpectedStatus: expected, EvidencePath: record.EvidencePath, MechanismEvidence: kinds, Passed: record.Status == expected && required, Error: record.Error})
	}
	path := filepath.Join(r.Workspace, "acceptance", "v2", "evidence", prefix+".json")
	if err := writeAcceptance(path, evidence); err != nil {
		return AcceptanceEntry{}, err
	}
	hash, err := FileSHA256(path)
	if err != nil {
		return AcceptanceEntry{}, err
	}
	passed := true
	for _, check := range evidence.Checks {
		passed = passed && check.Passed
	}
	status := "failed"
	if passed {
		status = "passed"
	}
	relative, _ := filepath.Rel(r.Workspace, path)
	return AcceptanceEntry{Name: "benchmark:" + prefix, Status: status, EvidencePath: filepath.ToSlash(relative), EvidenceSHA256: hash, Limitations: []string{item.FidelityBoundary, "No physical hardware evidence was produced."}}, nil
}

func relativeRunPath(workspace, path string) (string, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("run evidence path escapes workspace")
	}
	return filepath.ToSlash(relative), nil
}

func waitRun(ctx context.Context, manager *ael.RunManager, id string) (ael.RunRecord, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		record, err := manager.Get(ctx, id)
		if err != nil {
			return ael.RunRecord{}, err
		}
		if record.Status != ael.RunQueued && record.Status != ael.RunRunning {
			return record, nil
		}
		select {
		case <-ctx.Done():
			manager.Cancel(id)
			return ael.RunRecord{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
func mechanismEvidenceKinds(bundle *ael.EvidenceBundle) []string {
	if bundle == nil {
		return nil
	}
	set := map[string]bool{}
	if len(bundle.Artifacts) > 0 {
		set["artifact"] = true
	}
	for _, event := range bundle.Events {
		if strings.Contains(event.FidelityRef, "tool-executed") || strings.Contains(event.Type, "firmware.") || strings.Contains(event.Type, "connection.") {
			set["tool_event"] = true
		}
	}
	var result []string
	for item := range set {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}
func containsAll(values, required []string) bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, value := range required {
		if !set[value] {
			return false
		}
	}
	return true
}
func systemPathIndex(workspace string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(workspace, "benchmarks", "v2", "systems", "*.yaml"))
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, path := range files {
		system, err := ael.LoadSystem(workspace, path)
		if err != nil {
			return nil, err
		}
		relative, _ := filepath.Rel(workspace, path)
		result[system.ID] = filepath.ToSlash(relative)
	}
	return result, nil
}
func writeAcceptance(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(payload, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
