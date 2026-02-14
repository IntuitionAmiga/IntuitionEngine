# Intuition Engine Machine Monitor

## Overview

The Machine Monitor is a built-in system-level debugger inspired by the Commodore 64/Amiga Action Replay cartridge, HRTMon, and the Commodore Plus/4 built-in monitor. Press **F9** at any time to freeze the entire system and enter the monitor. Press **x** or **Esc** to resume execution.

The monitor works with all six CPU types (IE64, IE32, M68K, Z80, 6502, X86) and handles multi-CPU scenarios, including coprocessors and music playback engines.

## Quick Start

1. Run any program: `./bin/IntuitionEngine -ie64 program.ie64`
2. Press **F9** to freeze and enter the monitor
3. Type `r` to see registers
4. Type `d` to disassemble around the program counter
5. Type `s` to single-step one instruction
6. Type `x` to resume execution

## Address Formats

All commands accept addresses in these formats:

| Format | Example | Description |
|--------|---------|-------------|
| `$hex` | `$1000` | Hex with dollar sign (classic monitor convention) |
| `0xhex` | `0x1000` | Hex with 0x prefix |
| bare hex | `1000` | Bare hexadecimal (default) |
| `#decimal` | `#4096` | Decimal with hash prefix |

## Command Reference

### Execution Control

#### `s [count]` — Single-Step

Execute one (or more) instructions on the focused CPU. Displays changed registers in green, followed by the next instruction to be executed.

```
> s
Step: 1 instruction(s), 1 cycle(s)
  R1: $0 -> $2A
> 001008: E0 00 00 00 00 00 00 00  nop
```

Step 10 instructions:
```
> s #10
Step: 10 instruction(s), 10 cycle(s)
```

#### `g [addr]` — Go/Continue

Resume execution and exit the monitor. Optionally set the PC before resuming.

```
> g          (resume from current PC)
> g $2000    (set PC to $2000, then resume)
```

#### `x` — Exit Monitor

Resume all CPUs and close the monitor overlay. Equivalent to pressing Esc.

### Inspection

#### `r` — Show Registers

Display all registers of the focused CPU. Registers that changed since the last step are shown in green.

```
> r
PC   $00001000
R0   $0000000000000000
R1   $000000000000002A    (green = changed)
...
```

#### `r <name> <value>` — Set Register

Modify a register value.

```
> r pc $2000
PC = $2000
> r r1 #42
R1 = $2A
```

#### `d [addr] [count]` — Disassemble

Disassemble instructions starting from an address (default: current PC, 16 lines). The current PC is marked with `>`, breakpoints with `*`.

```
> d
> 001000: 01 81 00 00 2A 00 00 00  move.l r1, #$2A
  001008: E0 00 00 00 00 00 00 00  nop
* 001010: 01 81 00 00 FF 00 00 00  move.l r1, #$FF

> d $2000 8    (disassemble 8 instructions from $2000)
```

#### `m [addr] [count]` — Memory Dump

Display memory in hex + ASCII format (default: from PC, 8 lines of 16 bytes).

```
> m $1000 4
001000: 01 81 00 00 2A 00 00 00  E0 00 00 00 00 00 00 00  ....*...........
001010: 01 81 00 00 FF 00 00 00  00 00 00 00 00 00 00 00  ................
001020: 00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  ................
001030: 00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  ................
```

### Memory Modification

#### `w <addr> <bytes..>` — Write Bytes

Write individual bytes to memory.

```
> w $1000 DE AD BE EF
Wrote 4 byte(s) at $1000
```

#### `f <start> <end> <byte>` — Fill Memory

Fill a memory range with a single byte value.

```
> f $2000 $20FF 00
Filled $2000-$20FF with $00
```

### Memory Tools

#### `h <start> <end> <bytes..>` — Hunt/Search

Search a memory range for a byte pattern.

```
> h $0 $FFFF DE AD
Found at $1000
Found at $3456
```

#### `c <start> <end> <dest>` — Compare Memory

Compare two memory ranges and report differences.

```
> c $1000 $100F $2000
$1000: DE != 00 (at $2000)
$1001: AD != 00 (at $2001)
```

#### `t <start> <end> <dest>` — Transfer/Copy Memory

Copy a memory range to another location.

```
> t $1000 $100F $2000
Transferred 16 bytes from $1000 to $2000
```

### Breakpoints

#### `b <addr>` — Set Breakpoint

Set a breakpoint at an address. When the CPU executes an instruction at this address, the monitor activates automatically.

```
> b $1010
Breakpoint set at $1010
```

#### `bc <addr>` / `bc *` — Clear Breakpoint(s)

Clear a single breakpoint or all breakpoints.

```
> bc $1010
Breakpoint cleared at $1010

> bc *
All breakpoints cleared
```

#### `bl` — List Breakpoints

List all breakpoints across all CPUs.

```
> bl
$1010 (id:0 IE64)
$0400 (id:3 coproc:Z80)
```

When a breakpoint is hit during normal execution, the monitor activates automatically, freezes all CPUs, and focuses on the CPU that hit the breakpoint:

```
BREAK at $1010 on IE64 (id:0)
```

### Multi-CPU Commands

#### `cpu` — List CPUs

List all registered CPUs with their ID, label, status, and program counter. The focused CPU is marked with `*`.

```
> cpu
*id:0   IE64         [FROZEN ]  PC=$1000
 id:1   coproc:Z80   [FROZEN ]  PC=$0040
 id:2   coproc:6502  [FROZEN ]  PC=$0200
```

#### `cpu <id|label>` — Switch Focus

Switch the focused CPU by stable ID or label. All register/disassembly/step commands operate on the focused CPU.

```
> cpu 1
Focused on id:1 coproc:Z80
```

If a label matches multiple CPUs, the command lists matches and asks for the ID:
```
> cpu 6502
Ambiguous label, use ID:
  id:2 coproc:6502
  id:4 music:6502
```

#### `freeze <id|label|*>` — Freeze CPU

Freeze a specific CPU or all CPUs.

```
> freeze 1       (freeze CPU id:1)
> freeze z80     (freeze by label, must be unambiguous)
> freeze *       (freeze all)
```

#### `thaw <id|label|*>` — Thaw CPU

Resume a specific CPU while the monitor stays open. This allows advanced debugging where some CPUs run while others are frozen.

```
> thaw 1         (thaw CPU id:1)
> thaw *         (thaw all)
```

### Audio Control

#### `fa` — Freeze Audio

Freeze audio playback. By default, audio continues playing while the monitor is open (it's output-only and doesn't affect memory state). Use this command to silence audio during debugging.

```
> fa
Audio frozen
```

#### `ta` — Thaw Audio

Resume audio playback.

```
> ta
Audio thawed
```

### Help

#### `?` / `help` — Command Reference

Display a quick command reference.

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| Enter | Submit command |
| Esc | Exit monitor (same as `x`) |
| Up/Down | Navigate command history |
| PgUp/PgDn | Scroll output buffer |
| F9 | Toggle monitor on/off |
| F10 | Hard reset (works while monitor is active) |

## CPU-Specific Notes

### IE64 (64-bit RISC)
- 32 general-purpose 64-bit registers: R0-R31
- R0 is always zero, R31 is the stack pointer (SP)
- Fixed 8-byte instruction encoding
- Register display: 16-digit hex (`$0000000000001000`)

### IE32 (32-bit RISC)
- 16 named registers: PC, SP, A, X, Y, Z, B-W
- Fixed 8-byte instruction encoding
- Register display: 8-digit hex (`$00001000`)

### M68K (Motorola 68020)
- Data registers D0-D7, address registers A0-A7
- A7 is the stack pointer, A6 is typically the frame pointer
- SR (status register), USP (user stack pointer)
- Variable-length instructions (2-10 bytes)
- Big-endian byte order

### Z80
- Main registers: A, F, B, C, D, E, H, L
- Shadow registers: A', F', B', C', D', E', H', L'
- Index registers: IX, IY
- Other: SP, PC, I, R
- Register display: 4-digit hex (`$0040`)

### 6502 (MOS Technology)
- A (accumulator), X, Y (index registers)
- SP (stack pointer, 8-bit), PC (program counter, 16-bit)
- SR (status register with N/V/-/B/D/I/Z/C flags)
- All instructions are 1-3 bytes

### X86 (Intel 32-bit)
- General: EAX, EBX, ECX, EDX
- Index: ESI, EDI
- Pointer: ESP, EBP
- EIP (instruction pointer), EFLAGS
- Segment registers: CS, DS, ES, FS, GS, SS
- Variable-length instructions (1-15 bytes)

## Multi-CPU Debugging Workflows

### Debugging a Coprocessor

1. Press F9 to enter the monitor
2. `cpu` to list all CPUs — coprocessors appear automatically
3. `cpu 1` to focus on the coprocessor
4. `r` to inspect registers, `d` to disassemble
5. `s` to single-step the coprocessor
6. `cpu 0` to switch back to the primary CPU
7. `x` to resume all

### Stepping One CPU While Others Run

1. Press F9 (all CPUs frozen)
2. `thaw 1` — let the coprocessor run freely
3. `s` — step the primary CPU while coprocessor executes
4. `freeze *` — re-freeze everything to inspect shared state
5. `m $3000 4` — examine shared memory

### Setting a Breakpoint and Continuing

1. `b $2000` — set breakpoint at address $2000
2. `x` — exit monitor and resume
3. When execution reaches $2000, the monitor activates automatically
4. `r` — inspect the state at the breakpoint
5. `bc $2000` — clear the breakpoint
6. `x` — resume

## Display

The monitor uses an 80x30 character grid rendered with the Amiga 1200 Topaz bitmap font. Colors follow classic monitor conventions:

- **White**: Default text
- **Cyan**: Headers, labels, informational messages
- **Yellow**: Current PC line in disassembly
- **Red**: Breakpoint markers, error messages
- **Green**: Changed register values
- **Dim blue**: Inactive/separator text
