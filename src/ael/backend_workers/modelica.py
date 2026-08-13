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
        text = source.read_text(encoding="utf-8")
        replacements = {
            "thermal_resistance_K_per_W": self.inputs.get("thermal_resistance_K_per_W", 18.0),
            "sleep_current_A": self.inputs.get("sleep_current_A", 0.000008),
            "duty_cycle": self.inputs.get("duty_cycle", 0.01),
            "input_power": self.inputs.get("input_power", 0.1),
            "rf_retries": self.inputs.get("rf_retries", 0),
        }
        for name, value in replacements.items():
            text = text.replace("{{" + name + "}}", str(float(value)))

        # The checked-in domain asset intentionally keeps the Modelica class and
        # its OMC commands together so it is a single, auditable benchmark input.
        # `omc` does not accept a class declaration directly in a `.mos` command
        # script, however.  Materialize the class as a `.mo` file and make the
        # generated `.mos` script load it before executing the remaining commands.
        # This is syntax separation only; neither section is synthesized here.
        model_end = "end AelDomain;"
        boundary = text.find(model_end)
        if boundary < 0:
            raise ValueError("OpenModelica benchmark asset is missing 'end AelDomain;'")
        boundary += len(model_end)
        model = self.runtime_dir / "AelDomain.mo"
        script = self.runtime_dir / "ael-domain.mos"
        model.write_text(text[:boundary].strip() + "\n", encoding="utf-8")
        commands = text[boundary:].lstrip()
        script.write_text(
            'loadFile("AelDomain.mo");\n' + commands,
            encoding="utf-8",
        )
        result = self.run_tool([str(script)], cwd=script.parent)
        combined = f"{result.stdout}\n{result.stderr}"
        log = self.runtime_dir / f"step-{self.virtual_time_us + step_us}.log"
        log.write_text(combined, encoding="utf-8")
        metrics, events = self.parse_output(combined, self.virtual_time_us + step_us)
        result_file = self.component.properties.get("result_file")
        artifacts: dict[str, str] = {}
        if result_file:
            path = self.runtime_dir / result_file
            if path.exists():
                artifacts["result"] = self.artifact_reference(path)
        artifacts["log"] = self.artifact_reference(log)
        return metrics.copy(), metrics, events, artifacts


if __name__ == "__main__":
    ModelicaWorker().serve()
