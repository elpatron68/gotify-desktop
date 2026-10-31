#!/usr/bin/env bash
# Patch product version in build/config.yml and wails.json (run from repo root).
# APP_VERSION may be "0.1.9", "v0.1.9", or a CI dev string.
# NSIS / packaging requires numeric X.Y.Z — non-release builds use 0.0.0.
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
CONFIG_YML="${CONFIG_YML:-build/config.yml}"
VERSION_GO="${VERSION_GO:-internal/version/version.go}"

export NUMERIC VERSION WAILS_JSON CONFIG_YML VERSION_GO
python3 - <<'PY'
import json
import os
import re
from pathlib import Path

numeric = os.environ["NUMERIC"]

wails_json = Path(os.environ["WAILS_JSON"])
if wails_json.is_file():
    data = json.loads(wails_json.read_text(encoding="utf-8"))
    info = data.setdefault("info", {})
    info["productVersion"] = numeric
    wails_json.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    print(f"{wails_json} productVersion={numeric}")

config_yml = Path(os.environ["CONFIG_YML"])
if config_yml.is_file():
    text = config_yml.read_text(encoding="utf-8")
    updated, n = re.subn(
        r'(?m)^(\s*version:\s*")[^"]*(")',
        rf'\g<1>{numeric}\g<2>',
        text,
        count=1,
    )
    if n != 1:
        raise SystemExit(f"could not patch version in {config_yml}")
    config_yml.write_text(updated, encoding="utf-8")
    print(f"{config_yml} version={numeric}")

version_go = Path(os.environ["VERSION_GO"])
if version_go.is_file():
    text = version_go.read_text(encoding="utf-8")
    updated, n = re.subn(
        r'(const Version = ")[^"]*(")',
        r'\g<1>' + numeric + r'\2',
        text,
        count=1,
    )
    if n != 1:
        raise SystemExit(f"could not patch Version in {version_go}")
    version_go.write_text(updated, encoding="utf-8")
    print(f"{version_go} Version={numeric}")
PY
