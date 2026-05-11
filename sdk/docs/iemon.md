# Intuition Engine Machine Monitor

## Overview

The Machine Monitor is a built-in system-level debugger inspired by the Commodore 64/Amiga Action Replay cartridge, HRTMon, and the Commodore Plus/4 built-in monitor. Press **F9** at any time to freeze the entire system and enter the monitor. Press **x** or **Esc** to resume execution.

The monitor works with all six CPU types (IE64, IE32, M68K, Z80, 6502, X86) and handles multi-CPU scenarios, including coprocessors.
It is also exposed to IEScript Lua via the `dbg.*` API for scripted debugging workflows. See [iescript.md](iescript.md) for the full `dbg.*` module reference.

**Availability:** The monitor is always built into the engine — no command-line flag or build tag is required. Press F9 from any running program to enter. If an IEScript Lua REPL is also bound (F8), the monitor takes priority while it is active and F8 is suppressed.

## Quick Start

1. Run any typed program: `./bin/IntuitionEngine program.ie64`
2. Press **F9** to freeze and enter the monitor
3. Type `r` to see registers
4. Type `d` to disassemble around the program counter
5. Type `s` to single-step one instruction
6. Type `x` to resume execution

## Address Formats

Address arguments accept these formats:

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

Count arguments for `s`, `m`, `trace`, and `bt` are decimal by default. Use `$` or `0x` for hexadecimal counts. The `d` (disassemble) line count is the exception: it is parsed as an address and so defaults to **hexadecimal** (e.g. `d $2000 10` disassembles 0x10 = 16 lines). Bare address arguments remain hexadecimal by default. The `#decimal` prefix is only recognised for address/value arguments (e.g. `r r1 #42`), not for count arguments.

## Command Reference

### Execution Control

#### `s [count]` - Single-Step

Execute one (or more) instructions on the focused CPU. Displays changed registers in green, followed by the next instruction to be executed.

```
> s
Step: 1 instruction(s), 1 cycle(s)
  R1: $0 -> $2A
> 001008: E0 00 00 00 00 00 00 00  nop
```

Step 10 instructions (counts are decimal):
```
> s 10
Step: 10 instruction(s), 10 cycle(s)
```

#### `g [addr]` - Go/Continue

Resume execution and exit the monitor. Optionally set the PC before resuming.

```
> g          (resume from current PC)
> g $2000    (set PC to $2000, then resume)
```

If the supplied address fails to parse, the command resumes from the current PC without setting it and without an error message.

#### `x` - Exit Monitor

Resume all CPUs and close the monitor overlay. Equivalent to pressing Esc.

### Inspection

#### `r` - Show Registers

Display all registers of the focused CPU. Registers that changed since the last step are shown in green.

```
> r
PC   $00001000
R0   $0000000000000000
R1   $000000000000002A    (green = changed)
...
```

#### `r <name> <value>` - Set Register

Modify a register value.

```
> r pc $2000
PC = $2000
> r r1 #42
R1 = $2A
```

#### `d [addr] [count]` - Disassemble

Disassemble instructions starting from an address (default: current PC, 16 lines). The current PC is marked with `>`, breakpoints with `*`.

```
> d
> 001000: 01 81 00 00 2A 00 00 00  move.l r1, #$2A
  001008: E0 00 00 00 00 00 00 00  nop
* 001010: 01 81 00 00 FF 00 00 00  move.l r1, #$FF

> d $2000 8    (disassemble 8 instructions from $2000)
```

**Branch annotations:** Branch and jump instructions are annotated with target information. Backward branches (likely loops) are marked with `<- LOOP` in magenta. Lines that are branch targets within the visible window are prefixed with `T`.

#### `m [addr] [count]` - Memory Dump

Display memory in hex + ASCII format (default: from PC, 8 lines of 16 bytes). The address column follows the focused CPU width, so IE64 dumps preserve full 64-bit addresses.

```
> m $1000 4
001000: 01 81 00 00 2A 00 00 00  E0 00 00 00 00 00 00 00  ....*...........
001010: 01 81 00 00 FF 00 00 00  00 00 00 00 00 00 00 00  ................
001020: 00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  ................
001030: 00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  ................
```

### Memory Modification

#### `w <addr> <bytes..>` - Write Bytes

Write individual bytes to memory.

```
> w $1000 DE AD BE EF
Wrote 4 byte(s) at $1000
```

#### `f <start> <end> <byte>` - Fill Memory

Fill a memory range with a single byte value.

```
> f $2000 $20FF 00
Filled $2000-$20FF with $00
```

### Memory Export/Import

#### `save <start> <end> <filename>` - Save Memory to File

Save a range of memory to a raw binary file. The maximum save range is the bus-reported total guest RAM, not a fixed legacy 32 MiB limit.

```
> save $1000 $1FFF dump.bin
Saved $1000 bytes to dump.bin
```

#### `load <filename> <addr>` - Load File into Memory

Load a raw binary file into memory at the specified address.

```
> load dump.bin $2000
Loaded 4096 bytes at $2000
```

File size is capped for safety; `iemon` rejects files larger than the active visible RAM reported by the bus, ensuring loads cannot exceed the autodetected guest memory window.

### Memory Tools

#### `h <start> <end> <bytes..>` - Hunt/Search

Search a memory range for a byte pattern.

```
> h $0 $FFFF DE AD
Found at $1000
Found at $3456
```

#### `c <start> <end> <dest>` - Compare Memory

Compare two memory ranges and report differences.

```
> c $1000 $100F $2000
$1000: DE != 00 (at $2000)
$1001: AD != 00 (at $2001)
```

#### `t <start> <end> <dest>` - Transfer/Copy Memory

Copy a memory range to another location.

```
> t $1000 $100F $2000
Transferred 16 bytes from $1000 to $2000
```

#### `u <addr>` - Run Until

Run the program until the PC reaches the specified address, then stop and re-enter the monitor. Internally sets a temporary breakpoint that is automatically cleared when hit.

```
> u $2000
```

The monitor exits and execution resumes. When the PC reaches `$2000`, the monitor activates automatically and the temporary breakpoint is removed. If run-until temporarily disables an existing conditional breakpoint, the condition is restored when that stop is handled.

If execution never reaches the target address before the monitor is re-entered for another reason, the temporary breakpoint remains set and will fire on a future run. Use `bc <addr>` to clear it explicitly if you no longer want the stop.

#### `bs` - Backstep (Undo Step)

Rewind the focused CPU to the state before the last `s` (single-step) command. Restores focused CPU registers and the CPU-local memory snapshot captured by that adapter.

```
> s
Step: 1 instruction(s), 1 cycle(s)
  R1: $0 -> $2A
> bs
Backstep: restored to PC=$001000
```

A ring buffer of up to 32 CPU-local snapshots is maintained. Each stepped instruction saves a snapshot before stepping; `bs` pops the most recent one.

**Note:** Only the focused CPU adapter's registers and memory view are restored. Device/chip runtime state (timers, audio envelopes, video scanline position), other CPUs, and coprocessor state are not included. Whole-machine snapshots are future work.

### Breakpoints

#### `b <addr> [condition]` - Set Breakpoint

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
| `[$1000].W==$1234` | Memory word at $1000 equals 0x1234 |
| `[$1000].L==$12345678` | Memory long at $1000 equals 0x12345678 |
| `hitcount>10` | Breakpoint hit count exceeds 10 |

Operators: `==`, `!=`, `<`, `>`, `<=`, `>=`

#### `bc <addr>` / `bc *` - Clear Breakpoint(s)

Clear a single breakpoint by address, or clear all breakpoints on the currently focused CPU. `bc *` clears only the focused CPU's breakpoints; use `cpu <id>` then `bc *` on each CPU to clear globally.

```
> bc $1010
Breakpoint cleared at $1010

> bc *
All breakpoints cleared
```

#### `bl` - List Breakpoints

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

#### `ww <addr>` - Set Write Watchpoint

Monitor a memory address for writes. When any instruction modifies the watched byte, the monitor activates and displays the old and new values. Read and read/write access watchpoints are not implemented in this hardening pass.

```
> ww $1000
Write watchpoint set at $1000
```

When triggered:

```
WATCH at $1000 on IE64 (id:0): $00 -> $FF
```

#### `wc <addr>` / `wc *` - Clear Watchpoint(s)

Clear a single watchpoint by address, or clear all watchpoints on the focused CPU.

```
> wc $1000
Watchpoint cleared at $1000

> wc *
All watchpoints cleared
```

#### `wl` - List Watchpoints

List all watchpoints on the focused CPU.

```
> wl
$1000 (write)
$2000 (write)
```

### Multi-CPU Commands

#### `cpu` - List CPUs

List all registered CPUs with their ID, label, status, and program counter. The focused CPU is marked with `*`.

```
> cpu
*id:0   IE64         [FROZEN ]  PC=$1000
 id:1   coproc:Z80   [FROZEN ]  PC=$0040
 id:2   coproc:6502  [FROZEN ]  PC=$0200
```

#### `cpu <id|label>` - Switch Focus

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

#### `freeze <id|label|*>` - Freeze CPU

Freeze a specific CPU or all CPUs.

```
> freeze 1       (freeze CPU id:1)
> freeze coproc:z80     (freeze by label, must be unambiguous)
> freeze *       (freeze all)
```

#### `thaw <id|label|*>` - Thaw CPU

Resume a specific CPU while the monitor stays open. This allows advanced debugging where some CPUs run while others are frozen.

```
> thaw 1         (thaw CPU id:1)
> thaw *         (thaw all)
```

### Stack Trace

#### `bt [depth]` - Backtrace

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

| CPU | Source | Slot Size | Notes |
|-----|--------|-----------|-------|
| IE64 | SP | 8 bytes (LE) | Full 64-bit return addresses; the legacy 25-bit mask was retired in PLAN_MAX_RAM slice 3 |
| IE32 | SP | 4 bytes (LE) | - |
| M68K | A6 frame-link chain | 4 bytes (BE) | Walks `prevA6 = mem[A6]; ret = mem[A6+4]` (LINK/UNLK convention). Returns "No stack frames found" if A6 is 0, so leaf or link-less functions produce no trace |
| Z80 | SP | 2 bytes (LE) | - |
| 6502 | SP (page 1) | 2 bytes (LE) | Each frame is tagged `(low confidence)` in output; reads from `$0100 + ((SP+1) & 0xFF)` upward, adding +1 because JSR pushes return-1 |
| X86 | ESP | 4 bytes (LE) | Raw stack slot scan; no EBP frame-chain walk |

### Save/Load Machine State

#### `ss [filename]` - Save State

Save a complete snapshot of the focused CPU's registers and memory to disk. Default filename: `snapshot.iem`.

```
> ss
State saved to snapshot.iem (CPU+memory)

> ss mystate.iem
State saved to mystate.iem (CPU+memory)
```

#### `sl [filename]` - Load State

Restore a previously saved snapshot, overwriting all CPU registers and memory.

```
> sl
State loaded from snapshot.iem (CPU+memory)

> sl mystate.iem
State loaded from mystate.iem (CPU+memory)
```

**Note:** `ss`/`sl` operate only on the focused CPU and its memory view. Other CPUs and device/chip runtime state (timers, audio envelopes, video scanline position) are not included. `sl` refuses to load a snapshot whose CPU type differs from the focused CPU.

### Trace and Write History

#### `trace <count>` - Trace Instructions

Execute exactly N instructions on the focused CPU, logging each instruction and register changes. The trace runs synchronously while the monitor is active.

```
> trace 10
001000: move.l r1, #$2A              R1=$2A
001008: add.l r2, r1, #$10           R2=$3A
...
Trace complete: 10 instructions
```

If a breakpoint is hit during tracing, the trace stops early:

```
> trace 1000
...
Trace stopped at breakpoint $1010
```

#### `trace file <path>` / `trace file off` - File Output

Direct trace output to a file instead of the scrollback buffer. Use `trace file off` to resume scrollback output.

```
> trace file trace.log
Trace output to trace.log

> trace 10000

> trace file off
Trace file output stopped
```

Trace files are synced before they are closed.

#### `trace watch add <addr>` - Track Memory Writes

Add a memory address to the write-tracking list. During subsequent `trace` runs, writes to tracked addresses are recorded.

```
> trace watch add $1000
Trace watch added at $1000
```

#### `trace watch del <addr>` - Stop Tracking

Remove an address from the write-tracking list.

```
> trace watch del $1000
Trace watch removed at $1000
```

#### `trace watch list` - List Tracked Addresses

```
> trace watch list
  $1000
  $2000
```

#### `trace history show <addr>` - Show Write History

Display recorded writes to a tracked address, including the PC that performed the write, and old/new values.

```
> trace history show $1000
$1000: 4 writes recorded
  Step #42   PC=$001234  $00 -> $FF
  Step #108  PC=$005678  $FF -> $42
  Step #203  PC=$001234  $42 -> $00
  Step #510  PC=$009ABC  $00 -> $01
```

#### `trace history clear [addr|*]` - Clear History

Clear write history for a specific address or all addresses.

```
> trace history clear $1000
> trace history clear *
```

### I/O Register Viewer

#### `io [device]` - View I/O Registers

Display formatted I/O register values for a hardware device. Without arguments, lists available devices. Use `io all` to dump every device's registers at once.

```
> io
Available I/O devices:
  ahx
  antic
  audio
  coproc
  exec
  fileio
  gtia
  irqdiag
  media
  pokey
  psg
  sid
  sid2
  sid3
  sn76489
  sysinfo
  ted
  terminal
  ula
  vga
  video
  voodoo

> io vga
=== VGA Registers ===
VGA_MODE       ($F1000) = $00000013  [19]
VGA_STATUS     ($F1004) = $00000000  [0]
VGA_CTRL       ($F1008) = $00000001  [1]
...

> io all
(dumps all 22 devices)
```

Register widths are per-register (1, 2, or 4 bytes). Values are displayed in the appropriate width with both hex and decimal representations.

### Hex Editor

#### `e <addr>` - Enter Hex Editor

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

#### `script <filename>` - Run Command Script

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

#### `macro <name> <commands>` - Define Macro

Define a named macro as a semicolon-separated list of commands. Invoke the macro by typing its name.

```
> macro inspect r ; d ; m sp 4
Macro 'inspect' defined (3 commands)

> inspect
(runs r, then d, then m sp 4)
```

Macros persist for the duration of the session.

### Audio Control

#### `fa` - Freeze Audio

Freeze audio playback. By default, audio continues playing while the monitor is open (it's output-only and doesn't affect memory state). Use this command to silence audio during debugging.

```
> fa
Audio frozen
```

#### `ta` - Thaw Audio

Resume audio playback.

```
> ta
Audio thawed
```

### Help

#### `?` / `help` - Command Reference

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
| Shift+Left/Right | Extend selection by one character |
| Shift+Up/Down | Extend selection by one line |
| Shift+Home/End | Extend selection to start/end of line |
| Ctrl+Shift+C | Copy selected text to clipboard |
| Ctrl+Shift+X | Cut selected text (input line only) |
| Middle mouse button | Paste selection (or clipboard if nothing selected) |
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
2. `cpu` to list all CPUs - coprocessors appear automatically
3. `cpu 1` to focus on the coprocessor
4. `r` to inspect registers, `d` to disassemble
5. `s` to single-step the coprocessor
6. `cpu 0` to switch back to the primary CPU
7. `x` to resume all

### Stepping One CPU While Others Run

1. Press F9 (all CPUs frozen)
2. `thaw 1` - let the coprocessor run freely
3. `s` - step the primary CPU while coprocessor executes
4. `freeze *` - re-freeze everything to inspect shared state
5. `m $3000 4` - examine shared memory

### Setting a Breakpoint and Continuing

1. `b $2000` - set breakpoint at address $2000
2. `x` - exit monitor and resume
3. When execution reaches $2000, the monitor activates automatically
4. `r` - inspect the state at the breakpoint
5. `bc $2000` - clear the breakpoint
6. `x` - resume

### Using Conditional Breakpoints

1. `b $1000 r1==$FF` - break only when R1 is 0xFF
2. `x` - resume execution
3. The breakpoint is checked each time PC reaches $1000, but only fires when R1 equals 0xFF

### Tracing a Memory Write

1. `trace watch add $3000` - track writes to $3000
2. `trace 1000` - trace 1000 instructions (decimal)
3. `trace history show $3000` - see which instructions wrote to $3000 and what values they wrote

### Saving and Restoring State

1. `ss checkpoint.iem` - save current state
2. (debug, modify registers, step through code)
3. `sl checkpoint.iem` - restore to the saved state

### Using Macros for Repetitive Tasks

1. `macro dump r ; d ; m sp 4 ; bt` - define a macro that shows registers, disassembly, stack memory, and backtrace
2. `s` - step an instruction
3. `dump` - run the macro to inspect everything at once

## Display

The monitor overlay is a character grid sized to the current video mode (`screenWidth / 8` columns × `screenHeight / 16` rows, e.g. 100×37 at the 800×600 default), rendered with the Amiga Topaz 8 bitmap font. Colors follow classic monitor conventions:

- **White**: Default text
- **Cyan**: Headers, labels, informational messages
- **Yellow**: Current PC line in disassembly
- **Red**: Breakpoint markers, error messages
- **Green**: Changed register values, modified bytes in hex editor
- **Magenta**: Backward branch / loop markers
- **Dim blue**: Inactive/separator text

## GURU MEDITATION Fault Lines (IE64)

When an IE64 fault escapes the IntuitionOS / `iexec` kernel, the kernel
itself prints a `GURU MEDITATION` line on the host console with the
full fault context. The machine monitor does **not** synthesize this
line; it is emitted from kernel string tables (`sdk/intuitionos/iexec/boot/strings.s`).
The cause-code table below is reproduced here for convenience when
reading those reports; the canonical source is `IE64_ISA.md` §12.8.
M15.6 adds the `SKEF` and `SKAC` causes:

| Cause | Label | Trigger |
|------:|-------|---------|
| 0     | `page-not-present` | PTE `P==0` |
| 1     | `read-denied`      | PTE `R==0` on load |
| 2     | `write-denied`     | PTE `W==0` on store |
| 3     | `exec-denied`      | PTE `X==0` on instruction fetch |
| 4     | `user-supervisor`  | User mode access to `PTE_U==0` page |
| 5     | `priv`             | User-mode execution of a privileged instruction |
| 6     | `syscall`          | `SYSCALL` instruction |
| 7     | `misaligned`       | Atomic RMW with misaligned address |
| 8     | `timer`            | Timer interrupt (via INTR_VEC) |
| 9     | `skef`             | Supervisor instruction fetch from user page (`MMU_CTRL.SKEF`) |
| 10    | `skac`             | Supervisor data access to user page with `MMU_CTRL.SKAC` set and `MMU_CTRL.SUA` clear |

The `SKEF` and `SKAC` lines are new in M15.6 and indicate a kernel
bug: either a stray supervisor fetch into a user page or a missing
`SUAEN` / `SUADIS` bracket around a kernel user-memory access. See
`IE64_COOKBOOK.md` "Supervisor-User Access Helpers" and
`IntuitionOS/IExec.md` for the canonical usercopy helpers that make
this class of fault impossible when used correctly.

The emitted line format otherwise follows the M15.5 contract:
`cause`, `PC`, `ADDR`, `task`, `ACCESS`, `MODE`, `CLASS`, and `PTE`
bits are all present. Nested-trap state (outer `FAULT_PC`,
`CR_SAVED_SUA`) is preserved architecturally by the CPU's
trap-frame stack and is not part of the printed line.

Note: the monitor's IE64 disassembler recognises the new `suaen` and
`suadis` mnemonics so listings of the shipped iexec kernel show the
helper bracket at its real source locations rather than as raw
`dc.b $F3` / `dc.b $F4`.

## Common Pitfalls

- **`d` count is hexadecimal.** `d $1000 10` shows 0x10 = 16 lines, not 10. Counts for `s`, `m`, `trace`, and `bt` are decimal.
- **`#N` does not work for count arguments.** It is only honoured for address/value parsing (e.g. `r r1 #42`, `m #4096`). `s #10` silently steps one instruction.
- **Watchpoints are write-only.** `ww` traps on store. There is no read or read/write watch in this revision.
- **`ss`/`sl` are focused-CPU only.** Other CPUs, video/audio/timer device state, and coprocessor state are not in the snapshot. `sl` will refuse a snapshot whose CPU type differs from the focused CPU.
- **`bs` (backstep) is focused-CPU only.** It restores that adapter's registers and memory view from the per-CPU ring (max 32 entries). It does not roll back device state or other CPUs.
- **M68K backtrace requires A6 frame links.** Code that does not use LINK/UNLK (or has not yet set A6) will produce "No stack frames found".
- **6502 backtrace is heuristic.** Each frame is tagged `(low confidence)`; the walker scans page 1 upward from SP+1 and assumes JSR-pushed return addresses.
- **`bc *` / `wc *` clear only the focused CPU.** Switch CPUs and repeat to clear globally; `bl` lists across all CPUs but `bc *` does not.
- **Run-until leaks its temp breakpoint if never hit.** If the PC never reaches the `u` target before you re-enter the monitor for another reason, clear it explicitly with `bc <addr>`.
- **`g <addr>` silently ignores parse errors** and resumes from the current PC. Use `r pc <addr>` first if you want a hard error on bad input.
