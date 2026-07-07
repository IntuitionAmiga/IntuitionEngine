
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 54 - Building A Complete Intro

A complete intro is not just one effect. It is a small production. It
preloads assets, starts music, initialises video, runs timed sections,
changes intensity, presents foreground material, and exits cleanly.

The examples in Part VII already contain the parts. This chapter names
the structure.

## 54.1 Intro Skeleton

Use this order:

```text
1. Allocate buffers.
2. Load or build assets.
3. Start music.
4. Configure video and presentation helpers.
5. Initialise timeline variables.
6. Run the frame loop.
7. Switch sections by time.
8. Fade out, stop audio, disable helpers.
```

The order can change, but the responsibilities should stay visible.

## 54.2 Preload

Assets should be loaded before the first timed section when possible:

```basic
FB=MEMALLOC(1228800,4096)
BB=MEMALLOC(1228800,4096)
TX=MEMALLOC(2097152,4096)
CP=MEMALLOC(4096,4096)
SB=MEMALLOC(2162688,4096)
BLOAD "SCROLL.RGBA",SB
```

Do not load a large asset in the middle of a tight visual section unless
the pause is deliberate.

## 54.3 Start Music

Start the audio before the main visual loop:

```basic
SOUND PLAY "INTRO.MID"
T0=PEEK32(&H000F075C)/1000000
```

If the music player exposes a position register, use it. If it does not
advance yet, use the machine timer as a fallback.

## 54.4 Sections

Keep section logic near the top of the frame loop:

```basic
TI=INT(TM*100)
SEC=0
IF TI>=4500 THEN SEC=1
IF TI>=9000 THEN SEC=2
IF TI>=15000 THEN SEC=3
IF TI>=21800 THEN GOTO 9000
```

Drawing routines should read `SEC`, `IN`, `PU`, and other named
variables. They should not all contain their own independent timelines.

## 54.5 Transitions

A transition is a controlled change of state:

| Transition | Common method |
|------------|---------------|
| Fade in | Increase colour or alpha over time. |
| Fade out | Decrease colour or fill darker frames. |
| Change effect | Switch section and reinitialise phase variables. |
| Drop to logo | Stop updating the background, draw foreground only. |
| Exit | Stop audio, disable copper, clear screen. |

The transition should be visible in the code. If an effect simply stops
because the loop falls through, it is harder to debug.

## 54.6 Clean Exit

The ending in a BASIC intro should stop active helpers:

```basic
SOUND PLAY STOP
COPPER OFF
BLIT FILL FB,640,480,&H00000000,2560
VSYNC
END
```

If a worker CPU or player was started by the intro, stop it explicitly.
The machine is shared. Leave the next programme a clean bus.

## 54.7 A Checklist

Before calling an intro complete, check:

- Can it start from a clean BASIC prompt?
- Does the first frame appear without a long black pause?
- Does the music start once?
- Are all buffers separated?
- Are VBlank and blitter waits in the right places?
- Is section timing driven by one clock?
- Does the ending stop audio and presentation helpers?

Chapter 55 explains when to keep this structure in BASIC and when to
move parts to native code or automation.
