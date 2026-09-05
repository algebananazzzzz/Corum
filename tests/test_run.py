from __future__ import annotations

import json
from pathlib import Path

import pytest
import yaml
from jsonschema import ValidationError, validate

from corum.cli import main
from corum.run import RunManifest
from corum.state import read_canvas_state, write_canvas_state


SCHEMAS = Path(__file__).parents[1] / "schemas"


def test_canvas_state_is_atomically_replaced_without_temp_residue(tmp_path):
    course_dir = tmp_path / "courses" / "CS3103"
    (course_dir / "state").mkdir(parents=True)
    original = {"schema": 1, "synced_at": None, "sources": {"announcements": {}}}
    updated = {
        "schema": 1,
        "synced_at": "2026-09-06T12:00:00+08:00",
        "sources": {"announcements": {"9": "2026-09-06T11:00:00+08:00"}},
    }
    write_canvas_state(course_dir, original)
    write_canvas_state(course_dir, updated)
    assert read_canvas_state(course_dir) == updated
    assert sorted(path.name for path in (course_dir / "state").iterdir()) == ["canvas.json"]


def test_run_manifest_derives_structured_canvas_statuses():
    unchanged = RunManifest.create("CS3103", {"jira": False, "wiki": True})
    changed = RunManifest.create(
        "CS3103", {"jira": False, "wiki": True}, changes=["file 9 · lectures/L2.pdf"]
    )
    failed = RunManifest.create(
        "CS3103", {"jira": False, "wiki": True}, failures={"pages": "HTTP 403"}
    )
    partial = RunManifest.create(
        "CS3103",
        {"jira": False, "wiki": True},
        changes=["syllabus changed"],
        failures={"pages": "HTTP 403"},
    )
    assert [manifest.canvas.status for manifest in (unchanged, changed, failed, partial)] == [
        "up_to_date",
        "changed",
        "failed",
        "partial",
    ]
    assert changed.canvas.changes[0].model_dump() == {
        "kind": "file",
        "summary": "file 9 · lectures/L2.pdf",
    }
    assert failed.canvas.failures[0].model_dump() == {"source": "pages", "error": "HTTP 403"}


def test_new_schemas_validate_real_state_and_manifest_shapes():
    state = {"schema": 1, "synced_at": None, "sources": {"syllabus": None, "files": {"1": "a.pdf"}}}
    manifest = RunManifest.create("CS3103", {"jira": False, "wiki": True}).model_dump(mode="json")
    validate(state, json.loads((SCHEMAS / "canvas-state.schema.json").read_text()))
    validate(manifest, json.loads((SCHEMAS / "run-manifest.schema.json").read_text()))


def test_run_manifest_schema_rejects_unknown_stage_status():
    manifest = RunManifest.create("CS3103", {"jira": False, "wiki": True}).model_dump(mode="json")
    manifest["canvas"]["status"] = "unknown"
    with pytest.raises(ValidationError):
        validate(manifest, json.loads((SCHEMAS / "run-manifest.schema.json").read_text()))


def make_cli_vault(tmp_path: Path) -> Path:
    vault = tmp_path / "vault"
    course = vault / "courses" / "CS3103"
    (course / "state").mkdir(parents=True)
    (vault / "corum.yaml").write_text(
        yaml.safe_dump(
            {
                "schema": 1,
                "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
                "canvas": {"host": "https://canvas.example.edu"},
                "features": {"jira": {"enabled": False}, "wiki": {"enabled": False}},
                "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
            }
        )
    )
    (course / "course.yaml").write_text(
        yaml.safe_dump({"schema": 1, "code": "CS3103", "canvas": {"id": 93794, "sources": []}})
    )
    write_canvas_state(course, {"schema": 1, "synced_at": None, "sources": {}})
    return vault


@pytest.mark.parametrize("selection", [["CS3103"], ["--all"]])
def test_sync_cli_selects_courses_and_emits_json_dry_run(tmp_path, monkeypatch, capsys, selection):
    vault = make_cli_vault(tmp_path)
    monkeypatch.chdir(vault)
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")

    assert main(["sync", *selection, "--dry-run", "--json"]) == 0

    output = json.loads(capsys.readouterr().out)
    assert output["dry_run"] is True
    assert [course["course"] for course in output["courses"]] == ["CS3103"]
    assert output["courses"][0]["canvas"]["status"] == "up_to_date"
    assert not (vault / "courses/CS3103/state/latest-run.json").exists()


def test_sync_cli_rejects_courses_together_with_all(tmp_path, monkeypatch, capsys):
    vault = make_cli_vault(tmp_path)
    monkeypatch.chdir(vault)
    assert main(["sync", "CS3103", "--all"]) == 1
    assert "course codes or --all" in capsys.readouterr().out


def test_sync_cli_reports_an_existing_lock_without_a_traceback(tmp_path, monkeypatch, capsys):
    vault = make_cli_vault(tmp_path)
    monkeypatch.chdir(vault)
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")
    (vault / "courses/CS3103/state/.sync.lock").write_text("another process")

    assert main(["sync", "CS3103", "--dry-run"]) == 1
    assert "already locked" in capsys.readouterr().out
