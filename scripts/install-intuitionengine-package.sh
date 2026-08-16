#!/usr/bin/env bash
set -euo pipefail
fail(){ echo "install-intuitionengine-package: $*" >&2; exit 1; }
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
package= root= target= keyring= app_version= base_status=
while (($#)); do
  case "$1" in
    --package|--root|--target|--keyring|--app-version|--base-status) (($#>=2))||fail "missing value for $1"; case "$1" in --package) package=$2;;--root) root=$2;;--target) target=$2;;--keyring) keyring=$2;;--app-version) app_version=$2;;--base-status) base_status=$2;;esac; shift 2;;
    *) fail "unknown argument: $1";;
  esac
done
[[ -f "$package" && -d "$root" && -n "$target" && -f "$keyring" ]] || fail "package, root, target and keyring are required"
package_name=$(dpkg-deb -f "$package" Package); [[ "$package_name" == "$target" ]] || fail "package $package_name does not match $target"
package_version=$(dpkg-deb -f "$package" Version)
package_arch=$(dpkg-deb -f "$package" Architecture)
case "$target" in
    intuitionengine-amd64-v3) expected_arch=amd64 ;;
    intuitionengine-arm64-pi4|intuitionengine-arm64-pi5) expected_arch=arm64 ;;
    *) fail "unsupported update target: $target" ;;
esac
[[ "$package_arch" == "$expected_arch" ]] || fail "package $package_name has architecture $package_arch, expected $expected_arch"
if [[ -n "$app_version" && "$package_version" != "$app_version-1" ]]; then
    fail "package $package_name has version $package_version, expected $app_version-1"
fi
mkdir -p "$root/etc/ie" "$root/etc/apt/sources.list.d" "$root/usr/share/keyrings"
printf '%s\n' "$target" >"$root/etc/ie/update-target"
printf '%s\n' 'deb [signed-by=/usr/share/keyrings/intuitionengine-archive-keyring.gpg] https://intuitionengine.io stable main' >"$root/etc/apt/sources.list.d/intuitionengine.list"
install -m0644 "$keyring" "$root/usr/share/keyrings/intuitionengine-archive-keyring.gpg"
dpkg-deb --extract "$package" "$root"
checksum_manifest="$root/usr/share/intuitionengine/IntuitionEngine.sha256"
staged_binary="$root/opt/ie/IntuitionEngine"
expected_checksum="$(awk '
    NF { records++ }
    NF == 2 && $2 == "/opt/ie/IntuitionEngine" { matches++; checksum = $1 }
    END {
        if (records != 1 || matches != 1) exit 1
        print checksum
    }
' "$checksum_manifest")" || fail "invalid IntuitionEngine checksum manifest"
[[ "$expected_checksum" =~ ^[[:xdigit:]]{64}$ ]] || fail "invalid IntuitionEngine checksum value"
actual_checksum="$(sha256sum "$staged_binary" | awk '{print $1}')"
[[ "$actual_checksum" == "$expected_checksum" ]] || fail "staged IntuitionEngine checksum mismatch"
cp -p "$root/opt/ie/IntuitionEngine" "$root/opt/ie/IntuitionEngine.previous"

# The image is populated offline and cannot execute ARM64 maintainer scripts
# on an x64 host.  Seed the same installed-package records that dpkg would
# create after a successful initial configure, then retain the scripts for
# the first real upgrade performed by APT inside the image.
status_file="$root/var/lib/dpkg/status"
info_dir="$root/var/lib/dpkg/info"
mkdir -p "$root/var/lib/dpkg" "$info_dir"
if [[ -n "$base_status" ]]; then
    [[ -f "$base_status" ]] || fail "missing base dpkg status: $base_status"
    cp "$base_status" "$status_file"
fi
if [[ -f "$status_file" ]]; then
    filtered_status="$status_file.filtered"
    "$script_dir/merge-intuitionengine-dpkg-status.sh" --input "$status_file" --output "$filtered_status"
    mv "$filtered_status" "$status_file"
fi
description=$(dpkg-deb -f "$package" Description | sed -n '1p')
{
    printf 'Package: %s\n' "$package_name"
    printf 'Status: install ok installed\nVersion: %s\nArchitecture: %s\n' "$package_version" "$package_arch"
    printf 'Maintainer: IntuitionEngine release team <packages@intuitionengine.io>\nDescription: %s\n\n' "$description"
} >>"$status_file"
dpkg-deb -c "$package" | awk '{print $6}' | sed -e 's#^\./##' -e 's#/$##' -e 's#^#/#' >"$info_dir/$package_name.list"
control_root="$root/.intuitionengine-control"
mkdir -p "$control_root"
dpkg-deb --control "$package" "$control_root" >/dev/null
for maintainer_script in preinst postinst; do
    if [[ -f "$control_root/$maintainer_script" ]]; then
        install -m0755 "$control_root/$maintainer_script" "$info_dir/$package_name.$maintainer_script"
    fi
done
rm -rf "$control_root"
