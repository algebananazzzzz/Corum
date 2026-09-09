package jira

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncEpicWritesDeterministicState(t *testing.T) {
	root := t.TempDir()
	courseDir := filepath.Join(root, "courses", "CS3103", "state")
	if err := os.MkdirAll(courseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	client := &fakeSyncClient{
		epic: remoteEpic("STUDY-1", "CS3103 — Algorithms"),
		children: []map[string]any{
			remoteIssue("STUDY-10"), remoteIssue("STUDY-2"),
		},
	}
	for _, child := range client.children {
		child["fields"].(map[string]any)["parent"] = map[string]any{"key": "STUDY-1"}
	}
	result, err := SyncEpic(context.Background(), root, "CS3103", "STUDY-1", SyncOptions{Project: "STUDY", Client: client, Now: func() time.Time { return time.Date(2026, 9, 9, 1, 2, 3, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	if result.IssueCount != 2 {
		t.Fatalf("IssueCount = %d", result.IssueCount)
	}
	state, err := readJiraState(filepath.Join(courseDir, "jira.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Issues) != 2 || state.Issues[0].Key != "STUDY-2" || state.Issues[1].Key != "STUDY-10" {
		t.Fatalf("issues were not sorted deterministically: %+v", state.Issues)
	}
}

func TestSyncEpicFailurePreservesExistingState(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "courses", "CS3103", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stateDir, "jira.json")
	original := []byte(`{"version":2,"reconciled_at":null,"issues":[]}`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := SyncEpic(context.Background(), root, "CS3103", "STUDY-1", SyncOptions{Project: "STUDY", Client: &fakeSyncClient{fetchErr: os.ErrInvalid}})
	if err == nil {
		t.Fatal("SyncEpic succeeded")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("failed sync changed existing state")
	}
}

type fakeSyncClient struct {
	epic     map[string]any
	children []map[string]any
	fetchErr error
}

func remoteIssue(key string) map[string]any {
	return map[string]any{"key": key, "fields": map[string]any{
		"issuetype": map[string]any{"name": "Task"}, "summary": "Summary " + key,
		"status": map[string]any{"name": "To Do"}, "labels": []any{},
		"updated": "2026-09-09T00:00:00+0800",
	}}
}

func remoteEpic(key, summary string) map[string]any {
	value := remoteIssue(key)
	value["fields"].(map[string]any)["issuetype"] = map[string]any{"name": "Epic"}
	value["fields"].(map[string]any)["summary"] = summary
	return value
}

func (f *fakeSyncClient) FetchIssue(context.Context, string) (map[string]any, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.epic, nil
}
func (f *fakeSyncClient) EpicChildren(context.Context, string) ([]map[string]any, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.children, nil
}
func (*fakeSyncClient) CreateIssue(context.Context, map[string]any) (string, error) { panic("unused") }
func (*fakeSyncClient) UpdateFields(context.Context, string, map[string]any) error  { panic("unused") }
func (*fakeSyncClient) TransitionIssue(context.Context, string, string) error       { panic("unused") }
