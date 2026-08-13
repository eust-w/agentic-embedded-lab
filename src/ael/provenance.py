from __future__ import annotations

import platform
from pathlib import Path

AUTHORITATIVE_SIMULATION_PLATFORM = "Ubuntu 24.04 x86_64"


def detect_platform() -> str:
    """Return a truthful label for the machine producing acceptance evidence."""

    machine = platform.machine() or "unknown"
    if platform.system() == "Linux":
        return f"{_linux_release()} {machine}"
    if platform.system() == "Darwin":
        return f"macOS {platform.mac_ver()[0] or 'unknown'} {machine}"
    return f"{platform.system() or 'unknown'} {platform.release() or 'unknown'} {machine}"


def _linux_release() -> str:
    os_release = Path("/etc/os-release")
    if not os_release.is_file():
        return "Linux"
    values: dict[str, str] = {}
    for raw_line in os_release.read_text(encoding="utf-8").splitlines():
        if "=" not in raw_line or raw_line.startswith("#"):
            continue
        key, value = raw_line.split("=", 1)
        values[key] = value.strip().strip('"')
    name = values.get("NAME", "Linux")
    version = values.get("VERSION_ID", values.get("VERSION", ""))
    return f"{name} {version}".strip()
