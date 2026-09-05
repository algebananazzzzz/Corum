"""Capture configured Canvas sources into a Corum course's raw directory."""

from __future__ import annotations

import asyncio
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
from urllib.parse import urlsplit

from corum.config import CourseConfig, load_workspace, resolve_features
from corum.run import RunManifest
from corum.state import course_sync_lock, read_canvas_state, write_canvas_state, write_latest_run

from . import convert, placement
from .client import CanvasClient


def content_hash(payload: object) -> str:
    canonical = json.dumps(payload, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
    return "sha1:" + hashlib.sha1(canonical.encode()).hexdigest()


def module_digest(module: dict, items: list[dict]) -> str:
    return content_hash(
        {
            "name": module.get("name"),
            "position": module.get("position"),
            "items": [
                {
                    "id": item.get("id"),
                    "position": item.get("position"),
                    "title": item.get("title"),
                    "type": item.get("type"),
                    "indent": item.get("indent"),
                }
                for item in items
            ],
        }
    )


def _instant(raw: str | None) -> datetime | None:
    if not raw:
        return None
    return datetime.fromisoformat(raw.replace("Z", "+00:00"))


def _minute(raw: str | None) -> datetime | None:
    moment = _instant(raw)
    return moment.replace(second=0, microsecond=0) if moment else None


def new_announcements(records: list[dict], seen: dict) -> list[dict]:
    return [record for record in records if str(record["id"]) not in seen]


def new_assignments(records: list[dict], seen: dict) -> list[dict]:
    return [
        record
        for record in records
        if str(record["id"]) not in seen
        or _minute(record.get("due_at")) != _minute(seen[str(record["id"])])
    ]


def new_files(records: list[dict], seen: dict) -> list[dict]:
    return [record for record in records if str(record["id"]) not in seen]


def changed_pages(pages: list[dict], seen: dict) -> list[dict]:
    changed = []
    for page in pages:
        url = page["url"]
        if url not in seen:
            changed.append(page)
        elif seen[url] is not None:
            updated = _instant(page.get("updated_at"))
            if updated and updated > _instant(seen[url]):
                changed.append(page)
    return changed


def announcement_name(record: dict) -> str:
    timestamp = record.get("posted_at") or record.get("delayed_post_at") or record.get("created_at")
    converted = convert.sgt(timestamp) if timestamp else None
    return f"{converted[:10] if converted else 'undated'}-{convert.slug(record.get('title'))}"


def unique(target: Path, discriminator: str) -> Path:
    if not target.exists():
        return target
    return target.with_name(f"{target.stem}-{convert.slug(discriminator)}{target.suffix}")


def _now() -> str:
    return datetime.now(convert.TZ).isoformat(timespec="seconds")


def _under_raw(root: Path, relative: str) -> Path:
    resolved_root = root.resolve()
    target = (resolved_root / relative).resolve()
    if not target.is_relative_to(resolved_root):
        raise ValueError(f"refusing to write outside {resolved_root}: {relative!r}")
    return target


def _validated_course_dir(vault: Path, code: str) -> Path:
    if not code or Path(code).name != code or code in {".", ".."}:
        raise ValueError(f"invalid course code: {code!r}")
    vault_root = vault.resolve()
    courses_root = (vault_root / "courses").resolve()
    if not courses_root.is_relative_to(vault_root):
        raise ValueError(f"courses directory resolves outside vault: {vault_root / 'courses'}")
    configured_course = courses_root / code
    if not configured_course.is_dir():
        raise ValueError(f"course code {code} has no configured course directory")
    course_dir = configured_course.resolve()
    if not course_dir.is_relative_to(courses_root):
        raise ValueError(f"course directory resolves outside vault: {courses_root / code}")
    for name in ("raw", "state"):
        child = (course_dir / name).resolve()
        if not child.is_relative_to(course_dir):
            raise ValueError(f"{name} directory resolves outside course: {course_dir / name}")
    return course_dir


def write_body(path: Path, fields: dict, html: str, link_list: list[dict], image_paths: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        convert.frontmatter(fields, link_list) + convert.to_markdown(html, image_paths),
        encoding="utf-8",
    )


def write_modules(path: Path, trees: list[tuple[dict, list[dict]]]) -> None:
    lines: list[str] = []
    for module, items in trees:
        lines.extend([f"## {module.get('name')}\n", "| # | Type | Title |", "| --- | --- | --- |"])
        for item in items:
            title = (item.get("title") or "").replace("|", r"\|")
            lines.append(f"| {item.get('position')} | {item.get('type')} | {title} |")
        lines.append("")
    path.parent.mkdir(parents=True, exist_ok=True)
    header = convert.frontmatter({"source": "canvas", "kind": "modules", "fetched": _now()})
    path.write_text(header + "\n".join(lines).rstrip() + "\n", encoding="utf-8")


async def _save_images(client: CanvasClient, html: str, folder: Path) -> tuple[dict, list[str]]:
    paths: dict[str, str] = {}
    problems: list[str] = []
    for index, source in enumerate(convert.images(html), start=1):
        name = Path(urlsplit(source).path).name or f"image-{index}"
        try:
            target = _under_raw(folder, name)
            await client.download(source, target)
        except Exception as error:
            problems.append(f"image not downloaded: {error}")
        else:
            paths[source] = name
    return paths, problems


async def fetch_announcements(
    client: CanvasClient,
    course_dir: Path,
    course_id: int,
    seen: dict,
    dry_run: bool,
    canvas_host: str,
):
    records = await client.get_all(f"/courses/{course_id}/discussion_topics", only_announcements=True)
    changes: list[str] = []
    entries: dict[str, str | None] = {}
    failures: list[str] = []
    for record in new_announcements(records, seen):
        changes.append(f"announcement {record['id']} · {record.get('title')}")
        if dry_run:
            continue
        html = record.get("message") or ""
        target = unique(
            _under_raw(course_dir / "raw", f"announcements/{announcement_name(record)}.md"),
            str(record["id"]),
        )
        image_paths, problems = await _save_images(client, html, target.parent)
        failures.extend(problems)
        write_body(
            target,
            {
                "source": "canvas",
                "kind": "announcement",
                "id": record["id"],
                "url": record.get("html_url"),
                "title": record.get("title"),
                "author": record.get("user_name"),
                "posted": convert.sgt(record.get("posted_at")),
                "fetched": _now(),
            },
            html,
            convert.links(html, canvas_host),
            image_paths,
        )
        if not problems:
            entries[str(record["id"])] = convert.sgt(record.get("posted_at"))
    return changes, entries, failures


async def fetch_assignments(
    client: CanvasClient,
    course_dir: Path,
    course_id: int,
    seen: dict,
    dry_run: bool,
    canvas_host: str,
):
    records = await client.get_all(f"/courses/{course_id}/assignments")
    changes: list[str] = []
    entries: dict[str, str | None] = {}
    for record in new_assignments(records, seen):
        due = convert.sgt(record.get("due_at"))
        changes.append(f"assignment {record['id']} · {record.get('name')} · due {due or 'none'}")
        if dry_run:
            continue
        html = record.get("description") or ""
        target = unique(
            _under_raw(course_dir / "raw", f"assignments/{convert.slug(record.get('name'))}.md"),
            str(record["id"]),
        )
        write_body(
            target,
            {
                "source": "canvas",
                "kind": "assignment",
                "id": record["id"],
                "url": record.get("html_url"),
                "title": record.get("name"),
                "due": due,
                "points": record.get("points_possible"),
                "fetched": _now(),
            },
            html,
            convert.links(html, canvas_host),
            {},
        )
        entries[str(record["id"])] = due
    return changes, entries, []


async def fetch_files(
    client: CanvasClient,
    course_dir: Path,
    course_id: int,
    seen: dict,
    folders: dict[str, str],
    dry_run: bool,
):
    records = await client.get_all(f"/courses/{course_id}/files")
    known = await placement.folder_paths(client, course_id)
    changes: list[str] = []
    entries: dict[str, str] = {}
    failures: list[str] = []
    for record in new_files(records, seen):
        relative = placement.place(known.get(record.get("folder_id")), record["display_name"], folders)
        if dry_run:
            changes.append(f"file {record['id']} · {relative}")
            continue
        try:
            target = _under_raw(course_dir / "raw", relative)
            await client.download(record["url"], target)
        except Exception as error:
            failures.append(f"file {record['id']} not downloaded: {error}")
            continue
        changes.append(f"file {record['id']} · {relative}")
        entries[str(record["id"])] = relative
    return changes, entries, failures


async def _pages_via_modules(client: CanvasClient, course_id: int) -> list[dict]:
    pages = []
    for module in await client.get_all(f"/courses/{course_id}/modules"):
        items = await client.get_all(f"/courses/{course_id}/modules/{module['id']}/items")
        for item in items:
            if item.get("type") == "Page" and item.get("page_url"):
                pages.append(await client.get(f"/courses/{course_id}/pages/{item['page_url']}"))
    return pages


async def fetch_pages(
    client: CanvasClient,
    course_dir: Path,
    course_id: int,
    seen: dict,
    dry_run: bool,
    canvas_host: str,
):
    try:
        listed = await client.get_all(f"/courses/{course_id}/pages")
    except Exception:
        listed = await _pages_via_modules(client, course_id)
    try:
        front = await client.get(f"/courses/{course_id}/front_page")
    except Exception:
        pass
    else:
        listed = [front] + [page for page in listed if page.get("url") != front.get("url")]
    changes: list[str] = []
    entries: dict[str, str | None] = {}
    for page in changed_pages(listed, seen):
        changes.append(f"page {page['url']} · {page.get('title')}")
        if dry_run:
            continue
        body = page.get("body")
        if body is None:
            page = await client.get(f"/courses/{course_id}/pages/{page['url']}")
            body = page.get("body") or ""
        target = unique(
            _under_raw(course_dir / "raw", f"pages/{convert.slug(page.get('title'))}.md"),
            page["url"],
        )
        write_body(
            target,
            {
                "source": "canvas",
                "kind": "page",
                "page_url": page["url"],
                "url": page.get("html_url"),
                "title": page.get("title"),
                "updated": convert.sgt(page.get("updated_at")),
                "fetched": _now(),
            },
            body,
            convert.links(body, canvas_host),
            {},
        )
        entries[page["url"]] = convert.sgt(page.get("updated_at"))
    return changes, entries, []


async def fetch_modules(
    client: CanvasClient,
    course_dir: Path,
    course_id: int,
    seen: dict,
    dry_run: bool,
):
    trees: list[tuple[dict, list[dict]]] = []
    entries: dict[str, str] = {}
    changes: list[str] = []
    for module in await client.get_all(f"/courses/{course_id}/modules"):
        items = await client.get_all(f"/courses/{course_id}/modules/{module['id']}/items")
        trees.append((module, items))
        digest = module_digest(module, items)
        if seen.get(str(module["id"])) != digest:
            changes.append(f"module {module['id']} · {module.get('name')}")
        entries[str(module["id"])] = digest
    if not changes:
        return [], {}, []
    if dry_run:
        return changes, {}, []
    write_modules(course_dir / "raw" / "modules.md", trees)
    return changes, entries, []


async def fetch_syllabus(
    client: CanvasClient,
    course_dir: Path,
    course_id: int,
    seen: str | None,
    dry_run: bool,
    canvas_host: str,
):
    record = await client.get(f"/courses/{course_id}", **{"include[]": "syllabus_body"})
    html = record.get("syllabus_body") or ""
    digest = content_hash(html)
    if digest == seen:
        return [], seen, []
    if dry_run:
        return ["syllabus changed"], seen, []
    write_body(
        course_dir / "raw" / "syllabus.md",
        {"source": "canvas", "kind": "syllabus", "url": record.get("html_url"), "fetched": _now()},
        html,
        convert.links(html, canvas_host),
        {},
    )
    return ["syllabus changed"], digest, []


async def sync_course(vault: Path, course: CourseConfig, dry_run: bool) -> RunManifest:
    course_dir = _validated_course_dir(vault, course.code)
    with course_sync_lock(course_dir):
        workspace = load_workspace(vault, validate_jira=False)
        resolved_features = resolve_features(workspace, course)
        if resolved_features.jira:
            workspace = load_workspace(vault)
        features = resolved_features.model_dump()
        token = os.environ.get("CORUM_CANVAS_TOKEN")
        if not token:
            raise ValueError("CORUM_CANVAS_TOKEN environment variable is required")
        client = CanvasClient(str(workspace.canvas.host), token)
        state = read_canvas_state(course_dir)
        sources = state["sources"]
        host = str(workspace.canvas.host).rstrip("/")
        jobs = {}
        for source in course.canvas.sources:
            seen = sources.get(source)
            if source == "announcements":
                jobs[source] = fetch_announcements(client, course_dir, course.canvas.id, seen or {}, dry_run, host)
            elif source == "assignments":
                jobs[source] = fetch_assignments(client, course_dir, course.canvas.id, seen or {}, dry_run, host)
            elif source == "files":
                jobs[source] = fetch_files(
                    client, course_dir, course.canvas.id, seen or {}, course.canvas.folders, dry_run
                )
            elif source == "pages":
                jobs[source] = fetch_pages(client, course_dir, course.canvas.id, seen or {}, dry_run, host)
            elif source == "modules":
                jobs[source] = fetch_modules(client, course_dir, course.canvas.id, seen or {}, dry_run)
            elif source == "syllabus":
                jobs[source] = fetch_syllabus(client, course_dir, course.canvas.id, seen, dry_run, host)

        results = await asyncio.gather(*jobs.values(), return_exceptions=True)
        changes: list[str] = []
        failures: list[tuple[str, str]] = []
        updates: dict[str, object] = {}
        for source, result in zip(jobs, results):
            if isinstance(result, Exception):
                failures.append((source, str(result)))
                continue
            source_changes, entries, source_failures = result
            changes.extend(source_changes)
            failures.extend((source, failure) for failure in source_failures)
            updates[source] = entries

        if not dry_run:
            for source, entries in updates.items():
                if source == "syllabus":
                    sources[source] = entries
                elif entries:
                    sources.setdefault(source, {}).update(entries)
            state["synced_at"] = _now()
            write_canvas_state(course_dir, state)

        manifest = RunManifest.create(course.code, features, changes=changes, failures=failures)
        if not dry_run:
            write_latest_run(course_dir, manifest.model_dump(mode="json"))
        return manifest
