from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from ael.contracts import BackendName, Event, SystemComponent
from ael.security import resolve_workspace_path

from .base import Adapter, AdapterProbe, AdapterStepResult


class NativeAnalysisAdapter(Adapter):
    """Deterministic, declarative host analysis for build/configuration benchmarks.

    The adapter intentionally has no command field and cannot execute user-provided code.
    Its JSON model is a table of case identifiers and fixed/faulty expected metrics.
    """

    backend = BackendName.NATIVE

    def __init__(self) -> None:
        self.component: SystemComponent | None = None
        self.cases: dict[str, dict[str, dict[str, Any]]] = {}
        self.inputs: dict[str, Any] = {}

    def probe(self) -> AdapterProbe:
        return AdapterProbe(
            backend=self.backend,
            available=True,
            command=None,
            detected_version="1",
            expected_version="1",
            reason="declarative analysis only; no arbitrary command execution",
        )

    def prepare(self, component: SystemComponent, seed: int) -> None:
        del seed
        if not component.model:
            raise ValueError("native analysis component requires a JSON model")
        workspace = Path.cwd().resolve()
        path = resolve_workspace_path(workspace, component.model, must_exist=True)
        payload = json.loads(path.read_text(encoding="utf-8"))
        if set(payload) != {"version", "cases"} or payload["version"] != 1:
            raise ValueError("native analysis model must contain only version=1 and cases")
        if not isinstance(payload["cases"], dict):
            raise ValueError("native analysis cases must be an object")
        self.component = component
        self.cases = payload["cases"]
        self.inputs = {}

    def inject(self, target: str, value: Any, virtual_time_us: int) -> list[Event]:
        key = target.rsplit(".", 1)[-1]
        if key not in {"case_id", "fixed"}:
            raise ValueError(f"unsupported native analysis input: {key}")
        self.inputs[key] = value
        return [
            Event(
                sequence=0,
                virtual_time_us=virtual_time_us,
                source=self.component.id if self.component else "native",
                type="native.analysis_input",
                payload={"name": key, "value": value},
                fidelity_ref="native:declarative-analysis",
            )
        ]

    def step(self, virtual_time_us: int, step_us: int) -> AdapterStepResult:
        case_id = str(int(self.inputs.get("case_id", 0)))
        variant = "fixed" if bool(self.inputs.get("fixed", False)) else "faulty"
        try:
            metrics = self.cases[case_id][variant]
        except KeyError as exception:
            raise ValueError(f"native model has no {case_id}/{variant} case") from exception
        events = [
            Event(
                sequence=0,
                virtual_time_us=virtual_time_us + step_us,
                source=self.component.id if self.component else "native",
                type="native.analysis_complete",
                payload={"case_id": case_id, "variant": variant, "metrics": metrics},
                fidelity_ref="native:declarative-analysis",
            )
        ]
        return AdapterStepResult(outputs=dict(metrics), metrics=dict(metrics), events=events)

    def snapshot(self, destination: str) -> str | None:
        path = resolve_workspace_path(Path.cwd(), destination)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(self.inputs, sort_keys=True) + "\n", encoding="utf-8")
        return str(path)

    def shutdown(self) -> None:
        self.component = None
        self.inputs = {}
