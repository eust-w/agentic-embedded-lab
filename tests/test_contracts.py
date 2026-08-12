from __future__ import annotations

import pytest
from pydantic import ValidationError

from ael.contracts import ExperimentSpec, UnitValue


def test_unknown_fields_are_rejected() -> None:
    with pytest.raises(ValidationError, match="Extra inputs"):
        ExperimentSpec.model_validate(
            {
                "name": "bad",
                "system": "system.yaml",
                "duration_us": 1000,
                "unknown": True,
            }
        )


@pytest.mark.parametrize("unit", ["V", "mA", "Cel", "m/s", "dBm", "1"])
def test_supported_ucum_subset(unit: str) -> None:
    assert UnitValue(value=1, unit=unit).unit == unit


@pytest.mark.parametrize("unit", ["volts", "hello world", "🌡️"])
def test_unknown_or_unsafe_units_are_rejected(unit: str) -> None:
    with pytest.raises(ValidationError):
        UnitValue(value=1, unit=unit)


def test_timeline_must_align_to_macro_step() -> None:
    with pytest.raises(ValidationError, match="divisible"):
        ExperimentSpec(
            name="misaligned", system="system.yaml", duration_us=1500, macro_step_us=1000
        )
