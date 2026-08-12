from __future__ import annotations

import importlib.util
import json
from pathlib import Path

from fmpy import read_model_description


def load_packager():
    script = Path(__file__).parents[1] / "scripts/package_fmu.py"
    specification = importlib.util.spec_from_file_location("ael_package_fmu", script)
    assert specification and specification.loader
    module = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(module)
    return module


def test_generated_fmu_is_schema_valid_and_dimensionless_units_are_omitted(tmp_path) -> None:
    library = tmp_path / "TestFmu.so"
    library.write_bytes(b"test-only-placeholder")
    ports = tmp_path / "ports.json"
    ports.write_text(
        json.dumps(
            [
                {
                    "name": "input",
                    "direction": "input",
                    "data_type": "real",
                    "unit": "1",
                },
                {
                    "name": "output",
                    "direction": "output",
                    "data_type": "real",
                    "unit": "V",
                },
            ]
        ),
        encoding="utf-8",
    )
    output = tmp_path / "TestFmu.fmu"
    load_packager().package("TestFmu", library, ports, output)

    description = read_model_description(output, validate=True)
    input_variable = next(
        variable for variable in description.modelVariables if variable.name == "input"
    )
    assert input_variable.start == "0.0"
    assert input_variable.unit is None
