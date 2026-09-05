from __future__ import annotations

from pathlib import Path

import yaml

from corum.config import Features, load_course, load_workspace, resolve_features


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
