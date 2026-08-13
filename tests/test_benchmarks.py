from __future__ import annotations

from pathlib import Path

from typer.testing import CliRunner

from ael.benchmarks import load_catalog
from ael.cli import app
from ael.contracts import ExperimentSpec, ReleaseProfile, SystemManifest
from ael.fmi import validate_fmi_topology
from ael.io import load_document
from ael.service import AelService


def test_catalog_has_all_24_ordered_cases(workspace: Path) -> None:
    catalog = load_catalog(workspace)
    assert [case.id for case in catalog.cases] == list(range(1, 25))
    assert len({case.slug for case in catalog.cases}) == 24


def test_catalog_has_complete_executable_asset_contracts(workspace: Path) -> None:
    failures = load_catalog(workspace).validate_release(workspace)
    assert failures == []


def test_simulation_gate_requires_actions_acceptance_evidence(workspace: Path) -> None:
    result = AelService(workspace).release_check(ReleaseProfile.SIMULATION)
    assert result["ready"] is False
    assert "simulation acceptance manifest is missing" in result["failures"]


def test_all_faulty_fixed_specs_and_systems_are_strictly_valid(workspace: Path) -> None:
    for case in load_catalog(workspace).cases:
        assert case.faulty_experiment and case.fixed_experiment
        faulty = load_document(Path(case.faulty_experiment), ExperimentSpec, workspace)
        fixed = load_document(Path(case.fixed_experiment), ExperimentSpec, workspace)
        assert faulty.seed == fixed.seed == case.seed
        assert "faulty" in faulty.tags and "fixed" in fixed.tags
        assert faulty.assertions == fixed.assertions
        assert "mcu.fixed" not in Path(case.faulty_experiment).read_text(encoding="utf-8")
        assert "fault_scale" not in Path(case.fixed_experiment).read_text(encoding="utf-8")
        assert case.mechanism.execution_backend in case.backends
        system = load_document(Path(fixed.system), SystemManifest, workspace)
        validate_fmi_topology(system)


def test_benchmark_cli_accepts_serialized_passing_manifest(workspace: Path, monkeypatch) -> None:
    class PassingService:
        def run_benchmarks(self, case_ids: set[int] | None, source_revision: str):
            assert case_ids == {1}
            assert source_revision == "test-revision"
            return {"entries": [{"name": "benchmark:01", "status": "passed"}]}

    monkeypatch.setattr("ael.cli.service", lambda _workspace: PassingService())
    result = CliRunner().invoke(
        app,
        [
            "benchmark",
            "run",
            "--case-id",
            "1",
            "--source-revision",
            "test-revision",
            "--workspace",
            str(workspace),
        ],
    )
    assert result.exit_code == 0, result.output


def test_benchmark_cli_fails_closed_for_serialized_failure(workspace: Path, monkeypatch) -> None:
    class FailingService:
        def run_benchmarks(self, case_ids: set[int] | None, source_revision: str):
            return {"entries": [{"name": "benchmark:01", "status": "failed"}]}

    monkeypatch.setattr("ael.cli.service", lambda _workspace: FailingService())
    result = CliRunner().invoke(
        app,
        ["benchmark", "run", "--case-id", "1", "--workspace", str(workspace)],
    )
    assert result.exit_code == 2


def test_five_domain_case_asserts_every_backend_failure_signal(workspace: Path) -> None:
    case = next(item for item in load_catalog(workspace).cases if item.id == 24)
    assert case.fixed_experiment
    experiment = load_document(Path(case.fixed_experiment), ExperimentSpec, workspace)
    assert {assertion.metric for assertion in experiment.assertions} == {
        "antenna.failure",
        "network.failure",
        "mcu.failure",
        "circuit.failure",
        "plant.failure",
    }
