
# Chapter 13 - PSG and AY-3-8910

The PSG (Programmable Sound Generator) is the General Instrument
AY-3-8910 and its Yamaha-relabelled YM-2149 sibling: a three-voice
square-wave chip with a noise generator, an envelope generator,
and two general-purpose I/O ports. Intuition Engine's PSG is
faithful to both, so existing ZX Spectrum 128, Amstrad CPC, MSX,
and Atari ST PSG music plays unchanged.

## 13.1 What the PSG can show

| Item              | Value                                  |
|-------------------|----------------------------------------|
| Tone channels     | `3` (A, B, C)                          |
| Noise generator   | `1`, shared between channels via mixer |
| Envelope          | `1`, shared between channels via control |
| Volume control    | Per-channel 4-bit volume               |
| Register count    | `16` (`0x00`–`0x0F`)                   |

## 13.2 The register block

The PSG sits at `0xF0C00`–`0xF0C0F`. Each register is one byte at a
4-byte-stride MMIO slot. The standard AY-3-8910 register layout is
preserved:

| Address    | Reg | Name                            |
|------------|-----|---------------------------------|
| `0xF0C00`  | `0` | Channel A frequency, low byte   |
| `0xF0C01`  | `1` | Channel A frequency, high nibble |
| `0xF0C02`  | `2` | Channel B frequency, low byte   |
| `0xF0C03`  | `3` | Channel B frequency, high nibble |
| `0xF0C04`  | `4` | Channel C frequency, low byte   |
| `0xF0C05`  | `5` | Channel C frequency, high nibble |
| `0xF0C06`  | `6` | Noise period, 5 bits             |
| `0xF0C07`  | `7` | Mixer / I/O control              |
| `0xF0C08`  | `8` | Channel A level                  |
| `0xF0C09`  | `9` | Channel B level                  |
| `0xF0C0A`  | `10`| Channel C level                  |
| `0xF0C0B`  | `11`| Envelope period, low byte        |
| `0xF0C0C`  | `12`| Envelope period, high byte       |
| `0xF0C0D`  | `13`| Envelope shape (4 bits)          |
| `0xF0C0E`  | `14`| I/O port A                       |
| `0xF0C0F`  | `15`| I/O port B                       |

### 13.2.1 Channel frequency

A channel's pitch is set by a 12-bit divider: bits `0`–`7` from the
low register and bits `8`–`11` from the low nibble of the high
register. The output frequency is

```
   f = clock / (16 × divider)
```

with the AY-style clock typically chosen from the machine being
reproduced. The IE ships these clock constants for fidelity:

| Clock                  | Value (Hz) |
|------------------------|------------|
| `PSG_CLOCK_ATARI_ST`   | `2,000,000` |
| `PSG_CLOCK_ZX_SPECTRUM`| `1,773,400` |
| `PSG_CLOCK_CPC`        | `1,000,000` |
| `PSG_CLOCK_MSX`        | `1,789,773` |

### 13.2.2 Mixer (register `7`)

| Bit | Field      | Meaning                  |
|-----|------------|--------------------------|
| 0   | `~TONE_A`  | `0` = tone A active.     |
| 1   | `~TONE_B`  | `0` = tone B active.     |
| 2   | `~TONE_C`  | `0` = tone C active.     |
| 3   | `~NOISE_A` | `0` = noise routed to A. |
| 4   | `~NOISE_B` | `0` = noise routed to B. |
| 5   | `~NOISE_C` | `0` = noise routed to C. |
| 6   | `IO_A_OUT` | `1` = I/O port A is output. |
| 7   | `IO_B_OUT` | `1` = I/O port B is output. |

Bits `0`–`5` are inverted by convention: writing `0` enables the
voice.

### 13.2.3 Level (registers `8`–`10`)

| Bit | Field     | Meaning |
|-----|-----------|---------|
| 0–3 | Level     | Volume `0`–`15`. |
| 4   | `ENV_EN`  | `1` = use envelope; level bits ignored. |

### 13.2.4 Envelope shape (register `13`)

| Value | Shape                              |
|-------|------------------------------------|
| `0`–`3`  | Decay, hold low.                |
| `4`–`7`  | Attack, hold low.               |
| `8`   | Saw down (repeat).                 |
| `9`   | Decay, hold low (one-shot).        |
| `10`  | Triangle (alternating).            |
| `11`  | Decay, hold high.                  |
| `12`  | Saw up (repeat).                   |
| `13`  | Attack, hold high.                 |
| `14`  | Triangle (alternating, inverted).  |
| `15`  | Attack, hold low.                  |

Writing to register `13` restarts the envelope, so the same shape
value is the conventional retrigger mechanism.

## 13.3 The PSG+ extension

Bit `0` of the IE-specific `PSG_PLUS_CTRL` register at `0xF0C20`
enables **PSG+** mode. When PSG+ is on, the chip widens its level
registers from 4 bits to 8 bits and offers per-channel pan; when
off, the chip behaves exactly like an AY-3-8910.

## 13.4 Hardware-mode AY playback

The PSG also has a small player that streams a register-frame log
straight into the chip. This is the format used by Vortex Tracker
and most ZX Spectrum 128 music players.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF0C10`  | `PSG_PLAY_PTR`    | Address of the register-frame stream. |
| `0xF0C14`  | `PSG_PLAY_LEN`    | Stream length in bytes. |
| `0xF0C18`  | `PSG_PLAY_CTRL`   | `1` = play, `0` = stop. |
| `0xF0C1C`  | `PSG_PLAY_STATUS` | Read-only status. |

Each `PSG_REG_COUNT` (`16`) bytes in the stream is one PSG frame.
The chip applies frames at the rate of the music being played
(`50` Hz for ZX Spectrum, `60` Hz for NTSC titles, etc.).

## 13.5 BASIC keywords

| Form                                  | Effect |
|---------------------------------------|--------|
| `PSG `*ch*`, `*freq*`, `*vol*         | Set channel *ch*'s 8-bit frequency divider and 4-bit volume. |
| `PSG MIXER `*value*                   | Write the mixer/I/O byte (register `7`). |
| `PSG ENVELOPE `*shape*`, `*period*    | Set the envelope shape and 16-bit period. |
| `PSG PLUS ON` / `PSG PLUS OFF`        | Enable / disable PSG+ mode. |
| `PSG PLAY `*addr*`[, `*len*`]`        | Start hardware-mode AY playback. |
| `PSG STOP`                            | Stop AY playback. |

A BASIC fragment that plays a tone on channel A:

```basic
10 POKE &H000F0800, 1                 : REM AUDIO_CTRL = on
20 PSG MIXER &H38                     : REM tones on, noise off
30 PSG 0, 113, 15                     : REM divider 113, level 15
```

The exact divider for a desired frequency depends on the chip
clock chosen; see §13.2.1.

## 13.6 Putting it together

To use the PSG: route tone or noise to each channel through the
mixer, set each channel's divider and level, and optionally
trigger the envelope by writing register `13`. For existing
chip-tune files, point `PSG_PLAY_PTR`/`LEN` at the register-frame
stream and write `1` to `PSG_PLAY_CTRL`.

The next chapter covers the SN76489, the PSG's three-voice cousin
from Texas Instruments.
