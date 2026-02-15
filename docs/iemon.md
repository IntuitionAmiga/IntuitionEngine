# Intuition Engine Machine Monitor

## Overview

The Machine Monitor is a built-in system-level debugger inspired by the Commodore 64/Amiga Action Replay cartridge, HRTMon, and the Commodore Plus/4 built-in monitor. Press **F9** at any time to freeze the entire system and enter the monitor. Press **x** or **Esc** to resume execution.

The monitor works with all six CPU types (IE64, IE32, M68K, Z80, 6502, X86) and handles multi-CPU scenarios, including coprocessors.

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

### Expression Evaluation

Most address arguments support simple expressions with register names and arithmetic:

```
> d pc+$20         (disassemble from PC+0x20)
> m sp-8           (memory dump from SP-8)
> d $1000+r1       (disassemble from 0x1000 + R1)
> b pc+$100        (set breakpoint at PC+0x100)
```

Operators: `+` and `-` only. Each term is either a register name or a numeric address.

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

**Branch annotations:** Branch and jump instructions are annotated with target information. Backward branches (likely loops) are marked with `<- LOOP` in magenta. Lines that are branch targets within the visible window are prefixed with `T`.

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

### Memory Export/Import

#### `save <start> <end> <filename>` — Save Memory to File

Save a range of memory to a raw binary file.

```
> save $1000 $1FFF dump.bin
Saved $1000 bytes to dump.bin
```

#### `load <filename> <addr>` — Load File into Memory

Load a raw binary file into memory at the specified address.

```
> load dump.bin $2000
Loaded 4096 bytes at $2000
```

File size is capped at 32MB for safety.

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

#### `u <addr>` — Run Until

Run the program until the PC reaches the specified address, then stop and re-enter the monitor. Internally sets a temporary breakpoint that is automatically cleared when hit.

```
> u $2000
```

The monitor exits and execution resumes. When the PC reaches `$2000`, the monitor activates automatically and the temporary breakpoint is removed.

#### `bs` — Backstep (Undo Step)

Rewind the focused CPU to the state before the last `s` (single-step) command. Restores all CPU registers and memory to their exact pre-step values.

```
> s
Step: 1 instruction(s), 1 cycle(s)
  R1: $0 -> $2A
> bs
Backstep: restored to PC=$001000
```

A ring buffer of up to 32 snapshots is maintained (16 for 32-bit CPUs due to memory cost). Each `s` command saves a snapshot before stepping; `bs` pops the most recent one.

**Note:** Only CPU registers and memory are restored. Device/chip runtime state (timers, audio envelopes, video scanline position) is not included in snapshots.

### Breakpoints

#### `b <addr> [condition]` — Set Breakpoint

Set a breakpoint at an address. When the CPU executes an instruction at this address, the monitor activates automatically. An optional condition causes the breakpoint to fire only when the condition is true.

```
> b $1010
Breakpoint set at $1010

> b $1010 r1==$FF
Conditional breakpoint set at $1010 (r1 == $FF)

> b $2000 [$1000]==$42
Conditional breakpoint set at $2000 ([$1000] == $42)

> b $3000 hitcount>10
Conditional breakpoint set at $3000 (hitcount > 10)
```

**Condition syntax:**

| Format | Description |
|--------|-------------|
| `r1==$FF` | Register R1 equals 0xFF |
| `[$1000]==$42` | Memory byte at $1000 equals 0x42 |
| `hitcount>10` | Breakpoint hit count exceeds 10 |

Operators: `==`, `!=`, `<`, `>`, `<=`, `>=`

#### `bc <addr>` / `bc *` — Clear Breakpoint(s)

Clear a single breakpoint by address, or clear all breakpoints on the currently focused CPU.

```
> bc $1010
Breakpoint cleared at $1010

> bc *
All breakpoints cleared for focused CPU
```

#### `bl` — List Breakpoints

List all breakpoints across all CPUs, including conditions and hit counts.

```
> bl
$1010 (id:0 IE64)
$2000 (id:0 IE64) if r1 == $FF [hits: 3]
$0400 (id:3 coproc:Z80)
```

When a breakpoint is hit during normal execution, the monitor activates automatically, freezes all CPUs, and focuses on the CPU that hit the breakpoint:

```
BREAK at $1010 on IE64 (id:0)
```

### Watchpoints

#### `ww <addr>` — Set Write Watchpoint

Monitor a memory address for writes. When any instruction modifies the watched byte, the monitor activates and displays the old and new values.

```
> ww $1000
Write watchpoint set at $1000
```

When triggered:

```
WATCH at $1000 on IE64 (id:0): $00 -> $FF
```

#### `wc <addr>` / `wc *` — Clear Watchpoint(s)

Clear a single watchpoint by address, or clear all watchpoints on the focused CPU.

```
> wc $1000
Watchpoint cleared at $1000

> wc *
All watchpoints cleared for focused CPU
```

#### `wl` — List Watchpoints

List all watchpoints on the focused CPU.

```
> wl
$1000 (write)
$2000 (write)
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

Labels are matched exactly (case-insensitive).

If an exact label matches multiple CPUs, the command lists matches and asks for the ID:
```
> cpu coproc:z80
Ambiguous label, use ID:
  id:1 coproc:Z80
  id:5 coproc:Z80
```

#### `freeze <id|label|*>` — Freeze CPU

Freeze a specific CPU or all CPUs.

```
> freeze 1       (freeze CPU id:1)
> freeze coproc:z80     (freeze by label, must be unambiguous)
> freeze *       (freeze all)
```

#### `thaw <id|label|*>` — Thaw CPU

Resume a specific CPU while the monitor stays open. This allows advanced debugging where some CPUs run while others are frozen.

```
> thaw 1         (thaw CPU id:1)
> thaw *         (thaw all)
```

### Stack Trace

#### `bt [depth]` — Backtrace

Walk the stack and display return addresses. Default depth is 16.

```
> bt
#0 $001234
#1 $005678
#2 $009ABC

> bt 4
#0 $001234
#1 $005678
#2 $009ABC
#3 $00DEF0
```

Stack walking is CPU-specific:

| CPU | SP Register | Slot Size | Notes |
|-----|------------|-----------|-------|
| IE64 | R31 | 8 bytes | Addresses masked to 25-bit |
| IE32 | SP | 4 bytes | — |
| M68K | A7 | 4 bytes | — |
| Z80 | SP | 2 bytes | — |
| 6502 | SP (page 1) | 2 bytes | +1 offset (JSR pushes return-1) |
| X86 | ESP | 4 bytes | — |

### Save/Load Machine State

#### `ss [filename]` — Save State

Save a complete snapshot of the focused CPU's registers and memory to disk. Default filename: `snapshot.iem`.

```
> ss
State saved to snapshot.iem

> ss mystate.iem
State saved to mystate.iem
```

#### `sl [filename]` — Load State

Restore a previously saved snapshot, overwriting all CPU registers and memory.

```
> sl
State loaded from snapshot.iem

> sl mystate.iem
State loaded from mystate.iem
```

**Note:** Only CPU registers and memory are saved/restored. Device/chip runtime state (timers, audio, video) is not included.

### Trace and Write History

#### `trace <count>` — Trace Instructions

Execute exactly N instructions on the focused CPU, logging each instruction and register changes. The trace runs synchronously while the monitor is active.

```
> trace #10
001000: move.l r1, #$2A              R1=$2A
001008: add.l r2, r1, #$10           R2=$3A
...
Trace complete: 10 instructions
```

If a breakpoint is hit during tracing, the trace stops early:

```
> trace #1000
...
Trace stopped at breakpoint $1010
```

#### `trace file <path>` / `trace file off` — File Output

Direct trace output to a file instead of the scrollback buffer. Use `trace file off` to resume scrollback output.

```
> trace file trace.log
Trace output -> trace.log

> trace #10000

> trace file off
Trace output -> scrollback
```

#### `trace watch add <addr>` — Track Memory Writes

Add a memory address to the write-tracking list. During subsequent `trace` runs, writes to tracked addresses are recorded.

```
> trace watch add $1000
Watch added: $1000
```

#### `trace watch del <addr>` — Stop Tracking

Remove an address from the write-tracking list.

```
> trace watch del $1000
Watch removed: $1000
```

#### `trace watch list` — List Tracked Addresses

```
> trace watch list
$1000
$2000
```

#### `trace history show <addr>` — Show Write History

Display recorded writes to a tracked address, including the PC that performed the write, and old/new values.

```
> trace history show $1000
$1000: 4 writes recorded
  Step #42   PC=$001234  $00 -> $FF
  Step #108  PC=$005678  $FF -> $42
  Step #203  PC=$001234  $42 -> $00
  Step #510  PC=$009ABC  $00 -> $01
```

#### `trace history clear [addr|*]` — Clear History

Clear write history for a specific address or all addresses.

```
> trace history clear $1000
> trace history clear *
```

### I/O Register Viewer

#### `io [device]` — View I/O Registers

Display formatted I/O register values for a hardware device. Without arguments, lists available devices. Use `io all` to dump every device's registers at once.

```
> io
Available devices: video terminal audio ahx psg pokey sid sid2 sid3 ted vga ula antic gtia fileio media exec coproc voodoo

> io vga
=== VGA Registers ===
VGA_MODE       ($F1000) = $00000013  [19]
VGA_STATUS     ($F1004) = $00000000  [0]
VGA_CTRL       ($F1008) = $00000001  [1]
...

> io all
(dumps all 17 devices)
```

Register widths are per-register (1, 2, or 4 bytes). Values are displayed in the appropriate width with both hex and decimal representations.

### Hex Editor

#### `e <addr>` — Enter Hex Editor

Open an interactive hex editor at the specified address. The display switches to a hex grid showing 16 rows of 16 bytes (256 bytes total).

```
> e $1000
```

**Hex Editor Controls:**

| Key | Action |
|-----|--------|
| Arrow keys | Move cursor |
| 0-9, A-F | Edit current nibble |
| PgUp/PgDn | Scroll by 256 bytes |
| Enter | Commit changes to memory |
| Esc | Discard changes and return to command mode |

Changed bytes are highlighted in green. The cursor position is shown with inverted colors.

### Scripting

#### `script <filename>` — Run Command Script

Execute monitor commands from a text file, one command per line. Lines starting with `#` are treated as comments and skipped.

```
> script setup.mon
```

Example script file (`setup.mon`):
```
# Set up breakpoints for debugging
b $1000
b $2000 r1==$FF
ww $3000
trace watch add $3000
```

Scripts can nest up to 8 levels deep.

#### `macro <name> <commands>` — Define Macro

Define a named macro as a semicolon-separated list of commands. Invoke the macro by typing its name.

```
> macro inspect r ; d ; m sp 4
Macro 'inspect' defined (3 commands)

> inspect
(runs r, then d, then m sp 4)
```

Macros persist for the duration of the session.

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
| Left/Right | Move cursor within input line |
| Home / End | Jump to start/end of input line |
| Delete | Delete character at cursor |
| Backspace | Delete character before cursor |
| Ctrl+A / Ctrl+E | Jump to start/end of input line |
| Ctrl+K | Kill from cursor to end of line |
| Ctrl+U | Kill from start of line to cursor |
| Ctrl+Left/Right | Jump by word |
| Ctrl+Shift+V | Paste from clipboard |
| PgUp/PgDn | Scroll output buffer |
| Mouse wheel | Scroll output buffer |
| F9 | Toggle monitor on/off |
| F10 | Hard reset (works while monitor is active) |

Cursor movement, delete, and backspace keys repeat automatically when held.

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

### Using Conditional Breakpoints

1. `b $1000 r1==$FF` — break only when R1 is 0xFF
2. `x` — resume execution
3. The breakpoint is checked each time PC reaches $1000, but only fires when R1 equals 0xFF

### Tracing a Memory Write

1. `trace watch add $3000` — track writes to $3000
2. `trace #1000` — trace 1000 instructions
3. `trace history show $3000` — see which instructions wrote to $3000 and what values they wrote

### Saving and Restoring State

1. `ss checkpoint.iem` — save current state
2. (debug, modify registers, step through code)
3. `sl checkpoint.iem` — restore to the saved state

### Using Macros for Repetitive Tasks

1. `macro dump r ; d ; m sp 4 ; bt` — define a macro that shows registers, disassembly, stack memory, and backtrace
2. `s` — step an instruction
3. `dump` — run the macro to inspect everything at once

## Display

The monitor uses an 80x30 character grid rendered with the Amiga 1200 Topaz bitmap font. Colors follow classic monitor conventions:

- **White**: Default text
- **Cyan**: Headers, labels, informational messages
- **Yellow**: Current PC line in disassembly
- **Red**: Breakpoint markers, error messages
- **Green**: Changed register values, modified bytes in hex editor
- **Magenta**: Backward branch / loop markers
- **Dim blue**: Inactive/separator text
