from __future__ import annotations

import re
from datetime import UTC, datetime
from enum import StrEnum
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from .constants import API_VERSION


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid", validate_assignment=True)


class ProblemCategory(StrEnum):
    BUILD = "build"
    CONFIGURATION = "configuration"
    BOOT = "boot"
    MEMORY = "memory"
    DIGITAL_IO = "digital_io"
    TIMING = "timing"
    INTERRUPT = "interrupt"
    DMA = "dma"
    SERIAL_BUS = "serial_bus"
    RTOS = "rtos"
    UPDATE = "update"
    ANALOG = "analog"
    POWER = "power"
    THERMAL = "thermal"
    NETWORK = "network"
    RF = "rf"
    ELECTROMAGNETIC = "electromagnetic"
    SECURITY = "security"
    HARDWARE_ONLY = "hardware_only"


class BackendName(StrEnum):
    NATIVE = "native"
    RENODE = "renode"
    NGSPICE = "ngspice"
    MODELICA = "openmodelica"
    OMSIMULATOR = "omsimulator"
    NS3 = "ns3"
    OPENEMS = "openems"
    HARDWARE = "hardware"
    SYNTHETIC = "synthetic"


class ModelState(StrEnum):
    DRAFT = "draft"
    GENERATED = "generated"
    STATIC_VALIDATED = "static_validated"
    CONFORMANCE_VALIDATED = "conformance_validated"
    HARDWARE_VALIDATED = "hardware_validated"
    PRODUCTION_APPROVED = "production_approved"
    DEPRECATED = "deprecated"


class ClaimStatus(StrEnum):
    UNVERIFIED = "unverified"
    MODEL_DEPENDENT = "model_dependent"
    SIMULATION_VALIDATED = "simulation_validated"
    HARDWARE_VALIDATED = "hardware_validated"
    PRODUCTION_APPROVED = "production_approved"


class RunStatus(StrEnum):
    QUEUED = "queued"
    RUNNING = "running"
    PASSED = "passed"
    FAILED = "failed"
    BLOCKED = "blocked"
    CANCELLED = "cancelled"


class WorkerKind(StrEnum):
    SIMULATION = "simulation"
    LAB = "lab"


class TaskStatus(StrEnum):
    QUEUED = "queued"
    LEASED = "leased"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"
    EXPIRED = "expired"


class ReleaseProfile(StrEnum):
    FOUNDATION = "foundation"
    SIMULATION = "simulation"
    PRODUCTION = "production"


class UnitValue(StrictModel):
    value: float
    unit: str = Field(min_length=1, max_length=32)

    @field_validator("unit")
    @classmethod
    def validate_unit(cls, value: str) -> str:
        if value == "1":
            return value
        allowed_symbols = {
            "A",
            "V",
            "W",
            "J",
            "Ohm",
            "S",
            "F",
            "H",
            "Hz",
            "s",
            "min",
            "h",
            "m",
            "g",
            "K",
            "Cel",
            "rad",
            "deg",
            "N",
            "Pa",
            "bit",
            "By",
            "dB",
            "dBm",
            "%",
            "C",
        }
        prefixes = ("p", "n", "u", "m", "c", "d", "k", "M", "G")
        tokens = re.findall(r"[A-Za-z%]+", value)
        if not tokens or any(
            token not in allowed_symbols
            and not any(
                token == f"{prefix}{symbol}" for prefix in prefixes for symbol in allowed_symbols
            )
            for token in tokens
        ):
            raise ValueError("unit must use supported UCUM symbols")
        if re.search(r"[^A-Za-z0-9./*^%()-]", value):
            raise ValueError("unit must be a compact UCUM-compatible expression")
        return value


class ProblemSpec(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["ProblemSpec"] = "ProblemSpec"
    name: str = Field(min_length=1, max_length=128)
    title: str = Field(min_length=1, max_length=256)
    category: ProblemCategory
    symptoms: list[str] = Field(min_length=1)
    target: str | None = None
    constraints: dict[str, Any] = Field(default_factory=dict)
    evidence: list[str] = Field(default_factory=list)
    desired_claims: list[str] = Field(default_factory=list)


class Port(StrictModel):
    name: str
    direction: Literal["input", "output", "bidirectional"]
    data_type: Literal["real", "integer", "boolean", "string", "bytes"]
    unit: str | None = None

    @field_validator("unit")
    @classmethod
    def validate_optional_unit(cls, value: str | None) -> str | None:
        if value is not None:
            UnitValue(value=0.0, unit=value)
        return value


class SystemComponent(StrictModel):
    id: str = Field(pattern=r"^[A-Za-z][A-Za-z0-9_.-]*$")
    type: str
    backend: BackendName
    model: str | None = None
    step_us: int | None = Field(default=None, gt=0)
    event_driven: bool = False
    direct_feedthrough: bool = False
    ports: list[Port] = Field(default_factory=list)
    properties: dict[str, Any] = Field(default_factory=dict)

    @model_validator(mode="after")
    def validate_step_policy(self) -> SystemComponent:
        if not self.event_driven and self.step_us is None:
            raise ValueError("periodic components require step_us")
        return self


class Connection(StrictModel):
    source: str = Field(pattern=r"^[A-Za-z][A-Za-z0-9_.-]*\.[A-Za-z][A-Za-z0-9_.-]*$")
    target: str = Field(pattern=r"^[A-Za-z][A-Za-z0-9_.-]*\.[A-Za-z][A-Za-z0-9_.-]*$")
    unit: str | None = None


class SystemManifest(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["SystemManifest"] = "SystemManifest"
    name: str = Field(min_length=1, max_length=128)
    description: str | None = None
    components: list[SystemComponent] = Field(min_length=1)
    connections: list[Connection] = Field(default_factory=list)
    fidelity: dict[str, str] = Field(default_factory=dict)

    @model_validator(mode="after")
    def validate_topology(self) -> SystemManifest:
        component_ids = [component.id for component in self.components]
        if len(component_ids) != len(set(component_ids)):
            raise ValueError("component ids must be unique")
        ports = {
            f"{component.id}.{port.name}"
            for component in self.components
            for port in component.ports
        }
        for connection in self.connections:
            if connection.source not in ports or connection.target not in ports:
                raise ValueError(f"connection references an unknown port: {connection}")
            source_component_id, source_port_name = connection.source.split(".", 1)
            target_component_id, target_port_name = connection.target.split(".", 1)
            components = {component.id: component for component in self.components}
            source_port = next(
                port
                for port in components[source_component_id].ports
                if port.name == source_port_name
            )
            target_port = next(
                port
                for port in components[target_component_id].ports
                if port.name == target_port_name
            )
            if source_port.direction == "input" or target_port.direction == "output":
                raise ValueError(f"connection has incompatible direction: {connection}")
            declared_units = {source_port.unit, target_port.unit, connection.unit} - {None}
            if len(declared_units) > 1:
                raise ValueError(f"connection has incompatible units: {connection}")
            if source_port.data_type != target_port.data_type:
                raise ValueError(f"connection has incompatible data types: {connection}")
        return self


class Stimulus(StrictModel):
    at_us: int = Field(ge=0)
    target: str
    value: Any
    unit: str | None = None


class Fault(StrictModel):
    at_us: int = Field(ge=0)
    target: str
    type: str
    parameters: dict[str, Any] = Field(default_factory=dict)


class Assertion(StrictModel):
    metric: str
    operator: Literal["eq", "ne", "lt", "le", "gt", "ge", "contains"]
    expected: Any
    unit: str | None = None
    critical: bool = True


class ExperimentSpec(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["ExperimentSpec"] = "ExperimentSpec"
    name: str = Field(min_length=1, max_length=128)
    system: str
    duration_us: int = Field(gt=0, le=86_400_000_000)
    macro_step_us: int = Field(default=1000, gt=0)
    timeout_s: int = Field(default=300, gt=0, le=86_400)
    requires_rollback: bool = False
    checkpoint_interval_us: int | None = Field(default=None, gt=0)
    seed: int = Field(default=0, ge=0)
    stimuli: list[Stimulus] = Field(default_factory=list)
    faults: list[Fault] = Field(default_factory=list)
    assertions: list[Assertion] = Field(default_factory=list)
    observables: list[str] = Field(default_factory=list)
    allowed_model_states: set[ModelState] = Field(
        default_factory=lambda: {
            ModelState.HARDWARE_VALIDATED,
            ModelState.PRODUCTION_APPROVED,
        }
    )
    tags: set[str] = Field(default_factory=set)

    @model_validator(mode="after")
    def validate_timeline(self) -> ExperimentSpec:
        for item in [*self.stimuli, *self.faults]:
            if item.at_us > self.duration_us:
                raise ValueError("stimulus/fault falls outside experiment duration")
        if self.duration_us % self.macro_step_us:
            raise ValueError("duration_us must be divisible by macro_step_us")
        if self.checkpoint_interval_us and (
            self.checkpoint_interval_us % self.macro_step_us
        ):
            raise ValueError("checkpoint_interval_us must align to macro_step_us")
        return self


class BehaviorField(StrictModel):
    name: str
    lsb: int = Field(ge=0, le=63)
    width: int = Field(gt=0, le=64)
    access: Literal["ro", "wo", "rw", "w1c", "w1s"]
    reset: int = Field(default=0, ge=0)
    side_effect: str | None = None

    @model_validator(mode="after")
    def validate_bits(self) -> BehaviorField:
        if self.lsb + self.width > 64:
            raise ValueError("field extends past 64 bits")
        if self.reset >= (1 << self.width):
            raise ValueError("reset value does not fit field width")
        return self


class BehaviorRegister(StrictModel):
    name: str
    offset: int = Field(ge=0)
    width: Literal[8, 16, 32, 64] = 32
    reset: int = Field(default=0, ge=0)
    fields: list[BehaviorField] = Field(default_factory=list)


class ClockBehavior(StrictModel):
    name: str
    source: str
    frequency: UnitValue
    enabled_when: str | None = None
    reset_domain: str | None = None


class TimerBehavior(StrictModel):
    name: str
    clock: str
    width: Literal[8, 16, 24, 32, 64]
    direction: Literal["up", "down", "up_down"] = "up"
    wrap_event: str | None = None
    compare_channels: int = Field(default=0, ge=0, le=16)


class InterruptBehavior(StrictModel):
    name: str
    line: int = Field(ge=0)
    trigger: Literal["level_high", "level_low", "rising", "falling", "pulse"]
    condition: str
    clear_condition: str | None = None


class DmaRequestBehavior(StrictModel):
    name: str
    request_line: int = Field(ge=0)
    condition: str
    direction: Literal["peripheral_to_memory", "memory_to_peripheral", "bidirectional"]
    width_bits: Literal[8, 16, 32, 64]


class TransactionBehavior(StrictModel):
    name: str
    protocol: Literal["uart", "i2c", "spi", "can", "can_fd", "custom"]
    role: Literal["controller", "target", "peer"]
    latency: UnitValue | None = None
    timeout: UnitValue | None = None
    crc: str | None = None


class FaultBehavior(StrictModel):
    name: str
    trigger: str
    effect: str
    recoverable: bool
    recovery: str | None = None


class PowerStateBehavior(StrictModel):
    name: str
    current: UnitValue | None = None
    entry_condition: str | None = None
    exit_condition: str | None = None
    wake_latency: UnitValue | None = None


class HardwareBehaviorIR(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["HardwareBehaviorIR"] = "HardwareBehaviorIR"
    name: str
    bus_width: Literal[8, 16, 32, 64] = 32
    size: int = Field(gt=0)
    registers: list[BehaviorRegister] = Field(default_factory=list)
    state_machines: list[dict[str, Any]] = Field(default_factory=list)
    clocks: list[ClockBehavior] = Field(default_factory=list)
    timers: list[TimerBehavior] = Field(default_factory=list)
    interrupts: list[InterruptBehavior] = Field(default_factory=list)
    dma_requests: list[DmaRequestBehavior] = Field(default_factory=list)
    transactions: list[TransactionBehavior] = Field(default_factory=list)
    faults: list[FaultBehavior] = Field(default_factory=list)
    timing: dict[str, UnitValue] = Field(default_factory=dict)
    power_states: list[PowerStateBehavior] = Field(default_factory=list)
    fmi_ports: list[Port] = Field(default_factory=list)

    @model_validator(mode="after")
    def validate_register_layout(self) -> HardwareBehaviorIR:
        names = [register.name for register in self.registers]
        if len(names) != len(set(names)):
            raise ValueError("register names must be unique")
        occupied: list[tuple[int, int, str]] = []
        for register in self.registers:
            end = register.offset + register.width // 8
            if end > self.size:
                raise ValueError(f"register {register.name} exceeds peripheral size")
            if register.reset >= (1 << register.width):
                raise ValueError(f"register {register.name} reset does not fit width")
            field_names = [field.name for field in register.fields]
            if len(field_names) != len(set(field_names)):
                raise ValueError(f"register {register.name} field names must be unique")
            field_bits: set[int] = set()
            for field in register.fields:
                if field.lsb + field.width > register.width:
                    raise ValueError(f"field {register.name}.{field.name} exceeds register width")
                bits = set(range(field.lsb, field.lsb + field.width))
                if field_bits & bits:
                    raise ValueError(f"register {register.name} contains overlapping fields")
                field_bits |= bits
            for start, previous_end, previous_name in occupied:
                if register.offset < previous_end and end > start:
                    raise ValueError(f"register {register.name} overlaps register {previous_name}")
            occupied.append((register.offset, end, register.name))
        return self


class ModelGenerationRequest(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["ModelGenerationRequest"] = "ModelGenerationRequest"
    id: str = Field(pattern=r"^[A-Za-z][A-Za-z0-9_.-]*$")
    name: str
    version: str
    backend: BackendName
    svd: str | None = None
    systemrdl: str | None = None
    datasheets: list[str] = Field(default_factory=list)
    drivers: list[str] = Field(default_factory=list)
    reference_models: list[str] = Field(default_factory=list)
    hardware_traces: list[str] = Field(default_factory=list)
    generator: str = "ael.svd-importer/v1"

    @model_validator(mode="after")
    def require_grounding(self) -> ModelGenerationRequest:
        if not any(
            [
                self.svd,
                self.systemrdl,
                self.datasheets,
                self.drivers,
                self.reference_models,
                self.hardware_traces,
            ]
        ):
            raise ValueError("model generation requires at least one grounding input")
        return self


class ModelPackage(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["ModelPackage"] = "ModelPackage"
    id: str
    name: str
    version: str
    backend: BackendName
    state: ModelState = ModelState.DRAFT
    source_paths: list[str] = Field(default_factory=list)
    source_hashes: dict[str, str] = Field(default_factory=dict)
    ir_path: str | None = None
    artifact_paths: list[str] = Field(default_factory=list)
    test_paths: list[str] = Field(default_factory=list)
    validation_evidence: list[str] = Field(default_factory=list)
    signature: str | None = None
    generated_by: str | None = None
    created_at: datetime = Field(default_factory=lambda: datetime.now(UTC))


class MetricTolerance(StrictModel):
    metric: str
    absolute: UnitValue | None = None
    relative_percent: float | None = Field(default=None, ge=0)

    @model_validator(mode="after")
    def require_tolerance(self) -> MetricTolerance:
        if self.absolute is None and self.relative_percent is None:
            raise ValueError("at least one tolerance is required")
        return self


class ValidationEnvelope(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["ValidationEnvelope"] = "ValidationEnvelope"
    id: str
    target: str
    hardware_revision: str
    model_id: str
    model_version: str
    conditions: dict[str, UnitValue]
    tolerances: list[MetricTolerance]
    sample_count: int = Field(ge=1)
    calibration_ids: list[str] = Field(default_factory=list)
    valid_from: datetime
    valid_until: datetime | None = None
    approved_by: str | None = None


class WorkerCapability(StrictModel):
    name: str
    version: str
    kind: Literal["backend", "board", "instrument", "service"]
    labels: dict[str, str] = Field(default_factory=dict)
    fidelity: dict[str, str] = Field(default_factory=dict)


class WorkerRegistration(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["WorkerRegistration"] = "WorkerRegistration"
    worker_id: str = Field(pattern=r"^[A-Za-z][A-Za-z0-9_.-]*$")
    worker_kind: WorkerKind
    capabilities: list[WorkerCapability]
    agent_version: str
    certificate_fingerprint: str = Field(pattern=r"^[a-fA-F0-9]{64}$")


class WorkerTask(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["WorkerTask"] = "WorkerTask"
    task_id: str
    task_type: Literal["experiment", "model_validation", "calibration", "hardware_validation"]
    required_capabilities: list[str] = Field(default_factory=list)
    payload: dict[str, Any]
    idempotency_key: str
    status: TaskStatus = TaskStatus.QUEUED
    lease_owner: str | None = None
    lease_token: str | None = None
    lease_expires_at: datetime | None = None
    attempts: int = Field(default=0, ge=0)
    created_at: datetime = Field(default_factory=lambda: datetime.now(UTC))


class WorkerHeartbeat(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["WorkerHeartbeat"] = "WorkerHeartbeat"
    task_id: str
    lease_token: str
    status: Literal["leased", "running"]
    progress: float = Field(default=0.0, ge=0.0, le=1.0)
    message: str | None = None


class WorkerTaskResult(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["WorkerTaskResult"] = "WorkerTaskResult"
    task_id: str
    lease_token: str
    status: Literal["succeeded", "failed", "cancelled"]
    artifact_hashes: dict[str, str] = Field(default_factory=dict)
    evidence_bundle: str | None = None
    error: str | None = None


class CalibrationRecord(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["CalibrationRecord"] = "CalibrationRecord"
    id: str
    instrument_id: str
    instrument_kind: str
    certificate_path: str
    certificate_sha256: str = Field(pattern=r"^[a-f0-9]{64}$")
    calibrated_at: datetime
    expires_at: datetime
    laboratory: str


class InstrumentEvidence(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["InstrumentEvidence"] = "InstrumentEvidence"
    instrument_id: str
    driver: str
    calibration_id: str
    operation: str
    parameters: dict[str, UnitValue | str | int | float | bool]
    raw_artifact_path: str
    raw_artifact_sha256: str = Field(pattern=r"^[a-f0-9]{64}$")
    captured_at: datetime


class InstrumentOperationRequest(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["InstrumentOperationRequest"] = "InstrumentOperationRequest"
    instrument_id: str
    operation: Literal[
        "power.configure",
        "power.output",
        "power.measure",
        "scope.capture",
        "logic.capture",
        "energy.measure",
        "chamber.set_temperature",
        "vna.sparameters",
        "spectrum.capture",
        "attenuator.set",
    ]
    parameters: dict[str, UnitValue | str | int | float | bool] = Field(default_factory=dict)
    calibration_id: str


class AcceptanceEntry(StrictModel):
    name: str
    status: Literal["passed", "failed", "blocked"]
    evidence_path: str | None = None
    evidence_sha256: str | None = Field(default=None, pattern=r"^[a-f0-9]{64}$")
    limitations: list[str] = Field(default_factory=list)


class AcceptanceManifest(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["AcceptanceManifest"] = "AcceptanceManifest"
    profile: ReleaseProfile
    source_revision: str
    platform: str
    entries: list[AcceptanceEntry]
    created_at: datetime
    signature: str | None = None

    @model_validator(mode="after")
    def require_unique_entries(self) -> AcceptanceManifest:
        names = [entry.name for entry in self.entries]
        if len(names) != len(set(names)):
            raise ValueError("acceptance entry names must be unique")
        return self


class Event(StrictModel):
    sequence: int = Field(ge=0)
    virtual_time_us: int = Field(ge=0)
    source: str
    type: str
    payload: dict[str, Any] = Field(default_factory=dict)
    parent_sequence: int | None = Field(default=None, ge=0)
    fidelity_ref: str | None = None


class Claim(StrictModel):
    id: str
    statement: str
    status: ClaimStatus
    evidence_paths: list[str]
    limitations: list[str] = Field(default_factory=list)
    validation_envelope_id: str | None = None
    hardware_validated: bool = False

    @model_validator(mode="after")
    def validate_production_claim(self) -> Claim:
        if self.status in {
            ClaimStatus.HARDWARE_VALIDATED,
            ClaimStatus.PRODUCTION_APPROVED,
        } and (not self.hardware_validated or self.validation_envelope_id is None):
            raise ValueError("hardware claims require an envelope and hardware validation")
        return self


class EvidenceBundle(StrictModel):
    api_version: Literal["ael.dev/v1"] = API_VERSION
    kind: Literal["EvidenceBundle"] = "EvidenceBundle"
    run_id: str
    status: RunStatus
    created_at: datetime
    completed_at: datetime | None = None
    experiment_hash: str
    system_hash: str
    model_hashes: dict[str, str] = Field(default_factory=dict)
    artifact_hashes: dict[str, str] = Field(default_factory=dict)
    event_count: int = Field(ge=0)
    assertion_results: list[dict[str, Any]] = Field(default_factory=list)
    claims: list[Claim] = Field(default_factory=list)
    fidelity_boundaries: list[str] = Field(default_factory=list)
    signature: str | None = None
