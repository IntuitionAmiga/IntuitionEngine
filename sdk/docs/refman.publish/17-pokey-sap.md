
# Chapter 17 - POKEY and SAP Playback

POKEY is the Atari 8-bit sound chip - four square-wave channels
with seven distortion modes each, a 17-bit polynomial noise
generator, and a small but versatile set of clock-routing modes
that let two channels combine into a 16-bit oscillator. It is the
chip behind the Atari 400/800/XL/XE sound and the SAP music
format used to preserve their songs.

## 17.1 What POKEY can show

| Item              | Value                                  |
|-------------------|----------------------------------------|
| Channels          | `4`                                    |
| Distortion modes  | `8` per channel (combinations of polynomial taps and pure tone) |
| Frequency range   | 8-bit divider per channel              |
| Volume control    | Per-channel 4-bit volume               |
| Clock options     | `64` kHz, `15` kHz, or `1.79` MHz (per channel) |
| Pair modes        | Channels `1+2` and `3+4` can combine to 16-bit |
| Filter modes      | High-pass on channels `1` and `2`      |
| Polynomial RNG    | Readable through `POKEY_RANDOM`        |

## 17.2 The register block

The chip sits in an eleven-byte block at `0xF0D00`–`0xF0D0A`.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF0D00`  | `POKEY_AUDF1`     | Channel 1 frequency divider. |
| `0xF0D01`  | `POKEY_AUDC1`     | Channel 1 control (distortion + volume). |
| `0xF0D02`  | `POKEY_AUDF2`     | Channel 2 divider. |
| `0xF0D03`  | `POKEY_AUDC2`     | Channel 2 control. |
| `0xF0D04`  | `POKEY_AUDF3`     | Channel 3 divider. |
| `0xF0D05`  | `POKEY_AUDC3`     | Channel 3 control. |
| `0xF0D06`  | `POKEY_AUDF4`     | Channel 4 divider. |
| `0xF0D07`  | `POKEY_AUDC4`     | Channel 4 control. |
| `0xF0D08`  | `POKEY_AUDCTL`    | Master audio control. |
| `0xF0D09`  | `POKEY_PLUS_CTRL` | POKEY+ mode enable (bit `0`). |
| `0xF0D0A`  | `POKEY_RANDOM`    | Read-only polynomial / RNG tap. |

### 17.2.1 `AUDC` (channel control) bits

| Bits | Field         | Meaning |
|------|---------------|---------|
| 0–3  | `VOLUME`      | Volume, `0`–`15`. |
| 4    | `VOLUME_ONLY` | Force DC output (no oscillator); the channel just produces the volume value. |
| 5–7  | `DISTORTION`  | One of eight distortion modes (table below). |

| `DISTORTION` | Algorithm |
|--------------|-----------|
| `0` | 17-bit poly + 5-bit poly |
| `1` | 5-bit poly only |
| `2` | 17-bit poly + 4-bit poly (most metallic) |
| `3` | 5-bit poly + 4-bit poly |
| `4` | 17-bit poly only (white noise) |
| `5` | Pure square wave |
| `6` | 4-bit poly only (buzzy) |
| `7` | 17-bit poly + pulse (50% duty) |

### 17.2.2 `AUDCTL` (master) bits

| Bit | Field         | Meaning |
|-----|---------------|---------|
| 0   | `CLOCK_15KHZ` | Use `15` kHz base clock instead of `64` kHz. |
| 1   | `HIPASS_CH1`  | High-pass filter ch1 by ch3. |
| 2   | `HIPASS_CH2`  | High-pass filter ch2 by ch4. |
| 3   | `CH4_BY_CH3`  | Ch4 clocked by ch3 (joined as a 16-bit oscillator). |
| 4   | `CH2_BY_CH1`  | Ch2 clocked by ch1 (16-bit). |
| 5   | `CH3_179MHZ`  | Ch3 uses `1.79` MHz clock. |
| 6   | `CH1_179MHZ`  | Ch1 uses `1.79` MHz clock. |
| 7   | `POLY9`       | Use 9-bit poly instead of 17-bit. |

### 17.2.3 Clocks

| Constant         | Value (Hz)  |
|------------------|-------------|
| `POKEY_CLOCK_NTSC` | `1,789,773` |
| `POKEY_CLOCK_PAL`  | `1,773,447` |

The output frequency of a channel is

```
   f = clock / ((1 + divider) × prescaler)
```

with `prescaler` chosen by `AUDCTL`: `28` for `64` kHz mode, `114`
for `15` kHz mode, or `1` for the `1.79` MHz modes.

## 17.3 The POKEY+ extension

Bit `0` of `POKEY_PLUS_CTRL` enables **POKEY+**: extended volume,
DAC mode, and additional distortion combinations. Bit-exact POKEY
behaviour resumes when the bit is cleared.

## 17.4 SAP playback

The SAP (Slight Atari Player) format is a wrapper around an Atari
6502 routine and the POKEY register changes it produces. The
chip's player handles loading and per-frame replay:

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF0D10`  | `SAP_PLAY_PTR`    | Address of the SAP data. |
| `0xF0D14`  | `SAP_PLAY_LEN`    | Length in bytes. |
| `0xF0D18`  | `SAP_PLAY_CTRL`   | Bit `0` = start, bit `1` = stop, bit `2` = loop. |
| `0xF0D1C`  | `SAP_PLAY_STATUS` | Bit `0` = busy, bit `1` = error. |
| `0xF0D20`  | `SAP_SUBSONG`     | Subsong number (`0`–`255`). |

## 17.5 BASIC keywords

| Form                                | Effect |
|-------------------------------------|--------|
| `POKEY `*ch*`, `*freq*`, `*ctrl*    | Set channel divider and control byte. |
| `POKEY CTRL `*value*                | Write the master `AUDCTL` byte. |
| `POKEY PLUS ON` / `POKEY PLUS OFF`  | Enable / disable POKEY+. |
| `SAP `*addr*`[, `*len*` [, `*subsong*`]]` | Start SAP playback. |
| `SAP STOP`                          | Stop SAP playback. |

A BASIC fragment that plays a pure tone through channel 1:

```basic
10 POKE &H000F0800, 1                : REM AUDIO_CTRL = on
20 POKE &H000F0D08, &H00              : REM AUDCTL: 64 kHz, no joins
30 POKEY 0, 121, &HAF                 : REM ch 1, divider 121, distortion=5 (pure), vol=15
```

## 17.6 Putting it together

POKEY's eight distortion modes plus its 16-bit pair modes and
`1.79` MHz clock give it more pitch range and timbre variety than
any of the other heritage chips. For melodic tracks, set the
distortion to `5` (pure tone) on the channels you want clean.
For drums and effects, use `4` (white noise) or `2` (metallic
noise). SAP playback handles existing Atari music directly.

The next chapter covers AHX, the Amiga chip-tune format.
