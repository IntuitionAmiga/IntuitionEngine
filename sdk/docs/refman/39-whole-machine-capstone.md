---
title: "Whole-Machine Capstone"
sources:
  - video_chip.go
  - audio_chip.go
  - file_io.go
  - sdk/include/ehbasic_hw_coproc.inc
  - sdk/include/ehbasic_file_io.inc
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 39 - Whole-Machine Capstone

The previous chapters describe cards one at a time. This last program
puts four of them in the same BASIC listing: VideoChip draws the
picture, SoundChip plays the chord, the file block writes a short note
to the disk volume, and the coprocessor block reports that no 6502
service is running yet. It is not a large program. Its purpose is to
show the book's central rule in one place: every part is reached through
the same machine.

## 39.1 Type the program

```basic
10 REM WHOLE MACHINE CAPSTONE
20 FB=&H00100000:ST=320*4
30 POKE &H000F0004,&H04:POKE &H000F0080,0:POKE &H000F0084,FB:POKE &H000F0000,1
40 BLIT FILL FB,320,200,&H00001830,ST
50 BLIT FILL FB+40*ST+40*4,240,80,&H00105090,ST
60 BLIT LINE 0,0,319,199,&H00FFFFFF:BLIT LINE 0,199,319,0,&H00FFCC40
70 POKE &H000F0800,1:SOUND 0,262,180,1,128:SOUND 1,330,140,2
80 ENVELOPE 0,4,8,180,16:ENVELOPE 1,4,8,180,16
90 GATE 0,ON:GATE 1,ON:SOUND FILTER 190,80,1:SOUND REVERB 70,100
100 N=&H00730000:D=&H00730100
110 POKE8 N,67:POKE8 N+1,65:POKE8 N+2,80:POKE8 N+3,46
120 POKE8 N+4,84:POKE8 N+5,88:POKE8 N+6,84:POKE8 N+7,0
130 POKE8 D,73:POKE8 D+1,69
140 POKE &H000F2200,N:POKE &H000F2204,D:POKE &H000F2208,2:POKE &H000F220C,2
150 PRINT "FILE ";PEEK(&H000F2210)
160 R=&H00030200:S=&H00030300:POKE R,123
170 Q=COCALL(3,1,R,4,S,4):PRINT "COP ";PEEK(&H000F234C)
180 FOR T=1 TO 30:NEXT T
190 GATE 0,OFF:GATE 1,OFF
RUN
FILE 0
COP 6
```

You should see a VideoChip screen with a coloured panel and crossed
lines. You should hear a short two-voice chord with filter and reverb.
The file status line should print `FILE 0`, meaning the write succeeded.
On a clean machine with no 6502 worker running, the coprocessor error
line should print `COP 6`, meaning `COPROC_ERR_NO_WORKER`.

## 39.2 What each part proves

Lines `20` to `60` use the VideoChip framebuffer and blitter. The
program chooses the `320` by `200` mode, points `FB_BASE` at VRAM, fills
two rectangles, and draws two diagonal lines.

Lines `70` to `90` use the IE-native SoundChip. They enable the
master mixer, set two voices, add envelopes, open the gates, and apply
the shared filter and reverb stages.

Lines `100` to `150` use the file block directly. The program writes
the `NUL`-terminated name string `"CAP.TXT"` and two data bytes into
RAM, points the file registers at those buffers, writes the byte count,
and fires `FILE_OP_WRITE`.

Lines `160` and `170` use the coprocessor block. The `COCALL` asks CPU
type `3`, the 6502, to handle operation `1`. Since this capstone does
not start a service program, the expected result is the documented
`COPROC_ERR_NO_WORKER` path. Chapter 32 shows the positive service
pattern when a worker file is present.

The important detail is not the size of the program. It is that the
same BASIC listing touches display memory, audio registers, disk I/O,
and cross-CPU control registers without changing computers.
