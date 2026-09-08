# Jira Epic Provisioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a Canvas-backed course safely reuse or create a Jira epic before its first Jira cache reconciliation.

**Architecture:** Preserve Canvas's display name in course configuration, add an explicit `jira ensure-epic` command, and use `JiraClient` exclusively as the adapter to Rovo's existing MCP tools. Exact-summary JQL lookup returns one reusable Epic or creates one; the command atomically stores its key. Correct Jira compact-offset parsing independently unblocks existing cache reconciliation.

**Tech Stack:** Go, Cobra-free CLI dispatch, `gopkg.in/yaml.v3`, Atlassian Rovo MCP client, Go standard-library tests.

**Spec:** `docs/superpowers/specs/2026-09-08-jira-epic-provisioning-design.md`

## Global Constraints

- Use only the existing authenticated Rovo MCP session and Jira tools; do not add a direct HTTP Jira client or new OAuth path.
- Course epic summary is exactly `{{COURSE_CODE}} — {{CANVAS_COURSE_NAME}}`.
- Reuse only one exact Epic match in the configured project; fail on duplicate matches.
- Existing `jira.epic` remains authoritative and is never overwritten.
- A course without a stored Canvas name fails explicitly; do not make up a code-only name.
- All production behavior is introduced test-first and must retain existing mutation safety guarantees.

---

### Task 1: Preserve Canvas names and fix compact offsets

**Files:** `internal/config/course.go`, `internal/vault/course_tracking.go`, `internal/vault/course_tracking_test.go`, `internal/jira/apply.go`, `internal/jira/apply_test.go`

**Interfaces:** Produces `config.CanvasCourse.Name string` as `canvas.name` and `normalizeJiraTimestamp(string) string` with RFC 3339 terminal offsets.

- [ ] Write `TestConfigureCanvasCoursesStoresCanvasName`, asserting that a selected Canvas course called `Computer Networks` reloads with `course.Canvas.Name == "Computer Networks"`.
- [ ] Run `go test ./internal/vault -run TestConfigureCanvasCoursesStoresCanvasName -count=1`; expect compile failure because `Name` is absent.
- [ ] Add `Name string \`yaml:"name,omitempty"\`` to `config.CanvasCourse`; set a trimmed name from `CanvasCourseChoice.Name` when creating the course. Keep blank legacy names valid.
- [ ] Run the focused vault test; expect PASS.
- [ ] Write `TestNormalizeJiraTimestamp` with `+0800 -> +08:00`, `-0700 -> -07:00`, and already normalized `+08:00`.
- [ ] Run `go test ./internal/jira -run TestNormalizeJiraTimestamp -count=1`; expect failure for compact offsets.
- [ ] Check the terminal sign at `len(value)-5`, insert the colon before the final two digits, and do not alter values with a terminal-colon offset.
- [ ] Run `go test ./internal/vault ./internal/jira -run 'TestConfigureCanvasCoursesStoresCanvasName|TestNormalizeJiraTimestamp' -count=1`; expect PASS. Commit `fix: preserve course names and Jira offsets`.

### Task 2: Add MCP-backed exact Epic lookup and creation

**Files:** `internal/jira/client.go`, `internal/jira/client_test.go`, `internal/jira/models.go`

**Interfaces:** Produces `EnsureEpic(ctx context.Context, project, summary string) (EpicResult, error)` and `EpicResult{Key string, Created bool}`. Uses only `searchJiraIssuesUsingJql` and `createJiraIssue` through `jsonCaller`.

- [ ] Write `TestJiraClientEnsureEpic`: one response reuses `STUDY-1`; assert JQL is exactly `project = "STUDY" AND issuetype = "Epic" AND summary = "CS3103 — Computer Networks"`. Add zero-result creation and duplicate-result no-create cases.
- [ ] Run `go test ./internal/jira -run TestJiraClientEnsureEpic -count=1`; expect failure because the type and method are absent.
- [ ] Implement JQL quoting via `json.Marshal`. Validate all candidate keys and Epic type. On zero matches call `createJiraIssue` with `cloudId`, `projectKey`, summary, and `issueType: "Epic"`; apply the existing malformed-create-key safety policy.
- [ ] Run the focused test; expect PASS. Commit `feat: provision course Jira epics through MCP`.

### Task 3: Persist an ensured epic and expose it through the CLI

**Files:** `internal/vault/init.go`, `internal/vault/vault_test.go`, `internal/cli/app.go`, `internal/cli/app_test.go`

**Interfaces:** Produces `vault.WriteCourse(root string, course config.Course) error`. Produces JSON `{ "course": "CS3103", "epic": "STUDY-1", "created": false }` from `corum jira ensure-epic CS3103`.

- [ ] Write `TestWriteCourse` that saves `course.Jira = &config.JiraCourse{Epic: "STUDY-1"}` then reloads it. Run `go test ./internal/vault -run TestWriteCourse -count=1`; expect missing method failure.
- [ ] Implement `WriteCourse`: validate, marshal, verify the existing `courses/<code>/course.yaml` target, and atomically replace it using `writeConfigAtomic`.
- [ ] Write `TestRunJiraEnsureEpic`: exact reuse writes the key/reports `created:false`; no match reports `created:true`; preconfigured epics and missing Canvas names make no remote calls.
- [ ] Run `go test ./internal/cli -run TestRunJiraEnsureEpic -count=1`; expect command failure.
- [ ] Add help, usage, dispatch, and a non-interactive project-local Rovo session. Before opening it reject disabled Jira, missing Canvas/name, and configured epic. Ensure using `course.Code+" — "+course.Canvas.Name`, atomically write `course.yaml`, then encode the result.
- [ ] Run `go test ./internal/vault ./internal/cli -run 'TestWriteCourse|TestRunJiraEnsureEpic' -count=1`; expect PASS. Commit `feat: add Jira epic initialization command`.

### Task 4: Provision before first-run cache reconciliation

**Files:** `agent-kit/skills/sync-course/SKILL.md`, `internal/integration/end_to_end_test.go`

**Interfaces:** The generated workflow calls `corum jira ensure-epic {{COURSE}}` when Jira is enabled and `course.yaml` lacks `jira.epic`, then builds its existing empty plan with the saved key.

- [ ] Write `TestJiraEnsureEpicHelp`, asserting `corum jira ensure-epic --help` prints `usage: corum jira ensure-epic COURSE`.
- [ ] Run `go test ./internal/integration -run TestJiraEnsureEpicHelp -count=1`; expected failure before the Task 3 command is wired; after Task 3 it proves its continued availability.
- [ ] Replace the sync skill's assumption that each course already has an epic: run ensure-epic, read the JSON response, then run existing empty-plan reconciliation. Retain existing exact-plan approval for child issues.
- [ ] Run `go test ./internal/integration -run TestJiraEnsureEpicHelp -count=1 && go test ./...`; expect PASS. Commit `docs: provision Jira epics before sync`.
