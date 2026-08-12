from __future__ import annotations

import json
import os
import select
import shutil
import subprocess
import sys
import uuid
from pathlib import Path
from typing import Any

from ael.backend_protocol import BackendOperation, BackendRequest, BackendResponse
from ael.contracts import BackendName, Event, SystemComponent

from .base import Adapter, AdapterProbe, AdapterStepResult


class BackendProtocolError(RuntimeError):
    pass


def backend_cpu_limit() -> float:
    raw = os.environ.get("AEL_BACKEND_CPUS", "2")
    try:
        value = float(raw)
    except ValueError as error:
        raise ValueError("AEL_BACKEND_CPUS must be numeric") from error
    if not 0.01 <= value <= 64:
        raise ValueError("AEL_BACKEND_CPUS must be between 0.01 and 64")
    return value


class SubprocessAdapter(Adapter):
    def __init__(
        self,
        backend: BackendName,
        worker_module: str,
        expected_version: str,
        timeout_s: int = 30,
    ) -> None:
        self.backend = backend
        self.worker_module = worker_module
        self.expected_version = expected_version
        self.timeout_s = timeout_s
        self.process: subprocess.Popen[str] | None = None
        self.launch_command: list[str] | None = None

    def probe(self) -> AdapterProbe:
        try:
            self._start()
            response = self._call(BackendOperation.PROBE)
            version = str(response.outputs.get("version", "")) or None
            available = bool(response.outputs.get("available", False))
            return AdapterProbe(
                backend=self.backend,
                available=available,
                command=f"{sys.executable} -m {self.worker_module}",
                detected_version=version,
                expected_version=self.expected_version,
                reason=response.error or response.outputs.get("reason"),
            )
        except Exception as exception:
            return AdapterProbe(
                backend=self.backend,
                available=False,
                command=f"{sys.executable} -m {self.worker_module}",
                detected_version=None,
                expected_version=self.expected_version,
                reason=f"backend worker unavailable: {exception}",
            )
        finally:
            self.shutdown()

    def prepare(self, component: SystemComponent, seed: int) -> None:
        self._start()
        self._call(
            BackendOperation.PREPARE,
            payload={"component": component.model_dump(mode="json"), "seed": seed},
        )

    def inject(self, target: str, value: Any, virtual_time_us: int) -> list[Event]:
        response = self._call(
            BackendOperation.INJECT,
            virtual_time_us=virtual_time_us,
            payload={"target": target, "value": value},
        )
        return response.events

    def step(self, virtual_time_us: int, step_us: int) -> AdapterStepResult:
        response = self._call(
            BackendOperation.STEP,
            virtual_time_us=virtual_time_us,
            payload={"step_us": step_us},
        )
        return AdapterStepResult(response.outputs, response.metrics, response.events)

    def snapshot(self, destination: str) -> str | None:
        response = self._call(BackendOperation.SNAPSHOT, payload={"destination": destination})
        return response.artifacts.get("snapshot")

    def shutdown(self) -> None:
        if self.process is None:
            return
        if self.process.poll() is None:
            try:
                self._call(BackendOperation.SHUTDOWN)
            except Exception:
                self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait(timeout=5)
        self.process = None

    def _start(self) -> None:
        if self.process is not None and self.process.poll() is None:
            return
        environment = {**os.environ, "PYTHONUNBUFFERED": "1"}
        image = os.environ.get(f"AEL_{self.backend.value.upper().replace('-', '_')}_IMAGE")
        if image:
            runtime = shutil.which(os.environ.get("AEL_OCI_RUNTIME", "podman"))
            if runtime is None:
                runtime = shutil.which("docker")
            if runtime is None:
                raise RuntimeError("backend image configured but no OCI runtime is installed")
            workspace = Path(os.environ.get("AEL_WORKSPACE", Path.cwd())).resolve()
            self.launch_command = [
                runtime,
                "run",
                "--rm",
                "--interactive",
                "--read-only",
                "--network=none",
                "--cap-drop=ALL",
                "--security-opt=no-new-privileges",
                "--pids-limit=512",
                "--memory=4g",
                f"--cpus={backend_cpu_limit():g}",
                "--tmpfs=/tmp:rw,nosuid,size=2g",
                "--mount",
                f"type=bind,src={workspace},dst=/workspace",
                "--workdir=/workspace",
                "--env=AEL_WORKSPACE=/workspace",
                "--entrypoint=python3",
                image,
                "-m",
                self.worker_module,
            ]
        else:
            self.launch_command = [sys.executable, "-m", self.worker_module]
        self.process = subprocess.Popen(
            self.launch_command,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
            env=environment,
        )

    def _call(
        self,
        operation: BackendOperation,
        *,
        virtual_time_us: int | None = None,
        payload: dict[str, Any] | None = None,
    ) -> BackendResponse:
        if self.process is None or self.process.stdin is None or self.process.stdout is None:
            raise BackendProtocolError("backend process is not running")
        request = BackendRequest(
            request_id=uuid.uuid4().hex,
            operation=operation,
            virtual_time_us=virtual_time_us,
            payload=payload or {},
        )
        self.process.stdin.write(request.model_dump_json() + "\n")
        self.process.stdin.flush()
        readable, _, _ = select.select([self.process.stdout], [], [], self.timeout_s)
        if not readable:
            raise TimeoutError(f"backend {self.backend} did not respond within {self.timeout_s}s")
        line = self.process.stdout.readline()
        if not line:
            stderr = self.process.stderr.read() if self.process.stderr else ""
            raise BackendProtocolError(
                f"backend exited with {self.process.poll()}; stderr={stderr[-2000:]}"
            )
        try:
            response = BackendResponse.model_validate(json.loads(line))
        except Exception as exception:
            raise BackendProtocolError(f"invalid backend response: {line[:500]}") from exception
        if response.request_id != request.request_id:
            raise BackendProtocolError("backend response request_id mismatch")
        if not response.ok:
            raise BackendProtocolError(response.error or "backend operation failed")
        return response
