#!/bin/bash
set -e

WORKSPACE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
APP_NAME="Aether Native"
BUILD_DIR="$WORKSPACE_DIR/build/mac"
APP_BUNDLE="$BUILD_DIR/$APP_NAME.app"
DEST_APP="/Applications/$APP_NAME.app"
USER_DEST_APP="$HOME/Applications/$APP_NAME.app"

echo "=================================================="
echo "🍏 Building Native macOS Application: $APP_NAME.app"
echo "=================================================="

rm -rf "$BUILD_DIR"
mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Resources"

# 1. Copy AppIcon if exists
if [ -f "$WORKSPACE_DIR/aether/ui/assets/AppIcon.icns" ]; then
    cp "$WORKSPACE_DIR/aether/ui/assets/AppIcon.icns" "$APP_BUNDLE/Contents/Resources/AppIcon.icns"
    echo "✓ AppIcon.icns packaged into Resources."
fi

# 2. Write Info.plist
cat << 'EOF' > "$APP_BUNDLE/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>Aether Native</string>
    <key>CFBundleIdentifier</key>
    <string>dev.aether.native</string>
    <key>CFBundleName</key>
    <string>Aether Native</string>
    <key>CFBundleDisplayName</key>
    <string>Aether Native</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>0.2.0</string>
    <key>CFBundleVersion</key>
    <string>2</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key>
    <true/>
    <key>NSAppTransportSecurity</key>
    <dict>
        <key>NSAllowsLocalNetworking</key>
        <true/>
        <key>NSAllowsArbitraryLoads</key>
        <true/>
    </dict>
</dict>
</plist>
EOF

# 3. Write Launcher executable
cat << EOF > "$APP_BUNDLE/Contents/MacOS/Aether Native"
#!/bin/bash
export PYTHONUNBUFFERED=1
export AEL_WORKSPACE="$WORKSPACE_DIR"
cd "$WORKSPACE_DIR"
exec "$WORKSPACE_DIR/.venv/bin/python" -m aether
EOF

chmod +x "$APP_BUNDLE/Contents/MacOS/Aether Native"

echo "✓ Application bundle assembled at: $APP_BUNDLE"

# 4. Install into Applications directory
echo "📦 Installing into macOS Applications..."
mkdir -p "$HOME/Applications"
rm -rf "$USER_DEST_APP"
cp -R "$APP_BUNDLE" "$USER_DEST_APP"
touch "$USER_DEST_APP"
echo "✓ Installed to: $USER_DEST_APP"

if [ -w "/Applications" ]; then
    rm -rf "$DEST_APP"
    cp -R "$APP_BUNDLE" "$DEST_APP"
    touch "$DEST_APP"
    echo "✓ Installed to system: $DEST_APP"
fi

# 5. Generate DMG Installer package
DMG_PATH="$WORKSPACE_DIR/build/mac/Aether-Native-Installer.dmg"
if command -v hdiutil &>/dev/null; then
    echo "💿 Creating macOS DMG Installer..."
    rm -f "$DMG_PATH"
    hdiutil create -volname "Aether Native" -srcfolder "$APP_BUNDLE" -ov -format UDZO "$DMG_PATH" > /dev/null
    echo "✓ DMG Installer created at: $DMG_PATH"
fi

echo "=================================================="
echo "🎉 macOS Installation Complete with Custom App Icon!"
echo "🚀 You can launch it from Spotlight, Launchpad, or:"
echo "   open \"$USER_DEST_APP\""
echo "=================================================="
