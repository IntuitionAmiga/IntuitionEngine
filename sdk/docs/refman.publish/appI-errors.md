
# Appendix I - Error Message Index

The printable errors a running Intuition Engine produces.
Grouped by source. Each entry has the exact text the machine
prints, the numeric code the runtime uses internally, and a
one-line explanation.

## I.1 EhBASIC runtime errors

The format on the screen is the message text, a space, the literal
word `ERROR`, then (when raised from a running program rather
than from direct mode) ` IN ` followed by the line number.

| Code | Printed text       | Meaning |
|------|--------------------|---------|
| `1`  | `SYNTAX`           | The tokenizer or parser cannot make sense of the statement. |
| `2`  | `DIVISION BY ZERO` | A `/`, `\`, or `MOD` operator saw a zero denominator. |
| `3`  | `UNDEFINED LINE`   | `GOTO`, `GOSUB`, `THEN`, or `RESTORE` referenced a line that does not exist. |
| `4`  | `NEXT WITHOUT FOR` | `NEXT` did not match a pending `FOR`. |
| `5`  | `RETURN WITHOUT GOSUB` | `RETURN` did not match a pending `GOSUB`. |
| `6`  | `OUT OF MEMORY`    | The program, variable, array, or string area is full. |
| `7`  | `ILLEGAL QUANTITY` | A function received an argument outside its domain (negative `SQR`, non-positive `LOG`, out-of-range `CHR$`). |
| `8`  | `OVERFLOW`         | An arithmetic operation overflowed the 32-bit floating-point range. |
| `9`  | `TYPE MISMATCH`    | A numeric expression saw a string, or vice versa. |
| `10` | `FC`               | Illegal function call. Raised by `POKE` to an unaligned address, by a failed `HOST` action, and by other built-in helpers when they reject their argument shape. |
| `11` | `REDIM`            | `DIM` named an array that already exists. |

Additional message strings produced by specific verbs:

| Verb context | Printed text |
|--------------|--------------|
| `LOAD`       | `?FILE NOT FOUND`. |
| `SAVE`       | `?FILE ERROR` (printed as a soft error; `SAVE` does not raise into the runtime). |

## I.2 Machine monitor (IE Mon)

The monitor prints short, lowercase-prefixed messages. Every
unrecognised command prints `unknown command`. The common ones:

| Printed text                  | Meaning |
|-------------------------------|---------|
| `?`                           | Generic syntax error in a monitor command line. |
| `unknown command`             | The first token did not name a command. |
| `bad address`                 | An address argument was outside the addressable range. |
| `bad range`                   | An end address was lower than its start address. |
| `bad value`                   | A value argument did not parse as a number. |
| `no breakpoint at <addr>`     | `bc` was given an address with no breakpoint. |
| `breakpoint list full`        | The breakpoint table cannot hold another entry. |
| `cpu frozen`                  | An operation tried to step a frozen CPU; thaw it first. |
| `file error`                  | A `save` or `load` sidecar command failed. |

The monitor's `h` (hunt) and `c` (compare) commands print
addresses of matches rather than diagnostic messages. The
disassembler prints `???` for an unknown opcode.

## I.3 File I/O block

When the File I/O block (Chapter 34) fails, `FILE_STATUS` reads
`1` and `FILE_ERROR_CODE` is one of:

| Code | Meaning |
|------|---------|
| `0`  | OK (paired with `FILE_STATUS = 0`). |
| `1`  | Not found. |
| `2`  | Permission. |
| `3`  | Path traversal. |

## I.4 HOST appliance block

The status byte at `0xF1408` (Chapter 35) reads one of:

| Code | Meaning |
|------|---------|
| `0`  | Running. |
| `1`  | OK (terminal). |
| `2`  | Error (terminal). |
| `3`  | Cancelled by user (terminal). |
| `4`  | Disabled (the system-action bridge is off). |
| `5`  | Idle (no command has been fired). |

A non-zero exit code at `0xF140C` after a terminal status of `2`
gives the underlying action's exit value; the meaning is
action-specific and is not normalised across subverbs.

## I.5 Image executor

The image executor (`RUN "<name>.ie*"`, Chapter 31) reports one of:

| Code | Meaning |
|------|---------|
| `0`  | OK (image loaded and started). |
| `1`  | Not supported (image type unknown on this CPU profile). |
| `2`  | Unsupported (extension recognised but the matching CPU is disabled). |
| `3`  | File error (the underlying File I/O read failed). |

`RUN` translates a non-zero result into `?FILE ERROR` for the
file-error cases and `?FC ERROR` for the unsupported cases.

## I.6 Coprocessor

`COSTATUS` (Chapter 31) reports:

| Code | Meaning |
|------|---------|
| `0`  | Idle. |
| `1`  | Running. |
| `2`  | Stopped cleanly (the worker exited). |
| `3`  | Faulted (the worker raised an exception). |

`COWAIT` blocks until the status leaves `1`; the status then
encodes the outcome.

## I.7 Raised by the CPU itself

Per CPU, the chapter (Ch 24-29) lists the trap and exception
vectors and their meanings. The monitor's `r` command displays
the current trap source when a CPU has stopped at one. Common
cross-CPU shapes:

- Division by zero raises the CPU's divide-by-zero exception
  (M68K vector 5, x86 `INT 0`, IE64 trap with `cause = DIV0`).
- An unaligned `32`-bit access on the IE64 raises an alignment
  fault; the address that caused it is in `CR_FAULT_ADDR`.
- An undefined opcode raises the CPU's illegal-instruction
  vector (M68K vector 4, x86 `INT 6`, Z80 silently re-executes
  on most undocumented prefixes, 6502 documents the undocumented
  opcodes - see Chapter 26).
