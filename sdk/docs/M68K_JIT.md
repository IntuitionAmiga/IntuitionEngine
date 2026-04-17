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

RTS uses a 2-entry MRU (most recently used) cache in M68KJITContext. Before each `callNative()`, the dispatcher updates the cache with the current block's PC and chain entry:

```
entry1 ← entry0  (shift)
entry0 ← {block.startPC, block.chainEntry}
```

In RTS-emitted code, the popped return address is compared against both entries. On hit, RTS chains directly to the matching chain entry. On miss, it returns to the Go dispatcher.

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

## File Inventory

### Implementation

| File | Build tag | Purpose |
|------|-----------|---------|
| `jit_m68k_common.go` | (none) | M68KJITContext, block scanner, instruction length calculator, liveness analysis |
| `jit_m68k_emit_amd64.go` | `amd64 && linux` | x86-64 native code emitter: instructions, chain entry/exit, lazy CCR |
| `jit_m68k_exec.go` | `amd64 && linux` | JIT dispatcher: chain patching, budget management, RTS cache, STOP/interrupt handling |
| `jit_m68k_dispatch.go` | `amd64 && linux` | Routes `m68kJitExecute()` through JIT or interpreter |
| `jit_m68k_dispatch_stub.go` | `!amd64 \|\| !linux` | Interpreter fallback for non-JIT platforms |
| `jit_common.go` | (none) | Shared: CodeBuffer, CodeCache, JITBlock, chainSlot (reused from IE64) |
| `jit_call.go` | `(amd64 \|\| arm64) && linux` | `callNative()` via `runtime.cgocall` (reused from IE64) |
| `jit_mmap.go` | `(amd64 \|\| arm64) && linux` | Executable memory allocator + `PatchRel32At` (reused from IE64) |

### Tests

| File | Build tag | Purpose |
|------|-----------|---------|
| `jit_m68k_common_test.go` | `amd64 && linux` | Instruction length, block scanner, liveness, terminators, chain infrastructure |
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
72      ChainBudget         Blocks remaining before Go return (init=64)
76      ChainCount          Accumulated instruction count during chaining
80      RTSCache0PC         MRU entry 0: M68K PC
88      RTSCache0Addr       MRU entry 0: chain entry address
96      RTSCache1PC         MRU entry 1: M68K PC
104     RTSCache1Addr       MRU entry 1: chain entry address
```

## Register Mapping (x86-64)

| x86-64 | M68K | Notes |
|--------|------|-------|
| RBX | D0 | Callee-saved, mapped |
| RBP | D1 | Callee-saved, mapped |
| R12 | A0 | Callee-saved, mapped |
| R13 | A7/SP | Callee-saved, mapped |
| R14 | CCR | Callee-saved, 5-bit XNZVC (lazy: may be stale when EFLAGS live) |
| R15 | — | JITContext pointer |
| RDI | — | &DataRegs[0] |
| RSI | — | &cpu.memory[0] |
| R8 | — | IOThreshold |
| R9 | — | &AddrRegs[0] |
| RAX,RCX,RDX,R10,R11 | — | Scratch |

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

- **Fast path**: Word-aligned addresses < 0xA0000 use direct `[memBase + addr]` with BSWAP for big-endian conversion.
- **I/O bail**: Addresses >= 0xA0000 or odd-aligned word/long access set `NeedIOFallback=1` and return to dispatcher, which re-executes via `StepOne()`.

## Self-Modifying Code Detection

Uses a heap-allocated code page bitmap (`(memSize+4095)>>12` bytes, 4KB pages). When a block is cached, its pages are marked in the bitmap. Store instructions in JIT-compiled code check the bitmap after each write; writes to code pages set `NeedInval`, triggering full cache flush, bitmap clear, and RTS cache clear on return to the dispatcher.

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
