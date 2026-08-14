from __future__ import annotations

from dataclasses import dataclass

API_VERSION = "ael.dev/v1"


@dataclass(frozen=True)
class ToolVersion:
    name: str
    version: str
    commands: tuple[str, ...]
    environment_variable: str | None = None


TOOL_VERSIONS: tuple[ToolVersion, ...] = (
    ToolVersion("renode", "1.16.1", ("renode",), "RENODE_BIN"),
    ToolVersion("zephyr", "4.4.2", ("west",), "ZEPHYR_BASE"),
    ToolVersion("zephyr-sdk", "1.0.1", ("arm-zephyr-eabi-gcc",), "ZEPHYR_SDK_INSTALL_DIR"),
    ToolVersion("ngspice", "46", ("ngspice",), "NGSPICE_BIN"),
    ToolVersion("openmodelica", "1.27.0", ("omc",), "OPENMODELICA_BIN"),
    ToolVersion("omsimulator", "2.1.3", ("OMSimulator", "omsimulator"), "OMSIMULATOR_BIN"),
    ToolVersion("ns-3", "3.47", ("ns3",), "NS3_BIN"),
    ToolVersion("openems", "0.0.36", ("openEMS",), "OPENEMS_BIN"),
)
