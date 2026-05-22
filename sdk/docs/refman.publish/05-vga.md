
# Chapter 5 - The VGA Controller

The VGA is the second of Intuition Engine's six picture sources. It
sits on compositor layer `10`, so it draws on top of the VideoChip
(layer `0`) and below every other source. Of all six chips, VGA is
the easiest to drive from BASIC: most of the BASIC graphics
keywords - `SCREEN`, `CLS`, `PLOT`, `PALETTE`, `COLOR`, `LOCATE`,
`SCROLL`, `VSYNC` - operate on the VGA.

This chapter describes what VGA can show, where its memory lives,
and how its registers work. The BASIC keywords are listed first
because most readers will use them long before they touch the
registers directly.

## 5.1 What VGA can show

VGA has four modes. You select one by writing its mode number to
the `VGA_MODE` register, or with the BASIC `SCREEN` statement.

| Mode value | Name             | Resolution | Colours | Memory model |
|------------|------------------|------------|---------|--------------|
| `&H03`     | Text             | `80` × `25` characters (`640` × `400` pixels) | 16 fg, 8 bg | Character/attribute pairs in text buffer. |
| `&H12`     | Mode 12h         | `640` × `480` pixels | 16        | Four bit-planes. |
| `&H13`     | Mode 13h         | `320` × `200` pixels | 256       | Linear (chain-4). |
| `&H14`     | Mode X           | `320` × `240` pixels | 256       | Four planes, unchained. |

The mode numbers and memory layouts match the conventions of the
IBM PC VGA, so existing PC programming literature is helpful - but
the addresses are IE addresses, given in the table below.

## 5.2 BASIC keywords

Each of the keywords below has a one-line summary here and a full
entry in Chapter 2.

| Keyword                  | Effect |
|--------------------------|--------|
| `SCREEN n`               | Set VGA mode `n`. Enables VGA. |
| `SCREEN ON` / `SCREEN OFF` | Enable / disable VGA. |
| `CLS [colour]`           | Clear the screen using the blitter. |
| `PLOT x, y [, colour]`   | Plot a single pixel. Mode 13h only. Default colour is `15`. |
| `PALETTE i, r, g, b`     | Set DAC palette entry `i` to (`r`, `g`, `b`). Each component is `0`–`63`. |
| `COLOR fg [, bg]`        | Set the text-mode attribute byte used by `PRINT`. `fg` is `0`–`15`; `bg` is `0`–`15`. |
| `LOCATE row, col`        | Place the text cursor at zero-based `(row, col)`. |
| `SCROLL dx, dy`          | Shift the displayed start address by `dy` rows and `dx` characters. |
| `VSYNC`                  | Wait for the vertical retrace by polling `VGA_STATUS`. |

A minimal BASIC program that puts up Mode 13h and draws a single
red pixel at the centre of the screen:

```basic
10 SCREEN &H13
20 PALETTE 1, 63, 0, 0
30 PLOT 160, 100, 1
```

## 5.3 The register block

The VGA register block runs from `0xF1000` to `0xF13FF`. All
registers are 32-bit words at 4-byte-aligned addresses. The block
is laid out as a set of small index/data pairs, each one mirroring
a section of the IBM VGA register file.

### 5.3.1 Core control

| Address    | Name        | Purpose |
|------------|-------------|---------|
| `0xF1000`  | `VGA_MODE`  | Mode select. Writing `&H03`, `&H12`, `&H13`, or `&H14` chooses a mode. |
| `0xF1004`  | `VGA_STATUS`| Status (read-only). |
| `0xF1008`  | `VGA_CTRL`  | Master enable. Bit `0` = on. |

`VGA_STATUS` bits:

| Bit | Name      | Meaning |
|-----|-----------|---------|
| 0   | `VSYNC`   | Vertical sync active. |
| 1   | `HSYNC`   | Horizontal sync (approximate). |
| 3   | `RETRACE` | Vertical retrace active. |

### 5.3.2 Sequencer

The sequencer governs how CPU writes are routed to the four bit
planes. Use the index/data pair to read or write one of five
sequencer registers; bit-plane mask has its own direct port.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF1010`  | `VGA_SEQ_INDEX`   | Index into the sequencer register file (`0`–`4`). |
| `0xF1014`  | `VGA_SEQ_DATA`    | Read/write the register selected by `VGA_SEQ_INDEX`. |
| `0xF1018`  | `VGA_SEQ_MAPMASK` | Direct port for the map-mask register (plane write enable, bits `0`–`3`). |

### 5.3.3 CRT controller (CRTC)

The CRTC owns the cursor position, the display start address (for
hardware scrolling), and the timing parameters. There are 25 CRTC
registers; the most useful ones have direct ports.

| Address    | Name                  | Purpose |
|------------|-----------------------|---------|
| `0xF1020`  | `VGA_CRTC_INDEX`      | Index (`0`–`24`). |
| `0xF1024`  | `VGA_CRTC_DATA`       | Read/write the indexed register. |
| `0xF1028`  | `VGA_CRTC_STARTHI`    | High byte of display start address (page flip). |
| `0xF102C`  | `VGA_CRTC_STARTLO`    | Low byte of display start address. |

Cursor position is held in CRTC indices `0x0E` (high) and `0x0F`
(low). The cursor scan lines are indices `0x0A` (start) and `0x0B`
(end). The `LOCATE` BASIC statement writes the cursor low and high
bytes through `VGA_CRTC_INDEX`/`DATA`.

### 5.3.4 Graphics controller

The graphics controller decides how CPU bytes are combined with the
existing VRAM bytes on a planar write.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF1030`  | `VGA_GC_INDEX`    | Index (`0`–`8`). |
| `0xF1034`  | `VGA_GC_DATA`     | Read/write the indexed register. |
| `0xF1038`  | `VGA_GC_READMAP`  | Direct port for the read-map select (plane read selector). |
| `0xF103C`  | `VGA_GC_BITMASK`  | Direct port for the bit-mask register. |

### 5.3.5 Attribute controller

Holds the 16 colour-attribute palette entries and the text-mode
attribute control bits.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF1040`  | `VGA_ATTR_INDEX`  | Index (`0`–`20`). |
| `0xF1044`  | `VGA_ATTR_DATA`   | Read the indexed register. |

### 5.3.6 DAC and palette

The DAC stores 256 RGB triples, six bits per component. When the
chip displays an indexed pixel, it looks up the triple, expands
each component from six to eight bits (`(v << 2) | (v >> 4)`), and
sends the result to the compositor.

| Address    | Name              | Purpose |
|------------|-------------------|---------|
| `0xF1050`  | `VGA_DAC_MASK`    | Pixel mask applied before lookup. |
| `0xF1054`  | `VGA_DAC_RINDEX`  | Read index. |
| `0xF1058`  | `VGA_DAC_WINDEX`  | Write index. |
| `0xF105C`  | `VGA_DAC_DATA`    | DAC data port. Reads/writes one component per access in R, G, B order. |
| `0xF1100`–`0xF13FF` | `VGA_PALETTE`/`VGA_PALETTE_END` | Direct window into 256 × 3 bytes of palette RAM. |

The DAC ports are a small state machine. Write the entry number to
`VGA_DAC_WINDEX`, then write `R`, `G`, `B` in succession to
`VGA_DAC_DATA`. The chip increments the write index after the
third byte. The `PALETTE` BASIC keyword uses this sequence.

```basic
100 POKE &H000F1058, 1                  : REM WINDEX = 1
110 POKE &H000F105C, 63                 : REM red
120 POKE &H000F105C, 0                  : REM green
130 POKE &H000F105C, 0                  : REM blue
```

The same entry can also be written directly through the palette
window:

```basic
200 POKE &H000F1100 + 1*3,     63       : REM red
210 POKE &H000F1100 + 1*3 + 1, 0        : REM green
220 POKE &H000F1100 + 1*3 + 2, 0        : REM blue
```

Each component is clamped to six bits on write.

## 5.4 The VRAM regions

VGA uses two separate regions of main memory:

| Address       | Size     | Used by |
|---------------|----------|---------|
| `0xA0000`     | `64` KB  | Graphics modes (`12h`, `13h`, Mode X). |
| `0xB8000`     | `32` KB  | Text mode (`03h`). |

These are the same conventions as the IBM PC. The graphics region
is divided into four 64 KB planes for planar modes; in linear modes
(Mode 13h) the chip applies chain-4 to make consecutive bytes go
to consecutive planes, giving an illusion of a flat 64000-byte
buffer.

In text mode the buffer holds pairs of bytes: character code, then
attribute. The attribute's low nibble is the foreground colour
(`0`–`15`); the next three bits are the background colour (`0`–`7`);
the top bit, if set, makes the character blink (depending on the
attribute-mode setting).

A bare `PRINT "HELLO"` writes characters into this buffer at the
current cursor position using the attribute set by the most recent
`COLOR` statement.

## 5.5 Hardware scrolling and page flipping

The pair of registers `VGA_CRTC_STARTHI` and `VGA_CRTC_STARTLO`
holds the byte offset within VRAM at which the displayed picture
begins. By changing this offset between frames you can:

- **Page flip.** Render to one half of VRAM while displaying the
  other, then swap by writing the new start address. The change
  takes effect on the next frame.
- **Scroll.** Shift the start address by one row's worth of bytes
  to scroll vertically; by one byte (Mode 13h) or one character
  (text mode) to scroll horizontally. The `SCROLL` BASIC keyword
  uses this in text mode.

The CRTC also exposes a line-compare register (index `0x18`) that
splits the screen into two scrolling regions; this is the classic
"status bar at the bottom" trick.

## 5.6 The vertical retrace

`VGA_STATUS` bit `0` is set whenever the chip is in the vertical
retrace interval. The cleanest way to update VRAM without tearing
is to poll this bit in a tight loop until it becomes set, then do
your writes. The `VSYNC` BASIC keyword does the polling for you,
with a one-million-iteration timeout so that a misconfigured chip
cannot lock up the program:

```basic
10 VSYNC
20 REM update VRAM here
```

## 5.7 Putting it together

The shortest road to a picture on VGA is:

1. Pick a mode and write it to `VGA_MODE` (or use `SCREEN`).
2. Set `VGA_CTRL` = `1` to enable the chip (`SCREEN` does this for
   you).
3. Load any palette entries you need through the DAC ports or the
   palette window (`PALETTE`).
4. Write pixels or characters into the appropriate VRAM window.
5. To animate, poll `VGA_STATUS` for the retrace or call `VSYNC`
   before each frame.

The next chapter covers the TED video chip, which produces the
C16/Plus-4 picture model and lives on layer `12`.
