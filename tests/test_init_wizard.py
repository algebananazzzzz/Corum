from __future__ import annotations

from contextlib import asynccontextmanager

import pytest
import yaml
from pydantic import ValidationError

from corum.init_wizard import run_init_wizard
from corum.jira.oauth_callback import OAuthLoginError
from corum.jira.rovo import AtlassianResource, JiraProject
from corum.prompts import PromptCancelled


class FakePrompts:
    def __init__(self, texts, confirms, *, select_index=0):
        self.texts = iter(texts)
        self.confirms = iter(confirms)
        self.select_index = select_index

    async def text(self, message, *, default="", validate=None):
        return next(self.texts)

    async def confirm(self, message, *, default=True):
        return next(self.confirms)

    async def select(self, message, choices):
        return choices[self.select_index][1]


def prompt_values(root):
    return [
        str(root),
        "Asia/Singapore",
        "AY2026/27 Semester 1",
        "https://canvas.example.edu",
    ]


@pytest.mark.asyncio
async def test_wizard_creates_confirmed_jira_disabled_vault(tmp_path):
    vault = tmp_path / "vault"
    prompts = FakePrompts(prompt_values(vault), [True, False, True])

    result = await run_init_wizard(vault, prompts)

    assert result == vault
    workspace = yaml.safe_load((vault / "corum.yaml").read_text())
    assert workspace["features"] == {
        "jira": {"enabled": False},
        "wiki": {"enabled": True},
    }
    assert "jira" not in workspace


@pytest.mark.asyncio
async def test_wizard_selects_current_compact_jira_resource(tmp_path):
    vault = tmp_path / "vault"
    prompts = FakePrompts(prompt_values(vault), [False, True, True])

    class Session:
        async def resources(self):
            return [AtlassianResource(id="cloud-1", name=None, url=None)]

        async def projects(self, cloud_id):
            return [JiraProject(key="TODO", name="Todo")]

    @asynccontextmanager
    async def session_factory():
        yield Session()

    await run_init_wizard(vault, prompts, session_factory=session_factory)

    workspace = yaml.safe_load((vault / "corum.yaml").read_text())
    assert workspace["features"]["jira"]["enabled"] is True
    assert workspace["features"]["wiki"]["enabled"] is False
    assert workspace["jira"] == {
        "cloud_id": "cloud-1",
        "project": "TODO",
        "transitions": {},
    }


@pytest.mark.asyncio
async def test_final_cancellation_writes_no_vault_files(tmp_path):
    vault = tmp_path / "vault"
    prompts = FakePrompts(prompt_values(vault), [True, False, False])

    with pytest.raises(PromptCancelled):
        await run_init_wizard(vault, prompts)

    assert not vault.exists()


@pytest.mark.asyncio
async def test_invalid_timezone_is_rejected_before_files_are_written(tmp_path):
    vault = tmp_path / "vault"
    prompts = FakePrompts(
        [str(vault), "Mars/Olympus", "Term", "https://canvas.example.edu"],
        [True, False, True],
    )

    with pytest.raises(ValidationError, match="timezone"):
        await run_init_wizard(vault, prompts)

    assert not vault.exists()


@pytest.mark.asyncio
async def test_oauth_cancellation_writes_no_vault_files(tmp_path):
    vault = tmp_path / "vault"
    prompts = FakePrompts(prompt_values(vault), [True, True])

    @asynccontextmanager
    async def cancelled_session():
        raise OAuthLoginError("cancelled")
        yield

    with pytest.raises(OAuthLoginError):
        await run_init_wizard(vault, prompts, session_factory=cancelled_session)

    assert not vault.exists()
