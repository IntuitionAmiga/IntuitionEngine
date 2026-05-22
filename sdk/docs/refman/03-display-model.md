---
title: "Display Model Overview"
sources:
  - video_compositor.go
  - video_chip.go
  - video_vga.go
  - video_ted.go
  - ted_video_constants.go
  - video_antic.go
  - antic_constants.go
  - video_ula.go
  - ula_constants.go
  - video_voodoo.go
  - voodoo_constants.go
  - sdk/include/ehbasic_hw_video.inc
---

# Chapter 3 - Display Model Overview

Intuition Engine has six independent picture-making chips. Each one
draws a frame in its own format and gives that frame to a single
piece of hardware called the **VideoCompositor**. The compositor
stacks the six frames on top of each other and sends the result to
the screen.

This chapter explains how the stack works, what the output looks
like, and how colours are chosen. Each of the six chips has a
chapter of its own that tells you how to program it directly.

## 3.1 The six picture sources

```
                          +-----------------+
                          |  VideoCompositor  |
                          +-----------------+
                                  |
   +------+--------+--------+--------+--------+--------+
   |      |        |        |        |        |        |
VideoChip VGA  TED video  ANTIC    ULA    Voodoo 3D
(layer 0) (10)   (12)     (12 + GTIA) (15)   (20)
```

Each source produces a frame at its own size and resolution. The
compositor accepts whatever size each source produces and scales it
to fit the output screen. The output screen is `1920` pixels wide
and `1080` pixels tall.

| Chip      | Best for                                          | See chapter |
|-----------|---------------------------------------------------|-------------|
| VideoChip | Bitmap and CLUT8 modes, copper list, blitter      | 4 |
| VGA       | Text and 256-colour graphics modes                | 5 |
| TED video | TED-class character and bitmap modes              | 6 |
| ANTIC     | Display-list video with GTIA companion            | 7 |
| ULA       | ZX-class attribute display                        | 8 |
| Voodoo 3D | Triangle rasterisation with Z-buffer and texture  | 9 |

You may use any of the six at the same time. You may also leave
some of them switched off. A source that is disabled or that
returns no frame contributes no pixels.

## 3.2 The layer stack

Every source has a fixed **layer number**. The compositor sorts the
sources by layer number, smallest first, and draws each one into
the output frame in that order. A later source paints on top of an
earlier one.

| Layer | Source     |
|-------|------------|
| `0`   | VideoChip  |
| `10`  | VGA        |
| `12`  | TED video  |
| `13`  | ANTIC      |
| `15`  | ULA        |
| `20`  | Voodoo 3D  |

The layer numbers are part of the hardware. They cannot be changed
from BASIC. To put a frame behind everything, use the VideoChip
(layer `0`). To put a frame on top of everything, use Voodoo 3D
(layer `20`).

The gaps between the numbers (`10` to `20`) leave room for future
sources without disturbing the existing order. Today only the six
sources above exist.

## 3.3 Mask blending

The compositor does not mix colours. It treats each source pixel as
either **opaque** or **transparent**. A pixel is opaque if its
alpha byte is non-zero, and transparent if its alpha byte is zero.

| Source alpha | What the compositor does |
|--------------|--------------------------|
| `0`          | Leave the destination pixel as it was. |
| any non-zero | Replace the destination pixel with the source's RGB. |

The result is the same as if you had laid one transparent acetate
on top of another and looked at the stack from above: the topmost
opaque pixel at any position is the one you see. Each chip's
colour-zero or background pixel is normally transparent, so the
chip below shows through where the chip above is "blank".

The compositor never partially blends two pixels. If you want
something that looks like a half-transparent overlay, write that
into the source's pixels directly - for example by drawing every
other pixel.

## 3.4 Output resolution and refresh

The compositor sends the final frame at the following fixed rate:

| Property      | Value         |
|---------------|---------------|
| Output width  | `1920` pixels |
| Output height | `1080` pixels |
| Frame rate    | `60` Hz       |
| Pixel format  | `RGBA`, 4 bytes per pixel |

A source that produces a frame at the full `1920` × `1080` resolution
is copied to the output 1:1. A source that produces a smaller frame
is scaled up by the compositor. The scale mode is **stretch-fill**:
the source frame is rescaled to fill the entire output area, even
if the aspect ratios differ.

## 3.5 The coordinate system

Every chip uses the same coordinate convention:

```
    (0, 0)              (W-1, 0)
       +-----------------+
       |                 |
       |                 |
       |                 |
       +-----------------+
    (0, H-1)         (W-1, H-1)
```

The origin is the top-left corner of the picture. `X` increases to
the right; `Y` increases downward. `W` and `H` are the width and
height of the source's frame, which may be smaller than the output
screen.

## 3.6 The palette

Each chip has its own palette. There is no global palette shared
between chips. A palette entry is a 24-bit value made of three
8-bit components for red, green, and blue.

From BASIC, the `PALETTE` and `PLOT` statements operate on the VGA
chip (see Chapter 5). The other chips are programmed through their
own MMIO registers - either with `POKE` from BASIC or directly from
machine language. Each chip's chapter lists the addresses.

The number of palette entries available depends on the chip. The
typical sizes are:

| Chip      | Palette size                                       |
|-----------|----------------------------------------------------|
| VideoChip | 256 entries (CLUT8 mode)                           |
| VGA       | 256 entries                                        |
| TED video | 121 unique colours (16 hues × 8 luminances + black)|
| ANTIC     | 256 entries (16 hues × 16 luminance steps)         |
| ULA       | 15 unique colours (8 base + 7 bright; black has no bright form) |
| Voodoo 3D | Per-vertex 24-bit colour, plus a 256-entry texture palette for indexed textures |

See the chip's chapter for the exact byte layout of its palette and
for any chip-specific extensions (such as ULA attribute pairing).

## 3.7 The vertical retrace

After the compositor finishes assembling one output frame, there is
a short pause before the next one begins. This pause is the
**vertical retrace**. It is the safe time to update display memory:
any changes you make during the retrace are picked up by the next
frame as a single update, with no tearing.

The `VSYNC` statement waits for the next vertical retrace:

```basic
10 VSYNC
20 REM update display memory here
```

Every chip exposes its own retrace status flag in its MMIO map. See
Appendix D for the addresses.

## 3.8 Choosing a chip

You normally pick a chip first and then program it for the rest of
the picture. Reasons to pick each one:

- **VideoChip** is the IE's general-purpose framebuffer chip. It
  handles 256-colour CLUT8 bitmaps and direct-colour RGBA bitmaps,
  a copper list for per-scanline changes, and a blitter for
  hardware copies. Pick it when you want maximum flexibility and a
  programmable raster.
- **VGA** is the cleanest text mode and the smoothest path to a
  scrolling 256-colour bitmap with rectangular tiles. Pick it for
  text adventures, scrolling shoot-em-ups, and anything that wants
  hardware character generation.
- **TED video** mirrors the C16/Plus-4 picture model and lets you
  reproduce that machine's look directly in hardware.
- **ANTIC + GTIA** mirrors the Atari 8-bit picture model. The
  display list lets you change the mode line by line and is the
  shortest path to colourful mixed-mode screens.
- **ULA** mirrors the ZX Spectrum picture model: a monochrome
  bitmap with 8×8 colour attributes.
- **Voodoo 3D** rasterises triangles into a Z-buffered RGBA frame.
  Pick it for 3D scenes.

You can layer two chips for tricks the compositor allows for free:
draw a bitmap on the VideoChip and place text from the VGA on top,
or use the ULA to overlay an attribute grid on an ANTIC display.
The compositor handles the stacking; you do not write any blending
code yourself.

## 3.9 What comes next

The remaining chapters of Part II describe each chip in turn. Each
chapter follows the same plan:

1. What the chip's frame looks like (mode list, resolution).
2. The MMIO map.
3. How to set a mode and draw a pixel.
4. The chip's hardware features (sprites, scrolling, copper, etc.).
5. Example BASIC programs.

Appendix D is the consolidated MMIO map for every chip. Appendix K
is a block diagram of the whole video system.
