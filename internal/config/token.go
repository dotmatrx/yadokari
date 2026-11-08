package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// TokenEnvVar overrides every stored token.
const TokenEnvVar = "WANIKANI_API_TOKEN"

const (
	keyringService = "yadokari"
	keyringUser    = "wanikani-api-token"
)

// Source says where a token came from or went to.
type Source string

const (
	FromEnv     Source = "environment"
	FromKeyring Source = "keyring"
	FromFile    Source = "file"
)

// ErrNoToken means no token is configured anywhere.
var ErrNoToken = errors.New("no WaniKani API token configured")

// Keyring is the subset of an OS keyring the token store needs.
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

// ErrKeyringNotFound is what a Keyring returns for a missing secret.
var ErrKeyringNotFound = keyring.ErrNotFound

type osKeyring struct{}

func (osKeyring) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (osKeyring) Set(s, u, p string) error        { return keyring.Set(s, u, p) }
func (osKeyring) Delete(s, u string) error        { return keyring.Delete(s, u) }

// OSKeyring is the desktop keyring (Secret Service on Linux).
var OSKeyring Keyring = osKeyring{}

// TokenStore finds and saves the API token: environment variable first, then
// the keyring, then a 0600 file. Saving prefers the keyring and falls back to
// the file only if the keyring is unavailable.
type TokenStore struct {
	Keyring Keyring
	File    string
	Getenv  func(string) string
}

// Load returns the token and where it came from, or ErrNoToken.
func (t TokenStore) Load() (string, Source, error) {
	if v := strings.TrimSpace(t.Getenv(TokenEnvVar)); v != "" {
		return v, FromEnv, nil
	}

	var keyringErr error
	if t.Keyring != nil {
		v, err := t.Keyring.Get(keyringService, keyringUser)
		switch {
		case err == nil && strings.TrimSpace(v) != "":
			return strings.TrimSpace(v), FromKeyring, nil
		case err != nil && !errors.Is(err, ErrKeyringNotFound):
			keyringErr = err // unavailable: try the file
		}
	}

	v, err := readTokenFile(t.File)
	if err == nil {
		return v, FromFile, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	if keyringErr != nil {
		return "", "", fmt.Errorf("%w (keyring unavailable: %v)", ErrNoToken, keyringErr)
	}
	return "", "", ErrNoToken
}

// Save stores the token in the keyring, or in the file if the keyring fails.
// Whichever is used, the other copy is removed so there is one source of truth.
func (t TokenStore) Save(token string) (Source, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("empty token")
	}
	if t.Keyring != nil {
		if err := t.Keyring.Set(keyringService, keyringUser, token); err == nil {
			if err := os.Remove(t.File); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return FromKeyring, fmt.Errorf("token saved to keyring, but removing old %s failed: %w", t.File, err)
			}
			return FromKeyring, nil
		}
	}
	if err := writeTokenFile(t.File, token); err != nil {
		return "", err
	}
	if t.Keyring != nil {
		_ = t.Keyring.Delete(keyringService, keyringUser) // best effort; it was failing anyway
	}
	return FromFile, nil
}

// Delete removes the token from the keyring and the file.
func (t TokenStore) Delete() error {
	var errs []error
	if t.Keyring != nil {
		if err := t.Keyring.Delete(keyringService, keyringUser); err != nil && !errors.Is(err, ErrKeyringNotFound) {
			errs = append(errs, err)
		}
	}
	if err := os.Remove(t.File); err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func readTokenFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return "", fmt.Errorf("%s is readable by other users (mode %04o); run: chmod 600 %s", path, perm, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(raw))
	if v == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return v, nil
}

func writeTokenFile(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".token-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(token + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
