#!/usr/bin/env python3
"""Inspect, validate, and safely replace labels in Obsidian Draw.io pairs."""

from __future__ import annotations

import argparse
import base64
import binascii
import html
import os
from pathlib import Path
import re
import sys
import tempfile
import urllib.parse
import xml.etree.ElementTree as ET
import zlib


EM_DASH = "\N{EM DASH}"
ENCODE_SAFE = "~()*!.'-_"


class PairError(ValueError):
    """A Draw.io pair is missing, malformed, or unsafe to edit."""


def sidecar_path(svg_path: Path) -> Path:
    return Path(f"{svg_path}.xml")


def parse_xml(path: Path, text: str) -> ET.Element:
    try:
        return ET.fromstring(text)
    except ET.ParseError as error:
        raise PairError(f"{path}: invalid XML: {error}") from error


def plain_label(value: str) -> str:
    plain = html.unescape(re.sub(r"<[^>]+>", " ", value))
    return " ".join(plain.replace("\u00a0", " ").split())


def label_tokens(value: str) -> list[str]:
    return re.findall(r"[A-Za-z0-9./+():→-]+", plain_label(value))


def local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def visible_label_nodes(node: ET.Element):
    """Yield complete rendered label containers without nested label fragments."""
    name = local_name(node.tag)
    if name == "text":
        yield node
        return
    if name == "div":
        has_nested_div = any(
            local_name(child.tag) == "div"
            for child in node.iter()
            if child is not node
        )
        if has_nested_div:
            for child in node:
                yield from visible_label_nodes(child)
        else:
            yield node
        return
    if name in {"span", "tspan"}:
        yield node
        return
    for child in node:
        yield from visible_label_nodes(child)


def replace_element_text(node: ET.Element, new: str) -> None:
    slots: list[tuple[ET.Element, str]] = []

    def collect(element: ET.Element) -> None:
        if element.text is not None:
            slots.append((element, "text"))
        for child in element:
            collect(child)
            if child.tail is not None:
                slots.append((child, "tail"))

    collect(node)
    for index, (element, attribute) in enumerate(slots):
        setattr(element, attribute, new if index == 0 else "")


def replace_source_label(value: str, new: str) -> str:
    if "<" not in value:
        return new
    parts = re.split(r"(<[^>]+>)", value)
    text_parts = [
        index
        for index, part in enumerate(parts)
        if not part.startswith("<") and html.unescape(part).strip()
    ]
    if len(text_parts) != 1:
        raise PairError(
            "formatted source label spans multiple text nodes; edit through Obsidian instead"
        )
    index = text_parts[0]
    match = re.fullmatch(r"(\s*)(.*?)(\s*)", parts[index], flags=re.S)
    assert match is not None
    parts[index] = f"{match.group(1)}{html.escape(new, quote=False)}{match.group(3)}"
    return "".join(parts)


def contains_sequence(haystack: list[str], needle: list[str]) -> bool:
    if not needle:
        return True
    width = len(needle)
    return any(
        haystack[index : index + width] == needle
        for index in range(len(haystack) - width + 1)
    )


def decode_payload(payload: str, path: Path) -> ET.Element:
    try:
        compressed = base64.b64decode(payload, validate=True)
        encoded = zlib.decompress(compressed, -15).decode("utf-8")
        xml_text = urllib.parse.unquote(encoded)
    except (binascii.Error, UnicodeDecodeError, zlib.error) as error:
        raise PairError(f"{path}: invalid compressed diagram payload") from error
    return parse_xml(path, xml_text)


def encode_payload(model: ET.Element) -> str:
    xml_text = ET.tostring(model, encoding="unicode")
    encoded = urllib.parse.quote(xml_text, safe=ENCODE_SAFE).encode("utf-8")
    compressor = zlib.compressobj(level=9, wbits=-15)
    compressed = compressor.compress(encoded) + compressor.flush()
    return base64.b64encode(compressed).decode("ascii")


def write_staged(path: Path, contents: str, descriptor: int | None = None) -> None:
    if descriptor is None:
        opened = path.open("x", encoding="utf-8")
    else:
        opened = os.fdopen(descriptor, "w", encoding="utf-8")
    with opened as stream:
        stream.write(contents)
        stream.flush()
        os.fsync(stream.fileno())


def restore_atomic(path: Path, contents: bytes) -> None:
    descriptor, scratch_name = tempfile.mkstemp(
        dir=path.parent, prefix=f".{path.name}-restore-"
    )
    scratch = Path(scratch_name)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(contents)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(scratch, path)
    except BaseException:
        scratch.unlink(missing_ok=True)
        raise


class DrawioPair:
    def __init__(self, svg_path: Path):
        self.svg_path = svg_path
        self.source_path = sidecar_path(svg_path)
        if svg_path.suffix.lower() != ".svg":
            raise PairError(f"{svg_path}: expected an .svg path")
        if not svg_path.is_file():
            raise PairError(f"{svg_path}: SVG does not exist")
        if not self.source_path.is_file():
            raise PairError(f"{self.source_path}: editable sidecar does not exist")

        self.svg_text = svg_path.read_text(encoding="utf-8")
        self.source_text = self.source_path.read_text(encoding="utf-8")
        self.svg_root = parse_xml(svg_path, self.svg_text)
        self.outer = parse_xml(self.source_path, self.source_text)
        if self.outer.tag != "mxfile":
            raise PairError(f"{self.source_path}: root must be mxfile")

        self.diagram = self.outer.find("diagram")
        if self.diagram is None:
            raise PairError(f"{self.source_path}: missing diagram element")

        model = self.diagram.find("mxGraphModel")
        if model is not None:
            self.compressed = False
            self.model = model
        else:
            payload = (self.diagram.text or "").strip()
            if not payload:
                raise PairError(f"{self.source_path}: diagram has no graph model")
            self.compressed = True
            self.model = decode_payload(payload, self.source_path)

        if self.model.tag != "mxGraphModel":
            raise PairError(f"{self.source_path}: decoded root must be mxGraphModel")
        if self.model.find("root") is None:
            raise PairError(f"{self.source_path}: graph model has no root")

    def cells(self):
        return self.model.iter("mxCell")

    def source_with_model(self) -> str:
        if self.compressed:
            self.diagram.text = encode_payload(self.model)
            for child in list(self.diagram):
                self.diagram.remove(child)
        else:
            self.diagram.text = None
            for child in list(self.diagram):
                self.diagram.remove(child)
            self.diagram.append(self.model)
        return ET.tostring(self.outer, encoding="unicode")

    def validate_table_structure(self) -> None:
        cells = list(self.cells())
        children: dict[str, list[ET.Element]] = {}
        for cell in cells:
            parent = cell.get("parent")
            if parent:
                children.setdefault(parent, []).append(cell)

        for cell in cells:
            style = cell.get("style", "")
            cell_id = cell.get("id", "?")
            if "shape=table;" in style:
                rows = [
                    child
                    for child in children.get(cell_id, [])
                    if "shape=tableRow;" in child.get("style", "")
                ]
                if not rows:
                    raise PairError(
                        f"{self.source_path}: table {cell_id} has no tableRow children"
                    )
            if "shape=tableRow;" not in style:
                continue
            if plain_label(cell.get("value", "")):
                raise PairError(
                    f"{self.source_path}: table row {cell_id} stores visible text on "
                    "the row; put it in partialRectangle child cells"
                )
            visible = [
                child
                for child in children.get(cell_id, [])
                if "shape=partialRectangle;" in child.get("style", "")
            ]
            if not visible:
                raise PairError(
                    f"{self.source_path}: table row {cell_id} has no "
                    "partialRectangle child cells"
                )

    def validate_label_sync(self) -> None:
        svg_tokens = label_tokens(" ".join(self.svg_root.itertext()))
        missing: list[str] = []
        for cell in self.cells():
            value = cell.get("value", "")
            tokens = label_tokens(value)
            if len(tokens) >= 2 and not contains_sequence(svg_tokens, tokens):
                missing.append(f"{cell.get('id', '?')}={plain_label(value)!r}")
        if missing:
            details = ", ".join(missing[:4])
            if len(missing) > 4:
                details += f", and {len(missing) - 4} more"
            raise PairError(
                f"{self.svg_path}: source labels are not present in SVG order: {details}"
            )

    def validate(self, strict: bool = False) -> None:
        if EM_DASH in self.svg_text:
            raise PairError(f"{self.svg_path}: contains an em dash")
        for cell in self.cells():
            value = cell.get("value")
            if value and EM_DASH in value:
                raise PairError(
                    f"{self.source_path}: cell {cell.get('id', '?')} contains an em dash"
                )
        self.validate_table_structure()
        if strict:
            self.validate_label_sync()

    def inspect(self) -> None:
        for cell in self.cells():
            value = cell.get("value")
            if not value:
                continue
            plain = html.unescape(re.sub(r"<[^>]+>", "", value))
            plain = plain.replace("\n", " / ").replace("\u00a0", " ")
            print(f"{cell.get('id', '?')}\t{plain}")

    def replace(self, old: str, new: str) -> tuple[int, int]:
        if not old:
            raise PairError("old text must not be empty")
        if old == new:
            raise PairError("old and new text are identical")
        if "\n" in old or "\n" in new:
            raise PairError("replace supports single-line labels only")
        if EM_DASH in new:
            raise PairError("new text contains an em dash")

        source_matches = [
            cell
            for cell in self.cells()
            if (value := cell.get("value")) and plain_label(value) == old
        ]
        source_hits = len(source_matches)
        for cell in source_matches:
            cell.set("value", replace_source_label(cell.get("value", ""), new))

        svg_hits = 0
        for node in visible_label_nodes(self.svg_root):
            if "".join(node.itertext()) == old:
                replace_element_text(node, new)
                svg_hits += 1
        if source_hits == 0:
            raise PairError(f"source does not contain exact text: {old!r}")
        if svg_hits == 0:
            raise PairError(
                "SVG does not contain the same exact text; edit through Obsidian instead"
            )
        updated_svg = ET.tostring(self.svg_root, encoding="unicode")
        updated_source = self.source_with_model()
        parse_xml(self.svg_path, updated_svg)
        reloaded = parse_xml(self.source_path, updated_source)
        if reloaded.find("diagram") is None:
            raise PairError("updated sidecar lost its diagram element")

        descriptor, staged_name = tempfile.mkstemp(
            dir=self.svg_path.parent,
            prefix=f".{self.svg_path.stem}-",
            suffix=".svg",
        )
        staged_svg = Path(staged_name)
        staged_source = sidecar_path(staged_svg)
        try:
            write_staged(staged_svg, updated_svg, descriptor)
            write_staged(staged_source, updated_source)
            DrawioPair(staged_svg).validate(strict=True)

            originals = {
                self.source_path: self.source_path.read_bytes(),
                self.svg_path: self.svg_path.read_bytes(),
            }
            replaced: list[Path] = []
            try:
                os.replace(staged_source, self.source_path)
                replaced.append(self.source_path)
                os.replace(staged_svg, self.svg_path)
                replaced.append(self.svg_path)
            except BaseException as error:
                try:
                    for path in reversed(replaced):
                        restore_atomic(path, originals[path])
                except BaseException as rollback_error:
                    raise PairError(
                        f"pair commit failed and rollback failed: {rollback_error}"
                    ) from error
                raise
        finally:
            staged_svg.unlink(missing_ok=True)
            staged_source.unlink(missing_ok=True)
        return source_hits, svg_hits


def command_validate(paths: list[Path], strict: bool = False) -> int:
    failed = False
    for path in paths:
        try:
            pair = DrawioPair(path)
            pair.validate(strict=strict)
            print(f"OK {path} + {pair.source_path}")
        except (OSError, PairError) as error:
            failed = True
            print(f"ERROR {error}", file=sys.stderr)
    return 1 if failed else 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    inspect_parser = subparsers.add_parser("inspect", help="list source cell labels")
    inspect_parser.add_argument("svg", type=Path)

    replace_parser = subparsers.add_parser(
        "replace", help="replace one exact single-line label in both files"
    )
    replace_parser.add_argument("svg", type=Path)
    replace_parser.add_argument("old")
    replace_parser.add_argument("new")

    validate_parser = subparsers.add_parser("validate", help="validate pairs")
    validate_parser.add_argument(
        "--strict", action="store_true", help="also check source/SVG label order"
    )
    validate_parser.add_argument("svgs", nargs="+", type=Path)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        if args.command == "inspect":
            pair = DrawioPair(args.svg)
            pair.validate()
            pair.inspect()
            return 0
        if args.command == "replace":
            pair = DrawioPair(args.svg)
            pair.validate()
            source_hits, svg_hits = pair.replace(args.old, args.new)
            print(
                f"Updated {args.svg}: {source_hits} source match(es), "
                f"{svg_hits} SVG match(es)"
            )
            return 0
        return command_validate(args.svgs, strict=args.strict)
    except (OSError, PairError) as error:
        print(f"ERROR {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
