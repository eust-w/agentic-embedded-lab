from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from abc import ABC, abstractmethod
from contextlib import suppress
from pathlib import Path
from typing import Any, TextIO

from ael.backend_protocol import BackendOperation, BackendRequest, BackendResponse
from ael.contracts import Event, SystemComponent
from ael.io import write_json
from ael.security import resolve_workspace_path

VERSION_PATTERN = re.compile(r"(?<!\d)(\d+(?:\.\d+){0,3})(?!\d)")
METRIC_PATTERN = re.compile(r"^AEL_METRIC\s+([A-Za-z0-9_.-]+)=(.+)$")
EVENT_PATTERN = re.compile(r"^AEL_EVENT\s+([A-Za-z0-9_.-]+)(?:\s+(.+))?$")


class BackendWorker(ABC):
    backend_name: str
    expected_version: str
    commands: tuple[str, ...]
    version_arguments: tuple[str, ...] = ("--version",)

    def __init__(self) -> None:
        workspace = os.environ.get("AEL_WORKSPACE")
        self.workspace = Path(workspace or Path.cwd()).resolve()
        self.component: SystemComponent | None = None
        self.seed = 0
        self.inputs: dict[str, Any] = {}
        self.virtual_time_us = 0
        self.runtime_dir: Path | None = None
        self.tool = self._resolve_tool()
        self.detected_version = self._version()

    def serve(self, input_stream: TextIO = sys.stdin, output_stream: TextIO = sys.stdout) -> None:
        for line in input_stream:
            request: BackendRequest | None = None
            try:
                request = BackendRequest.model_validate_json(line)
                response = self.handle(request)
            except Exception as exception:
                request_id = "invalid"
                with suppress(Exception):
                    request_id = json.loads(line).get("request_id", "invalid")
                response = BackendResponse.failure(
                    request_id, f"{type(exception).__name__}: {exception}"
                )
            output_stream.write(response.model_dump_json() + "\n")
            output_stream.flush()
            if request is not None and request.operation == BackendOperation.SHUTDOWN:
                return

    def handle(self, request: BackendRequest) -> BackendResponse:
        if request.operation == BackendOperation.PROBE:
            available = self.tool is not None and self.detected_version is not None
            reason = None
            if self.tool is None:
                reason = "tool executable not found"
            elif self.detected_version is None:
                reason = "tool version could not be detected"
            elif not self._version_matches(self.detected_version):
                available = False
                reason = (
                    f"version mismatch: expected {self.expected_version}, "
                    f"detected {self.detected_version}"
                )
            return BackendResponse(
                request_id=request.request_id,
                ok=True,
                outputs={
                    "available": available,
                    "version": self.detected_version,
                    "reason": reason,
                },
            )
        self._require_available()
        if request.operation == BackendOperation.PREPARE:
            component = SystemComponent.model_validate(request.payload["component"])
            if component.backend.value != self.backend_name:
                raise ValueError(
                    f"worker {self.backend_name} cannot prepare {component.backend.value}"
                )
            self.component = component
            self.seed = int(request.payload["seed"])
            runtime_root = self.workspace / ".ael" / "backend-runtime"
            runtime_root.mkdir(parents=True, exist_ok=True)
            self.runtime_dir = Path(
                tempfile.mkdtemp(prefix=f"ael-{self.backend_name}-", dir=runtime_root)
            )
            self.prepare()
            return BackendResponse(request_id=request.request_id, ok=True)
        self._require_prepared()
        if request.operation == BackendOperation.INJECT:
            target = str(request.payload["target"])
            value = request.payload["value"]
            key = target.rsplit(".", 1)[-1]
            self.inputs[key] = value
            event = Event(
                sequence=0,
                virtual_time_us=request.virtual_time_us or 0,
                source=self.component.id,
                type=f"{self.backend_name}.inject",
                payload={"target": target, "value": value},
                fidelity_ref=f"{self.backend_name}:tool-executed",
            )
            return BackendResponse(request_id=request.request_id, ok=True, events=[event])
        if request.operation == BackendOperation.STEP:
            step_us = int(request.payload["step_us"])
            self.virtual_time_us = request.virtual_time_us or self.virtual_time_us
            outputs, metrics, events, artifacts = self.step(step_us)
            self.virtual_time_us += step_us
            return BackendResponse(
                request_id=request.request_id,
                ok=True,
                outputs=outputs,
                metrics=metrics,
                events=events,
                artifacts=artifacts,
            )
        if request.operation == BackendOperation.SNAPSHOT:
            destination = resolve_workspace_path(self.workspace, request.payload["destination"])
            snapshot = self.snapshot(destination)
            return BackendResponse(
                request_id=request.request_id,
                ok=True,
                artifacts={"snapshot": self.artifact_reference(snapshot)},
            )
        if request.operation == BackendOperation.SHUTDOWN:
            self.shutdown()
            return BackendResponse(request_id=request.request_id, ok=True)
        raise ValueError(f"unsupported operation: {request.operation}")

    def prepare(self) -> None:
        if self.component and self.component.model:
            self.model_path()

    @abstractmethod
    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]: ...

    def snapshot(self, destination: Path) -> Path:
        destination.parent.mkdir(parents=True, exist_ok=True)
        write_json(
            destination,
            {
                "backend": self.backend_name,
                "virtual_time_us": self.virtual_time_us,
                "seed": self.seed,
                "inputs": self.inputs,
                "runtime_dir": str(self.runtime_dir),
            },
        )
        return destination

    def shutdown(self) -> None:
        self.component = None
        if self.runtime_dir is not None:
            shutil.rmtree(self.runtime_dir, ignore_errors=True)
            self.runtime_dir = None

    def artifact_reference(self, path: Path) -> str:
        resolved = path.resolve()
        try:
            return str(resolved.relative_to(self.workspace))
        except ValueError as error:
            raise ValueError("backend artifact is outside the workspace") from error

    def model_path(self) -> Path:
        self._require_prepared()
        if not self.component.model:
            raise ValueError(f"{self.backend_name} component requires a model path")
        return resolve_workspace_path(self.workspace, self.component.model, must_exist=True)

    def property_path(self, key: str, *, required: bool = False) -> Path | None:
        self._require_prepared()
        value = self.component.properties.get(key)
        if value is None:
            if required:
                raise ValueError(f"component property {key!r} is required")
            return None
        if not isinstance(value, str):
            raise ValueError(f"component property {key!r} must be a workspace path")
        return resolve_workspace_path(self.workspace, value, must_exist=True)

    def run_tool(
        self,
        arguments: list[str],
        *,
        timeout_s: int | None = None,
        cwd: Path | None = None,
        extra_environment: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        self._require_available()
        timeout = timeout_s or int(self.component.properties.get("timeout_s", 120))
        result = subprocess.run(
            [str(self.tool), *arguments],
            cwd=cwd or self.runtime_dir,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
            env={
                **os.environ,
                "AEL_SEED": str(self.seed),
                **(extra_environment or {}),
            },
        )
        if result.returncode != 0:
            stdout = self._diagnostic_excerpt(result.stdout)
            stderr = self._diagnostic_excerpt(result.stderr)
            raise RuntimeError(
                f"{self.backend_name} exited {result.returncode}:\n"
                f"--- stdout ---\n{stdout}\n"
                f"--- stderr ---\n{stderr}"
            )
        return result

    @staticmethod
    def _diagnostic_excerpt(output: str, limit: int = 6000) -> str:
        """Keep both the first error context and the final stack frames."""
        if not output:
            return "<empty>"
        if len(output) <= limit:
            return output
        half = limit // 2
        return f"{output[:half]}\n... <truncated {len(output) - limit} chars> ...\n{output[-half:]}"

    def parse_output(self, output: str, event_time_us: int) -> tuple[dict[str, Any], list[Event]]:
        metrics: dict[str, Any] = {}
        events: list[Event] = []
        for line in output.splitlines():
            metric_match = METRIC_PATTERN.match(line.strip())
            if metric_match:
                value: Any = metric_match.group(2)
                with suppress(ValueError):
                    value = float(value)
                metrics[metric_match.group(1)] = value
            event_match = EVENT_PATTERN.match(line.strip())
            if event_match:
                payload: dict[str, Any] = {}
                if event_match.group(2):
                    try:
                        payload = json.loads(event_match.group(2))
                    except json.JSONDecodeError:
                        payload = {"message": event_match.group(2)}
                events.append(
                    Event(
                        sequence=0,
                        virtual_time_us=event_time_us,
                        source=self.component.id if self.component else self.backend_name,
                        type=event_match.group(1),
                        payload=payload,
                        fidelity_ref=f"{self.backend_name}:tool-executed",
                    )
                )
            measurement = re.match(r"^ael_([a-z0-9_.-]+)\s*=\s*([-+0-9.eE]+)", line.strip().lower())
            if measurement:
                metrics[measurement.group(1)] = float(measurement.group(2))
        return metrics, events

    def _resolve_tool(self) -> Path | None:
        override = os.environ.get(f"AEL_{self.backend_name.upper()}_BIN")
        if override:
            candidate = Path(override)
            return candidate if candidate.is_file() else None
        for command in self.commands:
            resolved = shutil.which(command)
            if resolved:
                return Path(resolved)
        return None

    def _version(self) -> str | None:
        if self.tool is None:
            return None
        for argument in self.version_arguments:
            try:
                result = subprocess.run(
                    [str(self.tool), argument],
                    capture_output=True,
                    text=True,
                    timeout=10,
                    check=False,
                )
            except (OSError, subprocess.TimeoutExpired):
                continue
            output = f"{result.stdout}\n{result.stderr}"
            expected = re.search(
                rf"(?<![0-9.]){re.escape(self.expected_version)}(?![0-9.])", output
            )
            if expected:
                return self.expected_version
            match = VERSION_PATTERN.search(output)
            if match:
                return match.group(1)
        return None

    def _version_matches(self, detected: str) -> bool:
        return detected == self.expected_version or detected.startswith(f"{self.expected_version}.")

    def _require_available(self) -> None:
        if self.tool is None:
            raise RuntimeError(f"{self.backend_name} executable is not installed")
        if self.detected_version is None or not self._version_matches(self.detected_version):
            raise RuntimeError(
                f"{self.backend_name} version mismatch: expected {self.expected_version}, "
                f"detected {self.detected_version}"
            )

    def _require_prepared(self) -> None:
        if self.component is None or self.runtime_dir is None:
            raise RuntimeError("backend has not been prepared")
