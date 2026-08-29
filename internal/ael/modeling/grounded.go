package modeling

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
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/agent"
	"github.com/invopop/jsonschema"
)

type GroundedRequest struct {
	APIVersion            string      `json:"api_version"`
	ID                    string      `json:"id"`
	Name                  string      `json:"name"`
	Version               string      `json:"version"`
	Backend               ael.Backend `json:"backend"`
	Sources               []string    `json:"sources"`
	Model                 string      `json:"model"`
	PromptTemplateVersion string      `json:"prompt_template_version"`
	MaxAttempts           int         `json:"max_attempts"`
}

type GroundingReference struct {
	SourcePath   string `json:"source_path"`
	SourceSHA256 string `json:"source_sha256"`
	Locator      string `json:"locator"`
	Purpose      string `json:"purpose"`
}

type GroundingManifest struct {
	Sources []GroundingReference `json:"sources"`
}

type GenerationReceipt struct {
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	PromptTemplateVersion string `json:"prompt_template_version"`
	RequestSHA256         string `json:"request_sha256"`
	ResponseSHA256        string `json:"response_sha256"`
	ProviderRequestID     string `json:"provider_request_id,omitempty"`
	Attempts              int    `json:"attempts"`
}

func (r Registry) GenerateGrounded(ctx context.Context, request GroundedRequest, client *agent.ResponsesClient) (Package, error) {
	if request.APIVersion != APIVersion || !modelIDPattern.MatchString(request.ID) || request.Name == "" || request.Version == "" || len(request.Sources) == 0 || client == nil {
		return Package{}, errors.New("invalid grounded generation request")
	}
	if request.Model == "" {
		request.Model = agent.DefaultModel
	}
	if request.PromptTemplateVersion == "" {
		request.PromptTemplateVersion = "ael-hardware-ir/v2"
	}
	if request.MaxAttempts <= 0 {
		request.MaxAttempts = 2
	}
	if request.MaxAttempts > 4 {
		return Package{}, errors.New("max_attempts may not exceed four")
	}
	root, err := filepath.Abs(r.Workspace)
	if err != nil {
		return Package{}, err
	}
	sources := append([]string(nil), request.Sources...)
	sort.Strings(sources)
	var documents []string
	references := make([]GroundingReference, 0, len(sources))
	hashes := make(map[string]string)
	for _, source := range sources {
		path, err := modelWorkspacePath(root, source, true)
		if err != nil {
			return Package{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Package{}, err
		}
		if len(data) > 4<<20 {
			return Package{}, fmt.Errorf("grounding source %s exceeds 4 MiB", source)
		}
		if strings.IndexByte(string(data), 0) >= 0 {
			return Package{}, fmt.Errorf("grounding source %s is binary; extract it to auditable text first", source)
		}
		digest := sha256.Sum256(data)
		hash := hex.EncodeToString(digest[:])
		hashes[source] = hash
		locator := "lines:1-" + fmt.Sprint(strings.Count(string(data), "\n")+1)
		references = append(references, GroundingReference{SourcePath: source, SourceSHA256: hash, Locator: locator, Purpose: "Hardware Behavior IR grounding"})
		documents = append(documents, fmt.Sprintf("<source path=%q sha256=%q locator=%q>\n%s\n</source>", source, hash, locator, string(data)))
	}
	prompt := "Generate only behavior established by the supplied sources. Return strict HardwareBehaviorIR JSON. Every register, clock, timer, interrupt, DMA request, transaction, fault and power state must include a grounding entry whose value uses an exact source path and locator shown below. Omit uncertain behavior. Use SI/UCUM units.\nTarget: " + request.Name + "\n\n" + strings.Join(documents, "\n\n")
	if len(prompt) > 1_500_000 {
		return Package{}, errors.New("grounding prompt exceeds 1,500,000 characters")
	}
	promptDigest := sha256.Sum256([]byte(prompt))
	schema := (&jsonschema.Reflector{AllowAdditionalProperties: false, DoNotReference: true}).Reflect(IR{})
	schemaPayload, _ := json.Marshal(schema)
	var raw, requestID string
	var generationErr error
	usedAttempts := 0
	for attempt := 1; attempt <= request.MaxAttempts; attempt++ {
		usedAttempts = attempt
		var builder strings.Builder
		responseID := ""
		generationErr = client.Stream(ctx, agent.ResponseRequest{Model: request.Model, Input: []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": prompt}}}}, Reasoning: map[string]any{"effort": "high"}, Text: map[string]any{"format": map[string]any{"type": "json_schema", "name": "hardware_behavior_ir", "strict": true, "schema": json.RawMessage(schemaPayload)}}}, fmt.Sprintf("model-%s-%s-%d", request.ID, request.Version, attempt), func(event agent.ResponseEvent) error {
			if strings.Contains(event.Type, "output_text") {
				builder.WriteString(event.Delta)
			}
			if response, ok := event.Payload["response"].(map[string]any); ok {
				responseID, _ = response["id"].(string)
			}
			return nil
		})
		if generationErr != nil {
			continue
		}
		raw, requestID = strings.TrimSpace(builder.String()), responseID
		var ir IR
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if generationErr = decoder.Decode(&ir); generationErr != nil {
			continue
		}
		if generationErr = ir.Validate(); generationErr != nil {
			continue
		}
		if generationErr = validateGrounding(ir, references); generationErr != nil {
			continue
		}
		return r.persistGrounded(request, ir, references, hashes, raw, requestID, usedAttempts, hex.EncodeToString(promptDigest[:]))
	}
	return Package{}, fmt.Errorf("grounded generation failed after %d attempts: %w", usedAttempts, generationErr)
}

func validateGrounding(ir IR, references []GroundingReference) error {
	allowed := make(map[string]bool)
	for _, reference := range references {
		allowed[reference.SourcePath+"#"+reference.Locator] = true
	}
	required := []string{}
	for index := range ir.Registers {
		required = append(required, fmt.Sprintf("/registers/%d", index))
	}
	for index := range ir.Clocks {
		required = append(required, fmt.Sprintf("/clocks/%d", index))
	}
	for index := range ir.Timers {
		required = append(required, fmt.Sprintf("/timers/%d", index))
	}
	for index := range ir.Interrupts {
		required = append(required, fmt.Sprintf("/interrupts/%d", index))
	}
	for index := range ir.DMARequests {
		required = append(required, fmt.Sprintf("/dma_requests/%d", index))
	}
	for index := range ir.Transactions {
		required = append(required, fmt.Sprintf("/transactions/%d", index))
	}
	for index := range ir.Faults {
		required = append(required, fmt.Sprintf("/faults/%d", index))
	}
	for index := range ir.PowerStates {
		required = append(required, fmt.Sprintf("/power_states/%d", index))
	}
	for _, pointer := range required {
		entries := ir.Grounding[pointer]
		if len(entries) == 0 {
			return fmt.Errorf("grounding is missing for %s", pointer)
		}
		for _, entry := range entries {
			if !allowed[entry] {
				return fmt.Errorf("grounding %s references an unapproved source locator", pointer)
			}
		}
	}
	return nil
}

func (r Registry) persistGrounded(request GroundedRequest, ir IR, references []GroundingReference, hashes map[string]string, raw, requestID string, attempts int, promptHash string) (Package, error) {
	root, _ := filepath.Abs(r.Workspace)
	modelRoot := filepath.Join(root, ".aether", "models", request.ID, request.Version)
	if _, err := os.Stat(modelRoot); err == nil {
		return Package{}, errors.New("model version already exists")
	}
	if err := os.MkdirAll(filepath.Dir(modelRoot), 0o700); err != nil {
		return Package{}, err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(modelRoot), ".grounded-")
	if err != nil {
		return Package{}, err
	}
	defer os.RemoveAll(temporary)
	responseDigest := sha256.Sum256([]byte(raw))
	if err := writeModelJSON(filepath.Join(temporary, "behavior.ir.json"), ir); err != nil {
		return Package{}, err
	}
	if err := writeModelJSON(filepath.Join(temporary, "grounding-manifest.json"), GroundingManifest{Sources: references}); err != nil {
		return Package{}, err
	}
	if err := writeModelJSON(filepath.Join(temporary, "generation-receipt.json"), GenerationReceipt{Provider: "openai", Model: request.Model, PromptTemplateVersion: request.PromptTemplateVersion, RequestSHA256: promptHash, ResponseSHA256: hex.EncodeToString(responseDigest[:]), ProviderRequestID: requestID, Attempts: attempts}); err != nil {
		return Package{}, err
	}
	artifacts := []string{}
	if request.Backend == ael.BackendRenode {
		source, err := GenerateRenodeCSharp(ir, "Ael.Generated")
		if err != nil {
			return Package{}, err
		}
		if err := os.WriteFile(filepath.Join(temporary, "GeneratedPeripheral.cs"), []byte(source), 0o600); err != nil {
			return Package{}, err
		}
		artifacts = append(artifacts, filepath.ToSlash(filepath.Join(".aether", "models", request.ID, request.Version, "GeneratedPeripheral.cs")))
	}
	packageValue := Package{APIVersion: APIVersion, Kind: "ModelPackage", ID: request.ID, Name: request.Name, Version: request.Version, Backend: request.Backend, State: StateGenerated, SourcePaths: request.Sources, SourceHashes: hashes, IRPath: filepath.ToSlash(filepath.Join(".aether", "models", request.ID, request.Version, "behavior.ir.json")), ArtifactPaths: artifacts, GeneratedBy: "openai:" + request.Model, GroundingManifestPath: filepath.ToSlash(filepath.Join(".aether", "models", request.ID, request.Version, "grounding-manifest.json")), GenerationReceiptPath: filepath.ToSlash(filepath.Join(".aether", "models", request.ID, request.Version, "generation-receipt.json")), CreatedAt: time.Now().UTC()}
	if err := writeModelJSON(filepath.Join(temporary, "package.json"), packageValue); err != nil {
		return Package{}, err
	}
	if err := os.Rename(temporary, modelRoot); err != nil {
		return Package{}, err
	}
	return packageValue, nil
}
