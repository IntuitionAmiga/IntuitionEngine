# amd64 x86 JIT parity samples

Matched one-second `GOAMD64=v3` samples on the Intel Xeon W-11955M, with five
runs per benchmark. The baseline is detached committed `f5b0beb6`; `after` is
the uncommitted parity worktree.

| Workload | Baseline median | Current median | Change |
| --- | ---: | ---: | ---: |
| ALU JIT | 379,359 ns/op | 379,956 ns/op | 0.16% slower, within noise |
| Memory JIT | 421,771 ns/op | 416,045 ns/op | 1.36% faster |

The memory result is positive, but the five-run baseline includes two slower
outliers. These samples support retaining the correctness work and do not
support claiming a broad performance gain.
