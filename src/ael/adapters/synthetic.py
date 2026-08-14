from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from ael.contracts import BackendName, Event, SystemComponent

from .base import Adapter, AdapterProbe, AdapterStepResult


class SyntheticAdapter(Adapter):
    """Deterministic test adapter. Never eligible for validated claims."""

    backend = BackendName.SYNTHETIC

    def __init__(self) -> None:
        self.component: SystemComponent | None = None
        self.state: dict[str, Any] = {}
        self.sequence = 0

    def probe(self) -> AdapterProbe:
        return AdapterProbe(
            backend=self.backend,
            available=True,
            command="in-process",
            detected_version="1",
            expected_version="1",
            reason="test-only synthetic backend; not production evidence",
        )

    def prepare(self, component: SystemComponent, seed: int) -> None:
        self.component = component
        self.state = {"seed": seed, "counter": 0, **component.properties.get("initial", {})}

    def inject(self, target: str, value: Any, virtual_time_us: int) -> list[Event]:
        key = target.rsplit(".", 1)[-1]
        self.state[key] = value
        return [
            Event(
                sequence=0,
                virtual_time_us=virtual_time_us,
                source=self.component.id if self.component else "synthetic",
                type="synthetic.inject",
                payload={"target": target, "value": value},
                fidelity_ref="synthetic-test-only",
            )
        ]

    def step(self, virtual_time_us: int, step_us: int) -> AdapterStepResult:
        self.state["counter"] = int(self.state.get("counter", 0)) + 1
        gain = float(self.component.properties.get("gain", 1.0)) if self.component else 1.0
        input_value = float(self.state.get("input", 0.0))
        output = input_value * gain
        self.state["output"] = output
        return AdapterStepResult(
            outputs={"output": output},
            metrics={"counter": self.state["counter"], "output": output},
            events=[
                Event(
                    sequence=0,
                    virtual_time_us=virtual_time_us + step_us,
                    source=self.component.id if self.component else "synthetic",
                    type="synthetic.step",
                    payload={"counter": self.state["counter"], "output": output},
                    fidelity_ref="synthetic-test-only",
                )
            ],
        )

    def snapshot(self, destination: str) -> str | None:
        path = Path(destination)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(self.state, sort_keys=True) + "\n", encoding="utf-8")
        return str(path)

    def shutdown(self) -> None:
        self.component = None
