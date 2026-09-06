from __future__ import annotations

import pytest
from pydantic import ValidationError

from corum.config import load_workspace
from corum.workspace import initialize


def test_initialize_creates_a_valid_empty_vault(tmp_path):
    vault = tmp_path / "vault"

    assert initialize(vault) == vault
    assert (vault / "courses").is_dir()
    assert load_workspace(vault).workspace.timezone == "Asia/Singapore"


def test_initialize_refuses_nonempty_target(tmp_path):
    vault = tmp_path / "vault"
    vault.mkdir()
    (vault / "existing.txt").write_text("keep me")

    with pytest.raises(ValueError, match="non-empty"):
        initialize(vault)


def test_initialize_validates_explicit_workspace_before_writing(tmp_path):
    vault = tmp_path / "vault"

    with pytest.raises(ValidationError):
        initialize(vault, {})

    assert not vault.exists()
