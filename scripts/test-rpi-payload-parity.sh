#!/usr/bin/env bash
set -euo pipefail
x64_root="${1:?x64 payload directory is required}"
rpi_root="${2:?Raspberry Pi payload directory is required}"
for root in "$x64_root" "$rpi_root"; do
    [[ -d "$root" ]] || { echo "missing payload root: $root" >&2; exit 1; }
done
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
(cd "$x64_root" && find . -mindepth 1 -printf '%y %p\n' | sort) >"$tmp/x64-tree"
(cd "$rpi_root" && find . -mindepth 1 -printf '%y %p\n' | sort) >"$tmp/rpi-tree"
cmp "$tmp/x64-tree" "$tmp/rpi-tree" || { echo 'Raspberry Pi and x64 IESHARE payload directory trees differ' >&2; exit 1; }
(cd "$x64_root" && find . -type f -print0 | sort -z | xargs -0 sha256sum) >"$tmp/x64-files"
(cd "$rpi_root" && find . -type f -print0 | sort -z | xargs -0 sha256sum) >"$tmp/rpi-files"
cmp "$tmp/x64-files" "$tmp/rpi-files" || { echo 'Raspberry Pi and x64 IESHARE payload file contents differ' >&2; exit 1; }
