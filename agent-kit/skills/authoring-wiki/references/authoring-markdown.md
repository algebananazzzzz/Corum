# Authoring with Markdown

The syntax contract is [Markdown conventions](markdown-conventions.md). This guide selects the block that best exposes the knowledge.

## Quick reference

| Knowledge shape | Block |
| --- | --- |
| One concept and meaning | `**Term** — definition` |
| Unordered family | Bulleted list with bold starters |
| Ordered mechanism | Numbered list |
| Repeated comparison dimensions | Table |
| Actor messages or state changes | Mermaid process diagram |
| Spatial structure or packet layout | Editable Draw.io pair |
| Command and observation | `[!tool]` |
| Worked application | `[!example]` |
| Outside-course evidence | `[!research]` with source on first line |
| Likely harmful mistake | `[!warning]` |
| Irreducible visual evidence | `[!figure]` with caption |

## Selection rules

Use prose to introduce, connect, or interpret structured information. Turn parallel facts into a list. Use numbered lists for sequences. Use tables for subjects that share stable comparison dimensions, with concise cells. Use task lists for durable completion states.

Links point to supporting knowledge. Embeds place required knowledge in the current reading path while preserving one owning page. Footnotes hold optional qualifications. Keep definitions and answer-changing exceptions in the page body.

Mermaid earns its space when message order, branching, or state transition is harder to follow linearly. Disconnected properties belong in a list or table. Spatial layout, topology, nesting, and technical structure use `drawio-diagrams`.

## Complete pattern

```markdown
## Sliding Window
%% {{SOURCE}} p18-22 %%

**Sliding window** — limits how much unacknowledged data a sender may transmit.

- **Larger Window:** permits more data to remain in flight.
- **Smaller Window:** reduces the sender's outstanding data.

> [!example]
> With a 4 KB window and 1 KB segments, at most four segments remain unacknowledged.

> [!warning]
> Window capacity and segment count are related but not interchangeable units.
```

The definition, consequences, example, warning, and exact provenance each have one reading role. Apply the same single-purpose discipline to every page block.
