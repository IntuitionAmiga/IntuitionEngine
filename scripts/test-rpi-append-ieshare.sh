#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root_dir/scripts/rpi_append_ieshare.sh"
fail() { echo "FAIL: $*" >&2; exit 1; }
[[ -x "$script" ]] || fail "missing executable IESHARE append script"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/tools" "$tmp/payload"
touch "$tmp/image.img"

cat >"$tmp/tools/sfdisk" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == --version ]]; then exit 0; fi
if [[ "$1" == --dump ]]; then
  state="${SFDISK_STATE_FILE:?}"
  parts="$(cat "$state")"
  for part in $parts; do printf '%s%s : start=1, size=1, type=c\n' "$2" "$part"; done
  exit 0
fi
[[ "$1" == --append ]] || exit 1
grep -qx '1 2' "${SFDISK_STATE_FILE:?}" || exit 1
[[ "$(cat)" == '2,,c' ]] || exit 1
printf '1 2 3\n' >"$SFDISK_STATE_FILE"
printf '%s\n' "$*" >>"${SFDISK_LOG:?}"
EOF
cat >"$tmp/tools/guestfish" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${GUESTFISH_LOG:?}"
EOF
chmod +x "$tmp/tools/sfdisk" "$tmp/tools/guestfish"

expect_fail() {
    if "$@" >/dev/null 2>&1; then fail "unexpected success: $*"; fi
}

printf '1 2 3\n' >"$tmp/state"
: >"$tmp/sfdisk.log"
expect_fail env PATH="$tmp/tools:$PATH" SFDISK_STATE_FILE="$tmp/state" SFDISK_LOG="$tmp/sfdisk.log" GUESTFISH_LOG="$tmp/guestfish.log" "$script" "$tmp/image.img" "$tmp/payload"
[[ ! -s "$tmp/sfdisk.log" ]] || fail "append ran on an image with an existing third partition"

printf '1 2\n' >"$tmp/state"
: >"$tmp/sfdisk.log"
: >"$tmp/guestfish.log"
env PATH="$tmp/tools:$PATH" SFDISK_STATE_FILE="$tmp/state" SFDISK_LOG="$tmp/sfdisk.log" GUESTFISH_LOG="$tmp/guestfish.log" "$script" "$tmp/image.img" "$tmp/payload"
rg -q -- '--append' "$tmp/sfdisk.log" || fail "valid layout did not append IESHARE"
rg -q 'mkfs vfat /dev/sda3' "$tmp/guestfish.log" || fail "valid layout did not format the appended third partition"
rg -q 'root_end=' "$script" || fail "IESHARE append does not start after the root partition"

echo 'Raspberry Pi IESHARE append contracts passed'
