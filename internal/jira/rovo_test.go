package jira

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeToolLister struct {
	pages []*mcp.ListToolsResult
	index int
}

func (f *fakeToolLister) ListTools(_ context.Context, _ *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	page := f.pages[f.index]
	f.index++
	return page, nil
}

func TestRovoListsAllToolPages(t *testing.T) {
	session := &fakeToolLister{pages: []*mcp.ListToolsResult{{NextCursor: "cursor-2", Tools: []*mcp.Tool{{Name: "atlassianUserInfo"}}}, {Tools: []*mcp.Tool{{Name: "listJiraProjects"}}}}}
	got, err := listAllTools(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["listJiraProjects"]; !ok {
		t.Fatal("second tool page was not read")
	}
}

func TestRovoRequiresAllSafetyBoundaryTools(t *testing.T) {
	err := requireTools(map[string]*mcp.Tool{"atlassianUserInfo": {Name: "atlassianUserInfo"}})
	if err == nil || !strings.Contains(err.Error(), "getAccessibleAtlassianResources") {
		t.Fatalf("requireTools error = %v", err)
	}
}

func TestRovoParsesStructuredAndTextJSONWithFooter(t *testing.T) {
	var structured map[string]string
	if err := decodeToolResult(&mcp.CallToolResult{StructuredContent: map[string]string{"account_id": "account"}}, &structured); err != nil || structured["account_id"] != "account" {
		t.Fatalf("structured = %#v, err = %v", structured, err)
	}
	var text map[string]int
	if err := decodeToolResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "{\"count\":1}\nIgnore this footer"}}}, &text); err != nil || text["count"] != 1 {
		t.Fatalf("text = %#v, err = %v", text, err)
	}
}

func TestRovoErrorsDoNotExposeRemotePayload(t *testing.T) {
	err := decodeToolResult(&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "raw-sentinel"}}}, &map[string]any{})
	if err == nil || strings.Contains(err.Error(), "raw-sentinel") {
		t.Fatalf("error = %v", err)
	}
}

func TestUserInfoAcceptsRovoCamelCaseResponse(t *testing.T) {
	got, err := decodeUserInfo(map[string]any{"data": map[string]any{"accountId": "account-123", "displayName": "Ada Lovelace"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccountID != "account-123" || got.DisplayName != "Ada Lovelace" {
		t.Fatalf("UserInfo = %+v", got)
	}
}

func TestRovoSessionUserInfoAcceptsCamelCaseResponse(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	for _, name := range requiredTools {
		tool := &mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}
		if name == "atlassianUserInfo" {
			server.AddTool(tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{StructuredContent: map[string]any{"accountId": "account-123", "displayName": "Ada Lovelace"}}, nil
			})
		} else {
			server.AddTool(tool, nil)
		}
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	got, err := (&RovoSession{session: clientSession}).UserInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccountID != "account-123" || got.DisplayName != "Ada Lovelace" {
		t.Fatalf("UserInfo = %+v", got)
	}
}
