
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 22 - The Paula DMA Engine

Paula is the four-channel sample DMA engine. It reads signed 8-bit
sample bytes from memory and feeds them to four SoundChip DAC channels
at rates chosen by Paula-style period values. The program supplies the
sample address, length, period, and volume, then arms DMA with one
control write.

Use MOD playback in Chapter 19 for tracker modules. Use WAV playback in
Chapter 20 for RIFF/WAVE files. Use Paula when you want exact control of
raw sample buffers, completion status, double-buffering, and M68K level
3 interrupts.

## 22.1 First sound

Type this program. It builds four signed 8-bit sine samples in memory
and plays them as a short chord through all four Paula channels.

```basic
10 REM PAULA FOUR CHANNEL CHORD
20 POKE &H000F0800,1
30 A=&H00120000:N=4096:P=253
40 REM BUILD FOUR SIGNED 8 BIT SAMPLES
50 FOR C=0 TO 3
60 F=220
70 IF C=1 THEN F=277
80 IF C=2 THEN F=330
90 IF C=3 THEN F=440
100 FOR I=0 TO N-1
110 V=INT(SIN(I*TWOPI*F/14019)*100)
120 IF V<0 THEN V=V+256
130 POKE8 A+C*N+I,V
140 NEXT I
150 NEXT C
160 REM PTR, LEN, PERIOD, VOLUME
170 FOR C=0 TO 3
180 B=&H000F2260+C*16
190 POKE B,A+C*N
200 POKE B+4,N/2
210 POKE B+8,P
220 POKE B+12,40
230 NEXT C
240 REM CLEAR STATUS, THEN ARM ALL CHANNELS
250 POKE &H000F22A4,15
260 POKE &H000F22A0,&H800F
270 FOR T=1 TO 3000
280 NEXT T
290 PRINT PEEK(&H000F22A4)
```

You should hear a brief four-note chord. Line 290 prints the completion
status; after the chord has ended, bits `0-3` are set.

Lines 40 to 150 build four separate sample buffers. Lines 160 to 230 write each
channel's pointer, word length, period, and volume. Line 250 clears old
completion bits, and line 260 arms all four DMA channels at once.

## 22.2 What Paula can produce

| Item | Value |
|------|-------|
| Channels | `4` |
| Sample format | Signed 8-bit PCM |
| Length unit | Words, where one word is two sample bytes |
| Period clock | `3546895 / period` samples per second |
| Volume | `0-64` per channel |
| Channel output | Paula channel `n` feeds SoundChip DAC channel `n` |
| Completion status | One bit per channel |
| Interrupt | Optional M68K level 3 interrupt on completion |

Sample bytes are signed. `$00` is silence, `$7F` is a large positive
sample, and `$80` is a large negative sample. In BASIC, add `256` before
`POKE8` when a calculated sample is negative.

## 22.3 Register block

The Paula register block is `$F2260-$F22AF`. Channels are 16 bytes
apart.

| Channel | Base | End |
|---------|------|-----|
| `0` | `$F2260` | `$F226F` |
| `1` | `$F2270` | `$F227F` |
| `2` | `$F2280` | `$F228F` |
| `3` | `$F2290` | `$F229F` |

Each channel has the same layout:

| Offset | Name | Access | Purpose |
|--------|------|--------|---------|
| `$00` | `PTR` | write/read | Sample start address. Odd addresses are masked down to even. |
| `$04` | `LEN` | write/read | Length in words, so bytes divided by two. |
| `$08` | `PER` | write/read | Paula period. Output rate is `3546895 / PER`. |
| `$0C` | `VOL` | write/read | Volume `0-64`; larger writes clamp to `64`. |

The global registers are:

| Address | Name | Access | Purpose |
|---------|------|--------|---------|
| `$F22A0` | `AROS_AUD_DMACON` | write/read | Set or clear active DMA channel bits. |
| `$F22A4` | `AROS_AUD_STATUS` | write/read | Completion and error-style channel flags. |
| `$F22A8` | `AROS_AUD_INTENA` | write/read | Set or clear completion interrupt enables. |

The `AROS_AUD_*` names are the canonical register names for this Paula
block.

## 22.4 DMACON and INTENA

`AROS_AUD_DMACON` and `AROS_AUD_INTENA` use the same set-or-clear
format.

| Bit | Meaning |
|-----|---------|
| `15` | `1` means set the selected bits; `0` means clear them. |
| `0` | Channel 0 mask. |
| `1` | Channel 1 mask. |
| `2` | Channel 2 mask. |
| `3` | Channel 3 mask. |

Examples:

```basic
10 POKE &H000F22A0,&H8001:REM ENABLE CHANNEL 0
20 POKE &H000F22A0,&H800F:REM ENABLE CHANNELS 0-3
30 POKE &H000F22A0,1:REM DISABLE CHANNEL 0
40 POKE &H000F22A8,&H8001:REM ENABLE CH0 COMPLETION IRQ
50 POKE &H000F22A8,1:REM DISABLE CH0 COMPLETION IRQ
```

When a channel reaches the end of its buffer, the matching bit in
`AROS_AUD_STATUS` is set. Clear status bits by writing `1` bits back:

```basic
10 REM PAULA STATUS CLEAR
20 PRINT PEEK(&H000F22A4)
30 POKE &H000F22A4,15
40 PRINT PEEK(&H000F22A4)
```

The status register is cleared by writing `1` bits back to it. Line 30 clears
all four channel bits without changing `DMACON`.

If `AROS_AUD_INTENA` has the matching channel bit set, a completed
buffer also asserts M68K interrupt level `3`.

## 22.5 Setup order

To start one channel from a clean state:

1. Enable audio with `POKE &H000F0800,1`.
2. Write signed sample bytes to memory.
3. Write `PTR`, `LEN`, `PER`, and `VOL`.
4. Clear any old status bit by writing to `AROS_AUD_STATUS`.
5. Write `$8000` plus the channel mask to `AROS_AUD_DMACON`.

This is channel 0 only:

```basic
10 REM PAULA CHANNEL 0 SETUP
20 A=&H00124000:N=1024
30 REM BUILD A SIGNED SAMPLE BUFFER
40 FOR I=0 TO N-1
50 V=INT(SIN(I*TWOPI*330/14019)*100)
60 IF V<0 THEN V=V+256
70 POKE8 A+I,V
80 NEXT I
90 REM PTR, LEN, PERIOD, VOLUME
100 POKE &H000F2260,A
110 POKE &H000F2264,N/2
120 POKE &H000F2268,253
130 POKE &H000F226C,64
140 REM CLEAR OLD STATUS AND ARM CH0
150 POKE &H000F22A4,1
160 POKE &H000F22A0,&H8001
```

If `LEN` is zero, or `PER` is still zero when the channel is armed, the
channel is not accepted and its status bit is set. A direct write of
zero to `PER` is ignored, so an existing non-zero period is not erased
by accident.

## 22.6 Period and pitch

The sample rate is:

```text
rate = 3546895 / PER
```

Useful periods:

| Period | Approximate rate | Use |
|--------|------------------|-----|
| `124` | `28604` Hz | Bright samples. |
| `161` | `22030` Hz | Common sampled effects. |
| `253` | `14019` Hz | Classic tracker note rate. |
| `508` | `6982` Hz | Low-rate speech or drums. |
| `1015` | `3494` Hz | Very low-rate speech. |

The fetch engine runs from the Paula clock and writes to the mixer at
the machine audio rate. Fractional phase is preserved across a staged
next buffer, which keeps double-buffered streams smooth.

## 22.7 Latching and double-buffering

When `AROS_AUD_DMACON` arms a channel, Paula latches the current
`PTR`, `LEN`, `PER`, and `VOL` as the active buffer. Writes to those
registers while the channel is active do not disturb the current buffer.
They stage the next buffer instead.

When the active buffer ends:

- The channel status bit is set.
- If a staged next buffer has non-zero length and period, Paula switches
  to it without clearing the channel's `DMACON` bit.
- If there is no valid next buffer, the channel stops and its `DMACON`
  bit clears.

This fragment stages a second channel-0 buffer while the first is
playing:

```basic
10 REM FIRST BUFFER IS ALREADY ARMED ON CH0
20 REM STAGE NEXT PTR, LEN, PERIOD, VOLUME
30 POKE &H000F2260,&H00126000
40 POKE &H000F2264,512
50 POKE &H000F2268,253
60 POKE &H000F226C,48
```

The new values take effect only after the current buffer is exhausted.
That is the normal way to stream audio without a gap.

## 22.8 Stopping and error cases

To stop channel 0:

```basic
10 POKE &H000F22A0,1
```

To stop all four channels:

```basic
10 POKE &H000F22A0,15
```

If the sample pointer is beyond readable memory, Paula mutes the
channel, stops it, clears its `DMACON` bit, and sets the status bit. If
the matching interrupt-enable bit is set, this also raises the level 3
interrupt.

Pointers are even-aligned by masking off bit `0`. Length is not rounded:
`LEN=5` means ten sample bytes. Keep buffers even-sized unless you are
deliberately using an odd word count.

## 22.9 Limits

Paula plays raw signed 8-bit samples only. It does not parse WAV
headers, MOD patterns, or sample-loop metadata. Use the WAV and MOD
chapters for those formats.

Volume changes written during playback are staged with the next buffer,
because the active buffer uses its latched volume. For an immediate
volume effect, stop and re-arm the channel or use a SoundChip DAC path
from Chapter 12.
