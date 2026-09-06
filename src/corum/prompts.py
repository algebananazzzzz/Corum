"""Small injectable terminal-prompt boundary."""

from __future__ import annotations

from collections.abc import Callable, Sequence
from typing import Generic, Protocol, TypeVar

from InquirerPy import inquirer
from InquirerPy.base.control import Choice


T = TypeVar("T")


class PromptCancelled(ValueError):
    """The user cancelled an interactive command."""


class Prompts(Protocol):
    def text(
        self,
        message: str,
        *,
        default: str = "",
        validate: Callable[[str], bool | str] | None = None,
    ) -> str: ...

    def confirm(self, message: str, *, default: bool = True) -> bool: ...

    def select(self, message: str, choices: Sequence[tuple[str, T]]) -> T: ...


class TerminalPrompts(Generic[T]):
    """InquirerPy implementation used by the command line."""

    @staticmethod
    def _execute(factory: Callable[[], T]) -> T:
        try:
            return factory()
        except (EOFError, KeyboardInterrupt) as error:
            raise PromptCancelled("setup cancelled") from error

    def text(
        self,
        message: str,
        *,
        default: str = "",
        validate: Callable[[str], bool | str] | None = None,
    ) -> str:
        while True:
            value = self._execute(
                lambda: inquirer.text(message=message, default=default).execute()
            )
            if validate is None:
                return value
            result = validate(value)
            if result is True:
                return value
            print(result if isinstance(result, str) else "Invalid input")

    def confirm(self, message: str, *, default: bool = True) -> bool:
        return self._execute(
            lambda: bool(inquirer.confirm(message=message, default=default).execute())
        )

    def select(self, message: str, choices: Sequence[tuple[str, T]]) -> T:
        values = [Choice(name=label, value=value) for label, value in choices]
        return self._execute(
            lambda: inquirer.select(message=message, choices=values).execute()
        )
