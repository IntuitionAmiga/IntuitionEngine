# IE64 JIT Compiler

Technical reference for the IE64 Just-In-Time compiler. Covers the shared
infrastructure, dispatcher, native ARM64 and x86-64 backends, and the browser
wasm backend.

### Loop specialisations

Two conservative loop specialisations are implemented for amd64, ARM64 and
wasm, across single blocks and promoted regions. The first hoisted deduplicated
bounds and MMIO proofs for invariant `LOAD`, `STORE`, `DLOAD` and `DSTORE`
addresses, while retaining every store-side SMC probe. It rejected MMU, stack,
changing-pointer and alternate-entry loops. The second removed the back-edge
budget comparison only for an immediate-seeded, non-zero `SUB.Q` and `BNE`
counter loop whose integer-only body and exact retired count fitted within
`jitBudget`. Both matchers selected at most one loop per compiled unit.

The specialisations also separate ordinary memory fallback value 1 from speculative
precheck fallback value 2. A failed precheck returned to the loop head with the
prefix retirement count, then the dispatcher interpreted one instruction
without recording an MMIO deoptimisation. This protocol is internal to the JIT.

Qualifying loops use the specialised paths on native and wasm backends. MMU
loops, changing pointers and stack accesses retain the established checks.

---

## Overview

The IE64 JIT compiler translates blocks of IE64 machine code into native ARM64 or x86-64 instructions at runtime, executing them directly on the host CPU. This bypasses the Go interpreter loop and yields significant performance improvements for compute-heavy workloads.

The IE64 JIT is fully 64-bit. The block builder, return channel, PC, data and stack addresses, branch targets, and chain targets are all `uint64`; there is no `uint32` truncation. High virtual/physical PCs are scanned and compiled: `scanBlockBus` fetches instruction words through `bus.ReadPhys64WithFault` when the physical address is outside the low `cpu.memory` window, and stops cleanly on an unmapped page. High-address and MMU-on data, FP, and control-flow memory operations, plus unfused stack operations, route through the JITContext helper-exit protocol rather than bailing the whole instruction; the amd64 non-MMU fused JSR/RTS leaf high-SP case is the stack exception because it raw-indexes `[MemBase+SP]` before those guards (see "IE64 JIT 64-bit Execution Model" in `architecture.md` for the full contract). `DLOAD`/`DSTORE` use native low-window fast paths and helper exits for MMU/high/MMIO cases. FP64 transcendentals (`DSIN` through `DPOW`) compile to a helper-exit path that calls the same FPU methods as the interpreter, writes FP register pairs and FPSR, and resumes at the next PC. The remaining interpreter fallbacks are: atomics outside aligned non-MMU low-window RAM, fused JSR/RTS leaves under MMU (`compileBlockMMU` sets `mmuBail` for `emitBailToInterpreter`), MMU/privilege opcodes, FP32 transcendentals, the double-precision arithmetic/conversion opcodes neither backend emits (see "Category D: Double precision" below; amd64 and arm64 both emit the FP64 core natively and bail only for `DMOD`/`DABS`/`DNEG`/`DSQRT`/`FCVTSD`/`FCVTDS`), and any block *fetched from* a high physical PC that itself contains a stack op (`PUSH`/`POP`/`JSR`/`RTS`/`JSR_IND`). The high-PC stack-op restriction prevents the fused raw stack path from indexing beyond `cpu.memory[]` when SP is high. The low `cpu.memory[]` window is `min(autodetected total guest RAM, busMemCap)`. IE64-family modes cap this window at `busMemMaxBytes`, which is `0xFFFF0000`, rather than the complete 32-bit address range. The excluded top 64 KiB begins the M68K sign-extended alias window. Non-mmap hosts may clamp the window further. Addresses above the low window cover the guest's full active visible RAM through the bus / `Backing` interface, so JIT-executed code reaches the same address space the interpreter sees.

**Supported platforms:** ARM64/Linux, ARM64/macOS, ARM64/Windows, x86-64/Linux, x86-64/macOS, x86-64/Windows (x86-64 requires SSE4.1; release builds target x86-64-v3)

**Activation:** JIT is enabled by default on supported platforms. Disable with the `-nojit` flag.

---

## Architecture

```
IE64 Machine Code (at PROG_START)
        |
        v
  scanBlock()             jit_common.go    Block detection (up to 256 instructions)
        |
        v
  analyzeBlockRegs()      jit_common.go    Register liveness analysis
        |
        v
  compileBlock()          jit_emit_{arch}.go   Platform-specific code emission
        |
        v
  ExecMem.Write()         jit_mmap_*.go    Copy code into the platform executable-memory mapping
        |
        v
  CodeCache.Put()         jit_common.go    Cache by dispatcher key for O(1) lookup
        |
        v
  callNative()            jit_call.go      Execute via runtime.asmcgocall
        |
        v
  Dispatcher read         jit_exec.go      Read RetPC (uint64) + RetCount from JITContext
```

### Selected File Inventory

This table lists the main entry points and platform files. It is not an
exhaustive list of JIT source files.

| File | Build Tag | Purpose |
|------|-----------|---------|
| `jit_common.go` | (none) | JITContext, CodeBuffer, block scanner, register analysis, code cache |
| `jit_exec.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Dispatcher loop (`ExecuteJIT`), timer handling |
| `jit_call.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | `callNative` via `runtime.asmcgocall` plus darwin exec/write protection hooks |
| `jit_call_arm64.s` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 trampoline (`R0` receives `*jitCallArgs`; native block receives `JITContext*`) |
| `jit_call_amd64.s` | `amd64 && (linux \|\| darwin)` | SysV x86-64 trampoline |
| `jit_call_amd64_windows.s` | `amd64 && windows` | Windows x86-64 trampoline |
| `jit_emit_arm64.go` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 code emitter, including single blocks and regions |
| `jit_emit_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | x86-64 code emitter, including single blocks and regions |
| `jit_mmap.go` | `(amd64 \|\| arm64) && linux` | Linux dual-mapped executable memory (RW view + RX view) |
| `jit_mmap_windows.go` | `(amd64 \|\| arm64) && windows` | Windows executable memory backend |
| `jit_mmap_darwin_amd64.go` | `darwin && amd64` | macOS x86-64 executable memory backend |
| `jit_mmap_darwin_arm64.go` | `darwin && arm64` | macOS `MAP_JIT` executable memory backend |
| `jit_icache_arm64.go` | `arm64 && linux` | ARM64 icache flush (DC CVAU + IC IVAU) |
| `jit_icache_arm64_darwin.go` | `arm64 && darwin` | macOS arm64 icache invalidation via libSystem |
| `jit_icache_arm64_windows.go` | `arm64 && windows` | Windows ARM64 icache invalidation via `FlushInstructionCache` |
| `jit_icache_arm64.s` | `arm64 && linux` | ARM64 icache flush assembly |
| `jit_icache_amd64.go` | `amd64 && linux` | x86-64 icache no-op (coherent architecture) |
| `jit_icache_amd64_darwin.go` | `amd64 && darwin` | macOS x86-64 icache no-op |
| `jit_icache_amd64_windows.go` | `amd64 && windows` | Windows x86-64 icache no-op |
| `jit_dispatch.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Routes to `ExecuteJIT()` when enabled |
| `jit_dispatch_stub.go` | all other platforms | Fallback: always uses interpreter |
| `jit_common_amd64.go` | `amd64` | Enables IE64 JSR leaf fusion markers for the AMD64 emitter |
| `jit_common_other.go` | `!amd64` | Disables IE64 JSR leaf fusion markers for non-AMD64 emitters |
| `jit_abi_common.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Shared canonical JIT ABI registry, including IE64 AMD64 register pins |
| `jit_flags_common.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Shared lazy-flag and flag-liveness types used by backend analyses |
| `jit_ie64_abi.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Canonical AMD64 IE64 register-pinning constants and ABI consistency scaffold |
| `jit_ie64_flags_liveness.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Conservative IE64 flag-liveness scaffold for future region allocation work |
| `jit_tier_common.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Shared hot-block promotion policy (`TierController`) |
| `jit_tier_backends.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Per-backend no-op tier allocator registry; IE64 promotion is region-driven |
| `jit_region_common.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Shared region/superblock budget profile, including `IE64RegionProfile` |
| `jit_region_backends.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Region scanners, including `ScanRegionIE64` |
| `jit_chain_ordering.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Advisory chain-slot ordering invariant for AMD64 backends |
| `jit_ie64_region_policy.go` | `amd64 && (linux \|\| windows \|\| darwin)` | IE64 region-tier policy, statistics, and planning metadata |
| `jit_ie64_region_policy_stub.go` | non-amd64 | Linux ARM64 region policy and inactive stubs for unsupported hosts |
| `jit_ie64_observed_region.go` | (none) | Observed conditional and register-indirect path recording and validation |
| `jit_ie64_loop_analysis.go` | (none) | Invariant-memory and bounded-counter loop analysis |
| `jit_ie64_fpsr_liveness.go` | `(amd64 \|\| arm64) && (linux \|\| windows \|\| darwin)` | Native FPSR condition-code liveness analysis |
| `jit_ie64_fpu_residency.go` | (none) | FP32 and FP64 residency planning and eligibility |
| `jit_helper_resume_common.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Native helper continuation policy |
| `jit_mmio_poll_common.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Shared MMIO-poll loop matcher |
| `jit_mmio_poll_backends.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Per-backend MMIO-poll pattern descriptors, including IE64 |
| `jit_mmio_poll_wiring.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Runtime MMIO-poll predicate wiring from each CPU/bus |
| `jit_mmio_poll_exec_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | AMD64 MMIO-poll execution helpers, including IE64 |
| `jit_mmio_poll_exec_native.go` | `(amd64 \|\| arm64) && (linux \|\| windows \|\| darwin)` | Shared native IE64 MMIO-poll execution |
| `jit_fastpath_bitmaps.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Shared bitmap-probe shape metadata used by AMD64 emitters |
| `jit_fastpath_backends.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Audit registry for per-backend fast-path bitmap usage, including IE64 |
| `jit_exec_protect_darwin_arm64.go` | `darwin && arm64` | macOS arm64 `MAP_JIT` write-protection transitions |
| `jit_exec_protect_stub.go` | `!(darwin && arm64)` | No-op executable-memory protection hooks |
| `jit_wasm_runtime.go` | `js && wasm` | Browser wasm JIT runtime and module cache |
| `jit_wasm_ie64_emit.go` | (none) | IE64 wasm block and structured-region emitter |
| `jit_wasm_encoder.go` | (none) | wasm binary encoder used by the browser backend and host tests |
| `jit_exec_wasm.go` | `js && wasm` | Browser dispatch between the wasm JIT and interpreter |
| `jit_mmio_poll_exec_wasm.go` | `js && wasm` | Cooperative wasm MMIO poll service |

---

## Shared Infrastructure

### JITContext

Bridge between Go and native code. Passed as the sole argument to every JIT block.

The offsets below are mirrored as `jitCtxOff*` constants in `jit_common.go` and
verified against `unsafe.Offsetof` by `TestJITContext_*Offset` tests; treat
`jit_common.go` as the source of truth.

```
Offset  Type      Field            Description
0       uintptr   RegsPtr          &cpu.regs[0]
8       uintptr   MemPtr           &cpu.memory[0]
16      uint32    MemSize          len(cpu.memory)
20      uint32    IOStart          IO_REGION_START
24      uintptr   PCPtr            &cpu.PC
32      uintptr   LoadMemFn        (reserved)
40      uintptr   StoreMemFn       (reserved)
48      uintptr   CpuPtr           &cpu
56      uint32    NeedInval        Cache invalidation flag
60      uint32    NeedIOFallback   I/O bail flag
64      uintptr   IOBitmapPtr      &cpu.bus.ioPageBitmap[0]
72      uintptr   FPUPtr           &cpu.FPU
80      uint32    ChainBudget      Chained block-transition budget
84      uint32    ChainCount       Retired count accumulated while chaining
88      uint64    RTSCache0PC      MRU RTS target PC 0 (full 64-bit)
96      uintptr   RTSCache0Addr    MRU RTS target native entry 0
104     uint64    RTSCache1PC      MRU RTS target PC 1
112     uintptr   RTSCache1Addr    MRU RTS target native entry 1
120     uint64    RTSCache2PC      MRU RTS target PC 2
128     uintptr   RTSCache2Addr    MRU RTS target native entry 2
136     uint64    RTSCache3PC      MRU RTS target PC 3
144     uintptr   RTSCache3Addr    MRU RTS target native entry 3
152     uint64    RetPC            Next PC after block exit (full 64-bit)
160     uint32    RetCount         Retired instruction count for the exiting block
164     uint32    MMUEnabled       1 when MMU translation is active for the next block
168     uint32    NeedHelper       Helper opcode (HELPER_*; 0 = none)
172     uint32    HelperSize       IE64_SIZE_B/W/L/Q for memory ops
176     uint32    HelperRd         Destination/source register or FP-register index
184     uint64    HelperAddr       Virtual address (data ops) or call target (control flow)
192     uint64    HelperVal        Store/push value (input only); LOAD/POP -> integer reg via setReg, FLOAD/DLOAD -> FPU via FP setters. Never written back here
200     uint64    HelperPC         PC of the requesting instruction (for trapFault.faultPC)
208     uint64    LiveSP           SP flushed from the host register before helper exit
216     uintptr   ResumeAddr       Native continuation entry
224     uint64    ResumePC         Expected guest PC after helper completion
232     uint64    ResumePTBR       PTBR captured before native entry
240     uint32    ResumeCountBase  Instructions retired before the continuation
244     uint32    ResumeMMUEnabled MMU mode captured by the helper exit
248     uint32    ResumeValid      Non-zero when the continuation is usable
256     uint64    MicroTLBReadPrefix  Read-access key prefix for the native micro-TLB
264     uint64    MicroTLBWritePrefix Write-access key prefix for the native micro-TLB
272     [4]uint64 MicroTLBKeys     Four valid-prefixed virtual page keys
304     [4]uint64 MicroTLBPhys     Four translated physical page bases
336     uintptr   CodePageBitmapPtr  Low virtual code-page bitmap
344     uint64    InvalAddr        Start of a self-modifying write
352     uint32    InvalSize        Size of a self-modifying write
356     uint32    CodePageBitmapLen  Bounds the low virtual code-page bitmap
360     uint64    CodeHighStartPage  First marked high virtual code page
368     uint64    CodeHighEndPage    Last marked high virtual code page
376     uintptr   PhysCodeBitmapPtr  Low physical code-page bitmap
384     uint32    PhysCodeBitmapLen  Bounds the physical code-page bitmap
392     uintptr   CodePageSpansPtr   Per-page compiled-byte spans used by wasm stores
```

### Block Scanner

`scanBlock()` decodes IE64 instructions starting at a given physical PC until a block terminator is found or 256 instructions are reached. Terminators are included in the returned block. Current terminators are BRA, JMP, JSR64, RTS64, JSR_IND, HALT64, RTI64, WAIT64, all MMU/privilege opcodes (SYSCALL, ERET, MTCR, MFCR, TLBFLUSH, TLBINVAL, SMODE, SUAEN, SUADIS), and all atomic RMW opcodes (CAS, XCHG, FAA, FAND, FOR, FXOR).

`scanBlockBus()` / `scanBlockBusWithLimit()` are the 64-bit-aware fetch path: when the physical PC is outside the low `cpu.memory` window they read each instruction word through `bus.ReadPhys64WithFault`, stop cleanly when a page is unmapped (`ok == false`), and use the subtraction-form bound so a PC in the last bytes of `uint64` space does not wrap into a low page.

On AMD64 only, the scanner may replace a small register-only JSR leaf with fused markers plus the leaf body. The ARM64 scanner gate disables that fusion because its emitter does not honour those markers.

### Register Liveness

`analyzeBlockRegs()` computes bitmasks of which IE64 registers are read and
written by the block. ARM64 uses the combined read/write mask to load and
preserve only mapped registers used by the block, while loading SP
unconditionally. AMD64 loads every resident Tier 1 or region register and SP
unconditionally. Both backends use the write mask to limit normal exit
writeback, widened where loops or cumulative region state require it.

### CodeBuffer

Variable-length byte buffer with label/fixup support for forward references:
- `EmitBytes()` / `Emit32()` / `Emit64()` for code emission
- `Label()` / `FixupRel32()` / `Resolve()` for forward jump patching
- `PatchUint32()` for inline patching

### CodeCache

Maps a dispatcher key to `*JITBlock` for O(1) lookup. In non-MMU mode the key is the physical `startPC`; in MMU mode the cache uses `GetMMU`/`PutMMU` with the **exact** composite key `ie64CacheKey{ptbr, pc}` (not a lossy hash), described in [MMU Integration](#mmu-integration). Self-modifying-code checks first use 256-byte code-page marks as a coarse prefilter, then validate known nonzero write ranges against cached `JITBlockCoveredRanges` before invalidating. MMU virtual checks are scoped to the current `PTBR`; physical aliases still use the conservative whole-cache path.

### Region Tier

On AMD64 and Linux ARM64, hot IE64 blocks can be promoted from Tier 1 single-block JIT code to a compiled region. The dispatcher increments `JITBlock.execCount` on cache hits and asks the shared `TierController` whether the block is hot enough to promote. The default threshold is 64 re-entries, with promotion suppressed when the block is already promoted, was already attempted, or has an I/O-bail rate of 25% or higher.

`ie64FormRegion()` scans `cpu.memory` at flat physical indices and follows statically-known BRA/JMP terminators. Under MMU each virtual successor needs its own page-table walk before the scanner can read the correct physical bytes, so that case has a separate page-bounded former, `ie64FormRegionMMU()`. The dispatcher picks between them on `cpu.mmuEnabled`.

Both native region compilers emit one `JITBlock` for two or more IE64 blocks. Internal BRA/JMP targets transfer directly inside the region; external targets use the normal exit machinery. Back edges retain loop-budget and retired-count checks so native code cannot spin without returning to the dispatcher. AMD64 regions bind their four hottest guest registers to RBX, RBP, R12 and R13. Linux ARM64 regions retain the fixed R1-R14 mapping in X12 through X17 and X19 through X26, never X18. Dirty GPR and floating-point state is retained across internal edges and spilled at helper, bail and dispatcher exits. MMU region formation remains opt-in on both native backends.

Environment gates:

| Variable | Default | Effect |
|----------|---------|--------|
| `IE64_JIT_REGIONS` | on | `0`/`false`/`off`/`no` disables region promotion entirely |
| `IE64_JIT_REGION_MMU` | off | `1`/`true`/`on`/`yes` enables MMU region formation |
| `IE64_JIT_REGION_MAX_SPILLS` | no ceiling | amd64 spill-pressure ceiling for accepting a region plan |
| `IE64_JIT_STATS` | off | On amd64, `1` prints region and spill statistics |
| `IE64_JIT_RESUME` | on | `0`/`false`/`off`/`no` disables native continuation after a clean helper exit |
| `IE64_JIT_FP_RESIDENCY` | off | `1` enables eligible FP32 and FP64 residency on Linux/macOS amd64 and ARM64 |

Linux ARM64 enables non-MMU region promotion by default. Other ARM64 operating systems retain the single-block tier.

### ExecMem

16 MB executable-memory pool with a 16-byte-aligned bump allocator. Full cache
invalidations reset the pool. Range-based SMC invalidation resets it only when
the invalidation removes the final cached block.

Linux uses a W^X-safe dual mapping:

- **Linux backing pages** are created once via `memfd_create` (with
  `MFD_EXEC|MFD_CLOEXEC` where available, falling back to plain
  `MFD_CLOEXEC` on hardened kernels that reject `MFD_EXEC`).
- **Writable view** (`PROT_READ|PROT_WRITE`, no execute) is used by
  `ExecMem.Write`, `CodeBuffer.PatchUint32`, and `PatchRel32At`. This
  is where emit and patch operations target.
- **Execution view** (`PROT_READ|PROT_EXEC`, no write) is used by
  `callNative` for dispatch. At no point does any view hold both
  `PROT_WRITE` and `PROT_EXEC`.
- **Icache flush** on ARM64 splits `DC CVAU` against the writable
  VA (where the new bytes were actually deposited) and `IC IVAU`
  against the execution VA (which is the instruction path the CPU
  will refetch from). On x86-64 the icache is coherent with stores
  and no flush is needed.
- **`PatchRel32At`** takes an address in the writable view.
  Attempting the same store through the execution-view address
  faults, which is the invariant tested in `jit_mmap_test.go`.

Code that mutates generated code stays within
the writable view for all mutation and within the execution view
for all dispatch. The two views alias the same backing pages, so
an emit through the writable view is immediately visible to the
CPU fetch through the execution view after the icache flush.

On macOS amd64, the allocator uses a simple executable mapping shared by the x86-64 backends, and the icache hooks remain no-ops. On macOS arm64, the allocator uses a single `MAP_JIT` mapping instead of Linux-style dual views. Writes are bracketed by `pthread_jit_write_protect_np(false/true)` on a locked OS thread, and instruction cache invalidation is handled through `sys_icache_invalidate`.

---

## Return-Channel Contract

Every JIT block exit writes two **dedicated** `JITContext` fields - a full 64-bit next PC and a 32-bit retired-instruction count. This replaced the legacy `regs[0]`-packed `nextPC | (count << 32)` format, which truncated the PC to 32 bits.

```
ctx.RetPC    uint64   // next PC after the block exit (full 64-bit)
ctx.RetCount uint32   // retired instruction count for the exiting block
```

The dispatcher (`jit_exec.go`) reads them directly after `callNative`:
```go
cpu.PC = cpu.jitCtx.RetPC
executed := uint64(cpu.jitCtx.RetCount)
```

Native emitters load `RetPC` into the PC host register (`R15`/`X28`) as a full 64-bit immediate, so block exits and branch/JSR targets above `0xFFFFFFFF` round-trip without truncation. Chain transitions accumulate their predecessor counts into `ctx.ChainCount` (added by the dispatcher), so a chain entry no longer extracts a count from the PC register.

**Every exit path must set `RetPC` and the count:**
- Normal block end: `staticCount = len(instrs)`
- Branch taken: `staticCount = instrIdx + 1`
- I/O bail: `bailCount = ji.pcOffset / IE64_INSTR_SIZE`
- Backward-branch budget exit: dynamic count from loop counter
- Chained block exits: predecessor counts accumulated in `JITContext.ChainCount` are added by the dispatcher after `callNative` returns.

**Important distinction for bail paths:**
- `bailCount` (retired instruction count) goes into `RetCount` (via the count argument)
- `writtenSoFar` (register bitmask) goes into `emitEpilogue` to control which registers are stored back
- These are two unrelated values

---

## ARM64 Backend

### Register Mapping

```
ARM64    IE64    Purpose
X0       --      JITContext* (entry), scratch after prologue
X1-X4    --      Scratch
X5       --      &ioPageBitmap[0]
X6       --      &cpu.FPU (if hasFPU)
X7       --      Loop counter (if hasBackwardBranch)
X8       --      &cpu.regs[0] (register file base)
X9       --      &cpu.memory[0] (memory base)
X10      --      IO_REGION_START
X11      --      Scratch
X12-X17  R1-R6   Mapped IE64 registers
X18      --      Never allocated (AAPCS64 platform register)
X19-X26  R7-R14  Mapped IE64 registers (callee-saved)
X27      R31     IE64 SP (always resident)
X28      --      IE64 PC / return channel
XZR      R0      Hardwired zero
X29/X30  --      Go FP/LR (saved/restored)
```

14 IE64 registers are resident in ARM64 registers. R15-R30 are spilled to the register file in memory.

X18 is the AAPCS64 platform register and is never allocated: Darwin reserves it for the kernel and Windows/ARM64 keeps the thread environment block pointer in it. Excluding it costs one resident slot, which is why R15 spills. `ie64ToARM64Reg` is the single source of truth for the mapping; `arm64CalleeSavedPairs` names the IE64 registers living in each callee-saved pair, and `TestARM64RegMap_CalleeSavedPairsMatchMapping` pins the two together.

### Prologue/Epilogue

Fixed 112-byte frame. Saves/restores callee-saved pairs selectively based on register usage.

### Backward Branch Budget

Uses X7 as iteration counter. Budget = 4095 (fits ARM64 CMP imm12). Budget exceeded -> exit block, let dispatcher reset timer.

### Icache Flush

Required on ARM64. Uses DC CVAU + IC IVAU + DSB ISH + ISB per 64-byte cache line.

### Region Promotion

Linux ARM64 promotes hot non-MMU code to native regions by default and supports
MMU regions when `IE64_JIT_REGION_MMU=1`. Region formation follows static BRA
and JMP successors, with limits of 8 blocks and 512 guest instructions. Other
ARM64 operating systems retain the single-block tier.

For MMU-off low-memory code, a hot entry with a conditional branch to a target
outside its Tier 1 block records the executed block path. Recording runs one
compiled block at a time, accepts between two and eight distinct blocks, and
stops on loop closure. The recorded predecessor ends immediately after the
conditional. Its taken edge enters the recorded successor while the untaken
edge exits to the following instruction, so the discarded cold tail remains
available to ordinary dispatch. A valid static region is rebuilt and installed
if recording is rejected and no invalidation occurred.

An entry ending in `JMP rs, displacement`, where `rs` is not R0, uses the same
recording path. The region compares the computed target with the single
recorded target. A match keeps resident GPR state live across the internal
edge. Eligible FP state also remains resident when `IE64_JIT_FP_RESIDENCY=1`;
otherwise the FP register file remains memory-backed. A mismatch uses the
normal dynamic-jump exit with the computed 64-bit PC and exact retired count.

---

## x86-64 Backend

### Register Mapping

```
x86-64   IE64    Purpose                    Persistence
RDI      --      &cpu.regs[0] (reg base)    dedicated
RSI      --      &cpu.memory[0] (mem base)  dedicated
R8       --      IO_REGION_START            dedicated
R9       --      &ioPageBitmap[0]           dedicated
RAX      --      Scratch                    caller-saved
RCX      --      Scratch / shift count      caller-saved
RDX      --      Scratch                    caller-saved
R10      --      Scratch                    caller-saved
R11      --      Scratch                    caller-saved
RBX      R1      Mapped IE64 R1             callee-saved
RBP      R2      Mapped IE64 R2             callee-saved
R12      R3      Mapped IE64 R3             callee-saved
R13      R4      Mapped IE64 R4             callee-saved
R14      R31     IE64 SP                    callee-saved
R15      --      IE64 PC / return channel   callee-saved
```

5 IE64 registers are resident. R5-R30 are spilled. Fewer than ARM64 due to x86-64 having only 16 GPRs, but richer addressing modes partially compensate.

### Stack Frame Layout

```
RSP+0   = saved JITContext pointer (for I/O bail)
RSP+8   = FPU pointer (if hasFPU)
RSP+16  = loop counter (if hasBackwardBranch)
```

6 callee-saved pushes (48 bytes) + SUB RSP,24 = 72 + 8 (ret addr) = 80 bytes = 16-byte aligned.

### Encoding Considerations

- **RBP/R13 as base:** Always needs displacement byte even for offset 0 (ModRM encoding rule). Used only as data register (IE64 R2/R4), never as memory base.
- **R12 as base:** Requires SIB byte. Handled in encoding helpers.
- **Variable shifts:** Count must be in CL register. The emitter moves the shift count to RCX before the shift instruction.
- **Division safety:** x86-64 raises #DE on divide-by-zero. All DIV/IDIV are preceded by a zero-check with JZ to return 0.
- **CLZ:** Uses 32-bit BSR + XOR 31 sequence (no LZCNT dependency). Handles zero input explicitly (returns 32).

### Backward Branch Budget

Uses stack slot `[RSP+16]` as loop counter (no spare callee-saved register). Budget = 4095.

### Icache

No flush needed. x86-64 guarantees instruction cache coherency.

### Regions

Region formation uses `ScanRegionIE64()` and `ie64FormRegion()` to collect two or more statically-linked blocks within `IE64RegionProfile` limits (up to 8 blocks and 512 guest instructions). `ie64CompileRegion()` emits a single native block, preserves the normal chain-exit path for targets outside the region, and emits direct in-region jumps for BRA/JMP targets inside the region. Non-MMU promotion is enabled by default. When the MMU is active, `IE64_JIT_REGION_MMU=1` enables the page-bounded `ie64FormRegionMMU()` path.

Observed conditional and register-indirect paths use the same compiler,
residency plans, loop budget and external exits. Static BRA and `JMP R0`
formation is unchanged.

---

## wasm Backend

The js/wasm build has no native executable-memory backend and leaves
`jitAvailable` false. IE64 instead has a wasm bytecode tier. Other guest CPUs
remain interpreter-only in the browser.

`jit_exec_wasm.go` counts visits to block-start PCs. Once a block is hot,
`jit_wasm_ie64_emit.go` translates the shared `JITInstr` stream into a wasm
module and submits it to `WebAssembly.instantiate`. Compilation is asynchronous:
the interpreter continues until the module is installed at a cooperative yield.
Generated modules import the Go programme's linear memory through `__goMem` and
update the register file, JITContext and guest RAM in place. Runtime-generated
modules are not stored in the browser's compiled-wasm HTTP cache; that cache
applies only to the main `ie.wasm` application module.

Block-to-block dispatch uses a driver module and shared function table. Static
BRA chains can be promoted into one structured wasm function with bounded back
edges. Hot GPRs remain in `i64` locals and hot FP64 pairs remain in `f64` locals
across internal edges. FPSR condition-code liveness is analysed across the
whole region. Helper, SMC, external-branch and budget exits commit dirty state
and the dynamic retired count.

Triggered entries initially remain single-block functions. Invocation 64 and
each later recording step bypass the chain driver to expose one immediate
successor. Promotion is submitted asynchronously. Installation checks the
invalidation generation and the original entry block, removes its old SMC
ranges, and replaces the function in its existing table slot. A stale result is
discarded without changing the cache.

The wasm tier is disabled while the architectural timer or MMU is enabled.
Blocks are then neither compiled nor entered, and execution stays in the
interpreter. The supported compiled surface is the MMU-off integer core,
including ALU, load/store, branches, subroutine and stack operations, plus
FP64 arithmetic, moves, comparisons, conversions, DINT rounding modes, sticky
exception flags and condition codes. FP64 transcendentals, FP32 and remaining
system opcodes stay in the interpreter.

The compiler stops a block before its first unsupported opcode, leaving that
instruction for the interpreter. Supported instructions that need dispatcher
work leave generated code through the wasm helper protocol. Stack operations
serviced by the dispatcher use raw RAM, as they do in the interpreter, and do
not invoke MMIO callbacks even if the stack lies on a bitmap-marked I/O page.
Repeated exits from the canonical LOAD, AND, branch-back MMIO loop are
recognised and transferred to a cooperative parking poll service. The service
yields between reads because device state cannot advance while the single wasm
thread is spinning.

Generated stores first check a merged `[min,max]` compiled-code span for each
marked 256-byte page. A write outside that span stays in generated code. A
write inside the span leaves the chain, after which the runtime checks exact
disjoint block ranges and invalidates only blocks covering the written bytes.
A gap between separately installed ranges can therefore cause an exit without
causing invalidation. Region formation rejects disjoint members that share a
page with such a gap, reducing that candidate to a single block. The function
table permits 65,536 allocated slots before a compacting flush. Slots left
behind by range invalidation are reclaimed by that flush.

Structured wasm loops implement the same invariant-memory prechecks and bounded
immediate-counter budget elision as the native backends. They cover single
blocks and promoted regions, retain store-side SMC probes, and reject MMU,
stack, changing-pointer, helper-capable and alternate-entry loops.

The wasm tier is enabled by default. `IE64_WASM_JIT=0` or `/demo/?jit=0`
disables it. `IE64_WASM_JIT_DIAG=1` or `/demo/?jitdiag=1` publishes dispatcher
counters to `__ieJITDiag` and reports livelock signatures in the browser
console.

The wasm differential suite executes generated modules under wazero and
compares them with the interpreter:

```bash
go test -tags headless -run 'TestWasmJIT_|TestWasmEnc_' .
make test-wasm-node
```

The Node suite covers the JITContext ABI, hot-loop parity, MMU gating, timer
gating, stack access on MMIO-marked pages, cooperative MMIO polling and the kill
switch.

---

## I/O Dual-Path Memory Access

In low-window, MMU-off blocks, LOAD and STORE use a two-path strategy:

1. **Fast path** (addr < IO_REGION_START): Direct memory access via base+index. Falls through on the common path.
2. **Slow path** (addr >= IO_REGION_START):
   - Check `ioPageBitmap[addr >> 8]`
   - If I/O page: set `NeedIOFallback=1`, write `ctx.RetPC` (the bailing instruction's PC) and the retired count via `emitPackedPCAndCount` (`RetCount`/`ChainCount`), store writtenSoFar registers, return to dispatcher
   - If non-I/O page (e.g., VRAM): direct memory access

The dispatcher re-executes the bailing instruction via the interpreter after the block returns.

The direct `[memBase+addr]` fast path is taken only when the MMU is off **and** `addr` is inside the low `cpu.memory` window (size-aware bound `addr <= MemSize - accessBytes`). Otherwise - a high address, or any access while the MMU is on - the emitter takes the JITContext helper exit (`HELPER_LOAD`/`HELPER_STORE` etc.): it writes the request fields, flushes `LiveSP` and `HelperPC`, returns through the epilogue, and the dispatcher services the op via `cpu.loadMem`/`storeMem` (full `uint64` translation + fault semantics) before re-entering the JIT. High-PC code is itself scanned and compiled via the bus fetch path; the one exception is a block fetched from a high physical PC that contains a stack op, which is run through `interpretOne()` (see Overview).

### Fast MMIO Poll Shortcut

On AMD64 and ARM64, the IE64 dispatcher tries `tryFastIE64MMIOPollLoop()` before normal block-cache lookup. This is a Go-side shortcut for tight MMIO polling loops, not emitted native code. It is disabled when the MMU is enabled, when the CPU or bus is nil, or when the current PC is outside `cpu.memory`.

The native shortcut recognises three loop forms:

1. A three-instruction bit test: `LOAD`, immediate `AND`, then `BEQ` or `BNE` back to the load.
2. A bounded bit test: `LOAD`, optional register `EOR`, immediate or register `AND`, forward `BNE`, `SUB.Q #1` on a counter, then `BNE` back to the load.
3. A bounded equality test: `LOAD`, `MOVE.Q #value` into a comparison register, forward `BNE`, `SUB.Q #1` on a counter, then `BNE` back to the load or its preceding setup instruction.

All three forms require a non-zero load destination, MMU-off execution and an address accepted by `MachineBus.IsIOAddress`. Addresses above `0xFFFFFFFF` are rejected. The shortcut performs each read through `cpu.loadMem`, reproduces the matched arithmetic and register writes, and reports the instructions retired by the selected form. When the three-instruction loop stops taking its back edge, execution continues after the loop. A successful bounded test branches to its forward target, while counter exhaustion continues at its timeout path. Reaching `DefaultPollIterationCap`, an external interrupt, or a stop request leaves the PC at the appropriate loop entry so normal dispatch can resume safely.

Countdown and equality variants use the same interrupt, trap and retired-count rules on both native backends.

---

## FPU JIT

IE64 FPU operations are classified into five categories:

### Category A: Integer Bitwise on FP Registers
FMOV, FABS, FNEG, FMOVI, FMOVO, FMOVECR, FMOVSR, FMOVCR, FMOVSC, FMOVCC

Operate on the FP register file (16 x 32-bit at FPUPtr) using integer bit manipulation.

### Category B: Native FP Instructions
FADD, FSUB, FMUL, FDIV, FSQRT, FINT, FCMP, FCVTIF, FCVTFI (native on both platforms)

- **ARM64:** Uses S-register instructions (FADD, FSUB, FRINTN/M/Z/P for FINT, FCVTZS for FCVTFI) via FMOV W<->S transfers
- **x86-64:** Uses SSE scalar instructions (ADDSS, SUBSS, ROUNDSS, UCOMISS, CVTSI2SS, CVTTSS2SI, etc.) via MOVD XMM<->GPR transfers. SSE4.1 (ROUNDSS) is the runtime baseline for the amd64 JIT: `initJIT` checks for it (`checkJITHostFeatures`) and, if absent, falls back to the interpreter instead of enabling the JIT. Release builds still target x86-64-v3 (`GOAMD64=v3`) for codegen quality, but lower `GOAMD64` levels build and run fine. FCVTFI emits saturating and NaN checks around CVTTSS2SI to preserve interpreter exception behaviour.

### FPSR semantics

Category B arithmetic maintains the full FPSR, matching the interpreter bit for bit:

- **Condition codes** (bits 27:24) may be deferred but never dropped. The backward liveness pass (`jit_ie64_fpsr_liveness.go`) runs on amd64 and arm64, and both backends honour both of its marks. An update is elided outright only when a later non-faulting FP instruction overwrites the whole field before any observer (`fpsrCCDead`); an update no observer inside the block can reach is instead deferred to the block's exit funnels and rebuilt there by re-reading the writer's destination register (`fpsrCCSink`). Sinking takes the classifier out of hot loop bodies. Region compilers carry liveness across internal edges, clear pending materialisation state at sub-block boundaries and materialise the newest architectural condition state at every external exit.
- **Sticky exception flags** (bits 3:0: IO, DZ, OE, UE) are eager and never elided: a raised flag stays observable until software clears it. They cannot be sunk, because each rule depends on that operation's own operands rather than on the final register value. A fast-path gate skips the classifier when the result is neither infinite, NaN, nor (for the ops with an underflow rule) zero.
- `FSQRT` raises IO for a negative operand, excluding -0.0 and NaN.
- `FCMP` raises IO on an unordered compare, and reports infinity through CC_I.

The host FP status registers (MXCSR, ARM64 FPSR) cannot stand in for the classifier: hardware raises the invalid flag for signalling-NaN *operands* and underflow for *denormal results*, and the IE64 rules exclude both.

`jit_ie64_fp_parity_common_test.go` drives the native FP32 arithmetic through an IEEE-754 special-value matrix. `jit_ie64_fp_audit_test.go` adds parity coverage for FP comparisons, conversions, integer rounding, square root, modulo and native FP64 arithmetic. Both compare the relevant architectural state, including FPSR, with the interpreter and build for both native backends.

### Category C: Helper Exit
DSIN, DCOS, DTAN, DATAN, DLOG, DEXP, DPOW

The FP64 transcendental opcodes are emitted as helper exits rather than whole-block fallbacks. The dispatcher calls the same Go FPU method used by the interpreter, so result bits, pair validation, PC advance, and FPSR side effects stay deterministic across backends.

### Category D: Double precision

amd64 and arm64 emit the same FP64 core natively: `DMOV`, `DADD`, `DSUB`, `DMUL`, `DDIV`, `DINT`, `DCMP`, `DCVTIF`, `DCVTFI`. Both bail for `DMOD`, `DABS`, `DNEG`, `DSQRT`, `FCVTSD`, `FCVTDS`. The two lists are deliberately identical, so this table describes both backends rather than one each.

Native FP64 arithmetic bails to the interpreter whenever an operand is non-finite (`emitDPairNonFiniteBailAMD64`, `emitDPairNonFiniteBailARM64`), so NaN and infinity operands take interpreter semantics by construction and the native path only has to account for overflow, underflow and divide-by-zero on finite inputs.

That bail also discharges every `!isInf(operand)` and `!isNaN(operand)` clause in the `IE64FPU` exception rules: those clauses are statically true on any path that reaches the native code. This is why the emitted classifiers only ever inspect the *result*, and why the emitters look shorter than the interpreter methods they mirror.

`IE64FPU` (`fpu_ie64.go`) defines these rules. Both native backends follow its clause order so their classifiers can be compared directly with the interpreter implementation.

`DLOAD` and `DSTORE` are JIT-emitted on both backends with native low-window fast paths and helper exits (`HELPER_DLOAD`/`HELPER_DSTORE`) for MMU, high-address, MMIO, or invalid-pair cases. They are not in `needsFallback()` and do not force whole-block fallback.

### Category E: Interpreter Bail
FMOD, FSIN, FCOS, FTAN, FATAN, FLOG, FEXP, FPOW on both backends.

---

## Fallback Rules

The JIT falls back to the interpreter in these cases:

| Condition | Mechanism |
|-----------|-----------|
| HALT, WAIT, RTI as first instruction | `needsFallback()` in scanner, dispatcher calls `interpretOne()` |
| HALT, WAIT, RTI mid-block | Emitted as bail-to-interpreter (set NeedIOFallback, epilogue) |
| High virtual/physical PC | Scanned and compiled via `scanBlockBus` (bus fetch) - **not** a fallback |
| Unmapped physical instruction fetch | Scan/dispatch stops cleanly (`ReadPhys64WithFault` returns `ok=false`) |
| High-PC block containing a stack op (`PUSH`/`POP`/`JSR`/`RTS`/`JSR_IND`) | `highPhys && containsStackOp`, so the dispatcher runs the block via `interpretOne()` |
| High address or MMU-on data/stack/FP/control op | JITContext helper exit (serviced by the dispatcher) - **not** a whole-instruction bail |
| I/O page memory access | Dual-path: bail to interpreter on I/O bitmap hit |
| FP32 FMOD/transcendentals | Bail to interpreter on both backends |
| Double-precision arithmetic/conversion | `DMOD`/`DABS`/`DNEG`/`DSQRT`/`FCVTSD`/`FCVTDS` bail on both amd64 and arm64; the rest of the FP64 core is native on both. `DLOAD`/`DSTORE` and `DSIN` through `DPOW` are JIT-emitted via helper exit on both, not bailed |
| Non-finite operand to native FP64 arithmetic | Bail to interpreter (`emitDPairNonFiniteBailAMD64`, `emitDPairNonFiniteBailARM64`), so NaN/infinity operands take interpreter semantics |
| Atomic RMW (CAS, XCHG, FAA, FAND, FOR, FXOR) | Native only for aligned non-MMU low-window RAM; MMU-on, high-address, MMIO, or unaligned cases bail to interpreter |
| SEI64, CLI64 | Emitted as bail-to-interpreter (`emitBailToInterpreter`) so `interruptEnabled` is mutated; compiling them as NOPs silently dropped the state change under timer-off native execution |
| MMU/privilege opcodes (MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SYSCALL, SMODE, SUAEN, SUADIS) | Block terminators; first-instruction fallback in `needsFallback()`, otherwise emitted as bail-to-interpreter |
| ExecMem exhausted | `compileBlock` returns error, dispatcher calls `interpretOne()` |
| Self-modifying code | `NeedInval` records one exact write range, or uses `InvalSize=0` to request a full invalidation when the range is unavailable. By default, exact ranges remove overlapping cached blocks. `IE_JIT_SMC_RANGE=0` makes native execution invalidate the full cache instead. ExecMem resets only for a full invalidation or when no cached block remains |

---

## External Interrupt Delivery

External device interrupts (video VBI, display-list, blitter) reach the IE64 CPU through `IE64InterruptSink`. The sink is record-only: device goroutines never write architectural CPU state (`PC`, the stack, `inInterrupt`) directly. Instead they record a pending cause and the CPU goroutine performs delivery at a safe boundary. This removes both the data race on CPU-owned state and the lost-delivery bug under the JIT, where the dispatcher overwrites `cpu.PC` from `ctx.RetPC` after a native block returns and would have clobbered any asynchronous PC write.

Recording (`CPU64.handleExternalInterrupt`):

- The cause is OR-ed into the atomic `pendingIRQMask` field on `CPU64`.
- A gate is applied at record time: if interrupts are disabled (`interruptEnabled` false) or one is already in flight (`inInterrupt` true), the raise is dropped, not latched. This preserves the original edge-pulse drop timing.
- `Pulse` (edge) records the call argument. The level paths (`Assert`, `Deassert`, `Ack`, `SetMask`) reconcile the latch with the current level state: they record the derived unmasked-active set `pendingMask()` rather than the call argument, so acknowledging or masking one source does not lose another that is still active, and they clear from the latch any cause that is no longer pending (deasserted or masked) so a level change before the CPU polls does not deliver a stale cause. The level state and the latch reconcile are guarded by a mutex on the sink because device goroutines may call concurrently.

Delivery (`CPU64.deliverPendingExternalInterrupt`):

- Consumes `pendingIRQMask` with an atomic swap, re-checks the enable and in-flight gate (dropping if masked between recording and the poll), then vectors. MMU-on takes a trap frame and jumps to `intrVector` with the cause recorded in `faultAddr`. MMU-off pushes the current PC and jumps to `interruptVector`. The sequence mirrors the timer interrupt model.

Poll sites, all before the next instruction or block fetch so the interrupt takes priority over a fault or fetch at the interrupted PC, matching a hardware instruction boundary:

- Interpreter `Execute()`: at the top of the loop, before PC translation and fetch.
- `StepOne()`: at entry, before fetch. A delivered interrupt consumes the step.
- JIT `ExecuteJIT()`: a single poll at the top of the dispatcher loop, reached only after a native block's helper, IO-bail, and retired-count handling have completed. The helper dispatcher (`handleJITHelper`) also polls before it services a bailed memory/stack/control op, so helper-exit blocks (DLOAD/DSTORE, MMU-on memory, high/IO helpers) take the interrupt at the bailing instruction's PC like the interpreter rather than after the op runs.
- JIT fast paths (amd64 and ARM64): the MMIO poll-loop shortcut exits its spin when `pendingIRQMask` is set, leaving the PC at the loop head so the dispatcher delivers and resumes there. This watches `pendingIRQMask` because external IRQs no longer flip `inInterrupt`.

IE64 native code can chain up to `ie64ChainBudget` (256) block transitions inside one `callNative` without returning to Go, so a pending interrupt raised mid-chain is observed when the chain returns to the dispatcher rather than between every guest instruction. This is a latency difference from the interpreter, not a correctness one, and is acceptable for the video-class interrupts in scope. Tighter latency would require a pending check in the chain-dispatch epilogue.

---

## Testing

### Platform-Specific Tests

```bash
# ARM64 JIT tests (on ARM64 machine)
go test -v -run TestARM64_ -tags headless ./...

# x86-64 JIT tests (on x86-64 machine)
go test -v -run TestAMD64_ -tags headless ./...
```

### JIT-vs-Interpreter Parity

```bash
go test -v -run TestJIT_vs_Interpreter -tags headless ./...
```

Runs identical IE64 programs through both JIT and interpreter, comparing all register values.

### Shared Infrastructure Tests

```bash
go test -v -run 'Test(ScanBlock|NeedsFallback|CodeBuffer|CodeCache|ExecMem|JITContext_|AMD64_RTI_RegisterLiveness|ARM64_RTI_RegisterLiveness)' -tags headless ./...
```

Tests block scanning, fallback classification, register analysis, code-buffer
and code-cache behaviour, the JITContext layout, and ExecMem.

### Test Rig

Both backends use an identical `jitTestRig` pattern:
1. Create MachineBus + CPU64 + AllocExecMem(1MB) + newJITContext
2. `compileAndRun()` loads instructions at PROG_START, appends HALT, scans, strips terminal HALT, compiles, executes via callNative
3. Reads `ctx.RetPC` into `cpu.PC` (and clears it); `ctx.NeedHelper` can be inspected to assert helper-exit requests

Mid-block RTI/WAIT tests use manual scan+compile (no HALT stripping) to verify bail behaviour.

---

## Performance Guardrails

### Built In
- Fixed guest-to-host register mapping in Tier 1 blocks
- Weighted hot-register selection for promoted amd64 regions
- ARM64 loads mapped registers used by the block; AMD64 loads all resident registers; both limit normal exit writeback to registers that may have changed
- Shortest instruction forms (reg-imm ALU, direct base+disp spills)
- 32-bit host ops for IE64 `.L` size where semantics match
- Fast-path fall-through for normal RAM; I/O in slow-path branch
- Native hot-region promotion for IE64 blocks with static BRA/JMP successors

### Deferred
- Direct (non-helper) native fast path for high-physical data/stack access: high addresses currently route through the JITContext helper exit; inlining the sparse-backing / MMU translation into native code is a future perf item
- Native `DMOD`, `DABS`, `DNEG`, `DSQRT`, `FCVTSD`, `FCVTDS` on both backends
- Memory operands for spilled-source ALU
- Peephole patterns (MOVE imm, ADD/SUB imm, compare against zero)
- Smaller prologue variants for simple blocks

### Not Planned
- Full register allocation (linear scan, graph colouring, SSA)
- Instruction scheduling
- CPU-feature-dependent tricks (LZCNT, BMI2, AVX)

### Benchmarking

The benchmark suite in `ie64_benchmark_test.go` measures throughput for seven workload categories through both the interpreter and JIT: ALU, FPU, memory, MMIO, MMU mixed, mixed and call/return.

```bash
go test -tags headless -run='^$' -bench BenchmarkIE64_ -benchtime 3s ./...
```

Each benchmark reports ns/op and instructions/op. MIPS can be derived: `MIPS = instructions/op / ns/op * 1000`. See the file for detailed documentation of each workload's instruction mix.

### Performance-technique inventory

This inventory summarises the techniques used by the Linux and browser IE64 backends.

| Technique | Linux amd64 | Linux ARM64 | js/wasm |
|-----------|-------------|-------------|---------|
| Dirty resident-register exits and cold bailout tails | Implemented | Implemented | Implemented through local writeback and shared exits |
| Bounded static-JMP chase with timer and retired-count accounting | Implemented | Implemented through the shared native dispatcher | Implemented, with timer-enabled execution returning to normal dispatch |
| Proven constant RAM/MMIO access check elision with SMC retained | Implemented | Implemented | Implemented |
| Multi-block regions with internal edges and bounded back edges | Implemented | Implemented, including opt-in MMU regions | Implemented as one structured wasm function |
| Hot GPR residency across region edges | Four dynamically selected callee-saved hosts | Fixed R1-R14 mapping in X12-X17 and X19-X26 | `i64` locals |
| FP32 and FP64 residency | XMM8 through XMM15 when `IE64_JIT_FP_RESIDENCY=1` on Linux/macOS | V16 through V31 when `IE64_JIT_FP_RESIDENCY=1` | FP64 pairs in `f64` locals; FP32 is inapplicable |
| Backward FPSR condition-code liveness | Implemented | Implemented | Implemented across blocks and regions |
| MMIO poll acceleration | Implemented | Implemented | Implemented as a cooperative parking poll service |
| Integer flag liveness and compare-branch fusion | Inapplicable: IE64 branches compare GPR operands directly | Inapplicable | Inapplicable |
| x87 constant pooling | Inapplicable: IE64 constants use immediate materialisation | Inapplicable | Inapplicable |
| MMU JIT support | Implemented | Implemented, with region promotion opt-in | Inapplicable to this backend scope |

## MMU Integration

When the IE64 MMU is enabled (MMU_CTRL bit 0 = 1), the native emitters check
`ctx.MMUEnabled`, which the dispatcher refreshes before every `callNative`.
ARM64 routes non-atomic data, stack, FP and control-flow memory operations
through the JITContext helper exit. AMD64 first probes a four-entry native
micro-TLB for ordinary LOAD and STORE operations. A hit translates the virtual
page and continues through the native low-window RAM path. A miss uses the
helper exit, which performs the canonical translation and may fill the entry.
The remaining non-atomic amd64 memory operations use the helper exit directly;
atomics retain their whole-instruction bail path.

### Helper Exit for Memory Operations Under MMU

The following use the helper exit when the MMU is on, except that amd64 LOAD
and STORE may complete after a native micro-TLB hit. They also use the helper
when an address escapes the low window with the MMU off:

- **LOAD, STORE** (general-purpose memory access)
- **PUSH, POP** (stack operations)
- **JSR, RTS, JSR_IND** (subroutine call and return, all of which touch the stack)
- **FLOAD, FSTORE, DLOAD, DSTORE** (FP / FP64 memory access)

These do **not** re-execute through the interpreter as whole instructions. The
dispatcher performs only the requested semantic through the shared helpers and
advances the PC. When `IE64_JIT_RESUME` is enabled, a clean helper completion
may continue after the requesting instruction inside the same native block.
Otherwise execution returns through the normal JIT dispatcher.

Two cases still take a whole-instruction `mmuBail` path to `emitBailToInterpreter` under MMU rather than the helper exit:

- **Atomics** (CAS, XCHG, FAA, FAND, FOR, FXOR) - bailed under MMU (see note below).
- **Fused JSR/RTS leaf markers** - `compileBlockMMU` sets `mmuBail` on them so the raw `[memBase+SP]` fused fast path is suppressed and the guarded `OP_JSR64`/`OP_RTS64` bail path runs instead.

RTI is a block terminator and normally reaches the interpreter through `needsFallback()` when it is the first instruction or through an emitted bail path when it appears after earlier instructions in a block.

Non-memory instructions (ALU, single-precision FPU arithmetic, FP64 transcendental helper exits, branches, moves) are compiled to native code where the emitters support them and execute through the native block or helper-exit path.

**Note on atomics**: The six atomic memory operations (CAS, XCHG, FAA, FAND, FOR, FXOR) have native sequentially-consistent fast paths on both JIT backends for aligned, non-MMU, low-window RAM. MMU-on, high-address, MMIO, or unaligned cases bail to the interpreter so `atomicRMW64` remains the canonical trap and bus-semantics implementation.

### Block Fetch and Page Boundaries

Block scanning requires special handling under MMU:

- **Virtual PC translation**: Before scanning a block, the virtual PC is translated to a physical address through the MMU page table. The physical address is used to read instruction bytes from memory.
- **Cache key**: In MMU mode the code cache is keyed on the **exact** `(PTBR, virtualPC)` pair (`GetMMU`/`PutMMU`), not the physical address alone and not a lossy hash. This prevents two address spaces with the same virtual PC but different page tables from colliding on a stale native block.
- **Page boundary limit**: Blocks are limited to the current 4 KiB physical page during scanning. This prevents a single scanned block from crossing into bytes that may not correspond to the next virtual page mapping.

### Cache Invalidation

The JIT code cache must be flushed whenever the virtual-to-physical mapping changes. The following operations set the `jitNeedInval` flag, which causes the dispatcher to flush the code cache and reset the executable memory allocator before the next block lookup:

- **MTCR to PTBR** (CR0): The page table base has changed; all cached translations are stale.
- **MTCR to MMU_CTRL** (CR5): The MMU enable state has changed; cached blocks may have been compiled under different assumptions.
- **TLBFLUSH**: Bulk TLB invalidation implies page table changes that affect translation.
- **TLBINVAL**: Single-page TLB invalidation. Conservatively flushes the entire code cache (a targeted invalidation would require reverse-mapping virtual addresses to cached blocks).

Self-modifying-code invalidation uses a separate store-side path. Native and helper-side stores mark `NeedInval` only after a 256-byte page mark matches and the known virtual write range overlaps a compiled instruction range in the relevant cache. Non-MMU checks scan only non-MMU cached blocks. MMU checks scan only blocks for the current `PTBR`, so ordinary data writes on a marked page in another address space do not clear helper resume state, RTS cache entries, or cached code. A zero `InvalSize` remains the full-cache invalidation signal, used for multiple SMC stores and MMU physical-alias hits.

### Block Terminators

All 9 MMU/privilege instructions (MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SYSCALL, SMODE, SUAEN, SUADIS) are block terminators. The block scanner ends the current block when any of these opcodes is encountered. This ensures that:

- Privilege level changes (SMODE, SYSCALL, ERET) take effect before the next block is compiled.
- Cache invalidations (MTCR, TLBFLUSH, TLBINVAL) are processed by the dispatcher between blocks.
- Control flow changes (ERET, SYSCALL) are handled correctly.

### Future Work

- Extend native micro-TLB probing beyond amd64 LOAD and STORE operations.
- Add address-space identifiers if the architecture gains an ASID contract.
