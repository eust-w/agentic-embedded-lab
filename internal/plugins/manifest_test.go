package plugins

import (
	"crypto/ed25519"
	"crypto/rand"
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
