# Jira MCP Boundary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task with review checkpoints.

**Goal:** Make Jira MCP the agent-facing authority, configure it for Codex and Claude per project, and retain only a deterministic `sync-epic` Corum operation for rebuilding local Jira state.

**Architecture:** Add a small project-config writer for `.codex/config.toml` and `.mcp.json`; update the embedded skills to call Jira MCP directly; replace the current generic Jira client/apply/provisioning surface with one read-only epic synchronizer that owns its own MCP read connection and atomically rebuilds `state/jira.json`.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk`, YAML/JSON state files, embedded agent-kit assets, Go tests.

**Spec:** `docs/superpowers/specs/2026-09-09-jira-mcp-boundary-design.md`

## Global Constraints

- Ordinary Jira reads and singular mutations are performed through Jira MCP by the agent.
- Corum exposes only deterministic `sync-epic` Jira runtime behavior in this iteration; no batch executor.
- `sync-epic` leaves the previous cache untouched on any failed or incomplete remote read.
- Project config writes preserve unrelated settings and never write bearer or refresh tokens.
- Existing course epic assignments and `jira.json` remain readable; migration performs no Jira write.

### Task 1: Add failing tests for project-local MCP configuration

**Files:**
- Create: `internal/jira/mcp_config_test.go`
- Modify: `internal/jira/mcp_config.go` (created in Task 2)

**Interfaces:**
- `writeProjectMCPConfig(root string) error`
- `codexConfigPath(root string) string`
- `claudeConfigPath(root string) string`

- [ ] **Step 1: Write tests for preserving and merging config.** Cover absent files, existing unrelated TOML/JSON entries, idempotent repeated writes, exact Atlassian URL, and absence of token-like keys/values.
- [ ] **Step 2: Run `go test ./internal/jira -run TestProjectMCPConfig -count=1`.** Confirm the tests fail because the writer does not exist.

### Task 2: Implement project-local Codex and Claude MCP config writing

**Files:**
- Create: `internal/jira/mcp_config.go`
- Modify: `internal/cli/app.go`
- Test: `internal/jira/mcp_config_test.go`

**Interfaces:**
- `writeProjectMCPConfig(root string) error` writes `.codex/config.toml` and `.mcp.json` atomically.
- The named server is `atlassian-jira`; URL is `https://mcp.atlassian.com/v2/mcp`.

- [ ] **Step 1: Implement strict TOML/JSON merge with atomic replacement.** Preserve unrelated keys, use `0644` for non-secret config, and reject malformed existing files without overwriting them.
- [ ] **Step 2: Wire `runConfigure`’s Jira branch to call the writer before/alongside the existing interactive prompt.** Keep configuration failure recoverable and explain which file failed.
- [ ] **Step 3: Run the focused config tests and `go test ./internal/cli -run Test.*Configure.*Jira -count=1`.** Confirm all pass.
- [ ] **Step 4: Commit `feat: configure project Jira MCP clients`.**

### Task 3: Add failing tests for deterministic epic synchronization

**Files:**
- Create: `internal/jira/sync_test.go`
- Modify: `internal/jira/sync.go` (created in Task 4)

**Interfaces:**
- `SyncEpic(ctx context.Context, root, courseCode, epicKey string, options SyncOptions) (SyncResult, error)`.
- `SyncOptions` supplies the MCP caller, clock-independent request behavior, and output path for tests.

- [ ] **Step 1: Write tests for epic and child fetches, pagination, parent/project validation, normalized fields, stable issue ordering, and atomic replacement.** Use an in-memory MCP caller that records tool names/arguments and returns structured pages.
- [ ] **Step 2: Write a failure test proving malformed/incomplete responses preserve the old `jira.json` byte-for-byte.**
- [ ] **Step 3: Run `go test ./internal/jira -run TestSyncEpic -count=1`.** Confirm the tests fail before implementation.

### Task 4: Implement the sole Corum Jira runtime operation

**Files:**
- Create: `internal/jira/sync.go`
- Modify: `internal/jira/state.go` only where existing state helpers are reusable without broad Jira dispatch
- Modify: `internal/cli/app.go`
- Test: `internal/jira/sync_test.go`

**Interfaces:**
- `SyncEpic` independently authenticates/connects to Atlassian MCP, fetches the configured epic and all children, validates ownership, normalizes to the existing `JiraState`, sorts deterministically, and atomically writes the cache.
- CLI command: `corum jira sync-epic COURSE` with optional `--epic ISSUE-KEY` constrained to the course’s configured epic.

- [ ] **Step 1: Implement the minimum read-only MCP connection inside the sync path.** Do not expose generic tool dispatch or singular mutation methods; keep credentials separate from Codex/Claude client stores.
- [ ] **Step 2: Implement normalization using existing schema-compatible fields and deterministic ordering.** Reject missing keys, wrong project, wrong parent, malformed dates, and incomplete pagination.
- [ ] **Step 3: Implement atomic cache replacement and JSON result output.** On any error, preserve the prior file.
- [ ] **Step 4: Replace Jira command routing with `sync-epic`; remove handlers for `create-epic`, `apply`, `status`, and `logout`.** Update usage text and command-path detection.
- [ ] **Step 5: Run focused sync and CLI tests.** Confirm all pass.
- [ ] **Step 6: Commit `feat: add deterministic Jira epic sync`.**

### Task 5: Remove obsolete Jira wrapper surface

**Files:**
- Delete: `internal/jira/client.go`, `internal/jira/plan.go`, `internal/jira/apply.go`, `internal/jira/rovo.go`, `internal/jira/oauth.go`, `internal/jira/callback.go`, related obsolete state/auth files and tests after preserving sync dependencies.
- Modify: `go.mod`, `go.sum`, `internal/jira/models.go`, `internal/jira/state.go`
- Modify: all CLI and UI references found by `rg -n 'jira\.(Open|Apply|NewJiraClient)|runJira(CreateEpic|Apply)|ensureCourseEpic|RovoSession' .`

- [ ] **Step 1: Remove obsolete production symbols only after Task 4 compiles.** Retain schema/state types needed by sync and non-Jira configuration validation.
- [ ] **Step 2: Remove now-unused dependencies and update tests.** Do not remove the MCP SDK needed by `sync-epic`.
- [ ] **Step 3: Run `go test ./...`.** Fix only regressions caused by the approved boundary change.
- [ ] **Step 4: Commit `refactor: remove Jira operation wrapper`.**

### Task 6: Rewrite embedded skills and integration coverage

**Files:**
- Modify: `agent-kit/skills/sync-course/SKILL.md`
- Modify: `agent-kit/skills/scope-course/SKILL.md`
- Modify: `agent-kit/skills/sync-course/references/error-handling.md`
- Modify: `agent-kit/skills/scope-course/references/jira-board.md`
- Modify: `internal/integration/end_to_end_test.go`
- Modify: affected skill/CLI tests

- [ ] **Step 1: Replace all `corum jira create-epic/apply/status/logout` instructions with direct Jira MCP guidance.** Require `corum jira sync-epic COURSE` before planning and after approved mutations; state that uncertain writes are reconciled before retry.
- [ ] **Step 2: Update scope output to describe individual MCP operations, not a Corum apply payload.** Preserve evidence and approval requirements.
- [ ] **Step 3: Add integration assertions that toolkit installation contains direct MCP instructions and fresh initialization writes both client config files.**
- [ ] **Step 4: Run `go test ./...` and `go vet ./...`.**
- [ ] **Step 5: Commit `docs: route Jira workflows through MCP`.**

### Task 7: Final verification and migration check

**Files:**
- Modify only if verification finds a concrete issue.

- [ ] **Step 1: Run `go test ./...` from a clean working tree.**
- [ ] **Step 2: Run `go vet ./...` and `git diff --check`.
- [ ] **Step 3: Inspect `git diff --stat` and search for forbidden wrapper instructions with `rg -n 'corum jira (create-epic|apply|status|logout)' agent-kit internal README.md SECURITY.md`.**
- [ ] **Step 4: Confirm no token-like fields are emitted by config tests and no migration test performs a remote Jira write.**
- [ ] **Step 5: Commit any final test-only corrections separately and report exact verification output.**
