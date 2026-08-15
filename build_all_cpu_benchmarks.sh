#!/usr/bin/env bash
#
# build_all_cpu_benchmarks.sh - Build a test binary that contains every CPU
# interpreter and JIT benchmark
# (6502 + Z80 + M68K + IE64 + x86) so the whole suite can be shipped to
# another machine and run there without a Go toolchain or the source tree.
#
# Sister of build_6502_benchmarks.sh with the same
# PGO two-pass option - but the resulting binary contains the full set
# of CPU benches the run_all_cpu_benches.sh report consumes.
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
#     CGO_ENABLED   must be 1 for Linux native JIT benchmarks.
#     PGO           1 (default) two-pass profile-guided build:
#                     pass 1: build unoptimised profiling binary
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

export GOAMD64=v3

BENCH_BIN="${BENCH_BIN:-./all_cpu_bench.test}"
BENCH_TAGS="${BENCH_TAGS:-osusergo netgo headless novulkan}"
PGO="${PGO:-1}"
PGO_PROFILE="${PGO_PROFILE:-./default.pgo}"
PGO_TIME="${PGO_TIME:-1s}"
PGO_PATTERN='Benchmark(6502|Z80|M68K|IE32|IE64|X86JIT)_.+_(Interpreter|JIT)$'

if [ -z "${CGO_ENABLED+set}" ]; then
    CGO_ENABLED=1
fi
export CGO_ENABLED

if [ "${CGO_ENABLED}" != "1" ]; then
    echo "error: Linux native JIT benchmarks require CGO_ENABLED=1" >&2
    exit 1
fi
cgo_desc="enabled"

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
        echo "set PGO=0 to skip PGO and produce a non-optimised binary" >&2
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

link_desc=$(file -b "${BENCH_BIN}" 2>/dev/null || printf 'linkage not inspected')

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
