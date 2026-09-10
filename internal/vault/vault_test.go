package vault

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/algebananazzzzz/Corum/internal/config"
)

func testWorkspace() config.Workspace {
	return config.Workspace{
		Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"},
		Canvas:    &config.CanvasWorkspace{URL: "https://canvas.example.edu"},
	}
}

func testAssets(label string) fs.FS {
	return fstest.MapFS{
		"agent-kit/AGENTS.base.md":                          &fstest.MapFile{Data: []byte(label + " agents")},
		"agent-kit/skills/example/SKILL.md":                 &fstest.MapFile{Data: []byte(label + " skill")},
		"agent-kit/assets/calendar/ay2026_27_semester_1.md": &fstest.MapFile{Data: []byte(label + " semester one")},
		"agent-kit/assets/calendar/ay2026_27_semester_2.md": &fstest.MapFile{Data: []byte(label + " semester two")},
	}
}

func TestInitializeCopiesSelectedCalendar(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	workspace := testWorkspace()
	workspace.Workspace.Term = "AY2026/27 Semester 2"

	if err := Initialize(root, workspace, testAssets("calendar")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "Term_Calendar.md"))
	if err != nil || string(got) != "calendar semester two" {
		t.Fatalf("selected calendar = %q, %v", got, err)
	}
}

func initializedVault(t *testing.T, label string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "vault")
	if err := Initialize(root, testWorkspace(), testAssets(label)); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInitializeValidatesBeforeItsFirstWrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := testWorkspace()
	workspace.Workspace.Timezone = "invalid"
	if err := Initialize(root, workspace, testAssets("new")); err == nil {
		t.Fatal("Initialize() succeeded")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("invalid initialization created %s: %v", root, err)
	}
}

func TestInitializeRefusesNonemptyTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(root, testWorkspace(), testAssets("new")); err == nil {
		t.Fatal("Initialize() succeeded")
	}
	if got, err := os.ReadFile(filepath.Join(root, "keep")); err != nil || string(got) != "user" {
		t.Fatalf("user file = %q, %v", got, err)
	}
}

func TestToolkitLinksSurviveVaultMove(t *testing.T) {
	root := initializedVault(t, "old")
	moved := filepath.Join(t.TempDir(), "moved vault")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{".claude", ".codex", ".agents"} {
		data, err := os.ReadFile(filepath.Join(moved, directory, "skills", "example", "SKILL.md"))
		if err != nil || string(data) != "old skill" {
			t.Fatalf("moved skill: %q, %v", data, err)
		}
	}
}

func TestInitializeAndValidateDoNotWriteGlobalConfiguration(t *testing.T) {
	globalConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", globalConfig)
	root := initializedVault(t, "old")
	if _, courses, err := Validate(root); err != nil || len(courses) != 0 {
		t.Fatalf("Validate() = %v courses, %v", len(courses), err)
	}
	if _, err := os.Stat(filepath.Join(globalConfig, "corum")); !os.IsNotExist(err) {
		t.Fatalf("global Corum configuration exists: %v", err)
	}
}

func TestValidateRejectsDuplicateCodes(t *testing.T) {
	root := initializedVault(t, "old")
	for _, directory := range []string{"A", "B"} {
		path := filepath.Join(root, "courses", directory, "course.yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("code: SAME\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := Validate(root); err == nil {
		t.Fatal("Validate() accepted duplicate course codes")
	}
}

func TestWriteCourseReplacesExistingCourseConfiguration(t *testing.T) {
	root := initializedVault(t, "test")
	writeCourseFixture(t, root, "CS3103", "code: CS3103\ncanvas:\n  id: 1\n  sources: [assignments]\n")
	course, err := config.LoadCourse(root, "CS3103")
	if err != nil {
		t.Fatal(err)
	}
	course.Jira = &config.JiraCourse{Epic: "STUDY-1"}
	if err := WriteCourse(root, course); err != nil {
		t.Fatal(err)
	}
	stored, err := config.LoadCourse(root, "CS3103")
	if err != nil || stored.Jira == nil || stored.Jira.Epic != "STUDY-1" {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}
}

func TestRefreshToolkitPreservesUserContentAndRemovesRetiredSkills(t *testing.T) {
	root := initializedVault(t, "old")
	for relative, contents := range map[string]string{
		"skills/custom/SKILL.md":          "user skill",
		"courses/CS101/wiki/index.md":     "user notes",
		"skills/authoring-wiki/SKILL.md":  "retired skill",
		"skills/drawio-diagrams/SKILL.md": "retired skill",
		"skills/linting-wiki/SKILL.md":    "retired skill",
	} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := RefreshToolkit(root, testAssets("new")); err != nil {
		t.Fatal(err)
	}
	for relative, want := range map[string]string{
		"AGENTS.md":                   "new agents",
		"skills/example/SKILL.md":     "new skill",
		"skills/custom/SKILL.md":      "user skill",
		"courses/CS101/wiki/index.md": "user notes",
	} {
		got, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", relative, got, err)
		}
	}
	for _, name := range []string{"authoring-wiki", "drawio-diagrams", "linting-wiki"} {
		if _, err := os.Stat(filepath.Join(root, "skills", name)); !os.IsNotExist(err) {
			t.Fatalf("retired skill %s still installed: %v", name, err)
		}
	}
}

func TestRefreshToolkitRejectsEscapingSkillPath(t *testing.T) {
	root := initializedVault(t, "old")
	outside := t.TempDir()
	if err := os.RemoveAll(filepath.Join(root, "skills", "example")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "skills", "example")); err != nil {
		t.Fatal(err)
	}
	if err := RefreshToolkit(root, testAssets("new")); err == nil {
		t.Fatal("refresh followed an escaping skill symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside files = %v, %v", entries, err)
	}
}
