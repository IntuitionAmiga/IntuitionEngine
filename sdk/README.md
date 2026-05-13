# Intuition Engine SDK

Developer kit for building software that runs on the Intuition Engine retro hardware emulator. Includes platform headers, curated examples, music assets, and build scripts for all six supported CPU cores.

## Directory Layout

```
sdk/
  bin/              Assembler and tool binaries (built by make)
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
    prebuilt/         Assembled demos and boot assets
    basic/            EhBASIC examples
    assets/           Music files and binary data used by examples
  cputest/            M68K 68020/FPU CPU validation suite (Go generator + vasm assembly)
  scripts/            Build helper scripts for each CPU target
                      (including aros_*.ies test harnesses for AROS boot/input/path testing)
```

## Quick Start

1. Build SDK tools and pre-assemble demos supported by the installed toolchains:
   ```bash
   make sdk          # Builds SDK tools and assembles available demos into prebuilt/
   ```

2. Run a pre-assembled demo directly:
   ```bash
   ./bin/IntuitionEngine sdk/examples/prebuilt/vga_text_hello.iex
   ```

   Or from the EhBASIC prompt:
   ```basic
   RUN "sdk/examples/prebuilt/vga_text_hello.iex"
   EMUTOS
   ```

   The `EMUTOS` command boots the EmuTOS operating system (requires `make basic-emutos` build or a local `etos256us.img` file).

   Boot AROS (M68K Workbench-style desktop):
   ```bash
   ./bin/IntuitionEngine -aros
   ./bin/IntuitionEngine -aros -aros-drive ~/amiga-files
   ```

   Build AROS ROM from source with `make aros-rom`.

3. Or build and run from source (IE32 VGA text mode):
   ```bash
   sdk/bin/ie32asm sdk/examples/asm/vga_text_hello.asm
   ./bin/IntuitionEngine sdk/examples/asm/vga_text_hello.iex
   ```

4. Build each per-CPU script's default example set:
   ```bash
   ./sdk/scripts/build-all.sh
   ```

## Scripting (IEScript)

IEScript is the Lua 5.1 automation layer for Intuition Engine. Scripts use the `.ies` extension and provide scripted control of the emulator via `sys`, `cpu`, `mem`, `term`, `audio`, `video`, `repl`, `rec`, `dbg`, `sym`, `regions`, `coproc`, and `media`. IEScript also exposes the `bit32` and `keys` global tables. Use cases include automated demo recording, test harnesses, and scripted debugging workflows. A product demo script is included at `scripts/ie_product_demo.ies`. See [IEScript Lua Automation](docs/iescript.md) for the full reference.

Prepare the product demo assets with:

```bash
make build-showreel-deps
```

Or build and launch it directly with:

```bash
make run-showreel
```

```bash
./bin/IntuitionEngine demo.ie64 -script demo.ies
./bin/IntuitionEngine -script demo.ies demo.ie64
```

Or from the EhBASIC prompt:
```basic
RUN "demo.ies"
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

## Selected Demo Matrix

The examples below cover the main CPU, video, audio, and coprocessor paths. They are a README-level orientation, not an exhaustive list of every file in `sdk/examples/asm`.

### Rotozoomer Series

The rotozoomer examples demonstrate a hardware-accelerated rotating/zooming texture using the Mode7 blitter. The series includes one assembly version per CPU core plus BASIC and EmuTOS variants. Each version is commented to show both the shared algorithm and CPU-specific programming patterns.

| Example | CPU | Video | Audio | Description |
|---------|-----|-------|-------|-------------|
| `rotozoomer.asm` | IE32 | IEVideoChip | AHX | Mode7 blitter + Amiga tracker music |
| `rotozoomer_ie64.asm` | IE64 | IEVideoChip | SAP/POKEY | Mode7 blitter + Atari 8-bit music |
| `rotozoomer_68k.asm` | M68K | IEVideoChip | TED audio | Mode7 blitter + C264 music |
| `rotozoomer_z80.asm` | Z80 | IEVideoChip | SID | Mode7 blitter + C64 music |
| `rotozoomer_65.asm` | 6502 | IEVideoChip | AHX | Mode7 blitter + Amiga tracker music |
| `rotozoomer_x86.asm` | x86 | IEVideoChip | PSG | Mode7 blitter + AY-3-8910 music |
| `rotozoomer_basic.bas` | IE64 (BASIC) | IEVideoChip | SID | Mode7 blitter from EhBASIC |
| `rotozoomer_gem.asm` | M68K (EmuTOS) | IEVideoChip | -- | Mode7 blitter in a GEM desktop window |

### Video Chip Showcases

| Example | CPU | Video | Audio | Description |
|---------|-----|-------|-------|-------------|
| `vga_text_hello.asm` | IE32 | VGA (text) | -- | Simplest demo: coloured text on an 80x25 screen |
| `vga_mode13h_fire.asm` | IE32 | VGA (Mode 13h) | -- | Classic DOS-era 256-colour fire effect |
| `vga_modex_circles.asm` | IE32 | VGA (Mode X) | -- | Animated circles in 320x240 planar mode |
| `vga_mode12h_bars.asm` | IE32 | VGA (Mode 12h) | -- | Colour bars in 640x480 4-plane mode |
| `vga_text_sap_demo.asm` | Z80 | VGA (text) | POKEY/SAP | VGA text mode with SAP music playback |
| `ula_rotating_cube_65.asm` | 6502 | ULA (Spectrum) | AHX | Wireframe 3D cube on ZX Spectrum display |
| `ted_121_colors_68k.asm` | M68K | TED (Plus/4) | PSG | Full-screen plasma using all 121 TED colours |
| `antic_plasma_x86.asm` | x86 | ANTIC/GTIA | SID | Atari 8-bit display list + Player/Missile graphics |
| `rotating_cube_copper_68k.asm` | M68K | IEVideoChip + Copper | -- | 3D cube with copper rasterbars |
| `mandelbrot_ie64.asm` | IE64 | IEVideoChip | -- | Real-time Mandelbrot fractal |
| `voodoo_mega_demo.asm` | IE32 | Voodoo 3D | SID | Textured 3D scenes with SID music |
| `voodoo_cube_68k.asm` | M68K | Voodoo 3D | -- | Z-buffered 3D cube on 3DFX Voodoo hardware |
| `voodoo_3dfx_logo_68k.asm` | M68K | Voodoo 3D | -- | Textured 3DFX logo flyby with fog |
| `voodoo_triangle_68k.asm` | M68K | Voodoo 3D | -- | Flat-shaded triangle |
| `voodoo_tunnel_z80.asm` | Z80 | Voodoo 3D | -- | Texture-mapped tunnel effect |
| `robocop_intro.asm` | IE32 | IEVideoChip + Copper | PSG | Copper rasterbars + blitter sprite + sine scrolltext |
| `robocop_intro_68k.asm` | M68K | IEVideoChip + Copper | PSG | Copper rasterbars + blitter sprite + sine scrolltext |
| `robocop_intro_65.asm` | 6502 | IEVideoChip + Copper | PSG | Copper rasterbars + blitter sprite + sine scrolltext |
| `robocop_intro_z80.asm` | Z80 | IEVideoChip + Copper | PSG | Copper rasterbars + blitter sprite + sine scrolltext |

### Coprocessor Communication

| Example | CPU | Description |
|---------|-----|-------------|
| `coproc_caller_ie32.asm` | IE32 | Launches a worker, sends a request, reads the result |
| `coproc_service_ie32.asm` | IE32 | Worker that polls a ring buffer and processes requests |
| `coproc_caller_65.asm` / `coproc_service_65.asm` | 6502 | 8-bit caller/service pair using gateway MMIO |
| `coproc_caller_68k.asm` / `coproc_service_68k.asm` | M68K | 68020 caller/service pair using direct MMIO |
| `coproc_caller_z80.asm` / `coproc_service_z80.asm` | Z80 | Z80 caller/service pair using gateway MMIO |
| `coproc_caller_x86.asm` / `coproc_service_x86.asm` | x86 | Flat 32-bit x86 caller/service pair using direct MMIO |

## Coverage Summary

### CPU Cores
IE32, IE64, M68K (68020 with 68881/68882 FPU), Z80, 6502, x86 (32-bit)

### Video Chips
IEVideoChip, VGA, ULA, TED video, ANTIC/GTIA, Voodoo SST-1 style 3D, and the copper coprocessor. See [Architecture](docs/architecture.md), [Compositor](docs/compositor.md), and [Voodoo ABI](docs/ie_voodoo_abi.md) for register maps and rendering details.

### Audio Engines
SoundChip/SFX, PSG/AY/YM, native SN76489 VGM writes, SID/Multi-SID, POKEY/SAP, TED audio, AHX/THX, ProTracker MOD, WAV PCM, and AROS Paula-style DMA. See [Architecture](docs/architecture.md), [Sound MMIO](docs/ie_sfx_mmio.md), [WAV Player](docs/wav_player.md), and [Demo Matrix](docs/demo-matrix.md) for format, MMIO, and example coverage details.

## Build Scripts

Individual target scripts and a master script for the default per-CPU sets:

```bash
./sdk/scripts/build-ie32.sh              # Default IE32 example set
./sdk/scripts/build-ie64.sh              # Default IE64 example set
./sdk/scripts/build-m68k.sh              # Default M68K example set
./sdk/scripts/build-z80.sh               # Default Z80 example set
./sdk/scripts/build-6502.sh              # Default 6502 example set
./sdk/scripts/build-x86.sh               # Default x86 example set
make cputest-bin                          # M68K 68020/FPU CPU validation suite (bare-metal binary)
./sdk/scripts/build-all.sh               # Default sets for all CPU targets; skips missing toolchains

# Build a single file:
./sdk/scripts/build-m68k.sh sdk/examples/asm/voodoo_cube_68k.asm
```

Environment variables for custom tool paths:

| Variable | Default | Description |
|----------|---------|-------------|
| `IE_BIN_DIR` | `./sdk/bin` | Path to IE assembler binaries |
| `VASM_M68K` | `vasmm68k_mot` | M68K assembler path |
| `VASM_Z80` | `vasmz80_std` | Z80 assembler path |
| `CA65` / `LD65` | `ca65` / `ld65` | cc65 toolchain paths |
| `NASM` | `nasm` | x86 assembler path |

## Include File Stability

The `sdk/include/` headers are the compatibility-facing register/macro surface for SDK users.
Runtime Go constants/handlers are the implementation source of truth, and shared include/runtime constants are guarded by repository consistency tests (`sdk_include_consistency_test.go`).

## IntuitionOS SDK Notes

IntuitionOS runtime ELFs carry IOSM metadata and use the runtime ELF contract documented under [IntuitionOS ELF](docs/IntuitionOS/ELF.md) and [IntuitionOS IExec](docs/IntuitionOS/IExec.md). Keep ABI-level details in those documents rather than in this SDK overview.

## Further Documentation

- [IE64 Instruction Set Reference](docs/IE64_ISA.md)
- [IE64 Cookbook](docs/IE64_COOKBOOK.md)
- [Tutorial](docs/TUTORIAL.md)
- [EhBASIC Guide](docs/ehbasic_ie64.md)
- [IEScript Lua Automation](docs/iescript.md)
- [IE32 to IE64 Migration](docs/ie32to64.md)
- [EmuTOS Integration Guide](docs/ie_emutos.md)
- [IntuitionOS ELF](docs/IntuitionOS/ELF.md)
- [IntuitionOS IExec](docs/IntuitionOS/IExec.md)

## AROS Support

AROS (Amiga Research Operating System) runs on the IE M68K core with a Workbench-style desktop. Build the AROS ROM with `make aros-rom`, then boot with `./bin/IntuitionEngine -aros`. The AROS DOS handler maps a host directory as the IE: volume. Test harnesses in `scripts/aros_*.ies` cover boot, path resolution, and input handling. The `iewarp.library` AROS shared library offloads compute-intensive tasks to the IE64 coprocessor.

## EmuTOS Porting Stubs

The SDK includes EmuTOS machine-target scaffolding in `sdk/emutos/`.
See [docs/ie_emutos.md](docs/ie_emutos.md) for the full hardware map, build instructions, and GEM programming guide.
