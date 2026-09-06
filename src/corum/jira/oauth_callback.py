"""Local callback used by the MCP SDK's OAuth authorization flow."""

from __future__ import annotations

import asyncio
import threading
import webbrowser
from collections.abc import Callable
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

from mcp.shared.auth import AuthorizationCodeResult


class OAuthLoginError(RuntimeError):
    """Interactive OAuth authorization did not complete."""


class LoopbackOAuthCallback:
    """Receive one OAuth callback on an ephemeral loopback port."""

    def __init__(
        self,
        timeout: float = 300.0,
        browser_open: Callable[[str], bool] = webbrowser.open,
    ) -> None:
        self.timeout = timeout
        self._browser_open = browser_open
        self._loop = asyncio.get_running_loop()
        self._result: asyncio.Future[AuthorizationCodeResult] = (
            self._loop.create_future()
        )
        callback = self

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                parsed = urlsplit(self.path)
                if parsed.path != "/callback":
                    self._reply(HTTPStatus.NOT_FOUND, "Not found")
                    return
                query = parse_qs(parsed.query)
                error = query.get("error", [None])[0]
                code = query.get("code", [None])[0]
                if error or not code:
                    callback._finish(
                        error=OAuthLoginError("Atlassian authorization was denied")
                    )
                    self._reply(
                        HTTPStatus.BAD_REQUEST,
                        "Corum could not complete authorization. You may close this tab.",
                    )
                    return
                callback._finish(
                    value=AuthorizationCodeResult(
                        code=code,
                        state=query.get("state", [None])[0],
                        iss=query.get("iss", [None])[0],
                    )
                )
                self._reply(
                    HTTPStatus.OK,
                    "Corum is connected to Atlassian. You may close this tab.",
                )

            def _reply(self, status: HTTPStatus, message: str) -> None:
                body = message.encode("utf-8")
                self.send_response(status)
                self.send_header("Content-Type", "text/plain; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, format: str, *args: object) -> None:
                return

        self._server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = int(self._server.server_address[1])
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)
        self._thread.start()

    def _finish(
        self,
        *,
        value: AuthorizationCodeResult | None = None,
        error: BaseException | None = None,
    ) -> None:
        def complete() -> None:
            if self._result.done():
                return
            if error is not None:
                self._result.set_exception(error)
            elif value is not None:
                self._result.set_result(value)

        self._loop.call_soon_threadsafe(complete)

    @property
    def redirect_uri(self) -> str:
        return f"http://127.0.0.1:{self.port}/callback"

    async def redirect_handler(self, authorization_url: str) -> None:
        print(f"Open this URL to connect Atlassian:\n{authorization_url}")
        try:
            opened = self._browser_open(authorization_url)
        except Exception:  # noqa: BLE001 - browser launchers raise platform-specific errors
            opened = False
        if not opened:
            print(
                "The browser did not open automatically; open the URL above manually."
            )

    async def callback_handler(self) -> AuthorizationCodeResult:
        try:
            return await asyncio.wait_for(
                asyncio.shield(self._result), timeout=self.timeout
            )
        except TimeoutError as error:
            raise OAuthLoginError("Atlassian authorization timed out") from error

    async def aclose(self) -> None:
        await asyncio.to_thread(self._server.shutdown)
        self._server.server_close()
        self._thread.join(timeout=1)
