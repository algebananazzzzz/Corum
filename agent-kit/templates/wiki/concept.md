---
tier: concept
course: "{{COURSE}}"
sources:
  - "[[courses/{{COURSE}}/raw/{{SOURCE_PATH}}]]"
  - "{{BACKBONE_SOURCE_URL}}"
prereqs:
  - "[[courses/{{COURSE}}/wiki/concepts/{{PREREQUISITE}}]]"
---

# {{CONCEPT}}

> [!note]
> Intermediate tier: deep and concise. Complete every successfully read source in
> frontmatter; each page range belongs below a section or in the index's deliberate
> skipped table. Put an exact `%% {{SOURCE_LABEL}} p{{RANGE}} %%` below each supported
> heading. Use the authoring-wiki Markdown rules. Practice uses `[!tool]` for commands
> and observations and `[!example]` for worked examples. `[!research]` names material
> outside captured course sources. Order sections by the mechanism's lifecycle or
> path, not the source outline. Strip this callout when instantiating.

> [!abstract]
> {{WHAT_IT_IS_AND_WHY_IT_EXISTS}}

---

## {{MECHANISM_PART}}
%% {{SOURCE_LABEL}} p{{RANGE}} %%

| Dimension | Value |
| --- | --- |
| Mechanism | {{MECHANISM}} |
| When | {{WHEN}} |
| Gotcha | {{GOTCHA}} |

> [!example]
> {{WORKED_EXAMPLE}}

> [!tool]
> `{{COMMAND}}` — {{WHAT_TO_OBSERVE}}

## {{WIRE_OR_DATA_PART}}
%% {{SOURCE_LABEL}} p{{RANGE}} %%

| Field | Width | Value |
| --- | --- | --- |
| {{FIELD}} | {{WIDTH}} | {{VALUE}} |

> [!warning]
> {{ANSWER_CHANGING_MISTAKE}}

> [!research]
> {{RESEARCH_SOURCE}}. {{NECESSARY_GAP_FILLED}}

---

## Sources

| Source | Role |
| --- | --- |
| [[courses/{{COURSE}}/raw/{{SOURCE_PATH}}]] | Course completeness baseline |
| [{{BACKBONE_TITLE}}]({{BACKBONE_SOURCE_URL}}) | Structure and framing |
| {{PRIMARY_SOURCE}} | Precision or conflict resolution |
