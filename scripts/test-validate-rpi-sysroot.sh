#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
validator="${root_dir}/scripts/validate_rpi_sysroot.sh"
overlay_builder="${root_dir}/scripts/prepare_rpi_cross_overlay.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
sysroot="$tmp/sysroot"
toolchain_root="$tmp/toolchain"
overlay="$tmp/overlay"
[[ -x "$overlay_builder" ]] || { echo 'missing Pi cross-overlay builder' >&2; exit 1; }
mkdir -p "$sysroot/lib" "$sysroot/usr/include/jack" "$sysroot/usr/lib64/pipewire-0.3/jack" "$sysroot/usr/lib64/pkgconfig" "$toolchain_root/usr/lib64" "$tmp/bin"
touch "$sysroot/lib/ld-linux-aarch64.so.1" "$sysroot/usr/include/jack/jack.h" "$toolchain_root/usr/lib64/libc.so"
cc -shared -Wl,-soname,libjack.so.0 -o "$sysroot/usr/lib64/pipewire-0.3/jack/libjack.so.0.3.1408" /dev/null
touch "$sysroot/usr/lib64/libpipewire-0.3.so.0.1408.0"
printf 'Name: jack\n' >"$sysroot/usr/lib64/pkgconfig/jack.pc"
"$overlay_builder" "$sysroot" "$toolchain_root" "$overlay"
readelf -dW "$overlay/lib/libjack.so" | grep -Fq 'Library soname: [libjack.so.0]' || {
    echo 'Pi cross overlay does not preserve the JACK libjack.so.0 ABI' >&2
    exit 1
}
touch "$tmp/golden.img"

cat >"$tmp/bin/fake-cc" <<'EOF'
#!/usr/bin/env bash
while (($#)); do
  if [[ "$1" == -o ]]; then touch "$2"; exit 0; fi
  shift
done
exit 1
EOF
cat >"$tmp/bin/file" <<'EOF'
#!/usr/bin/env bash
echo 'ELF 64-bit LSB executable, ARM aarch64'
EOF
cat >"$tmp/bin/pkg-config" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$tmp/bin/guestfish" <<'EOF'
#!/usr/bin/env bash
echo true
EOF
chmod +x "$tmp/bin"/*

rg -q 'is-symlink "\$1"' "$validator" || { echo 'validator does not accept Debian loader symlinks' >&2; exit 1; }

run() { PATH="$tmp/bin:$PATH" CROSS_SYSROOT="$sysroot" CROSS_TOOLCHAIN_SYSROOT="$toolchain_root" CROSS_CC=fake-cc CROSS_PKG_CONFIG_LIBDIR="$overlay/pkgconfig" CROSS_PKG_CONFIG_OVERLAY_DIR="$overlay" CROSS_PKG_CONFIG_SYSROOT_DIR="" "$validator" "$tmp/golden.img"; }
run

if PATH="$tmp/bin:$PATH" CROSS_SYSROOT="$sysroot" CROSS_TOOLCHAIN_SYSROOT="$toolchain_root" CROSS_CC=fake-cc CROSS_PKG_CONFIG_LIBDIR=/usr/lib/pkgconfig CROSS_PKG_CONFIG_OVERLAY_DIR="$overlay" CROSS_PKG_CONFIG_SYSROOT_DIR="" "$validator" "$tmp/golden.img" >/dev/null 2>&1; then
    echo 'validator accepted unexpected pkg-config path' >&2; exit 1
fi
rm "$sysroot/usr/include/jack/jack.h"
if run >/dev/null 2>&1; then
    echo 'validator accepted missing JACK header' >&2; exit 1
fi
echo 'Raspberry Pi sysroot contracts passed'
