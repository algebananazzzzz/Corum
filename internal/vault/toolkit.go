package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/lockfile"
)

const toolkitTemporaryPrefix = ".corum-toolkit-"

const toolkitLockName = ".toolkit.lock"

var removeAll = os.RemoveAll

var toolkitLinks = []struct{ path, target string }{
	{"CLAUDE.md", "AGENTS.md"},
	{".claude/skills", "../skills"},
	{".codex/skills", "../skills"},
	{".agents/skills", "../skills"},
}

type assetFile struct {
	path string
	data []byte
	mode fs.FileMode
}

type replacement struct {
	target    string
	stage     string
	backup    string
	hadOld    bool
	installed bool
}

// SyncToolkit updates the toolkit only for the active project. Current
// toolkits are left untouched.
func SyncToolkit(root string, assets fs.FS, version string) error {
	if toolkitIsCurrent(root, version) {
		return nil
	}
	return syncToolkit(root, assets, version, os.Rename)
}

// RefreshToolkit replaces the complete toolkit with the version embedded in Corum.
func RefreshToolkit(root string, assets fs.FS, version string) error {
	return syncToolkit(root, assets, version, os.Rename)
}

func toolkitIsCurrent(root, version string) bool {
	data, err := os.ReadFile(filepath.Join(root, ".corum", "toolkit-version"))
	if err != nil || string(data) != version+"\n" {
		return false
	}
	for _, link := range toolkitLinks {
		path := filepath.Join(root, link.path)
		if parent := filepath.Dir(path); parent != filepath.Clean(root) {
			if info, err := os.Lstat(parent); err != nil || !info.IsDir() {
				return false
			}
		}
		target, err := os.Readlink(path)
		if err != nil || target != link.target {
			return false
		}
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}

func syncToolkit(root string, assets fs.FS, version string, rename func(string, string) error) error {
	corumDir := filepath.Join(root, ".corum")
	if err := os.MkdirAll(corumDir, 0o755); err != nil {
		return err
	}
	lock, err := lockfile.TryAcquire(filepath.Join(corumDir, toolkitLockName))
	if err != nil {
		return err
	}
	defer lock.Close()
	payload, err := collectToolkit(assets, version)
	if err != nil {
		return err
	}
	// Under the lock, stale stages are disposable; backups may still be needed for recovery.
	if err := cleanToolkitTemporaryFiles(root, false); err != nil {
		return err
	}
	id := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	stageRoot := filepath.Join(corumDir, toolkitTemporaryPrefix+"stage-"+id)
	defer removeAll(stageRoot)
	if err := installPayload(stageRoot, payload); err != nil {
		return err
	}
	items := []string{"AGENTS.md", "skills", "templates"}
	for _, link := range toolkitLinks {
		items = append(items, link.path)
	}
	items = append(items, ".corum/toolkit-version")
	replacements := make([]replacement, 0, len(items))
	for _, item := range items {
		parent := filepath.Dir(filepath.Join(root, item))
		if info, err := os.Lstat(parent); err == nil {
			if parent != filepath.Clean(root) && !info.IsDir() {
				return fmt.Errorf("toolkit parent %s must be a directory, not a symlink or file", parent)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		name := strings.ReplaceAll(item, "/", "-")
		replacements = append(replacements, replacement{
			target: filepath.Join(root, filepath.FromSlash(item)),
			stage:  filepath.Join(stageRoot, filepath.FromSlash(item)),
			backup: filepath.Join(corumDir, toolkitTemporaryPrefix+"backup-"+id+"-"+name),
		})
	}
	for index := range replacements {
		item := &replacements[index]
		if err := os.MkdirAll(filepath.Dir(item.target), 0o755); err != nil {
			return rollbackResult(err, rollback(replacements, rename))
		}
		if _, err := os.Lstat(item.target); err == nil {
			if err := rename(item.target, item.backup); err != nil {
				return rollbackResult(fmt.Errorf("backup %s: %w", item.target, err), rollback(replacements, rename))
			}
			item.hadOld = true
		} else if !os.IsNotExist(err) {
			return rollbackResult(err, rollback(replacements, rename))
		}
		if err := rename(item.stage, item.target); err != nil {
			return rollbackResult(fmt.Errorf("install %s: %w", item.target, err), rollback(replacements, rename))
		}
		item.installed = true
	}
	for _, item := range replacements {
		if item.hadOld {
			_ = removeAll(item.backup)
		}
	}
	if err := cleanToolkitTemporaryFiles(root, true); err != nil {
		return fmt.Errorf("clean completed toolkit transaction: %w", err)
	}
	return nil
}

func rollbackResult(cause, rollbackErr error) error {
	if rollbackErr == nil {
		return cause
	}
	return errors.Join(cause, fmt.Errorf("rollback failure: %w", rollbackErr))
}

func rollback(items []replacement, rename func(string, string) error) error {
	var errs []error
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if item.installed {
			if err := removeAll(item.target); err != nil {
				errs = append(errs, fmt.Errorf("remove replacement %s: %w", item.target, err))
				continue
			}
		}
		if item.hadOld {
			if err := rename(item.backup, item.target); err != nil {
				errs = append(errs, fmt.Errorf("restore %s: %w", item.target, err))
			}
		}
	}
	return errors.Join(errs...)
}

func cleanToolkitTemporaryFiles(root string, cleanBackups bool) error {
	corumDir := filepath.Join(root, ".corum")
	entries, err := os.ReadDir(corumDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, toolkitTemporaryPrefix+"stage-") || (cleanBackups && strings.HasPrefix(name, toolkitTemporaryPrefix+"backup-")) {
			if err := removeAll(filepath.Join(corumDir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectToolkit(assets fs.FS, version string) ([]assetFile, error) {
	payload := []assetFile{}
	agents, err := fs.ReadFile(assets, "agent-kit/AGENTS.base.md")
	if err != nil {
		return nil, fmt.Errorf("read embedded AGENTS.md: %w", err)
	}
	payload = append(payload, assetFile{path: "AGENTS.md", data: agents, mode: 0o644})
	for _, source := range []string{"agent-kit/skills", "agent-kit/templates"} {
		err := fs.WalkDir(assets, source, func(item string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			data, err := fs.ReadFile(assets, item)
			if err != nil {
				return err
			}
			rel := strings.TrimPrefix(item, "agent-kit/")
			payload = append(payload, assetFile{path: filepath.FromSlash(rel), data: data, mode: entry.Type().Perm()})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read embedded toolkit %s: %w", source, err)
		}
	}
	payload = append(payload, assetFile{path: filepath.Join(".corum", "toolkit-version"), data: []byte(version + "\n"), mode: 0o644})
	return payload, nil
}

func installPayload(root string, payload []assetFile) error {
	for _, file := range payload {
		target := filepath.Join(root, file.path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := file.mode.Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, file.data, mode); err != nil {
			return err
		}
	}
	for _, link := range toolkitLinks {
		target := filepath.Join(root, link.path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(link.target, target); err != nil {
			return fmt.Errorf("link %s: %w", target, err)
		}
	}
	return nil
}
