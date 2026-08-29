package ael

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Engine struct {
	Workspace         string
	BackendExecutable string
	BackendTimeout    time.Duration
}

func LoadExperiment(root, path string) (Experiment, error) {
	var value Experiment
	err := loadStrictWorkspaceYAML(root, path, &value)
	return value, err
}

func LoadSystem(root, path string) (System, error) {
	var value System
	err := loadStrictWorkspaceYAML(root, path, &value)
	return value, err
}

func (e Engine) RunFiles(ctx context.Context, experimentPath, systemPath, sourceRevision string) (EvidenceBundle, string, error) {
	root, err := filepath.Abs(e.Workspace)
	if err != nil {
		return EvidenceBundle{}, "", err
	}
	var experiment Experiment
	if err := loadStrictWorkspaceYAML(root, experimentPath, &experiment); err != nil {
		return EvidenceBundle{}, "", err
	}
	var system System
	if err := loadStrictWorkspaceYAML(root, systemPath, &system); err != nil {
		return EvidenceBundle{}, "", err
	}
	if experiment.SystemID != system.ID {
		return EvidenceBundle{}, "", fmt.Errorf("experiment system_id %s does not match system %s", experiment.SystemID, system.ID)
	}
	if err := validateModelPaths(root, system); err != nil {
		return EvidenceBundle{}, "", err
	}
	executable := e.BackendExecutable
	if executable == "" {
		return EvidenceBundle{}, "", errors.New("AEL backend executable is required; silent backend fallback is forbidden")
	}
	timeout := e.BackendTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	factories := make(map[Backend]AdapterFactory)
	for _, component := range system.Components {
		backend := component.Backend
		if backend == BackendHardware {
			return EvidenceBundle{}, "", errors.New("hardware backend requires an independently authorized Lab Worker")
		}
		factories[backend] = func(component Component) (Adapter, error) {
			return NewProcessAdapter(ProcessConfig{Executable: executable, Arguments: []string{"--backend", string(component.Backend), "--workspace", root}, Directory: root, Timeout: timeout})
		}
	}
	bundle, runErr := (Scheduler{Factories: factories}).Run(ctx, experiment, system, sourceRevision)
	if bundle.RunID == "" {
		return bundle, "", runErr
	}
	evidencePath, evidenceErr := (EvidenceWriter{Workspace: root}).Write(bundle)
	if evidenceErr != nil {
		return bundle, "", errors.Join(runErr, evidenceErr)
	}
	return bundle, evidencePath, runErr
}

func loadStrictWorkspaceYAML(root, requested string, target any) error {
	path, err := resolveWorkspacePath(root, requested)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var normalized any
	if err := yaml.Unmarshal(data, &normalized); err != nil {
		return fmt.Errorf("decode %s: %w", requested, err)
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("validate %s: %w", requested, err)
	}
	return nil
}

func resolveWorkspacePath(root, requested string) (string, error) {
	if requested == "" {
		return "", errors.New("workspace path is required")
	}
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
		return "", errors.New("path escapes AEL workspace")
	}
	return path, nil
}

func validateModelPaths(root string, system System) error {
	for _, component := range system.Components {
		if component.Model == "" {
			return fmt.Errorf("component %s has no model", component.ID)
		}
		path, err := resolveWorkspacePath(root, component.Model)
		if err != nil {
			return fmt.Errorf("component %s model: %w", component.ID, err)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("component %s model: %w", component.ID, err)
		}
		for _, key := range []string{"firmware", "source"} {
			value, ok := component.Properties[key].(string)
			if !ok || value == "" {
				continue
			}
			path, err := resolveWorkspacePath(root, value)
			if err != nil {
				return fmt.Errorf("component %s property %s: %w", component.ID, key, err)
			}
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("component %s property %s: %w", component.ID, key, err)
			}
		}
	}
	return nil
}
