from __future__ import annotations

import json
import os

import pytest
from mcp.shared.auth import OAuthClientInformationFull, OAuthMetadata, OAuthToken

from corum.jira.auth import (
    AuthCacheError,
    FileTokenStorage,
    auth_exists,
    clear_auth,
    restore_auth,
    snapshot_auth,
)


@pytest.mark.asyncio
async def test_token_and_client_registration_round_trip_in_one_private_file(tmp_path):
    path = tmp_path / "corum" / "auth.json"
    storage = FileTokenStorage(path)
    assert await storage.get_tokens() is None
    assert await storage.get_client_info() is None

    await storage.set_tokens(
        OAuthToken(
            access_token="synthetic-access",
            refresh_token="synthetic-refresh",
            expires_in=3600,
        )
    )
    await storage.set_client_info(
        OAuthClientInformationFull(client_id="synthetic-client")
    )

    assert (await storage.get_tokens()).refresh_token == "synthetic-refresh"
    assert (await storage.get_client_info()).client_id == "synthetic-client"
    document = json.loads(path.read_text())
    assert document["schema"] == 1
    assert document["tokens"]["access_token"] == "synthetic-access"
    assert document["client_info"]["client_id"] == "synthetic-client"
    if os.name == "posix":
        assert path.parent.stat().st_mode & 0o777 == 0o700
        assert path.stat().st_mode & 0o777 == 0o600


@pytest.mark.asyncio
async def test_each_storage_setter_preserves_the_other_record(tmp_path):
    path = tmp_path / "auth.json"
    storage = FileTokenStorage(path)
    await storage.set_client_info(OAuthClientInformationFull(client_id="client-1"))
    await storage.set_tokens(OAuthToken(access_token="access-1"))
    await storage.set_tokens(OAuthToken(access_token="access-2"))

    assert (await storage.get_client_info()).client_id == "client-1"
    assert (await storage.get_tokens()).access_token == "access-2"


@pytest.mark.asyncio
async def test_expired_cached_access_token_is_returned_as_refresh_only(tmp_path):
    path = tmp_path / "auth.json"
    storage = FileTokenStorage(path)
    await storage.set_client_info(OAuthClientInformationFull(client_id="client-1"))
    await storage.set_server_metadata(
        OAuthMetadata(
            issuer="https://auth.example/tenant",
            authorization_endpoint="https://auth.example/authorize",
            token_endpoint="https://auth.example/token",
        )
    )
    await storage.set_tokens(
        OAuthToken(
            access_token="expired-access", refresh_token="refresh-1", expires_in=1
        )
    )
    document = json.loads(path.read_text())
    document["token_expires_at"] = 0
    path.write_text(json.dumps(document))
    path.chmod(0o600)

    tokens = await storage.get_tokens()

    assert tokens is not None
    assert tokens.access_token == ""
    assert tokens.refresh_token == "refresh-1"
    assert str((await storage.get_server_metadata()).token_endpoint) == (
        "https://auth.example/token"
    )


@pytest.mark.asyncio
async def test_corrupt_cache_error_does_not_include_contents(tmp_path):
    path = tmp_path / "auth.json"
    path.write_text("not-json synthetic-secret")
    path.chmod(0o600)

    with pytest.raises(AuthCacheError) as caught:
        await FileTokenStorage(path).get_tokens()

    assert str(path) in str(caught.value)
    assert "synthetic-secret" not in str(caught.value)


@pytest.mark.skipif(os.name != "posix", reason="POSIX permission semantics")
def test_group_readable_cache_is_rejected(tmp_path):
    path = tmp_path / "auth.json"
    path.write_text('{"schema": 1, "tokens": null, "client_info": null}')
    path.chmod(0o640)

    with pytest.raises(AuthCacheError, match="permissions are unsafe"):
        auth_exists(path)


@pytest.mark.asyncio
async def test_snapshot_restore_and_clear_are_scoped_to_auth_file(tmp_path):
    path = tmp_path / "config" / "auth.json"
    sibling = path.parent / "keep.txt"
    storage = FileTokenStorage(path)
    await storage.set_tokens(OAuthToken(access_token="original"))
    sibling.write_text("keep")
    original = snapshot_auth(path)

    await storage.set_tokens(OAuthToken(access_token="replacement"))
    restore_auth(original, path)

    assert (await storage.get_tokens()).access_token == "original"
    assert clear_auth(path) is True
    assert clear_auth(path) is False
    assert sibling.read_text() == "keep"
