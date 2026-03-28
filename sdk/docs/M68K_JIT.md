# M68020 JIT Compiler Technical Reference

## Overview

The M68020 JIT compiler translates basic blocks of 68020 machine code into native x86-64 instructions at runtime. It follows the same architecture as the IE64 JIT: scan a block of instructions, compile to native code, cache the result, and dispatch via `callNative()`.

**Platform support:** amd64/linux only. ARM64 backend is planned but not yet implemented.

**Activation:** M68K JIT is enabled by default on supported platforms when a CPU is created via `NewM68KRunner()`. Disable with the `-nojit` CLI flag, which also disables the IE64 JIT.

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
                                        |
                                   [callNative()]
                                        |
                                   [Read RetPC/RetCount]
                                        |
                              [NeedInval? → invalidate]
                              [NeedIOFallback? → StepOne()]
                              [Check interrupts/exceptions]
```

## File Inventory

### Implementation

| File | Build tag | Purpose |
|------|-----------|---------|
| `jit_m68k_common.go` | (none) | M68KJITContext, block scanner, instruction length calculator, liveness analysis |
| `jit_m68k_emit_amd64.go` | `amd64 && linux` | x86-64 native code emitter for all supported instructions |
| `jit_m68k_exec.go` | `amd64 && linux` | JIT dispatcher loop with STOP/interrupt/exception semantics |
| `jit_m68k_dispatch.go` | `amd64 && linux` | Routes `m68kJitExecute()` through JIT or interpreter |
| `jit_m68k_dispatch_stub.go` | `!amd64 \|\| !linux` | Interpreter fallback for non-JIT platforms |
| `jit_common.go` | (none) | Shared: CodeBuffer, CodeCache, JITBlock, ExecMem (reused from IE64) |
| `jit_call.go` | `(amd64 \|\| arm64) && linux` | `callNative()` via `runtime.cgocall` (reused from IE64) |
| `jit_mmap.go` | `(amd64 \|\| arm64) && linux` | Executable memory allocator (reused from IE64) |

### Tests

| File | Build tag | Purpose |
|------|-----------|---------|
| `jit_m68k_common_test.go` | `amd64 && linux` | Instruction length calculator, block scanner, liveness analysis, terminator/fallback detection |
| `jit_m68k_emit_amd64_test.go` | `amd64 && linux` | x86-64 emitter unit tests (individual instruction verification) |
| `jit_m68k_exec_test.go` | `amd64 && linux` | Integration tests through full JIT dispatcher |
| `m68k_jit_benchmark_test.go` | `amd64 && linux` | JIT vs interpreter comparative benchmarks (ALU, MemCopy, Call) |

## M68KJITContext Layout

```
Offset  Field               Description
0       DataRegsPtr         &cpu.DataRegs[0]
8       AddrRegsPtr         &cpu.AddrRegs[0]
16      MemPtr              &cpu.memory[0]
24      MemSize             len(cpu.memory)
28      IOThreshold         0xA0000 (fast-path boundary)
32      SRPtr               &cpu.SR
40      CpuPtr              &cpu
48      NeedInval           Self-modification flag
52      NeedIOFallback      I/O bail flag
56      RetPC               Next PC after block execution
60      RetCount            Instructions retired in block
64      CodePageBitmapPtr   Pointer to code page bitmap
```

## Register Mapping (x86-64)

| x86-64 | M68K | Notes |
|--------|------|-------|
| RBX | D0 | Callee-saved, mapped |
| RBP | D1 | Callee-saved, mapped |
| R12 | A0 | Callee-saved, mapped |
| R13 | A7/SP | Callee-saved, mapped |
| R14 | CCR | Callee-saved, 5-bit XNZVC |
| R15 | — | JITContext pointer |
| RDI | — | &DataRegs[0] |
| RSI | — | &cpu.memory[0] |
| R8 | — | IOThreshold |
| R9 | — | &AddrRegs[0] |
| RAX,RCX,RDX,R10,R11 | — | Scratch |

## CCR (Condition Code Register)

Maintained in R14 as a 5-bit value matching M68K SR bits 0-4:

| Bit | Flag | Updated by |
|-----|------|------------|
| 0 | C (Carry) | ADD, SUB, CMP, NEG, shifts |
| 1 | V (Overflow) | ADD, SUB, CMP, NEG |
| 2 | Z (Zero) | All flag-modifying instructions |
| 3 | N (Negative) | All flag-modifying instructions |
| 4 | X (Extend) | ADD, SUB, NEG, shifts (X=C) |

Extraction uses SETcc instructions to capture all flags before any SHL/OR clobbers them.

## Memory Access

- **Fast path**: Word-aligned addresses < 0xA0000 use direct `[memBase + addr]` with BSWAP for big-endian conversion.
- **I/O bail**: Addresses >= 0xA0000 or odd-aligned word/long access set `NeedIOFallback=1` and return to dispatcher, which re-executes via `StepOne()`.

## Self-Modifying Code Detection

Uses a heap-allocated code page bitmap (`(memSize+4095)>>12` bytes, 4KB pages). When a block is cached, its pages are marked in the bitmap. Store instructions in JIT-compiled code check the bitmap after each write; writes to code pages set `NeedInval`, triggering full cache flush and bitmap clear on return to the dispatcher.

## Backward Branch Optimisation

DBRA/Bcc loops targeting earlier instructions within the same block execute as native x86-64 backward jumps, avoiding dispatcher re-entry overhead. A budget counter (4095 iterations) limits execution before returning to the dispatcher for interrupt checking and GC safety.

## Supported Instructions (Tier 1)

MOVEQ, MOVE.B/W/L (all Tier 1 addressing modes), ADD, SUB, CMP, AND, OR, EOR, NOT, NEG, CLR, TST, SWAP, EXT/EXTB, BRA, BSR, Bcc (all 16 conditions), RTS, JSR, JMP, DBcc, Scc, ADDQ, SUBQ, LEA, PEA, LINK/UNLK, LSL, LSR, ASR, ADDA, SUBA, NOP.

## Addressing Modes (Tier 1)

Dn, An, (An), (An)+, -(An), (d16,An), (d8,An,Xn) brief, abs.W, abs.L, (d16,PC), (d8,PC,Xn) brief, #imm.

Unsupported modes (68020 full format with memory indirection) bail to interpreter.

## Bail to Interpreter

The following instructions always fall back to the interpreter via `StepOne()`:

- All FPU (Line F / 0xFxxx)
- Line A traps (0xAxxx)
- STOP, RTE, RTR, RESET, TRAP, TRAPV
- MOVEC, MOVES, CAS, CAS2
- BCD: ABCD, SBCD, PACK, UNPK
- CHK, CHK2, CMP2, CALLM, RTM
- MOVEP, TAS, BKPT
- MOVEM (Tier 2, not yet JIT-compiled)
- Any instruction using 68020 full-format addressing (memory indirect)

## Benchmark Results

Intel i5-8365U @ 1.60 GHz, Go 1.26, `go test -tags headless -bench BenchmarkM68K_`:

| Workload | Interpreter | JIT | Speedup |
|----------|-------------|-----|---------|
| ALU (MOVEQ+ADD+SUB+AND+OR+ADDQ+SWAP in DBRA loop) | 2.6 ms | 361 us | 7.2x |
| MemCopy (MOVE.L (A0)+,(A1)+ in DBRA loop) | 775 us | 203 us | 3.8x |
| Call (JSR+RTS in loop) | 1.2 ms | 9.7 ms | 0.13x |

Call workloads are slower under JIT because JSR/RTS are block terminators that exit to the dispatcher every iteration. This is expected for cross-block control flow.
