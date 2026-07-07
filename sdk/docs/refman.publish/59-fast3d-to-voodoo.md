
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 59 - Fast3D To Voodoo

The graphics port is a translation problem. The game emits display-list
style drawing work. Intuition Engine has a Voodoo rasteriser. The port
does not need to preserve the original graphics hardware. It needs to
preserve the picture.

## 59.1 The Drawing Path

The path is:

```text
game display commands
    |
    v
Fast3D translator
    |
    v
clip-space vertices, colours, texture coordinates
    |
    v
IE Voodoo adapter
    |
    v
Voodoo registers, texture memory, command stream
```

The translator understands the game command stream. The Voodoo side
understands the IE rasteriser. Between them is a small drawing contract:
vertices, texture state, combine mode, clip state, and draw calls.

## 59.2 Vertices And Triangles

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

## 59.3 Texture Upload

The port keeps CPU-side copies of textures in a high-RAM texture store.
When a texture is needed by Voodoo, the adapter streams it into the
Voodoo texture window. The checked engine also exposes a bulk texture
upload extension, so a whole ARGB8888 image can be copied from guest RAM
instead of being written one word at a time.

That optimisation keeps the same visible contract. Texture pixels still
belong to the guest. Voodoo still consumes its texture window. Only the
transfer path changes.

## 59.4 Command Streams

Single MMIO writes are easy to understand, but a large port can issue
many of them per frame. The Voodoo command-stream extension lets guest
RAM contain address/value pairs. Submitting the stream replays those
pairs through the normal Voodoo register path.

The case study uses this to reduce per-write overhead while keeping the
same register truth.

## 59.5 The General IE Lesson

When porting graphics to IE, translate intent into the closest native
card. Do not carry an old rasteriser forward just because the original
machine had one. Preserve the command meaning, then let Voodoo,
VideoChip, or another IE display card draw it.
