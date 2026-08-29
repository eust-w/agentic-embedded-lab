package plugins

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ManifestVersion = "aether.plugin/v1"

type Permission string

const (
	PermissionFiles         Permission = "files"
	PermissionCommands      Permission = "commands"
	PermissionNetwork       Permission = "network"
	PermissionBrowser       Permission = "browser"
	PermissionComputerUse   Permission = "computer_use"
	PermissionSecrets       Permission = "secrets"
	PermissionExternalWrite Permission = "external_write"
)

type ProcessEntry struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments,omitempty"`
	Protocol   string   `json:"protocol"`
}

type Manifest struct {
	APIVersion  string        `json:"api_version"`
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Description string        `json:"description"`
	Permissions []Permission  `json:"permissions"`
	Skills      []string      `json:"skills,omitempty"`
	Hooks       []string      `json:"hooks,omitempty"`
	MCP         []string      `json:"mcp,omitempty"`
	WASM        []string      `json:"wasm,omitempty"`
	Process     *ProcessEntry `json:"process,omitempty"`
	KeyID       string        `json:"key_id,omitempty"`
	Signature   string        `json:"signature,omitempty"`
}

type TrustStore interface {
	PublicKey(keyID string) (ed25519.PublicKey, bool)
}

type StaticTrustStore map[string]ed25519.PublicKey

func (s StaticTrustStore) PublicKey(keyID string) (ed25519.PublicKey, bool) {
	key, ok := s[keyID]
	return key, ok
}

func LoadManifest(path string, trust TrustStore, developmentMode bool) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode plugin manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	if manifest.Signature == "" {
		if developmentMode {
			return manifest, nil
		}
		return Manifest{}, errors.New("unsigned plugins require development mode")
	}
	key, ok := trust.PublicKey(manifest.KeyID)
	if !ok {
		return Manifest{}, errors.New("plugin signing key is not trusted")
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil {
		return Manifest{}, errors.New("plugin signature is not valid base64")
	}
	payload, err := manifest.signingPayload()
	if err != nil {
		return Manifest{}, err
	}
	if !ed25519.Verify(key, payload, signature) {
		return Manifest{}, errors.New("plugin signature verification failed")
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.APIVersion != ManifestVersion || m.ID == "" || m.Name == "" || m.Version == "" {
		return errors.New("plugin api_version, id, name, and version are required")
	}
	for _, relative := range append(append(append([]string{}, m.Skills...), m.Hooks...), m.WASM...) {
		if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("plugin path escapes package: %s", relative)
		}
	}
	if m.Process != nil && (filepath.IsAbs(m.Process.Executable) || m.Process.Protocol != "grpc") {
		return errors.New("process plugins require a package-relative executable and grpc protocol")
	}
	return nil
}

func (m Manifest) signingPayload() ([]byte, error) {
	m.Signature = ""
	return json.Marshal(m)
}

func Sign(manifest Manifest, keyID string, privateKey ed25519.PrivateKey) (Manifest, error) {
	manifest.KeyID = keyID
	manifest.Signature = ""
	payload, err := manifest.signingPayload()
	if err != nil {
		return Manifest{}, err
	}
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	return manifest, nil
}
