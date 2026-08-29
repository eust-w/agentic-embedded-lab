package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SkillMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type Skill struct {
	SkillMetadata
	Instructions string `json:"instructions"`
}

func DiscoverSkills(roots []string) ([]SkillMetadata, error) {
	var result []SkillMetadata
	seen := make(map[string]bool)
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name(), "SKILL.md")
			data, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			metadata, _, err := parseSkill(path, string(data))
			if err != nil {
				return nil, err
			}
			if seen[metadata.Name] {
				continue
			}
			seen[metadata.Name] = true
			result = append(result, metadata)
		}
	}
	return result, nil
}

func LoadSkill(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	metadata, instructions, err := parseSkill(path, string(data))
	if err != nil {
		return Skill{}, err
	}
	return Skill{SkillMetadata: metadata, Instructions: instructions}, nil
}

func parseSkill(path, content string) (SkillMetadata, string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 4 || strings.TrimSpace(lines[0]) != "---" {
		return SkillMetadata{}, "", errors.New("SKILL.md requires YAML frontmatter")
	}
	metadata := SkillMetadata{Path: path}
	end := -1
	for index := 1; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "---" {
			end = index
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"")
		switch strings.TrimSpace(key) {
		case "name":
			metadata.Name = value
		case "description":
			metadata.Description = value
		}
	}
	if end < 0 || metadata.Name == "" || metadata.Description == "" {
		return SkillMetadata{}, "", fmt.Errorf("invalid SKILL.md metadata in %s", path)
	}
	return metadata, strings.TrimSpace(strings.Join(lines[end+1:], "\n")), nil
}
