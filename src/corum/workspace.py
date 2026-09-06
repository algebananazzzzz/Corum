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
    "features": {"jira": {"enabled": False}, "wiki": {"enabled": True}},
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


def initialize(
    root: Path,
    workspace: WorkspaceConfig | dict | None = None,
) -> Path:
    root = root.resolve()
    if root.exists() and (not root.is_dir() or any(root.iterdir())):
        raise ValueError(f"refusing to initialize non-empty target: {root}")
    value = WorkspaceConfig.model_validate(workspace or DEFAULT_WORKSPACE)
    agent_kit = _agent_kit()
    root.mkdir(parents=True, exist_ok=True)
    (root / "courses").mkdir()
    (root / "corum.yaml").write_text(
        yaml.safe_dump(
            value.model_dump(mode="json", exclude_none=True),
            sort_keys=False,
        ),
        encoding="utf-8",
    )
    (root / "AGENTS.md").write_text(
        (agent_kit / "AGENTS.base.md").read_text(encoding="utf-8"),
        encoding="utf-8",
    )
    shutil.copytree(agent_kit / "skills", root / "skills")
    shutil.copytree(agent_kit / "templates", root / "templates")
    return root


def validate_vault(root: Path) -> tuple[WorkspaceConfig, list[CourseConfig]]:
    workspace = load_workspace(root, validate_jira=False)
    courses = []
    courses_directory = root / "courses"
    course_files = (
        sorted(courses_directory.glob("*/course.yaml"))
        if courses_directory.exists()
        else []
    )
    if not course_files:
        if workspace.features.jira.enabled:
            workspace = load_workspace(root)
        return workspace, courses
    for course_file in course_files:
        directory_code = course_file.parent.name
        course = load_course(root, directory_code, validate_jira=False)
        if course.code != directory_code:
            raise ValueError(
                f"course code {course.code} does not match directory {directory_code}"
            )
        features = resolve_features(workspace, course)
        if features.jira:
            if workspace.jira is None:
                workspace = load_workspace(root)
            course = load_course(root, directory_code)
            if workspace.jira is None or course.jira is None:
                raise ValueError(
                    f"enabled Jira requires workspace and course Jira configuration for {course.code}"
                )
        courses.append(course)
    return workspace, courses


def validate_selected_courses(
    root: Path,
    requested_codes: list[str],
) -> tuple[WorkspaceConfig, list[CourseConfig]]:
    """Validate only explicitly selected courses, leaving unrelated files untouched."""
    workspace = load_workspace(root, validate_jira=False)
    courses_directory = root / "courses"
    available = {
        path.parent.name.upper(): path.parent.name
        for path in courses_directory.glob("*/course.yaml")
    } if courses_directory.exists() else {}
    missing = [code.upper() for code in requested_codes if code.upper() not in available]
    if missing:
        raise ValueError(f"no course configuration for: {', '.join(missing)}")

    courses: list[CourseConfig] = []
    for requested in requested_codes:
        directory_code = available[requested.upper()]
        course = load_course(root, directory_code, validate_jira=False)
        if course.code != directory_code:
            raise ValueError(
                f"course code {course.code} does not match directory {directory_code}"
            )
        if resolve_features(workspace, course).jira:
            if workspace.jira is None:
                workspace = load_workspace(root)
            course = load_course(root, directory_code)
            if workspace.jira is None or course.jira is None:
                raise ValueError(
                    f"enabled Jira requires workspace and course Jira configuration for {course.code}"
                )
        courses.append(course)
    return workspace, courses
