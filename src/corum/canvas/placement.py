"""Map Canvas file folders to course raw-material paths."""

from __future__ import annotations

from .client import CanvasClient


ROOT_PREFIX = "course files/"


async def folder_paths(client: CanvasClient, course_id: int) -> dict[int, str]:
    """Return Canvas folder IDs mapped to paths relative to the file root."""
    folders = await client.get_all(f"/courses/{course_id}/folders")
    paths: dict[int, str] = {}
    for folder in folders:
        full_name = folder.get("full_name") or ""
        paths[folder["id"]] = full_name.removeprefix(ROOT_PREFIX)
    return paths


def place(folder_path: str | None, filename: str, folders: dict[str, str]) -> str:
    """Return the raw-material path, preferring the longest configured mapping."""
    if not folder_path:
        return filename
    for source in sorted(folders, key=len, reverse=True):
        if folder_path == source or folder_path.startswith(f"{source}/"):
            return f"{folders[source]}/{filename}"
    return f"{folder_path}/{filename}"
