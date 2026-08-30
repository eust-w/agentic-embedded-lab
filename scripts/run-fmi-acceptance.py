#!/usr/bin/env python3
"""Removed Python FMI implementation.

FMI/SSP acceptance is owned by the Go runtime. This compatibility entrypoint
fails explicitly so automation cannot accidentally execute archived Python
contracts or treat stale evidence as current.
"""

import sys

sys.stderr.write(
    "Python FMI acceptance was removed. Run: "
    "go run ./cmd/ael-fmi-acceptance --workspace .\n"
)
raise SystemExit(2)
