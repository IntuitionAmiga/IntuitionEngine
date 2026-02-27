# Changelog

All notable changes to Intuition Engine are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-02-15

### Added

#### CPU Cores
- **IE64** (64-bit RISC): 32 registers, native FP32 FPU, compare-and-branch architecture, no flags register. Default core.
- **IE32** (32-bit RISC): 16 registers, fixed 8-byte instructions.
- **M68020** (Motorola 68020): 95%+ instruction coverage with 68881/68882 FPU support.
- **Z80** (Zilog): Full instruction set with interrupt modes 0/1/2.
- **6502** (MOS): NMOS instruction set with zero page optimisation.
- **x86** (Intel 32-bit): 8086 instructions with 32-bit registers, flat memory model, x87 FPU (387 scope).
- IE32-to-IE64 assembly converter (`ie32to64`).

#### Video System
- **IEVideoChip**: 640x480/800x600/1024x768/1280x960 true-colour framebuffer with double buffering (default 1280x960).
- **VGA**: Text mode (80x25), Mode 13h (320x200x256), Mode 12h (640x480x16), ModeX.
- **ULA**: ZX Spectrum 256x192 display with attribute colour.
- **TED**: Commodore Plus/4 video with 121-colour palette.
- **ANTIC/GTIA**: Atari 8-bit display list processor with Player/Missile graphics.
- **Voodoo SST-1**: 3DFX hardware 3D with Z-buffer, Gouraud shading, texture mapping, fog, alpha blending, chromakey.
- Copper coprocessor for per-scanline raster effects.
- DMA blitter with copy, fill, line draw, and Mode7 (SNES-style rotation/scaling).
- Video compositor for multi-chip overlay rendering.
- Dirty rectangle tracking for efficient updates.

#### Audio System
- **IESoundChip**: 9-channel custom synthesiser (5 dedicated + 4 flexible waveform channels).
  - ADSR envelopes, PWM, frequency sweep, hard sync, ring modulation.
  - Global filter (LP/HP/BP), overdrive, reverb.
  - 44.1kHz 32-bit floating-point processing.
- **PSG** (AY-3-8910/YM2149): 3-channel square + noise with envelope generator. Supports `.ym`, `.ay`, `.vgm`, `.vgz`, `.vtx`, `.sndh`, `.pt3`, `.pt2`, `.pt1`, `.stc`, `.sqt`, `.asc`, `.ftc` playback. ZX Spectrum tracker formats use Z80 emulation (PT2/PT3/STC/SQT) or native Go players (PT1/ASC/FTC).
- **SID** (MOS 6581/8580): 3 voices with ADSR, ring modulation, hard sync, resonant filter. Supports `.sid` playback (PSID v1-v4, RSID). Multi-SID playback with up to 3 independent chips (9 voices), per-chip model selection from header flags.
- **POKEY** (Atari): 4-channel with polynomial counters and high-pass filter. Supports `.sap` playback.
- **TED** (Commodore Plus/4): 2-channel square wave audio. Supports `.ted` file playback.
- **AHX** (Amiga): Tracker-based module playback.
- **WAV**: PCM audio playback via SoundChip FLEX DAC mode.
- **MOD** (ProTracker): 4-channel .mod file playback via SoundChip FLEX DAC mode with Amiga A500/A1200 filter emulation. MMIO registers at `$F0BC0-$F0BD7`, `-mod` CLI flag, MediaLoader auto-detection, and EhBASIC `SOUND MOD` commands.
- **SN76489** support via VGM command `0x50` with clock-accurate frequency scaling to AY registers and dynamic noise-tracks-tone2.
- PLUS enhanced modes for PSG, SID, POKEY, TED, and AHX with logarithmic volume curves.

#### VGM Support
- `.vgm` and `.vgz` (gzip-compressed) file playback.
- AY-3-8910/YM2149 chip events (`0xA0`).
- SN76489/SN76496 chip events (`0x50`) with automatic frequency conversion.
- Graceful skip of unsupported chip commands (YM2413, YM2612, YM2151, OPL series, Sega PCM, DAC streams).

#### SID Enhancements
- Multi-SID file support: Sid2Addr/Sid3Addr parsed from v3/v4 headers with per-chip 6581/8580 model from flags.
- RSID handling: PlayAddress=0 interrupt-driven playback, embedded load addresses, CIA/VBI speed selection per subsong.
- SID+ enhanced mode with 2dB-per-step logarithmic volume curve.

#### EhBASIC Interpreter
- Full EhBASIC port on IE64 with FP32 soft-float library.
- Language: variables, arrays (DIM), strings, math functions, control flow (IF/THEN/ELSE, FOR/NEXT, WHILE/WEND, DO/LOOP, GOTO, GOSUB), DATA/READ, INPUT, ON GOTO/GOSUB.
- Hardware commands: SCREEN, CLS, PLOT, PALETTE, VSYNC, LOCATE, COLOR, SOUND, ENVELOPE, GATE, POKE/PEEK, WAIT.
- Extended commands: BLIT (COPY/FILL/LINE/MASK/ALPHA/MODE7/MEMCOPY/WAIT), COPPER, VOODOO (triangles, texture, fog, alpha, chromakey, dither, Z-buffer), ULA, TED, ANTIC/GTIA.
- Sound playback: SOUND PLAY "file", SOUND STOP, all chip engines accessible from BASIC.
- Utility: HEX$, BIN$, TRON/TROFF, CALL, USR(), CONT, SWAP, BITSET/BITCLR/BITTST, MIN/MAX.
- REPL with line editor, RUN/LIST/NEW, and `-basic` / `-basic-image` launch modes.

#### Machine Monitor (F9 Debugger)
- Step-through debugger for all 6 CPU types.
- Breakpoints with hit counts and conditions.
- Trace logging, backstep history, run-until.
- I/O register viewer for all hardware chips.
- Clipboard paste support.

#### Coprocessor Subsystem
- Async cross-CPU remote procedure calls.
- Ring buffer communication protocol.
- Support from all CPU cores including EhBASIC.

#### Build System
- Build profiles: `full` (default), `novulkan` (software Voodoo), `headless` (CI/testing), `headless-novulkan` (CGO_ENABLED=0 portable).
- Version metadata injection via ldflags (`-version` flag).
- Feature introspection (`-features` flag).
- Desktop entry and MIME type integration for `.ie*` files.

#### IEScript Lua Automation
- 11-module Lua API: `sys`, `cpu`, `mem`, `term`, `audio`, `video`, `repl`, `rec`, `dbg`, `coproc`, `media`.
- `repl` module for programmatic overlay control (show/hide, print, clear, scroll).
- `audio.psg_metadata()` and `audio.sid_metadata()` for querying song title, author, and system.
- Frame-synchronised coroutine model with cooperative yielding.
- MP4+AAC recording via FFmpeg, PNG screenshot capture.
- Interactive F8 REPL overlay with command history and multiline input.

#### SDK
- `sdk/` developer package with curated examples, include files, and build scripts.
- Rotozoomer demo series: one per CPU core (IE32, IE64, M68K, Z80, 6502, x86) plus EhBASIC.
- Video chip showcase demos: VGA text, VGA Mode 13h fire, VGA Mode 12h bars, VGA Mode X circles, VGA text + SAP music, ULA cube, TED plasma, ANTIC plasma.
- Voodoo 3D demos: mega demo, spinning cube, 3DFX logo flyby, filled triangle, textured tunnel.
- Robocop intro demo across 4 CPUs (IE32, M68K, 6502, Z80) with copper rasterbars and blitter sprite.
- Automated product demo script (`sdk/scripts/ie_product_demo.ies`) showcasing all 6 CPUs, 6 video chips, and 20 audio formats.
- Coprocessor communication examples.
- Per-target build scripts with environment variable overrides.

#### Platform Support
- **Linux** (x86_64, aarch64): Official platform with Ebiten graphics and Oto audio.
- **Windows** (x86_64, ARM64): Experimental with `novulkan` profile.
- Single-instance mode with IPC-based file handoff.
- F10 hard reset with full runtime state rebuild.
- Ebiten runtime status bar with live CPU/chip state (F12 toggle).
- Multi-resolution support, fullscreen mode, and display scaling.

#### Documentation
- Complete technical reference in README.md (6 CPUs, memory map, hardware registers, sound system, video system).
- Developer guide in DEVELOPERS.md (toolchains, build profiles, testing, contribution).
- SDK documentation with demo matrix and build instructions.
- Tutorial: step-by-step demoscene intro across 4 CPU architectures.
- IE64 ISA reference and cookbook.
- EhBASIC language guide.
- Machine monitor reference.

[1.0.0]: https://github.com/intuitionamiga/IntuitionEngine/releases/tag/v1.0.0
