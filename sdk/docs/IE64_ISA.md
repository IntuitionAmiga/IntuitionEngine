# IE64 Instruction Set Architecture Reference

Intuition Engine 64-bit RISC CPU -- Complete ISA Specification

(c) 2024-2026 Zayn Otley -- GPLv3 or later

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Register File](#2-register-file)
3. [Instruction Encoding](#3-instruction-encoding)
4. [Complete Instruction Reference](#4-complete-instruction-reference)
    - [4.1 Data Movement](#41-data-movement)
    - [4.2 Load/Store](#42-loadstore)
    - [4.3 Arithmetic](#43-arithmetic)
    - [4.4 Logical](#44-logical)
    - [4.5 Shifts](#45-shifts)
    - [4.6 Floating Point (FPU)](#46-floating-point-fpu)
    - [4.7 Branches](#47-branches)
    - [4.8 Subroutine / Stack](#48-subroutine--stack)
    - [4.9 System](#49-system)
5. [Pseudo-Instructions](#5-pseudo-instructions)
6. [Addressing Modes](#6-addressing-modes)
7. [Branch Architecture](#7-branch-architecture)
8. [Memory Map](#8-memory-map)
9. [Interrupt/Timer System](#9-interrupttimer-system)
10. [64-bit Constant Loading](#10-64-bit-constant-loading)
11. [Assembly Language Quick Reference](#11-assembly-language-quick-reference)
12. [Memory Management Unit](#12-memory-management-unit)

---

## 1. Architecture Overview

The IE64 is a 64-bit RISC load-store CPU designed for the Intuition Engine platform. It uses a clean, regular instruction encoding with the following core characteristics:

- **Word size**: 64-bit registers, 64-bit data path
- **Instruction width**: Fixed 8 bytes (64 bits) per instruction
- **Byte order**: Little-endian throughout (instruction encoding, memory access, immediates)
- **Architecture class**: Load-store (all computation on registers; memory accessed only via LOAD/STORE)
- **Condition model**: Compare-and-branch (no flags register)
- **Register file**: 32 general-purpose 64-bit registers (R0 hardwired to zero)
- **Address space**: 64-bit physical/virtual (PLAN_MAX_RAM.md slice 3 widened the bus to `uint64`); IE64 sees the full active visible RAM. The legacy 25-bit PC mask was retired in slice 3. Active visible RAM is reported through `CR_RAM_SIZE_BYTES` and the `SYSINFO_ACTIVE_RAM_LO/HI` MMIO pair; total guest RAM is reported through `SYSINFO_TOTAL_RAM_LO/HI`.
- **Stack**: Full-descending, R31 serves as stack pointer. Hardware enforces 8-byte granularity for PUSH/POP; the IntuitionOS ABI requires 16-byte alignment at call boundaries (see [`IE64_ABI.md`](IE64_ABI.md))
- **Interrupt model**: Single vector, maskable, with timer support

---

## 2. Register File

The IE64 has 32 general-purpose 64-bit registers, addressed by a 5-bit field (0-31).

| Register | Alias | Description |
|----------|-------|-------------|
| R0       | --    | Hardwired zero. Reads always return 0. Writes are silently discarded. |
| R1-R30   | --    | General-purpose registers. 64-bit read/write. |
| R31      | SP    | Stack pointer. Used implicitly by PUSH, POP, JSR, RTS, RTI, and interrupt entry. Initialized to `0x9F000` on reset. |

**Floating Point Registers (F0-F15)**:
- 16 dedicated 32-bit registers for IEEE-754 single-precision floating point.
- Accessed via dedicated FPU instructions (0x60-0x7C).
- Initialized to 0.0 on reset.

**Program Counter (PC)**:
- 64-bit internal register, not directly addressable.
- Full 64-bit PC. The historical 25-bit `PC & 0x1FFFFFF` mask was retired in PLAN_MAX_RAM.md slice 3 along with the rest of the IE64 `uint32` plumbing; the CPU now reaches the full active visible RAM, which may exceed 4 GiB on hosts with sufficient memory.
- Initialized to `0x1000` (PROG_START) on reset.
- Advanced by 8 after each non-branch instruction.

**There is no flags register.** All conditional branches use explicit register-register comparison within the branch instruction itself.

---

## 3. Instruction Encoding

Every IE64 instruction is exactly 8 bytes (64 bits), encoded in little-endian byte order.

### 3.1 Byte-Level Format

```
Byte:    0         1              2              3            4    5    6    7
       +--------+----------+----------+----------+------+------+------+------+
       | Opcode | Rd|Sz|X  | Rs|unused| Rt|unused|       imm32 (LE)         |
       +--------+----------+----------+----------+------+------+------+------+
Bits:   [7:0]    [7:3][2:1][0] [7:3][2:0] [7:3][2:0]    [31:0]
```

### 3.2 Field Definitions

| Field  | Byte | Bits     | Width | Description |
|--------|------|----------|-------|-------------|
| opcode | 0    | [7:0]    | 8     | Instruction opcode |
| Rd     | 1    | [7:3]    | 5     | Destination register index (0-31) |
| Size   | 1    | [2:1]    | 2     | Operand size code |
| X      | 1    | [0]      | 1     | Operand mode: 0 = register Rt, 1 = immediate imm32 |
| Rs     | 2    | [7:3]    | 5     | First source register index (0-31) |
| unused | 2    | [2:0]    | 3     | Reserved (must be 0) |
| Rt     | 3    | [7:3]    | 5     | Second source register index (0-31) |
| unused | 3    | [2:0]    | 3     | Reserved (must be 0) |
| imm32  | 4-7  | [31:0]   | 32    | 32-bit immediate value (little-endian) |

### 3.3 Field Extraction Formulas

```
rd   = byte1 >> 3           // upper 5 bits of byte 1
size = (byte1 >> 1) & 0x03  // bits [2:1] of byte 1
xbit = byte1 & 1            // bit [0] of byte 1
rs   = byte2 >> 3           // upper 5 bits of byte 2
rt   = byte3 >> 3           // upper 5 bits of byte 3
imm32 = bytes[4] | (bytes[5] << 8) | (bytes[6] << 16) | (bytes[7] << 24)
```

### 3.4 Encoding Helper (Assembler)

```
instr[0] = opcode
instr[1] = (rd << 3) | (size << 1) | xbit
instr[2] = rs << 3
instr[3] = rt << 3
instr[4..7] = imm32 (little-endian)
```

### 3.5 Size Codes

| Code | Suffix | Width  | Mask               |
|------|--------|--------|--------------------|
| 0    | `.B`   | 8-bit  | `val & 0xFF`       |
| 1    | `.W`   | 16-bit | `val & 0xFFFF`     |
| 2    | `.L`   | 32-bit | `val & 0xFFFFFFFF` |
| 3    | `.Q`   | 64-bit | `val` (no mask)    |

If no size suffix is specified in assembly, the default is `.Q` (64-bit).

### 3.6 Third Operand Resolution

When X=0, the third operand is the value of register Rt: `operand3 = regs[rt]`

When X=1, the third operand is the immediate, zero-extended to 64 bits: `operand3 = uint64(imm32)`

---

## 4. Complete Instruction Reference

### 4.1 Data Movement

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| MOVE     | `0x01` | `move.s Rd, Rs` | `Rd = maskToSize(Rs, s)` | N | B/W/L/Q |
| MOVE     | `0x01` | `move.s Rd, #imm` | `Rd = maskToSize(imm32, s)` | N | B/W/L/Q |
| MOVT     | `0x02` | `movt Rd, #imm` | `Rd = (Rd & 0x00000000FFFFFFFF) \| (imm32 << 32)` | N | -- |
| MOVEQ    | `0x03` | `moveq Rd, #imm` | `Rd = signExtend32to64(imm32)` | N | -- |
| LEA      | `0x04` | `lea Rd, disp(Rs)` | `Rd = Rs + signExtend32to64(imm32)` | N | -- |

**MOVE** (opcode `0x01`):
- Register form (X=0): copies Rs to Rd, masked to the specified size.
- Immediate form (X=1): loads a zero-extended 32-bit immediate into Rd, masked to the specified size.
- Encoding: `Rd` in byte 1[7:3], `Rs` in byte 2[7:3] (register form), `imm32` in bytes 4-7 (immediate form).

**MOVT** (opcode `0x02`):
- Loads a 32-bit immediate into the upper 32 bits of Rd, preserving the lower 32 bits.
- Always uses X=1. Size is forced to Q internally.
- Used in conjunction with `move.l` to construct 64-bit constants.

**MOVEQ** (opcode `0x03`):
- Sign-extends a 32-bit immediate to 64 bits and stores in Rd.
- The immediate is interpreted as a signed 32-bit value: `Rd = int64(int32(imm32))`.
- Always uses X=1. Size is forced to Q internally.

**LEA** (opcode `0x04`):
- Computes an effective address without accessing memory.
- The displacement (imm32) is sign-extended to 64 bits before adding to Rs.
- Always uses X=1. Size is forced to Q internally.

#### Data Movement Semantics

| Mnemonic | Sign-extend | Zero-extend | Size Mask Applied |
|----------|-------------|-------------|-------------------|
| MOVE (reg)  | No | No | Yes (to specified size) |
| MOVE (imm)  | No | Yes (imm32 to 64-bit) | Yes (to specified size) |
| MOVT     | No | No | No (upper 32 bits replaced) |
| MOVEQ    | Yes (32 to 64) | No | No |
| LEA      | Yes (disp, 32 to 64) | No | No |

---

### 4.2 Load/Store

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| LOAD     | `0x10` | `load.s Rd, (Rs)` | `Rd = mem[Rs]` | Read | B/W/L/Q |
| LOAD     | `0x10` | `load.s Rd, disp(Rs)` | `Rd = mem[Rs + signExtend(disp)]` | Read | B/W/L/Q |
| STORE    | `0x11` | `store.s Rd, (Rs)` | `mem[Rs] = maskToSize(Rd, s)` | Write | B/W/L/Q |
| STORE    | `0x11` | `store.s Rd, disp(Rs)` | `mem[Rs + signExtend(disp)] = maskToSize(Rd, s)` | Write | B/W/L/Q |

**LOAD** (opcode `0x10`):
- Reads from memory at address `Rs + signExtend32to64(imm32)`, truncated to 32-bit address.
- The displacement is sign-extended from 32 to 64 bits before being added to Rs.
- The loaded value is zero-extended to 64 bits (for sizes B, W, L) and stored in Rd.
- X bit is set to 1 by the assembler when displacement is non-zero.

**STORE** (opcode `0x11`):
- Writes to memory at address `Rs + signExtend32to64(imm32)`, truncated to 32-bit address.
- The value from Rd is masked to the specified size before writing.
- VRAM direct-write fast path: stores to the VRAM region (`0xA0000`-end) bypass the bus for performance.

#### Load/Store Semantics

| Mnemonic | Direction | Sign-extend on Load | Size Mask on Store | Address Calculation |
|----------|-----------|--------------------|--------------------|---------------------|
| LOAD     | Mem -> Reg | No (zero-extended) | N/A | `uint32(int64(Rs) + int64(int32(imm32)))` |
| STORE    | Reg -> Mem | N/A | Yes | `uint32(int64(Rs) + int64(int32(imm32)))` |

#### Memory Access Widths

| Size | Bytes Read/Written | Bus Method |
|------|--------------------|------------|
| `.B` | 1 | Read8/Write8 |
| `.W` | 2 | Read16/Write16 |
| `.L` | 4 | Read32/Write32 |
| `.Q` | 8 | Read64/Write64 |

> **64-bit memory access**: `.Q` loads and stores go through the MachineBus (`Read64`/`Write64`). For plain RAM (no I/O region on either 32-bit half), the bus uses a single 64-bit read/write. If the access spans an I/O region, the bus may split it into two 32-bit halves. For MMIO64 regions receiving a split write, a read-modify-write is performed on backing memory to preserve the untouched half. These semantics are transparent for normal RAM access but matter when accessing hardware registers with `.Q` size.

---

### 4.3 Arithmetic

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| ADD      | `0x20` | `add.s Rd, Rs, Rt` | `Rd = maskToSize(Rs + Rt, s)` | N | B/W/L/Q |
| ADD      | `0x20` | `add.s Rd, Rs, #imm` | `Rd = maskToSize(Rs + imm32, s)` | N | B/W/L/Q |
| SUB      | `0x21` | `sub.s Rd, Rs, Rt` | `Rd = maskToSize(Rs - Rt, s)` | N | B/W/L/Q |
| SUB      | `0x21` | `sub.s Rd, Rs, #imm` | `Rd = maskToSize(Rs - imm32, s)` | N | B/W/L/Q |
| MULU     | `0x22` | `mulu.s Rd, Rs, Rt` | `Rd = maskToSize(Rs * Rt, s)` (unsigned) | N | B/W/L/Q |
| MULU     | `0x22` | `mulu.s Rd, Rs, #imm` | `Rd = maskToSize(Rs * imm32, s)` (unsigned) | N | B/W/L/Q |
| MULS     | `0x23` | `muls.s Rd, Rs, Rt` | `Rd = maskToSize(int64(Rs) * int64(Rt), s)` (signed) | N | B/W/L/Q |
| MULS     | `0x23` | `muls.s Rd, Rs, #imm` | `Rd = maskToSize(int64(Rs) * int64(imm32), s)` (signed) | N | B/W/L/Q |
| DIVU     | `0x24` | `divu.s Rd, Rs, Rt` | `Rd = maskToSize(Rs / Rt, s)` (unsigned) | N | B/W/L/Q |
| DIVU     | `0x24` | `divu.s Rd, Rs, #imm` | `Rd = maskToSize(Rs / imm32, s)` (unsigned) | N | B/W/L/Q |
| DIVS     | `0x25` | `divs.s Rd, Rs, Rt` | `Rd = maskToSize(int64(Rs) / int64(Rt), s)` (signed) | N | B/W/L/Q |
| DIVS     | `0x25` | `divs.s Rd, Rs, #imm` | `Rd = maskToSize(int64(Rs) / int64(imm32), s)` (signed) | N | B/W/L/Q |
| MOD      | `0x26` | `mod.s Rd, Rs, Rt` | `Rd = maskToSize(Rs % Rt, s)` (unsigned) | N | B/W/L/Q |
| MOD      | `0x26` | `mod.s Rd, Rs, #imm` | `Rd = maskToSize(Rs % imm32, s)` (unsigned) | N | B/W/L/Q |
| NEG      | `0x27` | `neg.s Rd, Rs` | `Rd = maskToSize(-int64(Rs), s)` | N | B/W/L/Q |
| MODS     | `0x28` | `mods.s Rd, Rs, Rt/#imm` | Signed modulo with truncation-toward-zero semantics | N | B/W/L/Q |
| MULHU    | `0x29` | `mulhu Rd, Rs, Rt` | Upper 64 bits of unsigned `Rs * Rt` | N | Q |
| MULHS    | `0x2A` | `mulhs Rd, Rs, Rt` | Upper 64 bits of signed `Rs * Rt` | N | Q |

**Division by zero**: If the divisor (Rt or imm32) is zero, the result is 0 (no exception raised). This applies to DIVU, DIVS, and MOD.

**NEG** is a 2-operand instruction: it reads Rs and writes the two's complement negation to Rd.

#### Arithmetic Semantics

| Mnemonic | Signed | Division-by-Zero | Result Masking |
|----------|--------|------------------|----------------|
| ADD      | No (unsigned add) | N/A | Yes |
| SUB      | No (unsigned sub) | N/A | Yes |
| MULU     | No (unsigned multiply) | N/A | Yes |
| MULS     | Yes (signed multiply) | N/A | Yes |
| DIVU     | No (unsigned divide) | Result = 0 | Yes |
| DIVS     | Yes (signed divide) | Result = 0 | Yes |
| MOD      | No (unsigned modulo) | Result = 0 | Yes |
| NEG      | Yes (two's complement) | N/A | Yes |

---

### 4.4 Logical

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| AND      | `0x30` | `and.s Rd, Rs, Rt` | `Rd = maskToSize(Rs & Rt, s)` | N | B/W/L/Q |
| AND      | `0x30` | `and.s Rd, Rs, #imm` | `Rd = maskToSize(Rs & imm32, s)` | N | B/W/L/Q |
| OR       | `0x31` | `or.s Rd, Rs, Rt` | `Rd = maskToSize(Rs \| Rt, s)` | N | B/W/L/Q |
| OR       | `0x31` | `or.s Rd, Rs, #imm` | `Rd = maskToSize(Rs \| imm32, s)` | N | B/W/L/Q |
| EOR      | `0x32` | `eor.s Rd, Rs, Rt` | `Rd = maskToSize(Rs ^ Rt, s)` | N | B/W/L/Q |
| EOR      | `0x32` | `eor.s Rd, Rs, #imm` | `Rd = maskToSize(Rs ^ imm32, s)` | N | B/W/L/Q |
| NOT      | `0x33` | `not.s Rd, Rs` | `Rd = maskToSize(~Rs, s)` | N | B/W/L/Q |

**NOT** is a 2-operand instruction. It performs bitwise complement of Rs, masked to the specified size, and stores the result in Rd.

---

### 4.5 Shifts

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| LSL      | `0x34` | `lsl.s Rd, Rs, Rt` | `Rd = maskToSize(Rs << (Rt & 63), s)` | N | B/W/L/Q |
| LSL      | `0x34` | `lsl.s Rd, Rs, #imm` | `Rd = maskToSize(Rs << (imm32 & 63), s)` | N | B/W/L/Q |
| LSR      | `0x35` | `lsr.s Rd, Rs, Rt` | `Rd = maskToSize(Rs >> (Rt & 63), s)` | N | B/W/L/Q |
| LSR      | `0x35` | `lsr.s Rd, Rs, #imm` | `Rd = maskToSize(Rs >> (imm32 & 63), s)` | N | B/W/L/Q |
| ASR      | `0x36` | `asr.s Rd, Rs, Rt` | `Rd = maskToSize(signedRs >> (Rt & 63), s)` | N | B/W/L/Q |
| ASR      | `0x36` | `asr.s Rd, Rs, #imm` | `Rd = maskToSize(signedRs >> (imm32 & 63), s)` | N | B/W/L/Q |
| CLZ      | `0x37` | `clz.l Rd, Rs` | `Rd = LeadingZeros32(uint32(Rs))` | N | L |
| SEXT     | `0x38` | `sext.s Rd, Rs` | Sign-extend byte/word/long source to 64 bits | N | B/W/L/Q |
| ROL      | `0x39` | `rol.s Rd, Rs, Rt/#imm` | Rotate left within the selected width | N | B/W/L/Q |
| ROR      | `0x3A` | `ror.s Rd, Rs, Rt/#imm` | Rotate right within the selected width | N | B/W/L/Q |
| CTZ      | `0x3B` | `ctz.l Rd, Rs` | `Rd = TrailingZeros32(uint32(Rs))` | N | L |
| POPCNT   | `0x3C` | `popcnt.l Rd, Rs` | `Rd = OnesCount32(uint32(Rs))` | N | L |
| BSWAP    | `0x3D` | `bswap.l Rd, Rs` | `Rd = ReverseBytes32(uint32(Rs))` | N | L |

**Shift amount masking**: The shift count is always masked to 6 bits (`& 63`), limiting the effective shift range to 0-63.

**CLZ (Count Leading Zeros)**: A 2-operand instruction that counts the number of leading zero bits in the low 32 bits of Rs and stores the result in Rd. The result is an integer in the range 0–32: zero if the most-significant bit is set, 32 if the input is zero. Only the `.l` size suffix is supported. Writing to R0 is silently discarded (as with all instructions).

This instruction is particularly useful for O(1) floating-point normalisation, integer log₂ computation, and highest-set-bit detection. See the IE64 Cookbook for worked examples.

**ASR sign extension**: Before performing the arithmetic right shift, the source value is sign-extended according to the current size:

| Size | Sign-extension |
|------|----------------|
| `.B` | `int64(int8(Rs))` |
| `.W` | `int64(int16(Rs))` |
| `.L` | `int64(int32(Rs))` |
| `.Q` | `int64(Rs)` |

The result is then masked to the specified size after the shift.

**LSL and LSR** operate on the unsigned 64-bit value in Rs. The result is masked to the specified size.

---

### 4.6 Floating Point (FPU)

The IE64 FPU provides native single-precision (`f*`) and double-precision
(`d*`) IEEE-754 operations. Single-precision values use the 16 scalar registers
`f0`-`f15`. Double-precision values use even-odd register pairs:

- `d0` = `f0:f1`
- `d1` = `f2:f3`
- `d2` = `f4:f5`
- `d3` = `f6:f7`
- `d4` = `f8:f9`
- `d5` = `f10:f11`
- `d6` = `f12:f13`
- `d7` = `f14:f15`

All `d*` mnemonics require even-numbered FP operands in assembly. Writing a
double clobbers both halves of the pair.

#### 4.6.1 FPU Data Movement

| Mnemonic | Opcode | Syntax | Operation |
|----------|--------|--------|-----------|
| FMOV     | `0x60` | `fmov fd, fs` | `fd = fs` (FP copy) |
| FLOAD    | `0x61` | `fload fd, disp(rs)` | `fd = mem32[rs + disp]` |
| FSTORE   | `0x62` | `fstore fs, disp(rd)` | `mem32[rd + disp] = fs` |
| FMOVECR  | `0x78` | `fmovecr fd, #idx` | `fd = ROM_Constant[idx]` |

**FLOAD/FSTORE** always transfer 4 bytes (32 bits) between memory and an FP register. The `disp` is a signed 32-bit immediate. **FSTORE** uses `fs` as the source floating-point register and `rd` as the base integer register for the effective address.

**FMOVECR** loads a constant from the FPU ROM (indices 0-15). Indices outside this range load 0.0 and set the Z condition code.

| Index | Constant | Index | Constant |
|-------|----------|-------|----------|
| 0     | Pi       | 8     | 1.0      |
| 1     | e        | 9     | 2.0      |
| 2     | log2(e)  | 10    | 10.0     |
| 3     | log10(e) | 11    | 100.0    |
| 4     | ln(2)    | 12    | 1000.0   |
| 5     | ln(10)   | 13    | 0.5      |
| 6     | log10(2) | 14    | FLT_MIN  |
| 7     | 0.0      | 15    | FLT_MAX  |

#### 4.6.2 FPU Arithmetic

| Mnemonic | Opcode | Syntax | Operation |
|----------|--------|--------|-----------|
| FADD     | `0x63` | `fadd fd, fs, ft` | `fd = fs + ft` |
| FSUB     | `0x64` | `fsub fd, fs, ft` | `fd = fs - ft` |
| FMUL     | `0x65` | `fmul fd, fs, ft` | `fd = fs * ft` |
| FDIV     | `0x66` | `fdiv fd, fs, ft` | `fd = fs / ft` |
| FMOD     | `0x67` | `fmod fd, fs, ft` | `fd = fs % ft` |
| FABS     | `0x68` | `fabs fd, fs` | `fd = \|fs\|` |
| FNEG     | `0x69` | `fneg fd, fs` | `fd = -fs` |
| FSQRT    | `0x6A` | `fsqrt fd, fs` | `fd = sqrt(fs)` |
| FINT     | `0x6B` | `fint fd, fs` | `fd = round(fs)` (uses FPCR mode) |

#### 4.6.3 FPU Transcendentals

| Mnemonic | Opcode | Syntax | Operation |
|----------|--------|--------|-----------|
| FSIN     | `0x71` | `fsin fd, fs` | `fd = sin(fs)` |
| FCOS     | `0x72` | `fcos fd, fs` | `fd = cos(fs)` |
| FTAN     | `0x73` | `ftan fd, fs` | `fd = tan(fs)` |
| FATAN    | `0x74` | `fatan fd, fs` | `fd = atan(fs)` |
| FLOG     | `0x75` | `flog fd, fs` | `fd = ln(fs)` |
| FEXP     | `0x76` | `fexp fd, fs` | `fd = e^fs` |
| FPOW     | `0x77` | `fpow fd, fs, ft` | `fd = fs^ft` |

#### 4.6.4 FPU Comparison and Conversion

| Mnemonic | Opcode | Syntax | Operation |
|----------|--------|--------|-----------|
| FCMP     | `0x6C` | `fcmp rd, fs, ft` | `rd = (fs < ft ? -1 : (fs > ft ? 1 : 0))` |
| FCVTIF   | `0x6D` | `fcvtif fd, rs` | `fd = float32(int32(rs))` |
| FCVTFI   | `0x6E` | `fcvtfi rd, fs` | `rd = int32(fs)` (saturating) |
| FMOVI    | `0x6F` | `fmovi fd, rs` | `fd = bits_to_float(uint32(rs))` |
| FMOVO    | `0x70` | `fmovo rd, fs` | `rd = uint64(float_to_bits(fs))` |

**FCMP** performs an explicit comparison. It handles infinity correctly: `+Inf` is equal to `+Inf` (sets Z bit, rd=0) and greater than all finite values. NaNs are unordered and set the NaN condition code and IO exception flag.

**FCVTFI** saturates to `INT32_MAX` or `INT32_MIN` on overflow. NaNs return 0 and set the IO exception flag.

#### 4.6.5 FPU Status and Control

| Mnemonic | Opcode | Syntax | Operation |
|----------|--------|--------|-----------|
| FMOVSR   | `0x79` | `fmovsr rd` | `rd = FPSR` |
| FMOVCR   | `0x7A` | `fmovcr rd` | `rd = FPCR` |
| FMOVSC   | `0x7B` | `fmovsc rs` | `FPSR = rs` (masked) |
| FMOVCC   | `0x7C` | `fmovcc rs` | `FPCR = rs` |

**FPSR (Status Register)**:
- Bits 27:24 - Condition Codes (N, Z, I, NaN). Overwritten per instruction.
- Bits 3:0 - Exception Flags (UE, OE, DZ, IO). Sticky (IEEE-754).
- **FMOVSC** masks the input value to preserve only these valid bits; bits 23:4 are reserved and always read as zero.

**FPCR (Control Register)**:
- Bits 1:0 - Rounding Mode:
  - 00: Nearest (default)
  - 01: Toward Zero (truncate)
  - 10: Toward -Inf (floor)
  - 11: Toward +Inf (ceil)

#### 4.6.6 Double-Precision (Register Pairs)

| Mnemonic | Opcode | Syntax | Operation |
|----------|--------|--------|-----------|
| DMOV     | `0x80` | `dmov fd, fs` | `fd = fs` |
| DLOAD    | `0x81` | `dload fd, disp(rs)` | `fd = mem64[rs + disp]` |
| DSTORE   | `0x82` | `dstore fs, disp(rs)` | `mem64[rs + disp] = fs` |
| DADD     | `0x83` | `dadd fd, fs, ft` | `fd = fs + ft` |
| DSUB     | `0x84` | `dsub fd, fs, ft` | `fd = fs - ft` |
| DMUL     | `0x85` | `dmul fd, fs, ft` | `fd = fs * ft` |
| DDIV     | `0x86` | `ddiv fd, fs, ft` | `fd = fs / ft` |
| DMOD     | `0x87` | `dmod fd, fs, ft` | `fd = fmod(fs, ft)` |
| DABS     | `0x88` | `dabs fd, fs` | `fd = \|fs\|` |
| DNEG     | `0x89` | `dneg fd, fs` | `fd = -fs` |
| DSQRT    | `0x8A` | `dsqrt fd, fs` | `fd = sqrt(fs)` |
| DINT     | `0x8B` | `dint fd, fs` | `fd = round(fs)` |
| DCMP     | `0x8C` | `dcmp rd, fs, ft` | `rd = -1/0/1` |
| DCVTIF   | `0x8D` | `dcvtif fd, rs` | `fd = float64(int64(rs))` |
| DCVTFI   | `0x8E` | `dcvtfi rd, fs` | `rd = int64(fs)` (saturating) |
| FCVTSD   | `0x8F` | `fcvtsd fd, fs` | `fd = float64(float32(fs))` |
| FCVTDS   | `0x90` | `fcvtds fd, fs` | `fd = float32(float64(fs))` |

Notes:
- `dload`/`dstore` always transfer 8 bytes.
- `dcvtfi` saturates to `INT64_MAX`/`INT64_MIN` on overflow and sets IO.
- `fcvtsd` requires an even destination. `fcvtds` requires an even source.
- Double-precision opcodes are unsized; size suffixes are not used.

---

### 4.7 Branches

All branches are PC-relative. The branch offset is stored as a signed 32-bit value in the imm32 field. The new PC is calculated as:

```
PC_new = PC_current + signExtend32to64(offset)
```

If the branch is not taken, PC advances by 8 (one instruction).

| Mnemonic | Opcode | Syntax | Condition | Comparison |
|----------|--------|--------|-----------|------------|
| BRA      | `0x40` | `bra label` | Always | -- |
| BEQ      | `0x41` | `beq Rs, Rt, label` | `Rs == Rt` | Unsigned (equality) |
| BNE      | `0x42` | `bne Rs, Rt, label` | `Rs != Rt` | Unsigned (equality) |
| BLT      | `0x43` | `blt Rs, Rt, label` | `int64(Rs) < int64(Rt)` | Signed |
| BGE      | `0x44` | `bge Rs, Rt, label` | `int64(Rs) >= int64(Rt)` | Signed |
| BGT      | `0x45` | `bgt Rs, Rt, label` | `int64(Rs) > int64(Rt)` | Signed |
| BLE      | `0x46` | `ble Rs, Rt, label` | `int64(Rs) <= int64(Rt)` | Signed |
| BHI      | `0x47` | `bhi Rs, Rt, label` | `Rs > Rt` | Unsigned |
| BLS      | `0x48` | `bls Rs, Rt, label` | `Rs <= Rt` | Unsigned |
| JMP      | `0x49` | `jmp (Rs)` / `jmp disp(Rs)` | `PC = Rs + signExtend(disp)` | Register-indirect |

**Encoding note for conditional branches**: Rs is in byte 2[7:3], Rt is in byte 3[7:3], and the branch offset is in bytes 4-7 (imm32). The Rd field (byte 1[7:3]) is unused (set to 0). The assembler computes the offset as `target_address - current_PC`.

**BRA** uses only the imm32 field. Rs and Rt fields are unused.

**JMP** (opcode `0x49`):
- Computes the effective address as `Rs + signExtend(imm32)`, masked to the 25-bit address space.
- Transfers control to the effective address.
- Does not modify the stack. No return address is saved.
- Rs is in byte 2[7:3], the optional displacement is in bytes 4-7 (imm32).
- Enables computed jumps, jump tables, and register-indirect branching.

---

### 4.7 Subroutine / Stack

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| JSR      | `0x50` | `jsr label` | `SP -= 8; mem[SP] = PC + 8; PC = PC + offset` | Write | Q |
| RTS      | `0x51` | `rts` | `PC = mem[SP]; SP += 8` | Read | Q |
| PUSH     | `0x52` | `push Rs` | `SP -= 8; mem[SP] = Rs` | Write | Q |
| POP      | `0x53` | `pop Rd` | `Rd = mem[SP]; SP += 8` | Read | Q |
| JSR      | `0x54` | `jsr (Rs)` / `jsr disp(Rs)` | `SP -= 8; mem[SP] = PC + 8; PC = Rs + signExtend(disp)` | Write | Q |

**JSR** (opcode `0x50`, PC-relative):
- Decrements SP (R31) by 8.
- Stores the return address (PC + 8, i.e., the instruction after the JSR) at the new SP.
- Branches to `PC + signExtend32to64(offset)`.
- Encoding: offset is `target_address - current_PC` in imm32.

**JSR** (opcode `0x54`, register-indirect):
- Decrements SP (R31) by 8.
- Stores the return address (PC + 8) at the new SP.
- Computes the effective address as `Rs + signExtend(imm32)`, masked to the 25-bit address space.
- Transfers control to the effective address.
- Rs is in byte 2[7:3], the optional displacement is in bytes 4-7 (imm32).
- The assembler disambiguates: `jsr label` emits opcode `0x50`, `jsr (Rs)` emits opcode `0x54`.
- Enables function pointers, vtable dispatch, and callback patterns.

**RTS** (opcode `0x51`):
- Reads the 64-bit return address from mem[SP].
- Sets PC to that address.
- Increments SP by 8.
- No operands.

**PUSH** (opcode `0x52`):
- Decrements SP by 8 (pre-decrement).
- Stores the full 64-bit value of Rs at the new SP.
- Encoding: the register is in the Rs field (byte 2[7:3]).

**POP** (opcode `0x53`):
- Reads the 64-bit value from mem[SP].
- Stores it in Rd.
- Increments SP by 8 (post-increment).
- Encoding: the register is in the Rd field (byte 1[7:3]).

All stack operations use 64-bit (8-byte) transfers regardless of size suffix. The stack pointer is always R31.

---

### 4.9 System

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| NOP      | `0xE0` | `nop` | No operation; PC += 8 | N | -- |
| HALT     | `0xE1` | `halt` | Stops execution | N | -- |
| SEI      | `0xE2` | `sei` | Enable interrupts (set TIMER_CTRL bit 1) | N | -- |
| CLI      | `0xE3` | `cli` | Disable interrupts (clear TIMER_CTRL bit 1) | N | -- |
| RTI      | `0xE4` | `rti` | Return from interrupt | Read | Q |
| WAIT     | `0xE5` | `wait #usec` | Sleep for `imm32` microseconds; PC += 8 | N | -- |

**HALT** (opcode `0xE1`):
- Sets the running flag to false, terminating the Execute() loop.
- The PC is not advanced.

**SEI** (opcode `0xE2`):
- Sets the interrupt-enable bit (bit 1) in TIMER_CTRL (CR11).
- The next timer expiration after SEI will trigger an interrupt. Expirations that occurred while interrupts were disabled are not queued or replayed.
- SEI is an alias for setting TIMER_CTRL bit 1; the same state can be written directly via MTCR.

**CLI** (opcode `0xE3`):
- Clears the interrupt-enable bit (bit 1) in TIMER_CTRL (CR11).
- The timer continues running and counting, but expirations are silently discarded (not queued for later delivery).
- CLI is an alias for clearing TIMER_CTRL bit 1; the same state can be written directly via MTCR.

**RTI** (opcode `0xE4`):
- Reads the 64-bit return address from mem[SP].
- Sets PC to that address.
- Increments SP by 8.
- Clears the `inInterrupt` flag, allowing further interrupts.
- Functionally identical to RTS except it also clears the in-interrupt state.

**WAIT** (opcode `0xE5`):
- Pauses execution for the specified number of microseconds.
- The immediate value is in the imm32 field (X=1 in encoding).
- If imm32 is 0, no delay occurs.

---

## 5. Pseudo-Instructions

Pseudo-instructions are expanded by the assembler into one or more real instructions before encoding. They do not have their own opcodes.

| Pseudo | Syntax | Expansion | Notes |
|--------|--------|-----------|-------|
| `la`   | `la Rd, addr` | `lea Rd, addr(r0)` | Load address into register |
| `li`   | `li Rd, #imm32` | `move.l Rd, #imm32` | Load 32-bit immediate (fits in 32 bits) |
| `li`   | `li Rd, #imm64` | `move.l Rd, #lo32` + `movt Rd, #hi32` | Load 64-bit immediate (2 instructions) |
| `beqz` | `beqz Rs, label` | `beq Rs, r0, label` | Branch if Rs == 0 |
| `bnez` | `bnez Rs, label` | `bne Rs, r0, label` | Branch if Rs != 0 |
| `bltz` | `bltz Rs, label` | `blt Rs, r0, label` | Branch if Rs < 0 (signed) |
| `bgez` | `bgez Rs, label` | `bge Rs, r0, label` | Branch if Rs >= 0 (signed) |
| `bgtz` | `bgtz Rs, label` | `bgt Rs, r0, label` | Branch if Rs > 0 (signed) |
| `blez` | `blez Rs, label` | `ble Rs, r0, label` | Branch if Rs <= 0 (signed) |

### 5.1 `la` -- Load Address

```
la r5, $A0000
; Expands to:
lea r5, $A0000(r0)     ; r5 = r0 + $A0000 = 0 + $A0000 = $A0000
```

Since R0 is hardwired to zero, `lea Rd, disp(r0)` effectively loads the displacement value as an absolute address.

> **Limitation**: Because `la` expands textually to `lea Rd, expr(r0)`, the address expression must not contain parentheses. For example, `la r1, BASE+(1*4)` will fail because `(1*4)` is misinterpreted as a register addressing mode. To work around this, precompute the address with separate arithmetic instructions or restructure the expression to avoid inner parentheses (e.g., use `BASE+4` instead of `BASE+(1*4)`).

### 5.2 `li` -- Load Immediate

For values that fit in 32 bits (0 to 0xFFFFFFFF):
```
li r3, #42
; Expands to:
move.l r3, #42
```

For values requiring 64 bits:
```
li r3, #$DEADBEEF_CAFEBABE
; Expands to:
move.l r3, #$CAFEBABE        ; load lower 32 bits
movt   r3, #$DEADBEEF        ; set upper 32 bits
```

### 5.3 Zero-Comparison Branches

All zero-comparison pseudo-branches exploit R0 = 0:
```
beqz r5, loop       ; branch if r5 == 0
; Expands to:
beq r5, r0, loop    ; compare r5 with r0 (which is 0)
```

---

## 6. Addressing Modes

The IE64 supports the following addressing modes:

| Mode | Syntax | Description | Used By |
|------|--------|-------------|---------|
| Immediate | `#imm` | 32-bit immediate value, zero-extended to 64 bits | MOVE, ALU ops, WAIT |
| Register | `Rs` or `Rt` | Register contents (64-bit) | MOVE, ALU ops, branches |
| Register-indirect (data) | `(Rs)` | Memory at address in Rs | LOAD, STORE |
| Register-indirect (control) | `(Rs)` | Transfer control to address in Rs | JMP, JSR |
| Displacement | `disp(Rs)` | Memory at `Rs + signExtend(disp)` | LOAD, STORE, LEA, JMP, JSR |
| PC-relative | `label` (assembler computes offset) | `PC + signExtend(offset)` | BRA, Bcc, JSR |

### 6.1 Immediate Addressing

The 32-bit immediate (imm32) is zero-extended to 64 bits when used as an operand (`operand3 = uint64(imm32)`). This means:

- Unsigned values 0 to 0xFFFFFFFF can be loaded directly.
- Negative values require MOVEQ (which sign-extends) or the `li` pseudo-instruction for 64-bit values.
- The X bit (byte 1, bit 0) must be 1 to select immediate mode.

### 6.2 Displacement Addressing

Used by LOAD, STORE, and LEA. The displacement is stored in imm32 and sign-extended to 64 bits before being added to the base register:

```
addr = uint32(int64(Rs) + int64(int32(imm32)))
```

This provides a +/- 2GB displacement range (though the effective address is truncated to 32 bits).

If displacement is zero, the assembler syntax `(Rs)` is used, and the X bit may be 0 or 1 (assembler sets X=1 only when displacement is non-zero).

---

## 7. Branch Architecture

### 7.1 Compare-and-Branch

The IE64 uses compare-and-branch instructions instead of a separate flags register. Each conditional branch instruction encodes:

- Two source registers (Rs, Rt) for comparison
- A signed 32-bit PC-relative offset

The comparison and branch are performed atomically in a single instruction. This eliminates the need for separate compare (CMP) and branch instructions, and avoids hazards associated with flag registers in pipelined implementations.

### 7.2 Register-Indirect Transfer

**JMP** (opcode `0x49`) provides register-indirect unconditional transfer. The target address is computed from a register plus an optional signed 32-bit displacement, then masked to the 25-bit address space. This enables computed jumps, jump tables, and dispatch through register-held addresses.

**JSR** (opcode `0x54`) provides the same register-indirect addressing for subroutine calls. It pushes the return address before transferring control, so a standard `rts` returns to the caller.

### 7.3 PC-Relative Offsets

All branch offsets are signed 32-bit values stored in the imm32 field. The effective target address is:

```
target = PC + signExtend32to64(offset)
```

Where `PC` is the address of the branch instruction itself (not PC+8).

The assembler computes: `offset = target_address - branch_address`.

### 7.3 R0-as-Zero Idioms

Since R0 is hardwired to zero, comparisons against zero are natural:

| Idiom | Instruction | Meaning |
|-------|-------------|---------|
| Branch if zero | `beq Rs, r0, label` | Rs == 0 |
| Branch if nonzero | `bne Rs, r0, label` | Rs != 0 |
| Branch if negative | `blt Rs, r0, label` | int64(Rs) < 0 |
| Branch if non-negative | `bge Rs, r0, label` | int64(Rs) >= 0 |
| Branch if positive | `bgt Rs, r0, label` | int64(Rs) > 0 |
| Branch if non-positive | `ble Rs, r0, label` | int64(Rs) <= 0 |
| Move zero to Rd | `move.q Rd, r0` | Rd = 0 |
| Test equal to value | `move.q Rt, #val` then `beq Rs, Rt, label` | Rs == val |

The `beqz`, `bnez`, `bltz`, `bgez`, `bgtz`, and `blez` pseudo-instructions automate these patterns.

### 7.4 Signed vs. Unsigned Comparisons

| Branch | Comparison Type | Condition |
|--------|----------------|-----------|
| BEQ    | Equality (unsigned/signed irrelevant) | Rs == Rt |
| BNE    | Equality (unsigned/signed irrelevant) | Rs != Rt |
| BLT    | Signed | int64(Rs) < int64(Rt) |
| BGE    | Signed | int64(Rs) >= int64(Rt) |
| BGT    | Signed | int64(Rs) > int64(Rt) |
| BLE    | Signed | int64(Rs) <= int64(Rt) |
| BHI    | Unsigned | uint64(Rs) > uint64(Rt) |
| BLS    | Unsigned | uint64(Rs) <= uint64(Rt) |

Note: For unsigned "greater than or equal" and "less than", use the complementary conditions:
- Unsigned `Rs >= Rt`: Use `bls Rt, Rs, label` (swap operands and use BLS) or check with BHI + BEQ.
- Unsigned `Rs < Rt`: Use `bhi Rt, Rs, label` (swap operands and use BHI).

---

## 8. Memory Map

### 8.1 Address Space Overview

The IE64 uses the same memory map as the Intuition Engine platform. PC and LOAD/STORE addresses are full 64-bit; IE64 reaches the full active visible RAM (which may exceed 4 GiB on hosts with sufficient memory) and reports it via `CR_RAM_SIZE_BYTES` and the `SYSINFO_ACTIVE_RAM_LO/HI` MMIO pair. Total guest RAM is reported through `SYSINFO_TOTAL_RAM_LO/HI`. The historical 25-bit/32 MB mask was retired in PLAN_MAX_RAM.md slice 3.

| Address Range | Size | Description |
|---------------|------|-------------|
| `$00000-$00FFF` | 4 KB | Interrupt vector table and system area |
| `$01000-$9EFFF` | 636 KB | Program code and data (PROG_START = `$1000`) |
| `$9F000-$9FFFF` | 4 KB | Stack area (STACK_START = `$9F000`, grows downward) |
| `$A0000-$AFFFF` | 64 KB | VGA VRAM window |
| `$B8000-$BFFFF` | 32 KB | VGA text buffer |
| `$F0000-$F0054` | 84 B | Video chip registers |
| `$F0700-$F07FF` | 256 B | Terminal/serial registers |
| `$F0800-$F0B3F` | 832 B | Audio chip registers |
| `$F0C00-$F0C20` | 33 B | PSG (AY-3-8910/YM2149) registers, playback helper, and PSG+ control |
| `$F0C30-$F0C3F` | 16 B | Native SN76489 latch/data, ready, and LFSR mode registers |
| `$F0D00-$F0D1D` | 29 B | POKEY registers |
| `$F0E00-$F0E2D` | 45 B | SID (6581/8580) registers |
| `$F0F00-$F0F5F` | 96 B | TED registers |
| `$F1000-$F13FF` | 1 KB | VGA registers |
| `$F2000-$F200B` | 12 B | ULA (ZX Spectrum) registers |
| `$F2100-$F213C` | 60 B | ANTIC registers |
| `$F2140-$F21B7` | 120 B | GTIA registers |
| `$F2200-$F221F` | 32 B | File I/O registers |
| `$F4000-$F43FF` | 1 KB | Voodoo 3D registers |
| `$100000-$5FFFFF` | 5 MB | Video RAM (VRAM_START = `$100000`) |

### 8.2 Initial State After Reset

| Register/State | Value |
|----------------|-------|
| PC | `$1000` (PROG_START) |
| R0 | `0` (hardwired) |
| R1-R30 | `0` |
| R31 (SP) | `$9F000` (STACK_START) |
| Interrupt enabled | `false` |
| In-interrupt flag | `false` |
| Timer enabled | `false` |
| Timer state | TIMER_STOPPED (0) |
| Timer count | 0 |
| Timer period | 0 |

### 8.3 Stack

The stack grows downward from `$9F000` toward lower addresses. All stack operations (PUSH, POP, JSR, RTS, RTI, interrupt entry) use 8-byte (64-bit) transfers.

```
High memory:
  $9F000  <-- Initial SP (STACK_START)
  $9EFF8  <-- SP after first PUSH
  $9EFF0  <-- SP after second PUSH
  ...
Low memory:
  $01000  <-- PROG_START (stack must not grow past program area)
```

---

## 9. Interrupt/Timer System

### 9.1 Interrupt Vector

The IE64 uses a single interrupt vector stored in the internal `interruptVector` field of the CPU. This field is initialized to `0` on reset. There is currently no assembly-level instruction or memory-mapped mechanism to change `interruptVector` from user code. (By contrast, the IE32 reads the vector from a memory-mapped vector table via `cpu.Read32(VECTOR_TABLE)`, but the IE64 does not implement this pattern.)

The vector table area at `$0000` in the memory map is reserved for future use. A future revision may add a memory-mapped vector table or a dedicated instruction to set the interrupt vector.

### 9.2 Timer Registers

The IE64 timer is integrated into the CPU and uses atomic fields (not memory-mapped I/O registers in the IE64 implementation). The timer state is managed through:

| Field | Type | Description |
|-------|------|-------------|
| timerCount | atomic.Uint64 | Current countdown value |
| timerPeriod | atomic.Uint64 | Auto-reload value |
| timerState | atomic.Uint32 | TIMER_STOPPED (0), TIMER_RUNNING (1), or TIMER_EXPIRED (2) |
| timerEnabled | atomic.Bool | Master enable for the timer subsystem |

### 9.3 Timer Operation

The timer is decremented once every `SAMPLE_RATE` (44100) instruction cycles:

1. A cycle counter increments every instruction.
2. When `cycleCounter >= 44100`, the counter resets to 0 and the timer count is decremented.
3. When the count reaches 0:
   - `timerState` is set to `TIMER_EXPIRED`.
   - If interrupts are enabled and the CPU is not already servicing an interrupt, the interrupt handler fires.
   - The count is reloaded from `timerPeriod` (if the timer is still enabled).
4. If count is 0 but period is non-zero, the count is reloaded from period and the state becomes `TIMER_RUNNING`.

### 9.4 Interrupt Flow

The following describes the internal CPU mechanics when an interrupt fires:

1. Check: `interruptEnabled == true` AND `inInterrupt == false`. If either fails, the interrupt is suppressed.
2. Set `inInterrupt = true` (prevents nesting).
3. Push the current PC onto the stack: `SP -= 8; mem[SP] = PC`.
4. Set `PC = interruptVector`.
5. The ISR executes. It must end with `RTI`.
6. `RTI` pops the return address: `PC = mem[SP]; SP += 8`.
7. `RTI` clears `inInterrupt`, re-enabling interrupt delivery.

> **Implementation note**: When the MMU is enabled and INTR_VEC (CR7) is nonzero, the unified timer interrupt model is used (see section 12.12). Timer interrupts save PC to FAULT_PC, set FAULT_CAUSE=8, perform automatic stack switching, and jump to INTR_VEC. The handler returns via ERET. When the MMU is off or INTR_VEC is zero, the legacy model is used: `handleInterrupt()` pushes PC and jumps to `interruptVector`, returning via RTI.

### 9.5 Interrupt Programming Patterns

#### 9.5.1 Unified Model (MMU enabled)

When the MMU is enabled, timer interrupts can be delivered through INTR_VEC (CR7) using the ERET-based trap model. The handler uses the same return path as syscalls and faults:

```
    ; Kernel initialization (supervisor mode)
    move.l r1, #kernel_stack_top
    mtcr 8, r1              ; KSP = kernel stack
    move.l r1, #timer_handler
    mtcr 7, r1              ; INTR_VEC = handler address
    move.l r1, #44100
    mtcr 9, r1              ; TIMER_PERIOD = 44100 cycles
    move.l r1, #3
    mtcr 11, r1             ; TIMER_CTRL = enable + interrupt enable
    ; ... set up page table, enable MMU, ERET to user mode ...

timer_handler:
    push r1
    push r2
    ; ... handle timer interrupt ...
    pop r2
    pop r1
    eret                    ; return to interrupted code (stack auto-restored)
```

#### 9.5.2 Legacy Model (MMU disabled)

Without the MMU, timer interrupts use the classic push-PC/RTI mechanism:

```
    org $1000
start:
    sei                     ; enable interrupts
    ; ... main program ...
    halt

isr_handler:
    push r1
    push r2
    ; ... handle interrupt ...
    pop r2
    pop r1
    rti                     ; return and re-enable interrupts
```

> **Note**: In the legacy model, `interruptVector` is an internal CPU field. Host/test code can set the vector directly on the CPU struct (e.g., `cpu.interruptVector = addr`), though this field is unexported and internal.

### 9.6 SEI/CLI Semantics

| Instruction | Effect |
|-------------|--------|
| SEI | Sets TIMER_CTRL (CR11) bit 1, enabling interrupt delivery. The timer continues running; the next expiration after SEI will trigger an interrupt. Note: expirations that occurred while interrupts were disabled are not queued or replayed. |
| CLI | Clears TIMER_CTRL (CR11) bit 1, disabling interrupt delivery. Timer continues running and counting, but interrupts are not delivered. |

Interrupts do not nest: if `inInterrupt` is true, no new interrupt is delivered regardless of the `interruptEnabled` flag.

---

## 10. 64-bit Constant Loading

Because the imm32 field is only 32 bits wide, loading a full 64-bit constant requires two instructions.

### 10.1 Pattern: MOVE.L + MOVT

```
; Load 0xDEADBEEF_CAFEBABE into R5
move.l r5, #$CAFEBABE      ; R5 = 0x00000000_CAFEBABE (lower 32 bits)
movt   r5, #$DEADBEEF      ; R5 = 0xDEADBEEF_CAFEBABE (upper 32 bits set)
```

Step by step:
1. `move.l r5, #$CAFEBABE` -- loads the 32-bit immediate `$CAFEBABE` into R5. Since this is a `.L` (32-bit) operation, the result is `0x00000000_CAFEBABE`.
2. `movt r5, #$DEADBEEF` -- takes the current value of R5, clears the upper 32 bits, and ORs in `$DEADBEEF << 32`. Result: `0xDEADBEEF_CAFEBABE`.

MOVT operation: `Rd = (Rd & 0x00000000FFFFFFFF) | (imm32 << 32)`

### 10.2 The `li` Pseudo-Instruction

The `li` pseudo-instruction automates this:

```
li r5, #$DEADBEEF_CAFEBABE
; Automatically expands to 2 instructions (16 bytes)
```

For values that fit in 32 bits, `li` emits only one instruction:

```
li r5, #42
; Expands to just: move.l r5, #42 (8 bytes)
```

### 10.3 MOVEQ Alternative

For signed 32-bit values that need sign-extension to 64 bits:

```
moveq r5, #-1      ; R5 = 0xFFFFFFFF_FFFFFFFF (sign-extended from 32-bit -1)
moveq r5, #$80000000  ; R5 = 0xFFFFFFFF_80000000 (sign-extended)
```

MOVEQ interprets imm32 as a signed 32-bit integer and sign-extends it: `Rd = int64(int32(imm32))`.

---

## 11. Assembly Language Quick Reference

The IE64 assembler uses 68K-flavored syntax. Mnemonics, register names, and directives are case-insensitive. Symbol names (labels, equates, sets) are case-sensitive.

### 11.1 Directives

| Directive | Syntax | Description |
|-----------|--------|-------------|
| `org`     | `org $addr` | Set the assembly origin address |
| `equ`     | `NAME equ value` | Define an immutable constant (case-sensitive name) |
| `set`     | `NAME set value` | Define a reassignable constant (case-sensitive name) |
| `dc.b`    | `dc.b val, val, ...` | Emit byte data. Supports `"string"` with escape sequences. |
| `dc.w`    | `dc.w val, val, ...` | Emit 16-bit little-endian words |
| `dc.l`    | `dc.l val, val, ...` | Emit 32-bit little-endian longwords |
| `dc.q`    | `dc.q val, val, ...` | Emit 64-bit little-endian quadwords |
| `ds.b`    | `ds.b n` | Reserve n zero bytes |
| `ds.w`    | `ds.w n` | Reserve n*2 zero bytes |
| `ds.l`    | `ds.l n` | Reserve n*4 zero bytes |
| `ds.q`    | `ds.q n` | Reserve n*8 zero bytes |
| `align`   | `align n` | Align to n-byte boundary (zero-fill padding) |
| `incbin`  | `incbin "file"` | Include binary file contents |
| `incbin`  | `incbin "file", offset, length` | Include binary file segment |
| `include` | `include "file"` | Include source file (with circular inclusion detection) |

### 11.2 Labels

**Global labels** are defined with a trailing colon and are visible throughout the source:

```
my_routine:
    nop
    rts
```

**Local labels** begin with a dot and are scoped to the preceding global label:

```
outer:
    nop
.loop:
    sub.q r1, r1, #1
    bnez r1, .loop      ; resolves to outer.loop
    rts

other:
.loop:                  ; this is other.loop, distinct from outer.loop
    nop
    rts
```

Local labels are stored internally as `global_label.local_label` (e.g., `outer.loop`).

### 11.3 Macros

Macros are defined with the `macro` / `endm` block syntax:

```
clear   macro
        move.q \1, r0       ; \1 = first parameter
        endm
```

Invoking the macro:

```
clear r5                     ; expands to: move.q r5, r0
```

**Parameter substitution**: Parameters are referenced as `\1` through `\9` within the macro body. Up to 9 parameters are supported.

**`narg` pseudo-symbol**: Within a macro body, `narg` (case-insensitive) is replaced with the number of arguments passed to the macro invocation.

```
multi   macro
        if narg == 1
            move.q \1, r0
        endif
        if narg == 2
            move.q \1, \2
        endif
        endm
```

**Recursive expansion**: Macro bodies can contain macro invocations. Expansion depth is limited to 100 levels to prevent infinite recursion.

> **Name collision warning**: Macro names must not match `equ` or `set` constant names (case-insensitive comparison). Because macro expansion occurs before directive processing, a line like `FOO equ 42` will be treated as an invocation of a macro named `foo` (with `equ` and `42` as arguments) rather than as a constant definition. This silently shadows the equate. Rename the macro or the constant to avoid the collision.

### 11.4 Conditional Assembly

```
DEBUG equ 1

    if DEBUG
        ; This code is assembled only if DEBUG != 0
        push r1
        jsr debug_print
        pop r1
    else
        ; This code is assembled when DEBUG == 0
        nop
    endif
```

- `if expr` -- evaluates the expression; assembles the following block if the result is non-zero.
- `else` -- optional; toggles assembly for the alternative block.
- `endif` -- terminates the conditional block.
- Conditional blocks can be nested.

### 11.5 Repeat Blocks

```
    rept 4
        nop
    endr
; Expands to 4 NOP instructions (32 bytes)
```

- `rept count` -- repeats the enclosed block `count` times.
- `endr` -- terminates the repeat block.
- Repeat blocks can be nested.
- The count is evaluated as an expression.

### 11.6 Expressions

Expressions are evaluated as 64-bit signed integers. The following operators are supported, listed from lowest to highest precedence:

| Precedence | Operators | Description |
|------------|-----------|-------------|
| 1 (lowest) | `==` `!=` `<` `>` `<=` `>=` | Comparison (returns 1 for true, 0 for false) |
| 2          | `\|` | Bitwise OR |
| 3          | `^` | Bitwise XOR |
| 4          | `&` | Bitwise AND |
| 5          | `<<` `>>` | Left shift, right shift |
| 6          | `+` `-` | Addition, subtraction |
| 7          | `*` `/` | Multiplication, division |
| 8 (highest)| `-` `+` `~` (unary) | Unary negate, unary plus, bitwise NOT |

Parentheses `()` can be used for grouping to override precedence.

**Number formats**:

| Format | Example | Description |
|--------|---------|-------------|
| Decimal | `42`, `1000` | Standard decimal |
| Hex (`$` prefix) | `$FF`, `$CAFE_BABE` | Motorola-style hex |
| Hex (`0x` prefix) | `0xFF`, `0xCAFE` | C-style hex |
| Character literal | `'A'`, `'\n'` | ASCII character value |

The underscore `_` can be used as a visual separator in numeric literals and is ignored during parsing.

**Symbol resolution**: Expressions can reference labels, `equ` constants, and `set` variables. On pass 1 (label collection), unresolved forward references evaluate to 0.

### 11.7 String Escapes

The following escape sequences are recognized in `dc.b` string literals and character literals:

| Escape | Character |
|--------|-----------|
| `\n`   | Newline (0x0A) |
| `\t`   | Tab (0x09) |
| `\r`   | Carriage return (0x0D) |
| `\\`   | Backslash (0x5C) |
| `\0`   | Null (0x00) |
| `\"`   | Double quote (0x22) |

### 11.8 Comments

Comments begin with `;` and extend to the end of the line. Semicolons inside quoted strings are not treated as comment delimiters.

```
move.q r1, #42      ; this is a comment
dc.b "hello; world" ; the semicolon in the string is literal
```

---

## 12. Memory Management Unit

The IE64 includes an optional single-level paged MMU that provides virtual-to-physical address translation, page-level access control, and a supervisor/user privilege model. When enabled, all instruction fetches, loads, and stores are translated through a software-managed page table. The MMU is disabled on reset; supervisor code must build a page table and explicitly enable translation.

### 12.1 Privilege Levels

The IE64 operates in one of two privilege levels:

| Level | MMU_CTRL.1 | Description |
|-------|------------|-------------|
| Supervisor | 1 | Full access. Can execute MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL. Can access all pages regardless of U bit. |
| User | 0 | Restricted. Privileged instructions cause a fault (cause code 5). Can only access pages with U=1. |

On reset the CPU is in supervisor mode. Transitioning to user mode is done via ERET (which clears supervisor mode and jumps to FAULT_PC). Returning to supervisor mode occurs only via a trap (fault or SYSCALL), which implicitly sets supervisor mode before jumping to the trap vector. The SMODE instruction reads the current mode into a register for introspection.

### 12.2 Control Registers

Sixteen control registers manage the MMU, thread state, timer, stack switching, trap state, and the live RAM-size discovery slot added by PLAN_MAX_RAM slice 10. They are accessed via MTCR (write) and MFCR (read).

| CR | Name | R/W | Description |
|----|------|-----|-------------|
| CR0 | PTBR | RW | Page Table Base Register. Physical address of the page table. |
| CR1 | FAULT_ADDR | RW | Virtual address that caused the most recent fault, or the syscall number (imm32) for SYSCALL. Writable so handlers can communicate information back. See 12.14 for the trap-stack semantics. |
| CR2 | FAULT_CAUSE | RW | Cause code of the most recent fault (see 12.7). Writable for handler flexibility. See 12.14 for the trap-stack semantics. |
| CR3 | FAULT_PC | RW | PC saved at trap entry. Used by ERET to resume. **Writable**: trap handlers must be able to modify this before ERET (e.g., to skip a faulting instruction or redirect execution). See 12.14 for the trap-stack semantics. |
| CR4 | TRAP_VEC | RW | Physical address of the trap handler entry point. Jumped to on any fault or SYSCALL. |
| CR5 | MMU_CTRL | Special | Bit 0: MMU enable (RW). Bit 1: supervisor mode (RO). Bit 2: SKEF enable (RW). Bit 3: SKAC enable (RW). Bit 4: SUA latch (RO via MTCR; mutated only by SUAEN/SUADIS). See 12.2.1. |
| CR6 | TP | RW | Thread Pointer. User-readable via MFCR (exception to the normal supervisor-only rule). Writable only in supervisor mode via MTCR. Intended for thread-local storage (TLS) base address. |
| CR7 | INTR_VEC | RW | Interrupt vector address. When MMU is enabled and INTR_VEC is nonzero, timer interrupts use the unified ERET-based entry model instead of the legacy push-PC/RTI model. Supervisor-only. |
| CR8 | KSP | RW | Kernel Stack Pointer. Automatically swapped with R31 on user-to-supervisor transitions. Supervisor-only. |
| CR9 | TIMER_PERIOD | RW | Timer reload period in instruction-cycle units. Supervisor-only. |
| CR10 | TIMER_COUNT | RW | Current timer countdown value. Supervisor-only. |
| CR11 | TIMER_CTRL | RW | Bit 0 = timer enable, Bit 1 = interrupt enable. SEI/CLI are aliases for setting/clearing bit 1. Supervisor-only. |
| CR12 | USP | RW | Saved User Stack Pointer. Readable/writable in supervisor mode for context switch. Set automatically on user-to-supervisor transition. Supervisor-only. |
| CR13 | PREV_MODE | RO | Previous privilege mode saved by `trapEntry`: 0 = trap came from user mode, 1 = trap came from supervisor mode. Read-only; set automatically on any trap/interrupt entry. Used by fault handlers to distinguish user faults (kill task) from kernel faults (panic). See 12.14 for the trap-stack semantics. |
| CR14 | SAVED_SUA | RW | SUA latch snapshot taken on trap entry and consumed on ERET. Readable by kernel handlers that observe the interrupted code path's SUA state; writable so handlers can stage a custom value before ERET. See 12.2.1 and 12.14. Supervisor-only. |
| CR15 | RAM_SIZE_BYTES | RO | **Read-only, live read** of the active CPU/profile-visible guest RAM in bytes (`bus.ActiveVisibleRAM()`). Every MFCR observes the current value, so a later `ApplyProfileVisibleCeiling` is immediately visible to subsequent reads — no boot-time snapshot is taken. MTCR to CR15 raises `FAULT_ILLEGAL_INSTRUCTION` (cause 11). Supervisor-only. PLAN_MAX_RAM slice 10. |

**PTBR** must point to the start of the page table in physical memory. The page table is 64 KiB (8192 entries x 8 bytes) and must be naturally aligned.

**TRAP_VEC** must be set before enabling the MMU, or faults will jump to address 0.

**MMU_CTRL** bit 0 is the master enable. Writing 1 activates translation for all subsequent memory accesses. Bit 1 (supervisor mode) is read-only; it reflects the current privilege level and cannot be written by MTCR. Bits 2–4 are the M15.6 SMEP/SMAP-equivalent controls described below.

#### 12.2.1 SKEF / SKAC / SUA (MMU_CTRL bits 2–4)

These bits are the IE64 SMEP/SMAP-equivalent guards introduced by M15.6.

| Bit | Name | MTCR writable? | Description |
|-----|------|-----------------|-------------|
| 2 | SKEF | Yes | Supervisor-Kernel-Execute-Fault. When set, a supervisor instruction fetch from a page with `PTE_U==1` raises `FAULT_SKEF` (cause 9). |
| 3 | SKAC | Yes | Supervisor-Kernel-Access-Check. When set, a supervisor read or write on a page with `PTE_U==1` raises `FAULT_SKAC` (cause 10), **unless** the `SUA` latch is also set. |
| 4 | SUA  | No (RO via MTCR) | Supervisor-User-Access latch. Mutated only by the privileged `SUAEN` and `SUADIS` opcodes. When set and `SKAC` is enabled, kernel data accesses to user pages succeed; when clear they fault with `FAULT_SKAC`. |

**Trap-entry / ERET discipline.** On trap entry the `SUA` latch is
snapshotted into `CR_SAVED_SUA` (CR14) and then forcibly cleared so a
kernel handler cannot inherit an open supervisor-user-access window
from the interrupted code path. On ERET the saved value is restored
into the live latch when returning to supervisor mode (nested return);
user-mode ERET clears the live latch unconditionally. See 12.14 for
how the trap stack preserves `CR_SAVED_SUA` across nested traps
automatically.

**Helper idiom.** Kernel user-memory access must bracket every
supervisor load/store against a user pointer with `SUAEN` and
`SUADIS`:

```
    suaen                       ; open the access window
    load.b  r3, (r1)            ; user load
    store.b r3, (r2)            ; kernel store
    suadis                      ; close the access window
```

The canonical `copy_from_user` / `copy_to_user` /
`copy_cstring_from_user` helpers enforce this pattern; see
`IE64_COOKBOOK.md` for the worked example.

### 12.3 Page Table Format

The MMU uses a single-level page table with 8192 entries, covering a 13-bit virtual page number (VPN) space. Each page is 4 KiB (12-bit offset). The page table occupies 64 KiB of contiguous physical memory.

```
Page Table (64 KiB at PTBR):
  Entry 0:      PTE for VPN 0x0000
  Entry 1:      PTE for VPN 0x0001
  ...
  Entry 8191:   PTE for VPN 0x1FFF
```

Each entry is 8 bytes (64 bits), addressed as `PTBR + VPN * 8`.

### 12.4 Page Table Entry (PTE) Format

```
Bit:  63                       26  25            13  12  7  6  5  4  3  2  1  0
     +---------------------------+----------------+-----+--+--+--+--+--+--+--+
     |        reserved (0)       |   PPN (13 bits)| rsvd| D| A| U| X| W| R| P|
     +---------------------------+----------------+-----+--+--+--+--+--+--+--+
```

| Bit(s) | Name | Description |
|--------|------|-------------|
| 0 | P (Present) | Page is mapped. If P=0, any access faults with cause 0. |
| 1 | R (Read) | Read permission. If R=0, loads fault with cause 1. |
| 2 | W (Write) | Write permission. If W=0, stores fault with cause 2. |
| 3 | X (Execute) | Execute permission. If X=0, instruction fetch faults with cause 3. |
| 4 | U (User) | User-accessible. If U=0, user-mode access faults with cause 4. |
| 5 | A (Accessed) | Set by hardware on any successful translation (read, write, or execute). |
| 6 | D (Dirty) | Set by hardware on write access. Only set when the access is a store; reads and fetches do not set D. |
| 12:7 | -- | Reserved, must be 0. |
| 25:13 | PPN | Physical Page Number (13 bits). Physical address = PPN << 12 | offset. |
| 63:26 | -- | Reserved, must be 0. |

**A/D bit semantics:**

- The A and D bits are set by the MMU hardware in the `translateAddr` path, after all permission checks have passed. Both the TLB-hit and TLB-miss paths perform A/D updates.
- A is set on every successful translation regardless of access type (read, write, execute).
- D is set only on write accesses.
- The bits are written back to the page table entry in memory only when they change (i.e., when the bit was previously 0). This avoids unnecessary memory writes on repeated accesses to the same page.
- **Architectural constraint**: Page tables must reside in normal RAM (below `IO_REGION_START`). The A/D write-back performs a direct memory store to the PTE; if the page table were in an I/O region, the write-back would corrupt device registers.
- Kernel software clears A/D bits by rewriting the PTE directly in memory and then flushing the TLB (via `TLBFLUSH` or `TLBINVAL`) to ensure the cached TLB entry is also updated. This is the basis for page reclamation and working-set estimation algorithms.

### 12.5 Virtual Address Format

```
Bit:  24                    12  11                 0
     +-----------------------+---------------------+
     |    VPN (13 bits)      |   Offset (12 bits)  |
     +-----------------------+---------------------+
```

PLAN_MAX_RAM.md slice 4 widened the MMU to 64-bit virtual + 64-bit physical addressing with a 6-level sparse radix page table:

- Top level: 7 bits, 128 entries.
- Levels 1..5: 9 bits each, 512 entries × 8 bytes per intermediate/leaf table.
- Total VPN width: 52 bits (top 7 + 5 × 9 = 52); page size remains 4 KiB.
- PTE format: bits 0..6 are flags (P/R/W/X/U/A/D), bits 7..11 reserved, bits 12..63 are PPN (`PTE_PPN_SHIFT = 12`, `PTE_PPN_BITS = 52`).
- Page count is no longer a fixed `MMU_NUM_PAGES = 8192`; it derives from active visible RAM (queried via `CR_RAM_SIZE_BYTES` or the SYSINFO MMIO pair).

Historical: prior to slice 4 the IE64 used a single-level 8192-entry flat page table over a 25-bit / 32 MB virtual address space. That format is retired; old IE64/IExec binaries are not supported across the migration.

### 12.6 MMU Instructions

Nine opcodes in the System range. All except SYSCALL and SMODE are privileged (supervisor-only); executing them in user mode faults with cause code 5 (privilege violation). MFCR has a special exception: reading CR6 (TP) is permitted in user mode.

| Mnemonic | Opcode | Syntax | Operation | Privilege |
|----------|--------|--------|-----------|-----------|
| MTCR | `0xE6` | `mtcr CRn, Rs` | `CR[Rd] = Rs` | Supervisor |
| MFCR | `0xE7` | `mfcr Rd, CRn` | `Rd = CR[Rs]` | Supervisor (CR6/TP: Any) |
| ERET | `0xE8` | `eret` | Consume and pop active trap frame; `PC = CR3 (FAULT_PC)` | Supervisor |
| TLBFLUSH | `0xE9` | `tlbflush` | Flush entire TLB + invalidate JIT cache | Supervisor |
| TLBINVAL | `0xEA` | `tlbinval Rs` | Invalidate TLB entry for VA in Rs + invalidate JIT cache | Supervisor |
| SYSCALL | `0xEB` | `syscall #imm32` | Trap to supervisor; syscall number from imm32 | Any |
| SMODE | `0xEC` | `smode Rd` | `Rd = 1` if supervisor, `Rd = 0` if user | Any |
| SUAEN | `0xF3` | `suaen` | Set the `SUA` latch (MMU_CTRL bit 4) | Supervisor |
| SUADIS | `0xF4` | `suadis` | Clear the `SUA` latch (MMU_CTRL bit 4) | Supervisor |

**MTCR** (opcode `0xE6`):
- Writes the value of general-purpose register Rs to control register CRn.
- The CR index is encoded in the Rd field (0-12).
- All CRs are writable except MMU_CTRL bit 1 (supervisor mode), which is silently ignored on write. This means trap handlers can modify FAULT_PC (CR3) before ERET to redirect execution.
- Writing to PTBR or MMU_CTRL bit 0 invalidates the JIT code cache and flushes the TLB.
- `CR_FAULT_PC` is not a user-service escape hatch: user-mode `MTCR CR_FAULT_PC` faults with cause 5 (`FAULT_PRIV`) just like any other privileged MTCR. Only supervisor-mode trap handlers may rewrite it.

**MFCR** (opcode `0xE7`):
- Reads control register CRn into general-purpose register Rd.
- The CR index is encoded in the Rs field (0-12).
- **User-mode exception**: MFCR is normally supervisor-only, but reading CR6 (TP) is permitted in user mode. This allows user-space threads to access thread-local storage without a syscall. All other CR indices fault with cause code 5 (privilege violation) in user mode.

**ERET** (opcode `0xE8`):
- Returns from a trap handler. Sets PC to the value saved in CR3 (FAULT_PC) and pops the active trap frame (see 12.14).
- If the previous mode (before the trap) was user: saves R31 to KSP (CR8), restores R31 from USP (CR12), switches to user mode, and clears the live `SUA` latch.
- If the previous mode was supervisor: stays in supervisor mode with no stack swap and restores the live `SUA` latch from the active frame's `CR_SAVED_SUA`.
- Does not pop a software stack. The trap entry does not push to the data stack — the push/pop here refers to the CPU's internal trap-frame stack (12.14).
- The handler can modify FAULT_PC via MTCR before ERET to redirect execution; this modifies the active frame, not an outer frame.
- For fault traps, `FAULT_PC = faulting PC`, so `ERET` re-executes the faulting instruction unless the handler rewrites `CR_FAULT_PC` first.
- For `SYSCALL`, `FAULT_PC = PC+8`, so `ERET` skips the trapping instruction by default.

**TLBFLUSH** (opcode `0xE9`):
- Invalidates all 64 entries of the software TLB.
- Must be executed after bulk page table modifications.

**TLBINVAL** (opcode `0xEA`):
- Invalidates the single TLB entry corresponding to the virtual address in Rs.
- Used after modifying a single page table entry.

**SYSCALL** (opcode `0xEB`):
- Initiates a synchronous trap to supervisor mode.
- The syscall number is the instruction's imm32 field (encoded with xbit=1). The handler reads it from CR1 (FAULT_ADDR) via MFCR.
- Saves PC+8 to CR3 (FAULT_PC), imm32 to CR1 (FAULT_ADDR), cause code 6 to CR2 (FAULT_CAUSE).
- Sets supervisor mode and jumps to CR4 (TRAP_VEC).
- Convention: syscall arguments are passed in R1-R6; return values in R1.
- Can be executed in both user and supervisor mode.

**SMODE** (opcode `0xEC`):
- Reads the current privilege mode into Rd: 1 = supervisor, 0 = user.
- This is a query instruction, not a mode-switching instruction. Mode is changed only by trap entry (sets supervisor) and ERET (clears supervisor).
- Can be executed in both user and supervisor mode.

**SUAEN** (opcode `0xF3`):
- Sets the SUA latch (MMU_CTRL bit 4).
- Single-cycle privileged operation with no operands.
- Must be paired with a subsequent `SUADIS` closing the same supervisor-user-access window.
- Attempting to execute in user mode faults with cause code 5 (privilege violation).

**SUADIS** (opcode `0xF4`):
- Clears the SUA latch (MMU_CTRL bit 4).
- Single-cycle privileged operation with no operands.
- Always safe to execute; clearing an already-clear latch is a no-op.
- Attempting to execute in user mode faults with cause code 5 (privilege violation).

#### Encoding

All seven opcodes use the standard 8-byte IE64 instruction format:

```
MTCR:     [0xE6] [CRn<<3 | 0 | 0] [Rs<<3] [0] [0 0 0 0]       ; Rd field = CR index (0-12)
MFCR:     [0xE7] [Rd<<3 | 0 | 0]  [CRn<<3] [0] [0 0 0 0]      ; Rs field = CR index (0-12)
ERET:     [0xE8] [0] [0] [0] [0 0 0 0]
TLBFLUSH: [0xE9] [0] [0] [0] [0 0 0 0]
TLBINVAL: [0xEA] [0] [Rs<<3] [0] [0 0 0 0]                     ; Rs = register holding VPN
SYSCALL:  [0xEB] [0 | 0 | 1] [0] [0] [imm32 LE]                ; xbit=1, syscall # in imm32
SMODE:    [0xEC] [Rd<<3 | 0 | 0] [0] [0] [0 0 0 0]             ; Rd = destination register
SUAEN:    [0xF3] [0] [0] [0] [0 0 0 0]
SUADIS:   [0xF4] [0] [0] [0] [0 0 0 0]
```

### 12.7 Trap Model

Traps are raised by faults (translation errors, permission violations) and by the SYSCALL instruction. On any trap:

1. CR1 (FAULT_ADDR) is set to the faulting virtual address (for faults) or the syscall number (for SYSCALL).
2. CR2 (FAULT_CAUSE) is set to the cause code.
3. CR3 (FAULT_PC) is set to the relevant PC (see below).
4. Automatic stack switching occurs: user R31 is saved to USP (CR12), R31 is loaded from KSP (CR8). See section 12.11.
5. The CPU switches to supervisor mode.
6. PC is set to CR4 (TRAP_VEC).

**Differentiated PC save:**
- **SYSCALL**: CR3 = PC + 8 (address of the instruction *after* SYSCALL). ERET resumes execution past the syscall.
- **Faults**: CR3 = faulting PC (address of the instruction that caused the fault). ERET re-executes the faulting instruction after the handler fixes the page table.

This distinction means trap handlers do not need to adjust the return address; ERET always restores CR3 directly.

### 12.8 Fault Cause Codes

| Code | Name | Trigger |
|------|------|---------|
| 0 | Page Not Present | Access to a page with P=0. |
| 1 | Read Denied | Load from a page with R=0. |
| 2 | Write Denied | Store to a page with W=0. |
| 3 | Execute Denied | Instruction fetch from a page with X=0. |
| 4 | User/Supervisor | User-mode access to a page with U=0. |
| 5 | Privilege Violation | User-mode execution of a privileged instruction (MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SUAEN, SUADIS). |
| 6 | Syscall | SYSCALL instruction executed. |
| 7 | Misaligned | Atomic memory operation (CAS, XCHG, FAA, FAND, FOR, FXOR) with address not 8-byte aligned. |
| 8 | Timer Interrupt | Timer interrupt (delivered via INTR_VEC when MMU enabled). |
| 9 | SKEF | Supervisor instruction fetch from a page with `PTE_U==1` while `MMU_CTRL.SKEF` is set. |
| 10 | SKAC | Supervisor data read or write on a page with `PTE_U==1` while `MMU_CTRL.SKAC` is set and `MMU_CTRL.SUA` is clear. |
| 11 | Illegal Instruction | Opcode-level invariants the CPU cannot otherwise enforce. Currently raised only by `MTCR Rs, CR_RAM_SIZE_BYTES` (the CR is read-only — see 12.2). |

### 12.9 Translation Lookaside Buffer (TLB)

The MMU maintains a 64-entry direct-mapped software TLB to cache page table lookups.

- **Indexing**: `TLB[VPN & 63]`. Each entry stores the VPN tag and the full PTE.
- **Lookup**: On every translated access, the TLB is checked first. On a hit (VPN tag matches), the cached PTE is used. On a miss, the page table is walked, the PTE is loaded, permission checks are performed, and the result is cached in the TLB.
- **Invalidation**: TLBFLUSH clears all 64 entries. TLBINVAL clears only the entry matching the given VA's VPN. Writing PTBR or MMU_CTRL via MTCR also flushes the entire TLB.
- **Coherency**: The TLB is not automatically coherent with page table memory. After modifying a PTE in memory, software must execute TLBINVAL for the affected page (or TLBFLUSH for bulk changes) before the new mapping takes effect.

### 12.10 W^X Security Model

The IE64 MMU enforces a Write XOR Execute policy at the page level. A page may be writable or executable, but not both simultaneously:

- **Code pages**: P=1, X=1, W=0, with R optional. M15.6 R4 makes execute-only user text (`P=1, R=0, X=1`) a first-class contract.
- **Data/stack pages**: P=1, R=1, W=1, X=0 (readable + writable, not executable).

This prevents code injection attacks: an attacker who can write to memory cannot execute that memory, and executable memory cannot be modified. To load new code, supervisor software must map the target pages as writable, write the code, then remap as executable (with appropriate TLB invalidation between steps).

### 12.11 Automatic Stack Switching

The IE64 provides automatic kernel/user stack separation via KSP (CR8) and USP (CR12). On any user-to-supervisor transition (SYSCALL, fault, or timer interrupt):

1. The previous privilege mode is saved internally.
2. The current user R31 (stack pointer) is saved to USP (CR12).
3. R31 is loaded from KSP (CR8), giving the handler a kernel stack.
4. The CPU switches to supervisor mode.

On ERET:

1. If the previous mode was user: R31 is saved to KSP (CR8), R31 is restored from USP (CR12), and the CPU returns to user mode.
2. If the previous mode was supervisor: the CPU stays in supervisor mode with no stack swap.

This allows trap handlers to execute on a dedicated kernel stack without any software stack-switching prologue. The kernel must initialize KSP (via MTCR) before entering user mode for the first time.

### 12.12 Unified Timer Interrupt Model

The IE64 supports two timer interrupt models, selected automatically based on the MMU state and INTR_VEC (CR7):

**Unified model** (MMU enabled, INTR_VEC nonzero):

Timer interrupts are delivered through the same trap mechanism as faults and SYSCALLs:

1. PC is saved to CR3 (FAULT_PC).
2. CR2 (FAULT_CAUSE) is set to 8 (FAULT_TIMER).
3. Automatic stack switching occurs (user R31 saved to USP, R31 loaded from KSP).
4. The CPU switches to supervisor mode.
5. PC is set to INTR_VEC (CR7).

The handler returns via ERET, which restores the stack and privilege mode automatically. This model eliminates the need for RTI and provides a consistent trap-return path for all supervisor entry points.

**Legacy model** (MMU disabled, or INTR_VEC is zero):

Timer interrupts use the classic push-PC/RTI mechanism:

1. The current PC is pushed onto the stack: `SP -= 8; mem[SP] = PC`.
2. The `inInterrupt` flag is set (prevents nesting).
3. PC is set to `interruptVector`.
4. The handler returns via RTI, which pops PC and clears `inInterrupt`.

This model is retained for backwards compatibility with programs that do not use the MMU.

### 12.13 Atomic Memory Operations

The IE64 provides six atomic read-modify-write (RMW) instructions for lock-free synchronisation. These instructions use a dedicated encoding form that repurposes all three register fields and the immediate field:

#### Encoding: Memory RMW Form

```
Byte:    0         1              2              3            4    5    6    7
       +--------+----------+----------+----------+------+------+------+------+
       | Opcode | Rd<<3|0|0| Rs<<3    | Rt<<3    |       imm32 (LE)         |
       +--------+----------+----------+----------+------+------+------+------+
```

- **Rd**: Destination register (receives the old value read from memory)
- **Rs**: Base register (memory address source)
- **Rt**: Operand register (value to swap, add, or use in bitwise operation)
- **imm32**: Signed displacement added to Rs to form the effective address

Effective address: `addr = uint32(int64(Rs) + int64(int32(imm32)))`

#### Instruction Table

| Mnemonic | Opcode | Syntax | Operation |
|----------|--------|--------|-----------|
| CAS      | `0xED` | `cas Rd, disp(Rs), Rt` | `old = [addr]; if old == Rd then [addr] = Rt; Rd = old` |
| XCHG     | `0xEE` | `xchg Rd, disp(Rs), Rt` | `old = [addr]; [addr] = Rt; Rd = old` |
| FAA      | `0xEF` | `faa Rd, disp(Rs), Rt` | `old = [addr]; [addr] = old + Rt; Rd = old` |
| FAND     | `0xF0` | `fand Rd, disp(Rs), Rt` | `old = [addr]; [addr] = old & Rt; Rd = old` |
| FOR      | `0xF1` | `for Rd, disp(Rs), Rt` | `old = [addr]; [addr] = old \| Rt; Rd = old` |
| FXOR     | `0xF2` | `fxor Rd, disp(Rs), Rt` | `old = [addr]; [addr] = old ^ Rt; Rd = old` |

When the displacement is zero, the assembler accepts `(Rs)` syntax: `cas Rd, (Rs), Rt`.

#### Semantics

- **Size**: Always 64-bit (`.Q`). No size suffix is accepted; atomic operations operate on naturally-aligned 64-bit words only.
- **Alignment**: The effective address must be 8-byte aligned (`addr & 7 == 0`). A misaligned address causes a trap with FAULT_MISALIGNED (cause code 7).
- **I/O rejection**: Atomic operations are rejected if the effective address falls within the I/O region (`addr >= IO_REGION_START`). Attempting an atomic operation on an I/O address causes a trap.
- **Ordering**: All atomic operations are sequentially consistent. They act as full memory barriers.
- **MMU**: When the MMU is enabled, the effective address is translated as an `ACCESS_WRITE` operation through the normal page table translation path. A/D bits are set accordingly.
- **CAS (Compare-And-Swap)**: Reads the 64-bit value at `[addr]` into a temporary. If the temporary equals the current value of Rd, the value of Rt is written to `[addr]`. Regardless of whether the swap occurred, Rd receives the old value from memory. This allows the caller to detect success by comparing the returned old value against the expected value.
- **JIT**: Atomic instructions always bail to the interpreter, even when the MMU is disabled. They are infrequent synchronisation operations where correctness outweighs compilation overhead.

### 12.14 Trap-Frame Stack

M15.6 makes nested-trap state preservation architectural rather than
kernel-managed. The CPU owns a fixed-depth trap-frame stack that
holds the outer trap's `FAULT_PC`, `PREV_MODE`, `CR_SAVED_SUA`,
`FAULT_ADDR`, and `FAULT_CAUSE`. The live CR fields remain the
canonical "top of stack" accessed through `MFCR` / `MTCR`; the
stack is not directly visible to software.

```
              top of stack (active frame)
              ┌────────────────────────┐
              │ FAULT_PC    (CR3)      │
              │ FAULT_ADDR  (CR1)      │
              │ FAULT_CAUSE (CR2)      │
              │ PREV_MODE   (CR13)     │
              │ SAVED_SUA   (CR14)     │
              └────────────────────────┘
              ↑ readable/writable via MFCR / MTCR
              │
              │ outer frames (invisible to software)
              ▼
              ┌────────────────────────┐
              │   frame depth-1        │
              │   ...                  │
              │   frame 0              │ ← frame of first trap
              └────────────────────────┘
```

**Push.** On trap entry (fault, SYSCALL, timer interrupt) the CPU
snapshots the active frame (all five fields) onto the stack before
overwriting them. The snapshot happens first; subsequent trap-entry
bookkeeping (setting `PREV_MODE`, saving and clearing the `SUA`
latch into `SAVED_SUA`) then writes the new active-frame values.

**Pop.** On `ERET` the CPU consumes the active frame (uses
`FAULT_PC` as the new PC, restores the `SUA` latch from
`SAVED_SUA` or clears it on user return, and swaps stacks on user
return) and then pops the previous frame off the stack into the
active fields. When the stack is empty the active fields are
cleared to zero, matching the fresh-boot state.

**Overflow.** The stack depth is fixed. Exceeding it halts the CPU
with a diagnostic (`IE64: trap stack overflow ...`): runaway
nested faults are always a kernel bug and must be visible, not
silently dropped.

**Implications for kernel handlers.** Handlers do **not** need to
save and restore `CR_FAULT_PC` or `CR_SAVED_SUA` on the kernel
stack to survive a nested synchronous trap. The trap stack
preserves the outer context automatically. Existing kernel code
that performs a manual `MFCR CR_FAULT_PC` / `MTCR CR_FAULT_PC`
prologue around a possibly-faulting region still works — such
save/restore now writes the active frame, so the restore writes
back the same value already preserved. The older handler pattern
is thus redundant but harmless; new handlers should omit it.

**Reset.** `Reset()` clears the trap stack to depth 0 and zeroes
the frame slots so a reused CPU does not inherit a half-built
frame from a previous run.

---

## Appendix A: Opcode Map

### A.1 Opcode Summary Table

| Opcode | Hex    | Mnemonic | Category | Operands |
|--------|--------|----------|----------|----------|
| 0x01   | `$01`  | MOVE     | Data Movement | Rd, Rs / Rd, #imm |
| 0x02   | `$02`  | MOVT     | Data Movement | Rd, #imm |
| 0x03   | `$03`  | MOVEQ    | Data Movement | Rd, #imm |
| 0x04   | `$04`  | LEA      | Data Movement | Rd, disp(Rs) |
| 0x10   | `$10`  | LOAD     | Memory Access | Rd, disp(Rs) |
| 0x11   | `$11`  | STORE    | Memory Access | Rd, disp(Rs) |
| 0x20   | `$20`  | ADD      | Arithmetic | Rd, Rs, Rt/#imm |
| 0x21   | `$21`  | SUB      | Arithmetic | Rd, Rs, Rt/#imm |
| 0x22   | `$22`  | MULU     | Arithmetic | Rd, Rs, Rt/#imm |
| 0x23   | `$23`  | MULS     | Arithmetic | Rd, Rs, Rt/#imm |
| 0x24   | `$24`  | DIVU     | Arithmetic | Rd, Rs, Rt/#imm |
| 0x25   | `$25`  | DIVS     | Arithmetic | Rd, Rs, Rt/#imm |
| 0x26   | `$26`  | MOD      | Arithmetic | Rd, Rs, Rt/#imm |
| 0x27   | `$27`  | NEG      | Arithmetic | Rd, Rs |
| 0x28   | `$28`  | MODS     | Arithmetic | Rd, Rs, Rt/#imm |
| 0x29   | `$29`  | MULHU    | Arithmetic | Rd, Rs, Rt |
| 0x2A   | `$2A`  | MULHS    | Arithmetic | Rd, Rs, Rt |
| 0x30   | `$30`  | AND      | Logical | Rd, Rs, Rt/#imm |
| 0x31   | `$31`  | OR       | Logical | Rd, Rs, Rt/#imm |
| 0x32   | `$32`  | EOR      | Logical | Rd, Rs, Rt/#imm |
| 0x33   | `$33`  | NOT      | Logical | Rd, Rs |
| 0x34   | `$34`  | LSL      | Shift | Rd, Rs, Rt/#imm |
| 0x35   | `$35`  | LSR      | Shift | Rd, Rs, Rt/#imm |
| 0x36   | `$36`  | ASR      | Shift | Rd, Rs, Rt/#imm |
| 0x37   | `$37`  | CLZ      | Shift | Rd, Rs |
| 0x38   | `$38`  | SEXT     | Shift | Rd, Rs |
| 0x39   | `$39`  | ROL      | Shift | Rd, Rs, Rt/#imm |
| 0x3A   | `$3A`  | ROR      | Shift | Rd, Rs, Rt/#imm |
| 0x3B   | `$3B`  | CTZ      | Shift | Rd, Rs |
| 0x3C   | `$3C`  | POPCNT   | Shift | Rd, Rs |
| 0x3D   | `$3D`  | BSWAP    | Shift | Rd, Rs |
| 0x40   | `$40`  | BRA      | Branch | label |
| 0x41   | `$41`  | BEQ      | Branch | Rs, Rt, label |
| 0x42   | `$42`  | BNE      | Branch | Rs, Rt, label |
| 0x43   | `$43`  | BLT      | Branch | Rs, Rt, label |
| 0x44   | `$44`  | BGE      | Branch | Rs, Rt, label |
| 0x45   | `$45`  | BGT      | Branch | Rs, Rt, label |
| 0x46   | `$46`  | BLE      | Branch | Rs, Rt, label |
| 0x47   | `$47`  | BHI      | Branch | Rs, Rt, label |
| 0x48   | `$48`  | BLS      | Branch | Rs, Rt, label |
| 0x49   | `$49`  | JMP      | Branch | (Rs) / disp(Rs) |
| 0x50   | `$50`  | JSR      | Subroutine | label |
| 0x51   | `$51`  | RTS      | Subroutine | (none) |
| 0x52   | `$52`  | PUSH     | Stack | Rs |
| 0x53   | `$53`  | POP      | Stack | Rd |
| 0x54   | `$54`  | JSR      | Subroutine | (Rs) / disp(Rs) |
| 0x60   | `$60`  | FMOV     | FPU | fd, fs |
| 0x61   | `$61`  | FLOAD    | FPU | fd, disp(rs) |
| 0x62   | `$62`  | FSTORE   | FPU | fs, disp(rd) |
| 0x63   | `$63`  | FADD     | FPU | fd, fs, ft |
| 0x64   | `$64`  | FSUB     | FPU | fd, fs, ft |
| 0x65   | `$65`  | FMUL     | FPU | fd, fs, ft |
| 0x66   | `$66`  | FDIV     | FPU | fd, fs, ft |
| 0x67   | `$67`  | FMOD     | FPU | fd, fs, ft |
| 0x68   | `$68`  | FABS     | FPU | fd, fs |
| 0x69   | `$69`  | FNEG     | FPU | fd, fs |
| 0x6A   | `$6A`  | FSQRT    | FPU | fd, fs |
| 0x6B   | `$6B`  | FINT     | FPU | fd, fs |
| 0x6C   | `$6C`  | FCMP     | FPU | rd, fs, ft |
| 0x6D   | `$6D`  | FCVTIF   | FPU | fd, rs |
| 0x6E   | `$6E`  | FCVTFI   | FPU | rd, fs |
| 0x6F   | `$6F`  | FMOVI    | FPU | fd, rs |
| 0x70   | `$70`  | FMOVO    | FPU | rd, fs |
| 0x71   | `$71`  | FSIN     | FPU | fd, fs |
| 0x72   | `$72`  | FCOS     | FPU | fd, fs |
| 0x73   | `$73`  | FTAN     | FPU | fd, fs |
| 0x74   | `$74`  | FATAN    | FPU | fd, fs |
| 0x75   | `$75`  | FLOG     | FPU | fd, fs |
| 0x76   | `$76`  | FEXP     | FPU | fd, fs |
| 0x77   | `$77`  | FPOW     | FPU | fd, fs, ft |
| 0x78   | `$78`  | FMOVECR  | FPU | fd, #idx |
| 0x79   | `$79`  | FMOVSR   | FPU | rd |
| 0x7A   | `$7A`  | FMOVCR   | FPU | rd |
| 0x7B   | `$7B`  | FMOVSC   | FPU | rs |
| 0x7C   | `$7C`  | FMOVCC   | FPU | rs |
| 0x80   | `$80`  | DMOV     | FPU64 | fd, fs |
| 0x81   | `$81`  | DLOAD    | FPU64 | fd, disp(rs) |
| 0x82   | `$82`  | DSTORE   | FPU64 | fd, disp(rs) |
| 0x83   | `$83`  | DADD     | FPU64 | fd, fs, ft |
| 0x84   | `$84`  | DSUB     | FPU64 | fd, fs, ft |
| 0x85   | `$85`  | DMUL     | FPU64 | fd, fs, ft |
| 0x86   | `$86`  | DDIV     | FPU64 | fd, fs, ft |
| 0x87   | `$87`  | DMOD     | FPU64 | fd, fs, ft |
| 0x88   | `$88`  | DABS     | FPU64 | fd, fs |
| 0x89   | `$89`  | DNEG     | FPU64 | fd, fs |
| 0x8A   | `$8A`  | DSQRT    | FPU64 | fd, fs |
| 0x8B   | `$8B`  | DINT     | FPU64 | fd, fs |
| 0x8C   | `$8C`  | DCMP     | FPU64 | rd, fs, ft |
| 0x8D   | `$8D`  | DCVTIF   | FPU64 | fd, rs |
| 0x8E   | `$8E`  | DCVTFI   | FPU64 | rd, fs |
| 0x8F   | `$8F`  | FCVTSD   | FPU64 | fd, fs |
| 0x90   | `$90`  | FCVTDS   | FPU64 | fd, fs |
| 0xE0   | `$E0`  | NOP      | System | (none) |
| 0xE1   | `$E1`  | HALT     | System | (none) |
| 0xE2   | `$E2`  | SEI      | System | (none) |
| 0xE3   | `$E3`  | CLI      | System | (none) |
| 0xE4   | `$E4`  | RTI      | System | (none) |
| 0xE5   | `$E5`  | WAIT     | System | #usec |
| 0xE6   | `$E6`  | MTCR     | MMU | CRn, Rs |
| 0xE7   | `$E7`  | MFCR     | MMU | Rd, CRn |
| 0xE8   | `$E8`  | ERET     | MMU | (none) |
| 0xE9   | `$E9`  | TLBFLUSH | MMU | (none) |
| 0xEA   | `$EA`  | TLBINVAL | MMU | Rs |
| 0xEB   | `$EB`  | SYSCALL  | MMU | #imm32 |
| 0xEC   | `$EC`  | SMODE    | MMU | Rd |
| 0xED   | `$ED`  | CAS      | Atomic | Rd, disp(Rs), Rt |
| 0xEE   | `$EE`  | XCHG     | Atomic | Rd, disp(Rs), Rt |
| 0xEF   | `$EF`  | FAA      | Atomic | Rd, disp(Rs), Rt |
| 0xF0   | `$F0`  | FAND     | Atomic | Rd, disp(Rs), Rt |
| 0xF1   | `$F1`  | FOR      | Atomic | Rd, disp(Rs), Rt |
| 0xF2   | `$F2`  | FXOR     | Atomic | Rd, disp(Rs), Rt |
| 0xF3   | `$F3`  | SUAEN    | System | (none) |
| 0xF4   | `$F4`  | SUADIS   | System | (none) |

### A.2 Opcode Ranges

| Range | Category |
|-------|----------|
| `$01-$04` | Data Movement |
| `$10-$11` | Memory Access |
| `$20-$27` | Arithmetic |
| `$30-$37` | Logical / Shift |
| `$40-$49` | Branches |
| `$50-$54` | Subroutine / Stack |
| `$60-$7C` | Floating Point (FPU) |
| `$E0-$E5` | System |
| `$E6-$EC` | MMU |
| `$ED-$F2` | Atomic Memory Operations |

Any opcode not listed above causes the CPU to print an error message and halt execution.

---

## Appendix B: Encoding Examples

### B.1 `move.l r5, #$CAFEBABE`

```
Opcode = 0x01 (MOVE)
Rd = 5, Size = 2 (.L), X = 1 (immediate)
Rs = 0, Rt = 0
imm32 = 0xCAFEBABE

Byte 0: 0x01
Byte 1: (5 << 3) | (2 << 1) | 1 = 0x28 | 0x04 | 0x01 = 0x2D
Byte 2: 0 << 3 = 0x00
Byte 3: 0 << 3 = 0x00
Bytes 4-7: 0xBE 0xBA 0xFE 0xCA (little-endian)

Binary: 01 2D 00 00 BE BA FE CA
```

### B.2 `add.q r3, r1, r2`

```
Opcode = 0x20 (ADD)
Rd = 3, Size = 3 (.Q), X = 0 (register)
Rs = 1, Rt = 2
imm32 = 0

Byte 0: 0x20
Byte 1: (3 << 3) | (3 << 1) | 0 = 0x18 | 0x06 | 0x00 = 0x1E
Byte 2: 1 << 3 = 0x08
Byte 3: 2 << 3 = 0x10
Bytes 4-7: 0x00 0x00 0x00 0x00

Binary: 20 1E 08 10 00 00 00 00
```

### B.3 `beq r1, r2, target` (target 24 bytes ahead)

```
Opcode = 0x41 (BEQ)
Rd = 0 (unused), Size = 3 (.Q), X = 0
Rs = 1, Rt = 2
imm32 = 24 (signed offset = +24)

Byte 0: 0x41
Byte 1: (0 << 3) | (3 << 1) | 0 = 0x06
Byte 2: 1 << 3 = 0x08
Byte 3: 2 << 3 = 0x10
Bytes 4-7: 0x18 0x00 0x00 0x00

Binary: 41 06 08 10 18 00 00 00
```

### B.4 `store.b r7, 4(r10)`

```
Opcode = 0x11 (STORE)
Rd = 7, Size = 0 (.B), X = 1 (displacement non-zero)
Rs = 10, Rt = 0
imm32 = 4

Byte 0: 0x11
Byte 1: (7 << 3) | (0 << 1) | 1 = 0x38 | 0x00 | 0x01 = 0x39
Byte 2: 10 << 3 = 0x50
Byte 3: 0x00
Bytes 4-7: 0x04 0x00 0x00 0x00

Binary: 11 39 50 00 04 00 00 00
```

### B.5 `push r15`

```
Opcode = 0x52 (PUSH)
Rd = 0 (unused), Size = 3 (.Q), X = 0
Rs = 15, Rt = 0
imm32 = 0

Byte 0: 0x52
Byte 1: (0 << 3) | (3 << 1) | 0 = 0x06
Byte 2: 15 << 3 = 0x78
Byte 3: 0x00
Bytes 4-7: 0x00 0x00 0x00 0x00

Binary: 52 06 78 00 00 00 00 00
```

### B.6 `jmp (r5)`

```
Opcode = 0x49 (JMP)
Rd = 0 (unused), Size = 0, X = 0
Rs = 5, Rt = 0
imm32 = 0 (no displacement)

Byte 0: 0x49
Byte 1: 0x00
Byte 2: 5 << 3 = 0x28
Byte 3: 0x00
Bytes 4-7: 0x00 0x00 0x00 0x00

Binary: 49 00 28 00 00 00 00 00
```

### B.7 `jsr 16(r3)`

```
Opcode = 0x54 (JSR indirect)
Rd = 0 (unused), Size = 0, X = 0
Rs = 3, Rt = 0
imm32 = 16 (displacement)

Byte 0: 0x54
Byte 1: 0x00
Byte 2: 3 << 3 = 0x18
Byte 3: 0x00
Bytes 4-7: 0x10 0x00 0x00 0x00

Binary: 54 00 18 00 10 00 00 00
```
