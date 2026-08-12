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

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        script = self.runtime_dir / "ael-step.resc"
        next_snapshot = self.runtime_dir / f"state-{self.virtual_time_us + step_us}.save"
        lines: list[str] = []
        if self.renode_snapshot and self.renode_snapshot.exists():
            lines.append(f"Load @{self.renode_snapshot}")
        else:
            lines.append(f"include @{self.model_path()}")
            firmware = self.property_path("firmware")
            if firmware:
                lines.append(f"sysbus LoadELF @{firmware}")
        input_registers = self.component.properties.get("input_registers", {})
        for name, value in sorted(self.inputs.items()):
            if name in input_registers:
                lines.append(f"sysbus WriteDoubleWord {input_registers[name]} {int(value)}")
        lines.append(f'emulation RunFor "{step_us / 1_000_000:.9f}"')
        output_registers = self.component.properties.get("output_registers", {})
        for name, address in sorted(output_registers.items()):
            lines.append(
                f'python "print(\'AEL_REGISTER:{name}:%x\' % '
                f'self.Machine.SystemBus.ReadDoubleWord({address}))"'
            )
        lines.extend([f"Save @{next_snapshot}", "quit"])
        script.write_text("\n".join(lines) + "\n", encoding="utf-8")
        result = self.run_tool(["--disable-gui", str(script)])
        self.renode_snapshot = next_snapshot
        combined = f"{result.stdout}\n{result.stderr}"
        metrics, events = self.parse_output(combined, self.virtual_time_us + step_us)
        outputs = {
            match.group(1): int(match.group(2), 16)
            for match in REGISTER_RESULT.finditer(combined)
        }
        return outputs, metrics, events, {"renode_snapshot": str(next_snapshot)}

    def snapshot(self, destination: Path) -> Path:
        if self.renode_snapshot and self.renode_snapshot.exists():
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(self.renode_snapshot, destination)
            return destination
        return super().snapshot(destination)


if __name__ == "__main__":
    RenodeWorker().serve()
