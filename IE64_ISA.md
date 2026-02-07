# IE64 Instruction Set Architecture Reference

Intuition Engine 64-bit RISC CPU -- Complete ISA Specification

(c) 2024-2026 Zayn Otley -- GPLv3 or later

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Register File](#2-register-file)
3. [Instruction Encoding](#3-instruction-encoding)
4. [Complete Instruction Reference](#4-complete-instruction-reference)
5. [Pseudo-Instructions](#5-pseudo-instructions)
6. [Addressing Modes](#6-addressing-modes)
7. [Branch Architecture](#7-branch-architecture)
8. [Memory Map](#8-memory-map)
9. [Interrupt/Timer System](#9-interrupttimer-system)
10. [64-bit Constant Loading](#10-64-bit-constant-loading)
11. [Assembly Language Quick Reference](#11-assembly-language-quick-reference)

---

## 1. Architecture Overview

The IE64 is a 64-bit RISC load-store CPU designed for the Intuition Engine platform. It uses a clean, regular instruction encoding with the following core characteristics:

- **Word size**: 64-bit registers, 64-bit data path
- **Instruction width**: Fixed 8 bytes (64 bits) per instruction
- **Byte order**: Little-endian throughout (instruction encoding, memory access, immediates)
- **Architecture class**: Load-store (all computation on registers; memory accessed only via LOAD/STORE)
- **Condition model**: Compare-and-branch (no flags register)
- **Register file**: 32 general-purpose 64-bit registers (R0 hardwired to zero)
- **Address space**: 32MB physical, masked via `PC & 0x1FFFFFF`
- **Stack**: Full-descending, 8-byte aligned, R31 serves as stack pointer
- **Interrupt model**: Single vector, maskable, with timer support

---

## 2. Register File

The IE64 has 32 general-purpose 64-bit registers, addressed by a 5-bit field (0-31).

| Register | Alias | Description |
|----------|-------|-------------|
| R0       | --    | Hardwired zero. Reads always return 0. Writes are silently discarded. |
| R1-R30   | --    | General-purpose registers. 64-bit read/write. |
| R31      | SP    | Stack pointer. Used implicitly by PUSH, POP, JSR, RTS, RTI, and interrupt entry. Initialized to `0x9F000` on reset. |

**Program Counter (PC)**:
- 64-bit internal register, not directly addressable.
- Masked to 25 bits (`PC & 0x1FFFFFF`) before every fetch, limiting the effective address space to 32MB.
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

> **64-bit memory access**: `.Q` loads and stores go through the system bus (`Read64`/`Write64`). For plain RAM (no I/O region on either 32-bit half), the bus uses a single 64-bit read/write. If the access spans an I/O region, the bus may split it into two 32-bit halves. For MMIO64 regions receiving a split write, a read-modify-write is performed on backing memory to preserve the untouched half. These semantics are transparent for normal RAM access but matter when accessing hardware registers with `.Q` size.

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

**Shift amount masking**: The shift count is always masked to 6 bits (`& 63`), limiting the effective shift range to 0-63.

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

### 4.6 Branches

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

**Encoding note for conditional branches**: Rs is in byte 2[7:3], Rt is in byte 3[7:3], and the branch offset is in bytes 4-7 (imm32). The Rd field (byte 1[7:3]) is unused (set to 0). The assembler computes the offset as `target_address - current_PC`.

**BRA** uses only the imm32 field. Rs and Rt fields are unused.

---

### 4.7 Subroutine / Stack

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| JSR      | `0x50` | `jsr label` | `SP -= 8; mem[SP] = PC + 8; PC = PC + offset` | Write | Q |
| RTS      | `0x51` | `rts` | `PC = mem[SP]; SP += 8` | Read | Q |
| PUSH     | `0x52` | `push Rs` | `SP -= 8; mem[SP] = Rs` | Write | Q |
| POP      | `0x53` | `pop Rd` | `Rd = mem[SP]; SP += 8` | Read | Q |

**JSR** (opcode `0x50`):
- Decrements SP (R31) by 8.
- Stores the return address (PC + 8, i.e., the instruction after the JSR) at the new SP.
- Branches to `PC + signExtend32to64(offset)`.
- Encoding: offset is `target_address - current_PC` in imm32.

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

### 4.8 System

| Mnemonic | Opcode | Syntax | Operation | Mem | Size |
|----------|--------|--------|-----------|-----|------|
| NOP      | `0xE0` | `nop` | No operation; PC += 8 | N | -- |
| HALT     | `0xE1` | `halt` | Stops execution | N | -- |
| SEI      | `0xE2` | `sei` | Enable interrupts (`interruptEnabled = true`) | N | -- |
| CLI      | `0xE3` | `cli` | Disable interrupts (`interruptEnabled = false`) | N | -- |
| RTI      | `0xE4` | `rti` | Return from interrupt | Read | Q |
| WAIT     | `0xE5` | `wait #usec` | Sleep for `imm32` microseconds; PC += 8 | N | -- |

**HALT** (opcode `0xE1`):
- Sets the running flag to false, terminating the Execute() loop.
- The PC is not advanced.

**SEI** (opcode `0xE2`):
- Atomically sets the interrupt-enabled flag to true.
- The next timer expiration after SEI will trigger an interrupt. Expirations that occurred while interrupts were disabled are not queued or replayed.

**CLI** (opcode `0xE3`):
- Atomically sets the interrupt-enabled flag to false.
- The timer continues running and counting, but expirations are silently discarded (not queued for later delivery).

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
| Register-indirect | `(Rs)` | Memory at address in Rs | LOAD, STORE |
| Displacement | `disp(Rs)` | Memory at `Rs + signExtend(disp)` | LOAD, STORE, LEA |
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

### 7.2 PC-Relative Offsets

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

The IE64 uses the same memory map as the Intuition Engine platform. The PC is masked to 25 bits (32MB), but the full address space accessible via LOAD/STORE extends to 32-bit addresses for I/O and VRAM access.

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
| `$F0C00-$F0C1C` | 28 B | PSG (AY-3-8910) registers |
| `$F0D00-$F0D1D` | 29 B | POKEY registers |
| `$F0E00-$F0E2D` | 45 B | SID (6581/8580) registers |
| `$F0F00-$F0F5F` | 96 B | TED registers |
| `$F1000-$F13FF` | 1 KB | VGA registers |
| `$F2000-$F200B` | 12 B | ULA (ZX Spectrum) registers |
| `$100000-$4FFFFF` | 4 MB | Video RAM (VRAM_START = `$100000`) |

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

> **Implementation note**: The interrupt mechanism exists and functions at the Go implementation level (`handleInterrupt()` does push PC and jump to `interruptVector`). However, since there is no assembly-level way to set `interruptVector` to a non-zero value, user programs cannot currently utilize timer interrupts. Host/test code can set the vector directly on the CPU struct (e.g., `cpu.interruptVector = addr`), though this field is unexported and internal.

### 9.5 Interrupt Programming Pattern (Aspirational)

> **Not yet functional**: The following pattern shows the *intended* future usage of interrupt vectors. In the current implementation, `interruptVector` is an internal CPU field that cannot be set from assembly. The `dc.q` at `$0000` writes to memory but has no effect on the CPU's `interruptVector` field. This example is retained to document the planned design; it will become functional when a memory-mapped vector table or a dedicated instruction is added.

```
    org $0000
    dc.q isr_handler       ; (reserved) interrupt vector at $0000

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

### 9.6 SEI/CLI Semantics

| Instruction | Effect |
|-------------|--------|
| SEI | Sets `interruptEnabled = true`. The timer continues running; the next expiration after SEI will trigger an interrupt. Note: expirations that occurred while interrupts were disabled are not queued or replayed. |
| CLI | Sets `interruptEnabled = false`. Timer continues running and counting, but interrupts are not delivered. |

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
| 0x30   | `$30`  | AND      | Logical | Rd, Rs, Rt/#imm |
| 0x31   | `$31`  | OR       | Logical | Rd, Rs, Rt/#imm |
| 0x32   | `$32`  | EOR      | Logical | Rd, Rs, Rt/#imm |
| 0x33   | `$33`  | NOT      | Logical | Rd, Rs |
| 0x34   | `$34`  | LSL      | Shift | Rd, Rs, Rt/#imm |
| 0x35   | `$35`  | LSR      | Shift | Rd, Rs, Rt/#imm |
| 0x36   | `$36`  | ASR      | Shift | Rd, Rs, Rt/#imm |
| 0x40   | `$40`  | BRA      | Branch | label |
| 0x41   | `$41`  | BEQ      | Branch | Rs, Rt, label |
| 0x42   | `$42`  | BNE      | Branch | Rs, Rt, label |
| 0x43   | `$43`  | BLT      | Branch | Rs, Rt, label |
| 0x44   | `$44`  | BGE      | Branch | Rs, Rt, label |
| 0x45   | `$45`  | BGT      | Branch | Rs, Rt, label |
| 0x46   | `$46`  | BLE      | Branch | Rs, Rt, label |
| 0x47   | `$47`  | BHI      | Branch | Rs, Rt, label |
| 0x48   | `$48`  | BLS      | Branch | Rs, Rt, label |
| 0x50   | `$50`  | JSR      | Subroutine | label |
| 0x51   | `$51`  | RTS      | Subroutine | (none) |
| 0x52   | `$52`  | PUSH     | Stack | Rs |
| 0x53   | `$53`  | POP      | Stack | Rd |
| 0xE0   | `$E0`  | NOP      | System | (none) |
| 0xE1   | `$E1`  | HALT     | System | (none) |
| 0xE2   | `$E2`  | SEI      | System | (none) |
| 0xE3   | `$E3`  | CLI      | System | (none) |
| 0xE4   | `$E4`  | RTI      | System | (none) |
| 0xE5   | `$E5`  | WAIT     | System | #usec |

### A.2 Opcode Ranges

| Range | Category |
|-------|----------|
| `$01-$04` | Data Movement |
| `$10-$11` | Memory Access |
| `$20-$27` | Arithmetic |
| `$30-$36` | Logical / Shift |
| `$40-$48` | Branches |
| `$50-$53` | Subroutine / Stack |
| `$E0-$E5` | System |

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
