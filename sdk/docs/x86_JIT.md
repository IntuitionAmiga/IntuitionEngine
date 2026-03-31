# x86 JIT Compiler Technical Reference

## Overview

The x86 JIT compiler translates basic blocks of x86 machine code (8086 base + 386 32-bit extensions) into native x86-64 instructions at runtime. It follows the same architecture as the IE64 and M68K JITs: scan a block of instructions, compile to native code, cache the result, and dispatch via `callNative()`. Includes x87 FPU support via SSE2, self-loop native compilation, and multi-block region compilation.

**Platform support:** amd64/linux. ARM64 backend deferred.

**Activation:** Set `cpu.x86JitEnabled = true` and call `cpu.X86ExecuteJIT()` or `cpu.x86JitExecute()`. The dispatch function `x86JitExecute()` routes to the JIT when enabled, otherwise to the interpreter.

**Coverage:** 50+ instruction forms including MOV, ADD/SUB/AND/OR/XOR/CMP/TEST, INC/DEC, PUSH/POP, LEA, Jcc, JMP/CALL, SHL/SHR/SAR/ROL/ROR, NOT/NEG, MUL/IMUL/DIV/IDIV, MOVSX/MOVZX, SETcc, CMOVcc, BSF/BSR, LOOP, LEAVE, PUSHF, XCHG, CBW/CDQ, REP MOVSB/MOVSD/STOSB/STOSD/CMPSB/SCASB, x87 FADD/FSUB/FMUL/FDIV/FLD/FST/FSTP/FXCH/FCHS/FABS. RET is a block terminator handled by the Go dispatch loop (not JIT-compiled; target is stack-dependent). Segment-modifying instructions, far control flow, INT/IRET, and I/O port instructions fall back to the interpreter.

---

## Architecture

```
                    X86ExecuteJIT() Loop
                           |
              [Sync named regs -> jitRegs (once at entry)]
                           |
                    [Main loop]
                           |
                  [Check pending IRQ]
                           |
                     [Cache lookup]
                     hit? -> callNative()
                     miss? |
                 [x86ScanBlock()]
                        |
                 [x86NeedsFallback?]
                 yes -> cpu.Step() (sync jitRegs <-> named)
                 no  |
                 [x86CompileBlock()]
                        |
                 [Cache + code page bitmap mark]
                 [Patch chains (bidirectional, regMap-compatible)]
                        |
                 [Hot-block detection (execCount > 64)]
                 [-> x86FormRegion() for multi-block regions]
                 [-> x86CompileRegion() or x86CompileBlock(tier=1)]
                 [   profile-guided: skip if I/O bail rate > 25%]
                        |
                 [Update RTS cache (2-entry MRU)]
                 [callNative()]
                        |
                 [Self-loop: native backward branch]
                 [Budget counter (4095 iterations)]
                 [LEA-based counter updates (flag-preserving)]
                        |
                 [Chained execution: Block->Block->...]
                 [via patchable JMP rel32]
                        |
                 [Return on budget exhaustion]
                 [or NeedInval / NeedIOFallback]
                        |
              [Read RetPC/RetCount from context]
              [Update profile counters (chainHits, unchainedExits)]
              [NeedInval? -> invalidate cache + clear bitmaps + clear RTS cache]
              [NeedIOFallback? -> sync + cpu.Step() + sync]
                        |
              [Sync jitRegs -> named regs (once at exit)]
```

## File Inventory

| File | Build Tag | Purpose |
|------|-----------|---------|
| `jit_x86_common.go` | none | X86JITContext, X86JITInstr, block scanner, instruction length calculator, fallback tables, I/O bitmap builder, peephole optimizer, Tier 2 register allocator, multi-block region formation |
| `jit_x86_common_test.go` | none | Scanner, length calculator, context offset, bitmap, peephole, Tier 2 allocator, host feature detection tests |
| `jit_x86_cpuid.go` | `amd64 && linux` | Runtime CPUID host feature detection (BMI1, BMI2, AVX2, LZCNT, ERMS, FSRM) |
| `jit_x86_cpuid_amd64.s` | `amd64` | Assembly CPUID instruction wrapper |
| `jit_x86_cpuid_stub.go` | `!amd64 || !linux` | CPUID stub for non-amd64 platforms (all features false) |
| `jit_x86_emit_amd64.go` | `amd64 && linux` | x86-64 host emitters: prologue/epilogue, per-instruction emitters, EFLAGS passthrough, 8-bit register helpers, block chaining, deferred bail stubs, self-loop compilation, region compilation, REP range-safety, VEX/BMI2 encoding, x87 FPU emitters |
| `jit_x86_emit_amd64_test.go` | `amd64 && linux` | Per-instruction emitter tests, memory operand tests, FPU tests, REP tests, BMI2/LZCNT/ERMS tests |
| `jit_x86_exec.go` | `(amd64 || arm64) && linux` | X86ExecuteJIT main loop, init/free lifecycle, hot-block detection, chain patching, RTS cache management, profile counters |
| `jit_x86_exec_test.go` | `(amd64 || arm64) && linux` | Integration tests: HLT, multi-instruction, JIT-vs-interpreter equivalence, chaining, self-mod detection, dispatch, CMP/TEST+Jcc fusion, multi-block region |
| `jit_x86_dispatch.go` | `(amd64 || arm64) && linux` | Sets `x86JitAvailable = true`, routing function |
| `jit_x86_dispatch_stub.go` | `!(amd64 || arm64) || !linux` | Fallback stubs for unsupported platforms |
| `x86_jit_benchmark_test.go` | `(amd64 || arm64) && linux` | ALU/Memory/Mixed/String benchmark suite with Interpreter/JIT variants |

---

## X86JITContext Layout

Passed to every JIT-compiled x86 block as the sole argument (RDI on x86-64).

```
Offset  Type      Field               Description
0       uintptr   JITRegsPtr          &cpu.jitRegs[0] -- guest register file (x86 encoding order)
8       uintptr   MemPtr              &cpu.memory[0] -- direct memory base
16      uint32    MemSize             len(cpu.memory)
20      uint32    _pad0               alignment
24      uintptr   FlagsPtr            &cpu.Flags -- guest EFLAGS
32      uintptr   EIPPtr              &cpu.EIP
40      uintptr   CpuPtr              &cpu -- full CPU struct pointer
48      uint32    NeedInval           self-modification detected -> invalidate cache
52      uint32    NeedIOFallback      I/O page hit -> re-execute via interpreter
56      uint32    RetPC               next guest EIP after block
60      uint32    RetCount            retired guest instructions
64      uintptr   CodePageBitmapPtr   &codePageBitmap[0] -- self-mod detection bitmap
72      uintptr   IOBitmapPtr         &x86IOBitmap[0] -- I/O page bitmap (256-byte granularity)
80      uintptr   FPUPtr              unsafe.Pointer(cpu.FPU) -- FPU_X87 struct (not Go pointer field)
88      uintptr   SegRegsPtr          &cpu.jitSegRegs[0] -- segment registers (ES,CS,SS,DS,FS,GS)
96      uint32    ChainBudget         blocks remaining before mandatory Go return (init=64)
100     uint32    ChainCount          accumulated instruction count during chaining
104     uint32    RTSCache0PC         MRU RET target cache entry 0 -- guest PC
108     uint32    _pad1               alignment
112     uintptr   RTSCache0Addr       MRU entry 0 -- chain entry address
120     uint32    RTSCache1PC         MRU entry 1 -- guest PC
124     uint32    _pad2               alignment
128     uintptr   RTSCache1Addr       MRU entry 1 -- chain entry address
```

## Guest Register File

The JIT operates on `jitRegs [8]uint32` in x86 encoding order:

| Index | Guest Register | Notes |
|-------|---------------|-------|
| 0 | EAX | |
| 1 | ECX | Shift count source |
| 2 | EDX | MUL/DIV high word |
| 3 | EBX | |
| 4 | ESP | Stack pointer |
| 5 | EBP | |
| 6 | ESI | String source |
| 7 | EDI | String destination |

`syncJITRegsFromNamed()` / `syncJITRegsToNamed()` copy between `jitRegs` and the named CPU fields (EAX, ECX, etc.). During JIT execution, `jitRegs` is the canonical state. Named fields are synced once at JIT entry, once at JIT exit, and around interpreter fallback calls.

Segment registers use a parallel `jitSegRegs [6]uint16` array (ES=0, CS=1, SS=2, DS=3, FS=4, GS=5).

---

## Register Mapping (x86-64 Backend)

### Tier 1: Fixed Allocation

| Host Register | Guest Mapping | Purpose |
|--------------|--------------|---------|
| R15 | -- | JITContext pointer (callee-saved) |
| RSI | -- | Memory base (&memory[0]) |
| R9 | -- | I/O bitmap base (&x86IOBitmap[0]) |
| RBX | EAX | Mapped (callee-saved) |
| RBP | ECX | Mapped (callee-saved) |
| R12 | EDX | Mapped (callee-saved) |
| R13 | EBX | Mapped (callee-saved) |
| R14 | ESP | Mapped (callee-saved) |
| RAX | -- | Scratch (8-bit ops, MUL/DIV, LAHF) |
| RCX | -- | Scratch (shift count CL) |
| RDX | -- | Scratch (MUL/DIV output) |
| R8, R10, R11 | -- | Scratch |
| XMM0-XMM7 | -- | FPU scratch (x87 operand temps) |

Guest EBP, ESI, EDI are **spilled**: loaded from `jitRegs[]` into scratch registers on demand, stored back after modification.

### Tier 2: Per-Block Frequency-Based Allocation

Triggered when a block's execution count reaches 64 (profile-guided: skipped if I/O bail rate > 25%). `x86Tier2RegAlloc()` analyzes register access frequency across the block (including 0x0F opcode forms) and assigns the 5 callee-saved host slots to the most frequently used guest registers. ESP always gets a slot if used. The register mapping is stored in `JITBlock.regMap` for chain compatibility checking.

### Dirty-Register Tracking

`x86CompileState.dirtyMask` tracks which guest registers were written during block execution. The epilogue only stores dirty mapped registers back to `jitRegs[]`, eliminating unnecessary stores for read-only registers. The dirty mask is initialized from `x86AnalyzeBlockRegs()` static analysis and refined by per-instruction `x86MarkDirty()` calls.

### 8-bit Register Handling

x86 high-byte registers (AH, CH, DH, BH) are inaccessible with REX prefix on x86-64. Since mapped guest registers use REX-extended host registers (RBX, RBP, R12-R14), **all 8-bit operations go through `jitRegs[]` in memory**:

- **Low bytes** (AL/CL/DL/BL): Load 32-bit from `jitRegs[reg]` into RAX/RCX/RDX, operate on low byte, merge back with AND/OR.
- **High bytes** (AH/CH/DH/BH): Same load, SHR by 8, operate, SHL+merge back to bits 15:8.

### Stack Frame

```
6 callee-saved PUSHes (48 bytes)
+ SUB RSP, 40 (frame with loop counters)
= 88 + 8 (return address)
= 96 bytes (16-byte aligned)

Stack frame slots:
  [RSP+0]  = loop budget counter (self-loop blocks)
  [RSP+8]  = loop retired instruction counter
  [RSP+16] = loop start PC (for budget-exhaustion exit)
  [RSP+24..39] = reserved / alignment
```

---

## Self-Loop Native Compilation

When a block's backward Jcc targets its own start PC (self-loop), the JIT compiles it with a **native backward branch** instead of returning to Go on every iteration. This eliminates the `runtime.cgocall` round-trip overhead that otherwise dominates tight loops.

**Implementation:**
1. Block scanner detects backward Jcc targeting `startPC`
2. Prologue initializes loop budget (4095) and retired counter (0) in stack slots
3. Loop body is emitted normally (all instructions compile to native code)
4. At the backward Jcc: counter updates use **LEA** (flag-preserving) to avoid clobbering the guest ALU flags that the Jcc condition depends on
5. Native Jcc branches back to the loop body start label if condition is true
6. Budget check: if budget exhausted, exit to Go with `RetPC = startPC` and `RetCount = accumulated retired instructions`
7. Fall-through (condition false): exit normally with `RetCount = retired + final iteration`

**Budget counter:** 4095 iterations before mandatory Go return. Ensures interrupt delivery and cache invalidation remain responsive.

---

## Multi-Block Region Compilation

Hot blocks can be compiled together as a single native unit with one prologue/epilogue and internal native jumps. This eliminates per-block dispatch overhead for multi-block hot paths.

**Region formation** (`x86FormRegion()`):
1. Start from a hot block (execCount > 64)
2. Follow direct successor chain (JMP/Jcc targets)
3. Stop at indirect control flow, unsupported instructions, or region size limit (8 blocks, 512 instructions)
4. Detect back-edges (loops within the region)

**Region compilation** (`x86CompileRegion()`):
1. Single prologue with region-wide register allocation
2. Each block gets a label; internal JMP/Jcc become native jumps between labels
3. Back-edges include budget counter check (same as self-loop)
4. Single shared epilogue
5. Deferred bail stubs at region end

Regions are only formed for 3+ block sequences. Single-block self-loops use the lighter self-loop optimization. Two-block sequences use Tier 2 single-block recompilation.

---

## EFLAGS State Tracking

The x86-64 host's EFLAGS maps 1:1 to the x86 guest's flags (CF, PF, AF, ZF, SF, OF). After native ALU operations, host EFLAGS is already correct for the guest.

Compile-time tracking via `x86FlagState`:

| State | Meaning |
|-------|---------|
| `x86FlagsDead` | No valid flag state |
| `x86FlagsLiveArith` | Host EFLAGS valid from ADD/SUB/CMP/ADC/SBB |
| `x86FlagsLiveLogic` | Host EFLAGS valid from AND/OR/XOR/TEST (CF=OF=0) |
| `x86FlagsLiveInc` | Host EFLAGS valid from INC/DEC (CF preserved from prior) |
| `x86FlagsMaterialized` | Guest Flags word is up-to-date |

Jcc instructions emit native conditional branches directly when flags are live. CMP/TEST followed by Jcc works naturally -- the CMP/TEST sets `flagsLiveArith`/`flagsLiveLogic` and the Jcc emitter checks for live flags. When flags are not live, the Jcc emitter returns false at compile time, causing the block compiler to truncate the block before the Jcc. The Go dispatch loop then handles the Jcc via the interpreter on the next iteration. This is a compile-time fallback (shorter compiled block), not a runtime bail from compiled native code.

### Peephole Dead-Flag Analysis

`x86PeepholeFlags()` walks instructions backward to determine which flag-producing instructions have their output consumed. Returns a `[]bool` parallel to the instruction slice. This is wired into the compile path via `cs.flagsNeeded` and runs on every compilation (all tiers).

---

## Block Chaining

Direct block-to-block chaining eliminates Go dispatcher overhead between blocks.

### Chain Entry

A no-op label within the compiled block. Mapped guest registers stay live in host callee-saved registers from the previous block. No register reload needed (all Tier 1 blocks share the same fixed mapping; Tier 2 blocks only chain when `regMap` is identical).

### Chain Exit

Emitted at block terminators with statically-known targets (JMP rel8/rel32, CALL rel32):

1. Accumulate instruction count into `ChainCount`
2. Decrement `ChainBudget`; if exhausted -> unchained exit
3. Check `NeedInval`; if set -> unchained exit
4. Patchable `JMP rel32` to target block's chain entry

### Chain Safety

Chains are only patched when `source.regMap == target.regMap`. This prevents state corruption when Tier 2 blocks with different register allocations are adjacent. `x86PatchCompatibleChainsTo()` enforces this invariant.

### Unchained Exit

Stores only dirty mapped registers back to `jitRegs[]` (selective lightweight epilogue), writes `RetPC`/`RetCount` to the context, then full callee-saved restore + RET to return to Go.

### RET Address Cache (Infrastructure Only)

A 2-entry MRU cache (`RTSCache0PC/Addr`, `RTSCache1PC/Addr`) is maintained in the X86JITContext. The Go loop populates the cache before each `callNative()` call (shift entry 0 -> 1, write new -> 0) and clears it on cache invalidation. **No native code path currently consumes this cache.** RET (0xC3) is a block terminator with a stack-dependent target; `x86ResolveTerminatorTarget` returns false for RET, so it is handled by the Go dispatch loop via interpreter fallback. The cache infrastructure exists for future native RET chaining (matching the M68K JIT's RTS inline cache pattern).

---

## Memory Access

### I/O Bitmap

256-byte page granularity (`addr >> 8`), matching `MachineBus.ioPageBitmap`. Built by `buildX86IOBitmap()` which merges:

1. MachineBus I/O page bitmap (all MapIO-registered pages)
2. `translateIO` region (0xF000-0xFFFF -> 0xF0000+)
3. Bank control registers (0xF700-0xF7F1)
4. VGA VRAM (0xA0000-0xAFFFF)
5. Active bank windows (0x2000-0x9FFF when banking enabled)

### Fast Path (Direct Memory)

```asm
; addr in R10 (already masked to 25 bits)
MOV  ECX, R10d
SHR  ECX, 8              ; page index
TEST BYTE [R9 + RCX], 1  ; R9 = &ioPageBitmap[0]
JNZ  deferred_bail        ; -> shared slow path at block end
; fast path:
MOV  result, [RSI + R10]  ; RSI = &memory[0]
```

### Compile-Time Page Safety Elision

For constant effective addresses (`[disp32]`, mod=0 rm=5), the I/O bitmap page is checked at compile time. If the page is safe, the runtime IO check is skipped entirely. Similarly, self-mod checks are elided when the target page has no compiled code at compile time. Functions: `x86IsPageSafeAtCompileTime()`, `x86TryConstantEA()`, `x86EmitIOCheckMaybeElide()`, `x86EmitSelfModCheckMaybeElide()`.

### Deferred Bail Pattern

I/O checks and self-mod checks emit `JNZ` to deferred stubs collected during compilation. At block end, `x86EmitDeferredBails()` emits:

1. Per-bail stubs: write RetPC + RetCount + set NeedIOFallback or NeedInval
2. All stubs `JMP` to one shared exit
3. Shared exit: selective lightweight epilogue (dirty regs only) + full callee-saved restore + RET

This avoids inlining a full exit sequence at every memory access, keeping code compact and I-cache friendly.

### Effective Address Computation

`x86EmitComputeEA()` handles all ModR/M addressing modes:

- mod=0, rm=5: `[disp32]` (absolute address)
- mod=0, rm=4: SIB byte with optional disp32
- mod=0, rm=other: `[reg]`
- mod=1: `[reg + disp8]` or `[SIB + disp8]`
- mod=2: `[reg + disp32]` or `[SIB + disp32]`
- SIB: base + index*scale with scale=1/2/4/8

Result is masked to 25-bit address space (`AND reg, 0x01FFFFFF`).

---

## Self-Modifying Code Detection

After every JIT-compiled memory store, `x86EmitSelfModCheckMaybeElide()` tests `codePageBitmap[addr >> 8]` (skipped for compile-time-constant addresses on non-code pages). If the written page contains compiled code:

1. Store is already completed (the write happened)
2. `NeedInval = 1` set in context
3. `RetPC` = next instruction PC, `RetCount` = instructions retired including the store
4. Exit to Go via deferred bail

The Go loop then invalidates the entire code cache, resets ExecMem, clears the code page bitmap, clears the RTS cache, and resumes from `RetPC`.

---

## x87 FPU (Tier 1, SSE2)

The x87 FPU is JIT-compiled using SSE2 scalar double instructions on the x86-64 host. The `FPU_X87` struct stays in memory, accessed via `FPUPtr` in the context. Tag check infrastructure (`x86EmitFPUCheckTag`) is available for underflow/overflow detection.

### Compiled Instructions

| Opcode | Instruction | Implementation |
|--------|------------|---------------|
| D8 C0-C7 | FADD ST(0), ST(i) | ADDSD XMM0, XMM1 |
| D8 C8-CF | FMUL ST(0), ST(i) | MULSD |
| D8 E0-E7 | FSUB ST(0), ST(i) | SUBSD |
| D8 F0-F7 | FDIV ST(0), ST(i) | DIVSD |
| D9 C0-C7 | FLD ST(i) | Push: decrement TOP, MOVSD to new ST(0) |
| D9 C8-CF | FXCH ST(i) | Swap two reg slots via MOVSD |
| D9 E0 | FCHS | XORPD with sign-bit mask |
| D9 E1 | FABS | ANDPD with abs mask |
| D9 /0 mem | FLD mem32 | MOVSS + CVTSS2SD + push |
| DD /0 mem | FLD mem64 | MOVSD from memory + push |
| DD D0-D7 | FST ST(i) | Copy ST(0) to ST(i) |
| DD D8-DF | FSTP ST(i) | Copy + pop (increment TOP) |
| DD /2 mem | FST mem64 | MOVSD to memory |
| DD /3 mem | FSTP mem64 | MOVSD + pop |

### FPU State Management

TOP is read from FSW bits 13:11, updated via `x86EmitUpdateFSWTop()`. Physical register index = `(TOP + i) & 7`. FPU struct field offsets: regs at 0, FCW at 64, FSW at 66, FTW at 68.

### Interpreter Fallback

All transcendentals (FSIN, FCOS, etc.), integer conversion (FILD/FIST), comparisons (FCOM/FUCOM), control (FINIT/FLDCW/FSTCW/FSTSW), constants (FLD1/FLDPI), BCD, FCMOV, and FST/FSTP mem32 (requires PE exception flag) fall back to the interpreter.

---

## REP String Operations

REP MOVSB/MOVSD/STOSB/STOSD are compiled as native counted loops with range-safety fast paths. REP CMPSB and REP SCASB (REPE/REPNE) are compiled with flag-preserving LAHF/SAHF for ZF-based termination.

### Range-Safety Fast Path

For MOVSB/MOVSD/STOSB/STOSD, `x86EmitRangePageCheck()` verifies all pages in the source/destination range are non-I/O upfront:

1. Compute start and end page indices
2. Scan I/O bitmap for the range
3. If all pages safe: run tight fast loop with no per-iteration address masking (mask once at entry)
4. If any page unsafe: fall through to slow loop with per-iteration masking

### REP CMPSB/SCASB

REPE CMPSB: compare [ESI] vs [EDI] byte-by-byte, decrement ECX, continue while equal.
REPNE SCASB: scan [EDI] for AL, decrement ECX, continue while not equal.
Both use LAHF/SAHF to preserve CMP flags across the ECX decrement.

---

## Fallback Rules

| Category | Opcodes | Reason |
|----------|---------|--------|
| Segment register writes | 0x8E (MOV Sreg), PUSH/POP seg, LDS/LES/LFS/LGS/LSS | Segment state change |
| Far control flow | 0x9A (CALL far), 0xEA (JMP far), 0xCB/0xCA (RETF) | Segment change + complex state |
| Interrupts | 0xCC/0xCD/0xCE (INT), 0xCF (IRET) | Exception handling |
| I/O ports | 0xE4-0xE7 (IN/OUT imm), 0xEC-0xEF (IN/OUT DX), 0x6C-0x6F (INS/OUTS) | Hardware I/O |
| x87 Tier 2 | Transcendentals, FILD/FIST, FCOM, control, BCD, FST mem32 | Complex FPU state |
| Flag manipulation | CLC/STC/CLD/STD/CLI/STI/CMC | Direct flag register writes (deferred) |
| BCD arithmetic | DAA/DAS/AAA/AAS/AAM/AAD | Complex flag semantics |

---

## Profile-Guided Recompilation

Hot-block detection uses execution count with profile-guided promotion:

- **Threshold:** 64 executions before considering promotion
- **I/O bail rate check:** Blocks with > 25% I/O bail rate are not promoted (high fallback rate means JIT won't help)
- **Hysteresis:** `lastPromoteAt` prevents re-promoting recently promoted blocks
- **Counters tracked per JITBlock:** `execCount`, `chainHits`, `unchainedExits`, `ioBails`

Multi-block regions are preferred (3+ blocks with linear successor chain). Single self-loops use the lightweight self-loop optimization. Two-block sequences fall back to Tier 2 single-block recompilation.

---

## Testing

```bash
# Run all x86 JIT tests (122 tests)
go test -tags headless -run 'TestX86JIT_|TestX86InstrLength|TestX86ScanBlock|TestX86IsBlockTerminator|TestX86NeedsFallback|TestBuildX86IOBitmap|TestX86AnalyzeBlockRegs|TestSyncJIT|TestX86JITContext|TestX86PeepholeFlags|TestX86Tier2|TestX86HostFeatures' -v -count=1

# Run benchmarks (10 seconds per workload, same-session for fair ratios)
go test -tags headless -run='^$' -bench 'BenchmarkX86JIT_' -benchtime 10s -count=1

# Quick benchmark (3 seconds)
go test -tags headless -run='^$' -bench 'BenchmarkX86JIT_(ALU|Mixed)_JIT' -benchtime 3s
```

### Test Categories

| Category | Count | Description |
|----------|-------|-------------|
| Context field offsets | 20 | Verify struct layout matches constants (incl. RTS cache) |
| Register sync | 3 | Round-trip jitRegs <-> named fields |
| Instruction length | 12 | All opcode families, ModR/M, SIB, prefixes |
| Block scanner | 8 | Boundaries, terminators, prefixes, max size |
| Fallback detection | 1 | Segment/far/INT/IO fallback |
| Register analysis | 3 | Read/written bitmasks, Tier 2 allocator |
| I/O bitmap | 4 | TranslateIO, bank control, VRAM, clean RAM |
| Host feature detection | 2 | CPUID detection, package-level init |
| Peephole | 3 | Dead flags, live flags, DEC+JNZ |
| Emitter unit tests | 45+ | Per-instruction: emit, execute, verify state (incl. BMI2, LZCNT/TZCNT, ERMS) |
| Integration tests | 16 | Full programs via JIT, equivalence vs interpreter, CMP/TEST+Jcc fusion, multi-block region |
| Chain tests | 4 | JMP/CALL/multi-block/rel32 chaining |
| Self-mod test | 1 | Write to code region, verify cache invalidation |
| Benchmark correctness | 2 | ALU/CALL program equivalence |

---

## Benchmark Results

**Platform:** Intel Core i5-8365U @ 1.60GHz, Linux amd64, Go 1.24

Same-session measurements (warm CPU, mains power, `benchtime 10s`):

| Workload | Interpreter ns/op | JIT ns/op | Interpreter MIPS | JIT MIPS | Speedup |
|----------|------------------|----------|-----------------|---------|---------|
| **ALU** (10K-iteration register loop) | 3,487,308 | 37,558 | 25.8 | 2,397 | **93x** |
| **Memory** (10K store/load loop) | 3,153,375 | 47,950 | 19.0 | 1,251 | **66x** |
| **Mixed** (ALU + memory + shifts) | 5,081,658 | 51,125 | 17.7 | 1,761 | **99x** |
| **String** (REP STOSB 10K bytes) | 79,538 | 920 | -- | -- | **86x** |

ALU and Mixed benchmarks benefit most from self-loop native compilation which eliminates ~10,000 Go-native round-trips per benchmark run, replacing them with ~3 (at budget=4095). String benchmark benefits from ERMS hardware REP STOSB on proven-safe pages (native string instruction instead of JIT byte loop, 4.3x faster than pre-ERMS JIT loop).

---

## Host Feature Detection

At JIT init, `detectX86HostFeatures()` queries CPUID to detect available host CPU extensions. Features are stored in `x86HostFeatures` and passed to emitters via `x86CompileState.host`. Detection uses a local assembly CPUID helper (`jit_x86_cpuid_amd64.s`) with no external dependencies.

| Feature | CPUID Leaf | Used For |
|---------|-----------|----------|
| BMI1 | Leaf 7, EBX bit 3 | ANDN, BEXTR, TZCNT |
| BMI2 | Leaf 7, EBX bit 8 | SHLX/SHRX/SARX non-flag-affecting shifts |
| AVX2 | Leaf 7, EBX bit 5 | Future: 32-byte bulk memory ops |
| LZCNT | Leaf 0x80000001, ECX bit 5 | TZCNT for BSF, LZCNT for BSR (no false dependency) |
| ERMS | Leaf 7, EBX bit 9 | Hardware REP MOVSB/STOSB for proven-safe ranges |
| FSRM | Leaf 7, EDX bit 4 | Future: fast short REP MOVSB (Ice Lake+) |

### BMI2 Shift Optimization

When `HasBMI2` is true and the peephole dead-flag analysis determines a shift instruction's flag output has no consumer (`flagsNeeded[i] == false`), the emitter uses VEX-encoded SHLX/SHRX/SARX instead of standard SHL/SHR/SAR. These BMI2 shifts do not modify EFLAGS, preserving any prior flag state across the shift. Applies to Grp2 Ev,Ib (0xC1) with shift ops 4 (SHL), 5 (SHR), 7 (SAR).

### LZCNT/TZCNT for BSF/BSR

When `HasLZCNT` is true, BSF uses TZCNT and BSR uses LZCNT for better throughput (no false dependency on the destination register). Zero-input semantics are preserved: a TEST+JZ checks for zero before the TZCNT/LZCNT, leaving the destination unchanged and ZF=1 on zero input (matching the interpreter's behavior at `cpu_x86_grp.go:1167`). For BSR via LZCNT, the result is converted with `XOR dst, 31` to match BSR's bit-position convention.

### Hardware REP STOSB/MOVSB (ERMS)

When `HasERMS` is true and the page range is proven safe (all pages non-I/O), REP STOSB and REP MOVSB use native hardware string instructions instead of JIT byte loops. The emitter saves the RSI memory base register to a stack slot, sets up host RDI/RSI/RCX for the native REP instruction, executes CLD + REP STOSB/MOVSB, restores RSI, and computes updated guest ESI/EDI from the post-REP host register values. Only used when guest DF=0 (checked at emitter entry; DF=1 bails to interpreter).

---

## Performance Guardrails

### Built In

- Self-loop native backward branch compilation (4095-iteration budget)
- Multi-block hot-region compilation for 3+ block sequences
- Block chaining with 64-block budget before mandatory Go return
- Dirty-register tracking (selective epilogue stores)
- Deferred bail stubs (shared slow path, not per-access inline)
- Compile-time page-safety elision for constant effective addresses
- REP range-safety fast path (upfront page verification for string ops)
- jitRegs canonical during JIT execution (synced only at JIT entry/exit/interpreter fallback)
- Chain safety: regMap compatibility check prevents cross-allocation corruption
- Self-mod detection via code page bitmap on every store
- 256-byte I/O page bitmap covering adapter routing (translateIO, bank windows, VRAM)
- Profile-guided promotion (I/O bail rate, hysteresis)
- Peephole dead-flag analysis on all compilations
- CMP/TEST + Jcc fusion via native EFLAGS passthrough
- Runtime CPUID host feature detection (BMI1, BMI2, AVX2, LZCNT, ERMS, FSRM)
- BMI2 SHLX/SHRX/SARX for non-flag-affecting shifts when flag output is dead
- TZCNT/LZCNT for BSF/BSR with zero-input destination preservation
- DF (Direction Flag) check on all REP string emitters (bail to interpreter if DF=1)
- Hardware REP STOSB/MOVSB on ERMS CPUs for proven-safe ranges (native string ops)

### Deferred

- ARM64 backend
- AVX2 bulk memory for REP STOS/MOVS (VMOVDQU 32-byte loops)
- Native RET chaining via RTS cache (infrastructure exists in X86JITContext, not yet consumed by native code)
- Jcc two-way chain slots (taken + not-taken for inter-block conditional branches)
- Loop memory-check hoisting for linear base+stride patterns
- Superblock/trace compilation beyond basic region formation
