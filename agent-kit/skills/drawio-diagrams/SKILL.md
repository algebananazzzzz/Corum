---
name: drawio-diagrams
description: Use when a wiki page needs an editable packet layout, topology, spatial technical diagram, or paired SVG and SVG.XML asset.
---

# Draw.io Diagrams

Create a rendered SVG for Obsidian and an adjacent editable source from the same
graph model. This skill owns diagram mechanics; the active authoring or sync skill
still owns factual content and finalization.

## Choose the format

| Need | Format |
| --- | --- |
| Exact widths, spatial nesting, or manual editing | Draw.io |
| Protocol exchange, state machine, or branching process | Mermaid |
| Facts without meaningful spatial structure | Table |
| Photo, illustration, or bitmap | Image generation |

## Preserve the editable pair

| File | Role |
| --- | --- |
| `name.svg` | Rendered file embedded in Markdown |
| `name.svg.xml` | Canonical editable source recognized by the plugin |

- Inspect both files before editing; sidecars may contain compressed graph models.
- Never rebuild an existing pair merely to change labels. Preserve geometry and style.
- Never hand-author SVG and sidecar as independent layouts. Export the SVG from the
  final graph model through the plugin or a Draw.io exporter.
- Update source and SVG in one operation. Close a stale editor without saving before
  reopening an externally changed pair.
- Use `scripts/drawio_pair.py replace` only for exact single-line labels. Use the
  plugin for geometry, wrapping, or style changes.
- Read [Obsidian pair format](references/obsidian-format.md) for compressed sources,
  programmatic creation, and plugin recognition.

## Draw the content

- Use sharp 90-degree corners for boxes, cells, and connector turns.
- Put field names and widths inside boxes.
- Put only high-value constants in one compact legend line.
- Name payloads by their contents.
- When a layer palette helps, use blue for link, purple for network, orange or yellow
  for transport, green for payload, and gray for neutral overhead.
- Use colons or sentences instead of em dashes; en dashes remain valid for ranges.
- Embed the SVG directly as `![[courses/{{COURSE}}/wiki/assets/name.svg]]`.
- Self-created diagrams need no image attribution; factual sources remain on the page.

## Work safely

1. Inspect the Markdown, SVG, and sidecar.
2. Edit the owning concept before a derived reference.
3. Preserve unrelated cells and user edits.
4. Remove a replaced raster only after proving nothing else embeds it.
5. Strictly validate every created or layout-edited pair:

   ```console
   python skills/drawio-diagrams/scripts/drawio_pair.py validate --strict path/to/name.svg
   ```

6. Render and visually inspect the final SVG when an exporter is available.
7. Do not call the pair complete unless the SVG came from the final sidecar and strict
   validation passes. If no exporter exists, report that a plugin save remains needed.
8. Run the course wiki lint after changing a page.

## Helper commands

```console
python skills/drawio-diagrams/scripts/drawio_pair.py inspect path/to/name.svg
python skills/drawio-diagrams/scripts/drawio_pair.py replace path/to/name.svg 'old' 'new'
python skills/drawio-diagrams/scripts/drawio_pair.py validate --strict one.svg two.svg
```

Stop if the source and SVG do not contain the same label. Edit through the plugin
instead of forcing drift.
