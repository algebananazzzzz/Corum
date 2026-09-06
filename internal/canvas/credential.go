package canvas

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The Canvas token is a credential, so it lives in a private, ignored
// per-vault directory by default and in the platform user configuration
// directory only when no vault is involved. The CORUM_CANVAS_TOKEN
// environment variable always takes precedence for automation.

const credentialVersion = 1

// ErrNoCredential means no Canvas token is stored or supplied.
var ErrNoCredential = errors.New("no Canvas token found; run corum auth canvas")

type credentialFile struct {
	Version int    `json:"version"`
	Token   string `json:"token"`
}

// CredentialPathFor returns the project-local credential path when root is a
// validated vault and the global fallback path otherwise.
func CredentialPathFor(root string) (string, error) {
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate Canvas credential: %w", err)
		}
		return filepath.Join(dir, "corum", "canvas.json"), nil
	}
	return filepath.Join(root, ".config", "corum", "canvas.json"), nil
}

// ensureCredentialDir creates the private credential directory and a gitignore
// guard so credentials are never committed.
func ensureCredentialDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Canvas credential directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure Canvas credential directory: %w", err)
	}
	guard := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(guard); err != nil {
		if err := os.WriteFile(guard, []byte("*\n! .gitignore\n"), 0o600); err != nil {
			return fmt.Errorf("write Canvas credential guard: %w", err)
		}
	}
	return nil
}

// LoadCredential returns the effective Canvas token and whether a value is
// available: environment first, then project-local, then the global fallback.
func LoadCredential(root string) (string, error) {
	if token := os.Getenv("CORUM_CANVAS_TOKEN"); token != "" {
		return token, nil
	}
	if root != "" {
		path, err := CredentialPathFor(root)
		if err != nil {
			return "", err
		}
		token, err := readCredential(path)
		if err == nil {
			return token, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	path, err := CredentialPathFor("")
	if err != nil {
		return "", err
	}
	token, err := readCredential(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNoCredential
		}
		return "", err
	}
	return token, nil
}

func readCredential(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var value credentialFile
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("Canvas credential is corrupt: %w", err)
	}
	if value.Version != credentialVersion {
		return "", fmt.Errorf("unsupported Canvas credential version %d", value.Version)
	}
	if value.Token == "" {
		return "", fmt.Errorf("Canvas credential has no token")
	}
	return value.Token, nil
}

// SaveCredential writes the token to the effective path with private modes.
func SaveCredential(root, token string) error {
	path, err := CredentialPathFor(root)
	if err != nil {
		return err
	}
	if err := ensureCredentialDir(path); err != nil {
		return err
	}
	data, err := json.Marshal(credentialFile{Version: credentialVersion, Token: token})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write Canvas credential: %w", err)
	}
	if _, err := file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

// ClearCredential removes the credential at the effective path.
func ClearCredential(root string) (bool, error) {
	path, err := CredentialPathFor(root)
	if err != nil {
		return false, err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("clear Canvas credential: %w", err)
	}
	return true, nil
}
