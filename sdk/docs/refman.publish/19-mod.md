
# Chapter 19 - MOD Playback

The MOD player handles ProTracker `.mod` modules - the
four-channel sample-based format that dominated Amiga music from
the late 1980s. Unlike AHX, which synthesises every sample on the
fly, MOD plays back pre-recorded samples stored inside the module
file. The player drives the global mixer; no separate sound chip
is involved.

## 19.1 What MOD can show

| Item              | Value                                  |
|-------------------|----------------------------------------|
| Channels          | `4`                                    |
| Sample format     | Signed 8-bit, embedded in the module file |
| Max samples       | `31` (standard) per module             |
| Filter modes      | None, Amiga 500 (4.5 kHz), Amiga 1200 (28 kHz) |
| File format       | Standard ProTracker `.mod`             |

## 19.2 The register block

The player is a small block at `0xF0BC0`–`0xF0BD7`.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF0BC0`  | `MOD_PLAY_PTR`    | Address of the MOD data in memory. |
| `0xF0BC4`  | `MOD_PLAY_LEN`    | Length of the MOD data, in bytes. |
| `0xF0BC8`  | `MOD_PLAY_CTRL`   | Bit `0` = start, bit `1` = stop, bit `2` = loop. |
| `0xF0BCC`  | `MOD_PLAY_STATUS` | Bit `0` = playing, bit `1` = error. |
| `0xF0BD0`  | `MOD_FILTER_MODEL`| Output filter (`0` = none, `1` = A500, `2` = A1200). |
| `0xF0BD4`  | `MOD_POSITION`    | Current song position (read-only). |

### 19.2.1 The filter models

The Amiga 500's analogue output filter had a corner at about
`4.5` kHz, which gave its music a warmer, more rounded sound; the
Amiga 1200's corner was at about `28` kHz, much closer to flat.
`MOD_FILTER_MODEL` lets you choose which one (or neither) to
apply, so the same `.mod` can be heard as either machine would
have played it.

## 19.3 Programming model

To play a module:

1. Load the file into memory.
2. Write the base address to `MOD_PLAY_PTR` and the size to
   `MOD_PLAY_LEN`.
3. (Optional) write a filter model to `MOD_FILTER_MODEL`.
4. Write `1` to `MOD_PLAY_CTRL`.

To stop: write `2`. To loop: combine `1 | 4` (start + loop).

## 19.4 BASIC keywords

There is no dedicated `MOD` keyword in BASIC. To play a `.mod`
file from disk in one step, use the `SOUND PLAY` keyword
(Chapter 22); the media loader picks the MOD engine from the
`.mod` extension. To drive the player from BASIC directly, use
`POKE` against the registers above.

```basic
10 POKE &H000F0800, 1                : REM AUDIO_CTRL = on
20 BLOAD "song.mod", &H100000
30 POKE &H000F0BC0, &H00100000        : REM MOD_PLAY_PTR
40 POKE &H000F0BC4, 65536             : REM MOD_PLAY_LEN (size in bytes)
50 POKE &H000F0BD0, 1                 : REM filter model = A500
60 POKE &H000F0BC8, 1                 : REM start
```

## 19.5 Putting it together

MOD playback is the simplest way to add high-quality background
music to a program without writing any synthesis code. The
module file carries the samples, the patterns, and the order; the
player handles everything else.

The next chapter covers the WAV / sample player, used for short
sound effects and digitised speech.
