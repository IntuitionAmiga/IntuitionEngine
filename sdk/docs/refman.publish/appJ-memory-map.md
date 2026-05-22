
# Appendix J - Full Memory Map

Every region of the Intuition Engine address space, in address
order, with its size and purpose. The map is identical from the
IE64, IE32, M68K, and x86 sides; the 6502 and Z80 see windowed
views into it (Chapters 26 and 27).

## J.1 Low RAM and stacks

| Range                    | Size  | Purpose |
|--------------------------|-------|---------|
| `0x000000`-`0x0003FF`    | 1 KB  | M68K vector table; x86 IVT; trap vector base for IE64. |
| `0x000400`-`0x022FFF`    | ~140 KB | BASIC ROM image, system call shim, monitor stub. |
| `0x023000`-`0x04FFFF`    | 180 KB | BASIC program text (`BASIC_PROG_START`-). |
| `0x050000`-`0x057FFF`    | 32 KB | BASIC simple variables. |
| `0x058000`-`0x05FFFF`    | 32 KB | BASIC string variables. |
| `0x060000`-`0x08BFFF`    | 176 KB | BASIC arrays. |
| `0x08C000`-`0x08FFFF`    | 16 KB | BASIC string temporaries. |
| `0x090000`-`0x096FFF`    | 28 KB | BASIC `GOSUB` / `FOR` stack. |
| `0x097000`-`0x09EFFF`    | 32 KB | Free RAM (general user). |
| `0x09F000`-`0x09FFFF`    | 4 KB | IE32 stack (`STACK_START`). |

## J.2 PC-compatible VRAM apertures

| Range                  | Size  | Purpose |
|------------------------|-------|---------|
| `0x0A0000`-`0x0AFFFF`  | 64 KB | VGA graphics window (mode 13h, mode 12h linear). |
| `0x0B0000`-`0x0B7FFF`  | 32 KB | Reserved (PC-compatible monochrome text window; not implemented). |
| `0x0B8000`-`0x0BFFFF`  | 32 KB | VGA text buffer (`0xB8000`). |

## J.3 General RAM gap

| Range                  | Size  | Purpose |
|------------------------|-------|---------|
| `0x0C0000`-`0x0CFFFF`  | 64 KB | Free RAM (general user). |
| `0x0D0000`-`0x0DFFFF`  | 64 KB | Voodoo texture RAM. |
| `0x0E0000`-`0x0EFFFF`  | 64 KB | Free RAM (general user). |

## J.4 The MMIO region (`0xF0000`-`0xFFFFF`)

| Range                     | Size   | Device |
|---------------------------|--------|--------|
| `0xF0000`-`0xF049B`       | 1180 B | Video chip + palette + extended blitter. |
| `0xF0700`-`0xF07FF`       | 256 B  | Terminal / serial / input. |
| `0xF0800`-`0xF0B7F`       | 896 B  | SoundChip. |
| `0xF0BC0`-`0xF0BD7`       | 24 B   | MOD player. |
| `0xF0BD8`-`0xF0BF3`       | 28 B   | WAV player. |
| `0xF0C00`-`0xF0C20`       | 33 B   | PSG / AY. |
| `0xF0C30`-`0xF0C37`       | 8 B    | SN76489. |
| `0xF0C40`-`0xF0CFF`       | 192 B  | SID2 flex block. |
| `0xF0D00`-`0xF0D20`       | 33 B   | POKEY. |
| `0xF0D40`-`0xF0DFF`       | 192 B  | SID3 flex block. |
| `0xF0E00`-`0xF0E2D`       | 46 B   | SID. |
| `0xF0E80`-`0xF0EFF`       | 128 B  | SFX triggers. |
| `0xF0F00`-`0xF0F5F`       | 96 B   | TED audio + video. |
| `0xF1000`-`0xF13FF`       | 1 KB   | VGA registers. |
| `0xF1400`-`0xF140F`       | 16 B   | HOST appliance block. |
| `0xF2000`-`0xF2017`       | 24 B   | ULA registers. |
| `0xF2100`-`0xF213F`       | 64 B   | ANTIC. |
| `0xF2140`-`0xF21B7`       | 120 B  | GTIA. |
| `0xF2200`-`0xF221F`       | 32 B   | File I/O. |
| `0xF2260`-`0xF22AF`       | 80 B   | Amiga Paula DMA. |
| `0xF2300`-`0xF231F`       | 32 B   | Media loader. |
| `0xF2320`-`0xF233F`       | 32 B   | Image executor. |
| `0xF2340`-`0xF238F`       | 80 B   | Coprocessor. |
| `0xF2390`-`0xF23AF`       | 32 B   | Clipboard bridge. |
| `0xF23B0`-`0xF23BF`       | 16 B   | Coprocessor extended monitor. |
| `0xF23C0`-`0xF23DF`       | 32 B   | IRQ diagnostics. |
| `0xF23E0`-`0xF23FF`       | 32 B   | Bootstrap loader. |
| `0xF2400`-`0xF24FF`       | 256 B  | SysInfo (RAM-size ABI). |
| `0xF8000`-`0xF87FF`       | 2 KB   | Voodoo 3D registers. |
| `0xFA000`-`0xFBAFF`       | 6912 B | ULA VRAM aperture. |

## J.5 Main video RAM

| Range                       | Size  | Purpose |
|-----------------------------|-------|---------|
| `0x100000`-`0x5FFFFF`       | 5 MB  | Main VRAM aperture for VideoChip framebuffers; large modes may point `VIDEO_FB_BASE` into ordinary RAM. |

## J.6 High RAM and reserved

| Range                       | Size   | Purpose |
|-----------------------------|--------|---------|
| `0x600000`-`0xEFFFFF`       | 9 MB   | Free RAM (general user / coprocessor heap). |
| `0xF00000`-`0xFEFFFF`       | 960 KB | Reserved (high-MMIO mirror window). |
| `0xFF0000`-`0xFFFFFF`       | 64 KB  | Reserved (M68K sign-extended low-word aliases). |

## J.7 The 6502 / Z80 views

The 6502 sees a 16-bit address space `0x0000`-`0xFFFF`. Its
adapter routes the high page-window region as follows:

| 6502 range          | Maps to (32-bit) |
|---------------------|------------------|
| `$0000`-`$00FF`     | zero page in main RAM. |
| `$0100`-`$01FF`     | 6502 stack page in main RAM. |
| `$0200`-`$CFFF`     | main RAM. |
| `$D000`-`$D7FF`     | per-chip 6502-style apertures (PSG, SID, POKEY, TED, VGA, ULA). |
| `$D800`-`$DFFF`     | ULA paged VRAM data port. |
| `$E000`-`$EFFF`     | bank-selectable window into main RAM. |
| `$F000`-`$FFF9`     | MMIO mirror of `0xF0000`-`0xF0FF9`, with `$F700`-`$F705` and `$F7F0` intercepted as bank registers. |
| `$FFFA`-`$FFFF`     | NMI / reset / IRQ vectors. |

The Z80 sees the same shape with port I/O substituted for the
chip-aperture region; the bank-window scheme is identical.

## J.8 Notes

The map above is the engineering view: every byte of the 24-bit
bus that has a defined meaning. Most user programs use a small
sub-set of it - the BASIC variable area, the I/O region, and the
main video RAM - and never touch the rest. Programs that compete
for memory (a large in-memory dataset, a custom blitter routine,
a coprocessor worker that wants its own heap) should carve their
storage out of the "Free RAM" gaps in J.1, J.3, and J.6 rather
than colliding with the BASIC variable region.
