# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Intuition Engine — retro hardware emulator/VM with 6 CPU cores (IE32, IE64, M68K, 6502, Z80, x86), 5+ audio chips (SoundChip, PSG, SID, POKEY, TED, AHX), 6+ video chips (VideoChip, VGA, ULA, TED, ANTIC/GTIA, Voodoo 3D), copper coprocessor, DMA blitter, and Lua scripting. All emulator code lives in a flat `package main` in the root directory.
This project uses Go 1.26+ released in February 2026. We use modern Go syntax, paradigms and coding style at all times.

## Build

```bash
make                    # Build VM to bin/, tools to sdk/bin/
make novulkan           # Build without Vulkan (software Voodoo only)
make headless           # Build without display/audio (CI/testing)
make headless-novulkan  # Fully portable CGO_ENABLED=0 build
make basic              # Build with embedded EhBASIC interpreter
make emutos             # Build with embedded EmuTOS ROM image
make ie32asm            # Build IE32 assembler
make ie64asm            # Build IE64 assembler
go build ./...          # Quick dev build without compression
```

## Test

```bash
go test -tags headless ./...                                      # All tests (ALWAYS use -tags headless)
go test -tags headless -run TestName                              # Single test
go test -tags "headless m68k_test" -v -run TestHarte68000 -short  # M68K tests (sampling mode)
```

**Always use `-tags headless`** to avoid Ebiten/X11 display dependencies. M68K-specific tests additionally need the `m68k_test` tag.

## Assemble

```bash
sdk/bin/ie32asm sdk/examples/asm/program.asm                          # IE32
sdk/bin/ie64asm sdk/examples/asm/program.asm                          # IE64
vasmm68k_mot -Fbin -m68020 -devpac -o out.ie68 input.asm             # M68K (external)
make ie65asm SRC=sdk/examples/asm/program.asm                         # 6502 (needs cc65)
make ie80asm SRC=sdk/examples/asm/program.asm                         # Z80 (needs vasm)
```

## Build Tags

| Tag | Effect |
|-----|--------|
| `headless` | Stubs video (Ebiten), audio (Oto), Vulkan, clipboard — no CGO needed with `novulkan` |
| `novulkan` | Software-only Voodoo, no Vulkan SDK required |
| `embed_basic` | Embeds EhBASIC binary via `//go:embed` |
| `embed_emutos` | Embeds EmuTOS ROM via `//go:embed` |
| `m68k_test` | Enables M68K-specific test files |
| `audiolong` / `videolong` | Long-running chip tests |

## Architecture

### Core Pattern: MachineBus + MMIO

`MachineBus` (`machine_bus.go`) is the system backbone — 32MB contiguous memory with memory-mapped I/O. All peripherals register via `MapIO(start, end, readFn, writeFn)`. The bus uses a `ioPageBitmap []bool` fast path (page = 256 bytes) — non-I/O pages use direct unsafe pointer access with zero dispatch overhead.

### CPU Interface

All CPUs implement `EmulatorCPU` (`emulator_cpu.go`): `LoadProgram`, `Reset`, `Execute`, `Stop`, `StartExecution`. The active CPU is selected by file extension (`.iex`/`.ie32` → IE32, `.ie64` → IE64, `.ie68` → M68K, `.ie65` → 6502, `.ie80` → Z80, `.ie86` → x86, `.tos`/`.img` → EmuTOS).

### Video Compositing

All video chips implement `VideoSource` (`video_interface.go`): `GetFrame`, `IsEnabled`, `GetLayer`, `GetDimensions`, `SignalVSync`. The `VideoCompositor` composites enabled sources by Z-order layer. Voodoo and all retro video chips use a lock-free triple-buffer protocol for `GetFrame()` — calling `GetFrame()` does an atomic swap, so call it once and save the result.

### Program Executor

`ProgramExecutor` (`program_executor.go`) is an MMIO-driven program loader at `0xF2320-0xF233F`. It detects CPU mode from file extension and orchestrates full CPU mode switching via `runProgramWithFullReset` (stop CPU → stop compositor → recreate runner → reset all chips → reload → restart).

### Concurrency Model

Lock-free with atomics for hot paths. CPU `running` is `atomic.Bool`. No mutexes on CPU execution loops. Video chips use `sync.Mutex` (field named `mu`). I/O callbacks protect their own state. Memory bus mapping is immutable after `MapIO()` — no lock needed for dispatch. Reset() racing with Execute() is accepted by design (matches real hardware RESET behavior).

## File Naming Conventions

| Pattern | Contents |
|---------|----------|
| `cpu_*.go` | CPU emulators and runners |
| `video_*.go` | Video chips, compositor, backends |
| `audio_*.go` | Audio backend and waveform tests |
| `*_engine.go` | Chip sound engines (PSG, SID, POKEY, TED, AHX) |
| `*_player.go` | Music file format players |
| `*_constants.go` | Per-chip register address constants |
| `registers.go` | Central I/O register address map and memory map |
| `machine_bus.go` | Memory bus, I/O dispatch |
| `program_executor.go` | MMIO program loader |
| `runtime_helpers.go` | CPU mode detection, factory functions |
| `component_reset.go` | Reset() methods for all hardware |
| `emulator_cpu.go` | EmulatorCPU interface + global active state |
| `main.go` | Entry point, flag parsing, all I/O region mapping |
| `assembler/` | Assembler/disassembler tools (separate build tags) |
| `cmd/ie32to64/` | IE32-to-IE64 converter (separate package) |
| `sdk/` | Documentation, include files, examples, player routines |

## Code Style

- Go: standard gofmt, no extra comments/docstrings on unchanged code. Always run go fmt ./... and go fix ./... after creating or modifying .go files!
- Assembly: Motorola syntax for M68K/IE64, cc65 syntax for 6502
- Little-endian only — enforced at compile time by `le_check.go` / `be_unsupported.go`

## Key Memory Map Ranges

| Range | Device |
|-------|--------|
| `0x00000-0x9EFFF` | Main RAM |
| `0xA0000-0xBFFFF` | VGA VRAM/Text |
| `0xF0000-0xF0054` | VideoChip |
| `0xF0700-0xF07FF` | Terminal/Serial MMIO |
| `0xF0800-0xF0B3F` | SoundChip |
| `0xF0C00-0xF0FFF` | PSG / POKEY / SID / TED |
| `0xF1000-0xF13FF` | VGA Registers |
| `0xF2000-0xF21B7` | ULA / ANTIC+GTIA |
| `0xF2200-0xF237F` | File I/O / Media / Program Executor / Coprocessor |
| `0xF4000-0xF43FF` | Voodoo 3D |
| `0x100000-0x4FFFFF` | Video RAM (4MB) |

Full map in `registers.go`.

## External Tools

- `vasmm68k_mot` / `vasmz80_std` — VASM assemblers (http://sun.hasenbraten.de/vasm/)
- `ca65` / `ld65` — cc65 toolchain for 6502
- `sstrip`, `upx` — Binary compression (optional, release builds only)
- We use gcc 13 m68k cross-compiler for EmuTOS. Later versions do not compile it correctly
- **Known GCC 13 -mshort bug**: pointer arithmetic codegen can produce displacements off by $10000 (65536). EmuTOS `win_start()` compiles `(pw-1)->w_next = NULL` with displacement $10804 instead of $0804. Workaround: `fixWnodeChain()` in `gemdos_intercept.go` patches the WNODE chain after directory enumeration
- `../EmuTOS` - contains the EmuTOS source tree
