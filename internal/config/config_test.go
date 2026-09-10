package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validWorkspace = `workspace:
  timezone: Asia/Singapore
  term: AY2026/27 Semester 1
canvas:
  url: https://canvas.example.edu
jira:
  cloud_id: opaque-cloud
  project: TODO
`

const validCourse = `code: MATH101
canvas:
  id: 1
  sources: [announcements, assignments]
  folders:
    Course Materials: lectures
jira:
  epic: TODO-1
`

func writeConfig(t *testing.T, root, course, contents string) {
	t.Helper()
	path := filepath.Join(root, ".config", "corum", "corum.yaml")
	if course != "" {
		path = filepath.Join(root, "courses", course, "course.yaml")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadWorkspaceAcceptsCurrentShape(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "", validWorkspace)
	got, err := LoadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Canvas == nil || got.Jira == nil {
		t.Fatalf("LoadWorkspace() = %+v", got)
	}
}

func TestLoadRejectsUnknownFieldsAndExtraDocuments(t *testing.T) {
	for name, value := range map[string]string{
		"unknown":        validWorkspace + "unexpected: true\n",
		"extra document": validWorkspace + "---\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeConfig(t, root, "", value)
			if _, err := LoadWorkspace(root); err == nil {
				t.Fatal("LoadWorkspace() succeeded")
			}
		})
	}
}

func TestLoadWorkspaceDoesNotReadLegacyRootConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "corum.yaml"), []byte(validWorkspace), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkspace(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadWorkspace() error = %v, want not exist", err)
	}
}

func TestLoadWorkspaceValidatesTimezoneOriginIdentifiersAndPaths(t *testing.T) {
	cases := map[string]string{
		"timezone":       strings.Replace(validWorkspace, "Asia/Singapore", "Moon/Base", 1),
		"origin":         strings.Replace(validWorkspace, "https://canvas.example.edu", "https://user:pass@canvas.example.edu/path", 1),
		"project":        strings.Replace(validWorkspace, "project: TODO", "project: lower", 1),
		"credential key": strings.Replace(validWorkspace, "project: TODO", "token: never", 1),
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeConfig(t, root, "", contents)
			if _, err := LoadWorkspace(root); err == nil {
				t.Fatal("LoadWorkspace() succeeded")
			}
		})
	}
}

func TestLoadCourseValidatesIssueIdentifierAndPaths(t *testing.T) {
	cases := map[string]string{
		"issue":            strings.Replace(validCourse, "TODO-1", "todo-0", 1),
		"folder traversal": strings.Replace(validCourse, "lectures", "../lectures", 1),
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeConfig(t, root, "MATH101", contents)
			if _, err := LoadCourse(root, "MATH101"); err == nil {
				t.Fatal("LoadCourse() succeeded")
			}
		})
	}
}

func TestLoadCourseAcceptsHumanFolderNames(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "MATH101", validCourse)
	if _, err := LoadCourse(root, "MATH101"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCourseRejectsEscapingAndMismatchedCodes(t *testing.T) {
	root := t.TempDir()
	escaped := filepath.Join(root, "outside", "course.yaml")
	if err := os.MkdirAll(filepath.Dir(escaped), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(escaped, []byte(validCourse), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCourse(filepath.Join(root, "courses", "safe"), "../../../outside"); err == nil {
		t.Fatal("LoadCourse() accepted an escaping course code")
	}
	writeConfig(t, root, "MATH101", strings.Replace(validCourse, "code: MATH101", "code: OTHER", 1))
	if _, err := LoadCourse(root, "MATH101"); err == nil {
		t.Fatal("LoadCourse() accepted a declared code that differs from its directory")
	}
}

func TestLoadRejectsNullServiceBlocksAndMissingCanvasSources(t *testing.T) {
	for name, contents := range map[string]string{
		"workspace canvas null": strings.Replace(validWorkspace, "canvas:\n  url: https://canvas.example.edu", "canvas: null", 1),
		"workspace jira null":   strings.Replace(validWorkspace, "jira:\n  cloud_id: opaque-cloud\n  project: TODO", "jira: null", 1),
		"course canvas null":    strings.Replace(validCourse, "canvas:\n  id: 1\n  sources: [announcements, assignments]\n  folders:\n    Course Materials: lectures", "canvas: null", 1),
		"course jira null":      strings.Replace(validCourse, "jira:\n  epic: TODO-1", "jira: null", 1),
		"course sources absent": strings.Replace(validCourse, "  sources: [announcements, assignments]\n", "", 1),
		"course sources null":   strings.Replace(validCourse, "sources: [announcements, assignments]", "sources: null", 1),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if strings.HasPrefix(name, "workspace") {
				writeConfig(t, root, "", contents)
				if _, err := LoadWorkspace(root); err == nil {
					t.Fatal("LoadWorkspace() succeeded")
				}
				return
			}
			writeConfig(t, root, "MATH101", contents)
			if _, err := LoadCourse(root, "MATH101"); err == nil {
				t.Fatal("LoadCourse() succeeded")
			}
		})
	}
}

func TestMinimalWorkspaceAndCourse(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "", "workspace:\n  timezone: Asia/Singapore\n  term: AY2026/27 Semester 1\ncanvas:\n  url: https://canvas.example.edu\n")
	if _, err := LoadWorkspace(root); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, root, "CS101", "code: CS101\ncanvas:\n  id: 1\n  sources: [assignments]\n")
	if _, err := LoadCourse(root, "CS101"); err != nil {
		t.Fatal(err)
	}
}
