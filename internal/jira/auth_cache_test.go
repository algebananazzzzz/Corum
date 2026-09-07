package jira

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type foreignOwnedDirectoryInfo struct{}

func (foreignOwnedDirectoryInfo) Name() string       { return "corum" }
func (foreignOwnedDirectoryInfo) Size() int64        { return 0 }
func (foreignOwnedDirectoryInfo) Mode() os.FileMode  { return os.ModeDir | 0o700 }
func (foreignOwnedDirectoryInfo) ModTime() time.Time { return time.Time{} }
func (foreignOwnedDirectoryInfo) IsDir() bool        { return true }
func (foreignOwnedDirectoryInfo) Sys() any {
	return &syscall.Stat_t{Uid: uint32(os.Getuid() + 1)}
}

func TestAuthCacheRejectsForeignOwnedDirectory(t *testing.T) {
	err := checkPrivateDirectory(foreignOwnedDirectoryInfo{})
	if err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("checkPrivateDirectory error = %v, want ownership error", err)
	}
}

func TestAuthCacheMissingReturnsNotExist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "corum")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := loadAuthCache(filepath.Join(dir, "auth.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("loadAuthCache missing error = %v, want not exist", err)
	}
}

func TestAuthCachePathRequiresProjectRoot(t *testing.T) {
	if _, err := AuthCachePathFor(""); err == nil {
		t.Fatal("AuthCachePathFor accepted an empty project root")
	}
}

func TestAuthCacheRoundTripIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "auth.json")
	want := authRecord{Version: 1, ClientID: "client", Issuer: "https://issuer.example", AuthURL: "https://issuer.example/auth", TokenURL: "https://issuer.example/token", RedirectURL: "http://127.0.0.1:1234/callback", Scopes: []string{"read"}, Token: &oauth2.Token{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", Expiry: time.Now().Add(time.Hour)}}
	if err := saveAuthCache(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadAuthCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientID != want.ClientID || got.Token.AccessToken != want.Token.AccessToken || got.Token.RefreshToken != want.Token.RefreshToken {
		t.Fatalf("round trip = %+v", got)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, err = %v", dir.Mode().Perm(), err)
	}
	file, err := os.Stat(path)
	if err != nil || file.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, err = %v", file.Mode().Perm(), err)
	}
}

func TestAuthCacheRejectsPublicPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadAuthCache(path)
	if err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("loadAuthCache error = %v, want permissions error", err)
	}
}

func TestAuthCacheRejectsPublicDirectoryPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "corum")
	path := filepath.Join(dir, "auth.json")
	if err := saveAuthCache(path, authRecord{Version: 1, ClientID: "client", Token: &oauth2.Token{AccessToken: "synthetic"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := loadAuthCache(path)
	if err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("loadAuthCache error = %v, want directory permissions error", err)
	}
}

func TestAuthCacheCorruptionDoesNotExposeContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(`{"access_token":"secret-sentinel"`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadAuthCache(path)
	if err == nil || strings.Contains(err.Error(), "secret-sentinel") {
		t.Fatalf("loadAuthCache error = %v, want redacted corruption", err)
	}
}

func TestAuthCacheAtomicReplacementLeavesCompleteRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := saveAuthCache(path, authRecord{Version: 1, ClientID: "first", Token: &oauth2.Token{AccessToken: "one"}}); err != nil {
		t.Fatal(err)
	}
	if err := saveAuthCache(path, authRecord{Version: 1, ClientID: "second", Token: &oauth2.Token{AccessToken: "two"}}); err != nil {
		t.Fatal(err)
	}
	got, err := loadAuthCache(path)
	if err != nil || got.ClientID != "second" || got.Token.AccessToken != "two" {
		t.Fatalf("record = %+v, err = %v", got, err)
	}
}

func TestAuthCacheClearIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := saveAuthCache(path, authRecord{Version: 1, ClientID: "client", Token: &oauth2.Token{AccessToken: "synthetic"}}); err != nil {
		t.Fatal(err)
	}
	if removed, err := clearAuth(path); err != nil || !removed {
		t.Fatalf("first clear = %v, %v", removed, err)
	}
	if removed, err := clearAuth(path); err != nil || removed {
		t.Fatalf("second clear = %v, %v", removed, err)
	}
}
