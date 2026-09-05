from __future__ import annotations

from pathlib import Path

import pytest
import yaml
from pydantic import ValidationError

from corum.canvas import sync
from corum.config import Features, load_course, load_workspace, resolve_features
from corum.workspace import validate_vault


def write_workspace(root: Path, *, features: dict | None = None, include_jira: bool = True):
    value = {
        "schema": 1,
        "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
        "canvas": {"host": "https://canvas.example.edu"},
        "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
    }
    if features is not None:
        value["features"] = features
    if include_jira:
        value["jira"] = {"site": "https://example.atlassian.net", "project": "STUDY"}
    (root / "corum.yaml").write_text(yaml.safe_dump(value))


def write_course(root: Path, code: str, *, features: dict | None = None, include_jira: bool = True):
    folder = root / "courses" / code
    folder.mkdir(parents=True)
    value = {"schema": 1, "code": code, "canvas": {"id": 1, "sources": []}}
    if include_jira:
        value["jira"] = {"epic": "STUDY-1"}
    if features is not None:
        value["features"] = features
    (folder / "course.yaml").write_text(yaml.safe_dump(value))


def test_jira_and_wiki_default_enabled(tmp_path):
    write_workspace(tmp_path, features=None)
    write_course(tmp_path, "CS3103")
    workspace = load_workspace(tmp_path)
    course = load_course(tmp_path, "CS3103")
    assert resolve_features(workspace, course) == Features(jira=True, wiki=True)


def test_course_can_disable_jira_without_jira_configuration(tmp_path):
    write_workspace(tmp_path, include_jira=False)
    write_course(
        tmp_path,
        "CS3103",
        features={"jira": {"enabled": False}},
        include_jira=False,
    )
    assert resolve_features(load_workspace(tmp_path), load_course(tmp_path, "CS3103")).jira is False


@pytest.mark.asyncio
async def test_disabled_course_ignores_poisoned_dormant_jira_during_sync(tmp_path, monkeypatch):
    write_workspace(tmp_path)
    workspace_value = yaml.safe_load((tmp_path / "corum.yaml").read_text())
    workspace_value["jira"] = {"site": "http://unsafe.example", "project": "../BAD"}
    (tmp_path / "corum.yaml").write_text(yaml.safe_dump(workspace_value))
    write_course(
        tmp_path,
        "CS3103",
        features={"jira": {"enabled": False}},
    )
    course_file = tmp_path / "courses/CS3103/course.yaml"
    course_value = yaml.safe_load(course_file.read_text())
    course_value["jira"] = {"epic": "../BAD"}
    course_file.write_text(yaml.safe_dump(course_value))
    (course_file.parent / "state").mkdir()
    (course_file.parent / "raw").mkdir()
    (course_file.parent / "state/canvas.json").write_text(
        '{"schema": 1, "synced_at": null, "sources": {}}'
    )
    monkeypatch.setenv("CORUM_CANVAS_TOKEN", "secret")

    workspace, courses = validate_vault(tmp_path)
    manifest = await sync.sync_course(tmp_path, courses[0], dry_run=True)

    assert workspace.jira is None
    assert courses[0].jira is None
    assert manifest.effective_features["jira"] is False


@pytest.mark.parametrize(
    "jira",
    [
        {"site": "http://example.atlassian.net", "project": "STUDY"},
        {"site": "https://example.atlassian.net/jira", "project": "STUDY"},
        {"site": "https://example.atlassian.net", "project": "../STUDY"},
        {
            "site": "https://example.atlassian.net",
            "project": "STUDY",
            "transitions": {"this_week": ""},
        },
    ],
)
def test_jira_workspace_rejects_unsafe_sites_and_identifiers(tmp_path, jira):
    write_workspace(tmp_path)
    value = yaml.safe_load((tmp_path / "corum.yaml").read_text())
    value["jira"] = jira
    (tmp_path / "corum.yaml").write_text(yaml.safe_dump(value))

    with pytest.raises(ValidationError):
        load_workspace(tmp_path)


def test_jira_course_rejects_invalid_epic_key(tmp_path):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    value = yaml.safe_load((tmp_path / "courses/CS3103/course.yaml").read_text())
    value["jira"]["epic"] = "../MYSELF"
    (tmp_path / "courses/CS3103/course.yaml").write_text(yaml.safe_dump(value))

    with pytest.raises(ValidationError):
        load_course(tmp_path, "CS3103")


def test_workspace_rejects_non_iana_timezone(tmp_path):
    write_workspace(tmp_path)
    value = yaml.safe_load((tmp_path / "corum.yaml").read_text())
    value["workspace"]["timezone"] = "Mars/Olympus_Mons"
    (tmp_path / "corum.yaml").write_text(yaml.safe_dump(value))

    with pytest.raises(ValidationError, match="timezone"):
        load_workspace(tmp_path)


@pytest.mark.parametrize(
    ("target", "field", "value"),
    [
        ("workspace", "unexpected", True),
        ("canvas", "unexpected", True),
        ("calendar", "unexpected", True),
        ("features", "unexpected", {"enabled": True}),
    ],
)
def test_workspace_rejects_unknown_fields_recursively(tmp_path, target, field, value):
    write_workspace(tmp_path)
    document = yaml.safe_load((tmp_path / "corum.yaml").read_text())
    document.setdefault(target, {})[field] = value
    (tmp_path / "corum.yaml").write_text(yaml.safe_dump(document))

    with pytest.raises(ValidationError, match="extra"):
        load_workspace(tmp_path)


def test_disabled_jira_still_rejects_yaml_credentials_without_validating_its_endpoint(
    tmp_path,
):
    write_workspace(
        tmp_path,
        features={"jira": {"enabled": False}, "wiki": {"enabled": True}},
    )
    document = yaml.safe_load((tmp_path / "corum.yaml").read_text())
    document["jira"] = {
        "site": "http://dormant.example.invalid/path",
        "project": "../DORMANT",
        "api_token": "must-not-live-in-yaml",
    }
    (tmp_path / "corum.yaml").write_text(yaml.safe_dump(document))

    with pytest.raises(ValueError, match="credential-like"):
        load_workspace(tmp_path, validate_jira=False)


def test_course_rejects_credential_like_keys_hidden_in_free_form_mappings(tmp_path):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    path = tmp_path / "courses/CS3103/course.yaml"
    document = yaml.safe_load(path.read_text())
    document["canvas"]["folders"] = {"api_token": "lectures"}
    path.write_text(yaml.safe_dump(document))

    with pytest.raises(ValueError, match="credential-like"):
        load_course(tmp_path, "CS3103")
