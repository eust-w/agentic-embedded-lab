from __future__ import annotations

import argparse
import hashlib
import os
import socket
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from typing import Any

import httpx

from . import __version__
from .contracts import (
    WorkerCapability,
    WorkerHeartbeat,
    WorkerKind,
    WorkerRegistration,
    WorkerTask,
    WorkerTaskResult,
)


class OutboundWorker:
    def __init__(
        self,
        control_plane: str,
        registration: WorkerRegistration,
        *,
        certificate: tuple[str, str],
        ca_bundle: str,
        poll_seconds: float = 2.0,
    ) -> None:
        self.control_plane = control_plane.rstrip("/")
        self.registration = registration
        self.poll_seconds = poll_seconds
        self.client = httpx.Client(
            base_url=self.control_plane,
            cert=certificate,
            verify=ca_bundle,
            timeout=30,
            headers={"X-Client-Cert-SHA256": registration.certificate_fingerprint},
        )

    def register(self) -> None:
        response = self.client.post(
            "/v1/workers/register", json=self.registration.model_dump(mode="json")
        )
        response.raise_for_status()

    def run_forever(self) -> None:
        self.register()
        while True:
            response = self.client.post(f"/v1/workers/{self.registration.worker_id}/lease")
            response.raise_for_status()
            payload = response.json()
            if payload is None:
                time.sleep(self.poll_seconds)
                continue
            task = WorkerTask.model_validate(payload)
            self.execute(task)

    def execute(self, task: WorkerTask) -> None:
        if not task.lease_token:
            raise ValueError("leased task has no lease token")
        self._heartbeat(task, "running", 0.0, "task accepted")
        cancel_event = threading.Event()
        executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="ael-worker-task")
        future = executor.submit(self._dispatch, task, cancel_event)
        cancelled = False
        try:
            while not future.done():
                time.sleep(min(10.0, self.poll_seconds))
                try:
                    self._heartbeat(task, "running", 0.5, "task executing")
                except httpx.HTTPStatusError as exception:
                    if exception.response.status_code != 422:
                        raise
                    cancelled = True
                    cancel_event.set()
            result_payload = future.result()
            result = WorkerTaskResult(
                task_id=task.task_id,
                lease_token=task.lease_token,
                status="succeeded",
                artifact_hashes=result_payload.get("artifact_hashes", {}),
                evidence_bundle=result_payload.get("evidence_bundle"),
            )
        except Exception as exception:
            result = WorkerTaskResult(
                task_id=task.task_id,
                lease_token=task.lease_token,
                status="cancelled" if cancelled else "failed",
                error=f"{type(exception).__name__}: {exception}",
            )
        finally:
            executor.shutdown(wait=True, cancel_futures=True)
        if cancelled:
            return
        response = self.client.post(
            f"/v1/workers/{self.registration.worker_id}/complete",
            json=result.model_dump(mode="json"),
        )
        response.raise_for_status()

    def _heartbeat(self, task: WorkerTask, status: str, progress: float, message: str) -> None:
        heartbeat = WorkerHeartbeat(
            task_id=task.task_id,
            lease_token=task.lease_token or "",
            status=status,
            progress=progress,
            message=message,
        )
        response = self.client.post(
            f"/v1/workers/{self.registration.worker_id}/heartbeat",
            json=heartbeat.model_dump(mode="json"),
        )
        response.raise_for_status()

    def _dispatch(
        self, task: WorkerTask, cancel_event: threading.Event
    ) -> dict[str, Any]:
        if self.registration.worker_kind == WorkerKind.LAB:
            if cancel_event.is_set():
                raise RuntimeError("lab task cancelled before dispatch")
            return self._dispatch_lab(task)
        if task.task_type not in {"experiment", "model_validation"}:
            raise PermissionError(f"simulation worker cannot execute task type: {task.task_type}")
        # The task names a validated workspace document; it never contains a shell.
        from .service import AelService

        workspace = Path(os.environ["AEL_WORKSPACE"])
        service = AelService(workspace)
        result = service.run_experiment(
            Path(task.payload["experiment_path"]), cancel_event=cancel_event
        )
        if result.status == "cancelled":
            raise RuntimeError("experiment cancelled")
        bundle = result.evidence_dir / "bundle.json"
        digest = hashlib.sha256(bundle.read_bytes()).hexdigest()
        return {
            "evidence_bundle": str(bundle.relative_to(workspace)),
            "artifact_hashes": {"bundle.json": digest},
        }

    def _dispatch_lab(self, task: WorkerTask) -> dict[str, Any]:
        if task.task_type not in {"calibration", "hardware_validation"}:
            raise PermissionError(f"lab worker cannot execute task type: {task.task_type}")
        from .contracts import InstrumentOperationRequest
        from .io import load_document
        from .lab import InstrumentDefinition, LabController

        workspace = Path(os.environ["AEL_WORKSPACE"])
        request = load_document(
            Path(task.payload["request_path"]), InstrumentOperationRequest, workspace
        )
        raw_definition = task.payload["instrument"]
        if set(raw_definition) != {"id", "resource", "kind", "driver"}:
            raise ValueError("instrument definition has unknown or missing fields")
        definition = InstrumentDefinition(**raw_definition)
        evidence = LabController(workspace).execute(
            request,
            definition,
            Path(task.payload["calibration_path"]),
            Path(task.payload["output_path"]),
        )
        return {
            "evidence_bundle": evidence.raw_artifact_path + ".evidence.json",
            "artifact_hashes": {evidence.raw_artifact_path: evidence.raw_artifact_sha256},
        }


def _certificate_sha256(path: Path) -> str:
    import ssl

    pem = path.read_text(encoding="utf-8")
    der = ssl.PEM_cert_to_DER_cert(pem)
    return hashlib.sha256(der).hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser(description="AEL outbound simulation worker")
    parser.add_argument("--control-plane", required=True)
    parser.add_argument("--cert", required=True)
    parser.add_argument("--key", required=True)
    parser.add_argument("--ca", required=True)
    parser.add_argument("--worker-id", default=socket.gethostname())
    parser.add_argument("--capability", action="append", default=[])
    parser.add_argument("--kind", choices=[item.value for item in WorkerKind], default="simulation")
    arguments = parser.parse_args()
    registration = WorkerRegistration(
        worker_id=arguments.worker_id,
        worker_kind=WorkerKind(arguments.kind),
        capabilities=[
            WorkerCapability(
                name=name,
                version="configured",
                kind="instrument" if arguments.kind == "lab" else "backend",
            )
            for name in arguments.capability
        ],
        agent_version=__version__,
        certificate_fingerprint=_certificate_sha256(Path(arguments.cert)),
    )
    OutboundWorker(
        arguments.control_plane,
        registration,
        certificate=(arguments.cert, arguments.key),
        ca_bundle=arguments.ca,
    ).run_forever()
