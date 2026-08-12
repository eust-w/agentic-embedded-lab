from __future__ import annotations

import hashlib
import threading
from pathlib import Path

from ael.contracts import RunStatus
from ael.service import AelService

EXPERIMENT = Path("examples/experiments/synthetic-smoke.yaml")


def trace_hash(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def test_synthetic_run_writes_complete_evidence(workspace: Path) -> None:
    result = AelService(workspace).run_experiment(EXPERIMENT)
    assert result.status == RunStatus.PASSED
    expected = {
        "manifest.resolved.json",
        "experiment.resolved.json",
        "provenance.json",
        "events.jsonl",
        "assertions.json",
        "junit.xml",
        "summary.md",
        "bundle.json",
        "snapshots",
    }
    assert expected <= {path.name for path in result.evidence_dir.iterdir()}
    bundle = AelService(workspace).get_evidence(result.run_id)
    assert bundle["status"] == "passed"
    assert bundle["claims"][0]["status"] == "unverified"
    assert bundle["event_count"] > 0


def test_same_seed_has_identical_trace(workspace: Path) -> None:
    service = AelService(workspace)
    hashes = {
        trace_hash(service.run_experiment(EXPERIMENT).evidence_dir / "events.jsonl")
        for _ in range(20)
    }
    assert len(hashes) == 1


def test_missing_backend_blocks_without_mock(workspace: Path) -> None:
    system = workspace / "examples/systems/synthetic-loop.yaml"
    text = system.read_text(encoding="utf-8").replace("backend: synthetic", "backend: renode", 1)
    system.write_text(text, encoding="utf-8")
    result = AelService(workspace).run_experiment(EXPERIMENT)
    assert result.status == RunStatus.BLOCKED
    assert "adapter" in (result.error or "") or "not found" in (result.error or "")


def test_replay_and_compare(workspace: Path) -> None:
    service = AelService(workspace)
    first = service.run_experiment(EXPERIMENT)
    replay = service.replay(first.run_id)
    comparison = service.compare(first.run_id, replay.run_id)
    assert comparison["trace_hash_equal"] is True
    assert comparison["status_changed"] is False


def test_pre_cancelled_run_stops_safely_with_evidence(workspace: Path) -> None:
    cancel = threading.Event()
    cancel.set()
    result = AelService(workspace).run_experiment(EXPERIMENT, cancel_event=cancel)
    assert result.status == RunStatus.CANCELLED
    assert (result.evidence_dir / "bundle.json").is_file()
    assert "cancelled" in (result.error or "")
