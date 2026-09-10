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

Read `.config/corum/corum.yaml` and `courses/{{COURSE}}/course.yaml`. If the workspace has a `jira` block, resolve the epic below; otherwise, proceed to step 3.

When Jira is enabled, resolve the course epic:

1. Use the epic already mapped in `course.yaml` when available.
2. When the mapping is missing, use Jira MCP to search epics in the configured cloud and project with a matching course code. Verify the course identity, including the term when available. Reuse a unique matching epic; report ambiguous matches for resolution.
3. When a successful, complete search finds no match, create a course epic through Jira MCP using the course code and available course name.

Save the reused or created epic key as `jira.epic` in `course.yaml`, preserving its other settings. If a mapped epic is confirmed missing, repeat the lookup process. For unavailable Jira MCP, failed operations or ambiguous matches, follow [error handling](references/error-handling.md) and continue local scoping.

With an epic mapped, refresh its cache:

```console
corum jira sync-epic {{COURSE}}
```

Verify success and pass `courses/{{COURSE}}/state/jira.json` to `scope-course` for comparison.

## 3. Scope and review

Invoke `scope-course` with the retained changeset and available Jira cache. Present its plan for user review.

## 4. Apply approved work

When the user approves the scoped Jira actions, call the configured Jira MCP tools directly. After successful writes, run `corum jira sync-epic {{COURSE}}` again to refresh the cache. Report the observed results.

For command errors, partial captures or uncertain writes, read [error handling](references/error-handling.md).
