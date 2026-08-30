#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODE="${1:---development}"
APP="$ROOT/build/bin/Aether Desktop.app"
CONTENTS="$APP/Contents"
MACOS="$CONTENTS/MacOS"
RESOURCES="$CONTENTS/Resources"
FRAMEWORKS="$CONTENTS/Frameworks"
LAUNCH_AGENTS="$CONTENTS/Library/LaunchAgents"
DIST="$ROOT/build/dist"
DEFAULT_CHROMIUM="$ROOT/.ael/desktop-deps/chromium/chrome-mac/Chromium.app"
DEFAULT_SPARKLE="$ROOT/.ael/desktop-deps/sparkle/Sparkle.framework"

if [[ "$MODE" != "--development" && "$MODE" != "--release" ]]; then
  echo "用法: $0 [--development|--release]" >&2
  exit 2
fi
if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != "arm64" ]]; then
  echo "Aether Desktop 1.0 仅支持 Apple Silicon macOS。" >&2
  exit 1
fi
if [[ "$(sw_vers -productVersion | cut -d. -f1)" -lt 14 ]]; then
  echo "Aether Desktop 需要 macOS 14 或更高版本。" >&2
  exit 1
fi

mkdir -p "$DIST"
npm --prefix "$ROOT/frontend" ci
npm --prefix "$ROOT/frontend" run build

rm -rf "$APP"
mkdir -p "$MACOS" "$RESOURCES" "$FRAMEWORKS" "$LAUNCH_AGENTS"
cp "$ROOT/packaging/macos/Info.plist" "$CONTENTS/Info.plist"
printf 'APPL????' > "$CONTENTS/PkgInfo"
pushd "$ROOT" >/dev/null
CGO_LDFLAGS='-framework UniformTypeIdentifiers' GOPROXY=off GOCACHE="${GOCACHE:-/private/tmp/aether-go-build-cache}" \
  go build -buildvcs=false -trimpath -tags desktop,wv2runtime.download,production -ldflags '-w -s' -o "$MACOS/Aether Desktop" ./cmd/aether-desktop
GOCACHE="${GOCACHE:-/private/tmp/aether-go-build-cache}" go build -trimpath -o "$MACOS/aetherd" ./cmd/aetherd
GOCACHE="${GOCACHE:-/private/tmp/aether-go-build-cache}" go build -trimpath -o "$MACOS/ael-backend" ./cmd/ael-backend
GOCACHE="${GOCACHE:-/private/tmp/aether-go-build-cache}" go build -trimpath -o "$MACOS/aether-chrome-host" ./cmd/aether-chrome-host
popd >/dev/null

cp "$ROOT/packaging/macos/dev.aether.desktop.daemon.plist" "$LAUNCH_AGENTS/"
cp "$ROOT/packaging/macos/dev.aether.desktop.json.in" "$RESOURCES/"
cp -R "$ROOT/browser/chrome-extension" "$RESOURCES/ChromeExtension"

chromium_source="${AETHER_CHROMIUM_APP:-$DEFAULT_CHROMIUM}"
sparkle_source="${AETHER_SPARKLE_FRAMEWORK:-$DEFAULT_SPARKLE}"
if [[ -d "$chromium_source" ]]; then
  ditto "$chromium_source" "$RESOURCES/Chromium.app"
elif [[ "$MODE" == "--release" ]]; then
  echo "发布包必须通过 AETHER_CHROMIUM_APP 提供固定版本 Chromium.app。" >&2
  exit 1
else
  echo "开发包未内置 Chromium；浏览器功能将明确阻止启动。" >&2
fi

if [[ -d "$sparkle_source" ]]; then
  ditto "$sparkle_source" "$FRAMEWORKS/Sparkle.framework"
elif [[ "$MODE" == "--release" ]]; then
  echo "发布包必须通过 AETHER_SPARKLE_FRAMEWORK 提供固定版本 Sparkle.framework。" >&2
  exit 1
fi

if [[ "$MODE" == "--development" ]]; then
  if [[ -d "$RESOURCES/Chromium.app" ]]; then
    codesign --force --deep --sign - "$RESOURCES/Chromium.app"
  fi
  codesign --force --sign - --entitlements "$ROOT/packaging/macos/entitlements.plist" "$APP"
  codesign --verify --deep --strict "$APP"
  echo "开发版应用：$APP"
  exit 0
fi

if ! xcodebuild -version >/dev/null 2>&1; then
  echo "发布打包需要完整 Xcode，当前只有 Command Line Tools。" >&2
  exit 1
fi
: "${AETHER_SIGN_IDENTITY:?发布包必须设置 AETHER_SIGN_IDENTITY}"
: "${AETHER_NOTARY_PROFILE:?发布包必须设置 AETHER_NOTARY_PROFILE}"
: "${AETHER_SPARKLE_FEED_URL:?发布包必须设置 AETHER_SPARKLE_FEED_URL}"
: "${AETHER_SPARKLE_PUBLIC_KEY:?发布包必须设置 AETHER_SPARKLE_PUBLIC_KEY}"

/usr/libexec/PlistBuddy -c "Add :SUFeedURL string $AETHER_SPARKLE_FEED_URL" "$CONTENTS/Info.plist"
/usr/libexec/PlistBuddy -c "Add :SUPublicEDKey string $AETHER_SPARKLE_PUBLIC_KEY" "$CONTENTS/Info.plist"

for binary in "$MACOS/aetherd" "$MACOS/ael-backend" "$MACOS/aether-chrome-host"; do
  codesign --force --timestamp --options runtime --sign "$AETHER_SIGN_IDENTITY" "$binary"
done
if [[ -d "$RESOURCES/Chromium.app" ]]; then
  codesign --force --deep --timestamp --options runtime --sign "$AETHER_SIGN_IDENTITY" "$RESOURCES/Chromium.app"
  codesign --verify --deep --strict "$RESOURCES/Chromium.app"
fi
codesign --force --timestamp --options runtime --entitlements "$ROOT/packaging/macos/entitlements.plist" --sign "$AETHER_SIGN_IDENTITY" "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
spctl --assess --type execute --verbose=2 "$APP"

DMG="$DIST/Aether-Desktop-1.0.0-arm64.dmg"
rm -f "$DMG"
hdiutil create -volname "Aether Desktop" -srcfolder "$APP" -format UDZO -ov "$DMG"
xcrun notarytool submit "$DMG" --keychain-profile "$AETHER_NOTARY_PROFILE" --wait
xcrun stapler staple "$APP"
xcrun stapler staple "$DMG"
codesign --verify --deep --strict "$APP"
spctl --assess --type open --context context:primary-signature --verbose=2 "$DMG"
shasum -a 256 "$DMG"
echo "已签名并公证：$DMG"
