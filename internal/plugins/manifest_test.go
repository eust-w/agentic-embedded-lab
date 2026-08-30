package plugins

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSignedManifestLoadsAndTamperingFails(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Sign(Manifest{APIVersion: ManifestVersion, ID: "renode", Name: "Renode", Version: "1.0.0", Permissions: []Permission{PermissionCommands}}, "official", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plugin.json")
	writeManifest(t, path, manifest)
	if _, err := LoadManifest(path, StaticTrustStore{"official": publicKey}, false); err != nil {
		t.Fatal(err)
	}
	manifest.Description = "tampered"
	writeManifest(t, path, manifest)
	if _, err := LoadManifest(path, StaticTrustStore{"official": publicKey}, false); err == nil {
		t.Fatal("expected tampered manifest to fail")
	}
}

func TestTrustStoreLoadsStrictEd25519Keys(t *testing.T) {
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	path := filepath.Join(t.TempDir(), "keys.json")
	payload, _ := json.Marshal(map[string]string{"official": base64.StdEncoding.EncodeToString(publicKey)})
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := LoadTrustStore(path)
	if err != nil || len(store["official"]) != ed25519.PublicKeySize {
		t.Fatalf("trust store: %#v %v", store, err)
	}
}

func TestUnsignedManifestRequiresDevelopmentMode(t *testing.T) {
	manifest := Manifest{APIVersion: ManifestVersion, ID: "local", Name: "Local", Version: "0.1.0"}
	path := filepath.Join(t.TempDir(), "plugin.json")
	writeManifest(t, path, manifest)
	if _, err := LoadManifest(path, StaticTrustStore{}, false); err == nil {
		t.Fatal("expected unsigned plugin to fail")
	}
	if _, err := LoadManifest(path, StaticTrustStore{}, true); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRejectsPathLikePluginIdentity(t *testing.T) {
	manifest := Manifest{APIVersion: ManifestVersion, ID: "../escape", Name: "Escape", Version: "1.0.0"}
	if err := manifest.Validate(); err == nil {
		t.Fatal("path-like plugin id was accepted")
	}
}

func writeManifest(t *testing.T, path string, manifest Manifest) {
	t.Helper()
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}
