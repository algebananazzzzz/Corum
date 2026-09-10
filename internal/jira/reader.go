package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type jsonCaller interface {
	CallJSON(context.Context, string, map[string]any, any) error
}

type SyncClient interface {
	FetchIssue(context.Context, string) (map[string]any, error)
	EpicChildren(context.Context, string) ([]map[string]any, error)
}

type JiraClient struct {
	session jsonCaller
	cloudID string
}

func NewJiraClient(session *RovoSession, cloudID string) (*JiraClient, error) {
	if session == nil {
		return nil, &ValidationError{Message: "Jira session is required"}
	}
	if !planIdentifierRE.MatchString(cloudID) {
		return nil, &ValidationError{Message: "Jira cloud ID has invalid syntax"}
	}
	return &JiraClient{session: session, cloudID: cloudID}, nil
}

func (c *JiraClient) FetchIssue(ctx context.Context, key string) (map[string]any, error) {
	if err := validateIssueKey(key, "Jira issue key"); err != nil {
		return nil, err
	}
	var raw any
	if err := c.session.CallJSON(ctx, "getJiraIssue", map[string]any{"cloudId": c.cloudID, "issueIdOrKey": key, "view": "full", "responseContentFormat": "markdown"}, &raw); err != nil {
		return nil, fmt.Errorf("could not fetch Jira issue")
	}
	return responseObject(raw, "Jira issue")
}

func (c *JiraClient) EpicChildren(ctx context.Context, epic string) ([]map[string]any, error) {
	if err := validateIssueKey(epic, "Jira epic issue key"); err != nil {
		return nil, err
	}
	quoted, _ := json.Marshal(epic)
	var issues []map[string]any
	var next string
	seen := map[string]bool{}
	for {
		args := map[string]any{"cloudId": c.cloudID, "jql": "parent = " + string(quoted), "maxResults": 100, "view": "full", "responseContentFormat": "markdown"}
		if next != "" {
			args["nextPageToken"] = next
		}
		var raw any
		if err := c.session.CallJSON(ctx, "searchJiraIssuesUsingJql", args, &raw); err != nil {
			return nil, fmt.Errorf("could not search Jira issues")
		}
		page, err := responseObject(raw, "Jira search result")
		if err != nil {
			return nil, err
		}
		values, ok := page["issues"].([]any)
		if !ok {
			if page["issues"] == nil {
				values = []any{}
				ok = true
			}
			if typed, typedOK := page["issues"].([]map[string]any); typedOK {
				issues = append(issues, typed...)
				values = nil
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("Atlassian returned invalid Jira search results")
		}
		for _, value := range values {
			item, valid := value.(map[string]any)
			if !valid {
				return nil, fmt.Errorf("Atlassian returned invalid Jira search results")
			}
			issues = append(issues, item)
		}
		if last, ok := page["isLast"].(bool); ok && last {
			return issues, nil
		}
		token, ok := page["nextPageToken"].(string)
		if !ok || token == "" {
			return issues, nil
		}
		if seen[token] {
			return nil, fmt.Errorf("Atlassian returned invalid Jira search pagination")
		}
		seen[token] = true
		next = token
	}
}

var planIdentifierRE = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
var issueKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[1-9][0-9]*$`)
var projectKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func validateIssueKey(value, label string) error {
	if !issueKeyRE.MatchString(value) {
		return &ValidationError{Message: label + " is invalid"}
	}
	return nil
}
func validateProjectKey(value, label string) error {
	if !projectKeyRE.MatchString(value) {
		return &ValidationError{Message: label + " is invalid"}
	}
	return nil
}
func validateDate(value, label string) error {
	if len(value) != 10 {
		return &ValidationError{Message: label + " is invalid"}
	}
	return nil
}
func stringSlice(value any, label string) ([]string, error) {
	raw, ok := value.([]any)
	if !ok {
		if typed, valid := value.([]string); valid {
			return typed, nil
		}
		return nil, &ValidationError{Message: label + " must be an array"}
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, valid := item.(string)
		if !valid {
			return nil, &ValidationError{Message: label + " must contain strings"}
		}
		result = append(result, text)
	}
	sort.Strings(result)
	return result, nil
}
func responseObject(raw any, label string) (map[string]any, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Atlassian returned invalid %s", label)
	}
	if nested, exists := object["data"]; exists {
		data, valid := nested.(map[string]any)
		if !valid {
			return nil, fmt.Errorf("Atlassian returned invalid %s", label)
		}
		object = data
	}
	return object, nil
}
func nestedNonblank(value any, key string) (string, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	text, ok := object[key].(string)
	return text, ok && strings.TrimSpace(text) != ""
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
