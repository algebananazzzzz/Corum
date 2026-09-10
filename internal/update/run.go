package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Run updates the installed executable only when explicitly requested.
// It returns the latest release tag and whether the executable was replaced.
func Run(ctx context.Context, version, apiBase string) (string, bool, error) {
	if _, ok := parseVersion(version); !ok {
		return "", false, errors.New("development build cannot self-update; install a release build")
	}
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return "", false, fmt.Errorf("unsupported update platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if apiBase == "" {
		apiBase = GitHubAPI
	}
	client := &http.Client{Timeout: 60 * time.Second}
	release, err := Latest(ctx, client, apiBase)
	if err != nil {
		return "", false, err
	}
	if _, ok := parseVersion(release.Tag); !ok {
		return "", false, fmt.Errorf("unsupported release version: %s", release.Tag)
	}
	if !Newer(release.Tag, version) {
		return release.Tag, false, nil
	}
	name := AssetName(release.Tag, runtime.GOOS, runtime.GOARCH)
	if release.Assets[name] == "" || release.Assets["checksums.txt"] == "" {
		return "", false, errors.New("release is missing the platform archive or checksums.txt")
	}
	archive, err := fetch(ctx, client, release.Assets[name])
	if err != nil {
		return "", false, err
	}
	checksums, err := fetch(ctx, client, release.Assets["checksums.txt"])
	if err != nil {
		return "", false, err
	}
	if err := Verify(archive, name, string(checksums)); err != nil {
		return "", false, err
	}
	binary, err := Extract(archive)
	if err != nil {
		return "", false, err
	}
	executable, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", false, err
	}
	if err := replace(executable, binary); err != nil {
		return "", false, fmt.Errorf("replace executable: %w", err)
	}
	return release.Tag, true, nil
}

// Stage beside the executable so rename is atomic and failures preserve it.
func replace(executable string, binary []byte) error {
	if len(binary) == 0 {
		return errors.New("archive contains an empty corum binary")
	}
	info, err := os.Stat(executable)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(executable), ".corum-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(binary); err != nil {
		return err
	}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), executable)
}
