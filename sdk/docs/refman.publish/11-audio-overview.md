
# Chapter 11 - Audio Architecture Overview

Where the picture side has six chips feeding one compositor, the
audio side has ten sound engines and a sample player feeding one
**mixer**. The mixer always runs at `44,100` samples per second and
sums the contributions of every active engine into a single stereo
output. The chips themselves are independent - turning one on does
not affect any other - and they can play together.

This chapter sketches the architecture and lists what lives where.
Chapters 12–21 cover the individual engines; Chapter 22 shows how
to drive them all from BASIC and from each CPU.

## 11.1 The sound engines

| Chip / Engine     | Chapter | Heritage                          |
|-------------------|---------|-----------------------------------|
| SoundChip + SFX   | 12      | IE-native 10-channel synth + 4 sample channels |
| PSG / AY-3-8910   | 13      | Sinclair Spectrum / Amstrad CPC / Atari ST |
| SN76489           | 14      | TI / Sega Master System / BBC Micro |
| SID family        | 15      | Commodore 64 (three SID chips: original 6581, 8580, plus IE's "SID+" extensions) |
| TED audio         | 16      | Commodore 16 / Plus-4             |
| POKEY + SAP       | 17      | Atari 8-bit                       |
| AHX               | 18      | Abyss High-Quality Music Format (Amiga) |
| MOD               | 19      | Amiga ProTracker, four channels   |
| WAV / sample      | 20      | Generic 8/16-bit sample player    |
| Paula DMA         | 21      | Amiga Paula-style four-channel DMA sample player |

Every engine has its own MMIO block. The SFX channels share a
small block separate from the SoundChip's tone channels.

## 11.2 The mixer

All engines write into the same output stream. The mixer:

- Sums each engine's stereo contribution after applying that
  engine's own per-channel volume.
- Applies a global filter (low-/high-/band-pass) selected through
  the `FILTER_*` registers at `0xF0820`–`0xF0830`.
- Applies optional overdrive (one `OVERDRIVE_CTRL` register at
  `0xF0A40`) and reverb (`REVERB_MIX` at `0xF0A50` and
  `REVERB_DECAY` at `0xF0A54`).
- Sends the result to the audio output of the Intuition Engine at
  `44,100` samples per second.

The master enable register is `AUDIO_CTRL` at `0xF0800`. Bit `0`
enables audio output; bit `1` freezes the mixer (useful while
swapping presets).

## 11.3 The SoundChip channel block

The IE-native SoundChip exposes its channels through a uniform
**flexible-channel** layout. Each channel is `0x40` bytes wide.
Inside one channel:

| Offset | Field            | Meaning |
|--------|------------------|---------|
| `0x00` | `FREQ`           | Frequency in `16.8` fixed-point Hz (`value / 256.0` = Hz). |
| `0x04` | `VOL`            | Volume, `0`–`255`. |
| `0x08` | `CTRL`           | Gate / control. |
| `0x0C` | `DUTY`           | PWM duty cycle. |
| `0x10` | `SWEEP`          | Pitch sweep configuration. |
| `0x14`–`0x20` | `ATK`/`DEC`/`SUS`/`REL` | ADSR envelope parameters. |
| `0x24` | `WAVE_TYPE`      | Waveform select (square, triangle, sine, saw, noise). |
| `0x28` | `PWM_CTRL`       | PWM rate and depth. |
| `0x2C` | `NOISEMODE`      | Noise generator algorithm (8 modes; see Chapter 12). |
| `0x30` | `PHASE`          | Write to reset the phase. |
| `0x34` | `RINGMOD`        | Ring-modulation source. |
| `0x38` | `SYNC`           | Hard-sync source. |
| `0x3C` | `DAC`            | Signed 8-bit sample (bypasses oscillator). |

There are ten such channels, mapped at three base addresses:

| Channels | Base       | End        |
|----------|------------|------------|
| `0`–`3`  | `0xF0A80`  | `0xF0B7F`  |
| `4`–`6`  | `0xF0C40`  | `0xF0CFF`  |
| `7`–`9`  | `0xF0D40`  | `0xF0DFF`  |

Channels `0`–`3` are the primary block used by BASIC's `SOUND`
keyword. Channels `4`–`9` are reachable directly from machine
language; they form the "SID2" and "SID3" voice groups described
in Chapter 15.

## 11.4 The SFX channels

In addition to the synth channels, there are four dedicated
**SFX** channels that play raw samples from main memory. They live
at `0xF0E80`–`0xF0EFF`, with stride `0x20`. Each channel:

| Offset | Field          | Meaning |
|--------|----------------|---------|
| `0x00` | `SFX_PTR`      | Address of the sample data. |
| `0x04` | `SFX_LEN`      | Sample length in bytes. |
| `0x08` | `SFX_LOOP_PTR` | Loop start (if looped). |
| `0x0C` | `SFX_LOOP_LEN` | Loop length. |
| `0x10` | `SFX_FREQ`     | Playback rate in Hz. |
| `0x14` | `SFX_VOL`      | Volume, `0`–`255`. |
| `0x18` | `SFX_FORMAT`   | `0` = signed 8-bit, `1` = unsigned 8-bit, `2` = signed 16-bit. |
| `0x1C` | `SFX_CTRL`     | Bit `0` = trigger, bit `1` = stop, bit `2` = loop enable. |

A `SFX_STATUS` shadow register reads back bit `0` = playing, bit `1`
= error.

## 11.5 The music-file player

Tracker and chip-tune music does not have to be poked into the
audio chips one register at a time. Intuition Engine has a
**media loader** that reads a file from disk, decodes it, and
drives the appropriate audio engine on the program's behalf.

| Address    | Name             | Purpose |
|------------|------------------|---------|
| `0xF2300`  | `MEDIA_NAME_PTR` | Pointer to a null-terminated filename. |
| `0xF2304`  | `MEDIA_SUBSONG`  | Subsong number. |
| `0xF2308`  | `MEDIA_CTRL`     | `1` = play, `2` = stop. |
| `0xF230C`  | `MEDIA_STATUS`   | `0` = idle, `1` = loading, `2` = playing, `3` = error. |

The BASIC keyword `SOUND PLAY "name"` uses this interface. The
loader picks the right engine from the file extension. Chapter 22
lists every supported extension.

## 11.6 The global effects

| Register     | Address    | Range      | Purpose |
|--------------|------------|------------|---------|
| `FILTER_CUTOFF`     | `0xF0820` | `0`–`255` | Cutoff. |
| `FILTER_RESONANCE`  | `0xF0824` | `0`–`255` | Q. |
| `FILTER_TYPE`       | `0xF0828` | `0`–`3`   | `0` off, `1` LP, `2` HP, `3` BP. |
| `FILTER_MOD_SOURCE` | `0xF082C` | `0`–`3`   | Modulation source channel. |
| `FILTER_MOD_AMOUNT` | `0xF0830` | `0`–`255` | Modulation depth. |
| `OVERDRIVE_CTRL`    | `0xF0A40` | `0`–`255` | Drive amount, `0` = off. |
| `REVERB_MIX`        | `0xF0A50` | `0`–`255` | Dry/wet. |
| `REVERB_DECAY`      | `0xF0A54` | `0`–`255` | Tail length. |

These effects sit at the end of the mixer chain and apply to the
summed output of every engine.

## 11.7 BASIC keywords

| Keyword                            | Routes to |
|------------------------------------|-----------|
| `SOUND `*ch*`, `*freq*`, `*vol*` …`| SoundChip (Chapter 12) |
| `SOUND PLAY "`*file*`"`            | Media loader (Chapter 22) |
| `SOUND STOP`                       | Media loader |
| `SOUND FILTER`/`REVERB`/`OVERDRIVE`| Mixer effects |
| `ENVELOPE`                         | SoundChip ADSR |
| `GATE`                             | SoundChip gate |
| `PSG `*…*                          | AY-3-8910 (Chapter 13) |
| `SID `*…*                          | SID family (Chapter 15) |
| `POKEY `*…*                        | POKEY (Chapter 17) |
| `AHX `*…*                          | AHX engine (Chapter 18) |
| `SAP`                              | POKEY SAP playback |

## 11.8 What comes next

Chapter 12 covers the SoundChip and SFX in full. The remaining
chapters of Part III take one chip each.
