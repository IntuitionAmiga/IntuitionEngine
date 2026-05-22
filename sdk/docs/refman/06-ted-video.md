---
title: "TED Video"
sources:
  - ted_video_constants.go
  - video_ted.go
  - sdk/include/ehbasic_hw_video.inc
---

# Chapter 6 - TED Video

The TED chip reproduces the picture model of the C16 and Plus/4. It
sits on compositor layer `12`, between VGA (layer `10`) and ANTIC
(layer `13`). TED's design folds video and audio into one chip; this
chapter covers the video half. The audio half is described in
Chapter 16.

## 6.1 What TED can show

| Item                    | Value                                |
|-------------------------|--------------------------------------|
| Display area            | `320` × `200` pixels                 |
| Character grid          | `40` × `25` cells of `8` × `8`       |
| Border                  | `32` left, `32` right, `35` top, `37` bottom |
| Total frame             | `384` × `272` pixels                 |
| Colours                 | `121` unique (16 hues × 8 luminances minus duplicated blacks) |
| Modes                   | Text, bitmap, multicolour bitmap, extended-colour text |

In multicolour mode each pixel is two screen pixels wide, so the
effective resolution drops to `160` × `200` with four colours per
cell instead of two.

## 6.2 BASIC keywords

The BASIC keyword `TED` introduces every TED subcommand. The
recognised forms are:

| Form                         | Effect |
|------------------------------|--------|
| `TED ON`                     | Enable the chip (sets `TED_V_ENABLE` bit `0`). |
| `TED OFF`                    | Disable the chip. |
| `TED MODE `*n*               | `0` = text, `1` = bitmap, `2` = multicolour bitmap. Sets the BMM and MCM bits and forces DEN on. |
| `TED COLOR `*bg*` [, `*border*`]` | Set background colour 0 and (optionally) the border colour. |
| `TED CHAR `*addr*            | Set the character/bitmap base register. |
| `TED VIDEO `*addr*           | Set the video matrix base register. |
| `TED CLS`                    | Fill the video matrix with the space character (`$20`) via the blitter. |
| `TED SCROLL `*dx*`, `*dy*    | Write the X and Y fine-scroll fields of `CTRL2` and `CTRL1` (each `0`–`7`). |

A minimal BASIC program that enables TED, paints the screen yellow
on a blue background, and prints a banner through the video matrix:

```basic
10 TED ON
20 TED COLOR 6, 14         : REM bg=blue luminance 0, border=dark blue
30 TED CLS
40 REM (continue with POKE into the video matrix and colour RAM)
```

## 6.3 The register block

TED's video registers live in the small block `0xF0F20`–`0xF0F6B`.
The chip's audio registers sit at `0xF0F00`–`0xF0F05` (Chapter 16).

| Address    | Name                  | Purpose                          |
|------------|-----------------------|----------------------------------|
| `0xF0F20`  | `TED_V_CTRL1`         | Control 1 (ECM, BMM, DEN, RSEL, YSCROLL). |
| `0xF0F24`  | `TED_V_CTRL2`         | Control 2 (RES, MCM, CSEL, XSCROLL). |
| `0xF0F28`  | `TED_V_CHAR_BASE`     | Character generator base (high nibble) and bitmap base (low nibble), in 1 KB steps. |
| `0xF0F2C`  | `TED_V_VIDEO_BASE`    | Video matrix base, in 1 KB steps (bits `3`–`7`). |
| `0xF0F30`  | `TED_V_BG_COLOR0`     | Background colour 0. |
| `0xF0F34`  | `TED_V_BG_COLOR1`     | Background colour 1 (multicolour). |
| `0xF0F38`  | `TED_V_BG_COLOR2`     | Background colour 2 (multicolour). |
| `0xF0F3C`  | `TED_V_BG_COLOR3`     | Background colour 3 (multicolour). |
| `0xF0F40`  | `TED_V_BORDER`        | Border colour. |
| `0xF0F44`  | `TED_V_CURSOR_HI`     | Cursor position, high byte. |
| `0xF0F48`  | `TED_V_CURSOR_LO`     | Cursor position, low byte. |
| `0xF0F4C`  | `TED_V_CURSOR_CLR`    | Cursor colour. |
| `0xF0F50`  | `TED_V_RASTER_LO`     | Current raster line, low byte (read-only). |
| `0xF0F54`  | `TED_V_RASTER_HI`     | Raster bit `8` in bit `0` (read-only). |
| `0xF0F58`  | `TED_V_ENABLE`        | Bit `0` = video enable. |
| `0xF0F5C`  | `TED_V_STATUS`        | Bit `0` = VBlank active (read-only). |
| `0xF0F60`  | `TED_V_RASTER_CMP_LO` | Raster compare line, low byte. |
| `0xF0F64`  | `TED_V_RASTER_CMP_HI` | Raster compare line bit `8` in bit `0`. |
| `0xF0F68`  | `TED_V_RASTER_STATUS` | Bit `7` = compare pending; write `1` to clear. |

Every register is a 32-bit word at a 4-byte-aligned address; only
the low byte of each is meaningful.

### 6.3.1 CTRL1 bits

| Bit | Name      | Meaning |
|-----|-----------|---------|
| 6   | `ECM`     | Extended colour mode. |
| 5   | `BMM`     | Bitmap mode. |
| 4   | `DEN`     | Display enable. Must be set for any picture. |
| 3   | `RSEL`    | `0` = 24 rows, `1` = 25 rows. |
| 0–2 | `YSCROLL` | Vertical fine-scroll, `0`–`7` raster lines. |

### 6.3.2 CTRL2 bits

| Bit | Name      | Meaning |
|-----|-----------|---------|
| 5   | `RES`     | Reset. |
| 4   | `MCM`     | Multicolour mode. |
| 3   | `CSEL`    | `0` = 38 columns, `1` = 40 columns. |
| 0–2 | `XSCROLL` | Horizontal fine-scroll, `0`–`7` pixels. |

The picture mode is the combination of `BMM`, `MCM`, and `ECM`:

| `BMM` | `MCM` | `ECM` | Picture            |
|-------|-------|-------|--------------------|
| 0     | 0     | 0     | Standard text. |
| 0     | 1     | 0     | Multicolour text. |
| 0     | 0     | 1     | Extended-colour text (8 distinct background colours from the top three bits of the character code). |
| 1     | 0     | 0     | Standard bitmap. |
| 1     | 1     | 0     | Multicolour bitmap (`160` × `200`). |

## 6.4 The colour byte

Every colour register and every byte of colour RAM holds an 8-bit
**colour byte** with this layout:

```
   bit 7 6 5 4 3 2 1 0
       0 L L L H H H H
         |-luminance| |---hue---|
```

`L` is the luminance (`0`–`7`); `H` is the hue (`0`–`15`). The hue
table is:

| Hue | Name        | Hue | Name           |
|-----|-------------|-----|----------------|
| `0` | Black       | `8` | Orange         |
| `1` | White       | `9` | Brown          |
| `2` | Red         | `10`| Yellow-green   |
| `3` | Cyan        | `11`| Pink           |
| `4` | Purple      | `12`| Blue-green     |
| `5` | Green       | `13`| Light blue     |
| `6` | Blue        | `14`| Dark blue      |
| `7` | Yellow      | `15`| Light green    |

Hue `0` (black) is the same colour at every luminance, which is
why TED has `121` unique colours rather than `128` (16 × 8).

## 6.5 The VRAM region

TED owns 16 KB of private VRAM beginning at `0xF3000`. This region
holds three things, all of which can be relocated through the
`TED_V_VIDEO_BASE` and `TED_V_CHAR_BASE` registers:

| Block          | Default offset (from `0xF3000`) | Size      |
|----------------|---------------------------------|-----------|
| Video matrix   | `0x0000`                        | `1024` B  |
| Colour RAM     | `0x0400`                        | `1024` B  |
| Character set  | `0x0800`                        | `2048` B  |

The video matrix is a 40 × 25 array of character codes. The colour
RAM is a 40 × 25 array of colour bytes, one per cell. The character
set is 256 entries of `8` bytes each; bit `7` of every byte is the
leftmost pixel.

`TED_V_VIDEO_BASE` selects the video matrix base in 1 KB steps,
through bits `3`–`7` of its low byte. `TED_V_CHAR_BASE` packs two
selectors into one byte: bits `4`–`7` choose the character set, and
bits `0`–`3` choose the bitmap base when in bitmap mode. Both
selectors are also in 1 KB steps.

## 6.6 The cursor

The cursor is a single character cell. Its position is the 11-bit
linear offset stored in `TED_V_CURSOR_HI`/`LO` (`row * 40 + col`),
and its colour is in `TED_V_CURSOR_CLR`. The cursor blinks
automatically at one cycle every `30` frames.

## 6.7 Raster interrupts

`TED_V_RASTER_LO` and bit `0` of `TED_V_RASTER_HI` report the
current scan-line as a 9-bit value (`0`–`311` over the full PAL
frame). Writing the same 9-bit value into the corresponding
compare registers arms the raster compare; when the raster reaches
the line, `TED_V_RASTER_STATUS` bit `7` is set and the chip raises
its interrupt line. The CPU clears the latch by writing `1` to
bit `7` of `TED_V_RASTER_STATUS`.

Raster interrupts are how the C16 and Plus/4 split the screen into
multiple regions with different scroll, colour, or character-set
choices. The same trick works here.

## 6.8 Putting it together

The shortest TED program from machine language:

1. Write a non-zero value to `TED_V_ENABLE` (or use `TED ON`).
2. Set the picture mode in `TED_V_CTRL1` and `TED_V_CTRL2` (or use
   `TED MODE`).
3. Choose where the video matrix and character set live with
   `TED_V_VIDEO_BASE` and `TED_V_CHAR_BASE`.
4. Fill the video matrix and colour RAM in the regions you chose.
5. To animate, poll `TED_V_STATUS` bit `0` for the vertical
   retrace, or arm a raster compare.

The next chapter covers ANTIC and GTIA, the Atari 8-bit picture
chips, which live on layer `13`.
