# Z80 JIT Compiler
## Overview

Block-based JIT compiler for the Z80 CPU emulation core. AMD64 has broad native lowering. Linux ARM64 emits tested native forms with frozen canonical-helper exits for the remainder. Browser js/wasm emits every non-observation opcode form, including indexed and guarded direct-memory operations, using a wasm32 context image and source-stamped cache. Port I/O uses a frozen canonical helper, so it does not re-decode mutable guest instruction bytes, and HALT returns to dispatcher polling for IRQ and NMI wake-up. It follows the same architecture as the existing 6502, M68K, x86, and IE64 JIT compilers, reusing the shared infrastructure (`jit_common.go`, `jit_mmap.go`, `jit_call.go`).

**Target workloads:** AY/tracker music playback, Spectrum-style demos, CP/M programs, coprocessor workers.

All continuous Z80 launch paths use the JIT by default when the host backend
is available. This includes bounded AY and tracker playback frames. Explicit
debugger single-step remains interpreter-driven so it retires exactly one
instruction. Playback retains its original flat 64 KiB memory contract: bank,
VRAM and translated machine-I/O address ranges remain ordinary playback RAM;
only Z80 port I/O is delegated to the AY capture bus.

## Feature inventory

| Technique | Decision | Evidence |
|-----------|----------|----------|
| Pinned-register native blocks | Adopted | `jit_z80_emit_amd64.go` pins A, F, BC, DE, HL and context in callee-saved host registers. |
| Partial flag liveness | Adopted | `jit_z80_flags_liveness.go` and emitted deferred-flag paths are covered by the Z80 flag tests. |
| Direct-memory guards | Adopted | `directPageBitmap` rejects bank, VRAM and MMIO pages before native memory access. |
| SMC generation and source stamps | Adopted | `MachineBus` physical-write publication, per-owner invalidation queues and entry source-byte checks. |
| Bounded block chaining and return-target cache | Adopted | Patchable chain exits and the two-entry RET cache observe the 64-block and 200-cycle bounds. |
| Fast MMIO polling | Adopted where its pattern matches | The native dispatcher only enters the dedicated poll path after live adapter validation. |
| Linux ARM64 direct lowering | Adopted | Every non-observation manifest row executes emitted ARM64 and is compared with the interpreter under QEMU. |
| js/wasm direct lowering | Adopted | Every non-observation manifest row executes emitted WebAssembly from the source-stamped wasm32 module cache. |
| Hot-block recompilation | Rejected | `Z80TierAllocator.PromoteBlock` is disabled: there is no repeatable workload evidence that a second tier pays for its cache pressure. |
| Static-chain regions | Adopted | `z80PromoteStaticRegion` eagerly compiles and patches up to four static JP/JR-connected blocks and 128 instructions. Each constituent keeps its source, generation, SMC and interrupt-boundary checks. |
| IE64 MMU, FPU and SIMD machinery | Inapplicable | Z80 has no equivalent guest MMU/FPU or vector execution contract. |

For amd64 performance changes, use `make z80-bench-baseline`, apply the
change, then use `make z80-bench-after z80-bench-compare` with the same
`BENCH_ITEM`. The wrappers run the four established JIT workloads three times
and preserve the raw samples plus the `benchstat` comparison. ARM64 and wasm
gates establish execution capability only and make no performance claim.

## Architecture

```
Z80 Program Memory
       |
  z80JITScanBlock()     — decode instructions, detect terminators/fallbacks
       |
  compileBlockZ80()     — emit native x86-64 via CodeBuffer
       |
  ExecMem.Write()       — copy to RWX mmap region
       |
  callNative()          — Go→native via runtime.cgocall
       |
  ExecuteJITZ80()       — exec loop: cache lookup, compile, execute, handle bail/inval
```

## Register Mapping (x86-64)

| Host Register | Z80 Register | Notes |
|---------------|-------------|-------|
| RBX (BL) | A | Callee-saved, most accessed 8-bit register |
| RBP (BPL) | F | Callee-saved, flags byte (needs REX for byte access) |
| R12W | BC (B=hi, C=lo) | Callee-saved, packed 16-bit pair |
| R13W | DE (D=hi, E=lo) | Callee-saved, packed 16-bit pair |
| R14W | HL (H=hi, L=lo) | Callee-saved, packed 16-bit pair |
| R15 | Context | Callee-saved, &Z80JITContext |
| RSI | MemBase | &MachineBus.memory[0] |
| R8 | DirectPageBM | &directPageBitmap[0] |
| R9 | CodePageBM | &codePageBitmap[0] |
| RAX,RCX,RDX | Scratch | General scratch (CL for shifts) |
| R10,R11 | Scratch | Additional scratch |

**Spilled registers:** Shadow set (A'/F'/B'-L'), IX, IY, SP, I, R, WZ, IM, IFF1, IFF2 are accessed via `[CpuPtr + offset]` from the stack frame.

## Register Mapping (ARM64)

| Host Register | Z80 Register |
|---------------|-------------|
| W19 | A |
| W20 | F |
| W21 | BC |
| W22 | DE |
| W23 | HL |
| W24 | SP (Z80) |
| X25 | MemBase |
| X26 | Context |
| X27 | DirectPageBM |
| X28 | CpuPtr |

## Memory Access Model

### Direct Page Bitmap

A 256-byte bitmap (`directPageBitmap[256]`) classifies each Z80 page:
- **0 = direct:** Native code and wasm modules read/write MachineBus RAM directly after a page guard.
- **1 = non-direct:** The emitter returns through `NeedBail`; the dispatcher executes the frozen canonical helper at that instruction boundary.

Non-direct pages: `$20-$7F` (bank windows), `$80-$BF` (VRAM), `$F0-$FF` (I/O translation). Additionally, any page with MachineBus I/O handlers is marked non-direct (checked after `SealMappings()`).

### Self-Modifying Code Detection

A `codePageBitmap[256]` tracks which pages have JIT-compiled blocks. On a guarded native or wasm store, the emitter checks this bitmap. If the target page has compiled code, it commits the write, sets `NeedInval`, and the dispatcher invalidates stale blocks before executing the next instruction.

All MachineBus physical RAM writes also publish a Z80 code-page generation to
each owning CPU. Only the owning dispatcher drains those generations and
mutates its cache. Bank and VRAM remaps use a separate mapping generation,
which invalidates logical-PC blocks even where no RAM byte changed. Every
cached block additionally retains a source-byte stamp checked before entry.

### Banked-Write Aliasing Guard

After any interpreter fallback execution, if bank windows are enabled (`bank1Enable || bank2Enable || bank3Enable || vramEnabled`), the exec loop conservatively flushes the entire JIT cache. This prevents aliased writes through bank windows from corrupting JIT-compiled code at overlapping physical addresses.

## Block Scanning

`z80JITScanBlock()` decodes instructions from raw memory, producing `[]JITZ80Instr`. Scanning stops at:

- **Block terminators:** JP, JR, JP cc, JR cc, DJNZ, CALL, CALL cc, RET, RET cc, RST, RETI, RETN, EI, DI, JP (HL/IX/IY), HALT
- **Helper or halt boundary:** I/O, block I/O, HALT, RLD/RRD and EX (SP),rr
- **Max block size:** 128 instructions
- **Page boundary:** Instruction crossing into non-direct page

Each `JITZ80Instr` stores: opcode, prefix, displacement, operand, length, cycle cost, and R register increment count (1 for unprefixed, 2 for CB/DD/FD/ED, 3 for DDCB/FDCB).

## Instruction Coverage

### Natively Compiled (Tier 1)
- **Load/Transfer:** LD r,r / LD r,n / LD r,(HL) / LD (HL),r / LD (HL),n / LD rp,nn / LD A,(BC)/(DE) / LD (BC)/(DE),A / LD SP,HL
- **ALU:** ADD/ADC/SUB/SBC/AND/OR/XOR/CP A,r / A,n / A,(HL) (with flag computation)
- **Increment:** INC/DEC r (8-bit with runtime flags) / INC/DEC rp (16-bit) / INC/DEC (HL)
- **Stack:** PUSH/POP rp including AF
- **Branch:** JP nn / JR e / JP cc,nn / JR cc,e / DJNZ (native loop)
- **Call/Return:** CALL nn / RET / CALL cc / RET cc / RST n (native stack push/pop)
- **Exchange:** EX AF,AF' / EXX / EX DE,HL
- **Rotate:** RLCA / RRCA / RLA / RRA
- **BCD:** DAA (via precomputed 2048-entry lookup table)
- **Misc:** CPL / SCF / CCF / DI / EI / NOP / LD A,(nn) / LD (nn),A / LD HL,(nn) / LD (nn),HL
- **16-bit ALU:** ADD HL,rp
- **ED prefix:** NEG / IM 0/1/2 / LD I,A / LD R,A / LD A,I / SBC HL,rp / ADC HL,rp / LDI / LDD / CPI / CPD / LD (nn),rp / LD rp,(nn)
- **CB prefix:** All 256 opcodes — RLC/RRC/RL/RR/SLA/SRA/SRL/SLL + BIT/SET/RES for both register and (HL) operands
- **DD/FD prefix:** LD r,(IX+d) / LD (IX+d),r / LD (IX+d),n / ALU A,(IX+d) / INC/DEC (IX+d) / LD IX,nn / INC IX / DEC IX / ADD IX,rp / LD SP,IX / PUSH IX / POP IX (same for IY)
- **DDCB/FDCB:** BIT/SET/RES/rotate b,(IX+d) — all indexed bit operations

### Canonical helper exits

Forms requiring host observation leave native
execution through a canonical helper. The helper receives an immutable decoded
payload and uses frozen instruction fetches, so a concurrent code mutation
cannot change its prefix, opcode, displacement or operand. Data and port
accesses remain on the live Z80 bus. This covers immediate and `(C)` port I/O
plus the block-I/O family. RLD/RRD, stack exchange, repeat memory operations
and refresh-register forms are emitted directly. HALT remains an explicit
architectural halt rather than a helper exit.

## Flag Strategy

The emitter uses a per-flag-bit peephole system for partial flag materialization.

### Peephole Pass

`z80PeepholeFlags(instrs []JITZ80Instr) []uint8` performs a backward scan over the instruction list and returns a parallel `[]uint8` bitmask array. Each element indicates which specific Z80 flag bits are consumed by subsequent instructions before the next flag producer overwrites them:

- `0x00` = no flags needed (dead producer, skip all flag materialization)
- `0xFF` (`z80FlagAll`) = all flags needed (full materialization)
- Intermediate values = partial materialization (only the needed flag bits are computed)

### Consumer Analysis

`z80InstrConsumedFlagMask(instr *JITZ80Instr) uint8` returns which specific flags each consumer instruction needs:

| Consumer | Flags Read |
|----------|-----------|
| JR NZ / JR Z | Z (0x40) |
| JR NC / JR C | C (0x01) |
| JP cc / CALL cc / RET cc | S, Z, PV, or C depending on condition |
| ADC / SBC (8-bit and 16-bit) | C (0x01) |
| RLA / RRA / RL / RR | C (0x01) |
| CCF | C (0x01) |
| DAA | N (0x02), H (0x10), C (0x01) |

### Flag Emitters

All flag computation functions accept a `flagMask uint8` parameter and conditionally skip individual flag bits not present in the mask:

- `z80EmitFlags_ADD(buf, flagMask)` — ADD/ADC: S, Z, H, P/V (overflow), C from result
- `z80EmitFlags_SUB(buf, flagMask)` — SUB/SBC/CP: same as ADD + N=1
- `z80EmitFlags_Logic(buf, isAND, flagMask)` — AND/OR/XOR: S, Z, P/V (parity via lookup table), H (1 for AND)
- `z80EmitFlags_INC_DEC_Runtime(buf, isDec, flagMask)` — INC/DEC: preserves C, runtime H and P/V

Individual flag bits (S=0x80, Z=0x40, Y=0x20, H=0x10, X=0x08, PV=0x04, N=0x02, C=0x01) are conditionally skipped when not present in `flagMask`. This enables partial materialization: e.g., `JR Z` only needs the Z flag, so only Z is computed and the remaining ~50 bytes of S/H/PV/C flag code are skipped.

### Coverage

The peephole pass covers all flag-producing instruction families: ADD/ADC/SUB/SBC/AND/OR/XOR/CP (register, immediate, and (HL) forms), INC/DEC (8-bit), DD/FD indexed ALU operations, and CB-prefix rotates/shifts. When `flagMask=0x00`, the entire flag materialization code (~60-80 bytes per instruction) is skipped.

## Block Chaining

The Z80 JIT uses block chaining to eliminate Go dispatch overhead between compiled blocks. Chained blocks execute entirely in native code, jumping directly from one block's exit to the next block's `chainEntry` via patchable `JMP rel32` instructions.

### Chain Entry / Chain Exit

Each compiled block has two entry points:
- **`entry`** (full prologue): used by Go dispatch on first entry. Loads registers from CPU struct, sets up stack frame.
- **`chainEntry`** (lightweight): used by chained blocks. Skips register loads — all Z80 registers are in callee-saved host registers (RBX=A, RBP=F, R12=BC, R13=DE, R14=HL) and survive JMPs within the same `callNative` frame.

### Chained Accounting

Three accumulator fields in `Z80JITContext` track state across chained blocks:
- **`ChainCycles`** (uint64): accumulated T-states across all blocks in the chain
- **`ChainCount`** (uint32): accumulated instruction count
- **`ChainRIncrements`** (uint32): accumulated R register increments

Every exit path (chain exit, bail, selfmod, epilogue) ADDs the current block's contribution to these accumulators before returning. The Go exec loop reads the final accumulated values — it never infers per-block values from the originally dispatched block.

### Chain Budget and Interrupt Responsiveness

Native chaining stretches interrupt latency from one-block to many-blocks. Two budget mechanisms control this:
- **`ChainBudget`** (block count): decremented at each chain exit, exits to Go when exhausted (default: 64)
- **`CycleBudget`** (cycle count): compared against accumulated `ChainCycles`, exits to Go when exceeded (default: 200 cycles ≈ 50us at 4MHz)

### RTS Cache (Return Target Chaining)

For RET instructions (dynamic target PC), a 2-entry MRU cache in the context enables native chaining:
1. After reading the return address from the stack, compare against `RTSCache0PC` / `RTSCache1PC`
2. On match, jump directly to the cached block's `chainEntry`
3. On miss, return to Go for normal dispatch

The Go exec loop populates the RTS cache when entering blocks that have `chainEntry != 0`.

### Chainable Terminators

| Terminator | Chaining | Notes |
|-----------|----------|-------|
| JP nn | Static chain exit | Target known at compile time |
| JR e | Static chain exit | Computed target |
| DJNZ | Static chain exit (both paths) | Taken + not-taken |
| JP cc,nn / JR cc,e | Static chain exit (both paths) | |
| CALL nn / RST n | Static chain exit | After stack push |
| CALL cc | Static chain exit (both paths) | |
| RET / RET cc | RTS cache | Dynamic target |
| RETI / RETN | RTS cache | Same as RET |
| EI / DI | Plain epilogue (returns to Go) | Must handle iffDelay |
| HALT | Plain epilogue (returns to Go) | |
| JP (HL/IX/IY) | Plain epilogue | Dynamic, no cache |

## Memory Specialization (DJNZ Loop Optimization)

For qualifying DJNZ loops, the JIT hoists page-check validation out of the loop body and uses unchecked direct memory access inside the loop.

### Loop Qualification

`z80AnalyzeDJNZLoop` identifies loops where:
- DJNZ is the last instruction, branching backward within the block
- Address registers (HL, DE, BC) used for memory access are only modified by INC rp (monotonic increment)
- No indexed (IX/IY) or absolute addressing
- No ED-prefix or complex memory operations

### Pre-loop Validation

On first block entry (from prologue, before `chainEntry`), a runtime check validates:
- All read-address pages are direct (directPageBitmap check)
- All write-address pages are direct AND have no compiled code (codePageBitmap check)

If validation fails, the block bails to Go for interpreter execution.

### Unchecked Access

Inside the qualifying loop body:
- `z80EmitMemReadUnchecked`: single `MOVZX EAX, [RSI+RAX]` (no page check)
- `z80EmitMemWriteUnchecked`: single `MOV [RSI+RAX], DL` (no page or self-mod check)

### Page-Crossing Guards

After each INC of an address register (HL, DE, BC), a guard checks if the low byte wrapped to 0x00:
```asm
TEST R14B, R14B  ; L == 0? (HL crossed page boundary)
JZ page_cross    ; exit block, return to Go for re-validation
```
On page cross, the block returns to Go at the next instruction's PC. Go re-enters and re-validates the new pages.

## Exec Loop Correctness Invariants

1. **NMI/IRQ/HALT** checked at every Go dispatch (chaining may delay checks by up to CycleBudget cycles)
2. **PC page safety:** Non-direct PC pages fall back to interpreter
3. **EI delay:** EI is a block terminator; exec loop uses `interpretZ80One()` for the one post-EI instruction
4. **Bail semantics:** `RetPC = current instruction PC` (interpreter re-executes). All exits merge chained accounting first.
5. **Self-mod:** Unpatch chains BEFORE invalidating blocks
6. **Banked-write alias:** `Z80BusAdapter` resolves the physical target before `MachineBus` publishes the exact written pages; unrelated cached pages survive
7. **R register:** Updated in Go exec loop from `ChainRIncrements` (accumulated across all chained blocks)
8. **HALT:** The dispatcher polls pending NMI and IRQ state before treating HALT as a stopped boundary, so an asserted interrupt wakes the CPU
9. **Chain accounting:** All exit paths (bail, selfmod, epilogue, chain exit) ADD to ChainCycles/ChainCount/ChainRIncrements before committing to RetCycles/RetCount

## Benchmark Results

Intel Core i5-8365U @ 1.60GHz, `go test -bench BenchmarkZ80_ -benchtime 3s`:

### Definitive Results (30-second runs, i5-8365U @ 1.60GHz)

| Workload | Interpreter | Interpreter MIPS | JIT | JIT MIPS | Speedup |
|----------|------------|-----------------|-----|----------|---------|
| ALU (register ops) | 43.8 us | 53 | 5.3 us | 433 | **8.2x** |
| Memory (load/store) | 24.2 us | 64 | 4.6 us | 333 | **5.3x** |
| Mixed (ALU+mem+stack) | 49.5 us | 41 | 9.0 us | 228 | **5.5x** |
| Call (CALL/RET) | 28.2 us | 36 | 11.0 us | 93 | **2.6x** |

### Cross-Architecture JIT Comparison (same machine, same benchtime)

| | Z80 JIT | 6502 JIT | M68K JIT |
|--|---------|----------|----------|
| ALU MIPS | 433 | 1405 | 1612 |
| Memory MIPS | 333 | 1106 | 335 |
| Mixed MIPS | 228 | 1389 | — |
| Call MIPS | 93 | 362 | 95 |

The Z80 JIT is 3x behind the 6502 JIT on ALU workloads due to **structural register packing overhead**: Z80 packs B:C/D:E/H:L into 16-bit host register pairs (R12W/R13W/R14W), requiring a 2-instruction MOVZX+SHR extraction per high-byte read. The 6502 maps A/X/Y to dedicated host registers with zero extraction cost. Each Z80 ALU iteration executes ~27 native instructions vs the 6502's ~11 for equivalent work. Note that the Z80 Memory benchmark (333 MIPS) is comparable to M68K's MemCopy (335 MIPS), and the Z80 Call benchmark (93 MIPS) is comparable to M68K's Call (95 MIPS). The gap is specifically in register-heavy ALU code. Closing it requires tier-2 register unpacking for hot blocks (see Future Work).

### Single-Instruction Interpreter Throughput

| Instruction | ns/op | MIPS |
|-------------|-------|------|
| NOP (dispatch) | 7.6 | 131 |
| INC r | 9.0 | 111 |
| LD A,n | 11.1 | 90 |
| LD HL,nn | 12.5 | 80 |
| JP nn | 12.5 | 80 |
| ADD A,r | 15.8 | 63 |
| LDIR | 19.0 | 53 |
| CALL+RET | 29.6 | 34 |

### Progress Over Development

| Workload | Initial JIT | Pre-Chaining | After Phases A-D | Improvement |
|----------|------------|-------------|-----------------|-------------|
| ALU | 30.6 us (1.9x) | 21.5 us (2.1x) | 5.3 us (8.2x) | 4.1x faster |
| Memory | 32.4 us (1.1x) | 20.6 us (1.2x) | 4.6 us (5.3x) | 4.5x faster |
| Mixed | 35.3 us (2.2x) | 23.9 us (2.0x) | 9.0 us (5.5x) | 2.7x faster |
| Call | — | 67.3 us (0.42x) | 11.0 us (2.6x) | 6.1x faster |

Key optimizations:
- **Block chaining** (Phase A): lightweight chain entry/exit, patchable JMP rel32, cycle-based interrupt budget, chained accounting (ChainCycles/ChainCount/ChainRIncrements). Eliminated Go dispatch overhead between blocks.
- **RTS cache**: 2-entry MRU return-target cache enables native RET→block chaining without Go round-trip. Critical for CALL/RET-heavy code.
- **Per-flag-bit partial materialization** (Phase B): peephole returns uint8 bitmask per instruction; flag emitters skip individual bits not needed by downstream consumers.
- **Unchecked memory access** (Phase C): DJNZ loop analyzer + pre-loop page validation + unchecked MemRead/MemWrite for proven-safe loops.
- **DJNZ fast path**: 3-instruction SUB+TEST+JZ replaces 9-instruction extract/dec/repack sequence.
- **Earlier**: shared exit trampoline, native LDIR loop, CB/DDCB coverage, self-mod write-before-check.

The ALU workload at 433 MIPS peaks with register-only operations in a natively-chained DJNZ loop with deferred flag materialization (flags computed only at loop exit, not per iteration). The Call workload went from 0.42x regression to 2.8x speedup via RTS cache chaining. The remaining 3-4x gap to the 6502 JIT (1405 MIPS ALU) is due to Z80 packed register extraction overhead — each B/D/H read requires 2 host instructions (MOVZX+SHR) that the 6502 avoids with dedicated host registers.

## Testing

Z80 JIT coverage includes native, ARM64 QEMU, Node wasm, and Chromium module tests:
- `jit_z80_full_fixture_test.go` - complete committed rotozoomer and Robocop binaries on fresh interpreter/JIT machines at four exact retired-instruction and deterministic frame checkpoints; compares CPU state, cycles, machine memory, versioned video state, changing framebuffer content, and IEMon snapshots on amd64, Linux ARM64 and Node wasm
- `jit_z80_common_test.go` — 25 tests: field offsets (including ChainCycles/ChainRIncrements/CycleBudget), parity table, direct page bitmap, scanner, peephole flag analysis
- `jit_z80_exec_test.go` — 76+ tests: end-to-end JIT execution, all prefix groups, self-mod, EI delay, interrupt, bail PC, cycle accuracy, banked alias, interpreter equivalence, ALU equivalence sweep (8 ops x 5 operands = 40 subtests), chain correctness (ChainBasic, ChainCallRet, ChainBudgetExhaustion, BailAfterChain, SelfModAfterChain, ChainCycleAccuracy, RETCache), memory loop optimization (unchecked, page-cross, non-direct, decrement, self-mod), lazy flags (branch consumer, CP+JR, dead producer, DAA, CB rotate)
- `jit_z80_emit_amd64_test.go` - 8 tests: per-instruction emission, register preservation, lazy flag elimination, memory bail
- `jit_z80_wasm_emit_test.go` and `jit_z80_wasm_runtime_js_test.go` - wazero ABI, exhaustive manifest admission, real WebAssembly differential execution, interrupt wake-up, helper, and invalidation coverage
- `jit_z80_wasm_browser_test.go` - Chromium module-instantiation and shared-memory access gate
- `z80_jit_benchmark_test.go` — 4 benchmark pairs (interpreter + JIT)

The matching IEScript diagnostics are
`sdk/scripts/z80_jit_rotozoomer_parity.ies` and
`sdk/scripts/z80_jit_robocop_parity.ies`. The amd64 median evidence is in
`benchmarks/z80_jit_parity_20260807`: every conformant workload improves and
the geomean latency falls by 9.47 per cent. ARM64 and wasm are capability
results only, with no performance claim.

Run: `go test -tags headless -run TestZ80JIT ./...`

## Future Work

- **Tier-2 register unpacking (D2 — primary remaining lever):** The 3-4x gap to the 6502 JIT is caused by Z80 packed register pairs (B:C in R12W, D:E in R13W, H:L in R14W). Each high-byte read (B, D, H) costs 2 host instructions (MOVZX+SHR). A tier-2 compiler could unpack hot blocks' registers into individual byte slots (stack or repurposed scratch registers), eliminating extraction overhead. `execCount` tracking is already wired; needs: promotion threshold check, tier-2 compile function with unpacked register allocation, tier-1/tier-2 equivalence tests.
- **6502-style N/Z flag deferral:** The 6502 JIT defers N/Z to a "pending register" (zero materialization cost). The Z80 always materializes into BPL. Deferring Z80's Z flag to the result register and testing it directly at conditional branches would save ~14 bytes per flag-producing instruction before a branch.
- **CP+branch fusion (D3):** Fuse `CP r; JR Z,target` into a single compare-and-branch using host flags.
- **Lower-cost concurrent generation guards:** patched chain edges currently validate both physical and logical mapping generations before every jump. Any future reduction must retain the stale-target regressions.
