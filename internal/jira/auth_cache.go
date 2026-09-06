package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/oauth2"
)

const authCacheVersion = 1

type authRecord struct {
	Version      int           `json:"version"`
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

// AuthCachePath is deliberately outside a vault so credentials never travel
// with workspace files.
func AuthCachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate Jira authentication cache: %w", err)
	}
	return filepath.Join(dir, "corum", "auth.json"), nil
}

// ClearAuth removes only Corum's OAuth cache. It is safe to call repeatedly.
func ClearAuth() (bool, error) {
	path, err := AuthCachePath()
	if err != nil {
		return false, err
	}
	return clearAuth(path)
}

// AuthSnapshot preserves the raw, private cache around an interactive vault
// configuration transaction. It is intentionally opaque to callers.
type AuthSnapshot struct {
	path   string
	bytes  []byte
	exists bool
}

func SnapshotAuth() (AuthSnapshot, error) {
	path, err := AuthCachePath()
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
	if record.Version != authCacheVersion || record.Token == nil {
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
	if record.Version == 0 {
		record.Version = authCacheVersion
	}
	if record.Version != authCacheVersion || record.Token == nil {
		return errors.New("refusing to save incomplete Jira authentication cache")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Jira authentication cache directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure Jira authentication cache directory: %w", err)
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
	// The released binary targets POSIX platforms. On those systems, reject a
	// cache owned by another account before it is read.
	if runtime.GOOS != "windows" {
		if uid, ok := fileUID(info); ok && uid != os.Getuid() {
			return errors.New("Jira authentication cache has unsafe ownership")
		}
	}
	return nil
}
