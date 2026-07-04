#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

export GOAMD64=v3

QEMU_AARCH64="${QEMU_AARCH64:-}"
if [[ -z "$QEMU_AARCH64" ]]; then
  QEMU_AARCH64="$(command -v qemu-aarch64-static || command -v qemu-aarch64 || true)"
fi
ARM64_QEMU_TEST_REGEX="${ARM64_QEMU_TEST_REGEX:-TestJITContext_FieldOffsets|TestIE64SMC|TestIE64FormRegion|TestIE64Region|TestIE64JIT_MMIOStoreMidBlock_ResumesInBlock|TestIE64JIT_HelperResume}"

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags "novulkan headless" .
if [[ -n "$QEMU_AARCH64" ]]; then
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -exec "$QEMU_AARCH64" -tags "novulkan headless" -run "$ARM64_QEMU_TEST_REGEX" -count 1 .
else
  echo "skipping Linux/arm64 qemu tests: qemu-aarch64 not found" >&2
fi
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags novulkan .
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -tags novulkan .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -tags novulkan .
IE_REQUIRE_JIT=1 CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go test -c -tags headless .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags novulkan .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go test -c -tags headless .
