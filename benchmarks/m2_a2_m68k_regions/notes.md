# M2 A2.2 M68K Region and Fallback Notes

Date: 2026-07-04

## Current Measurement

This checkout does not include a ROM-equipped AROS boot capture from this
session, so no boot-to-Wanderer wall-clock table is recorded here yet. The local
guard benchmark added for this slice is:

```bash
go test -tags headless -run '^$' -bench 'BenchmarkM68K_Mixed' -benchtime=1x -count=1 .
```

Observed on Intel Xeon W-11955M:

| Benchmark | Result | Instructions |
|-----------|--------|--------------|
| `BenchmarkM68K_Mixed_Interpreter` | 296451 ns/op | 40000 instructions/op |
| `BenchmarkM68K_Mixed_JIT` | 75855 ns/op | 40000 instructions/op |

## Implementation Notes

- M68K region floors now route through `TierController.ShouldPromoteRegion`.
- The production fallback path uses one-instruction fallback for unsupported
  leading instructions by default.
- Production-safe AROS block-head and epilogue shapes are covered by existing
  targeted tests.
- Full AROS `IE_PERF_ACCT=1` fallback-opcode ranking remains to be captured on a
  ROM-equipped machine.
