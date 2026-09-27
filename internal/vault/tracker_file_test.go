package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
)

func TestValidateTrackerFilesChecksShapeTrackerAndCourse(t *testing.T) {
	schema, err := os.ReadFile(filepath.Join("..", "..", "schemas", "tracker-state.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := config.Workspace{TaskTracker: "kaneo"}
	courses := []config.Course{{Code: "CS3103"}, {Code: "IS2218"}}
	valid := `{"tracker":"kaneo","group":{"id":"CS3103","name":"CS3103"},"synced_at":"2026-09-27T10:00:00+08:00","items":[{"id":"a","title":"[CS3103] Lab (W7)","type":"session","status":"to-do","done":false,"due":"2026-10-02T10:00:00+08:00"}]}`
	for _, tc := range []struct {
		name, file, want string
	}{
		{"valid", valid, ""},
		{"absent", "", ""},
		{"invalid json", `{"tracker":`, "tracker.json"},
		{"schema", strings.Replace(valid, `"done":false,`, "", 1), "tracker.json"},
		{"tracker", strings.Replace(valid, `"tracker":"kaneo"`, `"tracker":"jira"`, 1), "task tracker is"},
		{"group", strings.Replace(valid, `"name":"CS3103"`, `"name":"CS4225"`, 1), "course code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.file != "" {
				path := filepath.Join(root, "courses", "CS3103", "state", "tracker.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := ValidateTrackerFiles(root, workspace, courses, schema)
			if tc.want == "" && err != nil {
				t.Fatalf("ValidateTrackerFiles() = %v", err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("ValidateTrackerFiles() = %v, want error containing %q", err, tc.want)
			}
		})
	}
}
