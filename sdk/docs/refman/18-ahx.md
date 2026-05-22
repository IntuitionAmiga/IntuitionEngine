---
title: "The AHX Engine"
sources:
  - ahx_constants.go
  - ahx_engine.go
  - sdk/include/ehbasic_hw_audio.inc
---

# Chapter 18 - The AHX Engine

AHX (Abyss High-Quality Music) is the four-channel chip-tune
format from the Amiga scene of the mid-1990s. It uses a small
synthesis core based on the Amiga Paula chip to produce its
distinctive square-and-saw sound without storing any sample
data. Intuition Engine's AHX engine plays AHX files directly:
load the data, set the play pointer, and write to the control
register.

## 18.1 What AHX can show

| Item              | Value                                  |
|-------------------|----------------------------------------|
| Voices            | `4`, each driven by the AHX synthesis engine |
| File format       | Standard AHX (`.ahx`) modules           |
| Subsongs          | Up to `256` per file                    |
| Output            | Routed into the global mixer            |

## 18.2 The register block

The engine has a tiny register footprint at `0xF0B80`–`0xF0B91`.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF0B80`  | `AHX_PLUS_CTRL`   | AHX+ mode enable (bit `0`). |
| `0xF0B84`  | `AHX_PLAY_PTR`    | Address of the AHX data in memory. |
| `0xF0B88`  | `AHX_PLAY_LEN`    | Length of the AHX data in bytes. |
| `0xF0B8C`  | `AHX_PLAY_CTRL`   | Bit `0` = start, bit `1` = stop, bit `2` = loop. |
| `0xF0B90`  | `AHX_PLAY_STATUS` | Bit `0` = busy, bit `1` = error. |
| `0xF0B91`  | `AHX_SUBSONG`     | Subsong number (`0`–`255`). |

### 18.2.1 The AHX+ extension

Bit `0` of `AHX_PLUS_CTRL` enables **AHX+**: an IE-specific
extension that adds additional waveforms and an effects layer on
top of the standard AHX synthesis core. Bit-exact AHX behaviour
resumes when the bit is cleared.

## 18.3 BASIC keywords

| Form                                              | Effect |
|---------------------------------------------------|--------|
| `AHX PLAY `*addr*`[, `*len*` [, `*subsong*`]]`    | Start AHX playback from *addr*. |
| `AHX STOP`                                        | Stop playback. |
| `AHX PLUS ON` / `AHX PLUS OFF`                    | Enable / disable AHX+. |

A BASIC fragment that plays an AHX module already loaded into
memory at `&H100000`:

```basic
10 POKE &H000F0800, 1                : REM AUDIO_CTRL = on
20 AHX PLAY &H100000, &H4000          : REM 16 KB module
```

To play a file from disk in one step, use `SOUND PLAY` with the
`.ahx` extension (Chapter 22).

## 18.4 Putting it together

AHX is a small, fast engine. It costs almost no CPU time to keep
running once started; the engine renders each frame internally
and feeds the mixer. Two practical uses: background music for
games (start once, let it loop), and effect-driven music demos
(change `AHX_SUBSONG` to switch between intro and main themes
without reloading).

The next chapter covers ProTracker MOD playback, the four-channel
sample-based Amiga format that complements AHX.
