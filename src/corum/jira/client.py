"""Rovo MCP adapter for the Jira operations used by Corum plans."""

from __future__ import annotations

import json
from typing import Any

from corum.validation import (
    require_issue_key,
    require_nonblank,
    require_project_key,
    require_transition_id,
)

from .rovo import RovoError, RovoSession


class JiraMutationError(RuntimeError):
    """A Jira mutation response cannot prove a safe automatic retry."""

    def __init__(self, message: str, *, write_state: str = "unknown") -> None:
        super().__init__(message)
        self.write_state = write_state


def _data(value: object, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise RovoError(f"Atlassian returned invalid {label}")
    nested = value.get("data")
    if nested is not None:
        if not isinstance(nested, dict):
            raise RovoError(f"Atlassian returned invalid {label}")
        return nested
    return value


def _edit_fields(fields: dict[str, Any]) -> dict[str, Any]:
    mapped: dict[str, Any] = {}
    for name, value in fields.items():
        if name == "type":
            mapped["issuetype"] = {"name": value}
        elif name == "parent":
            mapped[name] = None if value is None else {"key": require_issue_key(value)}
        elif name == "due":
            mapped["duedate"] = value
        elif name in {"summary", "description", "labels"}:
            mapped[name] = value
        else:
            raise ValueError(f"unsupported Jira field: {name}")
    return mapped


class JiraClient:
    """Apply the narrow Jira operation set through one authenticated Rovo session."""

    def __init__(self, session: RovoSession, cloud_id: str) -> None:
        self._session = session
        self._cloud_id = require_nonblank(cloud_id, "Jira cloud ID")

    async def create_issue(self, fields: dict[str, Any]) -> str:
        arguments: dict[str, object] = {
            "cloudId": self._cloud_id,
            "projectKey": require_project_key(fields["project"]),
            "summary": fields["summary"],
            "issueType": fields["type"],
        }
        for name in ("description", "labels", "parent"):
            value = fields.get(name)
            if value is not None:
                arguments[name] = value
        if fields.get("due") is not None:
            arguments["additional_fields"] = {"duedate": fields["due"]}
        try:
            result = _data(
                await self._session.call_json("createJiraIssue", arguments),
                "created Jira issue",
            )
        except RovoError as error:
            raise JiraMutationError(
                "Jira create outcome is unknown",
                write_state="unknown",
            ) from error
        issue = result.get("issue")
        key = result.get("key")
        if key is None and isinstance(issue, dict):
            key = issue.get("key")
        try:
            return require_issue_key(key, "created Jira issue key")
        except ValueError as error:
            raise JiraMutationError(
                "successful Jira create response is missing a valid issue key",
                write_state="applied",
            ) from error

    async def update_fields(self, key: str, fields: dict[str, Any]) -> None:
        arguments: dict[str, object] = {
            "cloudId": self._cloud_id,
            "issueIdOrKey": require_issue_key(key),
            "fields": _edit_fields(fields),
        }
        if "description" in fields:
            arguments["contentFormat"] = "markdown"
        try:
            await self._session.call_json("editJiraIssue", arguments)
        except RovoError as error:
            raise JiraMutationError(
                "Jira update outcome is unknown",
                write_state="unknown",
            ) from error

    async def transition_issue(self, key: str, transition: str) -> None:
        try:
            await self._session.call_json(
                "transitionJiraIssue",
                {
                    "cloudId": self._cloud_id,
                    "issueIdOrKey": require_issue_key(key),
                    "transitionId": require_transition_id(transition),
                },
            )
        except RovoError as error:
            raise JiraMutationError(
                "Jira transition outcome is unknown",
                write_state="unknown",
            ) from error

    async def fetch_issue(self, key: str) -> dict[str, Any]:
        return _data(
            await self._session.call_json(
                "getJiraIssue",
                {
                    "cloudId": self._cloud_id,
                    "issueIdOrKey": require_issue_key(key),
                    "view": "full",
                    "responseContentFormat": "markdown",
                },
            ),
            "Jira issue",
        )

    async def epic_children(self, epic: str) -> list[dict[str, Any]]:
        epic_key = require_issue_key(epic, "Jira epic issue key")
        issues: list[dict[str, Any]] = []
        next_page: str | None = None
        while True:
            arguments: dict[str, object] = {
                "cloudId": self._cloud_id,
                "jql": f"parent = {json.dumps(epic_key)}",
                "maxResults": 100,
                "view": "full",
                "responseContentFormat": "markdown",
            }
            if next_page is not None:
                arguments["nextPageToken"] = next_page
            page = _data(
                await self._session.call_json("searchJiraIssuesUsingJql", arguments),
                "Jira search result",
            )
            values = page.get("issues")
            if not isinstance(values, list) or not all(isinstance(item, dict) for item in values):
                raise RovoError("Atlassian returned invalid Jira search results")
            issues.extend(values)
            token = page.get("nextPageToken")
            if page.get("isLast") is True or not isinstance(token, str) or not token:
                return issues
            next_page = token
