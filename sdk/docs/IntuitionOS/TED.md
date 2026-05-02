# TED Chip MMIO

The IE TED device exposes Plus/4-inspired audio/video registers as an IE-native MMIO surface. It is a self-contained chip device: TED video RAM is 16 KiB of private VRAM behind `0x0F3000`, not a view into guest system RAM.

## Video

Video registers live at `0x0F0F20-0x0F0F6B` and are 4-byte aligned for copper access. CPU adapter windows expose the same register indices to 6502 at `$D620-$D632` and to Z80 through ports `0xF2/0xF3`, indices `0x20-0x32`.

`TED_V_CTRL1` provides ECM, BMM, DEN, RSEL, and YSCROLL. `TED_V_CTRL2` provides MCM, CSEL, and XSCROLL. The renderer supports standard text, multicolor text, extended-color text, high-resolution bitmap, and multicolor bitmap modes.

`TED_V_CHAR_BASE` uses bits 4-7 as the charset base in 1 KiB steps and bits 0-3 as the bitmap base in 1 KiB steps. `TED_V_VIDEO_BASE` uses bits 3-7 as the matrix base in 1 KiB steps; color RAM follows the matrix at `matrix + 0x400`. Out-of-range bases fall back to the default layout instead of clamping.

## Raster

`TED_V_RASTER_LO` and `TED_V_RASTER_HI` expose the current visible raster line. `TED_V_RASTER_CMP_LO` and `TED_V_RASTER_CMP_HI` store a 9-bit compare target. `TED_V_RASTER_STATUS` bit 7 is a sticky compare-pending latch; write `0x80` to clear it.

Current v1 timing is frame-coherent and visible-line accurate. Compare values in hidden PAL lines are preserved but do not fire until a future cycle-driven device scheduler and IRQ routing layer is added.

## Audio

TED audio uses the Plus/4 `/8` sound clock divider: `freq_hz = (clock / 8) / (1024 - reg)`. Voice 2 noise routes to the TED 8-bit LFSR mode in the shared SoundChip.
