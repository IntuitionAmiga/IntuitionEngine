#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

export GOAMD64=v3

QEMU_AARCH64="${QEMU_AARCH64:-}"
if [[ -z "$QEMU_AARCH64" ]]; then
  QEMU_AARCH64="$(command -v qemu-aarch64-static || command -v qemu-aarch64 || true)"
fi

# GODEBUG=asyncpreemptoff=1 is required, not a preference. Go delivers its
# async-preemption signal at arbitrary points, including while a goroutine is
# executing JIT-compiled guest code, and qemu user mode mishandles signal
# delivery into a translation block that is running dynamically generated code.
# The result is SIGSEGV with no Go traceback, at a random point: measured 0 of 6
# runs passing with preemption on, and 8 of 8 with it off. It is not a product
# defect. The same code takes the same signals natively on amd64 across
# thousands of JIT executions per sweep without incident, the arm64 icache
# sequence is correct (DC CVAU on the writable alias, IC IVAU on the exec alias,
# with barriers), and the crash disappears under gdb, which intercepts signals
# before the guest sees them.
#
# Turning preemption off only affects this emulated test run. It does not change
# what is compiled or shipped.
ARM64_QEMU_GODEBUG="${ARM64_QEMU_GODEBUG:-asyncpreemptoff=1}"

ARM64_QEMU_TEST_REGEX="${ARM64_QEMU_TEST_REGEX:-TestJITContext_FieldOffsets|TestIE64SMC|TestIE64FormRegion|TestIE64Region|TestIE64JIT_MMIOStoreMidBlock_ResumesInBlock|TestIE64JIT_HelperResume|TestIE64Worker|TestARM64_|TestARM64RegMap_|TestJIT_ARM64_|TestJIT_vs_Interpreter_FP}"

# The only tests skipped are the three that qemu user mode genuinely cannot
# host: they need guest memory mapped above 4 GiB, so they read back host
# pointers. Everything else runs, including the emitter suite, the FP parity
# matrix and TestARM64RegMap_, which pins the exclusion of the X18 platform
# register that the build-only darwin/arm64 and windows/arm64 steps cannot
# catch.
ARM64_QEMU_SKIP_REGEX="${ARM64_QEMU_SKIP_REGEX:-TestJIT_ARM64_(IE64Load_HighBacking_EndToEnd|LOAD_HighAddr_HelperEndToEnd|POP_HighSP_HelperEndToEnd)}"

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags "novulkan headless" .
if [[ -n "$QEMU_AARCH64" ]]; then
  GODEBUG="$ARM64_QEMU_GODEBUG" CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -exec "$QEMU_AARCH64" -tags "novulkan headless" -run "$ARM64_QEMU_TEST_REGEX" -skip "$ARM64_QEMU_SKIP_REGEX" -count 1 .
else
  echo "skipping Linux/arm64 qemu tests: qemu-aarch64 not found" >&2
fi
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags novulkan .
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -tags novulkan .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -tags novulkan .
IE_REQUIRE_JIT=1 CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go test -c -tags headless .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags novulkan .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go test -c -tags headless .
