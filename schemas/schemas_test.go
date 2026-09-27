package schemas_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/canvas"
	"github.com/algebananazzzzz/Corum/internal/config"
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
		{"corum", config.Workspace{TaskTracker: "kaneo", Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "AY2026/27 Semester 1"}, Kaneo: &config.KaneoWorkspace{URL: "https://kaneo.example.test", Project: "TOD"}}, true},
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
		{"corum", `{"task_tracker":"jira","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"jira":{"site":"https://study.atlassian.net","project":"STUDY"}}`, true},
		{"corum", `{"task_tracker":"jira","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"}}`, false},
		{"corum", `{"task_tracker":"jira","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"jira":{"site":"https://study.atlassian.net"}}`, true},
		{"corum", `{"task_tracker":"kaneo","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"}}`, false},
		{"corum", `{"task_tracker":"kaneo","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"kaneo":{"url":"https://kaneo.example.test"}}`, true},
		{"corum", `{"task_tracker":"jira","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"jira":{"project":"STUDY"}}`, false},
		{"corum", `{"workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"jira":{"site":"https://study.atlassian.net","project":"STUDY"}}`, false},
		{"corum", `{"task_tracker":"jira","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"jira":{"site":"https://study.atlassian.net","project":"STUDY"},"kaneo":{"url":"https://kaneo.example.test"}}`, false},
		{"corum", `{"task_tracker":"kaneo","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"kaneo":{"url":"https://kaneo.example.test","project":"TOD"}}`, true},
		{"corum", `{"task_tracker":"jira","workspace":{"timezone":"Asia/Singapore","term":"AY2026/27 Semester 1"},"jira":{"cloud_id":"cloud-1","project":"STUDY"}}`, false},
		{"corum", `{"workspace":{"timezone":"Asia/Singapore"}}`, false},
		{"course", `{"code":"CS101","canvas":{"id":1,"name":"Algorithms","sources":["files"],"folders":{"Course Materials":"lectures"}}}`, true},
		{"course", `{"code":"CS101","jira":{"epic":"STUDY-1"}}`, false},
		{"course", `{"code":"CS101","canvas":{"id":1,"sources":["files"],"folders":{"Course Materials":"..\\outside"}}}`, false},
		{"course", `{"code":"CS101","canvas":{"id":1,"sources":["files","files"]}}`, false},
		{"tracker-state", `{"tracker":"kaneo","group":{"id":"CS3103","name":"CS3103"},"synced_at":"2026-09-27T10:00:00+08:00","items":[{"id":"yuzp4b0t","title":"[CS3103] Assignment 1","type":"task","status":"to-do","done":false,"due":"2026-10-02T12:00:00+08:00","labels":["assessment"],"description":"Submit","url":"https://kaneo.example.test/task/1","updated_at":"2026-09-26T18:25:40.317Z"}]}`, true},
		{"tracker-state", `{"tracker":"jira","group":{"id":"STUDY-1","name":"CS3103"},"synced_at":"2026-09-27T10:00:00Z","items":[{"id":"STUDY-12","title":"[CS3103] Lab (W7)","type":"session","status":"To Do","done":false,"due":"2026-10-02"}]}`, true},
		{"tracker-state", `{"tracker":"google_tasks","group":null,"synced_at":"2026-09-27T10:00:00Z","items":[]}`, true},
		{"tracker-state", `{"tracker":"trello","group":null,"synced_at":"2026-09-27T10:00:00Z","items":[]}`, false},
		{"tracker-state", `{"tracker":"jira","group":null,"items":[]}`, false},
		{"tracker-state", `{"tracker":"jira","group":null,"synced_at":"2026-09-27T10:00:00Z","items":[{"id":"STUDY-12","title":"Lab","status":"To Do"}]}`, false},
		{"tracker-state", `{"tracker":"jira","group":null,"synced_at":"2026-09-27T10:00:00Z","items":[{"id":"STUDY-12","title":"Lab","status":"To Do","done":false,"type":"epic"}]}`, false},
		{"tracker-state", `{"tracker":"kaneo","group":null,"synced_at":"2026-09-27T10:00:00Z","items":[{"id":"a","title":"Lab","status":"to-do","done":false,"due":"2 Oct 2026"}]}`, false},
		{"tracker-state", `{"tracker":"kaneo","group":null,"synced_at":"2026-09-27 10:00","items":[]}`, false},
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
