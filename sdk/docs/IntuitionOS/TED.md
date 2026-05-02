# TED Chip MMIO

The IE TED device exposes Plus/4-inspired audio/video registers as an IE-native MMIO surface. It is a self-contained chip device: TED video RAM is 16 KiB of private VRAM behind `0x0F3000`, not a view into guest system RAM.

## Private `.ted` Player Bus - NOT IE MMIO

These `$FFxx` addresses describe the embedded 6502's private playback bus inside `TEDPlaybackBus6502`, used only for `.ted` file compatibility. They are **not** the IE TED chip's MMIO surface. IE guests writing TED-style registers via the IE bus see the IE-native MMIO described in the Video, Raster, and Audio sections below.

The private bus mirrors the Plus/4 TED interrupt registers closely enough for cRTED, TEDMUSIC, and RealTED players:

| Address | Name | Semantics |
|---|---|---|
| `$FF09` | IRQ flags | Source flags: raster `0x02`, Timer 1 `0x08`, Timer 2 `0x10`, Timer 3 `0x40`. Bit 7 is read-derived summary and is not stored. Write `1` to source bits to acknowledge; writes to bit 7 are ignored. |
| `$FF0A` | IRQ mask / raster compare high | Full byte is preserved as the IRQ mask. Bit `0x01` is also raster compare bit 8, bit `0x02` enables raster IRQ, and bits `0x08`/`0x10`/`0x40` enable Timer 1/2/3 IRQs. |
| `$FF0B` | Raster compare low | Raster compare bits 7..0. Together with `$FF0A` bit 0 this forms a 9-bit compare value. |
| `$FF1C` | Raster low readback | Current raster line bits 7..0 from the private player-bus timing model. |
| `$FF1D` | Raster high/read compatibility | Private playback compatibility readback for raster polling loops. |

Raster compare latches once per frame when the private 6502 cycle stream crosses the programmed 9-bit compare line. PAL valid compare lines are `0..311`; NTSC valid compare lines are `0..261`. Programmed compare values outside the active line range are inert and do not fire at frame wrap.

Timer 1, Timer 2, and Timer 3 use `$FF00/$FF01`, `$FF02/$FF03`, and `$FF04/$FF05` latch/counter pairs. Writing either byte updates the latch, reloads the counter, and starts that timer. Underflow latches the matching `$FF09` source flag and asserts the embedded CPU IRQ only when the corresponding `$FF0A` mask bit is enabled.

RealTED files (`PlayAddr == 0`) run continuously on this private 6502 and re-enter via raster compare IRQs. Subtune selection follows the cRTED/TEDMUSIC convention: Init receives the selected zero-based subtune number in `A`, with `X=0`.

## Video

Video registers live at `0x0F0F20-0x0F0F6B` and are 4-byte aligned for copper access. CPU adapter windows expose the same register indices to 6502 at `$D620-$D632` and to Z80 through ports `0xF2/0xF3`, indices `0x20-0x32`.

`TED_V_CTRL1` provides ECM, BMM, DEN, RSEL, and YSCROLL. `TED_V_CTRL2` provides MCM, CSEL, and XSCROLL. The renderer supports standard text, multicolor text, extended-color text, high-resolution bitmap, and multicolor bitmap modes.

`TED_V_CHAR_BASE` uses bits 4-7 as the charset base in 1 KiB steps and bits 0-3 as the bitmap base in 1 KiB steps. `TED_V_VIDEO_BASE` uses bits 3-7 as the matrix base in 1 KiB steps; color RAM follows the matrix at `matrix + 0x400`. Out-of-range bases fall back to the default layout instead of clamping.

## Raster

`TED_V_RASTER_LO` and `TED_V_RASTER_HI` expose the current visible raster line. `TED_V_RASTER_CMP_LO` and `TED_V_RASTER_CMP_HI` store a 9-bit compare target. `TED_V_RASTER_STATUS` bit 7 is a sticky compare-pending latch; write `0x80` to clear it.

Current v1 timing is frame-coherent and visible-line accurate. Compare values in hidden PAL lines are preserved but do not fire until a future cycle-driven device scheduler and IRQ routing layer is added.

## Audio

TED audio uses the Plus/4 `/8` sound clock divider: `freq_hz = (clock / 8) / (1024 - reg)`. Voice 2 noise routes to the TED 8-bit LFSR mode in the shared SoundChip.
