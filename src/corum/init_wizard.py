"""Interactive Corum vault initialization."""

from __future__ import annotations

from collections.abc import Callable
from contextlib import AbstractAsyncContextManager
from pathlib import Path

from pydantic import ValidationError
import yaml

from .config import (
    CalendarFiles,
    CanvasWorkspace,
    FeatureSwitch,
    WorkspaceConfig,
    WorkspaceDetails,
    WorkspaceFeatures,
)
from .jira.rovo import RovoSession, open_rovo_session
from .jira.setup import JiraSelection, available_jira_choices, configured_workspace
from .prompts import PromptCancelled, Prompts
from .workspace import initialize


RovoSessionFactory = Callable[..., AbstractAsyncContextManager[RovoSession]]


def _validation_message(factory: Callable[[], object]) -> bool | str:
    try:
        factory()
    except (ValidationError, ValueError) as error:
        if isinstance(error, ValidationError):
            return str(error.errors()[0]["msg"])
        return str(error)
    return True


def _empty_target(value: str) -> bool | str:
    try:
        target = Path(value).expanduser().resolve()
        if target.exists() and (not target.is_dir() or any(target.iterdir())):
            return f"Target is not empty: {target}"
    except OSError as error:
        return f"Cannot inspect target: {error}"
    return True


async def select_jira(session: RovoSession, prompts: Prompts) -> JiraSelection:
    choices = await available_jira_choices(session)
    resource, projects = prompts.select(
        "Atlassian site",
        [
            (
                f"{resource.name or 'Jira site'} ({resource.url or resource.id})",
                (resource, projects),
            )
            for resource, projects in choices
        ],
    )
    project = prompts.select(
        "Jira project",
        [(f"{project.name} ({project.key})", project) for project in projects],
    )
    return JiraSelection(
        cloud_id=resource.id,
        site=str(resource.url) if resource.url is not None else None,
        project=project.key,
    )


async def run_init_wizard(
    proposed_root: Path,
    prompts: Prompts,
    *,
    session_factory: RovoSessionFactory = open_rovo_session,
) -> Path:
    root_text = prompts.text(
        "Vault path",
        default=str(proposed_root),
        validate=_empty_target,
    )
    root = Path(root_text).expanduser().resolve()
    target_check = _empty_target(str(root))
    if target_check is not True:
        raise ValueError(target_check)

    timezone = prompts.text(
        "Workspace timezone",
        default="Asia/Singapore",
        validate=lambda value: _validation_message(
            lambda: WorkspaceDetails(timezone=value, term="")
        ),
    )
    term = prompts.text("Academic term", default="AY2026/27 Semester 1")
    canvas_host = prompts.text(
        "Canvas URL",
        default="https://canvas.example.edu",
        validate=lambda value: _validation_message(lambda: CanvasWorkspace(host=value)),
    )
    wiki_enabled = prompts.confirm("Enable wiki authoring?", default=True)
    jira_enabled = prompts.confirm("Connect Jira?", default=False)

    workspace = WorkspaceConfig(
        schema=1,
        workspace=WorkspaceDetails(timezone=timezone, term=term),
        canvas=CanvasWorkspace(host=canvas_host),
        features=WorkspaceFeatures(
            jira=FeatureSwitch(enabled=False),
            wiki=FeatureSwitch(enabled=wiki_enabled),
        ),
        jira=None,
        calendar=CalendarFiles(timetable=Path("Timetable.md"), term=Path("Term_Calendar.md")),
    )
    if jira_enabled:
        async with session_factory() as session:
            workspace = configured_workspace(workspace, await select_jira(session, prompts))

    print("\nConfiguration preview:")
    print(yaml.safe_dump(workspace.model_dump(mode="json", exclude_none=True), sort_keys=False))
    if not prompts.confirm("Create this vault?", default=True):
        raise PromptCancelled("setup cancelled")
    return initialize(root, workspace)
