"""Command-line entry points for managing Corum vaults."""

from __future__ import annotations

import argparse
from pathlib import Path

from pydantic import ValidationError

from .workspace import initialize, validate_vault


def _path(value: str) -> Path:
    return Path(value)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="corum")
    commands = parser.add_subparsers(dest="command", required=True)
    for command in ("init", "doctor"):
        subparser = commands.add_parser(command)
        subparser.add_argument("path", nargs="?", type=_path, default=Path("."))
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        if args.command == "init":
            print(initialize(args.path))
        elif args.command == "doctor":
            _, courses = validate_vault(args.path)
            print(f"Corum vault is valid ({len(courses)} course(s))")
    except (OSError, ValueError, ValidationError) as error:
        print(f"corum: {error}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
