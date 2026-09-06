package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const toolkitTemporaryPrefix = ".corum-toolkit-"

var removeAll = os.RemoveAll

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

// SyncResult reports one independently attempted registered vault.
type SyncResult struct {
	Root string
	Err  error
}

// SyncToolkits updates each valid registered vault independently. Invalid and
// missing entries remain registered but are reported instead of blocking others.
func SyncToolkits(assets fs.FS, version string) []SyncResult {
	roots, err := Registered()
	if err != nil {
		return []SyncResult{{Err: err}}
	}
	results := make([]SyncResult, 0, len(roots))
	for _, root := range roots {
		if _, _, err := Validate(root); err != nil {
			results = append(results, SyncResult{Root: root, Err: err})
			continue
		}
		if toolkitIsCurrent(root, version) {
			results = append(results, SyncResult{Root: root})
			continue
		}
		err := syncToolkit(root, assets, version, os.Rename)
		results = append(results, SyncResult{Root: root, Err: err})
	}
	return results
}

func toolkitIsCurrent(root, version string) bool {
	data, err := os.ReadFile(filepath.Join(root, ".corum", "toolkit-version"))
	return err == nil && string(data) == version+"\n"
}

func syncToolkit(root string, assets fs.FS, version string, rename func(string, string) error) error {
	payload, err := collectToolkit(assets, version)
	if err != nil {
		return err
	}
	if err := cleanToolkitTemporaryFiles(root, false); err != nil {
		return err
	}
	corumDir := filepath.Join(root, ".corum")
	if err := os.MkdirAll(corumDir, 0o755); err != nil {
		return err
	}
	id := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	stageRoot := filepath.Join(corumDir, toolkitTemporaryPrefix+"stage-"+id)
	if err := installPayload(stageRoot, payload); err != nil {
		return err
	}
	defer removeAll(stageRoot)
	items := []struct{ name string }{{"AGENTS.md"}, {"skills"}, {"templates"}, {".corum/toolkit-version"}}
	replacements := make([]replacement, 0, len(items))
	for _, item := range items {
		name := strings.ReplaceAll(item.name, "/", "-")
		replacements = append(replacements, replacement{
			target: filepath.Join(root, filepath.FromSlash(item.name)),
			stage:  filepath.Join(stageRoot, filepath.FromSlash(item.name)),
			backup: filepath.Join(corumDir, toolkitTemporaryPrefix+"backup-"+id+"-"+name),
		})
	}
	for index := range replacements {
		item := &replacements[index]
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
	return nil
}
