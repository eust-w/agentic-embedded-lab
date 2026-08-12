from __future__ import annotations

from typing import Any

from ael.contracts import Event

from .base import BackendWorker


class ModelicaWorker(BackendWorker):
    backend_name = "openmodelica"
    expected_version = "1.27.0"
    commands = ("omc",)
    version_arguments = ("--version",)

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        source = self.model_path()
        script = self.runtime_dir / "ael-domain.mos"
        text = source.read_text(encoding="utf-8")
        replacements = {
            "fault_scale": self.inputs.get("fault_scale", 0),
            "input_power": self.inputs.get("input_power", 0.1),
            "rf_retries": self.inputs.get("rf_retries", 0),
        }
        for name, value in replacements.items():
            text = text.replace("{{" + name + "}}", str(float(value)))
        script.write_text(text, encoding="utf-8")
        result = self.run_tool([str(script)], cwd=script.parent)
        combined = f"{result.stdout}\n{result.stderr}"
        metrics, events = self.parse_output(combined, self.virtual_time_us + step_us)
        result_file = self.component.properties.get("result_file")
        artifacts: dict[str, str] = {}
        if result_file:
            path = self.runtime_dir / result_file
            if path.exists():
                artifacts["result"] = str(path)
        return metrics.copy(), metrics, events, artifacts


if __name__ == "__main__":
    ModelicaWorker().serve()
