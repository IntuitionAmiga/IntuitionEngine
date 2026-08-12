#!/usr/bin/env bash
set -euo pipefail
image="$1"
payload="$2"
guestfish_bin="${RPI_GUESTFISH:-guestfish}"
sfdisk_bin="${RPI_SFDISK:-sfdisk}"
"$guestfish_bin" --version >/dev/null 2>&1 || { echo 'guestfish is required' >&2; exit 1; }
# Create a FAT32 filesystem only in a new third partition. The partition table
# must be exactly the expected boot and root pair, so a changed golden layout
# cannot make /dev/sda3 refer to existing user data.
"$sfdisk_bin" --version >/dev/null 2>&1 || { echo 'sfdisk is required' >&2; exit 1; }
mapfile -t before_parts < <("$sfdisk_bin" --dump "$image" | awk '$2 == ":" { value = $1; if (match(value, /[0-9]+$/)) print substr(value, RSTART, RLENGTH) }')
[[ "${before_parts[*]}" == "1 2" ]] || { echo "expected exactly boot and root partitions 1 2, found: ${before_parts[*]:-none}" >&2; exit 1; }
# --append alone may choose a small pre-boot alignment gap.  Start after the
# existing root partition so IESHARE occupies only the newly enlarged tail.
root_end="$("$sfdisk_bin" --dump "$image" | awk '$2 == ":" && $1 ~ /2$/ { start = $0; sub(/.*start=[[:space:]]*/, "", start); sub(/,.*/, "", start); size = $0; sub(/.*size=[[:space:]]*/, "", size); sub(/,.*/, "", size); print start + size }')"
[[ "$root_end" =~ ^[0-9]+$ ]] || { echo 'unable to determine root partition end' >&2; exit 1; }
printf '%s,,c\n' "$root_end" | "$sfdisk_bin" --append "$image"
mapfile -t after_parts < <("$sfdisk_bin" --dump "$image" | awk '$2 == ":" { value = $1; if (match(value, /[0-9]+$/)) print substr(value, RSTART, RLENGTH) }')
[[ "${after_parts[*]}" == "1 2 3" ]] || { echo "IESHARE append did not create only partition 3, found: ${after_parts[*]:-none}" >&2; exit 1; }
"$guestfish_bin" --rw -a "$image" run : mkfs vfat /dev/sda3 : set-label /dev/sda3 IESHARE : mount /dev/sda3 / : copy-in "$payload/." /
