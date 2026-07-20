# IE USB Gamepad MMIO

The gamepad block is a single read-only memory-mapped register block that the
host fills once per frame and every guest reads. Controller differences are
resolved on the host by the Ebiten standard-layout mapping database, so guests
always see a fixed, vendor-neutral canonical layout. There is no SDL
dependency.

Base range: `0xF25C0-0xF25FF` (64 bytes). Writes are accepted but ignored, so
guest stores never fault under strict MMIO policy.

## Register map

| Address | Name | Width | Description |
|---------|------|-------|-------------|
| `0xF25C0` | `GAMEPAD_STATUS` | u32 | bits 0..3 pad connected mask, bits 8..11 connected count |

Each pad `p` (0..3) has a 12-byte record at `0xF25D0 + p*0x0C`:

| Offset | Name | Width | Description |
|--------|------|-------|-------------|
| `+0x00` | `BUTTONS` | u32 | Canonical button bitfield, latched at frame start |
| `+0x04` | `AXIS_LXY` | u32 | Left stick: X in low 16 bits, Y in high 16 bits (signed) |
| `+0x08` | `AXIS_RXY` | u32 | Right stick: X in low 16 bits, Y in high 16 bits (signed) |

## Canonical button bits

Fixed and vendor neutral, taken from Ebiten `StandardGamepadButton`:

```
0 Up    1 Down   2 Left   3 Right
4 A     5 B      6 X      7 Y
8 LB    9 RB     10 LT    11 RT    (digital triggers)
12 Select  13 Start  14 L3  15 R3  16 Home
```

Triggers are exposed as the digital `LT`/`RT` bits only. Ebiten exposes
standard triggers as buttons, not as portable analogue axes, so no separate
analogue trigger register is kept; this makes the ABI honest and lets four
12-byte records fit the block exactly.

## Axis semantics

Each stick axis is an Ebiten float clamped to the range -1..1 and scaled to
signed 16-bit (`value * 32767`). A NaN maps to 0. The published direction is
Ebiten's native direction, down and right positive; this is ABI. Guests that
prefer up-positive should negate the Y axis themselves.

## Addressing per architecture

The flat-addressing architectures (IE64, IE32, M68K, x86) reach the block
directly at `0xF25C0`. The two 16-bit CPUs cannot address it directly because
their hardwired `$Fxxx` window only reaches bus `$F0xxx`; they select the
extended bank window instead.

| Arch include | Access |
|--------------|--------|
| `ie64.inc` | `GAMEPAD_REGION_BASE equ 0xF25C0` (direct) |
| `ie32.inc` | `.equ GAMEPAD_REGION_BASE 0xF25C0` (direct) |
| `ie68.inc` | `GAMEPAD_REGION_BASE equ $F25C0` (direct) |
| `ie86.inc` | `GAMEPAD_REGION_BASE equ 0xF25C0` (direct) |
| `ie65.inc` | `GAMEPAD_IO_BANK = $0079`, window `GAMEPAD_STATUS = $25C0` (BANK1) |
| `ie80.inc` | `.set GAMEPAD_IO_BANK,0x0079`, window `.set GAMEPAD_STATUS,0x25C0` |

For the banked CPUs the effective bus address is `bank * 0x2000 + window
offset`: bank `0x79` gives `0xF2000`, plus offset `0x5C0`, seen as window
`$25C0` inside the `$2000-$3FFF` Bank 1 window. Each include also ships a
`SET_GAMEPAD_BANK` macro and the `JOY_*` button-bit constants.

## Guest operating systems (EmuTOS, AROS)

The EmuTOS and AROS images shipped here are IE-native M68K builds. They already
consume IE memory-mapped input directly: EmuTOS reads scancodes from
`SCAN_CODE` (`$F0740`) and AROS reads the terminal block in Amiga rawkey mode.
Neither build emulates Atari IKBD joystick packets or Amiga CIA and custom-chip
registers, so there is no native joystick hardware for a host-side adapter to
mirror. Because M68K is flat-addressing, both guests reach the gamepad block
directly at `$F25C0` through the `ie68.inc` equates, the same way they reach the
keyboard and mouse registers. No per-guest Go adapter is needed for native
access, and a native M68K read of the block is covered by
`TestGamepad_M68KGuestReadsCanonicalBlock`.

Wiring the block into the upstream joystick APIs (EmuTOS `Ikbd` joystick
packets, AROS `lowlevel.library/ReadJoyPort`) so unmodified controller code
sees a gamepad is deferred. That is guest-side driver work in the external
EmuTOS `MACHINE_IE` and `AROS-deadw00d` source trees, not a Go shim, and would
require standing up native IKBD or CIA hardware this build does not otherwise
provide. It is a separate change from the MMIO block and its native reader.

## Assembly example (IE64)

```
    move.l  r1, #GAMEPAD_PAD0_BASE     ; pad 0 record base
    load.l  r2, (r1)                   ; buttons
    and.l   r3, r2, #JOY_A             ; A pressed?
    beqz    r3, .not_pressed
```

## BASIC example

IE64 BASIC exposes three builtins: `PAD(n)` returns pad `n`'s button bitfield,
`PADX(n)` and `PADY(n)` return the signed left-stick axes. An out-of-range pad
index returns 0. Merge `joydefs.bas` for symbolic button names in the `JOYxxx`
form (BASIC variable names cannot contain `_`, so the library uses `JOYA`,
`JOYHOME`, and so on). Its definitions live at lines `60010`-`60026` and end
with `RETURN`, so call them once with `GOSUB 60000` before use.

```
10 MERGE "joydefs.bas"
20 GOSUB 60000
30 B = PAD(0)
40 IF B AND JOYA THEN PRINT "A pressed"
50 PRINT "stick:"; PADX(0); PADY(0)
60 GOTO 30
```

`MERGE` loads programme lines from a file into the current programme without a
`NEW`, so the `joydefs.bas` lines join your programme and existing lines and
variables are kept.

Both the interpreter and the RUN AOT compiler lower these builtins identically.
