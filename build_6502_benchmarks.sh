#!/usr/bin/env bash
#
# build_6502_benchmarks.sh - Build the portable 6502 benchmark binary.
#
# This script is a build helper only. It produces a fully static test
# binary (CGO_ENABLED=0 + osusergo + netgo + headless + novulkan,
# -trimpath, stripped link flags) that can be copied to another machine
# running the same OS/architecture and executed there without a Go
# toolchain, Vulkan SDK, audio libraries, libc, or the IntuitionEngine
# source tree.
#
# Both the interpreter AND JIT benchmarks run in the static binary. This
# is possible because the JIT call trampoline (jit_call.go) dispatches
# through runtime.asmcgocall — the raw g0 stack-switch primitive — rather
# than runtime.cgocall, which has an iscgo guard that fatals in
# CGO_ENABLED=0 builds. The asm trampolines in jit_call_{amd64,arm64}.s
# are already written to the asmcgocall contract ("called on the g0
# stack by asmcgocall") so no assembly changes were needed.
#
# After building, use the companion runner script to actually run the
# benchmarks and print the comparison table:
#
#     ./build_6502_benchmarks.sh            # produce ./6502_bench.test
#     ./run_6502_bench_report.sh            # run the binary + pretty table
#
# To package for shipping:
#
#     tar czf 6502_bench.tar.gz 6502_bench.test run_6502_bench_report.sh
#
# The runner script is intentionally self-contained: bash + awk only, no
# Python, Go, or codebase dependency. Your friend unpacks the tarball and
# runs ./run_6502_bench_report.sh — they get both Interpreter and JIT
# columns side by side.
#
# Environment variables:
#
#     BENCH_BIN     - output path (default: ./6502_bench.test)
#     BENCH_TAGS    - build tags (default: "osusergo netgo headless novulkan")
#                     osusergo + netgo swap the os/user and net packages
#                     to their pure-Go implementations, so nothing in the
#                     binary reaches for libc and the Go linker produces
#                     a fully static ELF.
#     CGO_ENABLED   - 0 (default, fully static; JIT still works via
#                     runtime.asmcgocall) or 1 (dynamically links libc —
#                     not needed for the JIT path since asmcgocall works
#                     in both modes, but provided as an escape hatch if
#                     you ever need cgo packages in a future benchmark).
#
# Exits non-zero if the Go test binary fails to build.

set -eu

BENCH_BIN="${BENCH_BIN:-./6502_bench.test}"
BENCH_TAGS="${BENCH_TAGS:-osusergo netgo headless novulkan}"
# Default to CGO disabled so the resulting binary is fully static and
# portable. The JIT benchmarks still run because jit_call.go routes
# through runtime.asmcgocall rather than runtime.cgocall.
if [ -z "${CGO_ENABLED+set}" ]; then
    CGO_ENABLED=0
fi
export CGO_ENABLED

if [ "${CGO_ENABLED}" = "0" ]; then
    cgo_desc="disabled (fully static binary, JIT via runtime.asmcgocall)"
else
    cgo_desc="enabled (dynamic libc linkage)"
fi

echo "Building 6502 benchmark binary:" >&2
echo "  target:   ${BENCH_BIN}" >&2
echo "  tags:     ${BENCH_TAGS}" >&2
echo "  CGO:      ${cgo_desc}" >&2
echo "  link:     -s -w -buildid=" >&2
echo "  path:     -trimpath" >&2
echo >&2

go test \
    -c \
    -tags "${BENCH_TAGS}" \
    -trimpath \
    -ldflags='-s -w -buildid=' \
    -o "${BENCH_BIN}" \
    .

if [ ! -x "${BENCH_BIN}" ]; then
    echo "error: build reported success but ${BENCH_BIN} is missing or not executable" >&2
    exit 1
fi

size=$(wc -c < "${BENCH_BIN}")
size_mib=$(awk -v s="${size}" 'BEGIN { printf "%.1f", s / 1024 / 1024 }')

# Verify the binary is actually static so we don't silently ship a binary
# that drags in libc. `file` reports "statically linked" for a pure Go
# build; "dynamically linked" means something pulled in a cgo dependency.
if command -v file >/dev/null 2>&1; then
    case "$(file "${BENCH_BIN}")" in
        *statically\ linked*) link_desc="statically linked" ;;
        *dynamically\ linked*) link_desc="dynamically linked (libc dependency)" ;;
        *) link_desc="unknown linkage" ;;
    esac
else
    link_desc="(file(1) unavailable, linkage unchecked)"
fi

echo "Built ${BENCH_BIN} (${size_mib} MiB, ${link_desc})" >&2
echo >&2
echo "Next steps:" >&2
echo "  Run locally with pretty table (default BENCH_TIME=3s):" >&2
echo "      ./run_6502_bench_report.sh" >&2
echo >&2
echo "  Override benchmark duration:" >&2
echo "      BENCH_TIME=5s ./run_6502_bench_report.sh" >&2
echo >&2
echo "  Run the binary directly with raw Go benchmark output:" >&2
echo "      ${BENCH_BIN} -test.run '^\$' \\" >&2
echo "          -test.bench 'Benchmark6502_(ALU|Memory|Call|Branch|Mixed)_' \\" >&2
echo "          -test.benchtime 3s -test.benchmem" >&2
echo >&2
echo "  Package for shipping to another machine (same OS/arch):" >&2
echo "      tar czf 6502_bench.tar.gz ${BENCH_BIN} run_6502_bench_report.sh" >&2
