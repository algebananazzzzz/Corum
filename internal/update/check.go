package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheVersion  = 1
	checkInterval = 24 * time.Hour
)

type cache struct {
	Version   int       `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// CachePath returns the per-user automatic update cache path.
func CachePath() string {
	directory, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(directory, "corum", "update-check.json")
}

func readCache(path string) (cache, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cache{}, false
	}
	var value cache
	if err := json.Unmarshal(data, &value); err != nil || value.Version != cacheVersion || value.CheckedAt.IsZero() {
		return cache{}, false
	}
	return value, true
}

func writeCache(path string, value cache) error {
	if path == "" {
		return errorsNoCacheDir()
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".update-check-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func errorsNoCacheDir() error {
	return fmt.Errorf("could not determine update cache directory")
}
