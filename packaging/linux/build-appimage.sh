#!/usr/bin/env bash
# Build a (thin) AppImage from the Wails Linux binary.
# Requires: appimagetool on PATH, binary linked against system WebKitGTK.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BINARY="${1:-$ROOT/build/bin/gotify-desktop}"
OUT_DIR="${2:-$ROOT/dist}"
ARCH_NAME="${3:-linux-amd64}"
VERSION="${APP_VERSION:-0.0.0}"

if [[ ! -x "$BINARY" && ! -f "$BINARY" ]]; then
  echo "binary not found: $BINARY" >&2
  exit 1
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

APPDIR="$WORKDIR/GotifyDesktop.AppDir"
mkdir -p \
  "$APPDIR/usr/bin" \
  "$APPDIR/usr/share/applications" \
  "$APPDIR/usr/share/icons/hicolor/256x256/apps"

install -m 755 "$BINARY" "$APPDIR/usr/bin/gotify-desktop"

# Desktop entry + AppImage Version key
sed "s/^X-AppImage-Version=.*/X-AppImage-Version=${VERSION}/" \
  "$ROOT/packaging/linux/gotify-desktop.desktop" \
  > "$APPDIR/usr/share/applications/gotify-desktop.desktop"
cp "$APPDIR/usr/share/applications/gotify-desktop.desktop" "$APPDIR/gotify-desktop.desktop"

ICON_SRC="$ROOT/build/appicon.png"
if [[ -f "$ICON_SRC" ]]; then
  cp "$ICON_SRC" "$APPDIR/usr/share/icons/hicolor/256x256/apps/gotify-desktop.png"
  cp "$ICON_SRC" "$APPDIR/gotify-desktop.png"
fi

cat > "$APPDIR/AppRun" <<'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
export PATH="$HERE/usr/bin:$PATH"
exec "$HERE/usr/bin/gotify-desktop" "$@"
EOF
chmod +x "$APPDIR/AppRun"

mkdir -p "$OUT_DIR"
OUT_FILE="$OUT_DIR/gotify-desktop-${ARCH_NAME}.AppImage"
export ARCH=x86_64
export VERSION
appimagetool "$APPDIR" "$OUT_FILE"
chmod +x "$OUT_FILE"
echo "wrote $OUT_FILE"
