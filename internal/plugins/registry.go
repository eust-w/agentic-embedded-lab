package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Installed struct {
	Manifest  Manifest  `json:"manifest"`
	Path      string    `json:"path"`
	Active    bool      `json:"active"`
	Revoked   bool      `json:"revoked"`
	Installed time.Time `json:"installed_at"`
}

type PermissionChangeError struct {
	Added []Permission
}

func (e *PermissionChangeError) Error() string {
	return fmt.Sprintf("plugin requests additional permissions: %v", e.Added)
}

type Registry struct {
	Root            string
	Trust           TrustStore
	DevelopmentMode bool
}

func (r Registry) Install(source string, approvePermissions bool) (Installed, error) {
	root, err := filepath.Abs(r.Root)
	if err != nil || root == "" {
		return Installed{}, errors.New("plugin registry root is required")
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return Installed{}, err
	}
	manifestPath := filepath.Join(source, ".codex-plugin", "plugin.json")
	if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
		manifestPath = filepath.Join(source, "plugin.json")
	}
	manifest, err := LoadManifest(manifestPath, r.Trust, r.DevelopmentMode)
	if err != nil {
		return Installed{}, err
	}
	if err := validatePackageFiles(source, manifest); err != nil {
		return Installed{}, err
	}
	if current, err := r.Current(manifest.ID); err == nil {
		added := addedPermissions(current.Manifest.Permissions, manifest.Permissions)
		if len(added) > 0 && !approvePermissions {
			return Installed{}, &PermissionChangeError{Added: added}
		}
	}
	versionRoot := filepath.Join(root, manifest.ID, "versions")
	if err := os.MkdirAll(versionRoot, 0o700); err != nil {
		return Installed{}, err
	}
	destination := filepath.Join(versionRoot, manifest.Version)
	if _, err := os.Stat(destination); err == nil {
		return Installed{}, errors.New("plugin version is already installed")
	}
	temporary, err := os.MkdirTemp(versionRoot, ".install-")
	if err != nil {
		return Installed{}, err
	}
	defer os.RemoveAll(temporary)
	if err := copyPackage(source, temporary); err != nil {
		return Installed{}, err
	}
	if manifest.Process != nil {
		if err := os.Chmod(filepath.Join(temporary, manifest.Process.Executable), 0o700); err != nil {
			return Installed{}, err
		}
	}
	if err := os.Rename(temporary, destination); err != nil {
		return Installed{}, err
	}
	if err := writeCurrent(root, manifest.ID, manifest.Version); err != nil {
		return Installed{}, err
	}
	_ = os.Remove(filepath.Join(root, manifest.ID, "revoked"))
	return Installed{Manifest: manifest, Path: destination, Active: true, Installed: time.Now().UTC()}, nil
}

func (r Registry) Current(id string) (Installed, error) {
	root, err := filepath.Abs(r.Root)
	if err != nil || id == "" || strings.ContainsAny(id, `/\\`) {
		return Installed{}, errors.New("valid registry root and plugin id are required")
	}
	pluginRoot := filepath.Join(root, id)
	versionData, err := os.ReadFile(filepath.Join(pluginRoot, "current"))
	if err != nil {
		return Installed{}, err
	}
	version := strings.TrimSpace(string(versionData))
	path := filepath.Join(pluginRoot, "versions", version)
	manifestPath := filepath.Join(path, ".codex-plugin", "plugin.json")
	if _, err := os.Stat(manifestPath); errors.Is(err, os.ErrNotExist) {
		manifestPath = filepath.Join(path, "plugin.json")
	}
	manifest, err := LoadManifest(manifestPath, r.Trust, r.DevelopmentMode)
	if err != nil {
		return Installed{}, err
	}
	_, revokeErr := os.Stat(filepath.Join(pluginRoot, "revoked"))
	return Installed{Manifest: manifest, Path: path, Active: revokeErr != nil, Revoked: revokeErr == nil}, nil
}

func (r Registry) Rollback(id, version string) (Installed, error) {
	root, err := filepath.Abs(r.Root)
	if err != nil || id == "" || version == "" || strings.ContainsAny(id+version, `/\\`) {
		return Installed{}, errors.New("valid plugin id and version are required")
	}
	if _, err := os.Stat(filepath.Join(root, id, "versions", version)); err != nil {
		return Installed{}, err
	}
	if err := writeCurrent(root, id, version); err != nil {
		return Installed{}, err
	}
	return r.Current(id)
}

func (r Registry) Revoke(id, reason string) error {
	root, err := filepath.Abs(r.Root)
	if err != nil || id == "" || strings.ContainsAny(id, `/\\`) {
		return errors.New("valid plugin id is required")
	}
	if _, err := r.Current(id); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"reason": strings.TrimSpace(reason), "revoked_at": time.Now().UTC()})
	return os.WriteFile(filepath.Join(root, id, "revoked"), append(payload, '\n'), 0o600)
}

func (r Registry) Versions(id string) ([]string, error) {
	root, err := filepath.Abs(r.Root)
	if err != nil || id == "" || strings.ContainsAny(id, `/\\`) {
		return nil, errors.New("valid plugin id is required")
	}
	entries, err := os.ReadDir(filepath.Join(root, id, "versions"))
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			versions = append(versions, entry.Name())
		}
	}
	sort.Strings(versions)
	return versions, nil
}

func (r Registry) List() ([]Installed, error) {
	root, err := filepath.Abs(r.Root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []Installed
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		installed, err := r.Current(entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, installed)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Manifest.ID < result[j].Manifest.ID })
	return result, nil
}

func validatePackageFiles(root string, manifest Manifest) error {
	paths := append(append(append(append([]string{}, manifest.Skills...), manifest.Hooks...), manifest.MCP...), manifest.WASM...)
	if manifest.Process != nil {
		paths = append(paths, manifest.Process.Executable)
	}
	for _, relative := range paths {
		path := filepath.Join(root, relative)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("plugin resource %s: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin resource %s may not be a symlink", relative)
		}
	}
	return nil
}

func copyPackage(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin packages may not contain symlinks: %s", relative)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if info, err := entry.Info(); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o700
		}
		return os.WriteFile(target, data, mode)
	})
}

func writeCurrent(root, id, version string) error {
	path := filepath.Join(root, id, "current")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(version+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func addedPermissions(previous, next []Permission) []Permission {
	existing := make(map[Permission]bool, len(previous))
	for _, permission := range previous {
		existing[permission] = true
	}
	var added []Permission
	for _, permission := range next {
		if !existing[permission] {
			added = append(added, permission)
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i] < added[j] })
	return added
}
