from __future__ import annotations

import asyncio
from urllib.error import HTTPError
from urllib.request import urlopen

import pytest

from corum.jira.oauth_callback import LoopbackOAuthCallback, OAuthLoginError


def request(url: str) -> None:
    with urlopen(url):
        pass


@pytest.mark.asyncio
async def test_callback_extracts_code_state_and_issuer_and_ignores_second_callback():
    callback = LoopbackOAuthCallback(timeout=1, browser_open=lambda _: True)
    try:
        await asyncio.to_thread(
            request,
            callback.redirect_uri
            + "?code=first&state=state-1&iss=https%3A%2F%2Fissuer",
        )
        await asyncio.to_thread(
            request,
            callback.redirect_uri + "?code=second&state=state-2",
        )

        result = await callback.callback_handler()

        assert result.code == "first"
        assert result.state == "state-1"
        assert result.iss == "https://issuer"
    finally:
        await callback.aclose()


@pytest.mark.asyncio
async def test_unknown_callback_path_returns_404_without_completing_login():
    callback = LoopbackOAuthCallback(timeout=0.05, browser_open=lambda _: True)
    try:
        with pytest.raises(HTTPError) as caught:
            await asyncio.to_thread(
                request,
                callback.redirect_uri.replace("/callback", "/wrong"),
            )
        assert caught.value.code == 404
        with pytest.raises(OAuthLoginError, match="timed out"):
            await callback.callback_handler()
    finally:
        await callback.aclose()


@pytest.mark.asyncio
async def test_provider_error_is_redacted():
    callback = LoopbackOAuthCallback(timeout=1, browser_open=lambda _: True)
    try:
        with pytest.raises(HTTPError):
            await asyncio.to_thread(
                request,
                callback.redirect_uri
                + "?error=access_denied&error_description=private",
            )
        with pytest.raises(OAuthLoginError) as caught:
            await callback.callback_handler()
        assert "private" not in str(caught.value)
        assert "access_denied" not in str(caught.value)
    finally:
        await callback.aclose()


@pytest.mark.asyncio
async def test_browser_failure_prints_manual_url_without_aborting(capsys):
    callback = LoopbackOAuthCallback(
        timeout=1,
        browser_open=lambda _: (_ for _ in ()).throw(RuntimeError("browser failed")),
    )
    try:
        await callback.redirect_handler("https://auth.example/authorize")
        output = capsys.readouterr().out
        assert "https://auth.example/authorize" in output
        assert "manually" in output
        assert "browser failed" not in output
    finally:
        await callback.aclose()
