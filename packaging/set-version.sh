#!/usr/bin/env bash
# Patch productVersion in wails.json from APP_VERSION (e.g. 0.1.9 or v0.1.9).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RAW="${APP_VERSION:-${1:-}}"
if [[ -z "$RAW" ]]; then
  echo "usage: APP_VERSION=0.1.9 $0" >&2
  exit 1
fi
VERSION="${RAW#v}"

python3 - <<PY
import json
from pathlib import Path
path = Path("${ROOT}/wails.json")
data = json.loads(path.read_text())
info = data.setdefault("info", {})
info["productVersion"] = "${VERSION}"
path.write_text(json.dumps(data, indent=2) + "\n")
print(f"wails.json productVersion=${VERSION}")
PY
