import pytest

from corum import prompts


@pytest.mark.asyncio
async def test_terminal_prompts_use_inquirerpys_async_execution(monkeypatch):
    class Question:
        async def execute_async(self):
            return True

    monkeypatch.setattr(prompts.inquirer, "confirm", lambda **kwargs: Question())

    assert await prompts.TerminalPrompts().confirm("Continue?") is True
