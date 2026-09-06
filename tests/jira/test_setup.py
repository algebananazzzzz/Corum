from pathlib import Path

from corum.config import (
    CalendarFiles,
    CanvasWorkspace,
    FeatureSwitch,
    WorkspaceConfig,
    WorkspaceDetails,
    WorkspaceFeatures,
)
from corum.jira.setup import JiraSelection, configured_workspace


def test_current_compact_resource_can_configure_workspace_without_site_url():
    workspace = WorkspaceConfig(
        schema=1,
        workspace=WorkspaceDetails(timezone="Asia/Singapore", term="Term"),
        canvas=CanvasWorkspace(host="https://canvas.example.edu"),
        features=WorkspaceFeatures(
            jira=FeatureSwitch(enabled=False),
            wiki=FeatureSwitch(enabled=True),
        ),
        calendar=CalendarFiles(timetable=Path("Timetable.md"), term=Path("Term_Calendar.md")),
    )

    configured = configured_workspace(
        workspace,
        JiraSelection(cloud_id="cloud-1", site=None, project="TODO"),
    )

    assert configured.jira is not None
    assert configured.jira.cloud_id == "cloud-1"
    assert configured.jira.site is None
    assert configured.jira.project == "TODO"
