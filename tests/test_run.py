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


def test_missing_canvas_state_bootstraps_only_watched_sources_without_writing(tmp_path):
    course_dir = tmp_path / "courses" / "CS3103"
    course_dir.mkdir(parents=True)

    state = read_canvas_state(course_dir, ["announcements", "syllabus"])

    assert state == {
        "schema": 1,
        "synced_at": None,
        "sources": {"announcements": {}, "syllabus": None},
    }
    assert not (course_dir / "state/canvas.json").exists()


def test_canvas_state_refuses_an_escaping_state_symlink(tmp_path):
    course_dir = tmp_path / "courses" / "CS3103"
    outside = tmp_path / "outside"
    course_dir.mkdir(parents=True)
    outside.mkdir()
    (course_dir / "state").symlink_to(outside, target_is_directory=True)

    with pytest.raises(ValueError, match="outside"):
        write_canvas_state(course_dir, {"schema": 1, "synced_at": None, "sources": {}})

    assert not (outside / "canvas.json").exists()


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
        "id": "legacy:file:00928bcc09c8e57b",
        "source": "file",
        "item_id": None,
        "kind": "file",
        "status": "changed",
        "summary": "file 9 · lectures/L2.pdf",
        "raw_path": None,
        "details": {},
    }
    assert failed.canvas.failures[0].model_dump() == {
        "id": "canvas:pages",
        "source": "pages",
        "item_id": None,
        "kind": "pages",
        "status": "failed",
        "error": "HTTP 403",
        "raw_path": None,
        "details": {},
    }


def test_run_manifest_normalizes_the_pre_rich_manifest_shape_on_read():
    legacy = {
        "schema": 1,
        "run_id": "20260906T120000+0800",
        "course": "CS3103",
        "effective_features": {"jira": True, "wiki": True},
        "canvas": {
            "status": "partial",
            "changes": [{"kind": "file", "summary": "file 9 · lectures/L2.pdf"}],
            "failures": [{"source": "pages", "error": "HTTP 403"}],
        },
        "jira": {"status": "pending"},
        "wiki": {"status": "pending"},
    }

    manifest = RunManifest.model_validate(legacy)

    assert manifest.canvas.changes[0].id == "legacy:file:00928bcc09c8e57b"
    assert manifest.canvas.changes[0].source == "file"
    assert manifest.canvas.failures[0].id == "canvas:pages"
    assert manifest.canvas.failures[0].kind == "pages"
    assert manifest.jira.applied == []


def test_run_id_uses_the_validated_workspace_timezone():
    from datetime import UTC, datetime

    manifest = RunManifest.create(
        "CS3103",
        {"jira": False, "wiki": False},
        now=datetime(2026, 9, 6, 1, 30, tzinfo=UTC),
        timezone="America/New_York",
    )

    assert manifest.run_id == "20260905T213000-0400"


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


def test_selected_course_sync_ignores_invalid_unrelated_course(tmp_path, monkeypatch, capsys):
    vault = make_cli_vault(tmp_path)
    unrelated = vault / "courses/BROKEN"
    unrelated.mkdir()
    (unrelated / "course.yaml").write_text("not: [valid", encoding="utf-8")
    monkeypatch.chdir(vault)
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")

    assert main(["sync", "CS3103", "--dry-run", "--json"]) == 0
    output = json.loads(capsys.readouterr().out)
    assert [entry["course"] for entry in output["courses"]] == ["CS3103"]


def test_wiki_finalization_rolls_back_both_state_files_if_second_replace_fails(
    tmp_path, monkeypatch
):
    from corum import state as state_module
    from corum.run import StageResult

    course = tmp_path / "courses/CS3103"
    (course / "state").mkdir(parents=True)
    (course / "state/wiki.json").write_text(
        '{"schema": 1, "ingested": {"old.md": "OLD"}}\n', encoding="utf-8"
    )
    manifest = RunManifest.create(
        "CS3103",
        {"jira": False, "wiki": True},
        timezone="Asia/Singapore",
    )
    state_module.write_latest_run(course, manifest.model_dump(mode="json"))
    wiki_before = (course / "state/wiki.json").read_bytes()
    run_before = (course / "state/latest-run.json").read_bytes()
    real_replace = state_module.os.replace
    failed = False

    def fail_latest_once(source, target):
        nonlocal failed
        if Path(target).name == "latest-run.json" and not failed:
            failed = True
            raise OSError("latest-run commit failed")
        real_replace(source, target)

    monkeypatch.setattr(state_module.os, "replace", fail_latest_once)

    with pytest.raises(OSError, match="latest-run commit failed"):
        state_module.finalize_wiki_state(
            course,
            expected_run_id=manifest.run_id,
            ingested={"new.md": "NEW"},
            stage=StageResult(status="applied").model_dump(mode="json"),
        )

    assert (course / "state/wiki.json").read_bytes() == wiki_before
    assert (course / "state/latest-run.json").read_bytes() == run_before
