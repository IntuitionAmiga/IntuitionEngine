# M1 A1 IE64 Helper Resume Notes

Date: 2026-07-04

## Current Measurement

The new helper-heavy benchmark names compile and run:

```bash
go test -tags headless -run '^$' -bench 'BenchmarkIE64_(MMIO|MMU_Mixed)' -benchtime=1x -count=1 .
```

Observed on Intel Xeon W-11955M:

| Benchmark | Result | Instructions |
|-----------|--------|--------------|
| `BenchmarkIE64_MMIO_Interpreter` | 574638 ns/op | 40004 instructions/op |
| `BenchmarkIE64_MMIO_JIT` | 959702 ns/op | 40004 instructions/op |
| `BenchmarkIE64_MMU_Mixed_Interpreter` | 792023 ns/op | 50004 instructions/op |
| `BenchmarkIE64_MMU_Mixed_JIT` | 2144306 ns/op | 50004 instructions/op |

These are smoke captures only. Full before/after `benchstat` evidence still
needs to be captured on a stable governor with `BENCH_COUNT=10`.

## Implementation Notes

- IE64 amd64 integer LOAD, STORE, PUSH, and POP helper exits can resume inside
  the current native block when the dispatcher guard passes.
- Resume is cancelled for pending interrupts, SMC invalidation, PTBR or MMU-mode
  changes, timer/debug state, or `IE64_JIT_RESUME=0`.
- MMU LOAD/STORE helpers fill a four-entry direct-mapped micro-TLB in
  `JITContext`; PTBR writes and TLB invalidation flush it.
- ARM64 and cross-backend helper-resume propagation remain open.
