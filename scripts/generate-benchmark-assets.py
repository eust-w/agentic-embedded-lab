#!/usr/bin/env python3
"""Mechanically render the checked-in benchmark pair configurations."""

from __future__ import annotations

from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]

SYSTEMS = {
    **{case_id: "benchmarks/systems/native-analysis.yaml" for case_id in range(1, 4)},
    **{case_id: "benchmarks/systems/renode-digital.yaml" for case_id in range(4, 17)},
    17: "benchmarks/systems/power-renode.yaml",
    18: "benchmarks/systems/ngspice-power.yaml",
    19: "benchmarks/systems/power-renode.yaml",
    20: "benchmarks/systems/modelica-domain.yaml",
    21: "benchmarks/systems/thermal-renode.yaml",
    22: "benchmarks/systems/ns3-network.yaml",
    23: "benchmarks/systems/network-renode.yaml",
    24: "benchmarks/systems/five-domain.yaml",
}

CAUSES = {
    1: ["Kconfig dependency false", "feature object omitted", "requested behavior absent"],
    2: ["mapping mismatch", "driver binds wrong resource", "peripheral access fails"],
    3: ["section exceeds RAM", "linker rejects image", "firmware is not produced"],
    4: ["clock source never ready", "boot wait does not exit", "application does not start"],
    5: ["polarity/debounce wrong", "edge is misclassified", "input action is incorrect"],
    6: ["counter wraps", "signed/branch elapsed math fails", "deadline is misclassified"],
    7: ["baud/frame mismatch", "receiver framing fails", "UART assertion fails"],
    8: ["ISR races ring head", "unread byte overwritten", "packet loss is observed"],
    9: ["IRQ priority unbounded", "control task starved", "deadline is missed"],
    10: ["DMA flag races consumer", "completion is lost", "transfer does not complete"],
    11: ["I2C NACK leaves bus busy", "recovery clocks omitted", "transaction retries exhaust"],
    12: ["SPI mode/CRC differs", "frame verification fails", "sample is rejected"],
    13: ["mutex order ABBA", "tasks block cyclically", "forward progress stops"],
    14: ["low task holds lock", "high task waits", "medium task causes deadline miss"],
    15: ["stack allocation unbounded", "guard trips", "HardFault evidence is captured"],
    16: ["health check never reaches feed", "watchdog resets", "boot loop repeats"],
    17: ["power loss interrupts OTA", "active image corrupts", "journal recovery determines boot"],
    18: ["load step exceeds headroom", "LDO enters dropout", "rail minimum crosses limit"],
    19: ["decoupling is insufficient", "rail transient crosses BOR", "MCU reset is requested"],
    20: ["sleep leakage is high", "average current rises", "battery-life target is missed"],
    21: ["thermal resistance is high", "temperature rises", "protection/throttling is late"],
    22: ["802.15.4 interference rises", "retry count increases", "loss and energy rise"],
    23: ["network partition occurs", "reconnect retries are unbounded", "policy assertion fails"],
    24: [
        "antenna geometry detunes",
        "RF loss raises network retries",
        "firmware retry policy raises current",
        "supply and thermal loads rise",
    ],
}


def target_for(case_id: int) -> str:
    if case_id <= 3:
        return "analysis"
    if case_id in {18}:
        return "circuit"
    if case_id in {20}:
        return "plant"
    if case_id in {22}:
        return "network"
    return "mcu"


def stimuli(case_id: int, fixed: bool) -> list[dict[str, object]]:
    result: list[dict[str, object]] = []
    if case_id <= 3:
        return [
            {"at_us": 0, "target": "analysis.case_id", "value": case_id},
            {"at_us": 0, "target": "analysis.fixed", "value": fixed},
        ]
    if case_id not in {18, 20, 22}:
        result.extend(
            [
                {"at_us": 0, "target": "mcu.case_id", "value": case_id},
                {"at_us": 0, "target": "mcu.fixed", "value": int(fixed)},
            ]
        )
    fault_scale = 0.0 if fixed else 1.0
    if case_id in {17, 18, 19, 24}:
        result.extend(
            [
                {"at_us": 0, "target": "circuit.fault_scale", "value": fault_scale},
                {"at_us": 0, "target": "circuit.load_microamp", "value": 60000.0},
                {"at_us": 0, "target": "circuit.rf_retries", "value": 0.0},
            ]
        )
    if case_id in {20, 21, 24}:
        result.extend(
            [
                {"at_us": 0, "target": "plant.fault_scale", "value": fault_scale},
                {"at_us": 0, "target": "plant.input_power", "value": 0.2},
                {"at_us": 0, "target": "plant.rf_retries", "value": 0.0},
            ]
        )
    if case_id in {22, 23, 24}:
        result.append(
            {"at_us": 0, "target": "network.fault_scale", "value": fault_scale}
        )
    if case_id == 24:
        result.extend(
            [
                {
                    "at_us": 0,
                    "target": "network.rf_loss_db",
                    "value": 0.0,
                },
                {"at_us": 0, "target": "mcu.network_retries", "value": 0.0},
                {"at_us": 0, "target": "antenna.detune", "value": 0.0 if fixed else 20.0},
            ]
        )
    return result


def render_experiment(case: dict[str, object], fixed: bool) -> dict[str, object]:
    case_id = int(case["id"])
    target = target_for(case_id)
    assertion_metric = f"{target}.failure"
    return {
        "api_version": "ael.dev/v1",
        "kind": "ExperimentSpec",
        "name": f"{case_id:02d}-{case['slug']}-{'fixed' if fixed else 'faulty'}",
        "system": SYSTEMS[case_id],
        "duration_us": 6000 if case_id in {17, 19, 21, 23, 24} else 3000,
        "macro_step_us": 1000,
        "timeout_s": 1800 if case_id == 24 else 300,
        "seed": 1000 + case_id,
        "stimuli": stimuli(case_id, fixed),
        "assertions": [
            {
                "metric": assertion_metric,
                "operator": "eq",
                "expected": 0,
                "critical": True,
            }
        ],
        "observables": [assertion_metric],
        "allowed_model_states": [
            "conformance_validated",
            "hardware_validated",
            "production_approved",
        ],
        "tags": ["benchmark", "fixed" if fixed else "faulty", f"case-{case_id:02d}"],
    }


def main() -> None:
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
            }
        )
        (case_dir / "README.md").write_text(
            f"# {case_id:02d} - {case['title']}\n\n"
            f"Causal chain: {' -> '.join(CAUSES[case_id])}.\n\n"
            f"Fidelity boundary: {case['fidelity_boundary']}\n\n"
            "`faulty.yaml` must fail its correctness assertion; `fixed.yaml` must pass. "
            "Neither result is physical hardware evidence.\n",
            encoding="utf-8",
        )
    catalog_path.write_text(yaml.safe_dump(catalog, sort_keys=False), encoding="utf-8")


if __name__ == "__main__":
    main()
