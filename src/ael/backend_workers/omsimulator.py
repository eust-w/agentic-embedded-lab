from __future__ import annotations

from typing import Any

from ael.contracts import Event

from .base import BackendWorker


class OMSimulatorWorker(BackendWorker):
    backend_name = "omsimulator"
    expected_version = "2.1.3"
    commands = ("OMSimulator", "omsimulator")
    version_arguments = ("--version", "-v")

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        model = self.model_path()
        result_file = self.runtime_dir / f"result-{self.virtual_time_us + step_us}.csv"
        result = self.run_tool(
            [
                str(model),
                f"--startTime={self.virtual_time_us / 1_000_000}",
                f"--stopTime={(self.virtual_time_us + step_us) / 1_000_000}",
                f"--resultFile={result_file}",
                f"--timeout={int(self.component.properties.get('timeout_s', 120))}",
            ]
        )
        metrics, events = self.parse_output(
            f"{result.stdout}\n{result.stderr}", self.virtual_time_us + step_us
        )
        return metrics.copy(), metrics, events, {"result": str(result_file)}


if __name__ == "__main__":
    OMSimulatorWorker().serve()
