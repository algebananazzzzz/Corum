"""Small first-party client for the Canvas REST API."""

from __future__ import annotations

import asyncio
from datetime import UTC, datetime
from email.utils import parsedate_to_datetime
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit

import httpx


class CanvasClient:
    """Fetch Canvas API resources with Canvas pagination and rate-limit handling."""

    _MAX_RATE_LIMIT_RETRIES = 3
    _MAX_RETRY_AFTER_SECONDS = 60.0

    def __init__(
        self,
        host: str,
        token: str,
        *,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self._base_url = f"{host.rstrip('/')}/api/v1/"
        self._origin = self._url_origin(host)
        self._headers = {"Authorization": f"Bearer {token}"}
        self._transport = transport

    @staticmethod
    def _url_origin(url: str) -> tuple[str, str, int | None]:
        parsed = urlsplit(url)
        scheme = parsed.scheme.lower()
        if scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError(f"download URL must use http or https: {url!r}")
        default_port = 443 if scheme == "https" else 80
        return scheme, parsed.hostname.lower(), parsed.port or default_port

    def _new_client(self, *, authenticated: bool = True) -> httpx.AsyncClient:
        return httpx.AsyncClient(
            base_url=self._base_url,
            headers=self._headers if authenticated else None,
            follow_redirects=True,
            timeout=httpx.Timeout(30.0, connect=10.0),
            transport=self._transport,
        )

    @classmethod
    def _retry_after(cls, response: httpx.Response) -> float:
        value = response.headers.get("Retry-After")
        if not value:
            return 0.0
        try:
            return min(max(float(value), 0.0), cls._MAX_RETRY_AFTER_SECONDS)
        except ValueError:
            try:
                retry_at = parsedate_to_datetime(value)
            except (TypeError, ValueError):
                return 0.0
            if retry_at.tzinfo is None:
                retry_at = retry_at.replace(tzinfo=UTC)
            seconds = (retry_at - datetime.now(UTC)).total_seconds()
            return min(max(seconds, 0.0), cls._MAX_RETRY_AFTER_SECONDS)

    async def _get_response(
        self,
        client: httpx.AsyncClient,
        endpoint: str,
        params: dict[str, Any] | None = None,
    ) -> httpx.Response:
        for attempt in range(self._MAX_RATE_LIMIT_RETRIES + 1):
            response = await client.get(endpoint.lstrip("/"), params=params)
            if response.status_code != httpx.codes.TOO_MANY_REQUESTS:
                response.raise_for_status()
                return response
            if attempt == self._MAX_RATE_LIMIT_RETRIES:
                response.raise_for_status()
            await response.aclose()
            await asyncio.sleep(self._retry_after(response))
        raise AssertionError("unreachable")

    async def get(self, endpoint: str, **params: Any) -> Any:
        """Return JSON from one Canvas endpoint."""
        async with self._new_client() as client:
            response = await self._get_response(client, endpoint, params or None)
            return response.json()

    async def get_all(self, endpoint: str, **params: Any) -> list[Any]:
        """Return every item from a Canvas list endpoint."""
        request_params = dict(params)
        request_params.setdefault("per_page", 100)
        records: list[Any] = []
        next_endpoint: str | None = endpoint

        async with self._new_client() as client:
            while next_endpoint is not None:
                response = await self._get_response(client, next_endpoint, request_params)
                records.extend(response.json())
                next_link = response.links.get("next")
                next_endpoint = next_link["url"] if next_link else None
                request_params = None
        return records

    async def download(self, url: str, target: Path) -> int:
        """Stream a Canvas download to ``target`` and return the byte count."""
        authenticated = self._url_origin(url) == self._origin
        target.parent.mkdir(parents=True, exist_ok=True)
        async with self._new_client(authenticated=authenticated) as client:
            for attempt in range(self._MAX_RATE_LIMIT_RETRIES + 1):
                async with client.stream("GET", url) as response:
                    if response.status_code == httpx.codes.TOO_MANY_REQUESTS:
                        if attempt == self._MAX_RATE_LIMIT_RETRIES:
                            response.raise_for_status()
                        delay = self._retry_after(response)
                    else:
                        response.raise_for_status()
                        size = 0
                        with target.open("wb") as output:
                            async for chunk in response.aiter_bytes():
                                size += output.write(chunk)
                        return size
                await asyncio.sleep(delay)
        raise AssertionError("unreachable")
