"""Machine-owned course state and per-course locking."""

from __future__ import annotations

from contextlib import contextmanager
import json
import os
from pathlib import Path
from pathlib import PurePosixPath
import tempfile
from typing import Iterator


_CANVAS_SOURCES = {"announcements", "assignments", "files", "pages", "modules", "syllabus"}


def _state_path(course_dir: Path, name: str) -> Path:
    resolved_course = course_dir.resolve()
    state_dir = (course_dir / "state").resolve()
    if not state_dir.is_relative_to(resolved_course):
        raise ValueError(f"state directory resolves outside course: {course_dir / 'state'}")
    return state_dir / name


def _empty_canvas_state(watched_sources: list[str] | tuple[str, ...]) -> dict:
    unknown = sorted(set(watched_sources) - _CANVAS_SOURCES)
    if unknown:
        raise ValueError(f"unknown watched Canvas source(s): {', '.join(unknown)}")
    return {
        "schema": 1,
        "synced_at": None,
        "sources": {
            source: None if source == "syllabus" else {}
            for source in watched_sources
        },
    }


def _validate_canvas_state(value: object) -> dict:
    if not isinstance(value, dict) or set(value) != {"schema", "synced_at", "sources"}:
        raise ValueError("canvas.json must contain only schema, synced_at, and sources")
    if value["schema"] != 1 or isinstance(value["schema"], bool):
        raise ValueError("canvas.json.schema must be 1")
    if value["synced_at"] is not None and not isinstance(value["synced_at"], str):
        raise ValueError("canvas.json.synced_at must be a string or null")
    sources = value["sources"]
    if not isinstance(sources, dict) or set(sources) - _CANVAS_SOURCES:
        raise ValueError("canvas.json.sources contains an invalid source")
    for source, entries in sources.items():
        if source == "syllabus":
            if entries is not None and not isinstance(entries, str):
                raise ValueError("canvas.json.sources.syllabus must be a string or null")
        elif not isinstance(entries, dict) or any(
            not isinstance(key, str) or item is not None and not isinstance(item, str)
            for key, item in entries.items()
        ):
            raise ValueError(f"canvas.json.sources.{source} must be a string ledger")
    return value


def read_canvas_state(
    course_dir: Path,
    watched_sources: list[str] | tuple[str, ...] = (),
) -> dict:
    """Read the Canvas-owned capture ledger for one course."""
    target = _state_path(course_dir, "canvas.json")
    try:
        value = json.loads(target.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return _empty_canvas_state(watched_sources)
    except json.JSONDecodeError as error:
        raise ValueError(f"could not parse {target}: {error}") from error
    return _validate_canvas_state(value)


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


def read_latest_run(course_dir: Path) -> dict | None:
    """Read and validate the latest run manifest when one exists."""
    target = _state_path(course_dir, "latest-run.json")
    try:
        raw = json.loads(target.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return None
    except json.JSONDecodeError as error:
        raise ValueError(f"could not parse {target}: {error}") from error
    from corum.run import RunManifest

    return RunManifest.model_validate(raw).model_dump(mode="json")


def update_latest_run_stage(course_dir: Path, stage: str, value: dict) -> bool:
    """Atomically replace one stage of an existing validated run manifest."""
    if stage not in {"jira", "wiki"}:
        raise ValueError(f"invalid run stage: {stage}")
    raw = read_latest_run(course_dir)
    if raw is None:
        return False
    from corum.run import RunManifest, StageResult

    stage_result = StageResult.model_validate(value)
    manifest = RunManifest.model_validate(raw).model_copy(update={stage: stage_result})
    write_latest_run(course_dir, manifest.model_dump(mode="json"))
    return True


def latest_stage_requires_reconciliation(course_dir: Path, stage: str) -> bool:
    """Return a persisted stage retry barrier from the latest run."""
    raw = read_latest_run(course_dir)
    return bool(raw and raw[stage].get("reconciliation_required"))


def _validate_wiki_state(value: object) -> dict:
    if not isinstance(value, dict) or set(value) != {"schema", "ingested"}:
        raise ValueError("wiki.json must contain only schema and ingested")
    if value["schema"] != 1 or isinstance(value["schema"], bool):
        raise ValueError("wiki.json.schema must be 1")
    if not isinstance(value["ingested"], dict):
        raise ValueError("wiki.json.ingested must be an object")
    normalized: dict[str, str | None] = {}
    for relative, provenance in value["ingested"].items():
        path = PurePosixPath(relative) if isinstance(relative, str) else None
        if (
            path is None
            or not relative
            or path.is_absolute()
            or ".." in path.parts
        ):
            raise ValueError(f"invalid wiki source path: {relative!r}")
        if provenance is not None and (
            not isinstance(provenance, str) or not provenance.strip()
        ):
            raise ValueError(
                f"{relative} must have a non-empty provenance label or null"
            )
        normalized[relative] = provenance
    return {"schema": 1, "ingested": normalized}


def _json_scratch(target: Path, value: dict) -> Path:
    target.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        "w",
        encoding="utf-8",
        dir=target.parent,
        prefix=f".{target.stem}-transaction-",
        suffix=".json",
        delete=False,
    ) as temporary:
        json.dump(value, temporary, indent=2, ensure_ascii=False)
        temporary.write("\n")
        temporary.flush()
        os.fsync(temporary.fileno())
        return Path(temporary.name)


def _restore_bytes(target: Path, original: bytes | None) -> None:
    if original is None:
        target.unlink(missing_ok=True)
        return
    with tempfile.NamedTemporaryFile(
        "wb",
        dir=target.parent,
        prefix=f".{target.stem}-rollback-",
        suffix=".json",
        delete=False,
    ) as temporary:
        temporary.write(original)
        temporary.flush()
        os.fsync(temporary.fileno())
        scratch = Path(temporary.name)
    try:
        os.replace(scratch, target)
    except BaseException:
        scratch.unlink(missing_ok=True)
        raise


def _replace_json_pair(
    first_target: Path,
    first_value: dict,
    second_target: Path,
    second_value: dict,
) -> None:
    originals = {
        first_target: first_target.read_bytes() if first_target.is_file() else None,
        second_target: second_target.read_bytes() if second_target.is_file() else None,
    }
    first_scratch = _json_scratch(first_target, first_value)
    second_scratch = _json_scratch(second_target, second_value)
    committed: list[Path] = []
    try:
        os.replace(first_scratch, first_target)
        committed.append(first_target)
        os.replace(second_scratch, second_target)
        committed.append(second_target)
    except BaseException:
        rollback_error: BaseException | None = None
        for target in reversed(committed):
            try:
                _restore_bytes(target, originals[target])
            except BaseException as error:
                rollback_error = error
        if rollback_error is not None:
            raise RuntimeError(
                f"wiki finalization failed and rollback also failed: {rollback_error}"
            ) from rollback_error
        raise
    finally:
        first_scratch.unlink(missing_ok=True)
        second_scratch.unlink(missing_ok=True)


def finalize_wiki_state(
    course_dir: Path,
    *,
    expected_run_id: str,
    ingested: dict[str, str | None],
    stage: dict,
) -> None:
    """Validate and transactionally commit wiki finalization plus its run stage."""
    from corum.run import RunManifest, StageResult

    state_dir = _state_path(course_dir, "wiki.json").parent
    lock_path = state_dir / ".wiki-finalize.lock"
    state_dir.mkdir(parents=True, exist_ok=True)
    try:
        descriptor = os.open(lock_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError as error:
        raise RuntimeError(f"wiki finalization is already locked: {lock_path}") from error
    try:
        os.close(descriptor)
        wiki_target = _state_path(course_dir, "wiki.json")
        latest_target = _state_path(course_dir, "latest-run.json")
        if wiki_target.is_file():
            try:
                current_wiki = _validate_wiki_state(
                    json.loads(wiki_target.read_text(encoding="utf-8"))
                )
            except json.JSONDecodeError as error:
                raise ValueError(f"could not parse {wiki_target}: {error}") from error
        else:
            current_wiki = {"schema": 1, "ingested": {}}
        pending = _validate_wiki_state({"schema": 1, "ingested": ingested})
        latest = read_latest_run(course_dir)
        if latest is None:
            raise ValueError("latest-run.json is required for wiki finalization")
        manifest = RunManifest.model_validate(latest)
        if manifest.run_id != expected_run_id:
            raise ValueError(
                f"finalization run_id {expected_run_id!r} does not match latest run "
                f"{manifest.run_id!r}"
            )
        if manifest.course != course_dir.name:
            raise ValueError("latest run course does not match selected course")
        if not manifest.effective_features["wiki"]:
            raise ValueError("wiki is disabled in the selected run manifest")
        stage_result = StageResult.model_validate(stage)
        merged_wiki = {
            "schema": 1,
            "ingested": {
                **current_wiki["ingested"],
                **pending["ingested"],
            },
        }
        updated_manifest = manifest.model_copy(update={"wiki": stage_result})
        if not pending["ingested"]:
            _write_json(latest_target, updated_manifest.model_dump(mode="json"))
            return
        _replace_json_pair(
            wiki_target,
            merged_wiki,
            latest_target,
            updated_manifest.model_dump(mode="json"),
        )
    finally:
        lock_path.unlink(missing_ok=True)


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
