"""Vault configuration models and loaders."""

from __future__ import annotations

from pathlib import Path
from typing import Literal
import warnings
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

import yaml
from pydantic import BaseModel, ConfigDict, Field, HttpUrl, ValidationInfo, field_validator

from .validation import (
    require_https_origin,
    require_identifier,
    require_issue_key,
    require_nonblank,
    require_project_key,
    require_transition_id,
)


warnings.filterwarnings(
    "ignore",
    message=r'Field name "schema" in ".*" shadows an attribute in parent ".*"',
    category=UserWarning,
)


class _StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class FeatureSwitch(_StrictModel):
    enabled: bool = True


class Features(_StrictModel):
    jira: bool = True
    wiki: bool = True


class WorkspaceDetails(_StrictModel):
    timezone: str
    term: str

    @field_validator("timezone")
    @classmethod
    def timezone_is_iana(cls, value: str) -> str:
        try:
            ZoneInfo(value)
        except (ZoneInfoNotFoundError, ValueError) as error:
            raise ValueError(f"workspace timezone is not a valid IANA timezone: {value!r}") from error
        return value


class CanvasWorkspace(_StrictModel):
    host: HttpUrl

    @field_validator("host")
    @classmethod
    def host_is_https_origin(cls, value: HttpUrl) -> HttpUrl:
        require_https_origin(str(value), "Canvas host")
        return value


class WorkspaceFeatures(_StrictModel):
    jira: FeatureSwitch = Field(default_factory=FeatureSwitch)
    wiki: FeatureSwitch = Field(default_factory=FeatureSwitch)


class JiraWorkspace(_StrictModel):
    cloud_id: str | None = None
    site: HttpUrl | None = None
    project: str
    transitions: dict[str, str] = Field(default_factory=dict)

    @field_validator("cloud_id")
    @classmethod
    def cloud_id_is_nonblank(cls, value: str | None) -> str | None:
        if value is None:
            return None
        return require_nonblank(value, "Jira cloud ID")

    @field_validator("site")
    @classmethod
    def site_is_https_origin(
        cls,
        value: HttpUrl | None,
        info: ValidationInfo,
    ) -> HttpUrl | None:
        if value is not None and not (info.context or {}).get("skip_jira_semantics"):
            require_https_origin(str(value))
        return value

    @field_validator("project")
    @classmethod
    def project_has_jira_key_syntax(cls, value: str, info: ValidationInfo) -> str:
        return value if (info.context or {}).get("skip_jira_semantics") else require_project_key(value)

    @field_validator("transitions")
    @classmethod
    def transitions_are_named_ids(
        cls, value: dict[str, str], info: ValidationInfo
    ) -> dict[str, str]:
        if (info.context or {}).get("skip_jira_semantics"):
            return value
        for name, transition_id in value.items():
            require_identifier(name, "Jira transition name")
            require_transition_id(transition_id)
        return value


class CalendarFiles(_StrictModel):
    timetable: Path
    term: Path


class WorkspaceConfig(_StrictModel):
    schema: Literal[1]
    workspace: WorkspaceDetails
    canvas: CanvasWorkspace
    features: WorkspaceFeatures = Field(default_factory=WorkspaceFeatures)
    jira: JiraWorkspace | None = None
    calendar: CalendarFiles


class CanvasCourse(_StrictModel):
    id: int = Field(gt=0)
    sources: list[Literal["announcements", "assignments", "files", "pages", "modules", "syllabus"]]
    folders: dict[str, str] = Field(default_factory=dict)


class CourseFeatures(_StrictModel):
    jira: FeatureSwitch | None = None
    wiki: FeatureSwitch | None = None


class JiraCourse(_StrictModel):
    epic: str

    @field_validator("epic")
    @classmethod
    def epic_has_issue_key_syntax(cls, value: str, info: ValidationInfo) -> str:
        if (info.context or {}).get("skip_jira_semantics"):
            return value
        return require_issue_key(value, "Jira epic key")


class WikiCourse(_StrictModel):
    split_rules: str = "default"


class CourseConfig(_StrictModel):
    schema: Literal[1]
    code: str
    canvas: CanvasCourse
    features: CourseFeatures | None = None
    jira: JiraCourse | None = None
    wiki: WikiCourse | None = None


_CREDENTIAL_KEY_PARTS = {
    "token",
    "password",
    "secret",
    "credential",
    "credentials",
    "authorization",
    "apikey",
    "accesstoken",
    "clientsecret",
    "canvastoken",
    "jiratoken",
}


def _reject_credential_keys(value: object, path: str = "configuration") -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            normalized = "".join(character for character in str(key).casefold() if character.isalnum())
            segments = [
                part
                for part in str(key).casefold().replace("-", "_").split("_")
                if part
            ]
            key_parts = set(segments)
            key_parts.update(
                first + second for first, second in zip(segments, segments[1:])
            )
            if normalized in _CREDENTIAL_KEY_PARTS or key_parts & _CREDENTIAL_KEY_PARTS:
                raise ValueError(f"credential-like key is forbidden in YAML at {path}.{key}")
            _reject_credential_keys(child, f"{path}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            _reject_credential_keys(child, f"{path}[{index}]")


def _load_yaml(path: Path) -> dict:
    with path.open() as stream:
        value = yaml.safe_load(stream)
    if not isinstance(value, dict):
        raise ValueError(f"configuration at {path} must be a mapping")
    _reject_credential_keys(value, str(path))
    return value


def load_workspace(root: Path, *, validate_jira: bool = True) -> WorkspaceConfig:
    raw = _load_yaml(root / "corum.yaml")
    workspace = WorkspaceConfig.model_validate(
        raw,
        context={"skip_jira_semantics": not validate_jira},
    )
    return workspace if validate_jira else workspace.model_copy(update={"jira": None})


def load_course(root: Path, code: str, *, validate_jira: bool = True) -> CourseConfig:
    raw = _load_yaml(root / "courses" / code / "course.yaml")
    course = CourseConfig.model_validate(
        raw,
        context={"skip_jira_semantics": not validate_jira},
    )
    return course if validate_jira else course.model_copy(update={"jira": None})


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
