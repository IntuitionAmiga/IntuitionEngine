# Intuition Engine - Performance Architecture Plan

Unified, adversarially verified list of architectural changes to improve runtime
performance without changing guest-visible semantics. Every claim below was
verified against the code on disk (2026-07-04); file:line references are to the
current tree. Ranked by expected win per effort.

---

## Phase 0 - Measure first

### 0. Extend performance accounting beyond x86

Status: base implementation present. `PerfAcct` is embedded in IE64, M68K, Z80,
6502, and x86 CPUs. `IE_PERF_ACCT=1` records native JIT time, interpreter or
fallback time, and retired guest instructions in the JIT dispatchers. Subsystem
accounting records compositor frames, audio pulls, and 32-bit bus slow paths.

This is not a speedup by itself - it is insurance against optimising the wrong
bottleneck. Cheap; do it first; it feeds everything below.

### 0b. Make JIT deopt reasons first-class

Classify every JIT exit uniformly: unsupported instruction, helper call, MMIO
guard, self-modifying-code invalidation, interrupt sample, code-cache pressure,
debug/watchpoint. The machinery half-exists (`NeedIOFallback` / `NeedInval` /
`NeedHelper` flags; `ioBails` already feeds the <25% promotion gate). Unify the
taxonomy and feed it back into tiering so blocks that always deopt stop getting
promoted. Pairs with Phase 0.

Status: base taxonomy present in `jit_deopt_reasons.go`. Dispatchers record
unsupported, helper, MMIO, SMC, interrupt, cache-pressure, and debug reasons.
`TierController.ShouldPromoteDeopt` applies the existing admission arithmetic to
taxonomy totals.

---

## Tier A - dramatic wins

### A1. Resumable inline MMIO/MMU helpers - fix the JIT exit protocol

Verified current cost:

- An MMIO access in native code causes: block exit → full state sync →
  `interpretOne` re-decode → map lookup → block re-entry
  (`jit_exec.go:600-639`).
- Helper exits never resume mid-block - after servicing, the block re-runs
  from the start (`jit_helper_dispatch.go:107`).
- The MMU check is a compiled-in per-op runtime branch
  (`jit_emit_amd64.go:2560-2566`), so with the MMU enabled every load, store,
  push, and pop terminates the block via a helper exit. Combined with region
  promotion being gated on `!cpu.mmuEnabled` (`jit_exec.go:450`), IntuitionOS
  MMU-on workloads run essentially block-per-memory-op.

Fix: emit a native `CALL` into a registered Go helper via a trampoline (the
same g0 `asmcgocall` machinery as `jit_call.go:51-67`) and continue the block
after return. Build it as shared infrastructure, the way
`jit_mmio_poll_common.go` unified the poll fast path. A formal
MemoryView/helper ABI is the *vehicle* for this work, not a standalone perf
item. This also lets the `ioBails < 25%` promotion gate stop excluding
device-heavy hot loops.

Note: device-side batching largely already exists (audio engines are
event-list batched, the blitter runs batched per refresh tick, chip mixing is
64-sample segmented). The JIT exit cost dominates; fix it before revisiting
device granularity.

Gates: JIT parity/differential tests, IntuitionOS boot, Harte suites.

Status: implemented for IE64 on amd64 and arm64. Helper exits can resume
inside the same native block for integer LOAD, STORE, PUSH, and POP paths when
the resume guard permits it. The dispatcher cancels resume on pending
interrupts, SMC invalidation, PTBR or MMU-mode changes, timer/debug state, and
the `IE64_JIT_RESUME=0` kill switch. A 4-entry direct-mapped MMU micro-TLB is
filled by LOAD/STORE helpers, probed natively, and flushed on PTBR writes and
TLB invalidation. `BenchmarkIE64_MMIO_{Interpreter,JIT}` and
`BenchmarkIE64_MMU_Mixed_{Interpreter,JIT}` cover the helper-heavy paths.
Linux/arm64 helper-resume parity is covered by the qemu-backed `make
test-cross` smoke path. x86, M68K, Z80, and 6502 still use backend-specific
bail/re-execute protocols rather than a continuation-address helper ABI; safe
follow-up work there is limited to backend-local fallback reductions unless
their emitters grow explicit resume labels and register-state contracts.

### A2. Region/tier coverage expansion, with a shared policy layer

Verified state:

- Single-block tier-2 promotion is retired on every backend
  (`jit_tier_backends.go` - all `PromoteBlock` return false).
- x86 dynamic register allocation already works (`x86Tier2RegAlloc` at
  `jit_x86_common.go:1090`, resolved through `x86CurrentCS.regMap` at
  `jit_x86_emit_amd64.go:79-86`). Region promotion is enabled by default with
  `X86_JIT_REGIONS=0` as the opt-out, and still requires a region floor of at
  least three blocks.
- IE64 regions run by default but use fixed 5-register pinning, no
  dynamic regMap, and are gated `!cpu.mmuEnabled` (`jit_exec.go:450`).
- M68K already has production region formation and promotion
  (`m68kFormRegion`, `m68kCompileRegion`, and
  `m68kTryPromoteJITRegion`). The A2 work is admission widening and fallback
  reduction, not a greenfield region path.
- Z80 and 6502 have no active production region promotion. Z80 has scanner and
  test surface without exec promotion. 6502 has dormant region-tier code gated
  off by `p65RegionRegionPromotion = false`.

Work items, in order:

1. **Centralise policy, keep emitters per-CPU.** Admission, tiering
   thresholds, region promotion, I/O-bail accounting, deopt classification,
   and invalidation handling belong in a shared `TierController`-level policy
   layer. Unify the decision machinery, not the JIT substrate.
2. **M68K region admission + fewer native exits.** AROS is the workload. Expand
   production-native-safe instruction coverage, reduce one-instruction
   fallback frequency, compile helper islands, add region promotion around
   hot M68K loops.
3. **x86:** fix the chain-coherence blocker (stale forward chain slots plus
   live old code in execMem on promotion - see
   `memory/project_x86_tier2_regmap_debt.md`), then default-enable regions.
4. **IE64:** dynamic register allocation inside regions; MMU-aware
   region formation (translate at region-form time, guard on PTBR - the
   `mmuBlocks` keying already exists).

M68K-first vs IE64-MMU-first is a workload decision (AROS vs IntuitionOS),
not a technical one. A1 and A2 compound: MMIO stops disqualifying blocks, so
regions get bigger.

Gates: Harte suites, AROS boot (hard gate for any register-mapping change),
CallChurn bench.

Status: shared region floors now live in `TierThresholds.RegionMinBlocks` and
all current backends route promotion floors through `TierController`. M68K
production fallback uses one-instruction fallback for unsupported leading
instructions and has targeted tests for AROS block-head and epilogue shapes.
`BenchmarkM68K_Mixed_{Interpreter,JIT}` was added for the mixed AROS-style
guard set. x86 promotion now retargets every inbound chain slot for a promoted
PC: compatible register maps jump to the new region entry, incompatible maps
fall through to the dispatcher, and matching RTS-cache entries are cleared.
The x86 region default is now on with `X86_JIT_REGIONS=0` as the kill switch.
IE64 has opt-in MMU-aware region formation behind `IE64_JIT_REGION_MMU=1`:
the builder translates each virtual block start through the current PTBR,
scans within the translated execute page, keeps virtual block keys for the
MMU cache, and uses the same compile-time MMU bail marking as per-block MMU
compilation. Dynamic IE64 region register allocation is still a planned
emitter refactor; the current implementation records region-level register
pressure on the emitted region block, reports it through region stats, and
supports `IE64_JIT_REGION_MAX_SPILLS` as an optional admission gate, but still
compiles with the existing fixed mapping. Broader CALL/RET parity gates remain
the guard set for future region widening.

### A3. Explicit video frame/layer ownership - leases / ring-buffered frames

The default pipeline is the hardware compositor
(`canUseHardwareCompositorLocked`, `video_compositor.go:569`), and it performs
roughly four full-frame passes per frame:

- `snapshotFrameLocked` copy (`video_chip.go:4140`)
- `appendCompositeLayer` clone (`append([]byte(nil), buf...)`,
  `video_compositor.go:664`)
- `cloneCompositorLayers` (`video_compositor.go:618`)
- `stageHardwareCompositorBuffer` copy (`video_compositor.go:296`)
- plus per-layer `WritePixels`

At 1080p60 that is on the order of 2 GB/s of pure memcpy. Introduce frame
leases or ring-buffered immutable frame objects so producers hand buffers to
the compositor/backend without defensive full-frame copies; `WritePixels`
becomes the only unavoidable copy. Preserve snapshot/reverse-debug semantics
with copy-on-snapshot. Apply the same treatment to the software fallback path
(finalFrame zero + copy chain at `video_compositor.go:847-849`, `:511`; ebiten
`video_backend_ebiten.go:238`, `:1405`).

Gates: demo video tests, desktop PNG comparisons, `WaitSwapIdle` tests.

Status: hardware-compositor ownership is implemented. `VideoFrameLeaseRing`
hands out retained RGBA slots for hardware layer collection and the software
fallback output handoff, snapshots are copy-on-demand, and `NormaliseAlpha`
preserves the hardware/software rule that zero-alpha non-black pixels become
opaque while transparent black stays transparent. Ebiten retains lease-backed
layer buffers until replacement or clear, while non-lease callers and
`IE_VIDEO_FRAME_LEASES=0` keep the defensive staging/output copies. The software
fallback still performs the final zero/blend pass before the backend
`UpdateFrame` copy.

---

## Tier B - solid wins

### B1. Range-scoped SMC invalidation

IE64 and x86 do a whole-cache flush plus `execMem.Reset()` on any code write
(`jit_exec.go:615-617`, `jit_x86_exec.go:440-442`) - a recompile storm. M68K
falls back to a full cache reset for deferred or unknown-size writes
(`jit_m68k_exec.go:3273-3281`). Z80 and 6502 already do page-granular
`InvalidateRange` and prove the pattern. Requires an execMem reclamation
policy (segmented arenas or per-block generations - the bump allocator cannot
free) plus an inbound-chain unpatch sweep (the `inboundChainSlots` registry
already exists for this). Cache pressure becomes a deopt class under 0b.

Status: implemented for x86 and IE64 range reporting. The amd64 x86 emitter
reports the self-modifying write address and size through `X86JITContext`, and
IE64 direct/helper stores report exact ranges through `JITContext`. Dispatchers
invalidate only overlapping `CodeCache` blocks when `IE_JIT_SMC_RANGE` is not
`0`; the code-page bitmaps are rebuilt from surviving cached blocks after range
invalidation, so a page shared by two cached blocks stays marked when only one
block is removed. `IE_JIT_SMC_RANGE=0` keeps the previous whole-cache flush
plus `execMem.Reset()` behaviour. Range invalidation now resets the bump
allocator when the removed block was the last live cached block. ExecMem now
uses logical arenas over the platform RW/RX mapping: cache replacement, range
invalidation, full invalidation, and explicit block removal release a block's
arena lease, and an arena becomes reusable once its last live block is evicted.

### B2. Direct-mapped dispatch cache

Every dispatcher iteration probes `map[uint64]*JITBlock`
(`jit_common.go:1025-1027`); no front cache exists (the 4-entry RTS MRU is
scoped to return targets only). Put a PC-indexed direct-mapped array (~4096
entries) in front; a hit is one compare plus one load, the map is the miss
path.

Status: implemented in `jit_dispatch_cache.go` and wired into `CodeCache.Get`,
`GetMMU`, `GetKey`, and the corresponding put and invalidation paths. Cache
entries carry the code-cache generation, so full and range invalidations expire
entries in O(1). `IE_JIT_DISPATCH_CACHE=0` disables the front cache.

### B3. x86: make `jitRegs` canonical

The full 8-register `syncJITRegsToNamed`/`syncJITRegsFromNamed` shuttle runs
twice unconditionally per outer dispatcher iteration
(`jit_x86_exec.go:141/146` around the break-in check, `:177/182` around the
MMIO poll probe) plus around every fallback. Make `jitRegs` the canonical
storage; sync named fields only for debugger/snapshot readers.

Status: partially implemented for the unconditional dispatcher probes. The x86
JIT loop now calls `debugHandleBreakInJIT`, which syncs named registers only
when a debug break-in hook is installed, and `tryFastMMIOPollLoopJIT`, which
updates canonical `jitRegs` directly for the common poll path. The JIT MMIO
byte-write fallback now handles recognised forms against canonical `jitRegs`
and only syncs to named registers when it must enter the interpreter `Step`
fallback. Interrupt paths still sync on demand because they need the
interpreter-style named-register handlers.
`TestX86JIT_BreakInSeesCurrentRegs`,
`TestX86JIT_MMIOPollUsesJITRegsWithoutNamedShuttle`, and
`TestX86JIT_Exec_MMIOByteWriteFallbackJITUsesCanonicalRegs` pin the behaviour.

### B4. Audio block render graph

Add block-capable ticker/mixer/post-FX interfaces (`TickBlock(n)`), pooled
scratch buffers, and chunk-level processing; keep the per-sample path as the
compatibility fallback (default interface impl loops `TickSample`, so no
engine breaks). Verified: no block API exists today, and `ReadSamples`
allocates `audioBlockState` + `make([]float32, len(dst))` on every oto pull
(`audio_chip.go:3133-3136`).

Magnitude caveat: idle engines already early-out on atomics before locking
(`sid_engine.go:1022-1024`, `psg_engine.go:289-291`), so the win applies to
active engines only (~44.1k dispatches/s each). Real but modest - ranked below
the video work.

Status: C3's allocation half is implemented: `SoundChip` reuses its
`audioBlockState` and mixer-capture scratch slice across `ReadSamples` calls,
with `TestReadSamples_ZeroAllocsSteadyState` pinning zero steady-state
allocations. The block-capable engine surface is implemented: `BlockTicker`
sits beside `SampleTicker`, PSG has a lock-once `TickBlock`, and SID has a
conservative bit-identical `TickBlock` wrapper. `ReadSamples` now uses a
guarded block graph when all registered tickers explicitly opt in through
`ReadSamplesBlockTicker`, no independent sample mixers are registered, and SFX
is inactive; otherwise it keeps the existing per-sample ticker loop to preserve
event flush boundaries. `TestReadSamples_UsesSafeTickerBlockGraph` and
`TestReadSamples_UnsafeBlockTickerFallsBackToSamples` pin the routing.

### B5. Coprocessor completion: event-driven wake

The completion watcher is an unconditional 100 µs ticker with no wake channel;
it scans even with zero coprocessors active and is started at EmuTOS/AROS boot
(`coprocessor_manager.go:191-224`). Workers should signal completion via a
channel; the ticker is demoted to a slow fallback.

A *full* coordinated guest-event scheduler layer is rejected for now: this is
one bad ticker, not a systemic problem, and a unifying layer would couple
deliberately decoupled subsystems and touch tuned interrupt-latency cadences
(M68K 256-instruction sample interval, STOP park ≤200 µs, 4096-iteration poll
cap). Revisit only if Phase-0 accounting shows wakeup jitter is a real
bottleneck.

Status: implemented. The watcher now wakes from a buffered completion channel
and uses a 10 ms fallback ticker. Response-status writes signal the watcher
through the bus, worker `done` closes signal the same wake channel, and the
watcher reaps dead workers before scanning completions.

---

## Tier C - cheap, cumulative

- **C1. Transparent hugepages.** `MADV_HUGEPAGE` on the guest-RAM mmap
  (`machine_bus_alloc_unix.go:46-64`) and JIT exec regions. Zero hits in the
  repo today; TLB relief on a multi-GiB bus, near-free.
  Status: implemented as a best-effort Linux hint for guest-RAM mmap and Linux
  ExecMem RW/RX mappings. Non-Linux platforms compile to a no-op.
- **C2. Single bitmap probe for aligned bus accesses.** Both MMIO-page probes
  always execute on the RAM fast path (`machine_bus.go:1859` - the `&&`
  short-circuits the wrong way for the common case), even when an aligned
  access cannot straddle a 256-byte page. Affects bus-path users only
  (Z80/x86/6502 interpreters, JIT bails, DMA - the IE32/IE64/M68K interpreters
  bypass the bus below 0xA0000 via cached memBase).
  **Warning:** do NOT fold the `addr >= 0xFFFF0000` guard into the bounds
  check - the sized entry points take `uint32` and `addr+4` wraps for
  `addr >= 0xFFFFFFFD` (`machine_bus.go:1847`), so the fold would pass the
  bounds check and reach an out-of-bounds unsafe load. The guard stays (or the
  bounds compare moves to 64-bit arithmetic first).
  Status: implemented for 16-bit, 32-bit, and 64-bit bus fast paths via
  `ioPageMappedForSizedAccess`, with tests for high-address wrap safety and
  unaligned RAM-to-I/O straddles.
- **C3. Hoist per-pull audio allocations** (`audioBlockState` +
  `mixerCapture`) onto the chip and reuse.
  Status: implemented and covered by `TestReadSamples_ZeroAllocsSteadyState`.
- **C4. Profile-informed tuning knobs.** Buffer sizes, promotion thresholds,
  iteration caps selected per profile (AROS, EmuTOS, BASIC, demos) -
  parameters only, never divergent code paths. Idiom recognition (the
  MMIO-poll pattern) remains the mechanism for fast paths.
  Status: implemented for existing tier-controller parameters through
  `IE_PERF_PROFILE`. The default profile reproduces current IE64, M68K, x86,
  and 6502 thresholds and the current 64-sample audio chunk. `latency` and
  `throughput` adjust promotion thresholds and the `ReadSamples` flush chunk;
  code paths do not diverge.

---

## Rejected / dropped (with reasons)

| Item | Reason |
|------|--------|
| Unified guest-event scheduler layer | One bad ticker (B5), not systemic; the layer couples tuned latency cadences; revisit with Phase-0 evidence. |
| Standalone sealed MemoryView ABI | Everything it would bundle already exists and is fast (sealed snapshot bypasses even the atomic load, `machine_bus.go:1065-1069`; JITs pin memBase/IO-bitmap in host registers). Zero cycles saved standalone; folded into A1 as the helper-ABI vehicle. |
| Profile-specific fast code paths | Test-matrix explosion and semantic-drift risk; idiom recognition already achieves the goal generically; profile hacks do not translate to the FPGA endgame. Narrow knob form kept as C4. |
| Blitter per-pixel `bus.Write32` rewrite | That path (`video_chip.go:2522`) is the non-VRAM-destination fallback only; EmuTOS/AROS blits hit VRAM row-wise `copy()`/bulk-fill fast paths (`video_chip.go:1790-1834`). Residual idea: row-band parallelism for very large ops (blitter is single-threaded under `chip.mu`) - modest. |
| VIDEO_STATUS `time.Now()` per read | Gated behind `!everSignaled` (`video_chip.go:2835-2854`); steady state is pure atomic loads. Bootstrap-only cost. |
| Sign-extend/bounds-check fold | Unsafe - uint32 wraparound, see C2 warning. |
| "Build x86 tier-2 regalloc" | Already built and working; the blockers are chain coherence and the default-off env gate (see A2). |
| Broad bus rewrites | RAM fast path benchmarks ~5-6 ns/op; sparse backing (~14 ns/op) serves only the IE64 high-address/test path (production high RAM uses lock-free `MmapBacking`). Optimise only where profiling shows pressure. |

## Already optimised - do not redo

Audio block mixing + atomic ticker caches; compositor dirty rects (software
path); voodoo state-stamped batching + async swap + row-band workers; MMIO
poll fast path (Go loop, 4096-iteration cap); M68K STOP park; cache-line-tuned
struct layouts; M68K A5/A6 + FP-xmm pinning; sealed bus snapshot + bitmap fast
path; mmap guest RAM + `MADV_DONTNEED` reset.

## Suggested sequence

1. Phase 0 / 0b - accounting + deopt taxonomy (cheap, immediate).
2. A1 - resumable helpers, IE64 first (smallest surface, biggest per-effort
   payoff, unblocks MMU-on IntuitionOS), then propagate as shared infra.
3. A2 - policy layer, then M68K regions (AROS), then x86 enablement, then
   IE64 region regalloc/MMU.
4. A3 - frame ownership / copy elimination.
5. B-tier ordered by profiling evidence from Phase 0.
