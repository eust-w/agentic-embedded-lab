package plugins

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryInstallPermissionDiffRollbackAndRevoke(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(nil)
	registry := Registry{Root: t.TempDir(), Trust: StaticTrustStore{"official": publicKey}}
	first := makePluginPackage(t, privateKey, "1.0.0", []Permission{PermissionFiles})
	installed, err := registry.Install(first, false)
	if err != nil || !installed.Active {
		t.Fatalf("install: %#v %v", installed, err)
	}
	second := makePluginPackage(t, privateKey, "1.1.0", []Permission{PermissionFiles, PermissionNetwork})
	if _, err := registry.Install(second, false); err == nil {
		t.Fatal("permission escalation did not require confirmation")
	} else {
		var change *PermissionChangeError
		if !errors.As(err, &change) || len(change.Added) != 1 || change.Added[0] != PermissionNetwork {
			t.Fatalf("unexpected permission change: %v", err)
		}
	}
	if _, err := registry.Install(second, true); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := registry.Rollback("fixture", "1.0.0")
	if err != nil || rolledBack.Manifest.Version != "1.0.0" {
		t.Fatalf("rollback failed: %#v %v", rolledBack, err)
	}
	if err := registry.Revoke("fixture", "compromised"); err != nil {
		t.Fatal(err)
	}
	current, err := registry.Current("fixture")
	if err != nil || !current.Revoked || current.Active {
		t.Fatalf("revoke failed: %#v %v", current, err)
	}
}

func makePluginPackage(t *testing.T, privateKey ed25519.PrivateKey, version string, permissions []Permission) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codex-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, err := Sign(Manifest{APIVersion: ManifestVersion, ID: "fixture", Name: "Fixture", Version: version, Permissions: permissions}, "official", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, ".codex-plugin", "plugin.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
