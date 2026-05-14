#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
BIN="$ROOT/bin/IntuitionEngine"
PROG="$ROOT/sdk/ab3d64/bin/ab3d2_ie64_redux_high_overdrive.ie64"
STUB_SRC="$ROOT/sdk/ab3d64/tools/generated/boot_stub.ie64.s"
STUB_BIN="$ROOT/sdk/ab3d64/tools/generated/boot_stub.ie64"
SCRIPT="$ROOT/sdk/ab3d64/tools/diag_boot_audit.ies"
SMOKE="$ROOT/sdk/ab3d64/tools/diag_headless_smoke.ies"
LOG="${1:-/tmp/ab3d64_audit.log}"
LAUNCH_CWD="${AB3D64_LAUNCH_CWD:-$ROOT/../alienbreed3d2/ab3d2_source}"

echo "[INFO] (re)building $BIN with the headless Go tag"
make -C "$ROOT" headless

echo "[INFO] building AB3D64 and diagnostic symbol module"
make -C "$ROOT" ab3d64
mkdir -p "$ROOT/sdk/ab3d64/tools/shots" \
         "$ROOT/sdk/ab3d64/tools/states" \
         "$ROOT/sdk/ab3d64/tools/memdumps"
cp "$PROG" "$ROOT/sdk/ab3d64/tools/generated/ab3d2_ie64_redux_high_overdrive.ie64"
printf 'org $1000\nhalt\n' > "$STUB_SRC"
"$ROOT/sdk/bin/ie64asm" -o "$STUB_BIN" "$STUB_SRC" >/dev/null

if [[ -d "$LAUNCH_CWD/media" ]]; then
    python3 "$ROOT/sdk/ab3d64/tools/unpack_sb_assets.py" \
        --source "$LAUNCH_CWD/media" \
        --out "$LAUNCH_CWD/_build/ie_unpacked/media"
fi

cd "$LAUNCH_CWD"

SMOKE_LOG="$(mktemp)"
trap 'rm -f "$SMOKE_LOG"' EXIT
if ! timeout 10 "$BIN" -ie64 -script "$SMOKE" "$STUB_BIN" > "$SMOKE_LOG" 2>&1; then
    cat "$SMOKE_LOG"
    echo "[FATAL] headless smoke pre-flight failed"
    exit 3
fi
if grep -q "^\[WARN\] headless screenshot capture inactive" "$SMOKE_LOG"; then
    echo "[INFO] screenshots unavailable; audit falls back to raw fb read"
fi
cat "$SMOKE_LOG"

rc=0
timeout 600 "$BIN" -ie64 -script "$SCRIPT" "$STUB_BIN" > "$LOG" 2>&1 || rc=$?
cat "$LOG"
exit "$rc"
