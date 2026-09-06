// Package config loads and validates Corum's clean v2 YAML configuration.
package config

import (
	"errors"
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
