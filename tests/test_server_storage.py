from __future__ import annotations

from pathlib import Path

from ael.contracts import (
    WorkerCapability,
    WorkerKind,
    WorkerRegistration,
    WorkerTask,
)
from ael.storage import WorkspaceLayout
from ael.storage_server import S3ContentAddressedStore, ServerStateStore


class FakeS3:
    class exceptions:
        class ClientError(Exception):
            pass

    def __init__(self) -> None:
        self.objects: dict[tuple[str, str], bytes] = {}

    def head_object(self, *, Bucket: str, Key: str):
        if (Bucket, Key) not in self.objects:
            raise self.exceptions.ClientError()

    def put_object(self, *, Bucket: str, Key: str, Body: bytes, **kwargs):
        self.objects[(Bucket, Key)] = Body

    def get_object(self, *, Bucket: str, Key: str):
        payload = self.objects[(Bucket, Key)]

        class Body:
            def read(self) -> bytes:
                return payload

        return {"Body": Body()}


def test_postgres_semantics_store_supports_idempotent_leases_with_sqlite(tmp_path: Path) -> None:
    store = ServerStateStore(
        WorkspaceLayout(tmp_path), f"sqlite+pysqlite:///{tmp_path / 'server.db'}"
    )
    registration = WorkerRegistration(
        worker_id="server-worker",
        worker_kind=WorkerKind.SIMULATION,
        capabilities=[WorkerCapability(name="renode", version="1", kind="backend")],
        agent_version="test",
        certificate_fingerprint="c" * 64,
    )
    store.register_worker(registration)
    task = WorkerTask(
        task_id="server-task",
        task_type="experiment",
        required_capabilities=["renode"],
        payload={"experiment_path": "case.yaml"},
        idempotency_key="server-key",
    )
    store.enqueue_task(task)
    leased = store.lease_task("server-worker")
    assert leased and leased.task_id == "server-task" and leased.lease_token


def test_s3_cas_verifies_round_trip_digest() -> None:
    store = object.__new__(S3ContentAddressedStore)
    store.bucket = "ael"
    store.prefix = "sha256"
    store.client = FakeS3()
    digest, key = store.put_bytes(b"evidence")
    assert key.endswith(digest[2:])
    assert store.get_bytes(digest) == b"evidence"
