from __future__ import annotations

import pytest

from ael.contracts import InstrumentOperationRequest, UnitValue
from ael.lab import InstrumentPolicy, load_board_catalog


def test_all_reference_boards_are_explicitly_unverified(workspace) -> None:
    catalog = load_board_catalog(workspace)
    assert len(catalog.boards) == 5
    assert {board.validation_status for board in catalog.boards} == {"unverified"}
    assert all(board.samples_required >= 3 for board in catalog.boards)


def test_instrument_policy_rejects_raw_commands_and_ranges() -> None:
    policy = InstrumentPolicy()
    with pytest.raises(ValueError, match="unknown parameters"):
        policy.validate(
            InstrumentOperationRequest(
                instrument_id="scope",
                operation="scope.capture",
                calibration_id="cal",
                parameters={"scpi": "*RST"},
            )
        )
    with pytest.raises(ValueError, match="above"):
        policy.validate(
            InstrumentOperationRequest(
                instrument_id="vna",
                operation="vna.sparameters",
                calibration_id="cal",
                parameters={
                    "start_frequency": UnitValue(value=1e9, unit="Hz"),
                    "stop_frequency": UnitValue(value=7e9, unit="Hz"),
                    "points": 101,
                },
            )
        )
