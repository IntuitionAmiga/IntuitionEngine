---
title: "The Terminal Serial Stream"
sources:
  - registers.go
  - terminal_io.go
---

# Chapter 37 - The Terminal Serial Stream

The Intuition Engine has one character-serial port, called the
terminal. It is the path BASIC uses for `PRINT`, `GET`, and
`INPUT`; it is also the path that a freshly-booted machine uses to
present its startup banner and the prompt. From a program's point
of view it is a byte stream in (keyboard) and a byte stream out
(console).

Every register lives in the terminal / serial block
(`0xF0700`-`0xF07FF`).

## 37.1 Output: a single write-only port

| Address    | Name        | R/W | Meaning |
|------------|-------------|-----|---------|
| `0xF0700`  | output byte | W   | Writing a byte sends one character to the terminal. Reading returns `0`. |

There is no buffer status to wait on for output. Writing
`0xF0700` enqueues the byte immediately. ASCII `0x0A` ends a line;
ASCII `0x0D` is interpreted as carriage return. Bytes outside the
printable ASCII range are sent verbatim and may be rendered as
glyphs in the current font (see Appendix B) or dropped, depending
on the terminal mode.

Two aliases exist for short-form addressing:

| Address       | Origin | Meaning |
|---------------|--------|---------|
| `0xF700`      | 16-bit absolute on M68K (`.W` addressing) - sign-extended below | Always reaches `0xF0700` on M68K. |
| `0xFFFFF700`  | 32-bit sign-extended form of `0xF700` | Same byte, same effect. |

The 6502 and Z80 do **not** see `0xF700` as the terminal output
register: their CPU adapters intercept that address as a
bank-control register before the bus translation runs. Bare 8-bit
code that needs to print uses the cooked terminal stream via its
bank-windowed view of `0xF0700`, the ULA (Chapter 8), the VGA
(Chapter 5), or one of the other display engines.

## 37.2 Output status

| Address    | Name           | R/W | Meaning |
|------------|----------------|-----|---------|
| `0xF0704`  | terminal status| R   | Bit `0` = at least one input byte is available; bit `1` = output ready. |

Bit `1` of the status register is always set: the terminal output
side never stalls. Bit `0` tells a program whether reading the
input register will return a real byte or zero.

## 37.3 Input: cooked stream

| Address    | Name           | R/W | Meaning |
|------------|----------------|-----|---------|
| `0xF0708`  | input dequeue  | R   | Read one input byte, then advance the input queue. Returns `0` if the queue is empty. |
| `0xF070C`  | line status    | R   | Bit `0` is set when at least one complete line (ending in `0x0A`) is queued. |

The cooked input stream is what `INPUT` reads. Characters are
delivered to it as the user types them, subject to the line-input
flag in section 37.5.

A program that wants raw single-key reads (no line buffering, no
waiting for `Enter`) uses the cooked-key path at `0xF0728` /
`0xF072C` described in Chapter 36 instead.

## 37.4 Local echo

| Address    | Name        | R/W | Meaning |
|------------|-------------|-----|---------|
| `0xF0710`  | echo flag   | R/W | Bit `0` = local echo. Default `1`. |

When local echo is on, the terminal prints each input character at
the cursor as the user types. Programs that read the input
register and want to do their own echo (a password prompt, a
single-character menu) write `0` to clear bit `0`. Writing `1`
turns echo back on.

## 37.5 Line-input mode

| Address    | Name             | R/W | Meaning |
|------------|------------------|-----|---------|
| `0xF0724`  | terminal control | R/W | Bit `0` = line-input mode. Default `1`. |

In line-input mode (the default) keystrokes accumulate in the
cooked input queue at `0xF0708` until the user types `Enter`,
which appends `0x0A` and unblocks any reader that was waiting for
a complete line. In single-character mode (bit `0` cleared) each
key is delivered to the cooked-key queue at `0xF0728` instead,
without buffering.

BASIC's `INPUT` uses line mode; BASIC's `GET` uses single-character
mode. Mixed-mode programs flip the flag explicitly before each
read.

## 37.6 The sentinel

| Address    | Name      | R/W | Meaning |
|------------|-----------|-----|---------|
| `0xF07F0`  | sentinel  | W   | Writing the exact value `0xDEAD` stops the current CPU. |

The sentinel is a way for a running program to stop itself
cleanly. The exact value `0xDEAD` is required; any other value is
ignored. The stop takes effect when the write completes; the CPU
that issued it does not see the next instruction.

In normal programs the sentinel is reserved for test drivers and
self-terminating image executor payloads. BASIC has no keyword
that writes it.

## 37.7 Reserved cursor and colour registers

| Address    | Name        | Status |
|------------|-------------|--------|
| `0xF0714`  | cursor X    | Reserved for future use. Reads return `0`; writes are ignored. |
| `0xF0718`  | cursor Y    | Same. |
| `0xF071C`  | fg colour   | Same. |
| `0xF0720`  | bg colour   | Same. |

These addresses are part of the terminal block's address map but
are not implemented in the current Intuition Engine. They are
documented so that programs do not accidentally collide with them.

## 37.8 A character at a time

The smallest useful interaction:

```ie64
    li      r2, #'?'
    store.b r2, 0xF0700        ; print '?'

.wait:
    load.l  r1, 0xF0704
    li      r3, #1
    and     r1, r1, r3
    beqz    r1, .wait
    load.l  r1, 0xF0708        ; consume the input byte
```

This is the building block for every text-mode program on the
Intuition Engine: print a prompt, poll the status bit, read one
byte. Everything in BASIC's `INPUT`, every dialog box drawn by a
machine-code program, and every transcript captured during
debugging is layered on top of this one byte-stream port.

## 37.9 What this chapter is not

The terminal is the only serial port the Intuition Engine has.
There is no UART, no parallel port, no IEC bus, no RS-232 line,
and no separate keyboard scan port. Programs that need binary
data move it through the File I/O block (Chapter 34) or through
the cross-CPU coprocessor channel (Chapter 31). Programs that
need to receive raw keystrokes use the input registers in
Chapter 36.
