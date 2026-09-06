# Jira Planning Conventions

Read this only when Jira is effectively enabled.

## Issue types

| Type | Means |
| --- | --- |
| `Task` | Finish required work by a deadline |
| `Session` | Attend a required or graded session |
| `Milestone` | Reach or attend a required one-off date |

Only required work belongs on the board. Assessed submissions, explicit
administrative obligations, mandatory or graded sessions, and required milestones
qualify. Optional items and sessions with unknown attendance do not. Lecture files
and recordings are knowledge sources unless they establish a separate action.

Every create uses the course epic from `course.yaml`. Summaries start with
`{{COURSE}}` and name the item. A recurring session also names its week.

## Dates

Convert source timestamps to the timezone in `.config/corum/corum.yaml`. Look up week numbers in
the configured term-calendar file. For a `Task` whose true local deadline is before
1500, use the preceding date as Jira `due`; this leaves a usable workday. Sessions
and milestones remain on the day they happen.

## Status

New issues use the board default. When an issue must move, name a transition declared
under the workspace Jira configuration. The exact plan carries the transition name;
`corum jira apply` resolves its configured ID.

## Details

Include every known detail needed to act without reopening Canvas. Begin with week,
local date, time, and venue when applicable. Times omit colons. Use Markdown links,
not bare URLs. Do not invent a venue, policy, date, or required status.

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

Use only meaningful board categories such as `lecture`, `tutorial`, `lab`,
`seminar`, `exam`, `assessment`, `must-attend`, `graded-attendance`, `online`, or
`offline-recorded`. Do not duplicate the course code as a label.
