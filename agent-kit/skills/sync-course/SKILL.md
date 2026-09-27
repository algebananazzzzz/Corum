---
name: sync-course
description: Use when the user asks to sync, catch up on, or reconcile one or all Canvas courses with their task tracker (Kaneo, Jira or Google Tasks), such as turning new assignments, announcements, deadline changes or class schedules into tracker items.
---

# Sync Course

`courses/{{COURSE}}/state/tracker.json` is the local copy of the course's remote tracker state. Update it after every write you make to the tracker. Never list the course's items from the tracker unless the file is missing or invalid, or the user asks to reconcile from remote; the only other tracker reads are the single-item checks in step 8.

```
- [ ] 1. Find the course codes
- [ ] 2. Read the tracker reference and check tracker access
- [ ] 3. Choose the tracker project and course groups, when missing
- [ ] 4. Run corum sync
- [ ] 5. Read tracker.json, or rebuild it from the tracker
- [ ] 6. Compare the Canvas changes with tracker.json and run the weekly checks
- [ ] 7. List the proposed rows and ask which to apply
- [ ] 8. Write the approved rows to the tracker and to tracker.json
```

Steps 1 to 3 run once per session, before any `corum sync`, so a sign-in that needs a restart happens before Canvas is fetched. For "all courses", then run steps 4 to 8 for each course in turn, finishing one course's writes before starting the next.

## 1. Find the course codes

List the directories under `courses/` whose `course.yaml` has a `canvas` block; each directory name is a course code. Match the course the user names against `code` and `canvas.name`, such as "big data" to `CS4225`, and ask when several match.

## 2. Read the tracker reference and check tracker access

Read `task_tracker` in `.config/corum/corum.yaml`. For `none` or no value, skip to step 4 and complete without `tracker.json`. Otherwise read the reference for that tracker, [kaneo.md](references/trackers/kaneo.md), [jira.md](references/trackers/jira.md) or [google-tasks.md](references/trackers/google-tasks.md). It maps each **bold operation** in this skill to that tool's calls.

Check that the tools in the reference's **Access** row are available. When they are not, give the user these sign-in steps and ask them to restart the session:

- Claude Code: run `/mcp`, select the reference's server, approve it if asked, and authenticate in the browser.
- Codex: run `codex mcp login <server>` in a terminal, then restart Codex in the vault.
- A CLI tracker: the reference's **Sign-in**.

Run **Find project** when the reference has it. When the tracker is unreachable, report it and continue from step 4 without `tracker.json`.

## 3. Configure the project and course groups, when missing

Do this step when `corum.yaml` lacks the reference's **Project** setting, or a selected course's `tracker.json` is missing or has `group: null`. Otherwise go to step 4.

1. When the reference has a **Project** setting, **List projects** and ask the user to choose from the list of projects.
2. When the reference has **Find course group**, run it for every tracked course. The group is named after the course code. Report each as found, missing (propose **Create course group**), or **Misnamed** (propose **Rename course group**). Report a course with several possible groups as ambiguous and ask user to choose.
3. Save the approved project under the tracker's block in `corum.yaml`, keeping every other key, then run `corum doctor` and fix what it reports.
4. Create or rename the approved course groups.

## 4. Run corum sync

Run `corum sync {{COURSE}} --json` from the vault root and check the exit code and the course's `dry_run`, `status`, `failures` and source statuses.

- `dry_run` is true: run it again without a dry run.
- Nonzero exit: use the successful changes in step 6 and report the failed sources as missing.

Keep the JSON output for step 6. On a course's first run every Canvas item is new, so compare the whole course.

## 5. Read tracker.json

Run `corum doctor` once per session; it checks the validity of `tracker.json`. If doctor rejects it or says it is from another tracker, or the user asks to reconcile from remote, rebuild it from the tracker:

1. **List items** through every page and keep the ones the reference's **Belongs to the course** row describes.
2. When the reference's **Full read check** fails, keep the previous file and report it as out of date. Before proposing to create an item missing from an out-of-date file, **Find item by title**.
3. Convert each item with the reference's `tracker.json` fields and replace the file in the shape [tracker-json.md](references/tracker-json.md) defines, with `synced_at` set to now.
4. Run `corum doctor` and fix each reported field from the data you read until it passes.

## 6. Compare the Canvas changes with tracker.json

Read the course's `course.yaml` and, for each successful change, its `raw_path` under `courses/{{COURSE}}/raw/`, keeping the change ID. In shared files such as `modules.md`, read only the changed item's section. Read other files under `raw/` only to clarify a specific requirement.

Find each requirement the changes touch: work to submit, a required or graded session to attend, or an exam. For each one, apply [items.md](references/items.md):

- Leave optional material in its source. Skip requirements whose date has passed and report only how many.
- Match it to an item in `tracker.json` by source link, `[{{COURSE}}]` title, type, and assignment or session details.
- Propose creating an item when nothing matches, or updating only the fields that differ, in the reference's item format. Apply an explicit change, such as an extension announcement, to its item even when the original source is unchanged.
- For an item that has the `[{{COURSE}}]` title prefix but is outside the course group, propose adding it to the group; for an item in the group without the prefix, propose adding the prefix.
- List conflicting sources and uncertain matches for the user to decide.

Without a tracker or `tracker.json`, list the requirements with their sources and say they are not matched to tracker items.

### Weekly checks

Run both checks on every sync, including one with no Canvas changes. The cutoff is the end of Friday in the week after the current one, with weeks starting on Monday in the workspace timezone.

- **Upcoming sessions.** For each required recurring session in the course schedule (syllabus, modules or pages), make sure an item exists for every occurrence through the cutoff, named with its week.
- **This week's work.** Move each item that is not done, is in the to-do status, and is due on or before the cutoff to the This Week status. Leave items with no due date and items already started. Skip this when the reference has no This Week status.

Example: on Wednesday 30 September 2026 the cutoff is Friday 9 October 2026, 2359. The W8 lab on 9 October gets an item; the W9 lab on 16 October waits. A Kaneo item in `to-do` due 8 October moves to `this-week`; one due 12 October stays.

### Examples

| Canvas change | `tracker.json` | Proposed row |
| --- | --- | --- |
| Announcement extends Assignment 1 to 18 September, 1400 | Task due 15 September, 1700 | Update the due date and deadline text, citing the announcement. Jira stores 17 September because the deadline is before 1500. |
| Same announcement | Due date and deadline text already match | None |
| New graded assignment | No matching item | Create a `task` item |
| Required lab moves rooms | Session has the old room | Update the venue only |
| New optional lecture recording | No matching item | None; it stays in the source |
| Fetch failed for an assignment | Task exists | None; report the missing source |
| Any change | `group` is `null` | Create the course group before its items |

## 7. List the proposed rows and ask which to apply

Number the rows from 1, in the order they must be applied: course groups, creates, updates, label changes, then status moves. Merge rows for the same item. Each row states:

- **Action:** create course group, create, update, change labels or move.
- **Item:** its `id`, or the new item's title.
- **Fields:** new values in the tracker's format, with before and after values for updates.
- **Source:** change IDs, raw paths and the reason.

Show the rows, or say that nothing needs to change, with any missing sources, and ask which rows to apply: all, some by number, or none.

## 8. Write the approved rows

Apply only the approved rows, in order. Before the first write in a session, read the write tool's schema or `--help`. Send only the approved fields.

- **Update, label change or move:** first **Get item** by its `id`. When it differs from `tracker.json`, update the file with it and apply the row only if step 6 still calls for it; otherwise skip the row and say why, such as a move for an item the user already finished. When the item no longer exists, remove it from the file and skip the row. Then **Update item** or **Move item**, using **Add label** and **Remove label** for label changes where the reference has them.
- **Create:** **Create item** directly.

After each write, update that item in `tracker.json` from the write's response, including a new item's `id`. When a check or write fails, stop and report that row and the rest as not applied. When a write's outcome is unknown, such as after a timeout, **Get item** (update or move) or **Find item by title** (create) before retrying, and record what you find.

When the rows are done, set `synced_at` to now, run `corum doctor`, and fix what it reports from the write responses. Report each row's result by number, including skipped rows.
