---
title: "Music-Synchronised Effects"
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 53 - Music-Synchronised Effects

A simple demo loop advances by frames. A music-synchronised demo advances
by musical time.

The supplied `resonance.bas` demo reads the MIDI playback position when
it can, falls back to the machine timer when it cannot, and derives
visual intensity from that time. The picture is still a VideoChip frame
loop, but the section changes are driven by the soundtrack.

## 53.1 Music Time

The key variables are:

```basic
500 F=0:A=0:Z=0:PM=0:RT0=PEEK32(&H000F075C)/1000000
520 RT=PEEK32(&H000F075C)/1000000:MP=PEEK32(&H000F0BB0)
530 IF MP>PM THEN TM=MP/44100:PM=MP:GOTO 560
540 TM=RT-RT0
```

`MP` is the MIDI playback sample position. When it advances, the demo
uses it as the clock. If the player has not advanced yet, `TM` comes
from the machine timer at `$F075C`.

That fallback is important. A demo should keep moving while the audio
engine starts or while a file is still settling.

## 53.2 Sections And Intensity

The demo turns time into broad visual sections:

```basic
560 IN=0.12:TI=INT(TM*100)
570 IF TI>=4500 THEN IN=0.28
580 IF TI>=9000 THEN IN=0.55
590 IF TI>=15000 THEN IN=0.9
600 IF TI>=20000 THEN IN=0.22
610 IF TI>=21800 THEN GOTO 3700
```

`IN` is a density control. Low values keep the picture restrained.
Higher values add brighter colours, more stars, stronger bars, and more
motion.

`TI` is time in hundredths of a second. It makes section tests compact.

## 53.3 Pulses And Phrases

Short note pulses become local variables:

```basic
1500 REM MIDI CHORD PULSE ENVELOPE FOR BARS
1505 PU=0:DR=0
1510 IF TI<62 THEN PU=30:DR=1
1512 IF TI>=88 THEN IF TI<142 THEN PU=16:DR=1
```

`PU` is a pulse amount. `DR` is a direction. Later drawing code uses
them to push bars, change colours, and add short flashes.

Longer phrases use separate variables such as `PB` and `PD`. That
separates a one-beat accent from a multi-bar build-up.

## 53.4 Timeline Rather Than Loop

The frame loop still exists:

```basic
910 BLIT MEMCOPY BB,FB,1228800
940 VSYNC
950 A=A+0.012+IN*0.026:IF A>6.28318 THEN A=A-6.28318
960 Z=Z+0.018+IN*0.04:IF Z>6.28318 THEN Z=Z-6.28318
970 GOTO 510
```

But the loop is now controlled by a timeline. The same code can draw
calm sections and dense sections because `IN`, `PU`, `PB`, and `PD`
change the parameters.

## 53.5 Why Demos Use Timelines

Music-synchronised demos are often timelines because music has form:

| Musical event | Visual response |
|---------------|-----------------|
| Start | Prepare a restrained scene. |
| First phrase | Introduce motion. |
| Build-up | Increase density and brightness. |
| Accent | Pulse bars, flashes, or scale. |
| Peak | Show the richest combination. |
| Ending | Fade out and stop cleanly. |

The machine does not know those meanings. The programme creates them by
mapping playback position to variables.

## 53.6 Limits

- MIDI position is measured in output samples, not in bars or beats.
- A fallback timer keeps motion alive, but it is not musically exact.
- Avoid long blocking work after reading the music clock.
- Keep the variables small and named. A timeline becomes unreadable if
  every drawing routine reads the player position directly.

Chapter 54 uses the same timing variables to polish the presentation
with copper bands, overlays, and foreground elements.
