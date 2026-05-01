#!/usr/bin/env bash
#
# build_all_cpu_benchmarks.sh - Build a portable, fully-static test
# binary that contains every CPU interpreter + JIT benchmark
# (6502 + Z80 + M68K + IE64 + x86) so the whole suite can be shipped to
# another machine and run there without a Go toolchain, Vulkan SDK,
# audio libraries, libc, or the IntuitionEngine source tree.
#
# Sister of build_6502_benchmarks.sh — same static-binary recipe, same
# PGO two-pass option — but the resulting binary contains the full set
# of CPU benches the run_all_cpu_benches.sh report consumes.
#
# Both interpreter AND JIT benchmarks run in the static binary. JIT
# works under CGO_ENABLED=0 because jit_call.go dispatches through
# runtime.asmcgocall (g0 stack-switch primitive) rather than
# runtime.cgocall (which has an iscgo guard that fatals in
# CGO_ENABLED=0 builds). The asm trampolines in jit_call_{amd64,arm64}.s
# are written to the asmcgocall contract.
#
# After building, use the companion runner script to actually run the
# benchmarks and print the comparison table:
#
#     ./build_all_cpu_benchmarks.sh         # produce ./all_cpu_bench.test
#     ./run_all_cpu_benches.sh              # run live (rebuilds via go test)
#
# To package for shipping to another machine:
#
#     tar czf all_cpu_bench.tar.gz all_cpu_bench.test run_all_cpu_benches.sh
#
# Note: run_all_cpu_benches.sh as shipped invokes `go test` directly.
# To run the standalone binary on a machine without a Go toolchain,
# call it with the -test.* flag form:
#
#     IE6502_ASM_INTERP=1 ./all_cpu_bench.test \
#         -test.run='^$' \
#         -test.bench='Benchmark(6502|Z80|M68K|IE32|IE64|X86JIT)_.+_(Interpreter|JIT)$' \
#         -test.benchtime=3s -test.benchmem
#     IE6502_ASM_INTERP=0 ./all_cpu_bench.test \
#         -test.run='^$' \
#         -test.bench='^Benchmark6502_.+_Interpreter$' \
#         -test.benchtime=3s -test.benchmem
#
# Then pipe both runs through the awk pivot at the bottom of
# run_all_cpu_benches.sh.
#
# Environment variables:
#
#     BENCH_BIN     output path (default: ./all_cpu_bench.test)
#     BENCH_TAGS    build tags (default: "osusergo netgo headless novulkan")
#                   osusergo + netgo swap os/user and net to pure-Go
#                   implementations so nothing reaches libc and the Go
#                   linker produces a fully static ELF.
#     CGO_ENABLED   0 (default, fully static; JIT still works via
#                   runtime.asmcgocall) or 1 (dynamic libc).
#     PGO           1 (default) two-pass profile-guided build:
#                     pass 1: build unoptimized profiling binary
#                     pass 2: collect profile across all CPU benches
#                     pass 3: rebuild with -pgo=<profile>
#                   0 disables PGO entirely (single-pass).
#     PGO_PROFILE   collected CPU profile path
#                   (default: ./default.pgo - go's auto-detected name).
#     PGO_TIME      -test.benchtime for profile collection
#                   (default: 1s; long enough for stable samples,
#                   short enough to keep two-pass build time reasonable).
#
# Exits non-zero if the Go test binary fails to build.

set -eu

cd "$(dirname "$0")"

BENCH_BIN="${BENCH_BIN:-./all_cpu_bench.test}"
BENCH_TAGS="${BENCH_TAGS:-osusergo netgo headless novulkan}"
PGO="${PGO:-1}"
PGO_PROFILE="${PGO_PROFILE:-./default.pgo}"
PGO_TIME="${PGO_TIME:-1s}"
PGO_PATTERN='Benchmark(6502|Z80|M68K|IE32|IE64|X86JIT)_.+_(Interpreter|JIT)$'

if [ -z "${CGO_ENABLED+set}" ]; then
    CGO_ENABLED=0
fi
export CGO_ENABLED

if [ "${CGO_ENABLED}" = "0" ]; then
    cgo_desc="disabled (fully static binary, JIT via runtime.asmcgocall)"
else
    cgo_desc="enabled (dynamic libc linkage)"
fi

if [ "${PGO}" = "0" ]; then
    pgo_desc="disabled (single-pass build)"
else
    pgo_desc="enabled (two-pass: profile with ${PGO_TIME}, rebuild with -pgo=${PGO_PROFILE})"
fi

echo "Building all-CPU benchmark binary:" >&2
echo "  target:   ${BENCH_BIN}" >&2
echo "  tags:     ${BENCH_TAGS}" >&2
echo "  CGO:      ${cgo_desc}" >&2
echo "  PGO:      ${pgo_desc}" >&2
echo "  link:     -s -w -buildid=" >&2
echo "  path:     -trimpath" >&2
echo >&2

if [ "${PGO}" = "0" ]; then
    go test \
        -c \
        -tags "${BENCH_TAGS}" \
        -trimpath \
        -ldflags='-s -w -buildid=' \
        -o "${BENCH_BIN}" \
        .
else
    echo "[pgo 1/3] building profiling binary" >&2
    go test \
        -c \
        -tags "${BENCH_TAGS}" \
        -trimpath \
        -o "${BENCH_BIN}" \
        .

    if [ ! -x "${BENCH_BIN}" ]; then
        echo "error: profiling build reported success but ${BENCH_BIN} is missing" >&2
        exit 1
    fi

    echo "[pgo 2/3] collecting profile from ${PGO_PATTERN} (benchtime=${PGO_TIME})" >&2
    "${BENCH_BIN}" \
        -test.run='^$' \
        -test.bench="${PGO_PATTERN}" \
        -test.benchtime="${PGO_TIME}" \
        -test.cpuprofile="${PGO_PROFILE}" \
        >/dev/null

    if [ ! -s "${PGO_PROFILE}" ]; then
        echo "error: ${PGO_PROFILE} is missing or empty after profile run" >&2
        echo "set PGO=0 to skip PGO and produce a non-optimized binary" >&2
        exit 1
    fi
    profile_size=$(wc -c < "${PGO_PROFILE}")
    echo "[pgo 2/3] collected ${PGO_PROFILE} (${profile_size} bytes)" >&2

    echo "[pgo 3/3] rebuilding with -pgo=${PGO_PROFILE}" >&2
    go test \
        -c \
        -tags "${BENCH_TAGS}" \
        -trimpath \
        -ldflags='-s -w -buildid=' \
        -pgo="${PGO_PROFILE}" \
        -o "${BENCH_BIN}" \
        .
fi

if [ ! -x "${BENCH_BIN}" ]; then
    echo "error: build reported success but ${BENCH_BIN} is missing or not executable" >&2
    exit 1
fi

size=$(wc -c < "${BENCH_BIN}")
size_mib=$(awk -v s="${size}" 'BEGIN { printf "%.1f", s / 1024 / 1024 }')

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
echo "  Run live with the pivot table (rebuilds in-place via go test):" >&2
echo "      ./run_all_cpu_benches.sh" >&2
echo >&2
echo "  Run the standalone binary on a machine WITHOUT the source tree" >&2
echo "  (raw Go benchmark output; pipe through the awk block at the" >&2
echo "  bottom of run_all_cpu_benches.sh to get the table):" >&2
echo "      ${BENCH_BIN} -test.run '^\$' \\" >&2
echo "          -test.bench '${PGO_PATTERN}' \\" >&2
echo "          -test.benchtime 3s -test.benchmem" >&2
echo >&2
echo "      # 6502 Go-interp baseline pass (asm path off):" >&2
echo "      IE6502_ASM_INTERP=0 ${BENCH_BIN} -test.run '^\$' \\" >&2
echo "          -test.bench '^Benchmark6502_.+_Interpreter\$' \\" >&2
echo "          -test.benchtime 3s -test.benchmem" >&2
echo >&2
echo "  Package for shipping to another machine (same OS/arch):" >&2
echo "      tar czf all_cpu_bench.tar.gz ${BENCH_BIN} run_all_cpu_benches.sh" >&2
