package vault

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/algebananazzzzz/Corum/internal/lockfile"

	"github.com/algebananazzzzz/Corum/internal/config"
)

func failRenameAt(wants ...int) func(string, string) error {
	failures := map[int]bool{}
	for _, want := range wants {
		failures[want] = true
	}
	count := 0
	return func(old, new string) error {
		count++
		if failures[count] {
			return errors.New("injected rename failure")
		}
		return os.Rename(old, new)
	}
}

func testWorkspace() config.Workspace {
	return config.Workspace{
		Version:   2,
		Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "Term"},
		Canvas:    &config.CanvasWorkspace{URL: "https://canvas.example.edu"},
		Calendar:  config.Calendar{Timetable: "Timetable.md", Term: "Term.md"},
	}
}

func testAssets(label string) fs.FS {
	return fstest.MapFS{
		"agent-kit/AGENTS.base.md":                          &fstest.MapFile{Data: []byte(label + " agents")},
		"agent-kit/skills/example/SKILL.md":                 &fstest.MapFile{Data: []byte(label + " skill")},
		"agent-kit/templates/template.md":                   &fstest.MapFile{Data: []byte(label + " template")},
		"agent-kit/templates/wiki/index.md":                 &fstest.MapFile{Data: []byte(label + " index")},
		"agent-kit/assets/calendar/ay2026_27_semester_1.md": &fstest.MapFile{Data: []byte(label + " semester one")},
		"agent-kit/assets/calendar/ay2026_27_semester_2.md": &fstest.MapFile{Data: []byte(label + " semester two")},
	}
}

func TestInitializeCopiesSelectedCalendar(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	workspace := testWorkspace()
	workspace.Workspace.Term = "AY2026/27 Semester 2"
	workspace.Calendar.Term = "Term_Calendar.md"

	if err := Initialize(root, workspace, testAssets("calendar"), "test"); err != nil {
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
	if err := Initialize(root, testWorkspace(), testAssets(label), label); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInitializeValidatesBeforeItsFirstWrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "vault")
	workspace := testWorkspace()
	workspace.Version = 1
	if err := Initialize(root, workspace, testAssets("new"), "new"); err == nil {
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
	if err := Initialize(root, testWorkspace(), testAssets("new"), "new"); err == nil {
		t.Fatal("Initialize() succeeded")
	}
	if got, err := os.ReadFile(filepath.Join(root, "keep")); err != nil || string(got) != "user" {
		t.Fatalf("user file = %q, %v", got, err)
	}
}

func TestInitializeCreatesOnlyExpectedTree(t *testing.T) {
	root := initializedVault(t, "v1")
	var got []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root {
			rel, _ := filepath.Rel(root, path)
			got = append(got, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".agents", ".agents/skills", ".claude", ".claude/skills", ".codex", ".codex/skills",
		".config", ".config/corum", ".config/corum/.gitignore", ".config/corum/corum.yaml", ".config/corum/toolkit-version",
		"AGENTS.md", "CLAUDE.md", "courses",
		"skills", "skills/example", "skills/example/SKILL.md",
		"templates", "templates/template.md", "templates/wiki", "templates/wiki/index.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("initialized tree = %#v, want %#v", got, want)
	}
}

func TestInitializeKeepsToolkitMetadataInProjectConfiguration(t *testing.T) {
	root := initializedVault(t, "v1")
	if _, err := os.Stat(filepath.Join(root, ".corum")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("project-local .corum directory = %v, want absent", err)
	}
	assertToolkitVersion(t, root, "v1")
}

func assertToolkitLinks(t *testing.T, root, label string) {
	t.Helper()
	for path, target := range map[string]string{
		"CLAUDE.md":      "AGENTS.md",
		".claude/skills": "../skills",
		".codex/skills":  "../skills",
		".agents/skills": "../skills",
	} {
		got, err := os.Readlink(filepath.Join(root, path))
		if err != nil || got != target {
			t.Fatalf("Readlink(%s) = %q, %v; want %q", path, got, err, target)
		}
	}
	assertFileContent(t, filepath.Join(root, "AGENTS.md"), label+" agents")
	for _, directory := range []string{".claude", ".codex", ".agents"} {
		assertFileContent(t, filepath.Join(root, directory, "skills/example/SKILL.md"), label+" skill")
	}
}

func TestToolkitLinksSurviveVaultMove(t *testing.T) {
	root := initializedVault(t, "old")
	moved := filepath.Join(t.TempDir(), "moved vault")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	assertToolkitLinks(t, moved, "old")
}

func TestSyncToolkitRepairsLegacyLayoutAtSameVersion(t *testing.T) {
	root := initializedVault(t, "old")
	for _, path := range []string{"AGENTS.md", "CLAUDE.md", ".claude/skills", ".codex/skills", ".agents/skills"} {
		if err := os.Remove(filepath.Join(root, path)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("prior agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(root, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SyncToolkit(root, testAssets("old"), "old"); err != nil {
		t.Fatal(err)
	}
	assertToolkitLinks(t, root, "old")
	assertFileContent(t, settings, "preserve")
}

func TestToolkitFailureRestoresLinks(t *testing.T) {
	for failAt := 1; failAt <= 16; failAt++ {
		root := initializedVault(t, "old")
		if err := syncToolkit(root, testAssets("new"), "new", failRenameAt(failAt)); err == nil {
			t.Fatalf("rename %d did not fail", failAt)
		}
		assertToolkitLinks(t, root, "old")
		assertToolkitVersion(t, root, "old")
	}
}

func TestSyncToolkitRejectsAgentDirectorySymlink(t *testing.T) {
	root := initializedVault(t, "old")
	outside := t.TempDir()
	if err := os.Rename(filepath.Join(root, ".claude"), filepath.Join(root, ".claude-saved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".claude")); err != nil {
		t.Fatal(err)
	}
	if err := SyncToolkit(root, testAssets("new"), "new"); err == nil {
		t.Fatal("SyncToolkit accepted a symlinked agent directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("external directory changed: %v, %v", entries, err)
	}
	assertToolkitVersion(t, root, "old")
	assertFileContent(t, filepath.Join(root, "CLAUDE.md"), "old agents")
}

func TestInitializedConfigurationConformsToV2BlockSchema(t *testing.T) {
	root := initializedVault(t, "v1")
	contents, err := os.ReadFile(filepath.Join(root, ".config", "corum", "corum.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "jira: null") || strings.Contains(string(contents), "canvas: null") || strings.Contains(string(contents), "wiki: null") {
		t.Fatalf("initialized configuration contains a null service block:\n%s", contents)
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil {
		t.Fatalf("initialized configuration is not valid v2 configuration: %v", err)
	}
	if workspace.Jira != nil || workspace.Canvas == nil {
		t.Fatalf("initialized services = %+v", workspace)
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
		if err := os.WriteFile(path, []byte("version: 2\ncode: SAME\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := Validate(root); err == nil {
		t.Fatal("Validate() accepted duplicate course codes")
	}
}

func TestWriteCourseReplacesExistingCourseConfiguration(t *testing.T) {
	root := initializedVault(t, "test")
	writeCourseFixture(t, root, "CS3103", "version: 2\ncode: CS3103\ncanvas:\n  id: 1\n  sources: [assignments]\n")
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

func TestSyncToolkitRefusesConcurrentProcessBeforeAnyWrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := initializedVault(t, "old")
	toolkitDir := config.ProjectDir(root)
	lock, err := lockfile.Acquire(filepath.Join(toolkitDir, toolkitLockName))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syncToolkit(root, testAssets("new"), "new", os.Rename); !errors.Is(err, lockfile.ErrLocked) {
		t.Fatalf("syncToolkit error = %v, want ErrLocked", err)
	}
	assertToolkitVersion(t, root, "old")
}

func TestToolkitOverwritesOnlyOwnedFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := initializedVault(t, "old")
	userPath := filepath.Join(root, "courses", "user.md")
	if err := os.WriteFile(userPath, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncToolkit(root, testAssets("new"), "new", os.Rename); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(root, "AGENTS.md"), "new agents")
	assertFileContent(t, filepath.Join(root, "skills", "example", "SKILL.md"), "new skill")
	assertFileContent(t, filepath.Join(root, "templates", "template.md"), "new template")
	assertToolkitVersion(t, root, "new")
	assertFileContent(t, userPath, "preserve")
}

func TestSyncToolkitSkipsCurrentProject(t *testing.T) {
	root := initializedVault(t, "new")
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("current sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SyncToolkit(root, testAssets("new"), "new"); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, agents, "current sentinel")
}

func TestToolkitFailureRollsBackOwnedFiles(t *testing.T) {
	for _, failAt := range []int{2, 8} {
		t.Run("rename", func(t *testing.T) {
			root := initializedVault(t, "old")
			err := syncToolkit(root, testAssets("new"), "new", failRenameAt(failAt))
			if err == nil {
				t.Fatal("syncToolkit succeeded")
			}
			assertToolkitVersion(t, root, "old")
			assertFileContent(t, filepath.Join(root, "AGENTS.md"), "old agents")
			assertFileContent(t, filepath.Join(root, "skills", "example", "SKILL.md"), "old skill")
			assertFileContent(t, filepath.Join(root, "templates", "template.md"), "old template")
		})
	}
}

func TestToolkitReportsRollbackFailureAndRetainsBackup(t *testing.T) {
	root := initializedVault(t, "old")
	err := syncToolkit(root, testAssets("new"), "new", failRenameAt(2, 3))
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("syncToolkit() error = %v", err)
	}
	entries, readErr := os.ReadDir(config.ProjectDir(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), toolkitTemporaryPrefix+"backup-") {
			assertFileContent(t, filepath.Join(config.ProjectDir(root), entry.Name()), "old agents")
			return
		}
	}
	t.Fatal("rollback removed the recoverable backup")
}

func TestToolkitKeepsRecoveryBackupAcrossAnotherFailedSync(t *testing.T) {
	root := initializedVault(t, "old")
	if err := syncToolkit(root, testAssets("new"), "new", failRenameAt(2, 3)); err == nil {
		t.Fatal("first sync succeeded")
	}
	if err := syncToolkit(root, testAssets("newer"), "newer", failRenameAt(1)); err == nil {
		t.Fatal("second sync succeeded")
	}
	entries, err := os.ReadDir(config.ProjectDir(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), toolkitTemporaryPrefix+"backup-") {
			assertFileContent(t, filepath.Join(config.ProjectDir(root), entry.Name()), "old agents")
			return
		}
	}
	t.Fatal("second failed sync discarded the recoverable backup")
}

func TestToolkitReportsRollbackRemovalFailureAndRetainsBackup(t *testing.T) {
	root := initializedVault(t, "old")
	agents := filepath.Join(root, "AGENTS.md")
	originalRemoveAll := removeAll
	removeAll = func(path string) error {
		if path == agents {
			return errors.New("injected removal failure")
		}
		return originalRemoveAll(path)
	}
	t.Cleanup(func() { removeAll = originalRemoveAll })
	err := syncToolkit(root, testAssets("new"), "new", failRenameAt(6))
	if err == nil || !strings.Contains(err.Error(), "injected removal failure") || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("syncToolkit() error = %v", err)
	}
	entries, readErr := os.ReadDir(config.ProjectDir(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), toolkitTemporaryPrefix+"backup-") && strings.Contains(entry.Name(), "AGENTS.md") {
			assertFileContent(t, filepath.Join(config.ProjectDir(root), entry.Name()), "old agents")
			return
		}
	}
	t.Fatal("rollback removed the backup after replacement removal failed")
}

func assertToolkitVersion(t *testing.T, root, want string) {
	t.Helper()
	assertFileContent(t, filepath.Join(config.ProjectDir(root), "toolkit-version"), want+"\n")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("ReadFile(%s) = %q, %v; want %q", path, got, err, want)
	}
}
