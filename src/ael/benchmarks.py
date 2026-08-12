from __future__ import annotations

import json
from datetime import UTC, datetime
from pathlib import Path
from typing import TYPE_CHECKING, Any

from pydantic import Field

from .contracts import (
    AcceptanceEntry,
    AcceptanceManifest,
    BackendName,
    ProblemCategory,
    ReleaseProfile,
    RunStatus,
    StrictModel,
)
from .io import load_document, sha256_file, write_json
from .security import resolve_workspace_path

if TYPE_CHECKING:
    from .service import AelService


class BenchmarkCase(StrictModel):
    id: int = Field(ge=1, le=24)
    slug: str
    title: str
    category: ProblemCategory
    backends: list[BackendName] = Field(min_length=1)
    readiness: str
    faulty_asset: str | None = None
    fixed_asset: str | None = None
    experiment: str | None = None
    faulty_experiment: str | None = None
    fixed_experiment: str | None = None
    causal_chain: list[str] = Field(default_factory=list)
    seed: int = Field(default=0, ge=0)
    hardware_target: str | None = None
    fidelity_boundary: str


class BenchmarkCatalog(StrictModel):
    version: str
    cases: list[BenchmarkCase]

    def validate_release(self) -> list[str]:
        failures: list[str] = []
        ids = [case.id for case in self.cases]
        if ids != list(range(1, 25)):
            failures.append("catalog must contain ordered benchmark ids 1..24")
        for case in self.cases:
            if case.readiness != "executable":
                failures.append(f"{case.id:02d}-{case.slug}: readiness={case.readiness}")
            if not all(
                [
                    case.faulty_asset,
                    case.fixed_asset,
                    case.faulty_experiment,
                    case.fixed_experiment,
                    case.causal_chain,
                ]
            ):
                failures.append(f"{case.id:02d}-{case.slug}: missing executable assets")
        return failures


def load_catalog(workspace: Path) -> BenchmarkCatalog:
    return load_document(workspace / "benchmarks/catalog.yaml", BenchmarkCatalog, workspace)


class BenchmarkRunner:
    def __init__(self, service: AelService) -> None:
        self.service = service
        self.workspace = service.layout.root

    def run_case(self, case: BenchmarkCase) -> AcceptanceEntry:
        assert case.faulty_experiment and case.fixed_experiment
        assert case.faulty_asset and case.fixed_asset
        evidence_dir = self.workspace / "acceptance" / "evidence"
        evidence_dir.mkdir(parents=True, exist_ok=True)
        checks: list[dict[str, Any]] = []
        for label, relative_path, expected in (
            ("faulty", case.faulty_experiment, RunStatus.FAILED),
            ("fixed", case.fixed_experiment, RunStatus.PASSED),
        ):
            path = resolve_workspace_path(self.workspace, relative_path, must_exist=True)
            result = self.service.run_experiment(path)
            checks.append(
                {
                    "variant": label,
                    "run_id": result.run_id,
                    "status": result.status,
                    "expected_status": expected,
                    "evidence_dir": str(result.evidence_dir.relative_to(self.workspace)),
                    "passed": result.status == expected,
                    "error": result.error,
                }
            )
        asset_hashes = {
            "faulty": sha256_file(
                resolve_workspace_path(self.workspace, case.faulty_asset, must_exist=True)
            ),
            "fixed": sha256_file(
                resolve_workspace_path(self.workspace, case.fixed_asset, must_exist=True)
            ),
        }
        payload = {
            "benchmark": f"{case.id:02d}-{case.slug}",
            "checks": checks,
            "asset_hashes": asset_hashes,
            "causal_chain": case.causal_chain,
            "fidelity_boundary": case.fidelity_boundary,
            "hardware_validated": False,
        }
        evidence_path = evidence_dir / f"{case.id:02d}-{case.slug}.json"
        write_json(evidence_path, payload)
        passed = all(item["passed"] for item in checks)
        return AcceptanceEntry(
            name=f"benchmark:{case.id:02d}-{case.slug}",
            status="passed" if passed else "failed",
            evidence_path=str(evidence_path.relative_to(self.workspace)),
            evidence_sha256=sha256_file(evidence_path),
            limitations=[case.fidelity_boundary, "No physical hardware evidence was produced."],
        )

    def run(
        self,
        case_ids: set[int] | None = None,
        *,
        source_revision: str = "working-tree",
    ) -> AcceptanceManifest:
        catalog = load_catalog(self.workspace)
        selected = [case for case in catalog.cases if not case_ids or case.id in case_ids]
        entries = [self.run_case(case) for case in selected]
        if any(case.id == 24 for case in selected):
            case_24 = next(item for item in entries if item.name.startswith("benchmark:24-"))
            entries.append(
                AcceptanceEntry(
                    name="cross-domain:five-backend",
                    status=case_24.status,
                    evidence_path=case_24.evidence_path,
                    evidence_sha256=case_24.evidence_sha256,
                    limitations=[
                        "Functional five-domain co-simulation only; no calibrated equivalence."
                    ],
                )
            )
        if case_ids is None:
            entry_by_name = {entry.name: entry for entry in entries}
            for backend in (
                BackendName.RENODE,
                BackendName.NGSPICE,
                BackendName.MODELICA,
                BackendName.NS3,
                BackendName.OPENEMS,
            ):
                relevant = [case for case in selected if backend in case.backends]
                source_entries = [
                    entry_by_name[f"benchmark:{case.id:02d}-{case.slug}"] for case in relevant
                ]
                status = (
                    "passed"
                    if source_entries
                    and all(entry.status == "passed" for entry in source_entries)
                    else "failed"
                )
                payload = {
                    "backend": backend,
                    "cases": [entry.name for entry in source_entries],
                    "case_evidence_sha256": {
                        entry.name: entry.evidence_sha256 for entry in source_entries
                    },
                    "status": status,
                    "hardware_validated": False,
                }
                backend_path = (
                    self.workspace / "acceptance/evidence" / f"backend-{backend.value}.json"
                )
                write_json(backend_path, payload)
                entries.append(
                    AcceptanceEntry(
                        name=f"backend:{backend.value}",
                        status=status,
                        evidence_path=str(backend_path.relative_to(self.workspace)),
                        evidence_sha256=sha256_file(backend_path),
                        limitations=["Backend conformance; no hardware equivalence."],
                    )
                )
        manifest = AcceptanceManifest(
            profile=ReleaseProfile.SIMULATION,
            source_revision=source_revision,
            platform="Ubuntu 24.04 x86_64",
            entries=entries,
            created_at=datetime.now(UTC),
        )
        destination = self.workspace / "acceptance" / "simulation.json"
        write_json(destination, manifest)
        return manifest


def validate_acceptance_manifest(
    workspace: Path, manifest: AcceptanceManifest, expected_names: set[str]
) -> list[str]:
    failures: list[str] = []
    entries = {entry.name: entry for entry in manifest.entries}
    for name in sorted(expected_names):
        entry = entries.get(name)
        if entry is None:
            failures.append(f"missing acceptance entry: {name}")
            continue
        if entry.status != "passed":
            failures.append(f"acceptance entry not passed: {name}={entry.status}")
            continue
        if not entry.evidence_path or not entry.evidence_sha256:
            failures.append(f"acceptance entry has no hashed evidence: {name}")
            continue
        try:
            path = resolve_workspace_path(workspace, entry.evidence_path, must_exist=True)
        except (FileNotFoundError, PermissionError, ValueError) as exception:
            failures.append(f"acceptance evidence is unavailable: {name}: {exception}")
            continue
        if sha256_file(path) != entry.evidence_sha256:
            failures.append(f"acceptance evidence digest mismatch: {name}")
            continue
        try:
            json.loads(path.read_text(encoding="utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exception:
            failures.append(f"acceptance evidence is not valid JSON: {name}: {exception}")
    return failures
