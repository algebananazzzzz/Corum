package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/oauth2"
)

type authRecord struct {
	ClientID     string        `json:"client_id"`
	ClientSecret string        `json:"client_secret,omitempty"`
	Issuer       string        `json:"issuer"`
	AuthURL      string        `json:"auth_url"`
	TokenURL     string        `json:"token_url"`
	AuthStyle    int           `json:"auth_style"`
	RedirectURL  string        `json:"redirect_url"`
	Scopes       []string      `json:"scopes"`
	Token        *oauth2.Token `json:"token"`
}

// AuthCachePathFor returns the project-local cache path under a validated
// vault.
func AuthCachePathFor(root string) (string, error) {
	if root == "" {
		return "", errors.New("Jira authentication cache requires a project root")
	}
	return filepath.Join(root, ".config", "corum", "auth.json"), nil
}

// AuthSnapshot preserves the raw, private cache around an interactive vault
// configuration transaction. It is intentionally opaque to callers.
type AuthSnapshot struct {
	path   string
	bytes  []byte
	exists bool
}

// SnapshotAuthFor preserves the raw cache for a vault.
func SnapshotAuthFor(root string) (AuthSnapshot, error) {
	path, err := AuthCachePathFor(root)
	if err != nil {
		return AuthSnapshot{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return AuthSnapshot{path: path}, nil
	}
	if err != nil {
		return AuthSnapshot{}, err
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return AuthSnapshot{}, err
	}
	if err := checkPrivateDirectory(dir); err != nil {
		return AuthSnapshot{}, err
	}
	if err := checkPrivateFile(info); err != nil {
		return AuthSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AuthSnapshot{}, err
	}
	return AuthSnapshot{path: path, bytes: append([]byte(nil), data...), exists: true}, nil
}

func (s AuthSnapshot) Restore() error {
	if !s.exists {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".auth-restore-*.json")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(s.bytes); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, s.path)
}

// ensureCredentialGuard adds a gitignore so project-local credentials cannot
// be committed.
func ensureCredentialGuard(dir string) error {
	guard := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(guard); err == nil {
		return nil
	}
	if err := os.WriteFile(guard, []byte("/auth.json\n/canvas.json\n/.auth-*.json\n"), 0o600); err != nil {
		return fmt.Errorf("write credential guard: %w", err)
	}
	return nil
}

func clearAuth(path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("clear Jira authentication cache: %w", err)
	}
	return true, nil
}

func loadAuthCache(path string) (authRecord, error) {
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return authRecord{}, err
	}
	if err := checkPrivateDirectory(dir); err != nil {
		return authRecord{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return authRecord{}, err
	}
	if err := checkPrivateFile(info); err != nil {
		return authRecord{}, err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return authRecord{}, fmt.Errorf("read Jira authentication cache: %w", err)
	}
	var record authRecord
	if err := json.Unmarshal(bytes, &record); err != nil {
		return authRecord{}, errors.New("Jira authentication cache is corrupt")
	}
	if record.Token == nil {
		return authRecord{}, errors.New("Jira authentication cache has an unsupported format")
	}
	return record, nil
}

func checkPrivateDirectory(info os.FileInfo) error {
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return errors.New("Jira authentication cache directory has unsafe permissions")
	}
	return checkPrivateOwnership(info)
}

func saveAuthCache(path string, record authRecord) (err error) {
	if record.Token == nil {
		return errors.New("refusing to save incomplete Jira authentication cache")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Jira authentication cache directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure Jira authentication cache directory: %w", err)
	}
	if err := ensureCredentialGuard(dir); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return errors.New("encode Jira authentication cache")
	}
	file, err := os.CreateTemp(dir, ".auth-*.json")
	if err != nil {
		return fmt.Errorf("create Jira authentication cache: %w", err)
	}
	temporary := file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(temporary)
		}
	}()
	if err = file.Chmod(0o600); err == nil {
		_, err = file.Write(append(encoded, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write Jira authentication cache: %w", err)
	}
	if err = os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace Jira authentication cache: %w", err)
	}
	return nil
}

func checkPrivateFile(info os.FileInfo) error {
	if info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("Jira authentication cache has unsafe permissions")
	}
	return checkPrivateOwnership(info)
}

func checkPrivateOwnership(info os.FileInfo) error {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Getuid() {
		return errors.New("Jira authentication cache has unsafe ownership")
	}
	return nil
}
