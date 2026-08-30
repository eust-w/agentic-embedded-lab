package release

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
)

type Profile string

const (
	Foundation Profile = "foundation"
	Simulation Profile = "simulation"
	Software   Profile = "software"
	Production Profile = "production"
)

type Result struct {
	Profile  Profile  `json:"profile"`
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures"`
	Checked  []string `json:"checked"`
}

func Check(workspace string, profile Profile) (Result, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return Result{}, err
	}
	result := Result{Profile: profile}
	for _, path := range []string{"go.mod", "frontend/package-lock.json", "schemas/v2/ael-experiment.schema.json", "schemas/v2/hardware-behavior-ir.schema.json"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			result.Failures = append(result.Failures, "foundation artifact missing: "+path)
		} else {
			result.Checked = append(result.Checked, path)
		}
	}
	if profile == Foundation {
		result.Passed = len(result.Failures) == 0
		return result, nil
	}
	catalog, err := benchmark.Load(root, "benchmarks/v2/catalog.yaml")
	if err != nil {
		return Result{}, err
	}
	for _, failure := range catalog.Validate(root) {
		result.Failures = append(result.Failures, failure.Error())
	}
	result.Checked = append(result.Checked, "benchmarks/v2/catalog.yaml")
	manifest, err := loadManifest(filepath.Join(root, "acceptance", "v2", "simulation.json"))
	if err != nil {
		result.Failures = append(result.Failures, "simulation evidence: "+err.Error())
	} else {
		result.Failures = append(result.Failures, validateSimulation(root, manifest, catalog)...)
		result.Checked = append(result.Checked, "acceptance/v2/simulation.json")
	}
	if profile == Simulation {
		result.Passed = len(result.Failures) == 0
		return result, nil
	}
	software, err := loadManifest(filepath.Join(root, "acceptance", "v2", "software.json"))
	if err != nil {
		result.Failures = append(result.Failures, "software evidence: "+err.Error())
	} else {
		result.Failures = append(result.Failures, validateEntries(root, software, []string{"deployment:compose", "storage:postgres-s3", "security:oidc-mtls", "worker:lease-recovery", "supply-chain:sbom-signature"})...)
		result.Checked = append(result.Checked, "acceptance/v2/software.json")
	}
	if profile == Software {
		result.Passed = len(result.Failures) == 0
		return result, nil
	}
	production, err := loadManifest(filepath.Join(root, "acceptance", "v2", "production.json"))
	if err != nil {
		result.Failures = append(result.Failures, "production evidence: "+err.Error())
	} else {
		result.Failures = append(result.Failures, validateEntries(root, production, []string{"hardware:stm32f407", "hardware:hifive1", "hardware:nrf52840", "hardware:esp32-s3", "hardware:rp2040", "calibration:instruments", "claims:production-approved"})...)
		result.Checked = append(result.Checked, "acceptance/v2/production.json")
	}
	result.Passed = len(result.Failures) == 0
	return result, nil
}

func loadManifest(path string) (benchmark.AcceptanceManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return benchmark.AcceptanceManifest{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var value benchmark.AcceptanceManifest
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	if value.APIVersion != ael.APIVersion {
		return value, fmt.Errorf("expected %s evidence, got %s", ael.APIVersion, value.APIVersion)
	}
	return value, nil
}
func validateSimulation(root string, manifest benchmark.AcceptanceManifest, catalog benchmark.Catalog) []string {
	names := []string{"cross-domain:five-backend", "fmi:five-domain", "firmware:arm-riscv", "environment:qualified", "determinism:trace-matrix"}
	for _, backend := range []string{"zephyr_build", "renode", "ngspice", "openmodelica", "ns3", "openems"} {
		names = append(names, "backend:"+backend)
	}
	for _, item := range catalog.Cases {
		names = append(names, fmt.Sprintf("benchmark:%02d-%s", item.ID, item.Slug))
	}
	return validateEntries(root, manifest, names)
}
func validateEntries(root string, manifest benchmark.AcceptanceManifest, required []string) []string {
	entries := map[string]benchmark.AcceptanceEntry{}
	for _, entry := range manifest.Entries {
		entries[entry.Name] = entry
	}
	var failures []string
	sort.Strings(required)
	for _, name := range required {
		entry, ok := entries[name]
		if !ok {
			failures = append(failures, "missing acceptance entry: "+name)
			continue
		}
		if entry.Status != "passed" {
			failures = append(failures, fmt.Sprintf("acceptance entry %s=%s", name, entry.Status))
			continue
		}
		if entry.EvidencePath == "" || entry.EvidenceSHA256 == "" {
			failures = append(failures, "acceptance entry has no hashed evidence: "+name)
			continue
		}
		if filepath.IsAbs(entry.EvidencePath) || strings.Contains(filepath.Clean(entry.EvidencePath), ".."+string(filepath.Separator)) {
			failures = append(failures, "acceptance evidence path is not workspace-relative: "+name)
			continue
		}
		path := filepath.Join(root, entry.EvidencePath)
		digest, err := benchmark.FileSHA256(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s evidence: %v", name, err))
			continue
		}
		if digest != entry.EvidenceSHA256 {
			failures = append(failures, "evidence hash mismatch: "+name)
		}
		if strings.HasPrefix(name, "benchmark:") {
			failures = append(failures, validateCaseRuns(root, path, manifest.SourceRevision)...)
		}
	}
	return failures
}

func validateCaseRuns(root, path, revision string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	var evidence benchmark.CaseEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return []string{"invalid benchmark evidence: " + err.Error()}
	}
	var failures []string
	for _, check := range evidence.Checks {
		if filepath.IsAbs(check.EvidencePath) || !strings.HasPrefix(filepath.ToSlash(check.EvidencePath), "runs/") {
			failures = append(failures, fmt.Sprintf("benchmark %s %s run path is not portable", evidence.Benchmark, check.Variant))
			continue
		}
		failures = append(failures, validateRunBundle(filepath.Join(root, filepath.FromSlash(check.EvidencePath)), revision)...)
	}
	return failures
}

func validateRunBundle(runRoot, revision string) []string {
	required := []string{"provenance.json", "events.jsonl", "assertions.json", "artifacts.json", "experiment.resolved.json", "system.resolved.json", "junit.xml", "summary.md"}
	var failures []string
	for _, name := range required {
		if info, err := os.Stat(filepath.Join(runRoot, name)); err != nil || !info.Mode().IsRegular() {
			failures = append(failures, "run evidence missing: "+filepath.Join(runRoot, name))
		}
	}
	if len(failures) > 0 {
		return failures
	}
	var provenance struct {
		SourceRevision string `json:"source_revision"`
		TraceSHA256    string `json:"trace_sha256"`
	}
	data, err := os.ReadFile(filepath.Join(runRoot, "provenance.json"))
	if err != nil || json.Unmarshal(data, &provenance) != nil {
		return append(failures, "invalid run provenance: "+runRoot)
	}
	if revision == "" || provenance.SourceRevision != revision {
		failures = append(failures, "run source revision mismatch: "+runRoot)
	}
	events, err := os.Open(filepath.Join(runRoot, "events.jsonl"))
	if err == nil {
		var values []ael.Event
		scanner := bufio.NewScanner(events)
		for scanner.Scan() {
			var event ael.Event
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				failures = append(failures, "invalid event stream: "+runRoot)
				break
			}
			values = append(values, event)
		}
		events.Close()
		payload, _ := json.Marshal(values)
		digest := fmt.Sprintf("%x", sha256.Sum256(payload))
		if digest != provenance.TraceSHA256 {
			failures = append(failures, "event trace hash mismatch: "+runRoot)
		}
	}
	var artifacts map[string]string
	data, err = os.ReadFile(filepath.Join(runRoot, "artifacts.json"))
	if err == nil && json.Unmarshal(data, &artifacts) == nil {
		for name, reference := range artifacts {
			path, digest, ok := strings.Cut(reference, "#sha256=")
			if !ok || filepath.IsAbs(path) || strings.Contains(filepath.Clean(path), ".."+string(filepath.Separator)) {
				failures = append(failures, "invalid artifact reference: "+name)
				continue
			}
			actual, err := benchmark.FileSHA256(filepath.Join(runRoot, filepath.FromSlash(path)))
			if err != nil || actual != digest {
				failures = append(failures, "artifact hash mismatch: "+name)
			}
		}
	}
	return failures
}
