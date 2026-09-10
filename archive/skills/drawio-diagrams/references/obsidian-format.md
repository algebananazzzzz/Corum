# Obsidian Draw.io Pair Format

The plugin recognizes an editable SVG through its companion XML sidecar.

| Rendered | Editable source |
| --- | --- |
| `diagram.svg` | `diagram.svg.xml` |

The Markdown page embeds the SVG. Editing loads the sidecar and saving updates both files.

## Source encodings

| State | Shape |
| --- | --- |
| Fresh or generated | `diagram` directly contains `mxGraphModel` |
| Plugin-saved | URI-encoded XML, raw-DEFLATE compressed, then Base64 encoded |

Open each sidecar with a Draw.io-compatible editor and preserve its graph model.

## Edit methods

| Change | Method |
| --- | --- |
| Exact single-line label | Plugin or Draw.io-compatible editor |
| Position, size, color, wrapping | Plugin or Draw.io-compatible editor |
| New layout | Create one graph model, then export its SVG |
| Missing or stale render | Open the SVG in the plugin and save |

Open the current pair in the editor after external changes.

## Table structure

- A `shape=table` cell contains `shape=tableRow` children.
- A table row uses `value=""`.
- Visible text lives in `shape=partialRectangle` children.

## Validation

Confirm pair presence, parseable XML, payload decoding, table structure, label agreement, and the explanation punctuation. Use an XML validator when available, then render and visually inspect the SVG.
