---
title: "Per-Engine MMIO Maps"
sources:
  - registers.go
  - video_chip.go
  - audio_chip.go
  - psg_constants.go
  - sn76489_constants.go
  - sid_constants.go
  - pokey_constants.go
  - ted_constants.go
  - ted_video_constants.go
  - vga_constants.go
  - ula_constants.go
  - antic_constants.go
  - voodoo_constants.go
  - mod_constants.go
  - wav_constants.go
  - sfx_constants.go
  - file_io_constants.go
  - coprocessor_constants.go
---

# Appendix D - Per-Engine MMIO Maps

Every memory-mapped device, in address order. Each register is
`32`-bit on the bus unless noted; only the documented low bits
carry information.

## D.1 Video chip (`0xF0000`-`0xF049B`)

| Address    | Name              | R/W | Notes |
|------------|-------------------|-----|-------|
| `0xF0000`  | `VIDEO_CTRL`      | R/W | Enable, mode select, sync source. |
| `0xF0004`  | `VIDEO_MODE`      | R/W | Mode encoding: 320x200, 320x240, 640x480, 800x600, 1024x768, 1280x960, 1920x1080. |
| `0xF0008`  | `VIDEO_STATUS`    | R   | VBlank flag, HBlank flag, raster line. |
| `0xF000C`  | `VIDEO_FB_BASE`   | R/W | Framebuffer base address. |
| `0xF0010`  | `COPPER_CTRL`     | R/W | Copper enable. |
| `0xF0014`  | `COPPER_PTR`      | R/W | Copper-list base address. |
| `0xF0018`  | `COPPER_PC`       | R   | Current copper program counter. |
| `0xF001C`  | `COPPER_STATUS`   | R   | Copper running / stopped. |
| `0xF0020`-`0xF0044` | `BLT_*`  | R/W | Blitter source A/B/C/D, mask, modulo, control. |
| `0xF0048`-`0xF0054` | `VIDEO_RASTER_*` | R/W | Raster split lines and per-line palette swaps. |
| `0xF0078`-`0xF0477` | palette  | R/W | 256-entry 32-bit RGBA palette. |
| `0xF0488`-`0xF049B` | `BLT_EXT_*` | R/W | Extended blitter (large modes). |

## D.2 Terminal / serial / input (`0xF0700`-`0xF07FF`)

| Address    | Name              | R/W | Notes |
|------------|-------------------|-----|-------|
| `0xF0700`  | `TERM_OUT`        | W   | Print one byte. |
| `0xF0704`  | `TERM_STATUS`     | R   | Bit `0` input avail, bit `1` output ready. |
| `0xF0708`  | `TERM_IN`         | R   | Dequeue cooked input byte. |
| `0xF070C`  | `TERM_LINE_STATUS`| R   | Bit `0` complete line in queue. |
| `0xF0710`  | `TERM_ECHO`       | R/W | Local echo. |
| `0xF0724`  | `TERM_CTRL`       | R/W | Bit `0` line-input mode. |
| `0xF0728`  | `TERM_KEY_IN`     | R   | Dequeue raw key. |
| `0xF072C`  | `TERM_KEY_STATUS` | R   | Bit `0` raw key avail. |
| `0xF0730`  | `MOUSE_X`         | R   | Absolute X (low `16` bits). |
| `0xF0734`  | `MOUSE_Y`         | R   | Absolute Y. |
| `0xF0738`  | `MOUSE_BUTTONS`   | R   | Bit `0` L, `1` R, `2` M. |
| `0xF073C`  | `MOUSE_STATUS`    | R   | Bit `0` changed (clears on read). |
| `0xF0740`  | `SCAN_CODE`       | R   | Dequeue raw scancode. |
| `0xF0744`  | `SCAN_STATUS`     | R   | Bit `0` scancode avail. |
| `0xF0748`  | `SCAN_MODIFIERS`  | R   | Bit `0` shift, `1` ctrl, `2` alt, `3` caps. |
| `0xF074C`  | `MOUSE_CTRL`      | R/W | Bit `0` request relative mode. |
| `0xF0750`  | `RTC_EPOCH`       | R   | Unix epoch seconds. |
| `0xF0754`  | `MOUSE_DX`        | R   | Signed accumulated dX (clears on read). |
| `0xF0758`  | `MOUSE_DY`        | R   | Signed accumulated dY. |
| `0xF07F0`  | `TERM_SENTINEL`   | W   | Write `0xDEAD` to stop CPU. |

## D.3 SoundChip (`0xF0800`-`0xF0B7F`)

| Range              | Block | Notes |
|--------------------|-------|-------|
| `0xF0800`-`0xF08FF`| Global | `AUDIO_CTRL`, `ENV_SHAPE`, filter, reverb, overdrive. |
| `0xF0900`-`0xF093F`| Square | `SQ_FREQ`, `SQ_DUTY`, `SQ_ENV_*`, `SQ_GATE`. |
| `0xF0940`-`0xF097F`| Triangle | Same shape. |
| `0xF0980`-`0xF09BF`| Sine | Same shape. |
| `0xF09C0`-`0xF09FF`| Noise | LFSR period, volume, gate. |
| `0xF0A00`-`0xF0A1F`| Sync / ring-mod | Source-channel selects. |
| `0xF0A20`-`0xF0A6F`| Sawtooth | Same shape. |
| `0xF0A80`-`0xF0B7F`| Flex 0-3 | 64-byte per-channel block. |

## D.4 SFX triggers (`0xF0E80`-`0xF0EFF`)

| Per-channel offset | Field | Notes |
|---|---|---|
| `+0x00` | `SFX_PTR` | Sample base address. |
| `+0x04` | `SFX_LEN` | Sample length in bytes. |
| `+0x08` | `SFX_LOOP_PTR` | Loop point. |
| `+0x0C` | `SFX_LOOP_LEN` | Loop length. |
| `+0x10` | `SFX_FREQ` | Playback rate. |
| `+0x14` | `SFX_VOL` | Volume (`0`-`64`). |
| `+0x18` | `SFX_FORMAT` | Sample format. |
| `+0x1C` | `SFX_CTRL` | Trigger / one-shot / loop. |

Four channels at `0xF0E80`, `0xF0EA0`, `0xF0EC0`, `0xF0EE0`.

## D.5 PSG / AY-3-8910 (`0xF0C00`-`0xF0C20`)

| Offset | Register |
|--------|----------|
| `+0x00`-`+0x0F` | R0-R15, one byte per register (byte-contiguous). |
| `+0x10` | `PSG_PLAY_PTR` (32-bit). |
| `+0x14` | `PSG_PLAY_LEN` (32-bit). |
| `+0x18` | `PSG_PLAY_CTRL` (bit `0` start, bit `1` stop, bit `2` loop). |
| `+0x1C` | `PSG_PLAY_STATUS` (bit `0` busy, bit `1` error). |
| `+0x20` | `PSG_PLUS_CTRL`. |

## D.6 SN76489 (`0xF0C30`-`0xF0C37`)

Documented in full in Chapter 14.

## D.7 SID family

Each SID instance is a byte-contiguous block in the original
6581/8580 register layout. Three independent instances live at
three bases:

| Block | Range                | Notes |
|-------|----------------------|-------|
| SID   | `0xF0E00`-`0xF0E1C`  | Primary SID. |
| SID2  | `0xF0E30`-`0xF0E4C`  | Second independent SID. |
| SID3  | `0xF0E50`-`0xF0E6C`  | Third independent SID. |

Within each instance the offsets follow the original chip layout,
one byte per register:

| Offset | Register                              |
|--------|---------------------------------------|
| `+0x00`-`+0x06` | Voice 1: `FREQ_LO`, `FREQ_HI`, `PW_LO`, `PW_HI`, `CTRL`, `AD`, `SR`. |
| `+0x07`-`+0x0D` | Voice 2 (same shape). |
| `+0x0E`-`+0x14` | Voice 3 (same shape). |
| `+0x15`-`+0x17` | Filter: `FC_LO`, `FC_HI`, `RES_FILT`. |
| `+0x18` | `MODE_VOL`. |
| `+0x19` | SID+ control (`SID_PLUS_CTRL`) / `POT_X` mirror. |
| `+0x1A` | `POT_Y`. |
| `+0x1B` | `OSC3`. |
| `+0x1C` | `ENV3`. |

The primary SID block at `0xF0E00` has an attached `.sid` file
player block in the same `32`-byte window:

| Offset (from `0xF0E00`) | Register |
|-------|----------|
| `+0x20` | `SID_PLAY_PTR` (32-bit). |
| `+0x24` | `SID_PLAY_LEN` (32-bit). |
| `+0x28` | `SID_PLAY_CTRL` (start / stop / loop). |
| `+0x2C` | `SID_PLAY_STATUS` (busy / error). |
| `+0x2D` | `SID_SUBSONG`. |

The two flexible audio blocks `0xF0C40`-`0xF0CFF` and
`0xF0D40`-`0xF0DFF` are not SID2 and SID3. They are
extra-channel mirrors (channels 4-6 and 7-9 in the global
SoundChip channel space) and have the flex-channel layout
documented under SoundChip (D.3), not the SID layout. A program
that wants SID2 / SID3 should program them at `0xF0E30` and
`0xF0E50`.

## D.8 POKEY (`0xF0D00`-`0xF0D20`)

| Offset | Register |
|--------|----------|
| `+0x00` | `POKEY_AUDF1`. |
| `+0x04` | `POKEY_AUDC1`. |
| `+0x08`-`+0x1C` | `AUDF2`-`AUDF4`, `AUDC2`-`AUDC4`. |
| `+0x20` | `POKEY_AUDCTL`. |
| `+0x24`-`+0x2F` | `SAP_PLAY_*` player. |

## D.9 TED audio (`0xF0F00`-`0xF0F1F`)

| Offset | Register |
|--------|----------|
| `+0x00` | `TED_FREQ1_LO`. |
| `+0x04` | `TED_FREQ1_HI`. |
| `+0x08` | `TED_FREQ2_LO`. |
| `+0x0C` | `TED_FREQ2_HI`. |
| `+0x10` | `TED_SND_CTRL`. |
| `+0x14`-`+0x1F` | `TED_PLAY_*` player. |

## D.10 TED video (`0xF0F20`-`0xF0F5F`)

40x25 text with 121-colour cells; see Chapter 6.

## D.11 VGA (`0xF1000`-`0xF13FF`)

| Range | Subsystem |
|-------|-----------|
| `0xF1000`-`0xF103F` | `VGA_MODE`, `VGA_STATUS`, `VGA_CTRL`. |
| `0xF1040`-`0xF107F` | Sequencer (`VGA_SEQ_*`). |
| `0xF1080`-`0xF10FF` | CRTC (`VGA_CRTC_*`). |
| `0xF1100`-`0xF113F` | Graphics controller (`VGA_GC_*`). |
| `0xF1140`-`0xF117F` | Attribute controller (`VGA_ATTR_*`). |
| `0xF1180`-`0xF11FF` | DAC + palette index/data. |
| `0xF1200`-`0xF13FF` | Palette RAM. |

## D.12 ULA (`0xF2000`-`0xF2017`)

| Offset | Register |
|--------|----------|
| `+0x00` | `ULA_BORDER` (bits `0`-`2`). |
| `+0x04` | `ULA_CTRL`. |
| `+0x08` | `ULA_STATUS`. |
| `+0x0C` | `ULA_ADDR_LO`. |
| `+0x10` | `ULA_ADDR_HI`. |
| `+0x14` | `ULA_DATA`. |

ULA VRAM aperture at `0xFA000`-`0xFBAFF`.

## D.13 ANTIC + GTIA (`0xF2100`-`0xF21B7`)

ANTIC block at `0xF2100`-`0xF213F` with `DMACTL`, `CHACTL`,
`DLISTL/H`, `HSCROL`, `VSCROL`, `PMBASE`, `CHBASE`, `WSYNC`,
`VCOUNT`, `NMIEN/IST/RES`. GTIA at `0xF2140`-`0xF21B7` with
`COLPM0`-`3`, `COLPF0`-`3`, `COLBK`, `PRIOR`, `GRACTL`, `CONSOL`.

## D.14 File I/O (`0xF2200`-`0xF221F`)

| Offset | Register |
|--------|----------|
| `+0x00` | `FILE_NAME_PTR`. |
| `+0x04` | `FILE_DATA_PTR`. |
| `+0x08` | `FILE_DATA_LEN`. |
| `+0x0C` | `FILE_CTRL` (write triggers). |
| `+0x10` | `FILE_STATUS`. |
| `+0x14` | `FILE_RESULT_LEN`. |
| `+0x18` | `FILE_ERROR_CODE`. |

## D.15 Amiga Paula DMA (`0xF2260`-`0xF22AF`)

Four-channel DMA sample engine. Per-channel address, length,
period, volume, and control registers. Documented in Chapter 21.

## D.16 Media loader (`0xF2300`-`0xF231F`)

Dispatches by file extension into the matching audio engine. Used
by `SOUND PLAY` (Chapter 22).

## D.17 Image executor (`0xF2320`-`0xF233F`)

The `RUN` keyword writes the image filename here and triggers the
load. The block has filename pointer, control, status, and result
fields shaped like the File I/O block.

## D.18 Coprocessor (`0xF2340`-`0xF238F`)

`COSTART`, `COSTOP`, `COCALL`, `COSTATUS`, `COWAIT`, plus the
ring-buffer base/head/tail registers. Six worker types
(Chapter 31).

## D.19 IRQ diagnostics (`0xF23C0`-`0xF23DF`)

| Offset | Register |
|--------|----------|
| `+0x00` | In-service mask. |
| `+0x04` | State flags. |
| `+0x08` | Pending mask. |
| `+0x0C` | Delivered counters. |
| `+0x10` | Blocked counters. |
| `+0x14` | RTE count. |
| `+0x18` | Consecutive STOP spins. |
| `+0x1C` | Watchdog event count. |

## D.20 SysInfo (`0xF2400`-`0xF24FF`)

| Offset | Register |
|--------|----------|
| `+0x00` | Total RAM, low 32 bits. |
| `+0x04` | Total RAM, high 32 bits. |
| `+0x08` | Active RAM, low. |
| `+0x0C` | Active RAM, high. |

## D.21 HOST appliance block (`0xF1400`-`0xF140F`)

| Offset | Name     | Width  | Notes |
|--------|----------|--------|-------|
| `+0x00` | command  | byte   | Subverb enum (W). |
| `+0x04` | trigger  | byte   | Non-zero fires the command (W). |
| `+0x08` | status   | byte   | Terminal-state enum (R). |
| `+0x0C` | exit     | 32-bit | Full `32`-bit exit code from the underlying action (R); byte lanes at `+0x0C`-`+0x0F` return successive bytes of the same value. |

See Chapter 35 for the subverb enum and the state machine.
Reachable from IE64, IE32, M68K, and x86; not reachable from the
6502 or Z80.

## D.22 Voodoo 3D (`0xF8000`-`0xF87FF`)

Status, framebuffer base, clip rect, triangle setup, texture
descriptors, fog, alpha, chroma-key, Z-buffer. Documented in
Chapter 9. Texture RAM at `0xD0000`-`0xDFFFF`.
