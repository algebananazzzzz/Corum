"""Creation and validation of Corum vaults."""

from __future__ import annotations

from pathlib import Path
import shutil
import sysconfig

import yaml

from .config import CourseConfig, WorkspaceConfig, load_course, load_workspace, resolve_features


DEFAULT_WORKSPACE = {
    "schema": 1,
    "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
    "canvas": {"host": "https://canvas.example.edu"},
    "features": {"jira": {"enabled": True}, "wiki": {"enabled": True}},
    "jira": {"site": "https://example.atlassian.net", "project": "STUDY"},
    "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
}


def _agent_kit() -> Path:
    candidates = (
        Path(sysconfig.get_path("data")) / "share/corum/agent-kit",
        Path(__file__).resolve().parents[2] / "agent-kit",
    )
    for candidate in candidates:
        if (candidate / "AGENTS.base.md").is_file():
            return candidate
    raise RuntimeError("installed Corum agent kit is missing")


def initialize(root: Path) -> Path:
    root = root.resolve()
    if root.exists() and (not root.is_dir() or any(root.iterdir())):
        raise ValueError(f"refusing to initialize non-empty target: {root}")
    agent_kit = _agent_kit()
    root.mkdir(parents=True, exist_ok=True)
    (root / "courses").mkdir()
    (root / "corum.yaml").write_text(yaml.safe_dump(DEFAULT_WORKSPACE, sort_keys=False))
    (root / "AGENTS.md").write_text(
        (agent_kit / "AGENTS.base.md").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    shutil.copytree(agent_kit / "skills", root / "skills")
    shutil.copytree(agent_kit / "templates", root / "templates")
    return root


def validate_vault(root: Path) -> tuple[WorkspaceConfig, list[CourseConfig]]:
    workspace = load_workspace(root)
    courses = []
    courses_directory = root / "courses"
    if not courses_directory.exists():
        return workspace, courses
    for course_file in sorted(courses_directory.glob("*/course.yaml")):
        directory_code = course_file.parent.name
        course = load_course(root, directory_code)
        if course.code != directory_code:
            raise ValueError(
                f"course code {course.code} does not match directory {directory_code}"
            )
        features = resolve_features(workspace, course)
        if features.jira and (workspace.jira is None or course.jira is None):
            raise ValueError(f"enabled Jira requires workspace and course Jira configuration for {course.code}")
        courses.append(course)
    return workspace, courses
