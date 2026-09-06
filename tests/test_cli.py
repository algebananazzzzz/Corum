from __future__ import annotations

import io
from contextlib import asynccontextmanager

import pytest
import yaml
from mcp.shared.auth import OAuthToken

from corum import cli
from corum.jira.auth import FileTokenStorage
from corum.jira.rovo import AtlassianResource, JiraProject
from corum.workspace import initialize


class FakePrompts:
    def __init__(self, confirms):
        self.confirms = iter(confirms)

    async def confirm(self, message, *, default=True):
        return next(self.confirms)

    async def select(self, message, choices):
        return choices[0][1]


class Session:
    async def user_info(self):
        return {"displayName": "Example User", "accountId": "account-1"}

    async def resources(self):
        return [AtlassianResource(id="cloud-1", name=None, url=None)]

    async def projects(self, cloud_id):
        return [JiraProject(key="TODO", name="Todo")]


def test_noninteractive_init_requires_defaults(tmp_path, monkeypatch, capsys):
    monkeypatch.setattr(cli.sys, "stdin", io.StringIO())

    assert cli.main(["init", str(tmp_path / "vault")]) == 1

    assert "use --defaults" in capsys.readouterr().out


def test_defaults_init_never_constructs_prompts(tmp_path, monkeypatch):
    class ForbiddenPrompts:
        def __init__(self):
            raise AssertionError("defaults must not construct prompts")

    monkeypatch.setattr(cli, "TerminalPrompts", ForbiddenPrompts)
    vault = tmp_path / "vault"

    assert cli.main(["init", "--defaults", str(vault)]) == 0
    assert (
        yaml.safe_load((vault / "corum.yaml").read_text())["features"]["jira"][
            "enabled"
        ]
        is False
    )


@pytest.mark.asyncio
async def test_login_updates_only_secret_free_workspace_fields(tmp_path, monkeypatch):
    vault = tmp_path / "vault"
    initialize(vault)
    cache = tmp_path / "config" / "auth.json"
    monkeypatch.setattr(cli, "auth_cache_path", lambda: cache)

    @asynccontextmanager
    async def fake_session():
        yield Session()

    monkeypatch.setattr(cli, "open_rovo_session", fake_session)

    await cli._jira_login(vault, FakePrompts([True]))

    workspace = yaml.safe_load((vault / "corum.yaml").read_text())
    assert workspace["jira"] == {
        "cloud_id": "cloud-1",
        "project": "TODO",
        "transitions": {},
    }
    assert "token" not in str(workspace).casefold()


@pytest.mark.asyncio
async def test_cancelled_replacement_login_restores_auth_and_yaml(
    tmp_path, monkeypatch
):
    vault = tmp_path / "vault"
    initialize(vault)
    workspace_before = (vault / "corum.yaml").read_bytes()
    cache = tmp_path / "config" / "auth.json"
    storage = FileTokenStorage(cache)
    await storage.set_tokens(OAuthToken(access_token="original"))
    auth_before = cache.read_bytes()
    monkeypatch.setattr(cli, "auth_cache_path", lambda: cache)

    @asynccontextmanager
    async def fake_session():
        await storage.set_tokens(OAuthToken(access_token="replacement"))
        yield Session()

    monkeypatch.setattr(cli, "open_rovo_session", fake_session)

    with pytest.raises(ValueError, match="login cancelled"):
        await cli._jira_login(vault, FakePrompts([True, False]))

    assert cache.read_bytes() == auth_before
    assert (vault / "corum.yaml").read_bytes() == workspace_before


@pytest.mark.asyncio
async def test_status_prints_account_and_workspace_without_auth_material(
    tmp_path, monkeypatch, capsys
):
    vault = tmp_path / "vault"
    initialize(vault)
    monkeypatch.setattr(cli, "auth_exists", lambda: True)

    @asynccontextmanager
    async def fake_session(**kwargs):
        assert kwargs == {"interactive": False}
        yield Session()

    monkeypatch.setattr(cli, "open_rovo_session", fake_session)

    await cli._jira_status(vault)

    output = capsys.readouterr().out
    assert "Example User" in output
    assert "access_token" not in output
    assert "refresh_token" not in output


def test_logout_is_idempotent(monkeypatch, capsys):
    values = iter([True, False])
    monkeypatch.setattr(cli, "clear_auth", lambda: next(values))

    cli._jira_logout()
    cli._jira_logout()

    assert capsys.readouterr().out.splitlines() == ["Logged out", "No Jira session"]
