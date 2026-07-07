---
title: "Wobble, Texture Building, And Logo Motion"
sources:
  - sdk/examples/basic/wobble_zoom.bas
  - ehbasic_splash_wobble_test.go
  - ehbasic_aot_test.go
  - video_chip.go
  - sdk/include/ehbasic_hw_video.inc
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 51 - Wobble, Texture Building, And Logo Motion

A plain rotozoomer uses a static texture. The next step is to change the
texture before Mode 7 samples it.

The supplied `wobble_zoom.bas` demo builds a `1024` by `512` work
texture each frame. It places a logo into that texture one row at a
time, with a sine offset per row. Then it feeds the work texture into
the same Mode 7 blitter path used by Chapter 46.

## 51.1 The Memory Shape

The demo uses four buffers:

| Buffer | Purpose |
|--------|---------|
| `FB` | Front framebuffer. |
| `BB` | Back framebuffer. |
| `TX` | Large Mode 7 work texture. |
| `SR` | Source logo image. |

The important change is `TX`. It is not just loaded once. It is rebuilt
every frame.

## 51.2 Start Music First

The demo starts music before the heavy visual setup:

```basic
100 REM START MIDI FIRST
110 SOUND PLAY "SONG.MID"
120 PRINT "MEDIA_TYPE=";PEEK32(&H000F2310)
```

That gives the audio engine time to start while the programme prepares
video memory. The `MEDIA_TYPE` read proves which media player the loader
selected.

## 51.3 Build The Wobbled Texture

This is the core idea:

```basic
410 BLIT FILL TX,TW,TH,&H00000000,TS
420 FOR Y=0 TO 91
430 DY=OY+Y
440 X=OX+INT(24*SIN(T+Y*0.12))
450 DX=X:SX=0:CW=640
460 IF DX<0 THEN SX=0-DX:CW=640-SX:DX=0
470 IF DX+CW>TW THEN CW=TW-DX
480 IF CW<=0 THEN GOTO 520
490 SA=SR+Y*ST+SX*4
500 DA=TX+DY*TS+DX*4
510 BLIT COPY SA,DA,CW,1,ST,TS
520 NEXT Y
```

Each source row is copied into a different horizontal position. The
offset is `SIN(T+Y*0.12)`, so neighbouring rows move by slightly
different amounts. The result is a wobbled logo inside the Mode 7
texture.

Lines `460` to `480` clip the copy so a row that moves partly off the
texture does not write outside the destination.

## 51.4 Feed The Texture To Mode 7

After building the work texture, the demo uses the ordinary rotozoomer
path:

```basic
610 SC=1.7+SIN(Z)*0.9
620 CA=COS(A)/SC:SA=SIN(A)/SC
630 DC=INT(CA*FP):DS=INT(SA*FP)
640 U0=INT((CU-HW*CA+HH*SA)*FP)
650 V0=INT((CV-HW*SA-HH*CA)*FP)
660 BLIT MODE7 TX,BB,640,480,U0,V0,DC,DS,0-DS,DC,1023,511,TS,ST
710 BLIT MEMCOPY BB,FB,1228800
720 VSYNC
```

The masks are `1023` and `511` because the work texture is `1024` by
`512`.

## 51.5 Texture-Before-Transform

The order matters:

1. Clear the work texture.
2. Build or copy source material into it.
3. Run Mode 7 from the work texture into the back buffer.
4. Present the back buffer.
5. Advance phases.

This is a powerful demo pattern. You can replace the logo rows with
text, sprites, bars, or generated noise. Mode 7 does not care. It only
samples the texture it is given.

## 51.6 Limits

- Rebuilding a large texture every frame is heavier than using a static
  texture.
- Row-copy effects should clip before writing.
- A power-of-two texture lets the Mode 7 masks wrap naturally.
- If the texture build becomes too expensive, move that part to native
  code or reduce the number of changed rows.

Chapter 52 uses the same ideas, but the timing comes from music.
