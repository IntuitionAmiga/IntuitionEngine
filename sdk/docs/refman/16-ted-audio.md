---
title: "TED Audio"
sources:
  - ted_constants.go
  - sdk/include/ehbasic_hw_audio.inc
---

# Chapter 16 - TED Audio

TED is the C16/Plus-4 sound-and-video chip. Chapter 6 covers its
video half; this chapter covers its audio half. TED audio is a
small two-voice generator with a shared noise mode and a master
volume - the chip that produced the C16's distinctive bleeps and
arpeggios.

## 16.1 What TED audio can show

| Item              | Value                                  |
|-------------------|----------------------------------------|
| Voices            | `2`, each a square wave                |
| Frequency range   | 10-bit divider per voice               |
| Noise             | One mode, replaces voice 2             |
| Master volume     | `0`–`8` (values `9`–`15` clamp to `8`) |
| Sound clock       | `110,840` Hz PAL / `111,860` Hz NTSC (main clock / 8) |

## 16.2 The register block

The audio block is six bytes wide, at `0xF0F00`–`0xF0F05`. The
layout matches the Plus/4's `$FF0E`–`$FF12` exactly, plus the
IE-specific TED+ control byte.

| Address    | Name           | Plus/4 alias | Purpose |
|------------|----------------|--------------|---------|
| `0xF0F00`  | `TED_FREQ1_LO` | `$FF0E`      | Voice 1 frequency low byte. |
| `0xF0F01`  | `TED_FREQ2_LO` | `$FF0F`      | Voice 2 frequency low byte. |
| `0xF0F02`  | `TED_FREQ2_HI` | `$FF10`      | Voice 2 frequency high bits (low 2 bits). |
| `0xF0F03`  | `TED_SND_CTRL` | `$FF11`      | Sound control (volume, voice enables, noise, DAC). |
| `0xF0F04`  | `TED_FREQ1_HI` | `$FF12`      | Voice 1 frequency high bits (low 2 bits). |
| `0xF0F05`  | `TED_PLUS_CTRL`| -            | TED+ mode enable (bit `0`). |

### 16.2.1 `TED_SND_CTRL` bits

| Bit | Field         | Meaning |
|-----|---------------|---------|
| 0–3 | `VOLUME`      | Master volume `0`–`8`; values `9`–`15` clamp to `8`. |
| 4   | `SND1ON`      | Voice 1 enable. |
| 5   | `SND2ON`      | Voice 2 enable. |
| 6   | `SND2NOISE`   | Voice 2 is in noise mode. |
| 7   | `SNDDC`       | DAC direct-output mode. |

### 16.2.2 Frequency divider

A voice's pitch is set by a 10-bit divider: `FREQn_LO` for the low
byte and bits `0`–`1` of `FREQn_HI` for the upper two bits. The
output frequency is approximately

```
   f = sound_clock / (2 × (1024 − divider))
```

with `sound_clock` = `110,840` Hz on PAL or `111,860` Hz on NTSC.

## 16.3 The TED+ extension

Bit `0` of `TED_PLUS_CTRL` enables **TED+**: extended volume and
DAC modes. With TED+ off, the chip is bit-exact to a Plus/4.

## 16.4 The TED music player

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF0F10`  | `TED_PLAY_PTR`    | Address of the music data. |
| `0xF0F14`  | `TED_PLAY_LEN`    | Length in bytes. |
| `0xF0F18`  | `TED_PLAY_CTRL`   | Bit `0` = start, bit `1` = stop, bit `2` = loop. |
| `0xF0F1C`  | `TED_PLAY_STATUS` | Bit `0` = busy, bit `1` = error. |

The player accepts both the TEDMUSIC (`.tmf`) format and embedded
6502 player binaries.

## 16.5 BASIC keywords

| Form                                | Effect |
|-------------------------------------|--------|
| `TED TONE `*ch*`, `*divider*        | Set voice *ch* (`1`–`2`) frequency. |
| `TED VOL `*level*                   | Set master volume (`0`–`8`). |
| `TED NOISE ON` / `TED NOISE OFF`    | Replace voice 2 with the noise generator. |
| `TED PLUS ON` / `TED PLUS OFF`      | Enable / disable TED+. |
| `TED PLAY `*addr*`[, `*len*`]`      | Start music playback. |
| `TED STOP`                          | Stop playback. |

A BASIC fragment that plays a two-note arpeggio:

```basic
10 POKE &H000F0800, 1                : REM AUDIO_CTRL = on
20 TED VOL 8
30 POKE &H000F0F03, &H38              : REM SND_CTRL: voices 1+2 on, max volume
40 TED TONE 1, 540                    : REM low note
50 TED TONE 2, 600                    : REM detuned
```

## 16.6 Putting it together

TED audio is the simplest two-voice engine on this machine. Most
programs use it for retro arpeggios or as a tinny background
voice. For full Plus/4 tracker music, point `TED_PLAY_PTR`/`LEN`
at the data and write `1` to `TED_PLAY_CTRL`.

The next chapter covers the POKEY, the Atari 8-bit sound chip
with four channels and the SAP playback format.
