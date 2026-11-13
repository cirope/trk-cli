package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cirope/trk-cli/internal/api"
	"github.com/zalando/go-keyring"
)

// ErrNoToken means no session is stored for the host.
var ErrNoToken = errors.New("not signed in")

// ErrKeyringNotFound is what a Keyring returns for a missing item.
var ErrKeyringNotFound = keyring.ErrNotFound

const keyringService = "trk-cli"

// TokenStore keeps one token per host.
type TokenStore interface {
	Get(host string) (api.Token, error)
	Set(host string, token api.Token) error
	Delete(host string) error
}

// Keyring is the operating system's keychain.
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

// SystemKeyring is the real keychain: macOS Keychain, Windows Credential
// Manager or the Secret Service on Linux.
type SystemKeyring struct{}

// Get implements Keyring.
func (SystemKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }

// Set implements Keyring.
func (SystemKeyring) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

// Delete implements Keyring.
func (SystemKeyring) Delete(service, user string) error { return keyring.Delete(service, user) }

// Store keeps tokens in the keychain and, where there is none (a
// container, a headless server), in File. Notice is called with the
// file's path the first time a token is written there.
type Store struct {
	Keyring Keyring
	File    *FileStore
	Notice  func(path string)
}

// Get implements TokenStore.
func (s *Store) Get(host string) (api.Token, error) {
	v, err := s.Keyring.Get(keyringService, host)
	if err == nil {
		return api.Token(v), nil
	}
	return s.File.Get(host)
}

// Set implements TokenStore.
func (s *Store) Set(host string, token api.Token) error {
	if err := s.Keyring.Set(keyringService, host, token.Reveal()); err == nil {
		return s.File.Delete(host) // An older file copy must not linger.
	}
	_, statErr := os.Stat(s.File.Path)
	if err := s.File.Set(host, token); err != nil {
		return err
	}
	if errors.Is(statErr, os.ErrNotExist) && s.Notice != nil {
		s.Notice(s.File.Path)
	}
	return nil
}

// Delete implements TokenStore.
func (s *Store) Delete(host string) error {
	err := s.Keyring.Delete(keyringService, host)
	fileErr := s.File.Delete(host)
	if err != nil && !errors.Is(err, ErrKeyringNotFound) && fileErr != nil {
		return fmt.Errorf("forgetting the session: %w", fileErr)
	}
	return fileErr
}

// FileStore keeps tokens in a JSON file only its owner can read.
type FileStore struct {
	Path string
	mu   sync.Mutex
}

type credentials struct {
	Tokens map[string]string `json:"tokens"`
}

func (f *FileStore) read() (credentials, error) {
	c := credentials{Tokens: map[string]string{}}
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("reading %s: %w", f.Path, err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		// The decoder's message can quote the file; never show it.
		return c, fmt.Errorf("reading %s: the file is damaged; delete it and run trk login", f.Path)
	}
	if c.Tokens == nil {
		c.Tokens = map[string]string{}
	}
	return c, nil
}

// Get implements TokenStore.
func (f *FileStore) Get(host string) (api.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.read()
	if err != nil {
		return "", err
	}
	v, ok := c.Tokens[host]
	if !ok {
		return "", ErrNoToken
	}
	return api.Token(v), nil
}

// Set implements TokenStore.
func (f *FileStore) Set(host string, token api.Token) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.read()
	if err != nil {
		return err
	}
	c.Tokens[host] = token.Reveal()
	return f.write(c)
}

// Delete implements TokenStore. Deleting nothing is not an error.
func (f *FileStore) Delete(host string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.read()
	if err != nil {
		return err
	}
	if _, ok := c.Tokens[host]; !ok {
		return nil
	}
	delete(c.Tokens, host)
	return f.write(c)
}

func (f *FileStore) write(c credentials) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(f.Path, b, 0o600)
}

// MemoryStore is a TokenStore for tests.
type MemoryStore struct {
	mu     sync.Mutex
	tokens map[string]api.Token
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{tokens: map[string]api.Token{}} }

// Get implements TokenStore.
func (m *MemoryStore) Get(host string) (api.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[host]
	if !ok {
		return "", ErrNoToken
	}
	return t, nil
}

// Set implements TokenStore.
func (m *MemoryStore) Set(host string, token api.Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[host] = token
	return nil
}

// Delete implements TokenStore.
func (m *MemoryStore) Delete(host string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, host)
	return nil
}

// Config is trk's non-secret settings, kept as JSON in the platform's
// config directory.
type Config struct {
	Host string `json:"host,omitempty"` // The default host.
}

// LoadConfig reads the config at path; a missing file is an empty config.
func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path) //nolint:gosec // trk's own config file.
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("reading %s: %w", path, err)
	}
	return c, nil
}

// Save writes the config to path.
func (c Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

// writeFileAtomic writes data to a temporary file beside path and renames
// it into place, creating the directory (0700) if needed.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(perm); err != nil && !isWindows() {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func isWindows() bool { return filepath.Separator == '\\' }
