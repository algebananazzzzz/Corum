"""Small injectable terminal-prompt boundary."""

from __future__ import annotations

from collections.abc import Awaitable, Callable, Sequence
from typing import Protocol, TypeVar

from InquirerPy import inquirer
from InquirerPy.base.control import Choice

T = TypeVar("T")


class PromptCancelled(ValueError):
    """The user cancelled an interactive command."""


class Prompts(Protocol):
    async def text(
        self,
        message: str,
        *,
        default: str = "",
        validate: Callable[[str], bool | str] | None = None,
    ) -> str: ...

    async def confirm(self, message: str, *, default: bool = True) -> bool: ...

    async def select(self, message: str, choices: Sequence[tuple[str, T]]) -> T: ...


class TerminalPrompts:
    """InquirerPy implementation used by the command line."""

    @staticmethod
    async def _execute(factory: Callable[[], Awaitable[T]]) -> T:
        try:
            return await factory()
        except (EOFError, KeyboardInterrupt) as error:
            raise PromptCancelled("setup cancelled") from error

    async def text(
        self,
        message: str,
        *,
        default: str = "",
        validate: Callable[[str], bool | str] | None = None,
    ) -> str:
        while True:
            value = await self._execute(
                lambda: inquirer.text(message=message, default=default).execute_async()
            )
            if validate is None:
                return value
            result = validate(value)
            if result is True:
                return value
            print(result if isinstance(result, str) else "Invalid input")

    async def confirm(self, message: str, *, default: bool = True) -> bool:
        return bool(
            await self._execute(
                lambda: inquirer.confirm(
                    message=message,
                    default=default,
                ).execute_async()
            )
        )

    async def select(self, message: str, choices: Sequence[tuple[str, T]]) -> T:
        values = [Choice(name=label, value=value) for label, value in choices]
        return await self._execute(
            lambda: inquirer.select(message=message, choices=values).execute_async()
        )
