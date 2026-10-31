#!/usr/bin/env bash
# Patch productVersion in wails.json (run from repo root).
# APP_VERSION may be "0.1.9", "v0.1.9", or a CI dev string.
# NSIS VIProductVersion requires numeric X.Y.Z — non-release builds use 0.0.0.
set -euo pipefail

RAW="${APP_VERSION:-${1:-}}"
if [[ -z "$RAW" ]]; then
  echo "usage: APP_VERSION=0.1.9 $0" >&2
  exit 1
fi
VERSION="${RAW#v}"

# Strip pre-release / metadata so NSIS and Windows version resources stay valid.
NUMERIC="$(printf '%s' "$VERSION" | sed -E 's/[^0-9].*$//')"
if [[ ! "$NUMERIC" =~ ^[0-9]+(\.[0-9]+){0,3}$ ]]; then
  NUMERIC="0.0.0"
fi
# Ensure at least major.minor.patch
IFS=. read -r A B C _ <<< "${NUMERIC}..."
A="${A:-0}"; B="${B:-0}"; C="${C:-0}"
NUMERIC="${A}.${B}.${C}"

WAILS_JSON="${WAILS_JSON:-wails.json}"
if [[ ! -f "$WAILS_JSON" ]]; then
  echo "wails.json not found at: $WAILS_JSON (cwd=$(pwd))" >&2
  exit 1
fi

export NUMERIC VERSION WAILS_JSON
python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["WAILS_JSON"])
data = json.loads(path.read_text(encoding="utf-8"))
info = data.setdefault("info", {})
info["productVersion"] = os.environ["NUMERIC"]
path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
print(f"wails.json productVersion={os.environ['NUMERIC']} (from {os.environ['VERSION']})")
PY
