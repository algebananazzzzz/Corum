# Obsidian Draw.io Pair Format

The plugin recognizes an SVG as editable only when an XML sidecar exists at the
exact companion path.

| Rendered | Editable source |
| --- | --- |
| `diagram.svg` | `diagram.svg.xml` |

The Markdown page embeds only the SVG. Editing loads the sidecar; saving rewrites
both files.

## Source encodings

| State | Shape |
| --- | --- |
| Fresh or generated | `diagram` directly contains `mxGraphModel` |
| Plugin-saved | URI-encoded XML, raw-DEFLATE compressed, then Base64 encoded |

A one-line sidecar is not necessarily corrupt. Open it with the Draw.io-compatible
editor rather than replacing its graph.

## Safe edit boundary

| Change | Method |
| --- | --- |
| Exact single-line label | Plugin or Draw.io-compatible editor |
| Position, size, color, wrapping | Plugin or Draw.io-compatible editor |
| New layout | Create one graph model, then export its SVG |
| Missing or stale render | Open the SVG in the plugin and save |

If a changed pair is open, close the stale editor without saving and reopen it.

## Table structure

- A `shape=table` cell contains `shape=tableRow` children.
- A table row has `value=""`.
- Visible text lives in `shape=partialRectangle` children.
- Never put visible row text on `tableRow`; it may rotate or collapse.

## Validation

Confirm pair presence, parseable XML, payload decoding, table structure, label
agreement, and the no-em-dash rule. Use an XML validator when one is available,
then render and visually inspect the SVG; textual validation never replaces visual
inspection.
