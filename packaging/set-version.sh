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

# Keep leading major[.minor[.patch[.build]]], drop -dev / +meta suffixes for NSIS.
if [[ "$VERSION" =~ ^([0-9]+)(\.([0-9]+))?(\.([0-9]+))?(\.([0-9]+))? ]]; then
  A="${BASH_REMATCH[1]:-0}"
  B="${BASH_REMATCH[3]:-0}"
  C="${BASH_REMATCH[5]:-0}"
  NUMERIC="${A}.${B}.${C}"
else
  NUMERIC="0.0.0"
fi

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
