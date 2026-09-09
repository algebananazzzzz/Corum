package jira

import (
	"context"
	"testing"
)

func TestEpicChildrenIncludesCloudIDOnEverySearchPage(t *testing.T) {
	caller := &recordingJSONCaller{responses: []any{
		map[string]any{
			"issues":        []any{map[string]any{"key": "TODO-2"}},
			"nextPageToken": "page-2",
			"isLast":        false,
		},
		map[string]any{
			"issues": []any{map[string]any{"key": "TODO-3"}},
			"isLast": true,
		},
	}}
	client := &JiraClient{session: caller, cloudID: "cloud-123"}

	issues, err := client.EpicChildren(context.Background(), "TODO-14")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issue count = %d, want 2", len(issues))
	}
	if len(caller.arguments) != 2 {
		t.Fatalf("search call count = %d, want 2", len(caller.arguments))
	}
	for page, arguments := range caller.arguments {
		if arguments["cloudId"] != "cloud-123" {
			t.Fatalf("page %d cloudId = %#v, want cloud-123", page+1, arguments["cloudId"])
		}
	}
	if caller.arguments[1]["nextPageToken"] != "page-2" {
		t.Fatalf("second page token = %#v, want page-2", caller.arguments[1]["nextPageToken"])
	}
}

type recordingJSONCaller struct {
	responses []any
	arguments []map[string]any
}

func (c *recordingJSONCaller) CallJSON(_ context.Context, name string, arguments map[string]any, target any) error {
	if name != "searchJiraIssuesUsingJql" {
		panic("unexpected tool: " + name)
	}
	c.arguments = append(c.arguments, arguments)
	*(target.(*any)) = c.responses[len(c.arguments)-1]
	return nil
}
