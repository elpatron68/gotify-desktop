#!/usr/bin/env bash
# Create a UDZO DMG containing the Wails .app bundle.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
APP="${1:-$ROOT/build/bin/gotify-desktop.app}"
OUT_DIR="${2:-$ROOT/dist}"
ARCH_NAME="${3:-darwin-arm64}"
VOLNAME="${DMG_VOLNAME:-Gotify Desktop}"

if [[ ! -d "$APP" ]]; then
  echo "app bundle not found: $APP" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"
OUT_FILE="$OUT_DIR/gotify-desktop-${ARCH_NAME}.dmg"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

ditto "$APP" "$STAGE/Gotify Desktop.app"
ln -s /Applications "$STAGE/Applications"

hdiutil create \
  -volname "$VOLNAME" \
  -srcfolder "$STAGE" \
  -ov \
  -format UDZO \
  "$OUT_FILE"

echo "wrote $OUT_FILE"
