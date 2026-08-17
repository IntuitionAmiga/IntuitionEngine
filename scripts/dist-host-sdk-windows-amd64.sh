#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compiler="${HOST_SDK_CC:-x86_64-w64-mingw32-gcc}"
objdump="${HOST_SDK_OBJDUMP:-x86_64-w64-mingw32-objdump}"

HOST_SDK_ARCH=amd64 \
HOST_SDK_GOOS=windows \
HOST_SDK_GOARCH=amd64 \
HOST_SDK_NAME=intuition-engine-host-sdk-windows-amd64 \
HOST_SDK_CC="${compiler}" \
HOST_SDK_OBJDUMP="${objdump}" \
HOST_SDK_BINARY_SUFFIX=.exe \
HOST_SDK_ARCHIVE_FORMAT=zip \
exec bash "${root_dir}/scripts/dist-host-sdk-linux-amd64.sh"
