#!/usr/bin/env bash
set -euo pipefail
fail(){ echo "build-intuitionengine-deb: $*" >&2; exit 1; }
target= version= binary= output_dir=
while (($#)); do
  case "$1" in
    --target|--app-version|--binary|--output-dir) (($#>=2))||fail "missing value for $1"; case "$1" in --target) target=$2;;--app-version) version=$2;;--binary) binary=$2;;--output-dir) output_dir=$2;;esac; shift 2;;
    *) fail "unknown argument: $1";;
  esac
done
case "$target" in
  intuitionengine-amd64-v3) arch=amd64;;
  intuitionengine-arm64-pi4|intuitionengine-arm64-pi5) arch=arm64;;
  *) fail "unsupported target: $target";;
esac
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "--app-version must be semantic x.y.z"
[[ -f "$binary" && -n "$output_dir" ]] || fail "binary and output directory are required"
command -v dpkg-deb >/dev/null || fail "dpkg-deb is required"
command -v file >/dev/null || fail "file is required"
command -v strings >/dev/null || fail "strings is required"
case "$arch" in
  amd64) file -b "$binary" | grep -Eqi 'ELF.*x86-64' || fail "binary is not an amd64 ELF" ;;
  arm64) file -b "$binary" | grep -Eqi 'ELF.*(aarch64|ARM aarch64)' || fail "binary is not an arm64 ELF" ;;
esac
strings -a "$binary" | grep -Fx -- "$version" >/dev/null || fail "binary does not contain exact APP_VERSION $version"
root=$(mktemp -d "${TMPDIR:-/tmp}/intuitionengine-deb.XXXXXX"); trap 'rm -rf "$root"' EXIT
mkdir -p "$root/DEBIAN" "$root/opt/ie" "$root/usr/lib/intuitionengine" "$root/usr/share/intuitionengine"
install -m0755 "$binary" "$root/opt/ie/IntuitionEngine"
sha256sum "$root/opt/ie/IntuitionEngine" | sed 's#  .*#  /opt/ie/IntuitionEngine#' >"$root/usr/share/intuitionengine/IntuitionEngine.sha256"
cat >"$root/DEBIAN/control" <<EOF
Package: $target
Version: ${version}-1
Section: misc
Priority: optional
Architecture: $arch
Maintainer: IntuitionEngine release team <packages@intuitionengine.io>
Description: IntuitionEngine live appliance binary for $target
 The target-specific IntuitionEngine executable and guarded restart hook.
EOF
cat >"$root/DEBIAN/preinst" <<EOF
#!/bin/sh
set -eu
[ -r /etc/ie/update-target ] && [ "\$(sed -n 's/[[:space:]]//gp' /etc/ie/update-target)" = "$target" ] || exit 1
if [ "\${2:-}" ]; then [ -f /opt/ie/IntuitionEngine ] || exit 1; cp -p /opt/ie/IntuitionEngine /opt/ie/IntuitionEngine.previous; fi
EOF
cat >"$root/DEBIAN/postinst" <<EOF
#!/bin/sh
set -eu
[ -r /etc/ie/update-target ] && [ "\$(sed -n 's/[[:space:]]//gp' /etc/ie/update-target)" = "$target" ] || exit 1
[ -x /opt/ie/IntuitionEngine ] || exit 1
exec /usr/lib/intuitionengine/package-check "\$@"
EOF
cat >"$root/usr/lib/intuitionengine/package-check" <<EOF
#!/bin/sh
set -eu
b=/opt/ie/IntuitionEngine; p=/opt/ie/IntuitionEngine.previous; c=/usr/share/intuitionengine/IntuitionEngine.sha256
[ -x "\$b" ] && [ -r "\$c" ] || exit 1
sha256sum -c "\$c" >/dev/null 2>&1 || { [ -f "\$p" ] || exit 1; cp -p "\$p" "\$b"; exit 1; }
"\$b" -version 2>/dev/null | grep -Fq "Intuition Engine ${version}" || { [ -f "\$p" ] || exit 1; cp -p "\$p" "\$b"; exit 1; }
[ "\${2:-}" ] || exit 0
systemctl restart greetd.service || { cp -p "\$p" "\$b"; systemctl restart greetd.service || true; exit 1; }
systemctl is-active --quiet greetd.service || { cp -p "\$p" "\$b"; systemctl restart greetd.service || true; exit 1; }
EOF
chmod 0755 "$root/DEBIAN/preinst" "$root/DEBIAN/postinst" "$root/usr/lib/intuitionengine/package-check"
mkdir -p "$output_dir"; output="$output_dir/${target}_${version}-1_${arch}.deb"; rm -f "$output"
dpkg-deb --build --root-owner-group "$root" "$output" >/dev/null
printf '%s\n' "$output"
