#!/bin/sh
set -eu

PYTHON_BIN="${AEL_PYTHON:-.venv/bin/python}"
"$PYTHON_BIN" -m pytest
"$PYTHON_BIN" -m ael schema export schemas/v1
"$PYTHON_BIN" -m ael validate examples/experiments/synthetic-smoke.yaml
"$PYTHON_BIN" -m ael run examples/experiments/synthetic-smoke.yaml
