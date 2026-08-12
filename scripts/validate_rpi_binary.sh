#!/usr/bin/env bash
# Verify an ARM64 release binary against both its selected sysroot and the
# actual Raspberry Pi golden root filesystem. This is entirely read-only.
set -euo pipefail

fail() { echo "validate-rpi-binary: $*" >&2; exit 1; }
binary="${1:?ARM64 binary path is required}"
sysroot="${2:?sysroot path is required}"
golden="${3:?golden image path is required}"
[[ -f "$binary" ]] || fail "missing binary: $binary"
[[ -d "$sysroot" ]] || fail "missing sysroot: $sysroot"
[[ -f "$golden" ]] || fail "missing golden image: $golden"
command -v readelf >/dev/null 2>&1 || fail "missing readelf"
command -v guestfish >/dev/null 2>&1 || fail "missing guestfish"

file -b "$binary" | grep -Eqi 'ELF.*(aarch64|ARM aarch64)' || fail "binary is not ARM64 ELF"
interpreter="$(readelf -lW "$binary" | sed -n 's/.*Requesting program interpreter: \([^]]*\).*/\1/p')"
[[ -n "$interpreter" ]] || fail "binary has no ELF interpreter"
[[ -e "$sysroot$interpreter" ]] || fail "sysroot lacks ELF interpreter: $interpreter"

mapfile -t needed < <(readelf -dW "$binary" | sed -n 's/.*Shared library: \[\([^]]*\)\].*/\1/p')
(( ${#needed[@]} > 0 )) || fail "binary has no dynamic dependencies"
printf '%s\n' "${needed[@]}" | grep -qx 'libjack.so.0' || fail "binary does not depend on libjack.so.0"
sysroot_has_soname() {
    local library="$1" candidate
    if find "$sysroot" \( -type f -o -type l \) -name "$library" -print -quit | grep -q .; then
        return 0
    fi
    # The established cross sysroot deliberately provides PipeWire JACK under
    # a versioned filename. Its DT_SONAME, rather than its directory or file
    # name, is the ABI the release binary records and the Debian golden loads.
    while IFS= read -r candidate; do
        readelf -dW "$candidate" 2>/dev/null | grep -Fq "Library soname: [$library]" && return 0
    done < <(find "$sysroot" -type f -name "${library%.so.*}.so*" -print)
    return 1
}
for library in "${needed[@]}"; do
    sysroot_has_soname "$library" || fail "sysroot lacks dependency ABI: $library"
done

golden_has() {
    guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : is-file "$1" | grep -qx true || \
        guestfish --ro -a "$golden" run : mount-ro /dev/sda2 / : is-symlink "$1" | grep -qx true
}
golden_has "$interpreter" || fail "golden root filesystem lacks interpreter: $interpreter"
for library in "${needed[@]}"; do
    # Debian may install a versioned library behind the SONAME symlink.
    golden_has "/usr/lib/aarch64-linux-gnu/$library" || \
        golden_has "/lib/aarch64-linux-gnu/$library" || \
        fail "golden root filesystem lacks dependency: $library"
done
