#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
release_dir="${1:-$root_dir/release}"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

if ! compgen -G "$release_dir/*.tar.xz" > /dev/null && \
   ! compgen -G "$release_dir/*.zip" > /dev/null; then
  echo "no release archives found in $release_dir" >&2
  exit 1
fi

is_runtime_archive() {
  local name="$1"
  case "$name" in
    *-linux-*.tar.xz|*-darwin-*.tar.xz|*-windows-*.zip)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

check_common_layout() {
  local extract_dir="$1"
  local binary_path="$2"
  local tool_path="$3"

  test -e "$binary_path"
  test -f "$extract_dir/sdk/intuitionos/system/SYS/IOSSYS/C/Version"
  test -f "$extract_dir/sdk/intuitionos/system/SYS/IOSSYS/LIBS/dos.library"
  test -d "$extract_dir/AROS/C"
  test -d "$extract_dir/AROS/Libs"
  test -f "$extract_dir/AROS/S/Startup-Sequence"
  test -f "$extract_dir/README.md"
  cmp -s "$extract_dir/README.md" "$root_dir/README.md"
  test -e "$tool_path"
}

checked=0
for archive in "$release_dir"/*.tar.xz "$release_dir"/*.zip; do
  [ -e "$archive" ] || continue
  name="$(basename "$archive")"
  if ! is_runtime_archive "$name"; then
    echo "skipping non-runtime archive: $name"
    continue
  fi

  checked=$((checked + 1))
  extract_root="$tmpdir/${name%.*}"
  mkdir -p "$extract_root"

  case "$archive" in
    *.tar.xz)
      tar -C "$extract_root" -xf "$archive"
      ;;
    *.zip)
      unzip -q "$archive" -d "$extract_root"
      ;;
  esac

  top_level="$(find "$extract_root" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  if [[ -z "$top_level" ]]; then
    echo "archive $name did not extract a top-level directory" >&2
    exit 1
  fi

  case "$name" in
    *windows-*.zip)
      check_common_layout "$top_level" "$top_level/IntuitionEngine.exe" "$top_level/sdk/bin/ie64asm.exe"
      ;;
    *)
      check_common_layout "$top_level" "$top_level/IntuitionEngine" "$top_level/sdk/bin/ie64asm"
      ;;
  esac
done

if [[ "$checked" -eq 0 ]]; then
  echo "no runtime release archives found in $release_dir" >&2
  exit 1
fi

echo "distribution layout checks passed for archives in $release_dir"
