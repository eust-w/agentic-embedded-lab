#!/bin/bash
set -euo pipefail

APP="${1:-/Applications/Aether Desktop.app}"
HOST="$APP/Contents/MacOS/aether-chrome-host"
TEMPLATE="$APP/Contents/Resources/dev.aether.desktop.json.in"
DESTINATION="$HOME/Library/Application Support/Google/Chrome/NativeMessagingHosts/dev.aether.desktop.json"

test -x "$HOST"
test -f "$TEMPLATE"
mkdir -p "$(dirname "$DESTINATION")"
escaped_host="${HOST//\\/\\\\}"
escaped_host="${escaped_host//&/\\&}"
escaped_host="${escaped_host//|/\\|}"
sed "s|@AETHER_CHROME_HOST_PATH@|$escaped_host|g" "$TEMPLATE" > "$DESTINATION.tmp"
chmod 600 "$DESTINATION.tmp"
mv "$DESTINATION.tmp" "$DESTINATION"
echo "已安装 Chrome Native Messaging Host：$DESTINATION"
