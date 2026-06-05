
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 9 - The Voodoo 3D Rasteriser

Voodoo is Intuition Engine's hardware triangle rasteriser. It draws
flat-shaded, Gouraud-shaded, textured, depth-tested, alpha-tested,
fogged, chroma-keyed, and dithered triangles into an RGBA framebuffer
with an associated Z buffer. Voodoo is compositor layer `20`, so its
visible pixels sit above the other picture sources.

The programmer's path is the same as the rest of this guide: type
BASIC commands first, then use `POKE`, `PEEK`, `POKE8`, and `PEEK8`
when direct register control is useful. The register names keep their
Voodoo heritage, but the addresses, formats, side effects, and examples
below are the Intuition Engine contract.

## 9.1 What Voodoo can show

Use Voodoo when a picture is easier to describe as triangles than as
tiles or rectangles: a spinning sign, a shaded cockpit panel, a textured
floor, a depth-tested overlay, or a sprite-like quad that needs alpha.
It is still one card on the same bus. BASIC sets up the state, vertices
become fixed-point register values, and the compositor places the
finished frame above the other display chips.

| Item                | Value                                  |
|---------------------|----------------------------------------|
| Output              | RGBA framebuffer with depth buffer     |
| Default resolution  | `640` by `480`                         |
| Maximum resolution  | `800` by `600`                         |
| Primitive           | Triangle, defined by three vertices    |
| Per-vertex data     | Position, RGBA, Z, S, T, and W         |
| Texture memory      | `64` KB at `$D0000`                   |
| Texture sizes       | up to `256x256` in the texture memory  |
| Texture formats     | paletted, intensity, alpha, and ARGB   |
| Depth functions     | 8 functions, numbered `0` to `7`       |
| Alpha test/blend    | Yes                                    |
| Fog                 | Per-pixel depth fog                    |
| Chroma key          | Yes                                    |
| Dither              | Ordered `4x4` or `2x2`                 |

## 9.2 Programming model

Voodoo is state-machine driven:

1. Enable the chip with `VOODOO ON`.
2. Set the picture size with `VOODOO DIM`.
3. Set draw state, usually `FBZ_MODE`, `ALPHA_MODE`, and
   `FBZCOLOR_PATH`.
4. Clear the back buffer with `VOODOO CLEAR`.
5. Write vertices with `VERTEX A`, `VERTEX B`, and `VERTEX C`.
6. Write colour, depth, texture, or W values with `VOODOO TRI...`
   commands.
7. Submit the triangle with `TRIANGLE`.
8. Publish the frame with `VOODOO SWAP`.

The simplest triangle is a typed BASIC program:

```basic
10 REM VOODOO FIRST TRIANGLE
20 VOODOO ON
30 VOODOO DIM 640,480
40 POKE32 &H000F8110,&H00008200
50 VOODOO CLEAR &HFF101020
60 VERTEX A 320,70
70 VERTEX B 540,390
80 VERTEX C 100,390
90 VOODOO TRICOLOR 4096,0,0,4096
100 TRIANGLE
110 VOODOO SWAP
```

Line `40` enables RGB writes and selects the back draw target. Line
`90` uses 12.12 fixed-point colour: `4096` means full intensity and
`0` means off. The result is a red triangle on a dark background.

## 9.3 BASIC keywords

The `VOODOO` keyword introduces most Voodoo subcommands. `VERTEX`,
`TRIANGLE`, `ZBUFFER`, and `TEXTURE` are companion hardware verbs that
operate on the same state.

| Form                                              | Effect |
|---------------------------------------------------|--------|
| `VOODOO ON` / `VOODOO OFF`                        | Enable or disable the chip. |
| `VOODOO DIM w,h`                                  | Set output dimensions and resize the buffers if valid. |
| `VOODOO CLEAR colour`                             | Store a 32-bit clear word in `COLOR0` and trigger `FAST_FILL_CMD`. |
| `VOODOO SWAP`                                     | Flush queued triangles and publish the frame. |
| `VOODOO CLIP x0,y0,x1,y1`                         | Set the scissor rectangle. |
| `VOODOO COMBINE mode`                             | Write `FBZCOLOR_PATH`. Common values are `0` iterated, `1` texture, `97` modulate. |
| `VOODOO LFB mode`                                 | Write `LFB_MODE`. |
| `VOODOO PIXEL x,y,word`                           | Write one 32-bit word into the linear texture/LFB aperture at `$D0000 + ((y * width + x) * 4)`. |
| `VOODOO RGB ON` / `VOODOO RGB OFF`                | Set or clear the RGB-write bit in `FBZ_MODE`. |
| `VOODOO TRICOLOR r,g,b[,a]`                       | Write `START_R`, `START_G`, `START_B`, and optional `START_A`. Values are 12.12 fixed point. |
| `VOODOO TRISHADE drdx,drdy,dgdx,dgdy,dbdx,dbdy`   | Write colour gradient registers. |
| `VOODOO TRIDEPTH start_z,dzdx,dzdy`               | Write depth start and gradient registers. Values are 20.12 fixed point. |
| `VOODOO TRIUV start_s,start_t,dsdx,dtdx,dsdy,dtdy` | Write texture coordinate start and gradient registers. Values are 14.18 fixed point. |
| `VOODOO TRIW start_w,dwdx,dwdy`                   | Write perspective W start and gradient registers. Values are 2.30 fixed point. |
| `VOODOO ALPHA TEST ON` / `OFF`                    | Set or clear alpha-test enable. |
| `VOODOO ALPHA FUNC n`                             | Set alpha-test function bits `1` to `3`. |
| `VOODOO ALPHA BLEND ON` / `OFF`                   | Set or clear alpha-blend enable. |
| `VOODOO ALPHA SRC n` / `VOODOO ALPHA DST n`       | Set source and destination blend factors. |
| `VOODOO FOG ON` / `OFF`                           | Set or clear fog enable. |
| `VOODOO FOG COLOR r,g,b`                          | Set fog colour as an RGB triplet. |
| `VOODOO DITHER ON` / `OFF`                        | Set or clear ordered dither. |
| `VOODOO CHROMAKEY ON` / `OFF`                     | Set or clear chroma-key discard. |
| `VOODOO CHROMAKEY COLOR r,g,b`                    | Set the chroma-key colour. |
| `VERTEX A x,y`                                    | Write vertex A, converting integer coordinates to 12.4 format. |
| `VERTEX B x,y`                                    | Write vertex B. |
| `VERTEX C x,y`                                    | Write vertex C. |
| `TRIANGLE`                                        | Queue the current triangle. |
| `ZBUFFER ON` / `OFF`                              | Set or clear depth-test enable. |
| `ZBUFFER FUNC n`                                  | Set depth function bits `5` to `7`. |
| `ZBUFFER WRITE ON` / `OFF`                        | Set or clear depth-buffer writes. |
| `TEXTURE ON` / `OFF`                              | Set or clear texture enable in `TEXTURE_MODE`. |
| `TEXTURE MODE mode`                               | Write `TEXTURE_MODE`. |
| `TEXTURE BASE lod,addr`                           | Write one texture base register. |
| `TEXTURE DIM w,h`                                 | Set upload width and height. |
| `TEXTURE UPLOAD`                                  | Commit texture memory into the texture sampler. |

## 9.4 Data formats

Voodoo registers are 32-bit words. `POKE32` writes a whole word. `PEEK32`
reads a whole word. `POKE8` may be used for byte work, but register
side effects happen only after byte `3` of a word has been written.

| Value                 | Format | Unit |
|-----------------------|--------|------|
| Vertex X/Y            | 12.4   | `1` pixel is `16` |
| R/G/B/A               | 12.12  | full intensity is `4096` |
| Z                     | 20.12  | `1.0` is `4096` |
| S/T texture coordinate| 14.18  | `1.0` is `262144` |
| W                     | 2.30   | `1.0` is `1073741824` |

`VERTEX A`, `VERTEX B`, and `VERTEX C` shift integer coordinates left
by four for you. `VOODOO TRICOLOR`, `VOODOO TRIDEPTH`, `VOODOO TRIUV`,
and `VOODOO TRIW` write the raw fixed-point words you supply.

`VOODOO CLEAR` and `COLOR0` use a 32-bit ARGB word: `$AARRGGBB`.
The texture memory aperture is byte-addressed RGBA. When a 32-bit
`POKE32` writes one RGBA texel into little-endian texture memory, the
word appears as `$AABBGGRR`. For example, `&HFF0000FF` stores an
opaque red texture pixel.

## 9.5 Register block

Voodoo's register block runs from `$F8000` to `$F87FF`. Registers
are 32-bit words at 4-byte aligned addresses.

### 9.5.1 Control and status

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `$F8000`  | `VOODOO_STATUS`   | Status. |
| `$F8004`  | `VOODOO_ENABLE`   | Bit `0` enables the chip. |

`VOODOO_STATUS` bits:

| Bits   | Field        | Meaning |
|--------|--------------|---------|
| 0      | `FBI_BUSY`   | Framebuffer interface busy. |
| 1      | `TMU_BUSY`   | Texture unit busy. |
| 2      | `SST_BUSY`   | Overall chip busy. |
| 6      | `VRETRACE`   | Vertical retrace active. |
| 7      | `SWAPBUF`    | Buffer swap pending. |
| 12-19  | `MEMFIFO`    | Coarse memory FIFO free-space field. |
| 20-24  | `PCIFIFO`    | Command FIFO free-space field. |

`MEMFIFO` is useful as a ready/not-ready field, not as a cycle-accurate
historical FIFO depth. It is non-zero while the queued triangle batch
can accept more triangles. It reads zero once the batch reaches its
`4096` triangle limit. `PCIFIFO` reports command space for the current
high-level engine path.

`FBI_BUSY` and `SST_BUSY` are set while `SWAP_BUFFER_CMD` is flushing
queued triangles, swapping buffers, and publishing the frame. A simple
BASIC program may not see these bits because the command completes
before the next `PEEK`, but machine code and IE Mon can still poll them
around a large swap. `TRIANGLE_CMD` itself does not set these busy bits;
it only appends work to the batch.

### 9.5.2 Vertex and attribute registers

| Address    | Name                          | Format |
|------------|-------------------------------|--------|
| `$F8008` to `$F801C` | `VERTEX_AX` to `VERTEX_CY` | 12.4 fixed point |
| `$F8020`  | `START_R`                     | 12.12 fixed point |
| `$F8024`  | `START_G`                     | 12.12 fixed point |
| `$F8028`  | `START_B`                     | 12.12 fixed point |
| `$F802C`  | `START_Z`                     | 20.12 fixed point |
| `$F8030`  | `START_A`                     | 12.12 fixed point |
| `$F8034`  | `START_S`                     | 14.18 fixed point |
| `$F8038`  | `START_T`                     | 14.18 fixed point |
| `$F803C`  | `START_W`                     | 2.30 fixed point |
| `$F8040` to `$F805C` | `DRDX` to `DWDX` | per-X gradients |
| `$F8060` to `$F807C` | `DRDY` to `DWDY` | per-Y gradients |

Without `COLOR_SELECT`, every submitted triangle uses the current
start values for all three vertices. Writing `COLOR_SELECT` with `0`,
`1`, or `2` selects vertex A, B, or C as the target for later
`START_*` writes. The selection applies to colour, Z, S, T, and W.
After `TRIANGLE`, Gouraud selection is reset for the next triangle.

### 9.5.3 Command registers

| Address    | Name                  | Purpose |
|------------|-----------------------|---------|
| `$F8080`  | `TRIANGLE_CMD`        | Queue one triangle using current state. |
| `$F8084`  | `FTRIANGLECMD`        | Queue one triangle through the fast-triangle alias. |
| `$F8088`  | `COLOR_SELECT`        | Select vertex `0`, `1`, or `2` for later attribute writes. |
| `$F8120`  | `NOP_CMD`             | No operation. |
| `$F8124`  | `FAST_FILL_CMD`       | Clear the drawing buffer with `COLOR0`. |
| `$F8128`  | `SWAP_BUFFER_CMD`     | Flush triangles and publish the frame. Bit `0` waits for retrace; bit `1` clears after swap. |

`TRIANGLE_CMD` queues work. It does not rasterise visible pixels and it
does not wait for the rasteriser to draw the triangle. The current
vertex and attribute state is copied into the triangle batch, up to
`4096` triangles. If the batch is already full, further `TRIANGLE_CMD`
writes are ignored until a swap flushes the batch.

Pixels appear after `SWAP_BUFFER_CMD`. That command updates dirty
pipeline state, flushes the queued triangles into the Voodoo backend,
clears the batch, swaps buffers, and publishes the frame to the
compositor. During that flush the status register reports `FBI_BUSY`
and `SST_BUSY`.

### 9.5.4 Mode and state

| Address    | Name                  | Purpose |
|------------|-----------------------|---------|
| `$F8104`  | `FBZCOLOR_PATH`       | Colour combine and source selects. |
| `$F8108`  | `FOG_MODE`            | Fog pipeline. |
| `$F810C`  | `ALPHA_MODE`          | Alpha test and alpha blend. |
| `$F8110`  | `FBZ_MODE`            | Framebuffer, Z, clipping, and draw enable. |
| `$F8114`  | `LFB_MODE`            | Linear framebuffer mode latch. |
| `$F8118`  | `CLIP_LEFT_RIGHT`     | `(left << 16) | right`. |
| `$F811C`  | `CLIP_LOW_Y_HIGH`     | `(top << 16) | bottom`. |

`FBZ_MODE` bits:

| Bit | Field              | Meaning |
|-----|--------------------|---------|
| 0   | `CLIPPING`         | Enable scissor. |
| 1   | `CHROMAKEY`        | Enable chroma-key discard. |
| 2   | `STIPPLE`          | Enable stipple mask. |
| 3   | `WBUFFER`          | Use W-buffer interpretation for depth normalisation. |
| 4   | `DEPTH_ENABLE`     | Enable depth test. |
| 5-7 | `DEPTH_FUNC`       | Depth compare function. |
| 8   | `DITHER`           | Enable ordered dither. |
| 9   | `RGB_WRITE`        | Enable RGB writes. |
| 10  | `DEPTH_WRITE`      | Enable depth writes. |
| 11  | `DITHER_2X2`       | Use 2x2 dither instead of 4x4. |
| 12  | `ALPHA_WRITE`      | Enable alpha buffer writes. |
| 14  | `DRAW_FRONT`       | Draw to front buffer. |
| 15  | `DRAW_BACK`        | Draw to back buffer. |
| 16  | `DEPTH_SOURCE`     | Select depth source. |
| 17  | `Y_ORIGIN`         | Use bottom-left Y origin when set. |
| 18  | `ALPHA_PLANES`     | Preserve interpolated alpha in the target. |
| 19  | `ALPHA_DITHER`     | Dither alpha. |
| 20-31 | `DEPTH_OFFSET`   | Depth bias. |

The default `FBZ_MODE` enables RGB, depth test, depth write, and depth
function `LESS`. If neither draw-target bit is set, the normal back
draw buffer is used.

Depth and alpha function numbers:

| Value | Function       |
|-------|----------------|
| `0`   | `NEVER`        |
| `1`   | `LESS`         |
| `2`   | `EQUAL`        |
| `3`   | `LESSEQUAL`    |
| `4`   | `GREATER`      |
| `5`   | `NOTEQUAL`     |
| `6`   | `GREATEREQUAL` |
| `7`   | `ALWAYS`       |

`ALPHA_MODE` bits:

| Bit  | Field         | Meaning |
|------|---------------|---------|
| 0    | `TEST_EN`     | Enable alpha test. |
| 1-3  | `TEST_FUNC`   | Alpha test function. |
| 4    | `BLEND_EN`    | Enable alpha blending. |
| 5    | `ANTIALIAS`   | Antialias flag. |
| 8-11 | `SRC_RGB`     | Source RGB blend factor. |
| 12-15| `DST_RGB`     | Destination RGB blend factor. |
| 16-19| `SRC_A`       | Source alpha blend factor. |
| 20-23| `DST_A`       | Destination alpha blend factor. |
| 24-31| `REF`         | Alpha-test reference value. |

Blend-factor numbers:

| Value | Factor |
|-------|--------|
| `0`   | zero |
| `1`   | source alpha |
| `2`   | colour average |
| `3`   | destination alpha |
| `4`   | one |
| `5`   | one minus source alpha |
| `6`   | one minus colour average |
| `7`   | one minus destination alpha |
| `15`  | source-alpha saturate |

## 9.6 Fog, chroma, stipple, and constants

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `$F8140` to `$F823C` | `FOG_TABLE_BASE` | 64 fog-table entries. |
| `$F81C4`  | `FOG_COLOR`       | Fog RGB. |
| `$F81C8`  | `ZA_COLOR`        | Z/A constant. |
| `$F81CC`  | `CHROMA_KEY`      | Chroma-key colour. |
| `$F81D0`  | `CHROMA_RANGE`    | Chroma-key range. |
| `$F81D4`  | `STIPPLE`         | Stipple pattern. |
| `$F81D8`  | `COLOR0`          | Constant colour 0, used by fast fill. |
| `$F81DC`  | `COLOR1`          | Constant colour 1. |

`VOODOO FOG COLOR r,g,b` and `VOODOO CHROMAKEY COLOR r,g,b` pack the
triplet as `(b << 16) | (g << 8) | r`. Fog blends the triangle colour
towards the fog colour by the fragment Z value clamped to `0.0` to
`1.0`. Chroma key discards a pixel if the final RGB colour matches the
key within a one-count tolerance; if `CHROMA_RANGE` is non-zero, the
key becomes the low RGB bound and range becomes the high RGB bound.

Stipple uses the low 32 bits as a `4` row by `8` column mask. A zero
stipple word lets every pixel through.

## 9.7 Texture unit

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `$F8300`  | `TEXTURE_MODE`    | Texture mode and format. |
| `$F8304`  | `TLOD`            | Level-of-detail control. |
| `$F8308`  | `TDETAIL`         | Detail texture control. |
| `$F830C` to `$F832C` | `TEX_BASE0` to `TEX_BASE8` | Base address per level. |
| `$F8330`  | `TEX_WIDTH`       | Upload width. |
| `$F8334`  | `TEX_HEIGHT`      | Upload height. |
| `$F8338`  | `TEX_UPLOAD`      | Commit texture memory to the sampler. |
| `$F8400` to `$F87FF` | `PALETTE_BASE` | 256 texture palette entries. |

`TEXTURE_MODE` bits:

| Bit  | Field           | Meaning |
|------|-----------------|---------|
| 0    | `ENABLE`        | Enable texture sampling. |
| 1-3  | `MINIFY`        | Minification filter. |
| 4    | `MAGNIFY`       | Magnification filter. |
| 5    | `CLAMP_S`       | Clamp S to `0.0` through `1.0`. |
| 6    | `CLAMP_T`       | Clamp T to `0.0` through `1.0`. |
| 8-11 | `FORMAT`        | Texture format. |
| 12   | `CHROMA`        | Texture chroma key flag. |
| 13   | `TRILINEAR`     | Trilinear flag. |
| 14   | `PERSPECTIVE`   | Perspective-correct S/T interpolation. |
| 15   | `DETAIL`        | Detail texture flag. |
| 16   | `SEQUENCE`      | Sequence flag. |

Texture format numbers:

| Code | Format |
|------|--------|
| `0`  | 8-bit palette |
| `1`  | YIQ |
| `2`  | A8 |
| `3`  | I8 |
| `4`  | AI44 |
| `5`  | P8 |
| `6`  | ARGB8332 |
| `7`  | AI88 |
| `8`  | ARGB1555 |
| `9`  | ARGB4444 |
| `10` | ARGB8888 |

Upload sequence:

1. Write texels into `$D0000`.
2. Set `TEXTURE DIM w,h`.
3. Set `TEXTURE MODE`.
4. Use `TEXTURE UPLOAD`.
5. Enable texturing with `TEXTURE ON`.
6. Select a texture combine path with `VOODOO COMBINE`.

Upload is ignored if width or height is zero, or if `w * h * 4`
exceeds `64` KB.

## 9.8 Colour pipeline

`FBZCOLOR_PATH` selects the local colour, the other colour, and the
combine function used when texturing is active.

| Bits  | Field          | Meaning |
|-------|----------------|---------|
| 0-1   | RGB source     | `0` iterated, `1` texture, `2` `COLOR1`, `3` LFB. |
| 2-3   | Alpha source   | Same encoding as RGB. |
| 4-6   | Combine mode   | One of the combine functions below. |
| 27    | Texture enable | Include texture colour in the path. |

| Combine code | Function       | Meaning |
|--------------|----------------|---------|
| `0`          | `ZERO`         | Output black. |
| `1`          | `CSUB_CL`      | Other minus local. |
| `2`          | `ALOCAL`       | Local times local alpha. |
| `3`          | `AOTHER`       | Local times other alpha. |
| `4`          | `CLOCAL`       | Local pass through. |
| `5`          | `ALOCAL_T`     | Local alpha times texture. |
| `6`          | `CLOC_MUL`     | Local times other. |
| `7`          | `AOTHER_T`     | Other alpha times texture. |

Convenience values accepted by `VOODOO COMBINE`:

| Value | Meaning |
|-------|---------|
| `0`   | Iterated vertex colour. |
| `1`   | Texture colour. |
| `97`  | Texture times vertex colour. |

## 9.9 Examples

### 9.9.1 Gouraud colour fan

`COLOR_SELECT` lets a program write a different colour at each vertex.
This produces a smooth red, green, and blue triangle.

```basic
10 REM VOODOO GOURAUD FAN
20 VOODOO ON
30 VOODOO DIM 640,480
40 POKE32 &H000F8110,&H00008200
50 VOODOO CLEAR &HFF000018
60 VERTEX A 320,60
70 VERTEX B 560,410
80 VERTEX C 80,410
90 POKE32 &H000F8088,0
100 VOODOO TRICOLOR 4096,0,0,4096
110 POKE32 &H000F8088,1
120 VOODOO TRICOLOR 0,4096,0,4096
130 POKE32 &H000F8088,2
140 VOODOO TRICOLOR 0,0,4096,4096
150 TRIANGLE
160 VOODOO SWAP
```

Expected result: a large triangle blends smoothly between red at the
top, green at the lower right, and blue at the lower left.

### 9.9.2 Depth-tested overlap

This draws a far blue triangle first and a nearer yellow triangle
second. With depth function `LESS`, the nearer triangle wins where the
two overlap.

```basic
10 REM VOODOO Z OVERLAP
20 VOODOO ON
30 VOODOO DIM 640,480
40 POKE32 &H000F8110,&H00008630
50 VOODOO CLEAR &HFF000000
60 VERTEX A 220,80
70 VERTEX B 520,390
80 VERTEX C 80,390
90 VOODOO TRICOLOR 0,0,4096,4096
100 VOODOO TRIDEPTH 3277,0,0
110 TRIANGLE
120 VERTEX A 330,120
130 VERTEX B 560,350
140 VERTEX C 160,350
150 VOODOO TRICOLOR 4096,4096,0,4096
160 VOODOO TRIDEPTH 819,0,0
170 TRIANGLE
180 VOODOO SWAP
```

Line `40` sets `DRAW_BACK`, `DEPTH_WRITE`, `RGB_WRITE`,
`DEPTH_ENABLE`, and depth function `LESS`. `3277` is about `0.8` in
20.12 fixed point. `819` is about `0.2`.

### 9.9.3 Texture upload and textured triangle

This builds a four-pixel texture in texture memory and maps it across
a triangle.

```basic
10 REM VOODOO 2 BY 2 TEXTURE
20 VOODOO ON
30 VOODOO DIM 640,480
40 POKE32 &H000F8110,&H00008200
50 VOODOO CLEAR &HFF080808
60 POKE32 &H000D0000,&HFF0000FF
70 POKE32 &H000D0004,&HFF00FF00
80 POKE32 &H000D0008,&HFFFF0000
90 POKE32 &H000D000C,&HFFFFFFFF
100 TEXTURE DIM 2,2
110 TEXTURE MODE &H0A61
120 TEXTURE UPLOAD
130 TEXTURE ON
140 VOODOO COMBINE 1
150 VERTEX A 320,60
160 VERTEX B 570,410
170 VERTEX C 70,410
180 POKE32 &H000F8088,0
190 VOODOO TRICOLOR 4096,4096,4096,4096
200 VOODOO TRIUV 0,0,0,0,0,0
210 POKE32 &H000F8088,1
220 VOODOO TRICOLOR 4096,4096,4096,4096
230 VOODOO TRIUV 262144,0,0,0,0,0
240 POKE32 &H000F8088,2
250 VOODOO TRICOLOR 4096,4096,4096,4096
260 VOODOO TRIUV 0,262144,0,0,0,0
270 TRIANGLE
280 VOODOO SWAP
```

`&H0A61` enables texturing, selects ARGB8888 format, and clamps S and
T. The texture words are written as little-endian RGBA texels. Expected
result: the triangle samples red, green, blue, and white texels.

### 9.9.4 Linear aperture inspection

`VOODOO PIXEL` writes a 32-bit word into the texture/LFB aperture. It
is useful for building texture memory and for checking address maths.

```basic
10 REM VOODOO PIXEL INSPECT
20 VOODOO ON
30 VOODOO DIM 320,200
40 VOODOO PIXEL 10,5,42
50 PRINT PEEK32(&H000D1928)
```

The printed value is `42`, because `(5 * 320 + 10) * 4` is `6440`,
and `$D0000 + 6440` is `$D1928`.

### 9.9.5 Alpha, fog, chroma key, and dither

This draws translucent fogged colour over a green keyed triangle. The
green triangle is discarded by the chroma key; the magenta triangle is
drawn with alpha blending and dither.

```basic
10 REM VOODOO PIPELINE FLAGS
20 VOODOO ON
30 VOODOO DIM 640,480
40 POKE32 &H000F8110,&H00048202
50 VOODOO CLEAR &HFF202020
60 VOODOO CHROMAKEY COLOR 0,255,0
70 VOODOO CHROMAKEY ON
80 VOODOO DITHER ON
90 VOODOO FOG COLOR 40,40,80
100 VOODOO FOG ON
110 POKE32 &H000F810C,&H00005110
120 VERTEX A 320,80
130 VERTEX B 540,390
140 VERTEX C 100,390
150 VOODOO TRICOLOR 0,4096,0,4096
160 VOODOO TRIDEPTH 3000,0,0
170 TRIANGLE
180 VOODOO CHROMAKEY OFF
190 VERTEX A 330,110
200 VERTEX B 520,360
210 VERTEX C 140,360
220 VOODOO TRICOLOR 4096,0,4096,2048
230 VOODOO TRIDEPTH 1800,0,0
240 TRIANGLE
250 VOODOO SWAP
```

Line `40` keeps alpha planes so the `2048` alpha on line `220` reaches
the blend unit. Line `110` sets blend enable, source factor
`SRC_ALPHA`, and destination factor `ONE_MINUS_SRC_ALPHA`. Expected
result: the green triangle leaves the cleared background untouched,
while the magenta triangle appears softened by fog, alpha, and dither.

## 9.10 Side effects and limits

Voodoo has these programming boundaries:

| Action | Side effect or limit |
|--------|----------------------|
| `VOODOO OFF` | Voodoo contributes no picture to the compositor. |
| `VOODOO DIM w,h` | Ignored unless both dimensions are positive and no larger than `800` by `600`. |
| `VOODOO CLEAR colour` | Clears the drawing buffer and resets depth to a far value for `LESS` style depth functions. |
| `TRIANGLE` | Queues one triangle, up to `4096` queued triangles; extra submissions are ignored until swap. |
| `VOODOO SWAP` | Flushes queued triangles, sets busy while the flush runs, swaps buffers, publishes the frame, and clears the batch. |
| `SWAP_BUFFER_CMD` bit `1` | Clears the drawing buffer after the swap using current `COLOR0`. |
| `POKE8` to registers | Updates the shadow byte immediately, but command side effects run only when byte `3` of the word is written. |
| Texture upload | Copies `w * h * 4` bytes from `$D0000` if the size fits in `64` KB. |
| Texture sampling | Uses point sampling, with wrap by default and clamp when `CLAMP_S` or `CLAMP_T` is set. |
| Chroma key | Discards a final pixel colour that matches the key or keyed range. |
| Fog | Blends by the clamped Z value. |
| Dither | Quantises RGB through an ordered matrix before writing. |
| Draw targets | Front, back, or both may be selected; if no target bit is set, the normal back draw buffer is used. |

Degenerate triangles with zero area are ignored. The rasteriser clips
to the picture bounds and then to the scissor rectangle when clipping
is enabled. Back-facing triangles are reordered internally so that
clockwise and anticlockwise vertex order both draw.
