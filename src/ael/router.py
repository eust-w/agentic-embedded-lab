from __future__ import annotations

from enum import StrEnum

from pydantic import Field

from .adapters import AdapterCatalog
from .contracts import BackendName, ProblemCategory, ProblemSpec, StrictModel


class RouteStatus(StrEnum):
    EXECUTABLE = "executable"
    MODEL_GENERATION_REQUIRED = "model_generation_required"
    HARDWARE_VALIDATION_REQUIRED = "hardware_validation_required"
    BLOCKED = "blocked"


CATEGORY_BACKENDS: dict[ProblemCategory, tuple[BackendName, ...]] = {
    ProblemCategory.BUILD: (BackendName.NATIVE,),
    ProblemCategory.CONFIGURATION: (BackendName.NATIVE, BackendName.RENODE),
    ProblemCategory.BOOT: (BackendName.RENODE,),
    ProblemCategory.MEMORY: (BackendName.NATIVE, BackendName.RENODE),
    ProblemCategory.DIGITAL_IO: (BackendName.RENODE,),
    ProblemCategory.TIMING: (BackendName.RENODE, BackendName.HARDWARE),
    ProblemCategory.INTERRUPT: (BackendName.RENODE,),
    ProblemCategory.DMA: (BackendName.RENODE,),
    ProblemCategory.SERIAL_BUS: (BackendName.RENODE,),
    ProblemCategory.RTOS: (BackendName.RENODE,),
    ProblemCategory.UPDATE: (BackendName.RENODE, BackendName.NGSPICE),
    ProblemCategory.ANALOG: (BackendName.NGSPICE,),
    ProblemCategory.POWER: (BackendName.NGSPICE, BackendName.MODELICA),
    ProblemCategory.THERMAL: (BackendName.MODELICA,),
    ProblemCategory.NETWORK: (BackendName.NS3,),
    ProblemCategory.RF: (BackendName.NS3, BackendName.OPENEMS),
    ProblemCategory.ELECTROMAGNETIC: (BackendName.OPENEMS,),
    ProblemCategory.SECURITY: (BackendName.NATIVE, BackendName.RENODE),
    ProblemCategory.HARDWARE_ONLY: (BackendName.HARDWARE,),
}


class RouteResult(StrictModel):
    problem: str
    category: ProblemCategory
    status: RouteStatus
    required_backends: list[BackendName]
    available_backends: list[BackendName]
    missing_backends: list[BackendName]
    reasons: list[str] = Field(default_factory=list)
    fidelity_boundary: str


def classify_problem(problem: ProblemSpec, catalog: AdapterCatalog) -> RouteResult:
    required = list(CATEGORY_BACKENDS[problem.category])
    probes = {backend: catalog.probe(backend) for backend in required}
    available = [backend for backend, probe in probes.items() if probe.available]
    missing = [backend for backend, probe in probes.items() if not probe.available]
    reasons = [probe.reason for probe in probes.values() if probe.reason]

    if problem.category == ProblemCategory.HARDWARE_ONLY:
        status = RouteStatus.HARDWARE_VALIDATION_REQUIRED
    elif missing:
        status = RouteStatus.MODEL_GENERATION_REQUIRED
    else:
        status = RouteStatus.EXECUTABLE

    return RouteResult(
        problem=problem.name,
        category=problem.category,
        status=status,
        required_backends=required,
        available_backends=available,
        missing_backends=missing,
        reasons=reasons,
        fidelity_boundary=(
            "Routing identifies an execution path; it does not establish simulator or hardware "
            "equivalence. Any resulting claim remains bounded by model state and validation "
            "envelope."
        ),
    )
