from __future__ import annotations

from collections.abc import Callable

from ael.constants import TOOL_VERSIONS
from ael.contracts import BackendName

from .base import Adapter, AdapterProbe
from .native import NativeAnalysisAdapter
from .process import ProcessTool
from .subprocess_adapter import SubprocessAdapter
from .synthetic import SyntheticAdapter

TOOL_BACKENDS = {
    "renode": BackendName.RENODE,
    "ngspice": BackendName.NGSPICE,
    "openmodelica": BackendName.MODELICA,
    "omsimulator": BackendName.OMSIMULATOR,
    "ns-3": BackendName.NS3,
    "openems": BackendName.OPENEMS,
}


class AdapterCatalog:
    def __init__(self) -> None:
        self._probe_cache: dict[BackendName, AdapterProbe] = {}
        self._factories: dict[BackendName, Callable[[], Adapter]] = {
            BackendName.ZEPHYR_BUILD: lambda: SubprocessAdapter(
                BackendName.ZEPHYR_BUILD,
                "ael.backend_workers.zephyr_build",
                "4.4.2",
                timeout_s=360,
            ),
            BackendName.SYNTHETIC: SyntheticAdapter,
            BackendName.NATIVE: NativeAnalysisAdapter,
            BackendName.RENODE: lambda: SubprocessAdapter(
                BackendName.RENODE, "ael.backend_workers.renode", "1.16.1"
            ),
            BackendName.NGSPICE: lambda: SubprocessAdapter(
                BackendName.NGSPICE, "ael.backend_workers.ngspice", "46"
            ),
            BackendName.MODELICA: lambda: SubprocessAdapter(
                BackendName.MODELICA, "ael.backend_workers.modelica", "1.27.0"
            ),
            BackendName.OMSIMULATOR: lambda: SubprocessAdapter(
                BackendName.OMSIMULATOR,
                "ael.backend_workers.omsimulator",
                "2.1.3",
            ),
            BackendName.NS3: lambda: SubprocessAdapter(
                BackendName.NS3, "ael.backend_workers.ns3", "3.47"
            ),
            BackendName.OPENEMS: lambda: SubprocessAdapter(
                BackendName.OPENEMS, "ael.backend_workers.openems", "0.0.36"
            ),
        }
        self._tools = {
            TOOL_BACKENDS[tool.name]: ProcessTool(TOOL_BACKENDS[tool.name], tool)
            for tool in TOOL_VERSIONS
            if tool.name in TOOL_BACKENDS
        }

    def register(self, backend: BackendName, factory: Callable[[], Adapter]) -> None:
        self._factories[backend] = factory
        self._probe_cache.pop(backend, None)

    def create(self, backend: BackendName) -> Adapter:
        try:
            return self._factories[backend]()
        except KeyError as error:
            raise RuntimeError(
                f"backend {backend} is declared but its runtime adapter is not installed"
            ) from error

    def probe(self, backend: BackendName) -> AdapterProbe:
        if backend in self._probe_cache:
            return self._probe_cache[backend]
        if backend in self._factories:
            probe = self._factories[backend]().probe()
            self._probe_cache[backend] = probe
            return probe
        if backend in self._tools:
            probe = self._tools[backend].probe()
            if probe.available:
                return AdapterProbe(
                    backend=probe.backend,
                    available=False,
                    command=probe.command,
                    detected_version=probe.detected_version,
                    expected_version=probe.expected_version,
                    reason="tool is installed but the AEL runtime adapter is not yet implemented",
                )
            return probe
        if backend == BackendName.HARDWARE:
            return AdapterProbe(
                backend=backend,
                available=False,
                command=None,
                detected_version=None,
                expected_version=None,
                reason="requires a configured worker capability",
            )
        return AdapterProbe(backend, False, None, None, None, "unknown backend")

    def probes(self) -> list[AdapterProbe]:
        return [self.probe(backend) for backend in BackendName]
