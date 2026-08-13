from __future__ import annotations

from pathlib import Path

from ael.contracts import ReleaseProfile
from ael.service import AelService


def test_software_gate_fails_closed_without_machine_evidence(workspace: Path) -> None:
    result = AelService(workspace).release_check(ReleaseProfile.SOFTWARE)
    assert result["ready"] is False
    assert "software production-topology acceptance manifest is missing" in result["failures"]


def test_production_gate_keeps_physical_validation_explicit(workspace: Path) -> None:
    result = AelService(workspace).release_check(ReleaseProfile.PRODUCTION)
    joined = "\n".join(result["failures"])
    assert "five reference platforms" in joined
    assert "instrument calibration" in joined
    assert "human approval" in joined
    assert result["boundary"].startswith("This check never infers hardware validation")
