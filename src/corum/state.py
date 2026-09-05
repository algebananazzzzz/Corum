"""Machine-owned course state and per-course locking."""

from __future__ import annotations

from contextlib import contextmanager
import json
import os
from pathlib import Path
import tempfile
from typing import Iterator


def _state_path(course_dir: Path, name: str) -> Path:
    return course_dir / "state" / name


def read_canvas_state(course_dir: Path) -> dict:
    """Read the Canvas-owned capture ledger for one course."""
    return json.loads(_state_path(course_dir, "canvas.json").read_text(encoding="utf-8"))


def _write_json(target: Path, value: dict) -> None:
    target.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        "w",
        encoding="utf-8",
        dir=target.parent,
        prefix=f".{target.stem}-",
        suffix=".json",
        delete=False,
    ) as temporary:
        json.dump(value, temporary, indent=2, ensure_ascii=False)
        temporary.write("\n")
        scratch = Path(temporary.name)
    try:
        scratch.replace(target)
    except BaseException:
        scratch.unlink(missing_ok=True)
        raise


def write_canvas_state(course_dir: Path, value: dict) -> None:
    """Atomically replace the Canvas-owned capture ledger."""
    _write_json(_state_path(course_dir, "canvas.json"), value)


def write_latest_run(course_dir: Path, value: dict) -> None:
    """Atomically replace the latest run manifest."""
    _write_json(_state_path(course_dir, "latest-run.json"), value)


@contextmanager
def course_sync_lock(course_dir: Path) -> Iterator[None]:
    """Hold the single Canvas sync lock for a course."""
    lock_path = _state_path(course_dir, ".sync.lock")
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    try:
        descriptor = os.open(lock_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError as error:
        raise RuntimeError(f"course is already locked for sync: {lock_path}") from error
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as lock:
            lock.write(f"pid={os.getpid()}\n")
        yield
    finally:
        lock_path.unlink(missing_ok=True)
