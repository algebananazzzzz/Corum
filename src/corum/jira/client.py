"""Small first-party client for the Jira Cloud REST API."""

from __future__ import annotations

import json
import re
from typing import Any
from urllib.parse import quote

import httpx

from corum.validation import (
    require_https_origin,
    require_issue_key,
    require_project_key,
    require_transition_id,
)


_INLINE_MARKUP = re.compile(r"\[([^\]]+)]\((https?://[^)]+)\)|\*\*([^*]+)\*\*")


def _inline_adf(value: str) -> list[dict[str, Any]]:
    nodes: list[dict[str, Any]] = []
    offset = 0
    for match in _INLINE_MARKUP.finditer(value):
        if match.start() > offset:
            nodes.append({"type": "text", "text": value[offset : match.start()]})
        if match.group(3) is not None:
            nodes.append(
                {"type": "text", "text": match.group(3), "marks": [{"type": "strong"}]}
            )
        else:
            nodes.append(
                {
                    "type": "text",
                    "text": match.group(1),
                    "marks": [{"type": "link", "attrs": {"href": match.group(2)}}],
                }
            )
        offset = match.end()
    if offset < len(value):
        nodes.append({"type": "text", "text": value[offset:]})
    return nodes or [{"type": "text", "text": ""}]


def _description_to_adf(value: str) -> dict[str, Any]:
    return {
        "type": "doc",
        "version": 1,
        "content": [
            {"type": "paragraph", "content": _inline_adf(line)}
            for line in value.splitlines() or [""]
        ],
    }


def _jira_fields(fields: dict[str, Any]) -> dict[str, Any]:
    mapped: dict[str, Any] = {}
    for name, value in fields.items():
        if name == "project":
            mapped[name] = {"key": require_project_key(value)}
        elif name == "type":
            mapped["issuetype"] = {"name": value}
        elif name == "parent":
            mapped[name] = None if value is None else {"key": require_issue_key(value)}
        elif name == "due":
            mapped["duedate"] = value
        elif name == "description":
            mapped[name] = None if value is None else _description_to_adf(value)
        elif name in {"summary", "labels"}:
            mapped[name] = value
        else:
            raise ValueError(f"unsupported Jira field: {name}")
    return mapped


class JiraClient:
    """Apply the narrow set of Jira operations required by Corum plans."""

    def __init__(
        self,
        site: str,
        email: str,
        token: str,
        *,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self._base_url = f"{require_https_origin(site)}/rest/api/3/"
        self._auth = httpx.BasicAuth(email, token)
        self._transport = transport

    @staticmethod
    def _issue_endpoint(key: str) -> str:
        segment = quote(require_issue_key(key), safe="")
        return f"issue/{segment}"

    def _new_client(self) -> httpx.AsyncClient:
        return httpx.AsyncClient(
            base_url=self._base_url,
            auth=self._auth,
            headers={"Accept": "application/json", "Content-Type": "application/json"},
            timeout=httpx.Timeout(30.0, connect=10.0),
            transport=self._transport,
        )

    async def create_issue(self, fields: dict[str, Any]) -> str:
        async with self._new_client() as client:
            response = await client.post("issue", json={"fields": _jira_fields(fields)})
            response.raise_for_status()
            return response.json()["key"]

    async def update_fields(self, key: str, fields: dict[str, Any]) -> None:
        async with self._new_client() as client:
            response = await client.put(
                self._issue_endpoint(key),
                json={"fields": _jira_fields(fields)},
            )
            response.raise_for_status()

    async def transition_issue(self, key: str, transition: str) -> None:
        transition_id = require_transition_id(transition)
        async with self._new_client() as client:
            response = await client.post(
                f"{self._issue_endpoint(key)}/transitions",
                json={"transition": {"id": transition_id}},
            )
            response.raise_for_status()

    async def fetch_issue(self, key: str) -> dict[str, Any]:
        async with self._new_client() as client:
            response = await client.get(
                self._issue_endpoint(key),
                params={"fields": "issuetype,summary,status,duedate,labels,description,updated"},
            )
            response.raise_for_status()
            return response.json()

    async def epic_children(self, epic: str) -> list[dict[str, Any]]:
        epic_key = require_issue_key(epic, "Jira epic issue key")
        body: dict[str, Any] = {
            "jql": f"parent = {json.dumps(epic_key)}",
            "maxResults": 100,
            "fields": [
                "issuetype",
                "summary",
                "status",
                "duedate",
                "labels",
                "description",
                "updated",
            ],
        }
        issues: list[dict[str, Any]] = []
        async with self._new_client() as client:
            while True:
                response = await client.post("search/jql", json=body)
                response.raise_for_status()
                page = response.json()
                issues.extend(page.get("issues", []))
                token = page.get("nextPageToken")
                if not token:
                    return issues
                body["nextPageToken"] = token
