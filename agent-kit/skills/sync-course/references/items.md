# Tracker items

These rules hold for every tracker. Each tracker reference's item format table says how to store them in that tool.

## Types

| Type | Means |
| --- | --- |
| `task` | Finish required work by a deadline |
| `session` | Attend a required or graded session |
| `milestone` | Reach or attend a required one-off date, such as an exam |

Put required work in the tracker: graded submissions, explicit administrative requirements, mandatory or graded sessions, and required milestones. Leave optional work, sessions with unknown attendance, lecture files and recordings in the course material. A source that contains a separate requirement gets its own item.

## Titles

Every item belongs to the course group named `{{COURSE}}`. Titles start with `[{{COURSE}}]` and name the item. A recurring session also names its week.

| Good | Bad |
| --- | --- |
| `[CS3103] Lab (W7)` | `CS3103 Lab (W7)` |
| `[IS2218] Midterm Test` | `Midterm Test` |

## Dates

Convert source timestamps to the timezone in `.config/corum/corum.yaml`. Use `Term_Calendar.md`, installed from the selected academic term, for week numbers. Check that its term matches the workspace; if no matching calendar is available, report it and use exact dates without inventing week numbers.

Record the true local deadline for a `task` and the local start for a `session` or `milestone` in the description. The tracker's item format table decides the due date stored from it.

## Status

New items use the tracker's to-do status. Moves go to the statuses named in the tracker's item format table.

## Details

Include every known detail needed to act from the item. Begin with week, local date, time, and venue when applicable. Format times without colons and write sources as Markdown links. Use the venue, policy, date and requirement details from the Canvas material.

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

Use category labels that describe the work or session: `lecture`, `tutorial`, `lab`, `seminar`, `exam`, `assessment`, `must-attend`, `graded-attendance`, `online`, or `offline-recorded`. The type and the course code are stored separately, as the tracker's item format table describes.
