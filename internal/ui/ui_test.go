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

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

type scriptedPrompts struct {
	answers []any
	index   int
}

func (p *scriptedPrompts) next() (any, error) {
	if p.index == len(p.answers) {
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
		SnapshotAuth:   func() (func() error, error) { return func() error { return nil }, nil },
	}
}

func TestJiraDisabledInitialization(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	prompts := &scriptedPrompts{answers: []any{root, "Asia/Singapore", "Term", "https://canvas.example.edu", true, false, true}}
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
}

func TestJiraEnabledInitializationSelectsProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	prompts := &scriptedPrompts{answers: []any{root, "Asia/Singapore", "Term", "https://canvas.example.edu", true, true, 0, 0, true}}
	deps := testDependencies(prompts)
	deps.OpenJira = func(context.Context) (JiraSession, error) {
		return fakeJira{resources: []Resource{{CloudID: "cloud", Name: "Site"}}, projects: map[string][]Project{"cloud": []Project{{Key: "TODO", Name: "Todo"}}}}, nil
	}
	if err := RunInit(context.Background(), root, deps); err != nil {
		t.Fatal(err)
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil || workspace.Jira == nil || workspace.Jira.Project != "TODO" || workspace.Jira.CloudID != "cloud" {
		t.Fatalf("workspace = %+v, %v", workspace, err)
	}
}

func TestInvalidInitAnswersWriteNothing(t *testing.T) {
	for name, answers := range map[string][]any{
		"timezone": {"Asia/NotAZone", "Term", "https://canvas.example.edu", true, false},
		"origin":   {"Asia/Singapore", "Term", "http://canvas.example.edu", true, false},
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
	prompts := &scriptedPrompts{answers: []any{root, "Asia/Singapore", "Term", "https://canvas.example.edu", true, false, false}}
	err := RunInit(context.Background(), root, testDependencies(prompts))
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("RunInit error = %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled root exists: %v", err)
	}
}

func TestOAuthCancellationRestoresPriorState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	prompts := &scriptedPrompts{answers: []any{root, "Asia/Singapore", "Term", "https://canvas.example.edu", true, true}}
	restored := false
	deps := testDependencies(prompts)
	deps.SnapshotAuth = func() (func() error, error) { return func() error { restored = true; return nil }, nil }
	deps.OpenJira = func(context.Context) (JiraSession, error) { return nil, ErrCancelled }
	err := RunInit(context.Background(), root, deps)
	if !errors.Is(err, ErrCancelled) || !restored {
		t.Fatalf("RunInit error = %v, restored = %v", err, restored)
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled root exists: %v", statErr)
	}
}

func TestCancelledVaultJiraLoginPreservesAuthAndConfiguration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	if err := vault.Initialize(root, workspace, uiAssets(), "test"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "corum.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	restored := false
	writeCalled := false
	prompts := &scriptedPrompts{answers: []any{0, 0, false}}
	err = RunJiraLogin(context.Background(), root, LoginDependencies{
		Prompts: prompts,
		OpenJira: func(context.Context) (JiraSession, error) {
			return fakeJira{resources: []Resource{{CloudID: "cloud", Name: "Site"}}, projects: map[string][]Project{"cloud": []Project{{Key: "TODO", Name: "Todo"}}}}, nil
		},
		SnapshotAuth: func() (func() error, error) { return func() error { restored = true; return nil }, nil },
		Write:        func(string, config.Workspace) error { writeCalled = true; return nil },
	})
	if !errors.Is(err, ErrCancelled) || !restored || writeCalled {
		t.Fatalf("RunJiraLogin() = %v, restored = %v, write = %v", err, restored, writeCalled)
	}
	after, err := os.ReadFile(filepath.Join(root, "corum.yaml"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("corum.yaml changed: %q, %v", after, err)
	}
}

func TestSummaryHasNoSecrets(t *testing.T) {
	value := initSummary(config.Workspace{Jira: &config.JiraWorkspace{CloudID: "cloud", Project: "TODO"}})
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
