#!/usr/bin/env bash
# Inspect a completed Raspberry Pi appliance image without booting it. This is
# separate from the builder so release images are checked after all writes.
set -euo pipefail

fail() { echo "rpi-live-verify: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"; }

board=""
image=""
binary=""
share=false

usage() {
    cat >&2 <<'EOF'
Usage: verify_rpi_live_image.sh --board pi4|pi400|pi5 --image FILE --binary FILE [--share]
EOF
}

while (($#)); do
    case "$1" in
        --board|--image|--binary)
            (($# >= 2)) || fail "missing value for $1"
            case "$1" in
                --board) board="$2" ;;
                --image) image="$2" ;;
                --binary) binary="$2" ;;
            esac
            shift 2 ;;
        --share) share=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) fail "unknown argument: $1" ;;
    esac
done

case "$board" in pi4|pi400|pi5) ;; *) fail "board must be pi4, pi400 or pi5" ;; esac
[[ -f "$image" ]] || fail "missing image: $image"
[[ -f "$binary" ]] || fail "missing source binary: $binary"
need guestfish
need file

guest_file() {
    guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : is-file "$1" | grep -qx true
}

for path in \
    /opt/ie/IntuitionEngine \
    /opt/ie/ie-session.sh \
    /opt/ie/ie-launch.sh \
    /usr/libexec/intuitionengine-host-helper \
    /etc/greetd/config.toml \
    /etc/apparmor.d/opt.ie.IntuitionEngine \
    /etc/apparmor.d/usr.libexec.intuitionengine-host-helper \
    /etc/systemd/system/ie-apparmor.service \
    /etc/systemd/system/ie-host-helper.service \
    /etc/systemd/system/ie-firewall.service \
    /etc/systemd/system/ie-no-vt-switch.service; do
    guest_file "$path" || fail "image lacks required appliance file: $path"
done

for link in ie-host-helper.service ie-no-vt-switch.service; do
    guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : is-symlink "/etc/systemd/system/multi-user.target.wants/$link" | grep -qx true || \
        fail "image lacks enabled service link: $link"
done
guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : is-symlink /etc/systemd/system/network-pre.target.wants/ie-firewall.service | grep -qx true || \
    fail "image lacks pre-network firewall service link"
guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : is-symlink /etc/systemd/system/greetd.service.requires/ie-apparmor.service | grep -qx true || \
    fail "image lacks enforced AppArmor service link"
guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : is-symlink /etc/systemd/system/ie-host-helper.service.requires/ie-apparmor.service | grep -qx true || \
    fail "image lacks enforced AppArmor service link for host helper"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
greetd_config="$tmp/greetd-config.toml"
guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : download /etc/greetd/config.toml "$greetd_config"
grep -Fxq '[initial_session]' "$greetd_config" || fail 'greetd config lacks automatic initial appliance session'
grep -Fxq 'command = "/opt/ie/ie-session.sh"' "$greetd_config" || fail 'greetd initial session does not launch the appliance'
grep -Fxq 'command = "/usr/sbin/agreety --cmd /opt/ie/ie-session.sh"' "$greetd_config" || fail 'greetd fallback is not a protocol-speaking greeter'
installed="$tmp/IntuitionEngine"
guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : download /opt/ie/IntuitionEngine "$installed"
cmp -s "$binary" "$installed" || fail "installed binary differs from requested board binary"
file -b "$installed" | grep -Eqi 'ELF.*(aarch64|ARM aarch64)' || fail "installed binary is not ARM64"

if "$share"; then
    guestfish --ro -a "$image" run : list-partitions | grep -qx /dev/sda3 || fail "image lacks IESHARE partition"
    guest_file /etc/systemd/system/ie-grow-share.service || fail "image lacks IESHARE grow service"
    guestfish --ro -a "$image" run : mount-ro /dev/sda2 / : is-symlink /etc/systemd/system/multi-user.target.wants/ie-grow-share.service | grep -qx true || \
        fail "image lacks enabled IESHARE grow service"
fi

echo "Raspberry Pi ${board} live image verification passed"
