from __future__ import annotations

import math
import threading
import time
from contextlib import suppress
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .adapters import Adapter, AdapterCatalog
from .contracts import ExperimentSpec, RunStatus, SystemManifest
from .evidence import EvidenceRecorder
from .storage import WorkspaceLayout


class ExperimentBlocked(RuntimeError):
    pass


@dataclass(frozen=True)
class RunResult:
    run_id: str
    status: RunStatus
    evidence_dir: Path
    error: str | None = None


class DeterministicScheduler:
    def __init__(
        self,
        layout: WorkspaceLayout,
        catalog: AdapterCatalog | None = None,
        cas: object | None = None,
    ) -> None:
        self.layout = layout
        self.catalog = catalog or AdapterCatalog()
        self.cas = cas

    def run(
        self,
        run_id: str,
        experiment: ExperimentSpec,
        system: SystemManifest,
        cancel_event: threading.Event | None = None,
    ) -> RunResult:
        recorder = EvidenceRecorder(
            self.layout, run_id, experiment, system, cas=self.cas
        )
        adapters: dict[str, Adapter] = {}
        status = RunStatus.RUNNING
        error: str | None = None
        metrics: dict[str, Any] = {}
        started = time.monotonic()
        try:
            for component in system.components:
                probe = self.catalog.probe(component.backend)
                if not probe.available:
                    raise ExperimentBlocked(f"{component.id}/{component.backend}: {probe.reason}")
                adapter = self.catalog.create(component.backend)
                adapter.prepare(component, experiment.seed)
                adapters[component.id] = adapter

            quantum = math.gcd(
                experiment.macro_step_us,
                *[
                    component.step_us
                    for component in system.components
                    if component.step_us is not None
                ],
            )
            stimuli = {item.at_us: [] for item in experiment.stimuli}
            faults = {item.at_us: [] for item in experiment.faults}
            for item in experiment.stimuli:
                stimuli[item.at_us].append(item)
            for item in experiment.faults:
                faults[item.at_us].append(item)

            connection_map: dict[str, list[str]] = {}
            for connection in system.connections:
                connection_map.setdefault(connection.source, []).append(connection.target)
            pending_injections: dict[int, list[tuple[str, Any]]] = {}

            for virtual_time in range(0, experiment.duration_us, quantum):
                if cancel_event and cancel_event.is_set():
                    status = RunStatus.CANCELLED
                    error = "experiment cancelled by control plane"
                    break
                if time.monotonic() - started > experiment.timeout_s:
                    raise TimeoutError(f"experiment exceeded {experiment.timeout_s}s wall timeout")

                for target, value in pending_injections.pop(virtual_time, []):
                    target_id = target.split(".", 1)[0]
                    recorder.add_events(adapters[target_id].inject(target, value, virtual_time))
                for stimulus in stimuli.get(virtual_time, []):
                    component_id = stimulus.target.split(".", 1)[0]
                    recorder.add_events(
                        adapters[component_id].inject(stimulus.target, stimulus.value, virtual_time)
                    )
                for fault in faults.get(virtual_time, []):
                    component_id = fault.target.split(".", 1)[0]
                    recorder.add_events(
                        adapters[component_id].inject(
                            f"{fault.target}.fault.{fault.type}", fault.parameters, virtual_time
                        )
                    )

                for component in system.components:
                    step_us = component.step_us or quantum
                    if virtual_time % step_us:
                        continue
                    result = adapters[component.id].step(virtual_time, step_us)
                    self._reject_non_finite(result.outputs)
                    self._reject_non_finite(result.metrics)
                    recorder.add_events(result.events)
                    recorder.add_artifacts(component.id, virtual_time + step_us, result.artifacts)
                    for name, value in result.metrics.items():
                        metrics[f"{component.id}.{name}"] = value
                    for port, value in result.outputs.items():
                        source = f"{component.id}.{port}"
                        metrics[source] = value
                        for target in connection_map.get(source, []):
                            pending_injections.setdefault(virtual_time + step_us, []).append(
                                (target, value)
                            )
                if (
                    experiment.checkpoint_interval_us
                    and (virtual_time + quantum) % experiment.checkpoint_interval_us == 0
                ):
                    checkpoint = virtual_time + quantum
                    for component_id, adapter in adapters.items():
                        adapter.snapshot(
                            str(
                                recorder.snapshot_dir
                                / f"{component_id}-{checkpoint:016d}.snapshot"
                            )
                        )

            if status != RunStatus.CANCELLED:
                critical_failure = False
                for assertion in experiment.assertions:
                    actual = metrics.get(assertion.metric)
                    passed = self._evaluate(actual, assertion.operator, assertion.expected)
                    recorder.add_assertion(assertion, actual, passed)
                    critical_failure = critical_failure or (assertion.critical and not passed)
                status = RunStatus.FAILED if critical_failure else RunStatus.PASSED
        except ExperimentBlocked as exception:
            status, error = RunStatus.BLOCKED, str(exception)
        except Exception as exception:  # evidence preservation is the safety boundary
            status, error = RunStatus.FAILED, f"{type(exception).__name__}: {exception}"
            for component_id, adapter in adapters.items():
                with suppress(Exception):
                    adapter.snapshot(str(recorder.snapshot_dir / f"{component_id}.json"))
        finally:
            for adapter in adapters.values():
                with suppress(Exception):
                    adapter.shutdown()

        recorder.finalize(status, error=error)
        return RunResult(run_id, status, recorder.run_dir, error)

    @staticmethod
    def _reject_non_finite(values: dict[str, Any]) -> None:
        for key, value in values.items():
            if isinstance(value, float) and not math.isfinite(value):
                raise FloatingPointError(f"non-finite value for {key}: {value}")

    @staticmethod
    def _evaluate(actual: Any, operator: str, expected: Any) -> bool:
        if operator == "eq":
            return actual == expected
        if operator == "ne":
            return actual != expected
        if operator == "contains":
            return actual is not None and expected in actual
        if actual is None:
            return False
        operations = {
            "lt": lambda: actual < expected,
            "le": lambda: actual <= expected,
            "gt": lambda: actual > expected,
            "ge": lambda: actual >= expected,
        }
        return bool(operations[operator]())
