#!/bin/sh
set -eu

if ! command -v python3.12 >/dev/null 2>&1; then
  echo "Python 3.12 is required. Install it with your package manager." >&2
  exit 1
fi

python3.12 -m venv .venv
.venv/bin/python -m pip install --upgrade pip
.venv/bin/python -m pip install -e '.[dev,mcp,server]'
.venv/bin/ael doctor

echo "Foundation environment ready. Missing simulator adapters remain explicit production gates."
