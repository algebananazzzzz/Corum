from pathlib import Path

import pytest

from corum.config import (
    CalendarFiles,
    CanvasWorkspace,
    FeatureSwitch,
    WorkspaceConfig,
    WorkspaceDetails,
    WorkspaceFeatures,
)
from corum.jira.rovo import AtlassianResource, JiraProject, RovoError
from corum.jira.setup import (
    JiraSelection,
    available_jira_choices,
    configured_workspace,
    write_workspace_atomic,
)


def test_current_compact_resource_can_configure_workspace_without_site_url():
    workspace = WorkspaceConfig(
        schema=1,
        workspace=WorkspaceDetails(timezone="Asia/Singapore", term="Term"),
        canvas=CanvasWorkspace(host="https://canvas.example.edu"),
        features=WorkspaceFeatures(
            jira=FeatureSwitch(enabled=False),
            wiki=FeatureSwitch(enabled=True),
        ),
        calendar=CalendarFiles(
            timetable=Path("Timetable.md"), term=Path("Term_Calendar.md")
        ),
    )

    configured = configured_workspace(
        workspace,
        JiraSelection(cloud_id="cloud-1", site=None, project="TODO"),
    )

    assert configured.jira is not None
    assert configured.jira.cloud_id == "cloud-1"
    assert configured.jira.site is None
    assert configured.jira.project == "TODO"


@pytest.mark.asyncio
async def test_available_choices_drop_resources_without_jira_projects():
    class Session:
        async def resources(self):
            return [
                AtlassianResource(id="empty", name=None, url=None),
                AtlassianResource(
                    id="ready", name="Study", url="https://study.atlassian.net"
                ),
            ]

        async def projects(self, cloud_id):
            return [] if cloud_id == "empty" else [JiraProject(key="TODO", name="Todo")]

    choices = await available_jira_choices(Session())

    assert [
        (resource.id, [project.key for project in projects])
        for resource, projects in choices
    ] == [("ready", ["TODO"])]


@pytest.mark.asyncio
async def test_available_choices_reject_when_no_project_is_accessible():
    class Session:
        async def resources(self):
            return [AtlassianResource(id="empty", name=None, url=None)]

        async def projects(self, cloud_id):
            return []

    with pytest.raises(RovoError, match="No accessible Jira projects"):
        await available_jira_choices(Session())


def test_atomic_workspace_update_preserves_transitions(tmp_path):
    path = tmp_path / "corum.yaml"
    workspace = WorkspaceConfig(
        schema=1,
        workspace=WorkspaceDetails(timezone="Asia/Singapore", term="Term"),
        canvas=CanvasWorkspace(host="https://canvas.example.edu"),
        features=WorkspaceFeatures(),
        jira={
            "site": "https://old.atlassian.net",
            "project": "OLD",
            "transitions": {"this_week": "2"},
        },
        calendar=CalendarFiles(
            timetable=Path("Timetable.md"), term=Path("Term_Calendar.md")
        ),
    )
    updated = configured_workspace(
        workspace,
        JiraSelection(cloud_id="cloud-1", site=None, project="TODO"),
    )

    write_workspace_atomic(path, updated)

    written = WorkspaceConfig.model_validate(
        __import__("yaml").safe_load(path.read_text())
    )
    assert written.jira is not None
    assert written.jira.transitions == {"this_week": "2"}
    assert written.jira.project == "TODO"
