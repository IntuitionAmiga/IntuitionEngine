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
