# Performance Benchmark Evidence

This directory stores before/after benchmark captures for landed performance
items. Use `bench/` for pprof artefacts and focused profiling notes. Use this
directory for benchstat input and output that supports a specific optimisation.

## Workflow

Capture each item in its own subdirectory:

```bash
make bench-baseline BENCH_ITEM=m1_a1_ie64_resume BENCH_REGEX='BenchmarkIE64_(MMIO|MMU)'
make bench-after BENCH_ITEM=m1_a1_ie64_resume BENCH_REGEX='BenchmarkIE64_(MMIO|MMU)'
make bench-compare BENCH_ITEM=m1_a1_ie64_resume
```

The Makefile writes:

- `before.txt`: raw `go test -bench` output from the pre-change tree.
- `after.txt`: raw `go test -bench` output from the changed tree.
- `benchstat.txt`: comparison from the pinned `golang.org/x/perf/cmd/benchstat`.

Each raw file begins with comment lines for the capture date, git SHA, CPU
model, governor, GOAMD64, tags, bench regex, benchtime, and count. Recapture the
baseline when the machine, governor, thermal state, or branch base changes.

## Acceptance

Use `BENCH_COUNT=10` and `BENCH_TIME=1s` unless the item needs a longer window.
Benchstat needs at least six samples. The primary benchmark set should improve
with `p < 0.05`, and the guard benchmark set should not show a significant
regression.

| Item | Hardware | Primary result | Guard result | Notes |
|------|----------|----------------|--------------|-------|
| m1_a1_ie64_resume | Intel Xeon W-11955M | Smoke: `BenchmarkIE64_MMIO_{Interpreter,JIT}` and `BenchmarkIE64_MMU_Mixed_{Interpreter,JIT}` run with `-benchtime=1x` | Full benchstat pending | Added benchmark surface for helper resume and MMU micro-TLB work. |
| m2_a2_m68k_regions | Intel Xeon W-11955M | Smoke: `BenchmarkM68K_Mixed_JIT` ran at 75.9 us/op for 40,000 instructions with `-benchtime=1x` | Full benchstat pending | Added mixed M68K guard benchmark for region/fallback work. |
| c3_audio_alloc | Intel Xeon W-11955M | `TestReadSamples_ZeroAllocsSteadyState` passes | Full benchstat pending | Added `BenchmarkSoundChip_ReadSamples` and enabled allocation reporting on the 64-sample block benchmark. |

Benchmarks are sensitive to thermal and scheduler noise. Prefer the performance
CPU governor and the same machine/session for before and after captures.
