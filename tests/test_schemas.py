from __future__ import annotations

import json
from pathlib import Path

import pytest
from jsonschema import ValidationError, validate


SCHEMAS = Path(__file__).parents[1] / "schemas"


def schema(name: str) -> dict:
    return json.loads((SCHEMAS / name).read_text())


def test_workspace_schema_rejects_jira_without_site_and_project():
    workspace = {
        "schema": 1,
        "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
        "canvas": {"host": "https://canvas.example.edu"},
        "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
        "jira": {},
    }

    with pytest.raises(ValidationError):
        validate(workspace, schema("corum.schema.json"))


def test_course_schema_rejects_jira_without_epic():
    course = {
        "schema": 1,
        "code": "CS3103",
        "canvas": {"id": 1, "sources": []},
        "jira": {},
    }

    with pytest.raises(ValidationError):
        validate(course, schema("course.schema.json"))
