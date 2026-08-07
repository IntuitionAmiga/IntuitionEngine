# Z80 JIT parity benchmark evidence

`before.txt` and `after.txt` use the four existing amd64 Z80 JIT workloads,
one-second samples and three repetitions on the same host. Every after median
is lower: ALU 8.420 to 7.471 microseconds, Memory 4.901 to 4.418 microseconds,
Mixed 8.867 to 8.163 microseconds, and Call 9.442 to 8.612 microseconds.
`benchstat.txt` reports a 9.47 per cent geomean latency reduction.

`head-invalid.txt` preserves the initially measured checked-in comparison
candidate. It is not the correctness baseline: its ALU workload ends at 11,526
cycles while the interpreter ends at 11,522 cycles. The parity branch fixes
that execution contract, and `TestZ80JIT_BenchmarkWorkloadsAreConformanceValid`
now gates complete CPU state and 64 KiB memory for every measured workload.
The valid `before.txt` was captured before the hot-counter batching and
generation-stamp dispatch optimisation, after the workload conformance gate
passed.
