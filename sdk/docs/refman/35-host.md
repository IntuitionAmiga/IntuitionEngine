---
title: "The HOST Command"
sources:
  - host_helper.go
  - sdk/include/ehbasic_hw_host.inc
  - registers.go
---

# Chapter 35 - The HOST Command

`HOST` is the BASIC verb that lets a program ask the Intuition
Engine appliance to perform a system-level action: bring up network
configuration, update the installed system packages, reboot the
machine, or power it off. Five subverbs are recognised.

`HOST` is not a tokenizer keyword. It is matched as raw ASCII at
statement dispatch in `exec_line`, so the four letters `H` `O`
`S` `T` are stored in the program text exactly as typed. In
expression context and in explicit `LET` assignments the same
letters are still an ordinary identifier and may be used as a
variable name; the statement form is recognised only when `HOST`
begins a statement.

## 35.1 Subverbs

| Form               | What it does |
|--------------------|--------------|
| `HOST NET`         | Open the network configuration screen. |
| `HOST UPDATE`      | Update the installed system packages. |
| `HOST REBOOT`      | Reboot the appliance. |
| `HOST POWEROFF`    | Power off the appliance. |
| `HOST HELP`        | Print the list of `HOST` subverbs to the terminal. |

`HOST` on its own (no subverb) is the same as `HOST HELP`. A
following colon (`HOST:`) is also the help form, so the keyword can
appear at the start of a colon-separated multi-statement line
without arguments.

Any other word after `HOST` produces a syntax error and the
statement aborts before the appliance is touched.

## 35.2 What each subverb does

### 35.2.1 `HOST NET`

Opens the appliance's network configuration screen. The user picks
a network, enters credentials, and confirms. The screen exits when
the user dismisses it. BASIC resumes the next statement when the
screen closes.

### 35.2.2 `HOST UPDATE`

Updates the installed system packages on the appliance. The action
runs through an authenticated channel; the appliance asks the user
to confirm before the change is applied. If the user cancels, the
statement returns without error and execution continues with the
next BASIC statement.

### 35.2.3 `HOST REBOOT`

Reboots the appliance. The running program stops, the machine
restarts, and the BASIC prompt comes back when the boot sequence
finishes. A program that calls `HOST REBOOT` never sees the next
statement on the same boot.

### 35.2.4 `HOST POWEROFF`

Powers the appliance off. As with `REBOOT`, the running program
does not return; the next time BASIC runs is after a manual
power-on.

### 35.2.5 `HOST HELP`

Prints a fixed list of the subverbs (`NET`, `UPDATE`, `REBOOT`,
`POWEROFF`, `HELP`) with a one-line description of each. The text
comes from a string in the BASIC ROM and is not configurable.

## 35.3 The register block

`HOST` is built on a four-register block at `0xF1400`. Every
register is `32`-bit. For the command, trigger, and status
registers only the low byte carries information; the exit
register is a full `32`-bit unsigned value, and reading any of
its four byte lanes returns the appropriate byte of that value.

| Offset | Name     | R/W | Width | Meaning |
|--------|----------|-----|-------|---------|
| `+0x00` | command  | W   | byte  | Subverb enum. `1` = NET, `2` = UPDATE, `3` = REBOOT, `4` = POWEROFF, `0` = none. |
| `+0x04` | trigger  | W   | byte  | Writing a non-zero value fires the command currently in the command register. |
| `+0x08` | status   | R   | byte  | Current state of the command. See below. |
| `+0x0C` | exit     | R   | 32-bit | Exit code from the underlying action, valid once status reaches a terminal state. Read as a full `32`-bit word; the four lane addresses `+0x0C`-`+0x0F` return the four bytes of the same value. |

The status register reads:

| Value | State         | Terminal? |
|-------|---------------|-----------|
| `0`   | running       | no  |
| `1`   | OK            | yes |
| `2`   | error         | yes |
| `3`   | cancelled by user | yes |
| `4`   | disabled (appliance refuses the command) | yes |
| `5`   | idle (no command has been fired) | yes |

A command starts in `running` when the trigger is written and
moves to one of the terminal values when the action finishes. The
exit register is only meaningful once the status is terminal.

## 35.4 The BASIC protocol

The BASIC verb implements this sequence:

1. Write the subverb enum to `+0x00`.
2. Write `1` to `+0x04` to fire it.
3. Poll the byte at `+0x08`. While it reads `0`, keep polling.
4. Once the byte is non-zero, check it against `1` and `3`.
   `1` (OK) and `3` (cancelled by user) are success-shaped; both
   return control to BASIC with no error.
5. Any other terminal value (`2`, `4`, or an unexpected `5`) raises
   `?FC ERROR` (illegal function call).

The disabled state (`4`) appears when the appliance has the
system-action bridge turned off entirely. From BASIC this looks the same as any
other failure: `?FC ERROR`, then the prompt.

## 35.5 Calling the block directly

The block is a normal MMIO region. The IE64, IE32, M68K, and
x86 CPUs reach it at `0xF1400` directly and drive it the same way
the BASIC verb does: write the enum, write the trigger, poll
status, read exit.

The 6502 and Z80 do **not** have a path to this block. Their CPU
adapters translate the small `$F000`-`$FFFF` aperture onto the
`0xF0000`-`0xF0FFF` page only; addresses in the `0xF1xxx` page
have no `16`-bit alias on either adapter, so the HOST appliance
block is unreachable from bare 8-bit code. Programs running on
the 6502 or Z80 ask a larger CPU to drive `HOST` for them through
the coprocessor channel (Chapter 31), or run the verb from BASIC.

From IE64:

```ie64
    la      r2, 0xF1400
    li      r1, #2                 ; UPDATE
    store.b r1, (r2)
    li      r3, #1
    store.b r3, 4(r2)

.poll:
    load.b  r4, 8(r2)
    li      r5, #0
    beq     r4, r5, .poll          ; running
    ; R4 now holds the terminal status
    load.l  r6, 12(r2)             ; R6 = full 32-bit exit code
```

The poll is a tight loop in the example above. Real programs
should run it from a timer-driven interrupt or yield between reads
so that the rest of the system continues to schedule.

## 35.6 What `HOST` does not do

`HOST` is the only BASIC verb that affects the appliance as a
whole. It does not:

- expose a generic shell;
- run arbitrary commands;
- read or write files on the volume (use `LOAD` / `SAVE` and the
  File I/O block, Chapter 34);
- send packets onto the network (the network is configured by
  `HOST NET`, but no BASIC verb sends raw network traffic);
- return any text output to the program. The action prints status
  to the appliance's own surface; the BASIC program only sees the
  terminal status code described above.

`HOST` is the on-ramp for system administration from inside a
program. Anything else the program needs from the outside world
goes through the file-I/O block, the terminal, or one of the input
or media chapters.
