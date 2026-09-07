---
name: authoring-wiki
description: Use when creating or revising course wiki explainers, concept pages, rapid-lookup references, or course guides.
---

# Authoring Wiki Pages

Read [Markdown conventions](references/markdown-conventions.md) before writing. Use [authoring blocks](references/authoring-markdown.md) to choose the smallest useful representation. For a concept, also read [concept boundaries](references/course-split-rules.md).

The LLM authors the wiki’s educational prose. Deterministic scripts identify defects and support review.

## Choose the target

| Target | Reader | Outcome | | --- | --- | --- | | Explainer | New to the material | Learn progressively | | Concept | Prerequisites known | Understand one concept completely | | Reference | Concepts mastered | Retrieve essential facts quickly | | Course guide | Enrolled student | Retrieve course conventions, schedule, milestones, and policies |

Use one tier per page. Start wiki pages with the matching template under `templates/wiki/` and start course guides with `templates/Conventions and Milestones.md`. Replace each template’s authoring callout with page content.

## Shared standard

- **Accuracy:** preserve course facts and verify researched claims. - **Scope:** organize around the reader's need, not source order. - **Language:** use concise connective prose that serves the tier. - **Structure:** give each block one purpose and use the simplest fitting block. - **Ownership:** link or embed knowledge from its owning page. - **Location:** store pages and figures below `courses/{{COURSE}}/wiki/`. - **Provenance:** give source-backed sections exact `%% {{label}} p{{range}} %%` markers and keep the source list in frontmatter.

## Sources and research

| Source | Role | | --- | --- | | Captured course source | Scope, terminology, emphasis, assessed knowledge | | Reputable secondary source | Framing, boundaries, explanation order, examples | | Primary or official source | Exact behavior, registered values, disputed claims |

Treat a successfully captured course source as trustworthy by default. Compare a concept’s treatment with one reputable secondary source, synthesize an original structure, and use primary sources for precise claims, conflicting accounts, and essential gaps. Cite researched claims where they appear and state meaningful conflicts.

Treat an unread or failed Canvas source as unknown input and report it for retry or manual review.

Use native Markdown for simple facts, Mermaid for processes, `drawio-diagrams` for spatial technical diagrams, and attributed external images for irreducible visuals with permitted reuse. Conclude research when the concept is coherent, essential gaps are filled, and new sources add peripheral detail. References distill concept pages.

## Explainers

Teach as if the reader is new. Order sections by prerequisite, explain why before internals, expand unfamiliar vocabulary, and use concrete examples. End every teaching section with questions immediately followed by answers:

```markdown
### Checkpoint

1. **Question:** {{question answerable from this section}}
   **Answer:** {{concise answer with reasoning}}
```

A checkpoint tests knowledge already taught. The explainer is complete when a beginner can follow it in order and answer each checkpoint from the page alone.

## Concepts

Build around the concept. Link prerequisites. Cover the purpose, mechanism, parts, interactions, constraints, and failure cases needed to reason about the concept. Separate neighboring concepts according to the configured split rule and link them where they interact. Integrate researched knowledge into the same mental model.

A concept is complete when a prepared reader can explain and apply it independently.

## References

Synthesize concept pages around one purpose. Link every contributing concept where its knowledge appears. Remove introductions, transitions, derivations, repeated definitions, and background. Keep formulas, mappings, decision rules, procedures, contrasts, thresholds, high-value exceptions, and answer-changing warnings. Each block must answer a likely lookup question, prevent a likely error, or link to the owning concept.

A reference is complete when the needed fact can be found in seconds and deeper explanation is one backlink away.

## Course guides

Maintain course guides at `courses/{{COURSE}}/Conventions and Milestones.md`. Record grading, weekly rhythm, milestones, late policies, and open course items in the template’s tables and lists. Keep milestones chronological and record captured dates, times, and venues in the workspace timezone. Add unavailable source material and unresolved questions to the Open section for follow-up.

A course guide is complete when students can quickly find the course convention, milestone, or policy that shapes their next action.

## Final review

| Tier | Required proof | | --- | --- | | Explainer | Progressive sections, beginner-safe language, checkpoint after each section | | Concept | Concept-centered synthesis, researched gaps, coherent depth, exact provenance | | Reference | Severe compression, rapid lookup, backlink to every contributing concept | | Course guide | Current grading, rhythm, milestones, policies, and open items in the course template |
