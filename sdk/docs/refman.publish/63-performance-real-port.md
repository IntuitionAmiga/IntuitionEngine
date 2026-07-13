
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 63 - Performance Work On A Real Port

Performance work on a large IE port is measurement work. Guessing is not
enough when one frame can involve game code, matrix work, texture
streams, Voodoo command submission, audio voice writes, file activity,
and input.

## 63.1 Count The Frame

The case-study port records counters for the work that matters:

| Counter family | What it answers |
|----------------|-----------------|
| Triangles and texture rectangles | How much picture work reached Voodoo |
| Draw calls | How often the drawing contract was invoked |
| MMIO writes | How much register traffic the frame generated |
| Command submits and pairs | How much work moved through Voodoo streams |
| Texture stream bytes | How much texture data crossed into Voodoo |
| Audio voice writes | How much audio control traffic was emitted |
| Clipped triangles | How much graphics work was rejected before drawing |
| Graphics-worker frames and time | Whether translation or presentation owns the frame |
| Audio-worker pumps and time | Whether sequence work keeps its cadence |
| Coprocessor drains and waits | Whether useful overlap was achieved |

These are not decorative numbers. They tell you whether a change moved
work out of the hot path or merely moved it somewhere harder to see.

## 63.2 Compare Like With Like

Frame-rate readings are useful only when the scene, frame group, and
instrumentation are known. A short race scene, a title screen, and a
blank smoke frame do not measure the same thing.

The case study uses groups of frames rather than single-frame claims. It
also separates time spent by the main M68K, graphics worker, IE64 TnL
worker, audio worker, and Voodoo submission path. That makes changes such
as command streams, texture upload paths, audio shadowing, and
coprocessor batching visible.

## 63.3 Reduce Traffic, Not Meaning

Several optimisations preserve the same visible behaviour:

- Keep texture copies in high RAM, retain uploaded generations in
  Voodoo slots, and bind unchanged generations by identifier;
  retransmit them when slots are not available.
- Submit Voodoo register writes through a command stream.
- Avoid rewriting unchanged audio voice fields.
- Submit one coarse frame contract to the graphics worker.
- Batch transform and lighting work for IE64.
- Keep only one graphics frame and one audio pump in flight, so overlap
  does not become unbounded ownership.
- Measure MMIO volume before and after each change.

The work is still the same game frame. The bus traffic is cleaner.

## 63.4 The General IE Lesson

Profile the contract, not your hopes. Count triangles, bytes, MMIO
writes, voice updates, command pairs, worker completions, and frame
groups. Attribute time to the pipeline stage that owns it, then optimise
the path that the measurements prove is hot.
