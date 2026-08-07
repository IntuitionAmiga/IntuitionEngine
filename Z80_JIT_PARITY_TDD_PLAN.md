# Z80 JIT cross-backend parity programme

## Summary

Bring the Z80 JIT to full `CPU_Z80` instruction-contract coverage on amd64,
Linux arm64 and js/wasm. All three backends must directly compile every opcode
form that can execute without required host observation; a canonical helper is
permitted only at a genuine observation boundary. Keep the JIT enabled whenever its backend is available;
`--nojit` maps to the runner's `DisableJIT` start-up opt-out. Preserve the
existing stopped-CPU IEScript diagnostic override, which may enable or disable
an available JIT but rejects changes while execution is active. Preserve
interpreter execution only at defined observation boundaries, with exact restart
state and no premature guest side effect.

## Investigation and TDD foundation

- Create an amd64 feature inventory against IE64, M68K, x86 and 6502 JIT
  techniques. Record each item as adopted, inapplicable, or deliberately
  rejected with an evidence-based reason.
- Add a generated or table-driven opcode manifest covering every `CPU_Z80`
  decoded form, including supported undocumented forms. Each row names its
  amd64, arm64 and wasm outcome as exactly one of direct emitted execution,
  canonical helper exit at a required host-observation boundary, or explicit
  halt, plus its proving differential test.
  A coverage test must prove that the declared outcome occurred on every
  available backend; interpreter bailout is not opcode coverage. An explicit
  halt is permitted only where `CPU_Z80` itself halts, and its test compares PC,
  cycles, R, IFF/interrupt behaviour and stop state. Every other
  interpreter-supported form is direct or a canonical helper exit.
- Add an interpreter-dispatch inventory test so any unclassified opcode fails
  CI. For every opcode family, write failing interpreter-versus-JIT tests before
  lowering it.
- Make the interpreter the initial oracle. When differential and conformance
  evidence proves it wrong, fix it first with a focused regression, then update
  all JIT paths and replace only source comments proved inaccurate by that
  evidence.

## Shared execution contract

- Refactor the scanner, admission policy, cycle and R-register accounting, flag
  liveness, direct-page/MMIO checks, SMC invalidation, chaining policy,
  diagnostics and opcode manifest into untagged Z80 JIT frontend code.
- Use one MachineBus-owned physical code-page generation publisher and Z80
  subscriber registry for every backend. Every guest, debugger, loader, DMA and
  host write route publishes the written physical range after mutation; each
  owning Z80 CPU has its own pending-invalidation queue and alone drains it,
  mutates its cache, unpatches chains and clears return-target entries. Every
  block and promoted region retains immutable physical code spans, source-byte
  stamps and page-generation snapshots. Cache invalidation and entry validation
  use those physical spans, and every patched chain exit validates its dispatch
  generation before jumping.
- Maintain a CPU-owned mapping generation distinct from physical code-page
  generations. Every bank-window and VRAM-bank register write publishes a
  mapping change before execution resumes. A compiled block or region records
  the mapping generation used to resolve its logical code range; dispatch and
  every patched chain edge reject a mismatch before executing stale logical-PC
  code.
- Retain and complete existing amd64 capabilities: pinned-register block
  execution, lazy partial flag materialisation, direct-memory guards, bounded
  chaining, return-target cache, DJNZ-loop specialisation and fast MMIO polling
  where sound.
- Add only narrow optimisations suited to Z80 semantics: hot-block
  recompilation if measurement proves it useful, bounded static-chain region
  promotion, and structured wasm loops. Move region scanning and policy to the
  untagged frontend so all three backends form the same regions. Region
  formation follows only static JP/JR edges, is limited to four blocks and 128
  total instructions, and uses the existing 64-block chain and 200-cycle
  interrupt bounds. It retains the existing loop precheck and flag-liveness
  rules per constituent block, and exits to the dispatcher for any unsupported,
  conditional, indirect, helper or observation boundary. It must preserve code
  and mapping generation validation, SMC unpatching, interrupt bounds and
  aggregate cycle, instruction and R-register accounting. Exclude
  IE64-specific MMU, FPU, SIMD and general optimiser machinery.
- Define one explicit exit contract for normal completion, helper execution,
  unsupported mapping, interrupt, debug, SMC and cache pressure: preserve PC,
  registers, memory effects, flags, cycles, instruction count, R increments and
  chain accounting exactly.
- Define a backend-neutral canonical helper ABI before admitting any helper
  row. Its context contains an exit reason, instruction-start PC, decoded
  prefix and opcode, operand and displacement, instruction length, resume PC,
  and retired-count, cycle and R-increment state. The helper consumes that
  immutable payload, publishes architectural state exactly once, and never
  re-decodes mutable guest bytes. Native offset and wasm32 context-image tests
  guard layout, publication and reset semantics.

## Backend completion

- Replace the Linux arm64 stub with a real emitter and execution path using a
  tested context ABI, callee-saved guest-register mapping, executable-memory
  lifecycle, instruction-cache flushing, direct-memory and SMC guards, bounded
  chaining and equivalent exit semantics. Make `z80JitAvailable` true only
  after this path executes real guest instructions.
- Add a js/wasm Z80 backend with a fixed wasm32 context image, module/cache
  lifecycle, guarded Go-memory access, source-byte and physical-generation
  validation, structured bounded dispatch and a `Z80_WASM_JIT=0` kill switch.
  It activates by default only when required wasm capabilities are present.
- Make backend provenance observable in tests and CPU-owned `cpu.jit_stats()`
  diagnostics. Extend IEScript documentation and tests to report native/wasm
  entries, helper exits, bailouts, invalidations, chained exits and reset
  behaviour.
- Preserve amd64 support on its existing supported hosts; add Linux arm64 and
  browser availability to constructors, runners, program executor, coprocessor
  worker and IEScript control paths. Test that every construction and launch
  path is default-on, `--nojit` selects interpreter start-up, and IEScript may
  change the state only while the selected CPU is stopped.

## Verification, fixtures and documentation

- Differentially compare complete CPU state, memory, cycles, stop state and
  bailout boundaries for every manifest row, including prefixes, shadow
  registers, IX/IY, I/R, IFF and interrupt modes.
- Add focused proofs for I/O helpers, block I/O, DAA/RLD/RRD, indexed and
  DDCB/FDCB forms, repeat instructions, EI delay, IRQ/NMI, HALT, bank windows,
  MMIO, page crossings, SMC, code-cache invalidation, chain-budget exhaustion
  and concurrent CPU ownership. Exercise every physical mutation route while
  native chaining and promoted regions are live, including writes spanning
  code-page boundaries and stale chained-target attempts. Cover bank-window and
  VRAM remaps that replace the physical bytes behind a logical PC without a RAM
  write, including a stale chained target and a promoted region. Prove every
  canonical helper row consumes its decoded payload without re-reading guest
  instruction bytes.
- Run amd64 tests natively. Run Linux arm64 correctness under QEMU with
  `GODEBUG=asyncpreemptoff=1`, requiring an executed arm64-emitted-block marker;
  do not collect or claim QEMU performance. Run wasm unit tests under wazero,
  js/wasm integration under Node, and a browser module-instantiation and
  memory-access check.
- Turn the committed Z80 rotozoomer and Robocop binaries into deterministic
  interpreter/JIT shadow-parity gates: fixed fixture hashes, program and asset
  roots, two fresh identically configured machines, headless deterministic
  scheduling, disabled audio, fixed retired-instruction and frame-boundary
  checkpoints, CPU/memory/cycle/device/framebuffer comparisons, known
  wait/completion conditions and an instruction watchdog. Use IEScript to load,
  configure stopped CPUs, collect diagnostics and artefacts; use IEMon to
  reduce failures, then preserve each reduction as a Go regression.
- Keep each existing amd64 benchmark median at least non-regressing. Admit new
  amd64 optimisations only with repeatable positive measurements; report arm64
  and wasm capability results without performance claims.
- Update `z80_jit.md`, canonical `sdk/docs/architecture.md`,
  `sdk/docs/iescript.md` and `sdk/docs/iemon.md`, plus affected source comments.
  Replace a source comment only when implementation or test evidence proves it
  inaccurate. Use British English, factual platform statements and no em
  dashes. Add documentation assertions rejecting stale claims that arm64 is a
  stub or that browser Z80 is interpreter-only after implementation.

## Assumptions

- Full coverage means every instruction form implemented by `CPU_Z80`, including
  supported undocumented forms. Every backend directly emits every form that
  does not require host observation; helper exits are valid only for required
  host observation.
- The existing amd64 JIT is a capability baseline, not proof of correctness. Its
  current documented fallback classifications must be revalidated by the
  manifest.
