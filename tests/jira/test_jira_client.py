from __future__ import annotations

from collections import defaultdict

import pytest

from corum.jira.client import JiraClient, JiraMutationError
from corum.jira.rovo import RovoError


def issue(key: str) -> dict:
    return {
        "key": key,
        "fields": {
            "issuetype": {"name": "Task"},
            "summary": f"Summary {key}",
            "status": {"name": "To Do"},
            "duedate": None,
            "labels": [],
            "description": None,
            "updated": "2026-09-06T00:00:00+08:00",
        },
    }


class FakeRovo:
    def __init__(self, responses: dict[str, list[object]]) -> None:
        self.responses = {name: list(values) for name, values in responses.items()}
        self.calls: list[tuple[str, dict[str, object]]] = []

    async def call_json(self, name: str, arguments: dict[str, object]) -> object:
        self.calls.append((name, arguments))
        value = self.responses[name].pop(0)
        if isinstance(value, BaseException):
            raise value
        return value


@pytest.mark.asyncio
async def test_client_maps_all_plan_operations_to_current_rovo_tools():
    rovo = FakeRovo(
        {
            "createJiraIssue": [{"data": {"key": "STUDY-2"}}],
            "editJiraIssue": [{"data": {}}],
            "transitionJiraIssue": [{"data": {}}],
            "getJiraIssue": [{"data": issue("STUDY-2")}],
            "searchJiraIssuesUsingJql": [
                {
                    "data": {
                        "issues": [issue("STUDY-2")],
                        "nextPageToken": "page-2",
                        "isLast": False,
                    }
                },
                {"data": {"issues": [issue("STUDY-3")], "isLast": True}},
            ],
        }
    )
    client = JiraClient(rovo, "cloud-1")

    assert (
        await client.create_issue(
            {
                "project": "STUDY",
                "type": "Task",
                "parent": "STUDY-1",
                "summary": "Task",
                "description": "**Deadline:** Friday",
                "due": "2026-09-11",
                "labels": ["assessment"],
            }
        )
        == "STUDY-2"
    )
    await client.update_fields(
        "STUDY-2",
        {
            "type": "Milestone",
            "parent": "STUDY-1",
            "summary": "Updated",
            "description": None,
            "due": None,
            "labels": [],
        },
    )
    await client.transition_issue("STUDY-2", "2")
    assert (await client.fetch_issue("STUDY-2"))["key"] == "STUDY-2"
    assert [value["key"] for value in await client.epic_children("STUDY-1")] == [
        "STUDY-2",
        "STUDY-3",
    ]

    assert rovo.calls == [
        (
            "createJiraIssue",
            {
                "cloudId": "cloud-1",
                "projectKey": "STUDY",
                "summary": "Task",
                "issueType": "Task",
                "description": "**Deadline:** Friday",
                "labels": ["assessment"],
                "parent": "STUDY-1",
                "additional_fields": {"duedate": "2026-09-11"},
            },
        ),
        (
            "editJiraIssue",
            {
                "cloudId": "cloud-1",
                "issueIdOrKey": "STUDY-2",
                "fields": {
                    "issuetype": {"name": "Milestone"},
                    "parent": {"key": "STUDY-1"},
                    "summary": "Updated",
                    "description": None,
                    "duedate": None,
                    "labels": [],
                },
                "contentFormat": "markdown",
            },
        ),
        (
            "transitionJiraIssue",
            {"cloudId": "cloud-1", "issueIdOrKey": "STUDY-2", "transitionId": "2"},
        ),
        (
            "getJiraIssue",
            {
                "cloudId": "cloud-1",
                "issueIdOrKey": "STUDY-2",
                "view": "full",
                "responseContentFormat": "markdown",
            },
        ),
        (
            "searchJiraIssuesUsingJql",
            {
                "cloudId": "cloud-1",
                "jql": 'parent = "STUDY-1"',
                "maxResults": 100,
                "view": "full",
                "responseContentFormat": "markdown",
            },
        ),
        (
            "searchJiraIssuesUsingJql",
            {
                "cloudId": "cloud-1",
                "jql": 'parent = "STUDY-1"',
                "maxResults": 100,
                "view": "full",
                "responseContentFormat": "markdown",
                "nextPageToken": "page-2",
            },
        ),
    ]


@pytest.mark.asyncio
async def test_create_without_returned_key_is_treated_as_applied():
    client = JiraClient(
        FakeRovo({"createJiraIssue": [{"data": {"id": "10001"}}]}), "cloud-1"
    )

    with pytest.raises(JiraMutationError, match="missing a valid issue key") as caught:
        await client.create_issue(
            {"project": "STUDY", "type": "Task", "parent": "STUDY-1", "summary": "Task"}
        )

    assert caught.value.write_state == "applied"


@pytest.mark.asyncio
@pytest.mark.parametrize("operation", ["create", "update", "transition"])
async def test_mutation_tool_failure_has_unknown_write_state(operation):
    tool = {
        "create": "createJiraIssue",
        "update": "editJiraIssue",
        "transition": "transitionJiraIssue",
    }[operation]
    client = JiraClient(
        FakeRovo({tool: [RovoError("raw payload sentinel")]}), "cloud-1"
    )

    with pytest.raises(JiraMutationError, match="outcome is unknown") as caught:
        if operation == "create":
            await client.create_issue(
                {
                    "project": "STUDY",
                    "type": "Task",
                    "parent": "STUDY-1",
                    "summary": "Task",
                }
            )
        elif operation == "update":
            await client.update_fields("STUDY-2", {"summary": "Changed"})
        else:
            await client.transition_issue("STUDY-2", "2")

    assert caught.value.write_state == "unknown"
    assert "raw payload sentinel" not in str(caught.value)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "operation", ["create", "update", "transition", "fetch", "children"]
)
async def test_invalid_identifiers_are_rejected_before_rovo_call(operation):
    rovo = FakeRovo(defaultdict(list))
    client = JiraClient(rovo, "cloud-1")

    with pytest.raises(ValueError):
        if operation == "create":
            await client.create_issue(
                {
                    "project": "STUDY",
                    "type": "Task",
                    "parent": "../BAD",
                    "summary": "Task",
                }
            )
        elif operation == "update":
            await client.update_fields("../BAD", {"summary": "Changed"})
        elif operation == "transition":
            await client.transition_issue("STUDY-2", "")
        elif operation == "fetch":
            await client.fetch_issue("../BAD")
        else:
            await client.epic_children('../BAD" OR project IS NOT EMPTY')

    assert rovo.calls == []
