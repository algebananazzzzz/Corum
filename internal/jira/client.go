package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type jsonCaller interface {
	CallJSON(context.Context, string, map[string]any, any) error
}

type Client interface {
	CreateIssue(context.Context, map[string]any) (string, error)
	UpdateFields(context.Context, string, map[string]any) error
	TransitionIssue(context.Context, string, string) error
	FetchIssue(context.Context, string) (map[string]any, error)
	EpicChildren(context.Context, string) ([]map[string]any, error)
}

type JiraClient struct {
	session jsonCaller
	cloudID string
}

func NewJiraClient(session *RovoSession, cloudID string) (*JiraClient, error) {
	return newJiraClient(session, cloudID)
}

func newJiraClient(session jsonCaller, cloudID string) (*JiraClient, error) {
	if session == nil {
		return nil, &ValidationError{Message: "Jira session is required"}
	}
	if !planIdentifierRE.MatchString(cloudID) {
		return nil, &ValidationError{Message: "Jira cloud ID has invalid syntax"}
	}
	return &JiraClient{session: session, cloudID: cloudID}, nil
}

func (c *JiraClient) CreateIssue(ctx context.Context, fields map[string]any) (string, error) {
	if err := validateCreateMap(fields); err != nil {
		return "", err
	}
	project := fields["project"].(string)
	arguments := map[string]any{"cloudId": c.cloudID, "projectKey": project, "summary": fields["summary"], "issueType": fields["type"]}
	for _, name := range []string{"description", "labels"} {
		if value, ok := fields[name]; ok && value != nil {
			arguments[name] = value
		}
	}
	if value, ok := fields["parent"]; ok && value != nil {
		arguments["parent"] = value
	}
	if value, ok := fields["due"]; ok && value != nil {
		arguments["additional_fields"] = map[string]any{"duedate": value}
	}
	var raw any
	if err := c.session.CallJSON(ctx, "createJiraIssue", arguments, &raw); err != nil {
		return "", &MutationError{Message: "Jira create outcome is unknown", State: WriteUnknown}
	}
	data, err := responseObject(raw, "created Jira issue")
	if err != nil {
		return "", &MutationError{Message: "successful Jira create response is missing a valid issue key", State: WriteApplied}
	}
	key, _ := data["key"].(string)
	if key == "" {
		if issue, ok := data["issue"].(map[string]any); ok {
			key, _ = issue["key"].(string)
		}
	}
	if err := validateIssueKey(key, "created Jira issue key"); err != nil {
		return "", &MutationError{Message: "successful Jira create response is missing a valid issue key", State: WriteApplied}
	}
	return key, nil
}

func (c *JiraClient) UpdateFields(ctx context.Context, key string, fields map[string]any) error {
	if err := validateIssueKey(key, "Jira issue key"); err != nil {
		return err
	}
	if fields == nil || len(fields) == 0 {
		return &ValidationError{Message: "Jira update fields cannot be empty"}
	}
	if err := validateUpdateFields(fields); err != nil {
		return err
	}
	mapped := make(map[string]any, len(fields))
	for name, value := range fields {
		switch name {
		case "type":
			mapped["issuetype"] = map[string]any{"name": value}
		case "parent":
			if value == nil {
				mapped[name] = nil
			} else {
				mapped[name] = map[string]any{"key": value}
			}
		case "due":
			mapped["duedate"] = value
		default:
			mapped[name] = value
		}
	}
	arguments := map[string]any{"cloudId": c.cloudID, "issueIdOrKey": key, "fields": mapped}
	if _, exists := fields["description"]; exists {
		arguments["contentFormat"] = "markdown"
	}
	var raw any
	if err := c.session.CallJSON(ctx, "editJiraIssue", arguments, &raw); err != nil {
		return &MutationError{Message: "Jira update outcome is unknown", State: WriteUnknown}
	}
	if _, err := responseObject(raw, "updated Jira issue"); err != nil {
		return &MutationError{Message: "successful Jira update response is invalid", State: WriteApplied}
	}
	return nil
}

func (c *JiraClient) TransitionIssue(ctx context.Context, key, transition string) error {
	if err := validateIssueKey(key, "Jira issue key"); err != nil {
		return err
	}
	if err := validateTransitionID(transition); err != nil {
		return err
	}
	var raw any
	arguments := map[string]any{"cloudId": c.cloudID, "issueIdOrKey": key, "transitionId": transition}
	if err := c.session.CallJSON(ctx, "transitionJiraIssue", arguments, &raw); err != nil {
		return &MutationError{Message: "Jira transition outcome is unknown", State: WriteUnknown}
	}
	if _, err := responseObject(raw, "transitioned Jira issue"); err != nil {
		return &MutationError{Message: "successful Jira transition response is invalid", State: WriteApplied}
	}
	return nil
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
		arguments := map[string]any{"cloudId": c.cloudID, "jql": "parent = " + string(quoted), "maxResults": 100, "view": "full", "responseContentFormat": "markdown"}
		if next != "" {
			arguments["nextPageToken"] = next
		}
		var raw any
		if err := c.session.CallJSON(ctx, "searchJiraIssuesUsingJql", arguments, &raw); err != nil {
			return nil, fmt.Errorf("could not search Jira issues")
		}
		page, err := responseObject(raw, "Jira search result")
		if err != nil {
			return nil, err
		}
		values, ok := page["issues"].([]any)
		if !ok {
			if typed, typedOK := page["issues"].([]map[string]any); typedOK {
				for _, item := range typed {
					issues = append(issues, item)
				}
				values = nil
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("Atlassian returned invalid Jira search results")
		}
		for _, value := range values {
			item, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Atlassian returned invalid Jira search results")
			}
			issues = append(issues, item)
		}
		if isLast, ok := page["isLast"].(bool); ok && isLast {
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

func validateCreateMap(fields map[string]any) error {
	if fields == nil {
		return &ValidationError{Message: "Jira create fields are required"}
	}
	for name := range fields {
		switch name {
		case "project", "type", "parent", "summary", "description", "due", "labels":
		default:
			return &ValidationError{Message: "unsupported Jira field: " + name}
		}
	}
	project, ok := fields["project"].(string)
	if !ok {
		return &ValidationError{Message: "Jira project is required"}
	}
	if err := validateProjectKey(project, "Jira project key"); err != nil {
		return err
	}
	typeName, ok := fields["type"].(string)
	if !ok || !validIssueType(typeName) {
		return &ValidationError{Message: "unsupported Jira issue type"}
	}
	parent, ok := fields["parent"].(string)
	if !ok {
		return &ValidationError{Message: "Jira parent is required"}
	}
	if err := validateIssueKey(parent, "Jira parent key"); err != nil {
		return err
	}
	summary, ok := fields["summary"].(string)
	if !ok || strings.TrimSpace(summary) == "" {
		return &ValidationError{Message: "Jira summary is required"}
	}
	if value, exists := fields["description"]; exists && value != nil {
		if _, ok := value.(string); !ok {
			return &ValidationError{Message: "Jira description must be a string"}
		}
	}
	if value, exists := fields["due"]; exists && value != nil {
		text, ok := value.(string)
		if !ok {
			return &ValidationError{Message: "Jira due date must be a string"}
		}
		if err := validateDate(text, "Jira due date"); err != nil {
			return err
		}
	}
	if value, exists := fields["labels"]; exists && value != nil {
		if _, err := stringSlice(value, "Jira labels"); err != nil {
			return err
		}
	}
	return nil
}

func responseObject(raw any, label string) (map[string]any, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Atlassian returned invalid %s", label)
	}
	if nested, exists := object["data"]; exists {
		data, ok := nested.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Atlassian returned invalid %s", label)
		}
		object = data
	}
	return object, nil
}
