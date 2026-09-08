# Jira Epic Provisioning Design

## Goal

Make Jira setup reliable for a newly initialized vault: normalize Jira's
compact timestamp offsets, then locate or create one course epic before the
Jira cache is reconciled.

## Existing integration

Corum already uses Atlassian's supported Jira operations through the
authenticated Rovo MCP session. `JiraClient` delegates to `getJiraIssue`,
`searchJiraIssuesUsingJql`, and `createJiraIssue`; it must not add a second
HTTP client or duplicate OAuth handling.

## Epic identity and provisioning

For a Canvas course, Corum will use this exact epic summary:

```
{{COURSE_CODE}} — {{CANVAS_COURSE_NAME}}
```

The provisioning command will search the configured Jira project for epics
with that exact summary. One match is reused and its issue key is stored in
the course's `jira.epic` configuration. No match causes Corum to create an
Epic through the existing `createJiraIssue` MCP tool, verify the created
issue, and store its key. More than one exact match is an error; Corum will
not guess which duplicate epic owns a course.

Provisioning applies only to a Canvas-backed course with Jira configured for
the vault and no existing `jira.epic`. Existing course epic configuration
remains authoritative and is never searched for or overwritten. The command
is explicit so an ordinary sync never creates Jira work without the user's
chosen initialization step.

## Command and data flow

Add a `corum jira ensure-epic COURSE` command. It loads the workspace and
course configuration, opens the existing project-local Jira/Rovo session,
and calls a new Jira-client epic lookup helper. It then atomically writes the
resolved key into `courses/COURSE/course.yaml`. Its JSON output reports the
course, epic key, and whether the epic was reused or created.

The generated `sync-course` skill will run this command when Jira is enabled,
the selected course has no `jira.epic`, and the course has Canvas metadata.
It will then run its existing empty-plan reconciliation. Courses without
Canvas metadata receive a clear configuration error instead of a fabricated
epic name.

Canvas course configuration must retain the Canvas display name captured at
course selection time. A legacy course whose stored Canvas name is blank
cannot safely satisfy the requested naming convention, so provisioning will
return an actionable error instead of silently falling back to the code-only
name.

## Timestamp normalization

`normalizeJiraTimestamp` will identify a compact terminal offset by checking
the sign at five characters from the end, then insert a colon before the last
two digits. It must preserve already RFC 3339-formatted offsets and non-offset
timestamps. Tests cover `+0800`, `-0700`, and existing `+08:00` values.

## Failure handling and tests

Epic lookup must use a project/type/summary-constrained JQL query with JSON
quoting for the summary. Empty, malformed, remote-error, and ambiguous
results fail before changing local configuration. Creating an epic treats a
missing or malformed returned issue key as an applied-but-unknown mutation,
matching existing Jira mutation safety rules.

Tests will cover exact lookup arguments, duplicate results, creation mapping,
atomic course configuration updates, CLI wiring with a fake session/client,
and the skill's documented greenfield flow. Existing client and integration
tests must remain green.
