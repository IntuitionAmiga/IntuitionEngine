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
14. [EmuTOS Integration](#14-emutos-integration)
15. [AROS Integration](#15-aros-integration)
16. [JIT Compilation](#16-jit-compilation)

---

# 1. Prerequisites

- **Go 1.26** or later
- `sstrip` and `upx` for binary optimisation (modify Makefile to skip if unavailable)
- No extra system packages are required for the default runtime path (Ebiten + Oto)

Optional dependencies for advanced features:
- **Vulkan** SDK/driver: required for the Voodoo Vulkan rasteriser path (not needed with `novulkan` profile)

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
make emutos            # VM with embedded EmuTOS ROM
make basic-emutos      # VM with embedded EhBASIC + EmuTOS ROM
make emutos-rom        # Build EmuTOS ROM from source (auto-clones if needed)

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
sdk/bin/ie32asm         # The IE32 assembler
sdk/bin/ie64asm         # The IE64 assembler
sdk/bin/ie32to64        # The IE32-to-IE64 converter
sdk/bin/ie64dis         # The IE64 disassembler
```

Version metadata (version, git commit, build date) is automatically injected via ldflags.

---

# 3. Build Profiles and Tags

## Profiles

| Profile | Command | CGO | Description |
|---------|---------|-----|-------------|
| **full** (default) | `make` | Yes | All features: Vulkan Voodoo, Ebiten display, Oto audio |
| **novulkan** | `make novulkan` | Yes | Software Voodoo rasteriser only, no Vulkan SDK needed |
| **headless** | `make headless` | Yes | No display, no audio, no Vulkan (CI/testing) |
| **headless-novulkan** | `make headless-novulkan` | No | Fully portable `CGO_ENABLED=0` build, cross-compile safe |

## Build Tags

| Tag | Effect |
|-----|--------|
| `headless` | Disable GUI/audio/video backends (stubs only) |
| `novulkan` | Disable Vulkan backend, use software Voodoo rasteriser |
| `embed_basic` | Embed pre-assembled EhBASIC binary for `-basic` flag |
| `embed_emutos` | Embed EmuTOS ROM image for `-emutos` flag and BASIC `EMUTOS` command |
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

See [IE32 to IE64 Migration](sdk/docs/ie32to64.md) for converter documentation.

### Assembling Programs

```bash
# IE32
sdk/bin/ie32asm program.asm                  # Produces program.iex

# IE64
sdk/bin/ie64asm program.asm                  # Produces program.ie64

# M68K
vasmm68k_mot -Fbin -m68020 -devpac -o out.ie68 input.asm

# 6502 (via Makefile helper)
make ie65asm SRC=sdk/examples/asm/program.asm  # Produces program.ie65

# Z80 (via Makefile helper)
make ie80asm SRC=sdk/examples/asm/program.asm  # Produces program.ie80

# x86
nasm -f bin -o program.ie86 program.asm
```

---

# 6. Development Workflow

A typical development cycle:

1. Write assembly (or BASIC) code
2. Assemble:
   ```bash
   sdk/bin/ie32asm program.asm
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

The `sdk/include/` directory provides hardware definition include files for each CPU architecture, plus the EhBASIC interpreter modules.

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

The `sdk/include/` directory is the canonical location for all include files.

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
# PSG (AY-3-8910/YM2149; VGM also supports SN76489)
./bin/IntuitionEngine -psg track.ym       # Atari ST YM format
./bin/IntuitionEngine -psg track.ay       # ZXAYEMUL (ZX Spectrum/Amstrad CPC/MSX auto-detected)
./bin/IntuitionEngine -psg track.vgm      # VGM stream (AY-3-8910 + SN76489)
./bin/IntuitionEngine -psg track.vgz      # VGM compressed (AY-3-8910 + SN76489)
./bin/IntuitionEngine -psg track.sndh     # Atari ST SNDH (with embedded M68K code)
./bin/IntuitionEngine -psg track.vtx      # Vortex Tracker (LHA-compressed YM)
./bin/IntuitionEngine -psg track.pt3      # ProTracker 3 (Z80 tracker)
./bin/IntuitionEngine -psg track.stc      # Sound Tracker Compiled (Z80 tracker)
./bin/IntuitionEngine -psg track.pt2      # ProTracker 2 (Z80 tracker)
./bin/IntuitionEngine -psg track.pt1      # ProTracker 1 (Z80 tracker)
./bin/IntuitionEngine -psg track.sqt      # SQ-Tracker (Z80 tracker)
./bin/IntuitionEngine -psg track.asc      # ASC Sound Master (Z80 tracker)
./bin/IntuitionEngine -psg track.ftc      # Fast Tracker ZX (Z80 tracker)
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

### IE64 Benchmarks

The IE64 benchmark suite measures CPU throughput through both the interpreter and JIT compiler across five workload categories: integer ALU, floating-point, memory access, mixed, and subroutine call/return.

```bash
# Run all IE64 benchmarks (skip normal tests with -run='^$')
go test -tags headless -run='^$' -bench BenchmarkIE64_ -benchtime 3s -count 3 ./...

# Compare JIT vs interpreter
go test -tags headless -run='^$' -bench 'BenchmarkIE64_(ALU|FPU|Memory|Mixed|Call)' -benchtime 3s ./...

# Run only JIT benchmarks
go test -tags headless -run='^$' -bench 'BenchmarkIE64_.*_JIT' -benchtime 3s ./...

# Run only interpreter benchmarks
go test -tags headless -run='^$' -bench 'BenchmarkIE64_.*_Interpreter' -benchtime 3s ./...
```

Benchmarks report ns/op and instructions/op. MIPS can be derived: `MIPS = instructions/op / ns/op * 1000`. JIT benchmarks skip automatically on platforms without JIT support. See `ie64_benchmark_test.go` for detailed documentation of each workload and its instruction mix.

Reference results on Intel Core i5-8365U @ 1.60 GHz (x86-64 JIT, `benchtime 3s`):

| Workload | Interpreter | JIT | Speedup |
|---|---|---|---|
| ALU | 1,058 µs | 157 µs | 6.7x |
| FPU | 1,242 µs | 372 µs | 3.3x |
| Memory | 813 µs | 105 µs | 7.7x |
| Mixed | 1,227 µs | 159 µs | 7.7x |
| Call/Return | 583 µs | 7,036 µs | 0.08x |

The Call benchmark is intentionally JIT-hostile (JSR/RTS exit the native block on every call). See [sdk/docs/IE64_JIT.md](sdk/docs/IE64_JIT.md) for full analysis.

---

# 10. Debugging

### Machine Monitor (F9)

Press F9 during execution to enter the step-through debugger. Supports all 6 CPU types with:
- Breakpoints with hit counts and conditions
- Trace logging and backstep history
- Run-until with conditional stops
- I/O register viewer for all hardware chips
- Clipboard copy/cut/paste (Ctrl+Shift+C/X/V), text selection (Shift+Arrow), middle mouse paste

See [docs/iemon.md](sdk/docs/iemon.md) for the full machine monitor reference.

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

Build release archives with `make release-all` (or individual targets like `make release-linux`). Each target builds with embedded EhBASIC and EmuTOS ROM, plus pre-assembled SDK demos. The `EMUTOS` command is available at the BASIC prompt in release builds.

Each archive contains: `IntuitionEngine` at the root, `sdk/bin/` with `ie32asm`, `ie64asm`, `ie32to64`, `ie64dis`, plus `README.md`, `CHANGELOG.md`, `DEVELOPERS.md`, and the full `sdk/` directory with pre-assembled demos and documentation.

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
assembler/            Assembler tool source code (ie32asm, ie64asm, ie64dis, ie32to64)
sdk/                  Curated SDK with examples and build scripts
sdk/docs/             Technical documentation
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

---

# 14. EmuTOS Integration

EmuTOS runs on the IE M68K core with GEM desktop, GEMDOS filesystem interception, and timer-driven interrupts.

### Runtime Flags

| Flag | Description |
|------|-------------|
| `-emutos` | Boot embedded EmuTOS ROM (requires `embed_emutos` build tag) |
| `-emutos-image <path>` | Boot EmuTOS from external ROM image |
| `-emutos-drive <dir>` | Host directory mapped as GEMDOS drive U: (default: `~/`) |

### Key Files

| File | Purpose |
|------|---------|
| `emutos_loader.go` | ROM loading, timer/VBlank IRQ generation, GEMDOS setup |
| `emutos_embed.go` | `//go:embed` for ROM image (`embed_emutos` tag) |
| `emutos_noembed.go` | Nil ROM fallback (`!embed_emutos` tag) |
| `gemdos_intercept.go` | TRAP #1 filesystem interception (drive U: mapping) |
| `gemdos_intercept_constants.go` | GEMDOS function numbers, error codes, DTA layout |
| `sdk/emutos/` | C shim files for building EmuTOS against IE MMIO |
| `sdk/docs/ie_emutos.md` | Full integration guide |

### ProgramExecutor

File extensions `.tos` and `.img` are detected as EmuTOS mode. The `EMUTOS` command at the BASIC prompt triggers boot via the `emutosSentinel` path.

### Build

```bash
make emutos             # VM with embedded EmuTOS ROM
make basic-emutos       # VM with embedded EhBASIC + EmuTOS ROM
make emutos-rom         # Build ROM from source (auto-clones EmuTOS repo, needs GCC 13 m68k cross-compiler)
```

### GCC 13 -mshort Codegen Bug

EmuTOS compiled with `-mshort` (2-byte `size_t`) triggers a GCC 13 bug: pointer arithmetic in `win_start()` produces displacement `$10804` instead of `$0804` (off by `$10000`). The last WNODE's `w_next` points past the array into FNODE memory, causing a bus error on desktop folder view switch. Workaround: `fixWnodeChain()` in `gemdos_intercept.go` patches the WNODE chain after directory enumeration.

---

# 15. AROS Integration

AROS (Amiga Research Operating System) runs on the IE M68K core with a full Workbench desktop, Shell, and host filesystem access.

### Runtime Flags

| Flag | Description |
|------|-------------|
| `-aros` | Boot AROS from embedded or default ROM image |
| `-aros-image <path>` | Boot AROS from external ROM image |
| `-aros-drive <dir>` | Host directory mapped as IE: volume (default: `~/`) |

### Key Files

| File | Purpose |
|------|---------|
| `aros_loader.go` | ROM loading, memory layout, IRQ generation, input handling |
| `aros_dos_intercept.go` | DOS handler MMIO filesystem bridge (IE: volume) |
| `aros_dos_constants.go` | AmigaDOS action codes, error codes, lock types |
| `aros_audio_dma.go` | Paula-compatible 4-channel DMA audio |
| `aros_audio_constants.go` | Audio DMA register addresses |
| `aros_embed.go` | `//go:embed` for ROM image (`embed_aros` tag) |
| `aros_noembed.go` | Nil ROM fallback (`!embed_aros` tag) |

### ProgramExecutor

The `AROS` command at the BASIC prompt triggers boot via the `arosSentinel` path. AROS mode is selected by CLI flags (`-aros`, `-aros-image`), not file extension.

### Memory Layout

| Region | Range | Size |
|--------|-------|------|
| Chip RAM A | `0x000000-0x09DFFF` | 630KB |
| Chip RAM B | `0x200000-0x6FFFFF` | 5MB |
| ROM | `0x600000-0x7FFFFF` | 2MB |
| Fast RAM | `0x800000-0x1DFFFFF` | 22MB |
| VRAM | `0x1E00000-0x1FFFFFF` | 2MB |

Total: 27.6MB (5.6MB chip + 22MB fast)

### DOS Handler

The DOS handler at MMIO `0xF2220-0xF225F` bridges AmigaDOS packet protocol to the host filesystem. Supported actions: LOCATE_OBJECT, FREE_LOCK, COPY_DIR (DupLock), EXAMINE_OBJECT, EXAMINE_NEXT, READ, WRITE, SEEK, FINDUPDATE, FINDINPUT, FINDOUTPUT, END, DELETE_OBJECT, RENAME_OBJECT, CREATE_DIR, SET_FILE_SIZE, SAME_LOCK, PARENT.

### Build

```bash
make aros-rom           # Build AROS ROM + filesystem from source
make aros               # Build VM with embedded AROS ROM
```

---

# 16. JIT Compilation

The IE64 CPU core includes a JIT compiler that translates IE64 machine code into native ARM64 or x86-64 instructions at runtime. JIT is enabled by default on supported platforms (Linux/arm64, Linux/amd64) and can be disabled with `-nojit`.

For full technical details (register mappings, return-channel contract, I/O dual-path, FPU categories, backward branch budget, fallback rules), see [sdk/docs/IE64_JIT.md](sdk/docs/IE64_JIT.md).

## Running JIT Tests

```bash
# x86-64 backend tests (on amd64)
go test -v -run TestAMD64_ -tags headless ./...

# ARM64 backend tests (on arm64)
go test -v -run TestARM64_ -tags headless ./...

# JIT-vs-interpreter parity (verifies JIT matches interpreter output)
go test -v -run TestJIT_vs_Interpreter -tags headless ./...

# Shared infrastructure (block scanner, code cache, register analysis)
go test -v -run TestJIT_ -tags headless ./...
```

## JIT Source Files

| File | Purpose |
|------|---------|
| `jit_common.go` | JITContext, CodeBuffer, block scanner, register analysis, code cache |
| `jit_emit_arm64.go` | ARM64 code emitter (15 mapped registers) |
| `jit_emit_amd64.go` | x86-64 code emitter (5 mapped registers) |
| `jit_exec.go` | Dispatcher loop, timer handling |
| `jit_call.go` | Native code invocation via `runtime.cgocall` |
| `jit_mmap.go` | Executable memory allocation (mmap RWX) |

## M68020 JIT

The M68020 CPU core includes a JIT compiler (amd64/linux only; ARM64 planned). It handles variable-length instructions, big-endian memory with byte-swap, 12+ addressing modes, and a 5-bit condition code register (XNZVC). JIT is enabled by default on supported platforms and disabled with `-nojit`.

Key optimisations:
- **Block chaining**: Direct JMP rel32 between compiled blocks (BRA/JMP/JSR/BSR/RTS/Bcc/DBcc), avoiding Go dispatcher overhead. Budget counter (64 blocks) for interrupt safety.
- **Lazy CCR**: Defers flag extraction from host EFLAGS, using direct x86 Jcc for M68K branch conditions. Eliminates ~12 instructions per flag-setter.
- **2-entry MRU RTS cache**: Fast subroutine returns without dispatcher round-trip.

For full technical details, see [sdk/docs/M68K_JIT.md](sdk/docs/M68K_JIT.md).

### Running M68K JIT Tests

```bash
# Infrastructure tests (scanner, length calc, liveness, chain infrastructure)
go test -v -run TestM68KJIT_ -tags headless ./...

# x86-64 emitter tests
go test -v -run TestM68KJIT_AMD64_ -tags headless ./...

# Integration tests (full dispatcher with chaining)
go test -v -run TestM68KJIT_Exec_ -tags headless ./...

# Benchmarks (JIT vs interpreter)
go test -tags headless -run='^$' -bench 'BenchmarkM68K_.*_(JIT|Interpreter)' -benchtime 3s ./...
```

### M68K JIT Source Files

| File | Purpose |
|------|---------|
| `jit_m68k_common.go` | M68KJITContext (with chain/RTS cache fields), block scanner, instruction length calculator |
| `jit_m68k_emit_amd64.go` | x86-64 code emitter: chain entry/exit, lazy CCR, 4 mapped registers |
| `jit_m68k_exec.go` | Dispatcher: chain patching, budget init, RTS cache, STOP/interrupt semantics |
| `jit_m68k_dispatch.go` | Platform routing |
| `jit_common.go` | JITBlock (with chainEntry/chainSlots), CodeCache.PatchChainsTo, chainSlot |
| `jit_mmap.go` | ExecMem + PatchRel32At for runtime chain patching |
