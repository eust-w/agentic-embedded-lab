#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
from pathlib import Path

from ael.service import AelService


def trace_hash(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("experiment", type=Path)
    parser.add_argument("--repeats", type=int, default=20)
    parser.add_argument("--warmup", action="store_true")
    arguments = parser.parse_args()
    service = AelService(Path.cwd())
    if arguments.warmup:
        service.run_experiment(arguments.experiment)
    hashes: list[str] = []
    statuses: list[str] = []
    for _ in range(arguments.repeats):
        result = service.run_experiment(arguments.experiment)
        statuses.append(result.status.value)
        hashes.append(trace_hash(result.evidence_dir / "events.jsonl"))
    if len(set(statuses)) != 1 or len(set(hashes)) != 1:
        raise SystemExit(
            f"determinism failed: statuses={sorted(set(statuses))} "
            f"hashes={sorted(set(hashes))}"
        )
    print(
        f"deterministic repeats={arguments.repeats} "
        f"status={statuses[0]} trace={hashes[0]}"
    )


if __name__ == "__main__":
    main()
