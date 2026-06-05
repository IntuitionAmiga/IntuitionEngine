---
title: "The SID Family"
sources:
  - sid_constants.go
  - sid_engine.go
  - audio_chip.go
  - sdk/include/ehbasic_hw_audio.inc
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 15 - The SID Family

The SID family gives Intuition Engine three MOS 6581/8580 style sound chips:
SID, SID2, and SID3. Each chip has three oscillators, ADSR envelopes, pulse
width control, ring modulation, oscillator sync, a resonant filter, and a
master volume register. BASIC drives the primary SID directly. The extra chips
are available through the same byte registers at their own addresses.

Start with one pulse voice:

```basic
10 REM SID FIRST PULSE
20 POKE32 &H000F0800,1
30 SID VOLUME 15
40 REM START A GATED PULSE
50 SID VOICE 1,8582,2048,&H41,&H88,&HF4
60 FOR T=1 TO 3000
70 NEXT T
80 REM CLEAR GATE TO RELEASE
90 SID VOICE 1,8582,2048,&H40,&H88,&HF4
```

Line 50 sets voice 1 frequency, 50 per cent pulse width, pulse waveform plus
gate, attack/decay byte `&H88`, and sustain/release byte `&HF4`. Line 90 clears
the gate bit so the release phase begins.

Try changing the pulse width from `2048` to `1024`. The pitch stays the same,
but the tone becomes thinner.

## 15.1 Shape of one SID

| Item            | Value |
|-----------------|-------|
| Voices          | `3` per chip |
| Waveforms       | Triangle, sawtooth, pulse, noise, and combined waveform masks |
| Envelope        | Four-bit attack, decay, sustain, and release |
| Modulation      | Sync and ring modulation from the previous voice |
| Pulse width     | 12-bit value, used by the pulse waveform |
| Filter          | Low-pass, band-pass, high-pass, resonance, and voice routing |
| Master volume   | Four-bit level in `MODE_VOL` |
| Readback        | Oscillator 3 and envelope 3 |

SID+ is an enhanced processing mode. It keeps the same SID registers but uses a
different volume curve with oversampling, light drive, and room processing for
the selected SID chip.

## 15.2 Register blocks

Each SID register block is `$1D` bytes wide.

| Chip | Base       | End        | Voices |
|------|------------|------------|--------|
| SID  | `$F0E00` | `$F0E1C` | `1` to `3` |
| SID2 | `$F0E30` | `$F0E4C` | `1` to `3` |
| SID3 | `$F0E50` | `$F0E6C` | `1` to `3` |

The offsets below are relative to the chip base.

| Offset | Name       | Purpose |
|--------|------------|---------|
| `$00` | `V1_FREQ_LO` | Voice 1 frequency low byte |
| `$01` | `V1_FREQ_HI` | Voice 1 frequency high byte |
| `$02` | `V1_PW_LO`   | Voice 1 pulse width low byte |
| `$03` | `V1_PW_HI`   | Voice 1 pulse width high nibble |
| `$04` | `V1_CTRL`    | Voice 1 control |
| `$05` | `V1_AD`      | Voice 1 attack and decay |
| `$06` | `V1_SR`      | Voice 1 sustain and release |
| `$07` to `$0D` | Voice 2 | Same seven registers |
| `$0E` to `$14` | Voice 3 | Same seven registers |
| `$15` | `FC_LO`      | Filter cutoff bits `0` to `2` |
| `$16` | `FC_HI`      | Filter cutoff bits `3` to `10` |
| `$17` | `RES_FILT`   | Resonance and filter routing |
| `$18` | `MODE_VOL`   | Filter mode, voice 3 off, and master volume |
| `$19` | `SID_PLUS_CTRL` | Write bit `0` for SID+ on or off |
| `$1B` | `OSC3`       | Read oscillator 3 output |
| `$1C` | `ENV3`       | Read envelope 3 output |

The original potentiometer registers are not connected to input hardware. The
`$19` write path is used for `SID_PLUS_CTRL`.

## 15.3 Voice data formats

The frequency register is a 16-bit phase increment:

```
frequency = register * clock / 16777216
register = frequency * 16777216 / clock
```

The primary SID defaults to the PAL-style clock `985248` Hz. The NTSC-style
clock value is `1022727` Hz.

Pulse width is a 12-bit value. `0` is fully low, `2048` is close to a square
wave, and `4095` is almost fully high. Only the low nibble of `PW_HI` is used.

`AD` and `SR` pack two nibbles each:

| Byte | High nibble | Low nibble |
|------|-------------|------------|
| `AD` | Attack      | Decay      |
| `SR` | Sustain     | Release    |

The control byte is:

| Bit | Name       | Effect |
|-----|------------|--------|
| `0` | `GATE`     | `1` attack/decay/sustain, `0` release |
| `1` | `SYNC`     | Sync oscillator to the previous voice |
| `2` | `RINGMOD`  | Ring modulation from the previous voice; triangle must be selected |
| `3` | `TEST`     | Reset oscillator and mute gated output |
| `4` | `TRIANGLE` | Triangle waveform |
| `5` | `SAWTOOTH` | Sawtooth waveform |
| `6` | `PULSE`    | Pulse waveform using `PW` |
| `7` | `NOISE`    | Noise waveform |

Multiple waveform bits may be set. The sound path keeps the combined waveform
mask for SID-style mixed waveforms.

## 15.4 BASIC voice examples

`SID VOICE v,freq,pw,ctrl,ad,sr` writes the seven voice registers for primary
SID voice `v`, where `v` is `1`, `2`, or `3`.

```basic
10 REM SID THREE VOICES
20 POKE32 &H000F0800,1
30 SID VOLUME 15
40 REM START PULSE, SAWTOOTH, TRIANGLE
50 SID VOICE 1,4291,2048,&H41,&H48,&HF5
60 SID VOICE 2,5407,1024,&H21,&H46,&HC5
70 SID VOICE 3,6430,0,&H11,&H26,&HA8
80 FOR T=1 TO 4000
90 NEXT T
100 REM RELEASE ALL THREE GATES
110 SID VOICE 1,4291,2048,&H40,&H48,&HF5
120 SID VOICE 2,5407,1024,&H20,&H46,&HC5
130 SID VOICE 3,6430,0,&H10,&H26,&HA8
```

Expected result: a three-voice chord using pulse, sawtooth, and triangle, then
all three gates release.

The three control bytes differ only in waveform and gate bits: `&H41` is pulse
plus gate, `&H21` is sawtooth plus gate, and `&H11` is triangle plus gate. The
release lines keep the waveform bits but clear bit `0`, so the envelopes enter
their release phase instead of stopping abruptly.

Try changing line 70 to use `&H81` for a noise voice in the third slot.

For a sync and ring-modulated lead, set up voice 1 as the source and voice 2
as the modulated voice:

```basic
10 REM SID SYNC RING LEAD
20 POKE32 &H000F0800,1
30 SID VOLUME 15
40 REM VOICE 1 IS THE MODULATION SOURCE
50 SID VOICE 1,3200,0,&H21,&H44,&HF6
60 SID VOICE 2,6400,0,&H17,&H44,&HF6
70 REM SWEEP THE MODULATED VOICE
80 FOR F=5200 TO 9000 STEP 160
90 SID VOICE 2,F,0,&H17,&H44,&HF6
100 FOR Q=1 TO 30
110 NEXT Q
120 NEXT F
130 REM RELEASE SOURCE AND LEAD
140 SID VOICE 1,3200,0,&H20,&H44,&HF6
150 SID VOICE 2,6400,0,&H16,&H44,&HF6
```

Voice 2 uses triangle, ring modulation, sync, and gate. The source is the
previous voice.

Line 50 starts voice 1 as a sawtooth source. Line 60 starts voice 2 with
triangle, ring modulation, sync, and gate all set. During the sweep only voice
2's frequency changes; the source keeps running so the modulation has something
to lock against.

Try reducing the `STEP` in line 80 to `80`. The sweep becomes smoother and lasts
longer.

## 15.5 Filter and master volume

`SID VOLUME level` writes the low four bits of `MODE_VOL` and preserves the
filter mode bits.

`SID FILTER cutoff,resonance,routing,mode` writes:

| Argument | Register effect |
|----------|-----------------|
| `cutoff` | `FC_LO = cutoff AND 7`, `FC_HI = INT(cutoff/8)` |
| `resonance` | High nibble of `RES_FILT` |
| `routing` | Low nibble of `RES_FILT`; bit `0` voice 1, bit `1` voice 2, bit `2` voice 3 |
| `mode` | Low nibble shifted into `MODE_VOL` bits `4` to `7` |

Filter mode bits are `1` low-pass, `2` band-pass, `4` high-pass, and `8` voice
3 off. Modes may be combined.

```basic
10 REM SID FILTER SWEEP
20 POKE32 &H000F0800,1
30 SID VOLUME 15
40 REM ROUTE VOICE 1 THROUGH LOW PASS
50 SID VOICE 1,4291,2048,&H41,&H44,&HF6
60 FOR C=80 TO 1800 STEP 40
70 SID FILTER C,12,1,1
80 FOR Q=1 TO 30
90 NEXT Q
100 NEXT C
110 REM RELEASE THE FILTERED VOICE
120 SID VOICE 1,4291,2048,&H40,&H44,&HF6
```

Expected result: a pulsed voice is routed through a resonant low-pass filter
whose cutoff rises during the loop.

The filter command in line 70 writes cutoff `C`, resonance `12`, routing bit
`1` for voice 1, and mode `1` for low-pass. The volume set earlier is preserved
while the mode bits change, so the sweep does not reset the master level.

Try changing the mode argument in line 70 from `1` to `4` for a high-pass sweep.

## 15.6 SID2 and SID3 by POKE8

BASIC keywords target the primary SID. Use `POKE8` for the second and third
chips. This example plays a sawtooth voice on SID2:

```basic
10 REM SID2 SAW VOICE
20 POKE32 &H000F0800,1
30 REM POINT B AT THE SID2 REGISTER BLOCK
40 B=&H000F0E30
50 F=4291
60 POKE8 B+24,15
70 REM VOICE 1 FREQUENCY AND PULSE WIDTH
80 POKE8 B+0,F AND 255
90 POKE8 B+1,INT(F/256) AND 255
100 POKE8 B+2,0
110 POKE8 B+3,0
120 REM ENVELOPE THEN CONTROL
130 POKE8 B+5,&H44
140 POKE8 B+6,&HF6
150 POKE8 B+4,&H21
160 FOR T=1 TO 3000
170 NEXT T
180 POKE8 B+4,&H20
```

This is the same seven-register voice layout used by primary SID, but addressed
through `B`. Lines 80 and 90 split the frequency word. Lines 130 and 140 set
the envelope bytes. Line 150 starts a gated sawtooth voice, and line 180 clears
the gate.

Try changing line 40 to `B=&H000F0E50` to move the same voice to SID3.

## 15.7 SID Plus

SID Plus follows the shared Plus rule from Chapter 11. `SID PLUS ON` writes
`1` to `SID_PLUS_CTRL` at `$F0E19` for the primary SID; `SID PLUS OFF` writes
`0`. The normal SID registers stay active. The SID-specific difference is a
different volume curve and per-voice mix gains for the selected SID chip.

```basic
10 REM SID PLUS COMPARE
20 POKE32 &H000F0800,1
30 SID VOLUME 15
40 SID VOICE 1,8582,2048,&H41,&H88,&HF4
50 REM LISTEN TO THE PLAIN SID FIRST
60 FOR T=1 TO 3000
70 NEXT T
80 SID PLUS ON
90 PRINT PEEK8(&H000F0E19)
100 REM NOW LISTEN TO SID PLUS
110 FOR T=1 TO 3000
120 NEXT T
130 SID PLUS OFF
140 PRINT PEEK8(&H000F0E19)
150 SID VOICE 1,8582,2048,&H40,&H88,&HF4
```

Lines 80 and 130 change only the processing path. The voice registers keep
their SID meanings. The two `PEEK8` lines print `1` and then `0`, confirming
the control byte.

Try changing the control byte in line 40 from `&H41` to `&H21`; the comparison
uses sawtooth instead of pulse.

## 15.8 Player registers

The primary SID also has a memory playback controller.

| Address   | Name              | Purpose |
|-----------|-------------------|---------|
| `$F0E20` | `SID_PLAY_PTR`    | Start address of the music data |
| `$F0E24` | `SID_PLAY_LEN`    | Length in bytes |
| `$F0E28` | `SID_PLAY_CTRL`   | Write `1` start, `2` stop, `5` start loop |
| `$F0E2C` | `SID_PLAY_STATUS` | Bit `0` busy, bit `1` error |
| `$F0E2D` | `SID_SUBSONG`     | Subsong number |

```basic
10 REM SID MEMORY PLAYBACK
20 REM START SUBSONG 0 FROM MEMORY
30 SID PLAY &H0000C000,4096,0
40 S=SID STATUS
50 PRINT S
60 IF S AND 2 THEN PRINT "SID ERROR"
```

If the memory block is valid SID music data, `SID STATUS` reports busy while the
player is active. If the pointer, length, or data is invalid, bit `1` is set.

Line 30 writes the playback pointer, length, subsong, and start command. Lines
40 to 60 sample the status and report only errors. They do not stop playback; a
successful SID tune should keep playing until it ends, loops, or a later stop
command is typed.

To stop SID playback later:

```basic
10 SID STOP
20 PRINT SID STATUS
```

## 15.9 Side effects and limits

Voice register writes take effect immediately. Raising a `GATE` bit starts that
voice's attack phase; clearing it starts release. The `TEST` bit resets the
oscillator phase and mutes gated output until cleared. `SYNC` and `RINGMOD` use
the previous voice, wrapping voice 1 to voice 3.

`OSC3` and `ENV3` read the current oscillator and envelope outputs for voice 3.
`MODE_VOL` bit `7` mutes voice 3 unless voice 3 is routed through the filter.
The filter external-input routing bit is stored, but there is no separate
external audio input for BASIC programs.

The next chapter covers TED audio, the two-voice sound chip from the same home
computer family as TED video.
