---
title: "The Terminal Serial Stream"
sources:
  - registers.go
  - terminal_io.go
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 38 - The Terminal Serial Stream

The terminal is the Intuition Engine's byte-stream console. BASIC
uses it for `PRINT`, `GET`, and `INPUT`. Machine code can use the
same registers for character output, cooked input, local echo, and
line-mode control.

Every register lives in the terminal block at `$F0700`-`$F07FF`.

## 38.1 Register Map

| Address | Name | R/W | Meaning |
|---------|------|-----|---------|
| `$F0700` | `TERM_OUT` | W | Write one byte to the terminal. Reads return `0`. |
| `$F0704` | `TERM_STATUS` | R | Bit `0` input available, bit `1` output ready. |
| `$F0708` | `TERM_IN` | R | Read one cooked input byte and advance the input queue. |
| `$F070C` | `TERM_LINE_STATUS` | R | Bit `0` set when a complete line is queued. |
| `$F0710` | `TERM_ECHO` | R/W | Bit `0` local echo. Default `1`. |
| `$F0724` | `TERM_CTRL` | R/W | Bit `0` line-input mode. Default `1`. |
| `$F07F0` | `TERM_SENTINEL` | W | Writing `$DEAD` stops the current CPU. |

Reserved terminal addresses:

| Address | Name | Status |
|---------|------|--------|
| `$F0714` | cursor X | Reserved. Reads return `0`; writes are ignored. |
| `$F0718` | cursor Y | Reserved. Reads return `0`; writes are ignored. |
| `$F071C` | foreground colour | Reserved. Reads return `0`; writes are ignored. |
| `$F0720` | background colour | Reserved. Reads return `0`; writes are ignored. |

## 38.2 Output

Writing a byte to `$F0700` sends one character to the terminal.
`TERM_STATUS` bit `1` is always set, so output never needs a wait
loop.

```basic
10 REM SEND ONE BYTE, THEN READ THE STATUS
20 POKE &H000F0700,ASC("?")
30 PRINT "READY ";PEEK(&H000F0704)
```

The first line prints `?`. The second line normally prints `READY 2`
when there is no waiting input, or `READY 3` when input is already
queued. Bit `1` is the output-ready bit.
Line `20` is a raw byte write to the terminal stream. It is the
same stream BASIC uses for `PRINT`.

ASCII `$0A` ends a line. ASCII `$0D` is carriage return. Other bytes
are sent as-is; printable results depend on the active font and screen
mode.

Two aliases exist for M68K short-form addressing:

| Address | Meaning |
|---------|---------|
| `$F700` | `16`-bit absolute form that reaches `$F0700` on M68K. |
| `$FFFFF700` | Sign-extended form of `$F700`; same effect. |

The 6502 and Z80 do not use `$F700` for terminal output. Their
small-address terminal routes are documented in Chapters 27 and 28.

## 38.3 Cooked Input

`TERM_IN` is the cooked byte stream used by BASIC `INPUT`. Check
`TERM_STATUS` bit `0` before reading if byte value `0` matters to
your program.

```basic
10 REM PROMPT, WAIT FOR COOKED INPUT, THEN CONSUME ONE BYTE
20 POKE &H000F0700,ASC("?")
30 IF (PEEK(&H000F0704) AND 1)=0 THEN GOTO 30
40 C=PEEK(&H000F0708)
50 PRINT "GOT ";C
```

If the next queued character is `A`, line `50` prints `GOT 65`.
Reading `$F0708` when the queue is empty returns `0`.
Line `30` tests `TERM_STATUS` bit `0`. Line `40` advances the
input queue by one byte.

`TERM_LINE_STATUS` bit `0` is set when a complete line ending in
ASCII `$0A` is queued. It clears after the queued line bytes have
been consumed.

## 38.4 Local Echo

When local echo is on, line input prints characters as they are typed.
A program that wants to draw its own prompt or hide typed characters
can turn echo off.

```basic
10 PRINT "ECHO ";PEEK(&H000F0710)
20 POKE &H000F0710,0
30 PRINT "ECHO ";PEEK(&H000F0710)
40 POKE &H000F0710,1
```

Expected output:

```text
ECHO 1
ECHO 0
```

## 38.5 Line-Input Mode

`TERM_CTRL` bit `0` selects line-input mode. The default is `1`.

In line-input mode, typed keys go to the cooked input queue and
`INPUT` waits for `Enter`. In single-character mode, keys go to the
cooked-key queue at `$F0728`, described in Chapter 37. BASIC `GET`
uses that single-character route.

```basic
10 PRINT "LINE ";PEEK(&H000F0724)
20 POKE &H000F0724,0
30 PRINT "LINE ";PEEK(&H000F0724)
40 POKE &H000F0724,1
```

Expected output:

```text
LINE 1
LINE 0
```

## 38.6 Sentinel

Writing the exact value `$DEAD` to `$F07F0` stops the current CPU.
Any other value is ignored.

This is for deliberate self-termination and test harnesses. BASIC has
no keyword for it. Do not type the write casually:

```basic
POKE &H000F07F0,&HDEAD
```

The CPU stops when the write completes and does not reach the next
statement.

## 38.7 What This Chapter Is Not

The terminal is the only byte-stream console in the machine. There is
no UART, parallel port, IEC bus, RS-232 line, or separate keyboard
scan port. Programs that need binary file data use Chapter 35.
Programs that need raw keystrokes use Chapter 37.
