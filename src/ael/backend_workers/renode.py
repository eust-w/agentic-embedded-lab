from __future__ import annotations

import re
import shutil
from pathlib import Path
from typing import Any

from ael.contracts import Event

from .base import BackendWorker

REGISTER_RESULT = re.compile(r"AEL_REGISTER:([A-Za-z0-9_.-]+):(?:0x)?([0-9A-Fa-f]+)")


class RenodeWorker(BackendWorker):
    backend_name = "renode"
    expected_version = "1.16.1"
    commands = ("renode",)
    version_arguments = ("-v", "--version")

    def __init__(self) -> None:
        super().__init__()
        self.renode_snapshot: Path | None = None
        self.renode_snapshot_time_us = 0

    def _register_io_lines(self) -> list[str]:
        lines: list[str] = []
        input_registers = self.component.properties.get("input_registers", {})
        for name, value in sorted(self.inputs.items()):
            if name in input_registers:
                lines.append(f"sysbus WriteDoubleWord {input_registers[name]} {int(value)}")
        output_registers = self.component.properties.get("output_registers", {})
        sentinels = self.component.properties.get("output_sentinels", {})
        for name, value in sorted(sentinels.items()):
            if name not in output_registers:
                raise ValueError(f"Renode output sentinel refers to unknown output {name!r}")
            lines.append(f"sysbus WriteDoubleWord {output_registers[name]} {int(value)}")
        return lines

    def _initialization_lines(self) -> list[str]:
        model = self.model_path()
        if model.suffix == ".repl":
            lines = [
                "using sysbus",
                f'mach create "{self.component.id}"',
                f"machine LoadPlatformDescription @{model}",
            ]
            performance_mips = self.component.properties.get("performance_mips")
            if performance_mips is not None:
                if not isinstance(performance_mips, int) or not 1 <= performance_mips <= 100000:
                    raise ValueError("Renode performance_mips must be an integer from 1 to 100000")
                lines.append(f"cpu PerformanceInMips {performance_mips}")
        else:
            lines = [f"include @{model}"]
        firmware = self.property_path("firmware")
        if firmware:
            lines.append(f"sysbus LoadELF @{firmware}")
            entry_symbol = self.component.properties.get("entry_symbol")
            if entry_symbol is not None:
                if not isinstance(entry_symbol, str) or not re.fullmatch(
                    r"[A-Za-z_][A-Za-z0-9_.]*", entry_symbol
                ):
                    raise ValueError("Renode entry_symbol is not a safe ELF symbol")
                lines.append(f'cpu PC `sysbus GetSymbolAddress "{entry_symbol}"`')
        lines.extend(self._register_io_lines())
        return lines

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        script = self.runtime_dir / "ael-step.resc"
        lines: list[str]
        if self.renode_snapshot and self.renode_snapshot.exists():
            lines = [f"Load @{self.renode_snapshot}"]
            lines.extend(self._register_io_lines())
            run_us = self.virtual_time_us + step_us - self.renode_snapshot_time_us
        else:
            # A batch-mode step replays deterministically from reset when no explicit
            # checkpoint exists. This avoids silently checkpointing large mapped
            # memories at every communication point.
            lines = self._initialization_lines()
            run_us = self.virtual_time_us + step_us
        lines.append(f'emulation RunFor "{run_us / 1_000_000:.9f}"')
        output_registers = self.component.properties.get("output_registers", {})
        for name, address in sorted(output_registers.items()):
            lines.append(
                f'python "print(\'AEL_REGISTER:{name}:%x\' % '
                f'self.Machine.SystemBus.ReadDoubleWord({address}))"'
            )
        lines.append("quit")
        script.write_text("\n".join(lines) + "\n", encoding="utf-8")
        result = self.run_tool(["--disable-gui", str(script)])
        combined = f"{result.stdout}\n{result.stderr}"
        metrics, events = self.parse_output(combined, self.virtual_time_us + step_us)
        outputs = {
            match.group(1): int(match.group(2), 16)
            for match in REGISTER_RESULT.finditer(combined)
        }
        return outputs, metrics, events, {}

    def snapshot(self, destination: Path) -> Path:
        if (
            self.renode_snapshot
            and self.renode_snapshot.exists()
            and self.renode_snapshot_time_us == self.virtual_time_us
        ):
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(self.renode_snapshot, destination)
            return destination
        if destination.suffix == ".snapshot":
            destination.parent.mkdir(parents=True, exist_ok=True)
            script = self.runtime_dir / "ael-checkpoint.resc"
            if self.renode_snapshot and self.renode_snapshot.exists():
                lines = [f"Load @{self.renode_snapshot}", *self._register_io_lines()]
                run_us = self.virtual_time_us - self.renode_snapshot_time_us
            else:
                lines = self._initialization_lines()
                run_us = self.virtual_time_us
            if run_us:
                lines.append(f'emulation RunFor "{run_us / 1_000_000:.9f}"')
            lines.extend([f"Save @{destination}", "quit"])
            script.write_text("\n".join(lines) + "\n", encoding="utf-8")
            self.run_tool(["--disable-gui", str(script)])
            self.renode_snapshot = destination
            self.renode_snapshot_time_us = self.virtual_time_us
            return destination
        return super().snapshot(destination)


if __name__ == "__main__":
    RenodeWorker().serve()
