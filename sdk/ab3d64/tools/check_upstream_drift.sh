#!/usr/bin/env bash
# Informational diff between the committed AB3D64 snapshot and a fresh
# rsync+strip of upstream. Output only; never gates anything.
#
# Usage: AB3D2_UPSTREAM=/path/to/alienbreed3d2/ab3d2_source \
#          tools/check_upstream_drift.sh
#
# Default upstream path: ../../../alienbreed3d2/ab3d2_source relative to
# sdk/ab3d64/. Missing upstream is a warning, not an error.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
upstream="${AB3D2_UPSTREAM:-$here/../../../alienbreed3d2/ab3d2_source}"

if [[ ! -d "$upstream" ]]; then
    echo "check_upstream_drift: upstream tree not found at $upstream" >&2
    echo "  set AB3D2_UPSTREAM to override; exiting cleanly." >&2
    exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

rsync -a --exclude=_build --exclude=obsoleted --exclude='ie/bin' \
      --exclude='bin' --exclude='*.o' \
      "$upstream"/ "$tmp/src/"
python3 "$here/tools/amiga_strip.py" "$tmp/src" >/dev/null

echo "Drift diff (committed src/  vs.  fresh upstream+strip):"
diff -rq "$here/src" "$tmp/src" || true
