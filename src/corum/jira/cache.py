"""Validate and atomically maintain one course's Jira cache."""

from __future__ import annotations

import json
import os
import re
import tempfile
from datetime import datetime
from pathlib import Path
from typing import Any

from corum.config import CourseConfig
from corum.validation import require_iso_date, require_issue_key


CACHE_FIELDS = {"schema", "reconciled_at", "issues"}
ISSUE_FIELDS = {
    "key",
    "type",
    "summary",
    "status",
    "due",
    "labels",
    "description",
    "updated_at",
}
OPTIONAL_ISSUE_FIELDS = ("due", "labels", "description", "updated_at")


class CacheError(ValueError):
    """Input or cache data does not satisfy the Jira cache contract."""


def _object(value: Any, name: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise CacheError(f"{name} must be a JSON object")
    return value


def _only_fields(value: dict[str, Any], allowed: set[str], name: str) -> None:
    unknown = sorted(set(value) - allowed)
    if unknown:
        raise CacheError(f"{name} has unknown field(s): {', '.join(unknown)}")


def _required_string(value: Any, name: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise CacheError(f"{name} must be a non-empty string")
    return value


def _optional_string(value: Any, name: str) -> str | None:
    if value is not None and not isinstance(value, str):
        raise CacheError(f"{name} must be a string or null")
    return value


def _optional_date(value: Any, name: str) -> str | None:
    if value is None:
        return None
    try:
        return require_iso_date(value, name)
    except ValueError as error:
        raise CacheError(str(error)) from error


def _issue_key(value: Any, name: str) -> str:
    try:
        return require_issue_key(value, name)
    except ValueError as error:
        raise CacheError(str(error)) from error


def _timestamp(value: Any, name: str) -> str:
    value = _required_string(value, name)
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise CacheError(f"{name} must be an ISO 8601 timestamp") from error
    if parsed.tzinfo is None or parsed.utcoffset() is None:
        raise CacheError(f"{name} must include a UTC offset")
    return parsed.isoformat()


def _optional_timestamp(value: Any, name: str) -> str | None:
    return None if value is None else _timestamp(value, name)


def _labels(value: Any, name: str) -> list[str] | None:
    if value is None:
        return None
    if not isinstance(value, list) or any(not isinstance(label, str) for label in value):
        raise CacheError(f"{name} must be an array of strings or null")
    return sorted(set(value))


def normalize_issue(value: Any, name: str = "issue") -> dict[str, Any]:
    raw = _object(value, name)
    _only_fields(raw, ISSUE_FIELDS, name)
    return {
        "key": _issue_key(raw.get("key"), f"{name}.key"),
        "type": _required_string(raw.get("type"), f"{name}.type"),
        "summary": _required_string(raw.get("summary"), f"{name}.summary"),
        "status": _required_string(raw.get("status"), f"{name}.status"),
        "due": _optional_date(raw.get("due"), f"{name}.due"),
        "labels": _labels(raw.get("labels"), f"{name}.labels"),
        "description": _optional_string(raw.get("description"), f"{name}.description"),
        "updated_at": _optional_timestamp(raw.get("updated_at"), f"{name}.updated_at"),
    }


def _natural_key(issue: dict[str, Any]) -> tuple[tuple[int, int | str], ...]:
    parts: list[tuple[int, int | str]] = []
    for part in re.split(r"(\d+)", issue["key"]):
        parts.append((1, int(part)) if part.isdigit() else (0, part.casefold()))
    return tuple(parts)


def _issues(value: Any, name: str = "issues") -> list[dict[str, Any]]:
    if not isinstance(value, list):
        raise CacheError(f"{name} must be an array")
    normalized = [normalize_issue(issue, f"{name}[{index}]") for index, issue in enumerate(value)]
    keys = [issue["key"] for issue in normalized]
    duplicates = sorted({key for key in keys if keys.count(key) > 1})
    if duplicates:
        raise CacheError(f"{name} has duplicate key(s): {', '.join(duplicates)}")
    return sorted(normalized, key=_natural_key)


def _configured_epic(course: CourseConfig) -> str:
    if course.jira is None:
        raise CacheError(f"course {course.code} has no Jira configuration")
    return _required_string(course.jira.epic, "course.yaml.jira.epic")


def _same_epic(raw: dict[str, Any], configured_epic: str, name: str) -> None:
    epic = _required_string(raw.get("epic"), f"{name}.epic")
    if epic != configured_epic:
        raise CacheError(
            f"{name}.epic {epic!r} does not match course.yaml epic {configured_epic!r}"
        )


def normalize_reconcile(raw_value: Any, configured_epic: str) -> dict[str, Any]:
    raw = _object(raw_value, "reconcile input")
    _only_fields(raw, {"epic", "reconciled_at", "complete", "issues"}, "reconcile input")
    if raw.get("complete") is not True:
        raise CacheError("reconcile input.complete must be true")
    _same_epic(raw, configured_epic, "reconcile input")
    return {
        "schema": 1,
        "reconciled_at": _timestamp(raw.get("reconciled_at"), "reconcile input.reconciled_at"),
        "issues": _issues(raw.get("issues"), "reconcile input.issues"),
    }


def normalize_cache(raw_value: Any) -> dict[str, Any]:
    raw = _object(raw_value, "jira.json")
    _only_fields(raw, CACHE_FIELDS, "jira.json")
    if raw.get("schema") != 1 or isinstance(raw.get("schema"), bool):
        raise CacheError("jira.json.schema must be 1")
    return {
        "schema": 1,
        "reconciled_at": _optional_timestamp(raw.get("reconciled_at"), "jira.json.reconciled_at"),
        "issues": _issues(raw.get("issues"), "jira.json.issues"),
    }


def normalize_upsert(
    raw_value: Any,
    current: dict[str, Any],
    configured_epic: str,
) -> dict[str, Any]:
    raw = _object(raw_value, "upsert input")
    _only_fields(raw, {"epic", "issue"}, "upsert input")
    _same_epic(raw, configured_epic, "upsert input")
    issue = normalize_issue(raw.get("issue"), "upsert input.issue")
    issues = [cached for cached in current["issues"] if cached["key"] != issue["key"]]
    issues.append(issue)
    return {**current, "issues": sorted(issues, key=_natural_key)}


def _target(vault: Path, course: CourseConfig) -> Path:
    if not course.code or Path(course.code).name != course.code or course.code in {".", ".."}:
        raise CacheError(f"invalid course code: {course.code!r}")
    vault_root = vault.resolve()
    courses_root = (vault_root / "courses").resolve()
    if not courses_root.is_relative_to(vault_root):
        raise CacheError(f"courses directory resolves outside vault: {vault_root / 'courses'}")
    course_dir = (courses_root / course.code).resolve()
    if not course_dir.is_dir() or not course_dir.is_relative_to(courses_root):
        raise CacheError(f"invalid configured course directory: {courses_root / course.code}")
    state_dir = (course_dir / "state").resolve()
    if not state_dir.is_relative_to(course_dir):
        raise CacheError(f"state directory resolves outside course: {course_dir / 'state'}")
    return state_dir / "jira.json"


def _read_cache(target: Path) -> dict[str, Any]:
    try:
        return normalize_cache(json.loads(target.read_text(encoding="utf-8")))
    except FileNotFoundError as error:
        raise CacheError(f"{target} is missing; reconcile it first") from error
    except (OSError, json.JSONDecodeError) as error:
        raise CacheError(f"could not read {target}: {error}") from error


def _write_atomic(target: Path, value: dict[str, Any]) -> None:
    target.parent.mkdir(parents=True, exist_ok=True)
    scratch: Path | None = None
    try:
        with tempfile.NamedTemporaryFile(
            "w",
            encoding="utf-8",
            dir=target.parent,
            prefix=".jira-",
            suffix=".json",
            delete=False,
        ) as temporary:
            scratch = Path(temporary.name)
            json.dump(value, temporary, indent=2, ensure_ascii=False)
            temporary.write("\n")
            temporary.flush()
            os.fsync(temporary.fileno())
        os.replace(scratch, target)
    except BaseException:
        if scratch is not None:
            scratch.unlink(missing_ok=True)
        raise


def reconcile(vault: Path, course: CourseConfig, raw_value: Any) -> dict[str, Any]:
    epic = _configured_epic(course)
    output = normalize_reconcile(raw_value, epic)
    _write_atomic(_target(vault, course), output)
    return {"course": course.code, "epic": epic, "issues": len(output["issues"])}


def upsert(vault: Path, course: CourseConfig, raw_value: Any) -> dict[str, Any]:
    epic = _configured_epic(course)
    target = _target(vault, course)
    current = _read_cache(target)
    output = normalize_upsert(raw_value, current, epic)
    _write_atomic(target, output)
    return {"course": course.code, "epic": epic, "issues": len(output["issues"])}
