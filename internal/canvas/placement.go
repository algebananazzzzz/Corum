package canvas

import (
	"fmt"
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
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write outside raw directory: %q", relative)
	}
	return target, nil
}
