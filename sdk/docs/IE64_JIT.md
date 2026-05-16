# IE64 JIT Compiler

Technical reference for the IE64 Just-In-Time compiler. Covers the shared infrastructure, dispatcher, and both platform-specific backends (ARM64 and x86-64).

---

## Overview

The IE64 JIT compiler translates blocks of IE64 machine code into native ARM64 or x86-64 instructions at runtime, executing them directly on the host CPU. This bypasses the Go interpreter loop and yields significant performance improvements for compute-heavy workloads.

PLAN_MAX_RAM.md slice 3 widened the interpreter, bus, and MMU address plumbing to 64-bit. The current IE64 JIT block builder and return channel remain `uint32` PC based. The dispatcher therefore falls back to the interpreter before block scanning when the virtual PC is above `0xFFFFFFFF`, when the translated executable physical address is outside `cpu.memory`, or when an MMU instruction fetch would require the high-physical bus fetch path. Correctness is preserved for high active visible RAM by using the interpreter for those cases. `LOAD`, `STORE`, `FLOAD`, `FSTORE`, `JMP`, and `JSR_IND` are still JIT-emitted for low-window, MMU-off code; under MMU, memory-touching instructions are marked `mmuBail` and re-executed by the interpreter for full virtual-address translation. `DLOAD` and `DSTORE` still force whole-block fallback because native 64-bit double memory transfer emitters have not landed.

**Supported platforms:** ARM64/Linux, ARM64/macOS, ARM64/Windows, x86-64/Linux, x86-64/macOS, x86-64/Windows

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
  ExecMem.Write()         jit_mmap.go      Copy via RW view + icache flush on RX view (W^X)
        |
        v
  CodeCache.Put()         jit_common.go    Cache by dispatcher key for O(1) lookup
        |
        v
  callNative()            jit_call.go      Execute via runtime.asmcgocall
        |
        v
  Dispatcher unpack       jit_exec.go      Extract uint32 PC + instruction count from regs[0]
```

### File Inventory

| File | Build Tag | Purpose |
|------|-----------|---------|
| `jit_common.go` | (none) | JITContext, CodeBuffer, block scanner, register analysis, code cache |
| `jit_exec.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Dispatcher loop (`ExecuteJIT`), timer handling |
| `jit_call.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | `callNative` via `runtime.asmcgocall` plus darwin exec/write protection hooks |
| `jit_call_arm64.s` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 trampoline (`R0` receives `*jitCallArgs`; native block receives `JITContext*`) |
| `jit_call_amd64.s` | `amd64 && (linux \|\| darwin)` | SysV x86-64 trampoline |
| `jit_call_amd64_windows.s` | `amd64 && windows` | Windows x86-64 trampoline |
| `jit_emit_arm64.go` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 code emitter (~2450 lines) |
| `jit_emit_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | x86-64 code emitter (~1850 lines) |
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
| `jit_ie64_turbo.go` | `amd64 && (linux \|\| windows \|\| darwin)` | IE64 turbo-region policy, statistics, and planning metadata |
| `jit_ie64_turbo_stub.go` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 IE64 turbo stubs; turbo is disabled on ARM64 |
| `jit_ie64_bench_turbo_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | AMD64 IE64 recognised benchmark-family turbo shortcuts |
| `jit_ie64_bench_turbo_stub.go` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 stubs for IE64 benchmark-family turbo shortcuts |
| `jit_mmio_poll_common.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Shared MMIO-poll loop matcher |
| `jit_mmio_poll_backends.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Per-backend MMIO-poll pattern descriptors, including IE64 |
| `jit_mmio_poll_wiring.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Runtime MMIO-poll predicate wiring from each CPU/bus |
| `jit_mmio_poll_exec_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | AMD64 MMIO-poll execution helpers, including IE64 |
| `jit_mmio_poll_exec_arm64_stub.go` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 MMIO-poll execution stubs |
| `jit_fastpath_bitmaps.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Shared bitmap-probe shape metadata used by AMD64 emitters |
| `jit_fastpath_backends.go` | `amd64 && (linux \|\| windows \|\| darwin)` | Audit registry for per-backend fast-path bitmap usage, including IE64 |
| `jit_exec_protect_darwin_arm64.go` | `darwin && arm64` | macOS arm64 `MAP_JIT` write-protection transitions |
| `jit_exec_protect_stub.go` | `!(darwin && arm64)` | No-op executable-memory protection hooks |

---

## Shared Infrastructure

### JITContext

Bridge between Go and native code. Passed as the sole argument to every JIT block.

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
88      uint32    RTSCache0PC      MRU RTS target PC 0
92      uint32    _pad0            Alignment padding
96      uintptr   RTSCache0Addr    MRU RTS target native entry 0
104     uint32    RTSCache1PC      MRU RTS target PC 1
108     uint32    _pad1            Alignment padding
112     uintptr   RTSCache1Addr    MRU RTS target native entry 1
120     uint32    RTSCache2PC      MRU RTS target PC 2
124     uint32    _pad2            Alignment padding
128     uintptr   RTSCache2Addr    MRU RTS target native entry 2
136     uint32    RTSCache3PC      MRU RTS target PC 3
140     uint32    _pad3            Alignment padding
144     uintptr   RTSCache3Addr    MRU RTS target native entry 3
```

### Block Scanner

`scanBlock()` decodes IE64 instructions starting at a given physical PC until a block terminator is found or 256 instructions are reached. Terminators are included in the returned block. Current terminators are BRA, JMP, JSR64, RTS64, JSR_IND, HALT64, RTI64, WAIT64, all MMU/privilege opcodes (SYSCALL, ERET, MTCR, MFCR, TLBFLUSH, TLBINVAL, SMODE, SUAEN, SUADIS), and all atomic RMW opcodes (CAS, XCHG, FAA, FAND, FOR, FXOR).

On AMD64 only, the scanner may replace a small register-only JSR leaf with fused markers plus the leaf body. The ARM64 scanner gate disables that fusion because its emitter does not honour those markers.

### Register Liveness

`analyzeBlockRegs()` computes bitmasks of which IE64 registers are read and written by the block. This minimises prologue/epilogue overhead -- only load registers the block reads, only store registers it writes.

### CodeBuffer

Variable-length byte buffer with label/fixup support for forward references:
- `EmitBytes()` / `Emit32()` / `Emit64()` for code emission
- `Label()` / `FixupRel32()` / `Resolve()` for forward jump patching
- `PatchUint32()` for inline patching

### CodeCache

Maps a dispatcher key to `*JITBlock` for O(1) lookup. In non-MMU mode the key is the physical `startPC`; in MMU mode the key is the PTBR-mixed virtual PC described in [MMU Integration](#mmu-integration). Invalidated on self-modifying code (writes to [PROG_START, STACK_START)).

### Turbo Region Tier

On AMD64, hot IE64 blocks can be promoted from Tier 1 single-block JIT code to a turbo region. The dispatcher increments `JITBlock.execCount` on cache hits and asks the shared `TierController` whether the block is hot enough to promote. The default threshold is 64 re-entries, with promotion suppressed when the block is already promoted, was already attempted, or has an I/O-bail rate of 25% or higher.

IE64 turbo promotion is currently non-MMU only. `ie64FormRegion()` scans `cpu.memory` at flat physical indices and follows statically-known BRA/JMP terminators; under MMU, each virtual successor would need its own page-table walk before the scanner could read the correct physical bytes. The dispatcher therefore gates region promotion with `!cpu.mmuEnabled`.

The AMD64 region compiler emits one native `JITBlock` for two or more IE64 blocks. Internal BRA/JMP targets become direct `JMP rel32` transfers inside the native region; external targets still use the normal chain-exit machinery. Back-edges inside a region keep the loop-budget and retired-count checks so native code cannot spin without returning to the dispatcher. Region promotion can be disabled with `IE64_JIT_TURBO=0`, and statistics print when `IE64_JIT_STATS=1`.

Separate from native region compilation, AMD64 also has recognised benchmark-family turbo shortcuts in `tryIE64TurboProgram()`. These run before normal block-cache lookup, only in non-MMU mode, only when the physical PC is `PROG_START`, only when the first opcode is a candidate `MOVE`, and only when timers, interrupt handling, and trap halt state are inactive. The currently wired patterns are the ALU, memory, and call benchmark loops. They execute specialised Go paths and return a retired-instruction count; they are not a general-purpose native compiler.

ARM64 builds include the tier controller and stub symbols, but `ie64TurboEnabled()` is false and `ie64CompileRegion()` returns an unsupported error. There is no ARM64 IE64 turbo-region compiler today.

### ExecMem

16 MB executable-memory pool with bump allocator (16-byte aligned). Reset on cache invalidation.

M15.6 uses a W^X-safe dual mapping on Linux:

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

Every JIT block exit stores a **packed 64-bit value** into `regs[0]` (the R0 slot, which is otherwise hardwired to zero):

```
regs[0] = nextPC | (retiredInstructionCount << 32)
```

The dispatcher (`jit_exec.go`) unpacks this:
```go
combined := cpu.regs[0]
cpu.PC = uint64(uint32(combined))          // lower 32 bits
executed := combined >> 32                  // upper 32 bits
```

The lower half is intentionally `uint32` in the current JIT. The dispatcher avoids native execution for PCs above `0xFFFFFFFF`; such instructions use the interpreter until the JIT PC and return-channel path is widened.

**Every exit path must pack both values:**
- Normal block end: `staticCount = len(instrs)`
- Branch taken: `staticCount = instrIdx + 1`
- I/O bail: `bailCount = ji.pcOffset / IE64_INSTR_SIZE`
- Backward-branch budget exit: dynamic count from loop counter
- Chained block exits: predecessor counts accumulated in `JITContext.ChainCount` are added by the dispatcher after `callNative` returns.

**Important distinction for bail paths:**
- `bailCount` (retired instruction count) goes into the packed return channel
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
X12-X26  R1-R15  Mapped IE64 registers (15 GPRs resident)
X27      R31     IE64 SP (always resident)
X28      --      IE64 PC / return channel
XZR      R0      Hardwired zero
X29/X30  --      Go FP/LR (saved/restored)
```

15 IE64 registers are resident in ARM64 registers. R16-R30 are spilled to the register file in memory.

### Prologue/Epilogue

Fixed 112-byte frame. Saves/restores callee-saved pairs selectively based on register usage.

### Backward Branch Budget

Uses X7 as iteration counter. Budget = 4095 (fits ARM64 CMP imm12). Budget exceeded -> exit block, let dispatcher reset timer.

### Icache Flush

Required on ARM64. Uses DC CVAU + IC IVAU + DSB ISH + ISB per 64-byte cache line.

### Turbo Regions

Not implemented on ARM64. The ARM64 backend provides IE64 region stubs so the shared dispatcher builds, but promotion is disabled and native region compilation is unsupported.

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

### Turbo Regions

Implemented for non-MMU IE64 code. Region formation uses `ScanRegionIE64()` and `ie64FormRegion()` to collect two or more statically-linked blocks within `IE64RegionProfile` limits (up to 8 blocks and 512 guest instructions). `ie64CompileRegion()` emits a single native block, preserves the normal chain-exit path for targets outside the region, and emits direct in-region jumps for BRA/JMP targets inside the region.

The AMD64 dispatcher also contains the recognised benchmark-family turbo shortcuts described in [Turbo Region Tier](#turbo-region-tier). Those shortcuts are Go fast paths, not emitted native blocks.

---

## I/O Dual-Path Memory Access

In low-window, MMU-off blocks, LOAD and STORE use a two-path strategy:

1. **Fast path** (addr < IO_REGION_START): Direct memory access via base+index. Falls through on the common path.
2. **Slow path** (addr >= IO_REGION_START):
   - Check `ioPageBitmap[addr >> 8]`
   - If I/O page: set `NeedIOFallback=1`, pack PC + bailCount, store writtenSoFar registers, return to dispatcher
   - If non-I/O page (e.g., VRAM): direct memory access

The dispatcher re-executes the bailing instruction via the interpreter after the block returns.

The native memory emitters use the low `cpu.memory` window. If dispatch starts from a high virtual PC or a translated executable physical address outside that window, the dispatcher does not scan or compile a native block and calls `interpretOne()` instead. Under MMU, `compileBlockMMU()` marks memory-touching instructions as `mmuBail`; those instructions return to the dispatcher and are re-executed through the interpreter so full `uint64` virtual-address translation and fault semantics remain canonical.

### Fast MMIO Poll Shortcut

On AMD64 only, the IE64 dispatcher tries `tryFastIE64MMIOPollLoop()` before normal block-cache lookup. This is a Go-side shortcut for tight MMIO polling loops, not emitted native code. It is disabled when the MMU is enabled, when the CPU or bus is nil, or when the current PC is outside `cpu.memory`.

The recognised IE64 shape is three instructions at the current PC:

1. `LOAD rd, disp(rs)` with `rd != R0`
2. `AND rd, rd, #mask` at the same operand size
3. `BEQ rd, r0, back_to_load` or `BNE rd, r0, back_to_load`

The shortcut computes the effective address with current IE64 address semantics, rejects addresses above `0xFFFFFFFF`, and requires `MachineBus.IsIOAddress(uint32(addr))`. When it matches, it repeatedly performs the MMIO load through `cpu.loadMem`, applies the mask, writes the masked value back to `rd`, and advances PC past the three-instruction loop when the branch condition becomes false. If the loop remains taken, it stops at `DefaultPollIterationCap`, leaves PC at the loop head, and reports `iterations * 3` retired instructions.

ARM64 provides stubs for the shared MMIO-poll entry points. It does not execute the IE64 fast poll shortcut today.

---

## FPU JIT

IE64 FPU operations are classified into three categories:

### Category A: Integer Bitwise on FP Registers
FMOV, FABS, FNEG, FMOVI, FMOVO, FMOVECR, FMOVSR, FMOVCR, FMOVSC, FMOVCC

Operate on the FP register file (16 x 32-bit at FPUPtr) using integer bit manipulation.

### Category B: Native FP Instructions
FADD, FSUB, FMUL, FDIV, FSQRT, FCMP, FCVTIF (native on both platforms)
FINT, FCVTFI (native on ARM64; bail to interpreter on x86-64)

- **ARM64:** Uses S-register instructions (FADD, FSUB, FRINTN/M/Z/P for FINT, FCVTZS for FCVTFI) via FMOV W<->S transfers
- **x86-64:** Uses SSE scalar instructions (ADDSS, SUBSS, UCOMISS, CVTSI2SS, etc.) via MOVD XMM<->GPR transfers. FINT bails to interpreter because ROUNDSS requires SSE4.1 which cannot be assumed on all amd64 targets. FCVTFI bails because the interpreter implements saturating conversion with NaN handling and IO exception flags that CVTTSS2SI cannot replicate.

### Category C: Interpreter Bail
FMOD, FSIN, FCOS, FTAN, FATAN, FLOG, FEXP, FPOW, and all double-precision opcodes (`DMOV` through `FCVTDS`)

The double-precision ISA is implemented by the interpreter. The current IE64 JIT emitters bail for those opcodes rather than duplicating the interpreter FPU status, conversion, and memory semantics. `DLOAD` and `DSTORE` additionally force whole-block fallback in `needsFallback()` because the native emitters do not implement 64-bit double memory transfers.

---

## Fallback Rules

The JIT falls back to the interpreter in these cases:

| Condition | Mechanism |
|-----------|-----------|
| HALT, WAIT, RTI as first instruction | `needsFallback()` in scanner, dispatcher calls `interpretOne()` |
| HALT, WAIT, RTI mid-block | Emitted as bail-to-interpreter (set NeedIOFallback, epilogue) |
| PC > `0xFFFFFFFF` | Dispatcher calls `interpretOne()` before scanning |
| Executable physical address outside `cpu.memory` | Dispatcher calls `interpretOne()` before scanning |
| I/O page memory access | Dual-path: bail to interpreter on I/O bitmap hit |
| FMOD/transcendentals and double-precision FPU opcodes | Bail to interpreter; DLOAD/DSTORE force whole-block fallback |
| Atomic RMW (CAS, XCHG, FAA, FAND, FOR, FXOR) | Always bail to interpreter (MMU-on and MMU-off; centralised SC semantics) |
| MODS, MULHU, MULHS | Bail to interpreter |
| MMU/privilege opcodes (MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SYSCALL, SMODE, SUAEN, SUADIS) | Block terminators; first-instruction fallback in `needsFallback()`, otherwise emitted as bail-to-interpreter |
| FINT (x86-64 only) | Bail to interpreter (ROUNDSS requires SSE4.1) |
| FCVTFI (x86-64 only) | Bail to interpreter (saturating + NaN semantics) |
| ExecMem exhausted | `compileBlock` returns error, dispatcher calls `interpretOne()` |
| Self-modifying code | `NeedInval` flag, cache + ExecMem reset |

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
go test -v -run TestJIT_ -tags headless ./...
```

Tests block scanning, register analysis, code cache, and ExecMem.

### Test Rig

Both backends use an identical `jitTestRig` pattern:
1. Create MachineBus + CPU64 + AllocExecMem(1MB) + newJITContext
2. `compileAndRun()` loads instructions at PROG_START, appends HALT, scans, strips terminal HALT, compiles, executes via callNative
3. Extracts packed PC from regs[0], zeros regs[0]

Mid-block RTI/WAIT tests use manual scan+compile (no HALT stripping) to verify bail behaviour.

---

## Performance Guardrails

### Built In
- Fixed guest-to-host register mapping (no dynamic allocation)
- Load only registers the block reads, store only those it writes
- Shortest instruction forms (reg-imm ALU, direct base+disp spills)
- 32-bit host ops for IE64 `.L` size where semantics match
- Fast-path fall-through for normal RAM; I/O in slow-path branch
- Non-MMU AMD64 hot-region promotion for IE64 blocks with static BRA/JMP successors
- Non-MMU AMD64 recognised-pattern shortcuts for selected IE64 benchmark loops

### Deferred
- Widening the JIT block builder and return channel to full `uint64` PC
- High-physical instruction fetch and direct data access in native blocks
- Native double-precision FPU emission
- MMU-aware IE64 region scanning and native region compilation
- ARM64 IE64 turbo-region compilation
- Memory operands for spilled-source ALU
- Peephole patterns (MOVE imm, ADD/SUB imm, compare against zero)
- Profiling-driven register residency tuning
- Smaller prologue variants for simple blocks

### Not Planned
- Full register allocation (linear scan, graph colouring, SSA)
- Dynamic guest-to-host remapping
- Instruction scheduling
- CPU-feature-dependent tricks (LZCNT, BMI2, AVX)

### Benchmarking

The benchmark suite in `ie64_benchmark_test.go` measures throughput for five workload categories through both the interpreter and JIT:

```bash
go test -tags headless -run='^$' -bench BenchmarkIE64_ -benchtime 3s ./...
```

Each benchmark reports ns/op and instructions/op. MIPS can be derived: `MIPS = instructions/op / ns/op * 1000`. See the file for detailed documentation of each workload's instruction mix.

#### Reference Results

Measured on an Intel Core i5-8365U @ 1.60 GHz (4C/8T, Whiskey Lake, 2019) running Linux amd64, `benchtime 3s`:

| Workload | Interpreter | JIT | Speedup | Interp MIPS | JIT MIPS |
|---|---|---|---|---|---|
| ALU (ADD/SUB/MUL/AND/OR/LSL) | 1,058 µs | 157 µs | **6.7x** | 85 | 574 |
| FPU (FADD/FSUB/FMUL/FDIV) | 1,242 µs | 372 µs | **3.3x** | 56 | 188 |
| Memory (LOAD/STORE sequential) | 813 µs | 105 µs | **7.7x** | 74 | 569 |
| Mixed (ALU+FP+Memory) | 1,227 µs | 159 µs | **7.7x** | 65 | 503 |
| Call (JSR/RTS loop) | 583 µs | 7,036 µs | **0.08x** | 86 | 7 |

The Call workload is intentionally JIT-hostile: every JSR and RTS terminates the native block, so each iteration pays dispatcher unpack, cache lookup, and prologue/epilogue twice. This isolates block-transition overhead and represents the worst case for the JIT. All other workloads compile into a single native block with a backward branch loop, where the JIT delivers 3-8x speedup over the interpreter.

---

## MMU Integration

When the IE64 MMU is enabled (MMU_CTRL bit 0 = 1), the JIT compiler adapts its behaviour to maintain correct virtual memory semantics. The current implementation is Stage 1: a conservative bail-to-interpreter strategy that prioritises correctness over performance.

### Stage 1: Interpreter Bail for Memory Operations

When MMU is active, the JIT compiler sets the `mmuBail` flag during block compilation via the `compileBlockMMU` wrapper. With this flag set, memory-touching and atomic instructions are emitted as immediate bail-to-interpreter exits rather than inline memory accesses:

- **LOAD, STORE** (general-purpose memory access)
- **PUSH, POP** (stack operations)
- **JSR, RTS** (subroutine call/return -- both touch the stack)
- **FLOAD, FSTORE** (FPU memory access)
- **CAS, XCHG, FAA, FAND, FOR, FXOR** (atomic memory RMW operations)

Each bailed instruction packs the current PC and retired instruction count into the return channel, stores back any modified registers, and returns to the dispatcher. The dispatcher then re-executes that single instruction through the interpreter, which performs full virtual address translation and permission checking.

RTI is a block terminator and normally reaches the interpreter through `needsFallback()` when it is the first instruction or through an emitted bail path when it appears after earlier instructions in a block.

Non-memory instructions (ALU, single-precision FPU arithmetic, branches, moves) are still compiled to native code where the emitters support them and execute at full JIT speed within the block.

**Note on atomics**: The six atomic memory operations (CAS, XCHG, FAA, FAND, FOR, FXOR) always bail to the interpreter regardless of whether the MMU is enabled. They are infrequent synchronisation operations where correctness outweighs compilation overhead, and the interpreter now owns the canonical sequentially-consistent implementation via `atomicRMW64` in `cpu_ie64.go`. Both JIT backends deliberately preserve that single source of truth rather than growing separate host-atomic sequences with subtly different semantics. The bail applies in both MMU-on and MMU-off modes.

### Block Fetch and Page Boundaries

Block scanning requires special handling under MMU:

- **Virtual PC translation**: Before scanning a block, the virtual PC is translated to a physical address through the MMU page table. The physical address is used to read instruction bytes from memory.
- **Cache key**: In MMU mode, the code cache key is `(PTBR * 0x9E3779B97F4A7C15) ^ virtualPC`, not the physical address alone. This prevents two address spaces with the same virtual PC but different page tables from sharing a stale native block.
- **Page boundary limit**: Blocks are limited to the current 4 KiB physical page during scanning. This prevents a single scanned block from crossing into bytes that may not correspond to the next virtual page mapping.

### Cache Invalidation

The JIT code cache must be flushed whenever the virtual-to-physical mapping changes. The following operations set the `jitNeedInval` flag, which causes the dispatcher to flush the code cache and reset the executable memory allocator before the next block lookup:

- **MTCR to PTBR** (CR0): The page table base has changed; all cached translations are stale.
- **MTCR to MMU_CTRL** (CR5): The MMU enable state has changed; cached blocks may have been compiled under different assumptions.
- **TLBFLUSH**: Bulk TLB invalidation implies page table changes that affect translation.
- **TLBINVAL**: Single-page TLB invalidation. Conservatively flushes the entire code cache (a targeted invalidation would require reverse-mapping virtual addresses to cached blocks).

### Block Terminators

All 9 MMU/privilege instructions (MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SYSCALL, SMODE, SUAEN, SUADIS) are block terminators. The block scanner ends the current block when any of these opcodes is encountered. This ensures that:

- Privilege level changes (SMODE, SYSCALL, ERET) take effect before the next block is compiled.
- Cache invalidations (MTCR, TLBFLUSH, TLBINVAL) are processed by the dispatcher between blocks.
- Control flow changes (ERET, SYSCALL) are handled correctly.

### Future Stages

- **Stage 2: Inline TLB check.** Emit a TLB lookup directly in the native code for each memory access. On TLB hit, perform the access inline with no dispatcher overhead. On TLB miss, bail to the interpreter for a full page table walk. This would recover most of the JIT speedup for memory-heavy workloads under MMU.
- **Stage 3: ASID-aware cache.** Tag code cache entries with an Address Space Identifier so that context switches between processes do not require a full cache flush. This would reduce the cost of MTCR to PTBR in multi-process scenarios.
