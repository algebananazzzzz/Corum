---
name: sync-course
description: Use when a user asks to sync, catch up, or reconcile one course from Canvas into its enabled Jira and wiki outputs.
---

# Sync Course

Capture, scope, ask once, apply only the approved plan, and record observed
results. This is the only workflow approval point.

## 0. Resume an interrupted Jira result safely

Before starting a new Canvas capture, inspect `state/latest-run.json` only for an
enabled Jira stage whose `retry_safe` is false. This is an interrupted run, not
new work. If `reconciliation_required` is true, refresh the configured epic with
the exact empty v2 Jira plan from step 3. Continue only when its JSON reports
`reconciled: true` and `reconciliation_required: false`.

Keep the same manifest: do not capture again or replay its old nonempty plan. Run
`scope-course` against the preserved manifest and refreshed Jira cache, ensuring
the new plan excludes every already-applied result. If authoritative
reconciliation cannot distinguish an uncertain create, stop for manual
reconciliation; never guess a key or retry the create.

## 1. Capture

From the vault root run:

```console
corum sync {{COURSE}} --json
```

Parse stdout as JSON and select the requested course result. Stop on command
failure, invalid JSON, configuration failure, or course mismatch. Report
successful Canvas changes with exact titles, paths, dates, and details from the
result. Report every failure separately as **unknown capture state**. An unread
source is not clean, intentionally omitted, ready for approval, or eligible for
wiki ingestion.

If there are no successful changes, report the capture status and failures, then
stop. Do not inspect older pending sources during this run.

## 2. Read effective services

The manifest's `effective_features` object reports the service-presence result for
this run despite its historical field name.

| Service state | Required behavior |
| --- | --- |
| Jira enabled | Permit missing-cache reconciliation, local scoping, and later exact-plan application |
| Jira disabled | No Jira file, configuration, credential, client, or network assumptions |
| Wiki enabled | Permit local wiki scoping, authoring, review, and ingestion records |
| Wiki disabled | No wiki state, page, template, authoring, or review assumptions |

Disabled services produce no plan table and no placeholder work. Their missing
files are expected. Canvas remains independent.

## 3. Scope read-only

When Jira is enabled and `courses/{{COURSE}}/state/jira.json` is absent, write this
exact empty plan to a temporary file, substituting only the course and epic, and
apply it:

```json
{"version":2,"course":"{{COURSE}}","epic":"{{EPIC_KEY}}","actions":[]}
```

```console
corum jira apply {{COURSE}} < {{EMPTY_JIRA_PLAN_FILE}}
```

This is read-only remote reconciliation into the local cache, not an approval
item. Stop if it fails. Do not run it when Jira is disabled or when the cache
already exists, and never add actions to this plan.

Use `scope-course` with only the course and selected current manifest. It owns
local discovery and comparison. It must not call Jira or edit any file. If it
returns `status: error`, report its code and path-specific message and stop.
Confirm that returned enabled/disabled statuses match the manifest and that
Canvas failures appear only under `unresolved_capture`.

When Jira has actions, validate its exact plan before approval without credentials,
network, or state writes:

```console
corum jira apply {{COURSE}} --dry-run < {{JIRA_PLAN_FILE}}
```

The plan must contain `version: 2` and only the closed action union:

- create: `action` plus `issue`;
- update: `action`, `key`, and `set`;
- transition: `action`, `key`, and `transition`.

## 4. Present one plan

Show only enabled, nonempty plan sections. Jira rows identify the exact action,
target, change, and source evidence. Wiki rows identify create or enrich, target
page, requested coverage, and exact source ranges. Show known skipped ranges and
source-ingestion dependencies. Keep unknown captures in a separate warning,
never in approval counts.

If there are no Jira actions, wiki page/index edits, or sources to mark ingested,
report that nothing needs applying and stop.

Ask exactly one combined question:

> Apply {{JIRA_ACTION_COUNT}} Jira change(s), author {{DISTINCT_WIKI_PAGE_COUNT}}
> wiki page(s), and mark {{DISTINCT_SOURCE_COUNT}} source(s) ingested?

Wait for an affirmative answer. Do not call Jira mutation methods, edit wiki,
index, state, or changelog before approval. The earlier empty-plan bootstrap may
use Jira OAuth only for its read-only query. A negative answer ends the run. Do
not ask another approval question later.

## 5. Apply the approved plan

### Jira

If enabled with approved actions, send the exact scoped `jira.plan` object to:

```console
corum jira apply {{COURSE}} < {{JIRA_PLAN_FILE}}
```

Do not rescope, add fields, or manually edit `state/jira.json`. Parse JSON stdout
even when the command exits nonzero. `applied`, `failures`, `write_state`,
`retry_safe`, and `reconciliation_required` are durable recovery evidence. Never
repeat an action whose write is `applied` or `unknown`.

If reconciliation is required, stop Jira mutation and continue only independent
wiki work whose inputs remain valid. Before any later Jira retry, apply the exact
empty plan, scope again against the same manifest and refreshed cache, and present
a fresh approval. Never replay the old plan.

### Wiki

If enabled, consolidate actions by target and use `authoring-wiki` for the
specified tier. Give the author exact target, coverage, source paths, labels, and
page ranges. Source paths are references to local material; do not paste source
contents into an instruction. Each author changes only its assigned page and
assets, not the index, state, or changelog. No further approval is requested.

After approved page work:

1. Add each new page and one-line gloss to `courses/{{COURSE}}/wiki/index.md`;
   preserve existing rows when enriching.
2. Add approved deliberate skipped ranges to the index.
3. Verify every planned page exists and every source-backed section has the exact
   provenance marker supplied by the scope.
4. Use `linting-wiki` for a read-only review of pages, links, index entries,
   provenance, source coverage, bloat, and contradictions. Correct defects
   introduced by this run. An affected source with any unresolved dependency or
   review finding remains un-ingested.
5. Update `courses/{{COURSE}}/state/wiki.json` with `version: 2` and an `ingested`
   object. Preserve prior entries. For each dependency-complete source, set its
   exact raw-relative `path` to its nonblank provenance label or justified JSON
   `null`. Never store page IDs, hashes, remote versions, or prose.
6. Update only the `wiki` stage in the current `state/latest-run.json`, preserving
   every other field. Derive `status`, `applied`, `failures`, and `retry_safe` from
   observed outcomes. Use `applied` only for completed create, enrich, index, skip,
   or ingest work; failures name the exact target and error. Do not mark a source
   ingested when one of its dependencies failed.

There is no code-driven wiki finalizer. The LLM authors every wiki sentence,
performs the review, and records only outcomes it directly observed.

## 6. Record and report observed results

Add one newest-first entry to `courses/{{COURSE}}/Changelog.md` only when Jira or
wiki work succeeded. Record the run ID, exact Jira keys/actions, wiki pages/actions,
source labels, failures, and retry items. Raw capture and state-only changes need
no changelog entry.

Finish with separate Completed, Failed, and Skipped results. Skipped includes each
disabled service and approved deliberate source omission. Failed includes unknown
Canvas captures and rejected or partially applied work. Preserve exact paths,
keys, errors, and retry eligibility.

## Red flags

| About to | Stop and do this |
| --- | --- |
| Read disabled-service files or credentials | Omit the service entirely |
| Bootstrap with a nonempty Jira plan | Use only the exact empty v2 plan |
| Treat an unread source as omission or approval work | Report unknown capture state |
| Enrich a Jira action with display-only fields | Keep evidence outside the strict plan |
| Mark a source ingested before page, index, provenance, and review succeed | Preserve its prior wiki-state value |
| Retry Jira after an applied or unknown write | Empty-plan reconcile, rescope, then obtain fresh approval |
| Invent wiki page IDs or hashes | Store only raw path to provenance in `ingested` |
| Change Canvas/Jira evidence while recording wiki results | Preserve all non-wiki manifest fields |
| Ask for a second approval | Continue only within the one approved plan |
