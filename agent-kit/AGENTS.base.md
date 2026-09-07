# Corum Wiki Guide

This vault contains course wikis under `courses/{{COURSE}}/wiki/`. Use the course material in `courses/{{COURSE}}/raw/` as the source for accurate, useful explanations.

## Wiki workflows

| Skill | Use when |
| --- | --- |
| `sync-course` | Synchronizing or reconciling a course wiki |
| `scope-course` | Identifying the course material and wiki work for a synchronization |
| `authoring-wiki` | Creating or revising an explainer, concept, reference page, or course guide |
| `linting-wiki` | Reviewing coverage, links, index consistency, provenance, or writing quality |
| `drawio-diagrams` | Creating or editing a wiki diagram and its editable Draw.io source |

## Course wiki structure

```text
courses/
  {{COURSE}}/
    raw/
    wiki/
      index.md
      explainers/
      concepts/
      references/
      assets/
    state/
      wiki.json
```

Use `wiki/index.md` as the course map. Place narrative learning pages in `explainers/`, focused ideas in `concepts/`, and concise lookup material in `references/`. Store diagram files and other page assets in `assets/`.

## Authoring standards

Build each page from the wiki templates in `templates/wiki/`. Write for a learner who needs a clear explanation, purposeful examples, and connections to related course ideas. Link related pages with paths relative to the current page.

Record source provenance with exact raw-relative paths and page ranges beneath supported headings. Add each published page and each intentionally unrepresented source range to `wiki/index.md`. Maintain `state/wiki.json` as the record of raw sources whose wiki coverage and review are complete.

## Review and completion

Use `linting-wiki` to review every completed source for coverage, provenance, links, index entries, and writing quality. Update the source record in `state/wiki.json` after its pages, index entries, provenance, and review are complete.

## Working paths

Run wiki workflows from the vault root. Interpret `raw/`, `wiki/`, `state/`, and `course.yaml` paths in skills relative to `courses/{{COURSE}}/`. Interpret Markdown links relative to the page containing the link and skill references relative to the skill containing the reference.
