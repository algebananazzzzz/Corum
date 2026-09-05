"""Command-line entry points for managing Corum vaults."""

from __future__ import annotations

import argparse
import asyncio
import json
from pathlib import Path

from pydantic import ValidationError

from .canvas.sync import sync_course
from .workspace import initialize, validate_vault


def _path(value: str) -> Path:
    return Path(value)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="corum")
    commands = parser.add_subparsers(dest="command", required=True)
    for command in ("init", "doctor"):
        subparser = commands.add_parser(command)
        subparser.add_argument("path", nargs="?", type=_path, default=Path("."))
    sync_parser = commands.add_parser("sync")
    sync_parser.add_argument("course", nargs="*")
    sync_parser.add_argument("--all", action="store_true")
    sync_parser.add_argument("--dry-run", action="store_true")
    sync_parser.add_argument("--json", action="store_true")
    return parser


async def _sync_selected(vault: Path, courses: list, dry_run: bool):
    return [await sync_course(vault, course, dry_run) for course in courses]


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        if args.command == "init":
            print(initialize(args.path))
        elif args.command == "doctor":
            _, courses = validate_vault(args.path)
            print(f"Corum vault is valid ({len(courses)} course(s))")
        elif args.command == "sync":
            if args.all and args.course:
                raise ValueError("choose course codes or --all, not both")
            if not args.all and not args.course:
                raise ValueError("provide at least one course code or --all")
            vault = Path(".").resolve()
            _, available = validate_vault(vault)
            by_code = {course.code.upper(): course for course in available}
            wanted = sorted(by_code) if args.all else [code.upper() for code in args.course]
            missing = [code for code in wanted if code not in by_code]
            if missing:
                raise ValueError(f"no course configuration for: {', '.join(missing)}")
            manifests = asyncio.run(
                _sync_selected(vault, [by_code[code] for code in wanted], args.dry_run)
            )
            if args.json:
                print(
                    json.dumps(
                        {
                            "dry_run": args.dry_run,
                            "courses": [manifest.model_dump(mode="json") for manifest in manifests],
                        },
                        ensure_ascii=False,
                        indent=2,
                    )
                )
            else:
                for manifest in manifests:
                    print(f"{manifest.course}: {manifest.canvas.status}")
                    for change in manifest.canvas.changes:
                        print(f"  + {change.summary}")
                    for failure in manifest.canvas.failures:
                        print(f"  ! {failure.source}: {failure.error}")
    except (OSError, RuntimeError, ValueError, ValidationError) as error:
        print(f"corum: {error}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
