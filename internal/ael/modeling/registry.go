package modeling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

type GenerationRequest struct {
	APIVersion string      `json:"api_version"`
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Version    string      `json:"version"`
	Backend    ael.Backend `json:"backend"`
	SVD        string      `json:"svd,omitempty"`
	SystemRDL  string      `json:"systemrdl,omitempty"`
	Generator  string      `json:"generator"`
}

type ConformanceEvidence struct {
	APIVersion                 string            `json:"api_version"`
	ModelID                    string            `json:"model_id"`
	Validator                  string            `json:"validator"`
	SourceIndependent          bool              `json:"source_independent"`
	RegisterLayoutPassed       bool              `json:"register_layout_passed"`
	CompilePassed              bool              `json:"compile_passed"`
	DriverTestsPassed          bool              `json:"driver_tests_passed"`
	PropertyTestsPassed        bool              `json:"property_tests_passed"`
	ReferenceTracePassed       bool              `json:"reference_trace_passed"`
	GeneratedTestsOnlyEvidence bool              `json:"generated_tests_are_only_evidence"`
	SandboxNetwork             string            `json:"sandbox_network"`
	SandboxReadOnly            bool              `json:"sandbox_read_only"`
	ArtifactHashes             map[string]string `json:"artifact_hashes"`
}

type Registry struct{ Workspace string }

var modelIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

func (r Registry) Generate(request GenerationRequest) (Package, error) {
	if request.APIVersion != APIVersion || !modelIDPattern.MatchString(request.ID) || request.Name == "" || request.Version == "" {
		return Package{}, errors.New("invalid model generation request")
	}
	if (request.SVD == "") == (request.SystemRDL == "") {
		return Package{}, errors.New("exactly one deterministic SVD or SystemRDL source is required")
	}
	root, err := filepath.Abs(r.Workspace)
	if err != nil {
		return Package{}, err
	}
	source := request.SVD
	importer := ImportSVD
	if request.SystemRDL != "" {
		source, importer = request.SystemRDL, ImportSystemRDL
	}
	sourcePath, err := modelWorkspacePath(root, source, true)
	if err != nil {
		return Package{}, err
	}
	ir, err := importer(sourcePath, request.Name)
	if err != nil {
		return Package{}, err
	}
	modelRoot := filepath.Join(root, ".aether", "models", request.ID, request.Version)
	if _, err := os.Stat(modelRoot); err == nil {
		return Package{}, errors.New("model version already exists")
	}
	if err := os.MkdirAll(filepath.Dir(modelRoot), 0o700); err != nil {
		return Package{}, err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(modelRoot), ".model-")
	if err != nil {
		return Package{}, err
	}
	defer os.RemoveAll(temporary)
	irPath := filepath.Join(temporary, "behavior.ir.json")
	if err := writeModelJSON(irPath, ir); err != nil {
		return Package{}, err
	}
	artifacts := []string{}
	if request.Backend == ael.BackendRenode {
		sourceCode, err := GenerateRenodeCSharp(ir, "Ael.Generated")
		if err != nil {
			return Package{}, err
		}
		if err := os.WriteFile(filepath.Join(temporary, "GeneratedPeripheral.cs"), []byte(sourceCode), 0o600); err != nil {
			return Package{}, err
		}
		artifacts = append(artifacts, filepath.ToSlash(filepath.Join(".aether", "models", request.ID, request.Version, "GeneratedPeripheral.cs")))
	}
	digest, err := hashFile(sourcePath)
	if err != nil {
		return Package{}, err
	}
	relativeSource, _ := filepath.Rel(root, sourcePath)
	generatedBy := request.Generator
	if generatedBy == "" {
		generatedBy = "ael.go-deterministic-importer/v2"
	}
	packageValue := Package{APIVersion: APIVersion, Kind: "ModelPackage", ID: request.ID, Name: request.Name, Version: request.Version, Backend: request.Backend, State: StateGenerated, SourcePaths: []string{filepath.ToSlash(relativeSource)}, SourceHashes: map[string]string{filepath.ToSlash(relativeSource): digest}, IRPath: filepath.ToSlash(filepath.Join(".aether", "models", request.ID, request.Version, "behavior.ir.json")), ArtifactPaths: artifacts, GeneratedBy: generatedBy, CreatedAt: time.Now().UTC()}
	if err := writeModelJSON(filepath.Join(temporary, "package.json"), packageValue); err != nil {
		return Package{}, err
	}
	if err := os.Rename(temporary, modelRoot); err != nil {
		return Package{}, err
	}
	return packageValue, nil
}

func (r Registry) Load(id, version string) (Package, string, error) {
	if !modelIDPattern.MatchString(id) || version == "" || strings.ContainsAny(version, `/\\`) {
		return Package{}, "", errors.New("invalid model id or version")
	}
	root, err := filepath.Abs(r.Workspace)
	if err != nil {
		return Package{}, "", err
	}
	path := filepath.Join(root, ".aether", "models", id, version, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Package{}, "", err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var packageValue Package
	if err := decoder.Decode(&packageValue); err != nil {
		return Package{}, "", err
	}
	return packageValue, path, nil
}

func (r Registry) Promote(id, version string, target ModelState, actor string, evidence *ConformanceEvidence) (Package, error) {
	packageValue, path, err := r.Load(id, version)
	if err != nil {
		return Package{}, err
	}
	if !CanTransition(packageValue.State, target, actor) {
		return Package{}, fmt.Errorf("transition %s -> %s is not permitted for %s", packageValue.State, target, actor)
	}
	if target == StateConformanceValidated {
		if evidence == nil || evidence.Validate() != nil {
			return Package{}, errors.New("independent conformance evidence is required")
		}
		evidencePath := filepath.Join(filepath.Dir(path), "conformance-evidence.json")
		if err := writeModelJSON(evidencePath, evidence); err != nil {
			return Package{}, err
		}
		relative, _ := filepath.Rel(r.Workspace, evidencePath)
		packageValue.ValidationEvidence = append(packageValue.ValidationEvidence, filepath.ToSlash(relative))
	}
	packageValue.State = target
	if err := writeModelJSON(path, packageValue); err != nil {
		return Package{}, err
	}
	return packageValue, nil
}

func (e ConformanceEvidence) Validate() error {
	if e.APIVersion != APIVersion || e.Validator == "" || !e.SourceIndependent || !e.RegisterLayoutPassed || !e.CompilePassed || !e.DriverTestsPassed || !e.PropertyTestsPassed || !e.ReferenceTracePassed || e.GeneratedTestsOnlyEvidence || e.SandboxNetwork != "none" || !e.SandboxReadOnly || len(e.ArtifactHashes) == 0 {
		return errors.New("conformance evidence does not satisfy the independent validation gate")
	}
	for _, digest := range e.ArtifactHashes {
		if len(digest) != 64 {
			return errors.New("artifact hash must be SHA-256")
		}
	}
	return nil
}

func modelWorkspacePath(root, requested string, mustExist bool) (string, error) {
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("model path escapes workspace")
	}
	if mustExist {
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
	}
	return path, nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
func writeModelJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o600)
}
