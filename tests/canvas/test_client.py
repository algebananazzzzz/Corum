from pathlib import Path

import httpx
import pytest

from corum.canvas.client import CanvasClient


@pytest.mark.asyncio
async def test_client_adds_canvas_api_prefix_and_bearer_authentication():
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url == "https://canvas.example.edu/api/v1/courses/1"
        assert request.headers["Authorization"] == "Bearer secret"
        return httpx.Response(200, json={"id": 1})

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    assert await client.get("/courses/1") == {"id": 1}


def handler(request: httpx.Request) -> httpx.Response:
    if request.url.params.get("page") == "2":
        return httpx.Response(200, json=[{"id": 2}])
    return httpx.Response(
        200,
        json=[{"id": 1}],
        headers={"Link": '<https://canvas.example.edu/api/v1/courses/1/files?page=2>; rel="next"'},
    )


@pytest.mark.asyncio
async def test_get_all_follows_canvas_next_links():
    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    assert await client.get_all("/courses/1/files") == [{"id": 1}, {"id": 2}]


@pytest.mark.asyncio
async def test_get_retries_a_rate_limited_request_using_retry_after():
    calls = 0

    def rate_limited_then_ok(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if calls == 1:
            return httpx.Response(429, headers={"Retry-After": "0"})
        return httpx.Response(200, json={"id": 1})

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(rate_limited_then_ok),
    )
    assert await client.get("/courses/1") == {"id": 1}
    assert calls == 2


@pytest.mark.asyncio
async def test_download_streams_response_to_target_and_returns_its_size(tmp_path: Path):
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.headers["Authorization"] == "Bearer secret"
        return httpx.Response(200, content=b"course file")

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    target = tmp_path / "files" / "notes.pdf"
    assert await client.download("https://files.example.edu/notes.pdf", target) == 11
    assert target.read_bytes() == b"course file"
