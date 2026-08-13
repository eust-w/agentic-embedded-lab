from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
from pathlib import Path
from typing import Any

from ael.contracts import Event

from .base import BackendWorker


class Ns3Worker(BackendWorker):
    backend_name = "ns3"
    expected_version = "3.47"
    commands = ("ns3",)

    def _version(self) -> str | None:
        if self.tool is None:
            return None
        attestation = self.tool.resolve().parent / ".ael-version"
        if attestation.is_file():
            detected = attestation.read_text(encoding="utf-8").strip()
            return detected or None
        for arguments in (("show", "version"), ("--version",)):
            try:
                result = subprocess.run(
                    [str(self.tool), *arguments],
                    capture_output=True,
                    text=True,
                    timeout=10,
                    check=False,
                )
            except (OSError, subprocess.TimeoutExpired):
                continue
            if self.expected_version in f"{result.stdout}\n{result.stderr}":
                return self.expected_version
        return None

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        source = self.model_path()
        ns3_root_raw = self.component.properties.get("ns3_root")
        if ns3_root_raw:
            ns3_root = self.property_path("ns3_root", required=True)
            assert ns3_root
        else:
            ns3_root = self.tool.resolve().parent
        arguments = [f"--{name}={value}" for name, value in sorted(self.inputs.items())]
        arguments.extend(
            [
                f"--seed={self.seed}",
                f"--stopUs={self.virtual_time_us + step_us}",
            ]
        )
        precompiled = os.environ.get("AEL_NS3_PRECOMPILED")
        digest_file = os.environ.get("AEL_NS3_MODEL_SHA256")
        if precompiled and digest_file:
            expected_digest = Path(digest_file).read_text(encoding="utf-8").strip()
            actual_digest = hashlib.sha256(source.read_bytes()).hexdigest()
            if actual_digest != expected_digest:
                raise RuntimeError(
                    "ns-3 model does not match the read-only precompiled image; rebuild it"
                )
            result = subprocess.run(
                [precompiled, *arguments],
                cwd=self.runtime_dir,
                capture_output=True,
                text=True,
                timeout=int(self.component.properties.get("timeout_s", 120)),
                check=False,
                env={**os.environ, "AEL_SEED": str(self.seed)},
            )
            if result.returncode != 0:
                raise RuntimeError(
                    f"ns3 exited {result.returncode}: {(result.stderr or result.stdout)[-4000:]}"
                )
        else:
            program = str(self.component.properties.get("program", "scratch/ael-network"))
            scratch = ns3_root / "scratch" / f"{program.rsplit('/', 1)[-1]}.cc"
            scratch.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, scratch)
            result = self.run_tool(["run", f"{program} {' '.join(arguments)}"], cwd=ns3_root)
        combined = f"{result.stdout}\n{result.stderr}"
        log = self.runtime_dir / f"step-{self.virtual_time_us + step_us}.log"
        log.write_text(combined, encoding="utf-8")
        metrics, events = self.parse_output(combined, self.virtual_time_us + step_us)
        return metrics.copy(), metrics, events, {"log": self.artifact_reference(log)}


if __name__ == "__main__":
    Ns3Worker().serve()
