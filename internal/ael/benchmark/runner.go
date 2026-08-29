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
}

func (r BenchmarkRunner) Run(ctx context.Context, catalog Catalog, selected map[int]bool, sourceRevision string) (AcceptanceManifest, error) {
	if r.Runs == nil {
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
	path := filepath.Join(r.Workspace, "acceptance", "v2", "simulation.json")
	if err := writeAcceptance(path, manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
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
		record, err := r.Runs.Start(ctx, ael.RunRequest{ProjectID: r.ProjectID, ExperimentPath: experimentPath, SystemPath: systemPath, SourceRevision: sourceRevision})
		if err != nil {
			return AcceptanceEntry{}, err
		}
		record, err = waitRun(ctx, r.Runs, record.ID)
		if err != nil {
			return AcceptanceEntry{}, err
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
