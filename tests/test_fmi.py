from __future__ import annotations

from pathlib import Path
from xml.etree import ElementTree

import pytest

from ael.adapters.base import AdapterStepResult
from ael.contracts import BackendName, Connection, Port, SystemComponent, SystemManifest
from ael.fmi import (
    FmiBridgeServer,
    FmiOrchestrator,
    build_schedule,
    export_ssp,
    validate_fmi_topology,
)


def component(identifier: str, feedthrough: bool = False) -> SystemComponent:
    return SystemComponent(
        id=identifier,
        type="test",
        backend=BackendName.SYNTHETIC,
        step_us=1000,
        direct_feedthrough=feedthrough,
        ports=[
            Port(name="input", direction="input", data_type="real"),
            Port(name="output", direction="output", data_type="real"),
        ],
    )


def test_schedule_uses_gcd() -> None:
    system = SystemManifest(
        name="multi",
        components=[component("a"), component("b").model_copy(update={"step_us": 2500})],
    )
    assert build_schedule(system, 5000).communication_step_us == 500


def test_algebraic_loop_is_rejected() -> None:
    system = SystemManifest(
        name="loop",
        components=[component("a", True), component("b", True)],
        connections=[
            Connection(source="a.output", target="b.input"),
            Connection(source="b.output", target="a.input"),
        ],
    )
    with pytest.raises(ValueError, match="algebraic loop"):
        validate_fmi_topology(system)


def test_ssp_export(tmp_path) -> None:
    system = SystemManifest(name="one-with-punctuation", components=[component("a")])
    output = export_ssp(system, tmp_path / "SystemStructure.ssd")
    assert output.exists()
    tree = ElementTree.parse(output)
    ssd_namespace = "http://ssp-standard.org/SSP1/SystemStructureDescription"
    ssc_namespace = "http://ssp-standard.org/SSP1/SystemStructureCommon"
    oms_namespace = "https://raw.githubusercontent.com/OpenModelica/OMSimulator/master/schema/oms.xsd"
    assert tree.getroot().attrib["name"] == "one_with_punctuation"
    ssd_tags = {
        "SystemStructureDescription",
        "System",
        "Elements",
        "Component",
        "Connectors",
        "Connector",
        "Connections",
        "Connection",
        "Annotations",
    }
    for node in tree.iter():
        local_name = node.tag.rsplit("}", 1)[-1]
        if local_name == "Annotations" and node.tag.startswith(f"{{{oms_namespace}}}"):
            expected_namespace = oms_namespace
        elif local_name in ssd_tags:
            expected_namespace = ssd_namespace
        elif local_name in {"Annotation", "Real", "Integer", "Boolean", "String"}:
            expected_namespace = ssc_namespace
        else:
            expected_namespace = oms_namespace
        assert node.tag.startswith(f"{{{expected_namespace}}}")
    assert tree.find(f".//{{{ssc_namespace}}}Real") is not None
    master = tree.find(f".//{{{oms_namespace}}}FixedStepMaster")
    assert master is not None
    assert master.attrib["stepSize"] == "0.001000000"
    xml = output.read_text(encoding="utf-8")
    assert "<ssd:SystemStructureDescription" in xml
    assert "<ssd:System " in xml
    assert "<ssc:Real" in xml


def test_nonrollback_component_is_explicit_in_schedule() -> None:
    renode = component("mcu").model_copy(update={"backend": BackendName.RENODE})
    schedule = build_schedule(SystemManifest(name="nonrollback", components=[renode]), 1000)
    assert schedule.non_rollback == {"mcu"}


def test_fmi_bridge_uses_short_workspace_relative_socket_path(tmp_path, monkeypatch) -> None:
    sockets = []

    class FakeBridge:
        def __init__(self, backend, components, socket_path, catalog, seed):
            sockets.append(socket_path)

        def start(self) -> None:
            pass

        def close(self) -> None:
            pass

    class Completed:
        returncode = 0
        stdout = ""
        stderr = ""

    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr("ael.fmi.FmiBridgeServer", FakeBridge)
    monkeypatch.setattr("ael.fmi.subprocess.run", lambda *args, **kwargs: Completed())
    system = SystemManifest(
        name="short-socket",
        components=[component("mcu").model_copy(update={"backend": BackendName.RENODE})],
    )
    FmiOrchestrator().run(
        system,
        tmp_path / "system.ssp",
        stop_time_s=0.001,
        timeout_s=1,
        seed=1,
    )
    assert len(sockets) == 1
    assert not sockets[0].is_absolute()
    assert len(str(sockets[0])) < 108


def test_fmi_bridge_accepts_qualified_ssp_instance_name() -> None:
    class FakeAdapter:
        def inject(self, target, value, virtual_time_us):
            assert target == "mcu.input"
            assert value == 2.0
            return []

        def step(self, virtual_time_us, step_us):
            assert (virtual_time_us, step_us) == (0, 1000)
            return AdapterStepResult(outputs={"output": 3.0}, metrics={}, events=[])

    bridge = object.__new__(FmiBridgeServer)
    bridge.components = {"mcu": component("mcu")}
    bridge.adapters = {"mcu": FakeAdapter()}
    assert bridge.exchange("STEP root.mcu 0 0.001 r1=2") == "OK r2=3.0"
    assert bridge.exchange("STEP /workspace/model/temp/mcu 0 0.001 r1=2") == "OK r2=3.0"


def test_fmi_bridge_rejects_nonterminal_component_match() -> None:
    bridge = object.__new__(FmiBridgeServer)
    bridge.components = {"mcu": component("mcu")}
    bridge.adapters = {}
    with pytest.raises(ValueError, match="unknown FMI component instance"):
        bridge.exchange("STEP /workspace/mcu/temp/other 0 0.001")


def test_fmi_orchestrator_fails_closed_on_coordinator_log_error(tmp_path, monkeypatch) -> None:
    class Completed:
        returncode = 0
        stdout = ""
        stderr = ""

    def fake_run(command, **kwargs):
        log_argument = next(item for item in command if item.startswith("--logFile="))
        Path(log_argument.split("=", 1)[1]).write_text(
            "info: started\nerror: hidden doStep failure\n",
            encoding="utf-8",
        )
        return Completed()

    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr("ael.fmi.subprocess.run", fake_run)
    system = SystemManifest(name="fail-closed", components=[component("test")])
    with pytest.raises(RuntimeError, match="reported an error"):
        FmiOrchestrator().run(
            system,
            tmp_path / "system.ssp",
            stop_time_s=0.001,
            timeout_s=1,
            seed=1,
        )
