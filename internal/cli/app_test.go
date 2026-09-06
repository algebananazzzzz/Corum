package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/buildinfo"
	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/update"
	"github.com/algebananazzzzz/Corum/internal/vault"
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

func TestRunUpdateRejectsDevelopmentBuild(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"update"}, nil, &out, &errOut); code != 1 {
		t.Fatalf("Run code = %d, stdout = %q, stderr = %q", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "development build cannot self-update") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunProcessReexecSyncsBeforeDispatchAndReportsPartialErrors(t *testing.T) {
	oldVersion := buildinfo.Version
	oldMaybe := maybeUpdate
	oldSync := syncVaultToolkits
	t.Cleanup(func() {
		buildinfo.Version = oldVersion
		maybeUpdate = oldMaybe
		syncVaultToolkits = oldSync
	})
	buildinfo.Version = "v2.0.0"
	t.Setenv(update.ReexecEnv, "1")
	maybeUpdate = func(context.Context, update.Options) (update.Outcome, error) {
		t.Fatal("re-executed process attempted another update")
		return update.Outcome{}, nil
	}
	var out, errOut bytes.Buffer
	syncCalls := 0
	syncVaultToolkits = func(fs.FS, string) []vault.SyncResult {
		syncCalls++
		if out.Len() != 0 {
			t.Fatal("command dispatched before toolkit sync")
		}
		return []vault.SyncResult{{Root: "/good"}, {Root: "/bad", Err: errors.New("injected toolkit failure")}}
	}
	if code := RunProcess(context.Background(), []string{"corum", "version"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("RunProcess code = %d, stderr = %q", code, errOut.String())
	}
	if syncCalls != 1 || out.String() != "v2.0.0\n" {
		t.Fatalf("sync calls = %d, stdout = %q", syncCalls, out.String())
	}
	if !strings.Contains(errOut.String(), "/bad") || !strings.Contains(errOut.String(), "injected toolkit failure") {
		t.Fatalf("stderr = %q", errOut.String())
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
	registered, err := vault.Registered()
	if err != nil || len(registered) != 1 {
		t.Fatalf("registered = %#v, err = %v", registered, err)
	}
	if code := Run(context.Background(), []string{"doctor", "/nonexistent/corum-vault"}, nil, &out, &errOut); code != 1 {
		t.Fatalf("invalid doctor path code = %d", code)
	}
}

func TestRunJiraLoginRejectsInvalidVaultBeforeTTY(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"jira", "login", "/nonexistent/corum-vault"}, nil, &out, &errOut); code != 1 {
		t.Fatalf("jira login code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "vault validation failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunJiraStatusRejectsInvalidVault(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"jira", "status", "/nonexistent/corum-vault"}, nil, &out, &errOut); code != 1 {
		t.Fatalf("jira status code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "vault validation failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunUpdateContinuationReportsWithoutSecondCheck(t *testing.T) {
	oldMaybe := maybeUpdate
	t.Cleanup(func() { maybeUpdate = oldMaybe })
	maybeUpdate = func(context.Context, update.Options) (update.Outcome, error) {
		t.Fatal("continuation invoked an update check")
		return update.Outcome{}, nil
	}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{update.ContinuationArg}, nil, &out, &errOut); code != 0 {
		t.Fatalf("continuation code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "corum updated to") {
		t.Fatalf("stdout = %q", out.String())
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

func TestRunSyncJSONEmitsPerCourseManifestsPreservingEarlierCourses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CORUM_CANVAS_TOKEN", "test-token")
	root := filepath.Join(t.TempDir(), "vault")
	if code := Run(context.Background(), []string{"init", "--defaults", root}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("init code = %d", code)
	}
	writeCourse(t, root, "CS3103", "version: 2\ncode: CS3103\ncanvas:\n  id: 1\n  sources:\n    - announcements\n")
	writeCourse(t, root, "CS3104", "version: 2\ncode: CS3104\ncanvas:\n  id: 2\n  sources:\n    - announcements\n")
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
	if first["course"] != "CS3103" || first["run_id"] == "" || first["canvas"] == nil || first["jira"] == nil || first["wiki"] == nil || first["effective_features"] == nil {
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

func TestRunJiraApplyDryRunEchoesPlanWithoutOAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "courses", "CS3103"), 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := "version: 2\nworkspace:\n  timezone: Asia/Singapore\n  term: AY2026/27 Semester 1\njira:\n  cloud_id: cloud-1\n  project: STUDY\n  transitions:\n    this_week: \"2\"\ncalendar:\n  timetable: Timetable.md\n  term: Term_Calendar.md\n"
	course := "version: 2\ncode: CS3103\njira:\n  epic: STUDY-1\n"
	if err := os.WriteFile(filepath.Join(root, "corum.yaml"), []byte(workspace), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "courses", "CS3103", "course.yaml"), []byte(course), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	input := bytes.NewBufferString(`{"version":2,"course":"CS3103","epic":"STUDY-1","actions":[]}`)
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"jira", "apply", "CS3103", "--dry-run"}, input, &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	var echoed map[string]any
	if err := json.Unmarshal(out.Bytes(), &echoed); err != nil {
		t.Fatal(err)
	}
	if echoed["version"] != float64(2) || len(echoed["actions"].([]any)) != 0 {
		t.Fatalf("output=%v", echoed)
	}
	if _, err := os.Stat(filepath.Join(root, "courses", "CS3103", "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run state=%v", err)
	}
}
