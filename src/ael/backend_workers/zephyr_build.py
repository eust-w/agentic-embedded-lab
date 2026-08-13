from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path
from typing import Any

from ael.contracts import Event

from .base import BackendWorker


class ZephyrBuildWorker(BackendWorker):
    backend_name = "zephyr_build"
    expected_version = "4.4.2"
    commands = ("west",)

    def _version(self) -> str | None:
        base = os.environ.get("ZEPHYR_BASE")
        if not base:
            return None
        version_file = Path(base) / "VERSION"
        if not version_file.is_file():
            return None
        values = {
            key.strip(): value.strip()
            for line in version_file.read_text(encoding="utf-8").splitlines()
            if "=" in line
            for key, value in [line.split("=", 1)]
        }
        try:
            return ".".join(values[key] for key in ("VERSION_MAJOR", "VERSION_MINOR", "PATCHLEVEL"))
        except KeyError:
            return None

    def step(
        self, step_us: int
    ) -> tuple[dict[str, Any], dict[str, Any], list[Event], dict[str, str]]:
        del step_us
        case_id = int(self.component.properties.get("case_id", 0))
        variant = str(self.component.properties.get("variant", ""))
        if case_id not in {1, 2, 3} or variant not in {"faulty", "fixed"}:
            raise ValueError("zephyr_build supports case_id 1..3 and faulty/fixed variants")
        source = self.property_path("source", required=True)
        build = self.runtime_dir / "build"
        config = source / "conf" / f"case{case_id}-{variant}.conf"
        overlay = (
            source
            / "overlays"
            / (f"case2-{variant}.overlay" if case_id == 2 else "reference.overlay")
        )
        if not config.is_file() or not overlay.is_file():
            raise FileNotFoundError(f"missing controlled Zephyr build asset for case {case_id}")
        command = [
            str(self.tool),
            "build",
            "-p",
            "always",
            "-b",
            "stm32f4_disco",
            str(source),
            "-d",
            str(build),
            "--",
            f"-DEXTRA_CONF_FILE={config}",
            f"-DDTC_OVERLAY_FILE={overlay}",
        ]
        result = subprocess.run(
            command,
            cwd=self.runtime_dir,
            capture_output=True,
            text=True,
            timeout=int(self.component.properties.get("timeout_s", 300)),
            check=False,
            env={**os.environ, "AEL_SEED": str(self.seed)},
        )
        log = self.runtime_dir / "zephyr-build.log"
        log.write_text(
            f"$ {' '.join(command)}\n\n{result.stdout}\n{result.stderr}", encoding="utf-8"
        )
        config_text = ""
        config_path = build / "zephyr" / ".config"
        if config_path.is_file():
            config_text = config_path.read_text(encoding="utf-8")
        dts_text = ""
        dts_path = build / "zephyr" / "zephyr.dts"
        if dts_path.is_file():
            dts_text = dts_path.read_text(encoding="utf-8")
        if case_id == 1:
            mechanism_failed = "CONFIG_AEL_FEATURE=y" not in config_text
            detail = "feature object omitted by resolved Kconfig dependency"
        elif case_id == 2:
            match = re.search(r"ael-probe-address\s*=\s*<\s*(0x[0-9a-fA-F]+)", dts_text)
            resolved = int(match.group(1), 16) if match else None
            mechanism_failed = resolved != 0x40011000
            detail = f"resolved devicetree address={resolved!r}"
        else:
            mechanism_failed = result.returncode != 0
            detail = f"link returncode={result.returncode}"
        events = [
            Event(
                sequence=0,
                virtual_time_us=self.virtual_time_us,
                source=self.component.id,
                type=f"zephyr.build.case{case_id}",
                payload={
                    "variant": variant,
                    "returncode": result.returncode,
                    "mechanism_failed": mechanism_failed,
                    "detail": detail,
                },
                fidelity_ref="zephyr_build:tool-executed",
            )
        ]
        artifacts = {"build_log": self.artifact_reference(log)}
        for label, path in (
            ("dotconfig", config_path),
            ("devicetree", dts_path),
            ("firmware_elf", build / "zephyr" / "zephyr.elf"),
        ):
            if path.is_file():
                artifacts[label] = self.artifact_reference(path)
        value = int(mechanism_failed)
        return {"failure": value}, {"failure": value}, events, artifacts


if __name__ == "__main__":
    ZephyrBuildWorker().serve()
