
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 16 - TED Audio

TED audio is a small two-voice sound generator. Voice 1 is a square wave. Voice
2 is a square wave or an 8-bit TED noise source. The chip has one shared control
byte for volume, voice enables, noise mode, and the stored direct-output bit.

The quickest audible setup is:

```basic
10 REM TED FIRST TONE
20 POKE32 &H000F0800,1
30 REM SET FREQUENCY, THEN ENABLE OUTPUT
40 TED TONE 1,900
50 POKE8 &H000F0F03,&H18
```

`TED TONE` writes the frequency registers. It does not enable a voice. Line 50
sets voice 1 on and master volume 8.

Try changing the tone value in line 40 to `960`. TED frequency values count up
towards higher notes.

## 16.1 Shape of the chip

| Item          | Value |
|---------------|-------|
| Voices        | `2` |
| Voice 1       | Square wave |
| Voice 2       | Square wave or TED noise |
| Frequency     | 10-bit register per voice |
| Volume        | Shared level `0` to `8`; stored values `9` to `15` play as `8` |
| Noise         | Voice 2 replacement, controlled by bit `6` of `TED_SND_CTRL` |
| TED Plus      | Enhanced processing mode at `TED_PLUS_CTRL` bit `0` |

TED audio uses the main TED clock divided by 8. PAL is `110840` Hz and NTSC is
`111860` Hz.

## 16.2 Register block

The audio register block is byte-wide from `$F0F00` to `$F0F05`.

| Address   | Name            | Purpose |
|-----------|-----------------|---------|
| `$F0F00` | `TED_FREQ1_LO`  | Voice 1 frequency low byte |
| `$F0F01` | `TED_FREQ2_LO`  | Voice 2 frequency low byte |
| `$F0F02` | `TED_FREQ2_HI`  | Voice 2 frequency bits `8` to `9` |
| `$F0F03` | `TED_SND_CTRL`  | Volume, enables, noise, direct-output bit |
| `$F0F04` | `TED_FREQ1_HI`  | Voice 1 frequency bits `8` to `9` |
| `$F0F05` | `TED_PLUS_CTRL` | Bit `0` enables TED Plus |

Use `POKE8` for direct register work. These are byte registers; the music player
registers later in this chapter are 32-bit pointer and length registers.

## 16.3 Frequency registers

Each voice has a 10-bit register value:

```
register = low + 256 * (high AND 3)
frequency = sound_clock / (1024 - register)
```

If the register value is `1024` or higher, it is treated as `1023`, giving the
highest pitch. Low register values produce low pitches.

`TED TONE ch,value` writes the two frequency bytes for voice `1` or `2`:

```basic
10 REM TED TWO VOICES
20 POKE32 &H000F0800,1
30 REM LOAD BOTH 10 BIT FREQUENCIES
40 TED TONE 1,900
50 TED TONE 2,940
60 REM ENABLE BOTH VOICES AT VOLUME 8
70 POKE8 &H000F0F03,&H38
```

Expected result: both square voices play together at volume 8. `&H38` is voice
1 on, voice 2 on, volume 8.

The two `TED TONE` lines only fill the frequency registers. Line 70 is the line
that makes the sound audible: bit `4` enables voice 1, bit `5` enables voice 2,
and the low nibble sets the shared volume.

Try changing line 70 to `POKE8 &H000F0F03,&H28`; only voice 2 remains enabled.

## 16.4 Sound control byte

`TED_SND_CTRL` at `$F0F03` is the central audio control byte.

| Bit | Field       | Meaning |
|-----|-------------|---------|
| `0` to `3` | `VOLUME` | Shared volume. Values above `8` are clamped by the sound path |
| `4` | `SND1ON` | Voice 1 output enable |
| `5` | `SND2ON` | Voice 2 output enable |
| `6` | `SND2NOISE` | Voice 2 uses TED noise instead of square wave |
| `7` | `SNDDC` | Stored direct-output bit |

`TED VOL level` changes only bits `0` to `3` and preserves the upper control
bits. `TED NOISE ON` sets bit `6`; `TED NOISE OFF` clears bit `6`. Neither
command enables voice 2 by itself.

```basic
10 REM TED NOISE HIT
20 POKE32 &H000F0800,1
30 REM VOICE 2 CLOCKS THE NOISE
40 TED TONE 2,990
50 FOR V=8 TO 0 STEP -1
60 REM VOICE 2 ON, NOISE ON, VOLUME V
70 POKE8 &H000F0F03,&H60+V
80 FOR Q=1 TO 80
90 NEXT Q
100 NEXT V
110 POKE8 &H000F0F03,0
```

Expected result: voice 2 makes a short noisy hit. `&H60` is voice 2 on plus
noise; the loop fades the shared volume.

Line 40 sets the voice 2 frequency, which also gives the noise generator its
pace. Line 70 builds the control byte from `&H60` plus the current volume:
bit `5` enables voice 2, bit `6` selects noise, and `V` fades from loud to
silent.

Try changing the delay loop in lines 80 to 90 from `80` to `30` for a shorter
percussion tick.

The `SNDDC` bit is stored in the register. The current TED audio path uses the
square/noise voices and shared volume; there is no separate BASIC DAC sample
register for TED audio.

## 16.5 Arpeggios

TED is especially good at quick single-voice arpeggios:

```basic
10 REM TED TINY ARP
20 POKE32 &H000F0800,1
30 REM VOICE 1 ON AT VOLUME 8
40 POKE8 &H000F0F03,&H18
50 FOR I=0 TO 127
60 REM REPEAT FOUR REGISTER VALUES
70 N=I-INT(I/4)*4
80 IF N=0 THEN D=860
90 IF N=1 THEN D=900
100 IF N=2 THEN D=930
110 IF N=3 THEN D=960
120 TED TONE 1,D
130 FOR Q=1 TO 40
140 NEXT Q
150 NEXT I
160 POKE8 &H000F0F03,0
```

Expected result: a bright four-step arpeggio, then silence.

Line 40 opens voice 1 before the loop starts. Lines 70 to 110 choose one of four
frequency register values. Line 120 writes the selected value, and the short
delay lets the ear hear each step before the next write.

Try changing line 110 from `960` to `980` for a sharper top note.

## 16.6 TED Plus

TED Plus follows the shared Plus rule from Chapter 11. `TED PLUS ON` writes
`1` to `TED_PLUS_CTRL` at `$F0F05`; `TED PLUS OFF` writes `0`. TED keeps the
same six audio registers. The TED-specific difference is an enhanced volume
curve, per-voice mix gains, oversampling, low-pass smoothing, drive, and room
processing.

```basic
10 REM TED PLUS COMPARE
20 POKE32 &H000F0800,1
30 TED TONE 1,920
40 POKE8 &H000F0F03,&H18
50 REM LISTEN TO PLAIN TED FIRST
60 FOR T=1 TO 2500
70 NEXT T
80 TED PLUS ON
90 PRINT PEEK8(&H000F0F05)
100 REM NOW LISTEN TO TED PLUS
110 FOR T=1 TO 2500
120 NEXT T
130 TED PLUS OFF
140 PRINT PEEK8(&H000F0F05)
150 POKE8 &H000F0F03,0
```

The tone continues while line 80 switches to the enhanced processing path.
Lines 90 and 140 print `1` and then `0`, so the listing proves the control byte
as well as changing the sound.

Try changing line 30 to `TED TONE 1,980`; the Plus comparison still uses the
same TED registers.

## 16.7 Player registers

The TED player streams TED music data from memory.

| Address   | Name              | Purpose |
|-----------|-------------------|---------|
| `$F0F10` | `TED_PLAY_PTR`    | Start address of the music data |
| `$F0F14` | `TED_PLAY_LEN`    | Length in bytes |
| `$F0F18` | `TED_PLAY_CTRL`   | Write `1` start, `2` stop, `5` start loop |
| `$F0F1C` | `TED_PLAY_STATUS` | Bit `0` busy, bit `1` error |

```basic
10 REM TED MEMORY PLAYBACK
20 REM START A TED MUSIC BLOCK
30 TED PLAY &H00010000,4096
40 S=TED STATUS
50 PRINT S
60 IF S AND 2 THEN PRINT "TED ERROR"
```

If the memory block contains valid TED music data, `TED STATUS` reports busy
while playback is active. If the pointer, length, or data is invalid, bit `1`
is set.

Line 30 writes the pointer, length, and start command. Lines 40 to 60 sample the
status and report only the error bit. They do not stop playback; a valid TED
music block should continue until it ends, loops, or a later stop command is
typed.

To stop TED playback later:

```basic
10 TED STOP
20 PRINT TED STATUS
```

## 16.8 Side effects and limits

Frequency, volume, enable, noise, and TED Plus changes take effect immediately.
Noise uses voice 2's output path, so voice 2 must be enabled for noise to be
heard. The same master volume controls both voices.

The `TED VOL` helper stores four bits because that is what the control register
contains. The sound output clamps values above `8` to the maximum TED volume.

The next chapter covers POKEY, a four-channel chip with richer timer and noise
controls.
