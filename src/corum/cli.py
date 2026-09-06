"""Command-line entry points for managing Corum vaults."""

from __future__ import annotations

import argparse
import asyncio
import json
from pathlib import Path
import sys

from httpx import HTTPError
from pydantic import ValidationError
import yaml

from .canvas.sync import sync_course
from .config import load_workspace, resolve_features
from .init_wizard import run_init_wizard, select_jira
from .jira import JiraClient, open_rovo_session
from .jira.apply import JiraDisabled, JiraPlan, apply_plan
from .jira.auth import (
    auth_cache_path,
    auth_exists,
    clear_auth,
    restore_auth,
    snapshot_auth,
)
from .jira.setup import configured_workspace, write_workspace_atomic
from .prompts import Prompts, TerminalPrompts
from .workspace import initialize, validate_selected_courses, validate_vault


def _path(value: str) -> Path:
    return Path(value)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="corum")
    commands = parser.add_subparsers(dest="command", required=True)
    init_parser = commands.add_parser("init")
    init_parser.add_argument("path", nargs="?", type=_path, default=Path("."))
    init_parser.add_argument("--defaults", action="store_true")
    doctor_parser = commands.add_parser("doctor")
    doctor_parser.add_argument("path", nargs="?", type=_path, default=Path("."))
    sync_parser = commands.add_parser("sync")
    sync_parser.add_argument("course", nargs="*")
    sync_parser.add_argument("--all", action="store_true")
    sync_parser.add_argument("--dry-run", action="store_true")
    sync_parser.add_argument("--json", action="store_true")
    jira_parser = commands.add_parser("jira")
    jira_commands = jira_parser.add_subparsers(dest="jira_command", required=True)
    login_parser = jira_commands.add_parser("login")
    login_parser.add_argument("path", nargs="?", type=_path, default=Path("."))
    status_parser = jira_commands.add_parser("status")
    status_parser.add_argument("path", nargs="?", type=_path, default=Path("."))
    jira_commands.add_parser("logout")
    apply_parser = jira_commands.add_parser("apply")
    apply_parser.add_argument("course")
    apply_parser.add_argument("--dry-run", action="store_true")
    return parser


async def _sync_selected(vault: Path, courses: list, dry_run: bool):
    return [await sync_course(vault, course, dry_run) for course in courses]


def _account_label(profile: dict[str, object]) -> str:
    name = profile.get("displayName") or profile.get("name") or "Atlassian user"
    email = profile.get("email") or profile.get("emailAddress")
    return f"{name} ({email})" if email else str(name)


async def _jira_login(vault: Path, prompts: Prompts) -> None:
    cache = auth_cache_path()
    original = snapshot_auth(cache)
    if original is not None and not prompts.confirm(
        "Replace the current Atlassian login?",
        default=False,
    ):
        raise ValueError("login cancelled")
    if original is not None:
        clear_auth(cache)

    committed = False
    try:
        async with open_rovo_session() as session:
            profile = await session.user_info()
            workspace_path = vault.resolve() / "corum.yaml"
            if workspace_path.is_file():
                workspace = load_workspace(vault.resolve())
                selected = await select_jira(session, prompts)
                updated = configured_workspace(workspace, selected)
                preview = {"features": updated.features, "jira": updated.jira}
                print("\nWorkspace Jira preview:")
                print(
                    yaml.safe_dump(
                        {key: value.model_dump(mode="json", exclude_none=True) for key, value in preview.items()},
                        sort_keys=False,
                    )
                )
                if not prompts.confirm("Update this workspace?", default=True):
                    raise ValueError("login cancelled")
                write_workspace_atomic(workspace_path, updated)
            print(f"Logged in as {_account_label(profile)}")
            committed = True
    finally:
        if not committed:
            restore_auth(original, cache)


async def _jira_status(vault: Path) -> None:
    if not auth_exists():
        print("No Jira session")
        return
    async with open_rovo_session(interactive=False) as session:
        profile = await session.user_info()
        print(f"Logged in as {_account_label(profile)}")
    workspace_path = vault.resolve() / "corum.yaml"
    if workspace_path.is_file():
        workspace = load_workspace(vault.resolve())
        if workspace.jira is not None:
            site = str(workspace.jira.site) if workspace.jira.site else workspace.jira.cloud_id
            print(f"Workspace Jira: {site} ({workspace.jira.project})")


def _jira_logout() -> None:
    print("Logged out" if clear_auth() else "No Jira session")


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        if args.command == "init":
            if args.defaults:
                print(initialize(args.path))
            elif not (sys.stdin.isatty() and sys.stdout.isatty()):
                raise ValueError("interactive initialization requires a terminal; use --defaults")
            else:
                print(asyncio.run(run_init_wizard(args.path, TerminalPrompts())))
        elif args.command == "doctor":
            _, courses = validate_vault(args.path)
            print(f"Corum vault is valid ({len(courses)} course(s))")
        elif args.command == "sync":
            if args.all and args.course:
                raise ValueError("choose course codes or --all, not both")
            if not args.all and not args.course:
                raise ValueError("provide at least one course code or --all")
            vault = Path(".").resolve()
            _, available = (
                validate_vault(vault)
                if args.all
                else validate_selected_courses(vault, args.course)
            )
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
        elif args.command == "jira" and args.jira_command == "login":
            asyncio.run(_jira_login(args.path, TerminalPrompts()))
        elif args.command == "jira" and args.jira_command == "status":
            asyncio.run(_jira_status(args.path))
        elif args.command == "jira" and args.jira_command == "logout":
            _jira_logout()
        elif args.command == "jira" and args.jira_command == "apply":
            vault = Path(".").resolve()
            workspace, courses = validate_vault(vault)
            by_code = {course.code.upper(): course for course in courses}
            code = args.course.upper()
            if code not in by_code:
                raise ValueError(f"no course configuration for: {code}")
            course = by_code[code]
            if not resolve_features(workspace, course).jira:
                raise JiraDisabled(f"Jira is disabled for {course.code}")
            plan = JiraPlan.model_validate_json(sys.stdin.read())
            if args.dry_run:
                asyncio.run(apply_plan(vault, course, plan, client=None, dry_run=True))
                print(json.dumps(plan.model_dump(mode="json", exclude_unset=True), indent=2))
            else:
                if workspace.jira is None:
                    raise ValueError(f"enabled Jira configuration is incomplete for {course.code}")
                async def run_jira_apply():
                    async with open_rovo_session(interactive=False) as session:
                        client = JiraClient(session, workspace.jira.cloud_id)
                        return await apply_plan(vault, course, plan, client)

                result = asyncio.run(run_jira_apply())
                print(json.dumps(result.model_dump(mode="json"), indent=2))
                if result.status in {"partial", "failed"}:
                    return 1
    except (HTTPError, OSError, RuntimeError, ValueError, ValidationError) as error:
        print(f"corum: {error}")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
