package cli

import (
	"bytes"
	"context"
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
