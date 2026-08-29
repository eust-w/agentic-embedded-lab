package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

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
