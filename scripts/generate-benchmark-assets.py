#!/usr/bin/env python3
"""Render the 24 mechanism-backed faulty/fixed benchmark contracts.

The generator deliberately selects different controlled assets for each variant.
No experiment is allowed to inject a boolean that directly chooses pass/fail.
"""

from __future__ import annotations

from copy import deepcopy
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]

CAUSES = {
    1: ["Kconfig dependency false", "feature object omitted", "requested behavior absent"],
    2: [
        "mapping mismatch",
        "generated devicetree contains wrong address",
        "oracle rejects mapping",
    ],
    3: ["section exceeds RAM", "linker rejects image", "firmware is not produced"],
    4: ["clock ready condition absent", "bounded boot poll expires", "boot assertion fails"],
    5: [
        "active-low bounce sampled",
        "polarity/debounce logic misclassifies edge",
        "input action fails",
    ],
    6: [
        "32-bit counter wraps",
        "elapsed-time implementation evaluates boundary",
        "deadline is classified",
    ],
    7: [
        "UART frame is encoded",
        "baud/parity configuration is checked",
        "peer accepts or rejects frame",
    ],
    8: [
        "RX producer fills ring",
        "ISR push meets full-buffer policy",
        "byte preservation oracle runs",
    ],
    9: ["IRQ consumes control budget", "1 kHz deadline is evaluated", "starvation is recorded"],
    10: [
        "memory transfer completes",
        "completion flag races consumer",
        "copied bytes and completion are checked",
    ],
    11: [
        "target leaves I2C bus busy",
        "recovery clocks and STOP are attempted",
        "retry limit is checked",
    ],
    12: ["SPI frame and CRC are produced", "target verifies CRC", "sample is accepted or rejected"],
    13: [
        "two lock orders form wait-for graph",
        "progress watchdog observes cycle",
        "deadlock is reported",
    ],
    14: ["high/medium/low scheduling contention", "lock wait is measured", "deadline oracle runs"],
    15: ["requested stack crosses guard budget", "fault evidence is emitted", "fault oracle runs"],
    16: [
        "health progress gates watchdog feed",
        "feed deadline expires",
        "reset-loop condition is recorded",
    ],
    17: [
        "power loss interrupts candidate commit",
        "boot journal selects a slot",
        "rollback oracle checks committed image",
    ],
    18: [
        "load current steps",
        "LDO impedance and decoupling produce rail transient",
        "minimum voltage is measured",
    ],
    19: [
        "load step crosses BOR threshold",
        "power FMU emits brownout",
        "firmware reset policy observes event",
    ],
    20: [
        "firmware power states define duty cycle",
        "current is integrated",
        "battery-life oracle runs",
    ],
    21: [
        "loss enters RC thermal network",
        "junction temperature rises",
        "throttle threshold is checked",
    ],
    22: [
        "LR-WPAN interferer transmits",
        "MAC retries and energy accumulate",
        "delivery oracle runs",
    ],
    23: [
        "Wi-Fi link is partitioned",
        "retry/backoff state advances",
        "reconnect budget is checked",
    ],
    24: [
        "antenna geometry detunes and openEMS computes S11",
        "mismatch loss changes ns-3 delivery and retries",
        "firmware retry policy changes current",
        "ngspice rail and Modelica temperature respond",
    ],
}

BACKEND = {
    **{case_id: "zephyr_build" for case_id in range(1, 4)},
    **{case_id: "renode" for case_id in range(4, 18)},
    18: "ngspice",
    19: "ngspice",
    20: "openmodelica",
    21: "openmodelica",
    22: "ns3",
    23: "ns3",
    24: "openems",
}


def load_yaml(path: str) -> dict[str, object]:
    return yaml.safe_load((ROOT / path).read_text(encoding="utf-8"))


def dump_yaml(path: str, value: object) -> None:
    destination = ROOT / path
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(yaml.safe_dump(value, sort_keys=False), encoding="utf-8")


def render_system_variants() -> None:
    build_source = "firmware/zephyr-build"
    for case_id in range(1, 4):
        for variant in ("faulty", "fixed"):
            dump_yaml(
                f"benchmarks/systems/zephyr-build-case{case_id:02d}-{variant}.yaml",
                {
                    "api_version": "ael.dev/v1",
                    "kind": "SystemManifest",
                    "name": f"zephyr-build-case{case_id:02d}-{variant}",
                    "components": [
                        {
                            "id": "build",
                            "type": "zephyr-controlled-build",
                            "backend": "zephyr_build",
                            "step_us": 1000,
                            "ports": [
                                {
                                    "name": "failure",
                                    "direction": "output",
                                    "data_type": "integer",
                                }
                            ],
                            "properties": {
                                "case_id": case_id,
                                "variant": variant,
                                "source": build_source,
                                "timeout_s": 300,
                            },
                        }
                    ],
                    "fidelity": {
                        "build": "tool-executed-zephyr-4.4.2",
                        "hardware": "unverified",
                    },
                },
            )

    template_names = (
        "renode-digital",
        "power-renode",
        "thermal-renode",
        "network-renode",
        "five-domain",
    )
    for name in template_names:
        template = load_yaml(f"benchmarks/systems/{name}.yaml")
        for variant in ("faulty", "fixed"):
            system = deepcopy(template)
            system["name"] = f"{system['name']}-{variant}"
            for component in system["components"]:
                ports = component.get("ports", [])
                component["ports"] = [
                    port for port in ports if port.get("name") not in {"fixed", "fault_scale"}
                ]
                if component["backend"] == "renode":
                    component["properties"]["firmware"] = (
                        f"firmware/zephyr/build-stm32-{variant}/zephyr/zephyr.elf"
                    )
                    component["properties"]["input_registers"].pop("fixed", None)
            system["connections"] = [
                item
                for item in system.get("connections", [])
                if not item["source"].endswith(".fault_scale")
                and not item["target"].endswith(".fault_scale")
            ]
            dump_yaml(f"benchmarks/systems/{name}-{variant}.yaml", system)


def system_for(case_id: int, fixed: bool) -> str:
    variant = "fixed" if fixed else "faulty"
    if case_id <= 3:
        return f"benchmarks/systems/zephyr-build-case{case_id:02d}-{variant}.yaml"
    if case_id <= 17:
        return f"benchmarks/systems/renode-digital-{variant}.yaml"
    if case_id == 18:
        return "benchmarks/systems/ngspice-power.yaml"
    if case_id == 19:
        return f"benchmarks/systems/power-renode-{variant}.yaml"
    if case_id == 20:
        return "benchmarks/systems/modelica-domain.yaml"
    if case_id == 21:
        return f"benchmarks/systems/thermal-renode-{variant}.yaml"
    if case_id == 22:
        return "benchmarks/systems/ns3-network.yaml"
    if case_id == 23:
        return f"benchmarks/systems/network-renode-{variant}.yaml"
    return f"benchmarks/systems/five-domain-{variant}.yaml"


def stimuli(case_id: int, fixed: bool) -> list[dict[str, object]]:
    values: list[dict[str, object]] = []
    if 4 <= case_id <= 17 or case_id in {19, 21, 23, 24}:
        values.append({"at_us": 0, "target": "mcu.case_id", "value": case_id})
    if case_id in {18, 19, 24}:
        values.extend(
            [
                {
                    "at_us": 0,
                    "target": "circuit.source_resistance_ohm",
                    "value": 12.0 if not fixed else 0.15,
                    "unit": "Ohm",
                },
                {
                    "at_us": 0,
                    "target": "circuit.capacitance_uF",
                    "value": 2.2 if not fixed else 47.0,
                    "unit": "uF",
                },
                {
                    "at_us": 0,
                    "target": "circuit.load_microamp",
                    "value": 160000.0 if not fixed else 60000.0,
                    "unit": "uA",
                },
                {"at_us": 0, "target": "circuit.rf_retries", "value": 0.0},
            ]
        )
    if case_id in {20, 21, 24}:
        values.extend(
            [
                {
                    "at_us": 0,
                    "target": "plant.thermal_resistance_K_per_W",
                    "value": 95.0 if not fixed else 18.0,
                    "unit": "K/W",
                },
                {
                    "at_us": 0,
                    "target": "plant.sleep_current_A",
                    "value": 0.0012 if not fixed else 0.000008,
                    "unit": "A",
                },
                {
                    "at_us": 0,
                    "target": "plant.duty_cycle",
                    "value": 0.20 if not fixed else 0.01,
                },
                {
                    "at_us": 0,
                    "target": "plant.input_power",
                    "value": 0.65 if not fixed else 0.18,
                    "unit": "W",
                },
                {"at_us": 0, "target": "plant.rf_retries", "value": 0.0},
            ]
        )
    if case_id in {22, 23, 24}:
        values.extend(
            [
                {
                    "at_us": 0,
                    "target": "network.interference_dbm",
                    "value": -42.0 if not fixed else -95.0,
                    "unit": "dBm",
                },
                {
                    "at_us": 0,
                    "target": "network.partition_ms",
                    "value": 800.0 if not fixed else 0.0,
                    "unit": "ms",
                },
                {"at_us": 0, "target": "network.retry_limit", "value": 3 if not fixed else 8},
            ]
        )
    if case_id in {22, 24}:
        values.append({"at_us": 0, "target": "network.protocol", "value": 0})
    if case_id == 23:
        values.append({"at_us": 0, "target": "network.protocol", "value": 1})
    if case_id == 24:
        values.append(
            {
                "at_us": 0,
                "target": "antenna.detune",
                "value": 20.0 if not fixed else 0.0,
                "unit": "mm",
            }
        )
    return values


def assertion_metrics(case_id: int) -> list[str]:
    if case_id <= 3:
        return ["build.failure"]
    if case_id <= 17:
        return ["mcu.failure"]
    if case_id == 18:
        return ["circuit.failure"]
    if case_id == 19:
        return ["circuit.failure"]
    if case_id == 20:
        return ["plant.failure"]
    if case_id == 21:
        return ["plant.failure"]
    if case_id == 22:
        return ["network.failure"]
    if case_id == 23:
        return ["network.failure", "mcu.failure"]
    return [
        "antenna.failure",
        "network.failure",
        "mcu.failure",
        "circuit.failure",
        "plant.failure",
    ]


def render_experiment(case: dict[str, object], fixed: bool) -> dict[str, object]:
    case_id = int(case["id"])
    metrics = assertion_metrics(case_id)
    return {
        "api_version": "ael.dev/v1",
        "kind": "ExperimentSpec",
        "name": f"{case_id:02d}-{case['slug']}-{'fixed' if fixed else 'faulty'}",
        "system": system_for(case_id, fixed),
        "duration_us": (
            1000
            if case_id in {1, 2, 3}
            else 6000
            if case_id in {17, 19, 21, 23, 24}
            else 3000
        ),
        "macro_step_us": 1000,
        "timeout_s": 1800 if case_id == 24 else 300,
        "seed": 1000 + case_id,
        "stimuli": stimuli(case_id, fixed),
        "assertions": [
            {"metric": metric, "operator": "eq", "expected": 0, "critical": True}
            for metric in metrics
        ],
        "observables": metrics,
        "allowed_model_states": [
            "conformance_validated",
            "hardware_validated",
            "production_approved",
        ],
        "tags": [
            "benchmark",
            "mechanism-backed",
            "fixed" if fixed else "faulty",
            f"case-{case_id:02d}",
        ],
    }


def variant_assets(case_id: int, variant: str, experiment: str) -> list[str]:
    if case_id <= 3:
        assets = [f"firmware/zephyr-build/conf/case{case_id}-{variant}.conf"]
        if case_id == 2:
            assets.append(f"firmware/zephyr-build/overlays/case2-{variant}.overlay")
        return assets
    if case_id <= 17 or case_id in {19, 21, 23, 24}:
        return [f"firmware/zephyr/{variant}.conf", experiment]
    return [experiment]


def main() -> None:
    render_system_variants()
    catalog_path = ROOT / "benchmarks/catalog.yaml"
    catalog = yaml.safe_load(catalog_path.read_text(encoding="utf-8"))
    for case in catalog["cases"]:
        case_id = int(case["id"])
        case_dir = ROOT / "benchmarks/cases" / f"{case_id:02d}-{case['slug']}"
        case_dir.mkdir(parents=True, exist_ok=True)
        faulty = case_dir / "faulty.yaml"
        fixed = case_dir / "fixed.yaml"
        faulty.write_text(yaml.safe_dump(render_experiment(case, False), sort_keys=False))
        fixed.write_text(yaml.safe_dump(render_experiment(case, True), sort_keys=False))
        relative_faulty = str(faulty.relative_to(ROOT))
        relative_fixed = str(fixed.relative_to(ROOT))
        declared = [backend for backend in case["backends"] if backend != "native"]
        case["backends"] = list(dict.fromkeys([*declared, BACKEND[case_id]]))
        case.update(
            {
                "readiness": "executable",
                "faulty_asset": relative_faulty,
                "fixed_asset": relative_fixed,
                "experiment": relative_fixed,
                "faulty_experiment": relative_faulty,
                "fixed_experiment": relative_fixed,
                "causal_chain": CAUSES[case_id],
                "seed": 1000 + case_id,
                "mechanism": {
                    "trigger": CAUSES[case_id][0],
                    "execution_backend": BACKEND[case_id],
                    "faulty_assets": variant_assets(case_id, "faulty", relative_faulty),
                    "fixed_assets": variant_assets(case_id, "fixed", relative_fixed),
                    "oracle": "tool-produced failure metric equals zero",
                    "required_evidence": ["tool_event", "artifact"],
                },
            }
        )
        (case_dir / "README.md").write_text(
            f"# {case_id:02d} - {case['title']}\n\n"
            f"Mechanism: {CAUSES[case_id][0]}.\n\n"
            f"Causal chain: {' -> '.join(CAUSES[case_id])}.\n\n"
            f"Fidelity boundary: {case['fidelity_boundary']}\n\n"
            "The variants select different controlled assets; neither experiment contains a "
            "direct pass/fail selector. Tool logs and mechanism events are retained in the "
            "Evidence Bundle. No result is physical-hardware evidence.\n",
            encoding="utf-8",
        )
    catalog["version"] = "0.2.0.dev0"
    catalog_path.write_text(yaml.safe_dump(catalog, sort_keys=False), encoding="utf-8")


if __name__ == "__main__":
    main()
