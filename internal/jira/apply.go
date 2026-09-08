package jira

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/lockfile"
)

func Apply(ctx context.Context, root string, workspace config.Workspace, course config.Course, plan Plan, client Client, dryRun bool) (ApplyResult, error) {
	jiraConfig, err := validateConfiguration(workspace, course, plan, !dryRun)
	if err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{Course: course.Code, Epic: plan.Epic, DryRun: dryRun, Applied: []AppliedAction{}, Failures: []ActionFailure{}, RetrySafe: true}
	if dryRun {
		result.Status = "pending"
		return result, nil
	}
	if client == nil {
		return ApplyResult{}, &ValidationError{Message: "Jira client is required"}
	}
	cachePath, manifestPath, err := statePaths(root, course)
	if err != nil {
		return ApplyResult{}, err
	}
	courseLock, err := lockfile.TryAcquire(filepath.Join(filepath.Dir(cachePath), ".course.lock"))
	if err != nil {
		return ApplyResult{}, fmt.Errorf("course %s: %w", course.Code, err)
	}
	defer courseLock.Close()
	previous, _, err := readManifestStage(manifestPath, course.Code)
	if err != nil {
		return ApplyResult{}, err
	}
	blocked := previous != nil && previous.ReconciliationRequired
	if len(plan.Actions) > 0 && blocked {
		return ApplyResult{}, fmt.Errorf("%w; apply an exact empty plan first", ErrReconciliationRequired)
	}
	if len(plan.Actions) == 0 {
		if blocked {
			if err := reconcile(ctx, cachePath, jiraConfig.Project, course, plan.Epic, client); err != nil {
				return ApplyResult{}, err
			}
			preserved, err := resultFromStage(course.Code, plan.Epic, *previous)
			if err != nil {
				return ApplyResult{}, err
			}
			preserved.ReconciliationRequired = false
			preserved.Reconciled = true
			if err := persistResult(manifestPath, workspace, course, preserved); err != nil {
				return ApplyResult{}, err
			}
			return preserved, nil
		}
		if previous != nil && previous.Reconciled && !previous.RetrySafe {
			return resultFromStage(course.Code, plan.Epic, *previous)
		}
		if _, err := readJiraState(cachePath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return ApplyResult{}, err
			}
			if err := reconcile(ctx, cachePath, jiraConfig.Project, course, plan.Epic, client); err != nil {
				return ApplyResult{}, err
			}
			result.Reconciled = true
		}
		result.Status = "up_to_date"
		if err := persistResult(manifestPath, workspace, course, result); err != nil {
			return ApplyResult{}, err
		}
		return result, nil
	}

	children, err := client.EpicChildren(ctx, plan.Epic)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("could not verify Jira epic children")
	}
	normalized, err := normalizeChildren(children, jiraConfig.Project, plan.Epic)
	if err != nil {
		return ApplyResult{}, err
	}
	owned := map[string]bool{}
	for _, issue := range normalized {
		owned[issue.Key] = true
	}
	var outside []string
	for _, action := range plan.Actions {
		if (action.Action == "update" || action.Action == "transition") && !owned[action.Key] {
			outside = append(outside, action.Key)
		}
	}
	if len(outside) > 0 {
		sort.Strings(outside)
		return ApplyResult{}, &ValidationError{Message: fmt.Sprintf("Jira action target(s) do not belong to configured epic %s: %s", plan.Epic, strings.Join(outside, ", "))}
	}
	now := time.Now().Format(time.RFC3339Nano)
	if err := writeJiraState(cachePath, JiraState{Version: 2, ReconciledAt: &now, Issues: normalized}); err != nil {
		return ApplyResult{}, err
	}

	for index, action := range plan.Actions {
		key := ""
		if action.Action != "create" {
			key = action.Key
		}
		if err := persistInFlight(manifestPath, workspace, course, plan, result, index, action, key); err != nil {
			return result, fmt.Errorf("persist Jira mutation barrier: %w", err)
		}
		var mutationErr error
		switch action.Action {
		case "create":
			key, mutationErr = client.CreateIssue(ctx, createFieldsMap(jiraConfig.Project, *action.Issue))
			if mutationErr == nil {
				if err := validateIssueKey(key, "created Jira issue key"); err != nil {
					mutationErr = &MutationError{Message: "successful Jira create response is missing a valid issue key", State: WriteApplied}
					key = ""
				}
			}
		case "update":
			key = action.Key
			mutationErr = client.UpdateFields(ctx, key, action.Set)
		case "transition":
			key = action.Key
			mutationErr = client.TransitionIssue(ctx, key, jiraConfig.Transitions[action.Transition])
		}
		if mutationErr != nil {
			return failedApply(workspace, course, plan, result, index, action, key, "mutation", mutationErr, manifestPath)
		}
		result.Applied = append(result.Applied, AppliedAction{ID: fmt.Sprintf("jira:%d:%s:%s", index, action.Action, key), ActionIndex: index, Action: action.Action, Key: key})
		rawIssue, fetchErr := client.FetchIssue(ctx, key)
		if fetchErr != nil {
			return failedApply(workspace, course, plan, result, index, action, key, "fetch", &MutationError{Message: "Jira fetch failed after an applied mutation", State: WriteApplied}, manifestPath)
		}
		issue, normalizeErr := normalizeRemoteIssue(rawIssue, jiraConfig.Project, plan.Epic, true)
		if normalizeErr != nil {
			return failedApply(workspace, course, plan, result, index, action, key, "fetch", &MutationError{Message: normalizeErr.Error(), State: WriteApplied}, manifestPath)
		}
		if issue.Key != key {
			return failedApply(workspace, course, plan, result, index, action, key, "fetch", &MutationError{Message: "Jira fetch returned a different issue key", State: WriteApplied}, manifestPath)
		}
		if cacheErr := upsertIssue(cachePath, issue); cacheErr != nil {
			return failedApply(workspace, course, plan, result, index, action, key, "cache", &MutationError{Message: cacheErr.Error(), State: WriteApplied}, manifestPath)
		}
		if index < len(plan.Actions)-1 {
			checkpoint := result
			checkpoint.Status = "partial"
			checkpoint.Failures = []ActionFailure{}
			checkpoint.ReconciliationRequired = true
			checkpoint.RetrySafe = false
			checkpoint.Reconciled = false
			if err := persistResult(manifestPath, workspace, course, checkpoint); err != nil {
				result.ReconciliationRequired = true
				result.RetrySafe = false
				return result, fmt.Errorf("persist Jira action checkpoint: %w", err)
			}
		}
	}
	result.Status = "applied"
	result.Reconciled = true
	if err := persistResult(manifestPath, workspace, course, result); err != nil {
		return result, err
	}
	return result, nil
}

func reconcile(ctx context.Context, cachePath, project string, course config.Course, epic string, client Client) error {
	rawEpic, err := client.FetchIssue(ctx, epic)
	if err != nil {
		return fmt.Errorf("could not reconcile configured Jira epic")
	}
	normalizedEpic, err := normalizeRemoteIssue(rawEpic, project, epic, false)
	if err != nil {
		return err
	}
	if normalizedEpic.Key != epic {
		return &ValidationError{Message: "configured Jira epic response has the wrong key"}
	}
	rawChildren, err := client.EpicChildren(ctx, epic)
	if err != nil {
		return fmt.Errorf("could not reconcile Jira epic children")
	}
	children, err := normalizeChildren(rawChildren, project, epic)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339Nano)
	return writeJiraState(cachePath, JiraState{Version: 2, ReconciledAt: &now, Issues: children})
}

func normalizeChildren(raw []map[string]any, project, epic string) ([]IssueState, error) {
	if raw == nil {
		return nil, &ValidationError{Message: "Jira epic children response is invalid"}
	}
	result := make([]IssueState, 0, len(raw))
	seen := map[string]bool{}
	for index, value := range raw {
		issue, err := normalizeRemoteIssue(value, project, epic, true)
		if err != nil {
			return nil, fmt.Errorf("Jira epic child %d: %w", index, err)
		}
		if seen[issue.Key] {
			return nil, &ValidationError{Message: "Jira epic children contain a duplicate key"}
		}
		seen[issue.Key] = true
		result = append(result, issue)
	}
	return result, nil
}

// normalizeJiraTimestamp rewrites Jira's compact numeric offset (+0800) into
// RFC 3339 form (+08:00) so time.Parse can read it.
func normalizeJiraTimestamp(value string) string {
	if len(value) >= 5 && (value[len(value)-4] == '+' || value[len(value)-4] == '-') && !strings.Contains(value, ":") {
		return value[:len(value)-4] + ":" + value[len(value)-3:]
	}
	return value
}

func normalizeRemoteIssue(raw map[string]any, project, epic string, requireParent bool) (IssueState, error) {
	key, ok := raw["key"].(string)
	if !ok {
		return IssueState{}, &ValidationError{Message: "Jira issue response is missing a key"}
	}
	if err := validateIssueKey(key, "Jira issue response key"); err != nil {
		return IssueState{}, err
	}
	separator := strings.LastIndexByte(key, '-')
	if separator < 1 || key[:separator] != project {
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
		parentKey, ok := parent["key"].(string)
		if !ok || parentKey != epic {
			return IssueState{}, &ValidationError{Message: "Jira child does not belong to the configured epic"}
		}
	}
	issueType, ok := nestedNonblank(fields["issuetype"], "name")
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
	if value, exists := fields["duedate"]; exists && value != nil {
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
	var err error
	if value, exists := fields["labels"]; exists && value != nil {
		labels, err = stringSlice(value, "Jira issue response labels")
		if err != nil {
			return IssueState{}, err
		}
		sort.Strings(labels)
		labels = deduplicate(labels)
	}
	description, err := descriptionMarkdown(fields["description"])
	if err != nil {
		return IssueState{}, err
	}
	var updated *string
	if value, exists := fields["updated"]; exists && value != nil {
		text, ok := value.(string)
		if !ok {
			return IssueState{}, &ValidationError{Message: "Jira issue response has invalid updated timestamp"}
		}
		text = normalizeJiraTimestamp(text)
		parsed, parseErr := time.Parse(time.RFC3339Nano, text)
		if parseErr != nil {
			return IssueState{}, &ValidationError{Message: "Jira issue response updated timestamp must include an offset"}
		}
		normalized := parsed.Format(time.RFC3339Nano)
		updated = &normalized
	}
	return IssueState{Key: key, Type: issueType, Summary: summary, Status: status, Due: due, Labels: labels, Description: description, UpdatedAt: updated}, nil
}

func nestedNonblank(value any, key string) (string, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	text, ok := object[key].(string)
	return text, ok && strings.TrimSpace(text) != ""
}

func deduplicate(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func descriptionMarkdown(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if text, ok := value.(string); ok {
		return &text, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, &ValidationError{Message: "Jira description must be a string, object, or null"}
	}
	content, ok := object["content"].([]any)
	if !ok {
		return nil, &ValidationError{Message: "Jira description document is invalid"}
	}
	blocks := make([]string, 0, len(content))
	for _, block := range content {
		blocks = append(blocks, renderADF(block))
	}
	text := strings.Join(blocks, "\n")
	return &text, nil
}

func renderADF(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if object["type"] == "text" {
		text, _ := object["text"].(string)
		if marks, ok := object["marks"].([]any); ok {
			for _, markValue := range marks {
				mark, ok := markValue.(map[string]any)
				if !ok {
					continue
				}
				switch mark["type"] {
				case "link":
					if attrs, ok := mark["attrs"].(map[string]any); ok {
						if href, ok := attrs["href"].(string); ok && href != "" {
							text = "[" + text + "](" + href + ")"
						}
					}
				case "strong":
					text = "**" + text + "**"
				}
			}
		}
		return text
	}
	content, _ := object["content"].([]any)
	var builder strings.Builder
	for _, child := range content {
		builder.WriteString(renderADF(child))
	}
	return builder.String()
}

func createFieldsMap(project string, fields IssueFields) map[string]any {
	result := map[string]any{"project": project, "type": fields.Type, "parent": fields.Parent, "summary": fields.Summary}
	if fields.Description != nil {
		result["description"] = *fields.Description
	}
	if fields.Due != nil {
		result["due"] = *fields.Due
	}
	if fields.Labels != nil {
		result["labels"] = *fields.Labels
	}
	return result
}

func failedApply(workspace config.Workspace, course config.Course, plan Plan, result ApplyResult, index int, action Action, key, phase string, cause error, manifestPath string) (ApplyResult, error) {
	state := classifyWrite(cause)
	retrySafe := state == WriteNotApplied && len(result.Applied) == 0
	actionCopy := action
	failure := ActionFailure{ID: fmt.Sprintf("jira:%d:%s:%s:%s", index, action.Action, func() string {
		if key == "" {
			return "unknown"
		}
		return key
	}(), phase), ActionIndex: index, Action: action.Action, Key: key, Phase: phase, Error: safeFailureMessage(cause), WriteState: state, RetrySafe: state == WriteNotApplied, ExactAction: &actionCopy}
	result.Status = "failed"
	if len(result.Applied) > 0 {
		result.Status = "partial"
	}
	result.Failures = []ActionFailure{failure}
	result.ReconciliationRequired = state != WriteNotApplied || len(result.Applied) > 0
	result.RetrySafe = retrySafe
	persistErr := persistResult(manifestPath, workspace, course, result)
	applyErr := fmt.Errorf("Jira action %d %s failed", index, phase)
	if persistErr != nil {
		return result, errors.Join(applyErr, fmt.Errorf("persist Jira failure evidence: %w", persistErr))
	}
	return result, applyErr
}

func persistInFlight(manifestPath string, workspace config.Workspace, course config.Course, plan Plan, result ApplyResult, index int, action Action, key string) error {
	actionCopy := action
	pending := result
	pending.Status = "failed"
	if len(result.Applied) > 0 {
		pending.Status = "partial"
	}
	pending.Failures = []ActionFailure{{
		ID:          fmt.Sprintf("jira:%d:%s:%s:mutation_in_flight", index, action.Action, firstNonempty(key, "unknown")),
		ActionIndex: index,
		Action:      action.Action,
		Key:         key,
		Phase:       "mutation_in_flight",
		Error:       "Jira mutation outcome is unknown",
		WriteState:  WriteUnknown,
		RetrySafe:   false,
		ExactAction: &actionCopy,
	}}
	pending.ReconciliationRequired = true
	pending.RetrySafe = false
	pending.Reconciled = false
	return persistResult(manifestPath, workspace, course, pending)
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func safeFailureMessage(err error) string {
	var mutation *MutationError
	if errors.As(err, &mutation) {
		return mutation.Error()
	}
	var validation *ValidationError
	if errors.As(err, &validation) {
		return validation.Error()
	}
	return "Jira mutation outcome is unknown"
}

func classifyWrite(err error) WriteState {
	var mutation *MutationError
	if errors.As(err, &mutation) {
		if mutation.State == WriteNotApplied || mutation.State == WriteApplied || mutation.State == WriteUnknown {
			return mutation.State
		}
		return WriteUnknown
	}
	var validation *ValidationError
	if errors.As(err, &validation) {
		return WriteNotApplied
	}
	return WriteUnknown
}
