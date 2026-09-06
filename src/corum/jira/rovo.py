"""Narrow client wrapper for Atlassian's Rovo MCP server."""

from __future__ import annotations

from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
import json
from typing import Any
import webbrowser

import httpx2
from mcp import ClientSession, types
from mcp.client.auth import OAuthClientProvider
from mcp.client.streamable_http import streamable_http_client
from mcp.shared.auth import OAuthClientMetadata
from pydantic import AliasChoices, AnyUrl, BaseModel, Field, HttpUrl, ValidationError

from .auth import AuthCacheError, FileTokenStorage
from .oauth_callback import LoopbackOAuthCallback, OAuthLoginError


ROVO_MCP_URL = "https://mcp.atlassian.com/v2/mcp"
ROVO_MCP_TRANSPORT_URL = f"{ROVO_MCP_URL}?tools=all"


class RovoError(RuntimeError):
    """A redacted Rovo MCP operation failure."""


class LoginRequired(RovoError):
    """A command cannot start an interactive OAuth flow."""


class AtlassianResource(BaseModel):
    id: str = Field(validation_alias=AliasChoices("id", "cloudId"))
    url: HttpUrl | None = None
    name: str | None = None
    products: list[dict[str, object]] = Field(default_factory=list)


class JiraProject(BaseModel):
    key: str
    name: str


def _unwrap_list(value: object, keys: tuple[str, ...]) -> list[object]:
    current = value
    for _ in range(3):
        if isinstance(current, list):
            return current
        if not isinstance(current, dict):
            break
        nested = next((current[key] for key in keys if isinstance(current.get(key), list)), None)
        if nested is not None:
            return nested
        current = current.get("data")
    raise RovoError("Atlassian returned an unexpected response")


class RovoSession:
    """Expose only the Rovo operations used by Corum."""

    def __init__(self, session: ClientSession) -> None:
        self._session = session
        self._tools: dict[str, types.Tool] | None = None

    async def list_tools(self) -> dict[str, types.Tool]:
        if self._tools is not None:
            return self._tools
        tools: dict[str, types.Tool] = {}
        cursor: str | None = None
        try:
            while True:
                params = types.PaginatedRequestParams(cursor=cursor) if cursor else None
                page = await self._session.list_tools(params=params)
                tools.update((tool.name, tool) for tool in page.tools)
                cursor = page.next_cursor
                if cursor is None:
                    break
        except Exception as error:
            raise RovoError("Could not discover Atlassian tools") from error
        self._tools = tools
        return tools

    async def call_json(self, name: str, arguments: dict[str, object]) -> object:
        tools = await self.list_tools()
        if name not in tools:
            raise RovoError(f"Required Atlassian tool is unavailable: {name}")
        try:
            result = await self._session.call_tool(name, arguments)
        except Exception as error:
            raise RovoError(f"Atlassian tool failed: {name}") from error
        if not isinstance(result, types.CallToolResult) or result.is_error:
            raise RovoError(f"Atlassian tool failed: {name}")
        if result.structured_content is not None:
            return result.structured_content
        text_blocks = [block.text for block in result.content if isinstance(block, types.TextContent)]
        if len(text_blocks) != len(result.content) or not text_blocks:
            raise RovoError(f"Atlassian tool returned an unsupported response: {name}")
        try:
            value, _ = json.JSONDecoder().raw_decode("".join(text_blocks).lstrip())
            return value
        except json.JSONDecodeError as error:
            raise RovoError(f"Atlassian tool returned invalid JSON: {name}") from error

    async def user_info(self) -> dict[str, object]:
        value = await self.call_json("atlassianUserInfo", {})
        if not isinstance(value, dict):
            raise RovoError("Atlassian returned invalid account information")
        return value

    async def resources(self) -> list[AtlassianResource]:
        values = _unwrap_list(
            await self.call_json("getAccessibleAtlassianResources", {}),
            ("resources", "values", "results"),
        )
        try:
            resources = [AtlassianResource.model_validate(value) for value in values]
        except ValidationError as error:
            raise RovoError("Atlassian returned invalid site information") from error
        resources = [
            resource
            for resource in resources
            if not resource.products
            or any(product.get("id") == "jira" for product in resource.products)
        ]
        return sorted(resources, key=lambda item: ((item.name or "").casefold(), item.id))

    async def projects(self, cloud_id: str) -> list[JiraProject]:
        values = _unwrap_list(
            await self.call_json(
                "listJiraProjects",
                {"cloudId": cloud_id, "maxResults": 100},
            ),
            ("projects", "values", "results"),
        )
        try:
            projects = [JiraProject.model_validate(value) for value in values]
        except ValidationError as error:
            raise RovoError("Atlassian returned invalid project information") from error
        return sorted(projects, key=lambda item: (item.name.casefold(), item.key))


async def _login_required(_: str) -> None:
    raise LoginRequired("Jira session is missing or revoked; run corum jira login")


async def _callback_unavailable():
    raise LoginRequired("Jira session is missing or revoked; run corum jira login")


@asynccontextmanager
async def open_rovo_session(
    storage: FileTokenStorage | None = None,
    *,
    browser_open: Callable[[str], bool] = webbrowser.open,
    timeout: float = 120.0,
    interactive: bool = True,
) -> AsyncIterator[RovoSession]:
    """Open one authenticated MCP session, optionally allowing browser login."""

    callback = (
        LoopbackOAuthCallback(timeout=timeout, browser_open=browser_open)
        if interactive
        else None
    )
    redirect_uri = callback.redirect_uri if callback else "http://127.0.0.1/callback"
    metadata = OAuthClientMetadata(
        client_name="Corum",
        redirect_uris=[AnyUrl(redirect_uri)],
        grant_types=["authorization_code", "refresh_token"],
        response_types=["code"],
        token_endpoint_auth_method="none",
    )
    auth = OAuthClientProvider(
        server_url=ROVO_MCP_URL,
        client_metadata=metadata,
        storage=storage or FileTokenStorage(),
        redirect_handler=callback.redirect_handler if callback else _login_required,
        callback_handler=callback.callback_handler if callback else _callback_unavailable,
    )
    try:
        async with httpx2.AsyncClient(auth=auth, follow_redirects=True) as client:
            async with streamable_http_client(
                ROVO_MCP_TRANSPORT_URL,
                http_client=client,
            ) as streams:
                async with ClientSession(streams[0], streams[1]) as session:
                    await session.initialize()
                    yield RovoSession(session)
    except (AuthCacheError, LoginRequired, OAuthLoginError):
        raise
    except Exception as error:
        raise RovoError("Could not connect to Atlassian") from error
    finally:
        if callback is not None:
            await callback.aclose()
