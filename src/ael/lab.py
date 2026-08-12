from __future__ import annotations

import shutil
import subprocess
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from pydantic import Field

from .contracts import (
    CalibrationRecord,
    InstrumentEvidence,
    InstrumentOperationRequest,
    StrictModel,
    UnitValue,
)
from .io import load_document, sha256_file, write_json
from .security import resolve_workspace_path


class BoardTarget(StrictModel):
    id: str
    display_name: str
    architecture: str
    board_revision: str
    debug_backend: str
    openocd_interface: str | None = None
    openocd_target: str | None = None
    jlink_device: str | None = None
    samples_required: int = Field(default=3, ge=3)
    validation_status: str = "unverified"


class BoardCatalog(StrictModel):
    version: str
    boards: list[BoardTarget]


@dataclass(frozen=True)
class InstrumentDefinition:
    id: str
    resource: str
    kind: str
    driver: str


OPERATION_PARAMETERS: dict[str, dict[str, tuple[str, float | None, float | None]]] = {
    "power.configure": {
        "voltage": ("V", 0.0, 60.0),
        "current_limit": ("A", 0.0, 20.0),
    },
    "power.output": {"enabled": ("bool", None, None)},
    "power.measure": {},
    "scope.capture": {
        "duration": ("s", 1e-9, 3600.0),
        "sample_rate": ("Hz", 1.0, 20e9),
    },
    "logic.capture": {
        "duration": ("s", 1e-9, 3600.0),
        "sample_rate": ("Hz", 1.0, 2e9),
    },
    "energy.measure": {"duration": ("s", 1e-6, 86400.0)},
    "chamber.set_temperature": {"temperature": ("Cel", -20.0, 85.0)},
    "vna.sparameters": {
        "start_frequency": ("Hz", 1.0, 6e9),
        "stop_frequency": ("Hz", 1.0, 6e9),
        "points": ("int", 2.0, 100001.0),
    },
    "spectrum.capture": {
        "center_frequency": ("Hz", 1.0, 6e9),
        "span": ("Hz", 1.0, 6e9),
    },
    "attenuator.set": {"attenuation": ("dB", 0.0, 120.0)},
}


class InstrumentPolicy:
    def validate(self, request: InstrumentOperationRequest) -> dict[str, Any]:
        schema = OPERATION_PARAMETERS[request.operation]
        unknown = set(request.parameters) - set(schema)
        missing = set(schema) - set(request.parameters)
        if unknown:
            raise ValueError(f"unknown parameters for {request.operation}: {sorted(unknown)}")
        if missing:
            raise ValueError(f"missing parameters for {request.operation}: {sorted(missing)}")
        normalized: dict[str, Any] = {}
        for name, (expected_unit, minimum, maximum) in schema.items():
            value = request.parameters[name]
            if expected_unit == "bool":
                if not isinstance(value, bool):
                    raise ValueError(f"{name} must be boolean")
                normalized[name] = value
                continue
            if expected_unit == "int":
                if not isinstance(value, int):
                    raise ValueError(f"{name} must be integer")
                numeric = float(value)
            else:
                unit_value = (
                    value if isinstance(value, UnitValue) else UnitValue.model_validate(value)
                )
                if unit_value.unit != expected_unit:
                    raise ValueError(f"{name} requires unit {expected_unit}")
                numeric = unit_value.value
            if minimum is not None and numeric < minimum:
                raise ValueError(f"{name} is below the allowed range")
            if maximum is not None and numeric > maximum:
                raise ValueError(f"{name} is above the allowed range")
            normalized[name] = int(numeric) if expected_unit == "int" else numeric
        return normalized


class VisaInstrumentDriver:
    """Internal SCPI implementation; callers can only request allow-listed operations."""

    def __init__(self, definition: InstrumentDefinition) -> None:
        try:
            import pyvisa
        except ImportError as exception:
            raise RuntimeError("install AEL with the worker extra") from exception
        self.definition = definition
        self.resource = pyvisa.ResourceManager().open_resource(definition.resource)

    def execute(self, operation: str, parameters: dict[str, Any], output: Path) -> None:
        output.parent.mkdir(parents=True, exist_ok=True)
        if operation == "power.configure":
            self.resource.write(f"VOLT {parameters['voltage']:.9g}")
            self.resource.write(f"CURR {parameters['current_limit']:.9g}")
            response = self.resource.query("MEAS:VOLT?;:MEAS:CURR?")
        elif operation == "power.output":
            self.resource.write("OUTP ON" if parameters["enabled"] else "OUTP OFF")
            response = self.resource.query("OUTP?")
        elif operation == "power.measure":
            response = self.resource.query("MEAS:VOLT?;:MEAS:CURR?")
        elif operation == "chamber.set_temperature":
            self.resource.write(f"TEMP {parameters['temperature']:.9g}")
            response = self.resource.query("TEMP?")
        elif operation == "attenuator.set":
            self.resource.write(f"ATT {parameters['attenuation']:.9g}")
            response = self.resource.query("ATT?")
        elif operation == "scope.capture":
            self.resource.write(f"TIM:RANG {parameters['duration']:.9g}")
            self.resource.write(f"ACQ:SRAT {parameters['sample_rate']:.9g}")
            self.resource.write("DIG")
            self.resource.query("*OPC?")
            response = self.resource.query("WAV:DATA?")
        elif operation == "vna.sparameters":
            self.resource.write(f"SENS:FREQ:STAR {parameters['start_frequency']:.9g}")
            self.resource.write(f"SENS:FREQ:STOP {parameters['stop_frequency']:.9g}")
            self.resource.write(f"SENS:SWE:POIN {parameters['points']}")
            self.resource.write("INIT:IMM")
            self.resource.query("*OPC?")
            response = self.resource.query("CALC:DATA? SDATA")
        elif operation == "spectrum.capture":
            self.resource.write(f"FREQ:CENT {parameters['center_frequency']:.9g}")
            self.resource.write(f"FREQ:SPAN {parameters['span']:.9g}")
            self.resource.write("INIT:IMM")
            self.resource.query("*OPC?")
            response = self.resource.query("TRAC:DATA? TRACE1")
        else:
            raise NotImplementedError(
                f"driver {self.definition.driver} does not implement {operation}"
            )
        output.write_text(response.strip() + "\n", encoding="utf-8")


class SigrokLogicDriver:
    def __init__(self, definition: InstrumentDefinition) -> None:
        self.definition = definition

    def execute(self, operation: str, parameters: dict[str, Any], output: Path) -> None:
        if operation != "logic.capture":
            raise ValueError("sigrok driver only accepts logic.capture")
        executable = shutil.which("sigrok-cli")
        if executable is None:
            raise RuntimeError("sigrok-cli is not installed on this Lab Worker")
        output.parent.mkdir(parents=True, exist_ok=True)
        completed = subprocess.run(
            [
                executable,
                "--driver",
                self.definition.driver,
                "--config",
                f"samplerate={parameters['sample_rate']:.0f}",
                "--time",
                f"{parameters['duration']:.9g}",
                "--output-file",
                str(output),
            ],
            capture_output=True,
            text=True,
            timeout=max(30, int(parameters["duration"] + 10)),
            check=False,
        )
        if completed.returncode != 0:
            raise RuntimeError(f"sigrok capture failed: {completed.stderr[-2000:]}")


class JoulescopeDriver:
    def __init__(self, definition: InstrumentDefinition) -> None:
        self.definition = definition

    def execute(self, operation: str, parameters: dict[str, Any], output: Path) -> None:
        if operation != "energy.measure":
            raise ValueError("Joulescope driver only accepts energy.measure")
        try:
            from joulescope import scan
        except ImportError as exception:
            raise RuntimeError("install joulescope on the Lab Worker") from exception
        devices = scan(config="auto")
        if not devices:
            raise RuntimeError("no Joulescope device detected")
        device = devices[0]
        device.open()
        try:
            data = device.read(contiguous_duration=parameters["duration"])
        finally:
            device.close()
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_bytes(data.tobytes())


class LabController:
    def __init__(self, workspace: Path) -> None:
        self.workspace = workspace.resolve()
        self.policy = InstrumentPolicy()

    def execute(
        self,
        request: InstrumentOperationRequest,
        definition: InstrumentDefinition,
        calibration_path: Path,
        output_path: Path,
    ) -> InstrumentEvidence:
        if request.instrument_id != definition.id:
            raise PermissionError("instrument request does not match configured resource")
        calibration = load_document(calibration_path, CalibrationRecord, self.workspace)
        now = datetime.now(UTC)
        if calibration.instrument_id != definition.id:
            raise ValueError("calibration does not belong to the configured instrument")
        if not calibration.calibrated_at <= now <= calibration.expires_at:
            raise ValueError("instrument calibration is not current")
        certificate = resolve_workspace_path(
            self.workspace, calibration.certificate_path, must_exist=True
        )
        if sha256_file(certificate) != calibration.certificate_sha256:
            raise ValueError("calibration certificate digest mismatch")
        parameters = self.policy.validate(request)
        output = resolve_workspace_path(self.workspace, output_path)
        if definition.kind == "logic_analyzer":
            driver: Any = SigrokLogicDriver(definition)
        elif definition.kind == "power_analyzer":
            driver = JoulescopeDriver(definition)
        else:
            driver = VisaInstrumentDriver(definition)
        driver.execute(request.operation, parameters, output)
        evidence = InstrumentEvidence(
            instrument_id=definition.id,
            driver=definition.driver,
            calibration_id=calibration.id,
            operation=request.operation,
            parameters=request.parameters,
            raw_artifact_path=str(output.relative_to(self.workspace)),
            raw_artifact_sha256=sha256_file(output),
            captured_at=now,
        )
        write_json(output.with_suffix(output.suffix + ".evidence.json"), evidence)
        return evidence


class BoardDebugger:
    def __init__(self, workspace: Path) -> None:
        self.workspace = workspace.resolve()

    def flash(self, target: BoardTarget, firmware: Path) -> subprocess.CompletedProcess[str]:
        image = resolve_workspace_path(self.workspace, firmware, must_exist=True)
        if target.debug_backend == "openocd":
            executable = shutil.which("openocd")
            if executable is None or not target.openocd_interface or not target.openocd_target:
                raise RuntimeError("OpenOCD target is not completely configured")
            command = [
                executable,
                "-f",
                target.openocd_interface,
                "-f",
                target.openocd_target,
                "-c",
                f"program {image} verify reset exit",
            ]
        elif target.debug_backend == "jlink":
            executable = shutil.which("JLinkExe")
            if executable is None or not target.jlink_device:
                raise RuntimeError("J-Link target is not completely configured")
            script = self.workspace / ".ael" / "jlink" / f"{target.id}.jlink"
            script.parent.mkdir(parents=True, exist_ok=True)
            script.write_text(f"loadfile {image}\nr\ng\nexit\n", encoding="utf-8")
            command = [
                executable,
                "-device",
                target.jlink_device,
                "-if",
                "SWD",
                "-speed",
                "4000",
                "-CommanderScript",
                str(script),
            ]
        else:
            raise ValueError(f"unsupported debugger backend: {target.debug_backend}")
        return subprocess.run(command, capture_output=True, text=True, timeout=120, check=False)


def load_board_catalog(workspace: Path) -> BoardCatalog:
    return load_document(workspace / "lab/boards.yaml", BoardCatalog, workspace)
