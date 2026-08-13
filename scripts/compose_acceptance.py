#!/usr/bin/env python3
"""Exercise the local Compose control-plane/worker production topology."""

from __future__ import annotations

import argparse
import hashlib
import json
import ssl
import subprocess
import time
from pathlib import Path
from typing import Any

import boto3
import httpx
from botocore.config import Config


def run(compose: list[str], *arguments: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [*compose, *arguments], capture_output=True, text=True, check=True, timeout=180
    )


def wait_for(call: Any, *, timeout: float = 120.0) -> Any:
    deadline = time.monotonic() + timeout
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            value = call()
            if value:
                return value
        except Exception as error:  # service recovery is the behavior under test
            last_error = error
        time.sleep(1)
    raise TimeoutError(f"service did not recover: {last_error}")


def worker_fingerprint(certificate: Path) -> str:
    der = ssl.PEM_cert_to_DER_cert(certificate.read_text(encoding="utf-8"))
    return hashlib.sha256(der).hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--workspace", type=Path, default=Path.cwd())
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--project-name", default="ael-acceptance")
    args = parser.parse_args()
    workspace = args.workspace.resolve()
    certs = workspace / ".ael/dev-certs"
    compose = [
        "docker",
        "compose",
        "--project-name",
        args.project_name,
        "-f",
        str(workspace / "deploy/compose.yaml"),
    ]
    fingerprint = worker_fingerprint(certs / "worker.crt")
    user_base = "https://127.0.0.1:9443"
    worker_base = "https://127.0.0.1:8443"
    ca = str(certs / "ca.crt")
    worker_cert = (str(certs / "worker.crt"), str(certs / "worker.key"))
    worker_tls = ssl.create_default_context(cafile=ca)
    worker_tls.load_cert_chain(*worker_cert)
    oidc = httpx.Client(base_url="http://127.0.0.1:8080", timeout=10, trust_env=False)
    anonymous_user = httpx.Client(
        base_url=user_base, verify=ca, timeout=10, trust_env=False
    )
    anonymous_worker = httpx.Client(
        base_url=worker_base, verify=ca, timeout=10, trust_env=False
    )

    def token() -> str:
        response = oidc.post("/token")
        response.raise_for_status()
        return str(response.json()["access_token"])

    access_token = wait_for(token)
    user = httpx.Client(
        base_url=user_base,
        verify=ca,
        headers={"Authorization": f"Bearer {access_token}"},
        timeout=30,
        trust_env=False,
    )
    worker = httpx.Client(
        base_url=worker_base,
        verify=worker_tls,
        timeout=30,
        trust_env=False,
    )
    report: dict[str, Any] = {"profile": "software", "checks": {}}

    anonymous = anonymous_user.get("/v1/project")
    report["checks"]["oidc_rejects_anonymous"] = anonymous.status_code == 401
    project = wait_for(lambda: user.get("/v1/project"))
    project.raise_for_status()
    report["checks"]["oidc_authorized_project"] = project.json()["storage_mode"] == "server"
    no_cert_failed = False
    try:
        anonymous_worker.post("/v1/workers/no-cert/lease")
    except httpx.TransportError:
        no_cert_failed = True
    report["checks"]["mtls_rejects_missing_client_certificate"] = no_cert_failed

    task = {
        "api_version": "ael.dev/v1",
        "kind": "WorkerTask",
        "task_id": "compose-synthetic-task",
        "task_type": "experiment",
        "required_capabilities": ["synthetic"],
        "payload": {"experiment_path": "examples/experiments/synthetic-smoke.yaml"},
        "idempotency_key": "compose-synthetic-task-v1",
    }
    enqueue = user.post("/v1/tasks", json=task)
    enqueue.raise_for_status()

    def completed() -> dict[str, Any] | None:
        response = user.get("/v1/tasks/compose-synthetic-task")
        response.raise_for_status()
        payload = response.json()
        return payload if payload["status"] in {"succeeded", "failed"} else None

    completed_task = wait_for(completed, timeout=180)
    report["checks"]["worker_executes_and_uploads_evidence"] = completed_task[
        "status"
    ] == "succeeded" and bool(completed_task.get("result", {}).get("artifact_hashes"))

    registration = {
        "api_version": "ael.dev/v1",
        "kind": "WorkerRegistration",
        "worker_id": "recovery-worker-a",
        "worker_kind": "simulation",
        "capabilities": [{"name": "recovery", "version": "test", "kind": "backend"}],
        "agent_version": "0.2.0.dev0",
        "certificate_fingerprint": fingerprint,
    }
    response = worker.post("/v1/workers/register", json=registration)
    response.raise_for_status()
    recovery_task = {
        **task,
        "task_id": "compose-recovery-task",
        "required_capabilities": ["recovery"],
        "idempotency_key": "compose-recovery-task-v1",
    }
    user.post("/v1/tasks", json=recovery_task).raise_for_status()
    first = worker.post("/v1/workers/recovery-worker-a/lease")
    first.raise_for_status()
    assert first.json()["attempts"] == 1
    run(
        compose,
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "ael",
        "-d",
        "ael",
        "-c",
        "UPDATE tasks SET lease_expires_at=NOW()-interval '1 second' "
        "WHERE task_id='compose-recovery-task';",
    )
    registration["worker_id"] = "recovery-worker-b"
    worker.post("/v1/workers/register", json=registration).raise_for_status()
    recovered = worker.post("/v1/workers/recovery-worker-b/lease")
    recovered.raise_for_status()
    report["checks"]["expired_lease_recovered_idempotently"] = (
        recovered.json()["task_id"] == "compose-recovery-task" and recovered.json()["attempts"] == 2
    )
    user.post("/v1/tasks/compose-recovery-task/cancel").raise_for_status()
    report["checks"]["leased_task_cancellation"] = (
        user.get("/v1/tasks/compose-recovery-task").json()["status"] == "cancelled"
    )

    s3 = boto3.client(
        "s3",
        endpoint_url="http://127.0.0.1:9000",
        aws_access_key_id="ael-development",
        aws_secret_access_key="development-only-secret",
        config=Config(proxies={}),
    )
    payload = b"ael-compose-evidence-retry"
    s3.put_object(Bucket="ael-development", Key="acceptance/before", Body=payload)
    run(compose, "stop", "minio")
    outage_observed = False
    try:
        s3.put_object(Bucket="ael-development", Key="acceptance/during", Body=payload)
    except Exception:
        outage_observed = True
    run(compose, "start", "minio")
    wait_for(lambda: s3.list_buckets())
    s3.put_object(Bucket="ael-development", Key="acceptance/retry", Body=payload)
    restored = s3.get_object(Bucket="ael-development", Key="acceptance/retry")["Body"].read()
    report["checks"]["s3_outage_and_retransmit"] = outage_observed and restored == payload

    run(compose, "restart", "postgres")
    access_token = wait_for(token)
    user.headers["Authorization"] = f"Bearer {access_token}"
    def project_after_database_restart() -> httpx.Response | None:
        response = user.get("/v1/project")
        return response if response.status_code == 200 else None

    recovered_project = wait_for(project_after_database_restart)
    report["checks"]["postgres_restart_recovery"] = recovered_project.status_code == 200

    migration_upgrade = run(
        compose,
        "exec",
        "-T",
        "postgres",
        "psql",
        "-At",
        "-U",
        "ael",
        "-d",
        "ael",
        "-c",
        "CREATE TABLE ael_migration_acceptance (id integer PRIMARY KEY); "
        "ALTER TABLE ael_migration_acceptance ADD COLUMN payload jsonb NOT NULL "
        "DEFAULT '{}'::jsonb; "
        "INSERT INTO ael_migration_acceptance VALUES (1, '{\"phase\":\"upgraded\"}'); "
        "SELECT payload->>'phase' FROM ael_migration_acceptance WHERE id=1;",
    )
    run(
        compose,
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "ael",
        "-d",
        "ael",
        "-c",
        "DROP TABLE ael_migration_acceptance;",
    )
    migration_rollback = run(
        compose,
        "exec",
        "-T",
        "postgres",
        "psql",
        "-At",
        "-U",
        "ael",
        "-d",
        "ael",
        "-c",
        "SELECT to_regclass('public.ael_migration_acceptance') IS NULL;",
    )
    report["checks"]["migration_upgrade_and_rollback"] = (
        "upgraded" in migration_upgrade.stdout and "t" in migration_rollback.stdout.split()
    )
    report["passed"] = all(report["checks"].values())
    report["hardware_validated"] = False
    report["limitations"] = [
        "Local software topology only.",
        "No board, instrument, calibration, or Validation Envelope evidence was produced.",
    ]
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    if not report["passed"]:
        raise SystemExit(2)


if __name__ == "__main__":
    main()
