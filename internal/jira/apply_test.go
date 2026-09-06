package jira

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/lockfile"
)

func validConfig() (config.Workspace, config.Course) {
	return config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "AY2026/27 Semester 1"}, Jira: &config.JiraWorkspace{CloudID: "cloud-1", Project: "STUDY", Transitions: map[string]string{"this_week": "2"}}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term_Calendar.md"}}, config.Course{Version: 2, Code: "CS3103", Jira: &config.JiraCourse{Epic: "STUDY-1"}}
}

func validPlan() Plan {
	return Plan{Version: 2, Course: "CS3103", Epic: "STUDY-1", Actions: []Action{}}
}

func TestPlanDecodeIsStrictVersionTwo(t *testing.T) {
	valid := `{"version":2,"course":"CS3103","epic":"STUDY-1","actions":[{"action":"update","key":"STUDY-2","set":{"due":null}}]}`
	plan, err := DecodePlan(strings.NewReader(valid))
	if err != nil || plan.Version != 2 {
		t.Fatalf("DecodePlan = %+v, %v", plan, err)
	}
	for _, raw := range []string{
		`{"version":1,"course":"CS3103","epic":"STUDY-1","actions":[]}`,
		`{"version":2,"course":"CS3103","epic":"STUDY-1","actions":[],"extra":true}`,
		`{"version":2,"course":"CS3103","epic":"STUDY-1","actions":[]} {}`,
		`{"version":2,"course":"CS3103","epic":"STUDY-1","actions":[{"action":"update","key":"STUDY-2","set":{}}]}`,
		`{"version":2,"course":"CS3103","epic":"STUDY-1","actions":[{"action":"update","key":"STUDY-2","set":{"summary":null}}]}`,
	} {
		if _, err := DecodePlan(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

type panicClient struct{}

func (*panicClient) CreateIssue(context.Context, map[string]any) (string, error) {
	panic("client called")
}
func (*panicClient) UpdateFields(context.Context, string, map[string]any) error {
	panic("client called")
}
func (*panicClient) TransitionIssue(context.Context, string, string) error { panic("client called") }
func (*panicClient) FetchIssue(context.Context, string) (map[string]any, error) {
	panic("client called")
}
func (*panicClient) EpicChildren(context.Context, string) ([]map[string]any, error) {
	panic("client called")
}

func TestDryRunNeverCallsClientOrWrites(t *testing.T) {
	root := t.TempDir()
	workspace, course := validConfig()
	plan := validPlan()
	plan.Actions = []Action{{Action: "create", Issue: &IssueFields{Type: "Task", Parent: "STUDY-1", Summary: "Task"}}}
	result, err := Apply(context.Background(), root, workspace, course, plan, &panicClient{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Status != "pending" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "courses", "CS3103", "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state exists: %v", err)
	}
}

func TestPlanMatchesCourseEpicParentsAndTransitionsBeforeRemote(t *testing.T) {
	workspace, course := validConfig()
	root := t.TempDir()
	for _, mutate := range []func(*Plan){
		func(p *Plan) { p.Course = "OTHER" },
		func(p *Plan) { p.Epic = "OTHER-1" },
		func(p *Plan) {
			p.Actions = []Action{{Action: "create", Issue: &IssueFields{Type: "Task", Parent: "OTHER-1", Summary: "Task"}}}
		},
		func(p *Plan) { p.Actions = []Action{{Action: "transition", Key: "STUDY-2", Transition: "unknown"}} },
	} {
		plan := validPlan()
		mutate(&plan)
		if _, err := Apply(context.Background(), root, workspace, course, plan, &panicClient{}, false); err == nil {
			t.Fatalf("accepted %+v", plan)
		}
	}
}

type recordingClient struct {
	events          []string
	children        []map[string]any
	createKey       string
	createError     error
	updateError     error
	transitionError error
	fetchError      map[string]error
}

func (c *recordingClient) CreateIssue(_ context.Context, _ map[string]any) (string, error) {
	c.events = append(c.events, "create")
	if c.createError != nil {
		return "", c.createError
	}
	if c.createKey == "" {
		return "STUDY-4", nil
	}
	return c.createKey, nil
}
func (c *recordingClient) UpdateFields(_ context.Context, key string, _ map[string]any) error {
	c.events = append(c.events, "update:"+key)
	return c.updateError
}
func (c *recordingClient) TransitionIssue(_ context.Context, key, _ string) error {
	c.events = append(c.events, "transition:"+key)
	return c.transitionError
}
func (c *recordingClient) FetchIssue(_ context.Context, key string) (map[string]any, error) {
	c.events = append(c.events, "fetch:"+key)
	if err := c.fetchError[key]; err != nil {
		return nil, err
	}
	if key == "STUDY-1" {
		issue := remoteIssue(key)
		issue["fields"].(map[string]any)["issuetype"] = map[string]any{"name": "Epic"}
		return issue, nil
	}
	issue := ownedRemoteIssue(key, "STUDY-1")
	return issue, nil
}
func (c *recordingClient) EpicChildren(_ context.Context, epic string) ([]map[string]any, error) {
	c.events = append(c.events, "children:"+epic)
	if c.children == nil {
		return []map[string]any{ownedRemoteIssue("STUDY-2", epic), ownedRemoteIssue("STUDY-3", epic)}, nil
	}
	return c.children, nil
}

func ownedRemoteIssue(key, epic string) map[string]any {
	issue := remoteIssue(key)
	issue["fields"].(map[string]any)["parent"] = map[string]any{"key": epic}
	return issue
}

func prepareRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "courses", "CS3103"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func createPlan() Plan {
	plan := validPlan()
	plan.Actions = []Action{{Action: "create", Issue: &IssueFields{Type: "Task", Parent: "STUDY-1", Summary: "Task"}}}
	return plan
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestApplyRunsActionsSequentiallyAndUpdatesCache(t *testing.T) {
	root := prepareRoot(t)
	workspace, course := validConfig()
	plan := validPlan()
	plan.Actions = []Action{
		{Action: "create", Issue: &IssueFields{Type: "Task", Parent: "STUDY-1", Summary: "New"}},
		{Action: "update", Key: "STUDY-2", Set: map[string]any{"summary": "Changed"}},
		{Action: "transition", Key: "STUDY-3", Transition: "this_week"},
	}
	client := &recordingClient{createKey: "STUDY-4"}
	result, err := Apply(context.Background(), root, workspace, course, plan, client, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"children:STUDY-1", "create", "fetch:STUDY-4", "update:STUDY-2", "fetch:STUDY-2", "transition:STUDY-3", "fetch:STUDY-3"}
	if strings.Join(client.events, "|") != strings.Join(want, "|") {
		t.Fatalf("events=%v", client.events)
	}
	if result.Status != "applied" || len(result.Applied) != 3 || !result.Reconciled {
		t.Fatalf("result=%+v", result)
	}
	state := readJSONMap(t, filepath.Join(root, "courses", "CS3103", "state", "jira.json"))
	if state["version"] != float64(2) || len(state["issues"].([]any)) != 3 {
		t.Fatalf("state=%v", state)
	}
}

func TestApplyValidationFailureIsNotAppliedAndPersisted(t *testing.T) {
	root := prepareRoot(t)
	workspace, course := validConfig()
	plan := validPlan()
	plan.Actions = []Action{{Action: "update", Key: "STUDY-2", Set: map[string]any{"summary": "Changed"}}}
	client := &recordingClient{updateError: &ValidationError{Message: "rejected before write"}}
	result, err := Apply(context.Background(), root, workspace, course, plan, client, false)
	if err == nil {
		t.Fatal("expected apply error")
	}
	if result.Status != "failed" || result.Failures[0].WriteState != WriteNotApplied || !result.RetrySafe || result.ReconciliationRequired {
		t.Fatalf("result=%+v", result)
	}
	manifest := readJSONMap(t, filepath.Join(root, "courses", "CS3103", "state", "latest-run.json"))
	stage := manifest["jira"].(map[string]any)
	if stage["failures"].([]any)[0].(map[string]any)["write_state"] != "not_applied" {
		t.Fatalf("manifest=%v", manifest)
	}
}

func TestUnknownWriteBlocksNextNonemptyPlan(t *testing.T) {
	root := prepareRoot(t)
	workspace, course := validConfig()
	client := &recordingClient{createError: &MutationError{Message: "outcome unknown", State: WriteUnknown}}
	result, err := Apply(context.Background(), root, workspace, course, createPlan(), client, false)
	if err == nil || !result.ReconciliationRequired {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	_, err = Apply(context.Background(), root, workspace, course, createPlan(), &panicClient{}, false)
	if !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("second apply error=%v", err)
	}
}

func TestApplyPersistsPartialEvidenceBeforeReturning(t *testing.T) {
	root := prepareRoot(t)
	workspace, course := validConfig()
	plan := validPlan()
	plan.Actions = []Action{{Action: "update", Key: "STUDY-2", Set: map[string]any{"summary": "Changed"}}, {Action: "transition", Key: "STUDY-3", Transition: "this_week"}}
	client := &recordingClient{transitionError: &MutationError{Message: "unknown", State: WriteUnknown}}
	result, err := Apply(context.Background(), root, workspace, course, plan, client, false)
	if err == nil || result.Status != "partial" || len(result.Applied) != 1 || !result.ReconciliationRequired {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	manifest := readJSONMap(t, filepath.Join(root, "courses", "CS3103", "state", "latest-run.json"))
	stage := manifest["jira"].(map[string]any)
	if len(stage["applied"].([]any)) != 1 || len(stage["failures"].([]any)) != 1 {
		t.Fatalf("manifest=%v", manifest)
	}
}

func TestReconciliationClearsBarrierOnlyWithExactEmptyPlan(t *testing.T) {
	root := prepareRoot(t)
	workspace, course := validConfig()
	failing := &recordingClient{createError: &MutationError{Message: "unknown", State: WriteUnknown}}
	if _, err := Apply(context.Background(), root, workspace, course, createPlan(), failing, false); err == nil {
		t.Fatal("expected failure")
	}
	recovery := &recordingClient{}
	result, err := Apply(context.Background(), root, workspace, course, validPlan(), recovery, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reconciled || result.ReconciliationRequired || result.Failures[0].WriteState != WriteUnknown {
		t.Fatalf("result=%+v", result)
	}
	want := []string{"fetch:STUDY-1", "children:STUDY-1"}
	if strings.Join(recovery.events, "|") != strings.Join(want, "|") {
		t.Fatalf("events=%v", recovery.events)
	}
	fresh := &recordingClient{}
	if _, err := Apply(context.Background(), root, workspace, course, createPlan(), fresh, false); err != nil {
		t.Fatal(err)
	}
}

func TestReconciliationRejectsUnownedChildrenAndKeepsBarrier(t *testing.T) {
	root := prepareRoot(t)
	workspace, course := validConfig()
	failing := &recordingClient{createError: &MutationError{Message: "unknown", State: WriteUnknown}}
	if _, err := Apply(context.Background(), root, workspace, course, createPlan(), failing, false); err == nil {
		t.Fatal("expected failure")
	}
	recovery := &recordingClient{children: []map[string]any{ownedRemoteIssue("OTHER-2", "STUDY-1")}}
	if _, err := Apply(context.Background(), root, workspace, course, validPlan(), recovery, false); err == nil {
		t.Fatal("accepted unowned issue")
	}
	_, err := Apply(context.Background(), root, workspace, course, createPlan(), &panicClient{}, false)
	if !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("barrier lost: %v", err)
	}
}

type crashDuringCreateClient struct{ root string }

func (c *crashDuringCreateClient) CreateIssue(context.Context, map[string]any) (string, error) {
	manifest := readJSONMapForHelper(filepath.Join(c.root, "courses", "CS3103", "state", "latest-run.json"))
	stage, _ := manifest["jira"].(map[string]any)
	failures, _ := stage["failures"].([]any)
	if stage["reconciliation_required"] != true || len(failures) != 1 {
		os.Exit(87)
	}
	failure, _ := failures[0].(map[string]any)
	exact, _ := failure["exact_action"].(map[string]any)
	issue, _ := exact["issue"].(map[string]any)
	if failure["write_state"] != "unknown" || exact["action"] != "create" || issue["summary"] != "Task" {
		os.Exit(88)
	}
	os.Exit(86)
	return "", nil
}
func (*crashDuringCreateClient) UpdateFields(context.Context, string, map[string]any) error {
	panic("unexpected update")
}
func (*crashDuringCreateClient) TransitionIssue(context.Context, string, string) error {
	panic("unexpected transition")
}
func (*crashDuringCreateClient) FetchIssue(context.Context, string) (map[string]any, error) {
	panic("unexpected fetch")
}
func (*crashDuringCreateClient) EpicChildren(_ context.Context, epic string) ([]map[string]any, error) {
	return []map[string]any{ownedRemoteIssue("STUDY-2", epic)}, nil
}

func readJSONMapForHelper(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	return value
}

func TestApplyCrashHelperProcess(t *testing.T) {
	root := os.Getenv("CORUM_JIRA_CRASH_HELPER")
	if root == "" {
		return
	}
	workspace, course := validConfig()
	_, _ = Apply(context.Background(), root, workspace, course, createPlan(), &crashDuringCreateClient{root: root}, false)
	os.Exit(89)
}

func TestApplyPersistsExactUnknownBarrierBeforeRemoteMutation(t *testing.T) {
	root := prepareRoot(t)
	command := exec.Command(os.Args[0], "-test.run=^TestApplyCrashHelperProcess$")
	command.Env = append(os.Environ(), "CORUM_JIRA_CRASH_HELPER="+root)
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 86 {
		t.Fatalf("crash helper exit = %v", err)
	}
	workspace, course := validConfig()
	if _, err := Apply(context.Background(), root, workspace, course, createPlan(), &panicClient{}, false); !errors.Is(err, ErrReconciliationRequired) {
		t.Fatalf("retry after crash error = %v, want ErrReconciliationRequired", err)
	}
}

type blockingMutationClient struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (c *blockingMutationClient) CreateIssue(context.Context, map[string]any) (string, error) {
	c.calls.Add(1)
	close(c.entered)
	<-c.release
	return "STUDY-4", nil
}
func (*blockingMutationClient) UpdateFields(context.Context, string, map[string]any) error {
	panic("unexpected update")
}
func (*blockingMutationClient) TransitionIssue(context.Context, string, string) error {
	panic("unexpected transition")
}
func (*blockingMutationClient) FetchIssue(_ context.Context, key string) (map[string]any, error) {
	return ownedRemoteIssue(key, "STUDY-1"), nil
}
func (*blockingMutationClient) EpicChildren(_ context.Context, epic string) ([]map[string]any, error) {
	return []map[string]any{ownedRemoteIssue("STUDY-2", epic)}, nil
}

func TestApplyRejectsConcurrentProcessBeforeSecondPreflight(t *testing.T) {
	root := prepareRoot(t)
	workspace, course := validConfig()
	first := &blockingMutationClient{entered: make(chan struct{}), release: make(chan struct{})}
	firstDone := make(chan error, 1)
	go func() {
		_, err := Apply(context.Background(), root, workspace, course, createPlan(), first, false)
		firstDone <- err
	}()
	<-first.entered
	second := &recordingClient{}
	_, secondErr := Apply(context.Background(), root, workspace, course, createPlan(), second, false)
	if !errors.Is(secondErr, lockfile.ErrLocked) {
		t.Fatalf("concurrent Apply() error = %v, calls = %v", secondErr, second.events)
	}
	if len(second.events) != 0 {
		t.Fatalf("second Apply reached Jira preflight: %v", second.events)
	}
	close(first.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}
