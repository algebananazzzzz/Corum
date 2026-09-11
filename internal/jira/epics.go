package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Epic struct {
	Key   string `json:"key"`
	Title string
}

func (s *RovoSession) Epics(ctx context.Context, cloud, project string) ([]Epic, error) {
	if err := validateProjectKey(project, "Jira project"); err != nil {
		return nil, err
	}
	result := []Epic{}
	token := ""
	seen := map[string]bool{}
	for {
		args := map[string]any{"cloudId": cloud, "jql": "project = " + project + " AND issuetype = Epic ORDER BY key", "maxResults": 100, "fields": []string{"summary"}}
		if token != "" {
			args["nextPageToken"] = token
		}
		var raw map[string]any
		if err := s.CallJSON(ctx, "searchJiraIssuesUsingJql", args, &raw); err != nil {
			return nil, err
		}
		object, err := responseObject(raw, "epic search")
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(object)
		if err != nil {
			return nil, err
		}
		var page struct {
			Issues []struct {
				Key    string `json:"key"`
				Fields struct {
					Summary string `json:"summary"`
				} `json:"fields"`
			} `json:"issues"`
			Next string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		for _, issue := range page.Issues {
			if err := validateIssueKey(issue.Key, "epic key"); err != nil {
				return nil, err
			}
			result = append(result, Epic{Key: issue.Key, Title: issue.Fields.Summary})
		}
		if page.Next == "" {
			return result, nil
		}
		if seen[page.Next] {
			return nil, fmt.Errorf("repeated Jira pagination")
		}
		seen[page.Next] = true
		token = page.Next
	}
}

func (s *RovoSession) CreateEpic(ctx context.Context, cloud, project, title string) (Epic, error) {
	if err := validateProjectKey(project, "Jira project"); err != nil {
		return Epic{}, err
	}
	if strings.TrimSpace(title) == "" {
		return Epic{}, fmt.Errorf("epic name is required")
	}
	var raw map[string]any
	if err := s.CallJSON(ctx, "createJiraIssue", map[string]any{"cloudId": cloud, "projectKey": project, "issueTypeName": "Epic", "summary": title}, &raw); err != nil {
		return Epic{}, err
	}
	value, err := responseObject(raw, "created epic")
	if err != nil {
		return Epic{}, err
	}
	key, _ := value["key"].(string)
	if err := validateIssueKey(key, "created epic key"); err != nil {
		return Epic{}, fmt.Errorf("Jira did not return an epic key; inspect the project before retrying")
	}
	return Epic{Key: key, Title: title}, nil
}
