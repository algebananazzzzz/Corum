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
func (p *scriptedPrompts) MultiSelect(string, []Choice) ([]int, error) {
	answer, err := p.next()
	if err != nil {
		return nil, err
	}
	return answer.([]int), nil
}

func TestCourseChoicesAreReadableAndPreselectTrackedCourses(t *testing.T) {
	courses := []canvas.CourseInfo{
		{ID: "7", CourseCode: "CS3103", Name: "Computer Networks Practice", Current: true},
		{ID: "8", Name: "Student Essentials", Current: true},
	}

	choices := courseChoices(courses, map[string]bool{"7": true})
	if got, want := choices[0].Label, "CS3103  Computer Networks Practice  · ID 7"; got != want {
		t.Fatalf("first label = %q, want %q", got, want)
	}
	if !choices[0].Selected {
		t.Fatal("tracked course was not preselected")
	}
	if choices[1].Selected || !strings.Contains(choices[1].Label, "Course") || !strings.Contains(choices[1].Label, "Student Essentials") {
		t.Fatalf("second choice = %+v", choices[1])
	}
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
	var loadingMessages []string
	deps := JiraAuthDependencies{
		Prompts: prompts,
		OpenJira: func(context.Context) (JiraSession, error) {
			return fakeJira{resources: []Resource{{CloudID: "cloud", Name: "Site"}}, projects: map[string][]Project{"cloud": []Project{{Key: "TODO", Name: "Todo"}}}}, nil
		},
		SnapshotAuth: func() (func() error, error) { return func() error { return nil }, nil },
		Write:        vault.WriteWorkspace,
		Loading: func(ctx context.Context, message string, task func(context.Context) error) error {
			loadingMessages = append(loadingMessages, message)
			return task(ctx)
		},
	}
	if err := RunJiraAuth(context.Background(), root, deps); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(loadingMessages, ","), "Complete Jira authentication in your browser…,Loading Jira sites…,Loading Jira projects…"; got != want {
		t.Fatalf("loading messages = %q, want %q", got, want)
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

func TestCanvasAuthSavesTokenAndTracksSelectedCourses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	var saved string
	var loadedToken string
	var loadingMessage string
	prompts := &scriptedPrompts{answers: []any{"tok-123", []int{0}}}
	deps := CanvasAuthDependencies{
		Prompts: prompts,
		Root:    root,
		Load: func(_ config.Workspace, token string) (*canvas.Client, error) {
			loadedToken = token
			return &canvas.Client{}, nil
		},
		Save: func(token string) error {
			saved = token
			return canvas.SaveCredential(root, token)
		},
		Clear: func() error { _, err := canvas.ClearCredential(root); return err },
		Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
			return []canvas.CourseInfo{{ID: "7", CourseCode: "CS3103", Name: "Algorithms", Current: true}}, nil
		},
		Loading: func(ctx context.Context, message string, task func(context.Context) error) error {
			loadingMessage = message
			return task(ctx)
		},
	}
	if err := RunCanvasAuth(context.Background(), deps); err != nil {
		t.Fatal(err)
	}
	if saved != "tok-123" {
		t.Fatalf("saved token = %q", saved)
	}
	if loadedToken != "tok-123" {
		t.Fatalf("client token = %q", loadedToken)
	}
	if loadingMessage != "Loading Canvas courses…" {
		t.Fatalf("loading message = %q", loadingMessage)
	}
	if token, err := canvas.LoadCredential(root); err != nil || token != "tok-123" {
		t.Fatalf("LoadCredential = %q, %v", token, err)
	}
	course, err := config.LoadCourse(root, "CS3103")
	if err != nil || course.Canvas == nil || course.Canvas.ID != 7 || len(course.Canvas.Sources) != 6 {
		t.Fatalf("tracked course = %+v, %v", course, err)
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
	deps := CanvasAuthDependencies{
		Prompts: &scriptedPrompts{answers: []any{"bad-token"}},
		Root:    root,
		Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
		Save:    func(token string) error { return canvas.SaveCredential(root, token) },
		Clear:   func() error { _, err := canvas.ClearCredential(root); return err },
		Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
			return nil, errors.New("401 Unauthorized")
		},
	}
	if err := RunCanvasAuth(context.Background(), deps); err == nil || !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Fatalf("RunCanvasAuth() = %v, want the token rejection", err)
	}
}

func TestCanvasAuthCancellationDoesNotPersistNewToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	saved := false
	deps := CanvasAuthDependencies{
		Prompts: &scriptedPrompts{answers: []any{"new-token", ErrCancelled}},
		Root:    root,
		Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
		Save:    func(string) error { saved = true; return nil },
		Clear:   func() error { return nil },
		Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
			return []canvas.CourseInfo{{ID: "7", CourseCode: "CS3103", Name: "Algorithms", Current: true}}, nil
		},
	}
	if err := RunCanvasAuth(context.Background(), deps); !errors.Is(err, ErrCancelled) {
		t.Fatalf("RunCanvasAuth() = %v", err)
	}
	if saved {
		t.Fatal("cancelled auth persisted the new token")
	}
}

func TestCanvasAuthClearsNewTokenWhenCourseUpdateFails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	saved, cleared := false, false
	deps := CanvasAuthDependencies{
		Prompts: &scriptedPrompts{answers: []any{"new-token", []int{0}}},
		Root:    root,
		Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
		Save:    func(string) error { saved = true; return nil },
		Clear:   func() error { cleared = true; return nil },
		Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
			return []canvas.CourseInfo{{ID: "999999999999999999999999", CourseCode: "CS3103", Name: "Algorithms", Current: true}}, nil
		},
	}
	if err := RunCanvasAuth(context.Background(), deps); err == nil {
		t.Fatal("RunCanvasAuth() succeeded")
	}
	if !saved || !cleared {
		t.Fatalf("saved = %v, cleared = %v", saved, cleared)
	}
}

func TestCanvasAuthLeavesTrackingUntouchedWhenNoCurrentCoursesExist(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	writePath := filepath.Join(root, "courses", "OLD", "course.yaml")
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		t.Fatal(err)
	}
	before := []byte("version: 2\ncode: OLD\ncanvas:\n  id: 7\n  sources: [assignments]\n")
	if err := os.WriteFile(writePath, before, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := canvas.SaveCredential(root, "token"); err != nil {
		t.Fatal(err)
	}
	deps := CanvasAuthDependencies{
		Prompts: &scriptedPrompts{answers: []any{"token"}},
		Root:    root,
		Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
		Save:    func(string) error { return errors.New("unexpected credential save") },
		Clear:   func() error { return errors.New("unexpected credential clear") },
		Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
			return []canvas.CourseInfo{{ID: "7", CourseCode: "OLD", Name: "Old Course", Current: false}}, nil
		},
	}
	if err := RunCanvasAuth(context.Background(), deps); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(writePath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("course changed: %q, %v", after, err)
	}
}

func TestRunAuthSkipsDisabledJira(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	deps := AuthDependencies{
		Prompts: &scriptedPrompts{answers: []any{false}},
		Canvas: CanvasAuthDependencies{
			Prompts: &scriptedPrompts{answers: []any{"tok", []int{0}}},
			Root:    root,
			Load:    func(config.Workspace, string) (*canvas.Client, error) { return &canvas.Client{}, nil },
			Save:    func(token string) error { return canvas.SaveCredential(root, token) },
			Clear:   func() error { _, err := canvas.ClearCredential(root); return err },
			Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
				return []canvas.CourseInfo{{ID: "1", CourseCode: "CS101", Name: "Intro", Current: true}}, nil
			},
		},
	}
	if err := RunAuth(context.Background(), root, deps); err != nil {
		t.Fatal(err)
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
