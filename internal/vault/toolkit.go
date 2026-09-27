package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var toolkitLinks = []struct{ path, target string }{
	{"CLAUDE.md", "AGENTS.md"},
	{".claude/skills", "../skills"},
	{".codex/skills", "../skills"},
	{".agents/skills", "../skills"},
}

var retiredToolkitPaths = []string{
	"skills/authoring-wiki",
	"skills/drawio-diagrams",
	"skills/linting-wiki",
	"skills/scope-course",
	"skills/sync-course/references/error-handling.md",
	"skills/sync-course/references/tracker-snapshot.md",
}

type assetFile struct {
	path string
	data []byte
}

// RefreshToolkit explicitly installs the embedded instructions and skills.
// Course content and unrelated user skills are never replaced.
func RefreshToolkit(root string, assets fs.FS) error {
	payload, err := collectToolkit(assets)
	if err != nil {
		return err
	}
	if err := installPayload(root, payload); err != nil {
		return err
	}
	// Remove only formerly bundled files, not user skills or pages.
	for _, retired := range retiredToolkitPaths {
		path, err := toolkitPath(root, retired)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func collectToolkit(assets fs.FS) ([]assetFile, error) {
	agents, err := fs.ReadFile(assets, "agent-kit/AGENTS.base.md")
	if err != nil {
		return nil, err
	}
	payload := []assetFile{{path: "AGENTS.md", data: agents}}
	err = fs.WalkDir(assets, "agent-kit/skills", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		payload = append(payload, assetFile{path: strings.TrimPrefix(path, "agent-kit/"), data: data})
		return nil
	})
	return payload, err
}

// toolkitPath rejects symlinks in managed paths before writing or removing.
func toolkitPath(root, relative string) (string, error) {
	path := root
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == ".." || part == "" {
			return "", fmt.Errorf("invalid toolkit path")
		}
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("toolkit path must not be a symlink: %s", relative)
		}
	}
	return path, nil
}

// UserSectionMarker splits AGENTS.md: Corum owns the text above it, and the
// user's own instructions below it survive toolkit updates.
const UserSectionMarker = "<!-- Your instructions go below this line. corum toolkit update keeps them. -->"

// mergeAgents keeps the user section of an installed AGENTS.md, or the base's
// starter section when the vault has none yet.
func mergeAgents(base, installed []byte) ([]byte, error) {
	managed, _, found := strings.Cut(string(base), UserSectionMarker)
	if !found {
		return nil, fmt.Errorf("AGENTS.base.md is missing the user section marker")
	}
	_, user, found := strings.Cut(string(installed), UserSectionMarker)
	if !found {
		return base, nil
	}
	return []byte(managed + UserSectionMarker + user), nil
}

func installPayload(root string, payload []assetFile) error {
	for _, file := range payload {
		target, err := toolkitPath(root, file.path)
		if err != nil {
			return err
		}
		if file.path == "AGENTS.md" {
			installed, err := os.ReadFile(target)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if file.data, err = mergeAgents(file.data, installed); err != nil {
				return err
			}
		}
		if err := writeConfigAtomic(target, file.data); err != nil {
			return err
		}
	}
	for _, link := range toolkitLinks {
		if _, err := toolkitPath(root, filepath.Dir(link.path)); err != nil {
			return err
		}
		path := filepath.Join(root, link.path)
		if target, err := os.Readlink(path); err == nil && target == link.target {
			continue
		}
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to replace existing path: %s", link.path)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(link.target, path); err != nil {
			return err
		}
	}
	return nil
}
