package instructions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Source struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Result struct {
	Content string   `json:"content"`
	Sources []Source `json:"sources"`
	Bytes   int      `json:"bytes"`
	Limited bool     `json:"limited"`
}

func Discover(globalDirectory, projectRoot, currentDirectory string, maxBytes int) (Result, error) {
	if maxBytes <= 0 {
		maxBytes = 32 << 10
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return Result{}, err
	}
	current, err := filepath.Abs(currentDirectory)
	if err != nil {
		return Result{}, err
	}
	relative, err := filepath.Rel(root, current)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Result{}, errors.New("current directory is outside project root")
	}
	var candidates []string
	if globalDirectory != "" {
		if path := firstExisting(globalDirectory); path != "" {
			candidates = append(candidates, path)
		}
	}
	directory := root
	if path := firstExisting(directory); path != "" {
		candidates = append(candidates, path)
	}
	if relative != "." {
		for _, part := range strings.Split(relative, string(filepath.Separator)) {
			directory = filepath.Join(directory, part)
			if path := firstExisting(directory); path != "" {
				candidates = append(candidates, path)
			}
		}
	}
	var result Result
	var builder strings.Builder
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, fmt.Errorf("read instructions %s: %w", path, err)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		separator := ""
		if builder.Len() > 0 {
			separator = "\n\n"
		}
		remaining := maxBytes - builder.Len() - len(separator)
		if remaining <= 0 {
			result.Limited = true
			break
		}
		if len(content) > remaining {
			content = content[:remaining]
			result.Limited = true
		}
		builder.WriteString(separator)
		builder.WriteString(content)
		result.Sources = append(result.Sources, Source{Path: path, Content: content})
		if result.Limited {
			break
		}
	}
	result.Content = builder.String()
	result.Bytes = builder.Len()
	return result, nil
}

func firstExisting(directory string) string {
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		path := filepath.Join(directory, name)
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}
