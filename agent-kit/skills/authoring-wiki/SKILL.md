---
name: authoring-wiki
description: Use when creating or revising course wiki concept pages or beginner explainers.
---

# Authoring Wiki Pages

Read [Authoring Markdown](references/authoring-markdown.md) for syntax and block choices. Use the [complete example](examples/arp.md) for pacing and density; shape each page around its own subject.

## Page Types

| Target | Reader | Outcome |
| --- | --- | --- |
| Concept | Prerequisites known | Understand one concept completely |
| Explainer | New to the material | Learn progressively |

Use the matching template under `templates/wiki/`. Retain its metadata and shape the body around the subject.

## Shared Standard

- **Ownership:** link existing prerequisite pages and embed assets using paths that resolve from the output.
- **Location:** store pages and figures below `courses/{{COURSE}}/wiki/`.
- **Provenance:** follow [Metadata and Provenance](references/authoring-markdown.md#metadata-and-provenance); keep the source inventory in frontmatter.

## Sources

| Source | Role |
| --- | --- |
| Captured course source | Required coverage, terminology, emphasis, assessed knowledge |
| Reputable secondary source | Teaching model, logical structure, explanations, examples, visuals |
| Primary or official source | Exact behavior, registered values, disputed claims |

Cite researched claims beside their explanations. Report unread sources as pending input. Research is sufficient when the concept is coherent, scoped questions are answered, and essential gaps are filled. Use the Markdown reference to select visuals and the `drawio-diagrams` skill for editable spatial diagrams.

## Concepts

Lecture material sets required coverage; a suitable secondary explanation guides the teaching.

1. **Scope:** read slides and discussion questions. For graphical pages, render and inspect the page image alongside extracted text. Save a working map before drafting, with one row per teaching point: source range, claim or question with its condition, destination heading. Include diagram branches and worked examples; record administrative slides as skipped.
2. **Research:** open and read a suitable secondary explanation of the named concept. Consider GeeksforGeeks, university teaching notes, or an established tutorial; choose for clarity, depth, and fit.
3. **Structure:** save an outline beside the map: each heading, the idea it teaches, and its chosen block or worked example. Use the secondary source's teaching model to shape the explanation within the page conventions. Read another source for essential gaps.
4. **Author:** open with a one-line definition followed immediately by the defining mechanism. Place motivation and prerequisites beside the mechanism they clarify. Integrate variants, limitations, and course details where they belong. Give each fact one home; use later references when another explanation needs it.
5. **Reconcile:** check each map row against the written explanation. For rules, match both the triggering condition and outcome; for diagrams, check labeled values and worked examples. Verify discrepancies with primary sources and state corrections beside the affected material.

> [!example] ARP
> **Opening:** definition → Request and Reply.
>
> **Message Format:** diagram for fields and common values → table of one concrete request and reply, with each host's addresses filled in.
>
> **Proxy ARP:** explain why the requester's subnet mask makes a remote host appear local.

A teaching question is covered when its answer is explained. Match each working-map row to the actual passage that teaches it; record remaining gaps as draft work.

Finish when the page teaches the concept independently, the secondary source has shaped that teaching, and coverage, links, and provenance pass the final review. If secondary research is unavailable, report it as outstanding.

Validate course-page markers by running `python scripts/validate_provenance.py {{PAGE}}` from this skill directory. Resolve reported errors before delivery; the working map establishes semantic coverage.

## Explainers

Use the same research and coverage process, with a beginner's reading path. Order sections by prerequisite, introduce purpose before internals, expand unfamiliar vocabulary, and use concrete examples. End each teaching section with a question immediately followed by its answer:

```markdown
### Checkpoint

**Question:** which address does a laptop resolve when sending through a gateway?

**Answer:** the gateway's local IP address, because the next Ethernet frame must reach the gateway.
```

The explainer is complete when a beginner can follow it in order and answer each checkpoint from the page alone.
