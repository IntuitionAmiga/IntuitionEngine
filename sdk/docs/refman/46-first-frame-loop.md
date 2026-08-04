---
title: "Your First Frame Loop"
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 46 - Your First Frame Loop

A demo is usually a loop with state. The state changes a little, the
machine waits for a frame boundary, the picture is drawn, and the loop
begins again.

The smallest useful loop has five jobs:

1. Choose a display mode.
2. Keep a frame counter.
3. Clear or cover the old frame.
4. Draw the new frame from the current state.
5. Wait for VBlank before continuing.

This chapter uses VideoChip because it gives a direct framebuffer and a
hardware blitter. Later chapters use the same loop shape with Mode 7,
music timing, copper bands, and multiple CPUs.

## 46.1 The Basic Shape

Type this programme:

```basic
10 REM FIRST FRAME LOOP
20 FB=&H00100000:ST=320*4
30 POKE32 &H000F0004,4
40 POKE32 &H000F0080,0
50 POKE32 &H000F0084,FB
60 POKE32 &H000F0000,1
70 X=0:DX=4:F=0
80 POKE32 &H000F2580,1
90 BLIT FILL FB,320,200,&H00000000,ST
100 BLIT FILL FB+80*ST+X*4,24,24,&H0000C878,ST
110 X=X+DX
120 IF X<0 THEN X=0:DX=4
130 IF X>296 THEN X=296:DX=-4
140 F=F+1
150 IF F<180 THEN GOTO 80
160 PRINT "FRAMES ";F
```

Expected result: a small green block moves across the VideoChip layer
and bounces at the edges. After `180` frames the programme prints the
frame count.

Lines `20` to `60` set the screen. Line `80` waits for the next frame
edge by writing to `WAIT_VBLANK`. Line `90` clears the old picture.
Line `100` draws the block at the current `X` position. Lines `110` to
`140` update the state for the next frame.

## 46.2 Why VBlank Comes First

It is tempting to draw and then wait. That works for many small
experiments, but the habit used in this guide is:

```basic
WAIT FOR FRAME
DRAW FRAME
ADVANCE STATE
```

The wait gives the loop a stable beat. The drawing that follows belongs
to one frame. If the drawing later becomes too expensive, the failure is
obvious: the loop misses the next beat.

`VSYNC` is the friendly BASIC spelling. `POKE32 &H000F2580,1` is the
MMIO spelling. They are the same kind of idea: wait for the display
clock rather than running as fast as the CPU can execute BASIC.

## 46.3 Sine Motion

Straight-line motion uses `X=X+DX`. Many demo motions use a phase and a
sine function:

```basic
10 REM SINE MOTION
20 FB=&H00100000:ST=320*4
30 POKE32 &H000F0004,4:POKE32 &H000F0080,0
40 POKE32 &H000F0084,FB:POKE32 &H000F0000,1
50 A=0:F=0
60 POKE32 &H000F2580,1
70 BLIT FILL FB,320,200,&H00000000,ST
80 X=148+INT(120*SIN(A))
90 Y=88+INT(50*SIN(A*1.7))
100 BLIT FILL FB+Y*ST+X*4,24,24,&H00C88040,ST
110 A=A+0.06:IF A>6.28318 THEN A=A-6.28318
120 F=F+1:IF F<240 THEN GOTO 60
```

Expected result: the block follows a curved path. `A` is not an angle
you see on screen. It is a phase. The phase is advanced every frame and
then used to derive visible positions.

## 46.4 Clearing, Covering, And Dirty Rectangles

The examples clear the whole screen because it is easy to understand.
That is not the only method.

| Method | Use it when |
|--------|-------------|
| Full clear | The whole screen changes, or the programme is simple. |
| Cover old object | Only one or two objects move. |
| Dirty rectangles | Many small objects move and the background is stable. |
| Back buffer | Drawing takes several steps and should not be seen half-finished. |

The blitter makes full clears cheap enough for many first effects.
Later, when the picture becomes denser, you will choose a back buffer or
dirty rectangles.

## 46.5 State Is The Demo

A demo loop is mostly state:

| State | Example |
|-------|---------|
| Frame counter | `F=F+1` |
| Position | `X`, `Y` |
| Velocity | `DX`, `DY` |
| Phase | `A`, `Z`, `T` |
| Intensity | `IN`, `PU`, `PB` |
| Section | Intro, build-up, peak, ending |

The rest of Part VII keeps this rule. A rotozoomer changes angle and
scale. A wobble effect changes row offsets. A music-synchronised intro
changes intensity and section from the playback clock.

## 46.6 Limits

- BASIC frame loops teach the machine clearly, but they are not the
  fastest way to run a heavy effect.
- `SIN` and `COS` are convenient in BASIC. Machine-code chapters replace
  them with lookup tables.
- A frame loop should not wait for keys, disk, or long calculations
  inside the visible part of the frame.
- If the blitter is doing the heavy work, poll or wait for its done bit
  before using the result.

Chapter 47 uses this loop shape for the first full-screen demo effect:
a hardware Mode 7 rotozoomer.
