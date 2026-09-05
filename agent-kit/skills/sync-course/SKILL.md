---
name: sync-course
description: Use when a user asks to sync, catch up, or reconcile one course from Canvas into its enabled Jira and wiki outputs.
---

# Sync Course

Capture, scope, ask once, apply only the approved plan, validate, and finalize from
observed results. This is the only workflow approval point.

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
actions sequentially and owns cache updates. Record each returned key. A failed
action remains failed; continue only independent work whose inputs remain valid.

### Wiki

If enabled, consolidate actions by target page and use `authoring-wiki` for the
specified tier. Give the author exact target, coverage, source paths, labels, and
page ranges. Source paths are references to local material; do not paste their
contents into an instruction. Each author changes only its assigned page and assets,
not the index, state, or changelog. No further approval is requested.

After successful page work:

1. Add each new page and one-line gloss to `courses/{{COURSE}}/wiki/index.md`;
   preserve existing rows when enriching.
2. Add approved deliberate skipped ranges to the index.
3. Verify every planned page exists and every source-backed section has the exact
   provenance marker supplied by the scope.
4. Run pre-finalization lint with every planned source as an in-memory preview. Use
   `--pending` for a nonempty label and `--pending-null` for a justified null:

   ```console
   python skills/linting-wiki/scripts/lint-wiki.py {{COURSE}} \
     --pending '{{LABEL}}={{SOURCE_PATH}}'
   python skills/linting-wiki/scripts/lint-wiki.py {{COURSE}} \
     --pending-null '{{SOURCE_PATH}}'
   ```

5. Correct defects introduced by this run. A page, index, provenance, or lint
   failure leaves every dependent source unfinalized and preserves its prior state.
6. Only after all dependencies pass, update that source under `ingested` in
   `courses/{{COURSE}}/state/wiki.json`. This state has only `schema: 1` and the
   source-to-provenance `ingested` mapping. Never add page IDs, remote IDs, versions,
   or content hashes. Create the file on first successful finalization only.

The LLM authors every wiki sentence. Validators report objective defects only.

## 6. Record observed results

Update `courses/{{COURSE}}/state/latest-run.json` stage statuses only from actual
outcomes: disabled remains `disabled`; no needed action is `up_to_date`; all applied
is `applied`; mixed success is `partial`; no successful attempted action is
`failed`. Never mark failed work synchronized.

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
| Invent wiki page IDs or hashes | Store only source provenance in `ingested` |
| Ask for a second approval | Continue only within the one approved plan |
