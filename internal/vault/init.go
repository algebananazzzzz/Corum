// Package vault owns initialization, validation, and replacement
// of the small toolkit boundary inside a Corum vault.
package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/algebananazzzzz/Corum/internal/config"
	"gopkg.in/yaml.v3"
)

// Initialize creates a new, empty vault. Configuration and embedded assets are fully checked before the target directory is created.
func Initialize(root string, workspace config.Workspace, assets fs.FS, toolkitVersion string) error {
	if err := config.ValidateWorkspace(workspace); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	payload, err := collectToolkit(assets, toolkitVersion)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	if info, err := os.Stat(root); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("refusing to initialize non-directory target %q", root)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("refusing to initialize non-empty target %q", root)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	contents, err := yaml.Marshal(workspace)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := config.EnsureProjectDir(root); err != nil {
		return err
	}
	if err := os.WriteFile(config.WorkspacePath(root), contents, 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "courses"), 0o755); err != nil {
		return err
	}
	return installPayload(root, payload)
}

// WriteWorkspace atomically replaces the project-local corum.yaml after validation.
func WriteWorkspace(root string, workspace config.Workspace) error {
	if err := config.ValidateWorkspace(workspace); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	data, err := yaml.Marshal(workspace)
	if err != nil {
		return err
	}
	if err := config.EnsureProjectDir(root); err != nil {
		return err
	}
	return writeConfigAtomic(config.WorkspacePath(root), data)
}

func writeConfigAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".corum-*.yaml")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
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
	return os.Rename(temporary, path)
}
