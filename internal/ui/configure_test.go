package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
)

func TestConfigurationMenuUsesActiveTrackerAndTrackedCourseCount(t *testing.T) {
	courses := []config.Course{
		{Code: "A", Canvas: &config.CanvasCourse{ID: 1}, Jira: &config.JiraCourse{Epic: "STUDY-1"}},
		{Code: "B", Canvas: &config.CanvasCourse{ID: 2}, GoogleTasks: &config.GoogleTasksCourse{ListID: "list"}},
		{Code: "OLD", Jira: &config.JiraCourse{Epic: "STUDY-2"}},
	}
	for _, tc := range []struct {
		tracker, label string
		count          int
	}{{"jira", "Jira Epic mapping", 5}, {"google_tasks", "Google Task List mapping", 5}, {"none", "", 4}} {
		choices := configurationChoices(config.Workspace{TaskTracker: tc.tracker, Jira: &config.JiraWorkspace{}}, courses)
		if len(choices) != tc.count {
			t.Fatalf("%s: %+v", tc.tracker, choices)
		}
		if tc.label != "" && (!strings.Contains(choices[3].Label, tc.label) || !strings.Contains(choices[3].Label, "1/2 courses mapped")) {
			t.Fatal(choices)
		}
	}
}

func TestMapCourseCommitsOnlySuccessfulActions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		answers   []any
		createErr error
		want      string
		wantErr   bool
		creates   int
	}{
		{"existing", []any{0, 0}, nil, "existing", false, 0},
		{"create", []any{1, "Typed name"}, nil, "created", false, 1},
		{"failure", []any{1, "Typed name"}, errors.New("offline"), "old", true, 1},
		{"cancel name", []any{1, ErrCancelled}, nil, "old", true, 0},
		{"back", []any{3}, nil, "old", false, 0},
		{"unmap", []any{2}, nil, "", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "courses", "CS101")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			original := []byte("code: CS101\ncanvas:\n  id: 1\n  sources: [assignments]\njira:\n  epic: STUDY-1\ngoogle_tasks:\n  list_id: old\n")
			path := filepath.Join(dir, "course.yaml")
			if err := os.WriteFile(path, original, 0644); err != nil {
				t.Fatal(err)
			}
			course, err := config.LoadCourse(root, "CS101")
			if err != nil {
				t.Fatal(err)
			}
			creates := 0
			store := destinationStore{Create: func(_ context.Context, name string) (destination, error) {
				creates++
				if name != "Typed name" {
					t.Fatal(name)
				}
				return destination{"created", name}, tc.createErr
			}}
			err = mapCourse(context.Background(), root, course, "google_tasks", []destination{{"existing", "Existing"}}, store, &scriptedPrompts{answers: tc.answers})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			saved, err := config.LoadCourse(root, "CS101")
			if err != nil {
				t.Fatal(err)
			}
			if got := mappingID(saved, "google_tasks"); got != tc.want {
				t.Fatalf("id=%q want %q", got, tc.want)
			}
			if saved.Jira.Epic != "STUDY-1" || saved.Canvas.ID != 1 {
				t.Fatal("unrelated settings lost")
			}
			if creates != tc.creates {
				t.Fatalf("creates=%d", creates)
			}
			if tc.wantErr {
				data, _ := os.ReadFile(path)
				if string(data) != string(original) {
					t.Fatal("failed action changed file")
				}
			}
		})
	}
}

func TestImportGoogleClientRequiresDesktopCredentials(t *testing.T) {
	target := filepath.Join(t.TempDir(), "client_secret.json")
	if err := importGoogleClient(target, []byte(`{"web":{"client_id":"id"}}`)); err == nil {
		t.Fatal("accepted web client")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("invalid import wrote credentials")
	}
	valid := []byte(`{"installed":{"client_id":"id","client_secret":"secret","project_id":"project"}}`)
	if err := importGoogleClient(target, valid); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if err := importGoogleClient(target, valid); err == nil {
		t.Fatal("overwrote existing client")
	}
}
