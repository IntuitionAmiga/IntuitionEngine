---
title: "The IE Runtime Layer"
sources:
  - ../mk64-ie/ie/ie_runtime.c
  - ../mk64-ie/ie/game.ld
  - ../mk64-ie/ie/loader_main.c
  - ../mk64-ie/ie/ie_memory_layout.h
  - ../mk64-ie/ie/ie_pack.h
  - ../mk64-ie/ie/ie_mmio.h
  - ../mk64-ie/ie/ie_platform_asset.c
  - ../mk64-ie/ie/ie_platform_log.c
  - ../mk64-ie/ie/ie_platform_time.c
  - ../mk64-ie/ie/pack_ie68.py
  - ../mk64-ie/ie/coproc/coproc_layout.h
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 59 - The IE Runtime Layer

The runtime layer is the part of the port that makes a large C
programme feel like a native IE programme.

It starts execution, clears memory, installs service functions, reads
packed data, writes terminal output, reports fatal failures, and keeps
the high memory layout away from the low MMIO apertures.

## 59.1 Memory Shape

The checked port uses a simple rule: keep the loaded game high, keep
device apertures low, and reserve known ranges for packed data and
texture storage.

| Region | Purpose |
|--------|---------|
| `$00001000` | Initial M68K image entry and low loader space |
| Below `$000A0000` | Low RAM before the first device aperture |
| `$00280000` to `$002FFFFF` | M68K graphics-worker window |
| `$003A0000` to `$0041FFFF` | IE64 transform-worker window |
| `$00420000` to `$0049FFFF` | M68K audio-worker window |
| `$00600000` | Asset staging and pack header area |
| `$00790000` to `$00792FFF` | Coprocessor mailbox |
| `$01000000` upward | Packed data window in the self-contained image |
| `$08000000` upward | Voodoo texture store used by the port |
| `$10000000` upward | High-linked game image |

The exact addresses are less important than the discipline. Large code,
large data, asset staging, texture storage, stack, and MMIO must not
fight for the same space.

## 59.2 Startup

The small loader begins at the M68K entry address. It either finds a
self-contained pack already in memory or reads the game image through
the File I/O device, copies it to its linked high address, and jumps to
it.

The game runtime then performs the normal bare-machine duties:

- Clear BSS.
- Preserve initialised data.
- Install graphics, audio, input, time, asset, save, and log services.
- Locate and start the packed graphics, audio, and transform services.
- Publish simple boot-status words for smoke checks.
- Enter the game loop.

When a fatal error occurs, the runtime writes a visible fatal marker in
RAM, prints a terminal message, and halts the M68K CPU.

## 59.3 File And Pack Access

The pack format exists so the programme can be self-contained. A table
of contents names each asset and gives its offset and size. It also names
the IE64 transform service and the two M68K service images. Multi-byte
pack fields are big-endian, matching the M68K side of the port.

If the pack is present, assets are served from RAM. If it is not
present, the same asset contract can read through the File I/O device.
The game core sees the same asset service either way. If an optional
worker service cannot start, its owning contract remains available on
the main M68K through the local path.

## 59.4 The General IE Lesson

A serious IE programme needs a runtime contract before it needs clever
effects. Decide where code, worker windows, mailbox rings, data, stack,
assets, textures, status words, and MMIO live. Then make the rest of the
programme use that layout through named services.
