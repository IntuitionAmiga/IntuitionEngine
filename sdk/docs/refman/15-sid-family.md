---
title: "The SID Family"
sources:
  - sid_constants.go
  - audio_chip.go
  - sdk/include/ehbasic_hw_audio.inc
  - sdk/include/ie64.inc
---

# Chapter 15 - The SID Family

The SID (Sound Interface Device) is the MOS 6581/8580 chip that
shipped in the Commodore 64. Intuition Engine has **three** SID
instances available simultaneously, named SID, SID2, and SID3.
Each one is a complete SID with three voices, a filter, and a
master volume. Together they give the IE nine SID voices,
matching the conventions of 6SID-style C64 demo music.

The IE adds a small extension called **SID+** that widens the
chip's 4-bit volume registers to 8 bits and lets a program
override the C64's master volume with a per-voice level. SID+ is
off by default; with it off, the chip is bit-exact compatible
with both the 6581 and the 8580.

## 15.1 What the SID can show

| Item               | Value                                  |
|--------------------|----------------------------------------|
| Voices per chip    | `3`, each with its own oscillator, envelope, control |
| Waveforms          | Triangle, sawtooth, pulse (with PWM), noise |
| Filter             | Low-pass / high-pass / band-pass with resonance |
| Envelope           | ADSR, exponential (SID-style) |
| Modulation         | Hard sync and ring modulation with the previous voice |
| Master volume      | 4-bit, 8-bit in SID+ mode |
| Chips available    | `3` independent SIDs (SID, SID2, SID3) |

## 15.2 The register block

Each SID is `0x1D` bytes wide. The three SIDs are mapped at:

| Chip  | Base       | End        |
|-------|------------|------------|
| SID   | `0xF0E00`  | `0xF0E1C`  |
| SID2  | `0xF0E30`  | `0xF0E4C`  |
| SID3  | `0xF0E50`  | `0xF0E6C`  |

The register layout below is for SID; SID2 and SID3 use the same
offsets relative to their bases.

### 15.2.1 Voice registers

Each voice has seven registers; voice `n` (`n` = `1`, `2`, `3`)
lives at offset `(n − 1) * 7`:

| Offset | Name      | Purpose                              |
|--------|-----------|--------------------------------------|
| `0x00` | `FREQ_LO` | Frequency low byte                   |
| `0x01` | `FREQ_HI` | Frequency high byte                  |
| `0x02` | `PW_LO`   | Pulse width low byte                 |
| `0x03` | `PW_HI`   | Pulse width high (low nibble only)   |
| `0x04` | `CTRL`    | Control byte (see below)             |
| `0x05` | `AD`      | Attack / decay (high / low nibble)   |
| `0x06` | `SR`      | Sustain / release                    |

`CTRL` bits:

| Bit | Name       | Meaning |
|-----|------------|---------|
| 0   | `GATE`     | `1` starts the envelope; `0` releases it. |
| 1   | `SYNC`     | Hard-sync to the previous voice. |
| 2   | `RINGMOD`  | Ring-modulate with the previous voice. |
| 3   | `TEST`     | Reset oscillator. |
| 4   | `TRIANGLE` | Enable triangle waveform. |
| 5   | `SAWTOOTH` | Enable sawtooth waveform. |
| 6   | `PULSE`    | Enable pulse waveform (uses `PW`). |
| 7   | `NOISE`    | Enable noise waveform. |

The waveform bits OR together: setting more than one combines
them, as on the real chip.

### 15.2.2 Filter and master volume

| Address    | Name           | Purpose |
|------------|----------------|---------|
| `0xF0E15`  | `FC_LO`        | Filter cutoff low (3 bits). |
| `0xF0E16`  | `FC_HI`        | Filter cutoff high byte. |
| `0xF0E17`  | `RES_FILT`     | Resonance (bits `4`–`7`) and per-voice routing (bits `0`–`3`). |
| `0xF0E18`  | `MODE_VOL`     | Master volume (bits `0`–`3`), filter mode (bits `4`–`6`), voice-3 mute (bit `7`). |
| `0xF0E19`  | `SID_PLUS_CTRL`| Bit `0` = SID+ mode enable. |

`RES_FILT` routing bits:

| Bit | Field    | Meaning |
|-----|----------|---------|
| 0   | `FILT_V1`| Route voice 1 through the filter. |
| 1   | `FILT_V2`| Route voice 2 through the filter. |
| 2   | `FILT_V3`| Route voice 3 through the filter. |
| 3   | `FILT_EXT`| Route external input through the filter. |
| 4–7 | `RES`    | Filter resonance, `0`–`15`. |

`MODE_VOL` bits `4`–`7`:

| Bit | Field   | Meaning |
|-----|---------|---------|
| 4   | `LP`    | Low-pass enabled. |
| 5   | `BP`    | Band-pass enabled. |
| 6   | `HP`    | High-pass enabled. |
| 7   | `3OFF`  | Voice 3 mute. |

### 15.2.3 Read-only registers

The original SID exposes potentiometer, oscillator-3, and
envelope-3 read-back registers at offsets `0x19`–`0x1C`. These
addresses also overlap with the IE-specific player registers
listed in §15.3.

## 15.3 The C64-style player

Each SID instance has a small player that streams C64-style SID
data into the chip. The block lives just after the read-only
registers of the primary SID:

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF0E20`  | `SID_PLAY_PTR`    | Address of the music data. |
| `0xF0E24`  | `SID_PLAY_LEN`    | Length in bytes. |
| `0xF0E28`  | `SID_PLAY_CTRL`   | `1` = play, `2` = stop. |
| `0xF0E2C`  | `SID_PLAY_STATUS` | Read-only status. |
| `0xF0E2D`  | `SID_SUBSONG`     | Subsong number (`0`–`255`). |

## 15.4 Chip model

Two SID variants are available: `SID_MODEL_6581` (the original,
warmer chip with a non-linear filter) and `SID_MODEL_8580` (the
revised chip with a cleaner filter). The IE selects between them
through internal configuration; their externally visible registers
are identical.

## 15.5 SID+ extension

Writing `1` to `SID_PLUS_CTRL` enables **SID+**:

- Master volume becomes 8-bit (`MODE_VOL` bits `0`–`3` map to a
  wider range).
- Each voice gains an independent volume that overrides the
  master.
- The chip otherwise behaves as a standard SID.

Writing `0` returns to bit-exact SID behaviour.

## 15.6 BASIC keywords

| Form                                                                | Effect |
|---------------------------------------------------------------------|--------|
| `SID VOICE `*v*`, `*freq*`, `*pw*`, `*ctrl*`, `*ad*`, `*sr*         | Program one voice (`v` is `1`–`3`). |
| `SID VOLUME `*level*                                                | Set master volume (`0`–`15`). |
| `SID FILTER `*cutoff*`, `*resonance*`, `*routing*`, `*mode*         | Configure the filter. |
| `SID PLUS ON` / `SID PLUS OFF`                                      | Enable / disable SID+. |
| `SID PLAY `*addr*`[, `*len*` [, `*subsong*`]]`                      | Start hardware-mode SID playback. |
| `SID STOP`                                                          | Stop SID playback. |

A BASIC fragment that plays a sustained C-major triad on the
primary SID:

```basic
10 POKE &H000F0800, 1                : REM AUDIO_CTRL = on
20 SID VOLUME 15
30 SID VOICE 1, 4291, 0, &H41, &H00, &HF0  : REM C, pulse + gate
40 SID VOICE 2, 5407, 0, &H41, &H00, &HF0  : REM E
50 SID VOICE 3, 6430, 0, &H41, &H00, &HF0  : REM G
```

## 15.7 Putting it together

A SID program runs at three levels. From BASIC: drive a single SID
with `SID VOICE`/`FILTER`/`VOLUME`. From machine language: address
SID, SID2, and SID3 directly through their MMIO blocks for full
nine-voice arrangements. For existing C64 music files, point
`SID_PLAY_PTR`/`LEN` at the data and write `1` to `SID_PLAY_CTRL`.

The next chapter covers TED audio, the C16's two-voice sibling.
