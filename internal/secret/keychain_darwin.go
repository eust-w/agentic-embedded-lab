//go:build darwin

package secret

import (
	"errors"

	"github.com/keybase/go-keychain"
)

type KeychainStore struct{}

func NewKeychainStore() *KeychainStore { return &KeychainStore{} }

func (s *KeychainStore) Set(service, account string, value []byte) error {
	_ = s.Delete(service, account)
	item := keychain.NewGenericPassword(service, account, "Aether Desktop credential", value, "")
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlockedThisDeviceOnly)
	return keychain.AddItem(item)
}

func (s *KeychainStore) Get(service, account string) ([]byte, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetAccount(account)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNotFound
	}
	return append([]byte(nil), results[0].Data...), nil
}

func (s *KeychainStore) Delete(service, account string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	err := keychain.DeleteItem(item)
	if errors.Is(err, keychain.ErrorItemNotFound) {
		return nil
	}
	return err
}
