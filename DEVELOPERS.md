# Intuition Engine Developer Guide

Build, test, and contribute guide for the Intuition Engine retro hardware emulator.

For the full technical reference (CPU architectures, memory map, hardware registers, sound/video systems), see [README.md](README.md).
For the SDK developer package, see [sdk/README.md](sdk/README.md).

---

# Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Building](#2-building)
3. [Build Profiles and Tags](#3-build-profiles-and-tags)
4. [Version and Feature Introspection](#4-version-and-feature-introspection)
5. [Toolchain Matrix](#5-toolchain-matrix)
6. [Development Workflow](#6-development-workflow)
7. [Include Files](#7-include-files)
8. [Running Programs](#8-running-programs)
9. [Testing](#9-testing)
10. [Debugging](#10-debugging)
11. [Platform Support](#11-platform-support)
12. [Packaging and Distribution](#12-packaging-and-distribution)
13. [Contributing](#13-contributing)

---

# 1. Prerequisites

- **Go 1.26** or later
- `sstrip` and `upx` for binary optimisation (modify Makefile to skip if unavailable)
- No extra system packages are required for the default runtime path (Ebiten + Oto)

Optional dependencies for advanced features:
- **Vulkan** SDK/driver: required for the Voodoo Vulkan rasteriser path (not needed with `novulkan` profile)
- **liblhasa**: required for LHA decompression on Linux (not needed with `novulkan headless` profile)

---

# 2. Building

The build process uses the provided Makefile:

```bash
# Build everything (VM + all tools)
make

# Build only the VM
make intuition-engine

# Build individual tools
make ie32asm           # IE32 assembler
make ie64asm           # IE64 assembler
make ie32to64          # IE32-to-IE64 converter
make ie64dis           # IE64 disassembler
make basic             # VM with embedded EhBASIC interpreter

# Install / uninstall
make install           # Install to /usr/local/bin (all built tools)
make uninstall         # Remove installed binaries

# Housekeeping
make clean             # Remove all build artifacts
make list              # List compiled binaries with sizes
make help              # Show all available targets
```

Build outputs:
```
./bin/IntuitionEngine   # The virtual machine
./bin/ie32asm           # The IE32 assembler
./bin/ie64asm           # The IE64 assembler
./bin/ie32to64          # The IE32-to-IE64 converter
./bin/ie64dis           # The IE64 disassembler
```

Version metadata (version, git commit, build date) is automatically injected via ldflags.

---

# 3. Build Profiles and Tags

## Profiles

| Profile | Command | CGO | Description |
|---------|---------|-----|-------------|
| **full** (default) | `make` | Yes | All features: Vulkan Voodoo, Ebiten display, Oto audio, liblhasa |
| **novulkan** | `make novulkan` | Yes | Software Voodoo rasteriser only, no Vulkan SDK needed |
| **headless** | `make headless` | Yes | No display, no audio, no Vulkan (CI/testing) |
| **headless-novulkan** | `make headless-novulkan` | No | Fully portable `CGO_ENABLED=0` build, cross-compile safe |

## Build Tags

| Tag | Effect |
|-----|--------|
| `headless` | Disable GUI/audio/video backends (stubs only) |
| `novulkan` | Disable Vulkan backend, use software Voodoo rasteriser |
| `embed_basic` | Embed pre-assembled EhBASIC binary for `-basic` flag |
| `ie64` | IE64 assembler build tag |
| `ie64dis` | IE64 disassembler build tag |
| `m68k` | Enable M68K-specific tests |
| `audiolong` | Enable long-running audio demonstration tests |
| `videolong` | Enable long-running video demonstration tests |

## Profile Capability Matrix

| Feature | full | novulkan | headless | headless-novulkan |
|---------|------|----------|----------|-------------------|
| Ebiten display | Yes | Yes | Stub | Stub |
| Oto audio | Yes | Yes | Stub | Stub |
| Vulkan Voodoo | Yes | No | No | No |
| Software Voodoo | Yes | Yes | Yes | Yes |
| liblhasa (LHA) | Yes | Yes | No | No |
| CGO required | Yes | Yes | Yes | No |
| Cross-compile | No | No | No | Yes |

## Direct go build

```bash
# Quick build without compression (development)
go build ./...

# With specific tags
go build -tags novulkan .
go build -tags headless .
CGO_ENABLED=0 go build -tags "novulkan headless" .
```

---

# 4. Version and Feature Introspection

```bash
# Version, commit, build date, Go version, OS/arch
./bin/IntuitionEngine -version

# Compiled-in feature flags and build profile
./bin/IntuitionEngine -features
```

---

# 5. Toolchain Matrix

| CPU Core | Assembler | File Extension | Install |
|----------|-----------|----------------|---------|
| IE32 | `ie32asm` (built-in) | `.iex` | `make ie32asm` |
| IE64 | `ie64asm` (built-in) | `.ie64` | `make ie64asm` |
| M68K | `vasmm68k_mot` | `.ie68` | [VASM](http://sun.hasenbraten.de/vasm/) (`make CPU=m68k SYNTAX=mot`) |
| Z80 | `vasmz80_std` | `.ie80` | [VASM](http://sun.hasenbraten.de/vasm/) (`make CPU=z80 SYNTAX=std`) |
| 6502 | `ca65` / `ld65` | `.ie65` | [cc65](https://cc65.github.io/) |
| x86 | `nasm` | `.ie86` | [NASM](https://www.nasm.us/) |

### Companion Tools

| Tool | Purpose | Install |
|------|---------|---------|
| `ie32to64` | Convert IE32 binaries to IE64 format | `make ie32to64` |
| `ie64dis` | Disassemble IE64 binaries | `make ie64dis` |

See [IE32 to IE64 Migration](docs/ie32to64.md) for converter documentation.

### Assembling Programs

```bash
# IE32
./bin/ie32asm program.asm                    # Produces program.iex

# IE64
./bin/ie64asm program.asm                    # Produces program.ie64

# M68K
vasmm68k_mot -Fbin -m68020 -devpac -o out.ie68 input.asm

# 6502 (via Makefile helper)
make ie65asm SRC=assembler/program.asm       # Produces program.ie65

# Z80 (via Makefile helper)
make ie80asm SRC=assembler/program.asm       # Produces program.ie80

# x86
nasm -f bin -o program.ie86 program.asm
```

---

# 6. Development Workflow

A typical development cycle:

1. Write assembly (or BASIC) code
2. Assemble:
   ```bash
   ./bin/ie32asm program.asm
   ```
3. Run:
   ```bash
   ./bin/IntuitionEngine -ie32 program.iex
   ```
4. Debug with the machine monitor (F9) or console output
5. Iterate

### Creating New Demonstrations

When adding new test demonstrations:

1. Use descriptive names that indicate what capability is being showcased
2. Include detailed logging explaining expected effects and technical aspects
3. Structure demos to progress from basic to complex effects
4. Clean up resources properly when complete
5. Add informative comments about algorithms and techniques used

---

# 7. Include Files

The `assembler/` directory provides hardware definition include files for each CPU architecture. The `sdk/include/` directory contains synced copies for the SDK.

| File | CPU | Assembler | Description |
|------|-----|-----------|-------------|
| `ie32.inc` | IE32 | ie32asm | Hardware constants (`.equ` directives) |
| `ie64.inc` | IE64 | ie64asm | Hardware constants and macros |
| `ie64_fp.inc` | IE64 | ie64asm | IEEE 754 FP32 math (hardware FPU wrappers) |
| `ie65.inc` | 6502 | ca65 | Hardware constants, macros, zero page allocation |
| `ie65.cfg` | 6502 | ld65 | Linker configuration |
| `ie68.inc` | M68K | vasmm68k_mot | Hardware constants with M68K macros |
| `ie80.inc` | Z80 | vasmz80_std | Hardware constants with Z80 macros |
| `ie86.inc` | x86 | NASM | Hardware constants, port I/O, VGA registers |

All include files provide:
- **Video registers**: VIDEO_CTRL, VIDEO_MODE, VIDEO_STATUS, blitter, copper, raster band
- **Audio registers**: PSG, POKEY, SID, TED raw registers and player control
- **Memory constants**: VRAM_START, SCREEN_W/H, LINE_BYTES
- **Blitter operations**: BLT_OP_COPY, BLT_OP_FILL, BLT_OP_LINE, BLT_OP_MASKED, BLT_OP_ALPHA, BLT_OP_MODE7
- **Copper opcodes**: COP_WAIT_MASK, COP_MOVE_RASTER_*, COP_END
- **Timer registers**: TIMER_CTRL, TIMER_COUNT, TIMER_RELOAD
- **Coprocessor helpers**: coproc_start, coproc_stop, coproc_enqueue, coproc_poll, coproc_wait

### Include File Stability

The `sdk/include/` headers define the stable hardware register map for v1.x. The canonical source of truth is `assembler/*.inc` in the main repository. SDK copies are synced by `make sdk` and at release time.

### 8-Bit CPU Banking

The 6502 and Z80 use a banking system to access the full 32MB address space:

| Window | Address Range | Purpose | Bank Register |
|--------|---------------|---------|---------------|
| Bank 1 | $2000-$3FFF | Sprite data | BANK1_REG_LO/HI |
| Bank 2 | $4000-$5FFF | Font data | BANK2_REG_LO/HI |
| Bank 3 | $6000-$7FFF | General data | BANK3_REG_LO/HI |
| VRAM | $8000-$BFFF | Video memory (16KB) | VRAM_BANK_REG |

---

# 8. Running Programs

### CPU Modes

```bash
# Default: start EhBASIC on IE64
./bin/IntuitionEngine

# Run programs on specific CPU cores
./bin/IntuitionEngine -ie32 program.iex
./bin/IntuitionEngine -ie64 program.ie64
./bin/IntuitionEngine -m68k program.ie68
./bin/IntuitionEngine -z80 program.ie80
./bin/IntuitionEngine -x86 program.ie86
./bin/IntuitionEngine -m6502 program.bin
./bin/IntuitionEngine -m6502 --load-addr 0x0600 --entry 0x0600 program.bin

# EhBASIC interpreter
./bin/IntuitionEngine -basic              # Embedded image (requires make basic)
./bin/IntuitionEngine -basic-image file   # Custom BASIC binary
./bin/IntuitionEngine -term               # Console terminal (no GUI window)
```

### Running from EhBASIC

Programs can also be launched from the BASIC interpreter prompt using `RUN`:

```basic
RUN "program.iex"                         : REM Load and run IE32 binary
RUN "demo.ie64"                           : REM Load and run IE64 binary
RUN "game.ie68"                           : REM Load and run M68K binary
RUN "effect.ie80"                         : REM Load and run Z80 binary
RUN "intro.ie86"                          : REM Load and run x86 binary
RUN "test.ie65"                           : REM Load and run 6502 binary
```

The `RUN` command auto-detects the CPU core from the file extension.

### Music Playback

```bash
# PSG (AY-3-8910/YM2149)
./bin/IntuitionEngine -psg track.ym       # Atari ST YM format
./bin/IntuitionEngine -psg track.ay       # ZXAYEMUL (with embedded Z80 player)
./bin/IntuitionEngine -psg track.vgm      # VGM stream
./bin/IntuitionEngine -psg track.vgz      # VGM compressed
./bin/IntuitionEngine -psg track.sndh     # Atari ST SNDH (with embedded M68K code)
./bin/IntuitionEngine -psg+ track.ym      # Enhanced audio

# SID (Commodore 64)
./bin/IntuitionEngine -sid tune.sid       # PSID/RSID playback
./bin/IntuitionEngine -sid+ tune.sid      # Enhanced audio
./bin/IntuitionEngine -sid-pal tune.sid   # PAL timing
./bin/IntuitionEngine -sid-ntsc tune.sid  # NTSC timing

# POKEY (Atari 8-bit)
./bin/IntuitionEngine -pokey track.sap
./bin/IntuitionEngine -pokey+ track.sap   # Enhanced audio

# TED (Commodore Plus/4)
./bin/IntuitionEngine -ted track.prg

# AHX (Amiga)
./bin/IntuitionEngine -ahx module.ahx
./bin/IntuitionEngine -ahx+ module.ahx   # Enhanced with stereo spread
```

### Enhanced Audio Modes (PLUS)

The `+` variants (PSG+, SID+, POKEY+, TED+, AHX+) provide:
- 4x oversampling
- Second-order Butterworth lowpass filtering
- Subtle drive saturation
- Allpass diffuser room ambience

SID+ additionally preserves per-channel filter sweeps. AHX+ adds authentic Amiga stereo panning (L-R-R-L) and hardware PWM.

### Display Options

```bash
./bin/IntuitionEngine -width 1024 -height 768 -scale 2
./bin/IntuitionEngine -perf                # Enable MIPS reporting
```

### Runtime Controls

| Key | Action |
|-----|--------|
| F9 | Machine monitor (step-through debugger) |
| F10 | Hard reset |
| F12 | Toggle runtime status bar |

---

# 9. Testing

### Running Tests

```bash
# All tests (headless recommended for CI)
go test -tags headless ./...

# Single test
go test -tags headless -run TestName

# M68K Harte test suite
make testdata-harte                       # Download test data (one-time)
make test-harte                           # Full suite (~30 min)
make test-harte-short                     # Sampling mode (~5 min)

# Long-running demonstrations
go test -tags audiolong -run TestSineWave_BasicWaveforms
go test -tags videolong -run TestFireEffect
```

### Audio Demonstration Tests

| Test | Description |
|------|-------------|
| TestSineWave_BasicWaveforms | Pure sine wave generation |
| TestSquareWave_DutyCycle | Variable duty cycle |
| TestNoiseTypes | White, periodic, metallic noise |
| TestADSR_Envelope | Envelope timing accuracy |
| TestFilterModes | LP/HP/BP filter demonstration |

### Video Demonstration Tests

| Test | Description |
|------|-------------|
| TestFireEffect | Cellular automata fire |
| TestPlasmaWaves | Dynamic colour plasma |
| TestMetaballs | Organic blob rendering |
| TestTunnelEffect | Texture-mapped tunnel |
| TestRotozoom | Rotation and zoom effect |
| TestStarfield | 3D star simulation |
| TestMandelbrot | Fractal visualisation |
| TestParticles | Physics-based particles |

---

# 10. Debugging

### Machine Monitor (F9)

Press F9 during execution to enter the step-through debugger. Supports all 6 CPU types with:
- Breakpoints with hit counts and conditions
- Trace logging and backstep history
- Run-until with conditional stops
- I/O register viewer for all hardware chips
- Clipboard paste (Ctrl+Shift+V)

See [docs/iemon.md](docs/iemon.md) for the full machine monitor reference.

### Console Debug Output

Write to the debug output register (`0xF0700`) to print values during execution:

| CPU | Instruction |
|-----|-------------|
| IE32 | `STORE A, @0xF0700` |
| IE64 | `store.l r1, 0xF0700(r0)` |
| M68K | `move.l d0,$F0700.l` |
| Z80 | `ld ($F700),a` |
| 6502 | `sta $F700` |
| x86 | `mov [0xF0700], eax` |

### Hardware Status Registers

| Register | Address | Purpose |
|----------|---------|---------|
| VIDEO_STATUS | `$F0008` | VBlank flag (bit 1) |
| BLT_STATUS | `$F0044` | Blitter busy (bit 1) |
| PSG_PLAY_STATUS | `$F0C1C` | PSG player status |
| SID_PLAY_STATUS | `$F0E2C` | SID player status |
| SAP_PLAY_STATUS | `$F0D1C` | SAP player status |

---

# 11. Platform Support

| Platform | Status | Graphics | Audio | Notes |
|----------|--------|----------|-------|-------|
| **Linux x86_64** | Official | Ebiten | Oto | Primary development platform |
| **Linux aarch64** | Official | Ebiten | Oto | |
| **Windows x86_64** | Experimental | Ebiten | Oto | Use `novulkan` profile |
| **Windows ARM64** | Experimental | Ebiten | Oto | Use `novulkan` profile |

### Graphics Backend

**Ebiten** (primary): hardware-accelerated rendering via OpenGL/Metal/DirectX, automatic display scaling, VSync synchronisation, cross-platform window management.

**Headless** (testing): stub backend for CI/batch processing without a display.

### Audio Backend

**Oto** (primary): cross-platform audio output, low-latency playback (~20ms), 44.1kHz stereo.

**Headless** (testing): stub backend, no audio output.

---

# 12. Packaging and Distribution

### Desktop Integration

```bash
# Install .desktop entry and MIME type (system-wide, requires root)
make install-desktop-entry

# Set as default handler for .ie* files (per-user)
make set-default-handler
```

### Release Artifacts

Build release archives with `make release-all` (or individual targets like `make release-linux`). Each target builds both amd64 and arm64 archives with embedded EhBASIC and pre-assembled SDK demos.

Each archive contains: `IntuitionEngine`, `ie32asm`, `ie64asm`, `ie32to64`, `README.md`, `CHANGELOG.md`, `DEVELOPERS.md`, the full `docs/` directory, and the full `sdk/` directory with pre-assembled demos.

Additional release targets:

| Target | Description |
|--------|-------------|
| `make release-src` | Source archive via `git archive` (`.tar.xz`) |
| `make release-sdk` | Standalone SDK archive (`.zip`) |

| Platform | Format | Profile |
|----------|--------|---------|
| Linux (native arch) | `.tar.xz` | full |
| Windows amd64, arm64 | `.zip` | novulkan |

`make release-all` builds all of the above plus the source and SDK archives, and produces `SHA256SUMS` covering all artifacts.

---

# 13. Contributing

### Repository Structure

```
cpu_*.go              CPU emulators (IE32, IE64, M68K, Z80, 6502, x86)
video_*.go            Video chips (VideoChip, VGA, ULA, TED, ANTIC, Voodoo)
audio_chip.go         Audio engine core
*_engine.go           Sound chip engines (PSG, SID, POKEY, TED)
*_player.go           Music file format players
machine_bus.go        Memory mapping and I/O dispatch
main.go               CLI entry point and runtime
assembler/            IE32/IE64 assembler source and include files
sdk/                  Curated SDK with examples and build scripts
docs/                 Technical documentation
```

### Code Style

- **Go**: standard `gofmt`
- **Assembly**: Motorola syntax for M68K, cc65 syntax for 6502, NASM for x86

### Testing Guidelines

- Run tests with `-tags headless` to avoid display/audio dependencies
- Use `go test -v -run TestName` for targeted testing
- Long-running demos require `audiolong` or `videolong` tags
- See `make help` for all test-related targets

### Build Verification Checklist

Before submitting changes, verify:

```bash
# Code compiles
go build ./...

# Core tests pass
go test -tags headless ./...

# Build profiles still work
go build -tags novulkan .
CGO_ENABLED=0 go build -tags "novulkan headless" .
```
