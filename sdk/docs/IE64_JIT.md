# IE64 JIT Compiler

Technical reference for the IE64 Just-In-Time compiler. Covers the shared infrastructure, dispatcher, and both platform-specific backends (ARM64 and x86-64).

---

## Overview

The IE64 JIT compiler translates blocks of IE64 machine code into native ARM64 or x86-64 instructions at runtime, executing them directly on the host CPU. This bypasses the Go interpreter loop and yields significant performance improvements for compute-heavy workloads.

**Supported platforms:** ARM64/Linux, x86-64/Linux

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
  ExecMem.Write()         jit_mmap.go      Copy to RWX region + icache flush
        |
        v
  CodeCache.Put()         jit_common.go    Cache by startPC for O(1) lookup
        |
        v
  callNative()            jit_call.go      Execute via runtime.cgocall
        |
        v
  Dispatcher unpack       jit_exec.go      Extract PC + instruction count from regs[0]
```

### File Inventory

| File | Build Tag | Purpose |
|------|-----------|---------|
| `jit_common.go` | (none) | JITContext, CodeBuffer, block scanner, register analysis, code cache |
| `jit_exec.go` | `(amd64\|\|arm64) && linux` | Dispatcher loop (`ExecuteJIT`), timer handling |
| `jit_call.go` | `(amd64\|\|arm64) && linux` | `callNative` via `runtime.cgocall` |
| `jit_call_arm64.s` | `arm64 && linux` | ARM64 trampoline (X0 = JITContext*) |
| `jit_call_amd64.s` | `amd64 && linux` | x86-64 trampoline (RDI = JITContext*) |
| `jit_emit_arm64.go` | `arm64 && linux` | ARM64 code emitter (~2450 lines) |
| `jit_emit_amd64.go` | `amd64 && linux` | x86-64 code emitter (~1850 lines) |
| `jit_mmap.go` | `(amd64\|\|arm64) && linux` | Executable memory (mmap RWX, bump allocator) |
| `jit_icache_arm64.go` | `arm64 && linux` | ARM64 icache flush (DC CVAU + IC IVAU) |
| `jit_icache_arm64.s` | `arm64 && linux` | ARM64 icache flush assembly |
| `jit_icache_amd64.go` | `amd64 && linux` | x86-64 icache no-op (coherent architecture) |
| `jit_dispatch.go` | `(amd64\|\|arm64) && linux` | Routes to `ExecuteJIT()` when enabled |
| `jit_dispatch_stub.go` | `!(amd64\|\|arm64) \|\| !linux` | Fallback: always uses interpreter |

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

16MB mmap'd RWX region with bump allocator (16-byte aligned). Reset on cache invalidation.

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

The benchmark suite in `ie64_benchmark_test.go` measures throughput for five workload categories (ALU, FPU, Memory, Mixed, Call/Return) through both the interpreter and JIT:

```bash
go test -tags headless -run='^$' -bench BenchmarkIE64_ -benchtime 3s ./...
```

Each benchmark reports ns/op and instructions/op. See the file for detailed documentation of each workload's instruction mix and expected performance characteristics.
