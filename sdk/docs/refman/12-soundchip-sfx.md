---
title: "SoundChip and SFX"
sources:
  - audio_chip.go
  - sfx_constants.go
  - sdk/include/ehbasic_hw_audio.inc
---

# Chapter 12 - SoundChip and SFX

The SoundChip is Intuition Engine's primary synth. It has ten
flexible channels - each one a full oscillator with envelope,
sweep, ring-modulation, hard-sync, PWM, and DAC inputs - plus four
SFX sample channels. All channels are mixed into the global mixer
described in Chapter 11.

This chapter describes one channel in detail, then lists the SFX
channel registers, then shows the BASIC `SOUND` keyword's options.

## 12.1 A SoundChip channel

A channel is a `0x40`-byte block. Inside one channel:

| Offset | Name        | Bits | Purpose |
|--------|-------------|------|---------|
| `0x00` | `FREQ`      | 32   | Frequency in 16.8 fixed-point Hz. |
| `0x04` | `VOL`       | 8    | Output volume, `0`–`255`. |
| `0x08` | `CTRL`      | 8    | Gate and per-channel control. Bit `1` = gate. |
| `0x0C` | `DUTY`      | 8    | PWM duty (square wave) or pulse parameter. |
| `0x10` | `SWEEP`     | 8    | Sweep enable / direction / period / shift. |
| `0x14` | `ATK`       | 8    | Envelope attack. |
| `0x18` | `DEC`       | 8    | Envelope decay. |
| `0x1C` | `SUS`       | 8    | Envelope sustain. |
| `0x20` | `REL`       | 8    | Envelope release. |
| `0x24` | `WAVE_TYPE` | 8    | Waveform select (see below). |
| `0x28` | `PWM_CTRL`  | 8    | Bit `7` = PWM enable, bits `0`–`6` = rate. |
| `0x2C` | `NOISEMODE` | 8    | Noise generator algorithm (see below). |
| `0x30` | `PHASE`     | 8    | Write to reset the oscillator phase. |
| `0x34` | `RINGMOD`   | 8    | Bit `7` = enable, bits `0`–`2` = source channel. |
| `0x38` | `SYNC`      | 8    | Bit `7` = enable, bits `0`–`2` = source channel. |
| `0x3C` | `DAC`       | 8    | Signed 8-bit sample (bypasses oscillator + envelope). |

### 12.1.1 Channel locations

Ten channels are mapped in three blocks:

| Channels | Base       | Stride  | End       |
|----------|------------|---------|-----------|
| `0`–`3`  | `0xF0A80`  | `0x40`  | `0xF0B7F` |
| `4`–`6`  | `0xF0C40`  | `0x40`  | `0xF0CFF` |
| `7`–`9`  | `0xF0D40`  | `0x40`  | `0xF0DFF` |

Channels `0`–`3` are the **primary** group used by BASIC's `SOUND`
keyword. Channels `4`–`6` and `7`–`9` are the "SID2" and "SID3"
voice groups (Chapter 15) that give the chip three SID voices on
top of its primary four. Every channel uses the same register
layout shown in §12.1.

### 12.1.2 Waveforms

Each channel's `WAVE_TYPE` register selects one of five waveforms.
The values are zero-based:

| `WAVE_TYPE` | Waveform |
|-------------|----------|
| `0`         | Square (with `DUTY` controlling pulse width) |
| `1`         | Triangle |
| `2`         | Sine |
| `3`         | Sawtooth |
| `4`         | Noise (algorithm chosen by `NOISEMODE`) |

### 12.1.3 Noise modes

When `WAVE_TYPE = 4`, the noise generator is configured by
`NOISEMODE`:

| `NOISEMODE` | Algorithm                          |
|-------------|------------------------------------|
| `0`         | White (LFSR, default) |
| `1`         | Periodic / loop |
| `2`         | Metallic |
| `3`         | PSG-style (AY/YM) |
| `4`         | TED 8-bit |
| `5`         | SN76489 15-bit white |
| `6`         | SN76489 15-bit periodic |
| `7`         | SN76489 16-bit white |
| `8`         | SN76489 16-bit periodic |

This is how the SoundChip reproduces the noise characteristics of
the other chips' channels when used inside a single track.

### 12.1.4 Envelopes

Each channel has its own ADSR envelope, parameterised by `ATK`,
`DEC`, `SUS`, `REL`. Five envelope shapes are selectable through
the shared `ENV_SHAPE` register at `0xF0804`:

| `ENV_SHAPE` | Behaviour |
|-------------|-----------|
| `0`         | Standard ADSR (default). |
| `1`         | Saw-up: linear rise to `1.0`, then hold. |
| `2`         | Saw-down: linear fall to `0.0`, then hold. |
| `3`         | Loop: ADSR but loops after release. |
| `4`         | SID-style exponential ADSR. |

Per-channel shape registers live at `ENV_SHAPE_CH_BASE + ch*4`
(`0xF0860`).

### 12.1.5 Sweep, ring-mod, sync

The `SWEEP` register encodes a pitch sweep:

| Bit  | Field           | Meaning |
|------|-----------------|---------|
| 7    | Sweep enable    | `0` = off, `1` = on. |
| 3    | Direction       | `0` = up, `1` = down. |
| 4–6  | Period          | Speed (`0`–`7`). |
| 0–2  | Shift           | Amount per step (`0`–`7`). |

`RINGMOD` selects another channel as the ring-modulator source;
when bit `7` is set, the channel's output is multiplied by the
source's instantaneous amplitude. `SYNC` does the same for hard
sync: when the source crosses zero, the channel's oscillator is
forced to reset its phase.

### 12.1.6 DAC mode

Writing a signed 8-bit value to a channel's `DAC` register
overrides the oscillator and envelope for that sample and outputs
the byte directly. This lets a CPU drive the channel as a raw
sample player.

## 12.2 The SFX channels

In addition to the ten synth channels, the SoundChip has four
**SFX channels** that play samples from main memory. They live
in their own register block:

| Address    | Channel | Stride |
|------------|---------|--------|
| `0xF0E80`  | `0`     | `0x20` |
| `0xF0EA0`  | `1`     | `0x20` |
| `0xF0EC0`  | `2`     | `0x20` |
| `0xF0EE0`  | `3`     | `0x20` |

Inside one channel:

| Offset | Name             | Purpose |
|--------|------------------|---------|
| `0x00` | `SFX_PTR`        | Sample data address. |
| `0x04` | `SFX_LEN`        | Sample length, in bytes. |
| `0x08` | `SFX_LOOP_PTR`   | Loop start (if looped). |
| `0x0C` | `SFX_LOOP_LEN`   | Loop length. |
| `0x10` | `SFX_FREQ`       | Playback rate in Hz. |
| `0x14` | `SFX_VOL`        | Volume, `0`–`255`. |
| `0x18` | `SFX_FORMAT`     | `0` = signed 8-bit, `1` = unsigned 8-bit, `2` = signed 16-bit. |
| `0x1C` | `SFX_CTRL`       | Bit `0` = trigger, bit `1` = stop, bit `2` = loop enable. |

The block ends at `0xF0EFF`.

`SFX_CTRL` is also the status read-back: bit `0` = playing,
bit `1` = error.

## 12.3 BASIC keywords

The `SOUND` keyword groups several subcommands. See Chapter 2 for
full syntax.

| Form                                                         | Effect |
|--------------------------------------------------------------|--------|
| `SOUND `*ch*`, `*freq*`, `*vol*`[ , `*wave*`[ , `*duty*`]]`  | Set frequency, volume, and (optionally) waveform and duty cycle on channel `0`–`3`. |
| `SOUND FILTER `*cutoff*`, `*resonance*`, `*type*             | Configure the global filter. |
| `SOUND REVERB `*mix*`, `*decay*                              | Configure the global reverb. |
| `SOUND OVERDRIVE `*amount*                                   | Configure the global overdrive. |
| `SOUND NOISE `*ch*`, `*mode*                                 | Set the noise mode on channel `0`–`3`. |
| `SOUND WAVE `*ch*`, `*type*                                  | Set the waveform type on channel `0`–`3`. |
| `SOUND SWEEP `*ch*`, `*enable*`, `*period*`, `*shift*        | Configure a pitch sweep. |
| `SOUND SYNC `*ch*`, `*source*                                | Configure hard sync. |
| `SOUND RINGMOD `*ch*`, `*source*                             | Configure ring modulation. |
| `SOUND PLAY "`*file*`"[, `*subsong*`]`                       | Play a music file (Chapter 22). |
| `SOUND STOP`                                                 | Stop the current music. |
| `ENVELOPE `*ch*`, `*atk*`, `*dec*`, `*sus*`, `*rel*          | Set the ADSR envelope on channel `0`–`3`. |
| `GATE `*ch*`, ON` / `GATE `*ch*`, OFF`                       | Open or close a channel's gate. |

`SOUND` writes 16.8 fixed-point frequency (`Hz × 256`) into the
`FREQ` register and the volume into `VOL`. The optional waveform
and duty arguments go into `WAVE_TYPE` and `DUTY`.

## 12.4 A short example

A BASIC fragment that plays a sustained square-wave A4 (440 Hz)
through channel `0`:

```basic
10 POKE &H000F0800, 1                : REM AUDIO_CTRL = on
20 ENVELOPE 0, 50, 100, 200, 100     : REM ADSR
30 SOUND 0, 440, 200, 0, 128         : REM square, duty = 128
40 GATE 0, ON
```

To stop the note cleanly, `GATE 0, OFF`. To stop it abruptly,
write `0` to `SOUND 0, 0, 0`.

## 12.5 Putting it together

A SoundChip channel is the same kind of object whichever number it
has. Most programs use channels `0`–`3` for melody, then layer
percussion or noise on the higher channels. The SFX channels are
the fastest way to play short samples (gunshots, beeps, voice)
without disturbing the synth voices.

The next chapter covers the AY-3-8910 / YM-2149 PSG, the first of
the heritage engines.
