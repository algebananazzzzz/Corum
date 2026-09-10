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
func Initialize(root string, workspace config.Workspace, assets fs.FS) error {
	if err := config.ValidateWorkspace(workspace); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	calendar, err := selectedCalendarAsset(workspace, assets)
	if err != nil {
		return err
	}
	payload, err := collectToolkit(assets)
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
	if calendar != nil {
		if err := os.WriteFile(filepath.Join(root, "Term_Calendar.md"), calendar, 0o644); err != nil {
			return err
		}
	}
	return installPayload(root, payload)
}

func selectedCalendarAsset(workspace config.Workspace, assets fs.FS) ([]byte, error) {
	assetByTerm := map[string]string{
		"AY2026/27 Semester 1": "agent-kit/assets/calendar/ay2026_27_semester_1.md",
		"AY2026/27 Semester 2": "agent-kit/assets/calendar/ay2026_27_semester_2.md",
	}
	asset, ok := assetByTerm[workspace.Workspace.Term]
	if !ok {
		return nil, nil
	}
	data, err := fs.ReadFile(assets, asset)
	if err != nil {
		return nil, fmt.Errorf("read calendar asset: %w", err)
	}
	return data, nil
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

// WriteCourse atomically replaces an existing course configuration after
// validating the supplied course and its target path.
func WriteCourse(root string, course config.Course) error {
	if err := config.ValidateCourse(course); err != nil {
		return fmt.Errorf("validate course: %w", err)
	}
	if _, err := config.LoadCourse(root, course.Code); err != nil {
		return err
	}
	data, err := yaml.Marshal(course)
	if err != nil {
		return err
	}
	return writeConfigAtomic(filepath.Join(root, "courses", course.Code, "course.yaml"), data)
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
