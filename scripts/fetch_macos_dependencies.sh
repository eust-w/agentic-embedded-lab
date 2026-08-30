#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOWNLOADS="$ROOT/.ael/desktop-deps/downloads"
CHROMIUM_ROOT="$ROOT/.ael/desktop-deps/chromium"
SPARKLE_ROOT="$ROOT/.ael/desktop-deps/sparkle"
CHROMIUM_ARCHIVE="$DOWNLOADS/chromium-Mac_Arm-1688699.zip"
SPARKLE_ARCHIVE="$DOWNLOADS/Sparkle-2.9.4.tar.xz"

mkdir -p "$DOWNLOADS" "$CHROMIUM_ROOT" "$SPARKLE_ROOT"
curl -fL --retry 3 -o "$CHROMIUM_ARCHIVE" "https://commondatastorage.googleapis.com/chromium-browser-snapshots/Mac_Arm/1688699/chrome-mac.zip"
curl -fL --retry 3 -o "$SPARKLE_ARCHIVE" "https://github.com/sparkle-project/Sparkle/releases/download/2.9.4/Sparkle-2.9.4.tar.xz"
printf '%s  %s\n' 'fa980d55ea084890fb11400de4c3696ec35c1e426debd031ae0b980a7a0e7b03' "$CHROMIUM_ARCHIVE" | shasum -a 256 -c -
printf '%s  %s\n' 'ce89daf967db1e1893ed3ebd67575ed82d3902563e3191ca92aaec9164fbdef9' "$SPARKLE_ARCHIVE" | shasum -a 256 -c -
unzip -q -o "$CHROMIUM_ARCHIVE" -d "$CHROMIUM_ROOT"
tar -xf "$SPARKLE_ARCHIVE" -C "$SPARKLE_ROOT"
test -x "$CHROMIUM_ROOT/chrome-mac/Chromium.app/Contents/MacOS/Chromium"
test -d "$SPARKLE_ROOT/Sparkle.framework"
echo "已校验并解压 Chromium 1688699 与 Sparkle 2.9.4。"
