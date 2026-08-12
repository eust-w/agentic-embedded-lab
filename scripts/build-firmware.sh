#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
west build -p always -b stm32f4_disco "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-stm32"
west build -p always -b hifive1_revb "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-hifive1"
