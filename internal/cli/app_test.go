package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
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

func TestRunRejectsRenamedAuthCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"auth", "--help"}, nil, &out, &errOut); code != 2 {
		t.Fatalf("auth command code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "corum configure") {
		t.Fatalf("usage does not direct users to configure: %q", errOut.String())
	}
}

func TestRunInitDefaultsAndDoctor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"init", "--defaults", root}, nil, &out, &errOut); code != 0 {
		t.Fatalf("init code = %d, stderr = %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".config", "corum", "corum.yaml")); err != nil {
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

func TestRunDoctorOptionalPathDefaultsToWorkingDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	if code := Run(context.Background(), []string{"init", "--defaults", root}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("init code = %d", code)
	}
	t.Chdir(root)
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"doctor"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("doctor code = %d, stderr = %s", code, errOut.String())
	}
	if out.String() != "doctor: 0 courses\n" {
		t.Fatalf("doctor output = %q", out.String())
	}
	if code := Run(context.Background(), []string{"doctor", "/nonexistent/corum-vault"}, nil, &out, &errOut); code != 1 {
		t.Fatalf("invalid doctor path code = %d", code)
	}
}

func TestRunConfigureJiraRejectsInvalidVaultBeforeTTY(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"configure", "jira", "/nonexistent/corum-vault"}, nil, &out, &errOut); code != 1 {
		t.Fatalf("configure jira code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "vault validation failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunConfigureRequiresTerminalWithoutVault(t *testing.T) {
	// A valid vault path is required; a non-terminal stdin must be refused
	// before any prompt with a deterministic message.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	if code := Run(context.Background(), []string{"init", "--defaults", root}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("init code = %d", code)
	}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"configure", root}, nil, &out, &errOut); code != 2 {
		t.Fatalf("configure code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "terminal") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunInteractiveInitExplainsNonTTYFallback(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"init", "vault"}, &bytes.Buffer{}, &out, &errOut); code != 2 {
		t.Fatalf("Run code = %d, stderr = %s", code, errOut.String())
	}
	if errOut.String() != "interactive init requires a terminal; use corum init --defaults PATH\n" {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunSyncJSONPreservesEarlierCourseChanges(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CORUM_CANVAS_TOKEN", "test-token")
	root := filepath.Join(t.TempDir(), "vault")
	if code := Run(context.Background(), []string{"init", "--defaults", root}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("init code = %d", code)
	}
	writeCourse(t, root, "CS3103", "code: CS3103\ncanvas:\n  id: 1\n  sources:\n    - announcements\n")
	writeCourse(t, root, "CS3104", "code: CS3104\ncanvas:\n  id: 2\n  sources:\n    - announcements\n")
	t.Chdir(root)
	oldSync := syncCanvas
	t.Cleanup(func() { syncCanvas = oldSync })
	syncCanvas = func(_ context.Context, root string, workspace config.Workspace, course config.Course, _ canvas.SourceClient, _ bool) (canvas.StageResult, error) {
		result := canvas.StageResult{Course: course.Code, Changes: []canvas.Change{}, Failures: []canvas.Failure{}, Sources: []canvas.SourceResult{{Source: "announcements", Status: "up_to_date", Changes: []string{}, Failures: []string{}}}}
		if course.Code == "CS3104" {
			return result, errors.New("injected course failure")
		}
		result.Status = "up_to_date"
		return result, nil
	}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"sync", "--all", "--json"}, nil, &out, &errOut); code != 1 {
		t.Fatalf("sync code = %d, stderr = %q", code, errOut.String())
	}
	var manifests []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
	if err := decoder.Decode(&manifests); err != nil {
		t.Fatalf("manifests = %q, err = %v", out.String(), err)
	}
	if len(manifests) != 2 {
		t.Fatalf("manifest count = %d: %v", len(manifests), manifests)
	}
	first := manifests[0]
	if first["course"] != "CS3103" || first["status"] != "up_to_date" || first["changes"] == nil || first["failures"] == nil || first["run_id"] != nil || first["wiki"] != nil {
		t.Fatalf("first manifest = %#v", first)
	}
	if second := manifests[1]; second["course"] != "CS3104" {
		t.Fatalf("second manifest = %#v", second)
	}
	if !strings.Contains(errOut.String(), "injected course failure") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func writeCourse(t *testing.T, root, code, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "courses", code), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "courses", code, "course.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
