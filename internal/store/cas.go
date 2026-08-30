package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var artifactDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *Store) PutArtifact(reader io.Reader) (string, string, error) {
	temp, err := os.CreateTemp(filepath.Join(s.root, "cas"), "artifact-*")
	if err != nil {
		return "", "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temp, hash), reader); err != nil {
		_ = temp.Close()
		return "", "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", "", err
	}
	if err := temp.Close(); err != nil {
		return "", "", err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	destination := filepath.Join(s.root, "cas", "sha256", digest[:2], digest)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", "", err
	}
	if _, err := os.Stat(destination); err == nil {
		return digest, destination, nil
	}
	if err := os.Rename(tempName, destination); err != nil {
		return "", "", fmt.Errorf("commit artifact: %w", err)
	}
	return digest, destination, nil
}

func (s *Store) ArtifactPath(digest string) (string, error) {
	if !artifactDigest.MatchString(digest) {
		return "", errors.New("invalid artifact digest")
	}
	path := filepath.Join(s.root, "cas", "sha256", digest[:2], digest)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("artifact not found")
	}
	return path, nil
}
