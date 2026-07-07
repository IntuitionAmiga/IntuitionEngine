---
title: "Native IE Audio Instead Of RSP-Style Mixing"
sources:
  - ../mk64-ie/ie/ie_platform_audio.c
  - ../mk64-ie/src/audio/audio_api.h
  - ../mk64-ie/ie/ie_mmio.h
  - ../mk64-ie/PERF-NOTES.md
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 61 - Native IE Audio Instead Of RSP-Style Mixing

The audio port follows the same rule as the graphics port. Preserve the
musical behaviour at the right level. Do not reproduce the wrong machine
part merely because the original machine used it.

In the case-study port, the sequence player still owns musical timing,
note allocation, pitch, envelopes, and live-note decisions. The final
sample playback and mixing work moves to native IE voices.

## 61.1 The Audio Split

The split is:

| Old responsibility | IE responsibility |
|--------------------|-------------------|
| Sequence timing | Keep in the game audio code |
| Note selection | Keep in the game audio code |
| Pitch and envelope decisions | Keep in the game audio code |
| Sample voice playback | Map to IE SFX voices |
| Final mixing | Use the IE audio mixer |

The result is not a software copy of the old audio processor. It is the
same music logic speaking through Intuition Engine.

## 61.2 Voice Mapping

Each live note can be represented as an IE voice with a sample pointer,
length, frequency, volume, and control word. The IE side tracks the
voices, updates only what changed, and lets the mixer produce the final
output.

This matters because audio is updated many times during a frame. A port
that writes every voice field every time wastes bus bandwidth. A port
that shadows voice state can reduce writes while preserving the same
sound decisions.

## 61.3 Keeping Audio Fed

Long file or asset operations can starve audio if the game only pumps
sound at the end of a frame. The IE asset layer has a hook so audio work
can be serviced during longer transfers.

That is a small detail, but it is the difference between "the port
runs" and "the port feels like a programme".

## 61.4 The General IE Lesson

When moving audio to IE, keep composition and timing where they make
sense, then use the native audio engines for playback. Preserve the
song, not the old mixer implementation.
