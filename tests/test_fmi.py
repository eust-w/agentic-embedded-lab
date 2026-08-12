from __future__ import annotations

import pytest

from ael.contracts import BackendName, Connection, Port, SystemComponent, SystemManifest
from ael.fmi import build_schedule, export_ssp, validate_fmi_topology


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
    system = SystemManifest(name="one", components=[component("a")])
    output = export_ssp(system, tmp_path / "SystemStructure.ssd")
    assert output.exists()
    assert "SystemStructureDescription" in output.read_text(encoding="utf-8")


def test_nonrollback_component_is_explicit_in_schedule() -> None:
    renode = component("mcu").model_copy(update={"backend": BackendName.RENODE})
    schedule = build_schedule(SystemManifest(name="nonrollback", components=[renode]), 1000)
    assert schedule.non_rollback == {"mcu"}
