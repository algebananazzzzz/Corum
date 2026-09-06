from __future__ import annotations

import json

import pytest
from mcp import types

from corum.jira.rovo import RovoSession


class FakeMcpSession:
    def __init__(self, responses: dict[str, object]) -> None:
        self.responses = responses
        self.calls: list[tuple[str, dict[str, object]]] = []

    async def list_tools(self, *, params=None):
        return types.ListToolsResult(
            tools=[types.Tool(name=name, inputSchema={}) for name in self.responses]
        )

    async def call_tool(self, name, arguments):
        self.calls.append((name, arguments))
        value = self.responses[name]
        text = value if isinstance(value, str) else json.dumps(value)
        return types.CallToolResult(content=[types.TextContent(type="text", text=text)])


@pytest.mark.asyncio
async def test_call_json_accepts_rovo_discovery_footer_after_one_json_value():
    fake = FakeMcpSession({"discover": '{"results": []}\n\nalso matched (3 more)'})

    result = await RovoSession(fake).call_json("discover", {"query": "list Jira projects"})

    assert result == {"results": []}


@pytest.mark.asyncio
async def test_resources_normalize_current_compact_rovo_shape():
    fake = FakeMcpSession(
        {
            "getAccessibleAtlassianResources": {
                "data": {
                    "resources": [
                        {
                            "cloudId": "cloud-2",
                            "products": [{"id": "confluence", "access": "read-write"}],
                        },
                        {
                            "cloudId": "cloud-1",
                            "products": [{"id": "jira", "access": "read-write"}],
                        },
                    ]
                }
            }
        }
    )

    resources = await RovoSession(fake).resources()

    assert [resource.id for resource in resources] == ["cloud-1"]
    assert resources[0].name is None
    assert resources[0].url is None


@pytest.mark.asyncio
async def test_projects_parse_nested_values_and_request_full_page():
    fake = FakeMcpSession(
        {
            "listJiraProjects": {
                "data": {
                    "values": [
                        {"id": "10002", "key": "ZED", "name": "Zed"},
                        {"id": "10001", "key": "ALPHA", "name": "Alpha"},
                    ],
                    "isLast": True,
                    "startAt": 0,
                    "maxResults": 100,
                }
            }
        }
    )
    session = RovoSession(fake)

    projects = await session.projects("cloud-1")

    assert [project.key for project in projects] == ["ALPHA", "ZED"]
    assert fake.calls == [("listJiraProjects", {"cloudId": "cloud-1", "maxResults": 100})]
