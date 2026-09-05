#!/usr/bin/env python3
"""Report mechanical wiki defects without generating replacement prose."""

from __future__ import annotations

import argparse
import json
import re
import sysconfig
from pathlib import Path

from jsonschema import ValidationError as SchemaValidationError
from jsonschema import validate as validate_json
from pypdf import PdfReader
from pypdf.errors import PdfReadError

from corum.run import AppliedItem, StageFailure, StageResult
from corum.state import finalize_wiki_state, read_latest_run


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
    try:
        pages = len(PdfReader(path, strict=True).pages)
    except (OSError, PdfReadError, ValueError) as error:
        raise ValueError(f"could not inspect PDF {path}: {error}") from error
    if pages <= 0:
        raise ValueError(f"could not inspect PDF {path}: PDF has no pages")
    return pages


def raw_source(course: Path, relative: str) -> Path:
    raw = (course / "raw").resolve()
    source = (raw / relative).resolve()
    if not source.is_relative_to(raw):
        raise ValueError(f"source path resolves outside course raw directory: {relative}")
    try:
        with source.open("rb") as stream:
            stream.read(1)
    except OSError as error:
        raise ValueError(f"could not read source {relative}: {error}") from error
    return source


def lint(
    vault: Path,
    code: str,
    pending: list[str] | None = None,
    pending_null: list[str] | None = None,
) -> list[str]:
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
    for relative, label in ingested.items():
        if label is not None and (
            not isinstance(label, str) or not label.strip()
        ):
            raise ValueError(f"{relative} must have a non-empty provenance label or null")
    for item in pending or []:
        if "=" not in item:
            raise ValueError("pending source must be LABEL=RELATIVE_PATH")
        label, relative = item.split("=", 1)
        if not label.strip() or not relative:
            raise ValueError("pending source must be LABEL=RELATIVE_PATH")
        raw_source(course, relative)
        ingested[relative] = label
    for relative in pending_null or []:
        if not relative:
            raise ValueError("pending null source must be RELATIVE_PATH")
        raw_source(course, relative)
        ingested[relative] = None
    for relative, label in ingested.items():
        source = raw_source(course, relative)
        total = page_count(source)
        if not label:
            continue
        if total is None:
            continue
        valid_pages = set(range(1, total + 1))
        source_citations = cited.get(label, set())
        outside = source_citations - valid_pages
        if outside:
            findings.add(
                f"coverage  {relative} cited as {label}, out of range {as_ranges(outside)}"
            )
        gap = valid_pages - source_citations
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


def _schema(name: str) -> dict:
    candidates = (
        Path(sysconfig.get_path("data")) / "share/corum/schemas" / name,
        Path(__file__).resolve().parents[4] / "schemas" / name,
    )
    for candidate in candidates:
        if candidate.is_file():
            return json.loads(candidate.read_text(encoding="utf-8"))
    raise ValueError(f"installed Corum schema is missing: {name}")


def _course_path(vault: Path, code: str) -> Path:
    vault = vault.resolve()
    courses = (vault / "courses").resolve()
    course = (courses / code).resolve()
    if not course.is_relative_to(courses):
        raise ValueError("course resolves outside courses directory")
    return course


def _wiki_path(course: Path, relative: str, *, must_exist: bool) -> Path:
    wiki = (course / "wiki").resolve()
    target = (course / relative).resolve()
    if not target.is_relative_to(wiki):
        raise ValueError(f"wiki result path resolves outside course wiki: {relative}")
    if must_exist and not target.is_file():
        raise ValueError(f"wiki result path does not exist: {relative}")
    return target


def finalize(
    vault: Path,
    code: str,
    payload_path: Path,
) -> tuple[list[str], StageResult | None]:
    try:
        payload = json.loads(payload_path.read_text(encoding="utf-8"))
    except OSError as error:
        raise ValueError(f"could not read finalization payload {payload_path}: {error}") from error
    validate_json(payload, _schema("wiki-finalization.schema.json"))
    if payload["course"] != code:
        raise ValueError(
            f"finalization course {payload['course']!r} does not match selected course {code!r}"
        )

    course = _course_path(vault, code)
    latest = read_latest_run(course)
    if latest is None:
        raise ValueError("latest-run.json is required for wiki finalization")
    if latest["course"] != code or latest["run_id"] != payload["run_id"]:
        raise ValueError("finalization payload does not match current run manifest")
    if not latest["effective_features"]["wiki"]:
        raise ValueError("wiki is disabled in the current run manifest")

    changes = {item["id"]: item for item in latest["canvas"]["changes"]}
    finalized_ids: set[str] = set()
    finalized_paths: set[str] = set()
    ingested: dict[str, str | None] = {}
    pending: list[str] = []
    pending_null: list[str] = []
    for source in payload["sources"]:
        identifier = source["id"]
        if identifier in finalized_ids or source["path"] in finalized_paths:
            raise ValueError("finalization payload has duplicate source identity or path")
        change = changes.get(identifier)
        if change is None or change.get("raw_path") != source["path"]:
            raise ValueError(
                f"source {identifier!r} path does not match current run manifest"
            )
        raw_source(course, source["path"])
        finalized_ids.add(identifier)
        finalized_paths.add(source["path"])
        ingested[source["path"]] = source["provenance"]
        if source["provenance"] is None:
            pending_null.append(source["path"])
        else:
            pending.append(f"{source['provenance']}={source['path']}")

    known_ids = set(changes)
    result_ids: set[str] = set()
    for result in [*payload["applied"], *payload["failures"]]:
        if result["id"] in result_ids:
            raise ValueError(f"duplicate wiki result id: {result['id']}")
        result_ids.add(result["id"])
        unknown = sorted(set(result["source_ids"]) - known_ids)
        if unknown:
            raise ValueError(
                f"wiki result {result['id']!r} references unknown source(s): {', '.join(unknown)}"
            )
    failed_source_ids = {
        source_id
        for failure in payload["failures"]
        for source_id in failure["source_ids"]
    }
    unsafe_finalization = sorted(finalized_ids & failed_source_ids)
    if unsafe_finalization:
        raise ValueError(
            "failed wiki dependencies cannot be finalized: "
            + ", ".join(unsafe_finalization)
        )
    for result in payload["applied"]:
        _wiki_path(course, result["path"], must_exist=True)
    for result in payload["failures"]:
        if result["path"] is not None:
            _wiki_path(course, result["path"], must_exist=False)

    findings = lint(vault, code, pending, pending_null)
    if findings:
        return findings, None

    applied = [
        AppliedItem(
            id=f"wiki:finalize:{source['id']}",
            action="finalize",
            target=source["path"],
            details={
                "source_id": source["id"],
                "provenance": source["provenance"],
            },
        )
        for source in payload["sources"]
    ]
    applied.extend(
        AppliedItem(
            id=result["id"],
            action=result["action"],
            target=result["path"],
            details={"source_ids": result["source_ids"]},
        )
        for result in payload["applied"]
    )
    failures = [
        StageFailure(
            id=result["id"],
            action=result["action"],
            target=result["path"],
            error=result["error"],
            write_state=result["write_state"],
            retry_safe=result["retry_safe"],
            details={"source_ids": result["source_ids"]},
        )
        for result in payload["failures"]
    ]
    if failures and applied:
        status = "partial"
    elif failures:
        status = "failed"
    elif applied:
        status = "applied"
    else:
        status = "up_to_date"
    reconciliation_required = any(
        failure.write_state != "not_applied" for failure in failures
    )
    stage = StageResult(
        status=status,
        applied=applied,
        failures=failures,
        reconciliation_required=reconciliation_required,
        retry_safe=all(failure.retry_safe for failure in failures)
        and not reconciliation_required,
    )
    finalize_wiki_state(
        course,
        expected_run_id=payload["run_id"],
        ingested=ingested,
        stage=stage.model_dump(mode="json"),
    )
    return [], stage


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
    parser.add_argument(
        "--pending-null",
        action="append",
        default=[],
        metavar="RELATIVE_PATH",
        help="validate a planned null-provenance source without changing wiki state",
    )
    parser.add_argument(
        "--finalize",
        type=Path,
        metavar="PAYLOAD.json",
        help="validate and atomically commit an exact wiki finalization payload",
    )
    return parser


def main() -> int:
    args = build_parser().parse_args()
    finalization: StageResult | None = None
    try:
        if args.finalize is not None and (args.pending or args.pending_null):
            raise ValueError("--finalize cannot be combined with --pending options")
        if args.finalize is not None:
            findings, finalization = finalize(
                args.vault, args.course, args.finalize
            )
        else:
            findings = lint(
                args.vault, args.course, args.pending, args.pending_null
            )
    except (OSError, ValueError, RuntimeError, SchemaValidationError, json.JSONDecodeError) as error:
        print(f"lint-wiki: {error}")
        return 1
    if findings:
        print("\n".join(findings))
        return 2
    if finalization is None:
        print(f"{args.course} clean")
    elif finalization.status in {"partial", "failed"}:
        print(f"{args.course} wiki {finalization.status} recorded")
    else:
        print(f"{args.course} finalized")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
