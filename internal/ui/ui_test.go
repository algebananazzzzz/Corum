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

func uiAssets() fs.FS {
	return fstest.MapFS{
		"agent-kit/AGENTS.base.md":                          &fstest.MapFile{Data: []byte("agents\n" + vault.UserSectionMarker + "\n")},
		"agent-kit/skills/example/SKILL.md":                 &fstest.MapFile{Data: []byte("skill")},
		"agent-kit/templates/template.md":                   &fstest.MapFile{Data: []byte("template")},
		"agent-kit/assets/calendar/ay2026_27_semester_1.md": &fstest.MapFile{Data: []byte("semester one")},
		"agent-kit/assets/calendar/ay2026_27_semester_2.md": &fstest.MapFile{Data: []byte("semester two")},
	}
}

func TestCanvasAuthSavesTokenAndTracksSelectedCourses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}}
	if err := vault.Initialize(root, workspace, uiAssets()); err != nil {
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

func TestCanvasAuthAsksAgainUntilCanvasAcceptsToken(t *testing.T) {
	for _, tc := range []struct {
		name     string
		answers  []any
		courses  error
		wantErr  string
		wantSave string
	}{
		{"retry succeeds", []any{"bad-token", " ", "good-token", []int{0}}, nil, "", "good-token"},
		{"cancel after rejection", []any{"bad-token", ErrCancelled}, nil, ErrCancelled.Error(), ""},
		{"network failure stops", []any{"good-token"}, errors.New("dial tcp: no route to host"), "no route to host", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			root := filepath.Join(t.TempDir(), "vault")
			workspace := config.Workspace{Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}}
			if err := vault.Initialize(root, workspace, uiAssets()); err != nil {
				t.Fatal(err)
			}
			prompts := &scriptedPrompts{answers: tc.answers}
			tried := ""
			deps := CanvasAuthDependencies{
				Prompts: prompts,
				Root:    root,
				Load: func(_ config.Workspace, token string) (*canvas.Client, error) {
					tried = token
					return &canvas.Client{}, nil
				},
				Save:  func(token string) error { return canvas.SaveCredential(root, token) },
				Clear: func() error { _, err := canvas.ClearCredential(root); return err },
				Courses: func(context.Context, *canvas.Client) ([]canvas.CourseInfo, error) {
					if tc.courses != nil {
						return nil, tc.courses
					}
					if tried != "good-token" {
						return nil, &canvas.HTTPError{Status: 401, URL: "https://canvas.example.edu/api/v1/courses"}
					}
					return []canvas.CourseInfo{{ID: "7", CourseCode: "CS3103", Name: "Networks", Current: true}}, nil
				},
			}
			err := RunCanvasAuth(context.Background(), deps)
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("RunCanvasAuth() = %v, want %q", err, tc.wantErr)
			}
			if prompts.index != len(tc.answers) {
				t.Fatalf("used %d of %d answers", prompts.index, len(tc.answers))
			}
			saved, _ := canvas.LoadCredential(root)
			if saved != tc.wantSave {
				t.Fatalf("saved token = %q, want %q", saved, tc.wantSave)
			}
		})
	}
}

func TestCanvasAuthCancellationDoesNotPersistNewToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}}
	if err := vault.Initialize(root, workspace, uiAssets()); err != nil {
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
	workspace := config.Workspace{Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}}
	if err := vault.Initialize(root, workspace, uiAssets()); err != nil {
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
	workspace := config.Workspace{Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}}
	if err := vault.Initialize(root, workspace, uiAssets()); err != nil {
		t.Fatal(err)
	}
	writePath := filepath.Join(root, "courses", "OLD", "course.yaml")
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		t.Fatal(err)
	}
	before := []byte("code: OLD\ncanvas:\n  id: 7\n  sources: [assignments]\n")
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

func TestNonTTYGuidanceIsDeterministic(t *testing.T) {
	if got := NonTTYGuidance("init"); got != "interactive init requires a terminal; use corum init --defaults PATH" {
		t.Fatalf("NonTTYGuidance() = %q", got)
	}
}

func TestTokenFingerprintHidesTheSecret(t *testing.T) {
	token := "21450~abcdefghijklmnopqrstuvwxyz0123456789"
	got := tokenFingerprint(token)
	if got != "42 characters, 21450~…6789" || strings.Contains(got, "abcdefghij") {
		t.Fatalf("fingerprint = %q", got)
	}
	if got := tokenFingerprint("short"); got != "5 characters" {
		t.Fatalf("short fingerprint = %q", got)
	}
}
