from __future__ import annotations

import hashlib
import secrets
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any

from .contracts import (
    ModelPackage,
    RunStatus,
    TaskStatus,
    WorkerHeartbeat,
    WorkerRegistration,
    WorkerTask,
    WorkerTaskResult,
)
from .io import write_json
from .storage import WorkspaceLayout


class ServerDependencyUnavailable(RuntimeError):
    pass


def _sqlalchemy() -> dict[str, Any]:
    try:
        import sqlalchemy as sa
    except ImportError as exception:
        raise ServerDependencyUnavailable("install AEL with the server extra") from exception
    return {
        "JSON": sa.JSON,
        "Boolean": sa.Boolean,
        "Column": sa.Column,
        "DateTime": sa.DateTime,
        "Integer": sa.Integer,
        "MetaData": sa.MetaData,
        "String": sa.String,
        "Table": sa.Table,
        "Text": sa.Text,
        "create_engine": sa.create_engine,
        "func": sa.func,
        "select": sa.select,
    }


class S3ContentAddressedStore:
    def __init__(
        self,
        bucket: str,
        *,
        prefix: str = "sha256",
        endpoint_url: str | None = None,
        region_name: str | None = None,
    ) -> None:
        try:
            import boto3
        except ImportError as exception:
            raise ServerDependencyUnavailable("install AEL with the server extra") from exception
        self.bucket = bucket
        self.prefix = prefix.strip("/")
        self.client = boto3.client("s3", endpoint_url=endpoint_url, region_name=region_name)

    def put_bytes(self, payload: bytes) -> tuple[str, str]:
        digest = hashlib.sha256(payload).hexdigest()
        key = f"{self.prefix}/{digest[:2]}/{digest[2:]}"
        try:
            self.client.head_object(Bucket=self.bucket, Key=key)
        except self.client.exceptions.ClientError:
            self.client.put_object(
                Bucket=self.bucket,
                Key=key,
                Body=payload,
                ContentType="application/octet-stream",
                Metadata={"sha256": digest},
            )
        return digest, key

    def get_bytes(self, digest: str) -> bytes:
        key = f"{self.prefix}/{digest[:2]}/{digest[2:]}"
        response = self.client.get_object(Bucket=self.bucket, Key=key)
        payload = response["Body"].read()
        if hashlib.sha256(payload).hexdigest() != digest:
            raise ValueError("S3 artifact digest mismatch")
        return payload


class ServerStateStore:
    """PostgreSQL-oriented state store with transactional worker leases."""

    def __init__(self, layout: WorkspaceLayout, database_url: str) -> None:
        sa = _sqlalchemy()
        self.layout = layout
        layout.initialize()
        self.sa = sa
        self.engine = sa["create_engine"](database_url, pool_pre_ping=True)
        metadata = sa["MetaData"]()
        self.runs = sa["Table"](
            "runs",
            metadata,
            sa["Column"]("run_id", sa["String"](64), primary_key=True),
            sa["Column"]("status", sa["String"](32), nullable=False),
            sa["Column"]("experiment_path", sa["Text"](), nullable=False),
            sa["Column"]("evidence_path", sa["Text"]()),
            sa["Column"]("error", sa["Text"]()),
            sa["Column"]("created_at", sa["DateTime"](timezone=True), nullable=False),
            sa["Column"]("updated_at", sa["DateTime"](timezone=True), nullable=False),
        )
        self.models = sa["Table"](
            "models",
            metadata,
            sa["Column"]("model_id", sa["String"](255), primary_key=True),
            sa["Column"]("state", sa["String"](64), nullable=False),
            sa["Column"]("package_path", sa["Text"](), nullable=False),
            sa["Column"]("updated_at", sa["DateTime"](timezone=True), nullable=False),
        )
        self.workers = sa["Table"](
            "workers",
            metadata,
            sa["Column"]("worker_id", sa["String"](255), primary_key=True),
            sa["Column"]("worker_kind", sa["String"](32), nullable=False),
            sa["Column"]("registration", sa["JSON"](), nullable=False),
            sa["Column"]("certificate_fingerprint", sa["String"](64), nullable=False),
            sa["Column"]("last_seen_at", sa["DateTime"](timezone=True), nullable=False),
            sa["Column"]("enabled", sa["Boolean"](), nullable=False, default=True),
        )
        self.tasks = sa["Table"](
            "tasks",
            metadata,
            sa["Column"]("task_id", sa["String"](64), primary_key=True),
            sa["Column"]("task_type", sa["String"](64), nullable=False),
            sa["Column"]("required_capabilities", sa["JSON"](), nullable=False),
            sa["Column"]("payload", sa["JSON"](), nullable=False),
            sa["Column"]("idempotency_key", sa["String"](255), unique=True, nullable=False),
            sa["Column"]("status", sa["String"](32), nullable=False),
            sa["Column"]("lease_owner", sa["String"](255)),
            sa["Column"]("lease_token", sa["String"](64)),
            sa["Column"]("lease_expires_at", sa["DateTime"](timezone=True)),
            sa["Column"]("attempts", sa["Integer"](), nullable=False, default=0),
            sa["Column"]("result", sa["JSON"]()),
            sa["Column"]("created_at", sa["DateTime"](timezone=True), nullable=False),
            sa["Column"]("updated_at", sa["DateTime"](timezone=True), nullable=False),
        )
        self.audit_table = sa["Table"](
            "audit",
            metadata,
            sa["Column"]("sequence", sa["Integer"](), primary_key=True, autoincrement=True),
            sa["Column"]("timestamp", sa["DateTime"](timezone=True), nullable=False),
            sa["Column"]("actor", sa["String"](255), nullable=False),
            sa["Column"]("action", sa["String"](255), nullable=False),
            sa["Column"]("subject", sa["String"](255), nullable=False),
            sa["Column"]("details", sa["Text"](), nullable=False),
        )
        metadata.create_all(self.engine)

    def create_run(self, run_id: str, experiment_path: str) -> None:
        now = datetime.now(UTC)
        with self.engine.begin() as connection:
            connection.execute(
                self.runs.insert().values(
                    run_id=run_id,
                    status=RunStatus.QUEUED,
                    experiment_path=experiment_path,
                    created_at=now,
                    updated_at=now,
                )
            )

    def update_run(
        self,
        run_id: str,
        status: RunStatus,
        *,
        evidence_path: str | None = None,
        error: str | None = None,
    ) -> None:
        values: dict[str, Any] = {
            "status": status,
            "error": error,
            "updated_at": datetime.now(UTC),
        }
        if evidence_path is not None:
            values["evidence_path"] = evidence_path
        with self.engine.begin() as connection:
            connection.execute(
                self.runs.update().where(self.runs.c.run_id == run_id).values(**values)
            )

    def get_run(self, run_id: str) -> dict[str, Any] | None:
        with self.engine.connect() as connection:
            row = (
                connection.execute(self.sa["select"](self.runs).where(self.runs.c.run_id == run_id))
                .mappings()
                .first()
            )
        return dict(row) if row else None

    def list_runs(self, limit: int = 50) -> list[dict[str, Any]]:
        with self.engine.connect() as connection:
            rows = connection.execute(
                self.sa["select"](self.runs).order_by(self.runs.c.created_at.desc()).limit(limit)
            ).mappings()
            return [dict(row) for row in rows]

    def save_model(self, package: ModelPackage, package_path: Path) -> None:
        write_json(package_path, package)
        values = {
            "model_id": package.id,
            "state": package.state,
            "package_path": str(package_path.relative_to(self.layout.root)),
            "updated_at": datetime.now(UTC),
        }
        with self.engine.begin() as connection:
            existing = connection.execute(
                self.sa["select"](self.models.c.model_id).where(
                    self.models.c.model_id == package.id
                )
            ).first()
            if existing:
                connection.execute(
                    self.models.update()
                    .where(self.models.c.model_id == package.id)
                    .values(**values)
                )
            else:
                connection.execute(self.models.insert().values(**values))

    def get_model_record(self, model_id: str) -> dict[str, Any] | None:
        with self.engine.connect() as connection:
            row = (
                connection.execute(
                    self.sa["select"](self.models).where(self.models.c.model_id == model_id)
                )
                .mappings()
                .first()
            )
        return dict(row) if row else None

    def list_models(self) -> list[dict[str, Any]]:
        with self.engine.connect() as connection:
            rows = connection.execute(
                self.sa["select"](self.models).order_by(self.models.c.model_id)
            ).mappings()
            return [dict(row) for row in rows]

    def count_models(self, state: str) -> int:
        with self.engine.connect() as connection:
            return int(
                connection.execute(
                    self.sa["select"](self.sa["func"].count())
                    .select_from(self.models)
                    .where(self.models.c.state == state)
                ).scalar_one()
            )

    def audit(self, actor: str, action: str, subject: str, details: str) -> None:
        with self.engine.begin() as connection:
            connection.execute(
                self.audit_table.insert().values(
                    timestamp=datetime.now(UTC),
                    actor=actor,
                    action=action,
                    subject=subject,
                    details=details,
                )
            )

    def register_worker(self, registration: WorkerRegistration) -> None:
        values = {
            "worker_id": registration.worker_id,
            "worker_kind": registration.worker_kind,
            "registration": registration.model_dump(mode="json"),
            "certificate_fingerprint": registration.certificate_fingerprint.lower(),
            "last_seen_at": datetime.now(UTC),
            "enabled": True,
        }
        with self.engine.begin() as connection:
            existing = connection.execute(
                self.sa["select"](self.workers.c.worker_id).where(
                    self.workers.c.worker_id == registration.worker_id
                )
            ).first()
            statement = (
                self.workers.update()
                .where(self.workers.c.worker_id == registration.worker_id)
                .values(**values)
                if existing
                else self.workers.insert().values(**values)
            )
            connection.execute(statement)

    def worker(self, worker_id: str) -> WorkerRegistration | None:
        with self.engine.connect() as connection:
            row = (
                connection.execute(
                    self.sa["select"](self.workers).where(
                        self.workers.c.worker_id == worker_id,
                        self.workers.c.enabled.is_(True),
                    )
                )
                .mappings()
                .first()
            )
        return WorkerRegistration.model_validate(row["registration"]) if row else None

    def enqueue_task(self, task: WorkerTask) -> WorkerTask:
        now = datetime.now(UTC)
        with self.engine.begin() as connection:
            existing = (
                connection.execute(
                    self.sa["select"](self.tasks).where(
                        self.tasks.c.idempotency_key == task.idempotency_key
                    )
                )
                .mappings()
                .first()
            )
            if existing:
                return self._task(existing)
            connection.execute(
                self.tasks.insert().values(
                    task_id=task.task_id,
                    task_type=task.task_type,
                    required_capabilities=task.required_capabilities,
                    payload=task.payload,
                    idempotency_key=task.idempotency_key,
                    status=TaskStatus.QUEUED,
                    attempts=0,
                    created_at=task.created_at,
                    updated_at=now,
                )
            )
        return task

    def task(self, task_id: str) -> WorkerTask | None:
        with self.engine.connect() as connection:
            row = (
                connection.execute(
                    self.sa["select"](self.tasks).where(self.tasks.c.task_id == task_id)
                )
                .mappings()
                .first()
            )
        return self._task(row) if row else None

    def lease_task(self, worker_id: str, lease_seconds: int = 60) -> WorkerTask | None:
        if lease_seconds <= 0:
            raise ValueError("lease_seconds must be positive")
        registration = self.worker(worker_id)
        if registration is None:
            raise PermissionError(f"worker is not registered or enabled: {worker_id}")
        capability_names = {item.name for item in registration.capabilities}
        now = datetime.now(UTC)
        with self.engine.begin() as connection:
            connection.execute(
                self.tasks.update()
                .where(
                    self.tasks.c.status.in_([TaskStatus.LEASED, TaskStatus.RUNNING]),
                    self.tasks.c.lease_expires_at < now,
                )
                .values(
                    status=TaskStatus.QUEUED,
                    lease_owner=None,
                    lease_token=None,
                    lease_expires_at=None,
                    updated_at=now,
                )
            )
            candidates = connection.execute(
                self.sa["select"](self.tasks)
                .where(self.tasks.c.status == TaskStatus.QUEUED)
                .order_by(self.tasks.c.created_at, self.tasks.c.task_id)
                .limit(20)
                .with_for_update(skip_locked=True)
            ).mappings()
            row = next(
                (
                    item
                    for item in candidates
                    if set(item["required_capabilities"]) <= capability_names
                ),
                None,
            )
            if row is None:
                return None
            token = secrets.token_hex(32)
            connection.execute(
                self.tasks.update()
                .where(self.tasks.c.task_id == row["task_id"])
                .values(
                    status=TaskStatus.LEASED,
                    lease_owner=worker_id,
                    lease_token=token,
                    lease_expires_at=now + timedelta(seconds=lease_seconds),
                    attempts=self.tasks.c.attempts + 1,
                    updated_at=now,
                )
            )
            leased = dict(row)
            leased.update(
                status=TaskStatus.LEASED,
                lease_owner=worker_id,
                lease_token=token,
                lease_expires_at=now + timedelta(seconds=lease_seconds),
                attempts=row["attempts"] + 1,
            )
        return self._task(leased)

    def heartbeat_task(
        self, worker_id: str, heartbeat: WorkerHeartbeat, lease_seconds: int = 60
    ) -> WorkerTask:
        if lease_seconds <= 0:
            raise ValueError("lease_seconds must be positive")
        now = datetime.now(UTC)
        with self.engine.begin() as connection:
            result = connection.execute(
                self.tasks.update()
                .where(
                    self.tasks.c.task_id == heartbeat.task_id,
                    self.tasks.c.lease_owner == worker_id,
                    self.tasks.c.lease_token == heartbeat.lease_token,
                    self.tasks.c.status.in_([TaskStatus.LEASED, TaskStatus.RUNNING]),
                )
                .values(
                    status=TaskStatus(heartbeat.status),
                    lease_expires_at=now + timedelta(seconds=lease_seconds),
                    updated_at=now,
                )
            )
            if result.rowcount != 1:
                raise PermissionError("lease token is invalid or task is no longer active")
            row = (
                connection.execute(
                    self.sa["select"](self.tasks).where(self.tasks.c.task_id == heartbeat.task_id)
                )
                .mappings()
                .one()
            )
        return self._task(row)

    def complete_task(self, worker_id: str, result: WorkerTaskResult) -> WorkerTask:
        with self.engine.begin() as connection:
            updated = connection.execute(
                self.tasks.update()
                .where(
                    self.tasks.c.task_id == result.task_id,
                    self.tasks.c.lease_owner == worker_id,
                    self.tasks.c.lease_token == result.lease_token,
                    self.tasks.c.status.in_([TaskStatus.LEASED, TaskStatus.RUNNING]),
                )
                .values(
                    status=TaskStatus(result.status),
                    result=result.model_dump(mode="json"),
                    lease_expires_at=None,
                    updated_at=datetime.now(UTC),
                )
            )
            if updated.rowcount != 1:
                raise PermissionError("lease token is invalid or task is no longer active")
            row = (
                connection.execute(
                    self.sa["select"](self.tasks).where(self.tasks.c.task_id == result.task_id)
                )
                .mappings()
                .one()
            )
        return self._task(row)

    def cancel_task(self, task_id: str) -> WorkerTask:
        with self.engine.begin() as connection:
            result = connection.execute(
                self.tasks.update()
                .where(
                    self.tasks.c.task_id == task_id,
                    self.tasks.c.status.in_(
                        [TaskStatus.QUEUED, TaskStatus.LEASED, TaskStatus.RUNNING]
                    ),
                )
                .values(status=TaskStatus.CANCELLED, updated_at=datetime.now(UTC))
            )
            if result.rowcount != 1:
                raise ValueError("task is not cancellable")
            row = (
                connection.execute(
                    self.sa["select"](self.tasks).where(self.tasks.c.task_id == task_id)
                )
                .mappings()
                .one()
            )
        return self._task(row)

    @staticmethod
    def _task(row: Any) -> WorkerTask:
        return WorkerTask(
            task_id=row["task_id"],
            task_type=row["task_type"],
            required_capabilities=row["required_capabilities"],
            payload=row["payload"],
            idempotency_key=row["idempotency_key"],
            status=row["status"],
            lease_owner=row["lease_owner"],
            lease_token=row["lease_token"],
            lease_expires_at=row["lease_expires_at"],
            attempts=row["attempts"],
            result=row["result"],
            created_at=row["created_at"],
        )
