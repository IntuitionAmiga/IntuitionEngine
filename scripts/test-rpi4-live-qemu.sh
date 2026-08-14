#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root_dir/scripts/rpi4_live_qemu.sh"
fail() { echo "FAIL: $*" >&2; exit 1; }
[[ -x "$script" ]] || fail "missing executable Pi 4 QEMU smoke script"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
mkdir -p "$workdir/bin"
image="$workdir/pi.img"
printf 'release-image\n' >"$image"
source_sum="$(sha256sum "$image" | awk '{print $1}')"

cat >"$workdir/bin/qemu-img" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"${FAKE_QEMU_IMG_ARGS:?}"
: >"${@: -1}"
EOF
chmod +x "$workdir/bin/qemu-img"

cat >"$workdir/bin/timeout" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$1" >"${FAKE_TIMEOUT_ARGS:?}"
shift
exec "$@"
EOF
chmod +x "$workdir/bin/timeout"

cat >"$workdir/bin/qemu-system-aarch64" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-machine" && "${2:-}" == "help" ]]; then
    echo 'raspi4b Raspberry Pi 4'
    exit 0
fi
printf '%s\n' "$@" >"${FAKE_QEMU_ARGS:?}"
for arg in "$@"; do
    case "$arg" in
        file:*) log="${arg#file:}" ;;
    esac
done
case "${FAKE_QEMU_RESULT:?}" in
    success) touch "${FAKE_CURRENT_BOOT:?}"; echo 'systemd[1]: booting' >"$log"; exit 0 ;;
    stale) echo 'systemd[1]: booting' >"$log"; exit 0 ;;
    userspace) echo 'systemd[1]: booting' >"$log"; exit 0 ;;
    silent-timeout) exit 124 ;;
    failed) echo 'qemu: invalid image' >&2; exit 2 ;;
esac
EOF
chmod +x "$workdir/bin/qemu-system-aarch64"
cat >"$workdir/bin/guestfish" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == *'--rw'* && "$*" == *"-a ${FAKE_RELEASE_IMAGE:?}"* ]]; then
    printf 'mutated\n' >>"$FAKE_RELEASE_IMAGE"
fi
case "$*" in
    *'cat /cmdline.txt'*) echo 'console=serial0,115200 console=tty1 root=PARTUUID=ffca5ce2-02 rootfstype=ext4 rootwait' ;;
    *'download /kernel8_rt.img'*) : >"${@: -1}" ;;
    *'download /initramfs8_rt'*) : >"${@: -1}" ;;
    *'download /bcm2711-rpi-4-b.dtb'*) : >"${@: -1}" ;;
    *'rm-f /var/ie/state/ie-session.log'*|*'rm-f /var/ie/state/intuition-engine.log'*)
        touch "${FAKE_GUEST_LOGS_CLEARED:?}"
        ;;
    *'is-file /var/ie/state/ie-session.log'*)
        if [[ "${FAKE_QEMU_RESULT:?}" == success && -e "${FAKE_CURRENT_BOOT:?}" ]]; then
            echo true
        elif [[ "${FAKE_QEMU_RESULT:?}" == stale && ! -e "${FAKE_GUEST_LOGS_CLEARED:?}" ]]; then
            echo true
        else
            echo false
        fi
        ;;
    *'download /var/ie/state/ie-session.log'*)
        printf '%s\n' 'Intuition Engine greetd session started' >"${@: -1}"
        ;;
    *) echo "unexpected guestfish invocation: $*" >&2; exit 2 ;;
esac
EOF
chmod +x "$workdir/bin/guestfish"

run_case() {
    local result="$1"
    set +e
    rm -f "$workdir/current-boot" "$workdir/guest-logs-cleared"
    output="$(PATH="$workdir/bin:$PATH" FAKE_QEMU_RESULT="$result" FAKE_QEMU_ARGS="$workdir/qemu.args" FAKE_QEMU_IMG_ARGS="$workdir/qemu-img.args" FAKE_TIMEOUT_ARGS="$workdir/timeout.args" FAKE_CURRENT_BOOT="$workdir/current-boot" FAKE_GUEST_LOGS_CLEARED="$workdir/guest-logs-cleared" FAKE_RELEASE_IMAGE="$image" "$script" "$image" 2>&1)"
    status=$?
    set -e
}

run_case success
[[ "$status" -eq 0 ]] || fail "successful greetd boot returned $status: $output"
[[ "$output" == *'success, greetd started'* ]] || fail "successful boot was not recognised: $output"
[[ -f "${image}.qemu-session.log" ]] || fail "successful boot did not retain the appliance session log"
rg -q 'Intuition Engine greetd session started' "${image}.qemu-session.log" || fail "retained session log lacks the greetd marker"
[[ -e "$workdir/guest-logs-cleared" ]] || fail "QEMU smoke did not clear inherited guest success logs"
[[ "$(sha256sum "$image" | awk '{print $1}')" == "$source_sum" ]] || fail "QEMU smoke modified the release image"
rg -qx 'qcow2' "$workdir/qemu-img.args" || fail "QEMU smoke does not create a disposable qcow2 overlay"
rg -q "$(realpath "$image")" "$workdir/qemu-img.args" || fail "QEMU overlay does not use the release image as its backing file"
if rg -q "file=$image,format=raw,if=sd" "$workdir/qemu.args"; then
    fail "QEMU opens the release image directly for guest writes"
fi
rg -q 'format=qcow2,if=sd' "$workdir/qemu.args" || fail "QEMU does not boot from the disposable overlay"
rg -qx -- '-display' "$workdir/qemu.args" || fail "QEMU was not given an explicit display backend: $(tr '\n' ' ' <"$workdir/qemu.args")"
rg -qx 'gtk' "$workdir/qemu.args" || fail "QEMU did not select the GTK display backend: $(tr '\n' ' ' <"$workdir/qemu.args")"
if rg -qx -- '-nographic' "$workdir/qemu.args"; then
    fail "QEMU still disables its graphical window"
fi
for option in -kernel -initrd -dtb -append; do
    rg -qx -- "$option" "$workdir/qemu.args" || fail "QEMU direct boot is missing $option: $(tr '\n' ' ' <"$workdir/qemu.args")"
done
rg -q 'root=PARTUUID=ffca5ce2-02 rootfstype=ext4 rootwait' "$workdir/qemu.args" || fail "QEMU did not pass the image kernel command line"
rg -q 'earlycon=pl011,mmio32,0xfe201000' "$workdir/qemu.args" || fail "QEMU did not enable the emulated PL011 early console"
rg -q 'keep_bootcon' "$workdir/qemu.args" || fail "QEMU did not retain its working early console through userspace"
rg -q 'console=tty1.*console=ttyAMA0,115200' "$workdir/qemu.args" || fail "QEMU did not make PL011 the marker-visible system console"
rg -q 'apparmor=1 security=apparmor' "$workdir/qemu.args" || fail "QEMU did not activate the AppArmor security module"
rg -q 'lsm=[^ ]*apparmor' "$workdir/qemu.args" || fail "QEMU did not include AppArmor in the active LSM list"
[[ "$(<"$workdir/timeout.args")" == 300s ]] || fail "QEMU smoke timeout is not long enough for the measured Pi 4 TCG boot"
[[ "$output" == *'opening graphical Pi 4 console'* ]] || fail "QEMU launch gives no visible startup notice: $output"

run_case stale
[[ "$status" -eq 1 ]] || fail "stale guest success marker was accepted"
[[ "$output" == *'userspace reached'* ]] || fail "stale-marker boot was misdiagnosed: $output"

run_case userspace
[[ "$status" -eq 1 ]] || fail "userspace-only boot returned $status instead of 1"
[[ "$output" == *'userspace reached'* ]] || fail "userspace-only boot was misdiagnosed: $output"

rm -f "${image}.qemu-serial.log" "${image}.qemu-stderr.log"
run_case silent-timeout
[[ "$status" -eq 1 ]] || fail "silent timeout returned $status instead of 1"
[[ -f "${image}.qemu-serial.log" ]] || fail "silent timeout did not retain an empty serial log"
[[ "$output" == *'timeout with no serial output'* ]] || fail "silent timeout was misdiagnosed: $output"
[[ "$output" != *'No such file or directory'* ]] || fail "silent timeout searched a nonexistent log: $output"
[[ "$output" != *'before userspace'* ]] || fail "silent timeout made an unsupported userspace claim: $output"

run_case failed
[[ "$status" -eq 1 ]] || fail "QEMU startup failure returned $status instead of 1"
[[ -f "${image}.qemu-stderr.log" ]] || fail "QEMU startup failure did not retain stderr"
rg -q 'qemu: invalid image' "${image}.qemu-stderr.log" || fail "QEMU stderr was not captured"
[[ "$output" == *'QEMU exited with status 2 with no serial output'* ]] || fail "QEMU startup failure was misdiagnosed: $output"

echo 'Raspberry Pi 4 QEMU smoke contracts passed'
