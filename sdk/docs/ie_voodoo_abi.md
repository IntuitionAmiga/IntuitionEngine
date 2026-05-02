# IE Voodoo HLE ABI

This document defines the Intuition Engine Voodoo HLE bus contract. It is an IE-native chip ABI, not a PCI or Glide specification. Software backend output is the reference for implemented features.

The machine-readable register table is `sdk/docs/ie_voodoo_abi.tsv`. The table below mirrors that file at the register level; bit definitions remain in `voodoo_constants.go` and the SDK include files.

## Register Window

The register aperture is contiguous:

- `VOODOO_BASE = 0x0F8000`
- `VOODOO_END = 0x0F87FF`
- `VOODOO_TEXMEM_BASE = 0x0D0000`
- `VOODOO_TEXMEM_SIZE = 0x10000`

The expanded register window includes fog registers and the 256-entry palette area beginning at `VOODOO_PALETTE_BASE`.

## Register Classes

- `implemented`: writes have documented behavior and reads return the current shadow value, except status.
- `compat-readback`: writes are accepted and readable, but have no behavioral side effect until a later implementation phase promotes them.
- `reserved`: guest software must not depend on reads, writes, or side effects.

## CPU Access Contract

- IE64: 8/16/32/64-bit reads and writes to the register aperture. Naturally aligned 64-bit writes commit the low dword then the high dword while holding the chip lock.
- IE32, M68K, x86: 8/16/32-bit access to the register aperture. Aligned 16-bit and 32-bit writes are supported; misaligned wider writes are treated as the bus delivers them.
- Z80: no direct `0xF8xxx` MMIO. Use the port adapter at `0xB0..0xB7`.
- 6502: no direct `0xF8xxx` MMIO. Use the banked aperture at `VOODOO_6502_WINDOW_BASE = 0xE000`, selecting the 4KB target page through `VOODOO_6502_BANK_HI = 0xF7F2` and `VOODOO_6502_BANK_PAGE_HI = 0xF7F3` (`0x00F8` for registers, `0x00D0..0x00DF` for texture memory).
- Texture uploads: IE64/IE32/M68K/x86 can stream bytes, words, or dwords into `VOODOO_TEXMEM_BASE`. Z80 uses the existing `VOODOO_PORT_TEXSRC_*` adapter and triggers `VOODOO_TEX_UPLOAD`. 6502 streams through the banked aperture.

## Partial-Write Commit Model

Byte and word writes update the 32-bit shadow register with read-modify-write semantics. Side effects fire only when the dword is complete: a 32-bit aligned write, a 64-bit aligned write covering the dword, or the byte write to `offset % 4 == 3`.

This rule applies to command registers including `VOODOO_TRIANGLE_CMD`, `VOODOO_FTRIANGLECMD`, `VOODOO_FAST_FILL_CMD`, `VOODOO_SWAP_BUFFER_CMD`, `VOODOO_ENABLE`, and `VOODOO_TEX_UPLOAD`.

## Texture Model

Implemented texture upload uses the IE texture memory window and `VOODOO_TEXTURE_MODE`, `VOODOO_TEX_WIDTH`, `VOODOO_TEX_HEIGHT`, and `VOODOO_TEX_UPLOAD`. Supported guest-facing formats are ARGB8888, ARGB1555, ARGB4444, I8, A8, and P8. YIQ is reserved. Nearest and bilinear sampling are ABI-visible; mip/trilinear behavior is `compat-readback` until LOD registers are promoted.

Texture coordinates use 14.18 fixed point. W uses 2.30 fixed point and defaults to 1.0 for rasterization purposes.

## Raster Features

Implemented: vertex setup, flat and Gouraud color attributes, triangle command submission, fast fill including swap-clear, swap, scissor clipping, depth compare modes, alpha test/blend state, basic texture enable/wrap, fog mode/color/table state, chroma-key exact/range state, stipple state, LFB mode state, TLOD/texture-base state, palette table state, and slope register forwarding.

Compat-readback pending behavior: palette-indexed texture lookup and fog table raster lookup.

Clip coordinates use 16-bit packed fields. Existing raster code may clamp to backend dimensions.

## Lifecycle, Status, IRQ

Reset clears register shadow, texture memory, triangle batch, vertex assembly state, VBlank state, and backend state. Status bits are IE-native:

- `busy`: backend flush in progress.
- `swapPending`: swap has been requested and not completed.
- `MEMFIFO`: non-zero while triangle batch has room, zero when full.
- `vretrace`: active during the host VBlank window.

VBlank, swap-complete, and fifo-empty lifecycle callbacks are raised by the chip. Guest OS IRQ-line policy remains outside this chip ABI.

## Backend Parity

The software backend is the reference. Vulkan must match software output for implemented features within per-channel RGB delta <= 1/255 and depth delta <= 1 ULP in 20.12 fixed-point terms. Vulkan-only features are non-conformant unless promoted into this ABI.

## Register Table

See `ie_voodoo_abi.tsv` for the authoritative table. Any change to a `VOODOO_*` register address in Go must update the TSV and pass `TestVoodooABIDrift`.

## Out Of Scope

Real SST-1 PCI configuration space, Glide API behavior, DAC pass-through, SLI, and CMDFIFO mode 1 are outside this IE bus-chip ABI.
