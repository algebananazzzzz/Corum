"""Validate and sequentially apply exact Jira plans."""

from __future__ import annotations

from datetime import UTC, date, datetime
from pathlib import Path
from typing import Annotated, Any, Literal

from pydantic import AfterValidator, BaseModel, BeforeValidator, ConfigDict, Field, model_validator

from corum.config import CourseConfig, load_workspace, resolve_features
from corum.run import AppliedItem, RunManifest, StageFailure, StageResult
from corum.state import (
    latest_stage_requires_reconciliation,
    read_latest_run,
    update_latest_run_stage,
    write_latest_run,
)
from corum.validation import (
    require_identifier,
    require_iso_date,
    require_issue_key,
    require_nonblank,
    require_project_key,
    require_schema_one,
    require_transition_id,
)

from . import cache
from .client import JiraClient, JiraMutationError


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
    id: str
    action_index: int = Field(ge=0)
    action: Literal["create", "update", "transition"]
    key: IssueKey


class ActionFailure(_StrictModel):
    id: str
    action_index: int = Field(ge=0)
    action: Literal["create", "update", "transition"]
    key: IssueKey | None = None
    phase: Literal["mutation", "fetch", "cache"]
    error: str
    write_state: Literal["not_applied", "applied", "unknown"]
    retry_safe: bool


class ApplyResult(_StrictModel):
    course: str
    epic: str
    dry_run: bool
    status: Literal["pending", "up_to_date", "applied", "partial", "failed"]
    applied: list[AppliedAction]
    failures: list[ActionFailure] = Field(default_factory=list)
    reconciliation_required: bool = False
    retry_safe: bool = True
    reconciled: bool = False


class JiraDisabled(ValueError):
    """The selected course has Jira effectively disabled."""


class InvalidPlan(ValueError):
    """A syntactically valid plan does not target the selected configuration."""


class ReconciliationRequired(ValueError):
    """A prior uncertain Jira write must be reconciled before more actions."""


def _validate_plan(
    vault: Path,
    course: CourseConfig,
    plan: JiraPlan,
    *,
    require_cloud_id: bool,
):
    workspace = load_workspace(vault)
    if not resolve_features(workspace, course).jira:
        raise JiraDisabled(f"Jira is disabled for {course.code}")
    if workspace.jira is None or course.jira is None:
        raise InvalidPlan(f"enabled Jira configuration is incomplete for {course.code}")
    if require_cloud_id and workspace.jira.cloud_id is None:
        raise InvalidPlan("Jira OAuth configuration is incomplete; run corum jira login in this vault")
    try:
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
) -> list[dict[str, Any]]:
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
    return children


async def _preflight_owned_targets(
    vault: Path,
    course: CourseConfig,
    plan: JiraPlan,
    client: JiraClient,
) -> None:
    raw_children = await client.epic_children(plan.epic)
    children = [_normalized_jira_issue(raw) for raw in raw_children]
    owned = {issue["key"] for issue in children}
    targets = {
        action.key
        for action in plan.actions
        if isinstance(action, (UpdateAction, TransitionAction))
    }
    outside = sorted(targets - owned)
    if outside:
        raise InvalidPlan(
            "Jira action target(s) do not belong to configured epic "
            f"{plan.epic}: {', '.join(outside)}"
        )
    cache.reconcile(
        vault,
        course,
        {
            "epic": plan.epic,
            "reconciled_at": datetime.now(UTC).isoformat(),
            "complete": True,
            "issues": children,
        },
    )


def _stage_result(result: ApplyResult) -> StageResult:
    return StageResult(
        status=result.status,
        applied=[
            AppliedItem(
                id=item.id,
                action=item.action,
                target=item.key,
                details={"action_index": item.action_index, "key": item.key},
            )
            for item in result.applied
        ],
        failures=[
            StageFailure(
                id=item.id,
                action=item.action,
                target=item.key,
                error=item.error,
                write_state=item.write_state,
                retry_safe=item.retry_safe,
                details={"action_index": item.action_index, "phase": item.phase},
            )
            for item in result.failures
        ],
        reconciliation_required=result.reconciliation_required,
        retry_safe=result.retry_safe,
        reconciled=result.reconciled,
    )


def _record_result(vault: Path, course: CourseConfig, result: ApplyResult) -> ApplyResult:
    course_dir = vault / "courses" / course.code
    stage = _stage_result(result)
    updated = update_latest_run_stage(
        course_dir,
        "jira",
        stage.model_dump(mode="json"),
    )
    if not updated and result.reconciliation_required:
        workspace = load_workspace(vault)
        manifest = RunManifest.create(
            course.code,
            resolve_features(workspace, course).model_dump(),
            timezone=workspace.workspace.timezone,
        )
        manifest.jira = stage
        write_latest_run(course_dir, manifest.model_dump(mode="json"))
    return result


def _failed_result(
    course: CourseConfig,
    plan: JiraPlan,
    applied: list[AppliedAction],
    failure: ActionFailure,
) -> ApplyResult:
    return ApplyResult(
        course=course.code,
        epic=plan.epic,
        dry_run=False,
        status="partial" if applied else "failed",
        applied=applied,
        failures=[failure],
        reconciliation_required=failure.write_state != "not_applied" or bool(applied),
        retry_safe=failure.retry_safe and not applied,
    )


def _reconciled_result(
    course: CourseConfig,
    plan: JiraPlan,
    previous: dict,
) -> ApplyResult:
    """Preserve a partial run's evidence after its remote cache is refreshed."""
    stage = StageResult.model_validate(previous["jira"])
    applied = [
        AppliedAction(
            id=item.id,
            action_index=item.details["action_index"],
            action=item.action,
            key=item.target,
        )
        for item in stage.applied
    ]
    failures = [
        ActionFailure(
            id=item.id,
            action_index=item.details["action_index"],
            action=item.action,
            key=item.target,
            phase=item.details["phase"],
            error=item.error,
            write_state=item.write_state,
            retry_safe=item.retry_safe,
        )
        for item in stage.failures
    ]
    return ApplyResult(
        course=course.code,
        epic=plan.epic,
        dry_run=False,
        status=stage.status,
        applied=applied,
        failures=failures,
        reconciliation_required=False,
        retry_safe=stage.retry_safe,
        reconciled=True,
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
    jira = _validate_plan(vault, course, plan, require_cloud_id=not dry_run)
    if dry_run:
        return ApplyResult(
            course=course.code,
            epic=plan.epic,
            dry_run=True,
            status="pending",
            applied=[],
        )

    course_dir = vault / "courses" / course.code
    retry_blocked = latest_stage_requires_reconciliation(course_dir, "jira")
    if plan.actions and retry_blocked:
        raise ReconciliationRequired(
            "Jira reconciliation is required before retry; apply an exact empty plan first"
        )
    previous_run = read_latest_run(course_dir)

    if not plan.actions:
        reconciled = False
        if retry_blocked:
            await _reconcile_cache(vault, course, plan.epic, client)
            if previous_run is None:
                raise ReconciliationRequired(
                    "Jira reconciliation state disappeared during apply"
                )
            return _record_result(
                vault,
                course,
                _reconciled_result(course, plan, previous_run),
            )
        if (
            previous_run is not None
            and previous_run["jira"].get("reconciled") is True
            and previous_run["jira"].get("retry_safe") is False
        ):
            return _record_result(
                vault,
                course,
                _reconciled_result(course, plan, previous_run),
            )
        if not cache.exists(vault, course):
            await _reconcile_cache(vault, course, plan.epic, client)
            reconciled = True
        return _record_result(
            vault,
            course,
            ApplyResult(
                course=course.code,
                epic=plan.epic,
                dry_run=False,
                status="up_to_date",
                applied=[],
                reconciled=reconciled,
            ),
        )

    await _preflight_owned_targets(vault, course, plan, client)

    applied: list[AppliedAction] = []
    for index, action in enumerate(plan.actions):
        key: str | None = None
        try:
            if isinstance(action, CreateAction):
                returned_key = await client.create_issue(
                    {"project": jira.project, **_fields(action.issue)}
                )
                try:
                    key = require_issue_key(returned_key, "created Jira issue key")
                except ValueError as error:
                    raise JiraMutationError(
                        "successful Jira create response is missing a valid issue key",
                        write_state="applied",
                    ) from error
            elif isinstance(action, UpdateAction):
                key = action.key
                await client.update_fields(key, _fields(action.set))
            else:
                key = action.key
                await client.transition_issue(key, jira.transitions[action.transition])
        except Exception as error:
            write_state = getattr(error, "write_state", "unknown")
            failure = ActionFailure(
                id=f"jira:{index}:{action.action}:{key or 'unknown'}:mutation",
                action_index=index,
                action=action.action,
                key=key,
                phase="mutation",
                error=str(error),
                write_state=write_state,
                retry_safe=write_state == "not_applied",
            )
            return _record_result(
                vault,
                course,
                _failed_result(course, plan, applied, failure),
            )

        applied.append(
            AppliedAction(
                id=f"jira:{index}:{action.action}:{key}",
                action_index=index,
                action=action.action,
                key=key,
            )
        )
        try:
            raw_issue = await client.fetch_issue(key)
            issue = _normalized_jira_issue(raw_issue)
        except Exception as error:
            failure = ActionFailure(
                id=f"jira:{index}:{action.action}:{key}:fetch",
                action_index=index,
                action=action.action,
                key=key,
                phase="fetch",
                error=str(error),
                write_state="applied",
                retry_safe=False,
            )
            return _record_result(
                vault,
                course,
                _failed_result(course, plan, applied, failure),
            )
        try:
            await _upsert_with_recovery(vault, course, plan.epic, issue, client)
        except Exception as error:
            failure = ActionFailure(
                id=f"jira:{index}:{action.action}:{key}:cache",
                action_index=index,
                action=action.action,
                key=key,
                phase="cache",
                error=str(error),
                write_state="applied",
                retry_safe=False,
            )
            return _record_result(
                vault,
                course,
                _failed_result(course, plan, applied, failure),
            )

    return _record_result(
        vault,
        course,
        ApplyResult(
            course=course.code,
            epic=plan.epic,
            dry_run=False,
            status="applied",
            applied=applied,
            reconciled=True,
        ),
    )
