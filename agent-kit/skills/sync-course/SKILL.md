---
name: sync-course
description: Use when a user asks to sync, catch up, or reconcile one course through its configured Jira and wiki workflows.
---

# Sync Course

Synchronize one course through a single capture, scope, approval, application, and recording flow. The main workflow assumes successful operations. Read [error handling](references/error-handling.md) when a command, plan, or review produces an unexpected result.

## 1. Capture the course

Run this command from the vault root:

```console
corum sync {{COURSE}} --json
```

Select the requested course from the JSON result. The selected manifest supplies the current run’s Canvas changes and `effective_features` workflow-state snapshot.

## 2. Establish workflow states

`effective_features.jira` and `effective_features.wiki` decide whether the Jira and wiki workflows are enabled or disabled for the current run. A configuration change takes effect through a fresh capture, which creates a new manifest snapshot.

| Service | Enabled workflow | Disabled workflow |
| --- | --- | --- |
| Jira | Scope and apply Jira actions. | Omit Jira actions from the plan and preserve prior Jira records. |
| Wiki | Scope, author, review, and record wiki work. | Omit wiki actions from the plan and preserve prior wiki records. |

Jira uses the configured workspace and course Jira settings. Wiki uses the existing course wiki and its state, or initializes an empty wiki when no wiki pages or state exist.

## 3. Scope enabled work

Invoke `scope-course` with the selected manifest. Its output provides the Jira plan, Jira evidence, wiki page actions, source coverage, and ingestion dependencies for the enabled services.

When Jira is enabled and `courses/{{COURSE}}/state/jira.json` is absent, initialize the cache before running the main scope:

```json
{"version":2,"course":"{{COURSE}}","epic":"{{EPIC_KEY}}","actions":[]}
```

```console
corum jira apply {{COURSE}} < {{EMPTY_JIRA_PLAN_FILE}}
```

Build this empty plan from the selected course and its configured epic, then confirm its JSON result before continuing.

## 4. Present one plan

Present Jira Changes and Wiki Changes together. Jira rows include the action, target, change, and source evidence. Wiki rows include the page action, coverage, source paths, provenance labels, and page ranges. Under a disabled workflow heading, write `{{SERVICE}} is disabled for this course.` Under an enabled workflow heading with no actions, write `No {{SERVICE}} changes.`

### Jira Changes

| Action | Target | Planned change | Source evidence |
| --- | --- | --- | --- |
| Create | `{{ISSUE_SUMMARY}}` | Create a task for {{ASSIGNMENT}} due {{DATE_TIME}}. | `{{SOURCE_PATH}}` — `{{REASON}}` |

### Wiki Changes

| Action | Target | Planned change | Source evidence |
| --- | --- | --- | --- |
| Enrich | `{{CONCEPT}}` | Add a section on {{COVERAGE}}. | `{{SOURCE_PATH}}` — `{{LABEL}}`, `{{PAGES}}` |

Ask one combined approval question that states the Jira action count and wiki page count:

> Apply {{JIRA_ACTION_COUNT}} Jira change(s) and author {{DISTINCT_WIKI_PAGE_COUNT}} wiki page(s)?

An affirmative response authorizes the displayed plan.

## 5. Apply Jira work

For approved Jira actions, validate the scoped plan and apply it exactly:

```console
corum jira apply {{COURSE}} --dry-run < {{JIRA_PLAN_FILE}}
corum jira apply {{COURSE}} < {{JIRA_PLAN_FILE}}
```

Record the command’s observed action results in the Jira stage of `state/latest-run.json`.

## 6. Apply wiki work

Consolidate approved wiki actions by target page and invoke `authoring-wiki` for the specified tier. Provide each author with its target, coverage, source paths, provenance labels, and page ranges.

Add each new page and gloss to `courses/{{COURSE}}/wiki/index.md`. Add approved source ranges to the index. Invoke `linting-wiki` to review page content, links, index entries, provenance, coverage, and consistency.

Record each source with complete page, index, provenance, and review work in `courses/{{COURSE}}/state/wiki.json`. Record observed wiki results in the wiki stage of `state/latest-run.json`.

## 7. Record the run

Add a newest-first entry to `courses/{{COURSE}}/Changelog.md` for completed Jira or wiki work. Include the run ID, Jira actions, wiki pages, source labels, and follow-up items. Report the completed Jira and wiki outputs separately.
