---
name: sync-course
description: Use when a user asks to sync, catch up, or reconcile a course's obligations from Canvas and optionally Jira.
---

# Sync Course

## 1. Capture

From the vault root, run:

```console
corum sync {{COURSE}} --json
```

Verify the exit code and the selected course's `dry_run`, `status`, `failures`, and source statuses. Pass that course's JSON changeset to `scope-course` in this workflow. Defer captured-file reads to scoping.

## 2. Prepare current obligations

Read `.config/corum/corum.yaml` and `courses/{{COURSE}}/course.yaml`. Use `task_tracker` as the selected provider. For older configuration without that field, a `jira` block enables Jira; otherwise task tracking is disabled. An inactive provider's saved settings are retained for later reuse.

When Jira is enabled, resolve the course epic:

1. Use the epic already mapped in `course.yaml` when available.
2. For missing or invalid mappings, direct the user to `corum configure` → Jira Epic mapping. Suggestions may be offered, but configuration owns the mapping.

For unavailable Jira MCP, failed operations or unmapped courses, follow [error handling](references/error-handling.md) and continue local scoping.

With an epic mapped, refresh its cache:

```console
corum jira sync-epic {{COURSE}}
```

Verify success and pass `courses/{{COURSE}}/state/jira.json` to `scope-course` for comparison.

For `google_tasks`, use the saved `google_tasks.list_id`. Run `corum google sync-tasks {{COURSE}}` and pass the successful `state/google-tasks.json` cache to scoping. Missing mappings are configured through Google Task List mapping in `corum configure`. Use Google Tasks date-level scheduling; request no Calendar access.

## 3. Scope and review

Invoke `scope-course` with the retained changeset and available Jira cache. Present its plan for user review.

## 4. Apply approved work

When the user approves the scoped Jira actions, call the configured Jira MCP tools directly. After successful writes, run `corum jira sync-epic {{COURSE}}` again to refresh the cache. Report the observed results.

For approved Google Tasks changes, use the authenticated Google Workspace CLI: `gws tasks tasks insert` or `patch`, with `--params` containing the mapped `tasklist` (and `task` for updates), and `--json` containing only approved fields. If `gws` is unavailable, use `npx --yes @googleworkspace/cli@0.22.5` as the command prefix. Introspect the command schema before writes. Store source links and exact deadlines in notes; use `due` for the calendar date and `status` for `needsAction` or `completed`. Refresh the Google Tasks cache after successful writes. Never retry an uncertain creation automatically.

For command errors, partial captures or uncertain writes, read [error handling](references/error-handling.md).
