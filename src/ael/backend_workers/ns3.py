from __future__ import annotations

import shutil
from typing import Any

from ael.contracts import Event

from .base import BackendWorker


class Ns3Worker(BackendWorker):
    backend_name = "ns3"
    expected_version = "3.47"
    commands = ("ns3",)
    version_arguments = ("--version", "version")

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
        program = str(self.component.properties.get("program", "scratch/ael-network"))
        scratch = ns3_root / "scratch" / f"{program.rsplit('/', 1)[-1]}.cc"
        scratch.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, scratch)
        arguments = [
            f"--{name}={value}" for name, value in sorted(self.inputs.items())
        ]
        arguments.extend(
            [
                f"--seed={self.seed}",
                f"--stopUs={self.virtual_time_us + step_us}",
            ]
        )
        result = self.run_tool(
            ["run", f"{program} {' '.join(arguments)}"], cwd=ns3_root
        )
        metrics, events = self.parse_output(
            f"{result.stdout}\n{result.stderr}", self.virtual_time_us + step_us
        )
        return metrics.copy(), metrics, events, {}


if __name__ == "__main__":
    Ns3Worker().serve()
