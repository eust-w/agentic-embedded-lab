//go:build !darwin

package secret

type KeychainStore struct{ *MemoryStore }

func NewKeychainStore() *KeychainStore { return &KeychainStore{NewMemoryStore()} }
