#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import platform
import shutil
import uuid
import zipfile
from pathlib import Path
from xml.etree import ElementTree


def platform_directory() -> str:
    machine = platform.machine().lower()
    system = platform.system().lower()
    if system == "linux" and machine in {"x86_64", "amd64"}:
        return "linux64"
    if system == "darwin" and machine in {"arm64", "aarch64"}:
        return "darwin64"
    raise RuntimeError(f"unsupported FMU build platform: {system}/{machine}")


def scalar_variable(parent, port: dict[str, object], reference: int) -> None:
    causality = "input" if port["direction"] == "input" else "output"
    variable = ElementTree.SubElement(
        parent,
        "ScalarVariable",
        name=str(port["name"]),
        valueReference=str(reference),
        causality=causality,
        variability="continuous" if port["data_type"] == "real" else "discrete",
    )
    type_names = {
        "real": "Real",
        "integer": "Integer",
        "boolean": "Boolean",
        "string": "String",
    }
    type_node = ElementTree.SubElement(variable, type_names[str(port["data_type"])])
    if port.get("unit"):
        type_node.set("unit", str(port["unit"]))


def package(proxy: str, library: Path, ports_path: Path, destination: Path) -> None:
    ports = json.loads(ports_path.read_text(encoding="utf-8"))
    guid = "{" + str(uuid.uuid5(uuid.NAMESPACE_URL, f"ael.dev/fmu/{proxy}")) + "}"
    root = ElementTree.Element(
        "fmiModelDescription",
        fmiVersion="2.0",
        modelName=proxy,
        guid=guid,
        generationTool="Agentic Embedded Lab",
        variableNamingConvention="structured",
        numberOfEventIndicators="0",
    )
    ElementTree.SubElement(
        root,
        "CoSimulation",
        modelIdentifier=proxy,
        canHandleVariableCommunicationStepSize="true",
        canGetAndSetFMUstate=(
            "false" if proxy in {"RenodeFmu", "Ns3Fmu", "OpenEmsFmu"} else "true"
        ),
        canSerializeFMUstate="false",
    )
    declared_units = sorted(
        {str(port["unit"]) for port in ports if port.get("unit") and port["unit"] != "1"}
    )
    if declared_units:
        definitions = ElementTree.SubElement(root, "UnitDefinitions")
        for unit in declared_units:
            ElementTree.SubElement(definitions, "Unit", name=unit)
    variables = ElementTree.SubElement(root, "ModelVariables")
    for reference, port in enumerate(ports, start=1):
        scalar_variable(variables, port, reference)
    structure = ElementTree.SubElement(root, "ModelStructure")
    outputs = ElementTree.SubElement(structure, "Outputs")
    for index, port in enumerate(ports, start=1):
        if port["direction"] == "output":
            ElementTree.SubElement(outputs, "Unknown", index=str(index))

    staging = destination.parent / f".{destination.stem}-staging"
    if staging.exists():
        shutil.rmtree(staging)
    binary_directory = staging / "binaries" / platform_directory()
    binary_directory.mkdir(parents=True)
    extension = library.suffix
    shutil.copy2(library, binary_directory / f"{proxy}{extension}")
    ElementTree.ElementTree(root).write(
        staging / "modelDescription.xml", encoding="utf-8", xml_declaration=True
    )
    destination.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(destination, "w", zipfile.ZIP_DEFLATED) as archive:
        for path in sorted(staging.rglob("*")):
            if path.is_file():
                archive.write(path, path.relative_to(staging))
    shutil.rmtree(staging)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--proxy", required=True)
    parser.add_argument("--library", type=Path, required=True)
    parser.add_argument("--ports", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    arguments = parser.parse_args()
    package(arguments.proxy, arguments.library, arguments.ports, arguments.output)


if __name__ == "__main__":
    main()
