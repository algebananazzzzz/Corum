#!/usr/bin/env python3
"""Report mechanical wiki defects without generating replacement prose."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from pathlib import Path


MARKER = re.compile(r"%%\s*(\S+)\s+p([\d,\s-]+?)\s*%%")
LINK = re.compile(r"!?\[\[([^\]|#]+)")
TAG = re.compile(r"<[A-Za-z/!]")
SKIPPED = re.compile(r"^\|\s*(\S+)\s*\|\s*p([\d,\s p-]+?)\s*\|", re.M)


def links(body: str) -> set[str]:
    """Return normalized Obsidian link targets."""
    return {target.strip().rstrip("\\").strip() for target in LINK.findall(body)}


def pages_cited(spec: str) -> set[int]:
    """Expand a marker page spec such as ``4-7, 9``."""
    pages: set[int] = set()
    for part in spec.split(","):
        part = part.strip()
        if "-" in part:
            lower, upper = part.split("-", 1)
            pages.update(range(int(lower), int(upper) + 1))
        elif part:
            pages.add(int(part))
    return pages


def as_ranges(pages: set[int]) -> str:
    ranges: list[list[int]] = []
    run: list[int] = []
    for page in sorted(pages):
        if run and page == run[-1] + 1:
            run.append(page)
        else:
            if run:
                ranges.append(run)
            run = [page]
    if run:
        ranges.append(run)
    return ", ".join(
        f"p{item[0]}" if len(item) == 1 else f"p{item[0]}-{item[-1]}"
        for item in ranges
    )


def page_count(path: Path) -> int | None:
    if path.suffix.lower() != ".pdf":
        return None
    completed = subprocess.run(
        ["pdfinfo", str(path)], capture_output=True, text=True, check=False
    )
    for line in completed.stdout.splitlines():
        if line.startswith("Pages:"):
            return int(line.split()[1])
    return None


def raw_source(course: Path, relative: str) -> Path:
    raw = (course / "raw").resolve()
    source = (raw / relative).resolve()
    if not source.is_relative_to(raw):
        raise ValueError(f"source path resolves outside course raw directory: {relative}")
    return source


def lint(vault: Path, code: str, pending: list[str] | None = None) -> list[str]:
    vault = vault.resolve()
    courses = (vault / "courses").resolve()
    course = (courses / code).resolve()
    if not course.is_relative_to(courses):
        raise ValueError("course resolves outside courses directory")
    wiki = course / "wiki"
    if not wiki.is_dir():
        raise ValueError(f"{code} has no wiki")

    state_path = course / "state/wiki.json"
    state = json.loads(state_path.read_text()) if state_path.is_file() else {"ingested": {}}
    pages = sorted(wiki.rglob("*.md"))
    text = {path: path.read_text(encoding="utf-8") for path in pages}
    findings: set[str] = set()

    cited: dict[str, set[int]] = {}
    for body in text.values():
        for label, spec in MARKER.findall(body):
            cited.setdefault(label, set()).update(pages_cited(spec))

    index_path = wiki / "index.md"
    index_body = index_path.read_text(encoding="utf-8")
    skipped = index_body.split("## Skipped", 1)
    if len(skipped) == 2:
        for label, spec in SKIPPED.findall(skipped[1]):
            cited.setdefault(label, set()).update(pages_cited(spec.replace("p", "")))

    ingested = dict(state.get("ingested", {}))
    for item in pending or []:
        if "=" not in item:
            raise ValueError("pending source must be LABEL=RELATIVE_PATH")
        label, relative = item.split("=", 1)
        if not label or not relative:
            raise ValueError("pending source must be LABEL=RELATIVE_PATH")
        raw_source(course, relative)
        ingested[relative] = label
    for relative, label in ingested.items():
        if not label:
            continue
        total = page_count(raw_source(course, relative))
        if total is None:
            continue
        gap = set(range(1, total + 1)) - cited.get(label, set())
        if gap:
            findings.add(
                f"coverage  {relative} cited as {label}, uncited {as_ranges(gap)}"
            )
    claimed = {value for value in ingested.values() if value}
    for label in sorted(set(cited) - claimed):
        findings.add(f"coverage  marker {label} cites no finalized source")

    linked: set[str] = set()
    for path, body in text.items():
        if path.parent.name == "explainers":
            linked |= links(body)
    for path in pages:
        relative = str(path.relative_to(vault).with_suffix(""))
        if path.parent.name == "concepts" and relative not in linked:
            findings.add(f"orphan    {relative} no explainer links it")

    for path, body in text.items():
        for target in links(body):
            if not ((vault / target).exists() or (vault / f"{target}.md").exists()):
                findings.add(
                    f"dangling  {path.relative_to(vault)} links {target}"
                )
        without_code = re.sub(r"`[^`]*`", "", body)
        if TAG.search(without_code):
            findings.add(f"tag       {path.relative_to(vault)} holds an HTML tag")

    rows = links(index_body)
    for path in pages:
        if path.name == "index.md":
            continue
        relative = str(path.relative_to(vault).with_suffix(""))
        if relative not in rows:
            findings.add(f"drift     {relative} has no row in index.md")
    prefix = f"courses/{code}/wiki/"
    for row in rows:
        if row.startswith(prefix) and not (vault / f"{row}.md").exists():
            findings.add(f"drift     index.md row {row} has no page")

    return sorted(findings)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("course")
    parser.add_argument("--vault", type=Path, default=Path.cwd())
    parser.add_argument(
        "--pending",
        action="append",
        default=[],
        metavar="LABEL=RELATIVE_PATH",
        help="validate a planned source without changing wiki state",
    )
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        findings = lint(args.vault, args.course, args.pending)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"lint-wiki: {error}")
        return 1
    print("\n".join(findings) if findings else f"{args.course} clean")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
