# 6502 Interpreter Profiling Artifacts

This directory holds the CPU profiles captured between the legacy and the
fast 6502 interpreter paths, produced to satisfy verification step 11 of the
6502 optimization plan:

```
go test -tags headless -bench "Benchmark6502" -cpuprofile cpu.prof
```

## Files

- `interp_baseline.pprof` — cpu profile with `Execute()` forced to
  `executeLegacy()` (the original generic interpreter) for the comparison
  benchmarks `Benchmark6502_{ALU,Memory,Call,Branch,Mixed}_Interpreter` at
  `-benchtime 2s`.
- `interp_final.pprof` — cpu profile with `Execute()` routed to
  `ExecuteFast()` (the full validation-subset inline dispatch) on the same
  benchmarks and the same settings.

The matching `interp_baseline.test` / `interp_final.test` binaries used to
capture the profiles are **not** committed (19 MiB each). Go's pprof files
include enough symbol information for `go tool pprof -top`, `-list`, and
`-web` without the binary, which covers the summary table below and most
day-to-day inspection. If you need `-disasm` (raw machine code listing for
a specific function), rebuild the binaries with the `go test -c` commands
in the next section and point pprof at the fresh binary explicitly:

```bash
go tool pprof bench/interp_final.test bench/interp_final.pprof
```

## How they were captured

Baseline (temporarily patching `Execute()` to unconditionally delegate to
`executeLegacy()` so the router to `ExecuteFast()` is bypassed):

```bash
go test -tags headless -run '^$' \
    -bench 'Benchmark6502_(ALU|Memory|Call|Branch|Mixed)_Interpreter' \
    -benchtime 2s \
    -cpuprofile bench/interp_baseline.pprof \
    -o bench/interp_baseline.test .
```

Final (with `Execute()` restored to the production routing):

```bash
go test -tags headless -run '^$' \
    -bench 'Benchmark6502_(ALU|Memory|Call|Branch|Mixed)_Interpreter' \
    -benchtime 2s \
    -cpuprofile bench/interp_final.pprof \
    -o bench/interp_final.test .
```

## Top-cum summary (from `go tool pprof -top -cum`)

### `interp_baseline.pprof` — Duration 15.66s, Total samples 15.69s

```
     flat  flat%   sum%        cum   cum%
        0     0%     0%     15.46s 98.53%  (*CPU_6502).Execute (inline)
    2.87s 18.29% 18.29%     15.46s 98.53%  (*CPU_6502).executeLegacy
    2.12s 13.51% 31.80%      6.15s 39.20%  (*CPU_6502).readByte
    4.03s 25.69% 57.55%      4.03s 25.69%  (*Bus6502Adapter).ReadFast
    0.23s  1.47%          1.45s  9.24%     op6502_D0   (BNE)
    0.62s  3.95%          1.38s  8.80%     (*CPU_6502).branch
    0.14s  0.89%          1.27s  8.09%     op6502_20   (JSR)
    1.25s  7.97%          1.25s  7.97%     sync/atomic.(*Bool).Load
```

The legacy path spends ~40% of total time in `readByte` + `ReadFast` (every
byte fetch is a function call through the adapter) and dispatches each
instruction through `opcodeTable[opcode](cpu)` — visible as the scattered
`op6502_*` symbols.

### `interp_final.pprof` — Duration 12.63s, Total samples 12.66s

```
     flat  flat%   sum%        cum   cum%
    9.82s 77.57% 77.65%     12.48s 98.58%  (*CPU_6502).ExecuteFast
    2.00s 15.80% 93.44%      2.00s 15.80%  sync/atomic.(*Bool).Load
    0.49s  3.87% 97.31%      0.49s  3.87%  adc6502Binary
```

`ExecuteFast` holds ~78% of flat time, which means the dispatch + addressing
mode decoding + ALU bodies all ran inside the single big switch with minimal
function-call overhead. `adc6502Binary` shows up as the only visible ALU
helper (3.87%) — the other ALU helpers (`cmp6502`, `asl6502`, etc.) are
tiny enough that Go's inliner folded them fully into the switch and the
sampler never attributed a sample back to them. `sync/atomic.(*Bool).Load`
is now 16% of time — the per-instruction polls for `resetting`, `rdyLine`,
`nmiPending`, `irqPending`, and `running` are a real fraction of the fast
loop and a plausible target for future batching.

## Wall-clock speedup

| Workload | Baseline ns/op | Final ns/op | Speedup |
|----------|---------------:|------------:|--------:|
| ALU      |         27,011 |      13,110 |   2.06x |
| Memory   |         20,957 |       5,938 |   3.53x |
| Call     |         18,096 |       7,499 |   2.41x |
| Branch   |         16,555 |       8,164 |   2.03x |
| Mixed    |         26,467 |      14,458 |   1.83x |

(From a single 2-second run on Intel i5-8365U; results vary with thermal
state. The runner script `run_6502_bench_report.sh` from the repo root
produces the same numbers in MIPS form alongside the JIT results.)
