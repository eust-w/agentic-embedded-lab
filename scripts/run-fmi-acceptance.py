#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import ctypes
import json
import platform
import subprocess
from pathlib import Path

from fmpy import read_model_description
from package_fmu import package

from ael.contracts import AcceptanceEntry, AcceptanceManifest, SystemManifest
from ael.fmi import FMI_PROXY_NAMES, FmiOrchestrator, export_ssp_package
from ael.io import load_document, sha256_file, write_json

# OMSimulator 2.1.3 resolves this complete FMI 2.0 Co-Simulation table even
# when a capability is declared false in modelDescription.xml.  Validate the
# binary before packaging so an absent optional-operation stub produces a
# precise acceptance error instead of the coordinator's generic instantiate
# failure.
FMI2_CS_EXPORTS = (
    "fmi2GetVersion",
    "fmi2GetTypesPlatform",
    "fmi2SetDebugLogging",
    "fmi2Instantiate",
    "fmi2FreeInstance",
    "fmi2SetupExperiment",
    "fmi2EnterInitializationMode",
    "fmi2ExitInitializationMode",
    "fmi2Terminate",
    "fmi2Reset",
    "fmi2GetReal",
    "fmi2SetReal",
    "fmi2GetInteger",
    "fmi2SetInteger",
    "fmi2GetBoolean",
    "fmi2SetBoolean",
    "fmi2GetString",
    "fmi2SetString",
    "fmi2GetFMUstate",
    "fmi2SetFMUstate",
    "fmi2FreeFMUstate",
    "fmi2SerializedFMUstateSize",
    "fmi2SerializeFMUstate",
    "fmi2DeSerializeFMUstate",
    "fmi2GetDirectionalDerivative",
    "fmi2SetRealInputDerivatives",
    "fmi2GetRealOutputDerivatives",
    "fmi2DoStep",
    "fmi2CancelStep",
    "fmi2GetStatus",
    "fmi2GetRealStatus",
    "fmi2GetIntegerStatus",
    "fmi2GetBooleanStatus",
    "fmi2GetStringStatus",
)


def library_suffix() -> str:
    return ".dylib" if platform.system() == "Darwin" else ".so"


def validate_proxy_exports(library: Path) -> None:
    proxy = ctypes.CDLL(str(library))
    missing = [name for name in FMI2_CS_EXPORTS if not hasattr(proxy, name)]
    if missing:
        raise RuntimeError(f"{library.name} is missing FMI 2.0 exports: {', '.join(missing)}")


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
        library = build_dir / f"{proxy}{library_suffix()}"
        validate_proxy_exports(library)
        ports = build_dir / f"{component.id}-ports.json"
        ports.write_text(
            json.dumps([port.model_dump(mode="json") for port in component.ports], indent=2),
            encoding="utf-8",
        )
        fmu = build_dir / f"{component.id}.fmu"
        package(proxy, library, ports, fmu)
        read_model_description(fmu, validate=True)
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
    with result.result_file.open(newline="", encoding="utf-8") as stream:
        rows = list(csv.DictReader(stream))
    numeric_outputs: dict[str, float] = {}
    for row in rows:
        for name, raw in row.items():
            if name.lower() in {"time", "time [s]"} or raw in {None, ""}:
                continue
            try:
                numeric_outputs[name] = float(raw)
            except ValueError:
                continue
    nonzero_outputs = {
        name: value for name, value in numeric_outputs.items() if abs(value) > 1e-15
    }
    if not nonzero_outputs:
        raise RuntimeError("FMI acceptance produced no non-zero traced output")
    units = {
        f"{component.id}.{port.name}": port.unit
        for component in system.components
        for port in component.ports
        if port.unit is not None
    }
    if not units:
        raise RuntimeError("FMI acceptance topology declares no units")
    evidence = {
        "status": "passed",
        "ssp": str(ssp.relative_to(workspace)),
        "ssp_sha256": sha256_file(ssp),
        "result": str(result.result_file.relative_to(workspace)),
        "result_sha256": sha256_file(result.result_file),
        "nonzero_outputs": nonzero_outputs,
        "port_units": units,
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
