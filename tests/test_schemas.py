from __future__ import annotations

import json
from pathlib import Path

import pytest
from jsonschema import Draft202012Validator, FormatChecker, ValidationError, validate


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
def test_workspace_schema_rejects_unsafe_jira_boundaries(jira):
    workspace = {
        "schema": 1,
        "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
        "canvas": {"host": "https://canvas.example.edu"},
        "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
        "jira": jira,
    }
    validator = Draft202012Validator(
        schema("corum.schema.json"),
        format_checker=FormatChecker(),
    )

    assert list(validator.iter_errors(workspace))


def test_course_schema_rejects_invalid_epic_key():
    course = {
        "schema": 1,
        "code": "CS3103",
        "canvas": {"id": 1, "sources": []},
        "jira": {"epic": "../MYSELF"},
    }

    with pytest.raises(ValidationError):
        validate(course, schema("course.schema.json"))
