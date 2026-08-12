#!/usr/bin/env bash
# Build a local pkg-config overlay for the Pi runtime root. The supplied
# IntuitionSubtractor sysroot intentionally contains target runtime files, not
# the compiler's libc linker scripts, so it must not replace the toolchain
# sysroot during cross-linking.
set -euo pipefail

fail() { echo "prepare-rpi-cross-overlay: $*" >&2; exit 1; }

runtime_root="${1:?Pi runtime root is required}"
toolchain_root="${2:?toolchain sysroot is required}"
overlay="${3:?overlay directory is required}"
[[ -d "$runtime_root" ]] || fail "missing Pi runtime root: $runtime_root"
[[ -d "$toolchain_root" ]] || fail "missing toolchain sysroot: $toolchain_root"
runtime_root="$(cd "$runtime_root" && pwd)"
toolchain_root="$(cd "$toolchain_root" && pwd)"
overlay_parent="$(dirname "$overlay")"
mkdir -p "$overlay_parent"
overlay="$(cd "$overlay_parent" && pwd)/$(basename "$overlay")"
[[ -e "$toolchain_root/usr/lib64/libc.so" ]] || fail "toolchain sysroot lacks libc linker script"
[[ -f "$runtime_root/usr/include/jack/jack.h" ]] || fail "Pi runtime root lacks JACK headers"

jack_library="$(find "$runtime_root/usr/lib64/pipewire-0.3/jack" -maxdepth 1 -type f -name 'libjack.so.*' -print -quit)"
[[ -n "$jack_library" ]] || fail "Pi runtime root lacks PipeWire JACK library"
pipewire_library="$(find "$runtime_root/usr/lib64" -maxdepth 1 -type f -name 'libpipewire-0.3.so.*' -print -quit)"
[[ -n "$pipewire_library" ]] || fail "Pi runtime root lacks PipeWire runtime library"


rm -rf "$overlay"
mkdir -p "$overlay/lib" "$overlay/pkgconfig"
ln -s "$jack_library" "$overlay/lib/libjack.so"
cat >"$overlay/pkgconfig/jack.pc" <<EOF
Name: jack
Description: Intuition Engine Pi PipeWire JACK ABI
Version: 3.1408.0
Libs: -L$overlay/lib -Wl,-rpath-link,$runtime_root/usr/lib64 -ljack
Cflags: -I$runtime_root/usr/include -D_REENTRANT
EOF

echo "Prepared Pi cross overlay: $overlay"
