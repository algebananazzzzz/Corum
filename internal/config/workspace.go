// Package config loads and validates Corum's clean v2 YAML configuration.
package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Workspace is the root corum.yaml document.
type Workspace struct {
	Version   int              `yaml:"version"`
	Workspace WorkspaceDetails `yaml:"workspace"`
	Canvas    *CanvasWorkspace `yaml:"canvas"`
	Jira      *JiraWorkspace   `yaml:"jira"`
	Wiki      *WikiWorkspace   `yaml:"wiki"`
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

// LoadWorkspace reads corum.yaml below root without changing the vault.
func LoadWorkspace(root string) (Workspace, error) {
	var workspace Workspace
	path := filepath.Join(root, "corum.yaml")
	if err := decodeFile(path, &workspace); err != nil {
		return Workspace{}, fmt.Errorf("load workspace: %w", err)
	}
	if err := validateWorkspace(workspace); err != nil {
		return Workspace{}, fmt.Errorf("load workspace: %w", err)
	}
	return workspace, nil
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
	if err := rejectCredentialKeys(&node); err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytesReader(data))
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
