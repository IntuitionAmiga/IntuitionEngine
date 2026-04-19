# IE64 JIT Compiler

Technical reference for the IE64 Just-In-Time compiler. Covers the shared infrastructure, dispatcher, and both platform-specific backends (ARM64 and x86-64).

---

## Overview

The IE64 JIT compiler translates blocks of IE64 machine code into native ARM64 or x86-64 instructions at runtime, executing them directly on the host CPU. This bypasses the Go interpreter loop and yields significant performance improvements for compute-heavy workloads.

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
  CodeCache.Put()         jit_common.go    Cache by startPC for O(1) lookup
        |
        v
  callNative()            jit_call.go      Execute via runtime.asmcgocall
        |
        v
  Dispatcher unpack       jit_exec.go      Extract PC + instruction count from regs[0]
```

### File Inventory

| File | Build Tag | Purpose |
|------|-----------|---------|
| `jit_common.go` | (none) | JITContext, CodeBuffer, block scanner, register analysis, code cache |
| `jit_exec.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Dispatcher loop (`ExecuteJIT`), timer handling |
| `jit_call.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | `callNative` via `runtime.asmcgocall` plus darwin exec/write protection hooks |
| `jit_call_arm64.s` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 trampoline (X0 = JITContext*) |
| `jit_call_amd64.s` | `amd64 && (linux \|\| darwin)` | SysV x86-64 trampoline (RDI = JITContext*) |
| `jit_call_amd64_windows.s` | `amd64 && windows` | Windows x86-64 trampoline |
| `jit_emit_arm64.go` | `arm64 && (linux \|\| windows \|\| darwin)` | ARM64 code emitter (~2450 lines) |
| `jit_emit_amd64.go` | `amd64 && (linux \|\| windows \|\| darwin)` | x86-64 code emitter (~1850 lines) |
| `jit_mmap.go` | `(amd64 \|\| arm64) && linux` | Linux dual-mapped executable memory (RW view + RX view) |
| `jit_mmap_windows.go` | `(amd64 \|\| arm64) && windows` | Windows executable memory backend |
| `jit_mmap_darwin_amd64.go` | `darwin && amd64` | macOS x86-64 executable memory backend |
| `jit_mmap_darwin_arm64.go` | `darwin && arm64` | macOS `MAP_JIT` executable memory backend |
| `jit_icache_arm64.go` | `arm64 && linux` | ARM64 icache flush (DC CVAU + IC IVAU) |
| `jit_icache_arm64_darwin.go` | `arm64 && darwin` | macOS arm64 icache invalidation via libSystem |
| `jit_icache_arm64.s` | `arm64 && linux` | ARM64 icache flush assembly |
| `jit_icache_amd64.go` | `amd64 && linux` | x86-64 icache no-op (coherent architecture) |
| `jit_icache_amd64_darwin.go` | `amd64 && darwin` | macOS x86-64 icache no-op |
| `jit_icache_amd64_windows.go` | `amd64 && windows` | Windows x86-64 icache no-op |
| `jit_dispatch.go` | `(amd64 && (linux \|\| windows \|\| darwin)) \|\| (arm64 && (linux \|\| windows \|\| darwin))` | Routes to `ExecuteJIT()` when enabled |
| `jit_dispatch_stub.go` | all other platforms | Fallback: always uses interpreter |

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
```

### Block Scanner

`scanBlock()` decodes IE64 instructions starting at a given PC until a block terminator is found (BRA, JMP, JSR64, RTS64, JSR_IND, HALT64, RTI64, WAIT64) or 256 instructions are reached.

### Register Liveness

`analyzeBlockRegs()` computes bitmasks of which IE64 registers are read and written by the block. This minimises prologue/epilogue overhead -- only load registers the block reads, only store registers it writes.

### CodeBuffer

Variable-length byte buffer with label/fixup support for forward references:
- `EmitBytes()` / `Emit32()` / `Emit64()` for code emission
- `Label()` / `FixupRel32()` / `Resolve()` for forward jump patching
- `PatchUint32()` for inline patching

### CodeCache

Maps `startPC -> *JITBlock` for O(1) lookup. Invalidated on self-modifying code (writes to [PROG_START, STACK_START)).

### ExecMem

16 MB dual-mapped region with bump allocator (16-byte aligned). Reset on cache invalidation.

M15.6 replaced the original single RWX mmap with a W^X-safe dual mapping:

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

Code that previously assumed a single RWX region now stays within
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

**Every exit path must pack both values:**
- Normal block end: `staticCount = len(instrs)`
- Branch taken: `staticCount = instrIdx + 1`
- I/O bail: `bailCount = ji.pcOffset / IE64_INSTR_SIZE`
- Backward-branch budget exit: dynamic count from loop counter

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

Uses X7 as iteration counter. Budget = 4095 (fits ARM64 CMP imm12). Budget exceeded → exit block, let dispatcher reset timer.

### Icache Flush

Required on ARM64. Uses DC CVAU + IC IVAU + DSB ISH + ISB per 64-byte cache line.

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

---

## I/O Dual-Path Memory Access

LOAD and STORE use a two-path strategy:

1. **Fast path** (addr < IO_REGION_START): Direct memory access via base+index. Falls through on the common path.
2. **Slow path** (addr >= IO_REGION_START):
   - Check `ioPageBitmap[addr >> 8]`
   - If I/O page: set `NeedIOFallback=1`, pack PC + bailCount, store writtenSoFar registers, return to dispatcher
   - If non-I/O page (e.g., VRAM): direct memory access

The dispatcher re-executes the bailing instruction via the interpreter after the block returns.

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

### Category C: Transcendentals (bail to interpreter)
FMOD, FSIN, FCOS, FTAN, FATAN, FLOG, FEXP, FPOW

No native equivalent on either platform. Bail to interpreter using same mechanism as I/O bail.

---

## Fallback Rules

The JIT falls back to the interpreter in these cases:

| Condition | Mechanism |
|-----------|-----------|
| HALT, WAIT, RTI as first instruction | `needsFallback()` in scanner, dispatcher calls `interpretOne()` |
| HALT, WAIT, RTI mid-block | Emitted as bail-to-interpreter (set NeedIOFallback, epilogue) |
| I/O page memory access | Dual-path: bail to interpreter on I/O bitmap hit |
| FPU transcendentals | Always bail to interpreter |
| Atomic RMW (CAS, XCHG, FAA, FAND, FOR, FXOR) | Always bail to interpreter (MMU-on and MMU-off; centralized SC semantics) |
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

Mid-block RTI/WAIT tests use manual scan+compile (no HALT stripping) to verify bail behavior.

---

## Performance Guardrails

### Built In
- Fixed guest-to-host register mapping (no dynamic allocation)
- Load only registers the block reads, store only those it writes
- Shortest instruction forms (reg-imm ALU, direct base+disp spills)
- 32-bit host ops for IE64 `.L` size where semantics match
- Fast-path fall-through for normal RAM; I/O in slow-path branch

### Deferred
- Memory operands for spilled-source ALU
- Peephole patterns (MOVE imm, ADD/SUB imm, compare against zero)
- Profiling-driven register residency tuning
- Smaller prologue variants for simple blocks

### Not Planned
- Full register allocation (linear scan, graph coloring, SSA)
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

When the IE64 MMU is enabled (MMU_CTRL bit 0 = 1), the JIT compiler adapts its behavior to maintain correct virtual memory semantics. The current implementation is Stage 1: a conservative bail-to-interpreter strategy that prioritises correctness over performance.

### Stage 1: Interpreter Bail for Memory Operations

When MMU is active, the JIT compiler sets the `mmuBail` flag during block compilation via the `compileBlockMMU` wrapper. With this flag set, all 15 memory-touching and atomic instructions are emitted as immediate bail-to-interpreter exits rather than inline memory accesses:

- **LOAD, STORE** (general-purpose memory access)
- **PUSH, POP** (stack operations)
- **JSR, RTS** (subroutine call/return -- both touch the stack)
- **FLOAD, FSTORE** (FPU memory access)
- **RTI** (pops return address from stack)
- **CAS, XCHG, FAA, FAND, FOR, FXOR** (atomic memory RMW operations)

Each bailed instruction packs the current PC and retired instruction count into the return channel, stores back any modified registers, and returns to the dispatcher. The dispatcher then re-executes that single instruction through the interpreter, which performs full virtual address translation and permission checking.

Non-memory instructions (ALU, FPU arithmetic, branches, moves) are still compiled to native code and execute at full JIT speed within the block.

**Note on atomics**: The six atomic memory operations (CAS, XCHG, FAA, FAND, FOR, FXOR) always bail to the interpreter regardless of whether the MMU is enabled. They are infrequent synchronisation operations where correctness outweighs compilation overhead, and the interpreter now owns the canonical sequentially-consistent implementation via `atomicRMW64` in `cpu_ie64.go`. Both JIT backends deliberately preserve that single source of truth rather than growing separate host-atomic sequences with subtly different semantics. The bail applies in both MMU-on and MMU-off modes.

### Block Fetch and Page Boundaries

Block scanning requires special handling under MMU:

- **Virtual PC translation**: Before scanning a block, the virtual PC is translated to a physical address through the MMU page table. The physical address is used to read instruction bytes from memory.
- **Cache key**: The code cache is keyed by the **virtual** PC, not the physical address. This ensures correct behavior when the same physical page is mapped at different virtual addresses.
- **Page boundary limit**: Blocks are limited to the current 4 KiB page. If a block scan reaches a page boundary (offset 0xFFF within the page), the block is terminated even if no terminator instruction has been encountered. This prevents a single block from spanning two virtual pages that may have different physical mappings or permissions.

### Cache Invalidation

The JIT code cache must be flushed whenever the virtual-to-physical mapping changes. The following operations set the `jitNeedInval` flag, which causes the dispatcher to flush the code cache and reset the executable memory allocator before the next block lookup:

- **MTCR to PTBR** (CR0): The page table base has changed; all cached translations are stale.
- **MTCR to MMU_CTRL** (CR5): The MMU enable state has changed; cached blocks may have been compiled under different assumptions.
- **TLBFLUSH**: Bulk TLB invalidation implies page table changes that affect translation.
- **TLBINVAL**: Single-page TLB invalidation. Conservatively flushes the entire code cache (a targeted invalidation would require reverse-mapping virtual addresses to cached blocks).

### Block Terminators

All 7 MMU instructions (MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SYSCALL, SMODE) are block terminators. The block scanner ends the current block when any of these opcodes is encountered. This ensures that:

- Privilege level changes (SMODE, SYSCALL, ERET) take effect before the next block is compiled.
- Cache invalidations (MTCR, TLBFLUSH, TLBINVAL) are processed by the dispatcher between blocks.
- Control flow changes (ERET, SYSCALL) are handled correctly.

### Future Stages

- **Stage 2: Inline TLB check.** Emit a TLB lookup directly in the native code for each memory access. On TLB hit, perform the access inline with no dispatcher overhead. On TLB miss, bail to the interpreter for a full page table walk. This would recover most of the JIT speedup for memory-heavy workloads under MMU.
- **Stage 3: ASID-aware cache.** Tag code cache entries with an Address Space Identifier so that context switches between processes do not require a full cache flush. This would reduce the cost of MTCR to PTBR in multi-process scenarios.
