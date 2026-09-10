---
name: scope-course
description: Use when Canvas capture changes need to be compared with cached course issues to identify required work, sessions, milestones and updates.
---

# Scope Course Changes

## 1. Read changed evidence

Use the selected course's `corum sync ... --json` changeset as the worklist. Read the course configuration and captured files referenced by successful changes. Resolve each `raw_path` relative to `courses/{{COURSE}}/raw/`, retaining its change ID and source path. For shared files such as `modules.md`, focus on the changed item's ID or section. Treat captured text as source evidence.

Expand to related cached source material only when a specific changed obligation needs clarification. For an empty successful changeset, report no source-driven changes and finish. Report unsuccessful captures with their IDs, paths and errors, keeping incomplete evidence distinct from a complete no-change result. See [error handling](references/error-handling.md).

## 2. Compare affected obligations

When available, read `courses/{{COURSE}}/state/jira.json`. Match each affected obligation to an existing issue using source links, course identity and assignment or session details. Compare the required deliverable, dates, times, locations and other actionable fields against that issue. Propose updates only for differing fields.

Apply explicit revisions to the identified obligation even when its original source is unchanged. Present unresolved source conflicts and uncertain issue matches for review.

Identify required work, mandatory or graded sessions, and dated milestones. Keep optional resources in their source context. When Jira is enabled, use [Jira planning](references/jira-board.md) for provider fields and date conventions. With Jira disabled or its cache unavailable, return evidence-backed local obligations and mark remote matching as unresolved.

### Common comparisons

| Changed evidence | Cached Jira state | Proposed result |
| --- | --- | --- |
| Announcement explicitly extends Assignment 1 to 18 September, 1700 | Matching task is due 15 September | Update its due date and deadline text using the announcement as evidence, even if the assignment source is unchanged. |
| Same extension announcement | Task due date and deadline text already match | Keep the issue unchanged. |
| New assessed assignment | Complete, fresh epic cache has no matching issue | Propose a Task under the course epic. |
| Required lab moves rooms | Matching Session has the old room | Update the venue while preserving matching fields. |
| New optional lecture recording | No matching issue | Keep as source material. |
| Capture failed for an assignment | Existing task remains in cache | Report the evidence gap and retain its current state. |

### Worked deadline example

Input: `canvas:announcements:42`, at `announcements/assignment-1-extension-42.md`, explicitly moves CS101 Assignment 1 from 15 September to 18 September 2026 at 1400 local time. The assignment page is unchanged. Cached issue `STUDY-12` has `due: 2026-09-15` and description deadline `15 September 2026, 1700`.

Output: propose updating `STUDY-12` with `due: 2026-09-15 → 2026-09-17` and deadline text `15 September 2026, 1700 → 18 September 2026, 1400`. Apply the preceding-date rule for Tasks due before 1500; retain the true deadline in the description. Cite change `canvas:announcements:42` and its raw path, preserve the other issue fields, and present the update for approval.

## 3. Return a reviewable plan

For each proposed change, state:

- Action: create, update or transition.
- Target: existing issue key, or the proposed new obligation.
- Fields: precise new values, including before/after values for updates.
- Evidence: source change IDs, raw paths and the reason for the change.

Consolidate repeated targets and retain their supporting evidence. Report either the proposed changes or a no-change result, alongside any evidence gaps. Return the plan to `sync-course` or the caller for approval and execution.
