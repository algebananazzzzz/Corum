// Package config loads and validates Corum YAML configuration.
package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Workspace is the project-wide .config/corum/corum.yaml document.
type Workspace struct {
	Workspace WorkspaceDetails `yaml:"workspace"`
	Canvas    *CanvasWorkspace `yaml:"canvas,omitempty"`
	Jira      *JiraWorkspace   `yaml:"jira,omitempty"`
}

type WorkspaceDetails struct {
	Timezone string `yaml:"timezone"`
	Term     string `yaml:"term"`
}

type CanvasWorkspace struct {
	URL string `yaml:"url"`
}

type JiraWorkspace struct {
	CloudID string `yaml:"cloud_id"`
	Project string `yaml:"project"`
}

const projectGitignore = "/auth.json\n/canvas.json\n/.auth-*.json\n"

// ProjectDir returns the project-local directory for Corum configuration.
func ProjectDir(root string) string {
	return filepath.Join(root, ".config", "corum")
}

// WorkspacePath returns the project-local workspace configuration path.
func WorkspacePath(root string) string {
	return filepath.Join(ProjectDir(root), "corum.yaml")
}

// EnsureProjectDir creates private configuration storage without replacing an existing gitignore.
func EnsureProjectDir(root string) error {
	dir := ProjectDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	guard := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(guard); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(guard, []byte(projectGitignore), 0o600)
}

// LoadWorkspace reads .config/corum/corum.yaml below root without changing
// the vault.
func LoadWorkspace(root string) (Workspace, error) {
	workspace, err := loadWorkspaceFile(WorkspacePath(root))
	if err != nil {
		return Workspace{}, fmt.Errorf("load workspace: %w", err)
	}
	return workspace, nil
}

func loadWorkspaceFile(path string) (Workspace, error) {
	var workspace Workspace
	if err := decodeFile(path, &workspace); err != nil {
		return Workspace{}, err
	}
	if err := ValidateWorkspace(workspace); err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func decodeFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&node); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("configuration has more than one YAML document")
		}
		return err
	}
	if err := rejectNullServiceBlocks(&node); err != nil {
		return err
	}
	if err := rejectCredentialKeys(&node); err != nil {
		return err
	}
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}
