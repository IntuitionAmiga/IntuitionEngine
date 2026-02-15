# Intuition Engine SDK

Developer kit for building programs that run on the Intuition Engine retro hardware emulator. Includes platform headers, curated demo examples, music assets, and build scripts for all six supported CPU cores.

## Directory Layout

```
sdk/
  include/          Platform headers and linker configs
    ie32.inc          IE32 (custom 32-bit RISC) macro library + hardware register map
    ie64.inc          IE64 (custom 64-bit RISC) macro library + EhBASIC memory layout
    ie65.inc          6502 macro library + memory bank definitions
    ie65.cfg          6502 linker configuration (cc65)
    ie68.inc          M68K macro library
    ie80.inc          Z80 macro library
    ie86.inc          x86 (32-bit) macro library
  examples/
    asm/              Assembly language examples (one per CPU core + video/audio showcases)
    basic/            EhBASIC examples
    assets/           Music files and binary data used by examples
  scripts/            Build helper scripts for each CPU target
```

## Quick Start

1. Build the Intuition Engine VM:
   ```bash
   make              # Builds bin/IntuitionEngine + bin/ie32asm
   ```

2. Build and run the simplest demo (IE32 VGA text mode):
   ```bash
   ./bin/ie32asm sdk/examples/asm/vga_text_hello.asm
   ./bin/IntuitionEngine -ie32 vga_text_hello.iex
   ```

3. Build all examples with available toolchains:
   ```bash
   ./sdk/scripts/build-all.sh
   ```

## Toolchain Requirements

| CPU Core | Assembler | Install |
|----------|-----------|---------|
| IE32 | `ie32asm` (built-in) | `make ie32asm` |
| IE64 | `ie64asm` (built-in) | `make ie64asm` |
| M68K | `vasmm68k_mot` | [VASM](http://sun.hasenbraten.de/vasm/) |
| Z80 | `vasmz80_std` | [VASM](http://sun.hasenbraten.de/vasm/) |
| 6502 | `ca65` / `ld65` | [cc65](https://cc65.github.io/) |
| x86 | `nasm` | [NASM](https://www.nasm.us/) |

## Demo Matrix

Each example demonstrates a specific combination of CPU core, video chip, and audio engine.

### Rotozoomer Series (one per CPU core)

The rotozoomer is the canonical "hello world" demo: a hardware-accelerated rotating/zooming texture using the Mode7 blitter (same technique as SNES F-Zero and Mario Kart). Each version is heavily commented to teach both the algorithm and CPU-specific programming patterns.

| Example | CPU | Video | Audio | Description |
|---------|-----|-------|-------|-------------|
| `rotozoomer.asm` | IE32 | IEVideoChip | AHX | Mode7 blitter + Amiga tracker music |
| `rotozoomer_ie64.asm` | IE64 | IEVideoChip | SAP/POKEY | Mode7 blitter + Atari 8-bit music |
| `rotozoomer_68k.asm` | M68K | IEVideoChip | TED audio | Mode7 blitter + C264 music |
| `rotozoomer_z80.asm` | Z80 | IEVideoChip | SID | Mode7 blitter + C64 music |
| `rotozoomer_65.asm` | 6502 | IEVideoChip | AHX | Mode7 blitter + Amiga tracker music |
| `rotozoomer_x86.asm` | x86 | IEVideoChip | PSG | Mode7 blitter + AY-3-8910 music |
| `rotozoomer_basic.bas` | IE64 (BASIC) | IEVideoChip | SID | Mode7 blitter from EhBASIC |

### Video Chip Showcases

| Example | CPU | Video | Audio | Description |
|---------|-----|-------|-------|-------------|
| `vga_text_hello.asm` | IE32 | VGA (text) | -- | Simplest possible demo: colored text on 80x25 screen |
| `vga_mode13h_fire.asm` | IE32 | VGA (Mode 13h) | -- | Classic DOS-era 256-color fire effect |
| `copper_vga_bands.asm` | IE32 | VGA + Copper | -- | Amiga-style per-scanline palette manipulation |
| `ula_rotating_cube_65.asm` | 6502 | ULA (Spectrum) | AHX | Wireframe 3D cube on ZX Spectrum display |
| `ted_121_colors_68k.asm` | M68K | TED (Plus/4) | PSG | Full-screen plasma using all 121 TED colors |
| `antic_plasma_x86.asm` | x86 | ANTIC/GTIA | SID | Atari 8-bit display list + Player/Missile graphics |
| `voodoo_cube_68k.asm` | M68K | Voodoo 3D | -- | Z-buffered 3D cube on 3DFX Voodoo hardware |

### Coprocessor Communication

| Example | CPU | Description |
|---------|-----|-------------|
| `coproc_caller_ie32.asm` | IE32 | Launches a worker, sends a request, reads the result |
| `coproc_service_ie32.asm` | IE32 | Worker that polls a ring buffer and processes requests |

## Coverage Summary

### CPU Cores
IE32, IE64, M68020, Z80, 6502, x86 (32-bit)

### Video Chips
IEVideoChip (640x480 true color), VGA (text/Mode 13h/Mode 12h/ModeX), ULA (ZX Spectrum 256x192), TED (Commodore Plus/4 121 colors), ANTIC/GTIA (Atari 8-bit display lists), Voodoo SST-1 (3DFX hardware 3D), Copper coprocessor (per-scanline effects)

### Audio Engines
IESoundChip (custom synthesizer), PSG/AY-3-8910, SID (Commodore 64), POKEY/SAP (Atari 8-bit), TED audio (Commodore Plus/4), AHX (Amiga tracker)

## Build Scripts

Individual target scripts and a master build-all:

```bash
./sdk/scripts/build-ie32.sh              # All IE32 examples
./sdk/scripts/build-ie64.sh              # All IE64 examples
./sdk/scripts/build-m68k.sh              # All M68K examples
./sdk/scripts/build-z80.sh               # All Z80 examples
./sdk/scripts/build-6502.sh              # All 6502 examples
./sdk/scripts/build-x86.sh               # All x86 examples
./sdk/scripts/build-all.sh               # Everything (skips missing toolchains)

# Build a single file:
./sdk/scripts/build-m68k.sh sdk/examples/asm/voodoo_cube_68k.asm
```

Environment variables for custom tool paths:

| Variable | Default | Description |
|----------|---------|-------------|
| `IE_BIN_DIR` | `./bin` | Path to IE assembler binaries |
| `VASM_M68K` | `vasmm68k_mot` | M68K assembler path |
| `VASM_Z80` | `vasmz80_std` | Z80 assembler path |
| `CA65` / `LD65` | `ca65` / `ld65` | cc65 toolchain paths |
| `NASM` | `nasm` | x86 assembler path |

## Include File Stability

The `sdk/include/` headers define the stable hardware register map for v1.x. The canonical source of truth is `assembler/*.inc` in the main repository. SDK copies are synced at release time.

## Further Documentation

- [IE64 Instruction Set Reference](../docs/IE64_ISA.md)
- [IE64 Cookbook](../docs/IE64_COOKBOOK.md)
- [Tutorial](../docs/TUTORIAL.md)
- [EhBASIC Guide](../docs/ehbasic_ie64.md)
- [IE32 to IE64 Migration](../docs/ie32to64.md)
