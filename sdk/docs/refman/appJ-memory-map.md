---
title: "Full Memory Map"
sources:
  - registers.go
  - sdk/include/ie64.inc
  - sdk/include/ie32.inc
  - voodoo_constants.go
  - ula_constants.go
  - vga_constants.go
---

# Appendix J - Full Memory Map

The fixed low regions of the Intuition Engine `64`-bit physical bus,
in address order, with their size and purpose. IE64 can address beyond
this low window when RAM is present. IE32, M68K, and x86 see the low
`4` GB window directly; the 6502 and Z80 see windowed views into it
(Chapters 27 and 28).

## J.1 Low RAM and stacks

| Range                    | Size  | Purpose |
|--------------------------|-------|---------|
| `$000000`-`$0003FF`    | 1 KB  | M68K vector table; x86 IVT; trap vector base for IE64. |
| `$000400`-`$022FFF`    | ~140 KB | BASIC ROM image, system call shim, monitor stub. |
| `$023000`-`$04FFFF`    | 180 KB | BASIC program text (`BASIC_PROG_START`-). |
| `$050000`-`$057FFF`    | 32 KB | BASIC simple variables. |
| `$058000`-`$05FFFF`    | 32 KB | BASIC string variables. |
| `$060000`-`$08BFFF`    | 176 KB | BASIC arrays. |
| `$08C000`-`$08FFFF`    | 16 KB | BASIC string temporaries. |
| `$090000`-`$096FFF`    | 28 KB | BASIC `GOSUB` / `FOR` stack. |
| `$097000`-`$09EFFF`    | 32 KB | Free RAM (general user). |
| `$09F000`-`$09FFFF`    | 4 KB | IE32 stack (`STACK_START`). |

## J.2 PC-compatible VRAM apertures

| Range                  | Size  | Purpose |
|------------------------|-------|---------|
| `$0A0000`-`$0AFFFF`  | 64 KB | VGA graphics window (mode 13h, mode 12h linear). |
| `$0B0000`-`$0B7FFF`  | 32 KB | Reserved (PC-compatible monochrome text window; not implemented). |
| `$0B8000`-`$0BFFFF`  | 32 KB | VGA text buffer (`$B8000`). |

## J.3 General RAM gap

| Range                  | Size  | Purpose |
|------------------------|-------|---------|
| `$0C0000`-`$0CFFFF`  | 64 KB | Free RAM (general user). |
| `$0D0000`-`$0DFFFF`  | 64 KB | Voodoo texture RAM. |
| `$0E0000`-`$0EFFFF`  | 64 KB | Free RAM (general user). |

## J.4 The MMIO region (`$F0000`-`$FFFFF`)

| Range                     | Size   | Device |
|---------------------------|--------|--------|
| `$F0000`-`$F049B`       | 1180 B | Video chip + palette + extended blitter. |
| `$F0700`-`$F07FF`       | 256 B  | Terminal / serial / input. |
| `$F0800`-`$F0B7F`       | 896 B  | SoundChip channel registers. |
| `$F0B80`-`$F0B91`       | 18 B   | AHX player. |
| `$F0BA0`-`$F0BBF`       | 32 B   | MIDI/MUS player. |
| `$F0BC0`-`$F0BD7`       | 24 B   | MOD player. |
| `$F0BD8`-`$F0BF3`       | 28 B   | WAV player. |
| `$F0C00`-`$F0C20`       | 33 B   | PSG / AY. |
| `$F0C30`-`$F0C3F`       | 16 B   | SN76489. |
| `$F0C40`-`$F0CFF`       | 192 B  | SoundChip flex channels 4-6. |
| `$F0D00`-`$F0D20`       | 33 B   | POKEY. |
| `$F0D40`-`$F0DFF`       | 192 B  | SoundChip flex channels 7-9. |
| `$F0E00`-`$F0E1C`       | 29 B   | SID primary registers. |
| `$F0E20`-`$F0E2D`       | 14 B   | SID player block. |
| `$F0E30`-`$F0E4C`       | 29 B   | SID2 registers. |
| `$F0E50`-`$F0E6C`       | 29 B   | SID3 registers. |
| `$F0E80`-`$F0EFF`       | 128 B  | SFX triggers. |
| `$F0F00`-`$F0F6B`       | 108 B  | TED audio + video. |
| `$F1000`-`$F13FF`       | 1 KB   | VGA registers. |
| `$F1400`-`$F140F`       | 16 B   | HOST appliance block. |
| `$F2000`-`$F2017`       | 24 B   | ULA registers. |
| `$F2100`-`$F213F`       | 64 B   | ANTIC. |
| `$F2140`-`$F21FB`       | 188 B  | GTIA. |
| `$F2200`-`$F221F`       | 32 B   | File I/O. |
| `$F2260`-`$F22AF`       | 80 B   | Amiga Paula DMA. |
| `$F2300`-`$F231F`       | 32 B   | Media loader. |
| `$F2320`-`$F233F`       | 32 B   | RUN loader block. |
| `$F2340`-`$F238F`       | 80 B   | Coprocessor. |
| `$F2390`-`$F23AF`       | 32 B   | Clipboard bridge. |
| `$F23B0`-`$F23BF`       | 16 B   | Coprocessor extended monitor. |
| `$F23C0`-`$F23DF`       | 32 B   | IRQ diagnostics. |
| `$F23E0`-`$F23FF`       | 32 B   | Bootstrap loader. |
| `$F2400`-`$F24FF`       | 256 B  | SysInfo (RAM-size ABI). |
| `$F8000`-`$F87FF`       | 2 KB   | Voodoo 3D registers. |
| `$FA000`-`$FBAFF`       | 6912 B | ULA VRAM aperture. |

## J.5 Main video RAM

| Range                       | Size  | Purpose |
|-----------------------------|-------|---------|
| `$100000`-`$5FFFFF`       | 5 MB  | Main VRAM aperture for VideoChip framebuffers; large modes may point `VIDEO_FB_BASE` into ordinary RAM. |

## J.6 Low-window RAM and reserved aliases

| Range                       | Size   | Purpose |
|-----------------------------|--------|---------|
| `$00600000`-`$FFFEFFFF` | Dynamic | Extended RAM when backed by the current guest-RAM allocation. |
| `$FFFF0000`-`$FFFFFFFF` | 64 KB  | Sign-extended alias of `$00000000`-`$0000FFFF`. |

Guest RAM can extend beyond these fixed low ranges. Use the SysInfo
registers in Chapter 24 to discover total and active visible RAM. IE64
can address backed RAM above `$FFFFFFFF`; the compatibility CPUs remain
inside the low window or their documented profile caps.

## J.7 The 6502 / Z80 views

The 6502 sees a 16-bit address space `$0000`-`$FFFF`. Its
adapter routes the high page-window region as follows:

| 6502 range          | Maps to low bus address |
|---------------------|------------------|
| `$0000`-`$00FF`     | zero page in main RAM. |
| `$0100`-`$01FF`     | 6502 stack page in main RAM. |
| `$0200`-`$CFFF`     | main RAM. |
| `$D200`-`$D20A`     | POKEY registers. |
| `$D400`-`$D40F`     | PSG registers. |
| `$D500`-`$D55F`     | SID registers. |
| `$D600`-`$D605`     | TED audio registers. |
| `$D620`-`$D632`     | TED video registers. |
| `$D700`-`$D70D`     | VGA registers. |
| `$D800`-`$D817`     | ULA registers, including the paged VRAM data port. |
| `$E000`-`$EFFF`     | bank-selectable window into main RAM. |
| `$F000`-`$FFF9`     | MMIO mirror of `$F0000`-`$F0FF9`, with `$F700`-`$F705` and `$F7F0` intercepted as bank registers. |
| `$FFFA`-`$FFFF`     | NMI / reset / IRQ vectors. |

The Z80 sees the same shape with port I/O substituted for the
chip-aperture region; the bank-window scheme is identical.

## J.8 Notes

The map above is the compact engineering view of the low and device
regions that have fixed meanings inside the `64`-bit physical bus.
Most user programs use a small sub-set of it - the BASIC variable area,
the I/O region, and the main video RAM - and never touch the rest.
Programs that compete for memory (a large in-memory dataset, a custom
blitter routine, a coprocessor worker that wants its own heap) should
carve their storage out of the "Free RAM" gaps in J.1, J.3, and J.6,
or from boot-sized extended RAM reported by SysInfo, rather than
colliding with the BASIC variable region.
