package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const APIVersion = "aether.acceptance/v1"

type Status string

const (
	Accepted          Status = "accepted"
	Failed            Status = "failed"
	NotRun            Status = "not_run"
	ExternallyBlocked Status = "externally_blocked"
)

type CapabilityAcceptance struct {
	APIVersion           string   `json:"api_version"`
	ID                   string   `json:"id"`
	Category             string   `json:"category"`
	ImplementationStatus Status   `json:"implementation_status"`
	TestStatus           Status   `json:"test_status"`
	EvidenceStatus       Status   `json:"evidence_status"`
	Fidelity             string   `json:"fidelity"`
	SupportedPlatforms   []string `json:"supported_platforms"`
	KnownLimitations     []string `json:"known_limitations"`
	ExternalRequirements []string `json:"external_requirements"`
	SourceRevision       string   `json:"source_revision"`
	EvidenceSHA256       string   `json:"evidence_sha256"`
}

type definition struct {
	ID, Category, Evidence, Fidelity string
	Platforms, Limitations, External []string
}

var frozen = []definition{
	{"desktop.shell", "desktop", "acceptance/v2/capabilities.json", "software", []string{"macos-arm64"}, nil, nil},
	{"agent.runtime", "agent", "acceptance/v2/capabilities.json", "software", []string{"macos-arm64"}, nil, nil},
	{"engineering.git-terminal", "engineering", "acceptance/v2/capabilities.json", "software", []string{"macos-arm64"}, nil, nil},
	{"security.approval-sandbox", "security", "acceptance/v2/capabilities.json", "software", []string{"macos-arm64"}, nil, nil},
	{"customization.plugins-mcp", "customization", "acceptance/v2/capabilities.json", "software", []string{"macos-arm64"}, nil, nil},
	{"multiagent.automation", "automation", "acceptance/v2/capabilities.json", "software", []string{"macos-arm64"}, nil, nil},
	{"browser.computer-use", "browser", "acceptance/v2/capabilities.json", "software", []string{"macos-arm64"}, []string{"Accepted only for the recorded local test-site/application matrix."}, nil},
	{"ael.core", "simulation", "acceptance/v2/simulation.json", "simulation", []string{"linux-amd64", "macos-arm64-control"}, nil, nil},
	{"ael.fmi-five-domain", "simulation", "acceptance/v2/evidence/fmi-five-domain.json", "functional", []string{"linux-amd64"}, []string{"No calibrated hardware equivalence."}, nil},
	{"ael.simulation-extensions", "simulation-extension", "acceptance/v2/extensions.json", "functional", []string{"linux-amd64"}, []string{"Extension models are uncalibrated."}, nil},
	{"modeling.behavior-ir", "modeling", "acceptance/v2/capabilities.json", "conformance", []string{"linux-amd64", "macos-arm64"}, []string{"Agent promotion stops at conformance_validated."}, nil},
	{"server.worker-storage", "server", "acceptance/v2/capabilities.json", "software", []string{"linux-amd64"}, nil, nil},
	{"package.development", "package", "acceptance/v2/desktop-development.json", "development-package", []string{"macos-arm64"}, nil, nil},
	{"package.apple-distribution", "external", "", "external", []string{"macos-arm64"}, nil, []string{"Xcode", "Developer ID", "Notary profile", "Sparkle key and HTTPS feed"}},
	{"hardware.validation-envelope", "external", "", "external", []string{"hardware-lab"}, nil, []string{"15 reference boards", "calibrated instruments", "independent reviewer", "signed Validation Envelope"}},
}

func List(workspace, revision string) ([]CapabilityAcceptance, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	values := make([]CapabilityAcceptance, 0, len(frozen))
	for _, item := range frozen {
		value := CapabilityAcceptance{APIVersion: APIVersion, ID: item.ID, Category: item.Category, Fidelity: item.Fidelity, SupportedPlatforms: item.Platforms, KnownLimitations: item.Limitations, ExternalRequirements: item.External, SourceRevision: revision}
		if len(item.External) > 0 {
			value.ImplementationStatus, value.TestStatus, value.EvidenceStatus = Accepted, ExternallyBlocked, ExternallyBlocked
		} else {
			value.ImplementationStatus = Accepted
			path := filepath.Join(root, filepath.FromSlash(item.Evidence))
			digest, status := evidenceDigest(path, revision)
			value.EvidenceSHA256, value.EvidenceStatus, value.TestStatus = digest, status, status
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}

func Inspect(workspace, revision, id string) (CapabilityAcceptance, error) {
	values, err := List(workspace, revision)
	if err != nil {
		return CapabilityAcceptance{}, err
	}
	for _, value := range values {
		if value.ID == id {
			return value, nil
		}
	}
	return CapabilityAcceptance{}, fmt.Errorf("unknown capability %q", id)
}

func EvidencePath(id string) (string, error) {
	for _, item := range frozen {
		if item.ID == id {
			if item.Evidence == "" {
				return "", errors.New("capability has no software evidence path")
			}
			return item.Evidence, nil
		}
	}
	return "", fmt.Errorf("unknown capability %q", id)
}

func Report(workspace, revision string) (map[string]any, error) {
	values, err := List(workspace, revision)
	if err != nil {
		return nil, err
	}
	softwareAccepted, externalBlocked := true, true
	for _, value := range values {
		if value.Category == "external" {
			externalBlocked = externalBlocked && value.EvidenceStatus == ExternallyBlocked
			continue
		}
		softwareAccepted = softwareAccepted && value.ImplementationStatus == Accepted && value.TestStatus == Accepted && value.EvidenceStatus == Accepted
	}
	return map[string]any{"api_version": APIVersion, "source_revision": revision, "software_accepted": softwareAccepted, "external_boundaries_preserved": externalBlocked, "capabilities": values}, nil
}

func evidenceDigest(path, revision string) (string, Status) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", NotRun
	}
	if err != nil {
		return "", Failed
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return "", Failed
	}
	if status, ok := payload["status"].(string); ok && status != "passed" {
		return "", Failed
	}
	if passed, ok := payload["passed"].(bool); ok && !passed {
		return "", Failed
	}
	if source, ok := payload["source_revision"].(string); ok && source != "" && !sameRevision(source, revision) {
		return "", Failed
	}
	if entries, ok := payload["entries"].([]any); ok {
		for _, raw := range entries {
			item, _ := raw.(map[string]any)
			if !strings.EqualFold(fmt.Sprint(item["status"]), "passed") {
				return "", Failed
			}
		}
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), Accepted
}
func sameRevision(left, right string) bool {
	return left == right || len(left) >= 7 && len(right) >= 7 && (strings.HasPrefix(left, right) || strings.HasPrefix(right, left))
}
