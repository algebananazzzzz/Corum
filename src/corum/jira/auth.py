"""Portable storage for Atlassian OAuth material."""

from __future__ import annotations

import json
import os
import stat
import tempfile
import time
from pathlib import Path
from typing import Any

from mcp.shared.auth import OAuthClientInformationFull, OAuthMetadata, OAuthToken
from platformdirs import user_config_path
from pydantic import ValidationError


class AuthCacheError(RuntimeError):
    """The local OAuth cache cannot be used safely."""


def auth_cache_path() -> Path:
    return user_config_path("corum", appauthor=False) / "auth.json"


def _selected_path(path: Path | None) -> Path:
    return (path or auth_cache_path()).expanduser()


def _check_private(path: Path) -> None:
    if os.name != "posix" or not path.exists():
        return
    details = path.stat()
    if details.st_uid != os.getuid():
        raise AuthCacheError(f"OAuth cache is owned by another user: {path}")
    if details.st_mode & (stat.S_IRWXG | stat.S_IRWXO):
        raise AuthCacheError(f"OAuth cache permissions are unsafe: {path}")


def _read_document(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {
            "schema": 1,
            "tokens": None,
            "token_expires_at": None,
            "client_info": None,
            "server_metadata": None,
        }
    _check_private(path)
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise AuthCacheError(f"OAuth cache is unreadable: {path}") from error
    if not isinstance(value, dict) or value.get("schema") != 1:
        raise AuthCacheError(f"OAuth cache has an unsupported format: {path}")
    return value


def _write_private_json(path: Path, value: dict[str, Any]) -> None:
    if path.exists():
        _check_private(path)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    if os.name == "posix":
        os.chmod(path.parent, 0o700)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{path.name}.", dir=path.parent
    )
    temporary = Path(temporary_name)
    try:
        if os.name == "posix":
            os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            json.dump(value, stream, ensure_ascii=False)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    except BaseException:
        temporary.unlink(missing_ok=True)
        raise


class FileTokenStorage:
    """Implement the MCP SDK token-storage protocol with one private JSON file."""

    def __init__(self, path: Path | None = None) -> None:
        self.path = _selected_path(path)

    async def get_tokens(self) -> OAuthToken | None:
        document = _read_document(self.path)
        value = document.get("tokens")
        if value is None:
            return None
        try:
            tokens = OAuthToken.model_validate(value)
        except ValidationError as error:
            raise AuthCacheError(
                f"OAuth cache contains invalid tokens: {self.path}"
            ) from error
        expires_at = document.get("token_expires_at")
        can_refresh = (
            document.get("server_metadata") is not None and tokens.refresh_token
        )
        expired = isinstance(expires_at, int | float) and expires_at <= time.time()
        legacy_refresh = "token_expires_at" not in document and can_refresh
        if can_refresh and (expired or legacy_refresh):
            return tokens.model_copy(update={"access_token": ""})
        return tokens

    async def set_tokens(self, tokens: OAuthToken) -> None:
        document = _read_document(self.path)
        document["tokens"] = tokens.model_dump(mode="json")
        document["token_expires_at"] = (
            time.time() + tokens.expires_in if tokens.expires_in is not None else None
        )
        _write_private_json(self.path, document)

    async def get_client_info(self) -> OAuthClientInformationFull | None:
        value = _read_document(self.path).get("client_info")
        if value is None:
            return None
        try:
            return OAuthClientInformationFull.model_validate(value)
        except ValidationError as error:
            raise AuthCacheError(
                f"OAuth cache contains invalid client information: {self.path}"
            ) from error

    async def set_client_info(self, client_info: OAuthClientInformationFull) -> None:
        document = _read_document(self.path)
        document["client_info"] = client_info.model_dump(mode="json")
        _write_private_json(self.path, document)

    async def get_server_metadata(self) -> OAuthMetadata | None:
        value = _read_document(self.path).get("server_metadata")
        if value is None:
            return None
        try:
            return OAuthMetadata.model_validate(value)
        except ValidationError as error:
            raise AuthCacheError(
                f"OAuth cache contains invalid server metadata: {self.path}"
            ) from error

    async def set_server_metadata(self, metadata: OAuthMetadata) -> None:
        document = _read_document(self.path)
        document["server_metadata"] = metadata.model_dump(mode="json")
        _write_private_json(self.path, document)


def auth_exists(path: Path | None = None) -> bool:
    selected = _selected_path(path)
    if not selected.exists():
        return False
    _check_private(selected)
    return True


def clear_auth(path: Path | None = None) -> bool:
    selected = _selected_path(path)
    if not selected.exists():
        return False
    _check_private(selected)
    selected.unlink()
    return True


def snapshot_auth(path: Path | None = None) -> bytes | None:
    """Read cache bytes for an in-process login rollback without exposing them."""

    selected = _selected_path(path)
    if not selected.exists():
        return None
    _check_private(selected)
    try:
        return selected.read_bytes()
    except OSError as error:
        raise AuthCacheError(f"OAuth cache is unreadable: {selected}") from error


def restore_auth(snapshot: bytes | None, path: Path | None = None) -> None:
    """Restore a cache snapshot after a cancelled replacement login."""

    selected = _selected_path(path)
    if snapshot is None:
        if selected.exists():
            _check_private(selected)
            selected.unlink()
        return
    try:
        value = json.loads(snapshot)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise AuthCacheError(f"OAuth cache backup is invalid: {selected}") from error
    if not isinstance(value, dict) or value.get("schema") != 1:
        raise AuthCacheError(
            f"OAuth cache backup has an unsupported format: {selected}"
        )
    _write_private_json(selected, value)
