---
name: sync-course
description: Use when a user asks to sync, catch up, or reconcile one course from Canvas into its enabled Jira and wiki outputs.
---

# Sync Course

Capture, scope, ask once, apply only the approved plan, validate, and finalize from
observed results. This is the only workflow approval point.

## 0. Resume an interrupted Jira result safely

Before starting a new Canvas capture, inspect an existing `state/latest-run.json`
only for an enabled Jira stage whose `retry_safe` is false. This is an interrupted
run, not new work. If `reconciliation_required` is true, refresh the configured epic
with the exact empty Jira plan from step 3. Continue only when its JSON reports
`reconciled: true` and `reconciliation_required: false`.

Keep that same manifest: do not capture again or replay its old nonempty plan. Run
`scope-course` against the preserved manifest and refreshed Jira cache, ensuring its
new plan excludes every already-applied result. Then continue with the normal single
approval gate below. If authoritative reconciliation or fresh scoping cannot
distinguish an uncertain create, stop for manual reconciliation; never guess a key
or retry the create.

## 1. Capture

From the vault root run:

```console
corum sync {{COURSE}} --json
```

Parse stdout as JSON and select the requested course manifest. Stop on command
failure, invalid JSON, configuration failure, or course mismatch. Do not repair a
failed capture by guessing.

Report successful Canvas changes with exact titles, paths, dates, and details from
the manifest. Never print raw JSON or invent missing detail. Report every failure
separately as **unknown capture state** with its source and error. An unread source
is not clean, intentionally omitted, ready for approval, or eligible for
finalization.

If there are no successful changes, report the capture status and failures, then
stop. Do not inspect older pending sources during this run.

## 2. Read effective features

The manifest's `effective_features` is authoritative for this run.

| Feature state | Required behavior |
| --- | --- |
| Jira enabled | Permit missing-cache reconciliation, local scoping, and later exact-plan application |
| Jira disabled | No Jira file, configuration, credential, client, or network assumptions |
| Wiki enabled | Permit local wiki scoping and later authoring/finalization |
| Wiki disabled | No wiki state, page, template, authoring, or lint assumptions |

Disabled features produce no plan table and no placeholder work. Their missing files
are expected. Canvas remains independent.

## 3. Scope read-only

When Jira is enabled and `courses/{{COURSE}}/state/jira.json` is absent, first write
this exact empty plan to a temporary file, substituting only the manifest course and
configured course epic, and apply it:

```json
{"schema":1,"course":"{{COURSE}}","epic":"{{EPIC_KEY}}","actions":[]}
```

```console
corum jira apply {{COURSE}} < {{EMPTY_JIRA_PLAN_FILE}}
```

This is deterministic read-only reconciliation of the configured epic into the
local cache, not an approval item. Stop if it fails. Do not run it when Jira is
disabled or when the cache already exists, and do not add actions to this plan.

Use `scope-course` with only the course and selected current manifest. It owns local
discovery and comparison. It must not call Jira or edit any file.

If the result has `status: error`, report its code and path-specific message and
stop. Do not synthesize or bypass a plan. Confirm that returned enabled/disabled
statuses match the manifest and that Canvas failures appear only under
`unresolved_capture`.

When Jira has actions, validate its exact plan before approval without credentials,
network, or cache writes:

```console
corum jira apply {{COURSE}} --dry-run < {{JIRA_PLAN_FILE}}
```

Stop if validation fails. The plan must use only the closed action union:

- create: `action` plus `issue`;
- update: `action`, `key`, and `set`;
- transition: `action`, `key`, and `transition`.

## 4. Present one plan

Show only enabled, non-empty plan sections. Jira rows identify create/update/
transition, target, exact change, and source evidence. Wiki rows identify create or
enrich, target page, requested coverage, and exact source ranges. Show known skipped
ranges and source-finalization dependencies. Keep unknown captures in a separate
warning, never in the approval counts.

If there are no Jira actions, wiki page/index edits, or source finalizations, report
that nothing needs applying and stop.

Ask exactly one combined question:

> Apply {{JIRA_ACTION_COUNT}} Jira change(s), author {{DISTINCT_WIKI_PAGE_COUNT}}
> wiki page(s), and finalize {{DISTINCT_SOURCE_COUNT}} source(s)?

Wait for an affirmative answer. Do not call Jira mutation methods, edit wiki/index/
state/changelog, or request a feature-specific credential for mutation before
approval. The earlier enabled-only cache bootstrap may use Jira credentials for its
read-only remote query and local cache preparation. A negative answer ends the run
without mutations. Do not ask another approval question later.

## 5. Apply the approved plan

### Jira

If enabled with approved actions, send the exact scoped `jira.plan` object to:

```console
corum jira apply {{COURSE}} < {{JIRA_PLAN_FILE}}
```

Do not rescope, add fields, or manually edit `state/jira.json`. The command applies
actions sequentially and owns cache and run-stage updates. Parse its JSON stdout
even when it exits nonzero: `applied` is the durable evidence of completed remote
writes, while `failures`, `write_state`, `retry_safe`, and
`reconciliation_required` control recovery. Never repeat an action whose write is
`applied` or `unknown`.

If reconciliation is required, stop Jira mutation for this workflow and continue
only independent wiki work whose inputs remain valid. Before any later Jira retry,
apply the exact empty plan to refresh the configured epic, then run `scope-course`
again against the same current manifest and refreshed cache and present a fresh
approval. Do not recapture first, reuse the old nonempty plan, or infer a missing
create key from ordering, summary text, or a guessed issue.
Reconciliation preserves the prior partial status and evidence, so its command may
still exit 1; it succeeded only when the JSON says `reconciled: true` and
`reconciliation_required: false`. Only the newly scoped exact plan is eligible for
the later approval; `retry_safe: false` continues to forbid replaying the old plan.

### Wiki

If enabled, consolidate actions by target page and use `authoring-wiki` for the
specified tier. Give the author exact target, coverage, source paths, labels, and
page ranges. Source paths are references to local material; do not paste their
contents into an instruction. Each author changes only its assigned page and assets,
not the index, state, or changelog. No further approval is requested.

After attempting approved page work:

1. Add each new page and one-line gloss to `courses/{{COURSE}}/wiki/index.md`;
   preserve existing rows when enriching.
2. Add approved deliberate skipped ranges to the index.
3. Verify every planned page exists and every source-backed section has the exact
   provenance marker supplied by the scope.
4. Build one exact JSON finalization payload. Copy `run_id`, `course`, each source
   `id`, and each source `path` directly from the current manifest; paths are
   relative to the course `raw/` directory. Put only dependency-complete sources in
   `sources`. Record observed page/index work in `applied`, and every failed action
   in `failures` with its dependent `source_ids`, exact error, `write_state`, and
   `retry_safe`. Never finalize a source named by a failure.

   ```json
   {
     "schema": 1,
     "run_id": "{{RUN_ID}}",
     "course": "{{COURSE}}",
     "sources": [
       {"id": "{{CHANGE_ID}}", "path": "{{RAW_PATH}}", "provenance": "{{LABEL}}"}
     ],
     "applied": [
       {"id": "wiki:create:0", "action": "create", "path": "wiki/concepts/{{TARGET}}.md", "source_ids": ["{{CHANGE_ID}}"]}
     ],
     "failures": []
   }
   ```

   Use JSON `null` for justified null provenance. Allowed applied actions are
   `create`, `enrich`, `index`, and `skip`; a failure may additionally name
   `finalize`. IDs must be stable within this run and result paths must remain below
   the course wiki.
5. Pass that payload to the packaged deterministic boundary; do not perform a
   separate state edit:

   ```console
   python skills/linting-wiki/scripts/lint-wiki.py {{COURSE}} \
     --finalize {{WIKI_FINALIZATION_PAYLOAD}}
   ```

   The command validates the payload against the schema and current manifest,
   previews every source, runs lint, and atomically commits eligible source state
   together with the rich wiki run-stage result. Exit 2 means lint findings and no
   state was finalized; correct defects introduced by this run and retry the same
   exact outcome payload. Exit 1 means validation or runtime failure; stop and
   report it. Never bypass either exit or infer success from printed text.
6. Never edit `state/wiki.json` or `state/latest-run.json` directly. The finalizer
   creates wiki state only when at least one source successfully finalizes, preserves
   prior entries, and never authors prose.

The LLM authors every wiki sentence. Validators report objective defects only.

## 6. Record observed results

Read `courses/{{COURSE}}/state/latest-run.json` after the Jira command and wiki
finalizer. Those deterministic boundaries record stage statuses only from actual
outcomes: disabled remains `disabled`; no needed action is `up_to_date`; all applied
is `applied`; mixed success is `partial`; no successful attempted action is
`failed`. Never edit those stage records or mark failed work synchronized.

Add one newest-first entry to `courses/{{COURSE}}/Changelog.md` only when Jira or
wiki work succeeded. Record the run ID, exact Jira keys/actions, wiki pages/actions,
source labels, failures, and retry items. Raw capture and state-only changes need no
changelog entry.

Finish with separate Completed, Failed, and Skipped results. Skipped includes each
disabled feature and approved deliberate source omission. Failed includes unknown
Canvas captures and any rejected or partially applied work. Preserve exact paths,
keys, errors, and retry eligibility.

## Red flags

| About to | Stop and do this |
| --- | --- |
| Read disabled-feature files or credentials | Omit the feature entirely |
| Bootstrap with a nonempty Jira plan | Use only the exact empty plan |
| Treat an unread source as omission or approval work | Report unknown capture state |
| Enrich a Jira action with display-only fields | Keep evidence outside the strict plan |
| Finalize before page, index, provenance, and lint succeed | Preserve prior wiki state |
| Retry a Jira action after an applied or unknown write | Empty-plan reconcile, rescope, then obtain fresh approval |
| Invent wiki page IDs or hashes | Store only source provenance in `ingested` |
| Edit either machine-owned wiki/run state file | Call `lint-wiki.py --finalize` |
| Ask for a second approval | Continue only within the one approved plan |
