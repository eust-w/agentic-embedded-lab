from __future__ import annotations

import hashlib
import json
import secrets
import sqlite3
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import UTC, datetime
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
from .io import canonical_json, write_json


@dataclass(frozen=True)
class WorkspaceLayout:
    root: Path

    @property
    def state_dir(self) -> Path:
        return self.root / ".ael"

    @property
    def database(self) -> Path:
        return self.state_dir / "ael.sqlite3"

    @property
    def cas_dir(self) -> Path:
        return self.state_dir / "cas" / "sha256"

    @property
    def runs_dir(self) -> Path:
        return self.root / "runs"

    @property
    def models_dir(self) -> Path:
        return self.root / "models"

    def initialize(self) -> None:
        self.state_dir.mkdir(parents=True, exist_ok=True)
        self.cas_dir.mkdir(parents=True, exist_ok=True)
        self.runs_dir.mkdir(parents=True, exist_ok=True)
        self.models_dir.mkdir(parents=True, exist_ok=True)


class ContentAddressedStore:
    def __init__(self, layout: WorkspaceLayout) -> None:
        self.layout = layout

    def put_bytes(self, payload: bytes) -> tuple[str, Path]:
        digest = hashlib.sha256(payload).hexdigest()
        target = self.layout.cas_dir / digest[:2] / digest[2:]
        target.parent.mkdir(parents=True, exist_ok=True)
        if not target.exists():
            target.write_bytes(payload)
        return digest, target

    def put_value(self, value: Any) -> tuple[str, Path]:
        return self.put_bytes(canonical_json(value))


class StateStore:
    def __init__(self, layout: WorkspaceLayout) -> None:
        self.layout = layout
        layout.initialize()
        self._migrate()

    @contextmanager
    def connection(self) -> Iterator[sqlite3.Connection]:
        connection = sqlite3.connect(self.layout.database, timeout=30)
        connection.row_factory = sqlite3.Row
        try:
            yield connection
            connection.commit()
        finally:
            connection.close()

    def _migrate(self) -> None:
        with self.connection() as connection:
            connection.executescript(
                """
                PRAGMA journal_mode=WAL;
                PRAGMA foreign_keys=ON;
                CREATE TABLE IF NOT EXISTS runs (
                    run_id TEXT PRIMARY KEY,
                    status TEXT NOT NULL,
                    experiment_path TEXT NOT NULL,
                    evidence_path TEXT,
                    error TEXT,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS models (
                    model_id TEXT PRIMARY KEY,
                    state TEXT NOT NULL,
                    package_path TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS audit (
                    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
                    timestamp TEXT NOT NULL,
                    actor TEXT NOT NULL,
                    action TEXT NOT NULL,
                    subject TEXT NOT NULL,
                    details TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS workers (
                    worker_id TEXT PRIMARY KEY,
                    worker_kind TEXT NOT NULL,
                    registration_json TEXT NOT NULL,
                    certificate_fingerprint TEXT NOT NULL,
                    last_seen_at TEXT NOT NULL,
                    enabled INTEGER NOT NULL DEFAULT 1
                );
                CREATE TABLE IF NOT EXISTS tasks (
                    task_id TEXT PRIMARY KEY,
                    task_type TEXT NOT NULL,
                    required_capabilities_json TEXT NOT NULL,
                    payload_json TEXT NOT NULL,
                    idempotency_key TEXT NOT NULL UNIQUE,
                    status TEXT NOT NULL,
                    lease_owner TEXT,
                    lease_token TEXT,
                    lease_expires_at TEXT,
                    attempts INTEGER NOT NULL DEFAULT 0,
                    result_json TEXT,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );
                CREATE INDEX IF NOT EXISTS idx_tasks_status_created
                ON tasks(status, created_at);
                """
            )

    def create_run(self, run_id: str, experiment_path: str) -> None:
        now = datetime.now(UTC).isoformat()
        with self.connection() as connection:
            connection.execute(
                "INSERT INTO runs VALUES (?, ?, ?, NULL, NULL, ?, ?)",
                (run_id, RunStatus.QUEUED, experiment_path, now, now),
            )

    def update_run(
        self,
        run_id: str,
        status: RunStatus,
        *,
        evidence_path: str | None = None,
        error: str | None = None,
    ) -> None:
        with self.connection() as connection:
            connection.execute(
                """
                UPDATE runs
                SET status = ?, evidence_path = COALESCE(?, evidence_path),
                    error = ?, updated_at = ?
                WHERE run_id = ?
                """,
                (status, evidence_path, error, datetime.now(UTC).isoformat(), run_id),
            )

    def get_run(self, run_id: str) -> dict[str, Any] | None:
        with self.connection() as connection:
            row = connection.execute("SELECT * FROM runs WHERE run_id = ?", (run_id,)).fetchone()
        return dict(row) if row else None

    def list_runs(self, limit: int = 50) -> list[dict[str, Any]]:
        with self.connection() as connection:
            rows = connection.execute(
                "SELECT * FROM runs ORDER BY created_at DESC LIMIT ?", (limit,)
            ).fetchall()
        return [dict(row) for row in rows]

    def save_model(self, package: ModelPackage, package_path: Path) -> None:
        write_json(package_path, package)
        with self.connection() as connection:
            connection.execute(
                """
                INSERT INTO models VALUES (?, ?, ?, ?)
                ON CONFLICT(model_id) DO UPDATE SET
                  state = excluded.state,
                  package_path = excluded.package_path,
                  updated_at = excluded.updated_at
                """,
                (
                    package.id,
                    package.state,
                    str(package_path.relative_to(self.layout.root)),
                    datetime.now(UTC).isoformat(),
                ),
            )

    def get_model_record(self, model_id: str) -> dict[str, Any] | None:
        with self.connection() as connection:
            row = connection.execute(
                "SELECT * FROM models WHERE model_id = ?", (model_id,)
            ).fetchone()
        return dict(row) if row else None

    def list_models(self) -> list[dict[str, Any]]:
        with self.connection() as connection:
            rows = connection.execute("SELECT * FROM models ORDER BY model_id").fetchall()
        return [dict(row) for row in rows]

    def count_models(self, state: str) -> int:
        with self.connection() as connection:
            row = connection.execute(
                "SELECT COUNT(*) FROM models WHERE state = ?", (state,)
            ).fetchone()
        return int(row[0])

    def audit(self, actor: str, action: str, subject: str, details: str) -> None:
        with self.connection() as connection:
            connection.execute(
                """
                INSERT INTO audit(timestamp, actor, action, subject, details)
                VALUES (?, ?, ?, ?, ?)
                """,
                (datetime.now(UTC).isoformat(), actor, action, subject, details),
            )

    def register_worker(self, registration: WorkerRegistration) -> None:
        now = datetime.now(UTC).isoformat()
        payload = canonical_json(registration).decode()
        with self.connection() as connection:
            connection.execute(
                """
                INSERT INTO workers(
                  worker_id, worker_kind, registration_json,
                  certificate_fingerprint, last_seen_at, enabled
                ) VALUES (?, ?, ?, ?, ?, 1)
                ON CONFLICT(worker_id) DO UPDATE SET
                  worker_kind = excluded.worker_kind,
                  registration_json = excluded.registration_json,
                  certificate_fingerprint = excluded.certificate_fingerprint,
                  last_seen_at = excluded.last_seen_at
                """,
                (
                    registration.worker_id,
                    registration.worker_kind,
                    payload,
                    registration.certificate_fingerprint.lower(),
                    now,
                ),
            )

    def worker(self, worker_id: str) -> WorkerRegistration | None:
        with self.connection() as connection:
            row = connection.execute(
                "SELECT registration_json FROM workers WHERE worker_id = ? AND enabled = 1",
                (worker_id,),
            ).fetchone()
        return WorkerRegistration.model_validate_json(row[0]) if row else None

    def enqueue_task(self, task: WorkerTask) -> WorkerTask:
        now = datetime.now(UTC).isoformat()
        with self.connection() as connection:
            existing = connection.execute(
                "SELECT * FROM tasks WHERE idempotency_key = ?", (task.idempotency_key,)
            ).fetchone()
            if existing:
                return self._task_from_row(existing)
            connection.execute(
                """
                INSERT INTO tasks(
                  task_id, task_type, required_capabilities_json, payload_json,
                  idempotency_key, status, lease_owner, lease_token,
                  lease_expires_at, attempts, result_json, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, NULL, 0, NULL, ?, ?)
                """,
                (
                    task.task_id,
                    task.task_type,
                    json.dumps(task.required_capabilities, sort_keys=True),
                    json.dumps(task.payload, sort_keys=True),
                    task.idempotency_key,
                    TaskStatus.QUEUED,
                    task.created_at.isoformat(),
                    now,
                ),
            )
        return task

    def lease_task(self, worker_id: str, lease_seconds: int = 60) -> WorkerTask | None:
        if lease_seconds <= 0:
            raise ValueError("lease_seconds must be positive")
        registration = self.worker(worker_id)
        if registration is None:
            raise PermissionError(f"worker is not registered or enabled: {worker_id}")
        capability_names = {item.name for item in registration.capabilities}
        now = datetime.now(UTC)
        expires = datetime.fromtimestamp(now.timestamp() + lease_seconds, UTC)
        with self.connection() as connection:
            connection.execute("BEGIN IMMEDIATE")
            connection.execute(
                """
                UPDATE tasks
                SET status = ?, lease_owner = NULL, lease_token = NULL,
                    lease_expires_at = NULL, updated_at = ?
                WHERE status IN (?, ?) AND lease_expires_at < ?
                """,
                (
                    TaskStatus.QUEUED,
                    now.isoformat(),
                    TaskStatus.LEASED,
                    TaskStatus.RUNNING,
                    now.isoformat(),
                ),
            )
            rows = connection.execute(
                "SELECT * FROM tasks WHERE status = ? ORDER BY created_at, task_id",
                (TaskStatus.QUEUED,),
            ).fetchall()
            selected = next(
                (
                    row
                    for row in rows
                    if set(json.loads(row["required_capabilities_json"])) <= capability_names
                ),
                None,
            )
            if selected is None:
                return None
            token = secrets.token_hex(32)
            updated = connection.execute(
                """
                UPDATE tasks
                SET status = ?, lease_owner = ?, lease_token = ?,
                    lease_expires_at = ?, attempts = attempts + 1, updated_at = ?
                WHERE task_id = ? AND status = ?
                """,
                (
                    TaskStatus.LEASED,
                    worker_id,
                    token,
                    expires.isoformat(),
                    now.isoformat(),
                    selected["task_id"],
                    TaskStatus.QUEUED,
                ),
            )
            if updated.rowcount != 1:
                return None
            row = connection.execute(
                "SELECT * FROM tasks WHERE task_id = ?", (selected["task_id"],)
            ).fetchone()
        return self._task_from_row(row)

    def heartbeat_task(
        self, worker_id: str, heartbeat: WorkerHeartbeat, lease_seconds: int = 60
    ) -> WorkerTask:
        if lease_seconds <= 0:
            raise ValueError("lease_seconds must be positive")
        now = datetime.now(UTC)
        expires = datetime.fromtimestamp(now.timestamp() + lease_seconds, UTC)
        with self.connection() as connection:
            updated = connection.execute(
                """
                UPDATE tasks SET status = ?, lease_expires_at = ?, updated_at = ?
                WHERE task_id = ? AND lease_owner = ? AND lease_token = ?
                  AND status IN (?, ?)
                """,
                (
                    TaskStatus(heartbeat.status),
                    expires.isoformat(),
                    now.isoformat(),
                    heartbeat.task_id,
                    worker_id,
                    heartbeat.lease_token,
                    TaskStatus.LEASED,
                    TaskStatus.RUNNING,
                ),
            )
            if updated.rowcount != 1:
                raise PermissionError("lease token is invalid or task is no longer active")
            row = connection.execute(
                "SELECT * FROM tasks WHERE task_id = ?", (heartbeat.task_id,)
            ).fetchone()
        return self._task_from_row(row)

    def complete_task(self, worker_id: str, result: WorkerTaskResult) -> WorkerTask:
        now = datetime.now(UTC).isoformat()
        with self.connection() as connection:
            updated = connection.execute(
                """
                UPDATE tasks SET status = ?, result_json = ?, lease_expires_at = NULL,
                    updated_at = ?
                WHERE task_id = ? AND lease_owner = ? AND lease_token = ?
                  AND status IN (?, ?)
                """,
                (
                    TaskStatus(result.status),
                    canonical_json(result).decode(),
                    now,
                    result.task_id,
                    worker_id,
                    result.lease_token,
                    TaskStatus.LEASED,
                    TaskStatus.RUNNING,
                ),
            )
            if updated.rowcount != 1:
                raise PermissionError("lease token is invalid or task is no longer active")
            row = connection.execute(
                "SELECT * FROM tasks WHERE task_id = ?", (result.task_id,)
            ).fetchone()
        return self._task_from_row(row)

    def cancel_task(self, task_id: str) -> WorkerTask:
        with self.connection() as connection:
            updated = connection.execute(
                """
                UPDATE tasks SET status = ?, updated_at = ?
                WHERE task_id = ? AND status IN (?, ?, ?)
                """,
                (
                    TaskStatus.CANCELLED,
                    datetime.now(UTC).isoformat(),
                    task_id,
                    TaskStatus.QUEUED,
                    TaskStatus.LEASED,
                    TaskStatus.RUNNING,
                ),
            )
            if updated.rowcount != 1:
                raise ValueError("task is not cancellable")
            row = connection.execute("SELECT * FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
        return self._task_from_row(row)

    @staticmethod
    def _task_from_row(row: sqlite3.Row) -> WorkerTask:
        return WorkerTask(
            task_id=row["task_id"],
            task_type=row["task_type"],
            required_capabilities=json.loads(row["required_capabilities_json"]),
            payload=json.loads(row["payload_json"]),
            idempotency_key=row["idempotency_key"],
            status=row["status"],
            lease_owner=row["lease_owner"],
            lease_token=row["lease_token"],
            lease_expires_at=row["lease_expires_at"],
            attempts=row["attempts"],
            created_at=row["created_at"],
        )
