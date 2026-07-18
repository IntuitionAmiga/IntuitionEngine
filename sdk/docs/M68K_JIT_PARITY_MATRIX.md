# M68020 JIT Capability Parity Matrix

Milestone 1 artifact of `M68K_JIT_PARITY_PLAN.md`. Every row was verified
against live source on branch `68kJITparity`; evidence cells cite the live
symbol and file. Decisions use the plan's classification vocabulary.

Status legend: `yes` (live), `no` (absent), `n/a` (not meaningful for the
backend), `planned` (M68020 backend does not exist yet on that target).
M68020 arm64 and wasm columns are `planned` throughout because no M68020
JIT backend exists off amd64; the Decision column records what happens to
the capability when those backends are brought up.

## Core facilities

| Capability | IE64 amd64 | IE64 arm64 | IE64 wasm | IE64 evidence | M68020 amd64 | M68020 evidence | Decision |
|---|---|---|---|---|---|---|---|
| Block scan + compilation | yes | yes | yes | `scanBlock` (`jit_common.go`), `compileBlock` (`jit_emit_amd64.go`, `jit_emit_arm64.go`), `wasmCompileBlock` (`jit_wasm_ie64_emit.go`) | yes | `m68kScanBlock` (`jit_m68k_common.go:3257`), `M68KJITInstr` IR, per-group length decoders | Already implemented (amd64); backend lowering per new backend |
| Code cache + metadata | yes | yes | yes (own maps) | `CodeCache` family (`jit_common.go:1165+`); wasm `rt.cacheStore` (`jit_wasm_runtime.go`) | yes | `initM68KJIT`, `m68kResetJITCodeCache`, `m68kRebuildJITCodeMetadata[Range]`, block-bytes hash guard (`jit_m68k_exec.go`) | Already implemented (amd64); wasm backend needs its own module cache per milestone 5 decision |
| Dispatch loop | yes | yes | yes (own) | `(*CPU64).ExecuteJIT` (`jit_exec.go:136`), `wasmJITDispatch` (`jit_exec_wasm.go`) | yes | `M68KExecuteJIT` (`jit_m68k_exec.go:2535`), `m68kJitExecute` (`jit_m68k_dispatch.go`) | Already implemented (amd64); per-backend execution loops per milestone 2 |
| Generation-tagged direct dispatch cache | yes | yes | analogous | `JITDispatchCache`, `IE_JIT_DISPATCH_CACHE` (`jit_dispatch_cache.go`), embedded as `CodeCache.dispatch` | no | zero `dispatchCache` references in `jit_m68k_*` | Missing and applicable: port (optimisation slice, milestone 7) |
| Interpreter fallback gating | yes | yes | yes (allowlist) | `needsFallback` (`jit_common.go`); `wasmSupportedOpcode` (`jit_wasm_ie64_emit.go`) | yes | `m68kNeedsFallback:1392`, `m68kNeedsConservativeFallback:2855` (`jit_m68k_common.go`), ~90 `m68kIsNativeSupported*` predicates, fallback bursts (`jit_m68k_exec.go`) | Already implemented (amd64); reuse frontend predicates on new backends |
| Transcendental interpreter admission | n/a (IE64 DTrans helper exits) | n/a | n/a | `emitDTransHelperExitAMD64` (`jit_emit_amd64.go`) | yes | `m68kFPUInstrIsTranscendental` (`jit_m68k_common.go:1025`), `m68kInterpretTranscendentalBurst` (`jit_m68k_exec.go:1299`) | Implemented differently (M68020 semantics); keep |
| SMC detection + exact-range invalidation | yes | yes | yes (own bitmap) | `markJITSMCWrite`, `ie64InvalidateSMCRange*`, `IE_JIT_SMC_RANGE` (`jit_ie64_smc_range.go`); `rt.invalidateRange` (`jit_wasm_runtime.go`) | yes | `m68kEmitSMCRangeBailChecks` (`jit_m68k_emit_amd64.go:5192`), `m68kInvalidateJITCodeRange`, cross-thread queue + `m68kCoalesceInvalRanges` (`jit_m68k_exec.go`) | Already implemented (amd64); replicate per backend (milestones 3/5) |
| Exec-mem reset on empty cache | yes | yes | n/a | `resetExecMemWhenCacheEmpty` (`jit_common.go:1186`) | yes (shared) | shared `ExecMem` allocator via `m68kGetJITExecMem` (`jit_m68k_exec.go:458`) | Already shared infrastructure |
| Retired-instruction accounting | yes | yes | yes | `emitPackedPCAndCount` (`jit_emit_amd64.go`), `emitStoreRetCount` (`jit_emit_arm64.go`), `pushRetCount` (`jit_wasm_ie64_emit.go`) | yes, differentially validated | `m68kJITRetiredInstructionCount` ChainCount+RetCount contract (`jit_m68k_exec.go:1843`), `jit_m68k_count_accounting_test.go` | Already implemented (amd64); mandatory from day one on new backends (milestone 3 gate) |
| Differential/lockstep harness | test-suite based | test-suite based | parity harness | `tools/wasm/e2e_jit_parity.mjs`, IE64 parity tests | yes, runtime harness | `m68kJITLockstepSession/Snapshot/Boundary` (`jit_m68k_lockstep.go`), `m68kVerifyInterpPrePass` (`jit_m68k_exec.go:819`), `jit_m68k_differential_test.go` | Already implemented; extend grids per backend |
| Helper-exit dispatch model | yes | yes | yes | `handleJITHelper` (`jit_helper_dispatch.go:38`), per-op helper exits | yes | `m68kHandleJITHelper` (`jit_m68k_exec.go:1536`): FPU, interpreter, MMIO MOVE/CLR helpers | Already implemented (amd64); resume-model parity verified: M68020 has no helper-resume re-entry (IE64 `IE64_JIT_RESUME`); deferred pending a benchmark |
| Deopt taxonomy + perf accounting | yes | yes | stub | `jit_deopt_reasons.go`, `IE_PERF_ACCT` | yes | `recordBlockDeopt(DeoptSMC/DeoptMMIO)` (`jit_m68k_exec.go:3370`) | Already shared |

## Memory and control flow

| Capability | IE64 amd64 | IE64 arm64 | IE64 wasm | IE64 evidence | M68020 amd64 | M68020 evidence | Decision |
|---|---|---|---|---|---|---|---|
| Fast-path bitmap probe (RAM vs MMIO) | yes | no | no | `emitAMD64FastPathBitmapProbe`, `LookupFastPathBitmapShape` (`jit_fastpath_bitmaps.go`) | yes (own equivalent) | IOPageBitmap inline scan, `m68kCtxOffIOPageBitmapPtr/Len` (`jit_m68k_emit_amd64.go:5131`) | Implemented differently (backend equivalent); replicate the bitmap scan per backend |
| Const low-RAM / constant-address proof | yes | yes | yes | `ie64ConstLowRAMAccess` (`jit_common.go:1126`) | no | no constant-address proof in M68020 emitter | Missing and applicable to abs.W/abs.L EA modes: port as optimisation slice |
| Block chaining | yes | no ("arm64 has no chaining tier") | yes (chain driver) | `emitChainExit` (`jit_emit_amd64.go:997`), `PatchChainsTo`, `jit_chain_ordering.go`; wasm `drive()` (`jit_wasm_runtime.go`) | yes | `m68kEmitChainEntry/Exit` (`jit_m68k_emit_amd64.go:1029+`), `m68kJITDisableChains` kill switch | Already implemented (amd64); note IE64 arm64 itself lacks chaining, so M68020 arm64 chaining is applicable only after backend bring-up and is not blocked on IE64 precedent |
| Chain-slot ordering policy | yes | n/a | n/a | `chainSlot`, ordering invariant (`jit_chain_ordering.go`) | yes (shared `chainSlot`) | shared `chainSlot` from `jit_common.go`; bidirectional patching (`jit_m68k_exec.go`) | Already shared; verify ordering when arm64 chaining lands |
| Static jump chasing | yes | yes | shared analysis | `ie64ChaseStaticJumps*` (`jit_ie64_jmp_chase.go`, untagged) | yes | `m68kStaticJMPTrampolineTarget:2462`, `m68kChaseStaticJMPTrampolines:2502` (`jit_m68k_exec.go`) | Already implemented (amd64); reuse per backend |
| RTS/return inline cache | yes (4-entry MRU) | no | no | `jitCtxOffRTSCache0PC/Addr` probes (`jit_emit_amd64.go:4165`) | yes (8-entry MRU) | `RTSCache0..7` context fields, `m68kClearJITRTSCache` (`jit_m68k_exec.go:557`) | Already implemented (amd64), richer than IE64; backend lowering per new backend |
| Pattern-loop interpreter accelerators | no | no | no | — | yes | `tryM68KIndexedByteCopyCountLoop:1612`, `tryM68KLongFillCountLoop:1669` (`jit_m68k_exec.go`) | M68020-specific; keep |
| MMIO poll-loop specialisation | yes | yes | yes | `tryFastIE64MMIOPollLoop` (`jit_mmio_poll_exec_native.go`), `TryFastMMIOPoll` (`jit_mmio_poll_common.go`) | no (MMIO handled by helpers/fallback) | `m68kExecuteJITMMIOMOVEHelper/CLRHelper` (`jit_m68k_exec.go`) | Deferred pending a benchmark (M68020 guests poll via chipset emulation paths) |
| MMU micro-TLB inline probe | yes | no inline emitter (shared state only) | no | `emitMMUMicroTLBProbeAMD64` (`jit_emit_amd64.go:915`), `jit_mmu_microtlb_common.go` | n/a | M68020 core runs without IE64-style MMU translation | Unsafe or irrelevant (rejected by M68020 semantics) |
| High-address bail (guest above 4 GiB) | yes | yes | n/a | `emitHighAddrBailCheckAMD64/ARM64` | n/a | M68020 guest addressing is 32-bit | Rejected by M68020 semantics |
| Backward-branch in-block loops | yes (loop bodies) | yes | yes (structured loops) | loop plan emission per backend | yes | `m68kDetectBackwardBranches[WithMem]` (`jit_m68k_common.go:3756`), budgeted native loops | Already implemented (amd64); wasm needs structured-loop lowering (milestone 5) |

## Registers and FP

| Capability | IE64 amd64 | IE64 arm64 | IE64 wasm | IE64 evidence | M68020 amd64 | M68020 evidence | Decision |
|---|---|---|---|---|---|---|---|
| Guest register mapping (fixed) | partial map + spills | full map R1-R14 | per-block locals | `ie64ToAMD64Reg` (`jit_emit_amd64.go:92`), `ie64ToARM64Reg` skipping X18 (`jit_emit_arm64.go:91`), `wasmBuildGPRPlan` | yes, incl. A5/A6 pinning | `jit_m68k_abi.go` (D0→RBX, D1→RBP, A0→R12, A7→R13, A5→R9, A6→R8, CCR→R14), `m68kAnalyzeBlockRegs` (`jit_m68k_common.go:3438`) | Already implemented (amd64); arm64 map defined once in `jit_m68k_abi_arm64.go` (milestone 3), X18/X16/X17 reserved |
| Dirty-state (written-reg) tracking | yes | yes | yes | `analyzeBlockRegs`, `emitEpilogue(storeRegs)` | yes | `m68kAnalyzeBlockRegs`/`m68kBlockRegs`, dirty-only stores in chain exits | Already implemented (amd64); replicate per backend |
| Region-wide GPR residency | yes (amd64 only) | no | no | `ie64BuildRegionRegMap`, `ie64ResidentBinding` (`jit_ie64_region_policy.go:190+`) | no (regions use fixed map, no Tier-2 regalloc) | region limits noted at `m68kFormRegion` | Missing and applicable: port as optimisation slice (milestone 7), amd64 first |
| Integer flags/CCR liveness | yes (`ie64FlagsLiveness`) | yes | yes | `jit_ie64_flags_liveness.go` (untagged), `JITFlagLiveness`/`MaterializeFn` | yes (own mechanism: lazy CCR + per-block liveness) | `m68kClassifyCCR:60`, `m68kCCRLiveness:455` (`jit_m68k_ccr_liveness.go`); lazy EFLAGS scheme (`jit_m68k_emit_amd64.go`) | Implemented differently (M68020 semantics); no port. Cross-block CCR liveness permanently excluded (unsound: CCR observable at interrupt boundaries) |
| Native FPU support | yes (IE64 FP64/FP32) | yes | yes (clean subset) | FP emitters per backend | yes (68881 SSE) | `m68kDecodeNativeFPURegToReg/EA` (`jit_m68k_common.go:1180/1300`), `jit_m68k_fpu_sse_amd64.go`, `jit_m68k_fpu_ea_amd64.go` | Already implemented (amd64); wasm gets clean-mapping subset only (milestone 5), arm64 NEON lowering (milestone 3/4) |
| FPU register residency | yes (SysV gate) | yes | yes (locals) | `ie64BuildBlockFPPlan`, `IE64_JIT_FP_RESIDENCY` (`jit_ie64_fpu_residency.go`) | yes (xmm8-15 pinning, loop blocks, non-Windows) | `br.fpPinned` (`jit_m68k_emit_amd64.go:11334`), `m68kFPPinned` (`jit_m68k_fpu_sse_amd64.go:189`), platform gates | Implemented differently (backend equivalent); per-target equivalent on arm64/wasm |
| FPSR/FP-CC liveness elision | yes | yes | yes | `ie64FPSRCCWriterElidable`, CC sinking (`jit_ie64_fpsr_liveness.go`) | yes (lazy FPSR) | `m68kFPUNextInstrOverwritesCCNoFault` (`jit_m68k_common.go:1097`), applied `jit_m68k_emit_amd64.go:12337` | Already implemented (amd64); replicate per backend |
| Helper resume (re-enter block mid-way) | yes (`IE64_JIT_RESUME`) | yes | no | `canResumeJITHelper` (`jit_helper_resume_common.go`), `emitResumeEntryAMD64/ARM` | no | helper exits return to dispatcher | Deferred pending a benchmark |

## Optimisation slices (milestone 7 candidates)

| Capability | IE64 amd64 | IE64 arm64 | IE64 wasm | IE64 evidence | M68020 amd64 | M68020 evidence | Decision |
|---|---|---|---|---|---|---|---|
| Region formation + promotion | yes (`IE64_JIT_REGIONS`) | yes (linux) | yes | `ie64FormRegion`, `ie64PlanRegion` (`jit_ie64_region_policy.go`); `wasmFormRegion` | yes (default on) | `ScanRegionM68K` (`jit_region_backends.go:40`), `m68kFormRegion`/`m68kCompileRegion` (`jit_m68k_emit_amd64.go:11476+`), `m68kTryPromoteJITRegion` (`jit_m68k_exec.go:1954`) | Already implemented (amd64); resolve mislocation (region builder lives in the amd64 emitter file — milestone 2), then enable per backend |
| Constant folding (block + region) | yes | yes | yes | `ie64AnalyseConstFold` (`jit_ie64_const_fold.go`) | no | grep-confirmed absent | Port as slice; M68020 CCR proof required |
| Loop-invariant hoisting + loop precheck | yes | yes | yes | `ie64AnalyseLoop`, `ie64SelectLoopHoists` (`jit_ie64_loop_analysis.go`) | no | absent | Port as slice; alias proof required |
| Hoisted-access bounds-check elision | yes | yes | yes | `ie64CurrentAccessHoisted` | no | absent | Port with the hoisting slice |
| Bounded counter-loop budget removal | yes | yes | yes | `ie64BoundedCounterLoop` (`jit_ie64_loop_analysis.go`) | no (fixed 4095 budget) | backward-branch budget in emitter | Port as slice |
| Observed (trace-recorded) regions | yes | yes | yes | `ie64ObservedRecorder`, `ie64BuildObservedRegion` (`jit_ie64_observed_region.go`) | no | absent | Port as slice |
| Cold-exit outlining | yes | yes | no | `ie64ColdExitOutlineEligible` (`jit_ie64_cold_exit.go`), stubs per native backend | no | absent | Port as slice, native backends only |
| Indirect-target specialisation (JSR_IND-style) | yes | yes | yes | `emitJSR_IND_*` inline handling | partial (JMP/JSR abs are chained; no monomorphic indirect cache beyond RTS) | RTS cache only | Port as slice (monomorphic indirect-target cache) |
| JSR leaf fusion | yes | yes | no (rejects fused) | `analyzeJSRLeafFusion`, `isLeafFusionSafe` (`jit_common.go:389/435`) | yes | `m68kAnalyzeJSRLeafFusion:3300`, `m68kIsLeafFusionSafe:3332` (`jit_m68k_common.go`) | Already implemented (amd64); remaining work is backend lowering on arm64/wasm (milestones 4/6), not a fresh slice |
| JIT stats/diagnostics | yes (`IE64_JIT_STATS`) | stub | stub | `ie64JITStatsEnabled` (`jit_ie64_region_policy.go`) | yes (diag env family) | `m68kJITStrictMode`, `IE_DIAG_*` plumbing (`jit_m68k_exec.go:145-402,1896`) | Already implemented differently; keep |

## Rejected rows (per plan exclusions)

- Cross-block CCR liveness: permanently excluded, unsound (CCR observable at
  interrupt/exception boundaries).
- IE64 direct operand comparisons, fixed-width decode assumptions, IE64 MMU
  proofs, IE64 register-file allocation rules, high-address bail: rejected by
  M68020 semantics.

## Milestone 2 inputs recorded by this audit (and their resolutions)

- `jit_m68k_common.go` (untagged): no amd64 symbol leakage; two comments
  reference amd64 register names (lines ~165, ~2825) — they document why the
  context layout is shaped as it is; retained.
- `jit_m68k_ccr_liveness.go`: no leakage but was amd64-tagged despite being
  pure IR analysis. RESOLVED: untagged.
- `jit_flags_common.go`: backend-neutral vocabulary (`JITFlagState`,
  `JITFlagLiveness`, `MaterializeFn`) was amd64-tagged. RESOLVED: untagged.
- `jit_region_common.go` (profiles/enums) was amd64-tagged. RESOLVED: untagged.
- `ScanRegionM68K` + `RegionScanResult` were trapped in the amd64-tagged
  `jit_region_backends.go`. RESOLVED: moved to untagged
  `jit_region_scan_m68k.go`.
- `m68kFormRegion` decision (plan milestone 2 deliverable): it is pure
  frontend analysis (scan + admission predicates, no emission), so it becomes
  SHARED analysis, not per-backend. RESOLVED: moved with `m68kRegion` to
  untagged `jit_m68k_region_form.go`. `m68kCompileRegion` (emission) stays in
  the amd64 emitter; each backend implements its own region compiler.
- `m68kInstrMaySetGenericIOFallback` was pure opcode analysis inside
  `jit_m68k_emit_amd64.go`. RESOLVED: moved with the whole native-admission
  cluster (`m68kCanUseProductionNativeBlock`, `m68kBlockProductionNativeSafe`,
  `m68kInstrGenericIOFallbackUnsafe`, A7/control taint walk) to untagged
  `jit_m68k_admission.go`.
- Backend-neutral dispatch/tier policy extracted from `jit_m68k_exec.go` into
  untagged `jit_m68k_policy.go` (kill switches, warmup gating, burst sizing,
  interrupt-sample cadence, hotness, retired-count contract). Native
  execution mechanics stay in `jit_m68k_exec.go` (amd64), per plan.
- Per-target dispatch seams added: `jit_m68k_dispatch_arm64.go`,
  `jit_m68k_dispatch_wasm.go` (interpreter routes until milestones 3/5);
  `jit_m68k_dispatch_stub.go` tag narrowed to the remaining targets.

## Milestone 3 scaffolding delivered

- `jit_m68k_abi_arm64.go`: arm64 register mapping defined once (D0/D1/A0/A7/
  A5/A6/CCR/DataBase/MemBase/Ctx in callee-saved X19-X28; X18 never used,
  X16/X17 reserved for emitter scratch, X9-X15 scratch pool).
- `jit_m68k_abi_arm64_test.go`: pins reserved-register avoidance, uniqueness,
  callee-saved placement, scratch-pool disjointness (green under qemu-aarch64;
  hardware gates per plan still apply to residency/control-flow work).

## Milestone 3 slice 1 delivered: first native arm64 execution

Correctness-first minimal backend, differential-gated against the interpreter
from day one:

- `jit_m68k_emit_arm64.go`: A64 emitter for straight-line data-register
  prefixes (MOVEQ, MOVE.L reg/imm, TST.L, CLR.L, ADD/SUB/CMP/AND/OR/EOR .L
  reg-reg, ADDQ/SUBQ.L, NOP) with full CCR materialisation per instruction.
  Emitted blocks are leaf functions using only caller-saved registers
  (X0 ctx, X1 data-reg base, X3 SR, W4 CCR, W9-W15 scratch); the pinned
  X19-X28 mapping in `jit_m68k_abi_arm64.go` is the milestone 4 residency
  plan and is deliberately unused here. Adds the missing flag-setting A64
  encoders (ADDS/SUBS reg+imm, CSET, shifted ORR, 32-bit CMP immediate —
  the IE64 helper `arm64CMP_imm` is 64-bit and reads sign from bit 63).
- Flag semantics are parity with the interpreter, not the M68000PRM: IE's
  AND/OR/EOR preserve V and C (interpreter `SetFlagsNZ`, amd64
  `emitCCR_LogicPreserveVC`); the arm64 backend replicates this via
  `m68kA64FlagsLogicPreserveVC`. MOVE/MOVEQ/CLR/TST clear V and C, ADD/SUB
  set full XNZVC, CMP sets NZVC and preserves X.
- `jit_m68k_exec_arm64.go`: dispatcher with per-block-boundary interrupt and
  exception sampling, warmup-tier interpretation, single-instruction
  `StepOne` fallback for unsupported opcodes, and the shared retired-count
  contract (`m68kJITRetiredInstructionCount`). SMC is two-layered: same-thread
  writes are caught by the shared guest byte stamp
  (`m68kStampGuestBlockBytes`, moved to `jit_m68k_common.go`); cross-thread
  writes go through the shared invalidation queue and generation counter
  (enqueue/drain/coalesce and the bus-level entry moved to
  `jit_m68k_inval_queue.go`), with the arm64 dispatcher publishing the code
  envelope for the bus invalidator gate, draining at each loop head, and
  re-checking the `m68kJitInvalGen` snapshot immediately before native entry
  so a write between the stamp check and the call forces a re-loop (the same
  residual tail the amd64 dispatcher documents).
  `m68kJitAvailable` stays false on arm64 until the full milestone 3 gate
  (complete integer core, Harte exception cases, AROS boot on real hardware);
  tests opt in via `m68kJitEnabled`.
- `jit_m68k_arm64_backend_test.go`: operand-grid ALU differential (12x12
  values x 3 CCR seeds per shape), full-dispatcher differential including an
  unsupported-opcode fallback and STOP, retired-count equality, SMC stamp
  detection, native call smoke test. All green under qemu-aarch64.
- Remaining for milestone 3: memory EA modes with big-endian paths, byte/word
  sizes, shifts/rotates, LEA/PEA, exceptions with per-instruction resume PC,
  exact-range native-exit SMC invalidation, chaining, 68881 staging, Harte
  exception gate, AROS boot gate on real hardware.

## Milestone 3 slices 2 and 3 delivered: memory EAs, sizes, immediates, shifts

Slice 2 (memory effective addresses and operation sizes):

- EA modes lowered natively: Dn, An (word and long), (An), (An)+, -(An),
  (d16,An), (xxx).L and #imm sources. Absolute short, index and PC-relative
  formats remain interpreter fallback. The A7 byte rule (byte moves adjust
  the stack pointer by two) is applied at compile time.
- Sizes: byte, word and long for MOVE, MOVEA, TST, CLR, ADD, SUB, CMP, AND,
  OR, EOR, ADDQ, SUBQ (including the flagless address-register forms) and
  LEA. Byte and word register writes merge into the low bits only; flag
  extraction uses the shift-to-top trick so ARM NZCV matches the sized
  M68K result exactly.
- Big-endian access: loads and stores byte-swap through REV/REV16; the
  read-modify-write memory destinations swap in both directions.
- Every guest access is guarded inline: both bounds against MemSize plus an
  I/O bitmap probe of the first byte's page and, for multi-byte accesses,
  the last byte's page, so an access that crosses a 256-byte page boundary
  into an I/O page still bails. The amd64 shared single-access guard
  (m68kEmitMemAccessBailChecks, used by the RTS/LINK/UNLK stack paths) now
  takes an access size and applies the same two-page probe; amd64 word and
  long data accesses already scanned every page through the range checker. A
  failing guard exits through a per-instruction bail stub BEFORE any of the
  faulting instruction's side effects: RetPC is the faulting instruction,
  RetCount the fully retired predecessors, NeedIOFallback set, CCR flushed.
  The dispatcher interprets that one instruction and re-enters the loop,
  preserving exact retired accounting (verified differentially).
- Memory-to-memory MOVE guards BOTH addresses before committing either EA
  side effect; an uncommitted source postincrement or predecrement that
  targets the destination's base register is folded in at compile time.
- The I/O page bitmap builder moved to the shared frontend
  (m68kBuildJITIOPageBitmap in jit_m68k_common.go); amd64 and arm64 both
  call it at JIT initialisation.

Slice 3 (immediates, single-operand forms, shifts and rotates):

- ADDI, SUBI, CMPI, ANDI, ORI, EORI to data registers and memory (the CCR
  and SR immediate forms stay interpreter fallback), NEG, NOT, SWAP, EXT.W,
  EXT.L, EXTB.L and PEA.
- Immediate-count shifts and rotates on data registers, all sizes: LSL,
  LSR, ASL, ASR, ROL, ROR, ROXL, ROXR. Register-count forms and memory
  shift forms remain interpreter fallback.
- Interpreter parity quirks pinned by tests and honoured by the emitter:
  ASL carry and overflow are the OR of every shifted-out bit; the
  count-at-least-width shift forms use value-not-zero for carry; NOT, ANDI,
  ORI and EORI preserve X, V and C (SetFlagsNZ); ROL and ROR apply the
  rotate modulo AFTER the immediate zero-to-eight mapping, so ROL.B #8 and
  ROR.B #8 are complete no-ops that preserve the whole CCR.
- The interpreter's configured stack bounds are enforced natively on both
  backends: pushes (BSR, JSR, PEA, LINK) bail when the decremented A7 falls
  below stackLowerBound, pops (RTS, RTE, UNLK) when A7 is at or above
  stackUpperBound, matching Push32/Pop32 exactly. The bounds are read
  through context pointers because loaders retune them at runtime.

Test coverage: TestM68KARM64_DifferentialMemoryEAGrid (77 shapes),
TestM68KARM64_DifferentialShiftImmGrid (50 shapes), both over the operand
grid, three CCR seeds, comparing D/A registers, CCR, PC, retired counts and
the data and stack memory windows; TestM68KARM64_IOBailUnit (bail contract:
partial retirement, no side effects, pre-instruction CCR, bounds and I/O
page variants); TestM68KARM64_DispatcherIOFallback (end-to-end dispatcher
fallback with exact accounting). All green under qemu-aarch64; amd64 suite
unaffected (8669 tests).

Remaining milestone 3 work: absolute-short, index and PC-relative EA
formats; register-count and memory shifts; MOVEM, MULU/MULS/DIVU/DIVS,
BTST/BCHG/BCLR/BSET, Scc, TAS; branches and block terminators with
chaining and interrupt-boundary status publication; exceptions with
per-instruction resume PC; exact-range native-exit SMC invalidation; 68881
or staged fallback; Harte exception gate; AROS boot on real hardware.

## Milestone 3 slice 4 delivered (arm64 branches)

Scope of this slice:

- BRA with byte, word and long displacement as a native block terminator
  with a static exit PC.
- Bcc for all fourteen conditions (byte, word and long displacement) as a
  block-ending exit: the condition is evaluated from the live CCR and the
  resume PC (taken target or fallthrough) is computed at run time.
- DBcc for all sixteen conditions with ExecDBcc parity: condition true
  falls through without touching the counter; otherwise the low word of Dn
  decrements with the high word preserved, and the branch is taken unless
  the counter expires to minus one.
- BSR is not lowered; it stays with the interpreter's guarded stack path.
- Admission quirk pinned: the interpreter halts the machine when a taken
  BRA or Bcc target reaches ProfileTopOfRAM minus two, so such branches
  are rejected at admission and the interpreter keeps that behaviour.
  DBcc applies its taken target unchecked, exactly as ExecDBcc does.
- The supported-prefix rule now includes a supported branch as the final
  instruction of the block, which lets loop bodies ending in DBcc or Bcc
  run as single native blocks per iteration. No chaining yet; the
  dispatcher still samples interrupts and exceptions at every block
  boundary, and the branch retires as one instruction in the existing
  accounting contract.

Test coverage: TestM68KARM64_DifferentialBranchGrid (26 shapes: BRA all
displacement widths, every Bcc condition, DBcc condition and counter
interplay including low-word wrap and high-word preservation) over the
operand grid, seven counter seeds and four CCR seeds;
TestM68KARM64_BranchPrefixAdmission and
TestM68KARM64_BranchTargetOutOfProfileRAM (admission rules);
TestM68KARM64_DispatcherLoopDBRA (end-to-end DBRA loop through the
dispatcher with exact retired accounting). All green under qemu-aarch64;
amd64 suite unaffected (8669 tests).

### Milestone 3 slice 5 delivered (arm64 subroutine flow)

Slice 5 lowers the call and return terminators:

- BSR with byte, word and long displacement: the return address (the
  address after the whole instruction) is pushed through a guarded native
  stack write before A7 commits, then the block exits to the static
  target. The push guard bails with the BSR unexecuted, so the
  interpreter reproduces Push32's stack exceptions on the fallback path.
- JSR with the lowered control EA set (An), (d16,An) and (xxx).L. The
  effective address is computed before the push, matching ExecJsr's
  ordering (visible when the EA base is A7). Other control EA forms
  (absolute short, indexed, PC-relative) stay with the interpreter.
- JMP with the same EA set, as a pushless dynamic exit.
- RTS: guarded native pop (read at A7, then A7 += 4) with the popped
  address as the dynamic exit PC. A guard failure bails with the RTS
  unexecuted and the interpreter reproduces Pop32's checks.
- Admission quirks pinned: the interpreter halts on a taken BSR target at
  or beyond ProfileTopOfRAM minus two (after pushing), so such BSRs are
  rejected at admission; JSR and JMP apply their targets unchecked, as
  ExecJsr/ExecJmp do, so no target check is made for them.
- Stack bounds enforced (both backends): the native push (emitPushRet, PEA)
  and pop (RTS) paths check cpu.stackLowerBound/stackUpperBound through
  context pointers before the access guard and bail with the terminator
  unexecuted, so the interpreter fallback raises the exact Push32/Pop32 bus
  errors. amd64 gained the same checks across BSR/JSR/PEA/LINK (floor) and
  RTS/RTSNoChain/RTE/UNLK plus the fused JSR/RTS leaf pair (ceiling/floor).

Test coverage: TestM68KARM64_DifferentialCallReturnGrid (13 shapes: BSR
all widths and directions, JSR and JMP over each lowered EA, RTS) with
register, CCR, PC, retired-count and stack memory comparison;
TestM68KARM64_CallReturnPrefixAdmission (admission rules including the
indexed-EA rejection and the out-of-RAM BSR quirk);
TestM68KARM64_JSRStackBailUnit (pre-commit bail contract for the stack
push); TestM68KARM64_DispatcherCallReturn (end-to-end JSR/RTS pair
through the dispatcher with exact retired accounting). All green under
qemu-aarch64; amd64 suite unaffected (8669 tests).

## Milestone 3 slices 6-14 delivered: integer core completion and correctness gate

These slices complete the arm64 integer execution foundation. Every M68020
instruction is now either lowered natively or admitted-and-interpreted with an
exact per-instruction resume PC; differential grids run against the interpreter
under qemu-aarch64.

Native lowering added:

- Effective-address formats: absolute short ((xxx).W), PC-relative ((d16,PC),
  resolved to a compile-time constant address, read-only), and the brief-format
  index modes (d8,An,Xn) and (d8,PC,Xn) with word/long index size and scale.
  The 68020 full extension-word format (memory indirect, base/outer
  displacements) stays interpreter fallback. PC-relative and index-with-PC
  operands are rejected as write destinations.
  (TestM68KARM64_DifferentialExtendedEAGrid.)
- Memory shift/rotate by one (ASR/ASL/LSR/LSL/ROXR/ROXL/ROR/ROL on a word),
  matching ExecShiftRotateMemory including the interpreter quirk that memory
  ROL/ROR set X as well as C. (TestM68KARM64_DifferentialMemShiftGrid.)
- MULU.W/MULS.W (16x16 -> 32, N/Z with V/C/X preserved per SetFlagsNZ) and
  DIVU.W/DIVS.W (32/16) with a divide-by-zero bail (the interpreter raises the
  zero-divide trap), the quotient-out-of-range overflow path (V set, Dn
  unchanged), and remainder/quotient packing.
  (TestM68KARM64_DifferentialMulDivGrid, TestM68KARM64_DivZeroBail.)
- BTST/BCHG/BCLR/BSET with dynamic (Dn) and immediate bit sources over long
  register and byte memory destinations; Z-only flag update.
  (TestM68KARM64_DifferentialBitOpGrid.)
- Scc (all 16 conditions, byte register/memory, CCR untouched) and TAS
  (N/Z from the original byte, V/C cleared, X preserved, bit 7 set).
  (TestM68KARM64_DifferentialSccGrid, TestM68KARM64_DifferentialTAS.)
- MOVEM in both directions, word and long, predecrement and postincrement,
  including the base-register-in-list edge cases. The register mask is a
  compile-time constant so the transfers are unrolled; the whole contiguous
  access span is guarded once (a full per-page I/O scan, m68kA64EmitRangeGuard)
  before any transfer so a bail leaves the instruction unexecuted.
  (TestM68KARM64_DifferentialMOVEM, TestM68KARM64_MOVEMIOBail.)

Correctness gate satisfied by admitted-and-interpreted fallback:

- Exceptions with per-instruction resume PC: TRAP, TRAPV, CHK, illegal,
  Line-A and Line-F are never admitted into a native block, so they terminate
  the supported prefix and are interpreted with the interpreter's exact
  exception delivery; any native block that faults mid-execution (I/O, bounds,
  stack bound, zero divide) bails with RetPC = faulting instruction. End-to-end
  TRAP/CHK/illegal delivery matches the pure interpreter byte for byte,
  including the supervisor stack frame. (TestM68KARM64_ExceptionDelivery,
  TestM68KARM64_FallbackAdmission.)
- Interrupt-boundary SR publication: the arm64 backend operates directly on
  cpu.DataRegs/cpu.AddrRegs and flushes the live CCR into cpu.SR in every
  block epilogue, so the returning dispatcher's boundary interrupt sampling
  observes the predecessor's flags exactly. (TestM68KARM64_BoundarySRPublication.)

Deferred to milestone 4 (optimisation / capability completion), each correct
by fallback today and documented in jit_m68k_exec_arm64.go:

- Register-count shift/rotate (Dn count): the runtime count with clamp/modulo
  and the count-zero CCR-preservation makes it an optimisation; fallback
  reproduces ExecShiftRotate exactly.
- 68020 long MULL/DIVL (32x32 -> 32/64 and 64/32): fallback.
- Native block chaining: on amd64 it is coupled to register pinning across the
  chain edge; the arm64 backend defers register pinning to milestone 4, so
  native chaining ports there. The interrupt-boundary SR-publication invariant
  it must preserve is already satisfied and tested. ChainCount stays zero.
- Exact-range native-exit SMC invalidation: the conservative guest-byte stamp
  recompiles on any change before a block is re-entered, so stale native code
  never runs across a dispatch boundary; the precise-range fast path is the
  optimisation refinement deferred to milestone 4.
- 68881 floating point on arm64: F-line instructions fall back to the
  interpreter's 68881. Note this is the ARM64 backend only. The amd64 M68020
  JIT has had native 68881 support with lazy FPSR for some time
  (see the "Current M68020 JIT state" section and jit_m68k_emit_amd64.go);
  native 68881 lowering on arm64 is milestone 4 work.

Gate status: the integer and control-flow differential grids are green on
arm64 under qemu-aarch64 (307 arm64 subtests). The Harte exception gate and
the AROS-boot-on-real-hardware gate for residency/control-flow changes remain
outstanding per the plan and are not satisfiable in qemu. m68kJitAvailable
stays false on arm64 until those hardware gates pass.
