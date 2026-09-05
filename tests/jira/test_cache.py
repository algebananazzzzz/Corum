from __future__ import annotations

import json
from pathlib import Path

import pytest
import yaml

from corum.config import load_course
from corum.jira import cache


STAMP = "2026-09-03T14:30:00+08:00"


def write_workspace(root: Path) -> None:
    (root / "corum.yaml").write_text(
        yaml.safe_dump(
            {
                "schema": 1,
                "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
                "canvas": {"host": "https://canvas.example.edu"},
                "jira": {"site": "https://example.atlassian.net", "project": "STUDY"},
                "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
            }
        )
    )


def write_course(root: Path, code: str) -> None:
    folder = root / "courses" / code
    folder.mkdir(parents=True)
    (folder / "course.yaml").write_text(
        yaml.safe_dump(
            {
                "schema": 1,
                "code": code,
                "canvas": {"id": 1, "sources": []},
                "jira": {"epic": "STUDY-1"},
            }
        )
    )


def issue(key: str = "STUDY-2", **changes) -> dict:
    value = {
        "key": key,
        "type": "Task",
        "summary": f"Summary {key}",
        "status": "To Do",
        "due": None,
        "labels": ["session", "assessment", "session"],
        "description": None,
        "updated_at": "2026-09-03T09:12:41+08:00",
    }
    value.update(changes)
    return value


def reconcile(epic: str = "STUDY-1", issues: list[dict] | None = None, **changes) -> dict:
    value = {
        "epic": epic,
        "reconciled_at": STAMP,
        "complete": True,
        "issues": issues or [],
    }
    value.update(changes)
    return value


@pytest.fixture
def configured_vault(tmp_path):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    write_course(tmp_path, "CS4226")
    return tmp_path, load_course(tmp_path, "CS3103")


def test_reconcile_creates_only_the_requested_split_state_cache(configured_vault):
    vault, course = configured_vault

    result = cache.reconcile(vault, course, reconcile(issues=[issue()]))

    target = vault / "courses/CS3103/state/jira.json"
    assert result == {"course": "CS3103", "epic": "STUDY-1", "issues": 1}
    assert target.exists()
    assert not (vault / "courses/CS4226/state/jira.json").exists()
    assert "epic" not in json.loads(target.read_text())


def test_reordered_equivalent_input_produces_identical_bytes(configured_vault):
    vault, course = configured_vault
    cache.reconcile(vault, course, reconcile(issues=[issue("STUDY-10"), issue("STUDY-2")]))
    target = vault / "courses/CS3103/state/jira.json"
    before = target.read_bytes()

    reordered_issue = {
        "updated_at": "2026-09-03T09:12:41+08:00",
        "status": "To Do",
        "summary": "Summary STUDY-2",
        "type": "Task",
        "key": "STUDY-2",
        "labels": ["session", "assessment"],
        "description": None,
        "due": None,
    }
    cache.reconcile(
        vault,
        course,
        {
            "issues": [reordered_issue, issue("STUDY-10")],
            "complete": True,
            "reconciled_at": STAMP,
            "epic": "STUDY-1",
        },
    )

    assert target.read_bytes() == before
    assert [row["key"] for row in json.loads(before)["issues"]] == ["STUDY-2", "STUDY-10"]


@pytest.mark.parametrize(
    "bad",
    [
        reconcile(complete=False),
        reconcile(issues=[issue(description=5)]),
        reconcile(issues=[issue(), issue()]),
        reconcile(epic="STUDY-999"),
        reconcile(extra="raw REST envelope"),
        reconcile(reconciled_at="yesterday"),
    ],
)
def test_invalid_reconcile_preserves_previous_bytes(configured_vault, bad):
    vault, course = configured_vault
    cache.reconcile(vault, course, reconcile(issues=[issue()]))
    target = vault / "courses/CS3103/state/jira.json"
    before = target.read_bytes()

    with pytest.raises(cache.CacheError):
        cache.reconcile(vault, course, bad)

    assert target.read_bytes() == before


def test_empty_reconciliation_requires_explicit_complete(configured_vault):
    vault, course = configured_vault
    with pytest.raises(cache.CacheError, match="complete must be true"):
        cache.reconcile(
            vault,
            course,
            {"epic": "STUDY-1", "reconciled_at": STAMP, "issues": []},
        )
    cache.reconcile(vault, course, reconcile())
    assert json.loads((vault / "courses/CS3103/state/jira.json").read_text())["issues"] == []


def test_upsert_adds_then_replaces_without_changing_other_issues_or_stamp(configured_vault):
    vault, course = configured_vault
    cache.reconcile(vault, course, reconcile(issues=[issue("STUDY-2")]))
    cache.upsert(vault, course, {"epic": "STUDY-1", "issue": issue("STUDY-10")})
    replacement = issue("STUDY-2", summary="Changed", labels=["z", "a", "z"])
    cache.upsert(vault, course, {"epic": "STUDY-1", "issue": replacement})

    output = json.loads((vault / "courses/CS3103/state/jira.json").read_text())
    assert output["reconciled_at"] == STAMP
    assert [row["key"] for row in output["issues"]] == ["STUDY-2", "STUDY-10"]
    assert output["issues"][0]["summary"] == "Changed"
    assert output["issues"][0]["labels"] == ["a", "z"]
    assert output["issues"][1]["summary"] == "Summary STUDY-10"


@pytest.mark.parametrize("setup", ["missing", "wrong_epic"])
def test_upsert_rejects_missing_cache_or_wrong_configured_epic(configured_vault, setup):
    vault, course = configured_vault
    target = vault / "courses/CS3103/state/jira.json"
    if setup == "wrong_epic":
        cache.reconcile(vault, course, reconcile())
        before = target.read_bytes()

    payload = {"epic": "STUDY-999" if setup == "wrong_epic" else "STUDY-1", "issue": issue()}
    with pytest.raises(cache.CacheError):
        cache.upsert(vault, course, payload)

    if setup == "missing":
        assert not target.exists()
    else:
        assert target.read_bytes() == before


def test_missing_optional_issue_values_are_stored_as_null(configured_vault):
    vault, course = configured_vault
    minimal = {"key": "STUDY-3", "type": "Task", "summary": "S", "status": "To Do"}
    cache.reconcile(vault, course, reconcile(issues=[minimal]))

    stored = json.loads((vault / "courses/CS3103/state/jira.json").read_text())["issues"][0]
    assert all(stored[field] is None for field in cache.OPTIONAL_ISSUE_FIELDS)


def test_timestamps_are_canonicalized(configured_vault):
    vault, course = configured_vault
    payload = reconcile(
        reconciled_at="2026-09-03T14:30:00+0800",
        issues=[issue(updated_at="2026-09-03T09:12:41Z")],
    )
    cache.reconcile(vault, course, payload)

    stored = json.loads((vault / "courses/CS3103/state/jira.json").read_text())
    assert stored["reconciled_at"] == "2026-09-03T14:30:00+08:00"
    assert stored["issues"][0]["updated_at"] == "2026-09-03T09:12:41+00:00"


def test_failed_atomic_replace_preserves_previous_bytes_and_cleans_scratch(
    configured_vault, monkeypatch
):
    vault, course = configured_vault
    cache.reconcile(vault, course, reconcile(issues=[issue()]))
    target = vault / "courses/CS3103/state/jira.json"
    before = target.read_bytes()

    def fail_replace(source, destination):
        raise OSError("replace failed")

    monkeypatch.setattr(cache.os, "replace", fail_replace)
    with pytest.raises(OSError, match="replace failed"):
        cache.upsert(
            vault,
            course,
            {"epic": "STUDY-1", "issue": issue("STUDY-3")},
        )

    assert target.read_bytes() == before
    assert list(target.parent.glob(".jira-*.json")) == []
