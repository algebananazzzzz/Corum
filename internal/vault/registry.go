package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type registry struct {
	Version int      `json:"version"`
	Vaults  []string `json:"vaults"`
}

func registryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "corum", "vaults.json"), nil
}

// Register adds an absolute vault path to the versioned per-user registry.
func Register(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	paths, err := Registered()
	if err != nil {
		return err
	}
	paths = append(paths, root)
	return writeRegistry(paths)
}

// Registered returns sorted, deduplicated absolute registered vault paths.
func Registered() ([]string, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var value registry
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("read vault registry: %w", err)
	}
	if value.Version != 2 {
		return nil, fmt.Errorf("unsupported vault registry version %d", value.Version)
	}
	return normalizePaths(value.Vaults)
}

func normalizePaths(paths []string) ([]string, error) {
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("vault registry contains non-absolute path %q", path)
		}
		set[filepath.Clean(path)] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for path := range set {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func writeRegistry(paths []string) error {
	paths, err := normalizePaths(paths)
	if err != nil {
		return err
	}
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(registry{Version: 2, Vaults: paths})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vaults-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
