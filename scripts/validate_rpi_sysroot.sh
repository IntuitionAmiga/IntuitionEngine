#!/usr/bin/env bash
# Read-only ARM64 sysroot compatibility gate for Raspberry Pi release builds.
set -euo pipefail

fail() { echo "validate-rpi-sysroot: $*" >&2; exit 1; }
sysroot="${CROSS_SYSROOT:?CROSS_SYSROOT is required}"
toolchain_sysroot="${CROSS_TOOLCHAIN_SYSROOT:?CROSS_TOOLCHAIN_SYSROOT is required}"
cc="${CROSS_CC:?CROSS_CC is required}"
golden="${1:?golden image path or --preflight is required}"
preflight=false
if [[ "$golden" == "--preflight" ]]; then
    preflight=true
fi
[[ -d "$sysroot" ]] || fail "missing sysroot: $sysroot"
[[ -d "$toolchain_sysroot" ]] || fail "missing toolchain sysroot: $toolchain_sysroot"
sysroot="$(cd "$sysroot" && pwd)"
toolchain_sysroot="$(cd "$toolchain_sysroot" && pwd)"
if ! "$preflight"; then
    [[ -f "$golden" ]] || fail "missing golden image: $golden"
fi
command -v "$cc" >/dev/null 2>&1 || fail "missing cross compiler: $cc"
for path in \
    lib/ld-linux-aarch64.so.1 \
    usr/include/jack/jack.h; do
    [[ -e "$sysroot/$path" ]] || fail "missing ARM64 sysroot input: $path"
done
find "$sysroot" -type f \( -name 'libjack.so' -o -name 'libjack.so.*' \) -print -quit | grep -q . || fail "missing libjack in sysroot"
find "$sysroot" -type f -name jack.pc -print -quit | grep -q . || fail "missing jack.pc in sysroot"

pkg_libdir="${CROSS_PKG_CONFIG_LIBDIR:-$sysroot/usr/lib/pkgconfig}"
pkg_sysroot="${CROSS_PKG_CONFIG_SYSROOT_DIR:-}"
pkg_overlay="${CROSS_PKG_CONFIG_OVERLAY_DIR:-}"
[[ -z "$pkg_sysroot" ]] || fail "Pi overlay pkg-config must not rewrite absolute target paths"
[[ -d "$pkg_overlay/pkgconfig" ]] || fail "missing Pi pkg-config overlay: $pkg_overlay/pkgconfig"
IFS=: read -r -a pkg_dirs <<<"$pkg_libdir"
for dir in "${pkg_dirs[@]}"; do
    [[ "$dir" == "$pkg_overlay"/* ]] || fail "unexpected pkg-config path: $dir"
done
grep -Fq "$sysroot/usr/include" "$pkg_overlay/pkgconfig/jack.pc" || fail "Pi overlay does not use Pi JACK headers"
grep -Fq "$sysroot/usr/lib64" "$pkg_overlay/pkgconfig/jack.pc" || fail "Pi overlay does not use Pi JACK runtime libraries"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cat >"$tmp/probe.c" <<'EOF'
#include <jack/jack.h>
int main(void) { return jack_client_name_size() == 0; }
EOF
PKG_CONFIG_LIBDIR="$pkg_libdir" \
PKG_CONFIG_SYSROOT_DIR="$pkg_sysroot" \
"$cc" --sysroot="$toolchain_sysroot" "$tmp/probe.c" -o "$tmp/probe" $(PKG_CONFIG_LIBDIR="$pkg_libdir" PKG_CONFIG_SYSROOT_DIR="$pkg_sysroot" pkg-config --cflags --libs jack)
file -b "$tmp/probe" | grep -Eqi 'ELF.*(aarch64|ARM aarch64)' || fail "JACK link probe is not ARM64"
if "$preflight"; then
    echo "Raspberry Pi sysroot preflight passed"
    exit 0
fi

# Golden-image inspection deliberately stays offline. guestfish is required to
# resolve the loader and libjack inside the actual appliance root filesystem.
command -v guestfish >/dev/null 2>&1 || fail "missing guestfish for golden root filesystem validation"
golden_has() {
    guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : is-file "$1" | grep -qx true || \
        guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : is-symlink "$1" | grep -qx true
}
golden_has /lib/ld-linux-aarch64.so.1 || fail "golden root filesystem lacks ARM64 loader"
golden_has /usr/lib/aarch64-linux-gnu/libjack.so.0 || fail "golden root filesystem lacks libjack"
