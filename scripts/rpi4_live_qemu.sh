#!/usr/bin/env bash
set -euo pipefail
image="${1:?Pi 4 image path is required}"
qemu="$(command -v qemu-system-aarch64 || true)"
[[ -n "$qemu" ]] || { echo 'rpi4-live-qemu: skipped, qemu-system-aarch64 is unavailable'; exit 0; }
"$qemu" -machine help | grep -q raspi4b || { echo 'rpi4-live-qemu: skipped, installed QEMU has no raspi4b machine'; exit 0; }
[[ -f "$image" ]] || { echo "rpi4-live-qemu: missing image: $image" >&2; exit 1; }
log="${image}.qemu-serial.log"
set +e
timeout 120s "$qemu" -M raspi4b -drive "file=$image,format=raw,if=sd" -nographic -serial "file:$log"
status=$?
set -e
if rg -q 'Intuition Engine greetd session started' "$log"; then
    echo "rpi4-live-qemu: success, greetd started the IE appliance session; serial log retained at $log"
    exit 0
fi
if rg -q 'Reached target .*Multi-User|Started .*greetd|systemd\[1\]' "$log"; then
    echo "rpi4-live-qemu: userspace reached, but greetd did not start the IE session; serial log retained at $log" >&2
    exit 1
fi
if [[ "$status" -eq 124 ]]; then
    echo "rpi4-live-qemu: timeout before userspace; serial log retained at $log" >&2
else
    echo "rpi4-live-qemu: QEMU exited with status $status before userspace; serial log retained at $log" >&2
fi
exit 1
