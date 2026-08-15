# M68020 JIT Compiler Technical Reference

## Overview

The M68020 JIT compiler translates basic blocks of 68020 machine code into
amd64 or arm64 native code, or into wasm functions in the browser. Each backend
scans a block, compiles and caches it, then runs it through its own dispatcher.

**Memory model (PLAN_MAX_RAM.md):** the M68K CPU is a flat 32-bit guest with explicit source-owned profile bounds. PC-fetch, prefetch, branch-target and stack-tune checks consult `cpu.profileTopOfRAM` (`profile_bounds.go`), not a fixed 32 MB constant. EmuTOS and AROS install their profile top via `cpu.SetProfileTopOfRAM` at boot from `EmuTOS_PROFILE_TOP` / `AROS_PROFILE_TOP`; tests that construct an `M68KCPU` without a loader inherit `len(memory)` as the default. Active visible RAM and total guest RAM are exposed through the SYSINFO MMIO pairs (`SYSINFO_ACTIVE_RAM_LO/HI`, `SYSINFO_TOTAL_RAM_LO/HI`) for guest-side discovery.

**Platform support:** amd64 on Linux, macOS and Windows; arm64 on Linux, macOS
and Windows; and js/wasm. The native backends use executable memory. The wasm
backend uses an M68020-specific module cache and reuses only the generic wasm
encoder. The arm64 backend has passed its real-hardware execution gate and is
available through the normal M68K JIT activation path.

**Activation:** The amd64 JIT is enabled by default when a CPU is created via
`NewM68KRunner()`. The arm64 backend is also enabled by default. The wasm
backend is enabled by default when `__goMem`
is available; `M68K_WASM_JIT=0` disables it. The `-nojit` CLI flag disables
native JIT execution, including the IE64 JIT.

## Architecture

```
                    M68KExecuteJIT() Loop
                           |
         +-----------------+-------------------+
         |                                     |
    [STOP handler]                    [Normal execution]
    pendingException                        |
    pendingInterrupt                   [Cache lookup]
    IPL comparison                     hit? → callNative()
    INTENA gating                      miss? ↓
    watchdog + Gosched             [m68kScanBlock()]
                                        |
                                   [m68kNeedsFallback?]
                                   yes → StepOne()
                                   no  ↓
                                   [m68kCompileBlock()]
                                        |
                                   [Cache + bitmap mark]
                                   [Patch chains (bidirectional)]
                                        |
                                   [callNative()]
                                        |
                                   [Chained execution: Block→Block→...]
                                   [via patchable JMP rel32]
                                        |
                                   [Return on budget exhaustion]
                                   [or NeedInval / NeedIOFallback]
                                        |
                              [Read RetPC/RetCount]
                              [NeedInval? → invalidate + clear RTS cache]
                              [NeedIOFallback? → StepOne()]
                              [Check interrupts/exceptions]
```

## Block Chaining

The JIT uses direct block-to-block chaining to eliminate Go dispatcher overhead between blocks. Each compiled block has two entry points:

- **Full entry** (`execAddr`): Called by `callNative()`. Pushes callee-saved registers, loads base pointers, falls through to chain entry.
- **Chain entry** (`chainEntry`): Lightweight entry for chained transitions. Reloads mapped registers and CCR from memory, but does NOT push callee-saved registers (they were pushed by the first block's full entry).

Block terminators with statically-known targets (BRA, JMP abs, JSR abs, BSR, Bcc external, DBcc external) emit chain exits instead of full epilogues:

1. Store mapped registers and merge CCR to SR (lightweight epilogue)
2. Accumulate instruction count into `ChainCount`
3. Decrement `ChainBudget` (initialised to 64); if exhausted → return to Go
4. Check `NeedInval`; if set → return to Go
5. Patchable `JMP rel32` to target block's chain entry

The `JMP rel32` is initially unchained (points to the unchained exit path). When the target block is compiled, the dispatcher patches the displacement to jump directly to the target's chain entry. Patching is bidirectional: new blocks patch existing blocks' exits, and their own exits are patched against already-cached targets.

### RTS Inline Cache

RTS uses an 8-entry MRU (most recently used) cache in M68KJITContext
(`RTSCache0PC` .. `RTSCache7Addr`). Before each `callNative()`, the dispatcher
shifts the entries and installs the current block's PC and chain entry at slot 0.

In RTS-emitted code, the popped return address is compared against the entries.
On hit, RTS chains directly to the matching chain entry. On miss, it returns to
the Go dispatcher. The cache is cleared on invalidation
(`m68kClearJITRTSCache`) and can be disabled via `m68kJITDisableRTSCache`.

### Interrupt Safety

The chain budget (64 blocks) limits how many blocks execute in a single native call before returning to Go for interrupt/exception checking. This amortises the Go overhead while ensuring responsive interrupt delivery.

## Lazy CCR (Condition Code Register)

The JIT defers CCR extraction from host EFLAGS into R14. After x86-64 arithmetic (ADD/SUB/CMP/NEG) and logical (AND/OR/EOR/TEST) operations, the host flags map directly to M68K conditions:

| M68K | x86 Jcc | M68K | x86 Jcc |
|------|---------|------|---------|
| BEQ | JE | BNE | JNE |
| BCS | JB | BCC | JAE |
| BMI | JS | BPL | JNS |
| BVS | JO | BVC | JNO |
| BGE | JGE | BLT | JL |
| BGT | JG | BLE | JLE |
| BHI | JA | BLS | JBE |

**Flag state tracking** at compile time:

- `flagsMaterialized`: R14 holds valid 5-bit CCR
- `flagsLiveArith`: EFLAGS live from ADD/SUB/NEG; X saved to `[RSP+24]`
- `flagsLiveLogi`: EFLAGS live from AND/OR/EOR/MOVE/TST; V=0, C=0 implicit

**Rules:**
1. After arithmetic op: save X (CF) to stack slot via `SETB [RSP+24]`, set `flagsLiveArith`
2. After CMP: set `flagsLiveArith` (X unchanged, stack slot untouched)
3. After logical op: set `flagsLiveLogi` (no emission)
4. Before Bcc/DBcc/Scc: use direct x86 Jcc (no SETcc extraction needed)
5. Before non-flag EFLAGS clobbers (LEA, PEA, LINK, UNLK, ADDA, SUBA): materialize R14
6. At block exit: materialize R14 before merging to SR

This eliminates ~12 instructions of SETcc/SHL/OR extraction per flag-setting instruction in common sequences like CMP;BEQ or ADD;DBRA.

## Layer Boundary (parity plan milestone 2)

The M68020 JIT separates three layers:

1. **Frontend (untagged, shared by all M68020 backends):** block scanner,
   instruction representation and native-admission predicates
   (`jit_m68k_common.go`, `jit_m68k_admission.go`), CCR liveness analysis
   (`jit_m68k_ccr_liveness.go`), region scanning and formation
   (`jit_region_scan_m68k.go`, `jit_m68k_region_form.go`), and the
   dispatch/tier policy (`jit_m68k_policy.go`: kill switches, warmup gating,
   fallback-burst sizing, interrupt-sample cadence, hotness and the
   retired-count contract). These files must stay free of build tags and
   emitter symbols.
2. **Per-backend emitters:** `jit_m68k_emit_amd64.go` plus the FPU emitter
   files own instruction lowering and region compilation
   (`m68kCompileRegion`).
3. **Per-backend execution/runtime:** `jit_m68k_exec.go` owns native
   executable memory, chain patching, native dispatch, and amd64 runtime
   policy. Native and wasm dispatch are deliberately not forced through one
   abstraction.

Dispatch seams are per target: `jit_m68k_dispatch.go` (amd64),
`jit_m68k_dispatch_arm64.go` (arm64),
`jit_m68k_dispatch_wasm.go` (js/wasm), and
`jit_m68k_dispatch_stub.go` (everything else), so `m68kJitAvailable` flips
per target without disturbing the amd64 path.

## File Inventory

### Implementation

| File | Build tag | Purpose |
|------|-----------|---------|
| `jit_m68k_common.go` | (none) | M68KJITContext, block scanner, instruction length calculator, native-admission predicates, leaf-fusion analysis, backward-branch detection |
| `jit_m68k_abi.go` | `amd64 && (linux \|\| windows \|\| darwin)` | The single source of truth for the amd64 register mapping |
| `jit_m68k_ccr_liveness.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Per-block CCR classification and liveness analysis |
| `jit_m68k_emit_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | x86-64 native code emitter: instructions, chain entry/exit, lazy CCR, SMC range checks, region builder/compiler |
| `jit_m68k_exec.go` | `amd64 && (linux \|\| windows \|\| darwin)` | JIT dispatcher: chain patching, budget management, RTS cache, helper exits, invalidation queue, region promotion, STOP/interrupt handling |
| `jit_m68k_fpu_sse_amd64.go` / `jit_m68k_fpu_ea_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Native 68881 SSE emitters (reg-to-reg and EA forms), FP pinning |
| `jit_m68k_fpu_pin_unix.go` / `jit_m68k_fpu_pin_windows.go` | amd64 unix / windows | Platform gate for xmm8-15 FP pinning |
| `jit_m68k_lockstep.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Runtime JIT-versus-interpreter lockstep harness |
| `jit_m68k_dispatch.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Routes `m68kJitExecute()` through JIT or interpreter |
| `jit_m68k_dispatch_stub.go` | all other platforms | Interpreter fallback for non-JIT platforms |
| `jit_region_backends.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Region walker registry, including `ScanRegionM68K` |
| `jit_common.go` | (none) | Shared: CodeBuffer, CodeCache, JITBlock, chainSlot (reused from IE64) |
| `jit_call.go` | shared IE64 JIT trampoline | `callNative()` via `runtime.cgocall` (reused from IE64) |
| `jit_mmap.go` / `jit_mmap_darwin_amd64.go` / `jit_mmap_windows.go` | Linux / macOS amd64 / Windows | Executable memory allocator + `PatchRel32At` (reused from IE64) |

### Tests

| File | Build tag | Purpose |
|------|-----------|---------|
| `jit_m68k_common_test.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Instruction length, block scanner, liveness, terminators, chain infrastructure |
| `jit_m68k_emit_amd64_test.go` | `amd64 && (linux \|\| windows \|\| darwin)` | x86-64 emitter unit tests (individual instruction verification) |
| `jit_m68k_exec_test.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Integration tests through full JIT dispatcher |
| `m68k_jit_benchmark_test.go` | `amd64 && linux` | JIT vs interpreter comparative benchmarks (ALU, MemCopy, Call) |

## M68KJITContext Layout

The authoritative layout is the `M68KJITContext` struct in
`jit_m68k_common.go` and its `m68kCtxOff*` offset constants; the emitter
consumes only those constants, so the doc does not duplicate the offsets.
Notable fields: register-file and memory base pointers, `SRPtr`, `CpuPtr`,
`NeedInval` plus the exact-range pair `InvalAddr`/`InvalSize`,
`NeedIOFallback`, `RetPC`/`RetCount`, the code page bitmap pointer, the IO
page bitmap pointer/length, `ChainBudget`/`ChainCount`, and the 8-entry RTS
cache (`RTSCache0PC` .. `RTSCache7Addr`).

## Register Mapping (x86-64)

The mapping is defined once in `jit_m68k_abi.go` and must never be re-derived
inline.

| x86-64 | M68K | Notes |
|--------|------|-------|
| RBX | D0 | Callee-saved, mapped |
| RBP | D1 | Callee-saved, mapped |
| R12 | A0 | Callee-saved, mapped |
| R13 | A7/SP | Callee-saved, mapped |
| R14 | CCR | Callee-saved, 5-bit XNZVC (lazy: may be stale when EFLAGS live) |
| R15 | - | JITContext pointer |
| RDI | - | &DataRegs[0] (AddrRegs at a fixed delta) |
| RSI | - | &cpu.memory[0] |
| R8 | A6 | Pinned (freed by removing the IOThreshold pin) |
| R9 | A5 | Pinned (freed by removing the AddrBase pin) |
| RAX,RCX,RDX,R10,R11 | - | Scratch |

Loop blocks containing native FP additionally pin FP0-FP7 to xmm8-xmm15
(`m68kFPPinned`, `jit_m68k_fpu_sse_amd64.go`) on non-Windows hosts only;
Windows treats xmm8-15 as callee-saved so pinning stays off there
(`jit_m68k_fpu_pin_unix.go` / `jit_m68k_fpu_pin_windows.go`).

Stack frame: 40 bytes (`[RSP+0]`=ctx backup, `[RSP+8]`=SR pointer, `[RSP+16]`=loop counter, `[RSP+24]`=X flag byte for lazy CCR).

## CCR (Condition Code Register)

| Bit | Flag | Updated by |
|-----|------|------------|
| 0 | C (Carry) | ADD, SUB, CMP, NEG, shifts |
| 1 | V (Overflow) | ADD, SUB, CMP, NEG |
| 2 | Z (Zero) | All flag-modifying instructions |
| 3 | N (Negative) | All flag-modifying instructions |
| 4 | X (Extend) | ADD, SUB, NEG, shifts (X=C) |

With lazy CCR, extraction into R14 is deferred until needed. The X flag is saved to `[RSP+24]` by arithmetic ops; logical ops leave it unchanged. Materialization happens at block exits and before non-flag EFLAGS-clobbering instructions.

## Memory Access

- **Fast path**: RAM addresses use direct `[memBase + addr]` with BSWAP for big-endian conversion.
- **I/O classification**: an IO page bitmap (`m68kCtxOffIOPageBitmapPtr/Len`, scanned inline by emitted code) replaces the old fixed 0xA0000 `IOThreshold` cutoff. Accesses that hit an IO page either take an MMIO helper exit (`m68kExecuteJITMMIOMOVEHelper`, `m68kExecuteJITMMIOCLRHelper`) or set `NeedIOFallback=1` and return to the dispatcher for `StepOne()`.

## Self-Modifying Code Detection

Uses a heap-allocated code page bitmap (`(memSize+4095)>>12` bytes, 4KB pages). When a block is cached, its pages are marked in the bitmap. Store instructions in JIT-compiled code check the bitmap after each write. Writes to code pages record an exact invalidation range in `ctx.InvalAddr`/`ctx.InvalSize` (`m68kEmitSMCRangeBailChecks`); on return the dispatcher invalidates only that range via `m68kInvalidateJITCodeRange`, falling back to a full cache flush only when no range is available (size 0). Cross-thread writes are queued via `m68kEnqueueJITInvalidation`, coalesced (`m68kCoalesceInvalRanges`), and drained at dispatch boundaries. The RTS cache is cleared on invalidation.

## Backward Branch Optimisation

DBRA/Bcc loops targeting earlier instructions within the same block execute as native x86-64 backward jumps, avoiding dispatcher re-entry overhead. A budget counter (4095 iterations) limits execution before returning to the dispatcher for interrupt checking and GC safety.

## Supported Instructions

Native admission is decided per instruction by the `m68kIsNativeSupported*`
predicate family in `jit_m68k_common.go` (roughly 90 predicates), gated by
`m68kNeedsFallback` and `m68kNeedsConservativeFallback`. Native coverage
includes the integer core (MOVE all sizes, MOVEQ, ALU group, shifts, rotates,
CLR, TST, SWAP, EXT/EXTB, Scc, ADDQ/SUBQ, LEA, PEA, LINK/UNLK, ADDA/SUBA,
NOP), all static and conditional control flow (BRA, BSR, Bcc, DBcc, JSR, JMP,
RTS), plus MOVEM, MULU/MULS/MULL, DIVU/DIVS/DIVL, the bitfield group (BFTST,
BFEXTU, BFEXTS, BFFFO and the write forms), PACK/UNPK, NBCD, TAS, CHK,
MOVE to/from SR/CCR, and native 68881 FPU (see below).

## Addressing Modes

Dn, An, (An), (An)+, -(An), (d16,An), (d8,An,Xn) brief, abs.W, abs.L, (d16,PC), (d8,PC,Xn) brief, #imm.

Unsupported modes (68020 full format with memory indirection) bail to interpreter.

## 68881 FPU

FPU support is native, not fallback:

- Register-to-register and EA forms are decoded by
  `m68kDecodeNativeFPURegToReg` / `m68kDecodeNativeFPUEA`
  (`jit_m68k_common.go`). On amd64 they are emitted as SSE scalar code by
  `jit_m68k_fpu_sse_amd64.go` and `jit_m68k_fpu_ea_amd64.go`. On arm64 the
  supported register forms are emitted as native scalar ARM64 floating-point
  instructions by `jit_m68k_emit_arm64.go`.
- FBcc is emitted natively.
- Lazy FPSR: intermediate FPSR condition-code updates are elided when the
  next FPU instruction overwrites them without an observable fault point
  (`m68kFPUNextInstrOverwritesCCNoFault`).
- Transcendentals (and other non-mapping operations) are not compiled;
  blocks containing them run through the interpreter burst path
  (`m68kFPUInstrIsTranscendental`, `m68kInterpretTranscendentalBurst`).
- Remaining FPU cases exit through the FPU helper
  (`m68kExecuteJITFPUHelper`).

## Region Formation

Hot blocks are promoted into multi-block regions (default on,
`m68kJITDisableRegions` kill switch). The region walker is `ScanRegionM68K`
(`jit_region_backends.go`); the builder and compiler are `m68kFormRegion` /
`m68kCompileRegion` (currently in `jit_m68k_emit_amd64.go`). Promotion is
triggered by block hotness (`m68kTryPromoteJITRegion`). Regions reject
fused-leaf blocks, keep conservative CCR behaviour across member blocks, and
patch internal chain exits to local labels.

## JSR Leaf-Call Fusion

Short leaf subroutines called by JSR are fused inline
(`m68kAnalyzeJSRLeafFusion`, `m68kIsLeafFusionSafe`,
`jit_m68k_common.go`): fused push, inline body, synthetic return, with an
I/O bailout and architecturally correct stack effects.

## Bail to Interpreter

The following fall back to the interpreter via `StepOne()` or a fallback
burst:

- FPU transcendentals, extended/packed formats, and any FPU form without a
  native decode
- Line A traps (0xAxxx)
- STOP, RTE, RTR, RESET, TRAP, TRAPV
- MOVEC, MOVES, CAS, CAS2
- ABCD, SBCD
- CHK2, CMP2, CALLM, RTM
- MOVEP, BKPT
- Any instruction using 68020 full-format addressing (memory indirect)

## Differential and Lockstep Verification

The JIT carries a runtime lockstep harness (`jit_m68k_lockstep.go`:
`m68kJITLockstepSession/Snapshot/Boundary`) and an interpreter verify
pre-pass (`m68kVerifyInterpPrePass`, `m68kReportVerifyDivergence`).
Retired-instruction accounting (`m68kJITRetiredInstructionCount`,
ChainCount + RetCount contract) is validated differentially against
interpreter event counts (`jit_m68k_count_accounting_test.go`), alongside
the large differential suite (`jit_m68k_differential_test.go`) and the
Harte parity gate (`jit_m68k_harte_parity_test.go`).

## Benchmark Results

Intel i5-8365U @ 1.60 GHz, Go 1.26, `go test -tags headless -bench BenchmarkM68K_`:

| Workload | Interpreter | JIT | Speedup |
|----------|-------------|-----|---------|
| ALU (MOVEQ+ADD+SUB+AND+OR+ADDQ+SWAP in DBRA loop) | 729 us | 40 us | **18.1x** |
| MemCopy (MOVE.L (A0)+,(A1)+ in DBRA loop) | 264 us | 52 us | **5.1x** |
| Call (JSR+RTS in loop) | 389 us | 421 us | 0.9x |

Block chaining eliminates Go dispatcher overhead for JSR/RTS/BRA/JMP, bringing Call from 0.09x (pre-chaining) to near-parity. Lazy CCR eliminates ~12 instructions of flag extraction per flag-setter, giving the 18x ALU speedup.

## Host W^X

The M68K JIT shares the `jit_mmap.go` dual-mapped executable memory
with every other JIT backend. Emit and patch operations run through
the writable view (`PROT_READ|PROT_WRITE`); dispatch runs through the
execution view (`PROT_READ|PROT_EXEC`). At no point does either view
hold both write and execute permission. See
[`IE64_JIT.md`](IE64_JIT.md) for the full model and test contract.
