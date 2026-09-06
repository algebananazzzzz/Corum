package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewerUsesSemanticVersionOrder(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v1.10.0", "v1.9.0", true},
		{"2.0.0", "v1.99.99", true},
		{"v1.4.3", "1.4.2", true},
		{"v1.4.2", "v1.4.2", false},
		{"v1.4.1", "v1.4.2", false},
		{"garbage", "v1.0.0", false},
		{"v1.0.0", "dev", false},
	}
	for _, test := range tests {
		if got := Newer(test.latest, test.current); got != test.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", test.latest, test.current, got, test.want)
		}
	}
}

func TestAssetNameForReleaseTargets(t *testing.T) {
	tests := []struct{ tag, goos, goarch, want string }{
		{"v1.4.2", "linux", "amd64", "corum_1.4.2_linux_amd64.tar.gz"},
		{"1.4.2", "linux", "arm64", "corum_1.4.2_linux_arm64.tar.gz"},
		{"v2.0.0", "darwin", "amd64", "corum_2.0.0_darwin_amd64.tar.gz"},
		{"v2.0.0", "darwin", "arm64", "corum_2.0.0_darwin_arm64.tar.gz"},
	}
	for _, test := range tests {
		if got := AssetName(test.tag, test.goos, test.goarch); got != test.want {
			t.Errorf("AssetName(%q, %q, %q) = %q, want %q", test.tag, test.goos, test.goarch, got, test.want)
		}
	}
}

func TestLatestDecodesCorumRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/algebananazzzzz/Corum/releases/latest" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		io.WriteString(w, `{"tag_name":"v1.2.3","assets":[{"name":"checksums.txt","browser_download_url":"https://example.test/checksums"}]}`)
	}))
	defer server.Close()
	release, err := Latest(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if release.Tag != "v1.2.3" || release.Assets["checksums.txt"] != "https://example.test/checksums" {
		t.Fatalf("release = %#v", release)
	}
}

func TestVerifyExactArchiveChecksum(t *testing.T) {
	data := []byte("archive")
	name := "corum_1.2.3_linux_amd64.tar.gz"
	checksums := fmt.Sprintf("%x  other.tar.gz\n%x  %s\n", sha256.Sum256([]byte("other")), sha256.Sum256(data), name)
	if err := Verify(data, name, checksums); err != nil {
		t.Fatal(err)
	}
	if err := Verify(data, name, fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte("tampered")), name)); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("mismatch error = %v", err)
	}
	if err := Verify(data, name, "deadbeef  other.tar.gz\n"); err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Fatalf("missing-entry error = %v", err)
	}
}

func TestExtractReturnsOnlyCorumRegularFile(t *testing.T) {
	data := testArchive(t, map[string]string{"README.md": "docs", "release/corum": "binary"})
	got, err := Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("binary = %q", got)
	}
	if _, err := Extract(testArchive(t, map[string]string{"README.md": "docs"})); err == nil {
		t.Fatal("archive without corum accepted")
	}
	if _, err := Extract([]byte("not gzip")); err == nil {
		t.Fatal("invalid archive accepted")
	}
}

func TestDevelopmentAutomaticUpdateDoesNotCallNetwork(t *testing.T) {
	server := noNetworkServer(t)
	outcome, err := Maybe(context.Background(), Options{Version: "dev", APIBase: server.URL, CachePath: filepath.Join(t.TempDir(), "cache.json")})
	if err != nil || outcome.Checked {
		t.Fatalf("Maybe() = %#v, %v", outcome, err)
	}
}

func TestDisabledAutomaticUpdateDoesNotCallNetwork(t *testing.T) {
	server := noNetworkServer(t)
	_, err := Maybe(context.Background(), Options{Version: "1.0.0", APIBase: server.URL, Disable: true, CachePath: filepath.Join(t.TempDir(), "cache.json")})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnvironmentDisablesAutomaticUpdate(t *testing.T) {
	t.Setenv("DISABLE_AUTO_UPDATES", "1")
	server := noNetworkServer(t)
	_, err := Maybe(context.Background(), Options{Version: "1.0.0", APIBase: server.URL, CachePath: filepath.Join(t.TempDir(), "cache.json")})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticUpdateSkipsCacheYoungerThanDay(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "update-check.json")
	writeTestCache(t, path, cache{Version: cacheVersion, CheckedAt: now.Add(-23 * time.Hour), Latest: "v1.0.0"})
	server := noNetworkServer(t)
	outcome, err := Maybe(context.Background(), Options{Version: "1.0.0", APIBase: server.URL, CachePath: path, now: func() time.Time { return now }})
	if err != nil || outcome.Checked {
		t.Fatalf("Maybe() = %#v, %v", outcome, err)
	}
}

func TestAutomaticUpdateChecksAfterDay(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "update-check.json")
	writeTestCache(t, path, cache{Version: cacheVersion, CheckedAt: now.Add(-24 * time.Hour), Latest: "v1.0.0"})
	hits := 0
	server := releaseMetadataServer(t, "v1.0.0", &hits)
	outcome, err := Maybe(context.Background(), Options{Version: "1.0.0", APIBase: server.URL, CachePath: path, now: func() time.Time { return now }})
	if err != nil || !outcome.Checked || hits != 1 {
		t.Fatalf("Maybe() = %#v, %v; hits = %d", outcome, err, hits)
	}
	got, ok := readCache(path)
	if !ok || !got.CheckedAt.Equal(now) || got.Latest != "v1.0.0" {
		t.Fatalf("cache = %#v, ok = %v", got, ok)
	}
}

func TestFailedAutomaticAttemptIsCachedAndNonfatal(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "update-check.json")
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	options := Options{Version: "1.0.0", APIBase: server.URL, CachePath: path, now: func() time.Time { return now }}
	first, err := Maybe(context.Background(), options)
	if err != nil || first.Warning == "" {
		t.Fatalf("first Maybe() = %#v, %v", first, err)
	}
	second, err := Maybe(context.Background(), options)
	if err != nil || second.Checked || hits != 1 {
		t.Fatalf("second Maybe() = %#v, %v; hits = %d", second, err, hits)
	}
	got, ok := readCache(path)
	if !ok || !got.CheckedAt.Equal(now) {
		t.Fatalf("cache = %#v, ok = %v", got, ok)
	}
}

func TestExplicitUpdateFailureIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if _, err := Run(context.Background(), Options{Version: "v1.0.0", APIBase: server.URL, CachePath: filepath.Join(t.TempDir(), "cache.json")}); err == nil {
		t.Fatal("Run() accepted release failure")
	}
}

func TestExplicitDevelopmentUpdateFailsWithoutNetwork(t *testing.T) {
	server := noNetworkServer(t)
	if _, err := Run(context.Background(), Options{Version: "dev", APIBase: server.URL}); err == nil || !strings.Contains(err.Error(), "development build") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestUnsupportedPlatformFailsBeforeNetwork(t *testing.T) {
	server := noNetworkServer(t)
	if _, err := Run(context.Background(), Options{Version: "v1.0.0", APIBase: server.URL, GOOS: "plan9", GOARCH: "amd64"}); err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestChecksumFailureDoesNotReplaceOrReexec(t *testing.T) {
	archive := testArchive(t, map[string]string{"corum": "new binary"})
	options, replaced, reexecuted := updateOptions(t, archive, fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte("tampered")), AssetName("v2.0.0", runtime.GOOS, runtime.GOARCH)))
	if _, err := Run(context.Background(), options); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("Run() error = %v", err)
	}
	if *replaced || *reexecuted {
		t.Fatalf("replaced = %v, reexecuted = %v", *replaced, *reexecuted)
	}
}

func TestVerifiedUpdateReplacesThenReexecutesOriginalCommand(t *testing.T) {
	archive := testArchive(t, map[string]string{"corum": "new binary"})
	name := AssetName("v2.0.0", runtime.GOOS, runtime.GOARCH)
	options, replaced, reexecuted := updateOptions(t, archive, fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), name))
	outcome, err := Run(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Updated || !*replaced || !*reexecuted {
		t.Fatalf("Run() = %#v; replaced = %v, reexecuted = %v", outcome, *replaced, *reexecuted)
	}
}

func updateOptions(t *testing.T, archive []byte, checksums string) (Options, *bool, *bool) {
	t.Helper()
	name := AssetName("v2.0.0", runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/repos/algebananazzzzz/Corum/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":%q,"browser_download_url":%q},{"name":"checksums.txt","browser_download_url":%q}]}`, name, server.URL+"/archive", server.URL+"/checksums")
	})
	mux.HandleFunc("/archive", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc("/checksums", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, checksums) })
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	replaced := false
	reexecuted := false
	options := Options{
		Version:    "v1.0.0",
		APIBase:    server.URL,
		CachePath:  filepath.Join(t.TempDir(), "update-check.json"),
		Executable: "/tmp/corum",
		Args:       []string{"/tmp/corum", "doctor", "/vault"},
		Env:        []string{"KEEP=value", ReexecEnv + "=old"},
		apply: func(reader io.Reader) error {
			got, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			if string(got) != "new binary" {
				t.Fatalf("replacement = %q", got)
			}
			replaced = true
			return nil
		},
		reexec: func(executable string, args, env []string) error {
			if !replaced {
				t.Fatal("reexec happened before replacement")
			}
			if executable != "/tmp/corum" || strings.Join(args, " ") != "/tmp/corum doctor /vault" {
				t.Fatalf("reexec = %q %#v", executable, args)
			}
			if countEnv(env, ReexecEnv+"=1") != 1 || countEnv(env, ReexecEnv+"=old") != 0 || countEnv(env, "KEEP=value") != 1 {
				t.Fatalf("reexec env = %#v", env)
			}
			reexecuted = true
			return nil
		},
	}
	return options, &replaced, &reexecuted
}

func countEnv(env []string, want string) int {
	count := 0
	for _, item := range env {
		if item == want {
			count++
		}
	}
	return count
}

func noNetworkServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("automatic update contacted network")
	}))
	t.Cleanup(server.Close)
	return server
}

func releaseMetadataServer(t *testing.T, tag string, hits *int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[]}`, tag)
	}))
	t.Cleanup(server.Close)
	return server
}

func writeTestCache(t *testing.T, path string, value cache) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"version":%d,"checked_at":%q,"latest":%q}`, value.Version, value.CheckedAt.Format(time.RFC3339Nano), value.Latest)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
