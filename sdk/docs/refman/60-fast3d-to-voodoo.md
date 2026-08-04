---
title: "Fast3D To Voodoo"
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 60 - Fast3D To Voodoo

The graphics port is a translation problem. The game emits display-list
style drawing work. Intuition Engine has a Voodoo rasteriser. The port
does not need to preserve the original graphics hardware. It needs to
preserve the picture.

## 60.1 The Drawing Path

The path is:

```text
main M68K display commands
    |
    v
M68K worker 0: Fast3D translator
    |
    +----> IE64 worker: transformed and lit vertex batches
    |
    v
clip-space vertices, colours, texture coordinates
    |
    v
IE Voodoo adapter, command stream, and frame pacing
    |
    v
Voodoo registers, texture memory, and framebuffer
```

The main M68K produces a display list and submits one frame request.
M68K worker 0 snapshots the segment table and other per-frame state,
walks that list, and translates its commands. Repeated transform and
lighting work is batched for IE64. The graphics worker then submits the
resulting drawing work to Voodoo and waits for presentation.

The translator understands the game command stream. The Voodoo side
understands the IE rasteriser. Between them is a small drawing contract:
vertices, texture state, combine mode, clip state, and draw calls.

## 60.2 Vertices And Triangles

The Voodoo side receives vertices in a form that is ready for the final
screen transform. It performs perspective divide, viewport mapping, and
screen-space triangle submission.

The triangle still becomes ordinary Voodoo work:

- Select colour and texture state.
- Upload or bind texture data.
- Write vertex coordinates, depth, colour, and texture coordinates.
- Submit the triangle.

The important point is that Voodoo is not a hidden renderer. It is the
IE 3D card receiving work through its register path.

## 60.3 Texture Upload

The port keeps CPU-side copies of textures in a high-RAM texture store.
It checks `SYSINFO_FEATURES` before using retained Voodoo texture slots.
The first use of a texture generation selects a slot and uploads the
ARGB8888 image. A later switch back to the same unchanged generation
writes its identifier to `VOODOO_TEX_BIND`; the texels do not cross the
bus again. Changing the texture creates a new generation and causes a
fresh upload into that slot.

The bulk texture-upload extension lets the first upload copy the whole
image from guest RAM instead of writing one word at a time. If retained
slots are unavailable, the adapter falls back to uploading the selected
texture again. These optimisations preserve the same visible contract:
texture pixels belong to the guest, and each submitted triangle binds
the texture that is current at submission time.

## 60.4 Command Streams

Single MMIO writes are easy to understand, but a large port can issue
many of them per frame. The Voodoo command-stream extension lets guest
RAM contain address/value pairs. Submitting the stream replays those
pairs through the normal Voodoo register path.

The graphics worker uses this to reduce per-write overhead while keeping
the same register truth. Shared command and texture buffers let the
worker submit a frame without turning every translated operation into a
cross-CPU call.

## 60.5 The General IE Lesson

When porting graphics to IE, translate intent into the closest native
card. Put a whole translation stage on a worker when it can own the
request and its buffers cleanly. Preserve the command meaning, then let
Voodoo, VideoChip, or another IE display card draw it.
