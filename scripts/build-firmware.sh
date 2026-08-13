#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
west build -p always -b stm32f4_disco "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-stm32-faulty" -- \
  -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/faulty.conf"
west build -p always -b stm32f4_disco "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-stm32-fixed" -- \
  -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/fixed.conf"
west build -p always -b hifive1_revb "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-hifive1" -- \
  -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/fixed.conf"
