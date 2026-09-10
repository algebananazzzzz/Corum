---
name: scope-course
description: Use when a selected Corum capture manifest needs structured Jira and wiki plans for its enabled workflows.
---

# Scope Course Changes

Turn one selected `corum sync ... --json` manifest into structured Jira and wiki plans. This skill reads the manifest and course material, then returns the plan JSON. Read [error handling](references/error-handling.md) when a required input or prior result needs follow-up.

## 1. Read the current run

Read the selected manifest, `courses/{{COURSE}}/course.yaml`, and the successful current-run sources under `courses/{{COURSE}}/raw/`. Carry each source’s manifest `id`, exact `raw_path`, provenance label, and page range through the plan. Record unsuccessful captures in `unresolved_capture` with their ID, source, path, and error.

## 2. Establish workflow states

`effective_features.jira` and `effective_features.wiki` decide whether the Jira and wiki workflows are enabled or disabled for the selected manifest.

| Workflow | Enabled inputs | Disabled result |
| --- | --- | --- |
| Jira | Vault Jira settings, course epic, `state/jira.json`, and [Jira planning](references/jira-board.md) | `status: disabled`, `plan: null`, and empty evidence |
| Wiki | `state/wiki.json`, wiki pages and index, and captured sources | `status: disabled` with empty concept, ingestion, and ignored lists |

An enabled wiki with no state or pages begins as an empty wiki. An enabled Jira workflow uses the reconciled Jira cache supplied by `sync-course`.

## 3. Plan Jira actions

Create Jira actions for required work, required sessions, required dated milestones, and changed course obligations. Use the matching cached issue to produce an update or transition for a changed owned value. Read [Jira planning](references/jira-board.md) for issue types, dates, details, and labels.

Build `jira.actions` as an approval table describing the exact Jira MCP call for each action:

| Action | Fields |
| --- | --- |
| `create` | `action`, `issue` |
| `update` | `action`, `key`, `set` |
| `transition` | `action`, `key`, `transition` |

Put source IDs, raw paths, display titles, reasons, and before/after detail in `evidence`, keyed by `action_index`. Each action and its evidence together show the course change and its source.

## 4. Plan wiki actions

Create a wiki action when a source teaches a distinct concept. Enrich a wiki action when an existing concept gains substantive knowledge. Record administrative or duplicate sources as `ignored` with their reason.

Scope concepts around learner questions. A source can support several concepts, and several sources can support one concept. Each concept action identifies its page, source paths, provenance labels, page ranges, coverage, and reason.

For each ingestion-eligible source, record one outcome: `ready` with its page and index dependencies, `skipped` with its page ranges and reason, or `ignored` with its administrative reason. Completed ingestion follows successful page, index, provenance, and review work.

## 5. Return the plan

Return valid JSON in this shape:

```json
{
  "status": "ok",
  "course": "{{COURSE}}",
  "unresolved_capture": [
    {"id":"{{FAILURE_ID}}","source":"{{SOURCE}}","path":"{{RAW_PATH_OR_NULL}}","error":"{{CAPTURE_ERROR}}"}
  ],
  "jira": {
    "status": "enabled",
    "plan": {
      "version": 2,
      "course": "{{COURSE}}",
      "epic": "{{EPIC_KEY}}",
      "actions": [
        {"action":"create","issue":{"type":"Task","parent":"{{EPIC_KEY}}","summary":"{{COURSE}} {{SUMMARY}}","description":"{{DETAILS}}","due":"{{YYYY-MM-DD}}","labels":["assessment"]}},
        {"action":"update","key":"{{ISSUE_KEY}}","set":{"due":"{{YYYY-MM-DD}}"}},
        {"action":"transition","key":"{{ISSUE_KEY}}","transition":"{{TRANSITION_NAME}}"}
      ]
    },
    "evidence": [
      {"action_index":0,"source_ids":["{{CHANGE_ID}}"],"sources":["{{SOURCE_PATH}}"],"reason":"{{REASON}}"}
    ]
  },
  "wiki": {
    "status": "enabled",
    "concepts": [
      {"action":"enrich","page":"wiki/concepts/{{CONCEPT}}.md","sources":[{"id":"{{CHANGE_ID}}","path":"{{SOURCE_PATH}}","label":"{{LABEL}}","pages":"p3-18"}],"coverage":["{{KNOWLEDGE}}"],"reason":"{{REASON}}"}
    ],
    "ingestion": {
      "ready": [{"id":"{{CHANGE_ID}}","path":"{{SOURCE_PATH}}","value":"{{LABEL}}","after":["wiki/concepts/{{CONCEPT}}.md","wiki/index.md"]}],
      "skipped": []
    },
    "ignored": []
  }
}
```

Use empty arrays for an enabled workflow with no planned actions. Consolidate repeated Jira and wiki targets while preserving every source record.
