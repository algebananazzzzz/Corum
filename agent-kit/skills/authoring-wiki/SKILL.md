---
name: authoring-wiki
description: Use when creating or revising course wiki explainers, concept pages, or rapid-lookup references.
---

# Authoring Wiki Pages

Read [Markdown conventions](references/markdown-conventions.md) before writing. Use
[authoring blocks](references/authoring-markdown.md) to choose the smallest useful
representation. For a concept, also read
[concept boundaries](references/course-split-rules.md).

The LLM is the wiki's only author. Deterministic scripts may identify defects, but
must never generate or replace educational prose.

## Choose the target

| Target | Reader | Outcome |
| --- | --- | --- |
| Explainer | New to the material | Learn progressively |
| Concept | Prerequisites known | Understand one concept completely |
| Reference | Concepts mastered | Retrieve essential facts quickly |

Do not blend tiers. Use the corresponding template under `templates/wiki/` and
remove its authoring callout when instantiating it.

## Shared standard

- **Accuracy:** preserve course facts and verify researched claims.
- **Scope:** organize around the reader's need, not source order.
- **Language:** remove filler; use connective prose only when it helps the tier.
- **Structure:** give each block one purpose and use the simplest fitting block.
- **Ownership:** link or embed knowledge owned by another page instead of copying it.
- **Location:** write pages and figures only below `courses/{{COURSE}}/wiki/`.
- **Provenance:** give source-backed sections exact `%% {{label}} p{{range}} %%`
  markers and keep the source list in frontmatter.

## Sources and research

| Source | Role |
| --- | --- |
| Captured course source | Scope, terminology, emphasis, assessed knowledge |
| Reputable secondary source | Framing, boundaries, explanation order, examples |
| Primary or official source | Exact behavior, registered values, disputed claims |

Treat a successfully captured course source as trustworthy by default. Compare a
concept's treatment with one reputable secondary source, synthesize an original
structure, and use a primary source only when precision matters, sources conflict,
or a necessary gap remains. Cite researched claims where they appear and state
meaningful conflicts.

An unread or failed Canvas source is unknown input. Do not infer, summarize, skip,
approve, or mark its contents ingested. Report it for retry or manual review.

Use native Markdown for simple facts, Mermaid for processes, `drawio-diagrams` for
spatial technical diagrams, and an attributed external image only when it cannot be
recreated effectively and reuse is permitted. Stop researching once the concept is
coherent, necessary gaps are filled, and additional sources add peripheral detail.
References introduce no new research; they distill concept pages.

## Explainers

Teach as if the reader is new. Order sections by prerequisite, explain why before
internals, expand unfamiliar vocabulary, and use concrete examples. End every
teaching section with questions immediately followed by answers:

```markdown
### Checkpoint

1. **Question:** {{question answerable from this section}}
   **Answer:** {{concise answer with reasoning}}
```

A checkpoint tests knowledge already taught. The explainer is complete when a
beginner can follow it in order and answer each checkpoint from the page alone.

## Concepts

Build around the concept, not the lecture. Link prerequisites instead of reteaching
them. Cover the purpose, mechanism, parts, interactions, constraints, and failure
cases needed to reason about the concept. Separate neighboring concepts according
to the configured split rule and link them where they interact. Integrate researched
knowledge into the same mental model instead of appending detached notes.

A concept is complete when a prepared reader can explain and apply it without
returning to the source.

## References

Synthesize concept pages around one purpose. Link every contributing concept where
its knowledge appears. Remove introductions, transitions, derivations, repeated
definitions, and background. Keep formulas, mappings, decision rules, procedures,
contrasts, thresholds, high-value exceptions, and answer-changing warnings. Each
block must answer a likely lookup question, prevent a likely error, or link to the
owning concept.

A reference is complete when the needed fact can be found in seconds and deeper
explanation is one backlink away.

## Final review

| Tier | Required proof |
| --- | --- |
| Explainer | Progressive sections, beginner-safe language, checkpoint after each section |
| Concept | Concept-centered synthesis, researched gaps, coherent depth, exact provenance |
| Reference | Severe compression, rapid lookup, backlink to every contributing concept |
