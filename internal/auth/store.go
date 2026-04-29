package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/99designs/keyring"
)

// Store abstracts credential storage. The key is the full context key
// (e.g. "openobserve-cli/user@example.com/org@host") and the value is the
// Basic auth credential: "Basic base64(email:password)".
type Store interface {
	Store(key, token string) error
	Get(key string) (string, error)
	Delete(key string) error
	List() ([]string, error)
}

// NewKeychain returns a Store backed by the OS keyring.
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

func (s *keychainStore) Store(key, token string) error {
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

// FileStore is an encrypted-on-disk credential store, used as fallback when
// the OS keyring is unavailable (e.g. headless Linux / CI environments).
type FileStore struct {
	path  string
	gcm   cipher.AEAD
	nonce []byte
}

// NewFileStore returns a FileStore that encrypts credentials at rest using AES-GCM.
// The key is derived from the store directory path via SHA-256; the directory
// itself is protected by filesystem mode 0700.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating credential directory %q: %w", dir, err)
	}
	path := filepath.Join(dir, "credentials.json")

	key := sha256Key(dir)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	return &FileStore{path: path, gcm: gcm, nonce: nonce}, nil
}

// sha256Key derives a 32-byte AES key from the store directory path.
// The directory is already protected by filesystem mode 0700, so this is
// sufficient to prevent casual reading of credentials at rest.
func sha256Key(path string) []byte {
	h := sha256.Sum256([]byte(path))
	return h[:]
}

func (s *FileStore) Store(key, token string) error {
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

type credentialFile struct {
	Entries []credentialEntry `json:"entries"`
}

type credentialEntry struct {
	Key   string `json:"key"`
	Token string `json:"token"`
	Nonce []byte `json:"nonce,omitempty"`
}

func (s *FileStore) loadAll() (map[string]string, error) {
	content, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	var file credentialFile
	if err := json.Unmarshal(content, &file); err != nil {
		return make(map[string]string), nil
	}

	result := make(map[string]string)
	for _, e := range file.Entries {
		if e.Nonce != nil && len(e.Nonce) == s.gcm.NonceSize() {
			plaintext, err := s.gcm.Open(nil, e.Nonce, []byte(e.Token), nil)
			if err == nil {
				result[e.Key] = string(plaintext)
				continue
			}
		}
		result[e.Key] = e.Token
	}
	return result, nil
}

func (s *FileStore) saveAll(data map[string]string) error {
	var entries []credentialEntry
	for k, v := range data {
		nonce := make([]byte, s.gcm.NonceSize())
		copy(nonce, s.nonce)
		ciphertext := s.gcm.Seal(nil, nonce, []byte(v), nil)
		entries = append(entries, credentialEntry{
			Key:   k,
			Token: base64.StdEncoding.EncodeToString(ciphertext),
			Nonce: nonce,
		})
	}
	file := credentialFile{Entries: entries}
	content, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, content, 0600)
}

// ContextKey builds the keychain key for a given auth context.
// Format: "openobserve-cli/{user}/{org}@{host}".
func ContextKey(user, org, host string) string {
	return strings.Join([]string{"openobserve-cli", user, org + "@" + host}, "/")
}

// ErrNotFound is returned when a credential key is not found.
var ErrNotFound = fmt.Errorf("credential not found")
