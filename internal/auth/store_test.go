package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cirope/trk-cli/internal/api"
)

const host = "https://trk.test"

// fakeKeyring is an in-memory keychain that can be made unavailable.
type fakeKeyring struct {
	items  map[string]string
	broken bool
}

var errNoKeychain = errors.New("no keychain here")

func (k *fakeKeyring) Get(service, user string) (string, error) {
	if k.broken {
		return "", errNoKeychain
	}
	v, ok := k.items[service+"/"+user]
	if !ok {
		return "", ErrKeyringNotFound
	}
	return v, nil
}

func (k *fakeKeyring) Set(service, user, secret string) error {
	if k.broken {
		return errNoKeychain
	}
	if k.items == nil {
		k.items = map[string]string{}
	}
	k.items[service+"/"+user] = secret
	return nil
}

func (k *fakeKeyring) Delete(service, user string) error {
	if k.broken {
		return errNoKeychain
	}
	if _, ok := k.items[service+"/"+user]; !ok {
		return ErrKeyringNotFound
	}
	delete(k.items, service+"/"+user)
	return nil
}

func TestFileStore(t *testing.T) {
	dir := t.TempDir()
	fs := &FileStore{Path: filepath.Join(dir, "trk", "credentials.json")}

	if _, err := fs.Get(host); !errors.Is(err, ErrNoToken) {
		t.Fatalf("Get on nothing = %v, want ErrNoToken", err)
	}
	if err := fs.Set(host, "tok_a"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Set("https://other.test", "tok_b"); err != nil {
		t.Fatal(err)
	}
	if got, err := fs.Get(host); err != nil || got.Reveal() != "tok_a" {
		t.Errorf("Get = %q, %v", got.Reveal(), err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(fs.Path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("file mode = %o, want 600", perm)
		}
		dinfo, _ := os.Stat(filepath.Dir(fs.Path))
		if perm := dinfo.Mode().Perm(); perm != 0o700 {
			t.Errorf("dir mode = %o, want 700", perm)
		}
	}
	if err := fs.Delete(host); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Get(host); !errors.Is(err, ErrNoToken) {
		t.Errorf("Get after Delete = %v, want ErrNoToken", err)
	}
	if got, _ := fs.Get("https://other.test"); got.Reveal() != "tok_b" {
		t.Errorf("Delete removed another host's token")
	}
	if err := fs.Delete(host); err != nil {
		t.Errorf("Delete of nothing = %v, want nil", err)
	}
}

func TestFileStoreCorruptFileErrorHasNoToken(t *testing.T) {
	dir := t.TempDir()
	fs := &FileStore{Path: filepath.Join(dir, "credentials.json")}
	if err := os.WriteFile(fs.Path, []byte(`{"tokens":{"x":"tok_secret_123"`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := fs.Get(host)
	if err == nil || strings.Contains(err.Error(), "tok_secret_123") {
		t.Errorf("err = %v, want an error without the token", err)
	}
}

func TestStoreUsesTheKeychain(t *testing.T) {
	kr := &fakeKeyring{}
	fs := &FileStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	var notices []string
	s := &Store{Keyring: kr, File: fs, Notice: func(p string) { notices = append(notices, p) }}

	if err := s.Set(host, "tok"); err != nil {
		t.Fatal(err)
	}
	if kr.items[keyringService+"/"+host] != "tok" {
		t.Errorf("keychain = %v, want the token under %s", kr.items, keyringService)
	}
	if _, err := os.Stat(fs.Path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file was written although the keychain works")
	}
	if got, err := s.Get(host); err != nil || got.Reveal() != "tok" {
		t.Errorf("Get = %q, %v", got.Reveal(), err)
	}
	if err := s.Delete(host); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(host); !errors.Is(err, ErrNoToken) {
		t.Errorf("Get after Delete = %v, want ErrNoToken", err)
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none", notices)
	}
}

func TestStoreFallsBackToAFile(t *testing.T) {
	kr := &fakeKeyring{broken: true}
	fs := &FileStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	var notices []string
	s := &Store{Keyring: kr, File: fs, Notice: func(p string) { notices = append(notices, p) }}

	for range 2 {
		if err := s.Set(host, "tok"); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.Get(host); err != nil || got.Reveal() != "tok" {
		t.Errorf("Get = %q, %v", got.Reveal(), err)
	}
	if len(notices) != 1 || notices[0] != fs.Path {
		t.Errorf("notices = %v, want one naming %s", notices, fs.Path)
	}
	if err := s.Delete(host); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(host); !errors.Is(err, ErrNoToken) {
		t.Errorf("Get after Delete = %v, want ErrNoToken", err)
	}
}

// A token saved to the keychain replaces one left in the file earlier.
func TestStoreKeychainSupersedesFile(t *testing.T) {
	fs := &FileStore{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if err := fs.Set(host, "old"); err != nil {
		t.Fatal(err)
	}
	s := &Store{Keyring: &fakeKeyring{}, File: fs}
	if err := s.Set(host, "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Get(host); !errors.Is(err, ErrNoToken) {
		t.Errorf("the old file token survived: %v", err)
	}
}

func TestMemoryStore(t *testing.T) {
	var s TokenStore = NewMemoryStore()
	if _, err := s.Get(host); !errors.Is(err, ErrNoToken) {
		t.Fatal(err)
	}
	_ = s.Set(host, api.Token("t"))
	if got, _ := s.Get(host); got.Reveal() != "t" {
		t.Errorf("Get = %q", got.Reveal())
	}
	_ = s.Delete(host)
	if _, err := s.Get(host); !errors.Is(err, ErrNoToken) {
		t.Errorf("Get after Delete = %v", err)
	}
}

func TestConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trk", "config.json")
	c, err := LoadConfig(path)
	if err != nil || c.Host != "" {
		t.Fatalf("LoadConfig on nothing = %+v, %v; want empty", c, err)
	}
	c.Host = "https://trk.example"
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	c2, err := LoadConfig(path)
	if err != nil || c2.Host != "https://trk.example" {
		t.Errorf("LoadConfig = %+v, %v", c2, err)
	}
}
