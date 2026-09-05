"""Vault configuration models and loaders."""

from __future__ import annotations

from pathlib import Path
from typing import Literal
import warnings

import yaml
from pydantic import BaseModel, Field, HttpUrl


warnings.filterwarnings(
    "ignore",
    message=r'Field name "schema" in ".*" shadows an attribute in parent "BaseModel"',
    category=UserWarning,
)


class FeatureSwitch(BaseModel):
    enabled: bool = True


class Features(BaseModel):
    jira: bool = True
    wiki: bool = True


class WorkspaceDetails(BaseModel):
    timezone: str
    term: str


class CanvasWorkspace(BaseModel):
    host: HttpUrl


class WorkspaceFeatures(BaseModel):
    jira: FeatureSwitch = Field(default_factory=FeatureSwitch)
    wiki: FeatureSwitch = Field(default_factory=FeatureSwitch)


class JiraWorkspace(BaseModel):
    site: HttpUrl
    project: str
    transitions: dict[str, str] = Field(default_factory=dict)


class CalendarFiles(BaseModel):
    timetable: Path
    term: Path


class WorkspaceConfig(BaseModel):
    schema: Literal[1]
    workspace: WorkspaceDetails
    canvas: CanvasWorkspace
    features: WorkspaceFeatures = Field(default_factory=WorkspaceFeatures)
    jira: JiraWorkspace | None = None
    calendar: CalendarFiles


class CanvasCourse(BaseModel):
    id: int = Field(gt=0)
    sources: list[Literal["announcements", "assignments", "files", "pages", "modules", "syllabus"]]
    folders: dict[str, str] = Field(default_factory=dict)


class CourseFeatures(BaseModel):
    jira: FeatureSwitch | None = None
    wiki: FeatureSwitch | None = None


class JiraCourse(BaseModel):
    epic: str


class WikiCourse(BaseModel):
    split_rules: str = "default"


class CourseConfig(BaseModel):
    schema: Literal[1]
    code: str
    canvas: CanvasCourse
    features: CourseFeatures | None = None
    jira: JiraCourse | None = None
    wiki: WikiCourse | None = None


def _load_yaml(path: Path) -> dict:
    with path.open() as stream:
        value = yaml.safe_load(stream)
    if not isinstance(value, dict):
        raise ValueError(f"configuration at {path} must be a mapping")
    return value


def load_workspace(root: Path) -> WorkspaceConfig:
    return WorkspaceConfig.model_validate(_load_yaml(root / "corum.yaml"))


def load_course(root: Path, code: str) -> CourseConfig:
    return CourseConfig.model_validate(_load_yaml(root / "courses" / code / "course.yaml"))


def resolve_features(workspace: WorkspaceConfig, course: CourseConfig) -> Features:
    course_features = course.features
    return Features(
        jira=(
            course_features.jira.enabled
            if course_features is not None and course_features.jira is not None
            else workspace.features.jira.enabled
        ),
        wiki=(
            course_features.wiki.enabled
            if course_features is not None and course_features.wiki is not None
            else workspace.features.wiki.enabled
        ),
    )
