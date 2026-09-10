package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const integrationVersion = "v2.0.0"

type commandResult struct {
	stdout string
	stderr string
	err    error
}

func TestInstallerUsesVerifiedLocalReleaseArtifacts(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("installer supports Linux and macOS")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("installer supports amd64 and arm64")
	}
	binary := buildBinary(t)
	archive := archiveBinary(t, binary)

	t.Run("installs a matching release", func(t *testing.T) {
		server := installerServer(t, archive, false)
		installDirectory := filepath.Join(t.TempDir(), "bin")
		result := runInstaller(t, server.URL, installDirectory)
		mustSucceed(t, result)

		installed := filepath.Join(installDirectory, "corum")
		info, err := os.Stat(installed)
		if err != nil || info.Mode()&0o111 == 0 {
			t.Fatalf("installed binary mode = %v, %v", info, err)
		}
		environment := isolatedEnvironment(t)
		version := runBinary(installed, environment, "", "", "version")
		mustSucceed(t, version)
		if version.stdout != integrationVersion+"\n" {
			t.Fatalf("installed version = %q", version.stdout)
		}
	})

	t.Run("rejects a bad checksum before replacement", func(t *testing.T) {
		server := installerServer(t, archive, true)
		installDirectory := filepath.Join(t.TempDir(), "bin")
		target := filepath.Join(installDirectory, "corum")
		mustWrite(t, target, "existing installation\n")
		if err := os.Chmod(target, 0o755); err != nil {
			t.Fatal(err)
		}

		result := runInstaller(t, server.URL, installDirectory)
		if result.err == nil {
			t.Fatalf("installer accepted bad checksum: stdout = %q, stderr = %q", result.stdout, result.stderr)
		}
		if got := mustRead(t, target); got != "existing installation\n" {
			t.Fatalf("checksum failure replaced binary with %q", got)
		}
	})
}

func TestInstalledBinary(t *testing.T) {
	binary := buildBinary(t)

	t.Run("reports the linked version", func(t *testing.T) {
		environment := isolatedEnvironment(t)
		result := runBinary(binary, environment, "", "", "version")
		mustSucceed(t, result)
		if result.stdout != integrationVersion+"\n" || result.stderr != "" {
			t.Fatalf("version stdout = %q, stderr = %q", result.stdout, result.stderr)
		}
	})

	t.Run("prints root help successfully", func(t *testing.T) {
		environment := isolatedEnvironment(t)
		result := runBinary(binary, environment, "", "", "--help")
		mustSucceed(t, result)
		if !strings.Contains(result.stdout, "usage: corum init") || result.stderr != "" {
			t.Fatalf("help stdout = %q, stderr = %q", result.stdout, result.stderr)
		}
	})

	t.Run("headless init creates a valid project-local vault", func(t *testing.T) {
		environment := isolatedEnvironment(t)
		vault := filepath.Join(environment.root, "vault")
		mustSucceed(t, runBinary(binary, environment, "", "", "init", "--defaults", vault))
		if _, err := os.Stat(filepath.Join(vault, ".config", "corum", "corum.yaml")); err != nil {
			t.Fatalf("project-local configuration: %v", err)
		}
		if _, err := os.Stat(filepath.Join(vault, "corum.yaml")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected root configuration: %v", err)
		}
		for path, target := range map[string]string{
			"CLAUDE.md": "AGENTS.md", ".claude/skills": "../skills",
			".codex/skills": "../skills", ".agents/skills": "../skills",
		} {
			got, err := os.Readlink(filepath.Join(vault, path))
			if err != nil || got != target {
				t.Fatalf("Readlink(%s) = %q, %v; want %q", path, got, err, target)
			}
		}
		for _, directory := range []string{".claude", ".codex", ".agents"} {
			if _, err := os.Stat(filepath.Join(vault, directory, "skills", "sync-course", "SKILL.md")); err != nil {
				t.Fatalf("discover sync-course via %s: %v", directory, err)
			}
		}
		for _, name := range []string{"authoring-wiki", "drawio-diagrams", "linting-wiki"} {
			if _, err := os.Stat(filepath.Join(vault, "skills", name)); !os.IsNotExist(err) {
				t.Fatalf("archived skill installed: %s, %v", name, err)
			}
		}
		syncSkill, err := os.ReadFile(filepath.Join(vault, "skills", "sync-course", "SKILL.md"))
		if err != nil || !strings.Contains(string(syncSkill), "corum jira sync-epic {{COURSE}}") {
			t.Fatalf("installed sync-course skill does not reconcile Jira epics: %v", err)
		}

		result := runBinary(binary, environment, "", "", "doctor", vault)
		mustSucceed(t, result)
		if result.stdout != "doctor: 0 courses\n" {
			t.Fatalf("doctor stdout = %q", result.stdout)
		}
		assertNoGlobalCorumConfig(t, environment.config)
	})

	t.Run("toolkit update refreshes a selected vault", func(t *testing.T) {
		environment := isolatedEnvironment(t)
		vault := filepath.Join(environment.root, "vault")
		mustSucceed(t, runBinary(binary, environment, "", "", "init", "--defaults", vault))
		mustWrite(t, filepath.Join(vault, "AGENTS.md"), "stale toolkit\n")
		mustSucceed(t, runBinary(binary, environment, "", "", "doctor", vault))
		if got := mustRead(t, filepath.Join(vault, "AGENTS.md")); got != "stale toolkit\n" {
			t.Fatal("doctor unexpectedly refreshed toolkit")
		}

		result := runBinary(binary, environment, "", "", "toolkit", "update", vault)
		mustSucceed(t, result)
		if result.stdout != "toolkit updated\n" {
			t.Fatalf("toolkit update stdout = %q", result.stdout)
		}
		agents := mustRead(t, filepath.Join(vault, "AGENTS.md"))
		if !strings.Contains(agents, "# Corum Course Sync") || strings.Contains(agents, "stale toolkit") {
			t.Fatalf("AGENTS.md was not refreshed: %q", agents)
		}
		if target, err := os.Readlink(filepath.Join(vault, "CLAUDE.md")); err != nil || target != "AGENTS.md" {
			t.Fatalf("CLAUDE.md link = %q, %v", target, err)
		}
	})

}

type testEnvironment struct {
	root   string
	config string
	values []string
}

func isolatedEnvironment(t *testing.T) testEnvironment {
	t.Helper()
	root := t.TempDir()
	config := filepath.Join(root, "config")
	cache := filepath.Join(root, "cache")
	home := filepath.Join(root, "home")
	values := filteredEnvironment("HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "CORUM_CANVAS_TOKEN")
	values = append(values,
		"HOME="+home,
		"XDG_CONFIG_HOME="+config,
		"XDG_CACHE_HOME="+cache,
	)
	return testEnvironment{root: root, config: config, values: values}
}

func filteredEnvironment(keys ...string) []string {
	blocked := make(map[string]bool, len(keys))
	for _, key := range keys {
		blocked[key] = true
	}
	result := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !blocked[name] {
			result = append(result, value)
		}
	}
	return result
}

func buildBinary(t *testing.T) string {
	t.Helper()
	return buildBinaryVersion(t, integrationVersion)
}

func buildBinaryVersion(t *testing.T, version string) string {
	t.Helper()
	repository := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "corum")
	command := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X github.com/algebananazzzzz/Corum/internal/buildinfo.Version="+version, "-o", binary, "./cmd/corum")
	command.Dir = repository
	command.Env = append(filteredEnvironment("CGO_ENABLED"), "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build installed binary: %v\n%s", err, output)
	}
	return binary
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate integration test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
}

func archiveBinary(t *testing.T, binary string) []byte {
	t.Helper()
	contents, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "corum", Mode: 0o755, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func installerServer(t *testing.T, archive []byte, badChecksum bool) *httptest.Server {
	t.Helper()
	asset := fmt.Sprintf("corum_2.0.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	if badChecksum {
		digest = fmt.Sprintf("%x", sha256.Sum256([]byte("not the archive")))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/algebananazzzzz/Corum/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tag_name":"v2.0.0"}`)
	})
	mux.HandleFunc("/download/v2.0.0/"+asset, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/download/v2.0.0/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", digest, asset)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func runInstaller(t *testing.T, serverURL, installDirectory string) commandResult {
	t.Helper()
	realCurl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl is required for installer integration")
	}
	toolDirectory := t.TempDir()
	curlGuard := filepath.Join(toolDirectory, "curl")
	mustWrite(t, curlGuard, "#!/bin/sh\ncase \"$*\" in\n  *\"$CORUM_ALLOWED_TEST_URL\"*) exec \"$CORUM_REAL_CURL\" \"$@\" ;;\n  *) echo \"blocked non-test URL\" >&2; exit 97 ;;\nesac\n")
	if err := os.Chmod(curlGuard, 0o755); err != nil {
		t.Fatal(err)
	}
	environment := filteredEnvironment("HOME", "CORUM_INSTALL_DIR", "PATH", "_CORUM_TEST_API_BASE", "_CORUM_TEST_RELEASE_BASE", "CORUM_ALLOWED_TEST_URL", "CORUM_REAL_CURL")
	environment = append(environment,
		"HOME="+filepath.Join(t.TempDir(), "home"),
		"CORUM_INSTALL_DIR="+installDirectory,
		"PATH="+toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"),
		"_CORUM_TEST_API_BASE="+serverURL,
		"_CORUM_TEST_RELEASE_BASE="+serverURL+"/download",
		"CORUM_ALLOWED_TEST_URL="+serverURL,
		"CORUM_REAL_CURL="+realCurl,
	)
	command := exec.Command("sh", filepath.Join(repositoryRoot(t), "install.sh"))
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func runBinary(binary string, environment testEnvironment, directory, input string, arguments ...string) commandResult {
	command := exec.Command(binary, arguments...)
	command.Env = environment.values
	command.Dir = directory
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func mustSucceed(t *testing.T, result commandResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("command failed: %v\nstdout: %s\nstderr: %s", result.err, result.stdout, result.stderr)
	}
}

func mustExit(t *testing.T, result commandResult, want int) {
	t.Helper()
	var exitError *exec.ExitError
	if !errors.As(result.err, &exitError) || exitError.ExitCode() != want {
		t.Fatalf("exit = %v, want %d\nstdout: %s\nstderr: %s", result.err, want, result.stdout, result.stderr)
	}
}

func assertNoGlobalCorumConfig(t *testing.T, configDirectory string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(configDirectory, "corum")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("global Corum configuration was created: %v", err)
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	entries := []string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			entries = append(entries, fmt.Sprintf("dir %s", filepath.ToSlash(relative)))
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, fmt.Sprintf("file %s %q", filepath.ToSlash(relative), contents))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}
