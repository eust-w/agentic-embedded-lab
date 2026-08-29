package release

import (
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
	names := []string{"cross-domain:five-backend", "firmware:arm-riscv", "environment:qualified"}
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
		path := filepath.Join(root, entry.EvidencePath)
		digest, err := benchmark.FileSHA256(path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s evidence: %v", name, err))
			continue
		}
		if digest != entry.EvidenceSHA256 {
			failures = append(failures, "evidence hash mismatch: "+name)
		}
	}
	return failures
}
