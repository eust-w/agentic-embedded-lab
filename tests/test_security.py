from __future__ import annotations

from pathlib import Path

import pytest

from ael.security import SandboxSpec, resolve_workspace_path, sandbox_command


def test_workspace_path_rejects_symlink_escape(tmp_path: Path) -> None:
    outside = tmp_path.parent / "outside-secret"
    outside.write_text("secret", encoding="utf-8")
    link = tmp_path / "link"
    link.symlink_to(outside)
    with pytest.raises(ValueError, match="escapes workspace"):
        resolve_workspace_path(tmp_path, link, must_exist=True)


def test_sandbox_is_no_network_read_only_and_device_free(tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setattr("ael.security.shutil.which", lambda _: "/usr/bin/podman")
    command = sandbox_command(
        SandboxSpec(
            image="model-validator@sha256:" + "a" * 64,
            command=("validate",),
            workspace=tmp_path,
        )
    )
    assert "--network=none" in command
    assert "--read-only" in command
    assert "--cap-drop=ALL" in command
    assert all("--device" not in item for item in command)
