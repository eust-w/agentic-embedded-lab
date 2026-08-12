from __future__ import annotations

from pathlib import Path

import pytest

from ael.contracts import ModelState
from ael.modeling import ModelRegistry
from ael.storage import WorkspaceLayout


def test_svd_generation_and_governed_lifecycle(workspace: Path) -> None:
    registry = ModelRegistry(WorkspaceLayout(workspace))
    result = registry.generate(Path("examples/models/minimal-request.yaml"))
    assert result.package.state == ModelState.GENERATED
    assert result.ir_path.exists()
    static = registry.static_validate(result.package.id, actor="test-agent")
    assert static.state == ModelState.STATIC_VALIDATED
    evidence = workspace / "tests/register-conformance.json"
    evidence.parent.mkdir()
    evidence.write_text("{}\n", encoding="utf-8")
    conformance = registry.transition(
        result.package.id,
        ModelState.CONFORMANCE_VALIDATED,
        actor="test-agent",
        evidence=["tests/register-conformance.json"],
    )
    assert conformance.state == ModelState.CONFORMANCE_VALIDATED
    with pytest.raises(PermissionError, match="human approval"):
        registry.transition(
            result.package.id,
            ModelState.HARDWARE_VALIDATED,
            actor="test-agent",
            evidence=["trace.json"],
        )


def test_production_approval_requires_signature(workspace: Path) -> None:
    registry = ModelRegistry(WorkspaceLayout(workspace))
    result = registry.generate(Path("examples/models/minimal-request.yaml"))
    registry.static_validate(result.package.id, actor="test")
    conformance_path = workspace / "conformance.json"
    conformance_path.write_text("{}\n", encoding="utf-8")
    registry.transition(
        result.package.id,
        ModelState.CONFORMANCE_VALIDATED,
        actor="test",
        evidence=[str(conformance_path.relative_to(workspace))],
    )
    hardware_path = workspace / "hardware-diff.json"
    hardware_path.write_text("{}\n", encoding="utf-8")
    registry.transition(
        result.package.id,
        ModelState.HARDWARE_VALIDATED,
        actor="human",
        human_approved=True,
        evidence=[str(hardware_path.relative_to(workspace))],
    )
    review_path = workspace / "review.json"
    review_path.write_text("{}\n", encoding="utf-8")
    with pytest.raises(ValueError, match="signature"):
        registry.transition(
            result.package.id,
            ModelState.PRODUCTION_APPROVED,
            actor="human",
            human_approved=True,
            evidence=[str(review_path.relative_to(workspace))],
        )


def test_static_validation_detects_changed_grounding(workspace: Path) -> None:
    registry = ModelRegistry(WorkspaceLayout(workspace))
    result = registry.generate(Path("examples/models/minimal-request.yaml"))
    source = workspace / "examples/models/minimal.svd"
    source.write_text(source.read_text(encoding="utf-8") + "\n", encoding="utf-8")
    with pytest.raises(ValueError, match="grounding source changed"):
        registry.static_validate(result.package.id, actor="test")


def test_systemrdl_generation_emits_valid_ir_and_renode_source(workspace: Path) -> None:
    registry = ModelRegistry(WorkspaceLayout(workspace))
    result = registry.generate(Path("examples/models/minimal-rdl-request.yaml"))
    assert result.package.state == ModelState.GENERATED
    assert result.package.generated_by == "ael.systemrdl-importer/v1"
    source = workspace / result.package.artifact_paths[0]
    text = source.read_text(encoding="utf-8")
    assert "BasicDoubleWordPeripheral" in text
    assert "WithFlag" in text
