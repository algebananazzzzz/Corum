---
name: linting-wiki
description: Use when reviewing a course wiki for coverage, provenance, links, index consistency, writing quality, or contradictions.
---

# Review a Course Wiki

Review the selected course wiki and return clear findings. `sync-course` or the page author applies the resulting fixes and records the completed work. Read [Authoring Markdown](../authoring-wiki/references/authoring-markdown.md) before reviewing page structure.

## 1. Establish the review set

For a sync review, inspect every page, asset, index row, source, and provenance marker created or changed by the approved plan. Include directly linked pages when their links or shared facts affect the review.

For a course audit, enumerate the index and page tree, compare them in both directions, and inspect the source material supporting each page. Record unavailable sources and files as findings.

## 2. Run objective checks

| Check | Finding |
| --- | --- |
| Coverage | A planned source range lacks a provenance marker or deliberate skipped marker. |
| Provenance | A marker has a missing, blank, malformed, out-of-range, or wrong-source value. |
| Orphans | A concept lacks an explainer or index link. |
| Dangling links | A wiki link or embed has no matching file. |
| Index consistency | A page or index row lacks its corresponding entry. |
| Markdown | Raw HTML or rendering syntax conflicts with the Markdown conventions. |
| Wiki state | An ingestion record lacks its raw-relative source or completed dependencies. |

Read each affected source directly. Confirm cited PDF pages and planned ranges. A justified `null` ingestion value includes a readable source and a provenance rationale.

## 3. Run judgment checks

| Check | Finding | Recommended response |
| --- | --- | --- |
| Bloat | A large qualitative table, long introductory paragraph, or nested callout obscures the knowledge. | Use a lead-in and focused structured block. |
| Contradiction | The same fact differs between related pages. | Use direct provenance and report unresolved conflicts. |
| Heavy concept | A concept exceeds the course split threshold. | Apply the configured concept-boundary rule. |

Read flagged pages and their linking pages. When a page needs restructuring, invoke `authoring-wiki` and update its index entry and links with the page work.

## 4. Return findings

Return one findings table and an overall outcome. Use exact paths and source ranges.

| Page | Check | Finding | Outcome |
| --- | --- | --- | --- |
| `courses/{{COURSE}}/wiki/concepts/{{CONCEPT}}.md` | Coverage | `{{SOURCE}}` range uncited | Fix before ingestion. |

Use `clean` when the selected review set has no findings. Use `findings` when one or more issues remain. The calling workflow records the reviewed source and run outcome.
