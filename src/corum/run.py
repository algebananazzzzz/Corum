"""Typed, machine-readable Corum run manifests."""

from __future__ import annotations

from collections.abc import Iterable, Mapping
from datetime import datetime
import hashlib
from typing import Any, Literal
from zoneinfo import ZoneInfo

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


StageStatus = Literal[
    "disabled",
    "pending",
    "up_to_date",
    "changed",
    "applied",
    "partial",
    "failed",
]
SourceStatus = Literal["up_to_date", "changed", "partial", "failed"]
WriteState = Literal["not_applied", "applied", "unknown"]


class _StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class Change(_StrictModel):
    """One successful, stable Canvas item change."""

    id: str
    source: str
    item_id: str | None = None
    kind: str
    status: Literal["changed"] = "changed"
    summary: str
    raw_path: str | None = None
    details: dict[str, Any] = Field(default_factory=dict)

    @model_validator(mode="before")
    @classmethod
    def normalize_legacy_shape(cls, value: object) -> object:
        if not isinstance(value, dict):
            return value
        normalized = dict(value)
        kind = normalized.get("kind")
        summary = normalized.get("summary")
        if "source" not in normalized and isinstance(kind, str):
            normalized["source"] = kind
        if "id" not in normalized and isinstance(kind, str) and isinstance(summary, str):
            digest = hashlib.sha256(summary.encode("utf-8")).hexdigest()[:16]
            normalized["id"] = f"legacy:{kind}:{digest}"
        return normalized


class Failure(_StrictModel):
    """One source- or item-level Canvas capture failure."""

    id: str
    source: str
    item_id: str | None = None
    kind: str
    status: Literal["failed"] = "failed"
    error: str
    raw_path: str | None = None
    details: dict[str, Any] = Field(default_factory=dict)

    @model_validator(mode="before")
    @classmethod
    def normalize_legacy_shape(cls, value: object) -> object:
        if not isinstance(value, dict):
            return value
        normalized = dict(value)
        source = normalized.get("source")
        item_id = normalized.get("item_id")
        if "kind" not in normalized and isinstance(source, str):
            normalized["kind"] = source
        if "id" not in normalized and isinstance(source, str):
            normalized["id"] = f"canvas:{source}" + (
                f":{item_id}" if item_id is not None else ""
            )
        return normalized


class CanvasSourceResult(_StrictModel):
    source: str
    status: SourceStatus
    changes: list[str] = Field(default_factory=list)
    failures: list[str] = Field(default_factory=list)


class AppliedItem(_StrictModel):
    id: str
    action: str
    target: str
    details: dict[str, Any] = Field(default_factory=dict)


class StageFailure(_StrictModel):
    id: str
    action: str | None = None
    target: str | None = None
    error: str
    write_state: WriteState = "not_applied"
    retry_safe: bool = True
    details: dict[str, Any] = Field(default_factory=dict)


class StageResult(_StrictModel):
    status: StageStatus
    applied: list[AppliedItem] = Field(default_factory=list)
    failures: list[StageFailure] = Field(default_factory=list)
    reconciliation_required: bool = False
    retry_safe: bool = True
    reconciled: bool = False


class CanvasResult(_StrictModel):
    status: StageStatus
    changes: list[Change] = Field(default_factory=list)
    failures: list[Failure] = Field(default_factory=list)
    sources: list[CanvasSourceResult] = Field(default_factory=list)


class RunManifest(_StrictModel):
    schema: Literal[1] = 1
    run_id: str
    course: str
    effective_features: dict[str, bool]
    canvas: CanvasResult
    jira: StageResult
    wiki: StageResult

    @field_validator("effective_features")
    @classmethod
    def exact_feature_keys(cls, value: dict[str, bool]) -> dict[str, bool]:
        if set(value) != {"jira", "wiki"}:
            raise ValueError("effective_features must contain exactly jira and wiki")
        return value

    @staticmethod
    def _legacy_change(summary: str) -> Change:
        kind = summary.partition(" ")[0]
        digest = hashlib.sha256(summary.encode("utf-8")).hexdigest()[:16]
        return Change(
            id=f"legacy:{kind}:{digest}",
            source=kind,
            kind=kind,
            summary=summary,
        )

    @staticmethod
    def _legacy_failure(source: str, error: str) -> Failure:
        return Failure(
            id=f"canvas:{source}",
            source=source,
            kind=source,
            error=error,
        )

    @classmethod
    def create(
        cls,
        course: str,
        effective_features: dict[str, bool],
        *,
        changes: Iterable[Change | dict[str, Any] | str] | None = None,
        failures: (
            Mapping[str, str]
            | Iterable[Failure | dict[str, Any] | tuple[str, str]]
            | None
        ) = None,
        sources: Iterable[str] | None = None,
        now: datetime | None = None,
        timezone: str | None = None,
    ) -> "RunManifest":
        change_records = [
            entry
            if isinstance(entry, Change)
            else Change.model_validate(entry)
            if isinstance(entry, dict)
            else cls._legacy_change(entry)
            for entry in changes or []
        ]
        failure_items = failures.items() if isinstance(failures, Mapping) else failures or []
        failure_records = [
            entry
            if isinstance(entry, Failure)
            else Failure.model_validate(entry)
            if isinstance(entry, dict)
            else cls._legacy_failure(*entry)
            for entry in failure_items
        ]
        if failure_records and change_records:
            canvas_status = "partial"
        elif failure_records:
            canvas_status = "failed"
        elif change_records:
            canvas_status = "changed"
        else:
            canvas_status = "up_to_date"

        ordered_sources = list(sources or [])
        for item in [*change_records, *failure_records]:
            if item.source not in ordered_sources:
                ordered_sources.append(item.source)
        source_records: list[CanvasSourceResult] = []
        for source in ordered_sources:
            source_changes = [item.id for item in change_records if item.source == source]
            source_failures = [item.id for item in failure_records if item.source == source]
            if source_changes and source_failures:
                source_status: SourceStatus = "partial"
            elif source_failures:
                source_status = "failed"
            elif source_changes:
                source_status = "changed"
            else:
                source_status = "up_to_date"
            source_records.append(
                CanvasSourceResult(
                    source=source,
                    status=source_status,
                    changes=source_changes,
                    failures=source_failures,
                )
            )

        zone = ZoneInfo(timezone) if timezone is not None else None
        instant = now or datetime.now(zone).astimezone()
        if zone is not None:
            instant = instant.astimezone(zone)
        return cls(
            run_id=instant.strftime("%Y%m%dT%H%M%S%z"),
            course=course,
            effective_features=dict(effective_features),
            canvas=CanvasResult(
                status=canvas_status,
                changes=change_records,
                failures=failure_records,
                sources=source_records,
            ),
            jira=StageResult(
                status="pending" if effective_features["jira"] else "disabled"
            ),
            wiki=StageResult(
                status="pending" if effective_features["wiki"] else "disabled"
            ),
        )
