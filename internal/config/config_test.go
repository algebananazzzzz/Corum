package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validWorkspace = `version: 2
workspace:
  timezone: Asia/Singapore
  term: AY2026/27 Semester 1
canvas:
  url: https://canvas.example.edu
jira:
  cloud_id: opaque-cloud
  project: TODO
  transitions:
    this_week: "2"
wiki: {}
calendar:
  timetable: Timetable.md
  term: Term_Calendar.md
`

const validCourse = `version: 2
code: MATH101
canvas:
  id: 1
  sources: [announcements, assignments]
  folders:
    Course Materials: lectures
jira:
  epic: TODO-1
wiki: {}
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

func TestLoadWorkspaceAcceptsCleanV2Shape(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "", validWorkspace)
	got, err := LoadWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || got.Canvas == nil || got.Jira == nil || got.Wiki == nil {
		t.Fatalf("LoadWorkspace() = %+v", got)
	}
}

func TestLoadRejectsUnknownFieldsAndExtraDocuments(t *testing.T) {
	for name, value := range map[string]string{
		"unknown":        strings.Replace(validWorkspace, "calendar:", "unexpected: true\ncalendar:", 1),
		"extra document": validWorkspace + "---\nversion: 2\n",
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

func TestLoadRejectsVersionOneWithoutWriting(t *testing.T) {
	root := t.TempDir()
	contents := strings.Replace(validWorkspace, "version: 2", "version: 1", 1)
	writeConfig(t, root, "", contents)
	if _, err := LoadWorkspace(root); err == nil {
		t.Fatal("LoadWorkspace() accepted version 1")
	}
	got, err := os.ReadFile(filepath.Join(root, ".config", "corum", "corum.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Fatalf("corum.yaml changed to %q", got)
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

func TestLoadWorkspaceExplainsVersionOneVault(t *testing.T) {
	root := t.TempDir()
	contents := strings.Replace(validWorkspace, "version: 2", "version: 1", 1)
	writeConfig(t, root, "", contents)
	_, err := LoadWorkspace(root)
	if err == nil {
		t.Fatal("LoadWorkspace() accepted version 1")
	}
	if !strings.Contains(err.Error(), "version 1") || !strings.Contains(err.Error(), "version 2") {
		t.Fatalf("error = %v, want v1 migration guidance", err)
	}
}

func TestLoadWorkspaceValidatesTimezoneOriginIdentifiersAndPaths(t *testing.T) {
	cases := map[string]string{
		"timezone":           strings.Replace(validWorkspace, "Asia/Singapore", "Moon/Base", 1),
		"origin":             strings.Replace(validWorkspace, "https://canvas.example.edu", "https://user:pass@canvas.example.edu/path", 1),
		"project":            strings.Replace(validWorkspace, "project: TODO", "project: lower", 1),
		"transition":         strings.Replace(validWorkspace, "this_week: \"2\"", "this_week: nope", 1),
		"calendar traversal": strings.Replace(validWorkspace, "Timetable.md", "../Timetable.md", 1),
		"credential key":     strings.Replace(validWorkspace, "project: TODO", "token: never", 1),
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
		"workspace jira null":   strings.Replace(validWorkspace, "jira:\n  cloud_id: opaque-cloud\n  project: TODO\n  transitions:\n    this_week: \"2\"", "jira: null", 1),
		"workspace wiki null":   strings.Replace(validWorkspace, "wiki: {}", "wiki: null", 1),
		"course canvas null":    strings.Replace(validCourse, "canvas:\n  id: 1\n  sources: [announcements, assignments]\n  folders:\n    Course Materials: lectures", "canvas: null", 1),
		"course jira null":      strings.Replace(validCourse, "jira:\n  epic: TODO-1", "jira: null", 1),
		"course wiki null":      strings.Replace(validCourse, "wiki: {}", "wiki: null", 1),
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

func TestEffectiveServicesUseBlockPresence(t *testing.T) {
	workspace := Workspace{Canvas: &CanvasWorkspace{}, Jira: &JiraWorkspace{}, Wiki: &WikiWorkspace{}}
	course := Course{Canvas: &CanvasCourse{}, Wiki: &WikiCourse{}}
	got := Effective(workspace, course)
	if !got.Canvas || got.Jira || !got.Wiki {
		t.Fatalf("Effective() = %+v", got)
	}
}
