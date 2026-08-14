from __future__ import annotations

import json
import os
import platform
import shutil
import subprocess
from pathlib import Path
from typing import Any

RELEASE_AUTHORITY_POLICY = "qualified-execution-evidence"

BACKEND_IMAGE_ENVIRONMENTS = {
    "zephyr_build": "AEL_ZEPHYR_BUILD_IMAGE",
    "renode": "AEL_RENODE_IMAGE",
    "ngspice": "AEL_NGSPICE_IMAGE",
    "openmodelica": "AEL_OPENMODELICA_IMAGE",
    "omsimulator": "AEL_OMSIMULATOR_IMAGE",
    "ns3": "AEL_NS3_IMAGE",
    "openems": "AEL_OPENEMS_IMAGE",
}


def detect_platform() -> str:
    """Return a truthful label for the machine producing acceptance evidence."""

    machine = platform.machine() or "unknown"
    if platform.system() == "Linux":
        return f"{_linux_release()} {machine}"
    if platform.system() == "Darwin":
        return f"macOS {platform.mac_ver()[0] or 'unknown'} {machine}"
    return f"{platform.system() or 'unknown'} {platform.release() or 'unknown'} {machine}"


def capture_execution_environment(
    probes: list[dict[str, Any]], *, source_revision: str
) -> dict[str, Any]:
    """Capture the execution boundary that is allowed to issue software evidence.

    GitHub Actions is deliberately not special. A native Linux installation or a
    pinned Linux OCI image may qualify when every required backend reports its
    expected version. macOS-hosted acceptance therefore remains valid when the
    actual simulators and build tools execute inside inspected Linux images.
    """

    runtime_name = os.environ.get("AEL_OCI_RUNTIME", "podman")
    runtime = shutil.which(runtime_name) or shutil.which("docker")
    targets: dict[str, dict[str, Any]] = {}
    failures: list[str] = []
    for probe in probes:
        backend = str(probe["backend"])
        if not probe.get("available"):
            failures.append(f"{backend}: unavailable: {probe.get('reason') or 'unknown reason'}")
        image_variable = BACKEND_IMAGE_ENVIRONMENTS.get(backend)
        image = os.environ.get(image_variable, "") if image_variable else ""
        if image:
            if runtime is None:
                failures.append(f"{backend}: OCI image configured but runtime is unavailable")
                continue
            inspected = _inspect_image(runtime, image)
            if inspected is None:
                failures.append(f"{backend}: cannot inspect OCI image {image!r}")
                continue
            target = {
                "mode": "oci",
                "image": image,
                "image_id": inspected.get("Id"),
                "repo_digests": inspected.get("RepoDigests") or [],
                "os": inspected.get("Os"),
                "architecture": inspected.get("Architecture"),
            }
            targets[backend] = target
            if target["os"] != "linux" or not target["image_id"]:
                failures.append(f"{backend}: inspected image is not an identified Linux image")
        elif platform.system() == "Linux":
            targets[backend] = {
                "mode": "native-linux",
                "os": "linux",
                "architecture": platform.machine() or "unknown",
            }
        else:
            failures.append(
                f"{backend}: non-Linux host execution requires an inspected Linux OCI image"
            )
    if not source_revision or source_revision == "working-tree":
        failures.append("source revision is not immutable")
    return {
        "policy": RELEASE_AUTHORITY_POLICY,
        "qualified": not failures,
        "ci_required": False,
        "source_revision": source_revision,
        "control_platform": detect_platform(),
        "backend_probes": probes,
        "execution_targets": targets,
        "failures": failures,
        "hardware_validated": False,
    }


def validate_execution_environment(payload: object, source_revision: str) -> list[str]:
    if not isinstance(payload, dict):
        return ["execution environment evidence is not an object"]
    if isinstance(payload.get("checks"), dict):
        payload = payload["checks"]
    failures: list[str] = []
    if payload.get("policy") != RELEASE_AUTHORITY_POLICY:
        failures.append("execution environment uses an unsupported authority policy")
    if payload.get("qualified") is not True:
        failures.append("execution environment is not qualified")
    if payload.get("ci_required") is not False:
        failures.append("execution environment incorrectly requires a CI provider")
    if payload.get("source_revision") != source_revision:
        failures.append("execution environment source revision does not match acceptance")
    if payload.get("hardware_validated") is not False:
        failures.append("software execution evidence must not claim hardware validation")
    targets = payload.get("execution_targets")
    if not isinstance(targets, dict) or not targets:
        failures.append("execution environment has no qualified backend targets")
    elif any(
        not isinstance(target, dict) or target.get("os") != "linux"
        for target in targets.values()
    ):
        failures.append("all qualified backend targets must execute on Linux")
    return failures


def _inspect_image(runtime: str, image: str) -> dict[str, Any] | None:
    try:
        result = subprocess.run(
            [runtime, "image", "inspect", image],
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0:
        return None
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError:
        return None
    return payload[0] if isinstance(payload, list) and payload else None


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
