from __future__ import annotations

from typing import Any

from ael.contracts import Event

from .base import BackendWorker


class NgspiceWorker(BackendWorker):
    backend_name = "ngspice"
    expected_version = "46"
    commands = ("ngspice",)
    version_arguments = ("--version", "-v")

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        source = self.model_path()
        deck = self.runtime_dir / "ael.cir"
        defaults = {
            "fault_scale": 0,
            "load_microamp": 60000,
            "rf_retries": 0,
        }
        defaults.update(self.inputs)
        parameters = [
            f".param AEL_{name}={value}" for name, value in sorted(defaults.items())
        ]
        text = source.read_text(encoding="utf-8")
        source_lines = text.splitlines()
        if not source_lines:
            raise ValueError("ngspice model is empty")
        # SPICE always treats the first line as a title, even when it starts with a dot.
        deck.write_text(
            "\n".join([source_lines[0], *parameters, *source_lines[1:]]) + "\n",
            encoding="utf-8",
        )
        log = self.runtime_dir / f"step-{self.virtual_time_us + step_us}.log"
        raw = self.runtime_dir / f"step-{self.virtual_time_us + step_us}.raw"
        result = self.run_tool(
            ["-b", "-o", str(log), "-r", str(raw), str(deck)]
        )
        combined = result.stdout + "\n" + log.read_text(encoding="utf-8", errors="replace")
        metrics, events = self.parse_output(combined, self.virtual_time_us + step_us)
        return metrics.copy(), metrics, events, {"raw": str(raw), "log": str(log)}


if __name__ == "__main__":
    NgspiceWorker().serve()
