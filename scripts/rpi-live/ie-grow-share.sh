#!/bin/sh
set -eu

device="${IE_SHARE_DEVICE:-/dev/mmcblk0p3}"
mountpoint=/var/ie/share
if findmnt -rn -S LABEL=IESHARE >/dev/null 2>&1; then
    exit 0
fi
mkdir -p "$mountpoint"
mount -L IESHARE "$mountpoint"
