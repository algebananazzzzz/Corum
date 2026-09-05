#!/usr/bin/env python3
"""Inspect, validate, and safely replace labels in Obsidian Draw.io pairs."""

from __future__ import annotations

import argparse
import base64
import binascii
import html
from pathlib import Path
import re
import sys
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

        source_hits = 0
        for cell in self.cells():
            value = cell.get("value")
            if value and old in value:
                cell.set("value", value.replace(old, new))
                source_hits += value.count(old)

        svg_hits = self.svg_text.count(old)
        if source_hits == 0:
            raise PairError(f"source does not contain exact text: {old!r}")
        if svg_hits == 0:
            raise PairError(
                "SVG does not contain the same exact text; edit through Obsidian instead"
            )

        updated_svg = self.svg_text.replace(old, new)
        updated_source = self.source_with_model()
        parse_xml(self.svg_path, updated_svg)
        reloaded = parse_xml(self.source_path, updated_source)
        if reloaded.find("diagram") is None:
            raise PairError("updated sidecar lost its diagram element")

        self.svg_path.write_text(updated_svg, encoding="utf-8")
        self.source_path.write_text(updated_source, encoding="utf-8")
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
