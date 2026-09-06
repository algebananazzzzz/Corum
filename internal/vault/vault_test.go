package vault

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/algebananazzzzz/Corum/internal/config"
)

func failRenameAt(want int) func(string, string) error {
	count := 0
	return func(old, new string) error {
		count++
		if count == want {
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
		"agent-kit/AGENTS.base.md":          &fstest.MapFile{Data: []byte(label + " agents")},
		"agent-kit/skills/example/SKILL.md": &fstest.MapFile{Data: []byte(label + " skill")},
		"agent-kit/templates/template.md":   &fstest.MapFile{Data: []byte(label + " template")},
		"agent-kit/templates/wiki/index.md": &fstest.MapFile{Data: []byte(label + " index")},
	}
}

func initializedVault(t *testing.T, label string) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
		".corum", ".corum/toolkit-version", "AGENTS.md", "corum.yaml", "courses",
		"skills", "skills/example", "skills/example/SKILL.md",
		"templates", "templates/template.md", "templates/wiki", "templates/wiki/index.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("initialized tree = %#v, want %#v", got, want)
	}
}

func TestRegistryIsAbsoluteSortedAndDeduplicated(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := t.TempDir()
	b := t.TempDir()
	if err := Register(b); err != nil {
		t.Fatal(err)
	}
	if err := Register(a); err != nil {
		t.Fatal(err)
	}
	if err := Register(a); err != nil {
		t.Fatal(err)
	}
	got, err := Registered()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{a, b}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Registered() = %#v, want %#v", got, want)
	}
}

func TestValidateRegistersMovedVaultAndRejectsDuplicateCodes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := initializedVault(t, "old")
	if _, courses, err := Validate(root); err != nil || len(courses) != 0 {
		t.Fatalf("Validate() = %v courses, %v", len(courses), err)
	}
	registered, err := Registered()
	if err != nil || !reflect.DeepEqual(registered, []string{root}) {
		t.Fatalf("Registered() = %#v, %v", registered, err)
	}
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

func TestSyncToolkitsSkipsMissingVault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	missing := filepath.Join(t.TempDir(), "missing")
	if err := Register(missing); err != nil {
		t.Fatal(err)
	}
	got := SyncToolkits(testAssets("new"), "new")
	if len(got) != 1 || got[0].Root != missing || got[0].Err == nil {
		t.Fatalf("SyncToolkits() = %#v", got)
	}
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

func assertToolkitVersion(t *testing.T, root, want string) {
	t.Helper()
	assertFileContent(t, filepath.Join(root, ".corum", "toolkit-version"), want+"\n")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("ReadFile(%s) = %q, %v; want %q", path, got, err, want)
	}
}
