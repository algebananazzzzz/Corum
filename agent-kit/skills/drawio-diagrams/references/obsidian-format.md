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

A one-line sidecar is not necessarily corrupt. Inspect it with the helper rather
than replacing its graph.

## Safe edit boundary

| Change | Method |
| --- | --- |
| Exact single-line label | `drawio_pair.py replace` |
| Position, size, color, wrapping | Plugin editor |
| New programmatic layout | Generate one graph model, then export its SVG |
| Missing or stale render | Open the SVG in the plugin and save |

If a changed pair is open, close the stale editor without saving and reopen it.

## Table structure

- A `shape=table` cell contains `shape=tableRow` children.
- A table row has `value=""`.
- Visible text lives in `shape=partialRectangle` children.
- Never put visible row text on `tableRow`; it may rotate or collapse.

## Validation

```console
python skills/drawio-diagrams/scripts/drawio_pair.py validate --strict path/to/diagram.svg
xmllint --noout path/to/diagram.svg path/to/diagram.svg.xml
```

Strict validation checks pair presence, XML, payload decoding, tables, label order,
and the no-em-dash rule. It does not replace visual inspection.
