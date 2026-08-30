#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re

import OMSimulator


def identifier(value: str) -> str:
    result = re.sub(r"[^A-Za-z0-9_]", "_", value)
    return result if result and not result[0].isdigit() else f"ael_{result}"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ssp", required=True)
    parser.add_argument("--system-id", required=True)
    parser.add_argument("--starts", required=True)
    parser.add_argument("--result", required=True)
    parser.add_argument("--log", required=True)
    parser.add_argument("--stop-time", type=float, default=0.006)
    arguments = parser.parse_args()

    OMSimulator.setLogFile(arguments.log)
    model = OMSimulator.importFile(arguments.ssp)
    model.startTime = 0.0
    model.stopTime = arguments.stop_time
    model.resultFile = arguments.result
    model.instantiate()
    root = identifier(arguments.system_id)
    starts = json.loads(open(arguments.starts, encoding="utf-8").read())
    for target, spec in sorted(starts.items()):
        cref = f"{root}.{target}"
        if spec["type"] == "integer":
            model.setInteger(cref, int(spec["value"]))
        elif spec["type"] == "boolean":
            model.setBoolean(cref, bool(spec["value"]))
        else:
            model.setReal(cref, float(spec["value"]))
    model.initialize()
    model.simulate()
    model.terminate()
    model.delete()


if __name__ == "__main__":
    main()
