# Phase 9 Bucketed Uniformity Gate

Fixtures for `bench_uniformity_gate_test.go`.

## Gate matrix (reviewer-proof contract)

| Test | Skips when | Runs when |
|------|------------|-----------|
| `TestBenchUniformity_Metric1` | `IE_BENCH_GATE` env unset | `IE_BENCH_GATE=1` set + `current.txt` present |
| `TestBenchUniformity_OwnBaseline` | `IE_BENCH_GATE` env unset | `IE_BENCH_GATE=1` set + both `current.txt` and `baseline.txt` present |
| `TestBenchUniformity_Metric2` | `metric2.txt` absent | `metric2.txt` present (parser+gate auto-runs) |
| `TestBenchUniformity_GateMatrix` | never | always — pins the contract above |

Default `go test` on a fresh clone has no `current.txt`, no `metric2.txt`,
no `IE_BENCH_GATE`, so all three measurement gates pass-by-skip. CI cannot
perf-fail. Run measurement gates only on a clean mains-power machine.

The gate has three tests covering two metrics:

- **Metric 1 — bucketed cross-backend gate** (`TestBenchUniformity_Metric1`)
  - Candidate-parity bucket (ALUTight, MemStream, BranchDense): cell within
    ±20% of per-workload median. Provisional — widen or reclassify if
    measured sweeps stay structurally outside ±20% after the JIT-summit
    cleanups.
  - Structural bucket (CallChurn, Mixed): cell ≥ 0.33× per-workload median.
    High outliers are logged-not-gated; the regression check below catches
    inflated benchmarks.
- **Own-baseline regression** (`TestBenchUniformity_OwnBaseline`): each cell
  must not drop more than 20% below its committed baseline. Jumps ≥50%
  above baseline are logged-warned for reviewer judgment.
- **Metric 2 — per-backend real workloads** (`TestBenchUniformity_Metric2`):
  parser + 3-condition gate live. Skips when `metric2.txt` is absent.
  Format below; parser unit tests in `bench_uniformity_metric2_test.go`.

## `current.txt` (local measurement, gitignored)

Standard `go test -bench` output. The gate scans for lines beginning with
`Benchmark` and a side metric `<value> MIPS_host`. Run from the repo root:

```
go test -tags headless -run='^$' -bench='Benchmark.*_JIT$' -benchtime 3s ./... \
  | tee testdata/bench-uniformity/current.txt
```

The gate skips when this file is absent so the default `go test` does not
require local benchmark generation.

## `baseline.txt` (committed accepted floor)

Reviewer-gated. Encodes "we accept these numbers as the new perf floor"
decisions. Initial Phase 9 v1 baseline was bootstrapped from a clean
sweep after the bucketed gate landed; refresh by running the sweep, then
opening a PR labeled "Phase 9 baseline update" with the new file.

`current.txt` and `baseline.txt` share the same format. The gate rejects
a current cell whose MIPS dropped >20% below its baseline cell.

## `metric2.txt` (per-backend real workloads)

Per-backend canonical workloads. Records separated by `---`. Lines starting
with `#` and blank lines are ignored. Unknown keys cause a parse error.

```
backend=<6502|z80|m68k|ie64|x86>
workload=<short-name>
metric=<frames_per_sec|wallclock_ms_to_milestone>
phase0=<float>
current=<float>
real_time_target=<float>      ; units match `metric` (Hz or ms)
jit_bound_phase0=<float>      ; ns spent in JIT during Phase 0 sample
jit_bound_current=<float>     ; ns spent in JIT during current sample
io_bound_waiver=<true|false>  ; if true, condition (3) is waived
waiver_reason=<text>          ; required when io_bound_waiver=true
---
```

Per-backend gate (all three conditions):

  1. No regression vs Phase 0 (within ±5% of `phase0`).
  2. Reaches real-time. Direction depends on `metric`:
     - `frames_per_sec`: `current ≥ real_time_target`.
     - `wallclock_ms_to_milestone`: `current ≤ real_time_target`.
  3. ≥10% improvement on JIT-bound time vs Phase 0
     (`jit_bound_current ≤ 0.90 × jit_bound_phase0`). Waivable when
     `io_bound_waiver=true` with `waiver_reason` recorded.
