package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/lockfile"
)

type IssueState struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Summary     string   `json:"summary"`
	Status      string   `json:"status"`
	Due         *string  `json:"due"`
	Labels      []string `json:"labels"`
	Description *string  `json:"description"`
	UpdatedAt   *string  `json:"updated_at"`
}

type JiraState struct {
	Version      int          `json:"version"`
	ReconciledAt *string      `json:"reconciled_at"`
	Issues       []IssueState `json:"issues"`
}

type epicProvisioningState struct {
	Version int    `json:"version"`
	Summary string `json:"summary"`
}

// BeginEpicProvisioning records an Epic mutation barrier before a create can
// be attempted. Its boolean result means a prior uncertain attempt must be
// reconciled without creating another Epic.
func BeginEpicProvisioning(root string, course config.Course, summary string) (bool, error) {
	path, err := epicProvisioningPath(root, course)
	if err != nil {
		return false, err
	}
	state, err := readEpicProvisioning(path)
	if err == nil {
		if state.Summary != summary {
			return false, fmt.Errorf("Jira epic provisioning state does not match the selected course")
		}
		return true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := atomicWriteJSON(path, epicProvisioningState{Version: 1, Summary: summary}); err != nil {
		return false, err
	}
	return false, nil
}

// AcquireCourseLock obtains the shared per-course lock used by Jira mutation
// workflows. Callers must hold it across checking and creating an Epic.
func AcquireCourseLock(root string, course config.Course) (*lockfile.Lock, error) {
	cachePath, _, err := statePaths(root, course)
	if err != nil {
		return nil, err
	}
	return lockfile.TryAcquire(filepath.Join(filepath.Dir(cachePath), ".course.lock"))
}

// ClearEpicProvisioning removes the completed Epic mutation barrier.
func ClearEpicProvisioning(root string, course config.Course) error {
	path, err := epicProvisioningPath(root, course)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func epicProvisioningPath(root string, course config.Course) (string, error) {
	cachePath, _, err := statePaths(root, course)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cachePath), "jira-epic.json"), nil
}

func readEpicProvisioning(path string) (epicProvisioningState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return epicProvisioningState{}, err
	}
	var state epicProvisioningState
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil || state.Version != 1 || strings.TrimSpace(state.Summary) == "" {
		return epicProvisioningState{}, fmt.Errorf("Jira epic provisioning state is invalid")
	}
	return state, nil
}

type manifestApplied struct {
	ID      string         `json:"id"`
	Action  string         `json:"action"`
	Target  string         `json:"target"`
	Details map[string]any `json:"details"`
}

type manifestFailure struct {
	ID          string         `json:"id"`
	Action      string         `json:"action,omitempty"`
	Target      *string        `json:"target"`
	Error       string         `json:"error"`
	WriteState  WriteState     `json:"write_state"`
	RetrySafe   bool           `json:"retry_safe"`
	Details     map[string]any `json:"details"`
	ExactAction *Action        `json:"exact_action,omitempty"`
}

type manifestStage struct {
	Status                 string            `json:"status"`
	Applied                []manifestApplied `json:"applied"`
	Failures               []manifestFailure `json:"failures"`
	ReconciliationRequired bool              `json:"reconciliation_required"`
	RetrySafe              bool              `json:"retry_safe"`
	Reconciled             bool              `json:"reconciled"`
}

func statePaths(root string, course config.Course) (string, string, error) {
	if !planIdentifierRE.MatchString(course.Code) || filepath.Base(course.Code) != course.Code || course.Code == "." || course.Code == ".." {
		return "", "", fmt.Errorf("invalid course code")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve vault root: %w", err)
	}
	coursesPath := filepath.Join(rootPath, "courses")
	coursePath := filepath.Join(coursesPath, course.Code)
	resolvedCourse, err := filepath.EvalSymlinks(coursePath)
	if err != nil {
		return "", "", fmt.Errorf("resolve course directory: %w", err)
	}
	if !pathWithin(coursesPath, resolvedCourse) || resolvedCourse == coursesPath {
		return "", "", fmt.Errorf("course directory resolves outside vault")
	}
	info, err := os.Stat(resolvedCourse)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("invalid course directory")
	}
	stateDir := filepath.Join(resolvedCourse, "state")
	if info, err := os.Lstat(stateDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("state directory must not be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", "", err
	}
	return filepath.Join(stateDir, "jira.json"), filepath.Join(stateDir, "latest-run.json"), nil
}

func pathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func atomicWriteJSON(target string, value any) error {
	temporary, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+"-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	clean := true
	defer func() {
		if clean {
			_ = os.Remove(temporaryName)
		}
	}()
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return err
	}
	clean = false
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func writeJiraState(path string, state JiraState) error {
	state.Version = 2
	if state.Issues == nil {
		state.Issues = []IssueState{}
	}
	sort.Slice(state.Issues, func(i, j int) bool { return issueKeyLess(state.Issues[i].Key, state.Issues[j].Key) })
	return atomicWriteJSON(path, state)
}

func readJiraState(path string) (JiraState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return JiraState{}, err
	}
	var state JiraState
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return JiraState{}, err
	}
	if state.Version != 2 {
		return JiraState{}, fmt.Errorf("jira state version must be 2")
	}
	return state, nil
}

func issueKeyLess(a, b string) bool {
	separatorA := strings.LastIndexByte(a, '-')
	separatorB := strings.LastIndexByte(b, '-')
	if separatorA < 0 || separatorB < 0 || a[:separatorA] != b[:separatorB] {
		return a < b
	}
	numberA, _ := strconv.Atoi(a[separatorA+1:])
	numberB, _ := strconv.Atoi(b[separatorB+1:])
	return numberA < numberB
}

func upsertIssue(path string, issue IssueState) error {
	state, err := readJiraState(path)
	if err != nil {
		return err
	}
	updated := false
	for index := range state.Issues {
		if state.Issues[index].Key == issue.Key {
			state.Issues[index] = issue
			updated = true
			break
		}
	}
	if !updated {
		state.Issues = append(state.Issues, issue)
	}
	return writeJiraState(path, state)
}

func readManifestStage(path, course string) (*manifestStage, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, nil, fmt.Errorf("read latest run: %w", err)
	}
	var version int
	if err := json.Unmarshal(manifest["version"], &version); err != nil || version != 2 {
		return nil, nil, fmt.Errorf("latest run version must be 2")
	}
	var manifestCourse string
	if err := json.Unmarshal(manifest["course"], &manifestCourse); err != nil || manifestCourse != course {
		return nil, nil, fmt.Errorf("latest run course does not match selected course")
	}
	var stage manifestStage
	if err := json.Unmarshal(manifest["jira"], &stage); err != nil {
		return nil, nil, fmt.Errorf("read Jira run stage: %w", err)
	}
	return &stage, manifest, nil
}

func persistResult(path string, workspace config.Workspace, course config.Course, result ApplyResult) error {
	stage := stageFromResult(result)
	_, manifest, err := readManifestStage(path, course.Code)
	if err != nil {
		return err
	}
	stageJSON, err := json.Marshal(stage)
	if err != nil {
		return err
	}
	if manifest == nil {
		now := time.Now().In(time.FixedZone("local", 0))
		manifest = map[string]json.RawMessage{}
		values := map[string]any{
			"version": 2, "run_id": now.Format("20060102T150405-0700"), "course": course.Code,
			"effective_features": map[string]bool{"jira": true, "wiki": config.Effective(workspace, course).Wiki},
			"canvas":             map[string]any{"status": "pending", "changes": []any{}, "failures": []any{}},
			"wiki": map[string]any{"status": func() string {
				if config.Effective(workspace, course).Wiki {
					return "pending"
				}
				return "disabled"
			}(), "applied": []any{}, "failures": []any{}, "reconciliation_required": false, "retry_safe": true, "reconciled": false},
		}
		for key, value := range values {
			encoded, _ := json.Marshal(value)
			manifest[key] = encoded
		}
	}
	manifest["jira"] = stageJSON
	return atomicWriteJSON(path, manifest)
}

func stageFromResult(result ApplyResult) manifestStage {
	stage := manifestStage{Status: result.Status, Applied: []manifestApplied{}, Failures: []manifestFailure{}, ReconciliationRequired: result.ReconciliationRequired, RetrySafe: result.RetrySafe, Reconciled: result.Reconciled}
	for _, item := range result.Applied {
		stage.Applied = append(stage.Applied, manifestApplied{ID: item.ID, Action: item.Action, Target: item.Key, Details: map[string]any{"action_index": item.ActionIndex, "key": item.Key}})
	}
	for _, item := range result.Failures {
		var target *string
		if item.Key != "" {
			value := item.Key
			target = &value
		}
		stage.Failures = append(stage.Failures, manifestFailure{ID: item.ID, Action: item.Action, Target: target, Error: item.Error, WriteState: item.WriteState, RetrySafe: item.RetrySafe, Details: map[string]any{"action_index": item.ActionIndex, "phase": item.Phase}, ExactAction: item.ExactAction})
	}
	return stage
}

func resultFromStage(course, epic string, stage manifestStage) (ApplyResult, error) {
	result := ApplyResult{Course: course, Epic: epic, Status: stage.Status, Applied: []AppliedAction{}, Failures: []ActionFailure{}, ReconciliationRequired: stage.ReconciliationRequired, RetrySafe: stage.RetrySafe, Reconciled: stage.Reconciled}
	for _, item := range stage.Applied {
		index, err := detailIndex(item.Details)
		if err != nil {
			return ApplyResult{}, err
		}
		result.Applied = append(result.Applied, AppliedAction{ID: item.ID, ActionIndex: index, Action: item.Action, Key: item.Target})
	}
	for _, item := range stage.Failures {
		index, err := detailIndex(item.Details)
		if err != nil {
			return ApplyResult{}, err
		}
		phase, _ := item.Details["phase"].(string)
		key := ""
		if item.Target != nil {
			key = *item.Target
		}
		result.Failures = append(result.Failures, ActionFailure{ID: item.ID, ActionIndex: index, Action: item.Action, Key: key, Phase: phase, Error: item.Error, WriteState: item.WriteState, RetrySafe: item.RetrySafe, ExactAction: item.ExactAction})
	}
	return result, nil
}

func detailIndex(details map[string]any) (int, error) {
	switch value := details["action_index"].(type) {
	case float64:
		return int(value), nil
	case int:
		return value, nil
	default:
		return 0, fmt.Errorf("latest run Jira evidence is invalid")
	}
}
