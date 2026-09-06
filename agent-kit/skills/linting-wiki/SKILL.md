---
name: linting-wiki
description: Use when reviewing a course wiki for coverage, orphans, dangling links, index drift, bloat, or contradictions.
---

# Linting Wiki

This skill is a read-only review workflow. It inspects
`courses/{{COURSE}}/wiki/`, only the raw sources needed for the selected review,
and `courses/{{COURSE}}/state/wiki.json` when present. It never writes wiki prose
or state. The calling author or `sync-course` workflow applies justified fixes and
records observed results.

Read the [Markdown conventions](../authoring-wiki/references/markdown-conventions.md)
before judgment checks.

## Establish the review set

For a sync, review every page, asset, index row, source, and provenance marker
created or changed by the approved plan. Also inspect directly linked pages when
needed to detect dangling links or contradictions. Do not broaden into unrelated
historical cleanup.

For a whole-course audit, enumerate the wiki index and page tree first, then compare
them in both directions. Treat unreadable sources and files as findings, never as
clean inputs.

## Objective checks

| Check | Finds |
| --- | --- |
| Coverage | A planned source range that no provenance or deliberate skipped marker cites |
| Provenance | Missing, blank, malformed, out-of-range, or wrong-source markers |
| Orphans | A concept no explainer or index entry links |
| Dangling | A wiki link or embed that resolves to no file |
| Index drift | A page without an index row, or an index row without a page |
| Tags | Raw HTML that breaks the Markdown contract |
| State | A `wiki.json` entry that is not `version: 2`, is not raw-relative, or names a source whose dependencies did not succeed |

Read each affected source directly. For PDFs, confirm every cited page exists and
each planned range is accounted for. A justified `null` ingestion value still
requires a readable source and a reason why no provenance marker applies. Empty or
whitespace-only labels are invalid.

Any objective finding introduced by the current run must be corrected before the
affected source is added to `state/wiki.json`. Report older gaps without silently
ingesting their raw material.

## Judgment checks

Read flagged pages and the pages linking them.

| Check | Finds | Response |
| --- | --- | --- |
| Bloat | Qualitative table over five rows, paragraph over three sentences, nested callout | Merge dimensions or use a lead-in and one block |
| Contradiction | The same fact differs between concepts | Prefer direct provenance; report unresolved conflict |
| Heavy | Roughly over 200 lines or six content `##` sections | Split only by the configured concept-boundary rule |

A split includes the index and every link to the old page; otherwise leave the page
intact and report the issue. Do not generate replacement prose mechanically: the
LLM must understand and rewrite the content.

## Return findings

Return one table and an overall outcome. Use exact paths and source ranges.

| Page | Check | Finding | Outcome |
| --- | --- | --- | --- |
| `courses/{{COURSE}}/wiki/concepts/{{Concept}}.md` | Coverage | `{{SOURCE}}` range uncited | Must fix; source remains un-ingested |

`clean` means no finding remains in the selected review set. `findings` means at
least one issue remains. Never edit `state/wiki.json` or `latest-run.json` from this
skill; the approved `sync-course` caller owns those narrow outcome records.

## Red flags

| About to | Required response |
| --- | --- |
| Read `raw/` to fill an unrelated prior gap | Report it as a separate future ingest |
| Split without reading `course.yaml` and the boundary reference | Read them first |
| Leave links or index rows pointing to a removed page | Finish the split or revert it |
| Mark a source ingested despite a dependency or review failure | Preserve its prior wiki-state value |
| Write educational prose from a validator or script | Stop; use `authoring-wiki` |
