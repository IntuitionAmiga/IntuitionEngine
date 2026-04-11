#!/usr/bin/env bash
#
# run_6502_bench_report.sh - Run the portable 6502 benchmark binary and
# print a fixed-width Interpreter-vs-JIT comparison table for the five
# comparison workloads (ALU, Memory, Call, Branch, Mixed).
#
# This script is intentionally self-contained. It has no dependency on:
#
#   - The Go toolchain
#   - The IntuitionEngine source tree
#   - Python, Perl, or any interpreter beyond /bin/sh + awk
#
# You only need:
#
#   1. The standalone benchmark binary (default name: 6502_bench.test)
#      built by build_6502_benchmarks.sh on a machine with the source tree.
#   2. This script, next to the binary (or point BENCH_BIN at the binary).
#
# The shipping workflow is:
#
#     # on the dev machine
#     ./build_6502_benchmarks.sh
#     tar czf 6502_bench.tar.gz 6502_bench.test run_6502_bench_report.sh
#     # send the tarball to your friend
#
#     # on your friend's machine (same OS/arch)
#     tar xzf 6502_bench.tar.gz
#     ./run_6502_bench_report.sh
#
# Environment variables:
#
#     BENCH_BIN   - path to the benchmark binary (default: ./6502_bench.test)
#     BENCH_TIME  - -test.benchtime value (default: 3s)
#     BENCH_COUNT - -test.count (number of repetitions, default: 1)
#     RAW         - if set, also print the raw Go benchmark output
#
# Exits non-zero if the binary is missing, not executable, or fails.

set -eu

BENCH_BIN="${BENCH_BIN:-./6502_bench.test}"
BENCH_TIME="${BENCH_TIME:-3s}"
BENCH_COUNT="${BENCH_COUNT:-1}"
BENCH_PATTERN='Benchmark6502_(ALU|Memory|Call|Branch|Mixed)_'

if [ ! -e "${BENCH_BIN}" ]; then
    echo "error: benchmark binary not found at ${BENCH_BIN}" >&2
    echo "Build it on a machine with the source tree by running:" >&2
    echo "    ./build_6502_benchmarks.sh" >&2
    echo "or set BENCH_BIN=<path> if it lives elsewhere." >&2
    exit 2
fi
if [ ! -x "${BENCH_BIN}" ]; then
    echo "error: ${BENCH_BIN} exists but is not executable" >&2
    echo "Try: chmod +x ${BENCH_BIN}" >&2
    exit 2
fi

echo "Running ${BENCH_BIN}" >&2
echo "  pattern:    ${BENCH_PATTERN}" >&2
echo "  benchtime:  ${BENCH_TIME}" >&2
echo "  count:      ${BENCH_COUNT}" >&2
echo >&2

# Capture benchmark output. go test's -test.bench uses the same Go benchmark
# format regardless of how the test binary was built, so parsing is stable.
if ! OUTPUT=$("${BENCH_BIN}" \
        -test.run '^$' \
        -test.bench "${BENCH_PATTERN}" \
        -test.benchtime "${BENCH_TIME}" \
        -test.count "${BENCH_COUNT}" \
        -test.benchmem); then
    echo "error: benchmark binary failed (exit $?)" >&2
    if [ -n "${OUTPUT:-}" ]; then
        echo "--- binary output ---" >&2
        echo "${OUTPUT}" >&2
    fi
    exit 1
fi

if [ -n "${RAW:-}" ]; then
    echo "--- raw Go benchmark output ---"
    echo "${OUTPUT}"
    echo
fi

# Parse + print the fixed-width summary table in pure awk. awk is POSIX-
# mandated and present on every Unix-like host, so this report is portable
# to anywhere the benchmark binary itself will run.
#
# Expected benchmark line format (tab- or space-separated fields):
#   Benchmark6502_<Workload>_<Mode>-<CPU>  <iters>  <nsop> ns/op  \
#       <bytes> B/op  <allocs> allocs/op  <instrs> instructions/op
# The "instructions/op" column is produced by the custom
# b.ReportMetric() call in jit_6502_benchmark_test.go.
printf '%s\n' "${OUTPUT}" | awk '
BEGIN {
    nW = split("ALU Memory Call Branch Mixed", W, " ")
    nM = split("Interpreter JIT",              M, " ")
    COL = 12
}

/^Benchmark6502_/ {
    # Strip the trailing "-N" CPU suffix that go test appends to bench names.
    name = $1
    sub(/-[0-9]+$/, "", name)
    if (!match(name, /_(ALU|Memory|Call|Branch|Mixed)_(Interpreter|JIT)$/)) next
    pair = substr(name, RSTART + 1, RLENGTH - 1)
    split(pair, parts, "_")
    workload = parts[1]
    mode     = parts[2]

    # Scan remaining fields for "ns/op" and "instructions/op" tags.
    nsop = 0
    instrs = 0
    for (i = 2; i <= NF; i++) {
        if ($i == "ns/op"           && i > 2) nsop   = $(i-1) + 0
        if ($i == "instructions/op" && i > 2) instrs = $(i-1) + 0
    }
    if (nsop <= 0 || instrs <= 0) next

    key = mode "," workload
    ns[key]     += nsop
    ip[key]     += instrs
    seen[key]++
}

END {
    # Build separator bars.
    total = 18 + nW * (COL + 1)
    bar = ""
    dash = ""
    for (i = 0; i < total; i++) { bar = bar "="; dash = dash "-" }

    title = "6502 BENCHMARK RESULTS (MIPS)"
    lpad  = int((total - length(title)) / 2)

    print ""
    print bar
    printf "%*s%s\n", lpad, "", title
    print bar

    # Column header.
    printf "%-18s", "Mode"
    for (i = 1; i <= nW; i++) printf "|%*s", COL, W[i]
    printf "\n"

    # Separator row below header.
    printf "%-18s", ""
    for (i = 1; i <= nW; i++) printf "+%s", substr(dash, 1, COL)
    printf "\n"

    # Mode rows — values averaged if -test.count > 1.
    for (j = 1; j <= nM; j++) {
        printf "%-18s", M[j]
        for (i = 1; i <= nW; i++) {
            k = M[j] "," W[i]
            if (k in seen && seen[k] > 0) {
                avg_ns = ns[k] / seen[k]
                avg_ip = ip[k] / seen[k]
                if (avg_ns > 0) {
                    mips = avg_ip / avg_ns * 1000.0
                    printf "|%*.2f", COL, mips
                } else {
                    printf "|%*s", COL, "n/a"
                }
            } else {
                # Missing benchmark (e.g. JIT skipped on unsupported arch).
                printf "|%*s", COL, "skip"
            }
        }
        printf "\n"
    }
    print bar
    print ""

    # Speedup line, computed per workload from the ratio of the two modes.
    printf "Speedup (JIT / Interpreter, MIPS ratio):\n"
    printf "%-18s", ""
    for (i = 1; i <= nW; i++) {
        ik = "Interpreter," W[i]
        jk = "JIT," W[i]
        if ((ik in seen) && (jk in seen) && seen[ik] > 0 && seen[jk] > 0 \
            && ns[ik] > 0 && ns[jk] > 0) {
            imips = (ip[ik] / seen[ik]) / (ns[ik] / seen[ik]) * 1000.0
            jmips = (ip[jk] / seen[jk]) / (ns[jk] / seen[jk]) * 1000.0
            if (imips > 0) {
                label = sprintf("%.2fx", jmips / imips)
                printf "|%*s", COL, label
            } else {
                printf "|%*s", COL, "n/a"
            }
        } else {
            printf "|%*s", COL, "skip"
        }
    }
    printf "\n"
}
'
