# IE32 JIT

The IE32 JIT is available on Linux x64, Linux ARM64, and js/wasm builds when
the required runtime backend is available. It is selected by default when an
IE32 CPU is constructed. `--nojit` selects the interpreter for startup; a
stopped CPU can be changed with `cpu.set_jit_enabled()` for diagnostics.

The JIT and interpreter share the IE32 instruction decoder. Generated blocks
must leave timer, interrupt, MMIO, debugger, fault, and cache-invalidation
boundaries architecturally observable. A build that lacks a usable runtime
backend reports `backend = "none"` through `cpu.jit_stats()` and uses the
interpreter.

Native and wasm emitters use one CPU-context ABI. Build-time assertions keep
all emitted word and pointer offsets aligned and within native displacement
range; wasm builds additionally require the wasm32 pointer layout.

`cpu.jit_stats()` reports the selected backend, generated-code entries,
compiled blocks and static-jump-chased regions, direct and helper instructions,
generated-block chain links,
chain-budget exits, deoptimisations by helper boundary and source-stamp reason,
retired instructions, invalidations,
cache hits, MMIO poll iterations, and MMIO poll parks. The counters reset
on reset and program reload. An entry represents actual generated-code entry,
not compilation attempts or interpreter fallback. A compiled block represents
an opcode block lowered by the selected backend; it does not include the
dispatch marker used to establish native-entry provenance.
`cache_hits` counts execution of retained pure native blocks and reuse of a
compiled and instantiated wasm block. The wasm cache is content-keyed and
bounded; its block entrypoint takes the CPU address, so it is reusable across
CPUs sharing the Go linear memory. Native invalidation and reset clear native
retained blocks.
After a direct `RTS` or `RTI`, a two-entry native return cache can enter a
validated retained block without a general dispatch-cache lookup.
Short register-only `JSR` leaves ending in `RTS` may be fused into the caller.
The fused sequence retires the call and return but does not write the guest
stack; memory, division, interrupt, and control-flow leaves remain unfused.
The first compilation of a static jump remains a compact block. A second
uncached execution recompiles it as a bounded region and increments
`hot_recompilations`.

The currently direct-lowered subset is NOP; immediate, register, eligible
direct-RAM, and first-instruction guarded register-indirect loads; immediate,
register, eligible direct-RAM, and first-instruction guarded register-indirect
integer ALU operations; eligible
direct-RAM and first-instruction guarded register- or memory-indirect stores;
named register loads and stores; immediate integer ALU
operations; immediate and first-instruction guarded register-count shifts with
counts below 32; guarded non-zero register, register-indirect, or direct-RAM
DIV and MOD; register and first-instruction guarded indirect-memory INC and
DEC; eligible PUSH and POP operations; eligible
JSR and RTS operations; and unconditional and conditional branches. Interrupt
control and RTI use canonical one-instruction helpers. Direct control-flow targets can form a
bounded chain of generated blocks; an unsupported form, observation boundary,
or the chain budget resumes through the architectural dispatcher.
Backward conditional loops use the same fixed chain budget rather than an
unbounded native loop. This keeps retirement, timer, debugger, interrupt, and
cooperative-yield boundaries visible between bounded groups of iterations.
For an immediate-register `SUB counter,#1; JNZ counter,head` loop with a
statically bounded initial count and no observation boundary, the native and
wasm backends emit one bounded generated loop and return its exact dynamic
retired count. MMIO, helper, timer, debugger, and unknown-count loops retain
dispatcher boundaries.
The admitted loop body may also use direct RAM loads and stores after one
active-visible-RAM, MMIO, VRAM, and self-modifying-code guard. Repeated direct
stores publish their exact physical range when the generated loop exits.
Previously proven register-only fused JSR leaves are also admitted: their JSR,
leaf body, and synthetic RTS retain individual retirement counts while the
guest stack remains untouched.
`counted_loops` records each generated bounded-loop entry.
Bounded static forward `JMP` targets may also be chased into one generated
region. Its source stamp and invalidation ranges cover every emitted segment,
but deliberately exclude skipped bytes.
Reserved addressing bytes retain the ISA contract: read-style operands resolve
to zero and store-style operands use `operand32` as a direct destination.
Direct RAM accesses require an aligned address inside active visible RAM.
MMIO, direct VRAM leases, debug observation, unsupported addressing modes,
and faults resume through the interpreter. When the timer is active, eligible
forms execute in one-instruction generated blocks with the shared timer
transition before each instruction, preserving expiry and interrupt ordering.
Active access instrumentation uses interpreter single steps without changing
the selected JIT policy. Clearing the instrument resumes JIT routing at the
next instruction boundary.
With no timer, debugger, or bounded test checkpoint, a direct-MMIO `LOAD`
followed by a same-register backward `JZ` or `JNZ` is recognised as a bounded
poll loop. Each pair still executes through the canonical helper path, so MMIO
side effects remain observable; `mmio_poll_iterations` records the work. If a
complete `VIDEO_STATUS` VBlank batch observes no state change, the dispatcher
parks on the video device's next rising edge and resumes JIT execution at the
unchanged guest loop. This prevents a guest waiting for VBlank from repeatedly
paying polling overhead or abandoning the generated execution path.
`mmio_poll_parks` records these bounded device waits.

The shared frontend performs one conservative peephole: an immediate load
overwritten by a later same-register load in a contiguous run of immediate
loads may omit the first generated store. It preserves both retired-instruction
counting and final PC, and is disabled by timer and debug boundaries.
Backward register liveness extends that elision across register-only work when
the original value is proven unread before overwrite; every block exit and
unknown form remains conservatively live.
Two or more contiguous same-register immediate `ADD`, `SUB`, `MUL`, `AND`,
`OR`, or `XOR` operations retain that guest value in one host register and
perform a single dirty spill at the end of the run. Timer, debugger, helper,
control-flow, and memory boundaries end the run.
Within a static-jump-chased region, its unobservable forward `JMP` edge does
not end the run, so the same residency also crosses compact region segments.
`resident_spills_saved` records only spills avoided by generated executions,
including retained native-cache entries.
When such a run is immediately followed by a conditional branch on the same
register, native and wasm code compare the resident value directly while still
spilling the final architectural value before the branch exit.
Immediate pointer-register loads propagate through a generated block. A later
register-indirect operand with that unmodified base is specialised to its
constant direct-RAM address, subject to the ordinary RAM and MMIO admission
checks.
An immediate load immediately followed by a conditional branch on that same
register can also use the known branch outcome, while retaining both guest
instruction retirements and the loaded register value.
It also folds an immediate load followed by a same-register immediate integer
ALU operation, including `NOT`, into the final immediate load under those same
boundaries.

Generated IE32 code is invalidated through physical RAM-write generation
publication. Bus, CPU, and generated direct-RAM writers do not mutate a CPU
cache directly; the owning IE32 dispatcher drains the generation at an
execution boundary and removes only retained blocks whose source range overlaps
the published direct destination. Unresolved register- and memory-indirect
generated destinations conservatively remove every retained block. A direct
store that would overwrite a later instruction in
its current block instead resumes before the store, so it cannot execute stale
compiled instructions.
Retained native blocks also carry deterministic source-byte stamps. A stamp
mismatch rejects the cache entry even if an integration path failed to publish
a RAM-write generation; publication remains mandatory for prompt cross-CPU
invalidation.
If a native executable arena is exhausted, the dispatcher clears retained
blocks, resets the arena, and retries the candidate block once. The
`code_cache_resets` counter records successful recovery attempts.

Run `make test-ie32-jit-parity` for the non-vacuous x64, ARM64/QEMU, js/wasm
Node, and Chromium gate. Chromium both executes representative emitted modules
and compiles a Go js/wasm test binary which times the actual `CPU.Execute()`
interpreter and wasm-JIT routes. The browser harness takes five paired samples
for ALU, RAM, mixed, and call-heavy workloads, compares medians against the
5 percent budget, and requires the JIT to win at least three pairs. ARM64 is a
correctness target only. The gate includes `test-ie32-jit-race`, which
exercises IE32 generation publication, SMC, cache invalidation, and loader
invalidation under the Go race detector on Linux x64.

The rotozoomer and RoboCop fixture-shadow tests run each binary in an isolated
machine with an attached deterministic video device. At the instruction
checkpoint they compare CPU and RAM state, the serialised video-device state,
and the framebuffer hash between interpreter and generated execution.

Linux x64 performance comparisons use the matched `BenchmarkIE32_*_Interpreter`
and `BenchmarkIE32_*_JIT` benchmark pairs for ALU, RAM, mixed, and call-heavy
workloads. Run each pair repeatedly on the same host and compare medians.
The equivalent wasm evidence comes from the Node benchmark command and the
paired Chromium harness; neither uses QEMU timing.

## Three-backend technique ledger

This ledger is intentionally target-neutral. `TestIE32JIT_*` contract tests
run on x64, Linux ARM64 under QEMU, and js/wasm Node through
`make test-ie32-jit-parity`; generated-entry counters distinguish execution
from compilation.

| Technique | IE32 outcome | Proven by |
| --- | --- | --- |
| Opcode-form direct/helper/halt ledger | Direct emission where no observation is needed; one-instruction helpers for timing, interrupts and debugging | `TestIE32JITDirectManifestFormsMatchStepOne`, `TestIE32JITManifestKeepsObservationOpcodesAtHelperBoundary` |
| Shared frontend and ABI | Shared decoded form, liveness, source stamps, native offset assertions and wasm32 layout | `TestIE32JITFormLedgerClassifiesEveryOpcodeAndAddressingByte`, `TestIE32JITContextABIHasStableWordAlignment` |
| Guarded RAM and precise SMC | Active-RAM/MMIO/VRAM guards, physical generation publication and source stamps | `TestIE32JITDirectRAMAdmissionUsesActiveVisibleCeiling`, `TestIE32JIT_SourceStampRejectsUnpublishedCodeWrite` |
| Cache, reclamation and direct dispatch | Bounded native and wasm execution caches, stamp rejection and executable-arena recovery | `TestIE32JIT_PureBlockCacheReusesGeneratedCode`, `TestIE32JIT_ExhaustedCodeCacheResetsAndRetries` |
| Chains and return routing | Bounded chains plus native two-entry return cache; wasm uses its reusable entry cache because it has no native return address | `TestIE32JIT_ChainBudgetStopsAtExactRetirementBoundary`, `TestIE32JIT_ReturnCacheHitsAfterRTS` |
| Static regions and hot recompilation | Bounded forward-jump region promotion with discontiguous source ranges | `TestIE32JIT_StaticForwardJumpRegion`, `TestIE32JIT_StaticRegionTracksDiscontiguousSources` |
| Register/value optimisation | Liveness, dead-load removal, resident immediate ALU, constant folding, branch fusion and constant-address specialisation | `TestIE32JIT_ResidentImmediateALURunPreservesArchitecturalState`, `TestIE32JIT_ImmediateConstantFoldPreservesRetirement`, `TestIE32JIT_ConstantPointerSpecialisesLaterIndirectLoad` |
| Calls and loops | Safe leaf-call fusion, bounded counted loops, exact retirement and guarded direct-memory loop writes | `TestIE32JIT_FusedLeafCountedLoopReturnsExactRetirement`, `TestIE32JIT_GuardedRAMCountedLoopPublishesExactWriteRange` |
| Observation and yield boundaries | Timer, interrupt, WAIT, debugger trap-loop, MMIO polling and cooperative yield remain observable. An unchanged bounded VBlank poll parks on the video edge and resumes JIT execution. | `TestIE32JIT_TimerExpiryMatchesInterpreterAtEachBlockPosition`, `TestIE32JIT_WatchpointUsesStepThenResumesGeneratedRouting`, `TestIE32JIT_AcceleratesMMIOPollLoop`, `TestIE32JIT_ParksOnExhaustedVBlankPollAndResumesJIT` |
| Wasm timing evidence | Five paired interpreter/JIT browser samples per workload, median regression budget, and generated-entry provenance | `TestIE32WasmBrowser_PairedPerformanceHarness`, `TestIE32WasmBrowserPairedPerformance` |
| FPU/FPSR and MMU techniques | Structurally inapplicable: IE32 has neither an FPU/FPSR nor address translation | [IE32 ISA](IE32_ISA.md) |

The ISA contract is defined by [IE32_ISA.md](IE32_ISA.md). VM lifecycle,
memory, and MMIO integration are defined by [architecture.md](architecture.md).
