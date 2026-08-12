from __future__ import annotations

import os
from pathlib import Path

from ael.adapters.subprocess_adapter import SubprocessAdapter
from ael.contracts import BackendName, SystemComponent


def test_ngspice_process_adapter_executes_fixed_binary_protocol(
    tmp_path: Path, monkeypatch
) -> None:
    tool = tmp_path / "fake-ngspice"
    tool.write_text(
        """#!/usr/bin/env python3
import pathlib, sys
if '--version' in sys.argv or '-v' in sys.argv:
    print('ngspice-46')
    raise SystemExit(0)
log = pathlib.Path(sys.argv[sys.argv.index('-o') + 1])
raw = pathlib.Path(sys.argv[sys.argv.index('-r') + 1])
log.write_text('ael_failure = 0.000000e+00\\nAEL_EVENT circuit.ok {}\\n')
raw.write_bytes(b'raw')
""",
        encoding="utf-8",
    )
    tool.chmod(0o755)
    model = tmp_path / "model.cir"
    model.write_text(".end\n", encoding="utf-8")
    monkeypatch.setenv("AEL_WORKSPACE", str(tmp_path))
    monkeypatch.setenv("AEL_NGSPICE_BIN", str(tool))
    adapter = SubprocessAdapter(BackendName.NGSPICE, "ael.backend_workers.ngspice", "46")
    probe = adapter.probe()
    assert probe.available and probe.detected_version == "46"
    adapter.prepare(
        SystemComponent(
            id="circuit",
            type="test",
            backend=BackendName.NGSPICE,
            model="model.cir",
            step_us=1000,
        ),
        seed=7,
    )
    try:
        result = adapter.step(0, 1000)
        assert result.metrics["failure"] == 0.0
        assert result.events[0].type == "circuit.ok"
    finally:
        adapter.shutdown()
        os.environ.pop("AEL_NGSPICE_BIN", None)
