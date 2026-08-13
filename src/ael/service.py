from __future__ import annotations

import hashlib
import json
import os
import platform
import sys
import threading
import uuid
from concurrent.futures import Future, ThreadPoolExecutor
from pathlib import Path
from typing import Any

from .adapters import AdapterCatalog
from .benchmarks import BenchmarkRunner, load_catalog, validate_acceptance_manifest
from .constants import TOOL_VERSIONS
from .contracts import (
    EvidenceBundle,
    ExperimentSpec,
    ModelState,
    ProblemSpec,
    ReleaseProfile,
    RunStatus,
    SystemManifest,
    WorkerHeartbeat,
    WorkerRegistration,
    WorkerTask,
    WorkerTaskResult,
)
from .fmi import build_schedule, validate_fmi_topology
from .io import load_document
from .modeling import ModelRegistry
from .router import classify_problem
from .scheduler import DeterministicScheduler, RunResult
from .security import resolve_workspace_path
from .storage import StateStore, WorkspaceLayout


class AelService:
    def __init__(self, workspace: Path) -> None:
        self.layout = WorkspaceLayout(workspace.resolve())
        database_url = os.environ.get("AEL_DATABASE_URL")
        if database_url:
            from .storage_server import S3ContentAddressedStore, ServerStateStore

            self.store = ServerStateStore(self.layout, database_url)
            self.storage_mode = "server"
            bucket = os.environ.get("AEL_S3_BUCKET")
            if not bucket:
                raise ValueError("AEL_S3_BUCKET is required with AEL_DATABASE_URL")
            self.cas = S3ContentAddressedStore(
                bucket,
                endpoint_url=os.environ.get("AEL_S3_ENDPOINT"),
                region_name=os.environ.get("AEL_S3_REGION"),
            )
        else:
            self.store = StateStore(self.layout)
            self.storage_mode = "local"
            from .storage import ContentAddressedStore

            self.cas = ContentAddressedStore(self.layout)
        self.catalog = AdapterCatalog()
        self.scheduler = DeterministicScheduler(self.layout, self.catalog, self.cas)
        self.models = ModelRegistry(self.layout, self.store)
        self._executor = ThreadPoolExecutor(max_workers=4, thread_name_prefix="ael-run")
        self._futures: dict[str, Future[RunResult]] = {}
        self._lock = threading.Lock()

    def doctor(self) -> dict[str, Any]:
        probes = self.catalog.probes()
        return {
            "workspace": str(self.layout.root),
            "python": sys.version.split()[0],
            "platform": platform.platform(),
            "authoritative_platform": "Ubuntu 24.04 x86_64",
            "tools": [
                {
                    "backend": probe.backend,
                    "available": probe.available,
                    "command": probe.command,
                    "detected_version": probe.detected_version,
                    "expected_version": probe.expected_version,
                    "reason": probe.reason,
                }
                for probe in probes
            ],
            "pinned_toolchain": {tool.name: tool.version for tool in TOOL_VERSIONS},
        }

    def inspect(self) -> dict[str, Any]:
        models = self.store.list_models()
        benchmark = load_catalog(self.layout.root)
        release_failures = benchmark.validate_release(self.layout.root)
        return {
            "workspace": str(self.layout.root),
            "storage_mode": self.storage_mode,
            "runs": self.store.list_runs(),
            "models": models,
            "capabilities": self.doctor()["tools"],
            "benchmark": {
                "total": len(benchmark.cases),
                "executable": sum(case.readiness == "executable" for case in benchmark.cases),
                "release_ready": not release_failures,
                "release_gap_count": len(release_failures),
            },
            "claim_policy": (
                "Claims require model/version/envelope evidence; simulation never silently "
                "becomes hardware validation."
            ),
        }

    def release_check(self, profile: ReleaseProfile = ReleaseProfile.PRODUCTION) -> dict[str, Any]:
        benchmark = load_catalog(self.layout.root)
        failures: list[str] = []
        if profile == ReleaseProfile.FOUNDATION:
            from .schemas import SCHEMA_TYPES

            required_paths = [
                self.layout.root / "LICENSE",
                self.layout.root / "pyproject.toml",
                self.layout.root / "AGENTS.md",
                *[self.layout.root / "schemas/v1" / f"{name}.schema.json" for name in SCHEMA_TYPES],
            ]
            failures.extend(
                f"foundation artifact missing: {path.relative_to(self.layout.root)}"
                for path in required_paths
                if not path.is_file()
            )
        if profile in {
            ReleaseProfile.SIMULATION,
            ReleaseProfile.SOFTWARE,
            ReleaseProfile.PRODUCTION,
        }:
            failures.extend(benchmark.validate_release(self.layout.root))
            acceptance_path = self.layout.root / "acceptance" / "simulation.json"
            if not acceptance_path.is_file():
                failures.append("simulation acceptance manifest is missing")
            else:
                from .contracts import AcceptanceManifest

                acceptance = load_document(acceptance_path, AcceptanceManifest, self.layout.root)
                if acceptance.profile != ReleaseProfile.SIMULATION:
                    failures.append("simulation acceptance has the wrong profile")
                if acceptance.platform != "Ubuntu 24.04 x86_64":
                    failures.append("simulation acceptance is not from the authoritative platform")
                expected = {f"benchmark:{case.id:02d}-{case.slug}" for case in benchmark.cases}
                expected.update(
                    {
                        "cross-domain:five-backend",
                        "fmi:five-domain",
                        "backend:zephyr_build",
                        "backend:renode",
                        "backend:ngspice",
                        "backend:openmodelica",
                        "backend:ns3",
                        "backend:openems",
                    }
                )
                failures.extend(
                    validate_acceptance_manifest(self.layout.root, acceptance, expected)
                )
        if profile in {ReleaseProfile.SOFTWARE, ReleaseProfile.PRODUCTION}:
            from .contracts import AcceptanceManifest

            software_path = self.layout.root / "acceptance" / "software.json"
            if not software_path.is_file():
                failures.append("software production-topology acceptance manifest is missing")
            else:
                software = load_document(software_path, AcceptanceManifest, self.layout.root)
                if software.profile != ReleaseProfile.SOFTWARE:
                    failures.append("software acceptance has the wrong profile")
                software_expected = {
                    "deployment:compose",
                    "storage:postgres-s3",
                    "security:oidc-mtls",
                    "worker:lease-recovery",
                    "supply-chain:sbom-signature",
                }
                failures.extend(
                    validate_acceptance_manifest(self.layout.root, software, software_expected)
                )
        if profile == ReleaseProfile.PRODUCTION:
            production_models = self.store.count_models(ModelState.PRODUCTION_APPROVED)
            if production_models < 5:
                failures.append(f"production-approved capability packages: {production_models}/5")
            production_path = self.layout.root / "acceptance" / "production.json"
            if not production_path.is_file():
                failures.extend(
                    [
                        "five reference platforms have no current hardware differential bundles",
                        "instrument calibration and validation envelopes are unavailable",
                        "hardware and production capability packages require human approval",
                    ]
                )
            else:
                from .contracts import AcceptanceManifest

                production = load_document(production_path, AcceptanceManifest, self.layout.root)
                if production.profile != ReleaseProfile.PRODUCTION:
                    failures.append("production acceptance has the wrong profile")
                if not production.signature:
                    failures.append("production acceptance manifest is unsigned")
                production_expected = {
                    "hardware:stm32f407g-disc1",
                    "hardware:hifive1-revb",
                    "hardware:nrf52840-dk",
                    "hardware:esp32-s3-devkitc-1",
                    "hardware:rp2040-pico",
                    "calibration:lab",
                    "deployment:server-worker",
                    "security:mtls-oidc-recovery",
                    "license:approved",
                }
                failures.extend(
                    validate_acceptance_manifest(self.layout.root, production, production_expected)
                )
        return {
            "profile": profile,
            "release": "0.2.0-development-preview",
            "ready": not failures,
            "failures": failures,
            "boundary": "This check never infers hardware validation from simulation results.",
        }

    def run_benchmarks(
        self, case_ids: set[int] | None = None, source_revision: str = "working-tree"
    ) -> dict[str, Any]:
        manifest = BenchmarkRunner(self).run(case_ids, source_revision=source_revision)
        return manifest.model_dump(mode="json")

    def register_worker(self, registration: WorkerRegistration) -> dict[str, Any]:
        self.store.register_worker(registration)
        self.store.audit(
            registration.worker_id,
            "worker.register",
            registration.worker_id,
            json.dumps(
                {"capabilities": [item.name for item in registration.capabilities]},
                sort_keys=True,
            ),
        )
        return {"worker_id": registration.worker_id, "registered": True}

    def enqueue_task(self, task: WorkerTask) -> WorkerTask:
        return self.store.enqueue_task(task)

    def task_status(self, task_id: str) -> WorkerTask:
        task = self.store.task(task_id)
        if task is None:
            raise KeyError(f"unknown task: {task_id}")
        return task

    def lease_task(self, worker_id: str, lease_seconds: int = 60) -> WorkerTask | None:
        return self.store.lease_task(worker_id, lease_seconds)

    def heartbeat_task(
        self, worker_id: str, heartbeat: WorkerHeartbeat, lease_seconds: int = 60
    ) -> WorkerTask:
        return self.store.heartbeat_task(worker_id, heartbeat, lease_seconds)

    def complete_task(self, worker_id: str, result: WorkerTaskResult) -> WorkerTask:
        task = self.store.complete_task(worker_id, result)
        self.store.audit(worker_id, "task.complete", result.task_id, result.status)
        return task

    def cancel_task(self, task_id: str) -> WorkerTask:
        return self.store.cancel_task(task_id)

    def classify(self, path: Path) -> dict[str, Any]:
        problem = load_document(path, ProblemSpec, self.layout.root)
        return classify_problem(problem, self.catalog).model_dump(mode="json")

    def validate_experiment(self, path: Path) -> dict[str, Any]:
        experiment = load_document(path, ExperimentSpec, self.layout.root)
        system_path = resolve_workspace_path(self.layout.root, experiment.system, must_exist=True)
        system = load_document(system_path, SystemManifest, self.layout.root)
        warnings = validate_fmi_topology(system)
        schedule = build_schedule(system, experiment.macro_step_us)
        if experiment.requires_rollback and schedule.non_rollback:
            raise ValueError(
                "experiment requires rollback but components are non-rollback: "
                + ", ".join(sorted(schedule.non_rollback))
            )
        capabilities = {
            component.id: self.catalog.probe(component.backend) for component in system.components
        }
        missing = {
            component_id: probe.reason
            for component_id, probe in capabilities.items()
            if not probe.available
        }
        self._validate_model_states(system, experiment)
        return {
            "valid": True,
            "runnable": not missing,
            "experiment": experiment.name,
            "system": system.name,
            "communication_step_us": schedule.communication_step_us,
            "event_driven": sorted(schedule.event_driven),
            "non_rollback": sorted(schedule.non_rollback),
            "missing_capabilities": missing,
            "warnings": warnings,
        }

    def run_experiment(self, path: Path, cancel_event: threading.Event | None = None) -> RunResult:
        safe_path = resolve_workspace_path(self.layout.root, path, must_exist=True)
        run_id = uuid.uuid4().hex
        self.store.create_run(run_id, str(safe_path.relative_to(self.layout.root)))
        return self._execute(run_id, safe_path, cancel_event)

    def start_experiment(self, path: Path) -> str:
        safe_path = resolve_workspace_path(self.layout.root, path, must_exist=True)
        run_id = uuid.uuid4().hex
        self.store.create_run(run_id, str(safe_path.relative_to(self.layout.root)))
        with self._lock:
            self._futures[run_id] = self._executor.submit(self._execute, run_id, safe_path)
        return run_id

    def _execute(
        self,
        run_id: str,
        path: Path,
        cancel_event: threading.Event | None = None,
    ) -> RunResult:
        self.store.update_run(run_id, RunStatus.RUNNING)
        try:
            experiment = load_document(path, ExperimentSpec, self.layout.root)
            system_path = resolve_workspace_path(
                self.layout.root, experiment.system, must_exist=True
            )
            system = load_document(system_path, SystemManifest, self.layout.root)
            validate_fmi_topology(system)
            schedule = build_schedule(system, experiment.macro_step_us)
            if experiment.requires_rollback and schedule.non_rollback:
                raise ValueError(
                    "rollback requested for non-rollback components: "
                    + ", ".join(sorted(schedule.non_rollback))
                )
            self._validate_model_states(system, experiment)
            result = self.scheduler.run(run_id, experiment, system, cancel_event)
            self.store.update_run(
                run_id,
                result.status,
                evidence_path=str(result.evidence_dir.relative_to(self.layout.root)),
                error=result.error,
            )
            return result
        except Exception as exception:
            error = f"{type(exception).__name__}: {exception}"
            self.store.update_run(run_id, RunStatus.FAILED, error=error)
            return RunResult(run_id, RunStatus.FAILED, self.layout.runs_dir / run_id, error)

    def status(self, run_id: str) -> dict[str, Any]:
        record = self.store.get_run(run_id)
        if record is None:
            raise KeyError(f"unknown run: {run_id}")
        return record

    def get_evidence(self, run_id: str, artifact: str = "bundle.json") -> Any:
        record = self.status(run_id)
        if not record["evidence_path"]:
            return {"run_id": run_id, "status": record["status"], "ready": False}
        path = resolve_workspace_path(
            self.layout.root, Path(record["evidence_path"]) / artifact, must_exist=True
        )
        if path.suffix == ".json":
            return json.loads(path.read_text(encoding="utf-8"))
        return path.read_text(encoding="utf-8")

    def get_event_page(self, run_id: str, offset: int = 0, limit: int = 100) -> dict[str, Any]:
        if offset < 0 or not 1 <= limit <= 1000:
            raise ValueError("offset must be >= 0 and limit must be 1..1000")
        record = self.status(run_id)
        if not record["evidence_path"]:
            return {"events": [], "next_offset": None, "ready": False}
        path = resolve_workspace_path(
            self.layout.root, Path(record["evidence_path"]) / "events.jsonl", must_exist=True
        )
        lines = path.read_text(encoding="utf-8").splitlines()
        page = [json.loads(line) for line in lines[offset : offset + limit]]
        next_offset = offset + len(page) if offset + len(page) < len(lines) else None
        return {"events": page, "next_offset": next_offset, "total": len(lines)}

    def replay(self, run_id: str) -> RunResult:
        record = self.status(run_id)
        if not record["evidence_path"]:
            raise RuntimeError("run has no resolved evidence to replay")
        experiment_path = Path(record["evidence_path"]) / "experiment.resolved.json"
        return self.run_experiment(experiment_path)

    def compare(self, left: str, right: str) -> dict[str, Any]:
        left_bundle = EvidenceBundle.model_validate(self.get_evidence(left))
        right_bundle = EvidenceBundle.model_validate(self.get_evidence(right))
        left_events = self.get_evidence(left, "events.jsonl")
        right_events = self.get_evidence(right, "events.jsonl")
        left_hash = hashlib.sha256(left_events.encode()).hexdigest()
        right_hash = hashlib.sha256(right_events.encode()).hexdigest()
        return {
            "left": left,
            "right": right,
            "status_changed": left_bundle.status != right_bundle.status,
            "left_status": left_bundle.status,
            "right_status": right_bundle.status,
            "event_count_delta": right_bundle.event_count - left_bundle.event_count,
            "trace_hash_equal": left_hash == right_hash,
            "left_trace_hash": left_hash,
            "right_trace_hash": right_hash,
            "assertions_changed": (left_bundle.assertion_results != right_bundle.assertion_results),
        }

    def explain(self, run_id: str) -> dict[str, Any]:
        record = self.status(run_id)
        if not record["evidence_path"]:
            return {"run_id": run_id, "status": record["status"], "error": record["error"]}
        bundle = EvidenceBundle.model_validate(self.get_evidence(run_id))
        failures = [item for item in bundle.assertion_results if not item["passed"]]
        page = self.get_event_page(run_id, max(0, bundle.event_count - 20), 20)
        return {
            "run_id": run_id,
            "status": bundle.status,
            "failed_assertions": failures,
            "key_events": page["events"],
            "fidelity_boundaries": bundle.fidelity_boundaries,
            "causal_analysis": (
                "Recorded parent_sequence links are reported; causal attribution beyond those "
                "links remains a hypothesis until independently validated."
            ),
        }

    def _validate_model_states(self, system: SystemManifest, experiment: ExperimentSpec) -> None:
        for component in system.components:
            raw_state = component.properties.get("model_state")
            if raw_state is None:
                continue
            state = ModelState(raw_state)
            if state not in experiment.allowed_model_states:
                raise PermissionError(
                    f"component {component.id} uses model state {state}, not allowed by experiment"
                )
