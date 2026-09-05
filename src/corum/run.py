"""Typed, machine-readable Corum run manifests."""

from __future__ import annotations

from datetime import datetime
from collections.abc import Iterable, Mapping
from typing import Literal

from pydantic import BaseModel, Field


StageStatus = Literal["disabled", "pending", "up_to_date", "changed", "applied", "partial", "failed"]


class Change(BaseModel):
    kind: str
    summary: str


class Failure(BaseModel):
    source: str
    error: str


class StageResult(BaseModel):
    status: StageStatus


class CanvasResult(StageResult):
    changes: list[Change] = Field(default_factory=list)
    failures: list[Failure] = Field(default_factory=list)


class RunManifest(BaseModel):
    schema: Literal[1] = 1
    run_id: str
    course: str
    effective_features: dict[str, bool]
    canvas: CanvasResult
    jira: StageResult
    wiki: StageResult

    @classmethod
    def create(
        cls,
        course: str,
        effective_features: dict[str, bool],
        *,
        changes: list[str] | None = None,
        failures: Mapping[str, str] | Iterable[tuple[str, str]] | None = None,
        now: datetime | None = None,
    ) -> "RunManifest":
        change_records = [
            Change(kind=summary.partition(" ")[0], summary=summary) for summary in changes or []
        ]
        failure_items = failures.items() if isinstance(failures, Mapping) else failures or []
        failure_records = [Failure(source=source, error=error) for source, error in failure_items]
        if failure_records and change_records:
            canvas_status = "partial"
        elif failure_records:
            canvas_status = "failed"
        elif change_records:
            canvas_status = "changed"
        else:
            canvas_status = "up_to_date"
        instant = now or datetime.now().astimezone()
        return cls(
            run_id=instant.strftime("%Y%m%dT%H%M%S%z"),
            course=course,
            effective_features=dict(effective_features),
            canvas=CanvasResult(
                status=canvas_status,
                changes=change_records,
                failures=failure_records,
            ),
            jira=StageResult(status="pending" if effective_features["jira"] else "disabled"),
            wiki=StageResult(status="pending" if effective_features["wiki"] else "disabled"),
        )
