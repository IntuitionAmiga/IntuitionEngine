#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root_dir/scripts/rpi4_live_qemu.sh"
fail() { echo "FAIL: $*" >&2; exit 1; }
[[ -x "$script" ]] || fail "missing executable Pi 4 QEMU smoke script"
rg -q 'Intuition Engine greetd session started' "$script" || fail "QEMU smoke script does not recognise greetd session success"
if rg -q 'Started .*Intuition Engine|intuition-engine.*service.*started' "$script"; then
    fail "QEMU smoke script still expects a nonexistent IE systemd service"
fi
echo 'Raspberry Pi 4 QEMU smoke contracts passed'
