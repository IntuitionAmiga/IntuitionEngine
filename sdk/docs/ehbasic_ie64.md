# EhBASIC IE64 User Manual

Comprehensive reference for the Enhanced BASIC interpreter running on the Intuition Engine IE64 CPU.

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Getting Started](#2-getting-started)
3. [Language Reference](#3-language-reference)
4. [Statement Reference](#4-statement-reference)
5. [Function Reference](#5-function-reference)
6. [Video Programming Guide](#6-video-programming-guide)
7. [Audio Programming Guide](#7-audio-programming-guide)
8. [Memory Map Reference](#8-memory-map-reference)
9. [Hardware Register Reference](#9-hardware-register-reference)
10. [Machine Code Interface](#10-machine-code-interface)
11. [Runtime Errors](#11-runtime-errors)
12. [Example Programmes](#12-example-programmes)
13. [Appendices](#13-appendices)

---

## 1. Introduction

EhBASIC IE64 is a port of Lee Davison's Enhanced BASIC (EhBASIC) to the Intuition Engine's IE64 RISC processor. The original EhBASIC was written in 6502 assembly and later ported to the Motorola 68000; this version is a ground-up reimplementation in IE64 assembly, preserving the language semantics whilst taking advantage of the IE64's 64-bit register file and compare-and-branch instructions.

The Intuition Engine is a retro-inspired virtual machine that emulates multiple CPUs (6502, Z80, M68K, x86, IE32, IE64) alongside IE-native and compatibility-inspired video/audio hardware: VGA with copper coprocessor and blitter, ULA (ZX Spectrum), TED (Commodore 16|Plus/4), ANTIC/GTIA (Atari-inspired display list), Voodoo 3DFX, and a full complement of sound chips and players (SoundChip, PSG/AY-3-8910, SID/MOS 6581, POKEY, TED audio, AHX, MOD, WAV, MIDI/MUS, SAP, VGM/VGZ, SNDH, VTX, AY, YM, and Z80 PSG tracker formats). EhBASIC IE64 provides direct access to all of this hardware through extension commands.

### Floating-Point Arithmetic: Important Note

**This port uses IEEE 754 single-precision (FP32) arithmetic.** This differs from other EhBASIC ports: the original 6502 version and the 68K port both use a custom 6-byte packed floating-point format with a 7-bit biased exponent, providing approximately 9 decimal digits of precision.

EhBASIC IE64's FP32 representation gives:

- **Precision**: ~7 decimal digits (24-bit mantissa: 23 explicit + 1 implicit)
- **Range**: approximately +/-3.4 x 10^38
- **Special values**: +/-0, +/-Infinity, NaN

The trade-off is clear: FP32 sacrifices roughly 2 digits of precision compared to the original format, but gains IEEE 754 compatibility, hardware-friendly bit layouts, and simpler register handling (the 32-bit value sits in the lower half of a 64-bit IE64 register, leaving the upper 32 bits free for bit manipulation).

For most BASIC programmes, 7 digits is more than sufficient. Financial calculations or programmes that chain many arithmetic operations may notice rounding differences compared to the original EhBASIC.

---

## 2. Getting Started

### Building

EhBASIC IE64 is assembled from source and optionally embedded into the Intuition Engine binary.

```bash
# Assemble the BASIC interpreter
sdk/bin/ie64asm -I sdk/include sdk/examples/asm/ehbasic_ie64.asm

# Build the VM with embedded BASIC
make basic
```

The `make basic` target:
1. Assembles `sdk/examples/asm/ehbasic_ie64.asm`
2. Moves the generated image to `sdk/examples/prebuilt/ehbasic_ie64.ie64`
3. Builds the Intuition Engine binary with the `embed_basic` build tag, which embeds that prebuilt BASIC image via Go's `//go:embed` directive

Release builds always include `embed_basic`, so packaged Linux, Windows, and macOS archives can boot directly into the embedded BASIC prompt without an extra ROM file.

### Running

```bash
# Run with embedded BASIC image
./bin/IntuitionEngine -basic

# Run with a custom BASIC binary
./bin/IntuitionEngine -basic-image path/to/custom.ie64

# Run with console terminal (stdin/stdout, no GUI window)
./bin/IntuitionEngine -basic -term
```

On startup, EhBASIC IE64 displays a banner and the `Ready` prompt:

```
EhBASIC IE64 v3.1
(c) Zayn Otley, 2024-2026
Based on EhBASIC by Lee Davison
Ready
```

### Your First Programme

Type lines with line numbers to store them in the programme:

```basic
10 PRINT "Hello, World!"
20 END
RUN
```

Output:
```
Hello, World!
Ready
```

Type commands without line numbers to execute them immediately:

```basic
PRINT 2 + 2
```

Output:
```
 4
Ready
```

### REPL Commands

| Command | Description |
|---------|-------------|
| `RUN` | Execute the stored programme (interpreted) |
| `RUN AOT` | Compile the stored programme to native IE64 code, then run it (see [Native Compilation](#native-compilation)) |
| `COMPILE "name"` | Compile the stored programme to a standalone `name.ie64` file |
| `TRANSPILE "name"` | Transpile the stored programme to `name.asm` only (no assembly, no `.ie64`) |
| `ASSEMBLE "name"` | Assemble a user-written `name.asm` from disk to `name.ie64` with the in-guest assembler |
| `LIST` | Display the programme listing |
| `DIR` / `DIR "path"` | Display a File I/O sandbox directory listing |
| `NEW` | Clear the programme from memory |
| `EMUTOS` | Boot EmuTOS through the programme executor |
| `AROS` | Boot AROS through the programme executor |
| `INTUITIONOS` | Boot IntuitionOS through the programme executor |
| Line number followed by text | Store or replace a programme line |
| Line number alone | Delete that line |

### Native Compilation

In addition to the interpreter, EhBASIC IE64 can compile your stored programme to native IE64 machine code. Compilation is performed entirely inside the guest: a transpiler lowers the tokenised BASIC to an IE64 assembly text stream, and a private in-guest assembler encodes that stream to machine code. No host services or external tools are involved.

There are two entry points.

#### RUN AOT

`RUN AOT` compiles the current stored programme and runs the result immediately. It prints `Compiling to native code...`, emits native code into a top-of-RAM arena, and jumps to it. Variables, arrays, strings and the DATA pointer behave exactly as under interpreted `RUN`: execution restarts from the first line with control stacks reset.

```
10 FOR I=1 TO 5
20 PRINT I
30 NEXT I
RUN AOT
```

Because the arena sits alongside the resident interpreter, `RUN AOT` may delegate a statement to the resident runtime helper for that statement when a native lowering is not provided. This keeps behaviour identical to the interpreter for the full statement set whilst the control flow itself runs as native code.

#### COMPILE

`COMPILE "name"` writes a standalone flat `.ie64` image whose entry point is the programme start. The `.ie64` extension is appended case-insensitively when absent, so `COMPILE "DEMO"` writes `DEMO.ie64` and `COMPILE "DEMO.IE64"` is left unchanged. The file is written beside the most recently `LOAD`ed programme; if no programme has been loaded, it is written to the File I/O root.

A standalone image has no resident interpreter to delegate to. The image opens with a small bootstrap that sets the stack and the terminal pointer. Programmes that use only literal operands (for example `PRINT` of a string or number, `POKE`, unconditional `GOTO`) bundle just the few print helpers they need and stay lean. Programmes that use expressions, variables, arrays, strings or the `DATA`/`INPUT`/`LIST`/`SAVE` runtime bundle a position-fixed runtime image (the expression evaluator and the variable, array, string, floating-point and statement-execution routines) into the `.ie64` and call into it through a fixed jump table, so the compiled programme runs the same evaluator and statement handlers as the interpreter with no resident interpreter present. The bundled runtime makes the image self-contained: it runs in a bare machine with no host services, no sidecar files and (except for `SAVE`) no File I/O device.

Alongside `name.ie64`, `COMPILE` also writes `name.asm`, the transpiled IE64 assembly source it assembled, so you can inspect or reassemble the generated code.

#### TRANSPILE

`TRANSPILE "name"` runs only the first half of `COMPILE`: it transpiles the stored programme to IE64 assembly text and writes `name.asm`, without assembling it or producing a `.ie64` image. The `.asm` it writes is byte-for-byte identical to the sidecar `COMPILE` would write for the same programme, and it is placed in the same location (beside the most recently `LOAD`ed programme, or the File I/O root). Name validation, the direct-only/raw-root rejection and the unsupported-statement reporting are the same as `COMPILE`. Use it to read the generated assembly without producing a binary.

The emitted `.asm` is fully self-contained: any runtime support the programme needs (the runtime blob, the `fp_print` number-formatting closure, and the tokenised programme for `READ`/`DATA`) is inlined as `dc.b` data under the `__rtpay`, `__rtprog` and `fp_print` labels, exactly as `COMPILE` assembles it. This makes the `.asm` a true source artifact that `ASSEMBLE` reassembles back into the same `.ie64` image (`COMPILE "x"` and `TRANSPILE "x"` then `ASSEMBLE "x"` produce identical binaries). The trade-off is size: a programme that uses the runtime blob produces an `.asm` of roughly 190 KiB regardless of how short the BASIC is, because the ~34 KiB blob is written out as decimal `dc.b` bytes.

#### ASSEMBLE

`ASSEMBLE "name"` reads a user-written `name.asm` from disk, assembles it at `PROGRAM_START` with the in-guest private assembler, and writes `name.ie64`. Both files sit beside the most recently `LOADed` programme (or the File I/O root). It is a general assembler for IE64 source, independent of any stored BASIC programme (the stored programme is left untouched). Because `COMPILE`/`TRANSPILE` now emit fully self-contained assembly (the runtime blob and other support inlined as `dc.b` data), `ASSEMBLE` is the true inverse of `TRANSPILE`: a transpiled `.asm` reassembles into the same `.ie64` image that `COMPILE` produces directly. It also assembles hand-written IE64 source.

The in-guest assembler supports a deliberate subset:

- Instructions from the IE64 instruction set, with labels (`name:`) and PC-relative branches/`jsr`.
- Data directives `dc.b`/`dc.w`/`dc.l`/`dc.q` (comma-separated values and quoted strings) and `align`.
- Named constants from `ie64.inc` (for example `TERM_OUT`, `VGA_STATUS`), resolved through a build-time-baked constant table, so source can use the symbolic names directly.
- `include "ie64.inc"` is accepted as a no-op, because those constants are already available, so the normal source shape works:

```
include "ie64.inc"
start:
    move.l r26, #TERM_OUT
    move.q r1, #65
    store.b r1, (r26)
    halt
```

Anything outside that subset (any other `include`, `incbin`, `equ`, `org`, macros, conditionals, an unknown mnemonic, or an unresolved label/constant) is reported as a `?COMPILE ERROR` and no `.ie64` is written. A missing or unreadable `name.asm`, or a source larger than the assembler's limit (just under 1 MiB), raises `?FILE ERROR`.

#### Supported statements

These lower to native code and run under both `RUN AOT` and standalone `COMPILE`:

- `GOTO`, `GOSUB`, `RETURN`.
- `POKE`/`POKE8`/`DOKE`/`LOKE`, `BITSET`, `BITCLR`, `CALL`, `WAIT`, `VSYNC`.
- `PRINT` of a string literal or a numeric literal.
- `END`, `STOP`.

These use the bundled runtime (expressions, variables, arrays, strings, `DATA`) and now run under standalone `COMPILE` as well as `RUN AOT`:

- `IF ... THEN ...`, including `THEN <line>` (a jump) and `IF ... THEN ... ELSE ...` on a single line.
- `FOR ... NEXT`.
- `WHILE ... WEND` and `DO ... LOOP` with a bottom test (`LOOP UNTIL`, `LOOP WHILE`).
- `ON <expr> GOTO`/`GOSUB`.
- Implied `LET` (numeric, string and array-element assignment) and `DIM`.
- `PRINT` of any form (variables, expressions, `;` and `,` separators).
- `READ`/`DATA`/`RESTORE` (the tokenised programme is bundled so the `DATA` reader can scan it).
- `INPUT` (optional prompt and a variable list; reads from the terminal).
- `LIST` (detokenises and prints the bundled programme) and `SAVE "name"` (detokenises the bundled programme and writes it through the File I/O ABI).
- `BLOAD "name", addr` loads raw bytes to `addr` through the File I/O MMIO (the same `FILE_NAME_PTR`/`FILE_DATA_PTR`/`FILE_CTRL` path the interpreter uses), with the destination 2^32 range check. `RUN AOT` delegates to the resident handler; standalone bundles it. A standalone image needs a File I/O device mapped in the machine it runs on.

Direct-only commands (`RUN AOT`, `COMPILE`, `DIR`) and roots with no BASIC token (`HOST`, `COSTART`, `COSTOP`, `COWAIT`, `COCALL`, `COSTATUS`) cannot be compiled at all and are reported as such. Every remaining tokenised statement still runs under `RUN AOT` through resident delegation. `POKE`/`POKE8`/`DOKE`/`LOKE` with expression operands (variables or arithmetic) compile under `RUN AOT` by delegating to the resident handler; with integer-literal operands they take a faster inline store.

#### Limitations

- `LOAD` is rejected by a standalone `COMPILE`: it reconstructs a tokenised programme in memory, which needs the resident tokeniser and a REPL loop to run what was loaded, and a standalone image has neither. Use `RUN AOT` or the interpreter for programmes that load other programmes. (`BLOAD`, a raw binary load, is supported in both modes.)
- `POKE`/`POKE8`/`DOKE`/`LOKE` with expression operands compile under `RUN AOT` but not in a standalone `COMPILE` (which has no resident handler to delegate to); use integer-literal operands for standalone images.
- `ELSE` is supported for a single, non-nested `IF` per line. An `IF` whose `THEN` clause contains a second `IF` with its own `ELSE` is not lowered.
- `DO WHILE`/`DO UNTIL` (a top test) is not lowered; use `DO ... LOOP UNTIL`/`LOOP WHILE`.
- `STOP` under `RUN AOT` saves a native continuation and returns to the prompt; a typed `CONT` re-enters the compiled code where it stopped, with variables, `DATA` position, and open `FOR` loops preserved. Editing the programme, `NEW`, `LOAD`, or a fresh `RUN`/`RUN AOT` discards the pending continuation. A `STOP` reached *inside* an active `GOSUB` is not resumable: compiled `GOSUB`/`RETURN` use the hardware return stack, which is unwound when `STOP` returns to the prompt, so `CONT` resumes the post-`STOP` statements but the following `RETURN` reports `?RETURN WITHOUT GOSUB`. Use top-level `STOP`/`CONT`. A standalone `.ie64` has no REPL to return to, so it halts on `STOP` or `END`.
- `SAVE` in a standalone image needs a File I/O device mapped in the machine it runs on; `LIST` and all other bundled statements need no host services.

#### Errors

| Message | Cause |
|---------|-------|
| `Compiling to native code...` | Printed by `RUN AOT` before compilation begins |
| `?COMPILE ERROR IN <line>: <reason>` | A statement could not be compiled (for example a direct-only or non-token root) |
| `?FC ERROR IN 0` | A bad `COMPILE` filename (empty, path-like, absolute, contains `..` or a separator) |
| `?OUT OF MEMORY ERROR IN <line-or-0>: <reason>` | The compiler ran out of arena or the native image is too large |
| `?FILE ERROR IN 0` | The `COMPILE` write failed |

### Terminal Editor

EhBASIC runs inside a graphical terminal with full line-editing support. Text is rendered on the VideoChip framebuffer using a built-in font, with cursor blinking and scrollback history.

#### Editing Keys

| Key | Action |
|-----|--------|
| Left / Right arrow | Move cursor one character |
| Up / Down arrow | Move cursor one line |
| Home | Move cursor to start of line |
| End | Move cursor to end of text on line |
| Backspace | Delete character before cursor |
| Delete | Delete character at cursor |
| Enter | Submit the current line |
| Tab | Advance to next tab stop |

Typing inserts characters at the cursor position - existing text shifts right. Holding any key repeats the action automatically after a short delay.

#### Ctrl Shortcuts

| Shortcut | Action |
|----------|--------|
| Ctrl+A | Move cursor to start of line |
| Ctrl+E | Move cursor to end of line |
| Ctrl+K | Kill (clear) from cursor to end of line |
| Ctrl+U | Kill (clear) from start of input to cursor |
| Ctrl+L | Clear screen |
| Ctrl+Left | Jump one word left |
| Ctrl+Right | Jump one word right |
| Ctrl+Up | Recall previous command from history |
| Ctrl+Down | Recall next command from history |

Ctrl+U preserves the prompt text (e.g. `Ready`) and only clears user input. Ctrl+K works on any line.

#### Text Selection and Clipboard

| Shortcut | Action |
|----------|--------|
| Shift+Left/Right | Extend selection by one character |
| Shift+Up/Down | Extend selection by one line |
| Shift+Home | Extend selection to start of line |
| Shift+End | Extend selection to end of line |
| Ctrl+Shift+C | Copy selected text to clipboard |
| Ctrl+Shift+X | Cut selected text (input line only) |
| Ctrl+Shift+V | Paste from clipboard |
| Middle mouse button | Paste selection (or clipboard if nothing selected) |

Selected text is shown with inverted colours and is cleared on any non-selection input. Selection works on any visible text including scrollback. Selecting text automatically copies it to the OS clipboard. Middle mouse pastes the current selection if present, otherwise falls back to the OS clipboard. Cut removes text only from the current input line; output is read-only.

#### Scrollback and Navigation

| Key / Action | Effect |
|--------------|--------|
| Page Up | Scroll viewport up one page |
| Page Down | Scroll viewport down one page |
| Mouse wheel | Scroll viewport up/down |

The terminal maintains a scrollback buffer. Output that scrolls off the top of the screen is preserved and can be reviewed using Page Up/Down or the mouse wheel. The cursor is not moved by scrollback navigation.

#### Command History

Previously entered commands are saved for the duration of the session. Use Ctrl+Up and Ctrl+Down to cycle through them. When recalling history, the current input is saved and restored when you move past the end of the history list. History is cleared when the terminal is stopped and preserved across resets.

---

## 3. Language Reference

### 3.1 Data Types

EhBASIC IE64 supports two data types:

**Numeric** - IEEE 754 single-precision floating-point (FP32). All numeric values, including integers, are stored as FP32. Integer operations truncate the fractional part where needed. Range: approximately +/-3.4 x 10^38. Precision: ~7 decimal digits.

**String** - Null-terminated byte sequences stored on a string heap. String variables are identified by a trailing `$` suffix. When the heap fills, live string variables and protected expression temporaries are compacted back to the start of the heap; if compaction cannot free enough space, `?OUT OF MEMORY ERROR` is raised.

### 3.2 Variables

Variable names consist of letters (A-Z) and digits (0-9). The first character must be a letter. Names are case-insensitive (internally stored as uppercase). Names up to four characters keep the original packed-name representation; longer names are mixed into the internal tag so names such as `COUNT` and `COUNTER` do not collide.

```basic
X = 42
NAME$ = "ZAYN"
LONGVARIABLENAME = 100
```

**Numeric variables** hold FP32 values. An uninitialised variable returns 0.

**String variables** are indicated by a `$` suffix and hold a pointer to heap-allocated string data. An uninitialised string variable returns an empty string.

**Arrays** are declared with `DIM` and support one or more dimensions. Indices are zero-based:

```basic
DIM A(10)        : REM 11 elements: A(0) through A(10)
DIM B(5, 5)      : REM 6x6 = 36 elements
DIM C(1, 2, 3)   : REM 2x3x4 = 24 elements
```

Referencing an undeclared array auto-creates it with 11 elements per referenced dimension.

### 3.3 Operators

#### Arithmetic Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `+` | Addition (numeric) or concatenation (string) | `3 + 4` = 7 |
| `-` | Subtraction or unary negation | `10 - 3` = 7 |
| `*` | Multiplication | `6 * 7` = 42 |
| `/` | Division | `22 / 7` = 3.142857 |
| `^` | Exponentiation | `2 ^ 10` = 1024 |

#### Comparison Operators

| Operator | Description |
|----------|-------------|
| `=` | Equal to |
| `<` | Less than |
| `>` | Greater than |
| `<=` | Less than or equal to |
| `>=` | Greater than or equal to |
| `<>` | Not equal to |

Numeric and string comparisons return -1 (true) or 0 (false). String
comparisons are lexicographic byte comparisons.

#### Logical/Bitwise Operators

| Operator | Description |
|----------|-------------|
| `AND` | Bitwise AND |
| `OR` | Bitwise OR |
| `EOR` | Bitwise exclusive OR |
| `NOT` | Bitwise complement |
| `<<` | Logical left shift |
| `>>` | Logical right shift |

#### String Operator

| Operator | Description | Example |
|----------|-------------|---------|
| `+` | Concatenation | `"HEL" + "LO"` = `"HELLO"` |

### 3.4 Expression Precedence

From lowest to highest:

1. `OR`, `EOR` (bitwise)
2. `AND` (bitwise)
3. `NOT` (bitwise unary)
4. `=`, `<`, `>` (comparison)
5. `+`, `-`, `<<`, `>>` (addition, subtraction, logical shifts)
6. `*`, `/` (multiplication, division)
7. `^` (exponentiation)
8. `-` (unary negation)
9. Parentheses, function calls, literals, variables

### 3.5 Numeric Literals

```basic
42          : REM integer
3.14159     : REM floating-point
.5          : REM leading dot (0.5)
1.25E3      : REM scientific notation (1250)
&HFF        : REM hexadecimal (255)
```

### 3.6 String Literals

Strings are enclosed in double quotes:

```basic
"Hello, World!"
""              : REM empty string
```

### 3.7 Multiple Statements

A colon (`:`) separates multiple statements on one line:

```basic
10 X = 1 : Y = 2 : PRINT X + Y
```

### 3.8 Comments

The `REM` statement introduces a comment; everything after `REM` to the end of the line is ignored:

```basic
10 REM This is a comment
20 X = 42 : REM inline comment
```

---

## 4. Statement Reference

Statements are grouped by primary keyword. Hardware extension statements (video, audio, system) list their sub-commands under the primary keyword.

### ANTIC

Control the Atari-inspired IE-native ANTIC display-list processor.

```
ANTIC ON                    Enable ANTIC video
ANTIC OFF                   Disable ANTIC video
ANTIC DLIST addr            Set display list address
ANTIC MODE mode             Set DMA control register
ANTIC SCROLL dx, dy         Set hardware scroll offsets
ANTIC CHBASE base           Set character ROM base address
ANTIC PMBASE base           Set player/missile base address
ANTIC NMI mask              Set NMI enable mask
```

**Example:**
```basic
ANTIC ON
ANTIC DLIST &H2400
ANTIC MODE 34
```

### AHX

Control the AHX (Amiga Hybrid eXtensible) music player.

```
AHX PLAY addr [,len]       Play AHX module from memory
AHX PLUS ON                Enable enhanced AHX mode
AHX PLUS OFF               Disable enhanced AHX mode
AHX STOP                   Stop playback
```

### BITCLR

Clear a bit in a byte at a memory address.

```
BITCLR address, bit
```

- `address` - memory address
- `bit` - bit number (0-7)

**Example:**
```basic
BITCLR &HF0700, 3     : REM clear bit 3
```

### BITSET

Set a bit in a byte at a memory address.

```
BITSET address, bit
```

- `address` - memory address
- `bit` - bit number (0-7)

**Example:**
```basic
BITSET &HF0700, 3     : REM set bit 3
```

### BLOAD

Load a binary file directly into memory (raw bytes, no tokenisation).

```
BLOAD "filename", address
```

- `filename` - relative path resolved through the File I/O sandbox. Absolute paths and any path containing `..` are rejected.
- `address` - destination memory address

Unlike `LOAD`, `BLOAD` does **not** clear programme lines or variables. It only reads file bytes into memory. If the read fails, it prints `?FILE NOT FOUND` and returns to BASIC.

**Example:**
```basic
BLOAD "Yummy_Pizza.sid", &H710000
SID PLAY &H710000, 3725
```

### HOST

Dispatch a controlled host-side maintenance command through the host-helper
MMIO bridge. Helper-invoking subcommands are available only when Intuition
Engine is started with `-ehbasic-host`; otherwise those subcommands return
`?FC ERROR`.

```
HOST
HOST HELP
HOST NET
HOST UPDATE
HOST REBOOT
HOST POWEROFF
```

`HOST` and `HOST HELP` print the command summary and do not touch the MMIO
bridge. `HOST NET` starts the host WiFi picker/helper flow. `HOST UPDATE` runs the
privileged update helper; in non-appliance mode it requires operator
confirmation before the helper is invoked. `HOST REBOOT` and `HOST POWEROFF`
ask the helper to reboot or power off the host.

Display builds show a full-screen HOST overlay for long-running helper output.
`HOST NET` and `HOST UPDATE` stream helper output into that overlay with
PageUp/PageDown and mouse-wheel scrollback. When the command reaches a terminal
state, the overlay shows a five-second return-to-BASIC countdown; `Esc`,
`Enter`, or `Space` returns immediately.

The helper reports completion through the host-helper status and exit MMIO
registers. `HOST_STATUS_OK` and `HOST_STATUS_CANCEL` return to BASIC normally.
`HOST_STATUS_ERR` and `HOST_STATUS_DISABLED` raise `?FC ERROR` so BASIC programmes cannot
silently continue after a failed or disabled host operation. `HOST_STATUS_IDLE`
(`5`) is the pre-trigger MMIO state described in [§9.3](#93-host-helper-mmio);
a valid HOST command writes `HOST_MMIO_TRIGGER` before polling. Any unexpected
terminal helper status other than OK or cancel follows the same `?FC ERROR`
path.

`-ehbasic-host-appliance` skips the `HOST UPDATE` confirmation prompt and is
intended only for controlled appliance deployments.

In the x64 live USB image, the root-owned HOST helper broker, host-side
AppArmor, and firewall controls are documented in the live USB security model in
[`README.md`](../../README.md#live-usb-quick-start).

### DIR

List files in the File I/O sandbox. `DIR` is an immediate REPL command and is not stored as a tokenised BASIC statement. Paths are relative to the File I/O sandbox; absolute paths and any path containing `..` are rejected.

```
DIR
DIR "path"
```

Output is sorted one entry per line. Directories have a trailing `/`.

**Example:**
```basic
DIR
DIR "demos"
```

### BLIT

Blitter hardware operations for fast block transfers, fills, and line drawing.

```
BLIT COPY src, dst, w, h [,srcstride, dststride]
BLIT FILL dst, w, h, colour [,stride]
BLIT LINE x1, y1, x2, y2, colour [,stride]
BLIT MODE7 src, dst, dstW, dstH, u0, v0, duCol, dvCol, duRow, dvRow, texW, texH [,srcStride, dstStride]
BLIT MEMCOPY src, dst, len
BLIT M src, dst, len
BLIT WAIT
```

**BLIT COPY** - Copy a rectangular block of pixels from source to destination. Width and height are measured in pixels; strides are measured in bytes.

**BLIT FILL** - Fill a rectangular area with a solid colour.

**BLIT LINE** - Draw a line using the blitter hardware.

**BLIT MODE7** - Affine texture-mapped blit (Mode7). Renders a rotated/scaled texture into a destination rectangle at per-pixel resolution. Texture coordinates use signed 16.16 fixed-point. `texW`/`texH` are power-of-2 masks (for example `255` for a 256x256 texture).

**BLIT MEMCOPY** - Copy a contiguous linear block of bytes. `len` is a byte count, not a pixel count; for a full 640x480 RGBA32 framebuffer use `640*480*4` bytes. `BLIT M` is shorthand for `BLIT MEMCOPY`.

**BLIT WAIT** - Poll until the blitter has finished its current operation (includes timeout).

**Example:**
```basic
REM Fill a 100x50 box at VGA VRAM offset 1000
BLIT FILL &HA0000 + 1000, 100, 50, 15
BLIT WAIT

REM Mode7 rotozoom-style blit into a 640x480 backbuffer
FP=65536: SC=1.0: A=0.25
CA=COS(A)/SC: SA=SIN(A)/SC
DC=INT(CA*FP): DS=INT(SA*FP)
SU=INT((128-320*CA+240*SA)*FP)
SV=INT((128-320*SA-240*CA)*FP)
BLIT MODE7 &H600000, &H900000, 640, 480, SU, SV, DC, DS, 0-DS, DC, 255, 255, 1024, 2560
```

### BOX

Draw a filled rectangle on the VGA display.

```
BOX x1, y1, x2, y2 [,colour]
```

Default colour is 15 (white). Mode 13h only.

**Example:**
```basic
SCREEN &H13
BOX 10, 10, 100, 80, 4     : REM red filled rectangle
```

### CIRCLE

Draw a circle on the VGA display using the midpoint algorithm.

```
CIRCLE x, y, radius [,colour]
```

Default colour is 15 (white). Mode 13h only. Bounds-checked to 320x200.

**Example:**
```basic
SCREEN &H13
CIRCLE 160, 100, 50, 14    : REM yellow circle
```

### CLEAR

Reset all variables, the DATA pointer, and the GOSUB stack.

```
CLEAR
```

Programme text is preserved; only runtime state is cleared.

### CALL

Call a machine code subroutine at the given address. The address is evaluated as an expression.

```
CALL addr
```

The BASIC interpreter saves its internal registers (R14, R16, R17, R26, R28)
before calling the routine and restores them on return. The called routine must
end with `rts`.

**Example:**
```basic
REM poke a simple routine at address 131072 (0x020000)
POKE 131072, 17923 : POKE 131076, 42    : REM moveq r8, #42
POKE 131080, 81    : POKE 131084, 0     : REM rts
CALL 131072
```

See also: [USR](#usr), [Machine Code Interface](#10-machine-code-interface).

### CLS

Clear the screen.

```
CLS [colour]
```

Default colour is 0 (black). Behaviour depends on the active VGA mode:
- Mode 13h: fills 320x200 pixels via blitter
- Mode 12h: fills 38,400 bytes through the current planar aperture and active VGA plane mask
- Text mode: fills with spaces and attribute 0x07

### COLOR

Set the text-mode foreground and background colours.

```
COLOR foreground [,background]
```

Values are 0-15 (standard VGA text attributes).

### CONT

Continue execution after a STOP statement. CONT resumes from the point where the programme was interrupted.

```
CONT
```

After STOP saves the current line and text pointers, CONT restores them and continues execution. If no STOP has occurred, CONT has no effect.

### COPPER

Control the copper coprocessor display list. The copper executes instructions synchronised to the video raster, enabling per-scanline register changes.

```
COPPER LIST addr            Set copper list address (also sets build pointer)
COPPER ON                   Enable copper
COPPER OFF                  Disable copper
COPPER WAIT scanline        Emit WAIT instruction at build pointer
COPPER MOVE addr, value     Emit MOVE instruction at build pointer
COPPER END                  Emit END instruction at build pointer
```

`COPPER MOVE addr, value` takes an absolute register address (>= `&HA0000`). Addresses below `&HA0000` raise `?FC ERROR`. Each MOVE auto-emits a SETBASE instruction (12 bytes total). `COPPER WAIT` emits 4 bytes. `COPPER END` emits 4 bytes.

**Example:**
```basic
COPPER LIST &H50000
COPPER WAIT 100
COPPER MOVE &HF0050, &HFF0000    : REM set raster colour
COPPER END
COPPER ON
```

### COSTART / COSTOP / COWAIT

Control coprocessor worker slots from BASIC. These commands are recognised by
the executor as raw keyword text before normal token dispatch, so they are not
listed in the token table.

```
COSTART cpuType, "serviceFile"
COSTOP cpuType
COWAIT ticket [,timeoutMs]
```

- `cpuType` - 1=IE32, 2=IE64, 3=6502, 4=M68K, 5=Z80, 6=x86
- `serviceFile` - relative path resolved under the coprocessor manager base directory. Absolute paths and any path containing `..` are rejected.
- `ticket` - value returned by `COCALL(...)`
- `timeoutMs` - optional timeout; omitted value defaults to 1000

`COSTART` starts a service binary for the selected CPU type. `COSTOP` stops the
selected worker. `COWAIT` blocks until the ticket completes or the timeout
expires; use `COSTATUS(ticket)` after `COWAIT` to inspect the result.

To inspect worker monitor registers directly, write `COPROC_CPU_TYPE` first,
then read the 32-bit monitor registers:

```basic
POKE &HF2344, 2
PRINT PEEK(&HF23B0)
PRINT PEEK(&HF23B4)
```

### DATA

Define inline data values to be read by `READ`.

```
DATA value [,value ...]
```

Values are numeric literals separated by commas. DATA statements are skipped during normal execution and only consumed by READ.

### DEF FN

Define a single-argument numeric function.

```
DEF FNname(parameter) = expression
```

Function names use the normal variable-name parser after the `FN` prefix.
Definitions are stored in the interpreter's fixed DEF FN table, which has
eight slots. Recursive invocation is rejected at runtime.

**Example:**
```basic
10 DEF FND(X) = X * 2 + 1
20 PRINT FND(3)       : REM prints 7
```

### DEC

Decrement a numeric variable by 1.

```
DEC variable
```

**Example:**
```basic
X = 10
DEC X       : REM X is now 9
```

### DIM

Declare an array with explicit dimensions.

```
DIM name(size) [,name(size) ...]
DIM name(rows, cols) [,name(rows, cols) ...]
DIM name(d0, d1, d2 [, ...]) [,name(size) ...]
```

Indices are zero-based: `DIM A(10)` creates 11 elements (0 through 10).

Multi-dimensional arrays use row-major order: `DIM B(5,3)` creates a 6x4 array, and `DIM C(1,2,3)` creates a 2x3x4 array.

Multiple arrays can be declared on one line, separated by commas.

### DO...LOOP

Loop structure with optional exit condition.

```
DO
  ...statements...
LOOP [WHILE condition | UNTIL condition]
```

- `LOOP` alone: loops unconditionally (use a conditional exit inside)
- `LOOP WHILE cond`: continues looping while condition is true (non-zero)
- `LOOP UNTIL cond`: continues looping until condition becomes true

**Example:**
```basic
10 X = 1
20 DO
30   PRINT X
40   X = X + 1
50 LOOP UNTIL X > 5
```

### DOKE

Write a 16-bit (word) value to a memory address.

```
DOKE address, value
```

**Example:**
```basic
DOKE &H50000, &HFFFF
```

### EMUTOS

Boot the EmuTOS operating system from the BASIC prompt. This performs a full
CPU mode switch from IE64 to M68K and loads the EmuTOS ROM.

```
EMUTOS
```

The ROM is resolved in this order:
1. `-emutos-image <path>` command-line flag
2. Positional command-line ROM filename in direct `-emutos <file>` launches
3. Embedded ROM (if built with `make basic-emutos` or a release build that includes `embed_emutos`)
4. Local `etos256us.img`, `emutos.img`, `bin/etos256us.img`, or `bin/emutos.img`

If no ROM is available, prints `?EMUTOS NOT AVAILABLE`.

F10 performs a hard reset to the VM's original boot mode. If EmuTOS was
launched from BASIC with the `EMUTOS` command, F10 returns to BASIC with the
BASIC terminal video path restored; if the VM was started in EmuTOS mode, F10
boots EmuTOS again.

### AROS

Boot the AROS operating system from the BASIC prompt. This performs a full
CPU mode switch from IE64 to M68K and loads the AROS ROM.

```
AROS
```

The ROM is resolved in this order:
1. `-aros-image <path>` command-line flag
2. Positional command-line ROM filename in direct `-aros <file>` launches
3. Embedded ROM (if built with `make aros` or a release build that includes `embed_aros`)
4. Local `sdk/roms/aros-ie-m68k.rom`

If no ROM is available, prints `?AROS NOT AVAILABLE`.

F10 performs a hard reset to the VM's original boot mode. If AROS was launched
from BASIC with the `AROS` command, F10 returns to BASIC with the BASIC
terminal video path restored; if the VM was started in AROS mode, F10 boots
AROS again.

### INTUITIONOS

Boot IntuitionOS from the BASIC prompt through the programme executor.

```
INTUITIONOS
```

The kernel image is resolved from the IntuitionOS launch configuration: an
explicit `-intuitionos-image`, a live-share `Systems/IntuitionOS/Boot/iexec.ie64`
image, or the development image built by `make intuitionos`.

If no image is available, prints `?INTUITIONOS NOT AVAILABLE`.

### END

Terminate programme execution.

```
END
```

### ENVELOPE

Set the ADSR envelope for a flexible audio channel.

```
ENVELOPE channel, attack, decay, sustain, release
```

- `channel` - 0 to 3
- `attack` - attack time in milliseconds
- `decay` - decay time in milliseconds
- `sustain` - sustain level (0-255)
- `release` - release time in milliseconds

**Example:**
```basic
ENVELOPE 0, 10, 50, 200, 100
```

### FOR...NEXT

Counted loop.

```
FOR variable = initial TO limit [STEP increment]
  ...statements...
NEXT [variable]
```

STEP defaults to 1 if omitted. Negative STEP values count downward. The loop continues while:
- Positive STEP: variable <= limit
- Negative STEP: variable >= limit

**Example:**
```basic
FOR I = 1 TO 10 STEP 2
  PRINT I
NEXT I
```

### GATE

Open or close the audio gate for a flexible channel (triggers the ADSR envelope).

```
GATE channel, ON
GATE channel, OFF
```

**Example:**
```basic
SOUND 0, 440, 200
ENVELOPE 0, 10, 50, 200, 100
GATE 0, ON       : REM start the note
```

### GET

Read a single character from the terminal input buffer without waiting.

```
GET variable
GET string$
```

- Numeric variable: stores the ASCII code (0 if no character available)
- String variable: stores a single-character string ("" if nothing available)

**Example:**
```basic
10 GET K
20 IF K = 0 THEN GOTO 10
30 PRINT "You pressed: "; CHR$(K)
```

### GOSUB...RETURN

Call a subroutine at a specified line number. `RETURN` returns to the statement following the `GOSUB`.

```
GOSUB line_number
...
RETURN
```

Subroutines may be nested.

**Example:**
```basic
10 GOSUB 100
20 PRINT "Back from subroutine"
30 END
100 PRINT "In subroutine"
110 RETURN
```

### GOTO

Unconditional jump to a specified line number.

```
GOTO line_number
```

### GTIA

Control the IE-native GTIA graphics controller.

```
GTIA COLOR index, value        Set colour register (0-4 = playfield, 5-8 = player)
GTIA PRIOR value                Set priority register
GTIA PLAYER num, x [,size]     Set player position and optional size
GTIA MISSILE num, x            Set missile position
GTIA GRAFP num, pattern        Set player graphics pattern
GTIA GRAFM pattern             Set missile graphics pattern
GTIA GRACTL value               Set graphics control register
```

### IF...THEN...ELSE

Conditional execution.

```
IF condition THEN statement [ELSE statement]
IF condition THEN line_number [ELSE statement]
```

The condition is evaluated as a numeric expression; non-zero is true, zero is false.

If THEN is followed by a line number, an implicit GOTO is performed.

**Example:**
```basic
10 INPUT X
20 IF X > 0 THEN PRINT "Positive" ELSE PRINT "Non-positive"
```

### INC

Increment a numeric variable by 1.

```
INC variable
```

**Example:**
```basic
X = 5
INC X       : REM X is now 6
```

### INPUT

Read one or more values from the terminal.

```
INPUT ["prompt";] variable [,variable ...]
INPUT ["prompt",] variable [,variable ...]
```

Each variable reads one terminal line of up to 80 characters. Additional typed
characters are ignored once that per-variable input limit is reached. Numeric
variables parse the input as a numeric expression; string variables store the
entered text. A quoted prompt can be printed before the first value. Either `;`
or `,` may separate the prompt from the first variable.

**Example:**
```basic
10 INPUT "NAME"; N$
20 INPUT "AGE"; A
30 PRINT N$; " "; A
```

### LET

Assign a value to a variable. The `LET` keyword is optional.

```
[LET] variable = expression
[LET] string$ = string_expression
```

**Example:**
```basic
LET X = 42
Y = X * 2          : REM LET is optional
NAME$ = "HELLO"
```

### LINE

Draw a line on the VGA display using the blitter.

```
LINE x1, y1, x2, y2 [,colour]
```

Default colour is 15 (white).

**Example:**
```basic
SCREEN &H13
LINE 0, 0, 319, 199, 12
```

### LIST

Display the stored programme.

```
LIST
```

In a programme line, `LIST` displays the current stored programme and then continues
with the next statement or line.

### LOAD

Load a BASIC programme from a detokenised text file in the File I/O sandbox.

```
LOAD "filename"
```

The filename must be relative; absolute paths and any path containing `..` are
rejected. `LOAD` reads the file as text, clears the current programme and
variables, then stores numbered lines from the file. Lines without a leading
decimal line number are skipped. On read failure it prints `?FILE NOT FOUND`
and returns to BASIC.

### LOCATE

Set the text-mode cursor position.

```
LOCATE row, col
```

Positions are zero-based. Writes cursor position via VGA CRTC cursor registers (indices `0x0E`/`0x0F`).

### LOKE

Write a 32-bit (long) value to a memory address. Identical to POKE.

```
LOKE address, value
```

### LOOP

See [DO...LOOP](#doloop).

### NEW

Clear the programme from memory.

```
NEW
```

In a programme line, `NEW` clears programme text and variables, then stops the
current run.

### NEXT

See [FOR...NEXT](#fornext).

### ON...GOTO / ON...GOSUB

Computed branch. Evaluates the selector expression and jumps to the Nth line number in the list.

```
ON selector GOTO line1, line2, line3 ...
ON selector GOSUB line1, line2, line3 ...
```

The selector is 1-based: `ON 1 GOTO 100,200` jumps to line 100. If the selector is out of range (less than 1 or greater than the number of targets), execution falls through to the next statement.

**Example:**
```basic
10 INPUT C
20 ON C GOTO 100, 200, 300
30 PRINT "Invalid choice"
40 END
100 PRINT "Option 1" : END
200 PRINT "Option 2" : END
300 PRINT "Option 3" : END
```

### PALETTE

Set a VGA DAC palette entry.

```
PALETTE index, red, green, blue
```

- `index` - palette entry (0-255)
- `red`, `green`, `blue` - colour components (0-63, VGA 6-bit DAC)

**Example:**
```basic
PALETTE 1, 63, 0, 0     : REM bright red
```

### PLOT

Plot a single pixel on the VGA display (mode 13h).

```
PLOT x, y [,colour]
```

Default colour is 15 (white). Calculates the VRAM offset as `y*320 + x`.

**Example:**
```basic
SCREEN &H13
FOR I = 0 TO 319
  PLOT I, 100, I AND 255
NEXT I
```

### POKE

Write a value to memory as either 32-bit (`POKE`) or byte (`POKE8`).

```
POKE address, value
POKE8 address, value
```

Both address and value are evaluated as expressions and truncated to integers.
`POKE` stores 32 bits and requires a 4-byte aligned address; unaligned `POKE`
raises `?FC ERROR`. `POKE8` stores the low 8 bits and has no alignment
requirement.

**Example:**
```basic
POKE &HF0700, 65     : REM write 'A' to terminal output
```

### POKEY

Control the Atari POKEY sound chip.

```
POKEY channel, freq, ctrl      Set channel frequency and control (channel 1-4)
POKEY CTRL value               Set master audio control register
POKEY PLUS ON                  Enable POKEY+ enhanced mode
POKEY PLUS OFF                 Disable POKEY+ mode
```

### PRINT

Output values to the terminal.

```
PRINT [expression [; | ,] [expression ...]]
```

- Semicolon (`;`) suppresses the trailing newline and spaces
- Comma (`,`) inserts a TAB character
- `?` is an abbreviation for PRINT
- String expressions are printed as-is; numeric expressions are formatted as decimal

**Example:**
```basic
PRINT "X = "; X
PRINT "A", "B", "C"
? "Quick print"
```

### PSG

Control the PSG (AY-3-8910/YM2149) programmable sound generator. VGM/VGZ data containing SN76489 writes is played natively on the IE bus SN76489 chip; mixed SN + AY VGMs drive both chips.

```
PSG channel, freq, vol           Set channel frequency and volume
PSG PLUS ON                      Enable PSG+ enhanced mode
PSG PLUS OFF                     Disable PSG+ mode
PSG PLAY addr [,len]             Play PSG music data
PSG STOP                         Parsed, but see note below
PSG MIXER value                  Set mixer register (tone/noise control)
PSG ENVELOPE shape [,period]     Set envelope shape and optional period
```

Current source note: `PSG STOP` writes `0` to `PSG_PLAY_CTRL`, while the PSG
player stops only when bit 1 is written. To stop PSG file playback, use
`SOUND STOP`/`SOUND PLAY STOP`, or `POKE &HF0C18, 2`.

### READ

Read the next value from DATA statements.

```
READ variable [,variable ...]
```

Values are read in programme order across all DATA statements. Use RESTORE to reset the pointer.

**Example:**
```basic
10 DATA 10, 20, 30
20 READ A, B, C
30 PRINT A; B; C
```

### REM

Comment. Everything after REM to the end of the line is ignored.

```
REM comment text
```

### RESTORE

Reset the DATA read pointer to the beginning of the programme's DATA statements.

```
RESTORE
```

### RETURN

Return from a GOSUB subroutine. See [GOSUB...RETURN](#gosubreturn).

### RUN

Execute the stored programme or launch an external programme or script.

```
RUN
RUN "file.ext"
```

- `RUN` with no argument executes the stored BASIC programme from the beginning.
- `RUN "file.ext"` loads and launches an external file through the Program Executor. CPU binaries hand off execution to the appropriate CPU emulator; the IE64 CPU stops and the new CPU takes over.
- `RUN "file.ies"` launches an IEScript Lua automation script. See [iescript.md](iescript.md) for the full scripting reference.

The filename is resolved under the Program Executor base directory. Absolute
paths and any path containing `..` are rejected.

Supported extensions:

| Extension | Target |
|-----------|--------|
| `.iex`, `.ie32` | IE32 (custom 32-bit) |
| `.ie64` | IE64 (custom 64-bit RISC) |
| `.ie65` | 6502 (load at `&H0800`) |
| `.ie68` | M68K (Motorola 68020) |
| `.ie80` | Z80 |
| `.ie86` | x86 (32-bit flat) |
| `.tos`, `.img` | EmuTOS image |
| `.ies` | IEScript Lua automation script |

On error (file not found, unsupported extension), prints `?FILE ERROR` and returns to the REPL.

In a programme line, bare `RUN` restarts the stored BASIC programme from the
beginning. Variables are preserved, while DATA and control stacks are reset.

### SAP

Control the SAP (Slight Atari Player) music player.

```
SAP PLAY addr [,len [,subsong]]   Play SAP module
SAP STOP                          Stop playback
```

### SAVE

Save the current tokenised BASIC programme as detokenised text through the File
I/O sandbox.

```
SAVE "filename"
```

The filename must be relative; absolute paths and any path containing `..` are
rejected. `SAVE` writes one numbered source line per output line. On write
failure it raises `?FILE ERROR`.

### SCREEN

Set the VGA video mode.

```
SCREEN mode        Set VGA mode and enable display
SCREEN ON          Enable VGA display
SCREEN OFF         Disable VGA display
```

Supported modes:
- `&H13` - 320x200, 256 colours (Mode 13h)
- `&H12` - 640x480, 16 colours (Mode 12h, planar)
- `3` - 80x25 text mode

**Example:**
```basic
SCREEN &H13
CLS
PLOT 160, 100, 15
```

### SCROLL

Hardware scroll the VGA display.

```
SCROLL dx, dy
```

Sets the VGA CRTC start address offset for smooth scrolling.

### SID

Control the SID (MOS 6581/8580) sound chip.

```
SID VOICE num, freq, pw, ctrl, ad, sr   Programme a voice (num 1-3)
SID VOLUME vol                           Set master volume (0-15)
SID FILTER cutoff, resonance, routing, mode   Configure filter
SID PLUS ON                              Enable SID+ enhanced mode
SID PLUS OFF                             Disable SID+ mode
SID PLAY addr [,len [,subsong]]          Play SID music data
SID STOP                                 Stop playback
```

**SID VOICE** parameters:
- `num` - voice number (1-3)
- `freq` - 16-bit frequency value
- `pw` - 12-bit pulse width
- `ctrl` - control register (gate, waveform select, sync, ring mod)
- `ad` - attack/decay (upper nibble = attack, lower = decay)
- `sr` - sustain/release (upper nibble = sustain, lower = release)

### SOUND

Configure a flexible audio channel.

```
SOUND channel, freq, vol [,wave [,duty]]
SOUND FILTER cutoff, resonance, type
SOUND FILTER MOD source, amount
```

- `channel` - 0 to 3
- `freq` - frequency in Hz
- `vol` - volume (0-255)
- `wave` - waveform type (0=square, 1=triangle, 2=sine, 3=noise, 4=sawtooth)
- `duty` - duty cycle for square wave (0-255, 128=50%)

**SOUND FILTER** sets the global audio filter:
- `cutoff` - cutoff frequency (0-255, exponential 20Hz-20kHz)
- `resonance` - resonance (0-255)
- `type` - filter type (0=off, 1=lowpass, 2=highpass, 3=bandpass)

**SOUND FILTER MOD** sets filter cutoff modulation:
- `source` - modulation source channel (0-3)
- `amount` - modulation depth (0-255)

**SOUND REVERB** configures the reverb effect:
```
SOUND REVERB mix, decay
```
- `mix` - dry/wet mix (0-255)
- `decay` - decay time (0-255)

**SOUND OVERDRIVE** sets the overdrive (distortion) amount:
```
SOUND OVERDRIVE amount
```
- `amount` - drive amount (0-255)

**SOUND NOISE** sets the fixed noise-channel mode:
```
SOUND NOISE channel, mode
```
- `channel` - parsed by the current handler but not used for register
  selection; the handler writes the fixed `NOISE_MODE` register at `&HF09E0`
- `mode` - 0=white, 1=periodic, 2=metallic, 3=PSG-style, 4=TED 8-bit,
  5=SN76489 15-bit white, 6=SN76489 15-bit periodic, 7=SN76489 16-bit white,
  8=SN76489 16-bit periodic

Use direct `POKE` to a flexible channel's `+&H2C` `NOISEMODE` register when a
per-flex-channel noise mode is required.

**SOUND WAVE** sets the waveform type for a flexible channel:
```
SOUND WAVE channel, type
```
- `channel` - 0 to 3
- `type` - waveform type

**SOUND SWEEP** configures a pitch sweep on a flexible channel:
```
SOUND SWEEP channel, enable, period, shift
```
- `enable` - 1=on, 0=off
- `period` - sweep period
- `shift` - sweep shift amount

Packed as `enable | (period << 8) | (shift << 16)` in the FLEX_OFF_SWEEP register.

**SOUND SYNC** sets the legacy hard-sync source register for a channel:
```
SOUND SYNC channel, source
```
- `channel` - 0 to 3
- `source` - source channel number

The BASIC handler writes `SYNC_SOURCE_CH0 + channel*4`; it does not write the
flexible channel `+&H38` `SYNC` offset. Direct flex-register sync uses bit 7 as
the enable bit and bits 0-3 as the source channel.

**SOUND RINGMOD** sets the legacy ring-modulation source register for a channel:
```
SOUND RINGMOD channel, source
```
- `channel` - 0 to 3
- `source` - source channel number

The BASIC handler writes `RING_MOD_SOURCE_CH0 + channel*4`; it does not write
the flexible channel `+&H34` `RINGMOD` offset. Direct flex-register ring
modulation uses bit 7 as the enable bit and bits 0-3 as the source channel.

**SOUND MOD** controls the ProTracker MOD player:
```
SOUND MOD PLAY addr [,len]
SOUND MOD PLAY STOP
SOUND MOD STOP
SOUND MOD FILTER model
```
- `addr` - bus-memory address of MOD data
- `len` - optional data length; if omitted, the existing `MOD_PLAY_LEN` register value is used
- `model` - filter model: 0=none, 1=A500, 2=A1200

**SOUND PLAY** loads and plays a music file through the appropriate audio engine:
```
SOUND PLAY "filename.ext" [,subsong]
SOUND PLAY STOP
SOUND STOP
```
- `filename` - path to a music file resolved by the Media Loader sandbox. Relative paths resolve under the runtime base directory; absolute paths are accepted only when they remain inside that base directory.
- `subsong` - optional subsong index (default 0, used by SID, AHX, and SAP)

Supported formats are exactly those routed by `detectMediaType` in the Media
Loader:

| Extension | Engine |
|-----------|--------|
| `.sid` | SID (Commodore 64) |
| `.ym` | PSG/YM2149 frame dump |
| `.ay` | PSG/AY file; ZXAYEMUL Z80 files are executed through the AY Z80 player |
| `.vgm`, `.vgz` | VGM/VGZ; AY/YM and SN76489 streams are handled by the PSG player |
| `.vtx`, `.vt` | VTX compressed PSG stream |
| `.sndh`, `.snd` | Atari ST SNDH |
| `.pt3`, `.pt2`, `.pt1`, `.stc`, `.sqt`, `.asc`, `.ftc` | Z80 PSG tracker formats; these require the filename extension for `SOUND PLAY` routing |
| `.ted`, `.prg` | TED (Commodore 16|Plus/4) |
| `.ahx` | AHX (Amiga tracker) |
| `.sap` | POKEY/SAP |
| `.mod` | MOD (ProTracker) |
| `.wav` | WAV |
| `.mid`, `.midi`, `.mus` | MIDI/MUS through SoundChip RawlandMini |

The direct `PSG PLAY addr [,len]` MMIO path receives only a memory pointer and
length, so it can load formats that the PSG player can identify from their data
headers. Use `SOUND PLAY "file.ext"` for extension-only tracker formats such as
PT1/PT2/PT3, STC, SQT, ASC, and FTC.

`SOUND PLAY STOP` stops Media Loader/player playback.

`SOUND STOP` is a shorthand alias for `SOUND PLAY STOP`.

On an error that is visible during the command's bounded status poll, such as
file not found, path rejection, unsupported extension, bad format, or an
oversized staging-backed file, the command raises `?PLAY ERROR`.

**Example:**
```basic
SOUND 0, 440, 200, 0, 128    : REM 440Hz square wave, 50% duty
ENVELOPE 0, 10, 50, 200, 100
GATE 0, ON
SOUND REVERB 100, 200        : REM add reverb
SOUND PLAY "music.sid"       : REM play a SID tune
SOUND PLAY "music.sid", 3    : REM play subsong 3
SOUND PLAY STOP              : REM stop media playback
SOUND STOP                   : REM shorthand stop
```

### STOP

Halt programme execution. Saves the current line and text pointers so that CONT can resume execution from this point.

```
STOP
```

See also: [CONT](#cont).

### SWAP

Exchange the values of two numeric variables.

```
SWAP variable1, variable2
```

**Example:**
```basic
A = 10 : B = 20
SWAP A, B
PRINT A; B     : REM prints 20 10
```

### TRON

Enable trace mode. When trace is active, the interpreter prints the current line number in square brackets before executing each line.

```
TRON
```

**Example:**
```basic
10 TRON
20 PRINT "Hello"
30 TROFF
RUN
```

Output:
```
[20]Hello
```

See also: [TROFF](#troff).

### TROFF

Disable trace mode.

```
TROFF
```

See also: [TRON](#tron).

### TED

Control the TED (Commodore 16|Plus/4) video and audio hardware.

**Video sub-commands:**
```
TED ON                       Enable TED video
TED OFF                      Disable TED video
TED MODE mode                Set video mode (0=text, 1=bitmap, 2=multicolour)
TED COLOR bg [,border]       Set background and border colours
TED CHAR base                Set character ROM base address
TED VIDEO base               Set video matrix base address
TED CLS                      Clear TED screen
TED SCROLL dx, dy            Set hardware scroll offsets
```

**Audio sub-commands:**
```
TED TONE channel, freq       Set tone generator (channel 1-2)
TED VOL vol                  Set volume (0-15)
TED NOISE ON                 Enable noise generator
TED NOISE OFF                Disable noise generator
TED PLUS ON                  Enable TED+ enhanced mode
TED PLUS OFF                 Disable TED+ mode
TED PLAY addr [,len]         Play TED music data
TED STOP                     Parsed, but see note below
```

Current source note: `TED STOP` writes `0` to `TED_PLAY_CTRL`, while the TED
player stops only when bit 1 is written. To stop TED file playback, use
`SOUND STOP`/`SOUND PLAY STOP`, or `POKE &HF0F18, 2`.

### TEXTURE

Control Voodoo texture mapping. See [Voodoo 3D Programming](#67-voodoo-3d-programming).

```
TEXTURE ON                   Enable texturing
TEXTURE OFF                  Disable texturing
TEXTURE MODE mode            Set texture mode register
TEXTURE BASE lod, addr       Set texture base for LOD level
TEXTURE DIM w, h             Set texture dimensions
TEXTURE UPLOAD               Trigger texture upload
```

### TRIANGLE

Submit the current vertices A, B, C to the Voodoo rasteriser. See [Voodoo 3D Programming](#67-voodoo-3d-programming).

```
TRIANGLE
```

### ULA

Control the ULA (ZX Spectrum) video display.

```
ULA ON                       Enable ULA video
ULA OFF                      Disable ULA video
ULA BORDER colour            Set border colour (0-7)
ULA BRIGHT flag              Set bright attribute (0 or 1)
ULA INK colour               Set ink (foreground) colour (0-7)
ULA PAPER colour             Set paper (background) colour (0-7)
ULA FLASH flag               Set flash attribute (0 or 1)
ULA PLOT x, y                Plot pixel using ZX Spectrum VRAM addressing
ULA CLS [attr]               Clear screen (default attr 0x38)
ULA ATTR col, row, attr      Set character cell attribute
```

**Example:**
```basic
ULA ON
ULA BORDER 1
ULA INK 7 : ULA PAPER 0
ULA CLS
ULA PLOT 128, 96
```

### VERTEX

Define a Voodoo triangle vertex. See [Voodoo 3D Programming](#67-voodoo-3d-programming).

```
VERTEX A x, y               Set vertex A coordinates
VERTEX B x, y               Set vertex B coordinates
VERTEX C x, y               Set vertex C coordinates
```

Coordinates are converted to 12.4 fixed-point format internally.

### VOODOO

Control the Voodoo 3DFX graphics accelerator. See [Voodoo 3D Programming](#67-voodoo-3d-programming).

```
VOODOO ON                    Enable Voodoo
VOODOO OFF                   Disable Voodoo
VOODOO CLEAR [colour]        Clear framebuffer (default black)
VOODOO SWAP                  Swap display buffers
VOODOO DIM w, h              Set framebuffer dimensions
VOODOO CLIP l, t, r, b       Set clip rectangle
VOODOO COMBINE value         Set colour path blend mode
VOODOO LFB mode              Set linear framebuffer mode
VOODOO PIXEL x, y, colour   Write pixel to linear framebuffer
VOODOO TRICOLOR r, g, b [,a] Set start colour (and optional alpha)
VOODOO TRISHADE drdx, drdy, dgdx, dgdy, dbdx, dbdy
VOODOO TRIDEPTH start_z, dzdx, dzdy
VOODOO TRIUV start_s, start_t, dsdx, dtdx, dsdy, dtdy
VOODOO TRIW start_w, dwdx, dwdy
VOODOO ALPHA TEST ON/OFF     Enable/disable alpha testing
VOODOO ALPHA FUNC n          Set alpha test function (0-7)
VOODOO ALPHA BLEND ON/OFF    Enable/disable alpha blending
VOODOO ALPHA SRC n            Set source blend factor (0-15)
VOODOO ALPHA DST n            Set destination blend factor (0-15)
VOODOO FOG ON/OFF            Enable/disable fog
VOODOO FOG COLOR r, g, b     Set fog colour
VOODOO DITHER ON/OFF         Enable/disable dithering
VOODOO CHROMAKEY ON/OFF       Enable/disable chroma keying
VOODOO CHROMAKEY COLOR r,g,b  Set chroma key colour
VOODOO RGB ON/OFF             Enable/disable RGB buffer writes
```

**VOODOO TRICOLOR** sets the start colour registers (START_R, START_G, START_B, optionally START_A) used by the triangle rasteriser:
```basic
VOODOO TRICOLOR 255, 0, 0         : REM solid red
VOODOO TRICOLOR 128, 128, 0, 200  : REM with alpha
```

**VOODOO TRI\*** sub-commands set interpolation state for triangle rasterisation:
- `TRISHADE` writes colour gradients `DRDX`, `DRDY`, `DGDX`, `DGDY`, `DBDX`, and `DBDY`
- `TRIDEPTH` writes `START_Z`, `DZDX`, and `DZDY`
- `TRIUV` writes `START_S`, `START_T`, `DSDX`, `DTDX`, `DSDY`, and `DTDY`
- `TRIW` writes `START_W`, `DWDX`, and `DWDY`

**VOODOO ALPHA** sub-commands control alpha testing and blending in the ALPHA_MODE register:
- `ALPHA TEST ON/OFF` toggles bit 0 (ALPHA_TEST_EN)
- `ALPHA FUNC n` sets bits 1-3 (comparison function: 0=never, 1=less, 2=equal, 3=lequal, 4=greater, 5=notequal, 6=gequal, 7=always)
- `ALPHA BLEND ON/OFF` toggles bit 4 (ALPHA_BLEND_EN)
- `ALPHA SRC n` sets bits 8-11 (source RGB blend factor)
- `ALPHA DST n` sets bits 12-15 (destination RGB blend factor)

**VOODOO FOG** controls fog rendering via the FOG_MODE register:
- `FOG ON/OFF` toggles the FOG_ENABLE bit
- `FOG COLOR r,g,b` packs `(b<<16)|(g<<8)|r` into the FOG_COLOR register

**VOODOO DITHER** toggles the FBZ_DITHER bit (0x0100) in the FBZ_MODE register.

**VOODOO CHROMAKEY** controls chroma-key transparency via FBZ_MODE:
- `CHROMAKEY ON/OFF` toggles the FBZ_CHROMAKEY bit
- `CHROMAKEY COLOR r,g,b` packs `(b<<16)|(g<<8)|r` into the CHROMA_KEY register

**VOODOO RGB** toggles the FBZ_RGB_WRITE bit (0x0200) in the FBZ_MODE register. When disabled, triangle rasterisation computes colours but does not write them to the framebuffer (useful for depth-only passes).

### VSYNC

Wait for the VGA vertical blanking interval.

```
VSYNC
```

Polls the VGA status register until the vsync flag is set. Includes a timeout to prevent infinite loops.

### WAIT

Poll a memory address until a bit condition is met.

```
WAIT address, mask [,xor]
```

Logic: loops while `(PEEK(address) XOR xor) AND mask = 0`. The `xor` parameter defaults to 0 if omitted. Includes a timeout.

**Example:**
```basic
WAIT &HF1004, 1      : REM wait for VGA vsync bit
```

### WHILE...WEND

Conditional loop.

```
WHILE condition
  ...statements...
WEND
```

The condition is evaluated at the top of each iteration. If true (non-zero), the loop body executes. If false, execution skips to after WEND. Nested WHILE/WEND blocks are supported.

**Example:**
```basic
10 X = 1
20 WHILE X < 6
30   PRINT X
40   X = X + 1
50 WEND
```

### ZBUFFER

Control the Voodoo depth buffer. See [Voodoo 3D Programming](#67-voodoo-3d-programming).

```
ZBUFFER ON                   Enable depth testing
ZBUFFER OFF                  Disable depth testing
ZBUFFER FUNC func            Set depth comparison function
ZBUFFER WRITE ON             Enable depth buffer writes
ZBUFFER WRITE OFF            Disable depth buffer writes
```

---

## 5. Function Reference

Functions are listed alphabetically. All functions return FP32 numeric values unless noted as returning strings.

### ABS

```
ABS(x)
```

Returns the absolute value of `x`.

### ASC

```
ASC(string$)
```

Returns the ASCII code of the first character of the string. Returns 0 for an empty string.

### ATN

```
ATN(x)
```

Returns the arctangent of `x` in radians. Range: -PI/2 to PI/2.

### BITTST

```
BITTST(address, bit)
```

Tests bit `bit` (0-7) in the byte at `address`. Returns -1 (true) if the bit is set, 0 (false) if clear.

### BIN$

```
BIN$(n)
```

Returns a string containing the binary representation of the integer value `n`. (String function.)

**Example:**
```basic
PRINT BIN$(5)       : REM prints "101"
PRINT BIN$(255)     : REM prints "11111111"
```

### CHR$

```
CHR$(code)
```

Returns a single-character string with the given ASCII code. (String function.)

**Example:**
```basic
PRINT CHR$(65)     : REM prints "A"
```

### COCALL

```
COCALL(cpuType, op, reqPtr, reqLen, respPtr, respCap)
```

Enqueues an asynchronous request to a running coprocessor worker and returns a
ticket number. A return value of 0 means the request could not be enqueued.

- `cpuType` - 1=IE32, 2=IE64, 3=6502, 4=M68K, 5=Z80, 6=x86
- `op` - service-defined operation code
- `reqPtr`, `reqLen` - request buffer address and byte length
- `respPtr`, `respCap` - response buffer address and byte capacity

### COS

```
COS(x)
```

Returns the cosine of `x` (in radians).

### COSTATUS

```
COSTATUS(ticket)
```

Returns the status of a coprocessor request ticket:

| Value | Meaning |
|-------|---------|
| 0 | Pending |
| 1 | Running |
| 2 | Completed successfully |
| 3 | Error |
| 4 | Timeout |
| 5 | Worker down |

See also: [COCALL](#cocall), [COSTART / COSTOP / COWAIT](#costart--costop--cowait).

### DEEK

```
DEEK(address)
```

Reads a 16-bit (word) value from the given memory address. Compare with PEEK (32-bit) and LEEK (32-bit).

### EXP

```
EXP(x)
```

Returns e raised to the power `x`.

### FRE

```
FRE(x)
```

Returns the approximate number of free bytes available for variables and strings. The argument is evaluated but ignored (conventionally `FRE(0)`).

### HEX$

```
HEX$(n)
```

Returns a string containing the hexadecimal representation of the integer value `n`. (String function.)

**Example:**
```basic
PRINT HEX$(255)     : REM prints "FF"
PRINT HEX$(4096)    : REM prints "1000"
```

### INT

```
INT(x)
```

Truncates `x` to an integer (towards zero). Returns the result as FP32.

**Example:**
```basic
PRINT INT(3.7)      : REM prints 3
PRINT INT(-3.7)     : REM prints -3
```

### LEEK

```
LEEK(address)
```

Reads a 32-bit (long) value from the given memory address. Identical to PEEK.

### LEFT$

```
LEFT$(string$, n)
```

Returns the leftmost `n` characters of the string. (String function.)

**Example:**
```basic
PRINT LEFT$("HELLO", 3)     : REM prints "HEL"
```

### LEN

```
LEN(string$)
```

Returns the length of the string in bytes.

### LOG

```
LOG(x)
```

Returns the natural logarithm (base e) of `x`. Domain: `x > 0`.

### MAX

```
MAX(a, b)
```

Returns the larger of the two values.

**Example:**
```basic
PRINT MAX(3, 7)     : REM prints 7
```

### MID$

```
MID$(string$, start, length)
```

Returns a substring of `length` characters starting at position `start` (1-based). All three arguments are required. (String function.)

**Example:**
```basic
PRINT MID$("HELLO", 2, 3)    : REM prints "ELL"
```

### MIN

```
MIN(a, b)
```

Returns the smaller of the two values.

### PEEK

```
PEEK(address)
PEEK8(address)
```

`PEEK` reads a 32-bit value from the given memory address and requires a
4-byte aligned address; unaligned `PEEK` raises `?FC ERROR`. `PEEK8` reads one
byte and returns it as a value from 0 to 255.

**Example:**
```basic
PRINT PEEK(&HF1000)     : REM read VGA mode register
```

### PI

```
PI
```

Returns the constant pi (approximately 3.1415927). No parentheses required.

### POS

```
POS(x)
```

Returns the current terminal output column. The argument is evaluated but
ignored.

### RIGHT$

```
RIGHT$(string$, n)
```

Returns the rightmost `n` characters of the string. (String function.)

### RND

```
RND(x)
```

Returns a pseudo-random number in the range [0, 1). Uses a linear congruential generator. The argument is evaluated but ignored (conventionally `RND(1)`).

**Example:**
```basic
PRINT INT(RND(1) * 100)  : REM random integer 0-99
```

### SADD

```
SADD(stringVariable$)
```

Returns the address of the string data for a string variable. If the argument
is not a string variable, it returns 0.

### SGN

```
SGN(x)
```

Returns the sign of `x`: -1 (negative), 0 (zero), or 1 (positive).

### SIN

```
SIN(x)
```

Returns the sine of `x` (in radians).

### SQR

```
SQR(x)
```

Returns the square root of `x`. Domain: `x >= 0`. Returns 0 for negative inputs.

### STATUS (Chip)

Query the playback status of a sound chip or tracker engine.

```
PSG STATUS
SID STATUS
POKEY STATUS
TED STATUS
AHX STATUS
SAP STATUS
MOD STATUS
```

These are two-token expressions: write the chip keyword followed by `STATUS`,
with no parentheses, wherever a numeric expression is accepted.

Used as an expression (function context). `PSG STATUS`, `SID STATUS`,
`POKEY STATUS`, `TED STATUS`, `AHX STATUS`, and `SAP STATUS` return the raw
player status register value as FP32. In the current register layout, bit 0 is
busy/playing and bit 1 is error. `POKEY STATUS` reads the shared SAP player
status register because POKEY file playback is SAP-backed. `MOD STATUS` returns
only bit 0 of `MOD_PLAY_STATUS` as 0 or 1.

**Example:**
```basic
SID PLAY &H30000, 8192
10 IF SID STATUS = 1 THEN GOTO 10
PRINT "Playback finished"
```

### STR$

```
STR$(n)
```

Returns a string representation of the numeric value `n`. (String function.)

### TAN

```
TAN(x)
```

Returns the tangent of `x` (in radians).

### TWOPI

```
TWOPI
```

Returns the constant 2*pi (approximately 6.2831855). No parentheses required.

### UCASE$ / LCASE$

```
UCASE$(string$)
LCASE$(string$)
```

Return an upper-case or lower-case copy of the input string. Non-letter bytes
are copied unchanged.

### USR

```
USR(n)
```

Calls a user-defined machine code routine at address `n` and returns the result. The address is evaluated, the routine is called via register-indirect JSR, and the value of R8 on return is converted to FP32 and returned as the function result.

`USR` preserves R14, R16, R17, and R26 across the machine-code call. Unlike
`CALL`, it does not preserve R28.

**Example:**
```basic
REM poke moveq r8, #42 / rts at address 131072
POKE 131072, 17923 : POKE 131076, 42
POKE 131080, 81    : POKE 131084, 0
PRINT USR(131072)   : REM prints 42
```

See also: [CALL](#call), [Machine Code Interface](#10-machine-code-interface).

### VAL

```
VAL(string$)
```

Parses the string as a numeric expression and returns the result. Returns 0 if the string is not a valid number.

**Example:**
```basic
PRINT VAL("42")     : REM prints 42
```

### VARPTR

```
VARPTR(variable)
```

Returns the address of the variable storage slot. For a string variable, the
slot contains the pointer to the string data.

---

## 6. Video Programming Guide

The Intuition Engine provides multiple video subsystems, all accessible from BASIC. Each subsystem has its own display buffer, resolution, and colour model.

### 6.1 VGA Modes and VRAM Layout

The VGA subsystem supports three modes:

| Mode | Resolution | Colours | VRAM Address | Size |
|------|-----------|---------|-------------|------|
| `&H13` | 320x200 | 256 (palette) | `&HA0000` | 64,000 bytes |
| `&H12` | 640x480 | 16 (planar) | `&HA0000` | 64 KB planar aperture |
| `3` | 80x25 text | 16 fg + 8 bg | `&HB8000` | 4,000 bytes |

**Mode 13h** (320x200x256) is the simplest: each byte at `&HA0000 + y*320 + x` is a palette index. This is the default mode for PLOT, LINE, BOX, and CIRCLE.

**Mode 12h** (640x480x16) uses four bitplanes. Each plane contains 38,400
bytes, but BASIC sees the 64 KB VGA aperture at `&HA0000`, not a flat
153,600-byte framebuffer. Select writable planes through the VGA sequencer map
mask (`VGA_SEQ_MAPMASK`, or `VGA_SEQ_INDEX`=`2` followed by `VGA_SEQ_DATA`) or
use the blitter for drawing operations.

**Text Mode** (80x25) stores character/attribute pairs at `&HB8000`. Each cell is 2 bytes: character code followed by attribute byte (bits 0-3 = foreground, bits 4-6 = background, bit 7 = blink).

```basic
SCREEN &H13              : REM switch to mode 13h
CLS                      : REM clear screen
PLOT 160, 100, 15        : REM white pixel at centre
VSYNC                    : REM wait for vertical blank
```

### 6.2 Copper Coprocessor Programming

The copper is a display-list coprocessor that executes instructions synchronised to the video raster. It can modify any hardware register at specific scanline positions, enabling raster bars, split-screen effects, and per-line palette changes.

**Copper instructions** are 32-bit words:

| Instruction | Format | Description |
|-------------|--------|-------------|
| WAIT | bits 31:30=00, scanline in bits 23:12, X in bits 11:0 | Wait for raster position (BASIC sets X=0) |
| MOVE | opcode word (bits 31:30=01, reg index 23:16) + 32-bit data word | Write value to register |
| SETBASE | `0x80000000 \| (addr >> 2)` (bits 31:30=10) | Set I/O base for subsequent MOVEs |
| END | `0xC0000000` (bits 31:30=11) | End of copper list |

**Example: Rainbow bars**
```basic
10 SCREEN &H13
20 COPPER LIST &H50000
30 FOR I = 0 TO 199
40   COPPER WAIT I
50   COPPER MOVE &HF0050, I
60 NEXT I
70 COPPER END
80 COPPER ON
```

### 6.3 Blitter Operations

The blitter performs hardware-accelerated block operations: copy, fill, line drawing, alpha, Mode7, colour expansion, nearest-neighbour scaling, and byte-counted memory copy. Rectangular operations use source/destination addresses, width, height, and stride. `BLIT MEMCOPY` uses source, destination, and a byte length. Scale blits use `BLT_WIDTH`/`BLT_HEIGHT` as the source size and pack the destination size in `BLT_COLOR` as `(height << 16) | width`.

```basic
REM Copy 100x50 block
BLIT COPY &HA0000, &HA0000 + 32000, 100, 50

REM Fill rectangle with colour 4
BLIT FILL &HA0000 + 1000, 200, 100, 4

REM Draw a line
BLIT LINE 0, 0, 319, 199, 15, 320

REM Wait for completion
BLIT WAIT

REM Copy a full 640x480 RGBA32 backbuffer into the front buffer
BLIT MEMCOPY &H230000, &H100000, 640 * 480 * 4

REM Mode7 affine texture map (256x256 texture, 16.16 fixed-point UVs)
FP=65536
BLIT MODE7 &H600000, &H900000, 640, 480, 0, 0, FP, 0, 0, FP, 255, 255, 1024, 2560
```

### 6.4 ULA (ZX Spectrum) Programming

The ULA emulates the ZX Spectrum's 256x192 pixel display with a unique non-linear VRAM layout.

**VRAM layout** at IE aperture `&HFA000`:
- Bitmap: 6,144 bytes (three 2KB thirds, each with interleaved character rows)
- Attributes: 768 bytes at `&HFA000 + &H1800` (32x24 cells, each 8x8 pixels)

The `&H4000` bitmap and `&H5800` attribute addresses are ZX Spectrum guest
addresses. From EhBASIC on IE64, PEEK/POKE the `&HFA000` aperture or use the
ULA commands.

**Attribute byte format**: bits 0-2 = ink, bits 3-5 = paper, bit 6 = bright, bit 7 = flash.

```basic
ULA ON
ULA BORDER 1              : REM blue border
ULA CLS &H38              : REM white paper, black ink
ULA INK 2 : ULA PAPER 7   : REM set default colours
ULA PLOT 128, 96           : REM centre pixel
ULA ATTR 16, 12, &H46     : REM bright red on black
```

### 6.5 TED Video Programming

The TED emulates the Commodore 16|Plus/4 video chip with 40x25 text mode and 121 colours (16 hues x 8 luminances, minus duplicates).

```basic
TED ON
TED MODE 0                 : REM text mode
TED COLOR 0, 0             : REM black background and border
TED VIDEO &H50000          : REM set video matrix address
TED CHAR &H51000           : REM set character ROM address
TED CLS
```

### 6.6 ANTIC/GTIA Programming

ANTIC and GTIA together form an Atari-inspired IE-native video system. ANTIC handles display list execution and character/bitmap modes; GTIA handles colour registers, player/missile graphics, priority, and collisions.

```basic
ANTIC ON
ANTIC DLIST &H2400         : REM point to display list
ANTIC MODE 34              : REM standard playfield DMA
GTIA COLOR 0, &H28         : REM set playfield colour 0
GTIA COLOR 4, &H00         : REM background colour
GTIA PLAYER 0, 128, 0      : REM position player 0 at centre
GTIA GRAFP 0, &HFF         : REM solid 8-pixel player pattern
```

### 6.7 Voodoo 3D Programming

The Voodoo subsystem emulates 3Dfx Voodoo-style 3D rasterisation: per-triangle rendering with Gouraud shading, Z-buffering, texture mapping, alpha blending, and fog.

**Basic workflow:**
1. Enable Voodoo and set dimensions
2. Clear the framebuffer
3. Define vertices A, B, C
4. Submit the triangle
5. Swap buffers

```basic
10 VOODOO ON
20 VOODOO DIM 320, 200
30 VOODOO CLEAR 0
40 VERTEX A 160, 20
50 VERTEX B 60, 180
60 VERTEX C 260, 180
70 TRIANGLE
80 VOODOO SWAP
```

**Z-Buffer (depth testing):**
```basic
ZBUFFER ON                  : REM enable depth test
ZBUFFER FUNC 1              : REM less-than comparison
ZBUFFER WRITE ON            : REM write depth values
```

**Texture mapping:**
```basic
TEXTURE DIM 64, 64          : REM 64x64 texture
TEXTURE BASE 0, &H100000   : REM LOD 0 at VRAM address
TEXTURE ON                  : REM enable texturing
```

**Linear framebuffer (direct pixel access):**
```basic
VOODOO LFB 1                : REM enable LFB mode
VOODOO PIXEL 160, 100, &HFF0000  : REM red pixel at centre
```

---

## 7. Audio Programming Guide

The Intuition Engine features multiple audio subsystems, each emulating a different era and style of sound chip.

### 7.1 SoundChip (Flexible 4-Channel Synthesiser)

The main audio engine provides four flexible channels addressed by `SOUND`
channel numbers 0-3, plus fixed square, triangle, sine, noise, and sawtooth
paths used by lower-level registers. Flexible channels support selectable
waveforms, ADSR envelopes, duty/PWM control, sweep, sync, ring modulation,
noise mode, phase reset, and DAC writes. Global effects include filter,
overdrive, and reverb.

**Waveform types for flexible channels:**
| Value | Waveform |
|-------|----------|
| 0 | Square |
| 1 | Triangle |
| 2 | Sine |
| 3 | Noise |
| 4 | Sawtooth |

**Basic sound production:**
```basic
SOUND 0, 440, 200, 0, 128   : REM channel 0: 440Hz square, 50% duty
ENVELOPE 0, 10, 50, 200, 100 : REM 10ms attack, 50ms decay, sustain 200, 100ms release
GATE 0, ON                   : REM start the note
```

**Polyphonic chords:**
```basic
REM C major chord
SOUND 0, 262, 180, 2     : REM C4 sine
SOUND 1, 330, 180, 2     : REM E4 sine
SOUND 2, 392, 180, 2     : REM G4 sine
FOR I = 0 TO 2
  ENVELOPE I, 20, 100, 150, 200
  GATE I, ON
NEXT I
```

**Audio filter:**
```basic
SOUND FILTER 128, 200, 1  : REM lowpass at mid cutoff with high resonance
```

### 7.2 PSG (AY-3-8910/YM2149)

The PSG emulates the General Instrument AY-3-8910 / Yamaha YM2149 programmable sound generator, widely used in the ZX Spectrum 128, Amstrad CPC, and MSX.

Three square-wave tone channels plus one noise generator, with envelope control and a mixer register for tone/noise routing.

```basic
PSG 0, 284, 15            : REM channel 0: ~440Hz, max volume
PSG MIXER 56               : REM enable all tones, disable noise
PSG ENVELOPE 0, 1000       : REM sawtooth envelope, period 1000
```

**Playing PSG music data from memory:**
```basic
PSG PLAY &H100000, 4096   : REM play header-detectable PSG data
```

`PSG PLAY` has no filename extension, so use `SOUND PLAY` for PT1/PT2/PT3,
STC, SQT, ASC, and FTC tracker files.

Current source note: `PSG STOP` is parsed but only writes 0 to the player
control register. Use `SOUND STOP`, `SOUND PLAY STOP`, or `POKE &HF0C18,2` to
stop PSG media playback.

### 7.3 SID (MOS 6581/8580)

The SID emulates the Commodore 64's famous MOS Technology 6581 (and later 8580) sound chip. Three voices with four selectable waveforms (triangle, sawtooth, pulse, noise), ring modulation, oscillator sync, and a multi-mode filter (lowpass, bandpass, highpass).

```basic
REM Programme voice 1: 440Hz, 50% pulse, gate on, ADSR
SID VOICE 1, 7217, 2048, 65, &H22, &HF8
REM freq=7217 (440Hz SID freq), pw=2048 (50%), ctrl=65 (gate+pulse)
REM AD=&H22 (attack 2, decay 2), SR=&HF8 (sustain 15, release 8)

SID VOLUME 15              : REM max volume
SID FILTER 1024, 8, 1, 1  : REM lowpass filter on voice 1
```

**Playing .sid files:**
```basic
SID PLAY &H100000, 65536, 0   : REM play SID file, subsong 0
```

### 7.4 POKEY (Atari Audio)

POKEY emulates the Atari custom chip used in the Atari 400/800/XL/XE. Four channels with frequency dividers and distortion/waveform control.

```basic
POKEY 1, 100, &HA8         : REM channel 1: divider 100, pure tone vol 8
POKEY CTRL &H00            : REM default audio control
```

### 7.5 TED Sound

The TED audio section provides two tone generators and a noise generator, emulating the Commodore 16|Plus/4 sound capabilities.

```basic
TED TONE 1, 500            : REM voice 1 at 500Hz divider
TED VOL 8                  : REM volume 8
TED NOISE ON               : REM enable noise
```

Current source note: `TED STOP` is parsed but only writes 0 to the player
control register. Use `SOUND STOP`, `SOUND PLAY STOP`, or `POKE &HF0F18,2` to
stop TED media playback.

### 7.6 AHX (Amiga Tracker)

AHX plays Amiga-format AHX music modules. Load the module data into memory and use the player commands.

```basic
REM Assuming module loaded at &H100000
AHX PLAY &H100000, 8192
REM ... later:
AHX STOP
```

### 7.7 MOD Player (ProTracker)

The MOD player provides ProTracker .mod file playback using DAC mode on the SoundChip FLEX channels, with optional Amiga low-pass filter emulation.

```basic
REM Play a MOD file with A500 filter
SOUND MOD FILTER 1                  : REM 0=none, 1=A500, 2=A1200
SOUND MOD PLAY &H100000, 65536
REM ... later:
SOUND MOD STOP
```

**Checking playback status:**
```basic
IF MOD STATUS THEN PRINT "Playing"
```

**Filter models:**
| Value | Model | Description |
|-------|-------|-------------|
| 0 | None | No filtering, raw output |
| 1 | A500 | Amiga 500 RC filter (~4.5kHz) with LED filter |
| 2 | A1200 | Amiga 1200 RC filter (~28kHz) with LED filter |

### 7.8 Music File Playback

The unified `SOUND PLAY` command provides the simplest way to play music files:

```basic
SOUND PLAY "music.sid"        : REM auto-detect and play
SOUND PLAY "music.ahx", 2    : REM play subsong 2
SOUND PLAY STOP               : REM stop media playback
SOUND STOP                    : REM shorthand stop
```

`SOUND PLAY` chooses a player from the filename extension:

| Extension | Player route | Notes |
|-----------|--------------|-------|
| `.sid` | SID player | Commodore 64 PSID/RSID data; `MEDIA_SUBSONG` selects the subsong |
| `.ym` | PSG player | Atari ST YM frame data |
| `.ay` | PSG player | AY files; ZXAYEMUL files are rendered by the Z80 AY player |
| `.vgm`, `.vgz` | PSG player | VGM/VGZ AY/YM data, with SN76489 stream support when present |
| `.vtx`, `.vt` | PSG player | VTX compressed PSG streams; extension is used for direct file loading |
| `.sndh`, `.snd` | PSG player | Atari ST SNDH data rendered through the 68000 SNDH player |
| `.pt3`, `.pt2`, `.pt1`, `.stc`, `.sqt`, `.asc`, `.ftc` | PSG player | Z80 tracker formats; the extension is required because these formats do not have reliable magic bytes |
| `.ted`, `.prg` | TED player | Commodore 16|Plus/4 TED music data; `.prg` is routed to the TED player |
| `.ahx` | AHX player | Amiga AHX data; `SOUND PLAY` routes `.ahx` filenames only. `AHX PLAY addr [,len]` plays compatible AHX data already loaded in guest memory |
| `.sap` | POKEY/SAP player | Atari SAP data; `MEDIA_SUBSONG` selects the subsong |
| `.mod` | MOD player | ProTracker MOD data; MOD files are loaded directly by the MOD player instead of through the 64 KB staging buffer |
| `.wav` | WAV player | WAV sample playback; WAV files are loaded directly by the WAV player instead of through the 64 KB staging buffer |
| `.mid`, `.midi`, `.mus` | MIDI player | SMF type 0/1 and Doom MUS are rendered as a fixed IE SoundChip GM-style/chiptune interpretation using the RawlandMini patch table; this is not GM hardware emulation |

For low-level control, per-engine playback commands and registers are also
available:

| Command or route | Data accepted | Notes |
|------------------|---------------|-------|
| `SID PLAY addr [,len [,subsong]]` | SID data | Writes `SID_PLAY_PTR`, `SID_PLAY_LEN`, `SID_SUBSONG`, then starts `SID_PLAY_CTRL` |
| `SAP PLAY addr [,len [,subsong]]` | SAP data | Writes `SAP_PLAY_PTR`, `SAP_PLAY_LEN`, `SAP_SUBSONG`, then starts `SAP_PLAY_CTRL` |
| `PSG PLAY addr [,len]` | Header-detectable PSG data | Handles VGM/VGZ, YM, VTX, LHA-wrapped data, ZXAYEMUL AY, SNDH, and AY data; use `SOUND PLAY` for extension-only tracker formats. `PSG STOP` does not assert the player stop bit in the current source; use `SOUND STOP` or `POKE &HF0C18, 2` |
| `TED PLAY addr [,len]` | TED data | Starts `TED_PLAY_CTRL`; `SOUND PLAY` additionally routes `.prg` filenames to this player. `TED STOP` does not assert the player stop bit in the current source; use `SOUND STOP` or `POKE &HF0F18, 2` |
| `AHX PLAY addr [,len]` | AHX data | Starts `AHX_PLAY_CTRL`; `AHX_PLUS ON/OFF` writes `AHX_PLUS_CTRL` |
| `SOUND MOD PLAY addr [,len]` | MOD data already loaded in guest memory | Starts `MOD_PLAY_CTRL`; `SOUND MOD FILTER model` selects the MOD filter model |
| Raw WAV MMIO | WAV data | Use `WAV_PLAY_PTR`, `WAV_PLAY_LEN`, `WAV_PLAY_CTRL`, and `WAV_PLAY_STATUS`; no BASIC `WAV PLAY` statement exists |

`SID STOP`, `SAP STOP`, `AHX STOP`, and `SOUND MOD STOP` write the source-backed
stop value. `PSG STOP` and `TED STOP` are parsed but currently write `0`, which
does not stop their Go player implementations. WAV stop, pause, resume, loop,
channel base, volume, and mono control are exposed through the WAV MMIO
registers.

---

## 8. Memory Map Reference

### Main Memory Layout

This table shows the low-memory EhBASIC and legacy hardware apertures. Guest
RAM may be larger than this visible layout; discover the active and total RAM
sizes through the SYSINFO registers or, from IE64 machine code, `MFCR` control
register 15.

| Address Range | Size | Description |
|---------------|------|-------------|
| `&H00000`-`&H96FFF` | 604 KB | Main RAM and EhBASIC workspace below the hardware stack |
| `&H097000`-`&H09EFFF` | 32 KB | EhBASIC hardware stack region |
| `&H09F000` | - | Initial hardware stack top |
| `&HA0000`-`&HAFFFF` | 64 KB | VGA VRAM window |
| `&HB8000`-`&HBFFFF` | 32 KB | VGA text buffer |
| `&HF0000`-`&HFFFFF` | 64 KB | I/O region |
| `&H100000`-`&H5FFFFF` | 5 MB | Video RAM |
| `&H800000`-`&H80FFFF` | 64 KB | Media staging buffer (SOUND PLAY) |

### I/O and Hardware Aperture Map

| Address Range | Device |
|---------------|--------|
| `&HF0000`-`&HF049B` | Video Chip (copper, blitter, raster, palette, extended blitter) |
| `&HF0700`-`&HF07FF` | Terminal MMIO |
| `&HF0800`-`&HF0B7F` | Audio Chip (SoundChip) |
| `&HF0B80`-`&HF0B91` | AHX Player |
| `&HF0BC0`-`&HF0BD7` | MOD Player |
| `&HF0BD8`-`&HF0BF3` | WAV Player |
| `&HF0C00`-`&HF0C20` | PSG (AY-3-8910/YM2149) |
| `&HF0C30`-`&HF0C3F` | SN76489 |
| `&HF0C40`-`&HF0CFF` | SID2 flexible audio channels |
| `&HF0D00`-`&HF0D20` | POKEY |
| `&HF0D40`-`&HF0DFF` | SID3 flexible audio channels |
| `&HF0E00`-`&HF0E2D` | SID (MOS 6581) |
| `&HF0E30`-`&HF0E4C` | SID2 chip registers |
| `&HF0E50`-`&HF0E6C` | SID3 chip registers |
| `&HF0E80`-`&HF0EFF` | SFX Trigger |
| `&HF0F00`-`&HF0F6B` | TED (audio + video) |
| `&HF1000`-`&HF13FF` | VGA registers |
| `&HF1400`-`&HF140F` | Host Helper |
| `&HF2000`-`&HF2017` | ULA registers |
| `&HF2100`-`&HF213F` | ANTIC |
| `&HF2140`-`&HF21FB` | GTIA |
| `&HF2200`-`&HF221F` | File I/O |
| `&HF2220`-`&HF225F` | AROS DOS Handler (internal) |
| `&HF2260`-`&HF22AF` | Paula DMA audio bridge |
| `&HF2300`-`&HF231F` | Media Loader (SOUND PLAY) |
| `&HF2320`-`&HF233F` | Program Executor (RUN "file") |
| `&HF2340`-`&HF238F` | Coprocessor control |
| `&HF2390`-`&HF23AF` | Clipboard Bridge (internal) |
| `&HF23B0`-`&HF23BF` | Coprocessor monitor |
| `&HF23C0`-`&HF23DF` | IRQ diagnostics (internal) |
| `&HF23E0`-`&HF23FF` | Bootstrap HostFS (internal) |
| `&HF2400`-`&HF24FF` | SYSINFO read-only RAM-size registers |
| `&HD0000`-`&HDFFFF` | Voodoo texture memory |
| `&HF8000`-`&HF87FF` | Voodoo 3DFX registers |
| `&HFA000`-`&HFBAFF` | ULA VRAM aperture |

### EhBASIC Memory Layout

| Address | Size | Purpose |
|---------|------|---------|
| `&H001000`-`&H020FFF` | 128 KB | Interpreter code reservation |
| `&H021000`-`&H021FFF` | 4 KB | Input line buffer |
| `&H022000`-`&H022FFF` | 4 KB | Interpreter state block |
| `&H023000`-`&H04FFFF` | 180 KB | Programme text region (grows upward; capped by `BASIC_PROG_LIMIT`) |
| `&H050000`-`&H057FFF` | 32 KB | Numeric variable table region start |
| `&H058000`-`&H05FFFF` | 32 KB | String variable table region start |
| `&H060000`-`&H08BFFF` | 176 KB | Array storage region start |
| `&H08C000`-`&H08FFFF` | 16 KB | String temporaries / heap top |
| `&H090000`-`&H096FFF` | 28 KB | GOSUB/FOR/DO/WHILE stacks |
| `&H097000`-`&H09EFFF` | 32 KB | Hardware stack region |
| `&H09F000` | - | Initial stack top (SP) |

`line_store` rejects stored programme lines that would move `ST_PROG_END` beyond
`BASIC_PROG_LIMIT` (`&H050000`) and reports `?OUT OF MEMORY ERROR`. `var_init`
initialises variable/array pointers at fixed bases (`&H050000`, `&H058000`,
`&H060000`) and the string heap at `&H08C000`.

### State Block Offsets

The interpreter state block at `&H022000` contains:

| Offset | Field | Description |
|--------|-------|-------------|
| `+&H00` | ST_PROG_START | Start of BASIC programme text |
| `+&H04` | ST_PROG_END | End of programme text |
| `+&H08` | ST_VAR_START | Simple variable table start |
| `+&H0C` | ST_VAR_END | Simple variable table end |
| `+&H10` | ST_ARRAY_START | Array storage start |
| `+&H14` | ST_ARRAY_END | Array storage end |
| `+&H18` | ST_HEAP_TOP | String heap pointer |
| `+&H1C` | ST_HEAP_BOTTOM | Heap bottom limit |
| `+&H20` | ST_CURRENT_LINE | Line number being executed |
| `+&H24` | ST_TEXT_PTR | Execution cursor |
| `+&H28` | ST_DATA_PTR | DATA/READ pointer |
| `+&H2C` | ST_GOSUB_SP | GOSUB return stack pointer |
| `+&H30` | ST_FOR_SP | FOR/NEXT stack pointer |
| `+&H34` | ST_RANDOM_SEED | RND seed value |
| `+&H38` | ST_ERROR_FLAG | Last error code |
| `+&H3C` | ST_TRACE_FLAG | TRON/TROFF flag |
| `+&H40` | ST_DIRECT_MODE | 1=immediate, 0=running |
| `+&H44` | ST_SVAR_START | String variable table start |
| `+&H48` | ST_SVAR_END | String variable table end |
| `+&H4C` | ST_TEXT_ATTR | Text attribute used by `COLOR` |
| `+&H50` | ST_COPPER_BUILD | Copper build pointer |
| `+&H54` | ST_ULA_BRIGHT | ULA bright flag |
| `+&H58` | ST_ULA_INK | ULA ink colour |
| `+&H5C` | ST_ULA_PAPER | ULA paper colour |
| `+&H60` | ST_ULA_FLASH | ULA flash flag |
| `+&H64` | ST_CONT_LINE_PTR | Saved line pointer for `CONT` |
| `+&H68` | ST_CONT_TEXT_PTR | Saved text pointer for `CONT` |
| `+&H6C` | ST_ERROR_LINE | Last runtime error line |
| `+&H70` | ST_TERM_COL | Current terminal output column |

---

## 9. Hardware Register Reference

This section documents the I/O registers used directly by EhBASIC commands or
commonly accessed from EhBASIC via POKE/PEEK. Registers are grouped by
subsystem. Internal OS/debug bridge regions listed in the aperture map are
intentionally not expanded here unless they are useful from EhBASIC or
guest-side hardware code.

### 9.1 Terminal MMIO (`&HF0700`-`&HF07FF`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF0700` | TERM_OUT | W | Write character to output |
| `&HF0704` | TERM_STATUS | R | Bit 0: input available, Bit 1: output ready |
| `&HF0708` | TERM_IN | R | Read next input character (dequeues) |
| `&HF070C` | TERM_LINE_STATUS | R | Bit 0: complete line available |
| `&HF0710` | TERM_ECHO | R/W | Bit 0: local echo enable (default 1) |
| `&HF0714` | TERM_CURSOR_X | Reserved | Defined constant; current terminal handler returns 0 and ignores writes |
| `&HF0718` | TERM_CURSOR_Y | Reserved | Defined constant; current terminal handler returns 0 and ignores writes |
| `&HF071C` | TERM_FG_COLOR | Reserved | Defined constant; current terminal handler returns 0 and ignores writes |
| `&HF0720` | TERM_BG_COLOR | Reserved | Defined constant; current terminal handler returns 0 and ignores writes |
| `&HF0724` | TERM_CTRL | R/W | Bit 0: line input mode; default 1 |
| `&HF0728` | TERM_KEY_IN | R | Read next raw key (dequeues) |
| `&HF072C` | TERM_KEY_STATUS | R | Bit 0: raw key available |
| `&HF0730` | MOUSE_X | R | Absolute mouse X position, low 16 bits |
| `&HF0734` | MOUSE_Y | R | Absolute mouse Y position, low 16 bits |
| `&HF0738` | MOUSE_BUTTONS | R | Bit 0=left, bit 1=right, bit 2=middle |
| `&HF073C` | MOUSE_STATUS | R | Bit 0=mouse changed since last `MOUSE_STATUS` read; read clears flag |
| `&HF0740` | SCAN_CODE | R | Raw keyboard scancode dequeue |
| `&HF0744` | SCAN_STATUS | R | Bit 0=scancode available |
| `&HF0748` | SCAN_MODIFIERS | R | Bit 0=shift, bit 1=ctrl, bit 2=alt, bit 3=caps lock |
| `&HF074C` | MOUSE_CTRL | R/W | Bit 0=request relative/captured mouse mode |
| `&HF0750` | RTC_EPOCH | R | Host UTC seconds since Unix epoch |
| `&HF0754` | MOUSE_DX | R | Signed accumulated relative X delta; read clears |
| `&HF0758` | MOUSE_DY | R | Signed accumulated relative Y delta; read clears |
| `&HF07F0` | TERM_SENTINEL | W | Write `&HDEAD` to stop CPU |

### 9.2 VGA Registers (`&HF1000`-`&HF13FF`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF1000` | VGA_MODE | R/W | Mode: `&H03`=text, `&H12`=640x480, `&H13`=320x200 |
| `&HF1004` | VGA_STATUS | R | Bit 0: VSync, Bit 3: vertical retrace |
| `&HF1008` | VGA_CTRL | R/W | Bit 0: enable |
| `&HF1010` | VGA_SEQ_INDEX | W | Sequencer index |
| `&HF1014` | VGA_SEQ_DATA | R/W | Sequencer data |
| `&HF1018` | VGA_SEQ_MAPMASK | R/W | Direct plane write mask for planar modes |
| `&HF1020` | VGA_CRTC_INDEX | W | CRTC index |
| `&HF1024` | VGA_CRTC_DATA | R/W | CRTC data |
| `&HF1028` | VGA_CRTC_STARTHI | R/W | Display start address (high byte) |
| `&HF102C` | VGA_CRTC_STARTLO | R/W | Display start address (low byte) |
| `&HF1030` | VGA_GC_INDEX | W | Graphics Controller index |
| `&HF1034` | VGA_GC_DATA | R/W | Graphics Controller data |
| `&HF1038` | VGA_GC_READMAP | R/W | Direct read-plane select for planar modes |
| `&HF103C` | VGA_GC_BITMASK | R/W | Direct VGA bit mask |
| `&HF1040` | VGA_ATTR_INDEX | W | Attribute Controller index |
| `&HF1044` | VGA_ATTR_DATA | R/W | Attribute Controller data |
| `&HF1050` | VGA_DAC_MASK | R/W | Pixel mask |
| `&HF1054` | VGA_DAC_RINDEX | W | DAC read index |
| `&HF1058` | VGA_DAC_WINDEX | W | DAC write index |
| `&HF105C` | VGA_DAC_DATA | R/W | DAC data (R, G, B sequence) |
| `&HF1100`-`&HF13FF` | VGA_PALETTE | R/W | 256 x 3 bytes palette RAM |

### 9.3 Host Helper MMIO (`&HF1400`-`&HF140F`)

This security-sensitive aperture is mapped in the normal VM bus. Command
execution is enabled only when the VM is launched with `-ehbasic-host`.

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF1400` | HOST_MMIO_CMD | R/W | Command: 1=NET, 2=UPDATE, 3=REBOOT, 4=POWEROFF |
| `&HF1404` | HOST_MMIO_TRIGGER | W | Write non-zero to start the selected command |
| `&HF1408` | HOST_MMIO_STATUS | R | 0=running, 1=ok, 2=error, 3=user cancel, 4=disabled, 5=idle |
| `&HF140C` | HOST_MMIO_EXIT | R | 32-bit helper exit code |

Before the first trigger, `HOST_MMIO_STATUS` reads `HOST_STATUS_IDLE` (`5`). Without
`-ehbasic-host`, a trigger reports disabled status instead of running a host
command. Only one command may run at a time. A trigger while `HOST_MMIO_STATUS` is
running is ignored. Subword reads from `HOST_MMIO_EXIT` return the corresponding
shifted byte or word of the 32-bit exit code.

### 9.4 ULA Registers (`&HF2000`-`&HF2017`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF2000` | ULA_BORDER | R/W | Border colour (bits 0-2) |
| `&HF2004` | ULA_CTRL | R/W | Bit 0: enable, bit 1: VBlank IRQ enable, bit 2: auto-increment |
| `&HF2008` | ULA_STATUS | R | Bit 0: VBlank |
| `&HF200C` | ULA_ADDR_LO | R/W | Paged VRAM address low byte |
| `&HF2010` | ULA_ADDR_HI | R/W | Paged VRAM address high bits |
| `&HF2014` | ULA_DATA | R/W | Paged VRAM data byte |

ULA VRAM aperture at `&HFA000`: 6,144 bytes bitmap + 768 bytes attributes.

### 9.5 Audio Chip Registers (`&HF0800`-`&HF0B7F`)

#### Global Control

| Address | Name | Description |
|---------|------|-------------|
| `&HF0800` | AUDIO_CTRL | Master audio control |
| `&HF0804` | ENV_SHAPE | Global envelope shape |
| `&HF0820` | FILTER_CUTOFF | Cutoff frequency (0-255) |
| `&HF0824` | FILTER_RESONANCE | Resonance (0-255) |
| `&HF0828` | FILTER_TYPE | 0=off, 1=LP, 2=HP, 3=BP |
| `&HF082C` | FILTER_MOD_SOURCE | Modulation source channel, 0-3 |
| `&HF0830` | FILTER_MOD_AMOUNT | Modulation depth, 0-255 |
| `&HF0860` + `channel*4` | ENV_SHAPE_CH | Per-channel envelope shape |

#### Fixed Channels

| Channel | Freq | Vol | Ctrl | ADSR |
|---------|------|-----|------|------|
| Square | `&HF0900` | `&HF0904` | `&HF0908` | `&HF0930`-`&HF093C` |
| Triangle | `&HF0940` | `&HF0944` | `&HF0948` | `&HF0960`-`&HF096C` |
| Sine | `&HF0980` | `&HF0984` | `&HF0988` | `&HF0990`-`&HF099C` |
| Noise | `&HF09C0` | `&HF09C4` | `&HF09C8` | `&HF09D0`-`&HF09DC` |
| Sawtooth | `&HF0A20` | `&HF0A24` | `&HF0A28` | `&HF0A30`-`&HF0A3C` |

Frequency values are 16.8 fixed-point: Hz * 256.

Additional fixed-channel registers:

| Address | Name | Description |
|---------|------|-------------|
| `&HF090C` | SQUARE_DUTY | Duty cycle, 0-255 |
| `&HF0910` | SQUARE_SWEEP | Square sweep control |
| `&HF0914` | TRI_SWEEP | Triangle sweep control |
| `&HF0918` | SINE_SWEEP | Sine sweep control |
| `&HF0922` | SQUARE_PWM_CTRL | bit 7=PWM enable, bits 0-6=rate |
| `&HF09E0` | NOISE_MODE | Fixed noise-channel mode: 0=white, 1=periodic, 2=metallic, 3=PSG-style, 4=TED 8-bit, 5=SN76489 15-bit white, 6=SN76489 15-bit periodic, 7=SN76489 16-bit white, 8=SN76489 16-bit periodic |
| `&HF0A00`-`&HF0A0C` | SYNC_SOURCE_CH0-3 | Hard-sync sources for channels 0-3 |
| `&HF0A10`-`&HF0A1C` | RING_MOD_SOURCE_CH0-3 | Ring-mod sources for channels 0-3 |
| `&HF0A2C` | SAW_SWEEP | Sawtooth sweep control |
| `&HF0A60` | SYNC_SOURCE_CH4 | Hard-sync source for sawtooth |
| `&HF0A64` | RING_MOD_SOURCE_CH4 | Ring-mod source for sawtooth |

`SQUARE_PWM_CTRL` is byte-oriented and lives at `&HF0922`. `POKE` and `LOKE`
require 4-byte aligned addresses and therefore raise `?FC ERROR` for this
register; use `POKE8 &HF0922,value` for normal writes. `DOKE &HF0922,value` is
2-byte aligned and reaches the low byte, but also writes the neighbouring byte,
so `POKE8` is the precise access form.

#### Flexible Channels (64 bytes each)

| Channel | Base Address |
|---------|-------------|
| Flex 0 | `&HF0A80` |
| Flex 1 | `&HF0AC0` |
| Flex 2 | `&HF0B00` |
| Flex 3 | `&HF0B40` |

Offsets from channel base:

| Offset | Name | Description |
|--------|------|-------------|
| `+&H00` | FREQ | Frequency (16.8 fixed-point) |
| `+&H04` | VOL | Volume (0-255) |
| `+&H08` | CTRL | Control: bit 0=enable, bit 1=gate |
| `+&H0C` | DUTY | Duty cycle (0-255) |
| `+&H10` | SWEEP | Packed pitch sweep: enable, period, shift |
| `+&H14` | ATK | Attack time (ms) |
| `+&H18` | DEC | Decay time (ms) |
| `+&H1C` | SUS | Sustain level (0-255) |
| `+&H20` | REL | Release time (ms) |
| `+&H24` | WAVE_TYPE | Waveform (0-4) |
| `+&H28` | PWM_CTRL | PWM control |
| `+&H2C` | NOISEMODE | Noise mode |
| `+&H30` | PHASE | Phase reset |
| `+&H34` | RINGMOD | bit 7=enable, bits 0-2=source channel |
| `+&H38` | SYNC | bit 7=enable, bits 0-2=source channel |
| `+&H3C` | DAC | Signed 8-bit DAC sample value |

SID2 and SID3 flexible banks use the same 64-byte channel layout:

| Bank | Channels | Address Range |
|------|----------|---------------|
| SID2 flex | 3 channels | `&HF0C40`-`&HF0CFF` |
| SID3 flex | 3 channels | `&HF0D40`-`&HF0DFF` |

#### Effects

| Address | Name | Description |
|---------|------|-------------|
| `&HF0A40` | OVERDRIVE_CTRL | Drive amount (0-255) |
| `&HF0A50` | REVERB_MIX | Dry/wet mix (0-255) |
| `&HF0A54` | REVERB_DECAY | Decay time (0-255) |

### 9.6 SFX Trigger Registers (`&HF0E80`-`&HF0EFF`)

SFX trigger channels play raw sample buffers through SoundChip DAC paths. There
are four channels, 32 bytes each.

| Channel | Base Address |
|---------|--------------|
| 0 | `&HF0E80` |
| 1 | `&HF0EA0` |
| 2 | `&HF0EC0` |
| 3 | `&HF0EE0` |

| Offset | Name | R/W | Description |
|--------|------|-----|-------------|
| `+&H00` | SFX_PTR | R/W | Sample pointer in guest memory |
| `+&H04` | SFX_LEN | R/W | Sample length in bytes |
| `+&H08` | SFX_LOOP_PTR | R/W | Loop sample pointer |
| `+&H0C` | SFX_LOOP_LEN | R/W | Loop length in bytes |
| `+&H10` | SFX_FREQ | R/W | Playback frequency |
| `+&H14` | SFX_VOL | R/W | Volume |
| `+&H16` | SFX_PAN_RESERVED | R/W | Reserved |
| `+&H18` | SFX_FORMAT | R/W | 0=signed 8-bit, 1=unsigned 8-bit, 2=signed 16-bit |
| `+&H1C` | SFX_CTRL | R/W | bit 0=trigger, bit 1=stop, bit 2=loop enable |

### 9.7 Music Player Registers (`&HF0B80`-`&HF0BF3`)

These blocks are used by `AHX`, `SOUND MOD`, `SOUND PLAY`, and machine-code
programmes that load music data into guest memory themselves. Player control
bits are consistent unless noted: bit 0=start, bit 1=stop, bit 2=loop.

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF0B80` | AHX_PLUS_CTRL | R/W | AHX+ mode: 0=standard, 1=enhanced |
| `&HF0B84` | AHX_PLAY_PTR | R/W | 32-bit pointer to AHX data |
| `&HF0B88` | AHX_PLAY_LEN | R/W | 32-bit AHX data length |
| `&HF0B8C` | AHX_PLAY_CTRL | R/W | Control bits: 1=start, 2=stop, 4=loop |
| `&HF0B90` | AHX_PLAY_STATUS | R | bit 0=busy, bit 1=error |
| `&HF0B91` | AHX_SUBSONG | R/W | Subsong selection, 0-255 |
| `&HF0BA0` | MIDI_PLAY_PTR | R/W | 32-bit pointer to MIDI/MUS data |
| `&HF0BA4` | MIDI_PLAY_LEN | R/W | 32-bit MIDI/MUS data length |
| `&HF0BA8` | MIDI_PLAY_CTRL | R/W | bit 0=start, bit 1=stop, bit 2=loop, bit 3=pause, bit 4=loop apply only |
| `&HF0BAC` | MIDI_PLAY_STATUS | R | bit 0=busy, bit 1=error, bit 2=paused, bit 3=loading |
| `&HF0BB0` | MIDI_POSITION | R | Current sample position |
| `&HF0BB4` | MIDI_VOLUME | R/W | Global MIDI volume, 0-255 |
| `&HF0BB8` | MIDI_TEMPO_BPM | R | Current effective tempo; writes do not override tempo |
| `&HF0BC0` | MOD_PLAY_PTR | R/W | 32-bit pointer to MOD data |
| `&HF0BC4` | MOD_PLAY_LEN | R/W | 32-bit MOD data length |
| `&HF0BC8` | MOD_PLAY_CTRL | R/W | Control bits: 1=start, 2=stop, 4=loop |
| `&HF0BCC` | MOD_PLAY_STATUS | R | bit 0=playing, bit 1=error |
| `&HF0BD0` | MOD_FILTER_MODEL | R/W | 0=none, 1=A500 4.5 kHz, 2=A1200 28 kHz |
| `&HF0BD4` | MOD_POSITION | R | Current song position |
| `&HF0BD8` | WAV_PLAY_PTR | R/W | Low 32 bits of WAV data pointer |
| `&HF0BDC` | WAV_PLAY_LEN | R/W | 32-bit WAV data length |
| `&HF0BE0` | WAV_PLAY_CTRL | R/W | bit 0=start, bit 1=stop, bit 2=loop, bit 3=pause, bit 4=loop apply only |
| `&HF0BE4` | WAV_PLAY_STATUS | R | bit 0=busy, bit 1=error, bit 2=paused, bit 3=stereo active |
| `&HF0BE8` | WAV_POSITION | R | Current source frame position |
| `&HF0BEC` | WAV_PLAY_PTR_HI | R/W | High 32 bits of WAV data pointer |
| `&HF0BF0` | WAV_CHANNEL_BASE | R/W | Left DAC channel base; right uses base+1 |
| `&HF0BF1` | WAV_VOLUME_L | R/W | Left volume, 0-255 |
| `&HF0BF2` | WAV_VOLUME_R | R/W | Right volume, 0-255 |
| `&HF0BF3` | WAV_FLAGS | R/W | bit 0=force mono |

There is no BASIC `WAV STATUS` function. Use `SOUND PLAY` status indirectly
through playback behaviour, or read `WAV_PLAY_STATUS` directly.

### 9.8 PSG and SN76489 Registers (`&HF0C00`-`&HF0C3F`)

The AY-3-8910/YM2149 register file is exposed directly at `&HF0C00`-`&HF0C0F`.
IOA/IOB are storage-only on IE.

| Address | Name | Description |
|---------|------|-------------|
| `&HF0C00` | PSG_REG0 | Channel A fine tune |
| `&HF0C01` | PSG_REG1 | Channel A coarse tune |
| `&HF0C02` | PSG_REG2 | Channel B fine tune |
| `&HF0C03` | PSG_REG3 | Channel B coarse tune |
| `&HF0C04` | PSG_REG4 | Channel C fine tune |
| `&HF0C05` | PSG_REG5 | Channel C coarse tune |
| `&HF0C06` | PSG_REG6 | Noise period |
| `&HF0C07` | PSG_REG7 | Mixer enable register |
| `&HF0C08` | PSG_REG8 | Channel A amplitude |
| `&HF0C09` | PSG_REG9 | Channel B amplitude |
| `&HF0C0A` | PSG_REG10 | Channel C amplitude |
| `&HF0C0B` | PSG_REG11 | Envelope fine period |
| `&HF0C0C` | PSG_REG12 | Envelope coarse period |
| `&HF0C0D` | PSG_REG13 | Envelope shape |
| `&HF0C0E` | PSG_REG14 | IOA storage |
| `&HF0C0F` | PSG_REG15 | IOB storage |
| `&HF0C10` | PSG_PLAY_PTR | 32-bit player data pointer |
| `&HF0C14` | PSG_PLAY_LEN | 32-bit player data length |
| `&HF0C18` | PSG_PLAY_CTRL | bit 0=start, bit 1=stop, bit 2=loop |
| `&HF0C1C` | PSG_PLAY_STATUS | bit 0=busy, bit 1=error |
| `&HF0C20` | PSG_PLUS_CTRL | Enhanced mode: 0=standard, 1=enhanced |
| `&HF0C30` | SN_PORT_WRITE | W | SN76489 latch/data byte write |
| `&HF0C31` | SN_PORT_READY | R | bit 0=ready |
| `&HF0C32` | SN_PORT_MODE | R/W | 0=TI 15-bit LFSR, 1=Sega 16-bit LFSR |

### 9.9 POKEY and SAP Registers (`&HF0D00`-`&HF0D20`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF0D00` | POKEY_AUDF1 | Channel 1 frequency divider |
| `&HF0D01` | POKEY_AUDC1 | Channel 1 control: bits 0-3 volume, bit 4 volume-only, bits 5-7 distortion |
| `&HF0D02` | POKEY_AUDF2 | Channel 2 frequency divider |
| `&HF0D03` | POKEY_AUDC2 | Channel 2 control |
| `&HF0D04` | POKEY_AUDF3 | Channel 3 frequency divider |
| `&HF0D05` | POKEY_AUDC3 | Channel 3 control |
| `&HF0D06` | POKEY_AUDF4 | Channel 4 frequency divider |
| `&HF0D07` | POKEY_AUDC4 | Channel 4 control |
| `&HF0D08` | POKEY_AUDCTL | Master audio control |
| `&HF0D09` | POKEY_PLUS_CTRL | Enhanced mode: 0=standard, 1=enhanced |
| `&HF0D0A` | POKEY_RANDOM | Readable polynomial/RNG tap |
| `&HF0D10` | SAP_PLAY_PTR | 32-bit SAP data pointer |
| `&HF0D14` | SAP_PLAY_LEN | 32-bit SAP data length |
| `&HF0D18` | SAP_PLAY_CTRL | bit 0=start, bit 1=stop, bit 2=loop |
| `&HF0D1C` | SAP_PLAY_STATUS | bit 0=busy, bit 1=error |
| `&HF0D20` | SAP_SUBSONG | Subsong selection, 0-255 |

`POKEY_AUDCTL` bits: bit 0 selects 15 kHz base clock, bits 1-2 enable high-pass
filters, bits 3-4 join channels for 16-bit modes, bits 5-6 select 1.79 MHz
clocking for channels 3 and 1, and bit 7 selects 9-bit polynomial noise.

### 9.10 SID Registers (`&HF0E00`-`&HF0E2D`)

Each SID voice occupies 7 bytes.

| Voice | Base | Registers |
|-------|------|-----------|
| 1 | `&HF0E00` | FREQ_LO, FREQ_HI, PW_LO, PW_HI, CTRL, AD, SR |
| 2 | `&HF0E07` | FREQ_LO, FREQ_HI, PW_LO, PW_HI, CTRL, AD, SR |
| 3 | `&HF0E0E` | FREQ_LO, FREQ_HI, PW_LO, PW_HI, CTRL, AD, SR |

| Address | Name | Description |
|---------|------|-------------|
| `&HF0E15` | SID_FC_LO | Filter cutoff low bits |
| `&HF0E16` | SID_FC_HI | Filter cutoff high byte |
| `&HF0E17` | SID_RES_FILT | bits 0-3 filter routing, bits 4-7 resonance |
| `&HF0E18` | SID_MODE_VOL | bits 0-3 volume, bits 4-6 LP/BP/HP, bit 7 voice 3 off |
| `&HF0E19` | SID_PLUS_CTRL | Enhanced mode: 0=standard, 1=enhanced |
| `&HF0E1A` | SID_POT_Y | Potentiometer Y read register, not implemented |
| `&HF0E1B` | SID_OSC3 | Oscillator 3 output |
| `&HF0E1C` | SID_ENV3 | Envelope 3 output |
| `&HF0E20` | SID_PLAY_PTR | 32-bit SID data pointer |
| `&HF0E24` | SID_PLAY_LEN | 32-bit SID data length |
| `&HF0E28` | SID_PLAY_CTRL | bit 0=start, bit 1=stop, bit 2=loop |
| `&HF0E2C` | SID_PLAY_STATUS | bit 0=busy, bit 1=error |
| `&HF0E2D` | SID_SUBSONG | Subsong selection, 0-255 |

SID voice `CTRL` bits: bit 0=gate, bit 1=sync, bit 2=ring modulation, bit
3=test, bit 4=triangle, bit 5=sawtooth, bit 6=pulse, bit 7=noise.

Secondary SID chip register windows are also mapped. `SID2_BASE` is `&HF0E30`
through `&HF0E4C`; `SID3_BASE` is `&HF0E50` through `&HF0E6C`. Each secondary
SID uses the same 29-byte voice/filter/read-register layout as `SID_BASE` with
addresses relative to its own base. The BASIC `SID` statement targets the
primary SID command path; use `POKE8`/`PEEK` for direct secondary SID access.

### 9.11 TED Registers (`&HF0F00`-`&HF0F6B`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF0F00` | TED_FREQ1_LO | Voice 1 frequency low byte |
| `&HF0F01` | TED_FREQ2_LO | Voice 2 frequency low byte |
| `&HF0F02` | TED_FREQ2_HI | Voice 2 frequency high bits |
| `&HF0F03` | TED_SND_CTRL | bit 7=DAC mode, bit 6=voice 2 noise, bit 5=voice 2 enable, bit 4=voice 1 enable, bits 0-3 volume |
| `&HF0F04` | TED_FREQ1_HI | Voice 1 frequency high bits |
| `&HF0F05` | TED_PLUS_CTRL | Enhanced mode: 0=standard, 1=enhanced |
| `&HF0F10` | TED_PLAY_PTR | 32-bit TED data pointer |
| `&HF0F14` | TED_PLAY_LEN | 32-bit TED data length |
| `&HF0F18` | TED_PLAY_CTRL | bit 0=start, bit 1=stop, bit 2=loop |
| `&HF0F1C` | TED_PLAY_STATUS | bit 0=busy, bit 1=error |
| `&HF0F20` | TED_V_CTRL1 | ECM/BMM/DEN/RSEL/YSCROLL |
| `&HF0F24` | TED_V_CTRL2 | RES/MCM/CSEL/XSCROLL |
| `&HF0F28` | TED_V_CHAR_BASE | Character/bitmap base address selector |
| `&HF0F2C` | TED_V_VIDEO_BASE | Video matrix base address selector |
| `&HF0F30`-`&HF0F3C` | TED_V_BG_COLOR0-3 | Background colours |
| `&HF0F40` | TED_V_BORDER | Border colour |
| `&HF0F44` | TED_V_CURSOR_HI | Cursor position high byte |
| `&HF0F48` | TED_V_CURSOR_LO | Cursor position low byte |
| `&HF0F4C` | TED_V_CURSOR_CLR | Cursor colour |
| `&HF0F50` | TED_V_RASTER_LO | Raster line low byte |
| `&HF0F54` | TED_V_RASTER_HI | Raster high bit and flags |
| `&HF0F58` | TED_V_ENABLE | bit 0=video enable |
| `&HF0F5C` | TED_V_STATUS | bit 0=VBlank |
| `&HF0F60` | TED_V_RASTER_CMP_LO | Raster compare low byte |
| `&HF0F64` | TED_V_RASTER_CMP_HI | Raster compare high bit in bit 0 |
| `&HF0F68` | TED_V_RASTER_STATUS | bit 7=raster compare pending; write 1 to clear |

### 9.12 ANTIC Registers (`&HF2100`-`&HF213F`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF2100` | ANTIC_DMACTL | DMA control |
| `&HF2104` | ANTIC_CHACTL | Character control |
| `&HF2108` | ANTIC_DLISTL | Display list pointer (low) |
| `&HF210C` | ANTIC_DLISTH | Display list pointer (high) |
| `&HF2110` | ANTIC_HSCROL | Horizontal scroll (0-15) |
| `&HF2114` | ANTIC_VSCROL | Vertical scroll (0-15) |
| `&HF2118` | ANTIC_PMBASE | Player/missile base |
| `&HF211C` | ANTIC_CHBASE | Character set base |
| `&HF2120` | ANTIC_WSYNC | Wait for sync (write-only) |
| `&HF2124` | ANTIC_VCOUNT | Vertical counter, returns scanline/2 (read-only) |
| `&HF2128` | ANTIC_PENH | Light pen horizontal position (read-only) |
| `&HF212C` | ANTIC_PENV | Light pen vertical position (read-only) |
| `&HF2130` | ANTIC_NMIEN | NMI enable: bit 6=VBI, bit 7=DLI |
| `&HF2134` | ANTIC_NMIST | NMI status read / NMI reset write: bit 5=reset, bit 6=VBI pending, bit 7=DLI pending |
| `&HF2138` | ANTIC_ENABLE | Video enable (Bit 0), PAL mode (Bit 1) |
| `&HF213C` | ANTIC_STATUS | Status (Bit 0: VBlank) |

### 9.13 GTIA Registers (`&HF2140`-`&HF21FB`)

| Address Range | Description |
|---------------|-------------|
| `&HF2140` | GTIA_COLPF0 - playfield colour 0 |
| `&HF2144` | GTIA_COLPF1 - playfield colour 1 |
| `&HF2148` | GTIA_COLPF2 - playfield colour 2 |
| `&HF214C` | GTIA_COLPF3 - playfield colour 3 |
| `&HF2150` | GTIA_COLBK - background/border colour |
| `&HF2154`-`&HF2160` | GTIA_COLPM0-3 - player/missile colours |
| `&HF2164` | GTIA_PRIOR (priority) |
| `&HF2168` | GTIA_GRACTL (graphics control) |
| `&HF216C` | GTIA_CONSOL (console switches, read) |
| `&HF2170`-`&HF217C` | Player positions (HPOSP0-3) |
| `&HF2180`-`&HF218C` | Missile positions (HPOSM0-3) |
| `&HF2190`-`&HF219C` | GTIA_SIZEP0-3 - player sizes |
| `&HF21A0` | GTIA_SIZEM - missile sizes |
| `&HF21A4`-`&HF21B0` | GTIA_GRAFP0-3 - player graphics patterns |
| `&HF21B4` | GTIA_GRAFM - missile graphics pattern |
| `&HF21B8`-`&HF21F4` | Collision latches: M0PF-M3PF, P0PF-P3PF, M0PL-M3PL, P0PL-P3PL |
| `&HF21F8` | HITCLR collision latch clear |

### 9.14 Voodoo 3DFX Registers (`&HF8000`-`&HF87FF`, texture memory `&HD0000`-`&HDFFFF`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF8000` | VOODOO_STATUS | Status flags (read-only) |
| `&HF8004` | VOODOO_ENABLE | Enable register |
| `&HF8008`-`&HF801C` | VERTEX_AX..CY | Vertex coordinates (12.4 fixed-point) |
| `&HF8020`-`&HF8028` | START_R/G/B | Vertex colour attributes (12.12 fixed-point) |
| `&HF802C` | START_Z | Vertex Z attribute (20.12 fixed-point) |
| `&HF8030` | START_A | Vertex alpha attribute (12.12 fixed-point) |
| `&HF8034`-`&HF8038` | START_S/T | Texture coordinates (14.18 fixed-point) |
| `&HF803C` | START_W | Perspective W / 1-Z attribute (2.30 fixed-point) |
| `&HF8040`-`&HF805C` | DRDX..DWDX | Attribute gradients per X |
| `&HF8060`-`&HF807C` | DRDY..DWDY | Attribute gradients per Y |
| `&HF8080` | TRIANGLE_CMD | Submit triangle |
| `&HF8084` | FTRIANGLE_CMD | Fast triangle / strip command |
| `&HF8088` | COLOR_SELECT | Colour select |
| `&HF8104` | FBZCOLOR_PATH | Colour path control |
| `&HF8108` | FOG_MODE | Fog mode |
| `&HF810C` | ALPHA_MODE | Alpha blending mode |
| `&HF8110` | FBZ_MODE | Framebuffer Z mode (depth, clipping) |
| `&HF8114` | LFB_MODE | Linear framebuffer mode |
| `&HF8118` | CLIP_LEFT_RIGHT | Clip rectangle X |
| `&HF811C` | CLIP_LOW_Y_HIGH | Clip rectangle Y |
| `&HF8120` | NOP_CMD | No-op command |
| `&HF8124` | FAST_FILL_CMD | Fast fill command |
| `&HF8128` | SWAP_BUFFER_CMD | Swap buffers |
| `&HF8140` | FOG_TABLE_BASE | Base of 64 32-bit fog table entries |
| `&HF81C4` | FOG_COLOR | Fog colour |
| `&HF81C8` | ZA_COLOR | Z/alpha colour |
| `&HF81CC` | CHROMA_KEY | Chroma-key colour |
| `&HF81D0` | CHROMA_RANGE | Chroma-key range |
| `&HF81D4` | STIPPLE | Stipple pattern |
| `&HF81D8` | COLOR0 | Constant colour 0 |
| `&HF81DC` | COLOR1 | Constant colour 1 |
| `&HF8200`-`&HF8210` | FBI_INIT0-4 | Framebuffer interface configuration |
| `&HF8214` | VIDEO_DIM | Video dimensions |
| `&HF8218` | BACK_PORCH | Back porch timing |
| `&HF821C` | VIDEO_DIM_V | Vertical video dimensions |
| `&HF8220` | H_SYNC | Horizontal sync |
| `&HF8224` | V_SYNC | Vertical sync |
| `&HF822C` | DAC_DATA | DAC data register |
| `&HF8300` | TEXTURE_MODE | Texture mode |
| `&HF8304` | TLOD | Texture LOD control |
| `&HF8308` | TDETAIL | Texture detail control |
| `&HF830C` | TEX_BASE0 | Texture base address |
| `&HF8310`-`&HF832C` | TEX_BASE1-8 | Texture base addresses for LOD 1-8 |
| `&HF8330` | TEX_WIDTH | Texture width |
| `&HF8334` | TEX_HEIGHT | Texture height |
| `&HF8338` | TEX_UPLOAD | Texture upload trigger |
| `&HF8400`-`&HF87FF` | PALETTE_BASE | 256 32-bit texture palette entries |

`VOODOO_STATUS` bits: bit 0=FBI busy, bit 1=TMU busy, bit 2=SST busy, bit
6=vertical retrace, bit 7=swap-buffer pending. `FBZ_MODE` uses bit
0=clipping, bit 1=chroma key, bit 2=stipple, bit 3=W-buffer, bit 4=depth
enable, bits 5-7=depth function, bit 8=dither, bit 9=RGB write, bit 10=depth
write, bit 11=2x2 dither, bit 12=alpha write, bit 14=draw front, bit 15=draw
back, bit 16=depth source, bit 17=Y origin, bit 18=alpha planes, bit 19=alpha
dither, bits 20-31=depth offset.

### 9.15 Video Chip Registers (`&HF0000`-`&HF049B`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF0000` | VIDEO_CTRL | Video control |
| `&HF0004` | VIDEO_MODE | Video mode select |
| `&HF0008` | VIDEO_STATUS | Video status |
| `&HF000C` | COPPER_CTRL | Copper control (1=enable) |
| `&HF0010` | COPPER_PTR | Copper list pointer |
| `&HF0014` | COPPER_PC | Current copper programme counter |
| `&HF0018` | COPPER_STATUS | Copper status |
| `&HF001C` | BLT_CTRL | Control (write 1 to start) |
| `&HF0020` | BLT_OP | Operation (0=copy, 1=fill, 2=line, 3=masked, 4=alpha, 5=Mode7, 6=colour expand, 7=scale) |
| `&HF0024` | BLT_SRC | Blitter source address |
| `&HF0028` | BLT_DST | Blitter destination address |
| `&HF002C` | BLT_WIDTH | Blitter width |
| `&HF0030` | BLT_HEIGHT | Blitter height |
| `&HF0034` | BLT_SRC_STRIDE | Source stride |
| `&HF0038` | BLT_DST_STRIDE | Destination stride |
| `&HF003C` | BLT_COLOR | Fill/line colour; for scale, packed destination size `(height << 16) | width` |
| `&HF0040` | BLT_MASK | Mask address |
| `&HF0044` | BLT_STATUS | Blitter status |
| `&HF0048` | VIDEO_RASTER_Y | Raster Y position |
| `&HF004C` | VIDEO_RASTER_HEIGHT | Raster height |
| `&HF0050` | VIDEO_RASTER_COLOR | Raster colour, BGRA |
| `&HF0054` | VIDEO_RASTER_CTRL | Raster control |
| `&HF0058`-`&HF0074` | BLT_MODE7_* | Mode7 U/V origin, deltas, and texture masks |
| `&HF0078` | VIDEO_PAL_INDEX | CLUT palette write index |
| `&HF007C` | VIDEO_PAL_DATA | CLUT palette data, `0x00RRGGBB` |
| `&HF0080` | VIDEO_COLOR_MODE | 0=RGBA32, 1=CLUT8 indexed |
| `&HF0084` | VIDEO_FB_BASE | Bus-memory framebuffer base |
| `&HF0088`-`&HF0487` | VIDEO_PAL_TABLE | 256 direct CLUT palette entries, 32 bits each |
| `&HF0488` | BLT_FLAGS | Extended blitter flags |
| `&HF048C` | BLT_FG | Blitter foreground colour |
| `&HF0490` | BLT_BG | Blitter background colour |
| `&HF0494` | BLT_MASK_MOD | Blitter mask modulo |
| `&HF0498` | BLT_MASK_SRCX | Blitter mask source X |

### 9.16 File I/O Registers (`&HF2200`-`&HF221F`)

Used by `BLOAD`, `LOAD`, `SAVE`, and `DIR`.

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF2200` | FILE_NAME_PTR | W | Pointer to NUL-terminated filename |
| `&HF2204` | FILE_DATA_PTR | W | Pointer to data buffer |
| `&HF2208` | FILE_DATA_LEN | W | Data length for write |
| `&HF220C` | FILE_CTRL | W | 1=read, 2=write, 3=list; write triggers operation |
| `&HF2210` | FILE_STATUS | R | 0=OK, 1=error |
| `&HF2214` | FILE_RESULT_LEN | R | Bytes read or listed |
| `&HF2218` | FILE_ERROR_CODE | R | 0=OK, 1=not found, 2=permission, 3=traversal |
| `&HF221C` | FILE_READ_MAX | W | One-shot read cap in bytes; a larger file is refused (no copy); 0=unbounded |

### 9.17 Paula DMA Audio Bridge (`&HF2260`-`&HF22AF`)

This Paula-style DMA shim is used by AROS audio and is also programmable from
guest code. It drives SoundChip flex-channel DAC output from guest sample
buffers.

| Address Range | Description |
|---------------|-------------|
| `&HF2260`-`&HF226F` | Channel 0 registers |
| `&HF2270`-`&HF227F` | Channel 1 registers |
| `&HF2280`-`&HF228F` | Channel 2 registers |
| `&HF2290`-`&HF229F` | Channel 3 registers |
| `&HF22A0` | AROS_AUD_DMACON - bit 15=set/clear, bits 0-3=channels |
| `&HF22A4` | AROS_AUD_STATUS - completion flags bits 0-3, write to clear |
| `&HF22A8` | AROS_AUD_INTENA - bit 15=set/clear, bits 0-3=channel interrupt enables |

Per-channel offsets:

| Offset | Name | Description |
|--------|------|-------------|
| `+&H00` | PTR | Sample pointer in guest RAM |
| `+&H04` | LEN | Length in words, 1 word = 2 bytes |
| `+&H08` | PER | Paula-style period, frequency = 3546895 / period |
| `+&H0C` | VOL | Volume, 0-64 |

### 9.18 Media Loader Registers (`&HF2300`-`&HF231F`)

Used by `SOUND PLAY` to load and play music files.

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF2300` | MEDIA_NAME_PTR | W | Pointer to NUL-terminated filename in memory |
| `&HF2304` | MEDIA_SUBSONG | W | Subsong index (0=default) |
| `&HF2308` | MEDIA_CTRL | W | Control: 1=play, 2=stop all |
| `&HF230C` | MEDIA_STATUS | R | 0=idle, 1=loading, 2=playing, 3=error |
| `&HF2310` | MEDIA_TYPE | R | Detected type: 0=none, 1=SID, 2=PSG, 3=TED, 4=AHX, 5=POKEY/SAP, 6=MOD, 7=WAV, 8=MIDI |
| `&HF2314` | MEDIA_ERROR | R | 0=ok, 1=not-found, 2=bad-format, 3=unsupported, 4=path-invalid, 5=too-large |

Staging buffer: `&H800000`-`&H80FFFF` (64 KB) - transient copy used for player
paths that read through guest memory. SID, PSG, POKEY/SAP, MOD, WAV, and MIDI/MUS are
loaded directly from the host-side file bytes; MOD and WAV payloads may exceed
the staging-buffer size. TED and AHX are staged at this address before their
player control registers are started, so those payloads are limited by
`MEDIA_STAGING_SIZE`.

`MEDIA_TYPE` is selected from the filename extension:

| Extensions | MEDIA_TYPE |
|------------|------------|
| `.sid` | 1=SID |
| `.ym`, `.ay`, `.sndh`, `.vtx`, `.vt`, `.pt3`, `.pt2`, `.pt1`, `.stc`, `.sqt`, `.asc`, `.ftc`, `.vgm`, `.vgz`, `.snd` | 2=PSG |
| `.ted`, `.prg` | 3=TED |
| `.ahx` | 4=AHX |
| `.sap` | 5=POKEY/SAP |
| `.mod` | 6=MOD |
| `.wav` | 7=WAV |
| `.mid`, `.midi`, `.mus` | 8=MIDI |

For PSG media, `.sndh` and `.snd` mean the Atari ST SNDH filename variants,
not an arbitrary raw sound container.

### 9.19 Program Executor Registers (`&HF2320`-`&HF233F`)

Used by `RUN "file"` to launch external files and by guest programmes that need
to request the same full reset path as the F10 hard-reset hotkey. The
hard-reset-to-BASIC control reloads BASIC through the bounded BASIC loader and
restores the terminal/front-buffer video profile, so it does not retain
direct-VRAM display state from launched flat programmes.

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF2320` | EXEC_NAME_PTR | W | Pointer to NUL-terminated filename in memory |
| `&HF2324` | EXEC_CTRL | W | Control: 1=execute, 2=boot EmuTOS, 3=boot AROS, 4=boot IExec, 5=hard reset to BASIC |
| `&HF2328` | EXEC_STATUS | R | 0=idle, 1=loading, 2=running, 3=error |
| `&HF232C` | EXEC_TYPE | R | 0=none, 1=IE32, 2=IE64, 3=6502, 4=M68K, 5=Z80, 6=x86, 7=EmuTOS, 8=script, 9=AROS, 10=IExec |
| `&HF2330` | EXEC_ERROR | R | 0=ok, 1=not-found, 2=unsupported, 3=path-invalid, 4=load-failed |
| `&HF2334` | EXEC_SESSION | R | Monotonic session counter (increments per request) |

### 9.20 Coprocessor Control Registers (`&HF2340`-`&HF238F`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF2340` | COPROC_CMD | W | Command: 1=START, 2=STOP, 3=ENQUEUE, 4=POLL, 5=WAIT |
| `&HF2344` | COPROC_CPU_TYPE | R/W | Worker type: 1=IE32, 2=IE64, 3=6502, 4=M68K, 5=Z80, 6=x86 |
| `&HF2348` | COPROC_CMD_STATUS | R | Last command status: 0=ok, 1=error |
| `&HF234C` | COPROC_CMD_ERROR | R | Error: 0=none, 1=invalid CPU, 2=not found, 3=path invalid, 4=load failed, 5=queue full, 6=no worker, 7=stale ticket |
| `&HF2350` | COPROC_TICKET | R/W | ENQUEUE output ticket; POLL/WAIT input ticket |
| `&HF2354` | COPROC_TICKET_STATUS | R | Ticket status: 0=pending, 1=running, 2=ok, 3=error, 4=timeout, 5=worker down |
| `&HF2358` | COPROC_OP | R/W | Service-defined operation code |
| `&HF235C` | COPROC_REQ_PTR | R/W | Request buffer address |
| `&HF2360` | COPROC_REQ_LEN | R/W | Request byte length |
| `&HF2364` | COPROC_RESP_PTR | R/W | Response buffer address |
| `&HF2368` | COPROC_RESP_CAP | R/W | Response buffer capacity |
| `&HF236C` | COPROC_TIMEOUT | R/W | WAIT timeout in milliseconds |
| `&HF2370` | COPROC_NAME_PTR | R/W | NUL-terminated worker filename address |
| `&HF2374` | COPROC_WORKER_STATE | R | Bitmask of running workers |
| `&HF2378` | COPROC_STATS_OPS | R | Total dispatched operations |
| `&HF237C` | COPROC_STATS_BYTES | R | Total request bytes dispatched |
| `&HF2380` | COPROC_IRQ_CTRL | R/W | Bit 0 enables completion IRQs |
| `&HF2384` | COPROC_DISPATCH_OVERHEAD | R | Calibrated dispatch overhead in nanoseconds |
| `&HF2388` | COPROC_COMPLETED_TICKET | R | Most recent completed ticket |

`&HF2380` is also named `SYS_GC_TRIGGER` in some include files, but the VM maps
that address to the coprocessor manager as `COPROC_IRQ_CTRL`.

### 9.21 Coprocessor Monitor Registers (`&HF23B0`-`&HF23BF`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF23B0` | COPROC_RING_DEPTH | R | Ring occupancy for the selected `COPROC_CPU_TYPE` |
| `&HF23B4` | COPROC_WORKER_UPTIME | R | Seconds since the selected worker started |
| `&HF23B8` | COPROC_STATS_RESET | W | Write 1 to clear global stats and busy buckets |
| `&HF23BC` | COPROC_BUSY_PCT | R | Aggregate worker busy percentage, 0-100 |

Write `COPROC_CPU_TYPE` before reading `COPROC_RING_DEPTH` or `COPROC_WORKER_UPTIME`.

### 9.22 SYSINFO Registers (`&HF2400`-`&HF24FF`)

SYSINFO is a read-only MMIO window for RAM-size discovery. Writes are ignored.
Low and high registers form unsigned 64-bit byte counts.

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF2400` | SYSINFO_TOTAL_RAM_LO | R | Low 32 bits of total guest RAM |
| `&HF2404` | SYSINFO_TOTAL_RAM_HI | R | High 32 bits of total guest RAM |
| `&HF2408` | SYSINFO_ACTIVE_RAM_LO | R | Low 32 bits of active CPU/profile visible RAM |
| `&HF240C` | SYSINFO_ACTIVE_RAM_HI | R | High 32 bits of active CPU/profile visible RAM |

---

## 10. Machine Code Interface

### POKE and PEEK

The simplest way to interact with hardware from BASIC is through POKE and PEEK:

```basic
POKE &HF0700, 65    : REM write 'A' to terminal
V = PEEK(&HF1000)   : REM read VGA mode register
```

POKE writes a 32-bit value; `POKE8` writes an 8-bit value; DOKE writes 16-bit; LOKE writes 32-bit (same as POKE).
PEEK reads 32-bit; `PEEK8(address)` reads an 8-bit value; DEEK reads 16-bit; LEEK reads 32-bit (same as PEEK).
`POKE` and `LOKE` require 4-byte aligned addresses; use `POKE8` for byte-oriented or unaligned MMIO registers.

### Direct Memory Access

BASIC can access mapped guest memory and MMIO addresses with `PEEK` and `POKE`.
VGA mode `&H13` exposes the legacy 320x200 VRAM window at `&HA0000`; the
general Video RAM region starts at `&H100000`.

```basic
REM Write directly to VGA VRAM
FOR I = 0 TO 63999
  POKE &HA0000 + I, I AND 255
NEXT I
```

### Register Conventions (IE64 Assembly)

If you write IE64 assembly routines callable from BASIC (via CALL or USR), the following register conventions apply:

| Register | Usage |
|----------|-------|
| R0 | Always zero (hardwired) |
| R1-R7 | Scratch (caller-saved) |
| R8, R9 | FP32 operands and results |
| R10-R15 | Scratch (some routines push/pop) |
| R16 | State base pointer (preserved) |
| R17 | Text/token stream pointer (preserved) |
| R22 | Temporary value (BASIC expressions) |
| R26 | Cached TERM_OUT address |
| R27 | Cached TERM_STATUS address |
| R28 | Return status code. `CALL` preserves this register; `USR` does not. |
| R31 | Hardware stack pointer |

`CALL` saves R14, R16, R17, R26, and R28. `USR` saves R14, R16, R17, and R26
only.

IE64 machine code can also read control register 15 with `MFCR` to obtain the
active CPU/profile visible RAM size in bytes. That control register is
read-only; `MTCR` to register 15 raises an illegal-instruction fault.

### FP32 Calling Convention

The floating-point library uses:
- **Input**: R8 (first operand), R9 (second operand)
- **Output**: R8 (result)
- **Clobbers**: R1-R7

Available routines: `fp_add`, `fp_sub`, `fp_mul`, `fp_div`, `fp_neg`, `fp_abs`, `fp_cmp`, `fp_int`, `fp_float`, `fp_sqr`, `fp_sin`, `fp_cos`, `fp_tan`, `fp_atn`, `fp_log`, `fp_exp`, `fp_pow`.

---

## 11. Runtime Errors

Runtime errors are reported through the statement control channel. `R28=3`
signals a runtime error inside the interpreter; `exec_line` returns that status
in `R8`. `raise_error` stores the error code in `ST_ERROR_FLAG`, stores the
current line in `ST_ERROR_LINE`, prints `?<message> ERROR IN <line>`, and stops
the current programme run.

Current structured errors include:

| Condition | Behaviour |
|-----------|-----------|
| Undefined line (GOTO/GOSUB) | `?UNDEFINED LINE ERROR IN <line>` |
| RETURN without GOSUB | `?RETURN WITHOUT GOSUB ERROR IN <line>` |
| NEXT without FOR | `?NEXT WITHOUT FOR ERROR IN <line>` |
| WEND without WHILE | Treated as stack mismatch; execution stops |
| LOOP without DO | Treated as stack mismatch; execution stops |
| Division by zero | `?DIVISION BY ZERO ERROR IN <line>` |
| Square root of negative | Returns 0 |
| Array bounds, misaligned 32-bit PEEK/POKE, bad FC arguments | `?FC ERROR IN <line>` |
| LEFT$/RIGHT$/MID$ illegal bounds | `?ILLEGAL QUANTITY ERROR IN <line>` |
| Duplicate DIM | `?REDIM ERROR IN <line>` |
| Standalone ELSE or unknown statement token | `?SYNTAX ERROR IN <line>` |
| INPUT buffer full | Extra typed characters are ignored |
| DATA exhausted | READ returns 0 |

Use `TRON` to trace line execution when debugging control-flow issues.

---

## 12. Example Programmes

### 12.1 Hello World

```basic
10 PRINT "Hello, World!"
20 END
```

### 12.2 Fibonacci Sequence

```basic
10 A = 0 : B = 1
20 FOR I = 1 TO 20
30   PRINT A
40   C = A + B
50   A = B
60   B = C
70 NEXT I
```

### 12.3 Guess the Number

```basic
10 N = INT(RND(1) * 100) + 1
20 PRINT "I'm thinking of a number between 1 and 100."
30 INPUT G
40 IF G < N THEN PRINT "Too low!" : GOTO 30
50 IF G > N THEN PRINT "Too high!" : GOTO 30
60 PRINT "Correct! You got it!"
```

### 12.4 Temperature Converter

```basic
10 PRINT "Temperature Converter"
20 PRINT "1. Celsius to Fahrenheit"
30 PRINT "2. Fahrenheit to Celsius"
40 INPUT C
50 ON C GOSUB 100, 200
60 END
100 INPUT T
110 PRINT T; "C = "; T * 9 / 5 + 32; "F"
120 RETURN
200 INPUT T
210 PRINT T; "F = "; (T - 32) * 5 / 9; "C"
220 RETURN
```

### 12.5 VGA Plasma Effect

```basic
10 SCREEN &H13
20 REM Set up rainbow palette
30 FOR I = 0 TO 255
40   PALETTE I, (SIN(I * PI / 128) * 31) + 32, 0, (COS(I * PI / 128) * 31) + 32
50 NEXT I
60 T = 0
70 FOR Y = 0 TO 199
80   FOR X = 0 TO 319
90     C = SIN(X / 16 + T) + SIN(Y / 8 + T) + SIN((X + Y) / 16 + T)
100    POKE &HA0000 + Y * 320 + X, INT((C + 3) * 42) AND 255
110  NEXT X
120 NEXT Y
130 T = T + 0.1
140 VSYNC
150 GOTO 70
```

### 12.6 Copper Rainbow Bars

```basic
10 SCREEN &H13
20 CLS
30 COPPER LIST &H50000
40 FOR I = 0 TO 199
50   COPPER WAIT I
60   R = INT((SIN(I * PI / 100) + 1) * 127)
70   G = INT((SIN(I * PI / 100 + 2) + 1) * 127)
80   B = INT((SIN(I * PI / 100 + 4) + 1) * 127)
90   COPPER MOVE &HF0050, R * 65536 + G * 256 + B
100 NEXT I
110 COPPER END
120 COPPER ON
130 GOTO 130
```

### 12.7 ULA Drawing

```basic
10 ULA ON
20 ULA BORDER 2
30 ULA CLS
40 ULA INK 7 : ULA PAPER 0
50 REM Draw a box outline
60 FOR X = 50 TO 200
70   ULA PLOT X, 40
80   ULA PLOT X, 150
90 NEXT X
100 FOR Y = 40 TO 150
110   ULA PLOT 50, Y
120   ULA PLOT 200, Y
130 NEXT Y
140 ULA ATTR 10, 8, &H45
```

### 12.8 Sound Effects

```basic
10 REM Laser sound
20 FOR F = 2000 TO 200 STEP -20
30   SOUND 0, F, 200, 0, 128
40   GATE 0, ON
50   FOR D = 1 TO 50 : NEXT D
60 NEXT F
70 GATE 0, OFF
80 REM Explosion
90 SOUND 1, 100, 255, 3
100 ENVELOPE 1, 1, 200, 0, 500
110 GATE 1, ON
```

### 12.9 Voodoo Triangle

```basic
10 VOODOO ON
20 VOODOO DIM 320, 200
30 VOODOO CLEAR 0
40 REM Define a white triangle
50 VERTEX A 160, 20
60 VERTEX B 60, 180
70 VERTEX C 260, 180
80 TRIANGLE
90 VOODOO SWAP
100 GOTO 100
```

### 12.10 Bubble Sort

```basic
10 N = 10
20 DIM A(9)
30 REM Fill with random values
40 FOR I = 0 TO N - 1
50   A(I) = INT(RND(1) * 100)
60 NEXT I
70 PRINT "Before: ";
80 FOR I = 0 TO N - 1 : PRINT A(I); : NEXT I
90 PRINT
100 REM Bubble sort
110 FOR I = 0 TO N - 2
120   FOR J = 0 TO N - 2 - I
130     IF A(J) > A(J + 1) THEN SWAP A(J), A(J + 1)
140   NEXT J
150 NEXT I
160 PRINT "After:  ";
170 FOR I = 0 TO N - 1 : PRINT A(I); : NEXT I
180 PRINT
```

### 12.11 String Manipulation

```basic
10 A$ = "Hello"
20 B$ = "World"
30 C$ = A$ + ", " + B$ + "!"
40 PRINT C$
50 PRINT "Length: "; LEN(C$)
60 PRINT "Left 5: "; LEFT$(C$, 5)
70 PRINT "Right 6: "; RIGHT$(C$, 6)
80 PRINT "Mid 8,5: "; MID$(C$, 8, 5)
90 PRINT "ASCII of H: "; ASC(A$)
```

### 12.12 DO/LOOP Demonstration

```basic
10 REM Count with DO/LOOP WHILE
20 X = 1
30 DO
40   PRINT X;
50   INC X
60 LOOP WHILE X < 11
70 PRINT
80 REM Count down with LOOP UNTIL
90 Y = 10
100 DO
110   PRINT Y;
120   DEC Y
130 LOOP UNTIL Y < 1
140 PRINT
```

---

## 13. Appendices

### Appendix A: ASCII Table

| Dec | Hex | Char | Dec | Hex | Char | Dec | Hex | Char |
|-----|-----|------|-----|-----|------|-----|-----|------|
| 32 | 20 | (space) | 64 | 40 | @ | 96 | 60 | ` |
| 33 | 21 | ! | 65 | 41 | A | 97 | 61 | a |
| 34 | 22 | " | 66 | 42 | B | 98 | 62 | b |
| 35 | 23 | # | 67 | 43 | C | 99 | 63 | c |
| 36 | 24 | $ | 68 | 44 | D | 100 | 64 | d |
| 37 | 25 | % | 69 | 45 | E | 101 | 65 | e |
| 38 | 26 | & | 70 | 46 | F | 102 | 66 | f |
| 39 | 27 | ' | 71 | 47 | G | 103 | 67 | g |
| 40 | 28 | ( | 72 | 48 | H | 104 | 68 | h |
| 41 | 29 | ) | 73 | 49 | I | 105 | 69 | i |
| 42 | 2A | * | 74 | 4A | J | 106 | 6A | j |
| 43 | 2B | + | 75 | 4B | K | 107 | 6B | k |
| 44 | 2C | , | 76 | 4C | L | 108 | 6C | l |
| 45 | 2D | - | 77 | 4D | M | 109 | 6D | m |
| 46 | 2E | . | 78 | 4E | N | 110 | 6E | n |
| 47 | 2F | / | 79 | 4F | O | 111 | 6F | o |
| 48 | 30 | 0 | 80 | 50 | P | 112 | 70 | p |
| 49 | 31 | 1 | 81 | 51 | Q | 113 | 71 | q |
| 50 | 32 | 2 | 82 | 52 | R | 114 | 72 | r |
| 51 | 33 | 3 | 83 | 53 | S | 115 | 73 | s |
| 52 | 34 | 4 | 84 | 54 | T | 116 | 74 | t |
| 53 | 35 | 5 | 85 | 55 | U | 117 | 75 | u |
| 54 | 36 | 6 | 86 | 56 | V | 118 | 76 | v |
| 55 | 37 | 7 | 87 | 57 | W | 119 | 77 | w |
| 56 | 38 | 8 | 88 | 58 | X | 120 | 78 | x |
| 57 | 39 | 9 | 89 | 59 | Y | 121 | 79 | y |
| 58 | 3A | : | 90 | 5A | Z | 122 | 7A | z |
| 59 | 3B | ; | 91 | 5B | [ | 123 | 7B | { |
| 60 | 3C | < | 92 | 5C | \ | 124 | 7C | \| |
| 61 | 3D | = | 93 | 5D | ] | 125 | 7D | } |
| 62 | 3E | > | 94 | 5E | ^ | 126 | 7E | ~ |
| 63 | 3F | ? | 95 | 5F | _ | 127 | 7F | DEL |

### Appendix B: Colour Charts

#### VGA Default Palette (First 16 Colours)

| Index | Colour | R | G | B |
|-------|--------|---|---|---|
| 0 | Black | 0 | 0 | 0 |
| 1 | Blue | 0 | 0 | 42 |
| 2 | Green | 0 | 42 | 0 |
| 3 | Cyan | 0 | 42 | 42 |
| 4 | Red | 42 | 0 | 0 |
| 5 | Magenta | 42 | 0 | 42 |
| 6 | Brown | 42 | 21 | 0 |
| 7 | Light Grey | 42 | 42 | 42 |
| 8 | Dark Grey | 21 | 21 | 21 |
| 9 | Light Blue | 21 | 21 | 63 |
| 10 | Light Green | 21 | 63 | 21 |
| 11 | Light Cyan | 21 | 63 | 63 |
| 12 | Light Red | 63 | 21 | 21 |
| 13 | Light Magenta | 63 | 21 | 63 |
| 14 | Yellow | 63 | 63 | 21 |
| 15 | White | 63 | 63 | 63 |

#### ULA (ZX Spectrum) Colours

| Index | Normal | Bright |
|-------|--------|--------|
| 0 | Black | Black |
| 1 | Blue | Bright Blue |
| 2 | Red | Bright Red |
| 3 | Magenta | Bright Magenta |
| 4 | Green | Bright Green |
| 5 | Cyan | Bright Cyan |
| 6 | Yellow | Bright Yellow |
| 7 | White | Bright White |

#### TED Colour System

The TED uses a luma-chroma system with 16 hues and 8 luminance levels, yielding 121 unique colours (hue 0 at all luminances produces shades of grey).

Colour value = `(luminance << 4) | hue` where luminance is 0-7 and hue is 0-15.

#### ANTIC/GTIA Colour System

Atari colours use a hue-luminance system: `(hue << 4) | luminance`. 16 hues x 16 luminance levels = 256 colour values. Even luminance values only are visible (128 unique displayed colours).

### Appendix C: Token Table

#### Core Tokens (`&H80`-`&HE1`)

This table uses the token constants from `sdk/include/ehbasic_tokens.inc` and
the keyword mappings from `sdk/include/ehbasic_tokenizer.inc`. Several entries
are deliberate aliases because the core token range is full.

The token space is fully assigned. Composite comparison operators use the
existing `<`/`>` tokens followed by a raw marker byte; see
`ehbasic_token_map.md` for the token-space audit and migration notes.

`TK_WIDTH` is a historical symbol name from the original EhBASIC token space.
In this port the live keyword mapped to that token is `BLOAD`; the original
EhBASIC `WIDTH` command is not implemented.

| Hex | Token | Keyword | Type |
|-----|-------|---------|------|
| 80 | TK_END | END | Statement |
| 81 | TK_FOR | FOR | Statement |
| 82 | TK_NEXT | NEXT | Statement |
| 83 | TK_DATA | DATA | Statement |
| 84 | TK_INPUT | INPUT | Statement |
| 85 | TK_DIM | DIM | Statement |
| 86 | TK_READ | READ | Statement |
| 87 | TK_LET | LET | Statement |
| 88 | TK_DEC | DEC | Statement |
| 89 | TK_GOTO | GOTO | Statement |
| 8A | TK_RUN | RUN | Statement |
| 8B | TK_IF | IF | Statement |
| 8C | TK_RESTORE | RESTORE | Statement |
| 8D | TK_GOSUB | GOSUB | Statement |
| 8E | TK_RETURN | RETURN | Statement |
| 8F | TK_REM | REM | Statement |
| 90 | TK_STOP | STOP | Statement |
| 91 | TK_ON | ON | Statement |
| 92 | TK_NULL | TRON | Statement alias (`NULL` symbol reused) |
| 93 | TK_INC | INC | Statement |
| 94 | TK_WAIT | WAIT | Statement |
| 95 | TK_LOAD | LOAD | Statement |
| 96 | TK_SAVE | SAVE | Statement |
| 97 | TK_DEF | DEF/TROFF | Statement/alias |
| 98 | TK_POKE | POKE | Statement |
| 99 | TK_DOKE | DOKE | Statement |
| 9A | TK_LOKE | LOKE | Statement |
| 9B | TK_CALL | CALL | Statement |
| 9C | TK_DO | DO | Statement |
| 9D | TK_LOOP | LOOP | Statement |
| 9E | TK_PRINT | PRINT | Statement |
| 9F | TK_CONT | CONT | Statement |
| A0 | TK_LIST | LIST | Statement |
| A1 | TK_CLEAR | CLEAR | Statement |
| A2 | TK_NEW | NEW | Statement |
| A3 | TK_WIDTH | BLOAD | Statement |
| A4 | TK_GET | GET | Statement |
| A5 | TK_SWAP | SWAP | Statement |
| A6 | TK_BITSET | BITSET | Statement |
| A7 | TK_BITCLR | BITCLR | Statement |
| A8 | TK_TAB | TAB | Reserved token |
| A9 | TK_TO | TO | Keyword |
| AA | TK_FN | FN | Keyword |
| AB | TK_ELSE | ELSE | Keyword |
| AC | TK_THEN | THEN | Keyword |
| AD | TK_NOT | NOT | Operator |
| AE | TK_STEP | STEP | Keyword |
| AF | TK_UNTIL | UNTIL/WEND | Keyword/statement alias |
| B0 | TK_WHILE | WHILE | Statement |
| B1 | TK_PLUS | + | Operator |
| B2 | TK_MINUS | - | Operator |
| B3 | TK_MULT | * | Operator |
| B4 | TK_DIV | / | Operator |
| B5 | TK_POWER | ^ | Operator |
| B6 | TK_AND | AND | Operator |
| B7 | TK_EOR | EOR | Operator |
| B8 | TK_OR | OR | Operator |
| B9 | TK_RSHIFT | >> | Operator |
| BA | TK_LSHIFT | << | Operator |
| BB | TK_GT | > | Operator |
| BC | TK_EQUAL | = | Operator |
| BD | TK_LT | < | Operator |
| BE | TK_SGN | SGN | Function |
| BF | TK_INT | INT | Function |
| C0 | TK_ABS | ABS | Function |
| C1 | TK_USR | USR | Function |
| C2 | TK_FRE | FRE | Function |
| C3 | TK_POS | POS | Function |
| C4 | TK_SQR | SQR | Function |
| C5 | TK_RND | RND | Function |
| C6 | TK_LOG | LOG | Function |
| C7 | TK_EXP | EXP | Function |
| C8 | TK_COS | COS | Function |
| C9 | TK_SIN | SIN | Function |
| CA | TK_TAN | TAN | Function |
| CB | TK_ATN | ATN | Function |
| CC | TK_PEEK | PEEK | Function |
| CD | TK_DEEK | DEEK | Function |
| CE | TK_LEEK | LEEK | Function |
| CF | TK_SADD | SADD | Function |
| D0 | TK_LEN | LEN | Function |
| D1 | TK_STRS | STR$ | Function |
| D2 | TK_VAL | VAL | Function |
| D3 | TK_ASC | ASC | Function |
| D4 | TK_UCASES | UCASE$ | Function |
| D5 | TK_LCASES | LCASE$ | Function |
| D6 | TK_CHRS | CHR$ | Function |
| D7 | TK_HEXS | HEX$ | Function |
| D8 | TK_BINS | BIN$ | Function |
| D9 | TK_BITTST | BITTST | Function |
| DA | TK_MAX | MAX | Function |
| DB | TK_MIN | MIN | Function |
| DC | TK_PI | PI | Function |
| DD | TK_TWOPI | TWOPI | Function |
| DE | TK_VPTR | VARPTR | Function |
| DF | TK_LEFTS | LEFT$ | Function |
| E0 | TK_RIGHTS | RIGHT$ | Function |
| E1 | TK_MIDS | MID$ | Function |

`DIR` is implemented as an immediate REPL command and intentionally has no token entry.

`HOST` is also intentionally untokenized. It is recognized as a raw
statement in `exec_line` using the same word-boundary technique as
`COSTART`, `COSTOP`, and `COWAIT`. An earlier draft assigned `HOST`
the same byte (`0xDE`) as `TK_VPTR`, which caused `HOST` in expression
context to be mistaken for `VARPTR` and `VARPTR` in statement context
to invoke the `HOST` dispatcher. The token table now reserves `0xDE`
for `TK_VPTR` alone; the statement dispatch slot for `0xDE` routes to
`exec_do_unknown`.

#### Hardware Extension Tokens (`&HE2`-`&HFF`)

| Hex | Token | Keyword | Type |
|-----|-------|---------|------|
| E2 | TK_SCREEN | SCREEN | Statement |
| E3 | TK_CLS | CLS | Statement |
| E4 | TK_PLOT | PLOT | Statement |
| E5 | TK_PALETTE | PALETTE | Statement |
| E6 | TK_VSYNC_CMD | VSYNC | Statement |
| E7 | TK_LOCATE | LOCATE | Statement |
| E8 | TK_COLOR | COLOR | Statement |
| E9 | TK_LINE_CMD | LINE | Statement |
| EA | TK_CIRCLE | CIRCLE | Statement |
| EB | TK_BOX | BOX | Statement |
| EC | TK_SCROLL_CMD | SCROLL | Statement |
| ED | TK_COPPER | COPPER | Statement |
| EE | TK_BLIT | BLIT | Statement |
| EF | TK_SOUND | SOUND | Statement |
| F0 | TK_ENVELOPE | ENVELOPE | Statement |
| F1 | TK_GATE | GATE | Statement |
| F2 | TK_ULA | ULA | Statement |
| F3 | TK_TED_CMD | TED | Statement |
| F4 | TK_ANTIC | ANTIC | Statement |
| F5 | TK_GTIA | GTIA | Statement |
| F6 | TK_VOODOO | VOODOO | Statement |
| F7 | TK_PSG_CMD | PSG | Statement |
| F8 | TK_SID_CMD | SID | Statement |
| F9 | TK_POKEY_CMD | POKEY | Statement |
| FA | TK_AHX | AHX | Statement |
| FB | TK_SAP | SAP | Statement |
| FC | TK_ZBUFFER | ZBUFFER | Statement |
| FD | TK_VERTEX | VERTEX | Statement |
| FE | TK_TRIANGLE | TRIANGLE | Statement |
| FF | TK_TEXTURE | TEXTURE | Statement |

**Token summary**: 128 tokens total (98 core + 30 hardware extensions).

---

*EhBASIC IE64 is part of the Intuition Engine project.*
*Original EhBASIC by Lee Davison. IE64 port (c) 2024-2026 Zayn Otley.*
*Licence: GPLv3 or later.*
