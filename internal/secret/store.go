package secret

import (
	"errors"
	"sync"
)

var ErrNotFound = errors.New("secret not found")

type Store interface {
	Set(service, account string, value []byte) error
	Get(service, account string) ([]byte, error)
	Delete(service, account string) error
}

type MemoryStore struct {
	mu     sync.RWMutex
	values map[string][]byte
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{values: make(map[string][]byte)} }

func (s *MemoryStore) Set(service, account string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[service+"\x00"+account] = append([]byte(nil), value...)
	return nil
}

func (s *MemoryStore) Get(service, account string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[service+"\x00"+account]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *MemoryStore) Delete(service, account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, service+"\x00"+account)
	return nil
}
