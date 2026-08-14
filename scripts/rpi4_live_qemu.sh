#!/usr/bin/env bash
set -euo pipefail
image="${1:?Pi 4 image path is required}"
timeout_seconds="${RPI_QEMU_TIMEOUT_SECONDS:-300}"
[[ "$timeout_seconds" =~ ^[1-9][0-9]*$ ]] || { echo 'rpi4-live-qemu: RPI_QEMU_TIMEOUT_SECONDS must be a positive integer' >&2; exit 1; }
qemu="$(command -v qemu-system-aarch64 || true)"
[[ -n "$qemu" ]] || { echo 'rpi4-live-qemu: skipped, qemu-system-aarch64 is unavailable'; exit 0; }
"$qemu" -machine help | grep -q raspi4b || { echo 'rpi4-live-qemu: skipped, installed QEMU has no raspi4b machine'; exit 0; }
guestfish="$(command -v guestfish || true)"
[[ -n "$guestfish" ]] || { echo 'rpi4-live-qemu: guestfish is required to extract the Pi boot files' >&2; exit 1; }
qemu_img="$(command -v qemu-img || true)"
[[ -n "$qemu_img" ]] || { echo 'rpi4-live-qemu: qemu-img is required to create a disposable overlay' >&2; exit 1; }
[[ -f "$image" ]] || { echo "rpi4-live-qemu: missing image: $image" >&2; exit 1; }
log="${image}.qemu-serial.log"
stderr_log="${image}.qemu-stderr.log"
session_log="${image}.qemu-session.log"
: >"$log"
: >"$stderr_log"
rm -f "$session_log"
boot_dir="$(mktemp -d)"
trap 'rm -rf "$boot_dir"' EXIT
overlay="$boot_dir/pi-smoke.qcow2"
"$qemu_img" create -q -f qcow2 -F raw -b "$(realpath "$image")" "$overlay"
kernel="$boot_dir/kernel8_rt.img"
initramfs="$boot_dir/initramfs8_rt"
dtb="$boot_dir/bcm2711-rpi-4-b.dtb"
"$guestfish" --ro -a "$image" run : mount-ro /dev/sda1 / : download /kernel8_rt.img "$kernel"
"$guestfish" --ro -a "$image" run : mount-ro /dev/sda1 / : download /initramfs8_rt "$initramfs"
"$guestfish" --ro -a "$image" run : mount-ro /dev/sda1 / : download /bcm2711-rpi-4-b.dtb "$dtb"
cmdline="$("$guestfish" --ro -a "$image" run : mount-ro /dev/sda1 / : cat /cmdline.txt)"
qemu_cmdline="earlycon=pl011,mmio32,0xfe201000 keep_bootcon apparmor=1 security=apparmor lsm=landlock,lockdown,yama,integrity,apparmor,bpf $cmdline console=ttyAMA0,115200"
"$guestfish" --rw -a "$overlay" run : mount /dev/sda2 / : \
    rm-f /var/ie/state/ie-session.log : \
    rm-f /var/ie/state/intuition-engine.log : \
    rm-f /var/ie/state/xwayland.log
echo "rpi4-live-qemu: opening graphical Pi 4 console; validation runs for up to ${timeout_seconds} seconds"
set +e
timeout "${timeout_seconds}s" "$qemu" -M raspi4b -drive "file=$overlay,format=qcow2,if=sd" -kernel "$kernel" -initrd "$initramfs" -dtb "$dtb" -append "$qemu_cmdline" -display gtk -serial "file:$log" 2>"$stderr_log"
status=$?
set -e
if "$guestfish" --ro -a "$overlay" run : mount-ro /dev/sda2 / : is-file /var/ie/state/ie-session.log | grep -qx true; then
    "$guestfish" --ro -a "$overlay" run : mount-ro /dev/sda2 / : download /var/ie/state/ie-session.log "$session_log"
fi
if [[ -f "$session_log" ]] && rg -q 'Intuition Engine greetd session started' "$session_log"; then
    echo "rpi4-live-qemu: success, greetd started the IE appliance session; logs retained at $log and $session_log"
    exit 0
fi
if rg -q 'Reached target .*Multi-User|Started .*greetd|systemd\[1\]' "$log"; then
    echo "rpi4-live-qemu: userspace reached, but greetd did not start the IE session; serial log retained at $log" >&2
    exit 1
fi
if [[ "$status" -eq 124 ]]; then
    if [[ ! -s "$log" ]]; then
        echo "rpi4-live-qemu: timeout with no serial output; logs retained at $log and $stderr_log" >&2
    else
        echo "rpi4-live-qemu: timeout without a recognised userspace marker; logs retained at $log and $stderr_log" >&2
    fi
elif [[ ! -s "$log" ]]; then
    echo "rpi4-live-qemu: QEMU exited with status $status with no serial output; logs retained at $log and $stderr_log" >&2
else
    echo "rpi4-live-qemu: QEMU exited with status $status without a recognised userspace marker; logs retained at $log and $stderr_log" >&2
fi
exit 1
