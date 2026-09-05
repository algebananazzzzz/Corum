"""Validate and sequentially apply exact Jira plans."""

from __future__ import annotations

from datetime import UTC, date, datetime
from pathlib import Path
from typing import Annotated, Any, Literal

from pydantic import AfterValidator, BaseModel, BeforeValidator, ConfigDict, Field, model_validator

from corum.config import CourseConfig, load_workspace, resolve_features
from corum.validation import (
    require_https_origin,
    require_identifier,
    require_iso_date,
    require_issue_key,
    require_nonblank,
    require_project_key,
    require_schema_one,
    require_transition_id,
)

from . import cache
from .client import JiraClient


Identifier = Annotated[str, AfterValidator(require_identifier)]
IssueKey = Annotated[str, AfterValidator(require_issue_key)]
NonBlank = Annotated[str, AfterValidator(require_nonblank)]
PlanDate = Annotated[date, BeforeValidator(require_iso_date)]
SchemaOne = Annotated[Literal[1], BeforeValidator(require_schema_one)]


class _StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class CreateIssue(_StrictModel):
    type: Literal["Task", "Session", "Milestone"]
    parent: IssueKey
    summary: NonBlank
    description: str | None = None
    due: PlanDate | None = None
    labels: list[str] | None = None


class UpdateFields(_StrictModel):
    type: Literal["Task", "Session", "Milestone"] | None = None
    parent: IssueKey | None = None
    summary: NonBlank | None = None
    description: str | None = None
    due: PlanDate | None = None
    labels: list[str] | None = None

    @model_validator(mode="before")
    @classmethod
    def reject_non_clearable_nulls(cls, value):
        if isinstance(value, dict):
            null_fields = sorted(
                field for field in ("type", "parent", "summary") if field in value and value[field] is None
            )
            if null_fields:
                raise ValueError(f"update cannot clear: {', '.join(null_fields)}")
        return value

    @model_validator(mode="after")
    def require_one_field(self):
        if not self.model_fields_set:
            raise ValueError("update set must contain at least one field")
        return self


class CreateAction(_StrictModel):
    action: Literal["create"]
    issue: CreateIssue


class UpdateAction(_StrictModel):
    action: Literal["update"]
    key: IssueKey
    set: UpdateFields


class TransitionAction(_StrictModel):
    action: Literal["transition"]
    key: IssueKey
    transition: Identifier


PlanAction = Annotated[
    CreateAction | UpdateAction | TransitionAction,
    Field(discriminator="action"),
]


class JiraPlan(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema: SchemaOne
    course: Identifier
    epic: IssueKey
    actions: list[PlanAction]


class AppliedAction(_StrictModel):
    action: Literal["create", "update", "transition"]
    key: IssueKey


class ApplyResult(_StrictModel):
    course: str
    epic: str
    dry_run: bool
    applied: list[AppliedAction]


class JiraDisabled(ValueError):
    """The selected course has Jira effectively disabled."""


class InvalidPlan(ValueError):
    """A syntactically valid plan does not target the selected configuration."""


def _validate_plan(vault: Path, course: CourseConfig, plan: JiraPlan):
    workspace = load_workspace(vault)
    if not resolve_features(workspace, course).jira:
        raise JiraDisabled(f"Jira is disabled for {course.code}")
    if workspace.jira is None or course.jira is None:
        raise InvalidPlan(f"enabled Jira configuration is incomplete for {course.code}")
    try:
        require_https_origin(str(workspace.jira.site))
        require_project_key(workspace.jira.project)
        require_issue_key(course.jira.epic, "configured Jira epic key")
    except ValueError as error:
        raise InvalidPlan(f"invalid configured Jira value: {error}") from error
    if plan.course != course.code:
        raise InvalidPlan(
            f"plan course {plan.course!r} does not match selected course {course.code!r}"
        )
    if plan.epic != course.jira.epic:
        raise InvalidPlan(
            f"plan epic {plan.epic!r} does not match configured epic {course.jira.epic!r}"
        )
    for index, action in enumerate(plan.actions):
        parent = None
        if isinstance(action, CreateAction):
            parent = action.issue.parent
        elif isinstance(action, UpdateAction) and "parent" in action.set.model_fields_set:
            parent = action.set.parent
        if parent is not None and parent != plan.epic:
            raise InvalidPlan(
                f"actions[{index}] parent {parent!r} does not match plan epic {plan.epic!r}"
            )
        if isinstance(action, TransitionAction):
            if action.transition not in workspace.jira.transitions:
                raise InvalidPlan(
                    f"actions[{index}] names unknown Jira transition {action.transition!r}"
                )
            try:
                require_transition_id(workspace.jira.transitions[action.transition])
            except ValueError as error:
                raise InvalidPlan(
                    f"actions[{index}] has invalid configured transition: {error}"
                ) from error
    return workspace.jira


def _adf_to_markdown(value: Any) -> str | None:
    if value is None or isinstance(value, str):
        return value
    if not isinstance(value, dict):
        raise cache.CacheError("Jira description must be a string, object, or null")

    def render(node: Any) -> str:
        if not isinstance(node, dict):
            return ""
        if node.get("type") == "text":
            text = str(node.get("text", ""))
            for mark in node.get("marks", []):
                if mark.get("type") == "link" and mark.get("attrs", {}).get("href"):
                    text = f"[{text}]({mark['attrs']['href']})"
                elif mark.get("type") == "strong":
                    text = f"**{text}**"
            return text
        content = "".join(render(child) for child in node.get("content", []))
        return content

    blocks = value.get("content", [])
    return "\n".join(render(block) for block in blocks)


def _normalized_jira_issue(raw_value: Any) -> dict[str, Any]:
    if not isinstance(raw_value, dict) or not isinstance(raw_value.get("fields"), dict):
        raise cache.CacheError("Jira issue response must contain fields")
    fields = raw_value["fields"]
    issue_type = fields.get("issuetype") or {}
    status = fields.get("status") or {}
    return cache.normalize_issue(
        {
            "key": raw_value.get("key"),
            "type": issue_type.get("name") if isinstance(issue_type, dict) else None,
            "summary": fields.get("summary"),
            "status": status.get("name") if isinstance(status, dict) else None,
            "due": fields.get("duedate"),
            "labels": fields.get("labels"),
            "description": _adf_to_markdown(fields.get("description")),
            "updated_at": fields.get("updated"),
        },
        "Jira issue response",
    )


def _fields(value: BaseModel) -> dict[str, Any]:
    return value.model_dump(mode="json", exclude_unset=True)


async def _upsert_with_recovery(
    vault: Path,
    course: CourseConfig,
    epic: str,
    issue: dict[str, Any],
    client: JiraClient,
) -> None:
    try:
        cache.upsert(vault, course, {"epic": epic, "issue": issue})
        return
    except cache.CacheError:
        await _reconcile_cache(vault, course, epic, client)
    cache.upsert(vault, course, {"epic": epic, "issue": issue})


async def _reconcile_cache(
    vault: Path,
    course: CourseConfig,
    epic: str,
    client: JiraClient,
) -> None:
    children = [_normalized_jira_issue(raw) for raw in await client.epic_children(epic)]
    cache.reconcile(
        vault,
        course,
        {
            "epic": epic,
            "reconciled_at": datetime.now(UTC).isoformat(),
            "complete": True,
            "issues": children,
        },
    )


async def apply_plan(
    vault: Path,
    course: CourseConfig,
    plan: JiraPlan,
    client: JiraClient,
    *,
    dry_run: bool = False,
) -> ApplyResult:
    """Apply a fully validated plan in order and cache each resulting issue."""
    jira = _validate_plan(vault, course, plan)
    if dry_run:
        return ApplyResult(course=course.code, epic=plan.epic, dry_run=True, applied=[])

    if not plan.actions and not cache.exists(vault, course):
        await _reconcile_cache(vault, course, plan.epic, client)

    applied: list[AppliedAction] = []
    for action in plan.actions:
        if isinstance(action, CreateAction):
            key = await client.create_issue({"project": jira.project, **_fields(action.issue)})
        elif isinstance(action, UpdateAction):
            key = action.key
            await client.update_fields(key, _fields(action.set))
        else:
            key = action.key
            await client.transition_issue(key, jira.transitions[action.transition])
        issue = _normalized_jira_issue(await client.fetch_issue(key))
        await _upsert_with_recovery(vault, course, plan.epic, issue, client)
        applied.append(AppliedAction(action=action.action, key=key))
    return ApplyResult(course=course.code, epic=plan.epic, dry_run=False, applied=applied)
