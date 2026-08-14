from __future__ import annotations

import os
import re
import shutil
import subprocess
from dataclasses import dataclass

from ael.constants import ToolVersion
from ael.contracts import BackendName

from .base import AdapterProbe

VERSION_PATTERN = re.compile(r"(?<!\d)(\d+(?:\.\d+){0,3})(?!\d)")


@dataclass(frozen=True)
class ProcessTool:
    backend: BackendName
    definition: ToolVersion

    def resolve(self) -> str | None:
        if self.definition.environment_variable:
            configured = os.environ.get(self.definition.environment_variable)
            if configured:
                return configured if os.path.isfile(configured) else shutil.which(configured)
        for command in self.definition.commands:
            resolved = shutil.which(command)
            if resolved:
                return resolved
        return None

    def probe(self) -> AdapterProbe:
        command = self.resolve()
        if command is None:
            return AdapterProbe(
                backend=self.backend,
                available=False,
                command=None,
                detected_version=None,
                expected_version=self.definition.version,
                reason="tool executable not found",
            )
        detected = self._detect_version(command)
        return AdapterProbe(
            backend=self.backend,
            available=True,
            command=command,
            detected_version=detected,
            expected_version=self.definition.version,
            reason=None if detected else "executable found but version could not be detected",
        )

    @staticmethod
    def _detect_version(command: str) -> str | None:
        for argument in ("--version", "-v", "-version"):
            try:
                result = subprocess.run(
                    [command, argument],
                    capture_output=True,
                    text=True,
                    timeout=5,
                    check=False,
                )
            except (OSError, subprocess.TimeoutExpired):
                continue
            match = VERSION_PATTERN.search(f"{result.stdout}\n{result.stderr}")
            if match:
                return match.group(1)
        return None
