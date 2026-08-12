#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import platform
import subprocess
from pathlib import Path

from package_fmu import package

from ael.contracts import AcceptanceEntry, AcceptanceManifest, SystemManifest
from ael.fmi import FMI_PROXY_NAMES, FmiOrchestrator, export_ssp_package
from ael.io import load_document, sha256_file, write_json


def library_suffix() -> str:
    return ".dylib" if platform.system() == "Darwin" else ".so"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--workspace", type=Path, default=Path.cwd())
    parser.add_argument("--system", type=Path, default=Path("benchmarks/systems/five-domain.yaml"))
    parser.add_argument("--build-dir", type=Path, default=Path(".ael/build/fmi"))
    parser.add_argument("--om-simulator", default="OMSimulator")
    arguments = parser.parse_args()
    workspace = arguments.workspace.resolve()
    system = load_document(arguments.system, SystemManifest, workspace)
    build_dir = (workspace / arguments.build_dir).resolve()
    subprocess.run(
        ["cmake", "-S", str(workspace / "native/fmi-proxies"), "-B", str(build_dir)],
        check=True,
    )
    subprocess.run(["cmake", "--build", str(build_dir), "--parallel"], check=True)
    fmus: dict[str, Path] = {}
    for component in system.components:
        proxy = FMI_PROXY_NAMES[component.backend]
        ports = build_dir / f"{component.id}-ports.json"
        ports.write_text(
            json.dumps([port.model_dump(mode="json") for port in component.ports], indent=2),
            encoding="utf-8",
        )
        fmu = build_dir / f"{component.id}.fmu"
        package(proxy, build_dir / f"{proxy}{library_suffix()}", ports, fmu)
        fmus[component.id] = fmu
    ssp = export_ssp_package(system, build_dir / "five-domain.ssp", fmus)
    result = FmiOrchestrator().run(
        system,
        ssp,
        stop_time_s=0.006,
        timeout_s=1800,
        seed=1024,
        omsimulator=arguments.om_simulator,
    )
    evidence = {
        "status": "passed",
        "ssp": str(ssp.relative_to(workspace)),
        "ssp_sha256": sha256_file(ssp),
        "result": str(result.result_file.relative_to(workspace)),
        "result_sha256": sha256_file(result.result_file),
        "hardware_validated": False,
        "limitations": ["FMI functional exchange only; no calibrated equivalence."],
    }
    output = workspace / "acceptance/evidence/fmi-five-domain.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    manifest_path = workspace / "acceptance/simulation.json"
    manifest = load_document(manifest_path, AcceptanceManifest, workspace)
    entries = [entry for entry in manifest.entries if entry.name != "fmi:five-domain"]
    entries.append(
        AcceptanceEntry(
            name="fmi:five-domain",
            status="passed",
            evidence_path=str(output.relative_to(workspace)),
            evidence_sha256=sha256_file(output),
            limitations=["FMI functional exchange only; no calibrated equivalence."],
        )
    )
    write_json(manifest_path, manifest.model_copy(update={"entries": entries}))


if __name__ == "__main__":
    main()
