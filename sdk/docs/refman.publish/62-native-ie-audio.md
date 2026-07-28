
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 62 - Native IE Audio Instead Of RSP-Style Mixing

The audio port follows the same rule as the graphics port. Preserve the
musical behaviour at the right level. Do not reproduce the wrong machine
part merely because the original machine used it.

In the case-study port, the original sequence logic still owns musical
timing, note allocation, pitch, envelopes, and live-note decisions. That
logic runs on M68K worker instance `1`. The worker maps its live notes to
native IE voices. The IE mixer performs the final sample playback and
mixing.

## 62.1 The Audio Split

The split is:

| Old responsibility | IE responsibility |
|--------------------|-------------------|
| Sequence timing | Keep in the game audio code, run on M68K worker 1 |
| Note selection | Keep in the game audio code, run on M68K worker 1 |
| Pitch and envelope decisions | Keep in the game audio code, run on M68K worker 1 |
| Sample voice playback | Map to IE SFX voices |
| Final mixing | Use the IE audio mixer |

The result is not a software copy of the old audio processor. It is the
same music logic speaking through Intuition Engine.

## 62.2 Voice Mapping

Each live note can be represented as an IE voice with a sample pointer,
length, frequency, volume, and control word. The audio worker tracks the
voices, updates only what changed, and lets the mixer produce the final
output.

This matters because audio is updated many times during a frame. A port
that writes every voice field every time wastes bus bandwidth. A port
that shadows voice state can reduce writes while preserving the same
sound decisions.

## 62.3 The Audio Service Contract

The main M68K produces audio commands in a `256`-entry command ring. At
the normal audio cadence it sends the previous and current producer
indices to M68K worker instance `1`. The worker consumes that range,
advances sequence state, and writes the resulting voice controls.

Only one pump request is in flight. The main M68K waits for the previous
pump before reusing its request record. A high-water check forces a
synchronous drain if the producer approaches the point where its
eight-bit index could lap the worker.

The audio service uses mailbox ring `5` and acknowledges the current
layout version before startup completes. If the service cannot start or
initialise, the same pump remains available on the main M68K.

## 62.4 Keeping Audio Fed

Long file or asset operations can starve audio if the game only pumps
sound at the end of a frame. The IE asset layer has a hook so audio work
can be serviced during longer transfers.

That is a small detail, but it is the difference between "the port
runs" and "the port feels like a programme".

## 62.5 The General IE Lesson

When moving audio to IE, keep composition and timing at the useful
level, place periodic sequence work on a worker when its state boundary
is clear, and use native IE engines for playback. Preserve the song, not
the old mixer implementation.
