
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 60 - Hardware TnL With The Coprocessor System

Transform and lighting is a good coprocessor job. It is heavy enough to
matter, regular enough to batch, and naturally described by shared input
and output buffers.

In the checked case-study tree, the main game runs on M68K and the TnL
service runs as an IE64 coprocessor worker. The M68K side owns game
state and display-list traversal. The worker receives vertex batches,
matrices, lighting state, and output addresses.

Here "hardware TnL" means TnL work moved onto another IE bus CPU
through the coprocessor system. It is not a fixed-function TnL unit
inside Voodoo. Voodoo remains the raster card that receives the
transformed vertices later in the pipeline.

## 60.1 What Crosses The Boundary

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
the M68K side. The worker reads the request and writes the transformed
vertices back to shared RAM.

## 60.2 What Stays Local

The M68K side keeps:

- Game state.
- Display-list traversal.
- Matrix and light selection.
- Voodoo draw-call ordering.
- Decisions about when the previous batch must be drained.

The worker keeps:

- Repeated vertex transformation.
- Lighting calculation for each submitted vertex.
- Writing the transformed vertex records.
- Advancing its mailbox tail when a request is complete.

That split avoids turning the coprocessor into a remote procedure call
for every tiny operation.

## 60.3 Coarse Work Wins

The port does not send one multiplication at a time. It sends a batch.
The batch is large enough for the worker to do real work before the
caller has to inspect completion state.

The M68K caller also uses the shared mailbox ring directly for frequent
batch dispatch after the service has started. That keeps the hot path
close to RAM and avoids paying a full command-device cost for each
vertex group.

## 60.4 The General IE Lesson

Use coprocessors for coarse, shared-memory jobs. Put a complete piece of
work in RAM, pass a pointer, let the worker finish it, and read a small
status result. If the boundary is too fine, the mailbox becomes the
programme.
