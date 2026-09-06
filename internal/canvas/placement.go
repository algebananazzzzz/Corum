package canvas

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const rootPrefix = "course files/"

// Place maps a Canvas folder to its configured raw-material destination.
func Place(folder, filename string, folders map[string]string) string {
	folder = strings.TrimPrefix(folder, rootPrefix)
	keys := make([]string, 0, len(folders))
	for key := range folders {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		if folder == key || strings.HasPrefix(folder, key+"/") {
			return filepath.ToSlash(filepath.Join(folders[key], filename))
		}
	}
	if folder == "" {
		return filename
	}
	return filepath.ToSlash(filepath.Join(folder, filename))
}

// SafeDestination resolves a relative capture path and rejects escapes.
func SafeDestination(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", fmt.Errorf("unsafe capture path %q", relative)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve raw directory: %w", err)
	}
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write outside raw directory: %q", relative)
	}
	current := root
	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		candidate := filepath.Join(current, part)
		info, statErr := os.Lstat(candidate)
		if os.IsNotExist(statErr) {
			current = candidate
			continue
		}
		if statErr != nil {
			return "", statErr
		}
		resolved := candidate
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, statErr = filepath.EvalSymlinks(candidate)
			if statErr != nil {
				return "", fmt.Errorf("resolve capture parent: %w", statErr)
			}
			if !within(root, resolved) {
				return "", fmt.Errorf("refusing to write through a directory symlink outside raw directory: %q", relative)
			}
		}
		resolvedInfo, statErr := os.Stat(resolved)
		if statErr != nil || !resolvedInfo.IsDir() {
			return "", fmt.Errorf("capture parent is not a directory: %q", relative)
		}
		current = resolved
	}
	target = filepath.Join(current, filepath.Base(target))
	if info, statErr := os.Lstat(target); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("capture target must not be a symlink: %q", relative)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	return target, nil
}

func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func atomicWriteFile(target string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".canvas-write-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(name, target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
