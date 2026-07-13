---
title: "Why This Port Is Different"
sources:
  - ../mk64-ie/IE-PORT-NOTES.md
  - ../mk64-ie/src/platform/platform.h
  - ../mk64-ie/ie/ie_runtime.c
  - ../mk64-ie/ie/game.ld
  - ../mk64-ie/ie/coproc/ie_coproc.c
  - ../mk64-ie/ie/coproc/ie_coproc.h
  - ../mk64-ie/ie/coproc/tnl_service_ie64.asm
  - ../mk64-ie/ie/ie_gfx_voodoo.c
  - ../mk64-ie/ie/ie_platform_audio.c
  - ../mk64-ie/ie/ie_gfx_svc_client.c
  - ../mk64-ie/ie/ie_audio_svc_client.c
  - ../mk64-ie/ie/coproc/gfx_svc_main.c
  - ../mk64-ie/ie/coproc/audio_svc_main.c
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 56 - Why This Port Is Different

This part studies an open-source decompilation-derived porting tree and
original Intuition Engine integration work as an IE systems case study.
It does not reproduce ROM assets, require the reader to build the port,
or teach commercial-game asset extraction.

The case study is useful because it is not a whole-console preservation
layer and not a simple profile. It does not try to rebuild another console
inside Intuition Engine. It treats Intuition Engine as the target
machine.

The game core runs as an M68K programme with an MC68020-class address
space and FPU arithmetic. It is joined by two M68K workers and one IE64
worker. One M68K worker translates display lists and drives Voodoo. The
other advances the music and controls native IE voices. IE64 performs
batched transform and lighting work. Voodoo draws the triangles. The file
device supplies saves, while a packed image can supply the programme,
assets, and worker services together.

That is the important shape:

```text
Main M68K ----> game state, input, display lists, audio commands
    |
    +----> M68K worker 0 ----> display-list translation and frame pacing
    |             |
    |             +----> IE64 worker 0 ----> transform and lighting
    |             |
    |             +----> Voodoo ----> composited IE frame
    |
    +----> M68K worker 1 ----> sequence and note control
                                  |
                                  +----> IE SFX voices ----> IE mixer

Packed data and File I/O ----> platform contracts ----> game state
```

These are not four separate computers. They are four processors on the
same IE backplane, sharing programme data and sending coarse requests
through the coprocessor mailbox. Voodoo is another card on that same
machine.

## 56.1 Not Another Console In A Box

A preserved console boundary keeps the old machine intact. This port
breaks that boundary on purpose. The old game logic is kept where it is
useful, but the heavy platform work is moved onto Intuition Engine
devices.

The result is a normal IE programme with an unusual history:

- The main M68K runs the game loop and owns mutable game state.
- The FPU is used for game-side floating-point work.
- M68K worker 0 translates drawing work and submits it to Voodoo.
- IE64 transforms and lights batches of vertices for that worker.
- M68K worker 1 advances music and updates native IE voices.
- Shared RAM carries requests, responses, drawing data, and audio state.
- Voodoo and the IE mixer finish the visible and audible work.
- File I/O supplies assets, save data, and test-visible records.

The reader should notice what is missing. There is no reader path that
requires a ROM, an extraction step, or an external build. This chapter
uses the port as an engineering example only.

## 56.2 Why This Belongs In The Guide

Earlier chapters teach the parts separately: M68K, coprocessors, Voodoo,
audio, File I/O, input, IE Mon, and IE Script. A large port uses them
together.

The case study shows four rules that small examples cannot show as
well:

1. Keep ownership of mutable game state on one control CPU.
2. Move coarse throughput work to workers with clear contracts.
3. Use the machine's graphics and audio cards at their native boundary.
4. Measure each pipeline stage instead of guessing where time went.

## 56.3 The General IE Lesson

Intuition Engine rewards ports that choose a new machine boundary. Do
not recreate a whole old system when a native IE card can do the job
better. Keep the programme's behaviour, but let the backplane, CPUs,
Voodoo, audio engines, and file device be the machine it runs on.
