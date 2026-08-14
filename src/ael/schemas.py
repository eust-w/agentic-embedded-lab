from __future__ import annotations

from pathlib import Path

from .benchmarks import BenchmarkCatalog, BenchmarkMechanism
from .contracts import (
    AcceptanceManifest,
    CalibrationRecord,
    Claim,
    Event,
    EvidenceBundle,
    ExperimentSpec,
    HardwareBehaviorIR,
    InstrumentEvidence,
    InstrumentOperationRequest,
    ModelConformanceEvidence,
    ModelGenerationRequest,
    ModelPackage,
    ProblemSpec,
    SystemManifest,
    ValidationEnvelope,
    WorkerHeartbeat,
    WorkerRegistration,
    WorkerTask,
    WorkerTaskResult,
)
from .io import write_json

SCHEMA_TYPES = {
    "acceptance-manifest": AcceptanceManifest,
    "benchmark-catalog": BenchmarkCatalog,
    "benchmark-mechanism": BenchmarkMechanism,
    "calibration-record": CalibrationRecord,
    "instrument-evidence": InstrumentEvidence,
    "instrument-operation-request": InstrumentOperationRequest,
    "problem-spec": ProblemSpec,
    "system-manifest": SystemManifest,
    "experiment-spec": ExperimentSpec,
    "hardware-behavior-ir": HardwareBehaviorIR,
    "model-generation-request": ModelGenerationRequest,
    "model-conformance-evidence": ModelConformanceEvidence,
    "model-package": ModelPackage,
    "validation-envelope": ValidationEnvelope,
    "event": Event,
    "claim": Claim,
    "evidence-bundle": EvidenceBundle,
    "worker-registration": WorkerRegistration,
    "worker-task": WorkerTask,
    "worker-heartbeat": WorkerHeartbeat,
    "worker-task-result": WorkerTaskResult,
}


def export_schemas(destination: Path) -> list[Path]:
    destination.mkdir(parents=True, exist_ok=True)
    outputs: list[Path] = []
    for name, model in SCHEMA_TYPES.items():
        output = destination / f"{name}.schema.json"
        write_json(output, model.model_json_schema())
        outputs.append(output)
    return outputs
