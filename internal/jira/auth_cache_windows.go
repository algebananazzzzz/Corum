//go:build windows

package jira

import "os"

func fileUID(os.FileInfo) (int, bool) { return 0, false }
