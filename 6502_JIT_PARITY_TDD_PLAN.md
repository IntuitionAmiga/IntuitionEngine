# Linux 6502 JIT parity programme

## Summary

Bring the 6502 JIT to full documented NMOS 6502 opcode coverage on Linux AMD64, Linux ARM64 and js/wasm. The current interpreter is the initial parity reference, but a proven interpreter defect is corrected before JIT parity is established. QEMU validates Linux ARM64 correctness only; performance evidence is collected separately on Linux AMD64, physical Linux ARM64 and browsers.

## Coverage and frontend

- Support native 6502 JIT only on Linux AMD64 and Linux ARM64. Update 6502-specific build tags, availability tests and dispatch stubs so Windows and macOS retain interpreter support only.
- Create an untagged frontend for opcode inventory, scanning, admission, direct-page/MMIO analysis, cycle metadata, N/Z liveness, physical SMC generation checks and chain policy.
- Add two coverage guards:
  - A manifest with representative bytes, addressing form, backend path and named proving test for each documented opcode.
  - An interpreter dispatch-inventory test requiring an explicit decision for each opcode-table entry.
- Classify official NMOS instructions as `direct`, undocumented instructions as `interpreter-fallback`, and JAM/KIL as `halt`. Every `direct` form must compile and execute native semantics on Linux AMD64, Linux ARM64 and wasm in eligible direct RAM.
- Permit scanning and caching only for complete instruction ranges in mapping-stable plain RAM. Bank windows, VRAM windows, I/O pages and mapping boundaries are interpreter fetch boundaries.
- Keep dynamic interpreter fallback for MMIO, bank-window, visible-memory, debug and fault observation points. It restores the faulting PC and performs no guest side effect before interpreter re-execution.
- Pass immutable CPU-owned compilation input to each backend. Keep contexts, caches, statistics and invalidation state CPU-owned; add concurrent multi-CPU compilation and `-race` tests.

## Backends and validity contract

- Complete Linux AMD64 lowering for BRK, RTI, decimal ADC/SBC and all remaining documented forms.
- Implement BRK's native vector access through a dedicated `$FFFE/$FFFF` path. It bypasses broad I/O-page rejection only for the defined vector bytes and preserves PC advance, B/U pushes, interrupt-mask update, vector read and cycle behaviour.
- Add Linux ARM64 lowering and dispatch with a matching context ABI, executable-memory lifecycle, instruction-cache flush, pinned guest registers, bounded chaining, return-target cache and equivalent exit semantics.
- Define separate emitted ABI layouts:
  - Linux AMD64 and Linux ARM64 use the native `JIT6502Context` layout, guarded by `unsafe.Offsetof` tests.
  - wasm uses a fixed wasm32 context-image offset table, with independent field-publication, reset and return-state tests.
- Add wasm32 module caching, guarded direct-page lowering, structured tight loops and cooperative dispatch. `P65_WASM_JIT=0` disables it. Activation requires standard WebAssembly and exposed Go linear memory; unavailable capability disables the complete wasm JIT.
- Compile only plain-RAM code and record each block's physical MachineBus code pages, source bytes and generation snapshots.
- Route all physical mutators through one generation service. This covers every `Write8`, `Write16`, `Write32`, `Write64`, fault-aware and direct-memory write path, plus interpreter stores, native stores, other CPUs, debugger writes, DMA and program loads.
- Audit every writable alias of MachineBus backing memory. Migrate 6502-relevant host, loader, debugger, DMA and device writes to generation-aware APIs, or wrap exposed writable views so writes mark their physical range. Untracked raw backing-slice writes are forbidden while a 6502 JIT may execute; read-only views remain permitted.
- External writers atomically mark physical pages and enqueue invalidations. Only the owning 6502 execution thread drains them, mutates the cache, unpatches chains and clears RTS-cache entries.
- Entry paths validate source bytes and page generations. Patched chain exits compare atomic dispatch generation before jumping; a mismatch returns to the dispatcher. Validate target liveness and generation compatibility immediately before patching a chain.
- Port pinned guest registers, deferred N/Z materialisation, direct-page/MMIO guards, cycle batching, bounded chaining, RTS cache, fast MMIO-poll recognition and SMC-aware cache reset. Adapt dispatch caching and structured wasm loops. Permanently exclude SIMD, MMU or micro-TLB work, FPU work, regions and region promotion.

## TDD and acceptance

- Write failing manifest, admission, emitter and interpreter-differential tests before each opcode family.
- Compare A/X/Y/SP/SR/PC, stack, memory, cycles, vectors, stop state and bailout boundaries for every row.
- Use the current interpreter as the initial differential reference. When Klaus, decimal/interrupt conformance tests or a source-backed NMOS rule prove an interpreter defect, fix the interpreter first, add its regression, then update all JIT backends to the corrected contract.
- Cover NMOS indirect-JMP page-boundary behaviour, BRK PC advance and B/U pushes, RTI status restoration, decimal ADC/SBC flags, stack wraparound, `$FFFA-$FFFF` vectors, page crossings, IRQ/NMI, RDY, reset, break-in, MMIO, banks, SMC and reduced chain budgets.
- Test every generation-service write route, including `Write16`, `Write32` and `Write64` mutations that cross physical code-page boundaries, stale chained-target scenarios and writes made through every approved mutable backing-memory view.
- Add a static audit test for known direct backing-memory write sites and runtime tests that alter code through each approved route while native chaining is active.
- Run Linux AMD64 tests natively. Run named Linux ARM64 QEMU tests that assert ARM64 `jit6502Available`, compile and execute an ARM64 native block, and verify an ARM64 emitted-block marker or provenance counter. The gate must fail if its test inventory is empty or only interpreter execution occurred.
- Run wasm module tests under wazero; js/wasm dispatcher tests under Node; and browser module-instantiation and Go-memory-access tests.
- Run the Klaus functional, decimal and interrupt binaries with interpreter and JIT controls.
- Use only committed prebuilt rotozoomer and Robocop fixtures. Assert SHA-256 values for both binaries and required rendering assets; update hashes only in the reviewed fixture-update commit.
- Make both binaries deterministic interpreter/JIT shadow-parity gates: identical binaries and asset roots, fixed retired-instruction checkpoints, CPU/memory/cycle/device/framebuffer comparisons, completion or known wait-loop condition, and watchdog.
- Use IEScript for launch, stopped-CPU JIT configuration, diagnostics and failure artefacts. Use IEMon only to reduce a failure, then preserve it as a deterministic Go regression.
- Add CPU-owned 6502 `cpu.jit_stats()` output with documented fields, reset semantics and IEScript tests. Do not expose process-global counters.
- Retain optional Linux AMD64 optimisations only after semantic parity and statistically positive measurements. Measure Linux ARM64 performance only on physical hardware after QEMU correctness passes.

## Documentation and assumptions

- Correct inaccurate source comments in relevant 6502 JIT, backend, test, build and automation paths.
- Update `sdk/docs/6502_JIT.md`, canonical `sdk/docs/architecture.md`, `sdk/docs/iescript.md` and `AGENTS.md` to state Linux AMD64, Linux ARM64 and wasm support accurately.
- Add a documentation audit that rejects Windows/macOS 6502-JIT availability claims while retaining interpreter-support statements.
- Use British English, source-backed statements and no unsupported performance claims.
- Scope is official NMOS 6502 instructions only. Undocumented instructions remain explicit fallbacks and JAM/KIL remains a halt path.
- The required end-to-end gates are the committed 6502 rotozoomer and Robocop binaries.
