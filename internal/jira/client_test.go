package jira

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type callRecord struct {
	name string
	args map[string]any
}

type fakeJSONCaller struct {
	responses map[string][]any
	errors    map[string]error
	calls     []callRecord
}

func (f *fakeJSONCaller) CallJSON(_ context.Context, name string, args map[string]any, target any) error {
	f.calls = append(f.calls, callRecord{name: name, args: args})
	if err := f.errors[name]; err != nil {
		return err
	}
	values := f.responses[name]
	if len(values) == 0 {
		return errors.New("missing fake response")
	}
	f.responses[name] = values[1:]
	encoded, err := json.Marshal(values[0])
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func remoteIssue(key string) map[string]any {
	return map[string]any{
		"key": key,
		"fields": map[string]any{
			"issuetype":   map[string]any{"name": "Task"},
			"summary":     "Summary " + key,
			"status":      map[string]any{"name": "To Do"},
			"duedate":     nil,
			"labels":      []any{},
			"description": nil,
			"updated":     "2026-09-06T00:00:00+08:00",
		},
	}
}

func remoteEpic(key, summary string) map[string]any {
	issue := remoteIssue(key)
	fields := issue["fields"].(map[string]any)
	fields["issuetype"] = map[string]any{"name": "Epic"}
	fields["summary"] = summary
	return issue
}

func TestJiraClientEnsureEpic(t *testing.T) {
	ctx := context.Background()
	summary := "CS3103 — Computer Networks"
	t.Run("reuses one exact epic", func(t *testing.T) {
		caller := &fakeJSONCaller{responses: map[string][]any{
			"searchJiraIssuesUsingJql": []any{map[string]any{"data": map[string]any{"issues": []any{remoteEpic("STUDY-1", summary)}}}},
		}}
		client, _ := newJiraClient(caller, "cloud-1")
		result, err := client.EnsureEpic(ctx, "STUDY", summary)
		if err != nil || result != (EpicResult{Key: "STUDY-1"}) {
			t.Fatalf("EnsureEpic = %+v, %v", result, err)
		}
		want := map[string]any{"cloudId": "cloud-1", "jql": `project = "STUDY" AND issuetype = "Epic" AND summary ~ "CS3103"`, "maxResults": 50, "view": "full", "responseContentFormat": "markdown"}
		if !reflect.DeepEqual(caller.calls[0].args, want) {
			t.Fatalf("args = %#v, want %#v", caller.calls[0].args, want)
		}
	})
	t.Run("reuses one epic whose course code matches case insensitively", func(t *testing.T) {
		caller := &fakeJSONCaller{responses: map[string][]any{
			"searchJiraIssuesUsingJql": []any{map[string]any{"data": map[string]any{"issues": []any{remoteEpic("STUDY-1", "cs3103 — Existing Computer Networks")}}}},
		}}
		client, _ := newJiraClient(caller, "cloud-1")
		result, err := client.EnsureEpic(ctx, "STUDY", summary)
		if err != nil || result != (EpicResult{Key: "STUDY-1"}) {
			t.Fatalf("EnsureEpic = %+v, %v", result, err)
		}
		want := `project = "STUDY" AND issuetype = "Epic" AND summary ~ "CS3103"`
		if got := caller.calls[0].args["jql"]; got != want {
			t.Fatalf("JQL = %q, want %q", got, want)
		}
	})
	t.Run("creates when no epic exists", func(t *testing.T) {
		caller := &fakeJSONCaller{responses: map[string][]any{
			"searchJiraIssuesUsingJql": []any{map[string]any{"data": map[string]any{"issues": []any{}}}},
			"createJiraIssue":          []any{map[string]any{"data": map[string]any{"key": "STUDY-2"}}},
		}}
		client, _ := newJiraClient(caller, "cloud-1")
		result, err := client.EnsureEpic(ctx, "STUDY", summary)
		if err != nil || result != (EpicResult{Key: "STUDY-2", Created: true}) {
			t.Fatalf("EnsureEpic = %+v, %v", result, err)
		}
		want := map[string]any{"cloudId": "cloud-1", "projectKey": "STUDY", "summary": summary, "issueType": "Epic"}
		if !reflect.DeepEqual(caller.calls[1].args, want) {
			t.Fatalf("args = %#v, want %#v", caller.calls[1].args, want)
		}
	})
	t.Run("rejects ambiguous epic matches without creating", func(t *testing.T) {
		caller := &fakeJSONCaller{responses: map[string][]any{
			"searchJiraIssuesUsingJql": []any{map[string]any{"data": map[string]any{"issues": []any{remoteEpic("STUDY-1", summary), remoteEpic("STUDY-2", summary)}}}},
		}}
		client, _ := newJiraClient(caller, "cloud-1")
		if _, err := client.EnsureEpic(ctx, "STUDY", summary); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("EnsureEpic error = %v", err)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})
	t.Run("rejects a malformed epic result without creating", func(t *testing.T) {
		malformed := remoteEpic("STUDY-1", summary)
		delete(malformed["fields"].(map[string]any), "summary")
		caller := &fakeJSONCaller{responses: map[string][]any{
			"searchJiraIssuesUsingJql": []any{map[string]any{"data": map[string]any{"issues": []any{malformed}}}},
			"createJiraIssue":          []any{map[string]any{"data": map[string]any{"key": "STUDY-2"}}},
		}}
		client, _ := newJiraClient(caller, "cloud-1")
		if _, err := client.EnsureEpic(ctx, "STUDY", summary); err == nil {
			t.Fatal("EnsureEpic accepted a malformed Jira epic result")
		}
		if len(caller.calls) != 1 {
			t.Fatalf("calls = %#v", caller.calls)
		}
	})
}

func TestJiraClientEpicChildrenAcceptsNilIssues(t *testing.T) {
	caller := &fakeJSONCaller{responses: map[string][]any{
		"searchJiraIssuesUsingJql": []any{map[string]any{"data": map[string]any{"issues": nil, "isLast": true}}},
	}}
	client, _ := newJiraClient(caller, "cloud-1")
	children, err := client.EpicChildren(context.Background(), "STUDY-1")
	if err != nil {
		t.Fatalf("EpicChildren returned an error for an empty Jira result: %v", err)
	}
	if len(children) != 0 {
		t.Fatalf("EpicChildren = %#v, want no children", children)
	}
}

func TestJiraClientReconcileEpicNeverCreatesAfterAnUncertainAttempt(t *testing.T) {
	caller := &fakeJSONCaller{responses: map[string][]any{
		"searchJiraIssuesUsingJql": []any{map[string]any{"data": map[string]any{"issues": []any{}}}},
	}}
	client, _ := newJiraClient(caller, "cloud-1")
	_, err := client.ReconcileEpic(context.Background(), "STUDY", "CS3103 — Computer Networks")
	if !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("ReconcileEpic error = %v", err)
	}
	if len(caller.calls) != 1 || caller.calls[0].name != "searchJiraIssuesUsingJql" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestJiraClientMapsExactRovoArgumentsAndPagination(t *testing.T) {
	ctx := context.Background()
	caller := &fakeJSONCaller{responses: map[string][]any{
		"createJiraIssue":          {map[string]any{"data": map[string]any{"key": "STUDY-2"}}},
		"editJiraIssue":            {map[string]any{"data": map[string]any{}}},
		"transitionJiraIssue":      {map[string]any{"data": map[string]any{}}},
		"getJiraIssue":             {map[string]any{"data": remoteIssue("STUDY-2")}},
		"searchJiraIssuesUsingJql": {map[string]any{"data": map[string]any{"issues": []any{remoteIssue("STUDY-2")}, "nextPageToken": "page-2", "isLast": false}}, map[string]any{"data": map[string]any{"issues": []any{remoteIssue("STUDY-3")}, "isLast": true}}},
	}}
	client, err := newJiraClient(caller, "cloud-1")
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.CreateIssue(ctx, map[string]any{"project": "STUDY", "type": "Task", "parent": "STUDY-1", "summary": "Task", "description": "**Deadline:** Friday", "due": "2026-09-11", "labels": []string{"assessment"}})
	if err != nil || key != "STUDY-2" {
		t.Fatalf("CreateIssue = %q, %v", key, err)
	}
	if err := client.UpdateFields(ctx, "STUDY-2", map[string]any{"type": "Milestone", "parent": "STUDY-1", "summary": "Updated", "description": nil, "due": nil, "labels": []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := client.TransitionIssue(ctx, "STUDY-2", "2"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchIssue(ctx, "STUDY-2"); err != nil {
		t.Fatal(err)
	}
	children, err := client.EpicChildren(ctx, "STUDY-1")
	if err != nil || len(children) != 2 {
		t.Fatalf("EpicChildren = %v, %v", children, err)
	}

	want := []callRecord{
		{"createJiraIssue", map[string]any{"cloudId": "cloud-1", "projectKey": "STUDY", "summary": "Task", "issueType": "Task", "description": "**Deadline:** Friday", "labels": []string{"assessment"}, "parent": "STUDY-1", "additional_fields": map[string]any{"duedate": "2026-09-11"}}},
		{"editJiraIssue", map[string]any{"cloudId": "cloud-1", "issueIdOrKey": "STUDY-2", "fields": map[string]any{"issuetype": map[string]any{"name": "Milestone"}, "parent": map[string]any{"key": "STUDY-1"}, "summary": "Updated", "description": nil, "duedate": nil, "labels": []string{}}, "contentFormat": "markdown"}},
		{"transitionJiraIssue", map[string]any{"cloudId": "cloud-1", "issueIdOrKey": "STUDY-2", "transitionId": "2"}},
		{"getJiraIssue", map[string]any{"cloudId": "cloud-1", "issueIdOrKey": "STUDY-2", "view": "full", "responseContentFormat": "markdown"}},
		{"searchJiraIssuesUsingJql", map[string]any{"cloudId": "cloud-1", "jql": `parent = "STUDY-1"`, "maxResults": 100, "view": "full", "responseContentFormat": "markdown"}},
		{"searchJiraIssuesUsingJql", map[string]any{"cloudId": "cloud-1", "jql": `parent = "STUDY-1"`, "maxResults": 100, "view": "full", "responseContentFormat": "markdown", "nextPageToken": "page-2"}},
	}
	if !reflect.DeepEqual(caller.calls, want) {
		t.Fatalf("calls = %#v\nwant %#v", caller.calls, want)
	}
}

func TestJiraClientOmitsCreateOptionals(t *testing.T) {
	caller := &fakeJSONCaller{responses: map[string][]any{"createJiraIssue": {map[string]any{"data": map[string]any{"issue": map[string]any{"key": "STUDY-2"}}}}}}
	client, _ := newJiraClient(caller, "cloud-1")
	if _, err := client.CreateIssue(context.Background(), map[string]any{"project": "STUDY", "type": "Task", "parent": "STUDY-1", "summary": "Task"}); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"cloudId": "cloud-1", "projectKey": "STUDY", "summary": "Task", "issueType": "Task", "parent": "STUDY-1"}
	if !reflect.DeepEqual(caller.calls[0].args, want) {
		t.Fatalf("args = %#v", caller.calls[0].args)
	}
}

func TestJiraClientMalformedCreateKeyIsApplied(t *testing.T) {
	caller := &fakeJSONCaller{responses: map[string][]any{"createJiraIssue": {map[string]any{"data": map[string]any{"id": "10001"}}}}}
	client, _ := newJiraClient(caller, "cloud-1")
	_, err := client.CreateIssue(context.Background(), map[string]any{"project": "STUDY", "type": "Task", "parent": "STUDY-1", "summary": "Task"})
	var mutation *MutationError
	if !errors.As(err, &mutation) || mutation.State != WriteApplied {
		t.Fatalf("error = %#v", err)
	}
}

func TestJiraClientToolFailureIsUnknownAndRedacted(t *testing.T) {
	caller := &fakeJSONCaller{errors: map[string]error{"createJiraIssue": errors.New("raw payload secret")}}
	client, _ := newJiraClient(caller, "cloud-1")
	_, err := client.CreateIssue(context.Background(), map[string]any{"project": "STUDY", "type": "Task", "parent": "STUDY-1", "summary": "Task"})
	var mutation *MutationError
	if !errors.As(err, &mutation) || mutation.State != WriteUnknown || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %#v", err)
	}
}

func TestJiraClientRejectsIdentifiersBeforeRemoteAccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*JiraClient) error
	}{
		{"create", func(c *JiraClient) error {
			_, err := c.CreateIssue(context.Background(), map[string]any{"project": "STUDY", "type": "Task", "parent": "../BAD", "summary": "Task"})
			return err
		}},
		{"update", func(c *JiraClient) error {
			return c.UpdateFields(context.Background(), "../BAD", map[string]any{"summary": "Changed"})
		}},
		{"transition", func(c *JiraClient) error { return c.TransitionIssue(context.Background(), "STUDY-2", "") }},
		{"fetch", func(c *JiraClient) error { _, err := c.FetchIssue(context.Background(), "../BAD"); return err }},
		{"children", func(c *JiraClient) error {
			_, err := c.EpicChildren(context.Background(), `BAD" OR project IS NOT EMPTY`)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &fakeJSONCaller{}
			client, _ := newJiraClient(caller, "cloud-1")
			if err := tc.call(client); err == nil {
				t.Fatal("expected validation error")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("remote calls = %#v", caller.calls)
			}
		})
	}
}
