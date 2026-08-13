#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cache_dir="${AEL_ZEPHYR_CACHE_DIR:-${workspace}/.ael/zephyr-cache}"
mkdir -p "${cache_dir}"
west build -p always -b stm32f4_disco "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-stm32-faulty" -- \
  -DUSER_CACHE_DIR="${cache_dir}" \
  -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/faulty.conf"
west build -p always -b stm32f4_disco "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-stm32-fixed" -- \
  -DUSER_CACHE_DIR="${cache_dir}" \
  -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/fixed.conf"
west build -p always -b hifive1_revb "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-hifive1" -- \
  -DUSER_CACHE_DIR="${cache_dir}" \
  -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/fixed.conf"
