# Jira MCP Boundary Design

## Goal

Make the Jira MCP visible and directly usable to Codex and Claude agents. Corum
will stop encapsulating ordinary Jira reads and mutations. It will retain one
deterministic operation, `syncEpic`, whose sole responsibility is to rebuild a
course's local Jira cache from the remote epic.

## Decisions

Corum's Jira workflow has these ownership boundaries:

| Work | Owner | Local cache rule |
| --- | --- | --- |
| Read one issue or search Jira | Jira MCP, called by the agent | No cache write unless the agent requests an epic sync. |
| Create an epic; create, update, or delete one issue; transition one issue | Jira MCP, called by the agent | The agent must run `corum jira sync-epic EPIC` after a successful mutation. |
| Read and normalize the complete state of one epic | `corum jira sync-epic EPIC` | Atomically replace `courses/COURSE/state/jira.json`. |
| Apply a multi-operation batch | Not implemented | A future explicit batch executor may call `syncEpic` after it returns. |

There is deliberately no general-purpose Corum Jira mutation wrapper and no
local Jira plan or dry-run executor. A future batch executor is not required
for cache correctness: direct MCP mutations followed by `syncEpic` provide
that guarantee without hiding Jira operations from the agent.

## Project-local MCP setup

`corum configure jira [PATH]` will configure the same Atlassian Rovo MCP v2
endpoint (`https://mcp.atlassian.com/v2/mcp`) in both project-local client
configurations:

- `.codex/config.toml` for Codex;
- `.mcp.json` for Claude Code.

It must merge only Corum's named Jira server entry, preserve all unrelated
user-owned MCP entries and settings, and write no bearer token or refresh
token into the repository. The command reports the exact client commands
needed to complete each client's OAuth login. It collects the Jira cloud ID
and project key as validated user input; it does not query Jira to discover
them. Agents discover sites and projects through Jira MCP after login.
Authentication remains owned by Codex and Claude; Corum does not store or
share their credentials.

The generated toolkit will state that Jira MCP is the authoritative interface
for normal Jira work. It will name `sync-epic` as the required post-mutation
reconciliation step.

## `syncEpic`

The command is the only Corum Jira runtime operation in this iteration:

```
corum jira sync-epic COURSE [--epic ISSUE-KEY]
```

The course's configured epic is the default. `--epic` permits an explicit
epic only when it matches the course configuration; Corum must not silently
reassign course ownership. The function independently connects to Atlassian's
MCP endpoint, fetches the epic and every child issue, validates their project
and parent relationship, normalizes fields into the existing `jira.json`
schema, sorts deterministically, and atomically replaces the cache. Its JSON
output includes the course, epic, issue count, and cache path.

An MCP client cannot borrow Codex's or Claude's live connection. Therefore
the helper owns only the minimum protocol and authentication machinery needed
to read one epic. It does not expose generic tool dispatch, issue creation,
update, transition, or deletion APIs. A missing Corum read credential produces
an interactive OAuth prompt when `sync-epic` is first run; it must never infer
or copy another client's stored OAuth token. `corum configure jira` configures
the agent clients only, so their authentication and Corum's read-only
authentication remain separate.

If the remote response is incomplete, malformed, inaccessible, or cannot be
validated, `syncEpic` leaves the prior cache unchanged. There is no partial
cache update. A successful sync is a complete point-in-time replacement.

## Removed surface

Remove the local Rovo session wrapper, custom OAuth cache, generic Jira
client, plan decoding/validation, apply logic, epic-provisioning state, and
the following commands:

```
corum jira create-epic COURSE
corum jira apply COURSE [--dry-run]
corum jira status [PATH]
corum jira logout [PATH]
```

Remove their associated schemas, tests, and generated-skill instructions.
Preserve the existing local `jira.json` schema and its atomic-write safety
properties so historical state remains readable.

## Skill workflow

The scope skill uses the cache produced by `sync-epic`; it no longer emits a
Corum apply payload. The sync skill instructs the agent to:

1. Use Jira MCP directly to find or create the course epic, then save its key
   in `course.yaml` through the existing course configuration workflow.
2. Run `corum jira sync-epic COURSE` before planning whenever Jira is enabled.
3. Present individual MCP operations for approval.
4. Execute each approved operation through Jira MCP.
5. Run `sync-epic` once after all successful operations, then record the
   observed results in `latest-run.json` and the changelog.

On an uncertain direct mutation, the agent must not replay it. It first runs
`sync-epic`, scopes from the reconciled cache, and obtains fresh approval.

## Verification and migration

Tests will prove project-config merging and preservation, secret-free config
output, sync request shape/pagination, deterministic normalization and
ordering, atomic cache replacement, course/epic validation, and no cache
change on a failed sync. Integration tests will verify a fresh vault receives
both client configurations and toolkit instructions that use Jira MCP directly.

Existing vaults preserve `course.yaml` Jira epic assignments and `jira.json`.
`corum toolkit update` replaces the outdated workflow instructions. The next
explicit `sync-epic` refreshes historical cache data. No migration performs a
Jira write.
