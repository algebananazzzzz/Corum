---
name: linting-wiki
description: Use when linting or auditing a course wiki for coverage, orphans, dangling links, index drift, bloat, or contradictions.
---

# Linting Wiki

Read only `courses/{{COURSE}}/wiki/` and `courses/{{COURSE}}/state/wiki.json`.
The LLM applies justified fixes; the deterministic script reports objective defects
and never generates replacement prose.

Read the [Markdown conventions](../authoring-wiki/references/markdown-conventions.md)
before judgment checks.

## Mechanical checks

From the vault root run:

```console
python skills/linting-wiki/scripts/lint-wiki.py {{COURSE}}
```

| Check | Finds |
| --- | --- |
| Coverage | Pages of a finalized PDF source that no provenance or skipped marker cites |
| Orphans | A concept no explainer links |
| Dangling | A wiki link that resolves to no file |
| Index drift | Disk page without an index row, or row without a page |
| Tags | Raw HTML that breaks Obsidian rendering |

`sync-course` runs this after approved page and index edits. Any error introduced by
the run must be corrected before affected sources are finalized. A coverage finding
for an affected source means the authoring/finalization plan is incomplete.

Before finalization, preview each planned source in memory without changing state:

```console
python skills/linting-wiki/scripts/lint-wiki.py {{COURSE}} --pending '{{LABEL}}={{SOURCE_PATH}}'
```

Repeat `--pending` for multiple sources. This makes provenance and skipped ranges
checkable before `state/wiki.json` advances.

## Judgment checks

Read flagged pages and pages linking them.

| Check | Finds | Fix |
| --- | --- | --- |
| Bloat | Qualitative table over five rows, paragraph over three sentences, nested callout | Merge dimensions or use a lead-in and one block |
| Contradiction | The same fact differs between concepts | Prefer the statement with direct provenance; report unresolved conflict |
| Heavy | Roughly over 200 lines or six content `##` sections | Split only by the configured concept-boundary rule |

Fix objective and judgment defects introduced by the current run. Report older
coverage gaps rather than silently ingesting raw material. A split includes the
index and every link to the old page; otherwise leave it intact and report it.

## Report

| Page | Check | Finding | Outcome |
| --- | --- | --- | --- |
| `courses/{{COURSE}}/wiki/concepts/{{Concept}}.md` | Coverage | `{{SOURCE}}` range uncited | Reported; source remains unfinalized |

## Red flags

| About to | Required response |
| --- | --- |
| Read `raw/` to fill an unrelated prior gap | Report it as a separate ingest |
| Split without reading `course.yaml` and the boundary reference | Read them first |
| Leave links or index rows pointing to a removed page | Finish the split or revert it |
| Finalize despite index, provenance, or lint failure | Preserve the prior wiki-state value |
