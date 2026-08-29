package secret

import (
	"errors"
	"testing"
)

func TestMemoryStoreCopiesAndDeletesSecrets(t *testing.T) {
	store := NewMemoryStore()
	input := []byte("secret")
	if err := store.Set("dev.aether.desktop", "openai", input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	value, err := store.Get("dev.aether.desktop", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "secret" {
		t.Fatalf("unexpected secret copy: %q", value)
	}
	if err := store.Delete("dev.aether.desktop", "openai"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("dev.aether.desktop", "openai"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
