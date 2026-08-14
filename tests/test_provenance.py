from __future__ import annotations

from ael.provenance import (
    RELEASE_AUTHORITY_POLICY,
    capture_execution_environment,
    detect_platform,
    validate_execution_environment,
)


def test_detect_platform_never_returns_an_empty_label() -> None:
    assert detect_platform().strip()


def test_release_authority_is_evidence_based_and_ci_independent() -> None:
    assert RELEASE_AUTHORITY_POLICY == "qualified-execution-evidence"


def test_native_linux_environment_can_qualify_without_ci(monkeypatch) -> None:
    monkeypatch.setattr("ael.provenance.platform.system", lambda: "Linux")
    monkeypatch.setattr("ael.provenance.platform.machine", lambda: "x86_64")
    payload = capture_execution_environment(
        [
            {
                "backend": "renode",
                "available": True,
                "detected_version": "1.16.1",
                "expected_version": "1.16.1",
                "reason": None,
            }
        ],
        source_revision="a" * 40,
    )
    assert payload["qualified"] is True
    assert payload["ci_required"] is False
    assert validate_execution_environment(payload, "a" * 40) == []


def test_working_tree_environment_fails_closed(monkeypatch) -> None:
    monkeypatch.setattr("ael.provenance.platform.system", lambda: "Linux")
    payload = capture_execution_environment([], source_revision="working-tree")
    assert payload["qualified"] is False
    assert "source revision is not immutable" in payload["failures"]
