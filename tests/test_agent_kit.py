from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import Path

import yaml


ROOT = Path(__file__).parents[1]
AGENT_KIT = ROOT / "agent-kit"


def _frontmatter(path: Path) -> dict:
    _, frontmatter, _ = path.read_text(encoding="utf-8").split("---", 2)
    return yaml.safe_load(frontmatter)


def test_agent_kit_contains_only_declared_skills():
    root = AGENT_KIT / "skills"

    assert {path.name for path in root.iterdir() if path.is_dir()} == {
        "authoring-wiki",
        "drawio-diagrams",
        "linting-wiki",
        "scope-course",
        "sync-course",
    }


def test_each_skill_has_discoverable_frontmatter_and_resolved_references():
    for skill_dir in sorted((AGENT_KIT / "skills").iterdir()):
        skill = skill_dir / "SKILL.md"
        metadata = _frontmatter(skill)
        assert metadata["name"] == skill_dir.name
        assert metadata["description"].startswith("Use when ")
        assert len(yaml.safe_dump(metadata)) <= 1024

        relative_markdown_links = re.findall(
            r"\[[^]]+\]\((?!https?://)([^)#]+\.md)(?:#[^)]+)?\)",
            skill.read_text(encoding="utf-8"),
        )
        assert all((skill_dir / target).is_file() for target in relative_markdown_links)


def test_agent_kit_has_generic_templates_without_legacy_dependencies():
    expected_templates = {
        "Changelog.md",
        "Conventions and Milestones.md",
        "wiki/concept.md",
        "wiki/explainer.md",
        "wiki/index.md",
        "wiki/reference.md",
    }
    templates = AGENT_KIT / "templates"
    assert {
        str(path.relative_to(templates))
        for path in templates.rglob("*.md")
    } == expected_templates

    files = [
        path
        for path in AGENT_KIT.rglob("*")
        if path.is_file() and path.suffix in {".json", ".md", ".py", ".yaml"}
    ]
    combined = "\n".join(path.read_text(encoding="utf-8") for path in files)
    forbidden = (
        r"canvas[_-]mcp|Rovo MCP|\.mcp\.json|connectors/|"
        r"\b[A-Z]{2,4}\d{4}[A-Z]?\b|"
        r"\b[a-zA-Z0-9-]+\.atlassian\.net\b"
    )
    assert re.search(forbidden, combined, flags=re.IGNORECASE) is None


def test_linter_reports_index_drift_from_the_v1_vault_layout(tmp_path):
    course = tmp_path / "courses" / "DEMO"
    (course / "state").mkdir(parents=True)
    (course / "wiki" / "concepts").mkdir(parents=True)
    (course / "state" / "wiki.json").write_text(
        '{"schema": 1, "ingested": {}}', encoding="utf-8"
    )
    (course / "wiki" / "index.md").write_text("# Index\n", encoding="utf-8")
    (course / "wiki" / "concepts" / "Routing.md").write_text(
        "# Routing\n\n## Route lookup\n%% L1 p1 %%\n", encoding="utf-8"
    )

    completed = subprocess.run(
        [
            sys.executable,
            str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py"),
            "DEMO",
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert completed.returncode == 0
    assert "drift" in completed.stdout
    assert "courses/DEMO/wiki/concepts/Routing" in completed.stdout
    assert "marker L1 cites no finalized source" in completed.stdout

    preview = subprocess.run(
        [
            sys.executable,
            str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py"),
            "DEMO",
            "--pending",
            "L1=lectures/topic.md",
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert preview.returncode == 0
    assert "coverage" not in preview.stdout
    assert json.loads((course / "state/wiki.json").read_text()) == {
        "schema": 1,
        "ingested": {},
    }


def test_linter_rejects_a_pending_source_outside_the_course(tmp_path):
    course = tmp_path / "courses" / "DEMO"
    (course / "state").mkdir(parents=True)
    (course / "wiki").mkdir()
    (course / "state/wiki.json").write_text(
        '{"schema": 1, "ingested": {}}', encoding="utf-8"
    )
    (course / "wiki/index.md").write_text("# Index\n", encoding="utf-8")

    completed = subprocess.run(
        [
            sys.executable,
            str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py"),
            "DEMO",
            "--pending",
            "L1=../../outside.pdf",
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert completed.returncode == 1
    assert "outside course raw directory" in completed.stdout
