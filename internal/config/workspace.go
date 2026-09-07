// Package config loads and validates Corum's clean v2 YAML configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Workspace is the project-wide .config/corum/corum.yaml document.
type Workspace struct {
	Version   int              `yaml:"version"`
	Workspace WorkspaceDetails `yaml:"workspace"`
	Canvas    *CanvasWorkspace `yaml:"canvas,omitempty"`
	Jira      *JiraWorkspace   `yaml:"jira,omitempty"`
	Wiki      *WikiWorkspace   `yaml:"wiki,omitempty"`
	Calendar  Calendar         `yaml:"calendar"`
}

type WorkspaceDetails struct {
	Timezone string `yaml:"timezone"`
	Term     string `yaml:"term"`
}

type CanvasWorkspace struct {
	URL string `yaml:"url"`
}

type JiraWorkspace struct {
	CloudID     string            `yaml:"cloud_id"`
	Project     string            `yaml:"project"`
	Transitions map[string]string `yaml:"transitions"`
}

type WikiWorkspace struct{}

type Calendar struct {
	Timetable string `yaml:"timetable"`
	Term      string `yaml:"term"`
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

// MigrateWorkspace moves valid v2 root configuration into project-local storage.
// It refuses ambiguous layouts and validates before writing.
func MigrateWorkspace(root string) (bool, error) {
	legacy := filepath.Join(root, "corum.yaml")
	current := WorkspacePath(root)
	_, legacyErr := os.Stat(legacy)
	_, currentErr := os.Stat(current)
	legacyExists := legacyErr == nil
	currentExists := currentErr == nil
	if legacyErr != nil && !os.IsNotExist(legacyErr) {
		return false, legacyErr
	}
	if currentErr != nil && !os.IsNotExist(currentErr) {
		return false, currentErr
	}
	if legacyExists && currentExists {
		return false, errors.New("both corum.yaml and .config/corum/corum.yaml exist; remove the unintended copy")
	}
	if !legacyExists {
		return false, nil
	}
	if _, err := loadWorkspaceFile(legacy); err != nil {
		return false, err
	}
	if err := EnsureProjectDir(root); err != nil {
		return false, err
	}
	if err := os.Rename(legacy, current); err != nil {
		return false, err
	}
	return true, nil
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

// rejectVersionOne returns migration guidance for a v1 vault instead of a
// generic unknown-field failure, because v1 layouts are not upgradeable.
func rejectVersionOne(node *yaml.Node) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Value == "version" && value.Tag == "!!int" && value.Value == "1" {
			return errors.New("corum.yaml is a version 1 vault; Corum v2 requires a version 2 vault, and v1 vaults cannot be migrated in place")
		}
	}
	return nil
}

func decodeFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	if err := rejectNullServiceBlocks(&node); err != nil {
		return err
	}
	if err := rejectCredentialKeys(&node); err != nil {
		return err
	}
	if err := rejectVersionOne(&node); err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("configuration has more than one YAML document")
		}
		return err
	}
	return nil
}
