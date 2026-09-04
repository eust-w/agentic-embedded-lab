package release

import (
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
	"github.com/eust-w/agentic-embedded-lab/internal/packaging"
)

type Profile string

const (
	Foundation           Profile = "foundation"
	Desktop              Profile = "desktop"
	Agent                Profile = "agent"
	Simulation           Profile = "simulation"
	SimulationExtensions Profile = "simulation-extensions"
	Software             Profile = "software"
	DevelopmentPackage   Profile = "development-package"
	Production           Profile = "production"
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
	if profile == DevelopmentPackage {
		report := packaging.CheckBundle(filepath.Join(root, "build", "bin", "Aether Desktop.app"), false)
		result.Checked = append(result.Checked, "build/bin/Aether Desktop.app")
		if !report.Passed {
			result.Failures = append(result.Failures, report.Error().Error())
		}
		result.Passed = len(result.Failures) == 0
		return result, nil
	}
	if profile == Desktop || profile == Agent {
		manifest, err := loadManifest(filepath.Join(root, "acceptance", "v2", "capabilities.json"))
		if err != nil {
			result.Failures = append(result.Failures, "capability evidence: "+err.Error())
		} else {
			required := []string{"desktop.shell", "engineering.git-terminal", "browser.computer-use"}
			if profile == Agent {
				required = []string{"agent.runtime", "security.approval-sandbox", "customization.plugins-mcp", "multiagent.automation", "modeling.behavior-ir"}
			}
			result.Failures = append(result.Failures, validateEntries(root, manifest, required)...)
			result.Checked = append(result.Checked, "acceptance/v2/capabilities.json")
		}
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
	extensions, extensionsErr := loadManifest(filepath.Join(root, "acceptance", "v2", "extensions.json"))
	if extensionsErr != nil {
		result.Failures = append(result.Failures, "simulation extension evidence: "+extensionsErr.Error())
	} else {
		result.Failures = append(result.Failures, validateEntries(root, extensions, []string{"extension:rtl", "extension:motor", "extension:battery", "extension:sensor", "extension:pcb", "extension:emc"})...)
		result.Checked = append(result.Checked, "acceptance/v2/extensions.json")
	}
	if profile == SimulationExtensions {
		result.Passed = len(result.Failures) == 0
		return result, nil
	}
	capabilities, capabilityErr := loadManifest(filepath.Join(root, "acceptance", "v2", "capabilities.json"))
	if capabilityErr != nil {
		result.Failures = append(result.Failures, "capability evidence: "+capabilityErr.Error())
	} else {
		result.Failures = append(result.Failures, validateEntries(root, capabilities, []string{"desktop.shell", "agent.runtime", "engineering.git-terminal", "security.approval-sandbox", "customization.plugins-mcp", "multiagent.automation", "browser.computer-use", "modeling.behavior-ir", "server.worker-storage"})...)
		result.Checked = append(result.Checked, "acceptance/v2/capabilities.json")
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
		if strings.HasPrefix(name, "extension:") {
			failures = append(failures, validateExtensionRuns(root, path, manifest.SourceRevision)...)
		}
		if name == "determinism:trace-matrix" {
			failures = append(failures, validateDeterminismRuns(root, path, manifest.SourceRevision)...)
		}
		if strings.HasPrefix(name, "hardware:") || strings.HasPrefix(name, "calibration:") || strings.HasPrefix(name, "claims:") {
			failures = append(failures, validateProductionEvidence(root, path)...)
		}
	}
	return failures
}

func validateProductionEvidence(root, path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	var evidence struct {
		HardwareValidated bool                   `json:"hardware_validated"`
		HumanApproved     bool                   `json:"human_approved"`
		Envelope          ael.ValidationEnvelope `json:"envelope"`
	}
	if json.Unmarshal(data, &evidence) != nil || !evidence.HardwareValidated || !evidence.HumanApproved || evidence.Envelope.SignedBy == "" || evidence.Envelope.Signature == "" || len(evidence.Envelope.EvidenceRunIDs) == 0 {
		return []string{"production evidence lacks hardware validation, human approval, runs, or signature: " + path}
	}
	if err := ael.ValidateEnvelope(evidence.Envelope, time.Now().UTC()); err != nil {
		return []string{"production validation envelope is incomplete: " + err.Error()}
	}
	keysData, err := os.ReadFile(filepath.Join(root, "lab", "trusted-reviewers.json"))
	if err != nil {
		return []string{"trusted production reviewer registry is missing"}
	}
	var keys map[string]string
	if json.Unmarshal(keysData, &keys) != nil {
		return []string{"trusted reviewer registry is invalid"}
	}
	publicRaw, err := base64.StdEncoding.DecodeString(keys[evidence.Envelope.SignedBy])
	if err != nil || len(publicRaw) != ed25519.PublicKeySize {
		return []string{"production reviewer is not trusted: " + evidence.Envelope.SignedBy}
	}
	signature, err := base64.StdEncoding.DecodeString(evidence.Envelope.Signature)
	if err != nil {
		return []string{"production signature is invalid base64"}
	}
	envelope := evidence.Envelope
	envelope.Signature = ""
	payload, _ := json.Marshal(envelope)
	if !ed25519.Verify(ed25519.PublicKey(publicRaw), payload, signature) {
		return []string{"production envelope signature verification failed"}
	}
	return nil
}

func validateDeterminismRuns(root, path, revision string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	type matrixEntry struct {
		TraceHashes   []string `json:"trace_hashes"`
		OutcomeHashes []string `json:"outcome_hashes"`
		RunPaths      []string `json:"run_paths"`
		AllEqual      bool     `json:"all_equal"`
	}
	var evidence struct {
		SourceRevision string                 `json:"source_revision"`
		BenchmarkCount int                    `json:"benchmark_count"`
		BaseRepeats    int                    `json:"base_repeats"`
		StressRepeats  int                    `json:"stress_repeats"`
		StressCaseIDs  []int                  `json:"stress_case_ids"`
		Matrix         map[string]matrixEntry `json:"matrix"`
		AllEqual       bool                   `json:"all_equal"`
	}
	if json.Unmarshal(data, &evidence) != nil || evidence.SourceRevision != revision || evidence.BenchmarkCount != 24 || evidence.BaseRepeats < 2 || evidence.StressRepeats < 20 || !evidence.AllEqual || len(evidence.Matrix) != 24 {
		return []string{"determinism matrix is incomplete or stale: " + path}
	}
	stress := map[string]bool{}
	for _, id := range evidence.StressCaseIDs {
		stress[fmt.Sprintf("%02d-", id)] = true
	}
	var failures []string
	for name, entry := range evidence.Matrix {
		expected := evidence.BaseRepeats
		for prefix := range stress {
			if strings.HasPrefix(name, prefix) {
				expected = evidence.StressRepeats
			}
		}
		if !entry.AllEqual || len(entry.TraceHashes) != expected || len(entry.OutcomeHashes) != expected || len(entry.RunPaths) != expected {
			failures = append(failures, "determinism case is incomplete: "+name)
			continue
		}
		for index, runPath := range entry.RunPaths {
			if filepath.IsAbs(runPath) || !strings.HasPrefix(filepath.ToSlash(runPath), "runs/") {
				failures = append(failures, "invalid determinism run path: "+runPath)
				continue
			}
			runRoot := filepath.Join(root, filepath.FromSlash(runPath))
			failures = append(failures, validateRunBundle(runRoot, revision)...)
			var provenance struct {
				TraceSHA256 string `json:"trace_sha256"`
			}
			provenanceData, readErr := os.ReadFile(filepath.Join(runRoot, "provenance.json"))
			if readErr != nil || json.Unmarshal(provenanceData, &provenance) != nil || provenance.TraceSHA256 != entry.TraceHashes[index] {
				failures = append(failures, "determinism trace mismatch: "+runPath)
			}
			assertionsData, readErr := os.ReadFile(filepath.Join(runRoot, "assertions.json"))
			var assertions []ael.AssertionResult
			if readErr != nil || json.Unmarshal(assertionsData, &assertions) != nil || assertionOutcomeHash(assertions) != entry.OutcomeHashes[index] {
				failures = append(failures, "determinism outcome mismatch: "+runPath)
			}
		}
	}
	return failures
}

func assertionOutcomeHash(assertions []ael.AssertionResult) string {
	type outcome struct {
		ID          string  `json:"id"`
		Passed      bool    `json:"passed"`
		Expected    float64 `json:"expected"`
		Aggregation string  `json:"aggregation"`
	}
	values := make([]outcome, 0, len(assertions))
	for _, assertion := range assertions {
		values = append(values, outcome{ID: assertion.ID, Passed: assertion.Passed, Expected: assertion.Expected, Aggregation: assertion.Aggregation})
	}
	payload, _ := json.Marshal(values)
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

func validateExtensionRuns(root, path, revision string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{err.Error()}
	}
	var evidence struct {
		SourceRevision string `json:"source_revision"`
		Checks         []struct {
			RunPath string `json:"run_path"`
			Passed  bool   `json:"passed"`
		} `json:"checks"`
		Determinism struct {
			Repeats         int      `json:"repeats"`
			TraceHashes     []string `json:"trace_hashes"`
			AssertionHashes []string `json:"assertion_hashes"`
			RunPaths        []string `json:"run_paths"`
			AllEqual        bool     `json:"all_equal"`
		} `json:"determinism"`
	}
	if json.Unmarshal(data, &evidence) != nil || evidence.SourceRevision != revision {
		return []string{"invalid or stale extension evidence: " + path}
	}
	var failures []string
	for _, check := range evidence.Checks {
		if !check.Passed || filepath.IsAbs(check.RunPath) || !strings.HasPrefix(check.RunPath, "runs/") {
			failures = append(failures, "invalid extension run reference: "+check.RunPath)
			continue
		}
		failures = append(failures, validateRunBundle(filepath.Join(root, filepath.FromSlash(check.RunPath)), revision)...)
	}
	if evidence.Determinism.Repeats < 20 || !evidence.Determinism.AllEqual || len(evidence.Determinism.TraceHashes) != evidence.Determinism.Repeats || len(evidence.Determinism.AssertionHashes) != evidence.Determinism.Repeats || len(evidence.Determinism.RunPaths) != evidence.Determinism.Repeats {
		failures = append(failures, "extension determinism evidence is incomplete: "+path)
		return failures
	}
	for index, runPath := range evidence.Determinism.RunPaths {
		if filepath.IsAbs(runPath) || !strings.HasPrefix(filepath.ToSlash(runPath), "runs/") || evidence.Determinism.TraceHashes[index] == "" {
			failures = append(failures, "invalid extension determinism run reference: "+runPath)
			continue
		}
		runRoot := filepath.Join(root, filepath.FromSlash(runPath))
		failures = append(failures, validateRunBundle(runRoot, revision)...)
		var provenance struct {
			TraceSHA256 string `json:"trace_sha256"`
		}
		data, err := os.ReadFile(filepath.Join(runRoot, "provenance.json"))
		if err != nil || json.Unmarshal(data, &provenance) != nil || provenance.TraceSHA256 != evidence.Determinism.TraceHashes[index] {
			failures = append(failures, "extension determinism trace mismatch: "+runPath)
		}
		assertionHash, err := benchmark.FileSHA256(filepath.Join(runRoot, "assertions.json"))
		if err != nil || assertionHash != evidence.Determinism.AssertionHashes[index] {
			failures = append(failures, "extension determinism assertion mismatch: "+runPath)
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
