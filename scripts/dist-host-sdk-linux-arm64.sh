#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sysroot="${HOST_SDK_SYSROOT:-${CROSS_SYSROOT:-${root_dir}/../IntuitionSubtractor/sysroot-arm64}}"
compiler="${HOST_SDK_CC:-${CROSS_CC:-aarch64-linux-gnu-gcc}}"
qemu="${HOST_SDK_QEMU_AARCH64:-$(command -v qemu-aarch64-static || command -v qemu-aarch64 || true)}"
[[ -d "${sysroot}" ]] || { echo "dist-host-sdk-arm64: ARM64 sysroot is unavailable: ${sysroot}" >&2; exit 1; }
sysroot="$(cd "${sysroot}" && pwd)"

HOST_SDK_ARCH=arm64 \
HOST_SDK_GOARCH=arm64 \
HOST_SDK_NAME=intuition-engine-host-sdk-linux-arm64 \
HOST_SDK_CC="${compiler}" \
HOST_SDK_SYSROOT="${sysroot}" \
HOST_SDK_QEMU_AARCH64="${qemu}" \
exec bash "${root_dir}/scripts/dist-host-sdk-linux-amd64.sh"
