from __future__ import annotations

import json
from pathlib import Path

import yaml
from jsonschema import ValidationError, validate
import pytest

from corum.workspace import initialize, validate_vault


ROOT = Path(__file__).parents[1]


def write_course(
    vault: Path,
    code: str,
    *,
    features: dict,
    include_jira: bool,
) -> None:
    course = vault / "courses" / code
    course.mkdir()
    value = {
        "schema": 1,
        "code": code,
        "canvas": {"id": 1, "sources": []},
        "features": features,
    }
    if include_jira:
        value["jira"] = {"epic": "DEMO-1"}
    (course / "course.yaml").write_text(
        yaml.safe_dump(value, sort_keys=False), encoding="utf-8"
    )


def test_initialize_installs_a_self_contained_agent_vault(tmp_path):
    vault = tmp_path / "vault"

    initialize(vault)

    assert (vault / "AGENTS.md").is_file()
    assert (vault / "skills/sync-course/SKILL.md").is_file()
    assert (vault / "skills/scope-course/SKILL.md").is_file()
    assert (vault / "templates/wiki/concept.md").is_file()
    assert not (vault / "AGENTS.base.md").exists()
    assert "{{COURSE}}" in (vault / "templates/wiki/concept.md").read_text(
        encoding="utf-8"
    )


def test_initialized_canvas_only_course_has_no_optional_state(tmp_path):
    vault = tmp_path / "vault"
    initialize(vault)
    write_course(
        vault,
        "DEMO",
        features={"jira": {"enabled": False}, "wiki": {"enabled": False}},
        include_jira=False,
    )

    workspace, courses = validate_vault(vault)
    course = vault / "courses" / "DEMO"
    assert workspace.features.jira.enabled is True
    assert len(courses) == 1
    assert not (course / "state/jira.json").exists()
    assert not (course / "state/wiki.json").exists()


def test_wiki_state_schema_owns_only_source_finalization():
    schema = json.loads((ROOT / "schemas/wiki-state.schema.json").read_text())
    validate({"schema": 1, "ingested": {"lectures/topic.pdf": "L1"}}, schema)

    with pytest.raises(ValidationError):
        validate(
            {
                "schema": 1,
                "ingested": {},
                "pages": {"Routing": {"id": "123", "hash": "abc"}},
            },
            schema,
        )
