#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
validator="$root_dir/scripts/validate_rpi_binary.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/sysroot/lib" "$tmp/sysroot/usr/lib/aarch64-linux-gnu" "$tmp/sysroot/usr/lib64/pipewire-0.3/jack"
touch "$tmp/golden.img" "$tmp/binary" "$tmp/sysroot/lib/ld-linux-aarch64.so.1" "$tmp/sysroot/usr/lib/aarch64-linux-gnu/libc.so.6" "$tmp/sysroot/usr/lib64/pipewire-0.3/jack/libjack.so.0.3.1408"

cat >"$tmp/bin/file" <<'EOF'
#!/usr/bin/env bash
echo 'ELF 64-bit LSB executable, ARM aarch64'
EOF
cat >"$tmp/bin/readelf" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == -lW ]]; then
    echo 'Requesting program interpreter: /lib/ld-linux-aarch64.so.1]'
elif [[ "${!#}" == *libjack.so.0.3.1408 ]]; then
    echo 'Library soname: [libjack.so.0]'
else
    echo 'Shared library: [libjack.so.0]'
    echo 'Shared library: [libc.so.6]'
fi
EOF
cat >"$tmp/bin/guestfish" <<'EOF'
#!/usr/bin/env bash
if [[ "$*" == *'is-file '* || "$*" == *'is-symlink '* ]]; then
    echo true
else
    printf '/lib/ld-linux-aarch64.so.1\n/usr/lib/aarch64-linux-gnu/libjack.so.0\n/usr/lib/aarch64-linux-gnu/libc.so.6\n'
fi
EOF
chmod +x "$tmp/bin"/*

PATH="$tmp/bin:$PATH" "$validator" "$tmp/binary" "$tmp/sysroot" "$tmp/golden.img"
rg -q 'is-symlink "\$1"' "$validator" || { echo 'validator does not accept Debian loader symlinks' >&2; exit 1; }
rm "$tmp/sysroot/usr/lib64/pipewire-0.3/jack/libjack.so.0.3.1408"
if PATH="$tmp/bin:$PATH" "$validator" "$tmp/binary" "$tmp/sysroot" "$tmp/golden.img" >/dev/null 2>&1; then
    echo 'validator accepted missing sysroot libjack' >&2; exit 1
fi
echo 'Raspberry Pi ARM64 binary contracts passed'
