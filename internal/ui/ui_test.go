package ui

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

type scriptedPrompts struct {
	answers []any
	index   int
}

func (p *scriptedPrompts) next() (any, error) {
	if p.index >= len(p.answers) {
		return nil, errors.New("missing scripted answer")
	}
	answer := p.answers[p.index]
	p.index++
	if err, ok := answer.(error); ok {
		return nil, err
	}
	return answer, nil
}

func (p *scriptedPrompts) Input(string, string) (string, error) {
	answer, err := p.next()
	if err != nil {
		return "", err
	}
	return answer.(string), nil
}
func (p *scriptedPrompts) Password(string, string) (string, error) {
	answer, err := p.next()
	if err != nil {
		return "", err
	}
	return answer.(string), nil
}
func (p *scriptedPrompts) Confirm(string, bool) (bool, error) {
	answer, err := p.next()
	if err != nil {
		return false, err
	}
	return answer.(bool), nil
}
func (p *scriptedPrompts) Select(string, []Choice) (int, error) {
	answer, err := p.next()
	if err != nil {
		return 0, err
	}
	return answer.(int), nil
}

type fakeJira struct {
	resources []Resource
	projects  map[string][]Project
	err       error
}

func (j fakeJira) Resources(context.Context) ([]Resource, error) { return j.resources, j.err }
func (j fakeJira) Projects(_ context.Context, cloudID string) ([]Project, error) {
	return j.projects[cloudID], j.err
}
func (j fakeJira) Close() error { return nil }

func uiAssets() fs.FS {
	return fstest.MapFS{
		"agent-kit/AGENTS.base.md":          &fstest.MapFile{Data: []byte("agents")},
		"agent-kit/skills/example/SKILL.md": &fstest.MapFile{Data: []byte("skill")},
		"agent-kit/templates/template.md":   &fstest.MapFile{Data: []byte("template")},
	}
}

func testDependencies(prompts Prompter) InitDependencies {
	return InitDependencies{
		Prompts:        prompts,
		Assets:         uiAssets(),
		ToolkitVersion: "test",
		Initialize:     initializeVault,
	}
}

func TestLocalInitializationIsDeterministic(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	prompts := &scriptedPrompts{answers: []any{root, "Asia/Singapore", "Term", "https://canvas.example.edu", true, true}}
	if err := RunInit(context.Background(), root, testDependencies(prompts)); err != nil {
		t.Fatal(err)
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Jira != nil || workspace.Wiki == nil {
		t.Fatalf("workspace = %+v", workspace)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("toolkit not installed: %v", err)
	}
}

func TestInitNeverOpensJira(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	prompts := &scriptedPrompts{answers: []any{root, "Asia/Singapore", "Term", "https://canvas.example.edu", true, true}}
	if err := RunInit(context.Background(), root, testDependencies(prompts)); err != nil {
		t.Fatal(err)
	}
	// No Jira block and no credential directory should exist after a local init.
	workspace, err := config.LoadWorkspace(root)
	if err != nil || workspace.Jira != nil {
		t.Fatalf("workspace = %+v, %v", workspace, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".config", "corum", "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init wrote a Jira credential: %v", err)
	}
}

func TestInvalidInitAnswersWriteNothing(t *testing.T) {
	for name, answers := range map[string][]any{
		"timezone": {"Asia/NotAZone", "Term", "https://canvas.example.edu", true},
		"origin":   {"Asia/Singapore", "Term", "http://canvas.example.edu", true},
	} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "vault")
			prompts := &scriptedPrompts{answers: append([]any{root}, answers...)}
			if err := RunInit(context.Background(), root, testDependencies(prompts)); err == nil {
				t.Fatal("RunInit() succeeded")
			}
			if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid init created root: %v", err)
			}
		})
	}
}

func TestCancelledInitWritesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	prompts := &scriptedPrompts{answers: []any{root, "Asia/Singapore", "Term", "https://canvas.example.edu", true, false}}
	err := RunInit(context.Background(), root, testDependencies(prompts))
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("RunInit error = %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled root exists: %v", err)
	}
}

func TestJiraAuthWritesProjectSelection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	prompts := &scriptedPrompts{answers: []any{0, 0, true}}
	deps := JiraAuthDependencies{
		Prompts: prompts,
		OpenJira: func(context.Context) (JiraSession, error) {
			return fakeJira{resources: []Resource{{CloudID: "cloud", Name: "Site"}}, projects: map[string][]Project{"cloud": []Project{{Key: "TODO", Name: "Todo"}}}}, nil
		},
		SnapshotAuth: func() (func() error, error) { return func() error { return nil }, nil },
		Write:        vault.WriteWorkspace,
	}
	if err := RunJiraAuth(context.Background(), root, deps); err != nil {
		t.Fatal(err)
	}
	updated, err := config.LoadWorkspace(root)
	if err != nil || updated.Jira == nil || updated.Jira.Project != "TODO" || updated.Jira.CloudID != "cloud" {
		t.Fatalf("workspace = %+v, %v", updated, err)
	}
}

func TestJiraAuthCancellationRestoresPriorState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(config.WorkspacePath(root))
	if err != nil {
		t.Fatal(err)
	}
	restored := false
	writeCalled := false
	prompts := &scriptedPrompts{answers: []any{0, 0, false}}
	err = RunJiraAuth(context.Background(), root, JiraAuthDependencies{
		Prompts: prompts,
		OpenJira: func(context.Context) (JiraSession, error) {
			return fakeJira{resources: []Resource{{CloudID: "cloud", Name: "Site"}}, projects: map[string][]Project{"cloud": []Project{{Key: "TODO", Name: "Todo"}}}}, nil
		},
		SnapshotAuth: func() (func() error, error) { return func() error { restored = true; return nil }, nil },
		Write:        func(string, config.Workspace) error { writeCalled = true; return nil },
	})
	if !errors.Is(err, ErrCancelled) || !restored || writeCalled {
		t.Fatalf("RunJiraAuth() = %v, restored = %v, write = %v", err, restored, writeCalled)
	}
	after, err := os.ReadFile(config.WorkspacePath(root))
	if err != nil || string(after) != string(before) {
		t.Fatalf("corum.yaml changed: %q, %v", after, err)
	}
}

func TestJiraAuthJoinsRollbackError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	restored := false
	restoreErr := errors.New("injected restore failure")
	err := RunJiraAuth(context.Background(), root, JiraAuthDependencies{
		Prompts: &scriptedPrompts{},
		OpenJira: func(context.Context) (JiraSession, error) {
			return nil, errors.New("injected open failure")
		},
		SnapshotAuth: func() (func() error, error) {
			return func() error {
				restored = true
				return restoreErr
			}, nil
		},
		Write: func(string, config.Workspace) error { return nil },
	})
	if err == nil {
		t.Fatal("RunJiraAuth() succeeded")
	}
	if !restored {
		t.Fatal("restore was not attempted")
	}
	if !strings.Contains(err.Error(), "injected open failure") || !strings.Contains(err.Error(), "injected restore failure") {
		t.Fatalf("error = %v, want both open and restore errors", err)
	}
}

func TestCanvasAuthSavesTokenAndListsCourses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	var saved string
	prompts := &scriptedPrompts{answers: []any{"tok-123"}}
	var out strings.Builder
	deps := CanvasAuthDependencies{
		Prompts: prompts,
		Output:  &out,
		Root:    root,
		Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
		Save: func(token string) error {
			saved = token
			return canvas.SaveCredential(root, token)
		},
		Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
			return []canvas.CourseInfo{{ID: "7", CourseCode: "CS3103", Name: "Algorithms"}}, nil
		},
	}
	if err := RunCanvasAuth(context.Background(), deps); err != nil {
		t.Fatal(err)
	}
	if saved != "tok-123" {
		t.Fatalf("saved token = %q", saved)
	}
	if token, err := canvas.LoadCredential(root); err != nil || token != "tok-123" {
		t.Fatalf("LoadCredential = %q, %v", token, err)
	}
	if !strings.Contains(out.String(), "CS3103") || !strings.Contains(out.String(), "(id 7)") {
		t.Fatalf("course listing = %q", out.String())
	}
	// A credential file must exist project-local with a gitignore guard.
	if _, err := os.Stat(filepath.Join(root, ".config", "corum", "canvas.json")); err != nil {
		t.Fatalf("canvas credential missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".config", "corum", ".gitignore")); err != nil {
		t.Fatalf("gitignore guard missing: %v", err)
	}
}

func TestCanvasAuthRefusesInvalidToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	deps := CanvasAuthDependencies{
		Prompts: &scriptedPrompts{answers: []any{"bad-token"}},
		Output:  &out,
		Root:    root,
		Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
		Save:    func(token string) error { return canvas.SaveCredential(root, token) },
		Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
			return nil, errors.New("401 Unauthorized")
		},
	}
	if err := RunCanvasAuth(context.Background(), deps); err == nil || !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Fatalf("RunCanvasAuth() = %v, want the token rejection", err)
	}
}

func TestRunAuthSkipsDisabledJira(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	deps := AuthDependencies{
		Prompts: &scriptedPrompts{answers: []any{false}},
		Output:  &out,
		Canvas: CanvasAuthDependencies{
			Prompts: &scriptedPrompts{answers: []any{"tok"}},
			Output:  &out,
			Root:    root,
			Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
			Save:    func(token string) error { return canvas.SaveCredential(root, token) },
			Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
				return []canvas.CourseInfo{{ID: "1", CourseCode: "CS101", Name: "Intro"}}, nil
			},
		},
		HasJira: func(ws config.Workspace) bool { return ws.Jira != nil },
	}
	if err := RunAuth(context.Background(), root, deps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Jira skipped") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSummaryHasNoSecrets(t *testing.T) {
	value := initSummary(config.Workspace{Wiki: &config.WikiWorkspace{}})
	for _, forbidden := range []string{"cloud", "token", "secret", "password"} {
		if strings.Contains(strings.ToLower(value), forbidden) {
			t.Fatalf("summary exposes %q: %s", forbidden, value)
		}
	}
}

func TestNonTTYGuidanceIsDeterministic(t *testing.T) {
	if got := NonTTYGuidance("init"); got != "interactive init requires a terminal; use corum init --defaults PATH" {
		t.Fatalf("NonTTYGuidance() = %q", got)
	}
}
