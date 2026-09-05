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
    assert await client.download("https://canvas.example.edu/files/notes.pdf", target) == 11
    assert target.read_bytes() == b"course file"


@pytest.mark.asyncio
async def test_download_never_sends_canvas_credentials_to_a_cross_origin_asset(tmp_path: Path):
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url == "https://assets.example.net/image.png"
        assert "Authorization" not in request.headers
        return httpx.Response(200, content=b"image")

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    target = tmp_path / "image.png"
    assert await client.download("https://assets.example.net/image.png", target) == 5
    assert target.read_bytes() == b"image"


@pytest.mark.asyncio
async def test_download_rejects_non_http_asset_schemes_before_request(tmp_path: Path):
    def handler(request: httpx.Request) -> httpx.Response:
        raise AssertionError("invalid schemes must not reach the transport")

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    with pytest.raises(ValueError, match="http"):
        await client.download("file:///etc/passwd", tmp_path / "passwd")


@pytest.mark.asyncio
async def test_rejected_download_url_does_not_echo_its_verifier(tmp_path: Path):
    client = CanvasClient("https://canvas.example.edu", "secret")

    with pytest.raises(ValueError) as caught:
        await client.download(
            "file:///private/path?verifier=super-secret",
            tmp_path / "file.pdf",
        )

    assert "super-secret" not in str(caught.value)
    assert "verifier" not in str(caught.value).lower()


@pytest.mark.parametrize(
    "host",
    [
        "http://canvas.example.edu",
        "https://canvas.example.edu/canvas",
        "https://student@canvas.example.edu",
        "https://canvas.example.edu?tenant=other",
    ],
)
def test_client_rejects_non_https_or_non_origin_canvas_hosts(host):
    with pytest.raises(ValueError, match="Canvas host must be an HTTPS origin"):
        CanvasClient(host, "secret")


@pytest.mark.asyncio
async def test_get_all_rejects_cross_origin_pagination_before_sending_credentials():
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.url.host == "canvas.example.edu":
            return httpx.Response(
                200,
                json=[{"id": 1}],
                headers={"Link": '<https://attacker.example/api/items?page=2>; rel="next"'},
            )
        return httpx.Response(200, json=[{"id": 2}])

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )

    with pytest.raises(ValueError, match="Canvas API URL must stay on configured origin"):
        await client.get_all("/courses/1/files")

    assert [request.url.host for request in requests] == ["canvas.example.edu"]


@pytest.mark.asyncio
async def test_get_all_rejects_credential_bearing_pagination_urls():
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(
            200,
            json=[{"id": 1}],
            headers={
                "Link": '<https://user:password@canvas.example.edu/api/v1/items?page=2>; rel="next"'
            },
        )

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )

    with pytest.raises(ValueError, match="credential-free"):
        await client.get_all("/courses/1/files")

    assert len(requests) == 1


@pytest.mark.asyncio
async def test_authenticated_api_rejects_a_cross_origin_redirect():
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.url.host == "canvas.example.edu":
            return httpx.Response(302, headers={"Location": "https://attacker.example/steal"})
        return httpx.Response(200, json={"stolen": True})

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )

    with pytest.raises(ValueError, match="Canvas API URL must stay on configured origin"):
        await client.get("/courses/1")

    assert [request.url.host for request in requests] == ["canvas.example.edu"]


@pytest.mark.asyncio
async def test_download_http_error_redacts_canvas_verifier_from_exception(tmp_path: Path):
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(403, text="forbidden")

    client = CanvasClient(
        "https://canvas.example.edu",
        "secret",
        transport=httpx.MockTransport(handler),
    )

    with pytest.raises(Exception) as caught:
        await client.download(
            "https://canvas.example.edu/files/1?download=1&verifier=super-secret",
            tmp_path / "file.pdf",
        )

    assert "super-secret" not in str(caught.value)
    assert "verifier" not in str(caught.value).lower()
