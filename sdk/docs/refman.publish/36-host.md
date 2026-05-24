
# Chapter 36 - The HOST Command

`HOST` is the BASIC verb for whole-machine service actions. It can
open the network configuration screen, update installed system
packages, reboot the appliance, power it off, or print help.

`HOST` is a statement form, not an expression. It must begin a
statement. In expressions and explicit `LET` assignments, the same
letters may still be used as an ordinary variable name.

## 36.1 Subverbs

| Form               | What it does |
|--------------------|--------------|
| `HOST NET`         | Opens the network configuration screen. |
| `HOST UPDATE`      | Updates the installed system packages. |
| `HOST REBOOT`      | Reboots the appliance. |
| `HOST POWEROFF`    | Powers off the appliance. |
| `HOST HELP`        | Prints the list of `HOST` subverbs. |

`HOST` on its own is the same as `HOST HELP`. `HOST:` is also the
help form, so it can start a colon-separated line.

Any other word after `HOST` is a syntax error. The statement stops
before a service action is requested.

## 36.2 Typed Help Example

This is the safe first example. It does not touch the service-action
registers.

```basic
10 HOST HELP
20 PRINT "AFTER HELP"
```

Expected output begins like this:

```text
HOST commands:
HOST NET - configure WiFi
HOST UPDATE - update system packages
HOST REBOOT
HOST POWEROFF
HOST HELP
AFTER HELP
```

`HOST HELP` returns to BASIC, so line `20` still runs.
It is safe to type because it only prints the built-in help text.
It does not write the service-action MMIO trigger.

## 36.3 What Each Subverb Does

### 36.3.1 `HOST NET`

Opens the network configuration screen. BASIC resumes at the next
statement after the screen closes. If the user cancels, BASIC treats
that as a clean return rather than an error.

### 36.3.2 `HOST UPDATE`

Updates installed system packages. The appliance asks for
confirmation before the update is applied. If the user cancels,
BASIC continues with the next statement.

### 36.3.3 `HOST REBOOT`

Reboots the appliance. The running program stops. BASIC returns only
after the boot sequence has completed, so statements after
`HOST REBOOT` on the same run are not reached.

### 36.3.4 `HOST POWEROFF`

Powers the appliance off. The running program does not return. The
next BASIC session starts after a manual power-on.

### 36.3.5 `HOST HELP`

Prints the fixed help text shown above. The help text is stored with
BASIC and is not configurable.

## 36.4 Register Block

`HOST` uses a four-register MMIO block at `$F1400`. Registers are
`32`-bit when read or written with `PEEK` and `POKE`. The command,
trigger, and status values use the low byte; the exit register is a
full `32`-bit value.

| Address | Name | R/W | Meaning |
|---------|------|-----|---------|
| `$F1400` | command | W/R | Subverb enum: `1` NET, `2` UPDATE, `3` REBOOT, `4` POWEROFF, `0` none. |
| `$F1404` | trigger | W | Write a non-zero value to start the selected command. |
| `$F1408` | status | R | Current state of the command. |
| `$F140C` | exit | R | Exit code from the action, valid after a terminal status. |

Status values:

| Value | State | Terminal? |
|-------|-------|-----------|
| `0` | running | no |
| `1` | OK | yes |
| `2` | error | yes |
| `3` | cancelled by user | yes |
| `4` | disabled | yes |
| `5` | idle | yes |

Exit-code values used by the built-in actions:

| Value | Meaning |
|-------|---------|
| `0` | OK |
| `2` | Bad command input |
| `10` | Network password failed |
| `11` | No network signal |
| `12` | Unsupported network authentication |
| `13` | Network timeout |
| `20` | Package update step failed |
| `21` | Package upgrade step failed |

## 36.5 Read The Status Register

This direct MMIO example is safe: it only reads the current status.
Before a command has been fired, the status is normally `5`.

```basic
10 PRINT "HOST STATUS ";PEEK(&H000F1408)
```

Expected result before a command:

```text
HOST STATUS 5
```

If a command is already running, the value may be `0`. After a
completed command it will be one of the terminal values in the table
above.

This listing reads the same status register that the real `HOST`
subverbs poll after firing a command. It is a read-only check, so it
is the right example to type before experimenting with `HOST NET`,
`HOST UPDATE`, `HOST REBOOT`, or `HOST POWEROFF`.

## 36.6 BASIC Protocol

The BASIC verb performs this sequence:

1. Write the subverb enum to `$F1400`.
2. Write `1` to `$F1404`.
3. Poll `$F1408` while it reads `0`.
4. Treat status `1` and status `3` as clean returns to BASIC.
5. Treat status `2`, `4`, or an unexpected `5` as `?FC ERROR`.

State `4` means service actions are disabled. From BASIC this appears
as `?FC ERROR` after a requested action.

## 36.7 Direct Register Use

IE64, IE32, M68K, and x86 programs can reach `$F1400` directly. They
drive the block the same way BASIC does: write the command enum, write
the trigger, poll status, then read the exit code.

The 6502 and Z80 do not have a direct route to this block. Their
small-address MMIO apertures reach the `$F0000` page, not the
`$F1xxx` service-action page. Code running on those CPUs should ask a
larger CPU through the coprocessor channel, or use the BASIC `HOST`
verb.

Do not trigger `HOST REBOOT` or `HOST POWEROFF` from a test program
unless the power action is the result you want.

## 36.8 Limits

`HOST` does not expose a command shell. It does not run arbitrary
commands, read files, write files, or send network packets from a
BASIC program. File work goes through Chapter 35. Keyboard and
terminal traffic go through Chapters 37 and 38.
