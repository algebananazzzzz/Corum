package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var requiredTools = []string{
	"atlassianUserInfo", "getAccessibleAtlassianResources", "listJiraProjects",
	"getJiraIssue", "searchJiraIssuesUsingJql", "createJiraIssue", "editJiraIssue", "transitionJiraIssue",
}

// RovoError deliberately keeps protocol and remote payloads out of CLI output.
type RovoError struct{ message string }

func (e *RovoError) Error() string   { return e.message }
func rovoError(message string) error { return &RovoError{message: message} }

type toolLister interface {
	ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
}

type RovoSession struct {
	session  *mcp.ClientSession
	tools    map[string]*mcp.Tool
	callback *loopbackCallback
}

func (s *RovoSession) Close() error {
	if s == nil {
		return nil
	}
	if s.callback != nil {
		_ = s.callback.Close()
		s.callback = nil
	}
	if s.session == nil {
		return nil
	}
	return s.session.Close()
}

func (s *RovoSession) toolsFor(ctx context.Context) (map[string]*mcp.Tool, error) {
	if s.tools != nil {
		return s.tools, nil
	}
	tools, err := listAllTools(ctx, s.session)
	if err != nil {
		return nil, err
	}
	if err := requireTools(tools); err != nil {
		return nil, err
	}
	s.tools = tools
	return tools, nil
}

func listAllTools(ctx context.Context, session toolLister) (map[string]*mcp.Tool, error) {
	tools := make(map[string]*mcp.Tool)
	var cursor string
	for {
		page, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil || page == nil {
			return nil, rovoError("could not discover Atlassian tools")
		}
		for _, tool := range page.Tools {
			if tool != nil {
				tools[tool.Name] = tool
			}
		}
		if page.NextCursor == "" {
			return tools, nil
		}
		cursor = page.NextCursor
	}
}

func requireTools(tools map[string]*mcp.Tool) error {
	for _, name := range requiredTools {
		if tools[name] == nil {
			return rovoError("required Atlassian tool is unavailable: " + name)
		}
	}
	return nil
}

// CallJSON calls a verified Rovo tool and decodes structured content, or the
// first JSON value in all-text content. Remote error text is never returned.
func (s *RovoSession) CallJSON(ctx context.Context, name string, arguments map[string]any, target any) error {
	tools, err := s.toolsFor(ctx)
	if err != nil {
		return err
	}
	if tools[name] == nil {
		return rovoError("required Atlassian tool is unavailable: " + name)
	}
	result, err := s.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return rovoError("Atlassian tool failed: " + name)
	}
	if err := decodeToolResult(result, target); err != nil {
		return fmt.Errorf("Atlassian tool failed: %s: %w", name, err)
	}
	return nil
}

func decodeToolResult(result *mcp.CallToolResult, target any) error {
	if result == nil || result.IsError {
		return rovoError("Atlassian returned an unsuccessful response")
	}
	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil || json.Unmarshal(encoded, target) != nil {
			return rovoError("Atlassian returned an invalid response")
		}
		return nil
	}
	var text strings.Builder
	for _, content := range result.Content {
		block, ok := content.(*mcp.TextContent)
		if !ok {
			return rovoError("Atlassian returned an unsupported response")
		}
		text.WriteString(block.Text)
	}
	decoder := json.NewDecoder(strings.NewReader(text.String()))
	if err := decoder.Decode(target); err != nil {
		return rovoError("Atlassian returned invalid JSON")
	}
	return nil
}

type UserInfo struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type AtlassianResource struct {
	ID       string           `json:"id"`
	CloudID  string           `json:"cloudId"`
	URL      string           `json:"url"`
	Name     string           `json:"name"`
	Products []map[string]any `json:"products"`
}

type JiraProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (s *RovoSession) UserInfo(ctx context.Context) (UserInfo, error) {
	var raw any
	if err := s.CallJSON(ctx, "atlassianUserInfo", map[string]any{}, &raw); err != nil {
		return UserInfo{}, err
	}
	return decodeUserInfo(raw)
}

func decodeUserInfo(raw any) (UserInfo, error) {
	if object, ok := raw.(map[string]any); ok {
		if data, ok := object["data"]; ok {
			raw = data
		}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return UserInfo{}, rovoError("Atlassian returned invalid account information")
	}
	var wire struct {
		AccountID     string `json:"accountId"`
		DisplayName   string `json:"displayName"`
		LegacyAccount string `json:"account_id"`
		LegacyDisplay string `json:"display_name"`
		Email         string `json:"email"`
	}
	if json.Unmarshal(encoded, &wire) != nil {
		return UserInfo{}, rovoError("Atlassian returned invalid account information")
	}
	if wire.AccountID == "" {
		wire.AccountID = wire.LegacyAccount
	}
	if wire.DisplayName == "" {
		wire.DisplayName = wire.LegacyDisplay
	}
	return UserInfo{AccountID: wire.AccountID, DisplayName: wire.DisplayName, Email: wire.Email}, nil
}

func (s *RovoSession) Resources(ctx context.Context) ([]AtlassianResource, error) {
	var raw any
	if err := s.CallJSON(ctx, "getAccessibleAtlassianResources", map[string]any{}, &raw); err != nil {
		return nil, err
	}
	values, ok := unwrapList(raw, "resources", "values", "results")
	if !ok {
		return nil, rovoError("Atlassian returned invalid site information")
	}
	encoded, _ := json.Marshal(values)
	var resources []AtlassianResource
	if json.Unmarshal(encoded, &resources) != nil {
		return nil, rovoError("Atlassian returned invalid site information")
	}
	for i := range resources {
		if resources[i].ID == "" {
			resources[i].ID = resources[i].CloudID
		}
	}
	sort.Slice(resources, func(i, j int) bool { return strings.ToLower(resources[i].Name) < strings.ToLower(resources[j].Name) })
	return resources, nil
}

func (s *RovoSession) Projects(ctx context.Context, cloudID string) ([]JiraProject, error) {
	var projects []JiraProject
	startAt := 0
	for {
		arguments := map[string]any{"cloudId": cloudID, "maxResults": 100}
		if startAt != 0 {
			arguments["startAt"] = startAt
		}
		var raw any
		if err := s.CallJSON(ctx, "listJiraProjects", arguments, &raw); err != nil {
			return nil, err
		}
		values, ok := unwrapList(raw, "projects", "values", "results")
		if !ok {
			return nil, rovoError("Atlassian returned invalid project information")
		}
		encoded, _ := json.Marshal(values)
		var page []JiraProject
		if json.Unmarshal(encoded, &page) != nil {
			return nil, rovoError("Atlassian returned invalid project information")
		}
		projects = append(projects, page...)
		object, _ := raw.(map[string]any)
		if data, ok := object["data"].(map[string]any); ok {
			object = data
		}
		if object["isLast"] != false {
			break
		}
		if len(page) == 0 {
			return nil, rovoError("Atlassian returned invalid project pagination")
		}
		startAt += len(page)
	}
	sort.Slice(projects, func(i, j int) bool { return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name) })
	return projects, nil
}

func unwrapList(value any, keys ...string) ([]any, bool) {
	for range 3 {
		if list, ok := value.([]any); ok {
			return list, true
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		for _, key := range keys {
			if list, ok := object[key].([]any); ok {
				return list, true
			}
		}
		value = object["data"]
	}
	return nil, false
}
