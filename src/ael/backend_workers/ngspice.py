from __future__ import annotations

from typing import Any

from ael.contracts import Event

from .base import BackendWorker


class NgspiceWorker(BackendWorker):
    backend_name = "ngspice"
    expected_version = "46"
    commands = ("ngspice",)
    version_arguments = ("--version", "-v")

    def __init__(self) -> None:
        super().__init__()
        # A BOR observation is an event, not merely the value at the final
        # co-simulation communication point.  Preserve it for the duration of
        # the run so a later load reduction cannot erase the causal evidence.
        self.brownout_observed = False

    def prepare(self) -> None:
        super().prepare()
        self.brownout_observed = False

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        source = self.model_path()
        deck = self.runtime_dir / "ael.cir"
        defaults = {
            "source_resistance_ohm": 0.15,
            "capacitance_uF": 47.0,
            "load_microamp": 60000,
            "rf_retries": 0,
        }
        if "parameters" in self.component.properties and isinstance(
            self.component.properties["parameters"], dict
        ):
            defaults.update(self.component.properties["parameters"])
        defaults.update(self.inputs)
        parameters = [f".param AEL_{name}={value}" for name, value in sorted(defaults.items())]
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
        # ngspice 46 deliberately disables `.measure` in batch mode when
        # `-r` is present.  Run the authoritative measurement pass without a
        # raw-file override, then make a second evidence-only pass for the
        # waveform.  The deck is deterministic and both files are retained in
        # the Evidence Bundle.
        result = self.run_tool(["-b", "-o", str(log), str(deck)])
        self.run_tool(["-b", "-r", str(raw), str(deck)])
        combined = result.stdout + "\n" + log.read_text(encoding="utf-8", errors="replace")
        metrics, events = self.parse_output(combined, self.virtual_time_us + step_us)
        supply_voltage = metrics.get("supply_voltage")
        bor_threshold = float(self.component.properties.get("bor_threshold_V", 2.7))
        if isinstance(supply_voltage, (int, float)):
            # ngspice does not portably support a .measure PARAM expression
            # referencing another transient measurement. Derive the discrete
            # assertion signal from the measured rail minimum instead.
            threshold_crossed = not self.brownout_observed and supply_voltage < bor_threshold
            self.brownout_observed = self.brownout_observed or threshold_crossed
            metrics["failure"] = float(self.brownout_observed)
            if threshold_crossed:
                events.append(
                    Event(
                        sequence=0,
                        virtual_time_us=self.virtual_time_us + step_us,
                        source=self.component.id,
                        type="power.brownout_threshold_crossed",
                        payload={
                            "rail_voltage_V": supply_voltage,
                            "bor_threshold_V": bor_threshold,
                        },
                        fidelity_ref="ngspice:tool-executed",
                    )
                )
        return (
            metrics.copy(),
            metrics,
            events,
            {
                "raw": self.artifact_reference(raw),
                "log": self.artifact_reference(log),
            },
        )


if __name__ == "__main__":
    NgspiceWorker().serve()
