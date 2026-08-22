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
        import re

        source = self.model_path()
        text = source.read_text(encoding="utf-8")
        replacements = {
            "thermal_resistance_K_per_W": 18.0,
            "sleep_current_A": 0.000008,
            "duty_cycle": 0.01,
            "input_power": 0.1,
            "rf_retries": 0,
        }
        if "parameters" in self.component.properties and isinstance(
            self.component.properties["parameters"], dict
        ):
            replacements.update(self.component.properties["parameters"])
        replacements.update(self.inputs)
        for name, value in replacements.items():
            text = text.replace("{{" + name + "}}", str(float(value)))

        # The checked-in domain asset intentionally keeps the Modelica class and
        # its OMC commands together so it is a single, auditable benchmark input.
        # `omc` does not accept a class declaration directly in a `.mos` command
        # script, however. Materialize the class as a `.mo` file and make the
        # generated `.mos` script load it before executing the remaining commands.
        match = re.search(r"end\s+([A-Za-z0-9_]+)\s*;", text)
        if match:
            class_name = match.group(1)
            boundary = match.end()
            mo_name = f"{class_name}.mo"
            mos_name = f"{class_name.lower()}.mos"
            model = self.runtime_dir / mo_name
            script = self.runtime_dir / mos_name
            model.write_text(text[:boundary].strip() + "\n", encoding="utf-8")
            commands = text[boundary:].lstrip()
            script.write_text(f'loadFile("{mo_name}");\n' + commands, encoding="utf-8")
        elif source.suffix == ".mos":
            script = self.runtime_dir / source.name
            script.write_text(text, encoding="utf-8")
        else:
            raise ValueError(
                "OpenModelica model must end with 'end <ModelName>;' or be a .mos script"
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
