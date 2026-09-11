package google

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncTasksWritesDeterministicState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "courses", "CS3103", "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := fakeClient{tasks: []map[string]any{
		{"id": "b", "title": "Second", "status": "needsAction", "due": "2026-09-12T00:00:00Z", "parent": "a"},
		{"id": "a", "title": "First", "status": "completed", "completed": "2026-09-10T00:00:00Z"},
	}}
	result, err := SyncTasks(context.Background(), root, "CS3103", "list-1", SyncOptions{
		Client: client,
		Now:    func() time.Time { return time.Date(2026, 9, 11, 1, 2, 3, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskCount != 2 {
		t.Fatalf("TaskCount = %d", result.TaskCount)
	}
	data, err := os.ReadFile(filepath.Join(root, "courses", "CS3103", "state", "google-tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state TaskState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 2 || state.Tasks[0].ID != "a" || state.Tasks[1].ID != "b" {
		t.Fatalf("tasks were not sorted deterministically: %+v", state.Tasks)
	}
	if state.Tasks[1].Due == nil || *state.Tasks[1].Due != "2026-09-12" {
		t.Fatalf("due date was not normalized: %+v", state.Tasks[1])
	}
}

func TestSyncTasksFailurePreservesExistingState(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "courses", "CS3103", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"reconciled_at":null,"tasks":[]}`)
	if err := os.WriteFile(filepath.Join(stateDir, "google-tasks.json"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := SyncTasks(context.Background(), root, "CS3103", "list-1", SyncOptions{Client: fakeClient{err: os.ErrInvalid}})
	if err == nil {
		t.Fatal("SyncTasks succeeded")
	}
	got, _ := os.ReadFile(filepath.Join(stateDir, "google-tasks.json"))
	if string(got) != string(original) {
		t.Fatal("failed sync changed existing state")
	}
}

type fakeClient struct {
	tasks []map[string]any
	err   error
}

func (f fakeClient) ListTasks(context.Context, string) ([]map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tasks, nil
}
