---
title: "Copper, Raster Bands, And Layered Presentation"
sources:
  - sdk/examples/basic/resonance.bas
  - sdk/examples/asm/rotating_cube_copper_68k.asm
  - sdk/include/ehbasic_hw_system.inc
  - video_chip.go
  - ehbasic_test.go
  - psg_hardening_test.go
  - sdk/docs/refman/04-videochip.md
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 53 - Copper, Raster Bands, And Layered Presentation

Presentation polish is the difference between an effect and an intro.
The moving texture is the centre, but bands, overlays, scroll strips,
and meters make the frame feel composed.

The VideoChip copper and the blitter are the two main tools. The copper
changes registers at scanline positions. The blitter draws foreground
and background elements into buffers.

## 53.1 A BASIC Copper Band

The `resonance.bas` demo builds simple raster bands:

```basic
310 COPPER LIST CP
320 COPPER WAIT 0
330 COPPER MOVE &H000F0050,&H00000000
340 COPPER WAIT 84
350 COPPER MOVE &H000F0050,&H00000822
360 COPPER WAIT 184
370 COPPER MOVE &H000F0050,&H0018202C
380 COPPER WAIT 320
390 COPPER MOVE &H000F0050,&H00281608
400 COPPER WAIT 440
410 COPPER MOVE &H000F0050,&H00000000
420 COPPER END
430 COPPER ON
```

The list waits for scanlines and writes a colour register. The CPU does
not draw those bands every frame. The copper follows the beam.

## 53.2 Why Bands Work

A raster band is strong because it is tied to screen position, not object
position. It gives the frame a stage:

| Band use | Effect |
|----------|--------|
| Dark top and bottom | Focuses attention. |
| Mid-screen colour shift | Adds depth behind an effect. |
| Bright thin rail | Gives rhythm to music accents. |
| Copper gradient | Makes a flat background feel alive. |

The band is not a sprite. It is a presentation layer.

## 53.3 Foreground And Background

`resonance.bas` draws the Mode 7 texture first, then adds rails, stars,
glints, VU meters, and scroll strips:

```basic
620 GOSUB 1600
630 GOSUB 1900
640 GOSUB 1000
650 GOSUB 1300
660 GOSUB 2200
679 GOSUB 3500
```

Read this as layers:

1. Build texture.
2. Rotozoom it into the back buffer.
3. Draw screen-space rails and glints.
4. Draw stars.
5. Draw pulse bars.
6. Draw meters and scrolltext.

That is the same one-machine rule again. Mode 7, blitter fills, copper,
and audio timing are not separate demos. They are parts of one frame.

## 53.4 M68K Copper Example

The M68K rotating cube copper demo uses the copper for a raster
background while the CPU draws the foreground cube and scroller. Its
important rule is the same as BASIC:

```asm
; enable VideoChip for copper
; build copper list in RAM
; wait for VBlank
; update colours or list data
; draw foreground
```

The copper belongs to the VideoChip, so VideoChip must be enabled even
when another display source is doing much of the visible work.

## 53.5 VU Meters And Scroll Strips

Foreground polish is usually ordinary blitter work:

```basic
BLIT FILL BB+Y*ST+64*4,8,V0,&H00A87058,ST
BLIT FILL BB+Y*ST+80*4,8,V1,&H00708088,ST
BLIT COPY SB+SO,BB+420*ST,640,32,SS,ST
```

The exact addresses depend on the demo, but the idea is stable:

- A VU meter is a filled rectangle whose height follows a variable.
- A scroll strip is a prepared image copied through a moving source
  offset.
- A foreground bar is a `BLIT FILL` timed by music or frame phase.

## 53.6 Limits

- `COPPER WAIT` in BASIC waits on scanline with X position `0`.
- A copper list must end with `COPPER END`.
- Do not use copper to do large memory copies. Use the blitter.
- If the copper changes a register also written by the CPU, decide which
  one owns that register during the frame.

Chapter 54 puts the pieces into a complete intro structure.
