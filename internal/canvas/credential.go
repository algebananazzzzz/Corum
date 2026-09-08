package canvas

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/algebananazzzzz/Corum/internal/config"
)

const credentialVersion = 1

// ErrNoCredential means no Canvas token is stored or supplied.
var ErrNoCredential = errors.New("no Canvas token found; run corum configure canvas")

type credentialFile struct {
	Version int    `json:"version"`
	Token   string `json:"token"`
}

// CredentialPathFor returns the project-local credential path.
func CredentialPathFor(root string) (string, error) {
	if root == "" {
		return "", errors.New("Canvas credential requires a project root")
	}
	return filepath.Join(config.ProjectDir(root), "canvas.json"), nil
}

// LoadCredential returns the effective Canvas token: environment first, then
// the project-local credential.
func LoadCredential(root string) (string, error) {
	if token := os.Getenv("CORUM_CANVAS_TOKEN"); token != "" {
		return token, nil
	}
	path, err := CredentialPathFor(root)
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
	if err := config.EnsureProjectDir(root); err != nil {
		return err
	}
	data, err := json.Marshal(credentialFile{Version: credentialVersion, Token: token})
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o600, ".auth-*.json")
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
