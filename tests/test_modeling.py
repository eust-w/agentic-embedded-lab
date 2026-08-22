from __future__ import annotations

from pathlib import Path

import pytest

from ael.contracts import (
    GenerationReceipt,
    GroundingManifest,
    ModelConformanceEvidence,
    ModelState,
)
from ael.io import write_json
from ael.modeling import ModelRegistry
from ael.storage import WorkspaceLayout


def conformance_report(workspace: Path, model_id: str, name: str = "conformance.json") -> Path:
    path = workspace / name
    write_json(
        path,
        ModelConformanceEvidence(
            model_id=model_id,
            validator="ael-independent-test/v1",
            source_independent=True,
            register_layout_passed=True,
            compile_passed=True,
            driver_tests_passed=True,
            property_tests_passed=True,
            reference_trace_passed=True,
            sandbox_network="none",
            sandbox_read_only=True,
            artifact_hashes={"trace": "a" * 64},
        ),
    )
    return path


def test_svd_generation_and_governed_lifecycle(workspace: Path) -> None:
    registry = ModelRegistry(WorkspaceLayout(workspace))
    result = registry.generate(Path("examples/models/minimal-request.yaml"))
    assert result.package.state == ModelState.GENERATED
    assert result.ir_path.exists()
    static = registry.static_validate(result.package.id, actor="test-agent")
    assert static.state == ModelState.STATIC_VALIDATED
    evidence = conformance_report(workspace, result.package.id, "register-conformance.json")
    conformance = registry.conformance_validate(
        result.package.id,
        actor="test-agent",
        evidence=[str(evidence.relative_to(workspace))],
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
    conformance_path = conformance_report(workspace, result.package.id)
    registry.conformance_validate(
        result.package.id,
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


@pytest.mark.parametrize("provider", ["openai", "anthropic"])
def test_grounded_provider_replay_is_auditable(
    workspace: Path, monkeypatch: pytest.MonkeyPatch, provider: str
) -> None:
    fixture_dir = Path(__file__).parent / "fixtures/providers"
    monkeypatch.setenv("AEL_MODEL_PROVIDER_REPLAY_DIR", str(fixture_dir))
    registry = ModelRegistry(WorkspaceLayout(workspace))
    request = Path(f"examples/models/grounded-{provider}-request.yaml")
    result = registry.generate(request)
    assert result.package.state == ModelState.GENERATED
    assert result.package.grounding_manifest_path
    assert result.package.generation_receipt_path
    grounding = GroundingManifest.model_validate_json(
        (workspace / result.package.grounding_manifest_path).read_text(encoding="utf-8")
    )
    receipt = GenerationReceipt.model_validate_json(
        (workspace / result.package.generation_receipt_path).read_text(encoding="utf-8")
    )
    assert grounding.sources[0].locator == "lines:1-3"
    assert receipt.provider == provider
    assert receipt.recorded is True
    assert receipt.provider_request_id


def test_conformance_requires_evidence_outside_generated_model(workspace: Path) -> None:
    registry = ModelRegistry(WorkspaceLayout(workspace))
    result = registry.generate(Path("examples/models/minimal-request.yaml"))
    registry.static_validate(result.package.id, actor="test")
    with pytest.raises(ValueError, match="independent"):
        registry.conformance_validate(
            result.package.id,
            actor="test",
            evidence=[result.package.ir_path or ""],
        )


def test_openai_provider_chat_completions_mode(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    from ael.contracts import ModelGenerationConfig
    from ael.model_providers import OpenAIProvider

    captured_url = []

    def fake_post(url, headers, payload):
        captured_url.append(url)
        ir_json = (
            '{"api_version":"ael.dev/v1","kind":"HardwareBehaviorIR",'
            '"name":"TestPerip","bus_width":32,"size":256,"registers":[],"grounding":{}}'
        )
        return {
            "id": "chatcmpl-test",
            "choices": [{"message": {"content": ir_json}}],
        }, "req-test"

    monkeypatch.setattr("ael.model_providers._post_json", fake_post)
    monkeypatch.setenv("OPENAI_API_KEY", "sk-test")
    monkeypatch.setenv("AEL_OPENAI_API_MODE", "chat_completions")
    provider = OpenAIProvider()
    res = provider.generate("test prompt", ModelGenerationConfig(provider="openai", model="gpt-4o"))
    assert res.ir.name == "TestPerip"
    assert captured_url[0].endswith("/chat/completions")
