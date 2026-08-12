from __future__ import annotations

import math
import os
import socketserver
import subprocess
import tempfile
import threading
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from xml.etree import ElementTree

from .adapters import Adapter, AdapterCatalog
from .contracts import BackendName, SystemManifest

FMI_PROXY_NAMES = {
    BackendName.RENODE: "RenodeFmu",
    BackendName.NGSPICE: "NgspiceFmu",
    BackendName.MODELICA: "ModelicaFmu",
    BackendName.NS3: "Ns3Fmu",
    BackendName.OPENEMS: "OpenEmsFmu",
}


@dataclass(frozen=True)
class MultiRateSchedule:
    communication_step_us: int
    component_steps_us: dict[str, int | None]
    event_driven: set[str]
    non_rollback: set[str]


def build_schedule(system: SystemManifest, macro_step_us: int) -> MultiRateSchedule:
    periodic = [component.step_us for component in system.components if component.step_us]
    communication_step = math.gcd(macro_step_us, *periodic)
    return MultiRateSchedule(
        communication_step_us=communication_step,
        component_steps_us={component.id: component.step_us for component in system.components},
        event_driven={component.id for component in system.components if component.event_driven},
        non_rollback={
            component.id
            for component in system.components
            if component.backend in {BackendName.RENODE, BackendName.NS3, BackendName.OPENEMS}
        },
    )


def validate_fmi_topology(system: SystemManifest) -> list[str]:
    warnings: list[str] = []
    component_by_id = {component.id: component for component in system.components}
    graph: dict[str, set[str]] = {component.id: set() for component in system.components}
    for connection in system.connections:
        source_id = connection.source.split(".", 1)[0]
        target_id = connection.target.split(".", 1)[0]
        if component_by_id[source_id].direct_feedthrough:
            graph[source_id].add(target_id)

    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(node: str) -> None:
        if node in visiting:
            raise ValueError("unsolved direct-feedthrough algebraic loop detected")
        if node in visited:
            return
        visiting.add(node)
        for target in graph[node]:
            visit(target)
        visiting.remove(node)
        visited.add(node)

    for component_id in graph:
        visit(component_id)
    if any(component.backend == BackendName.OPENEMS for component in system.components):
        warnings.append(
            "openEMS is batch/event driven and does not participate in millisecond lockstep"
        )
    return warnings


def export_ssp(system: SystemManifest, destination: Path) -> Path:
    validate_fmi_topology(system)
    namespace = "http://ssp-standard.org/SSP1/SystemStructureDescription"
    root = ElementTree.Element(
        "SystemStructureDescription",
        attrib={"xmlns": namespace, "version": "1.0", "name": system.name},
    )
    system_node = ElementTree.SubElement(root, "System", name=system.name)
    elements = ElementTree.SubElement(system_node, "Elements")
    for component in system.components:
        proxy = FMI_PROXY_NAMES.get(component.backend, component.backend.value)
        component_node = ElementTree.SubElement(
            elements,
            "Component",
            name=component.id,
            source=component.model or f"ael-proxy://{proxy}",
        )
        connector_node = ElementTree.SubElement(component_node, "Connectors")
        for port in component.ports:
            connector = ElementTree.SubElement(
                connector_node,
                "Connector",
                name=port.name,
                kind="inout" if port.direction == "bidirectional" else port.direction,
            )
            type_name = {
                "real": "Real",
                "integer": "Integer",
                "boolean": "Boolean",
                "string": "String",
            }.get(port.data_type)
            if type_name is None:
                raise ValueError(f"SSP/FMI does not support port type: {port.data_type}")
            attributes = {"unit": port.unit} if port.unit and port.unit != "1" else {}
            ElementTree.SubElement(connector, type_name, **attributes)
    connections = ElementTree.SubElement(system_node, "Connections")
    for connection in system.connections:
        source_element, source_connector = connection.source.split(".", 1)
        target_element, target_connector = connection.target.split(".", 1)
        ElementTree.SubElement(
            connections,
            "Connection",
            startElement=source_element,
            startConnector=source_connector,
            endElement=target_element,
            endConnector=target_connector,
        )
    destination.parent.mkdir(parents=True, exist_ok=True)
    ElementTree.ElementTree(root).write(destination, encoding="utf-8", xml_declaration=True)
    return destination


def export_ssp_package(system: SystemManifest, destination: Path, fmus: dict[str, Path]) -> Path:
    with tempfile.TemporaryDirectory(prefix="ael-ssp-") as temporary:
        root = Path(temporary)
        resources = root / "resources"
        resources.mkdir()
        rewritten = system.model_copy(deep=True)
        for component in rewritten.components:
            fmu = fmus.get(component.id)
            if fmu is None:
                raise ValueError(f"missing FMU for component {component.id}")
            target = resources / f"{component.id}.fmu"
            target.write_bytes(fmu.read_bytes())
            component.model = f"resources/{target.name}"
        export_ssp(rewritten, root / "SystemStructure.ssd")
        destination.parent.mkdir(parents=True, exist_ok=True)
        with zipfile.ZipFile(destination, "w", zipfile.ZIP_DEFLATED) as archive:
            for path in sorted(root.rglob("*")):
                if path.is_file():
                    archive.write(path, path.relative_to(root))
    return destination


@dataclass(frozen=True)
class FmiRunResult:
    result_file: Path
    log_file: Path
    return_code: int


class _FmiRequestHandler(socketserver.StreamRequestHandler):
    def handle(self) -> None:
        server: FmiBridgeServer = self.server.bridge  # type: ignore[attr-defined]
        request = self.rfile.readline().decode().strip()
        try:
            response = server.exchange(request)
        except Exception as exception:
            response = f"ERROR {type(exception).__name__}:{exception}"
        self.wfile.write((response + "\n").encode())


class FmiBridgeServer:
    def __init__(
        self,
        backend: BackendName,
        components: list[Any],
        socket_path: Path,
        catalog: AdapterCatalog,
        seed: int,
    ) -> None:
        self.backend = backend
        self.components = {component.id: component for component in components}
        self.socket_path = socket_path
        self.adapters: dict[str, Adapter] = {}
        for component in components:
            adapter = catalog.create(backend)
            adapter.prepare(component, seed)
            self.adapters[component.id] = adapter
        self.server = socketserver.ThreadingUnixStreamServer(str(socket_path), _FmiRequestHandler)
        self.server.bridge = self  # type: ignore[attr-defined]
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

    def start(self) -> None:
        self.thread.start()

    def exchange(self, request: str) -> str:
        fields = request.split()
        if len(fields) < 4 or fields[0] != "STEP":
            raise ValueError("invalid FMI bridge request")
        component_id = fields[1]
        current_us = round(float(fields[2]) * 1_000_000)
        step_us = round(float(fields[3]) * 1_000_000)
        component = self.components[component_id]
        adapter = self.adapters[component_id]
        ports = {index: port for index, port in enumerate(component.ports, start=1)}
        for assignment in fields[4:]:
            reference_text, value_text = assignment.split("=", 1)
            reference = int(reference_text[1:])
            port = ports.get(reference)
            if port and port.direction in {"input", "bidirectional"}:
                value: Any
                if reference_text.startswith("r"):
                    value = float(value_text)
                elif reference_text.startswith("i"):
                    value = int(value_text)
                elif reference_text.startswith("b"):
                    value = bool(int(value_text))
                else:
                    raise ValueError("unsupported FMI assignment type")
                adapter.inject(f"{component_id}.{port.name}", value, current_us)
        result = adapter.step(current_us, step_us)
        values: list[str] = []
        for reference, port in ports.items():
            if port.direction not in {"output", "bidirectional"}:
                continue
            if port.name in result.outputs:
                prefix = {"real": "r", "integer": "i", "boolean": "b"}.get(port.data_type)
                if prefix is None:
                    raise ValueError(f"unsupported FMI port type: {port.data_type}")
                value = result.outputs[port.name]
                if port.data_type == "boolean":
                    value = int(bool(value))
                values.append(f"{prefix}{reference}={value}")
        return "OK" + (" " + " ".join(values) if values else "")

    def close(self) -> None:
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        for adapter in self.adapters.values():
            adapter.shutdown()


class FmiOrchestrator:
    def __init__(self, catalog: AdapterCatalog | None = None) -> None:
        self.catalog = catalog or AdapterCatalog()

    def run(
        self,
        system: SystemManifest,
        ssp: Path,
        *,
        stop_time_s: float,
        timeout_s: int,
        seed: int,
        omsimulator: str = "OMSimulator",
    ) -> FmiRunResult:
        validate_fmi_topology(system)
        servers: list[FmiBridgeServer] = []
        # Keep Unix sockets under the workspace so an isolated OMSimulator
        # container can access them through the scoped workspace mount. Never
        # expose the host-wide /tmp directory to the co-simulation process.
        runtime_root = Path.cwd().resolve() / ".ael" / "runtime"
        runtime_root.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix="ael-fmi-", dir=runtime_root) as temporary:
            root = Path(temporary)
            environment = os.environ.copy()
            grouped: dict[BackendName, list[Any]] = {}
            for component in system.components:
                if component.backend in FMI_PROXY_NAMES:
                    grouped.setdefault(component.backend, []).append(component)
            try:
                for backend, components in grouped.items():
                    socket_path = root / f"{backend.value}.sock"
                    server = FmiBridgeServer(backend, components, socket_path, self.catalog, seed)
                    server.start()
                    servers.append(server)
                    variable = f"AEL_FMI_SOCKET_{backend.value.upper().replace('-', '_')}"
                    environment[variable] = str(socket_path)
                result_file = ssp.parent / f"{ssp.stem}-result.csv"
                log_file = ssp.parent / f"{ssp.stem}-omsimulator.log"
                completed = subprocess.run(
                    [
                        omsimulator,
                        str(ssp),
                        "--mode=cs",
                        "--startTime=0",
                        f"--stopTime={stop_time_s}",
                        f"--timeout={timeout_s}",
                        f"--resultFile={result_file}",
                        f"--logFile={log_file}",
                    ],
                    capture_output=True,
                    text=True,
                    timeout=timeout_s + 10,
                    check=False,
                    env=environment,
                )
                if completed.returncode != 0:
                    raise RuntimeError(
                        f"OMSimulator exited {completed.returncode}: "
                        f"{(completed.stderr or completed.stdout)[-4000:]}"
                    )
                return FmiRunResult(result_file, log_file, completed.returncode)
            finally:
                for server in servers:
                    server.close()
