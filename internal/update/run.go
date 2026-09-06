package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/minio/selfupdate"
)

const ReexecEnv = "CORUM_UPDATE_REEXEC"

// ContinuationArg replaces "update" in the argv of a re-executed process after
// an explicit self-update, so the new process reports the outcome instead of
// performing a second metadata check.
const ContinuationArg = "update-continuation"

// Options supplies process and platform state to the updater. The zero values
// use the running process, current platform, GitHub API, and user cache.
type Options struct {
	Version    string
	APIBase    string
	CachePath  string
	Disable    bool
	GOOS       string
	GOARCH     string
	Executable string
	Args       []string
	Env        []string

	now         func() time.Time
	checkClient *http.Client
	workClient  *http.Client
	apply       func(io.Reader) error
	reexec      func(string, []string, []string) error
}

// Outcome describes an update check or replacement without changing the exit
// status of the command that initiated an automatic check.
type Outcome struct {
	Checked bool
	Updated bool
	Latest  string
	Warning string
	Skipped string
}

// Maybe performs the at-most-daily automatic update check. Operational errors
// are converted to a warning so the requested Corum command can still run.
func Maybe(ctx context.Context, options Options) (Outcome, error) {
	options = defaults(options)
	if options.Version == "dev" || options.Disable || os.Getenv("DISABLE_AUTO_UPDATES") == "1" || os.Getenv(ReexecEnv) == "1" {
		return Outcome{Skipped: "disabled"}, nil
	}
	now := options.now()
	previous, ok := readCache(options.CachePath)
	if ok && now.Sub(previous.CheckedAt) < checkInterval {
		return Outcome{Latest: previous.Latest, Skipped: "recently checked"}, nil
	}
	attempt := cache{Version: cacheVersion, CheckedAt: now, Latest: previous.Latest}
	if err := writeCache(options.CachePath, attempt); err != nil {
		return automaticWarning(err), nil
	}
	outcome := Outcome{Checked: true, Latest: previous.Latest}
	release, err := Latest(ctx, options.checkClient, options.APIBase)
	if err != nil {
		outcome.Warning = fmt.Sprintf("automatic update check failed: %v", err)
		return outcome, nil
	}
	outcome.Latest = release.Tag
	attempt.Latest = release.Tag
	if err := writeCache(options.CachePath, attempt); err != nil {
		outcome.Warning = fmt.Sprintf("automatic update cache failed: %v", err)
	}
	if !Newer(release.Tag, options.Version) {
		return outcome, nil
	}
	updated, err := installRelease(ctx, options, release)
	if err != nil {
		outcome.Warning = fmt.Sprintf("automatic update failed: %v", err)
		return outcome, nil
	}
	if !updated {
		outcome.Skipped = "another update is in progress"
		return outcome, nil
	}
	outcome.Updated = true
	return outcome, nil
}

// Run performs an explicit update, bypassing the automatic-check cache.
func Run(ctx context.Context, options Options) (Outcome, error) {
	options = defaults(options)
	if options.Version == "dev" {
		return Outcome{}, errors.New("development build cannot self-update; install a release build")
	}
	if err := validatePlatform(options.GOOS, options.GOARCH); err != nil {
		return Outcome{}, err
	}
	release, err := Latest(ctx, options.workClient, options.APIBase)
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{Checked: true, Latest: release.Tag}
	_ = writeCache(options.CachePath, cache{Version: cacheVersion, CheckedAt: options.now(), Latest: release.Tag})
	if !Newer(release.Tag, options.Version) {
		return outcome, nil
	}
	options.Args = explicitUpdateContinuationArgs(options.Args)
	updated, err := installRelease(ctx, options, release)
	if err != nil {
		return outcome, err
	}
	if !updated {
		outcome.Skipped = "another update is in progress"
		return outcome, nil
	}
	outcome.Updated = true
	return outcome, nil
}

// explicitUpdateContinuationArgs swaps the "update" argv slot for the
// continuation slot so the re-executed process does not re-run the explicit
// update check.
func explicitUpdateContinuationArgs(args []string) []string {
	result := make([]string, len(args))
	copy(result, args)
	for i, arg := range result {
		if arg == "update" {
			result[i] = ContinuationArg
			break
		}
	}
	return result
}

func installRelease(ctx context.Context, options Options, release Release) (bool, error) {
	if err := validatePlatform(options.GOOS, options.GOARCH); err != nil {
		return false, err
	}
	lock, acquired := acquireLock(filepath.Join(filepath.Dir(options.CachePath), "update.lock"))
	if !acquired {
		return false, nil
	}
	defer lock.release()

	name := AssetName(release.Tag, options.GOOS, options.GOARCH)
	archiveURL, ok := release.Assets[name]
	if !ok {
		return false, fmt.Errorf("release %s has no asset %s", release.Tag, name)
	}
	checksumsURL, ok := release.Assets["checksums.txt"]
	if !ok {
		return false, fmt.Errorf("release %s has no checksums.txt", release.Tag)
	}
	archive, err := fetch(ctx, options.workClient, archiveURL)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", name, err)
	}
	checksums, err := fetch(ctx, options.workClient, checksumsURL)
	if err != nil {
		return false, fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := Verify(archive, name, string(checksums)); err != nil {
		return false, err
	}
	binary, err := Extract(archive)
	if err != nil {
		return false, err
	}
	if err := options.apply(bytes.NewReader(binary)); err != nil {
		return false, fmt.Errorf("replace executable: %w", err)
	}
	environment := setEnv(options.Env, ReexecEnv, "1")
	if err := options.reexec(options.Executable, options.Args, environment); err != nil {
		return false, fmt.Errorf("re-exec updated binary: %w", err)
	}
	return true, nil
}

func defaults(options Options) Options {
	if options.APIBase == "" {
		options.APIBase = GitHubAPI
	}
	if options.CachePath == "" {
		options.CachePath = CachePath()
	}
	if options.GOOS == "" {
		options.GOOS = runtime.GOOS
	}
	if options.GOARCH == "" {
		options.GOARCH = runtime.GOARCH
	}
	if options.now == nil {
		options.now = time.Now
	}
	if options.checkClient == nil {
		options.checkClient = &http.Client{Timeout: 2 * time.Second}
	}
	if options.workClient == nil {
		options.workClient = &http.Client{Timeout: 60 * time.Second}
	}
	if options.Executable == "" {
		options.Executable, _ = os.Executable()
	}
	if options.Args == nil {
		options.Args = os.Args
	}
	if options.Env == nil {
		options.Env = os.Environ()
	}
	if options.apply == nil {
		options.apply = func(reader io.Reader) error {
			return selfupdate.Apply(reader, selfupdate.Options{})
		}
	}
	if options.reexec == nil {
		options.reexec = syscall.Exec
	}
	return options
}

func validatePlatform(goos, goarch string) error {
	if (goos != "linux" && goos != "darwin") || (goarch != "amd64" && goarch != "arm64") {
		return fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
	return nil
}

func automaticWarning(err error) Outcome {
	return Outcome{Warning: fmt.Sprintf("automatic update check failed: %v", err)}
}

func setEnv(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

type fileLock struct {
	file *os.File
}

func acquireLock(path string) (*fileLock, bool) {
	if path == "" || path == "." {
		return nil, false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, false
	}
	return &fileLock{file: file}, true
}

func (lock *fileLock) release() {
	if lock == nil || lock.file == nil {
		return
	}
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	_ = lock.file.Close()
}
