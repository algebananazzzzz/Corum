from __future__ import annotations

import json
from pathlib import Path

import pytest
import yaml
from jsonschema import Draft202012Validator, FormatChecker, ValidationError, validate

from corum.config import load_course, load_workspace


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


def test_valid_configuration_round_trips_through_models_and_published_schemas(
    tmp_path,
):
    workspace_document = {
        "schema": 1,
        "workspace": {"timezone": "America/New_York", "term": "Fall 2026"},
        "canvas": {"host": "https://canvas.example.edu"},
        "features": {
            "jira": {"enabled": True},
            "wiki": {"enabled": False},
        },
        "jira": {
            "site": "https://example.atlassian.net",
            "project": "STUDY",
            "transitions": {"this_week": "2"},
        },
        "calendar": {"timetable": "Timetable.md", "term": "Term.md"},
    }
    course_document = {
        "schema": 1,
        "code": "DEMO",
        "canvas": {
            "id": 1,
            "sources": ["announcements", "files"],
            "folders": {"Course Materials": "lectures"},
        },
        "features": {"jira": {"enabled": True}},
        "jira": {"epic": "STUDY-1"},
        "wiki": {"split_rules": "default"},
    }
    validate(workspace_document, schema("corum.schema.json"))
    validate(course_document, schema("course.schema.json"))
    (tmp_path / "corum.yaml").write_text(yaml.safe_dump(workspace_document))
    course = tmp_path / "courses/DEMO"
    course.mkdir(parents=True)
    (course / "course.yaml").write_text(yaml.safe_dump(course_document))

    workspace_model = load_workspace(tmp_path)
    course_model = load_course(tmp_path, "DEMO")

    validate(
        workspace_model.model_dump(mode="json"),
        schema("corum.schema.json"),
    )
    validate(
        course_model.model_dump(mode="json"),
        schema("course.schema.json"),
    )


@pytest.mark.parametrize(
    ("kind", "document"),
    [
        (
            "workspace",
            {
                "schema": 1,
                "workspace": {
                    "timezone": "Asia/Singapore",
                    "term": "Term",
                    "unexpected": True,
                },
                "canvas": {"host": "https://canvas.example.edu"},
                "calendar": {"timetable": "Timetable.md", "term": "Term.md"},
            },
        ),
        (
            "course",
            {
                "schema": 1,
                "code": "DEMO",
                "canvas": {
                    "id": 1,
                    "sources": [],
                    "folders": {"api_token": "secret"},
                },
            },
        ),
    ],
)
def test_configuration_models_and_schemas_both_reject_recursive_extras_and_credentials(
    tmp_path, kind, document
):
    filename = "corum.schema.json" if kind == "workspace" else "course.schema.json"
    with pytest.raises(ValidationError):
        validate(document, schema(filename))

    if kind == "workspace":
        (tmp_path / "corum.yaml").write_text(yaml.safe_dump(document))
        with pytest.raises(Exception):
            load_workspace(tmp_path)
    else:
        course = tmp_path / "courses/DEMO"
        course.mkdir(parents=True)
        (course / "course.yaml").write_text(yaml.safe_dump(document))
        with pytest.raises(Exception):
            load_course(tmp_path, "DEMO")


@pytest.mark.parametrize(
    ("kind", "credential_key"),
    [
        ("workspace", "clientSecret"),
        ("course", "accessToken"),
        ("course", "vendor_api_key"),
    ],
)
def test_free_form_configuration_maps_reject_credential_keys_in_models_and_schemas(
    tmp_path, kind, credential_key
):
    if kind == "workspace":
        document = {
            "schema": 1,
            "workspace": {"timezone": "Asia/Singapore", "term": "Term"},
            "canvas": {"host": "https://canvas.example.edu"},
            "jira": {
                "site": "https://example.atlassian.net",
                "project": "STUDY",
                "transitions": {credential_key: "2"},
            },
            "calendar": {"timetable": "Timetable.md", "term": "Term.md"},
        }
        filename = "corum.schema.json"
        (tmp_path / "corum.yaml").write_text(yaml.safe_dump(document))
        loader = lambda: load_workspace(tmp_path)
    else:
        document = {
            "schema": 1,
            "code": "DEMO",
            "canvas": {
                "id": 1,
                "sources": [],
                "folders": {credential_key: "lectures"},
            },
        }
        filename = "course.schema.json"
        course = tmp_path / "courses/DEMO"
        course.mkdir(parents=True)
        (course / "course.yaml").write_text(yaml.safe_dump(document))
        loader = lambda: load_course(tmp_path, "DEMO")

    with pytest.raises(ValidationError):
        validate(document, schema(filename))
    with pytest.raises(ValueError, match="credential-like"):
        loader()
