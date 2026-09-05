---
name: linting-wiki
description: Use when linting or auditing a course wiki for coverage, orphans, dangling links, index drift, bloat, or contradictions.
---

# Linting Wiki

Ordinary lint and preview modes read only `courses/{{COURSE}}/wiki/`, raw sources,
and `courses/{{COURSE}}/state/wiki.json`. The explicit `--finalize` mode is the only
writer: after schema, current-run, source, path, and lint validation, it atomically
records wiki state and the wiki stage in `state/latest-run.json`. The LLM applies
justified content fixes; the deterministic script never generates replacement prose.

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

For a deliberately null provenance value, preview its readable source explicitly:

```console
python skills/linting-wiki/scripts/lint-wiki.py {{COURSE}} --pending-null '{{SOURCE_PATH}}'
```

Repeat either option for multiple sources. The command verifies every source can be
read, every PDF can be inspected, and every cited PDF page exists. This makes
provenance and skipped ranges checkable before `state/wiki.json` advances. Empty or
whitespace-only labels are invalid; use `--pending-null` for an intentional null.

## Exact finalization

`sync-course` owns construction of the exact schema-1 finalization payload from the
current run manifest and observed authoring results. Commit it only through:

```console
python skills/linting-wiki/scripts/lint-wiki.py {{COURSE}} \
  --finalize {{WIKI_FINALIZATION_PAYLOAD}}
```

The payload contains exactly `schema`, `run_id`, `course`, `sources`, `applied`, and
`failures`. Every source uses its manifest change `id`, exact raw-relative `path`,
and provenance string or `null`; result records reference manifest IDs through
`source_ids`. A failed dependency cannot also appear in `sources`. Applied paths
must already exist below the course wiki. Do not edit either machine-owned state
file yourself.

Exit 0 means clean lint or a committed finalization. Exit 2 means objective findings
and no finalization commit. Exit 1 means invalid input, mismatched current state, or
a runtime failure. Treat both nonzero exits as failures; never infer success from
partial stdout.

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
| Directly edit wiki or latest-run state | Use the schema-validating `--finalize` mode |
