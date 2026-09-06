package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunVersionPrintsBuildVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"version"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("Run code = %d, stderr = %s", code, errOut.String())
	}
	if out.String() != "dev\n" {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"unknown"}, nil, &out, &errOut); code != 2 {
		t.Fatalf("Run code = %d", code)
	}
}

func TestRunInitDefaultsAndDoctor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"init", "--defaults", root}, nil, &out, &errOut); code != 0 {
		t.Fatalf("init code = %d, stderr = %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, "corum.yaml")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run(context.Background(), []string{"doctor", root}, nil, &out, &errOut); code != 0 {
		t.Fatalf("doctor code = %d, stderr = %s", code, errOut.String())
	}
	if out.String() != "doctor: 0 courses\n" {
		t.Fatalf("doctor output = %q", out.String())
	}
}
