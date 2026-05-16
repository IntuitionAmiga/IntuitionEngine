# IE32 Instruction Set Architecture Reference

Intuition Engine 32-bit RISC-like CPU - Complete ISA Specification

(c) 2024-2026 Zayn Otley - GPLv3 or later

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
    - [4.6 Branches](#46-branches)
    - [4.7 Subroutine / Stack](#47-subroutine--stack)
    - [4.8 Interrupt / Timer Control](#48-interrupt--timer-control)
    - [4.9 System](#49-system)
5. [Addressing Modes](#5-addressing-modes)
6. [Branch Architecture](#6-branch-architecture)
7. [Memory Map](#7-memory-map)
8. [Stack](#8-stack)
9. [Timer and Interrupt Model](#9-timer-and-interrupt-model)
10. [Assembly Language Quick Reference](#10-assembly-language-quick-reference)
11. [Execution Caveats](#11-execution-caveats)
12. [Appendix A: Opcode Map](#appendix-a-opcode-map)
13. [Appendix B: Encoding Examples](#appendix-b-encoding-examples)

---

## 1. Architecture Overview

The IE32 is a 32-bit RISC-like CPU implemented by `cpu_ie32.go`. It uses fixed
8-byte instructions, 16 general-purpose registers, a flat 32-bit address model,
and 32-bit little-endian memory operations.

- **Word size**: 32-bit registers and 32-bit arithmetic.
- **Instruction width**: Fixed 8 bytes per instruction.
- **Byte order**: Little-endian instruction operands and 32-bit memory accesses.
- **Register file**: 16 general-purpose 32-bit registers.
- **Address space**: 32-bit addresses. The default backing memory size is
  `DEFAULT_MEMORY_SIZE`, currently 32 MiB.
- **Integer condition model**: Conditional branches test one register against
  zero. There is no flags register.
- **Memory access width**: CPU load, store, stack, and ALU memory operands use
  32-bit reads and writes.
- **Stack**: Full-descending 32-bit stack. `SP` is an internal CPU register, not
  one of the 16 general-purpose registers.
- **Programme counter**: `PC` is a 32-bit internal CPU register, not directly
  addressable as a general-purpose register.

---

## 2. Register File

IE32 has 16 general-purpose 32-bit registers. Register operands are encoded in
the low 4 bits of a register field or operand field.

| Index | Register | Description |
|-------|----------|-------------|
| 0 | A | Accumulator and general-purpose register |
| 1 | X | General-purpose register |
| 2 | Y | General-purpose register |
| 3 | Z | General-purpose register |
| 4 | B | General-purpose register |
| 5 | C | General-purpose register |
| 6 | D | General-purpose register |
| 7 | E | General-purpose register |
| 8 | F | General-purpose register |
| 9 | G | General-purpose register |
| 10 | H | General-purpose register |
| 11 | S | General-purpose register |
| 12 | T | General-purpose register |
| 13 | U | General-purpose register |
| 14 | V | General-purpose register |
| 15 | W | General-purpose register |

**Special internal registers**:

| Register | Width | Initial value | Description |
|----------|-------|---------------|-------------|
| PC | 32-bit | `0x1000` | Programme counter |
| SP | 32-bit | `0x9F000` | Stack pointer |

`NewCPU` initialises `PC` to `PROG_START` (`0x1000`) and `SP` to
`STACK_START` (`0x9F000`). The general-purpose register fields default to zero
because the CPU struct is zero-initialised.

---

## 3. Instruction Encoding

Every IE32 instruction is exactly 8 bytes.

### 3.1 Byte-Level Format

```
Byte:    0        1        2        3        4    5    6    7
       +--------+--------+--------+--------+------+------+------+------+
       | opcode | reg    | mode   | zero   |       operand32 (LE)      |
       +--------+--------+--------+--------+------+------+------+------+
Bits:   [7:0]    [7:0]    [7:0]    [7:0]          [31:0]
```

### 3.2 Field Definitions

| Field | Byte | Width | Description |
|-------|------|-------|-------------|
| opcode | 0 | 8 bits | Instruction opcode |
| reg | 1 | 8 bits | Register field. Runtime uses the low 4 bits. |
| mode | 2 | 8 bits | Addressing mode for instructions that resolve an operand. |
| zero | 3 | 8 bits | Assembler emits zero. Current runtime ignores this byte. |
| operand32 | 4-7 | 32 bits | Immediate, address, target, register index, or encoded register plus offset. |

### 3.3 Field Extraction

```
opcode    = instr[0]
reg       = instr[1]
addrMode  = instr[2]
operand32 = instr[4] | (instr[5] << 8) | (instr[6] << 16) | (instr[7] << 24)
```

The runtime masks register indexes with `0x0F`, so raw machine code with high
bits set in a register field aliases to one of the 16 architectural registers.

### 3.4 Encoding Helper

```
instr[0] = opcode
instr[1] = register index, or 0 when unused
instr[2] = addressing mode, or 0 when unused
instr[3] = 0
instr[4..7] = operand32, little-endian
```

### 3.5 Addressing Mode Codes

| Code | Name | Meaning |
|------|------|---------|
| `0x00` | Immediate | Operand field is the value. |
| `0x01` | Register | Low 4 bits of operand field select a register. |
| `0x02` | Register indirect | Low 4 bits select a base register; remaining bits are an unsigned 32-bit offset contribution. |
| `0x03` | Memory indirect | For normal operand reads, operand field is read as memory. For store and memory `INC`/`DEC`, operand field points to the final address. |
| `0x04` | Direct | Operand field is a direct memory address. |

The assembler has syntax for immediate, register, register-indirect, and direct
operands. It defines the memory-indirect encoding constant, and the runtime
implements memory-indirect execution, but the assembler does not provide a
source syntax that emits mode `0x03`.

---

## 4. Complete Instruction Reference

### 4.1 Data Movement

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| LOAD | `0x01` | `LOAD R, operand` | `R = resolve(operand)` | Depends on operand |
| LDA | `0x20` | `LDA operand` | `A = resolve(operand)` | Depends on operand |
| LDX | `0x21` | `LDX operand` | `X = resolve(operand)` | Depends on operand |
| LDY | `0x22` | `LDY operand` | `Y = resolve(operand)` | Depends on operand |
| LDZ | `0x23` | `LDZ operand` | `Z = resolve(operand)` | Depends on operand |
| LDB | `0x3A` | `LDB operand` | `B = resolve(operand)` | Depends on operand |
| LDC | `0x3B` | `LDC operand` | `C = resolve(operand)` | Depends on operand |
| LDD | `0x3C` | `LDD operand` | `D = resolve(operand)` | Depends on operand |
| LDE | `0x3D` | `LDE operand` | `E = resolve(operand)` | Depends on operand |
| LDF | `0x3E` | `LDF operand` | `F = resolve(operand)` | Depends on operand |
| LDG | `0x3F` | `LDG operand` | `G = resolve(operand)` | Depends on operand |
| LDU | `0x40` | `LDU operand` | `U = resolve(operand)` | Depends on operand |
| LDV | `0x41` | `LDV operand` | `V = resolve(operand)` | Depends on operand |
| LDW | `0x42` | `LDW operand` | `W = resolve(operand)` | Depends on operand |
| LDH | `0x4C` | `LDH operand` | `H = resolve(operand)` | Depends on operand |
| LDS | `0x4D` | `LDS operand` | `S = resolve(operand)` | Depends on operand |
| LDT | `0x4E` | `LDT operand` | `T = resolve(operand)` | Depends on operand |

`LOAD R, operand` uses the destination register encoded in byte 1. The
register-specific load instructions encode the destination in the opcode and
also write the matching register index to byte 1.

Examples:

```asm
LOAD A, #42      ; A = 42
LOAD X, A        ; X = A
LDA VALUE        ; A = VALUE, as an immediate expression
LDA @VALUE       ; A = memory32[VALUE]
LDA [B]          ; A = memory32[B]
LDA [B+16]       ; A = memory32[B + 16]
```

### 4.2 Load/Store

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| STORE | `0x02` | `STORE R, operand` | `store(R, operand)` | Write |
| STA | `0x24` | `STA operand` | `store(A, operand)` | Write |
| STX | `0x25` | `STX operand` | `store(X, operand)` | Write |
| STY | `0x26` | `STY operand` | `store(Y, operand)` | Write |
| STZ | `0x27` | `STZ operand` | `store(Z, operand)` | Write |
| STB | `0x43` | `STB operand` | `store(B, operand)` | Write |
| STC | `0x44` | `STC operand` | `store(C, operand)` | Write |
| STD | `0x45` | `STD operand` | `store(D, operand)` | Write |
| STE | `0x46` | `STE operand` | `store(E, operand)` | Write |
| STF | `0x47` | `STF operand` | `store(F, operand)` | Write |
| STG | `0x48` | `STG operand` | `store(G, operand)` | Write |
| STU | `0x49` | `STU operand` | `store(U, operand)` | Write |
| STV | `0x4A` | `STV operand` | `store(V, operand)` | Write |
| STW | `0x4B` | `STW operand` | `store(W, operand)` | Write |
| STH | `0x4F` | `STH operand` | `store(H, operand)` | Write |
| STS | `0x50` | `STS operand` | `store(S, operand)` | Write |
| STT | `0x51` | `STT operand` | `store(T, operand)` | Write |

Store address calculation in the normal execution loop:

| Addressing mode | Store target |
|-----------------|--------------|
| Register indirect | `memory32[base register + encoded offset]` |
| Memory indirect | `memory32[memory32[operand32]]` |
| Immediate | `memory32[operand32]` |
| Register | `memory32[register index]`, not the contents of that register |
| Direct | `memory32[operand32]` |

This means that `STORE A, 0x5000`, `STORE A, #0x5000`, and `STORE A, @0x5000`
all write `A` to address `0x5000` in the normal execution loop. `STORE A, X`
assembles, but writes to address `1` because `X` is register index 1. Use direct
or register-indirect operands for stores.

Examples:

```asm
STORE A, @0x5000 ; memory32[0x5000] = A
STORE A, 0x5000  ; memory32[0x5000] = A
STA [B]          ; memory32[B] = A
STA [B+16]       ; memory32[B + 16] = A
```

All CPU load and store memory operations are 32-bit little-endian reads or
writes. There are no byte, halfword, or 8-byte CPU load/store opcodes.

### 4.3 Arithmetic

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| ADD | `0x03` | `ADD R, operand` | `R = R + resolve(operand)` | Depends on operand |
| SUB | `0x04` | `SUB R, operand` | `R = R - resolve(operand)` | Depends on operand |
| MUL | `0x14` | `MUL R, operand` | `R = R * resolve(operand)` | Depends on operand |
| DIV | `0x15` | `DIV R, operand` | `R = R / resolve(operand)` | Depends on operand |
| MOD | `0x16` | `MOD R, operand` | `R = R % resolve(operand)` | Depends on operand |

Arithmetic is unsigned 32-bit arithmetic with normal `uint32` wraparound. The
normal execution loop optimises multiplication, division, and modulo by
power-of-two operands, but the result is intended to match the corresponding
unsigned arithmetic operation.

Division or modulo by zero in the normal execution loop prints a diagnostic,
sets the CPU running flag to false, and stops execution.

Examples:

```asm
ADD A, #1
SUB X, Y
MUL A, @FACTOR
DIV A, [B+16]
MOD A, 10
```

### 4.4 Logical

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| AND | `0x05` | `AND R, operand` | `R = R & resolve(operand)` | Depends on operand |
| OR | `0x09` | `OR R, operand` | `R = R | resolve(operand)` | Depends on operand |
| XOR | `0x0A` | `XOR R, operand` | `R = R ^ resolve(operand)` | Depends on operand |
| NOT | `0x0D` | `NOT R` | `R = ^R` | None |

`NOT` takes one register operand. It does not resolve the instruction's
operand32 field in the normal execution loop.

### 4.5 Shifts

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| SHL | `0x0B` | `SHL R, operand` | `R = R << resolve(operand)` | Depends on operand |
| SHR | `0x0C` | `SHR R, operand` | `R = R >> resolve(operand)` | Depends on operand |

Shifts are logical shifts on `uint32` values. The normal execution loop uses the
resolved operand directly as the shift count.

### 4.6 Branches

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| JMP | `0x06` | `JMP target` | `PC = target` | None |
| JNZ | `0x07` | `JNZ R, target` | If `R != 0`, `PC = target`; else `PC += 8` | None |
| JZ | `0x08` | `JZ R, target` | If `R == 0`, `PC = target`; else `PC += 8` | None |
| JGT | `0x0E` | `JGT R, target` | If `int32(R) > 0`, `PC = target`; else `PC += 8` | None |
| JGE | `0x0F` | `JGE R, target` | If `int32(R) >= 0`, `PC = target`; else `PC += 8` | None |
| JLT | `0x10` | `JLT R, target` | If `int32(R) < 0`, `PC = target`; else `PC += 8` | None |
| JLE | `0x11` | `JLE R, target` | If `int32(R) <= 0`, `PC = target`; else `PC += 8` | None |

Branch and jump targets are absolute 32-bit addresses in the operand field.
The assembler accepts labels or equates as targets for `JMP`, `JSR`, and
conditional branches.

Conditional branches compare only the selected register against zero. They do
not compare two registers and do not use a flags register.

### 4.7 Subroutine / Stack

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| PUSH | `0x12` | `PUSH R` | `SP -= 4; memory32[SP] = R` | Write |
| POP | `0x13` | `POP R` | `R = memory32[SP]; SP += 4` | Read |
| JSR | `0x18` | `JSR target` | Push return address, then `PC = target` | Write |
| RTS | `0x19` | `RTS` | Pop return address into `PC` | Read |

`JSR` pushes `PC + 8`, then jumps to the absolute target address in operand32.
`RTS` pops a 32-bit return address from the stack.

The normal execution loop checks for stack overflow on push and stack underflow
on pop. On overflow or underflow it prints a diagnostic, clears the running
flag, and returns from the execution loop.

### 4.8 Interrupt / Timer Control

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| SEI | `0x1A` | `SEI` | Enable interrupt delivery | None |
| CLI | `0x1B` | `CLI` | Disable interrupt delivery | None |
| RTI | `0x1C` | `RTI` | Pop return address into `PC`; clear in-interrupt flag | Read |
| WAIT | `0x17` | `WAIT operand` | Sleep for `resolve(operand)` microseconds | Depends on operand |

`SEI` and `CLI` manipulate the CPU's internal interrupt-enable flag. `RTI` pops
the saved return address and clears the internal in-interrupt flag.

`WAIT` resolves its operand using the standard addressing modes. In the normal
execution loop, non-zero values sleep for that many microseconds. A zero value
does not sleep. Single-step execution advances `PC` for `WAIT` without sleeping.

### 4.9 System

| Mnemonic | Opcode | Syntax | Operation | Mem |
|----------|--------|--------|-----------|-----|
| NOP | `0xEE` | `NOP` | `PC += 8` | None |
| HALT | `0xFF` | `HALT` | Stop execution | None |

`HALT` prints a diagnostic, clears the running flag, and exits the normal
execution loop.

---

## 5. Addressing Modes

### 5.1 Immediate

Syntax:

```asm
#expr
```

Encoding:

| Field | Value |
|-------|-------|
| mode | `0x00` |
| operand32 | expression value |

For loads and ALU instructions, immediate mode supplies the value directly. For
stores and memory `INC`/`DEC`, the normal execution loop treats immediate mode
as a direct address because those instructions write to `operand32` for all
non-register-indirect and non-memory-indirect modes.

### 5.2 Bare Expression

Syntax:

```asm
expr
```

The assembler emits a bare resolved expression as immediate mode. This is
therefore a value for loads and ALU instructions, but an address for stores and
memory `INC`/`DEC` in the normal execution loop.

Example:

```asm
LOAD A, 0x5000   ; A = 0x5000
STORE A, 0x5000  ; memory32[0x5000] = A
```

### 5.3 Register

Syntax:

```asm
A
X
Y
Z
B
C
D
E
F
G
H
S
T
U
V
W
```

Encoding:

| Field | Value |
|-------|-------|
| mode | `0x01` |
| operand32 | register index |

For loads and ALU instructions, register mode resolves to the selected
register's current value. For `INC` and `DEC`, register mode increments or
decrements the selected register.

Register mode is not a useful store destination in the normal execution loop:
store instructions write to the numeric register index as a memory address.

### 5.4 Direct Memory

Syntax:

```asm
@expr
```

Encoding:

| Field | Value |
|-------|-------|
| mode | `0x04` |
| operand32 | expression value |

For loads and ALU instructions, direct mode resolves to `memory32[operand32]`.
For stores, direct mode writes to `memory32[operand32]`.

### 5.5 Register Indirect

Syntax:

```asm
[R]
[R+expr]
[R-expr]
```

Encoding:

| Field | Value |
|-------|-------|
| mode | `0x02` |
| operand32 bits 0-3 | base register index |
| operand32 bits 4-31 | offset bits |

The runtime computes:

```
address = register[operand32 & 0x0F] + (operand32 & 0xFFFFFFF0)
```

The assembler requires the offset expression to have its low 4 bits clear. In
normal source terms, use offsets that are multiples of 16. Negative offsets are
encoded as two's-complement 32-bit values and still pass the low-nibble check
when they are multiples of 16.

Examples:

```asm
LDA [B]       ; memory32[B]
LDA [B+16]    ; memory32[B + 16]
LDA [B-16]    ; memory32[B - 16], modulo 32-bit address arithmetic
```

### 5.6 Memory Indirect

Encoding:

| Field | Value |
|-------|-------|
| mode | `0x03` |
| operand32 | address used by the runtime mode handler |

For load, ALU, branch-helper operand resolution, and `WAIT`, the normal runtime
resolves memory-indirect mode exactly like direct mode:

```
value = memory32[operand32]
```

For stores and memory `INC`/`DEC`, memory-indirect mode uses `operand32` as the
address of a 32-bit pointer to the final target address:

```
target = memory32[operand32]
memory32[target] = new value
```

The assembler does not currently provide source syntax for this mode.

---

## 6. Branch Architecture

IE32 branch instructions use absolute 32-bit targets. There is no PC-relative
branch encoding.

`JMP` and `JSR` unconditionally load `PC` from operand32. Conditional branch
instructions use byte 1 to select a register and operand32 as the absolute
target address.

Signed branch tests reinterpret the register value as `int32`:

| Branch | Test |
|--------|------|
| JNZ | `R != 0` |
| JZ | `R == 0` |
| JGT | `int32(R) > 0` |
| JGE | `int32(R) >= 0` |
| JLT | `int32(R) < 0` |
| JLE | `int32(R) <= 0` |

The assembler target operand for `JMP`, `JSR`, and conditional branches must be
a defined label or equate by final assembly pass.

---

## 7. Memory Map

The CPU uses the shared machine bus. The following constants define the legacy
IE32 layout in the current runtime:

| Range / Address | Description |
|-----------------|-------------|
| `0x00000` | Interrupt vector table base. Timer interrupt entry reads a 32-bit handler address from this address. |
| `0x01000` | `PROG_START`. Programme load address and initial `PC`. |
| `0x02000` | `DATA_START` in the assembler constants. Also the stack overflow boundary. |
| `0x02000-0x9EFFF` | Conventional programme and data area. |
| `0x9F000` | `STACK_START`. Initial `SP`. |
| `0xA0000` | `IO_REGION_START`. Addresses below this use the CPU direct-memory fast path. Addresses at or above this go through the bus. |
| `0xF0800` | `IO_BASE`, the audio-chip I/O base used by runtime constants. |
| `0xF0804` | `TIMER_COUNT` constant in `cpu_ie32.go`. |
| `0xF0808` | `TIMER_PERIOD` constant in `cpu_ie32.go`. |

`LoadProgramBytes` clears and loads only the fixed programme load window
`[PROG_START, STACK_START)`. Bytes at and above `STACK_START` are not modified
by programme reload.

For non-I/O addresses, the CPU reads and writes 32-bit little-endian values
directly from the bus memory slice. Stores to an attached direct VRAM buffer can
bypass the bus when the configured VRAM range matches the target address. For
other addresses at or above `IO_REGION_START`, 32-bit reads and writes are
routed through the bus.

---

## 8. Stack

The stack is full-descending and stores 32-bit little-endian words.

Initial state:

```
SP = 0x9F000
```

Push:

```
SP = SP - 4
memory32[SP] = value
```

Pop:

```
value = memory32[SP]
SP = SP + 4
```

Normal execution checks:

| Operation | Failure condition |
|-----------|-------------------|
| Push / JSR / interrupt entry | `SP < STACK_BOTTOM + 4` before decrement |
| Pop / RTS / RTI | `SP >= STACK_START` before read |

`STACK_BOTTOM` is `0x2000`. The normal execution loop stops the CPU on stack
overflow or underflow.

---

## 9. Timer and Interrupt Model

The CPU has internal timer and interrupt state:

| Field | Meaning |
|-------|---------|
| `timerEnabled` | Timer active flag |
| `timerCount` | Current timer count |
| `timerPeriod` | Reload period |
| `timerState` | `TIMER_STOPPED`, `TIMER_RUNNING`, or `TIMER_EXPIRED` |
| `interruptEnabled` | Global interrupt delivery flag |
| `inInterrupt` | True while servicing an interrupt |

In the normal execution loop, if the timer is enabled, `cycleCounter` increments
once per executed instruction. When `cycleCounter` reaches `SAMPLE_RATE`, it is
reset to zero and `timerCount` is decremented if it is non-zero. When the count
reaches zero:

1. `timerState` becomes `TIMER_EXPIRED`.
2. If interrupts are enabled and the CPU is not already in an interrupt,
   interrupt handling is entered.
3. If the timer remains enabled, `timerCount` is reloaded from `timerPeriod`.

Interrupt entry pushes the current `PC` and then reads a 32-bit handler address
from `VECTOR_TABLE` (`0x0000`) into `PC`.

`SEI` enables interrupt delivery. `CLI` disables interrupt delivery. `RTI` pops
the saved `PC` and clears the in-interrupt flag.

The runtime constants `TIMER_COUNT` and `TIMER_PERIOD` are addresses in the I/O
range, but the timer state used by the CPU execution loop is held in CPU
internal atomic fields.

---

## 10. Assembly Language Quick Reference

### 10.1 Tool

The assembler command accepts:

```text
ie32asm [-v] [-o output] [-Werror] [-Wno-category] [-I dir]... <input.asm>
```

If `-o` is omitted, the output path is the input path with its extension
replaced by `.iex`.

### 10.2 Case Sensitivity

Instruction mnemonics and register names are case-sensitive in the current
assembler. The supported instruction mnemonics are uppercase. Directives are
lowercase.

### 10.3 Directives

| Directive | Effect |
|-----------|--------|
| `.word expr[, expr...]` | Emit one 32-bit little-endian value per expression. |
| `.byte expr[, expr...]` | Emit one byte per expression. |
| `.equ name expr` | Define an equate. |
| `.org expr` | Set the current assembly address. |
| `.space expr` | Emit `expr` zero bytes. |
| `.ascii "text"` | Emit string bytes. |
| `.asciz "text"` | Emit string bytes followed by `0x00`. |
| `.incbin "file"` | Include a binary file. |
| `.incbin "file", offset` | Include from byte offset to end of file. |
| `.incbin "file", offset, length` | Include a byte range. |
| `.include "file"` | Preprocess and assemble another source file. |

`.word` accepts values from `-2147483648` through `0xFFFFFFFF`, then emits the
low 32 bits. `.byte` accepts values from `-128` through `0xFF`, then emits the
low 8 bits.

`.org` may move the current address forward. If it moves backward, the assembler
records an `org-backward` warning and later rejects overlapping emitted bytes.

`.include` is expanded before assembly. Include resolution checks the including
file's directory first, then each `-I` directory in command-line order. Recursive
include processing tracks the current include stack and skips a file that is
already active in that stack.

`.incbin` uses the same path resolution order as `.include`. Included binary
payloads are cached by absolute path during assembly.

### 10.4 Labels

A label is written as:

```asm
name:
```

Labels may appear on the same source line as an instruction or directive:

```asm
start: LDA #1
```

The source preprocessor splits leading `label:` prefixes before assembly. A
colon is treated as a label separator only when the text before it contains no
spaces or tabs.

Duplicate labels are accepted. On the first layout pass, redefining a label
records a `duplicate-labels` warning unless that warning category is suppressed.
The later definition overwrites the earlier address.

### 10.5 Expressions

Expressions are evaluated by the assembler. Supported forms:

| Form | Meaning |
|------|---------|
| Decimal | `1234` |
| Hex with prefix | `0x1234`, `0X1234` |
| Hex with dollar prefix | `$1234` |
| Underscores | `1_000`, `$FF_FF` |
| Symbols | Labels and equates |
| Parentheses | `(expr)` |
| Unary operators | `+`, `-`, `~` |
| Binary operators | `*`, `/`, `+`, `-`, `&`, `^`, `|` |

Operator precedence, highest to lowest:

1. Parentheses and atoms.
2. Unary `+`, `-`, `~`.
3. `*`, `/`.
4. `+`, `-`.
5. `&`.
6. `^`.
7. `|`.

Division by zero in an assembly-time expression is an assembly error.

Instruction operands, `.equ`, and `.word` values must fit in signed 32-bit or
unsigned 32-bit range: `-2147483648` through `0xFFFFFFFF`. `.org`, `.space`, and
`.incbin` offset and length expressions must fit in `0` through `0xFFFFFFFF`.

### 10.6 String Escapes

`.ascii` and `.asciz` recognise these escapes:

| Escape | Byte emitted |
|--------|--------------|
| `\n` | Line feed |
| `\t` | Tab |
| `\r` | Carriage return |
| `\\` | Backslash |
| `\"` | Double quote |
| `\0` | NUL |
| `\xHH` | Byte with two hex digits |

For any other escaped character, the assembler emits that character.

### 10.7 Comments

`;` starts a comment unless it appears inside a string literal or character
literal. Comments are stripped before assembly.

### 10.8 Operand Splitting

Generic two-operand instructions split operands with `strings.Split` on comma.
They require exactly one comma in the source line. Register-specific load/store,
`INC`, `DEC`, `NOT`, `PUSH`, `POP`, `JMP`, `JSR`, `WAIT`, and zero-operand
instructions use whitespace-based parsing as implemented in the assembler.

Data directives and `.incbin` use comma splitting that respects quoted strings
and character literals.

---

## 11. Execution Caveats

### 11.1 Normal Execution Is the Architectural Reference

The continuous execution loop in `CPU.Execute` is the authoritative behaviour
for normal programme execution. `CPU.StepOne` is a debugger-oriented helper and
does not exactly match every normal execution path.

Known differences in `StepOne` include:

| Area | Normal execution | Single-step helper |
|------|------------------|--------------------|
| Store addressing | Handles register-indirect and memory-indirect store destinations. | Store opcodes write to operand32 directly. |
| `NOT` | Computes `R = ^R`. | Computes `R = ^resolvedOperand`. Assembled `NOT R` has operand32 zero. |
| `INC` / `DEC` | Supports register, register-indirect, memory-indirect, and direct/immediate memory forms. | Increments or decrements the register selected by byte 1. Assembled `INC R` and `DEC R` have byte 1 set to zero. |
| `DIV` / `MOD` by zero | Stops execution. | Leaves the destination unchanged and advances `PC`. |
| `WAIT` | Sleeps for non-zero resolved microseconds. | Does not sleep. |
| Shift count | Uses the resolved operand directly. | Masks the shift count with `31`. |
| Signed branches | Compare the selected register against zero. | `JGT`, `JGE`, `JLT`, and `JLE` compare against the resolved operand. |
| Stack bounds | Checks overflow and underflow. | Performs stack memory access without the same bounds checks. |

### 11.2 Raw Encoding Can Express More Than Source Syntax

The runtime decodes the mode byte directly. Raw machine code can use
memory-indirect mode `0x03`. The assembler currently has no source syntax for
that mode.

### 11.3 Store Register Operand Hazard

The assembler accepts register operands for store instructions because register
operands are part of the generic operand parser. In normal execution, stores do
not treat register mode as "store to address contained in that register".

Example:

```asm
STORE A, X
```

This writes `A` to address `1`, because `X` is register index 1. To store
through a register, use:

```asm
STORE A, [X]
```

### 11.4 Register-Indirect Offset Granularity

Register-indirect offset encoding stores the register index in the low 4 bits
of operand32. The runtime masks those bits out to recover the offset. The
assembler therefore rejects offsets whose low 4 bits are non-zero. Source-level
register-indirect offsets must be multiples of 16.

---

## Appendix A: Opcode Map

### A.1 Opcode Summary Table

| Opcode | Mnemonic | Category | Operand form |
|--------|----------|----------|--------------|
| `0x01` | LOAD | Data movement | `R, operand` |
| `0x02` | STORE | Load/store | `R, operand` |
| `0x03` | ADD | Arithmetic | `R, operand` |
| `0x04` | SUB | Arithmetic | `R, operand` |
| `0x05` | AND | Logical | `R, operand` |
| `0x06` | JMP | Branch | `target` |
| `0x07` | JNZ | Branch | `R, target` |
| `0x08` | JZ | Branch | `R, target` |
| `0x09` | OR | Logical | `R, operand` |
| `0x0A` | XOR | Logical | `R, operand` |
| `0x0B` | SHL | Shift | `R, operand` |
| `0x0C` | SHR | Shift | `R, operand` |
| `0x0D` | NOT | Logical | `R` |
| `0x0E` | JGT | Branch | `R, target` |
| `0x0F` | JGE | Branch | `R, target` |
| `0x10` | JLT | Branch | `R, target` |
| `0x11` | JLE | Branch | `R, target` |
| `0x12` | PUSH | Stack | `R` |
| `0x13` | POP | Stack | `R` |
| `0x14` | MUL | Arithmetic | `R, operand` |
| `0x15` | DIV | Arithmetic | `R, operand` |
| `0x16` | MOD | Arithmetic | `R, operand` |
| `0x17` | WAIT | Timer | `operand` |
| `0x18` | JSR | Stack / branch | `target` |
| `0x19` | RTS | Stack / branch | none |
| `0x1A` | SEI | Interrupt | none |
| `0x1B` | CLI | Interrupt | none |
| `0x1C` | RTI | Interrupt | none |
| `0x20` | LDA | Data movement | `operand` |
| `0x21` | LDX | Data movement | `operand` |
| `0x22` | LDY | Data movement | `operand` |
| `0x23` | LDZ | Data movement | `operand` |
| `0x24` | STA | Load/store | `operand` |
| `0x25` | STX | Load/store | `operand` |
| `0x26` | STY | Load/store | `operand` |
| `0x27` | STZ | Load/store | `operand` |
| `0x28` | INC | Arithmetic | `operand` |
| `0x29` | DEC | Arithmetic | `operand` |
| `0x3A` | LDB | Data movement | `operand` |
| `0x3B` | LDC | Data movement | `operand` |
| `0x3C` | LDD | Data movement | `operand` |
| `0x3D` | LDE | Data movement | `operand` |
| `0x3E` | LDF | Data movement | `operand` |
| `0x3F` | LDG | Data movement | `operand` |
| `0x40` | LDU | Data movement | `operand` |
| `0x41` | LDV | Data movement | `operand` |
| `0x42` | LDW | Data movement | `operand` |
| `0x43` | STB | Load/store | `operand` |
| `0x44` | STC | Load/store | `operand` |
| `0x45` | STD | Load/store | `operand` |
| `0x46` | STE | Load/store | `operand` |
| `0x47` | STF | Load/store | `operand` |
| `0x48` | STG | Load/store | `operand` |
| `0x49` | STU | Load/store | `operand` |
| `0x4A` | STV | Load/store | `operand` |
| `0x4B` | STW | Load/store | `operand` |
| `0x4C` | LDH | Data movement | `operand` |
| `0x4D` | LDS | Data movement | `operand` |
| `0x4E` | LDT | Data movement | `operand` |
| `0x4F` | STH | Load/store | `operand` |
| `0x50` | STS | Load/store | `operand` |
| `0x51` | STT | Load/store | `operand` |
| `0xEE` | NOP | System | none |
| `0xFF` | HALT | System | none |

### A.2 Addressing Mode Summary

| Mode | Name | Assembler syntax | Runtime resolution |
|------|------|------------------|--------------------|
| `0x00` | Immediate | `#expr` or bare `expr` | `operand32` |
| `0x01` | Register | `R` | `register[operand32 & 0x0F]` |
| `0x02` | Register indirect | `[R]`, `[R+expr]`, `[R-expr]` | `memory32[register[operand32 & 0x0F] + (operand32 & 0xFFFFFFF0)]` |
| `0x03` | Memory indirect | none | Read operands: `memory32[operand32]`. Store and memory `INC`/`DEC` targets: `memory32[memory32[operand32]]`. |
| `0x04` | Direct | `@expr` | `memory32[operand32]` |

---

## Appendix B: Encoding Examples

### B.1 `LDA #$12345678`

Fields:

| Field | Value |
|-------|-------|
| opcode | `0x20` |
| reg | `0x00` |
| mode | `0x00` |
| zero | `0x00` |
| operand32 | `0x12345678` |

Bytes:

```text
20 00 00 00 78 56 34 12
```

### B.2 `LOAD X, A`

Fields:

| Field | Value |
|-------|-------|
| opcode | `0x01` |
| reg | `0x01` |
| mode | `0x01` |
| zero | `0x00` |
| operand32 | `0x00000000` |

Bytes:

```text
01 01 01 00 00 00 00 00
```

### B.3 `STA @0x5000`

Fields:

| Field | Value |
|-------|-------|
| opcode | `0x24` |
| reg | `0x00` |
| mode | `0x04` |
| zero | `0x00` |
| operand32 | `0x00005000` |

Bytes:

```text
24 00 04 00 00 50 00 00
```

### B.4 `LDA [B+16]`

`B` is register index 4. The encoded operand is `0x10 | 0x04 = 0x14`.

Fields:

| Field | Value |
|-------|-------|
| opcode | `0x20` |
| reg | `0x00` |
| mode | `0x02` |
| zero | `0x00` |
| operand32 | `0x00000014` |

Bytes:

```text
20 00 02 00 14 00 00 00
```

### B.5 `JNZ A, loop`

If `loop` resolves to `0x00001020`:

Fields:

| Field | Value |
|-------|-------|
| opcode | `0x07` |
| reg | `0x00` |
| mode | `0x00` |
| zero | `0x00` |
| operand32 | `0x00001020` |

Bytes:

```text
07 00 00 00 20 10 00 00
```

### B.6 `PUSH W`

`W` is register index 15.

Fields:

| Field | Value |
|-------|-------|
| opcode | `0x12` |
| reg | `0x0F` |
| mode | `0x00` |
| zero | `0x00` |
| operand32 | `0x00000000` |

Bytes:

```text
12 0F 00 00 00 00 00 00
```
