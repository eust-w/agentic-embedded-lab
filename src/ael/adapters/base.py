from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any

from ael.contracts import BackendName, Event, SystemComponent


@dataclass(frozen=True)
class AdapterProbe:
    backend: BackendName
    available: bool
    command: str | None
    detected_version: str | None
    expected_version: str | None
    reason: str | None = None


@dataclass
class AdapterStepResult:
    outputs: dict[str, Any] = field(default_factory=dict)
    metrics: dict[str, Any] = field(default_factory=dict)
    events: list[Event] = field(default_factory=list)


class Adapter(ABC):
    backend: BackendName

    @abstractmethod
    def probe(self) -> AdapterProbe: ...

    @abstractmethod
    def prepare(self, component: SystemComponent, seed: int) -> None: ...

    @abstractmethod
    def inject(self, target: str, value: Any, virtual_time_us: int) -> list[Event]: ...

    @abstractmethod
    def step(self, virtual_time_us: int, step_us: int) -> AdapterStepResult: ...

    @abstractmethod
    def snapshot(self, destination: str) -> str | None: ...

    @abstractmethod
    def shutdown(self) -> None: ...
