from __future__ import annotations

import hashlib
import os
from io import StringIO
from pathlib import Path

import pytest

from ael.adapters.subprocess_adapter import SubprocessAdapter, backend_cpu_limit
from ael.backend_protocol import BackendOperation, BackendRequest, BackendResponse
from ael.backend_workers.ngspice import NgspiceWorker
from ael.backend_workers.ns3 import Ns3Worker
from ael.backend_workers.openems import OpenEmsWorker
from ael.contracts import BackendName, SystemComponent


def test_backend_cpu_limit_is_runner_safe_and_validated(monkeypatch) -> None:
    monkeypatch.delenv("AEL_BACKEND_CPUS", raising=False)
    assert backend_cpu_limit() == 2
    monkeypatch.setenv("AEL_BACKEND_CPUS", "0.5")
    assert backend_cpu_limit() == 0.5
    monkeypatch.setenv("AEL_BACKEND_CPUS", "65")
    with pytest.raises(ValueError, match="between"):
        backend_cpu_limit()


def test_backend_container_places_writable_config_under_tmpfs(
    tmp_path: Path, monkeypatch
) -> None:
    class FakeProcess:
        stdin = None
        stdout = None
        stderr = None

        @staticmethod
        def poll() -> None:
            return None

    monkeypatch.setenv("AEL_WORKSPACE", str(tmp_path))
    monkeypatch.setenv("AEL_RENODE_IMAGE", "ael-renode:test")
    monkeypatch.setattr("ael.adapters.subprocess_adapter.shutil.which", lambda _: "/bin/docker")
    monkeypatch.setattr(
        "ael.adapters.subprocess_adapter.subprocess.Popen", lambda *args, **kwargs: FakeProcess()
    )
    adapter = SubprocessAdapter(BackendName.RENODE, "ael.backend_workers.renode", "1.16.1")
    adapter._start()
    assert "--read-only" in adapter.launch_command
    assert "--env=HOME=/tmp/ael-home" in adapter.launch_command
    assert "--env=XDG_CONFIG_HOME=/tmp/ael-config" in adapter.launch_command


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
deck = pathlib.Path(sys.argv[-1]).read_text().splitlines()
assert deck[0] == 'AEL test deck'
assert deck[1].startswith('.param AEL_fault_scale=')
log.write_text('ael_failure = 0.000000e+00\\nAEL_EVENT circuit.ok {}\\n')
raw.write_bytes(b'raw')
""",
        encoding="utf-8",
    )
    tool.chmod(0o755)
    model = tmp_path / "model.cir"
    model.write_text("AEL test deck\n.end\n", encoding="utf-8")
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


def test_backend_worker_rejects_wrong_protocol_without_crashing(monkeypatch) -> None:
    monkeypatch.setenv("AEL_NGSPICE_BIN", "/does/not/exist")
    output = StringIO()
    NgspiceWorker().serve(
        StringIO(
            '{"api_version":"ael.dev/v1","request_id":"bad",'
            '"operation":"probe","payload":{}}\n'
        ),
        output,
    )
    response = BackendResponse.model_validate_json(output.getvalue())
    assert response.api_version == "ael.dev/backend/v1"
    assert response.ok is False
    assert response.request_id == "bad"


def test_ns3_version_probe_uses_the_wrapper_show_command(tmp_path: Path, monkeypatch) -> None:
    tool = tmp_path / "ns3"
    tool.write_text(
        "#!/bin/sh\n"
        "if [ \"$1 $2\" = \"show version\" ]; then echo 'ns-3.47'; exit 0; fi\n"
        "echo ns3 >&2\nexit 2\n",
        encoding="utf-8",
    )
    tool.chmod(0o755)
    monkeypatch.setenv("AEL_NS3_BIN", str(tool))
    worker = Ns3Worker()
    assert worker.detected_version == "3.47"


def test_ns3_version_probe_accepts_image_build_attestation(tmp_path: Path, monkeypatch) -> None:
    tool = tmp_path / "ns3"
    tool.write_text("#!/bin/sh\necho should-not-run >&2\nexit 99\n", encoding="utf-8")
    tool.chmod(0o755)
    (tmp_path / ".ael-version").write_text("3.47\n", encoding="utf-8")
    monkeypatch.setenv("AEL_NS3_BIN", str(tool))
    assert Ns3Worker().detected_version == "3.47"


def test_version_probe_prefers_pinned_semver_over_architecture_width(
    tmp_path: Path, monkeypatch
) -> None:
    tool = tmp_path / "openEMS"
    tool.write_text(
        "#!/bin/sh\necho 'openEMS 64bit -- version v0.0.36'\n", encoding="utf-8"
    )
    tool.chmod(0o755)
    monkeypatch.setenv("AEL_OPENEMS_BIN", str(tool))
    worker = OpenEmsWorker()
    assert worker.detected_version == "0.0.36"


def test_ns3_worker_executes_matching_read_only_precompiled_model(
    tmp_path: Path, monkeypatch
) -> None:
    tool = tmp_path / "ns3"
    tool.write_text("#!/bin/sh\nexit 99\n", encoding="utf-8")
    tool.chmod(0o755)
    (tmp_path / ".ael-version").write_text("3.47\n", encoding="utf-8")
    model = tmp_path / "network.cc"
    model.write_text("// fixed test network\n", encoding="utf-8")
    binary = tmp_path / "ael-network"
    binary.write_text(
        "#!/bin/sh\n"
        "echo 'AEL_METRIC packet_loss=0.01'\n"
        "echo 'AEL_EVENT ns3.network {\"calibrated\":false}'\n",
        encoding="utf-8",
    )
    binary.chmod(0o755)
    digest = tmp_path / "ael-network.sha256"
    digest.write_text(hashlib.sha256(model.read_bytes()).hexdigest() + "\n", encoding="utf-8")
    monkeypatch.setenv("AEL_WORKSPACE", str(tmp_path))
    monkeypatch.setenv("AEL_NS3_BIN", str(tool))
    monkeypatch.setenv("AEL_NS3_PRECOMPILED", str(binary))
    monkeypatch.setenv("AEL_NS3_MODEL_SHA256", str(digest))
    worker = Ns3Worker()
    component = SystemComponent(
        id="network",
        type="test",
        backend=BackendName.NS3,
        model="network.cc",
        step_us=1000,
    )
    response = worker.handle(
        BackendRequest(
            request_id="prepare",
            operation=BackendOperation.PREPARE,
            payload={"component": component.model_dump(mode="json"), "seed": 3},
        )
    )
    assert response.ok
    _, metrics, events, _ = worker.step(1000)
    assert metrics["packet_loss"] == 0.01
    assert events[0].type == "ns3.network"


def test_renode_adapter_uses_agent_readable_register_output(
    tmp_path: Path, monkeypatch
) -> None:
    tool = tmp_path / "renode"
    tool.write_text(
        "#!/usr/bin/env python3\n"
        "import pathlib, sys\n"
        "if '-v' in sys.argv or '--version' in sys.argv:\n"
        "    print('Renode 1.16.1')\n"
        "    raise SystemExit(0)\n"
        "script = pathlib.Path(sys.argv[-1]).read_text()\n"
        "assert 'self.Machine.SystemBus.ReadDoubleWord(537000968)' in script\n"
        "print('AEL_REGISTER:failure:1')\n",
        encoding="utf-8",
    )
    tool.chmod(0o755)
    model = tmp_path / "platform.resc"
    model.write_text("mach create 'test'\n", encoding="utf-8")
    monkeypatch.setenv("AEL_WORKSPACE", str(tmp_path))
    monkeypatch.setenv("AEL_RENODE_BIN", str(tool))
    adapter = SubprocessAdapter(BackendName.RENODE, "ael.backend_workers.renode", "1.16.1")
    adapter.prepare(
        SystemComponent(
            id="mcu",
            type="test",
            backend=BackendName.RENODE,
            model="platform.resc",
            step_us=1000,
            properties={"output_registers": {"failure": 0x2001FC08}},
        ),
        seed=1,
    )
    try:
        result = adapter.step(0, 1000)
        assert result.outputs["failure"] == 1
    finally:
        adapter.shutdown()
