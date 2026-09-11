package schemas_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
	googletasks "github.com/algebananazzzzz/Corum/internal/google"
	"github.com/algebananazzzzz/Corum/internal/jira"
	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"
)

// Exercise the real serializers: schemas must accept the files Corum writes,
// including a named course and optional values absent from a minimal cache.
func TestSchemasAcceptSerializedState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		yaml  bool
	}{
		{"corum", config.Workspace{Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "AY2026/27 Semester 1"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}}, true},
		{"course", config.Course{Code: "CS101", Canvas: &config.CanvasCourse{ID: 1, Name: "Algorithms", Sources: []string{"assignments"}}}, true},
		{"canvas-state", canvas.CanvasState{Sources: map[string]any{"assignments": map[string]any{"1": "2026-09-30T00:00:00Z"}, "files": map[string]any{"2": "slides.pdf"}, "syllabus": nil}}, false},
		{"jira-state", jira.JiraState{Issues: []jira.IssueState{{Key: "STUDY-1", Type: "Task", Summary: "Report", Status: "To Do"}}}, false},
		{"google-tasks-state", googletasks.TaskState{Tasks: []googletasks.TaskStateItem{{ID: "task-1", Title: "Report", Status: "needsAction"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value any
			if tc.yaml {
				data, err := yaml.Marshal(tc.value)
				if err != nil {
					t.Fatal(err)
				}
				if err := yaml.Unmarshal(data, &value); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := json.Marshal(tc.value)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &value); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(tc.name + ".schema.json")
			if err != nil {
				t.Fatal(err)
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(value); err != nil {
				t.Fatalf("serialized value %#v: %v", value, err)
			}
		})
	}
}

func TestSchemasRetainUsefulDataAndRejectInvalidShapes(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		valid      bool
	}{
		{"corum", `{"workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"jira":{"cloud_id":"cloud-1","project":"STUDY"}}`, true},
		{"corum", `{"workspace":{"timezone":"Asia/Singapore"}}`, false},
		{"course", `{"code":"CS101","canvas":{"id":1,"name":"Algorithms","sources":["files"],"folders":{"Course Materials":"lectures"}},"jira":{"epic":"STUDY-1"}}`, true},
		{"course", `{"code":"CS101","google_tasks":{"list_id":"@default"}}`, true},
		{"course", `{"code":"CS101","canvas":{"id":1,"sources":["files"],"folders":{"Course Materials":"..\\outside"}}}`, false},
		{"course", `{"code":"CS101","canvas":{"id":1,"sources":["files","files"]}}`, false},
		{"jira-state", `{"issues":[{"key":"STUDY-1","type":"Task","summary":"Report","status":"To Do"}]}`, true},
		{"jira-state", `{"issues":[{"key":"STUDY-1","type":"Task","summary":"Report","status":"To Do","due":"2026-09-30","labels":["assessment"],"description":"Submit the report","updated_at":"2026-09-10T00:00:00Z"}]}`, true},
		{"jira-state", `{"issues":[{"key":"STUDY-1","summary":"Report"}]}`, false},
		{"google-tasks-state", `{"tasks":[{"id":"task-1","title":"Report","status":"needsAction"}]}`, true},
		{"canvas-state", `{"sources":{"announcements":{"1":"2026-09-10"},"assignments":{"2":null},"files":{"3":"lecture.pdf"},"pages":{"intro":"2026-09-10T00:00:00Z"},"modules":{"4":"hash"},"syllabus":"hash"}}`, true},
		{"canvas-state", `{"sources":{"files":["lecture.pdf"]}}`, false},
	} {
		t.Run(tc.name+tc.data, func(t *testing.T) {
			data, err := os.ReadFile(tc.name + ".schema.json")
			if err != nil {
				t.Fatal(err)
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if err := json.Unmarshal([]byte(tc.data), &value); err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(value); (err == nil) != tc.valid {
				t.Fatalf("valid=%t: %v", tc.valid, err)
			}
		})
	}
}
