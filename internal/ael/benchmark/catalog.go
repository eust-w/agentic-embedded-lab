package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"gopkg.in/yaml.v3"
)

type Mechanism struct {
	Trigger          string   `json:"trigger"`
	ExecutionBackend string   `json:"execution_backend"`
	FaultyAssets     []string `json:"faulty_assets"`
	FixedAssets      []string `json:"fixed_assets"`
	Oracle           string   `json:"oracle"`
	RequiredEvidence []string `json:"required_evidence"`
}

type Case struct {
	ID               int       `json:"id"`
	Slug             string    `json:"slug"`
	Title            string    `json:"title"`
	Category         string    `json:"category"`
	Backends         []string  `json:"backends"`
	Readiness        string    `json:"readiness"`
	FaultyAsset      string    `json:"faulty_asset"`
	FixedAsset       string    `json:"fixed_asset"`
	Experiment       string    `json:"experiment"`
	FaultyExperiment string    `json:"faulty_experiment"`
	FixedExperiment  string    `json:"fixed_experiment"`
	CausalChain      []string  `json:"causal_chain"`
	Seed             int64     `json:"seed"`
	HardwareTarget   string    `json:"hardware_target,omitempty"`
	FidelityBoundary string    `json:"fidelity_boundary"`
	Mechanism        Mechanism `json:"mechanism"`
}

type Catalog struct {
	Version string `json:"version"`
	Cases   []Case `json:"cases"`
}

func Load(workspace, path string) (Catalog, error) {
	resolved, err := resolve(workspace, path)
	if err != nil {
		return Catalog{}, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return Catalog{}, err
	}
	var normalized any
	if err := yaml.Unmarshal(data, &normalized); err != nil {
		return Catalog{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return Catalog{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func (c Catalog) Validate(workspace string) []error {
	var failures []error
	if len(c.Cases) != 24 {
		failures = append(failures, fmt.Errorf("catalog must contain 24 cases, got %d", len(c.Cases)))
	}
	systems, systemErrors := loadSystems(workspace)
	failures = append(failures, systemErrors...)
	for index, item := range c.Cases {
		if item.ID != index+1 {
			failures = append(failures, fmt.Errorf("catalog id at position %d is %d", index+1, item.ID))
		}
		prefix := fmt.Sprintf("%02d-%s", item.ID, item.Slug)
		if item.Readiness != "executable" {
			failures = append(failures, fmt.Errorf("%s readiness=%s", prefix, item.Readiness))
		}
		if item.Mechanism.Trigger == "" || item.Mechanism.Oracle == "" || len(item.Mechanism.RequiredEvidence) == 0 {
			failures = append(failures, fmt.Errorf("%s has incomplete mechanism contract", prefix))
		}
		if !contains(item.Backends, item.Mechanism.ExecutionBackend) {
			failures = append(failures, fmt.Errorf("%s mechanism backend is undeclared", prefix))
		}
		if contains(item.Backends, "native") {
			failures = append(failures, fmt.Errorf("%s uses forbidden native formal backend", prefix))
		}
		if equalSets(item.Mechanism.FaultyAssets, item.Mechanism.FixedAssets) {
			failures = append(failures, fmt.Errorf("%s faulty and fixed assets are identical", prefix))
		}
		for _, asset := range append(append([]string{}, item.Mechanism.FaultyAssets...), item.Mechanism.FixedAssets...) {
			if _, err := resolveExisting(workspace, asset); err != nil {
				failures = append(failures, fmt.Errorf("%s asset: %w", prefix, err))
			}
		}
		if sameAssetContent(workspace, item.Mechanism.FaultyAssets, item.Mechanism.FixedAssets) {
			failures = append(failures, fmt.Errorf("%s faulty and fixed mechanism assets have identical content", prefix))
		}
		faultyPath := filepath.ToSlash(filepath.Join("benchmarks", "v2", "experiments", prefix+"-faulty.yaml"))
		fixedPath := filepath.ToSlash(filepath.Join("benchmarks", "v2", "experiments", prefix+"-fixed.yaml"))
		faulty, err := ael.LoadExperiment(workspace, faultyPath)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s faulty experiment: %w", prefix, err))
			continue
		}
		fixed, err := ael.LoadExperiment(workspace, fixedPath)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s fixed experiment: %w", prefix, err))
			continue
		}
		for variant, experiment := range map[string]ael.Experiment{"faulty": faulty, "fixed": fixed} {
			system, ok := systems[experiment.SystemID]
			if !ok {
				failures = append(failures, fmt.Errorf("%s %s references missing system %s", prefix, variant, experiment.SystemID))
				continue
			}
			if err := ael.Validate(experiment, system); err != nil {
				failures = append(failures, fmt.Errorf("%s %s: %w", prefix, variant, err))
			}
			for _, stimulus := range experiment.Stimuli {
				target := strings.ToLower(stimulus.Target)
				if strings.Contains(target, ".fixed") || strings.Contains(target, "fault_scale") || strings.Contains(target, "case_id") {
					failures = append(failures, fmt.Errorf("%s %s contains result selector %s", prefix, variant, stimulus.Target))
				}
			}
			for _, component := range system.Components {
				for _, key := range []string{"fixed", "fault_scale", "preset_failure", "expected_failure", "declared_result", "mock_backend", "fallback_backend"} {
					if _, ok := component.Properties[key]; ok {
						failures = append(failures, fmt.Errorf("%s %s component %s contains forbidden property %s", prefix, variant, component.ID, key))
					}
				}
			}
		}
		if item.ID >= 4 && item.ID <= 17 {
			expectedFaulty := fmt.Sprintf("build-case%02d-faulty", item.ID)
			expectedFixed := fmt.Sprintf("build-case%02d-fixed", item.ID)
			if !systemFirmwareContains(systems[faulty.SystemID], expectedFaulty) || !systemFirmwareContains(systems[fixed.SystemID], expectedFixed) {
				failures = append(failures, fmt.Errorf("%s does not use variant-specific firmware", prefix))
			}
		}
	}
	return failures
}

func loadSystems(workspace string) (map[string]ael.System, []error) {
	files, _ := filepath.Glob(filepath.Join(workspace, "benchmarks", "v2", "systems", "*.yaml"))
	result := map[string]ael.System{}
	var failures []error
	for _, path := range files {
		system, err := ael.LoadSystem(workspace, path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		result[system.ID] = system
	}
	return result, failures
}
func systemFirmwareContains(system ael.System, fragment string) bool {
	for _, component := range system.Components {
		if value, ok := component.Properties["firmware"].(string); ok && strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
func resolve(workspace, requested string) (string, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("benchmark path escapes workspace")
	}
	return path, nil
}
func resolveExisting(workspace, requested string) (string, error) {
	path, err := resolve(workspace, requested)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func equalSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a := append([]string{}, left...)
	b := append([]string{}, right...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func sameAssetContent(workspace string, left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a := []string{}
	b := []string{}
	for _, asset := range left {
		path, err := resolveExisting(workspace, asset)
		if err != nil {
			return false
		}
		hash, err := FileSHA256(path)
		if err != nil {
			return false
		}
		a = append(a, hash)
	}
	for _, asset := range right {
		path, err := resolveExisting(workspace, asset)
		if err != nil {
			return false
		}
		hash, err := FileSHA256(path)
		if err != nil {
			return false
		}
		b = append(b, hash)
	}
	sort.Strings(a)
	sort.Strings(b)
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}
func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

type Runner interface {
	Start(context.Context, ael.RunRequest) (ael.RunRecord, error)
	Get(context.Context, string) (ael.RunRecord, error)
}
