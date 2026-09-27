package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/vault"
)

func TestConfigurationMenuNamesActiveTracker(t *testing.T) {
	for tracker, want := range map[string]string{"": "None", "jira": "Jira", "kaneo": "Kaneo", "google_tasks": "Google Tasks"} {
		choices := configurationChoices(config.Workspace{TaskTracker: tracker})
		if len(choices) != 4 || choices[2].Label != "Task tracker · "+want {
			t.Fatalf("%q: %+v", tracker, choices)
		}
	}
}

func trackerVault(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "vault")
	workspace := config.Workspace{Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"}}
	if err := vault.Initialize(root, workspace, uiAssets()); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestConfigureTrackerSavesKaneoAndInstallsMCP(t *testing.T) {
	root := trackerVault(t)
	notice, err := configureTracker(root, &scriptedPrompts{answers: []any{0, " https://kaneo.example.test/ "}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notice, "Kaneo") {
		t.Fatalf("notice = %q", notice)
	}
	ws, err := config.LoadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.TaskTracker != "kaneo" || ws.Kaneo == nil || ws.Kaneo.URL != "https://kaneo.example.test" || ws.Kaneo.Project != "" {
		t.Fatalf("workspace = %+v", ws)
	}
	data, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil || !strings.Contains(string(data), "https://kaneo.example.test/api/mcp") {
		t.Fatalf(".mcp.json = %s, %v", data, err)
	}
}

// The agent saves the project on first sync; reselecting Kaneo at the same
// URL must keep it, while a new URL must drop it.
func TestConfigureTrackerKeepsAgentProjectOnlyForSameKaneoURL(t *testing.T) {
	root := trackerVault(t)
	if _, err := configureTracker(root, &scriptedPrompts{answers: []any{0, "https://kaneo.example.test"}}); err != nil {
		t.Fatal(err)
	}
	ws, err := config.LoadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	ws.Kaneo.Project = "TOD"
	if err := vault.WriteWorkspace(root, ws); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ url, want string }{{"https://kaneo.example.test/", "TOD"}, {"https://other.example.test", ""}} {
		if _, err := configureTracker(root, &scriptedPrompts{answers: []any{0, tc.url}}); err != nil {
			t.Fatal(err)
		}
		ws, err := config.LoadWorkspace(root)
		if err != nil || ws.Kaneo.Project != tc.want {
			t.Fatalf("%s: workspace = %+v, %v", tc.url, ws.Kaneo, err)
		}
	}
}

func TestConfigureTrackerSwitchDropsPreviousSettingsAndMCP(t *testing.T) {
	root := trackerVault(t)
	if _, err := configureTracker(root, &scriptedPrompts{answers: []any{1, "https://study.atlassian.net/"}}); err != nil {
		t.Fatal(err)
	}
	if ws, err := config.LoadWorkspace(root); err != nil || ws.Jira == nil || ws.Jira.Site != "https://study.atlassian.net" {
		t.Fatalf("Jira site not saved: %+v, %v", ws.Jira, err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil || !strings.Contains(string(data), "atlassian") {
		t.Fatalf("Jira MCP not installed: %s, %v", data, err)
	}
	if _, err := configureTracker(root, &scriptedPrompts{answers: []any{0, "https://kaneo.example.test"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := configureTracker(root, &scriptedPrompts{answers: []any{2}}); err != nil {
		t.Fatal(err)
	}
	ws, err := config.LoadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.TaskTracker != "google_tasks" || ws.Kaneo != nil || ws.Jira != nil {
		t.Fatalf("workspace = %+v", ws)
	}
	data, err = os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil || strings.Contains(string(data), "atlassian") || strings.Contains(string(data), "kaneo") {
		t.Fatalf(".mcp.json = %s, %v", data, err)
	}
}

func TestConfigureTrackerRejectsInvalidInputWithoutSaving(t *testing.T) {
	for name, answers := range map[string][]any{
		"insecure url":  {0, "http://kaneo.example.test"},
		"url with path": {0, "https://kaneo.example.test/api"},
		"jira insecure": {1, "http://study.atlassian.net"},
		"back":          {4},
		"cancel":        {0, ErrCancelled},
	} {
		t.Run(name, func(t *testing.T) {
			root := trackerVault(t)
			before, err := os.ReadFile(config.WorkspacePath(root))
			if err != nil {
				t.Fatal(err)
			}
			_, err = configureTracker(root, &scriptedPrompts{answers: answers})
			if err == nil {
				t.Fatal("configureTracker succeeded")
			}
			if name == "back" && !errors.Is(err, ErrCancelled) {
				t.Fatalf("back error = %v", err)
			}
			after, err := os.ReadFile(config.WorkspacePath(root))
			if err != nil || string(after) != string(before) {
				t.Fatalf("workspace changed: %s, %v", after, err)
			}
			if _, err := os.Stat(filepath.Join(root, ".mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("MCP config written: %v", err)
			}
		})
	}
}

func TestStageVaultCreatesTargetOnlyOnSuccess(t *testing.T) {
	build := func(fail error) func(string) error {
		return func(staging string) error {
			if err := os.WriteFile(filepath.Join(staging, "marker"), []byte("built"), 0o644); err != nil {
				return err
			}
			return fail
		}
	}
	for _, tc := range []struct {
		name        string
		existing    bool
		fail        error
		wantCreated bool
	}{
		{"new target", false, nil, true},
		{"empty existing target", true, nil, true},
		{"rejected token", false, errors.New("Canvas rejected that token"), false},
		{"cancelled", true, ErrCancelled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			target := filepath.Join(parent, "vault")
			if tc.existing {
				if err := os.Mkdir(target, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			err := stageVault(target, build(tc.fail))
			if tc.fail != nil && !errors.Is(err, tc.fail) || tc.fail == nil && err != nil {
				t.Fatalf("stageVault() = %v", err)
			}
			_, statErr := os.Stat(filepath.Join(target, "marker"))
			if (statErr == nil) != tc.wantCreated {
				t.Fatalf("vault created = %v, want %v", statErr == nil, tc.wantCreated)
			}
			if !tc.wantCreated && !tc.existing {
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("failed setup left %s: %v", target, err)
				}
			}
			entries, err := os.ReadDir(parent)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), "corum-init") {
					t.Fatalf("staging directory left behind: %s", entry.Name())
				}
			}
		})
	}
}

func TestStageVaultRefusesNonEmptyTargetBeforeSetup(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "notes.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := stageVault(target, func(string) error { called = true; return nil }); err == nil || called {
		t.Fatalf("stageVault() = %v, build called = %v", err, called)
	}
}
