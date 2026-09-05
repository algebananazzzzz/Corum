from __future__ import annotations

import importlib.util
import json
import os
import re
import subprocess
import sys
from pathlib import Path

import pytest
import yaml


ROOT = Path(__file__).parents[1]
AGENT_KIT = ROOT / "agent-kit"


def _frontmatter(path: Path) -> dict:
    _, frontmatter, _ = path.read_text(encoding="utf-8").split("---", 2)
    return yaml.safe_load(frontmatter)


def _write_minimal_pdf(path: Path) -> None:
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] >>",
    ]
    body = b"%PDF-1.4\n"
    offsets = [0]
    for number, value in enumerate(objects, 1):
        offsets.append(len(body))
        body += f"{number} 0 obj\n".encode() + value + b"\nendobj\n"
    xref = len(body)
    body += f"xref\n0 {len(objects) + 1}\n".encode()
    body += b"0000000000 65535 f \n"
    body += b"".join(f"{offset:010d} 00000 n \n".encode() for offset in offsets[1:])
    body += (
        f"trailer\n<< /Size {len(objects) + 1} /Root 1 0 R >>\n"
        f"startxref\n{xref}\n%%EOF\n"
    ).encode()
    path.write_bytes(body)


def _load_drawio_pair_module():
    path = AGENT_KIT / "skills/drawio-diagrams/scripts/drawio_pair.py"
    spec = importlib.util.spec_from_file_location("corum_test_drawio_pair", path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _write_drawio_pair(svg: Path) -> None:
    svg.write_text(
        '<svg xmlns="http://www.w3.org/2000/svg">'
        "<metadata>Short</metadata><text>Short</text>"
        "<text>Short suffix</text></svg>",
        encoding="utf-8",
    )
    Path(f"{svg}.xml").write_text(
        '<mxfile><diagram><mxGraphModel><root>'
        '<mxCell id="0"/><mxCell id="1" parent="0"/>'
        '<mxCell id="2" parent="1" value="Short"/>'
        '<mxCell id="3" parent="1" value="Short suffix"/>'
        "</root></mxGraphModel></diagram></mxfile>",
        encoding="utf-8",
    )


def _write_custom_drawio_pair(svg: Path, svg_body: str, cells: list[str]) -> None:
    svg.write_text(
        f'<svg xmlns="http://www.w3.org/2000/svg">{svg_body}</svg>',
        encoding="utf-8",
    )
    encoded_cells = "".join(
        f'<mxCell id="{index}" parent="1" value="{value}"/>'
        for index, value in enumerate(cells, 2)
    )
    Path(f"{svg}.xml").write_text(
        '<mxfile><diagram><mxGraphModel><root>'
        '<mxCell id="0"/><mxCell id="1" parent="0"/>'
        f"{encoded_cells}</root></mxGraphModel></diagram></mxfile>",
        encoding="utf-8",
    )


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
    (course / "raw" / "lectures").mkdir(parents=True)
    (course / "wiki" / "concepts").mkdir(parents=True)
    (course / "state" / "wiki.json").write_text(
        '{"schema": 1, "ingested": {}}', encoding="utf-8"
    )
    (course / "wiki" / "index.md").write_text("# Index\n", encoding="utf-8")
    (course / "wiki" / "concepts" / "Routing.md").write_text(
        "# Routing\n\n## Route lookup\n%% L1 p1 %%\n", encoding="utf-8"
    )
    (course / "raw" / "lectures" / "topic.md").write_text(
        "Captured topic", encoding="utf-8"
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


@pytest.mark.parametrize(
    ("relative", "contents", "message"),
    [
        ("missing.md", None, "could not read source"),
        ("corrupt.pdf", b"not a PDF", "could not inspect PDF"),
    ],
)
def test_linter_rejects_missing_or_corrupt_pending_sources(
    tmp_path, relative, contents, message
):
    course = tmp_path / "courses" / "DEMO"
    (course / "state").mkdir(parents=True)
    (course / "raw").mkdir()
    (course / "wiki").mkdir()
    (course / "state/wiki.json").write_text(
        '{"schema": 1, "ingested": {}}', encoding="utf-8"
    )
    (course / "wiki/index.md").write_text("# Index\n", encoding="utf-8")
    if contents is not None:
        (course / "raw" / relative).write_bytes(contents)

    completed = subprocess.run(
        [
            sys.executable,
            str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py"),
            "DEMO",
            "--pending",
            f"L1={relative}",
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert completed.returncode == 1
    assert message in completed.stdout


def test_linter_reports_citations_beyond_the_pdf_page_count(tmp_path):
    course = tmp_path / "courses" / "DEMO"
    (course / "state").mkdir(parents=True)
    (course / "raw").mkdir()
    (course / "wiki" / "concepts").mkdir(parents=True)
    (course / "state/wiki.json").write_text(
        '{"schema": 1, "ingested": {}}', encoding="utf-8"
    )
    (course / "wiki/index.md").write_text("# Index\n", encoding="utf-8")
    (course / "wiki/concepts/Topic.md").write_text(
        "# Topic\n\n%% L1 p1-999 %%\n", encoding="utf-8"
    )
    _write_minimal_pdf(course / "raw/topic.pdf")

    completed = subprocess.run(
        [
            sys.executable,
            str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py"),
            "DEMO",
            "--pending",
            "L1=topic.pdf",
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert completed.returncode == 0
    assert "out of range p2-999" in completed.stdout


def test_linter_previews_null_provenance_without_mutating_state(tmp_path):
    course = tmp_path / "courses" / "DEMO"
    (course / "state").mkdir(parents=True)
    (course / "raw/notes").mkdir(parents=True)
    (course / "wiki").mkdir()
    state = '{"schema": 1, "ingested": {}}'
    (course / "state/wiki.json").write_text(state, encoding="utf-8")
    (course / "wiki/index.md").write_text("# Index\n", encoding="utf-8")
    (course / "raw/notes/duplicate.md").write_text("duplicate", encoding="utf-8")

    completed = subprocess.run(
        [
            sys.executable,
            str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py"),
            "DEMO",
            "--pending-null",
            "notes/duplicate.md",
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert completed.returncode == 0
    assert completed.stdout.strip() == "DEMO clean"
    assert (course / "state/wiki.json").read_text(encoding="utf-8") == state


def test_linter_rejects_a_corrupt_null_provenance_pdf(tmp_path):
    course = tmp_path / "courses" / "DEMO"
    (course / "state").mkdir(parents=True)
    (course / "raw").mkdir()
    (course / "wiki").mkdir()
    (course / "state/wiki.json").write_text(
        '{"schema": 1, "ingested": {}}', encoding="utf-8"
    )
    (course / "wiki/index.md").write_text("# Index\n", encoding="utf-8")
    (course / "raw/corrupt.pdf").write_bytes(b"not a PDF")

    completed = subprocess.run(
        [
            sys.executable,
            str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py"),
            "DEMO",
            "--pending-null",
            "corrupt.pdf",
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert completed.returncode == 1
    assert "could not inspect PDF" in completed.stdout


def test_linter_rejects_empty_provenance_labels_in_state_and_preview(tmp_path):
    course = tmp_path / "courses" / "DEMO"
    (course / "state").mkdir(parents=True)
    (course / "raw/notes").mkdir(parents=True)
    (course / "wiki").mkdir()
    (course / "wiki/index.md").write_text("# Index\n", encoding="utf-8")
    (course / "raw/notes/topic.md").write_text("topic", encoding="utf-8")
    script = str(AGENT_KIT / "skills/linting-wiki/scripts/lint-wiki.py")

    (course / "state/wiki.json").write_text(
        '{"schema": 1, "ingested": {"notes/topic.md": ""}}', encoding="utf-8"
    )
    stored = subprocess.run(
        [sys.executable, script, "DEMO"],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )
    (course / "state/wiki.json").write_text(
        '{"schema": 1, "ingested": {}}', encoding="utf-8"
    )
    pending = subprocess.run(
        [sys.executable, script, "DEMO", "--pending", " =notes/topic.md"],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )

    assert stored.returncode == 1
    assert "non-empty provenance label" in stored.stdout
    assert pending.returncode == 1
    assert "pending source must be LABEL=RELATIVE_PATH" in pending.stdout


def test_drawio_replace_changes_only_exact_label_nodes(tmp_path):
    module = _load_drawio_pair_module()
    svg = tmp_path / "diagram.svg"
    _write_drawio_pair(svg)

    source_hits, svg_hits = module.DrawioPair(svg).replace("Short", "Long")

    assert (source_hits, svg_hits) == (1, 1)
    assert "Short suffix" in svg.read_text(encoding="utf-8")
    assert "<ns0:metadata>Short</ns0:metadata>" in svg.read_text(encoding="utf-8")
    assert "Short suffix" in Path(f"{svg}.xml").read_text(encoding="utf-8")
    assert "Long suffix" not in svg.read_text(encoding="utf-8")
    assert "Long suffix" not in Path(f"{svg}.xml").read_text(encoding="utf-8")
    module.DrawioPair(svg).validate(strict=True)


def test_drawio_replace_ignores_a_matching_fragment_in_a_larger_visible_label(tmp_path):
    module = _load_drawio_pair_module()
    svg = tmp_path / "diagram.svg"
    _write_custom_drawio_pair(
        svg,
        "<text>Short</text>"
        "<text><tspan>Short</tspan><tspan> suffix</tspan></text>",
        ["Short", "Short suffix"],
    )

    source_hits, svg_hits = module.DrawioPair(svg).replace("Short", "Long")

    assert (source_hits, svg_hits) == (1, 1)
    root = module.ET.fromstring(svg.read_text(encoding="utf-8"))
    labels = [
        "".join(node.itertext())
        for node in root.iter()
        if module.local_name(node.tag) == "text"
    ]
    assert labels == ["Long", "Short suffix"]
    module.DrawioPair(svg).validate(strict=True)


def test_drawio_replace_preserves_inline_source_label_markup(tmp_path):
    module = _load_drawio_pair_module()
    svg = tmp_path / "diagram.svg"
    _write_custom_drawio_pair(
        svg,
        "<text>Short</text>",
        ["&lt;b&gt;Short&lt;/b&gt;"],
    )

    module.DrawioPair(svg).replace("Short", "Long")

    pair = module.DrawioPair(svg)
    values = [cell.get("value") for cell in pair.cells() if cell.get("value")]
    assert values == ["<b>Long</b>"]
    pair.validate(strict=True)


def test_drawio_replace_failure_preserves_both_original_files(tmp_path, monkeypatch):
    module = _load_drawio_pair_module()
    svg = tmp_path / "diagram.svg"
    _write_drawio_pair(svg)
    source = Path(f"{svg}.xml")
    original_svg = svg.read_bytes()
    original_source = source.read_bytes()
    real_replace = os.replace

    def fail_svg_commit(staged, target):
        if Path(target) == svg:
            raise OSError("SVG commit failed")
        real_replace(staged, target)

    with monkeypatch.context() as patcher:
        patcher.setattr(os, "replace", fail_svg_commit)
        with pytest.raises(OSError, match="SVG commit failed"):
            module.DrawioPair(svg).replace("Short", "Long")

    assert svg.read_bytes() == original_svg
    assert source.read_bytes() == original_source
