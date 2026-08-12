from __future__ import annotations

from pathlib import Path

from fastapi.testclient import TestClient

from ael.api import create_app


def test_api_starts_and_reads_async_experiment(workspace: Path) -> None:
    with TestClient(create_app(workspace)) as client:
        response = client.post(
            "/v1/experiments", json={"path": "examples/experiments/synthetic-smoke.yaml"}
        )
        assert response.status_code == 200
        run_id = response.json()["run_id"]
        status = client.get(f"/v1/experiments/{run_id}")
        assert status.status_code == 200
        assert status.json()["status"] in {"queued", "running", "passed"}


def test_api_rejects_workspace_escape(workspace: Path) -> None:
    with TestClient(create_app(workspace)) as client:
        response = client.post("/v1/problems/classify", json={"path": "../problem.yaml"})
        assert response.status_code == 422


def test_worker_routes_require_matching_verified_fingerprint(
    workspace: Path, monkeypatch
) -> None:
    monkeypatch.setenv("AEL_ALLOW_INSECURE_WORKER_TESTS", "1")
    fingerprint = "d" * 64
    registration = {
        "api_version": "ael.dev/v1",
        "kind": "WorkerRegistration",
        "worker_id": "api-worker",
        "worker_kind": "simulation",
        "capabilities": [
            {"name": "renode", "version": "1.16.1", "kind": "backend"}
        ],
        "agent_version": "test",
        "certificate_fingerprint": fingerprint,
    }
    with TestClient(create_app(workspace)) as client:
        denied = client.post("/v1/workers/register", json=registration)
        assert denied.status_code == 401
        accepted = client.post(
            "/v1/workers/register",
            json=registration,
            headers={"X-Client-Cert-SHA256": fingerprint},
        )
        assert accepted.status_code == 200
        task = {
            "api_version": "ael.dev/v1",
            "kind": "WorkerTask",
            "task_id": "api-task",
            "task_type": "experiment",
            "required_capabilities": ["renode"],
            "payload": {"experiment_path": "case.yaml"},
            "idempotency_key": "api-key",
            "status": "queued",
            "attempts": 0,
        }
        assert client.post("/v1/tasks", json=task).status_code == 200
        lease = client.post(
            "/v1/workers/api-worker/lease",
            headers={"X-Client-Cert-SHA256": fingerprint},
        )
        assert lease.status_code == 200
        assert lease.json()["task_id"] == "api-task"
