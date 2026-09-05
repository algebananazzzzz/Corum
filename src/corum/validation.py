"""Shared validation for security-sensitive Jira boundary values."""

from __future__ import annotations

import re
from datetime import date
from urllib.parse import urlsplit, urlunsplit


PROJECT_KEY_PATTERN = r"[A-Z][A-Z0-9_]*"
ISSUE_KEY_PATTERN = rf"{PROJECT_KEY_PATTERN}-[1-9][0-9]*"
TRANSITION_ID_PATTERN = r"[0-9]+"


def _matching_string(value: object, pattern: str, name: str) -> str:
    if not isinstance(value, str) or re.fullmatch(pattern, value) is None:
        raise ValueError(f"{name} has invalid syntax: {value!r}")
    return value


def require_project_key(value: object, name: str = "Jira project key") -> str:
    return _matching_string(value, PROJECT_KEY_PATTERN, name)


def require_issue_key(value: object, name: str = "Jira issue key") -> str:
    return _matching_string(value, ISSUE_KEY_PATTERN, name)


def require_transition_id(value: object, name: str = "Jira transition ID") -> str:
    return _matching_string(value, TRANSITION_ID_PATTERN, name)


def require_identifier(value: object, name: str = "identifier") -> str:
    return _matching_string(value, r"\S+", name)


def require_nonblank(value: object, name: str = "value") -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{name} must be a non-blank string")
    return value


def require_iso_date(value: object, name: str = "date") -> str:
    if not isinstance(value, str) or re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}", value) is None:
        raise ValueError(f"{name} must be an ISO 8601 date")
    try:
        return date.fromisoformat(value).isoformat()
    except ValueError as error:
        raise ValueError(f"{name} must be an ISO 8601 date") from error


def require_schema_one(value: object) -> int:
    if isinstance(value, bool) or not isinstance(value, (int, float)) or value != 1:
        raise ValueError("schema must be integer 1")
    return 1


def require_https_origin(value: object) -> str:
    if not isinstance(value, str):
        raise ValueError("Jira site must be an HTTPS origin")
    try:
        parsed = urlsplit(value)
        parsed_port = parsed.port
    except ValueError as error:
        raise ValueError("Jira site must be an HTTPS origin") from error
    if (
        parsed.scheme.lower() != "https"
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.path not in {"", "/"}
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("Jira site must be an HTTPS origin")
    host = parsed.hostname
    if ":" in host:
        host = f"[{host}]"
    netloc = f"{host}:{parsed_port}" if parsed_port is not None else host
    return urlunsplit(("https", netloc, "", "", ""))
