//go:build !windows

package jira

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
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
