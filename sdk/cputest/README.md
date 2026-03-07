# M68K CPU Test Suite

Bare-metal 68020/FPU CPU validation suite. Test cases are generated from a Go catalog, assembled with vasm into a flat binary, and executed directly on the Go M68K emulator via `go test`.

Layout:

- `include/cputest_runtime_bare.inc`: bare-metal runtime (memory-mapped result reporting).
- `include/cputest_manifest.inc`: generated shard table and expected total.
- `generated/`: emitted case bodies from `cmd/gen_m68k_cputest`.
- `cputest_suite_bare.asm`: top-level assembly (includes runtime + manifest + all shards).

Generate the case includes:

```bash
go run ./cmd/gen_m68k_cputest
```

Build the binary:

```bash
make cputest-bin
```

Run the tests:

```bash
go test -tags "headless m68k_test" -v -run TestM68KCPUTestSuite
```

Each of the 437 cases appears as a named subtest. Failures include case name, input description, expected values, and actual values read from the binary's embedded strings.
