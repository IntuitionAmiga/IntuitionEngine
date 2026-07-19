# M68020 JIT Capability Parity Plan

## Framing

The goal is M68020 JIT capability parity with the IE64 JIT, across the amd64,
arm64, and wasm backends, where the capability is semantically valid for the
M68020. This is not a literal port of every IE64 optimisation. Parity means
equivalent capability, not identical implementation.

Two disciplines govern the whole plan:

1. Establish backend correctness before optimisation parity. A new backend
   earns optimisations only after its execution foundation passes differential
   parity.
2. Treat every optimisation as an independent, test-driven, measured slice.
   Retain only measurable wins, except where capability parity itself is the
   stated goal.

The inventory is drawn from the complete live IE64 JIT, not only the
optimisations committed in the last week. The recent Markdown plans are useful
history but are not a complete feature list. The canonical sources are the live
IE64 source and tests (`jit_common.go`, `jit_exec.go`, `jit_helper_dispatch.go`,
the emitters and dispatchers), plus `sdk/docs/IE64_JIT.md`,
`sdk/docs/architecture.md`, and the commit history.

British English throughout. Short technical sentences. No emdash characters.

## Current M68020 JIT state

The live M68020 amd64 JIT is more advanced than `sdk/docs/M68K_JIT.md` currently
suggests. It already has amd64 region formation, register pinning (including
A5/A6), native 68881 support with lazy FPSR, CCR liveness, chaining, static jump
work, SMC range invalidation, an RTS cache, transcendental interpreter
admission, and extensive differential and Harte parity testing. The
documentation is stale in places, particularly around FPU fallback and register
mapping. Correcting that staleness is part of milestone one, not an afterthought.

The M68020 JIT already shares a substantial amount of IE64-era infrastructure.
It uses the shared `CodeBuffer`, `JITBlock`, `CodeCache`, `chainSlot`, the
executable-memory allocator (`ExecMem`), and the native-call machinery. What is
M68020-specific is its block scanner (`m68kScanBlock`), its instruction
representation, its CCR analysis and classification, its emitter, and much of
its runtime and dispatch policy. Parity work is therefore a per-feature
decision: some facilities are already shared and reusable, others are M68020
specific and need backend-specific equivalents. Milestone 1 must record which is
which, and must not assume the M68020 owns infrastructure that is in fact
shared.

## Milestones

The work is a sequence of reviewable units. Do not bundle backend bring-up and
broad optimisation into one implementation step; that becomes unreviewable.

1. Live parity inventory and documentation correction.
2. Shared M68020 frontend preparation.
3. Minimal arm64 backend (correct execution foundation).
4. arm64 capability completion.
5. Minimal wasm backend (integer core).
6. wasm capability completion.
7. Optimisation slices, one at a time, across eligible backends.
8. Final inventory closure and documentation update.

Milestones 3 and 4 must reach solid differential parity before milestone 5
begins. arm64 is the cheaper proof that the frontend and emitter seam is clean;
wasm inherits that cleanliness or fails.

## Milestone 1: Live parity inventory and documentation correction

Produce one compact, source-verified capability matrix comparing the complete
live IE64 JIT against the current M68020 implementation. Rows are
capability-level, not per-opcode, to keep the inventory from becoming a
multi-day stall. Every row cites the live source symbol it was verified against.

Matrix shape:

```
 IE64 capability   IE64    IE64    IE64    IE64      M68020   M68020   M68020   M68020    Decision
 or optimisation   amd64   arm64   wasm    evidence  amd64    arm64    wasm     evidence
 ---------------   -----   -----   -----   --------  ------   ------   ------   --------  --------
 <capability>      status  status  status  <symbol>  status   planned  planned  <symbol>  <decision>
```

Every status row must carry both an IE64 evidence cell and a M68020 evidence
cell, each citing the live source symbol or file it was verified against. A row
without both evidence cells does not satisfy the source-citation requirement,
regardless of the displayed template.

Each M68020 cell and the decision classify the capability as one of:

- already implemented
- missing and applicable (port)
- applicable only to native backends
- applicable only after backend bring-up
- implemented differently because of M68020 semantics (backend-specific
  equivalent required)
- unsafe or irrelevant (rejected by M68020 semantics)
- deferred pending a benchmark

The inventory must cover the whole IE64 JIT, at minimum:

- Long-standing facilities: compilation, code cache, dispatch, invalidation,
  fallback, retired-instruction accounting, memory fast paths, chaining,
  platform ABI handling.
- Earlier optimisation work: register mapping, dirty-state tracking, static
  control-flow handling, region formation, MMIO specialisation, bounds-check
  and constant-address proofs.
- FP handling: 68881 support, FPU residency, condition-code liveness, helper
  boundaries, precise exit materialisation.
- Recent work: loop analysis, observed regions, cold-exit outlining, constant
  folding, invariant hoisting, indirect-target specialisation.
- Backend-specific capabilities already present in IE64 amd64, arm64, or wasm,
  even where another IE64 backend implements a different equivalent.

The rows below are unverified examples, not decisions. They illustrate the
matrix shape and seed the audit. The leaf-fusion correction (already implemented,
not absent) shows why: no row is a requirement until milestone 1 verifies it
against live source. Every row must be confirmed or corrected before any
implementation relies on it.

Unverified example rows:

The completed milestone 1 matrix carries separate IE64 evidence and M68020
evidence, never one overloaded symbol column, because the milestone is a
comparison of two distinct codebases.

| IE64 capability | IE64 evidence | M68020 evidence | Provisional decision |
|---|---|---|---|
| JSR leaf-call fusion | `analyzeJSRLeafFusion`, `isLeafFusionSafe` (`jit_common.go`) | `m68kAnalyzeJSRLeafFusion`, `m68kIsLeafFusionSafe`, `m68kFusedJSRLeafCall`, `m68kFusedRTSLeafReturn`; amd64 emits fused push, inline body, synthetic return, I/O bailout, architectural stack effects | Already implemented (amd64); implement backend lowering on arm64 and wasm |
| Fast-path bitmap probe (RAM versus MMIO) | `emitAMD64FastPathBitmapProbe`, `FastPathBitmapShape` | own bitmap code (not the shared probe) | Check whether M68020 already covers the same fast path |
| Chain-slot ordering policy | `chainSlot`, chain ordering | own chaining | Verify ordering matches; port only the delta |
| Flags-liveness abstraction | `JITFlagLiveness`, `MaterializeFn` | own CCR liveness (`jit_m68k_ccr_liveness.go`) | Different mechanism; confirm coverage, no port |
| Const low-RAM access fast path | `ie64ConstLowRAMAccess` | unknown | Check applicability to M68020 EA modes |
| Helper-exit dispatch model | `handleJITHelper` | has helper exits | Verify resume-model parity |
| High-address bail (guest above 4 GiB) | `jit_high_addr_bail` | absent | Reject: M68020 guest addressing is 32-bit |
| Generation-tagged dispatch cache | `jit_dispatch_cache.go` | absent | Port (optimisation slice) |
| Region formation | `jit_ie64_region_policy.go` | `ScanRegionM68K` (shared), `m68kFormRegion` (amd64-tagged) | Present (amd64); resolve the mislocation, then enable per backend |
| Constant folding | `jit_ie64_const_fold.go` | absent | Port as slice, M68020 CCR proof required |
| Loop-invariant hoisting | `jit_ie64_loop_analysis.go` | absent | Port as slice, alias proof required |
| Cold-exit outlining | `jit_ie64_cold_exit.go` | absent | Port as slice, native backends only |
| Observed regions | `jit_ie64_observed_region.go` | absent | Port as slice |
| FPU residency | `jit_ie64_fpu_residency.go` | present as SSE pinning (amd64) | Different mechanism; backend equivalent per target |
| Dead intermediate FPSR updates | `jit_ie64_fpsr_liveness.go` | present as lazy FPSR (amd64) | Already implemented (amd64); replicate per backend |

Documentation correction in this milestone: fix `sdk/docs/M68K_JIT.md` where it
is stale, especially FPU fallback coverage and register mapping. The matrix
itself lands in the repository as the durable inventory artifact.

Deliverable: the completed matrix, plus a corrected `M68K_JIT.md`. No
implementation.

## Milestone 2: Shared M68020 frontend preparation

Separate three layers cleanly: the M68020 frontend (shared across all M68020
backends), the per-backend emitters, and the per-backend execution and runtime
loops. This is sharing across the M68020 backends only. Do not merge the M68020
and IE64 frontends; they stay separate.

Define the frontend by responsibility, not by a single file, and enumerate every
untagged analysis file it spans. The frontend is the architecture-neutral
analysis: the block scanner, instruction representation, CCR analysis, and
region-formation analysis. It currently lives across at least
`jit_m68k_common.go`, `jit_m68k_ccr_liveness.go` (untagged), and
`jit_region_backends.go` (`ScanRegionM68K`, untagged). Audit all of them for
emitter leakage (`amd64RAX`, `amd64MOV_reg_reg`, `amd64Jcc_rel`, and the rest)
and lift each leaked symbol behind a backend-neutral seam.

Region formation is currently split and mislocated. `ScanRegionM68K` is untagged
shared analysis, but `m68kFormRegion` is inside the amd64-tagged
`jit_m68k_emit_amd64.go`, so region analysis that arm64 and wasm will need is
buried in the amd64 emitter. Explicitly decide, as a milestone 2 deliverable,
whether `m68kFormRegion` becomes shared analysis or receives per-backend
implementations. Do not leave arm64 and wasm dependent on analysis trapped in
the amd64 emitter.

`jit_m68k_exec.go` is not frontend code. It is the amd64 execution loop: it owns
native executable memory, chain patching, native dispatch, region compilation
calls, and amd64 runtime policy. It is legitimate for it to remain
backend-specific. Do not force native and wasm dispatch through one abstraction.

The work is therefore:

- Audit every frontend file identified by the responsibility-based inventory
  for emitter leakage and remove it, not `jit_m68k_common.go` alone. The known
  set is `jit_m68k_common.go`, `jit_m68k_ccr_liveness.go`, and
  `jit_region_backends.go`, plus any further untagged analysis file the
  inventory discovers.
- Identify the backend-neutral dispatch and tier policy worth extracting from
  `jit_m68k_exec.go` into a shared helper, and leave the native-specific
  execution mechanics where they are.
- Create separate amd64, arm64, and wasm execution and runtime files, each
  owning its own dispatch mechanics.
- Introduce arm64 and wasm build-tag variants of `jit_m68k_dispatch` and
  `jit_m68k_dispatch_stub` so `m68kJitAvailable` can be set per target without
  disturbing the amd64 path.

Deliverable: the leaked-symbol audit and its refactor, a documented
frontend/emitter/execution boundary, and the amd64 backend still green on the
full M68020 suite.

## Milestone 3: Minimal arm64 backend

A correct execution foundation before any optimisation. Implement, in order,
each gated by differential parity against the interpreter and the relevant Harte
subset:

- ABI and context layout (`jit_m68k_abi_arm64.go`), register mapping defined
  once and never re-derived inline. Reserve X18 (platform register on Apple and
  Windows arm64, never usable) and X16/X17 for emitter scratch. Learn from the
  IE64 arm64 X18 bug.
- Instruction lowering for the integer core: MOVE and all EA modes, the ALU
  group, shifts, rotates, MOVEQ, LEA, PEA, immediates.
- Big-endian memory paths. The M68020 guest is big-endian; the arm64 host is
  little-endian. Every guest access needs the correct byte order.
- CCR and X semantics, full materialisation, no liveness elision yet.
- Exceptions and interpreter fallback: TRAP, TRAPV, CHK, illegal, Line-A,
  Line-F, and 68020 exception behaviour, with per-instruction resume PC.
- SMC detection and invalidation.
- Exact retired-instruction accounting. Run the M68020 differential accounting
  harness from day one. Count accounting had subtle bugs on amd64, fixed only by
  differential matching to interpreter event counts. It is expensive to retrofit
  and cheap to build in now.
- Block chaining and interrupt boundaries, with the status register published to
  successors exactly as amd64 does.
- 68881 support, or an explicitly staged fallback with the fallback boundary
  documented.

Gate: the integer and control-flow differential grids green on arm64, plus the
Harte exception cases. Register residency and control-flow changes are gated by
AROS boot on real arm64 hardware, never on qemu.

## Milestone 4: arm64 capability completion

Once differential parity is solid, add the M68020-appropriate optimisations one
slice at a time, per milestone 7, on arm64. CCR liveness and fusion port here,
mapped to A64 NZCV, preserving the amd64 rule that only NZ-safe conditions may
drop V and C. Benchmarks are measured on the x13s, never on qemu.

## Milestone 5: Minimal wasm backend

Treat wasm as a distinct M68020 bytecode backend with its own emitter. The
M68020 needs its own emitter because of variable-length decoding, big-endian
accesses, effective-address side effects, CCR and X behaviour, alignment and
bus-error semantics, Line-A, Line-F, and 68020 exception behaviour, and 68881
state.

Runtime boundary prerequisite. Only the encoder and the lower-level
module-building machinery are genuinely generic today. `jit_wasm_runtime.go` and
`jit_exec_wasm.go` and the IE64 chain driver are currently tied directly to
`CPU64`, `JITContext`, 64-bit PCs, IE64 helper handling, IE64 timers, MMU
gating, observed-region state, and IE64 accounting. They are not reusable as-is.
Before milestone 5 codegen begins, make an explicit choice and record it:

- extract carefully bounded generic wasm primitives (module cache, asynchronous
  installation, function table, chain driver) that both IE64 and M68020 can
  consume; or
- implement an M68020-specific wasm runtime and reuse only the encoder and
  module-building patterns.

Do not bend the IE64 runtime into a multi-ISA abstraction by accident. Without
this decision recorded first, milestone 5 substantially understates the backend
work and risks an invasive, unplanned multi-ISA runtime refactor.

wasm has no flags register and structured control flow only. Model the CCR in
linear memory or locals and materialise on exit. Translate each block to its own
wasm function that returns a next-PC to the chain driver.

Target order:

- Integer blocks with conservative interpreter fallback.
- Branches and loops (in-block loops as structured wasm loops where the frontend
  proves the backward branch stays in-block, otherwise exit to the driver).
- Memory forms, including big-endian access and EA side effects.
- Regions.
- 68881 operations that map cleanly to wasm numeric instructions. The 68881 is
  80-bit extended; wasm has only f32 and f64. Only the clean-mapping subset
  ports. Extended and packed formats and transcendentals stay interpreter
  fallback, the same split the amd64 native EA work draws.

Gate: the integer and control-flow differential grids green under wazero,
mirroring `make test-wasm-node`, plus a browser smoke run. Adopt the wasm REPL
latency lessons: rAF-aligned cooperative yield, paint-first resume, hidden-tab
fallback, cadence tuned for latency because headless wasm fps is not trustworthy.

## Milestone 6: wasm capability completion

Add the wasm-relevant, M68020-appropriate optimisations one slice at a time, per
milestone 7. CCR liveness elision is high value here because the CCR is
memory-modelled. Region promotion and observed regions are deferred unless a
browser workload measures a need. Document every deferral explicitly; a silent
cap reads as full coverage.

## Milestone 7: Optimisation slices

Each optimisation is an independent slice with an M68020-specific proof. IE64's
optimisation analysis may provide a useful pattern, but its `JITInstr` analysis
must not automatically become a universal compiler IR. The M68020 needs its own
proofs.

Candidate slices, subject to the milestone 1 matrix:

- constant-only folding
- conservative loop-invariant instruction hoisting
- invariant memory-check hoisting
- bounded counter-loop budget removal
- observed hot-path region formation
- adjacent forward cold-exit outlining, native backends only
- region register residency
- dirty-only spills
- static jump chasing
- constant-address RAM and MMIO proofs
- monomorphic indirect-target specialisation
- FPU residency and dead intermediate FPSR updates
- generation-tagged direct dispatch cache

JSR leaf-call fusion is not an optimisation slice here. It already exists on
amd64; its remaining work is backend lowering on arm64 and wasm, handled under
the backend-capability gate in milestones 4 and 6, not designed afresh.

Two distinct TDD gates, because backend correctness and optimisation have
different acceptance criteria. Correctness features (exception delivery,
big-endian access, context layout, fallback, retired accounting) do not
necessarily have a meaningful optimisation shape or a performance acceptance
test, so they must not be forced through a benchmark gate.

Backend-capability gate (milestones 3 to 6):

1. Add a failing behavioural or differential test for the capability.
2. Implement the smallest slice.
3. Prove differential parity against the interpreter is green.
4. Run focused opcode, exception, accounting, SMC, and boundary and integration
   tests.
5. No benchmark acceptance; capability correctness is the goal.

Optimisation gate (milestone 7):

1. Add a failing optimisation-shape or instrumentation test.
2. Add or identify interpreter-versus-JIT differential coverage.
3. Prove the shape test fails while parity remains green.
4. Implement the smallest slice.
5. Run focused opcode, exception, accounting, SMC, and boundary tests.
6. Benchmark after correctness passes.
7. Retain only measurable wins, except where capability parity itself is the
   stated goal.

M68020 differential parity must compare registers, SR and CCR, PC, memory, stack
effects, FPU registers, FPCR, FPSR, FPIAR, exceptions, interrupt boundaries, and
retired counts. Harte cases remain a mandatory native gate because they
previously caught incorrect CCR reasoning.

## Explicitly excluded or hazardous

Not parity targets:

- Cross-block CCR liveness in its earlier form. M68020 CCR is observable at
  interrupt and exception boundaries, so a successor overwriting CCR does not
  make the predecessor's boundary value dead. Within-block elimination remains
  sound only when no observation point intervenes. The earlier cross-block
  attempt was abandoned as unsound and must not be resurrected.
- IE64-specific direct operand comparisons.
- Fixed-width decoding assumptions (the M68020 is variable-length).
- MMU proofs tied to IE64 addressing.
- Register-allocation rules based on IE64's larger uniform register file.
- High-address bail (IE64 guest above 4 GiB); the M68020 guest is 32-bit.

Correctness hazards any new backend inherits, to be understood before the
relevant step, and never misdiagnosed as a new-backend regression:

- High-RAM PC-ceiling latent bug. PC-ceiling removal was reverted because a
  native high-RAM bug faults on an odd PC during Delete-task in high RAM. arm64
  and wasm bring-up must not re-expose it, and the latent bug must be understood
  before either backend removes the ceiling.
- WBDock async-reschedule race. This is engine-independent and is not the JIT.
  New backends must not be blamed for it when AROS boot hiccups.

## Milestone 8: Final inventory closure and documentation update

Close the milestone 1 matrix: every row resolved to implemented, rejected with a
reason, or deferred with a recorded benchmark result. Update the documentation
surfaces to match the delivered state without overstating coverage:

- `sdk/docs/M68K_JIT.md`
- `sdk/docs/architecture.md` (JIT coverage and build-tag tables)
- `sdk/docs/wasm.md`

No parity claim beyond what is measured, and no claim of arm64 or wasm parity
until each is green on its differential and Harte gates and, for register and
control-flow work, on real-hardware AROS boot.
