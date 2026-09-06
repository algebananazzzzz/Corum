---
name: scope-course
description: Use when an accepted Corum capture run must be mapped read-only into conditional Jira, wiki, and source-ingestion plans.
---

# Scope Course Changes

Map one supplied `corum sync ... --json` course manifest to an executable plan.
This skill is read-only: do not call Jira, edit files, create issues, author pages,
or invent information missing from the capture.

## Read conditionally

Always read the selected course manifest, `courses/{{COURSE}}/course.yaml`, and
only successful current-run changes under `courses/{{COURSE}}/raw/`. Treat
manifest failures as unknown captures: copy their stable ID, source, raw path when
known, and error to `unresolved_capture`, but do not scope or ingest unseen
contents. Carry every successful change's manifest `id` and exact `raw_path`
through all evidence and source-ingestion records; never reconstruct either from
a title or summary.

| Effective service | Inputs |
| --- | --- |
| Jira enabled | `.config/corum/corum.yaml` Jira conventions, course epic, `state/jira.json`, and [Jira planning](references/jira-board.md) |
| Jira disabled | None: do not read Jira files, configuration, credentials, or network state |
| Wiki enabled | `state/wiki.json`, wiki index/pages, configured split rule, and changed raw sources |
| Wiki disabled | None: do not read wiki state, pages, templates, or raw content for wiki planning |

An enabled Jira cache is a caller-owned precondition. If `state/jira.json` is
absent, return `status: error` with code `jira_cache_missing` and its exact path;
the caller must run the exact empty-plan workflow before retrying. Never reconcile
or call Jira from this skill.

When the manifest carries a reconciled Jira `partial` or `failed` result, use its
`applied` and `failures` as prior-write evidence and the refreshed complete cache
as authoritative state. Produce a new minimal plan. Never reproduce an old action
merely because it failed to return a key; return an error for manual reconciliation
when the cache cannot disambiguate an uncertain create.

When wiki is enabled for the first time, absence of both `state/wiki.json` and
`wiki/` is an empty initial wiki. For any unreadable required enabled input, return
only `status: error` with a stable code and exact path. A disabled service's absent
files are expected and never errors.

## Decide Jira actions

Create only required actionable work, required sessions, or required dated
milestones with no matching cached issue. Update a matching issue only when an
owned value differs. Transition only when an effective configured transition is
needed. Optional work, unknown attendance, lecture files, and recordings produce
no Jira action unless they establish a separate obligation.

The `plan.actions` array is the exact `corum jira apply` payload. Each item is one
member of this closed union; extra fields are forbidden:

- `create`: `action` plus `issue` only.
- `update`: `action`, `key`, and `set` only.
- `transition`: `action`, `key`, and `transition` only.

Put source IDs, raw-relative paths, display titles, reasons, and before/after
detail in the parallel `evidence` array keyed by `action_index`, never inside an
action. Each action must be executable without rereading raw content.

## Decide wiki actions and ingestion

- **Create:** the source teaches a distinct configured-split concept absent from
  the wiki.
- **Enrich:** an existing concept lacks substantive knowledge introduced by the
  source.
- **No Action:** the source is administrative, duplicate, or adds no knowledge.

Scope concepts rather than lectures. One source may support several concepts and
several sources may support one concept. Give every source its manifest ID, exact
raw-relative path, stable provenance label, and exact PDF range. A concept action
states knowledge to cover and evidence ranges, not draft prose.

For every successfully read, ingestion-eligible source provide one outcome:

- `ready`: manifest ID, exact raw-relative path, nonblank provenance label or
  justified `null`, and every page/index dependency that must succeed first.
- `skipped`: manifest ID, path, label, page ranges, and deliberate index reason.
- `ignored`: manifest ID, path, administrative reason, and no wiki-state change.

`state/wiki.json` owns only `version: 2` and `ingested`. Never invent page IDs,
remote IDs, versions, or content hashes. A source becomes ingested only after all
planned pages, index edits, provenance checks, and wiki review succeed.

## Return JSON

Return only valid JSON with this structure:

```json
{
  "status": "ok",
  "course": "{{COURSE}}",
  "unresolved_capture": [
    {
      "id": "{{FAILURE_ID}}",
      "source": "{{SOURCE}}",
      "path": "{{RAW_PATH_OR_NULL}}",
      "error": "{{CAPTURE_ERROR}}"
    }
  ],
  "jira": {
    "status": "enabled",
    "plan": {
      "version": 2,
      "course": "{{COURSE}}",
      "epic": "{{EPIC_KEY}}",
      "actions": [
        {
          "action": "create",
          "issue": {
            "type": "Task",
            "parent": "{{EPIC_KEY}}",
            "summary": "{{COURSE}} {{SUMMARY}}",
            "description": "{{EXACT_DETAILS}}",
            "due": "{{YYYY-MM-DD}}",
            "labels": ["assessment"]
          }
        },
        {
          "action": "update",
          "key": "{{ISSUE_KEY}}",
          "set": {"due": "{{YYYY-MM-DD}}"}
        },
        {
          "action": "transition",
          "key": "{{ISSUE_KEY}}",
          "transition": "{{CONFIGURED_TRANSITION_NAME}}"
        }
      ]
    },
    "evidence": [
      {
        "action_index": 0,
        "source_ids": ["{{CHANGE_ID}}"],
        "sources": ["{{SOURCE_PATH}}"],
        "reason": "{{WHY_REQUIRED}}"
      }
    ]
  },
  "wiki": {
    "status": "enabled",
    "concepts": [
      {
        "action": "enrich",
        "page": "wiki/concepts/{{CONCEPT}}.md",
        "sources": [
          {
            "id": "{{CHANGE_ID}}",
            "path": "{{SOURCE_PATH}}",
            "label": "{{LABEL}}",
            "pages": "p3-18"
          }
        ],
        "coverage": ["{{KNOWLEDGE}}"],
        "reason": "{{WHY_MISSING}}"
      }
    ],
    "ingestion": {
      "ready": [
        {
          "id": "{{CHANGE_ID}}",
          "path": "{{SOURCE_PATH}}",
          "value": "{{LABEL}}",
          "after": ["wiki/concepts/{{CONCEPT}}.md", "wiki/index.md"]
        }
      ],
      "skipped": []
    },
    "ignored": []
  }
}
```

For a disabled service, return `status: disabled`, `plan: null`, and empty
`evidence` for Jira, or empty `concepts`, `ingestion.ready`,
`ingestion.skipped`, and `ignored` arrays for wiki. Use empty arrays for an enabled
section with no actions. Consolidate repeated targets while keeping exact closed
Jira action shapes.
