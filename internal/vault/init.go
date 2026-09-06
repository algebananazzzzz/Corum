// Package vault owns initialization, validation, registration, and replacement
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

// Initialize creates a new, empty v2 vault. Configuration and embedded assets
// are fully checked before the target directory is created.
func Initialize(root string, workspace config.Workspace, assets fs.FS, toolkitVersion string) error {
	if err := validateWorkspaceForInit(workspace); err != nil {
		return err
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
	if err := os.WriteFile(filepath.Join(root, "corum.yaml"), contents, 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "courses"), 0o755); err != nil {
		return err
	}
	if err := installPayload(root, payload); err != nil {
		return err
	}
	return Register(root)
}

func validateWorkspaceForInit(value config.Workspace) error {
	if err := config.ValidateWorkspace(value); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	return nil
}
