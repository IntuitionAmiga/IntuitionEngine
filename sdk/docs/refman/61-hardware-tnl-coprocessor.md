---
title: "Hardware TnL With The Coprocessor System"
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 61 - Hardware TnL With The Coprocessor System

Transform and lighting is a good coprocessor job. It is heavy enough to
matter, regular enough to batch, and naturally described by shared input
and output buffers.

In the checked case-study tree, the main game runs on M68K, display-list
translation runs on M68K worker instance `0`, and TnL runs on IE64
worker instance `0`. The main M68K owns game state and produces a frame
display list. The graphics worker owns traversal and Voodoo submission.
During traversal it sends complete vertex batches to IE64.

Here "hardware TnL" means TnL work moved onto another IE bus CPU
through the coprocessor system. It is not a fixed-function TnL unit
inside Voodoo. Voodoo remains the raster card that receives the
transformed vertices later in the pipeline.

## 61.1 Two Coprocessor Boundaries

The first request is a frame contract from the main M68K to the graphics
worker. It carries the display-list pointer, a snapshot of the segment
table, mirror state, and a bounded list of texture invalidations. Only
one frame is queued or in flight. Before the next frame is submitted,
the main M68K waits for the previous one to finish.

The second request is a vertex-batch contract from the graphics worker
to IE64. That request is smaller in scope but repeated during display-list
translation. It lets IE64 perform the numeric inner loop while the
graphics worker continues with the surrounding command stream.

## 61.2 The TnL Batch

Each batch contains:

| Field | Meaning |
|-------|---------|
| Matrix | Current transform matrix |
| Texture scale | S and T scale factors |
| Vertex count | Number of source vertices |
| Source pointer | Address of the input vertex array |
| Output pointer | Address of the transformed vertex array |
| Geometry mode | Lighting and texture-generation flags |
| Light state | Ambient, directional, and coefficient data |

The request header is `152` bytes. Multi-byte fields are big-endian on
the M68K side. IE64 reads the request and writes transformed vertices
back to shared RAM.

## 61.3 What Each CPU Owns

The main M68K keeps:

- Game state.
- Display-list production.
- Matrix and light selection.
- Input and simulation ordering.
- The decision to submit the next frame.

M68K worker instance `0` keeps:

- Display-list traversal.
- Texture invalidation and translation state.
- Voodoo draw-call ordering and command submission.
- Frame presentation and the one-frame flight limit.
- Decisions about when an IE64 batch must be drained.

IE64 worker instance `0` keeps:

- Repeated vertex transformation.
- Lighting calculation for each submitted vertex.
- Writing the transformed vertex records.
- Advancing its mailbox tail when a request is complete.

That split avoids turning the coprocessor into a remote procedure call
for every tiny operation.

## 61.4 Coarse Work Wins

The port does not send one multiplication at a time. It sends a batch.
The batch is large enough for the worker to do real work before the
caller has to inspect completion state.

After startup, the M68K workers use their assigned shared mailbox rings
directly for frequent dispatch. M68K worker 0 uses ring `4`; its IE64
service uses ring `10`. Both are derived from the final
`cpuTypeIndex * 2 + instance` rule and the uniform `$400` stride. The
services acknowledge mailbox layout version `1` before work is routed
to them.

Direct ring traffic keeps the hot path close to RAM and avoids paying a
full command-device cost for each vertex group. Compiler barriers keep
request stores before the head update and response reads after the
completion wait.

## 61.5 The General IE Lesson

Use coprocessors as a pipeline, not as a collection of remote arithmetic
instructions. Put a complete frame or vertex batch in RAM, pass stable
pointers, assign one owner to mutable state, and use a small completion
record. If the boundary is too fine, the mailbox becomes the programme.
