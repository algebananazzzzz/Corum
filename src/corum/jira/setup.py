"""Select Jira resources and update secret-free workspace configuration."""

from __future__ import annotations

from dataclasses import dataclass
import os
from pathlib import Path
import tempfile

from pydantic import HttpUrl
import yaml

from corum.config import FeatureSwitch, JiraWorkspace, WorkspaceConfig

from .rovo import AtlassianResource, JiraProject, RovoError, RovoSession


@dataclass(frozen=True)
class JiraSelection:
    cloud_id: str
    site: str
    project: str


async def available_jira_choices(
    session: RovoSession,
) -> list[tuple[AtlassianResource, list[JiraProject]]]:
    choices: list[tuple[AtlassianResource, list[JiraProject]]] = []
    for resource in await session.resources():
        projects = await session.projects(resource.id)
        if projects:
            choices.append((resource, projects))
    if not choices:
        raise RovoError("No accessible Jira projects were found")
    return choices


def configured_workspace(
    workspace: WorkspaceConfig,
    selection: JiraSelection,
) -> WorkspaceConfig:
    transitions = workspace.jira.transitions if workspace.jira else {}
    features = workspace.features.model_copy(
        update={"jira": FeatureSwitch(enabled=True)}
    )
    jira = JiraWorkspace(
        cloud_id=selection.cloud_id,
        site=HttpUrl(selection.site),
        project=selection.project,
        transitions=transitions,
    )
    return workspace.model_copy(update={"features": features, "jira": jira})


def write_workspace_atomic(path: Path, workspace: WorkspaceConfig) -> None:
    """Replace one workspace YAML file after validating the complete value."""

    validated = WorkspaceConfig.model_validate(workspace.model_dump(mode="json"))
    path.parent.mkdir(parents=True, exist_ok=True)
    scratch: Path | None = None
    try:
        with tempfile.NamedTemporaryFile(
            "w",
            encoding="utf-8",
            dir=path.parent,
            prefix=f".{path.stem}-",
            suffix=".yaml",
            delete=False,
        ) as temporary:
            scratch = Path(temporary.name)
            yaml.safe_dump(
                validated.model_dump(mode="json", exclude_none=True),
                temporary,
                sort_keys=False,
            )
            temporary.flush()
            os.fsync(temporary.fileno())
        os.replace(scratch, path)
    except BaseException:
        if scratch is not None:
            scratch.unlink(missing_ok=True)
        raise
