# M68K CPU Test Suite

This directory contains the portable AmigaOS-compatible 68020/FPU CPU test harness described in the project plan.

Current state:

- The runtime harness is real assembly and writes failure-only reports to `RAM:cputest`.
- Test shards are generated from a Go catalog so the repetitive instruction matrix can scale without making the Amiga-side source host-dependent.
- The initial catalog is representative rather than exhaustive. It establishes the build, runtime, sharding, and reporting pipeline that future catalog expansion will reuse.

Layout:

- `include/`: common runtime and generated shard manifest.
- `generated/`: emitted case bodies from `cmd/gen_m68k_cputest`.
- `cputest_*.asm`: suite and shard entrypoints that can be assembled independently.

Generate the case includes:

```bash
go run ./cmd/gen_m68k_cputest
```

Build the suite and shards:

```bash
./sdk/scripts/build-cputest.sh
```

The emitted sources remain plain Motorola syntax and can be reassembled on an Amiga with a suitable `vasmm68k_mot` setup. For FPU-enabled binaries, the build uses `-m68881`.
