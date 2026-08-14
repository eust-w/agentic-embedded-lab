from __future__ import annotations

import os
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path


def resolve_workspace_path(
    workspace: Path,
    candidate: str | Path,
    *,
    must_exist: bool = False,
) -> Path:
    root = workspace.resolve()
    raw = Path(candidate)
    path = raw.resolve() if raw.is_absolute() else (root / raw).resolve()
    if path != root and root not in path.parents:
        raise ValueError(f"path escapes workspace: {candidate}")
    if must_exist and not path.exists():
        raise FileNotFoundError(path)
    return path


@dataclass(frozen=True)
class SandboxSpec:
    image: str
    command: tuple[str, ...]
    workspace: Path
    timeout_s: int = 300
    memory_mb: int = 1024
    cpus: float = 1.0
    network: bool = False


class SandboxUnavailable(RuntimeError):
    pass


def sandbox_command(spec: SandboxSpec) -> list[str]:
    runtime = shutil.which(os.environ.get("AEL_OCI_RUNTIME", "podman"))
    if runtime is None:
        runtime = shutil.which("docker")
    if runtime is None:
        raise SandboxUnavailable("no OCI runtime found; generated code will not run on the host")
    workspace = spec.workspace.resolve()
    command = [
        runtime,
        "run",
        "--rm",
        "--read-only",
        "--cap-drop=ALL",
        "--security-opt=no-new-privileges",
        f"--memory={spec.memory_mb}m",
        f"--cpus={spec.cpus}",
        "--pids-limit=256",
        "--tmpfs=/tmp:rw,noexec,nosuid,size=256m",
        f"--mount=type=bind,src={workspace},dst=/workspace,readonly",
        "--workdir=/workspace",
    ]
    if not spec.network:
        command.append("--network=none")
    command.extend([spec.image, *spec.command])
    return command


def run_sandbox(spec: SandboxSpec) -> subprocess.CompletedProcess[str]:
    command = sandbox_command(spec)
    return subprocess.run(
        command,
        capture_output=True,
        text=True,
        timeout=spec.timeout_s,
        check=False,
        env={"PATH": os.environ.get("PATH", "")},
    )
