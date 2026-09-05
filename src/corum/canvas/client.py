"""Small first-party client for the Canvas REST API."""

from __future__ import annotations

import asyncio
from datetime import UTC, datetime
from email.utils import parsedate_to_datetime
from pathlib import Path
from typing import Any
from urllib.parse import parse_qsl, urlencode, urljoin, urlsplit, urlunsplit

import httpx

from corum.validation import require_https_origin


class CanvasRequestError(RuntimeError):
    """A Canvas request failed without exposing secret-bearing URLs."""


class CanvasClient:
    """Fetch Canvas API resources with Canvas pagination and rate-limit handling."""

    _MAX_RATE_LIMIT_RETRIES = 3
    _MAX_RETRY_AFTER_SECONDS = 60.0
    _MAX_REDIRECTS = 5

    def __init__(
        self,
        host: str,
        token: str,
        *,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        origin = require_https_origin(host, "Canvas host")
        self._base_url = f"{origin}/api/v1/"
        self._origin = self._url_origin(origin)
        self._headers = {"Authorization": f"Bearer {token}"}
        self._transport = transport

    @staticmethod
    def _url_origin(url: str) -> tuple[str, str, int]:
        parsed = urlsplit(url)
        scheme = parsed.scheme.lower()
        if scheme not in {"http", "https"} or not parsed.hostname:
            raise ValueError("download URL must use http or https")
        default_port = 443 if scheme == "https" else 80
        return scheme, parsed.hostname.lower(), parsed.port or default_port

    @staticmethod
    def _redacted_url(url: str) -> str:
        parts = urlsplit(url)
        kept = [
            (key, value)
            for key, value in parse_qsl(parts.query, keep_blank_values=True)
            if key.casefold() != "verifier"
        ]
        return urlunsplit(parts._replace(query=urlencode(kept)))

    def _api_url(self, endpoint: str) -> str:
        target = urljoin(self._base_url, endpoint) if urlsplit(endpoint).scheme else urljoin(
            self._base_url, endpoint.lstrip("/")
        )
        parsed = urlsplit(target)
        if parsed.username is not None or parsed.password is not None:
            raise ValueError("Canvas API URL must be credential-free")
        if self._url_origin(target) != self._origin:
            raise ValueError("Canvas API URL must stay on configured origin")
        return target

    @classmethod
    def _raise_status(cls, response: httpx.Response) -> None:
        if response.is_error:
            raise CanvasRequestError(
                f"Canvas request failed with HTTP {response.status_code}: "
                f"{cls._redacted_url(str(response.request.url))}"
            )

    @classmethod
    def _request_error(cls, error: httpx.RequestError) -> CanvasRequestError:
        return CanvasRequestError(
            f"Canvas request failed: {type(error).__name__}: "
            f"{cls._redacted_url(str(error.request.url))}"
        )

    def _new_client(self) -> httpx.AsyncClient:
        return httpx.AsyncClient(
            base_url=self._base_url,
            follow_redirects=False,
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
        target = self._api_url(endpoint)
        redirects = 0
        attempt = 0
        while True:
            try:
                response = await client.get(target, params=params, headers=self._headers)
            except httpx.RequestError as error:
                raise self._request_error(error) from None
            if response.is_redirect:
                location = response.headers.get("Location")
                if not location or redirects >= self._MAX_REDIRECTS:
                    self._raise_status(response)
                    raise CanvasRequestError("Canvas API redirect could not be followed safely")
                target = self._api_url(urljoin(str(response.request.url), location))
                params = None
                redirects += 1
                await response.aclose()
                continue
            if response.status_code != httpx.codes.TOO_MANY_REQUESTS:
                self._raise_status(response)
                return response
            if attempt == self._MAX_RATE_LIMIT_RETRIES:
                self._raise_status(response)
            await response.aclose()
            await asyncio.sleep(self._retry_after(response))
            attempt += 1

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
        self._url_origin(url)
        target.parent.mkdir(parents=True, exist_ok=True)
        async with self._new_client() as client:
            attempt = 0
            redirects = 0
            current = url
            while True:
                authenticated = self._url_origin(current) == self._origin
                try:
                    stream = client.stream(
                        "GET",
                        current,
                        headers=self._headers if authenticated else None,
                    )
                    async with stream as response:
                        if response.is_redirect:
                            location = response.headers.get("Location")
                            if not location or redirects >= self._MAX_REDIRECTS:
                                raise CanvasRequestError(
                                    "Canvas download redirect could not be followed safely"
                                )
                            current = urljoin(str(response.request.url), location)
                            self._url_origin(current)
                            redirects += 1
                            continue
                        if response.status_code == httpx.codes.TOO_MANY_REQUESTS:
                            if attempt == self._MAX_RATE_LIMIT_RETRIES:
                                self._raise_status(response)
                            delay = self._retry_after(response)
                            attempt += 1
                        else:
                            self._raise_status(response)
                            size = 0
                            with target.open("wb") as output:
                                async for chunk in response.aiter_bytes():
                                    size += output.write(chunk)
                            return size
                except httpx.RequestError as error:
                    raise self._request_error(error) from None
                await asyncio.sleep(delay)
