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
	Client  Client
	Project string
	Now     func() time.Time
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
