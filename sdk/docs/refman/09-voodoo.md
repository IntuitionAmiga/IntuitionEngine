---
title: "Voodoo 3D Rasterizer"
sources:
  - voodoo_constants.go
  - video_voodoo.go
  - voodoo_software.go
  - sdk/include/ehbasic_hw_voodoo.inc
---

# Chapter 9 - The Voodoo 3D Rasterizer

Voodoo is Intuition Engine's hardware triangle rasterizer. It draws
flat-shaded, Gouraud-shaded, and textured triangles into an RGBA
framebuffer with an associated Z-buffer. Voodoo sits on compositor
layer `20`, the top of the stack, so its picture covers every other
source where it draws.

This chapter describes Voodoo's programming model. The chip's
register set is closely modelled on the 3DFX Voodoo Graphics SST-1,
so existing 3DFX programming literature is a useful reference - but
the addresses are IE addresses, given in the tables below.

## 9.1 What Voodoo can show

| Item                | Value                                  |
|---------------------|----------------------------------------|
| Output              | RGBA framebuffer with depth (Z) buffer |
| Default resolution  | `640` × `480`                          |
| Maximum resolution  | `800` × `600`                          |
| Primitive           | Triangle, defined by three vertices    |
| Per-vertex data     | Position (12.4), RGBA (12.12), Z (20.12), S/T texture (14.18), W (2.30) |
| Texture sizes       | up to `256` × `256` from `64` KB of texture memory |
| Texture formats     | 11 formats including paletted, ARGB1555, ARGB4444, ARGB8888 |
| Depth functions     | 8 (NEVER through ALWAYS)               |
| Alpha test/blend    | Yes, 16 blend factors                  |
| Fog                 | Per-pixel, table-based                 |
| Chroma key          | Yes                                    |

## 9.2 Programming model

Voodoo is **state-machine driven**. A program prepares the chip by
writing into mode and state registers, builds a triangle by writing
three vertex positions and a set of "start" colour and texture
values, and finally issues a triangle command. The chip then
rasterises the triangle into the back buffer.

The shortest path from machine language:

1. Enable the chip (`VOODOO_ENABLE` = `1`).
2. Pick the output dimensions in `VOODOO_VIDEO_DIM`.
3. Configure `FBZ_MODE`, `ALPHA_MODE`, and `FBZCOLOR_PATH` for the
   desired pixel-pipeline behaviour.
4. (Optional) Upload textures into texture memory and configure
   `TEXTURE_MODE` and the texture base registers.
5. For each triangle:
   - Write vertex positions to `VERTEX_AX`/`AY`/`BX`/`BY`/`CX`/`CY`.
   - Write the start values (colour, Z, S, T, W) and any per-X and
     per-Y deltas.
   - Trigger rasterisation by writing to `TRIANGLE_CMD`.
6. Swap the buffers with `SWAP_BUFFER_CMD`.

## 9.3 BASIC keywords

The `VOODOO` keyword introduces every Voodoo subcommand. A
companion `VERTEX`, `TRIANGLE`, `ZBUFFER`, and `TEXTURE` are
recognised as standalone hardware verbs that operate on the same
state. See Chapter 2 for complete entries.

| Form                                              | Effect |
|---------------------------------------------------|--------|
| `VOODOO ON` / `VOODOO OFF`                        | Enable / disable the chip. |
| `VOODOO DIM `*w*`, `*h*                           | Set the output dimensions. |
| `VOODOO CLEAR `*r*`, `*g*`, `*b*                  | Fast-fill the back buffer with a colour. |
| `VOODOO SWAP`                                     | Swap front and back buffers. |
| `VOODOO CLIP `*x0*`, `*y0*`, `*x1*`, `*y1*        | Set scissor rectangle. |
| `VOODOO COMBINE `*mode*                           | Set the colour-combine mode (`ITERATED`/`TEXTURE`/`MODULATE`/`ADD`/`DECAL`). |
| `VOODOO LFB `*addr*                               | Set the linear-framebuffer base for LFB writes. |
| `VOODOO PIXEL `*x*`, `*y*`, `*r*`, `*g*`, `*b*    | Write one pixel through the LFB port. |
| `VOODOO RGB ON` / `OFF`                           | Enable / disable RGB writes. |
| `VOODOO TRICOLOR `*r*`, `*g*`, `*b*` [, `*a*`]`   | Set the per-triangle start colour at vertex A (writes `START_R`, `START_G`, `START_B`, and optionally `START_A`). For flat shading, leave the colour deltas at zero. |
| `VOODOO TRISHADE `*drdx*`, `*drdy*`, `*dgdx*`, `*dgdy*`, `*dbdx*`, `*dbdy*` | Set the six per-pixel colour gradients for Gouraud shading (writes `DRDX`/`DRDY`/`DGDX`/`DGDY`/`DBDX`/`DBDY`). |
| `VOODOO TRIDEPTH `*start_z*`, `*dzdx*`, `*dzdy*`  | Set the depth start value and gradients (writes `START_Z`, `DZDX`, `DZDY`). |
| `VOODOO TRIUV `*start_s*`, `*start_t*`, `*dsdx*`, `*dtdx*`, `*dsdy*`, `*dtdy*` | Set the texture coordinate start and gradient registers (writes `START_S`, `START_T`, `DSDX`, `DTDX`, `DSDY`, `DTDY`). |
| `VOODOO TRIW `*start_w*`, `*dwdx*`, `*dwdy*       | Set the perspective `W` (`1/Z`) start value and gradients (writes `START_W`, `DWDX`, `DWDY`). |
| `VOODOO ALPHA `*mode*                             | Configure the alpha pipeline (test enable/function, blend enable/factors). |
| `VOODOO FOG ON` / `OFF` / `COLOR `*r*`, `*g*`, `*b*` | Configure fog. |
| `VOODOO DITHER ON` / `OFF`                        | Enable / disable dithering. |
| `VOODOO CHROMAKEY ON` / `OFF` / `COLOR `*r*`, `*g*`, `*b*` | Configure chroma key. |
| `VERTEX A `*x*`, `*y*` / `VERTEX B ... / VERTEX C ...` | Write a vertex's screen position. |
| `TRIANGLE`                                        | Submit the current vertex set as one triangle. |
| `ZBUFFER ON` / `OFF` / `FUNC `*n*` / `WRITE `*flag* | Configure depth testing and depth writes. |
| `TEXTURE `*args*                                  | Upload texture data (Chapter 2 lists all forms). |

## 9.4 The register block

Voodoo's register block runs `0xF8000`–`0xF87FF`. Registers are
32-bit words at 4-byte-aligned addresses.

### 9.4.1 Control and status

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF8000`  | `VOODOO_STATUS`   | Status (read-only). |
| `0xF8004`  | `VOODOO_ENABLE`   | Bit `0` = enable. |

`VOODOO_STATUS` bits:

| Bits   | Field        | Meaning |
|--------|--------------|---------|
| 0      | `FBI_BUSY`   | Framebuffer interface busy. |
| 1      | `TMU_BUSY`   | Texture-mapping unit busy. |
| 2      | `SST_BUSY`   | Overall chip busy. |
| 6      | `VRETRACE`   | Vertical retrace active. |
| 7      | `SWAPBUF`    | Buffer-swap pending. |
| 12–19  | `MEMFIFO`    | Memory-FIFO entries (8 bits). |
| 20–24  | `PCIFIFO`    | PCI-FIFO entries (5 bits). |

### 9.4.2 Vertex registers

The six vertex coordinate registers hold three pairs of (X, Y) in
12.4 fixed-point format. The "start" registers hold the values of
colour, depth, and texture coordinates at vertex A; the eight "dX"
registers hold the per-X-step derivatives, and the eight "dY"
registers hold the per-Y-step derivatives.

| Address    | Name                          | Format |
|------------|-------------------------------|--------|
| `0xF8008`–`0xF801C` | `VERTEX_AX`–`CY`     | 12.4 fixed-point |
| `0xF8020`  | `START_R`                     | 12.12 fixed |
| `0xF8024`  | `START_G`                     | 12.12 fixed |
| `0xF8028`  | `START_B`                     | 12.12 fixed |
| `0xF802C`  | `START_Z`                     | 20.12 fixed |
| `0xF8030`  | `START_A`                     | 12.12 fixed |
| `0xF8034`  | `START_S`                     | 14.18 fixed |
| `0xF8038`  | `START_T`                     | 14.18 fixed |
| `0xF803C`  | `START_W`                     | 2.30 fixed |
| `0xF8040`–`0xF805C` | `DRDX` … `DWDX`      | 8 × per-X derivatives |
| `0xF8060`–`0xF807C` | `DRDY` … `DWDY`      | 8 × per-Y derivatives |

### 9.4.3 Command registers

| Address    | Name                  | Purpose |
|------------|-----------------------|---------|
| `0xF8080`  | `TRIANGLE_CMD`        | Rasterise one triangle using the current state. |
| `0xF8084`  | `FTRIANGLECMD`        | Fast triangle (triangle-strip mode). |
| `0xF8088`  | `COLOR_SELECT`        | Select the vertex (`0`/`1`/`2`) that subsequent colour writes target (Gouraud shading). |
| `0xF8120`  | `NOP_CMD`             | No operation. |
| `0xF8124`  | `FAST_FILL_CMD`       | Fast-fill the buffer with `COLOR0`. |
| `0xF8128`  | `SWAP_BUFFER_CMD`     | Swap front/back buffers (bit `0` = wait for VSync, bit `1` = clear after swap). |

### 9.4.4 Mode and state

| Address    | Name                  | Purpose |
|------------|-----------------------|---------|
| `0xF8104`  | `FBZCOLOR_PATH`       | Colour combine and source selects. |
| `0xF8108`  | `FOG_MODE`            | Fog pipeline configuration. |
| `0xF810C`  | `ALPHA_MODE`          | Alpha test/blend pipeline. |
| `0xF8110`  | `FBZ_MODE`            | Framebuffer Z mode (depth + clipping + draw enable). |
| `0xF8114`  | `LFB_MODE`            | Linear-framebuffer access mode. |
| `0xF8118`  | `CLIP_LEFT_RIGHT`     | Scissor left/right (10-bit each). |
| `0xF811C`  | `CLIP_LOW_Y_HIGH`     | Scissor top/bottom (10-bit each). |

`FBZ_MODE` (low 21 bits):

| Bit | Field              | Meaning |
|-----|--------------------|---------|
| 0   | `CLIPPING`         | Enable scissor. |
| 1   | `CHROMAKEY`        | Enable chroma keying. |
| 2   | `STIPPLE`          | Enable stipple. |
| 3   | `WBUFFER`          | Use `W` buffer instead of `Z`. |
| 4   | `DEPTH_ENABLE`     | Enable depth test. |
| 5–7 | `DEPTH_FUNC`       | Depth compare function (`0`–`7`). |
| 8   | `DITHER`           | Enable dithering. |
| 9   | `RGB_WRITE`        | Enable RGB writes. |
| 10  | `DEPTH_WRITE`      | Enable depth writes. |
| 11  | `DITHER_2X2`       | `2×2` dither (vs `4×4`). |
| 12  | `ALPHA_WRITE`      | Enable alpha writes. |
| 14  | `DRAW_FRONT`       | Draw to front buffer. |
| 15  | `DRAW_BACK`        | Draw to back buffer. |
| 16  | `DEPTH_SOURCE`     | `0` = iterated, `1` = floating-point. |
| 17  | `Y_ORIGIN`         | `0` = top-left, `1` = bottom-left. |
| 18  | `ALPHA_PLANES`     | Enable alpha planes. |
| 19  | `ALPHA_DITHER`     | Dither alpha. |
| 20–31 | `DEPTH_OFFSET`   | `12`-bit depth bias. |

The depth and alpha functions share the same encoding:

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

`ALPHA_MODE` low bits:

| Bit  | Field         | Meaning |
|------|---------------|---------|
| 0    | `TEST_EN`     | Enable alpha test. |
| 1–3  | `TEST_FUNC`   | Alpha test function. |
| 4    | `BLEND_EN`    | Enable alpha blending. |
| 5    | `ANTIALIAS`   | Enable antialiasing. |
| 8–11 | `SRC_RGB`     | Source RGB blend factor. |
| 12–15| `DST_RGB`     | Dest RGB blend factor. |
| 16–19| `SRC_A`       | Source alpha blend factor. |
| 20–23| `DST_A`       | Dest alpha blend factor. |
| 24–31| `REF`         | Reference value for alpha test. |

The 16 blend-factor codes follow the 3DFX convention (`0` = zero,
`1` = source alpha, `4` = one, `5` = `1 − src.A`, etc.).

### 9.4.5 Fog, chroma, stipple, constants

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF8140`–`0xF823C` | `FOG_TABLE_BASE` (64 entries × 4 bytes) | Per-zone fog blend factors. |
| `0xF81C4`  | `FOG_COLOR`       | Fog RGB. |
| `0xF81C8`  | `ZA_COLOR`        | Z/A constant. |
| `0xF81CC`  | `CHROMA_KEY`      | Chroma key colour. |
| `0xF81D0`  | `CHROMA_RANGE`    | Chroma key tolerance. |
| `0xF81D4`  | `STIPPLE`         | Stipple pattern. |
| `0xF81D8`  | `COLOR0`          | Constant colour 0 (used by `FAST_FILL_CMD`). |
| `0xF81DC`  | `COLOR1`          | Constant colour 1. |

### 9.4.6 Display timing and configuration

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF8200`–`0xF8210` | `FBI_INIT0`–`INIT4` | Framebuffer-interface init. |
| `0xF8214`  | `VIDEO_DIM`       | `width << 16 \| height`. |
| `0xF8218`  | `BACK_PORCH`      | Back-porch timing. |
| `0xF821C`  | `VIDEO_DIM_V`     | Vertical dimensions. |
| `0xF8220`  | `H_SYNC`          | Horizontal sync. |
| `0xF8224`  | `V_SYNC`          | Vertical sync. |
| `0xF822C`  | `DAC_DATA`        | DAC data port. |

### 9.4.7 Texture unit

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF8300`  | `TEXTURE_MODE`    | Texture pipeline configuration. |
| `0xF8304`  | `TLOD`            | LOD control. |
| `0xF8308`  | `TDETAIL`         | Detail texture control. |
| `0xF830C`–`0xF832C` | `TEX_BASE0`–`TEX_BASE8` | Texture base addresses per LOD. |
| `0xF8330`  | `TEX_WIDTH`       | Texture upload width (IE extension). |
| `0xF8334`  | `TEX_HEIGHT`      | Texture upload height (IE extension). |
| `0xF8338`  | `TEX_UPLOAD`      | Write to commit a texture upload (IE extension). |
| `0xF8400`–`0xF87FF` | `PALETTE_BASE` (256 entries) | Texture palette. |

`TEXTURE_MODE` low bits:

| Bit  | Field           | Meaning |
|------|-----------------|---------|
| 0    | `ENABLE`        | Enable texturing. |
| 1–3  | `MINIFY`        | Minification filter. |
| 4    | `MAGNIFY`       | `0` = point, `1` = bilinear. |
| 5    | `CLAMP_S`       | Clamp `S`. |
| 6    | `CLAMP_T`       | Clamp `T`. |
| 8–11 | `FORMAT`        | Texture format (table below). |
| 12   | `CHROMA`        | Texture chroma key. |
| 13   | `TRILINEAR`     | Trilinear filtering. |
| 14   | `PERSPECTIVE`   | Perspective correction. |
| 15   | `DETAIL`        | Detail texturing. |
| 16   | `SEQUENCE`      | Sequence enable. |

Texture formats:

| Code | Format        |
|------|---------------|
| `0`  | 8-bit paletted |
| `1`  | YIQ compressed |
| `2`  | A8            |
| `3`  | I8            |
| `4`  | AI44          |
| `5`  | P8            |
| `6`  | ARGB8332      |
| `7`  | AI88          |
| `8`  | ARGB1555      |
| `9`  | ARGB4444      |
| `10` | ARGB8888      |

## 9.5 Texture memory

Voodoo's texture memory is a separate `64` KB region beginning at
`0xD0000`. Textures are uploaded by writing pixel data into this
region, then setting `TEX_WIDTH`/`TEX_HEIGHT` and writing to
`TEX_UPLOAD` to commit. The chip then samples the texture during
triangle rasterisation according to the `TEXTURE_MODE` and the
`S`/`T`/`W` start values and derivatives.

## 9.6 The colour pipeline

`FBZCOLOR_PATH` selects how the iterated (vertex) colour, the
texture colour, and the constant colour are combined into the
final pixel. The two-source select and the combine mode are packed
into one register:

| Bits  | Field                 | Meaning |
|-------|-----------------------|---------|
| 0–1   | RGB source            | `0` = iterated, `1` = texture, `2` = `COLOR1`, `3` = LFB. |
| 2–3   | Alpha source          | Same encoding as RGB. |
| 4–6   | Combine mode          | One of eight functions (see table below). |
| 27    | `TEXTURE_ENABLE`      | Include texture colour in the path. |

| Combine code | Function       | Meaning |
|--------------|----------------|---------|
| `0`          | `ZERO`         | Output black. |
| `1`          | `CSUB_CL`      | `other − local`. |
| `2`          | `ALOCAL`       | `local × local_alpha`. |
| `3`          | `AOTHER`       | `local × other_alpha`. |
| `4`          | `CLOCAL`       | `local` (pass through). |
| `5`          | `ALOCAL_T`     | `local_alpha × texture`. |
| `6`          | `CLOC_MUL`     | `local × other` (modulate). |
| `7`          | `AOTHER_T`     | `other_alpha × texture`. |

## 9.7 Putting it together

The minimum machine-language sequence to draw one solid-colour
triangle from `(100, 100)` to `(200, 100)` to `(150, 200)` at full
red:

1. Write `1` to `VOODOO_ENABLE`.
2. Write `(640 << 16) | 480` to `VIDEO_DIM`.
3. Write a value into `FBZ_MODE` that enables RGB writes, depth
   writes, and draw to back buffer.
4. Write `0` to `FBZCOLOR_PATH` (iterated colour, no texture).
5. Write `100 << 4` to `VERTEX_AX`, `100 << 4` to `VERTEX_AY`,
   `200 << 4` to `VERTEX_BX`, `100 << 4` to `VERTEX_BY`,
   `150 << 4` to `VERTEX_CX`, `200 << 4` to `VERTEX_CY`.
6. Write `255 << 12` to `START_R`; zero to `START_G`, `START_B`,
   and the eight delta registers.
7. Write any value to `TRIANGLE_CMD`.
8. Write `1` to `SWAP_BUFFER_CMD` to swap with the next VSync.

The next chapter shows how the same chip and its companions can
build tile and sprite layers on top of the other picture sources.
