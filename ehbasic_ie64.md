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
11. [Error Messages](#11-error-messages)
12. [Example Programs](#12-example-programs)
13. [Appendices](#13-appendices)

---

## 1. Introduction

EhBASIC IE64 is a port of Lee Davison's Enhanced BASIC (EhBASIC) to the Intuition Engine's IE64 RISC processor. The original EhBASIC was written in 6502 assembly and later ported to the Motorola 68000; this version is a ground-up reimplementation in IE64 assembly, preserving the language semantics whilst taking advantage of the IE64's 64-bit register file and compare-and-branch instructions.

The Intuition Engine is a retro-inspired virtual machine that emulates multiple CPUs (6502, Z80, M68K, IE32, IE64) alongside authentic video and audio hardware: VGA with copper coprocessor and blitter, ULA (ZX Spectrum), TED (Commodore 16/Plus4), ANTIC/GTIA (Atari 8-bit), Voodoo 3DFX, and a full complement of sound chips (SoundChip, PSG/AY-3-8910, SID/MOS 6581, POKEY, TED audio, AHX tracker). EhBASIC IE64 provides direct access to all of this hardware through extension commands.

### Floating-Point Arithmetic: Important Note

**This port uses IEEE 754 single-precision (FP32) arithmetic.** This differs from other EhBASIC ports: the original 6502 version and the 68K port both use a custom 6-byte packed floating-point format with a 7-bit biased exponent, providing approximately 9 decimal digits of precision.

EhBASIC IE64's FP32 representation gives:

- **Precision**: ~7 decimal digits (24-bit mantissa: 23 explicit + 1 implicit)
- **Range**: approximately +/-3.4 x 10^38
- **Special values**: +/-0, +/-Infinity (no NaN)

The trade-off is clear: FP32 sacrifices roughly 2 digits of precision compared to the original format, but gains IEEE 754 compatibility, hardware-friendly bit layouts, and simpler register handling (the 32-bit value sits in the lower half of a 64-bit IE64 register, leaving the upper 32 bits free for bit manipulation).

For most BASIC programs, 7 digits is more than sufficient. Financial calculations or programs that chain many arithmetic operations may notice rounding differences compared to the original EhBASIC.

---

## 2. Getting Started

### Building

EhBASIC IE64 is assembled from source and optionally embedded into the Intuition Engine binary.

```bash
# Assemble the BASIC interpreter
bin/ie64asm assembler/ehbasic_ie64.asm

# Build the VM with embedded BASIC
make basic
```

The `make basic` target:
1. Assembles `assembler/ehbasic_ie64.asm` into `assembler/ehbasic_ie64.ie64`
2. Builds the Intuition Engine binary with the `embed_basic` build tag, which embeds the BASIC binary via Go's `//go:embed` directive

### Running

```bash
# Run with embedded BASIC image
./IntuitionEngine -basic

# Run with a custom BASIC binary
./IntuitionEngine -basic-image path/to/custom.ie64
```

On startup, EhBASIC IE64 displays a banner and the `Ready` prompt:

```
EhBASIC IE64 v1.0
Ready
```

### Your First Program

Type lines with line numbers to store them in the program:

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
| `RUN` | Execute the stored program |
| `LIST` | Display the program listing |
| `NEW` | Clear the program from memory |
| Line number followed by text | Store or replace a program line |
| Line number alone | Delete that line |

---

## 3. Language Reference

### 3.1 Data Types

EhBASIC IE64 supports two data types:

**Numeric** — IEEE 754 single-precision floating-point (FP32). All numeric values, including integers, are stored as FP32. Integer operations truncate the fractional part where needed. Range: approximately +/-3.4 x 10^38. Precision: ~7 decimal digits.

**String** — Null-terminated byte sequences stored on a string heap. Strings are allocated linearly; there is no garbage collection. String variables are identified by a trailing `$` suffix.

### 3.2 Variables

Variable names consist of letters (A-Z) and digits (0-9). The first character must be a letter. Names are case-insensitive (internally stored as uppercase). Only the first four characters are significant; longer names are silently truncated.

```basic
X = 42
NAME$ = "ZAYN"
LONGVARIABLENAME = 100   : REM same as LONG
```

**Numeric variables** hold FP32 values. An uninitialised variable returns 0.

**String variables** are indicated by a `$` suffix and hold a pointer to heap-allocated string data. An uninitialised string variable returns an empty string.

**Arrays** are declared with `DIM` and support one or two dimensions. Indices are zero-based:

```basic
DIM A(10)        : REM 11 elements: A(0) through A(10)
DIM B(5, 5)      : REM 6x6 = 36 elements
```

Referencing an undeclared array auto-creates it with 11 elements (1D) or 11x11 elements (2D).

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

Comparisons return -1 (true) or 0 (false).

#### Logical/Bitwise Operators

| Operator | Description |
|----------|-------------|
| `AND` | Bitwise AND |
| `OR` | Bitwise OR |
| `EOR` | Bitwise exclusive OR |
| `NOT` | Bitwise complement |

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
5. `+`, `-` (addition, subtraction)
6. `*`, `/` (multiplication, division)
7. `^` (exponentiation)
8. `-` (unary negation)
9. Parentheses, function calls, literals, variables

### 3.5 Numeric Literals

```basic
42          : REM integer
3.14159     : REM floating-point
.5          : REM leading dot (0.5)
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

Statements are listed alphabetically. Hardware extension statements (video, audio, system) are grouped under their primary keyword.

### ANTIC

Control the Atari 8-bit ANTIC video display processor.

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

- `address` — memory address
- `bit` — bit number (0-7)

**Example:**
```basic
BITCLR &HF0700, 3     : REM clear bit 3
```

### BITSET

Set a bit in a byte at a memory address.

```
BITSET address, bit
```

- `address` — memory address
- `bit` — bit number (0-7)

**Example:**
```basic
BITSET &HF0700, 3     : REM set bit 3
```

### BLIT

Blitter hardware operations for fast block transfers, fills, and line drawing.

```
BLIT COPY src, dst, w, h [,srcstride, dststride]
BLIT FILL dst, w, h, colour [,stride]
BLIT LINE x1, y1, x2, y2, colour, stride
BLIT MEMCOPY src, dst, len
BLIT WAIT
```

**BLIT COPY** — Copy a rectangular block of pixels from source to destination.

**BLIT FILL** — Fill a rectangular area with a solid colour.

**BLIT LINE** — Draw a line using the blitter hardware.

**BLIT MEMCOPY** — Copy a contiguous block of bytes.

**BLIT WAIT** — Poll until the blitter has finished its current operation (includes timeout).

**Example:**
```basic
REM Fill a 100x50 box at VGA VRAM offset 1000
BLIT FILL &HA0000 + 1000, 100, 50, 15
BLIT WAIT
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

Program text is preserved; only runtime state is cleared.

### CALL

Call a machine code subroutine at the given address. The address is evaluated as an expression.

```
CALL addr
```

The BASIC interpreter saves its internal registers (R14, R16, R17, R26) before calling the routine and restores them on return. The called routine must end with `rts`.

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
- Mode 12h: fills all four bitplanes
- Text mode: fills with spaces and attribute 0x07

### COLOR

Set the text-mode foreground and background colours.

```
COLOR foreground [,background]
```

Values are 0-15 (standard VGA text attributes).

### CONT

Continue execution after a STOP statement. CONT resumes from the point where the program was interrupted.

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
COPPER MOVE reg, value      Emit MOVE instruction at build pointer
COPPER END                  Emit END instruction at build pointer
```

**Example:**
```basic
COPPER LIST &H50000
COPPER WAIT 100
COPPER MOVE &HF0050, &HFF0000    : REM set raster colour
COPPER END
COPPER ON
```

### DATA

Define inline data values to be read by `READ`.

```
DATA value [,value ...]
```

Values are numeric literals separated by commas. DATA statements are skipped during normal execution and only consumed by READ.

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
```

Indices are zero-based: `DIM A(10)` creates 11 elements (0 through 10).

Two-dimensional arrays use row-major order: `DIM B(5,3)` creates a 6x4 array.

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

### END

Terminate program execution.

```
END
```

### ENVELOPE

Set the ADSR envelope for a flexible audio channel.

```
ENVELOPE channel, attack, decay, sustain, release
```

- `channel` — 0 to 3
- `attack` — attack time in milliseconds
- `decay` — decay time in milliseconds
- `sustain` — sustain level (0-255)
- `release` — release time in milliseconds

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

Control the Atari GTIA graphics controller.

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
INPUT variable [,variable ...]
```

Each variable prompts the user for input. Numeric variables parse the input as a numeric expression.

**Example:**
```basic
10 INPUT X
20 PRINT "You entered: "; X
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

Display the stored program.

```
LIST
```

`LIST` is handled by the REPL command parser in immediate mode. It is not dispatched from tokenised program lines.

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

Clear the program from memory.

```
NEW
```

`NEW` is handled by the REPL command parser in immediate mode. It is not dispatched from tokenised program lines.

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

- `index` — palette entry (0-255)
- `red`, `green`, `blue` — colour components (0-63, VGA 6-bit DAC)

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

Both address and value are evaluated as expressions and truncated to integers. `POKE` stores 32 bits; `POKE8` stores the low 8 bits.

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

Control the PSG (AY-3-8910/YM2149) programmable sound generator.

```
PSG channel, freq, vol           Set channel frequency and volume
PSG PLUS ON                      Enable PSG+ enhanced mode
PSG PLUS OFF                     Disable PSG+ mode
PSG PLAY addr [,len]             Play PSG music data
PSG STOP                         Stop playback
PSG MIXER value                  Set mixer register (tone/noise control)
PSG ENVELOPE shape [,period]     Set envelope shape and optional period
```

### READ

Read the next value from DATA statements.

```
READ variable [,variable ...]
```

Values are read in program order across all DATA statements. Use RESTORE to reset the pointer.

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

Reset the DATA read pointer to the beginning of the program's DATA statements.

```
RESTORE
```

### RETURN

Return from a GOSUB subroutine. See [GOSUB...RETURN](#gosubreturn).

### RUN

Execute the stored program from the beginning.

```
RUN
```

`RUN` is handled by the REPL command parser in immediate mode. It is not dispatched from tokenised program lines.

### SAP

Control the SAP (Slight Atari Player) music player.

```
SAP PLAY addr [,len [,subsong]]   Play SAP module
SAP STOP                          Stop playback
```

### SCREEN

Set the VGA video mode.

```
SCREEN mode        Set VGA mode and enable display
SCREEN ON          Enable VGA display
SCREEN OFF         Disable VGA display
```

Supported modes:
- `&H13` — 320x200, 256 colours (Mode 13h)
- `&H12` — 640x480, 16 colours (Mode 12h, planar)
- `3` — 80x25 text mode

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
SID VOICE num, freq, pw, ctrl, ad, sr   Program a voice (num 1-3)
SID VOLUME vol                           Set master volume (0-15)
SID FILTER cutoff, resonance, routing, mode   Configure filter
SID PLUS ON                              Enable SID+ enhanced mode
SID PLUS OFF                             Disable SID+ mode
SID PLAY addr, len [,subsong]            Play SID music file
SID STOP                                 Stop playback
```

**SID VOICE** parameters:
- `num` — voice number (1-3)
- `freq` — 16-bit frequency value
- `pw` — 12-bit pulse width
- `ctrl` — control register (gate, waveform select, sync, ring mod)
- `ad` — attack/decay (upper nibble = attack, lower = decay)
- `sr` — sustain/release (upper nibble = sustain, lower = release)

### SOUND

Configure a flexible audio channel.

```
SOUND channel, freq, vol [,wave [,duty]]
SOUND FILTER cutoff, resonance, type
```

- `channel` — 0 to 3
- `freq` — frequency in Hz
- `vol` — volume (0-255)
- `wave` — waveform type (0=square, 1=triangle, 2=sine, 3=noise, 4=sawtooth)
- `duty` — duty cycle for square wave (0-255, 128=50%)

**SOUND FILTER** sets the global audio filter:
- `cutoff` — cutoff frequency (0-255, exponential 20Hz-20kHz)
- `resonance` — resonance (0-255)
- `type` — filter type (0=off, 1=lowpass, 2=highpass, 3=bandpass)

**SOUND REVERB** configures the reverb effect:
```
SOUND REVERB mix, decay
```
- `mix` — dry/wet mix (0-255)
- `decay` — decay time (0-255)

**SOUND OVERDRIVE** sets the overdrive (distortion) amount:
```
SOUND OVERDRIVE amount
```
- `amount` — drive amount (0-255)

**SOUND NOISE** sets noise mode using a channel-qualified command:
```
SOUND NOISE channel, mode
```
- `channel` — channel number
- `mode` — noise mode value

**SOUND WAVE** sets the waveform type for a flexible channel:
```
SOUND WAVE channel, type
```
- `channel` — 0 to 3
- `type` — waveform type

**SOUND SWEEP** configures a pitch sweep on a flexible channel:
```
SOUND SWEEP channel, enable, period, shift
```
- `enable` — 1=on, 0=off
- `period` — sweep period
- `shift` — sweep shift amount

Packed as `enable | (period << 8) | (shift << 16)` in the FLEX_OFF_SWEEP register.

**SOUND SYNC** sets the hard-sync source for a channel:
```
SOUND SYNC channel, source
```
- `channel` — 0 to 3
- `source` — source channel number

**SOUND RINGMOD** sets the ring modulation source for a channel:
```
SOUND RINGMOD channel, source
```
- `channel` — 0 to 3
- `source` — source channel number

**Example:**
```basic
SOUND 0, 440, 200, 0, 128    : REM 440Hz square wave, 50% duty
ENVELOPE 0, 10, 50, 200, 100
GATE 0, ON
SOUND REVERB 100, 200        : REM add reverb
```

### STOP

Halt program execution. Saves the current line and text pointers so that CONT can resume execution from this point.

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

Control the TED (Commodore 16/Plus4) video and audio hardware.

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
TED STOP                     Stop playback
```

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
```

**VOODOO TRICOLOR** sets the start colour registers (START_R, START_G, START_B, optionally START_A) used by the triangle rasteriser:
```basic
VOODOO TRICOLOR 255, 0, 0         : REM solid red
VOODOO TRICOLOR 128, 128, 0, 200  : REM with alpha
```

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

### COS

```
COS(x)
```

Returns the cosine of `x` (in radians).

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
```

Reads a 32-bit value from the given memory address.

**Example:**
```basic
PRINT PEEK(&HF1000)     : REM read VGA mode register
```

### PI

```
PI
```

Returns the constant pi (approximately 3.1415927). No parentheses required.

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

Returns the square root of `x`. Domain: `x >= 0`.

### STATUS (Chip)

Query the playback status of a sound chip or tracker engine.

```
PSG STATUS
SID STATUS
POKEY STATUS
TED STATUS
AHX STATUS
SAP STATUS
```

Returns 1 if the chip is currently playing, 0 otherwise. Used as an expression (function context).

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

### USR

```
USR(n)
```

Calls a user-defined machine code routine at address `n` and returns the result. The address is evaluated, the routine is called via register-indirect JSR, and the value of R8 on return is converted to FP32 and returned as the function result.

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

---

## 6. Video Programming Guide

The Intuition Engine provides multiple video subsystems, all accessible from BASIC. Each subsystem has its own display buffer, resolution, and colour model.

### 6.1 VGA Modes and VRAM Layout

The VGA subsystem supports three modes:

| Mode | Resolution | Colours | VRAM Address | Size |
|------|-----------|---------|-------------|------|
| `&H13` | 320x200 | 256 (palette) | `&HA0000` | 64,000 bytes |
| `&H12` | 640x480 | 16 (planar) | `&HA0000` | 4 x 38,400 bytes |
| `3` | 80x25 text | 16 fg + 8 bg | `&HB8000` | 4,000 bytes |

**Mode 13h** (320x200x256) is the simplest: each byte at `&HA0000 + y*320 + x` is a palette index. This is the default mode for PLOT, LINE, BOX, and CIRCLE.

**Mode 12h** (640x480x16) uses four bitplanes. Each pixel requires one bit from each plane. Use the blitter for drawing operations.

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
| WAIT | `0x80000000 \| (scanline << 16)` | Wait until raster reaches scanline |
| MOVE | `(register << 16) \| (value & 0xFFFF)` | Write value to register |
| END | `0xFFFFFFFF` | End of copper list |
| SETBASE | `0x80000000 \| (addr >> 2)` | Set register base for subsequent MOVEs |

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

The blitter performs hardware-accelerated block operations: copy, fill, and line drawing. It operates on rectangular regions defined by source/destination addresses, width, height, and stride.

```basic
REM Copy 100x50 block
BLIT COPY &HA0000, &HA0000 + 32000, 100, 50

REM Fill rectangle with colour 4
BLIT FILL &HA0000 + 1000, 200, 100, 4

REM Draw a line
BLIT LINE 0, 0, 319, 199, 15, 320

REM Wait for completion
BLIT WAIT
```

### 6.4 ULA (ZX Spectrum) Programming

The ULA emulates the ZX Spectrum's 256x192 pixel display with a unique non-linear VRAM layout.

**VRAM layout** at `&H4000`:
- Bitmap: 6,144 bytes (three 2KB thirds, each with interleaved character rows)
- Attributes: 768 bytes at `&H5800` (32x24 cells, each 8x8 pixels)

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

The TED emulates the Commodore 16/Plus4 video chip with 40x25 text mode and 121 colours (16 hues x 8 luminances, minus duplicates).

```basic
TED ON
TED MODE 0                 : REM text mode
TED COLOR 0, 0             : REM black background and border
TED VIDEO &H50000          : REM set video matrix address
TED CHAR &H51000           : REM set character ROM address
TED CLS
```

### 6.6 ANTIC/GTIA Programming

ANTIC and GTIA together form the Atari 8-bit video system. ANTIC handles display list execution and character/bitmap modes; GTIA handles colour registers, player/missile graphics (sprites), and priority.

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

The main audio engine provides four fixed-waveform channels (square, triangle, sine, noise) plus four flexible channels with selectable waveforms, plus a sawtooth channel. All channels support ADSR envelopes, pulse-width modulation, sync, ring modulation, and global effects (filter, overdrive, reverb).

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

**Playing .ay music files:**
```basic
PSG PLAY &H100000, 4096   : REM play PSG data from VRAM
```

### 7.3 SID (MOS 6581/8580)

The SID emulates the Commodore 64's famous MOS Technology 6581 (and later 8580) sound chip. Three voices with four selectable waveforms (triangle, sawtooth, pulse, noise), ring modulation, oscillator sync, and a multi-mode filter (lowpass, bandpass, highpass).

```basic
REM Program voice 1: 440Hz, 50% pulse, gate on, ADSR
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

The TED audio section provides two tone generators and a noise generator, emulating the Commodore 16/Plus4 sound capabilities.

```basic
TED TONE 1, 500            : REM voice 1 at 500Hz divider
TED VOL 8                  : REM volume 8
TED NOISE ON               : REM enable noise
```

### 7.6 AHX (Amiga Tracker)

AHX plays Amiga-format AHX music modules. Load the module data into memory and use the player commands.

```basic
REM Assuming module loaded at &H100000
AHX PLAY &H100000, 8192
REM ... later:
AHX STOP
```

### 7.7 Music File Playback

Several audio engines support file-based playback of native music formats:

| Command | Format | Origin |
|---------|--------|--------|
| `SID PLAY` | .sid | Commodore 64 |
| `SAP PLAY` | .sap | Atari 8-bit |
| `PSG PLAY` | .ay | ZX Spectrum/CPC |
| `TED PLAY` | TED dumps | Commodore 16 |
| `AHX PLAY` | .ahx | Amiga |

All players support `STOP` to halt playback. SID and SAP players support subsong selection.

---

## 8. Memory Map Reference

### Main Memory Layout

| Address Range | Size | Description |
|---------------|------|-------------|
| `&H00000`-`&H9EFFF` | 636 KB | Main RAM |
| `&H9F000` | 4 KB | Hardware stack (IE32) |
| `&HA0000`-`&HAFFFF` | 64 KB | VGA VRAM window |
| `&HB8000`-`&HBFFFF` | 32 KB | VGA text buffer |
| `&HF0000`-`&HFFFFF` | 64 KB | I/O region |
| `&H100000`-`&H4FFFFF` | 4 MB | Video RAM |

### I/O Region Map

| Address Range | Device |
|---------------|--------|
| `&HF0000`-`&HF0057` | Video Chip (copper, blitter, raster) |
| `&HF0700`-`&HF07FF` | Terminal MMIO |
| `&HF0800`-`&HF0B7F` | Audio Chip (SoundChip) |
| `&HF0B80`-`&HF0B91` | AHX Player |
| `&HF0C00`-`&HF0C1C` | PSG (AY-3-8910) |
| `&HF0D00`-`&HF0D1D` | POKEY |
| `&HF0E00`-`&HF0E2D` | SID (MOS 6581) |
| `&HF0F00`-`&HF0F5F` | TED (audio + video) |
| `&HF1000`-`&HF13FF` | VGA registers |
| `&HF2000`-`&HF200B` | ULA (ZX Spectrum) |
| `&HF2100`-`&HF213F` | ANTIC |
| `&HF2140`-`&HF21B4` | GTIA |
| `&HF4000`-`&HF5000` | Voodoo 3DFX |

### EhBASIC Memory Layout

| Address | Size | Purpose |
|---------|------|---------|
| `&H001000`-`&H020FFF` | 128 KB | Interpreter code reservation |
| `&H021000`-`&H021FFF` | 4 KB | Input line buffer |
| `&H022000`-`&H022FFF` | 4 KB | Interpreter state block |
| `&H023000`-`&H04FFFF` | 180 KB | Program text region (grows upward) |
| `&H050000`-`&H057FFF` | 32 KB | Numeric variable table region start |
| `&H058000`-`&H05FFFF` | 32 KB | String variable table region start |
| `&H060000`-`&H08BFFF` | 176 KB | Array storage region start |
| `&H08C000`-`&H08FFFF` | 16 KB | String temporaries / heap top |
| `&H090000`-`&H096FFF` | 28 KB | GOSUB/FOR/DO/WHILE stacks |
| `&H097000`-`&H09EFFF` | 32 KB | Hardware stack region |
| `&H09F000` | - | Initial stack top (SP) |

`var_init` initialises variable/array pointers at fixed bases (`&H050000`, `&H058000`, `&H060000`) and the string heap at `&H08C000`.

### State Block Offsets

The interpreter state block at `&H022000` contains:

| Offset | Field | Description |
|--------|-------|-------------|
| `+&H00` | ST_PROG_START | Start of BASIC program text |
| `+&H04` | ST_PROG_END | End of program text |
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

---

## 9. Hardware Register Reference

This section documents every I/O register accessible via POKE/PEEK or the hardware commands. Registers are grouped by subsystem.

### 9.1 Terminal MMIO (`&HF0700`-`&HF07FF`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF0700` | TERM_OUT | W | Write character to output |
| `&HF0704` | TERM_STATUS | R | Bit 0: input available, Bit 1: output ready |
| `&HF0708` | TERM_IN | R | Read next input character (dequeues) |
| `&HF070C` | TERM_LINE_STATUS | R | Bit 0: complete line available |
| `&HF0710` | TERM_ECHO | R/W | Bit 0: local echo enable (default 1) |
| `&HF07F0` | TERM_SENTINEL | W | Write `&HDEAD` to stop CPU |

### 9.2 VGA Registers (`&HF1000`-`&HF13FF`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF1000` | VGA_MODE | R/W | Mode: `&H03`=text, `&H12`=640x480, `&H13`=320x200 |
| `&HF1004` | VGA_STATUS | R | Bit 0: VSync, Bit 3: vertical retrace |
| `&HF1008` | VGA_CTRL | R/W | Bit 0: enable |
| `&HF1010` | VGA_SEQ_INDEX | W | Sequencer index |
| `&HF1014` | VGA_SEQ_DATA | R/W | Sequencer data |
| `&HF1020` | VGA_CRTC_INDEX | W | CRTC index |
| `&HF1024` | VGA_CRTC_DATA | R/W | CRTC data |
| `&HF1028` | VGA_CRTC_STARTHI | W | Display start address (high byte) |
| `&HF102C` | VGA_CRTC_STARTLO | W | Display start address (low byte) |
| `&HF1050` | VGA_DAC_MASK | R/W | Pixel mask |
| `&HF1054` | VGA_DAC_RINDEX | W | DAC read index |
| `&HF1058` | VGA_DAC_WINDEX | W | DAC write index |
| `&HF105C` | VGA_DAC_DATA | R/W | DAC data (R, G, B sequence) |
| `&HF1100`-`&HF13FF` | VGA_PALETTE | R/W | 256 x 3 bytes palette RAM |

### 9.3 ULA Registers (`&HF2000`-`&HF200B`)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| `&HF2000` | ULA_BORDER | R/W | Border colour (bits 0-2) |
| `&HF2004` | ULA_CTRL | R/W | Bit 0: enable |
| `&HF2008` | ULA_STATUS | R | Bit 0: VBlank |

ULA VRAM at `&H4000`: 6,144 bytes bitmap + 768 bytes attributes.

### 9.4 Audio Chip Registers (`&HF0800`-`&HF0B7F`)

#### Global Control

| Address | Name | Description |
|---------|------|-------------|
| `&HF0800` | AUDIO_CTRL | Master audio control |
| `&HF0804` | ENV_SHAPE | Global envelope shape |
| `&HF0820` | FILTER_CUTOFF | Cutoff frequency (0-255) |
| `&HF0824` | FILTER_RESONANCE | Resonance (0-255) |
| `&HF0828` | FILTER_TYPE | 0=off, 1=LP, 2=HP, 3=BP |

#### Fixed Channels

| Channel | Freq | Vol | Ctrl | ADSR |
|---------|------|-----|------|------|
| Square | `&HF0900` | `&HF0904` | `&HF0908` | `&HF0930`-`&HF093C` |
| Triangle | `&HF0940` | `&HF0944` | `&HF0948` | `&HF0960`-`&HF096C` |
| Sine | `&HF0980` | `&HF0984` | `&HF0988` | `&HF0990`-`&HF099C` |
| Noise | `&HF09C0` | `&HF09C4` | `&HF09C8` | `&HF09D0`-`&HF09DC` |
| Sawtooth | `&HF0A20` | `&HF0A24` | `&HF0A28` | `&HF0A30`-`&HF0A3C` |

Frequency values are 16.8 fixed-point: Hz * 256.

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
| `+&H08` | CTRL | Control (Bit 1: gate) |
| `+&H0C` | DUTY | Duty cycle (0-255) |
| `+&H14` | ATK | Attack time (ms) |
| `+&H18` | DEC | Decay time (ms) |
| `+&H1C` | SUS | Sustain level (0-255) |
| `+&H20` | REL | Release time (ms) |
| `+&H24` | WAVE_TYPE | Waveform (0-4) |

#### Effects

| Address | Name | Description |
|---------|------|-------------|
| `&HF0A40` | OVERDRIVE_CTRL | Drive amount (0-255) |
| `&HF0A50` | REVERB_MIX | Dry/wet mix (0-255) |
| `&HF0A54` | REVERB_DECAY | Decay time (0-255) |

### 9.5 PSG Registers (`&HF0C00`-`&HF0C1C`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF0C00`-`&HF0C0D` | PSG_BASE | AY-3-8910 register array (14 registers) |
| `&HF0C0E` | PSG_PLUS_CTRL | Enhanced mode (0=standard, 1=enhanced) |
| `&HF0C10` | PSG_PLAY_PTR | Player pointer |
| `&HF0C14` | PSG_PLAY_LEN | Player length |
| `&HF0C18` | PSG_PLAY_CTRL | 1=start, 2=stop |
| `&HF0C1C` | PSG_PLAY_STATUS | Playback status |

### 9.6 POKEY Registers (`&HF0D00`-`&HF0D1D`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF0D00` | POKEY_AUDF1 | Channel 1 frequency divider |
| `&HF0D01` | POKEY_AUDC1 | Channel 1 control (distortion + volume) |
| `&HF0D02`-`&HF0D07` | AUDF2-4, AUDC2-4 | Channels 2-4 |
| `&HF0D08` | POKEY_AUDCTL | Master audio control |
| `&HF0D10`-`&HF0D1D` | SAP_PLAY_* | SAP player registers |

### 9.7 SID Registers (`&HF0E00`-`&HF0E2D`)

Each voice occupies 7 bytes:

| Offset | Name | Description |
|--------|------|-------------|
| +0 | FREQ_LO | Frequency low byte |
| +1 | FREQ_HI | Frequency high byte |
| +2 | PW_LO | Pulse width low byte |
| +3 | PW_HI | Pulse width high (bits 0-3) |
| +4 | CTRL | Gate, waveform, sync, ring mod |
| +5 | AD | Attack/Decay |
| +6 | SR | Sustain/Release |

Voice bases: V1=`&HF0E00`, V2=`&HF0E07`, V3=`&HF0E0E`.

Filter and volume at `&HF0E15`-`&HF0E19`.

### 9.8 TED Registers (`&HF0F00`-`&HF0F5F`)

Audio registers at `&HF0F00`-`&HF0F05`. Video control at `&HF0F20`-`&HF0F5F`. See sections 6.5 and 7.5 for details.

### 9.9 ANTIC Registers (`&HF2100`-`&HF213F`)

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
| `&HF2124` | ANTIC_VCOUNT | Vertical counter (read-only) |
| `&HF2130` | ANTIC_NMIEN | NMI enable |
| `&HF2138` | ANTIC_ENABLE | Video enable (Bit 0) |
| `&HF213C` | ANTIC_STATUS | Status (Bit 0: VBlank) |

### 9.10 GTIA Registers (`&HF2140`-`&HF21B4`)

| Address Range | Description |
|---------------|-------------|
| `&HF2140`-`&HF2150` | Playfield colours (COLPF0-3, COLBK) |
| `&HF2154`-`&HF2160` | Player/missile colours (COLPM0-3) |
| `&HF2164` | GTIA_PRIOR (priority) |
| `&HF2168` | GTIA_GRACTL (graphics control) |
| `&HF2170`-`&HF217C` | Player positions (HPOSP0-3) |
| `&HF2180`-`&HF218C` | Missile positions (HPOSM0-3) |
| `&HF2190`-`&HF21A0` | Player/missile sizes |
| `&HF21A4`-`&HF21B4` | Player/missile graphics patterns |

### 9.11 Voodoo 3DFX Registers (`&HF4000`-`&HF5000`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF4000` | VOODOO_STATUS | Status flags (read-only) |
| `&HF4004` | VOODOO_ENABLE | Enable register |
| `&HF4008`-`&HF401C` | VERTEX_AX..CY | Vertex coordinates (12.4 fixed-point) |
| `&HF4020`-`&HF403C` | START_R..W | Vertex attributes (12.12 fixed-point) |
| `&HF4080` | TRIANGLE_CMD | Submit triangle |
| `&HF4104` | FBZCOLOR_PATH | Colour path control |
| `&HF4108` | FOG_MODE | Fog mode |
| `&HF410C` | ALPHA_MODE | Alpha blending mode |
| `&HF4110` | FBZ_MODE | Framebuffer Z mode (depth, clipping) |
| `&HF4114` | LFB_MODE | Linear framebuffer mode |
| `&HF4118` | CLIP_LEFT_RIGHT | Clip rectangle X |
| `&HF411C` | CLIP_LOW_Y_HIGH | Clip rectangle Y |
| `&HF4124` | FAST_FILL_CMD | Fast fill command |
| `&HF4128` | SWAP_BUFFER_CMD | Swap buffers |
| `&HF4214` | VIDEO_DIM | Video dimensions |
| `&HF4300` | TEXTURE_MODE | Texture mode |
| `&HF430C` | TEX_BASE0 | Texture base address |
| `&HF4330` | TEX_WIDTH | Texture width |
| `&HF4334` | TEX_HEIGHT | Texture height |
| `&HF4338` | TEX_UPLOAD | Texture upload trigger |

### 9.12 Video Chip Registers (`&HF0000`-`&HF0057`)

| Address | Name | Description |
|---------|------|-------------|
| `&HF0000` | VIDEO_CTRL | Video control |
| `&HF0004` | VIDEO_MODE | Video mode select |
| `&HF0008` | VIDEO_STATUS | Video status |
| `&HF000C` | COPPER_CTRL | Copper control (1=enable) |
| `&HF0010` | COPPER_PTR | Copper list pointer |
| `&HF001C` | BLT_CTRL | Control (write 1 to start) |
| `&HF0020` | BLT_OP | Operation (0=copy, 1=fill, 2=line) |
| `&HF0024` | BLT_SRC | Blitter source address |
| `&HF0028` | BLT_DST | Blitter destination address |
| `&HF002C` | BLT_WIDTH | Blitter width |
| `&HF0030` | BLT_HEIGHT | Blitter height |
| `&HF0034` | BLT_SRC_STRIDE | Source stride |
| `&HF0038` | BLT_DST_STRIDE | Destination stride |
| `&HF003C` | BLT_COLOR | Fill/line colour |

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

### Direct Memory Access

All memory is accessible from BASIC. VRAM starts at `&H100000` and can be written directly:

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
| R28 | Return status code |
| R31 | Hardware stack pointer |

### FP32 Calling Convention

The floating-point library uses:
- **Input**: R8 (first operand), R9 (second operand)
- **Output**: R8 (result)
- **Clobbers**: R1-R7

Available routines: `fp_add`, `fp_sub`, `fp_mul`, `fp_div`, `fp_neg`, `fp_abs`, `fp_cmp`, `fp_int`, `fp_float`, `fp_sqr`, `fp_sin`, `fp_cos`, `fp_tan`, `fp_atn`, `fp_log`, `fp_exp`, `fp_pow`.

---

## 11. Error Messages

The current interpreter does not emit textual BASIC error messages, and it does not currently set structured runtime error codes during normal execution. `ST_ERROR_FLAG` exists in the state block layout but is only initialised/cleared at startup.

Current behaviour for common failure cases:

| Condition | Behaviour |
|-----------|-----------|
| Undefined line (GOTO/GOSUB) | Execution stops silently |
| RETURN without GOSUB | Treated as stack mismatch; execution stops |
| NEXT without FOR | Treated as stack mismatch; execution stops |
| WEND without WHILE | Treated as stack mismatch; execution stops |
| LOOP without DO | Treated as stack mismatch; execution stops |
| Division by zero | FP32 operation result (typically +/-Infinity) |
| Square root of negative | Returns 0 |
| INPUT buffer full | Extra typed characters are ignored |
| DATA exhausted | READ returns 0 |

Use `TRON` to trace line execution when debugging control-flow issues.

---

## 12. Example Programs

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

Tokens marked with \* are tokenised (recognised by the tokeniser) but do not yet have execution dispatch. Using them in a program will silently have no effect or return 0.

`RUN`, `LIST`, and `NEW` are tokenised keywords but are handled by the REPL command parser in immediate mode rather than the program-line statement dispatcher.

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
| 92 | TK_NULL | NULL | Statement |
| 93 | TK_INC | INC | Statement |
| 94 | TK_WAIT | WAIT | Statement |
| 95 | TK_LOAD | LOAD | Statement |
| 96 | TK_SAVE | SAVE | Statement |
| 97 | TK_DEF | DEF | Statement |
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
| A3 | TK_WIDTH | WIDTH | Statement |
| A4 | TK_GET | GET | Statement |
| A5 | TK_SWAP | SWAP | Statement |
| A6 | TK_BITSET | BITSET | Statement |
| A7 | TK_BITCLR | BITCLR | Statement |
| A8 | TK_TAB | TAB | Function |
| A9 | TK_TO | TO | Keyword |
| AA | TK_FN | FN | Keyword |
| AB | TK_SPC | SPC | Function |
| AC | TK_THEN | THEN | Keyword |
| AD | TK_NOT | NOT | Operator |
| AE | TK_STEP | STEP | Keyword |
| AF | TK_UNTIL | UNTIL/WEND | Keyword |
| B0 | TK_WHILE | WHILE | Statement |
| B1 | TK_PLUS | + | Operator |
| B2 | TK_MINUS | - | Operator |
| B3 | TK_MULT | * | Operator |
| B4 | TK_DIV | / | Operator |
| B5 | TK_POWER | ^ | Operator |
| B6 | TK_AND | AND | Operator |
| B7 | TK_EOR | EOR | Operator |
| B8 | TK_OR | OR | Operator |
| B9 | TK_RSHIFT | >> | Operator \* |
| BA | TK_LSHIFT | << | Operator \* |
| BB | TK_GT | > | Operator |
| BC | TK_EQUAL | = | Operator |
| BD | TK_LT | < | Operator |
| BE | TK_SGN | SGN | Function |
| BF | TK_INT | INT | Function |
| C0 | TK_ABS | ABS | Function |
| C1 | TK_USR | USR | Function |
| C2 | TK_FRE | FRE | Function |
| C3 | TK_POS | POS | Function \* |
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
| CF | TK_SADD | SADD | Function \* |
| D0 | TK_LEN | LEN | Function |
| D1 | TK_STRS | STR$ | Function |
| D2 | TK_VAL | VAL | Function |
| D3 | TK_ASC | ASC | Function |
| D4 | TK_UCASES | UCASE$ | Function \* |
| D5 | TK_LCASES | LCASE$ | Function \* |
| D6 | TK_CHRS | CHR$ | Function |
| D7 | TK_HEXS | HEX$ | Function |
| D8 | TK_BINS | BIN$ | Function |
| D9 | TK_BITTST | BITTST | Function |
| DA | TK_MAX | MAX | Function |
| DB | TK_MIN | MIN | Function |
| DC | TK_PI | PI | Function |
| DD | TK_TWOPI | TWOPI | Function |
| DE | TK_VPTR | VARPTR | Function \* |
| DF | TK_LEFTS | LEFT$ | Function |
| E0 | TK_RIGHTS | RIGHT$ | Function |
| E1 | TK_MIDS | MID$ | Function |

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
