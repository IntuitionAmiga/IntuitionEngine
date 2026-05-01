#!/usr/bin/env bash
#
# run_all_cpu_benches.sh - Run every CPU interpreter + JIT benchmark
# (6502, Z80, M68K, IE64, x86) and pivot the results into a fixed-width
# Interpreter-vs-JIT comparison table.
#
# 6502 has two interpreter implementations:
#   - the Go-coded interpreter (executeFast/ExecuteFast in cpu_six5go2.go)
#   - the hand-written amd64 asm interpreter (cpu_6502_interp_amd64.s,
#     selected by default when IE6502_ASM_INTERP is unset; disabled
#     when IE6502_ASM_INTERP={0|false|off})
# This script runs the 6502 sweep twice so both interpreter columns
# appear in the table.
#
# Naming contract (every CPU bench follows the pattern):
#   Benchmark<CPU>_<Workload>_(Interpreter|JIT)
#       CPU      = 6502 | Z80 | M68K | IE64 | X86JIT
#       Workload = ALU | Memory | Call | Branch | Mixed | FPU | String |
#                  Chain_BRA | LazyCCR_CMP_Bcc | MemCopy
# Each bench emits a `MIPS_host` side metric (host-normalized MIPS),
# which is the column the pivot uses.
#
# Usage:
#   ./run_all_cpu_benches.sh                # default 3s benchtime, 1 count
#   BENCH_TIME=1s ./run_all_cpu_benches.sh  # quick smoke
#   BENCH_COUNT=3 ./run_all_cpu_benches.sh  # 3 reps for variance
#   RAW=1 ./run_all_cpu_benches.sh          # also dump raw bench output
#
# Environment variables:
#   BENCH_TIME       -test.benchtime value (default: 3s)
#   BENCH_COUNT      -test.count value (default: 1)
#   RAW              if set, also print the raw Go benchmark output
#   GO_BUILD_TAGS    extra build tags (default: headless)
#   SKIP_ASM_INTERP  if set, skip the 6502 asm-interp second pass

set -eu

cd "$(dirname "$0")"

BENCH_TIME="${BENCH_TIME:-3s}"
BENCH_COUNT="${BENCH_COUNT:-1}"
GO_BUILD_TAGS="${GO_BUILD_TAGS:-headless}"
PATTERN='Benchmark(6502|Z80|M68K|IE32|IE64|X86JIT)_.+_(Interpreter|JIT)$'

if ! command -v go >/dev/null 2>&1; then
    echo "error: go toolchain not on PATH" >&2
    exit 2
fi

run_sweep() {
    local label="$1"
    shift
    echo ">>> [$label] go test -tags $GO_BUILD_TAGS -bench '$PATTERN' -benchtime $BENCH_TIME -count $BENCH_COUNT" >&2
    "$@" go test -tags "$GO_BUILD_TAGS" \
        -run '^$' \
        -bench "$PATTERN" \
        -benchtime "$BENCH_TIME" \
        -count "$BENCH_COUNT" \
        -benchmem \
        ./...
}

# Pass 1: every backend with default interpreter selection. For 6502
# this means the asm interpreter (cpu_6502_interp_amd64.s) — its init()
# enables itself unless IE6502_ASM_INTERP is set to 0/false/off.
PASS1=$(run_sweep "asm-interp" env)

# Pass 2: re-run only the 6502 _Interpreter benches with the asm path
# disabled, so the "Interp(Go)" column reflects the pure-Go interpreter
# (executeFast in cpu_six5go2.go).
PASS2=""
if [ -z "${SKIP_ASM_INTERP:-}" ]; then
    PASS2=$(env IE6502_ASM_INTERP=0 go test -tags "$GO_BUILD_TAGS" \
        -run '^$' \
        -bench '^Benchmark6502_.+_Interpreter$' \
        -benchtime "$BENCH_TIME" \
        -count "$BENCH_COUNT" \
        -benchmem \
        ./...)
fi

if [ -n "${RAW:-}" ]; then
    echo "--- pass 1 raw ---"
    echo "$PASS1"
    if [ -n "$PASS2" ]; then
        echo "--- pass 2 raw (6502 asm-interp) ---"
        echo "$PASS2"
    fi
    echo
fi

# Combine, marking the asm-interp lines so awk can route them to a
# separate column.
COMBINED=$(printf '%s\n=== ASM_INTERP_MARK ===\n%s\n' "$PASS1" "$PASS2")

printf '%s\n' "$COMBINED" | awk '
BEGIN {
    asm_mode = 0
}

/^=== ASM_INTERP_MARK ===$/ { asm_mode = 1; next }

/^Benchmark/ {
    name = $1
    sub(/-[0-9]+$/, "", name)  # strip Go bench -N CPU suffix
    if (!match(name, /^Benchmark(6502|Z80|M68K|IE32|IE64|X86JIT)_(.+)_(Interpreter|JIT)$/)) next

    # Pull (cpu, workload, mode) out of the matched name.
    # awk does not give us capture groups, so split on underscore and
    # reconstruct.
    bare = substr(name, 10)            # strip "Benchmark"
    n = split(bare, parts, "_")
    cpu = parts[1]
    mode = parts[n]
    workload = parts[2]
    for (i = 3; i < n; i++) workload = workload "_" parts[i]

    # Find MIPS_host side metric.
    mips = 0
    for (i = 2; i <= NF; i++) {
        if ($i == "MIPS_host" && i > 2) { mips = $(i-1) + 0; break }
    }
    if (mips == 0) next

    key = cpu "|" workload
    cpus[cpu] = 1
    workloads[cpu "|" workload] = workload
    keyset[key] = 1

    if (mode == "JIT")                  jit[key] = mips
    else if (cpu == "6502" && !asm_mode) asm_interp[key] = mips
    else if (cpu == "6502" && asm_mode)  go_interp[key] = mips
    else                                 go_interp[key] = mips
}

function fmt_mips(v,    s, neg, intpart, frac, out, i, len, comma_pos) {
    # Format a MIPS value with thousands-separator commas + 1 decimal:
    # 1234.5 -> "1,234.5"; "-" stays "-".
    if (v <= 0) return "-"
    s = sprintf("%.1f", v)
    n = split(s, dp, ".")
    intpart = dp[1]
    frac = (n > 1) ? dp[2] : "0"
    out = ""
    len = length(intpart)
    comma_pos = len % 3
    for (i = 1; i <= len; i++) {
        out = out substr(intpart, i, 1)
        if (i < len && ((i - comma_pos) % 3 == 0)) out = out ","
    }
    return out "." frac
}

function fmt_ratio(g, j) {
    if (g <= 0 || j <= 0) return "-"
    return sprintf("%.2fx", j / g)
}

function repeat(ch, n,    out, i) {
    out = ""
    for (i = 1; i <= n; i++) out = out ch
    return out
}

function rule(sep,    out) {
    out = "+"
    out = out repeat(sep, cpu_w + 2) "+"
    out = out repeat(sep, workload_w + 2) "+"
    out = out repeat(sep, mips_w + 2) "+"
    out = out repeat(sep, mips_w + 2) "+"
    out = out repeat(sep, mips_w + 2) "+"
    out = out repeat(sep, ratio_w + 2) "+"
    return out
}

END {
    cpu_w = 6
    workload_w = 20
    mips_w = 12
    ratio_w = 10
    TOP = rule("-")
    MID = TOP
    SEP = rule(".")
    BOT = TOP

    printf "\n"
    printf "%s\n", TOP
    printf "| %-*s | %-*s | %*s | %*s | %*s | %*s |\n", \
        cpu_w, "CPU", workload_w, "Workload", mips_w, "Interp(Go)", \
        mips_w, "Interp(asm)", mips_w, "JIT", ratio_w, "JIT/Interp"
    printf "%s\n", MID

    nCPU = split("6502 Z80 M68K IE32 IE64 X86JIT", cpu_order, " ")
    first_cpu = 1
    for (ci = 1; ci <= nCPU; ci++) {
        c = cpu_order[ci]
        nW = 0
        delete wlist
        for (k in keyset) {
            split(k, kp, "|")
            if (kp[1] == c) { nW++; wlist[nW] = kp[2] }
        }
        if (nW == 0) continue
        for (i = 1; i < nW; i++) for (j = i+1; j <= nW; j++)
            if (wlist[j] < wlist[i]) { t = wlist[i]; wlist[i] = wlist[j]; wlist[j] = t }

        if (!first_cpu) printf "%s\n", SEP
        first_cpu = 0

        for (wi = 1; wi <= nW; wi++) {
            w = wlist[wi]
            k = c "|" w
            gi = (k in go_interp) ? go_interp[k] : 0
            ai = (k in asm_interp) ? asm_interp[k] : 0
            jv = (k in jit) ? jit[k] : 0

            # JIT/Interp ratio prefers asm-interp denominator when present
            # (more honest for 6502 since the asm path is the production
            # interpreter), else falls back to Go interp.
            denom = (ai > 0) ? ai : gi
            ratio = fmt_ratio(denom, jv)

            cpu_label = (wi == 1) ? c : ""

            printf "| %-*s | %-*s | %*s | %*s | %*s | %*s |\n", \
                cpu_w, cpu_label, workload_w, w, mips_w, fmt_mips(gi), \
                mips_w, fmt_mips(ai), mips_w, fmt_mips(jv), ratio_w, ratio
        }
    }
    printf "%s\n", BOT
    printf "\n"
    printf "Units : MIPS_host  (host-normalized millions of guest instructions per second; higher is better)\n"
    printf "Ratio : JIT / Interp (asm-interp denominator when both present, else Go interp)\n"
    printf "Note  : 6502 is the only backend with a hand-written asm interpreter; others show \"-\" in that column.\n"
    printf "        IE32 has no JIT (per CLAUDE.md); shows \"-\" in JIT column.\n"
}'
