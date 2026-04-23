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

For M68K AROS interpreter closure work, treat this suite as one layer in a four-layer TDD loop:

- targeted Go unit tests for local decode/EA/exception fixes
- generated `cmd/gen_m68k_cputest` cases for widened 68020 semantic coverage
- IEScript debugger triage via `scripts/m68k_aros_ready_probe.ies` and `scripts/m68k_aros_fault_capture.ies`
- bounded `-aros -nojit` boot acceptance using `AROSBootHarness`, `ProbeAROSReadyState`, and `M68KFaultManifest`

When a new bounded-boot fault signature appears, add the narrow regression first, then extend the generated catalog if the failure represents a reusable opcode/addressing-mode form.
