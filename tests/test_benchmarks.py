from __future__ import annotations

from pathlib import Path

from ael.benchmarks import load_catalog
from ael.contracts import ExperimentSpec, ReleaseProfile, SystemManifest
from ael.fmi import validate_fmi_topology
from ael.io import load_document
from ael.service import AelService


def test_catalog_has_all_24_ordered_cases(workspace: Path) -> None:
    catalog = load_catalog(workspace)
    assert [case.id for case in catalog.cases] == list(range(1, 25))
    assert len({case.slug for case in catalog.cases}) == 24


def test_catalog_has_complete_executable_asset_contracts(workspace: Path) -> None:
    failures = load_catalog(workspace).validate_release()
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
        system = load_document(Path(fixed.system), SystemManifest, workspace)
        validate_fmi_topology(system)
