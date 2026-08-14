from __future__ import annotations

from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from ael.contracts import (
    TaskStatus,
    WorkerCapability,
    WorkerHeartbeat,
    WorkerKind,
    WorkerRegistration,
    WorkerTask,
    WorkerTaskResult,
)
from ael.storage import StateStore, WorkspaceLayout
from ael.worker import OutboundWorker


def registration() -> WorkerRegistration:
    return WorkerRegistration(
        worker_id="worker-1",
        worker_kind=WorkerKind.SIMULATION,
        capabilities=[WorkerCapability(name="renode", version="1.16.1", kind="backend")],
        agent_version="test",
        certificate_fingerprint="a" * 64,
    )


def task() -> WorkerTask:
    return WorkerTask(
        task_id="task-1",
        task_type="experiment",
        required_capabilities=["renode"],
        payload={"experiment_path": "case.yaml"},
        idempotency_key="key-1",
    )


def test_worker_lease_heartbeat_completion_and_idempotency(tmp_path: Path) -> None:
    store = StateStore(WorkspaceLayout(tmp_path))
    store.register_worker(registration())
    assert store.enqueue_task(task()).task_id == "task-1"
    duplicate = task().model_copy(update={"task_id": "task-duplicate"})
    assert store.enqueue_task(duplicate).task_id == "task-1"
    leased = store.lease_task("worker-1", lease_seconds=60)
    assert leased and leased.status == TaskStatus.LEASED and leased.lease_token
    running = store.heartbeat_task(
        "worker-1",
        WorkerHeartbeat(
            task_id=leased.task_id,
            lease_token=leased.lease_token,
            status="running",
            progress=0.5,
        ),
    )
    assert running.status == TaskStatus.RUNNING
    completed = store.complete_task(
        "worker-1",
        WorkerTaskResult(
            task_id=leased.task_id,
            lease_token=leased.lease_token,
            status="succeeded",
            artifact_hashes={"bundle": "b" * 64},
        ),
    )
    assert completed.status == TaskStatus.SUCCEEDED
    with pytest.raises(PermissionError):
        store.complete_task(
            "worker-1",
            WorkerTaskResult(
                task_id=leased.task_id,
                lease_token="wrong",
                status="succeeded",
            ),
        )


def test_expired_lease_is_recovered_by_an_eligible_worker(tmp_path: Path) -> None:
    store = StateStore(WorkspaceLayout(tmp_path))
    store.register_worker(registration())
    second = registration().model_copy(update={"worker_id": "worker-2"})
    store.register_worker(second)
    store.enqueue_task(task())
    first = store.lease_task("worker-1", lease_seconds=60)
    assert first and first.attempts == 1
    with store.connection() as connection:
        connection.execute(
            "UPDATE tasks SET lease_expires_at = ? WHERE task_id = ?",
            ((datetime.now(UTC) - timedelta(seconds=1)).isoformat(), first.task_id),
        )
    recovered = store.lease_task("worker-2", lease_seconds=60)
    assert recovered and recovered.lease_owner == "worker-2"
    assert recovered.attempts == 2


def test_outbound_worker_retries_transient_registration(monkeypatch) -> None:
    worker = object.__new__(OutboundWorker)
    worker.poll_seconds = 0
    worker.registration = registration()
    attempts = 0

    def register() -> None:
        nonlocal attempts
        attempts += 1
        if attempts == 1:
            import httpx

            raise httpx.ConnectError("transient")

    class Response:
        def raise_for_status(self) -> None:
            raise KeyboardInterrupt

    class Client:
        def post(self, path: str) -> Response:
            return Response()

    worker.register = register
    worker.client = Client()
    monkeypatch.setattr("ael.worker.time.sleep", lambda _: None)
    with pytest.raises(KeyboardInterrupt):
        worker.run_forever()
    assert attempts == 2
