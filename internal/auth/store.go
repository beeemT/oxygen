package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/99designs/keyring"
)

// Store abstracts credential storage. The key is the full context key
// (e.g. "oxygen/user@example.com/org@host") and the value is the
// Basic auth credential: "Basic base64(email:password)".
type Store interface {
	Store(key, token string) error
	Get(key string) (string, error)
	Delete(key string) error
	List() ([]string, error)
}

// NewKeychain returns a Store backed by the OS keyring (Keychain on macOS,
// libsecret on Linux, Credential Manager on Windows).
func NewKeychain(product string) (Store, error) {
	kr, err := keyring.Open(keyring.Config{
		ServiceName:     product,
		AllowedBackends: keyring.AvailableBackends(),
	})
	if err != nil {
		return nil, fmt.Errorf("opening keyring: %w", err)
	}

	return &keychainStore{kr: kr}, nil
}

// keychainStore wraps a keyring.Keyring.
type keychainStore struct{ kr keyring.Keyring }

func (s *keychainStore) Store(key string, token string) error {
	return s.kr.Set(keyring.Item{Key: key, Data: []byte(token)})
}

func (s *keychainStore) Get(key string) (string, error) {
	item, err := s.kr.Get(key)
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return "", ErrNotFound
		}

		return "", err
	}

	return string(item.Data), nil
}

func (s *keychainStore) Delete(key string) error {
	if err := s.kr.Remove(key); err != nil {
		if err == keyring.ErrKeyNotFound {
			return nil
		}

		return err
	}

	return nil
}

func (s *keychainStore) List() ([]string, error) {
	return s.kr.Keys()
}

// FileStore is an unencrypted file-based credential store used as fallback when
// the OS keyring is unavailable (e.g. headless Linux / CI environments).
// On systems with a keyring implementation, prefer NewKeychain.
type FileStore struct {
	path string
}

// NewFileStore returns a FileStore that stores credentials in a JSON file.
// Use only when OS keyring is unavailable.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating credential directory %q: %w", dir, err)
	}

	return &FileStore{path: filepath.Join(dir, "credentials.json")}, nil
}

func (s *FileStore) Store(key string, token string) error {
	data, err := s.loadAll()
	if err != nil {
		data = make(map[string]string)
	}
	data[key] = token

	return s.saveAll(data)
}

func (s *FileStore) Get(key string) (string, error) {
	data, err := s.loadAll()
	if err != nil {
		return "", ErrNotFound
	}
	v, ok := data[key]
	if !ok {
		return "", ErrNotFound
	}

	return v, nil
}

func (s *FileStore) Delete(key string) error {
	data, err := s.loadAll()
	if err != nil {
		return nil
	}
	delete(data, key)

	return s.saveAll(data)
}

func (s *FileStore) List() ([]string, error) {
	data, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	return keys, nil
}

func (s *FileStore) loadAll() (map[string]string, error) {
	content, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}

		return nil, err
	}
	var data map[string]string
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parsing credentials file: %w", err)
	}

	return data, nil
}

func (s *FileStore) saveAll(data map[string]string) error {
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, content, 0o600)
}

// ContextKey builds the keychain key for a given auth context.
// Format: "oxygen/{user}/{org}@{host}".
func ContextKey(user string, org string, host string) string {
	return strings.Join([]string{"oxygen", user, org + "@" + host}, "/")
}

// ErrNotFound is returned when a credential key is not found.
var ErrNotFound = fmt.Errorf("credential not found")
