from __future__ import annotations

import json
from contextlib import asynccontextmanager

import pytest
from mcp import types

from corum.jira import rovo
from corum.jira.rovo import RovoError, RovoSession


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

    result = await RovoSession(fake).call_json(
        "discover", {"query": "list Jira projects"}
    )

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
    assert fake.calls == [
        ("listJiraProjects", {"cloudId": "cloud-1", "maxResults": 100})
    ]


@pytest.mark.asyncio
async def test_user_info_unwraps_current_data_envelope():
    fake = FakeMcpSession({"atlassianUserInfo": {"data": {"accountId": "account-1"}}})

    assert await RovoSession(fake).user_info() == {"accountId": "account-1"}


@pytest.mark.asyncio
async def test_project_listing_follows_offset_pages():
    class PagedMcpSession(FakeMcpSession):
        def __init__(self):
            super().__init__({"listJiraProjects": {}})
            self.pages = [
                {
                    "data": {
                        "values": [{"id": "1", "key": "ALPHA", "name": "Alpha"}],
                        "isLast": False,
                        "startAt": 0,
                        "maxResults": 1,
                    }
                },
                {
                    "data": {
                        "values": [{"id": "2", "key": "ZED", "name": "Zed"}],
                        "isLast": True,
                        "startAt": 1,
                        "maxResults": 1,
                    }
                },
            ]

        async def call_tool(self, name, arguments):
            self.calls.append((name, arguments))
            return types.CallToolResult(
                content=[
                    types.TextContent(type="text", text=json.dumps(self.pages.pop(0)))
                ]
            )

    fake = PagedMcpSession()

    projects = await RovoSession(fake).projects("cloud-1")

    assert [project.key for project in projects] == ["ALPHA", "ZED"]
    assert fake.calls[1] == (
        "listJiraProjects",
        {"cloudId": "cloud-1", "maxResults": 100, "startAt": 1},
    )


@pytest.mark.asyncio
async def test_missing_tool_and_invalid_response_are_redacted():
    missing = RovoSession(FakeMcpSession({}))
    with pytest.raises(RovoError, match="unavailable"):
        await missing.call_json("getJiraIssue", {"token": "synthetic-secret"})

    invalid = RovoSession(FakeMcpSession({"getJiraIssue": "not-json synthetic-secret"}))
    with pytest.raises(RovoError) as caught:
        await invalid.call_json("getJiraIssue", {})
    assert "synthetic-secret" not in str(caught.value)


@pytest.mark.asyncio
async def test_open_session_preserves_an_error_raised_by_its_caller(monkeypatch):
    class HttpClient:
        def __init__(self, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

    class McpSession:
        def __init__(self, *args):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

        async def initialize(self):
            return None

    @asynccontextmanager
    async def transport(*args, **kwargs):
        yield object(), object()

    monkeypatch.setattr(rovo, "OAuthClientProvider", lambda **kwargs: object())
    monkeypatch.setattr(rovo.httpx2, "AsyncClient", HttpClient)
    monkeypatch.setattr(rovo, "streamable_http_client", transport)
    monkeypatch.setattr(rovo, "ClientSession", McpSession)

    with pytest.raises(ValueError, match="caller failure"):
        async with rovo.open_rovo_session(storage=object(), interactive=False):
            raise ValueError("caller failure")


@pytest.mark.asyncio
async def test_open_session_unwraps_login_required_from_transport_group(monkeypatch):
    class HttpClient:
        def __init__(self, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

    class McpSession:
        def __init__(self, *args):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

        async def initialize(self):
            raise ExceptionGroup(
                "transport",
                [rovo.LoginRequired("Jira session is missing; run corum jira login")],
            )

    @asynccontextmanager
    async def transport(*args, **kwargs):
        yield object(), object()

    monkeypatch.setattr(rovo, "OAuthClientProvider", lambda **kwargs: object())
    monkeypatch.setattr(rovo.httpx2, "AsyncClient", HttpClient)
    monkeypatch.setattr(rovo, "streamable_http_client", transport)
    monkeypatch.setattr(rovo, "ClientSession", McpSession)

    with pytest.raises(rovo.LoginRequired, match="run corum jira login"):
        async with rovo.open_rovo_session(storage=object(), interactive=False):
            pass
