---
name: drawio-diagrams
description: Use when a wiki page needs an editable packet layout, topology, spatial technical diagram, or paired SVG and SVG.XML asset.
---

# Draw.io Diagrams

Create an SVG for Obsidian and an adjacent editable source from the same graph model. This skill manages diagram mechanics; `authoring-wiki` and `sync-course` manage factual content and source coverage.

## Choose the format

| Need | Format |
| --- | --- |
| Exact widths, spatial nesting, or manual editing | Draw.io |
| Protocol exchange, state machine, or branching process | Mermaid |
| Facts without spatial structure | Table |
| Photo, illustration, or bitmap | Image generation |

## Maintain the editable pair

| File | Role |
| --- | --- |
| `name.svg` | Rendered file embedded in Markdown |
| `name.svg.xml` | Canonical editable source recognized by the plugin |

Inspect both files before editing. Use one graph model for the editable source and export the SVG from that model through the plugin or a Draw.io exporter. Update the source and SVG together. Read [Obsidian pair format](references/obsidian-format.md) for compressed sources, safe editing, and plugin recognition.

## Draw the content

- Use sharp 90-degree corners for boxes, cells, and connector turns.
- Put field names and widths inside boxes.
- Put high-value constants in one compact legend line.
- Name payloads by their contents.
- Use blue for link, purple for network, orange or yellow for transport, green for payload, and gray for neutral overhead when a layer palette helps.
- Use colons or sentences for explanations and en dashes for ranges.
- Embed the SVG as `![[courses/{{COURSE}}/wiki/assets/name.svg]]`.
- Record factual sources on the owning page.

## Verify the pair

1. Inspect the Markdown, SVG, and sidecar.
2. Edit the owning concept before derived references.
3. Preserve unrelated cells and user edits.
4. Remove a replaced raster after confirming its embeds have moved to the SVG.
5. Confirm both files exist, parse as XML, and share matching visible labels. Run an XML validator when available.
6. Render and visually inspect the final SVG.
7. Complete the pair when the SVG comes from the final sidecar and the source/render comparison succeeds.
8. Invoke `linting-wiki` after changing the owning page.

Reconcile label differences through the plugin or another Draw.io-compatible editor.
