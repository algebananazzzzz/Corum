from __future__ import annotations

import asyncio
import json
from pathlib import Path

import pytest
import yaml

from corum.canvas import sync
from corum.config import load_course


class FakeCanvasClient:
    def __init__(self, routes: dict[tuple[str, str], object] | None = None) -> None:
        self.routes = routes or {}
        self.downloads: list[tuple[str, Path]] = []

    def _route(self, method: str, endpoint: str) -> object:
        for (wanted_method, suffix), response in self.routes.items():
            if wanted_method == method and endpoint.endswith(suffix):
                if isinstance(response, Exception):
                    raise response
                return response
        raise AssertionError(f"unstubbed {method}: {endpoint}")

    async def get_all(self, endpoint: str, **params: object) -> list[dict]:
        return self._route("get_all", endpoint)  # type: ignore[return-value]

    async def get(self, endpoint: str, **params: object) -> dict:
        return self._route("get", endpoint)  # type: ignore[return-value]

    async def download(self, url: str, target: Path) -> int:
        response = self._route("download", url)
        if isinstance(response, bytes):
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(response)
            self.downloads.append((url, target))
            return len(response)
        raise AssertionError(f"invalid download response: {response!r}")


def make_vault(tmp_path: Path, *, sources: list[str], canvas_state: dict | None = None) -> tuple[Path, object]:
    vault = tmp_path / "vault"
    course_dir = vault / "courses" / "CS3103"
    (course_dir / "state").mkdir(parents=True)
    (course_dir / "raw").mkdir()
    (vault / "corum.yaml").write_text(
        yaml.safe_dump(
            {
                "schema": 1,
                "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
                "canvas": {"host": "https://canvas.example.edu"},
                "features": {"jira": {"enabled": True}, "wiki": {"enabled": False}},
                "jira": {"site": "https://example.atlassian.net", "project": "STUDY"},
                "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
            },
            sort_keys=False,
        )
    )
    (course_dir / "course.yaml").write_text(
        yaml.safe_dump(
            {
                "schema": 1,
                "code": "CS3103",
                "canvas": {
                    "id": 93794,
                    "sources": sources,
                    "folders": {"Lecture Slides": "lectures"},
                },
                "features": {"jira": {"enabled": False}, "wiki": {"enabled": True}},
                "wiki": {"split_rules": "default"},
            },
            sort_keys=False,
        )
    )
    state = canvas_state or {
        "schema": 1,
        "synced_at": "2026-08-25T00:00:00+08:00",
        "sources": {name: (None if name == "syllabus" else {}) for name in sources},
    }
    (course_dir / "state" / "canvas.json").write_text(json.dumps(state))
    return vault, load_course(vault, "CS3103")


def test_announcement_is_new_only_when_its_id_is_absent():
    records = [{"id": 100, "title": "seen"}, {"id": 101, "title": "new"}]
    assert [record["id"] for record in sync.new_announcements(records, {"100": "x"})] == [101]


@pytest.mark.parametrize(
    ("due_at", "seen", "changed"),
    [
        ("2026-09-04T15:59:59Z", "2026-09-04T23:59+08:00", False),
        ("2026-09-10T15:59:00Z", "2026-09-04T23:59+08:00", True),
        (None, None, False),
        ("2026-10-01T15:59:00Z", None, True),
    ],
)
def test_assignment_change_detection_uses_due_time_to_the_minute(due_at, seen, changed):
    records = [{"id": 200, "name": "A", "due_at": due_at}]
    assert bool(sync.new_assignments(records, {"200": seen})) is changed


def test_a_page_recorded_as_null_is_deliberately_skipped():
    page = {"url": "lesson", "updated_at": "2026-08-27T09:56:00Z"}
    assert sync.changed_pages([page], {"lesson": None}) == []


def test_a_page_is_changed_only_when_upstream_is_newer():
    pages = [
        {"url": "newer", "updated_at": "2026-08-27T09:56:00Z"},
        {"url": "older", "updated_at": "2026-08-01T00:00:00Z"},
    ]
    seen = {
        "newer": "2026-08-22T11:46:44+08:00",
        "older": "2026-08-27T17:56:00+08:00",
    }
    assert [page["url"] for page in sync.changed_pages(pages, seen)] == ["newer"]


def test_module_digest_ignores_student_progress_but_not_item_titles():
    original = [{"id": 1, "position": 1, "title": "Old", "type": "Page", "indent": 0}]
    renamed = [{**original[0], "title": "New"}]
    started = {"name": "Week 1", "position": 1, "state": "started"}
    completed = {"name": "Week 1", "position": 1, "state": "completed"}
    assert sync.module_digest(started, original) == sync.module_digest(completed, original)
    assert sync.module_digest(started, original) != sync.module_digest(started, renamed)


def test_syllabus_digest_is_stable_across_mapping_key_order():
    assert sync.content_hash({"a": 1, "b": 2}) == sync.content_hash({"b": 2, "a": 1})


def test_duplicate_paths_append_the_canvas_discriminator(tmp_path):
    target = tmp_path / "lesson.md"
    target.write_text("original")
    assert sync.unique(target, "page-one").name == "lesson-page-one.md"
    assert sync.unique(target, "page-two").name == "lesson-page-two.md"
    assert target.read_text() == "original"


@pytest.mark.asyncio
async def test_files_download_with_verifier_and_use_configured_folder(tmp_path):
    client = FakeCanvasClient(
        {
            ("get_all", "/files"): [
                {
                    "id": 1,
                    "display_name": "L3.pdf",
                    "folder_id": 7,
                    "url": "https://canvas.example.edu/files/1?verifier=secret",
                }
            ],
            ("get_all", "/folders"): [{"id": 7, "full_name": "course files/Lecture Slides"}],
            ("download", "/files/1?verifier=secret"): b"pdf",
        }
    )
    changes, entries = await sync.fetch_files(
        client, tmp_path, 1, {}, {"Lecture Slides": "lectures"}, dry_run=False
    )
    assert changes == ["file 1 · lectures/L3.pdf"]
    assert entries == {"1": "lectures/L3.pdf"}
    assert client.downloads[0][0].endswith("verifier=secret")
    assert (tmp_path / "raw" / "lectures" / "L3.pdf").read_bytes() == b"pdf"


@pytest.mark.asyncio
async def test_files_refuse_a_path_that_escapes_raw(tmp_path):
    client = FakeCanvasClient(
        {
            ("get_all", "/files"): [
                {"id": 1, "display_name": "/etc/passwd", "folder_id": None, "url": "https://c/1"}
            ],
            ("get_all", "/folders"): [],
        }
    )
    changes, entries = await sync.fetch_files(client, tmp_path, 1, {}, {}, dry_run=False)
    assert entries == {}
    assert client.downloads == []
    assert any("not downloaded" in change for change in changes)


@pytest.mark.asyncio
async def test_one_failed_file_does_not_discard_an_earlier_download(tmp_path):
    client = FakeCanvasClient(
        {
            ("get_all", "/files"): [
                {"id": 1, "display_name": "A.pdf", "folder_id": None, "url": "https://c/1"},
                {"id": 2, "display_name": "B.pdf", "folder_id": None, "url": "https://c/2"},
            ],
            ("get_all", "/folders"): [],
            ("download", "https://c/1"): b"ok",
            ("download", "https://c/2"): RuntimeError("HTTP 403"),
        }
    )
    changes, entries = await sync.fetch_files(client, tmp_path, 1, {}, {}, dry_run=False)
    assert entries == {"1": "A.pdf"}
    assert (tmp_path / "raw" / "A.pdf").exists()
    assert not (tmp_path / "raw" / "B.pdf").exists()
    assert any("file 2 not downloaded" in change for change in changes)


@pytest.mark.asyncio
async def test_dry_run_reports_files_without_downloading(tmp_path):
    client = FakeCanvasClient(
        {
            ("get_all", "/files"): [
                {"id": 1, "display_name": "A.pdf", "folder_id": None, "url": "https://c/1"}
            ],
            ("get_all", "/folders"): [],
        }
    )
    changes, entries = await sync.fetch_files(client, tmp_path, 1, {}, {}, dry_run=True)
    assert changes == ["file 1 · A.pdf"]
    assert entries == {}
    assert client.downloads == []
    assert not (tmp_path / "raw").exists()


@pytest.mark.asyncio
async def test_pages_fall_back_to_modules_when_pages_listing_is_closed(tmp_path):
    client = FakeCanvasClient(
        {
            ("get_all", "/pages"): RuntimeError("HTTP 404"),
            ("get_all", "/modules"): [{"id": 5, "name": "Week 1", "position": 1}],
            ("get_all", "/items"): [
                {"id": 9, "position": 1, "type": "Page", "title": "Lesson 1.3", "page_url": "lesson"}
            ],
            ("get", "/pages/lesson"): {
                "url": "lesson",
                "title": "Lesson 1.3",
                "updated_at": "2026-07-22T01:55:10Z",
                "body": '<a href="https://canvas.example.edu/courses/1">Course</a>',
            },
            ("get", "/front_page"): RuntimeError("HTTP 404"),
        }
    )
    changes, entries = await sync.fetch_pages(
        client, tmp_path, 1, {}, dry_run=False, canvas_host="https://canvas.example.edu"
    )
    assert changes == ["page lesson · Lesson 1.3"]
    assert "lesson" in entries
    text = (tmp_path / "raw" / "pages" / "lesson-1.3.md").read_text()
    assert "type: internal" in text


@pytest.mark.asyncio
async def test_syllabus_writes_only_when_body_digest_moves(tmp_path):
    body = {"syllabus_body": "<p>Weighting: 40%</p>"}
    client = FakeCanvasClient({("get", "/courses/1"): body})
    changes, digest = await sync.fetch_syllabus(
        client, tmp_path, 1, None, dry_run=False, canvas_host="https://canvas.example.edu"
    )
    assert changes == ["syllabus changed"]
    assert (tmp_path / "raw" / "syllabus.md").exists()
    changes_again, same = await sync.fetch_syllabus(
        client, tmp_path, 1, digest, dry_run=False, canvas_host="https://canvas.example.edu"
    )
    assert changes_again == []
    assert same == digest


def stub_capture_functions(monkeypatch, **overrides):
    async def empty(*args, **kwargs):
        return [], {}

    async def empty_syllabus(client, course_dir, course_id, seen, dry_run, canvas_host):
        return [], seen

    for source in ("announcements", "assignments", "files", "pages", "modules"):
        monkeypatch.setattr(sync, f"fetch_{source}", overrides.get(source, empty))
    monkeypatch.setattr(sync, "fetch_syllabus", overrides.get("syllabus", empty_syllabus))


@pytest.mark.asyncio
async def test_partial_source_failure_advances_only_successful_state(tmp_path, monkeypatch):
    async def announcement(*args, **kwargs):
        return ["announcement 9 · New"], {"9": "2026-08-28T11:59:02+08:00"}

    async def files(*args, **kwargs):
        raise RuntimeError("HTTP 403")

    vault, course = make_vault(tmp_path, sources=["announcements", "files"])
    stub_capture_functions(monkeypatch, announcements=announcement, files=files)
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")
    monkeypatch.setattr(sync, "CanvasClient", lambda host, token: FakeCanvasClient())

    manifest = await sync.sync_course(vault, course, dry_run=False)

    state = json.loads((vault / "courses/CS3103/state/canvas.json").read_text())
    assert state["sources"]["announcements"]["9"] == "2026-08-28T11:59:02+08:00"
    assert state["sources"]["files"] == {}
    assert manifest.canvas.status == "partial"
    assert manifest.canvas.failures[0].source == "files"


@pytest.mark.asyncio
async def test_dry_run_leaves_canvas_state_and_manifest_untouched(tmp_path, monkeypatch):
    async def announcement(*args, **kwargs):
        return ["announcement 9 · New"], {"9": "new"}

    vault, course = make_vault(tmp_path, sources=["announcements"])
    stub_capture_functions(monkeypatch, announcements=announcement)
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")
    monkeypatch.setattr(sync, "CanvasClient", lambda host, token: FakeCanvasClient())
    state_path = vault / "courses/CS3103/state/canvas.json"
    before = state_path.read_bytes()

    manifest = await sync.sync_course(vault, course, dry_run=True)

    assert manifest.canvas.status == "changed"
    assert state_path.read_bytes() == before
    assert not (vault / "courses/CS3103/state/latest-run.json").exists()
    assert not (vault / "courses/CS3103/state/.sync.lock").exists()


@pytest.mark.asyncio
async def test_real_run_always_writes_latest_manifest_with_effective_features(tmp_path, monkeypatch):
    vault, course = make_vault(tmp_path, sources=[])
    stub_capture_functions(monkeypatch)
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")
    monkeypatch.setattr(sync, "CanvasClient", lambda host, token: FakeCanvasClient())

    manifest = await sync.sync_course(vault, course, dry_run=False)

    latest = json.loads((vault / "courses/CS3103/state/latest-run.json").read_text())
    assert latest == manifest.model_dump(mode="json")
    assert manifest.effective_features == {"jira": False, "wiki": True}
    assert manifest.jira.status == "disabled"
    assert manifest.wiki.status == "pending"
    assert manifest.canvas.status == "up_to_date"


@pytest.mark.asyncio
async def test_existing_course_lock_fails_clearly_and_is_not_removed(tmp_path, monkeypatch):
    vault, course = make_vault(tmp_path, sources=[])
    lock = vault / "courses/CS3103/state/.sync.lock"
    lock.write_text("another process")
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")

    with pytest.raises(RuntimeError, match="already locked"):
        await sync.sync_course(vault, course, dry_run=False)

    assert lock.read_text() == "another process"
