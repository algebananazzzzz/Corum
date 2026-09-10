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

Select the course's result from the JSON array. It contains `course`, `dry_run`,
`status`, `changes`, `failures`, and `sources`. Each change identifies the source
item, its summary, its `raw_path` relative to the course's `raw/`, and available
metadata. Read those captured files to understand the evidence.

Consume this result now. There is no saved run manifest. A later capture uses the
Canvas cache and may return no changes for already captured material.

## 2. Read current obligations

Read `.config/corum/corum.yaml` and `courses/{{COURSE}}/course.yaml`.
If Jira is configured and the course has an epic, refresh its cache:

```console
corum jira sync-epic {{COURSE}}
```

Confirm success, then read `courses/{{COURSE}}/state/jira.json`.
If the epic is not configured, report that Jira reconciliation is unavailable.
Do not create an epic as a side effect of capture. If Jira is unavailable, scope
local obligations and identify the missing remote context.

## 3. Scope and review

Use `scope-course` with the changeset and available cached issues. Present the
proposed actions with their targets, changed fields and source evidence. Report
capture failures and uncertain matches alongside the plan. If there are no
supported changes, say so.

## 4. Apply approved work

When the user approves Jira changes, call the configured Jira MCP tools directly.
After successful writes, run `corum jira sync-epic {{COURSE}}` again to refresh the
cache. Report the observed results. Do not create run manifests or changelogs.

For command errors, partial captures or uncertain writes, read
[error handling](references/error-handling.md).
