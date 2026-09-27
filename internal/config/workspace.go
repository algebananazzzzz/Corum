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
	TaskTracker string           `yaml:"task_tracker,omitempty"`
	Workspace   WorkspaceDetails `yaml:"workspace"`
	Canvas      *CanvasWorkspace `yaml:"canvas,omitempty"`
	Jira        *JiraWorkspace   `yaml:"jira,omitempty"`
	Kaneo       *KaneoWorkspace  `yaml:"kaneo,omitempty"`
}

type WorkspaceDetails struct {
	Timezone string `yaml:"timezone"`
	Term     string `yaml:"term"`
}

type CanvasWorkspace struct {
	URL string `yaml:"url"`
}

// JiraWorkspace names the site and project whose epics hold course work.
// Init saves the site; the agent adds the project on first sync. The
// Atlassian MCP server accepts the site URL wherever a cloud ID is expected.
type JiraWorkspace struct {
	Site    string `yaml:"site"`
	Project string `yaml:"project,omitempty"`
}

// KaneoWorkspace names the self-hosted instance and the one project shared by
// every course; course labels separate the courses inside it. Init saves the
// URL for the MCP entry, and the agent adds the project on first sync.
type KaneoWorkspace struct {
	URL     string `yaml:"url"`
	Project string `yaml:"project,omitempty"`
}

const projectGitignore = "/canvas.json\n"

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
