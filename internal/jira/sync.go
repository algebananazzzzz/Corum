package jira

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/config"
)

// SyncOptions contains dependencies for deterministic epic reconciliation.
// Client is intentionally limited at the call site to the read operations
// used by SyncEpic; it exists here as an interface to keep the sync pure in
// tests and to avoid exposing Jira mutation dispatch.
type SyncOptions struct {
	Client  SyncClient
	Project string
	Now     func() time.Time
}

func normalizeChildren(raw []map[string]any, project, epic string) ([]IssueState, error) {
	if raw == nil {
		return nil, &ValidationError{Message: "Jira epic children response is invalid"}
	}
	seen := map[string]bool{}
	issues := make([]IssueState, 0, len(raw))
	for index, value := range raw {
		issue, err := normalizeRemoteIssue(value, project, epic, true)
		if err != nil {
			return nil, fmt.Errorf("Jira epic child %d: %w", index, err)
		}
		if seen[issue.Key] {
			return nil, &ValidationError{Message: "Jira epic children contain a duplicate key"}
		}
		seen[issue.Key] = true
		issues = append(issues, issue)
	}
	return issues, nil
}

func normalizeRemoteIssue(raw map[string]any, project, epic string, requireParent bool) (IssueState, error) {
	key, ok := raw["key"].(string)
	if !ok {
		return IssueState{}, &ValidationError{Message: "Jira issue response is missing a key"}
	}
	if err := validateIssueKey(key, "Jira issue response key"); err != nil {
		return IssueState{}, err
	}
	if !strings.HasPrefix(key, project+"-") {
		return IssueState{}, &ValidationError{Message: "Jira issue response is outside the configured project"}
	}
	fields, ok := raw["fields"].(map[string]any)
	if !ok {
		return IssueState{}, &ValidationError{Message: "Jira issue response must contain fields"}
	}
	if requireParent {
		parent, ok := fields["parent"].(map[string]any)
		if !ok {
			return IssueState{}, &ValidationError{Message: "Jira child response is missing its parent identifier"}
		}
		parentKey, _ := parent["key"].(string)
		if parentKey != epic {
			return IssueState{}, &ValidationError{Message: "Jira child does not belong to the configured epic"}
		}
	}
	typ, ok := nestedNonblank(fields["issuetype"], "name")
	if !ok {
		return IssueState{}, &ValidationError{Message: "Jira issue response has invalid issue type"}
	}
	summary, ok := fields["summary"].(string)
	if !ok || strings.TrimSpace(summary) == "" {
		return IssueState{}, &ValidationError{Message: "Jira issue response has invalid summary"}
	}
	status, ok := nestedNonblank(fields["status"], "name")
	if !ok {
		return IssueState{}, &ValidationError{Message: "Jira issue response has invalid status"}
	}
	var due *string
	if value := fields["duedate"]; value != nil {
		text, ok := value.(string)
		if !ok {
			return IssueState{}, &ValidationError{Message: "Jira issue response has invalid due date"}
		}
		if err := validateDate(text, "Jira issue response due date"); err != nil {
			return IssueState{}, err
		}
		due = &text
	}
	var labels []string
	if value, exists := fields["labels"]; exists && value != nil {
		var err error
		labels, err = stringSlice(value, "Jira issue response labels")
		if err != nil {
			return IssueState{}, err
		}
	}
	var description *string
	if value, ok := fields["description"].(string); ok {
		description = &value
	}
	var updated *string
	if value := fields["updated"]; value != nil {
		text, ok := value.(string)
		if !ok {
			return IssueState{}, &ValidationError{Message: "Jira issue response has invalid updated timestamp"}
		}
		text = normalizeJiraTimestamp(text)
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return IssueState{}, &ValidationError{Message: "Jira issue response updated timestamp must include an offset"}
		}
		normalized := parsed.Format(time.RFC3339Nano)
		updated = &normalized
	}
	return IssueState{Key: key, Type: typ, Summary: summary, Status: status, Due: due, Labels: labels, Description: description, UpdatedAt: updated}, nil
}

func normalizeJiraTimestamp(value string) string {
	if len(value) >= 5 && (value[len(value)-5] == '+' || value[len(value)-5] == '-') && value[len(value)-3] != ':' {
		return value[:len(value)-2] + ":" + value[len(value)-2:]
	}
	return value
}

type SyncResult struct {
	Course     string `json:"course"`
	Epic       string `json:"epic"`
	IssueCount int    `json:"issue_count"`
	CachePath  string `json:"cache_path"`
}

// SyncEpic fetches a complete epic snapshot and atomically replaces its local
// Jira cache. No cache mutation occurs until every remote response validates.
func SyncEpic(ctx context.Context, root, courseCode, epicKey string, options SyncOptions) (SyncResult, error) {
	if options.Client == nil {
		return SyncResult{}, &ValidationError{Message: "Jira sync client is required"}
	}
	if err := validateIssueKey(epicKey, "Jira epic issue key"); err != nil {
		return SyncResult{}, err
	}
	if err := validateProjectKey(options.Project, "Jira project key"); err != nil {
		return SyncResult{}, err
	}
	course := config.Course{Code: courseCode}
	cachePath, _, err := statePaths(root, course)
	if err != nil {
		return SyncResult{}, err
	}
	rawEpic, err := options.Client.FetchIssue(ctx, epicKey)
	if err != nil {
		return SyncResult{}, fmt.Errorf("could not fetch Jira epic: %w", err)
	}
	normalizedEpic, err := normalizeRemoteIssue(rawEpic, options.Project, epicKey, false)
	if err != nil {
		return SyncResult{}, fmt.Errorf("invalid Jira epic: %w", err)
	}
	if !strings.EqualFold(normalizedEpic.Type, "Epic") {
		return SyncResult{}, &ValidationError{Message: "configured Jira issue is not an Epic"}
	}
	rawChildren, err := options.Client.EpicChildren(ctx, epicKey)
	if err != nil {
		return SyncResult{}, fmt.Errorf("could not fetch Jira epic children: %w", err)
	}
	issues, err := normalizeChildren(rawChildren, options.Project, epicKey)
	if err != nil {
		return SyncResult{}, err
	}
	sort.Slice(issues, func(i, j int) bool { return issueKeyLess(issues[i].Key, issues[j].Key) })
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	reconciledAt := now().Format(time.RFC3339Nano)
	if err := writeJiraState(cachePath, JiraState{Version: 2, ReconciledAt: &reconciledAt, Issues: issues}); err != nil {
		return SyncResult{}, fmt.Errorf("could not write Jira state: %w", err)
	}
	return SyncResult{Course: courseCode, Epic: epicKey, IssueCount: len(issues), CachePath: cachePath}, nil
}
