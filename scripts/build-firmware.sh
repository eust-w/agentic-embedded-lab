#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cache_dir="${AEL_ZEPHYR_CACHE_DIR:-${workspace}/.ael/zephyr-cache}"
mkdir -p "${cache_dir}"
for mechanism in {04..17} 19 21 23 24; do
  for variant in faulty fixed; do
    west build -p always -b stm32f4_disco "${workspace}/firmware/zephyr" \
      -d "${workspace}/firmware/zephyr/build-case${mechanism}-${variant}" -- \
      -DUSER_CACHE_DIR="${cache_dir}" \
      -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/conf/case${mechanism}-${variant}.conf"
  done
done
west build -p always -b hifive1_revb "${workspace}/firmware/zephyr" \
  -d "${workspace}/firmware/zephyr/build-hifive1" -- \
  -DUSER_CACHE_DIR="${cache_dir}" \
  -DEXTRA_CONF_FILE="${workspace}/firmware/zephyr/conf/case04-fixed.conf"
