# Jira Planning Conventions

Read this when the Jira workflow is enabled.

## Issue types

| Type | Means |
| --- | --- |
| `Task` | Finish required work by a deadline |
| `Session` | Attend a required or graded session |
| `Milestone` | Reach or attend a required one-off date |

Place required work on the board. Assessed submissions, explicit administrative obligations, mandatory or graded sessions, and required milestones qualify. Keep optional items, sessions with unknown attendance, lecture files, and recordings in their course-source context. A source with a separate required obligation creates its matching board action.

Every create uses the course epic from `course.yaml`. Summaries start with `{{COURSE}}` and name the item. A recurring session also names its week.

## Dates

Convert source timestamps to the timezone in `.config/corum/corum.yaml`. Look up week numbers in the configured term-calendar file. For a `Task` whose true local deadline is before 1500, use the preceding date as Jira `due`; this leaves a usable workday. Sessions and milestones use the date on which they occur.

## Status

New issues use the board default. When an issue must move, call the Jira MCP transition tool with the transition declared under the workspace Jira configuration.

## Details

Include every known detail needed to act from the issue. Begin with week, local date, time, and venue when applicable. Format times without colons and write sources as Markdown links. Use captured venue, policy, date, and requirement information.

### Task

```markdown
{{What to produce and why.}}

**Deadline:** Week {{WEEK}} | {{LOCAL_DATE_TIME}}
**Weight:** {{WEIGHT_OR_OMIT}}
**Submission:** {{SUBMISSION_OR_OMIT}}
**Late policy:** {{POLICY_OR_OMIT}}

**Resources**
- [Assignment page on Canvas]({{SOURCE_URL}})
```

### Session or milestone

```markdown
Week {{WEEK}} | {{LOCAL_DATE_TIME}}, {{VENUE_OR_MODE}}

**Attendance:** {{REQUIREMENT_OR_OMIT}}
**Scope:** {{SCOPE_OR_OMIT}}
**Format:** {{FORMAT_OR_OMIT}}
```

## Labels

Use meaningful board categories such as `lecture`, `tutorial`, `lab`, `seminar`, `exam`, `assessment`, `must-attend`, `graded-attendance`, `online`, or `offline-recorded`. Use labels to describe the work or session.
