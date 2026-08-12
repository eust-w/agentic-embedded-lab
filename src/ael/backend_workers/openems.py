from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
from typing import Any

from ael.contracts import Event
from ael.io import write_json

from .base import BackendWorker


class OpenEmsWorker(BackendWorker):
    backend_name = "openems"
    expected_version = "0.0.36"
    commands = ("openEMS",)
    version_arguments = ("--version", "-v")

    def __init__(self) -> None:
        super().__init__()
        self.cache: dict[str, tuple[dict[str, Any], dict[str, str]]] = {}

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        cache_key = hashlib.sha256(
            self.model_path().read_bytes() + json.dumps(self.inputs, sort_keys=True).encode()
        ).hexdigest()
        persistent = self.workspace / ".ael" / "openems-cache" / f"{cache_key}.json"
        if cache_key in self.cache:
            metrics, artifacts = self.cache[cache_key]
            event = Event(
                sequence=0,
                virtual_time_us=self.virtual_time_us + step_us,
                source=self.component.id,
                type="openems.cache_hit",
                payload={"cache_key": cache_key},
                fidelity_ref="openems:tool-executed-cached",
            )
            return metrics.copy(), metrics.copy(), [event], artifacts.copy()
        if persistent.is_file():
            record = json.loads(persistent.read_text(encoding="utf-8"))
            metrics = record["metrics"]
            artifacts = record["artifacts"]
            self.cache[cache_key] = (metrics.copy(), artifacts.copy())
            event = Event(
                sequence=0,
                virtual_time_us=self.virtual_time_us + step_us,
                source=self.component.id,
                type="openems.persistent_cache_hit",
                payload={"cache_key": cache_key},
                fidelity_ref="openems:tool-executed-cached",
            )
            return metrics.copy(), metrics.copy(), [event], artifacts.copy()
        scenario = self.model_path()
        octave = shutil.which("octave-cli") or shutil.which("octave")
        if octave is None:
            raise RuntimeError("openEMS scenarios require octave-cli or octave")
        environment = {
            **os.environ,
            **{
                f"AEL_INPUT_{name.upper()}": str(value)
                for name, value in self.inputs.items()
            },
        }
        result = subprocess.run(
            [octave, "--no-gui", "--quiet", str(scenario)],
            cwd=scenario.parent,
            capture_output=True,
            text=True,
            timeout=int(self.component.properties.get("timeout_s", 1800)),
            check=False,
            env=environment,
        )
        if result.returncode != 0:
            raise RuntimeError(
                f"openEMS/Octave exited {result.returncode}: "
                f"{(result.stderr or result.stdout)[-4000:]}"
            )
        metrics, events = self.parse_output(
            f"{result.stdout}\n{result.stderr}", self.virtual_time_us + step_us
        )
        result_dir = self.component.properties.get("result_dir")
        artifacts = {"result_dir": str(scenario.parent / result_dir)} if result_dir else {}
        self.cache[cache_key] = (metrics.copy(), artifacts.copy())
        write_json(persistent, {"metrics": metrics, "artifacts": artifacts})
        return metrics.copy(), metrics, events, artifacts


if __name__ == "__main__":
    OpenEmsWorker().serve()
