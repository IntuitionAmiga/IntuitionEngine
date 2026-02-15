```ansi
██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░
```
# Intuition Engine System Documentation
## Complete Technical Reference & User Guide
## (c) 2024 - 2026 Zayn Otley
## https://github.com/intuitionamiga/IntuitionEngine
## License: GPLv3 or later
## https://www.youtube.com/@IntuitionAmiga/
[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/M4M61AHEFR)

**See also: [TUTORIAL.md](docs/TUTORIAL.md)** - Step-by-step guide to building a complete demoscene intro with multiple CPU architectures.

# Table of Contents

1. [System Overview](#1-system-overview)
   - CPU Options
   - Audio Capabilities
   - Video System
   - Quick Start
   - [1.7 Machine Monitor](#17-machine-monitor)
2. [Architecture](#2-architecture)
   - 2.1 Unified Memory
   - 2.2 Hardware I/O
3. [Memory Map & Hardware Registers](#3-memory-map--hardware-registers-detailed)
   - 3.1 System Vector Table
   - 3.2 Program Space
   - 3.3 Video Registers
   - 3.4 Timer Registers
   - 3.5 Sound Registers (Legacy and Flexible)
   - 3.6 PSG Registers
   - 3.7 POKEY Registers
   - 3.8 SID Registers
   - 3.9 TED Registers
   - 3.10 TED Video Chip Registers
   - 3.11 AHX Module Player Registers
   - 3.12 Hardware I/O Memory Map by CPU
   - 3.13 VGA Video Chip
   - 3.14 ULA Video Chip (ZX Spectrum)
   - 3.15 ANTIC Video Chip (Atari 8-bit)
   - 3.16 GTIA Color Control (Atari 8-bit)
   - 3.17 Coprocessor Subsystem
   - 3.18 Voodoo 3D Graphics
4. [IE32 CPU Architecture](#4-ie32-cpu-architecture)
   - 4.1 Register Set
   - 4.2 Status Flags
   - 4.3 Addressing Modes
   - 4.4 Instruction Format
   - 4.5 Instruction Set
   - 4.6 Memory and I/O Integration
   - 4.7 Interrupt Handling
   - 4.8 Compatibility Notes
5. [MOS 6502 CPU](#5-mos-6502-cpu)
   - 5.1 Register Set
   - 5.2 Status Flags
   - 5.3 Addressing Modes
   - 5.4 Instruction Set
   - 5.5 Memory and I/O Integration
   - 5.6 Interrupts and Vectors
   - 5.7 Compatibility Notes
6. [Zilog Z80 CPU](#6-zilog-z80-cpu)
   - 6.1 Register Set
   - 6.2 Status Flags
   - 6.3 Addressing Modes
   - 6.4 Instruction Set
   - 6.5 Memory and I/O Integration
   - 6.6 Interrupts
   - 6.7 Compatibility Notes
7. [Motorola 68020 CPU with FPU](#7-motorola-68020-cpu-with-fpu)
   - 7.1 Register Set
   - 7.2 Status Flags
   - 7.3 Addressing Modes
   - 7.4 Instruction Set
   - 7.5 FPU (68881/68882) Features
   - 7.6 Memory and I/O Integration
   - 7.7 Interrupts and Exceptions
   - 7.8 Compatibility Notes
8. [Intel x86 CPU (32-bit)](#8-intel-x86-cpu-32-bit)
   - 8.1 Register Set
   - 8.2 Status Flags
   - 8.3 Addressing Modes
   - 8.4 Instruction Set
   - 8.5 Memory and I/O Integration
   - 8.6 Interrupts
   - 8.7 Compatibility Notes
9. [IE64 CPU Architecture](#9-ie64-cpu-architecture)
   - 9.1 Register Set
   - 9.2 Instruction Format
   - 9.3 Addressing Modes
   - 9.4 Instruction Set
   - 9.5 FPU Logic
   - 9.6 Memory and I/O Integration
   - 9.7 Interrupt Handling
   - 9.8 Compatibility Notes
   - 9.9 EhBASIC IE64
10. [Assembly Language Reference](#10-assembly-language-reference)
    - 10.1 Basic Program Structure
    - 10.2 Assembler Directives
    - 10.3 Memory Access Patterns
    - 10.4 Stack Usage
    - 10.5 Interrupt Handlers
11. [Sound System](#11-sound-system)
    - Custom Audio Chip Overview
    - 11.1 Sound Channel Types
    - 11.2 Modulation System
    - 11.3 Global Effects
    - 11.4 PSG Sound Chip (AY-3-8910/YM2149)
    - 11.5 POKEY Sound Chip
    - 11.6 SID Sound Chip
    - 11.7 TED Sound Chip
    - 11.8 AHX Sound Chip
12. [Video System](#12-video-system)
    - 12.1 Display Modes
    - 12.2 Framebuffer Organisation
    - 12.3 Dirty Rectangle Tracking
    - 12.4 Double Buffering and VBlank Synchronisation
    - 12.5 Direct VRAM Access Mode
    - 12.6 Copper List Executor
    - 12.7 DMA Blitter
    - 12.8 Raster Band Fill
    - 12.9 Video Compositor
13. [Developer's Guide](#13-developers-guide)
    - 13.1 Development Environment Setup
    - 13.2 Building the System
    - 13.3 Development Workflow
    - 13.4 Assembler Include Files
    - 13.5 Debugging Techniques
14. [Implementation Details](#14-implementation-details)
    - 14.1 CPU Emulation
    - 14.2 Memory Architecture
    - 14.3 Audio System Architecture
15. [Platform Support](#15-platform-support)
    - 15.1 Supported Platforms
    - 15.2 Graphics Backends
    - 15.3 Audio Backends
    - 15.4 Runtime UI
16. [Running Demonstrations](#16-running-demonstrations)
    - 16.1 Quick Start
    - 16.2 Audio Demonstrations
    - 16.3 Visual Demonstrations
    - 16.4 CPU Test Suites
    - 16.5 Available Demonstrations
17. [Building from Source](#17-building-from-source)
    - 17.1 Prerequisites
    - 17.2 Build Commands
    - 17.3 Build Tags
    - 17.4 Development Workflow
    - 17.5 Creating New Demonstrations
18. [SDK Developer Package](#18-sdk-developer-package)

# 1. System Overview

The Intuition Engine is a virtual machine that emulates a complete retro-style computer system. It is a modern 64-bit RISC reimagining of the Commodore, Atari, Sinclair and IBM 8/16/32-bit home computers, with IE64 as the default core and five additional CPU cores.

## CPU Options

| CPU | Architecture | Registers | Features |
|-----|--------------|-----------|----------|
| **IE32** | 32-bit RISC | 16 general-purpose (A-H, S-W, X-Z) | Fixed 8-byte instructions, simple and fast |
| **M68K** | 32-bit CISC | 8 data (D0-D7), 8 address (A0-A7) | 95%+ instruction coverage, FPU support |
| **Z80** | 8-bit | AF, BC, DE, HL + alternates, IX, IY | Full instruction set, interrupt modes |
| **6502** | 8-bit | A, X, Y | NMOS instruction set, zero page optimisation |
| **x86** | 32-bit | EAX-EDX, ESI, EDI, EBP, ESP | 8086 instructions + 32-bit registers, flat memory model |
| **IE64** | 64-bit RISC | 32 general-purpose (R0=zero, R31=SP) | Native FP32 FPU, compare-and-branch, no flags register |

Default core: **IE64**. Additional cores: **IE32, M68K, x86, Z80, 6502**.

## Audio Capabilities

**Custom Synthesizer:**
- 5 dedicated waveform channels (square, triangle, sine, noise, sawtooth)
- 4 flexible channels with selectable waveforms
- ADSR envelopes, PWM, frequency sweep, hard sync, ring modulation
- Global filter (LP/HP/BP), overdrive, reverb
- 44.1kHz, 32-bit floating-point processing

**Classic Sound Chips (register-mapped to custom synth):**
- **AY/YM/PSG** (AY-3-8910/YM2149) - Supports .ym, .ay, .vgm, .sndh playback
- **POKEY** (Atari) - Supports .sap playback
- **SID** (6581/8580) - Supports .sid playback
- **Amiga AHX** module playback

## Video System

- Resolutions: 640×480, 800×600, 1024×768
- 32-bit RGBA framebuffer with double buffering
- Copper coprocessor for raster effects
- DMA blitter for fast copy/fill/line operations
- Dirty rectangle tracking for efficient updates
- Engines/chips: **IEVideoChip**, **VGA**, **ULA**, **TED video**, **ANTIC/GTIA**, **3DFX Voodoo**

## Quick Start

```bash
# Default: start EhBASIC on IE64
./bin/IntuitionEngine

# Run IE32 program
./bin/IntuitionEngine -ie32 program.iex

# Run M68K program
./bin/IntuitionEngine -m68k program.ie68

# Run x86 program
./bin/IntuitionEngine -x86 program.ie86

# Run IE64 program
./bin/IntuitionEngine -ie64 program.ie64

# Run EhBASIC interpreter
./bin/IntuitionEngine -basic

# Run EhBASIC with console terminal (no GUI window)
./bin/IntuitionEngine -basic -term

# Play PSG music
./bin/IntuitionEngine -psg music.ym

# Play SID music
./bin/IntuitionEngine -sid music.sid

# Run with performance measurement (MIPS reporting)
./bin/IntuitionEngine -perf -m68k program.ie68

# Display options
./bin/IntuitionEngine -scale 2 -ie32 program.iex      # 2x window scaling
./bin/IntuitionEngine -fullscreen -m68k program.ie68   # Start in fullscreen (F11 to toggle)
./bin/IntuitionEngine -width 800 -height 600 -ie64 program.ie64  # Override output resolution

# Version information
./bin/IntuitionEngine -version

# List compiled feature flags and build profile
./bin/IntuitionEngine -features
```

CPU modes that execute binaries (`-ie32`, `-ie64`, `-m68k`, `-m6502`, `-z80`, `-x86`) require a filename unless `-basic` is used.

## 1.4 Ebiten Window Controls

- `F9`: Open the Machine Monitor (debugger) — freezes all CPUs, shows registers and disassembly. See [iemon.md](docs/iemon.md) for full documentation.
- `F10`: Hard reset — performs a full power-on hardware reset and boots IE64 BASIC
- `F11`: Toggle fullscreen mode
- `F12`: Toggle the runtime status bar
- `Page Up` / `Page Down`: Scroll terminal scrollback buffer
- `Mouse wheel`: Scroll terminal scrollback buffer
- Status bar semantics: `CPU`, `VIDEO`, and `AUDIO` device names are shown in green when active and gray when inactive.

## 1.5 Single-Instance Mode

Opening an `*.ie*` file while Intuition Engine is already running sends the file to the running instance via Unix domain socket IPC. The running instance performs a full hardware reset and loads the new binary. If the file uses a different CPU architecture (e.g., opening a `.ie80` Z80 binary while an IE32 program is running), the CPU mode switches automatically.

Supported extensions: `.ie32`/`.iex` (IE32), `.ie64` (IE64), `.ie65` (6502), `.ie68` (M68K), `.ie80` (Z80), `.ie86` (X86).

## 1.6 Desktop Integration

Register Intuition Engine as the default handler for `*.ie*` files:

```bash
# Install .desktop entry and MIME type (system-wide, requires root)
sudo make install-desktop-entry

# Set as default handler for .ie* files (per-user)
make set-default-handler
```

After registration, double-clicking any `.ie*` file in a file manager will open it in Intuition Engine (or send it to an already-running instance).

## 1.7 Machine Monitor

Press **F9** at any time to freeze the entire system and enter the built-in Machine Monitor — a system-level debugger inspired by the Commodore 64/Amiga Action Replay cartridge and HRTMon. Press **x** or **Esc** to resume execution.

The monitor works with all six CPU types (IE64, IE32, M68K, Z80, 6502, X86) and handles multi-CPU scenarios including coprocessors.

### Quick Reference

| Command | Description |
|---------|-------------|
| `r` | Show all registers (changed values highlighted in green) |
| `r <name> <value>` | Set a register value |
| `d [addr] [count]` | Disassemble instructions with branch annotations |
| `m <addr> [count]` | Hex dump memory |
| `s [count]` | Single-step one or more instructions |
| `bs` | Backstep (undo last step, restores CPU + memory) |
| `g [addr]` | Resume execution (optionally from a new address) |
| `u <addr>` | Run until address |
| `b <addr> [cond]` | Set breakpoint (with optional condition) |
| `bc <addr>` / `bc *` | Clear breakpoint(s) |
| `bl` | List all breakpoints |
| `ww <addr>` | Set write watchpoint |
| `wc <addr\|*>` / `wl` | Clear/list watchpoints |
| `bt [depth]` | Stack backtrace |
| `f <addr> <len> <byte>` | Fill memory with a byte value |
| `w <addr> <bytes..>` | Write bytes to memory |
| `t <dst> <src> <len>` | Transfer (copy) memory |
| `c <addr1> <addr2> <len>` | Compare two memory regions |
| `h <start> <end> <bytes..>` | Search memory for byte pattern |
| `save <s> <e> <file>` | Save memory range to file |
| `load <file> <addr>` | Load file into memory |
| `ss` / `sl [file]` | Save/load machine state |
| `trace <count>` | Trace N instructions (+ write history) |
| `io [device\|all]` | I/O register viewer (use `all` for every device) |
| `e <addr>` | Hex editor mode |
| `script <file>` | Run command script |
| `macro <name> ...` | Define command macro |
| `cpu [n]` | Switch focus to CPU #n (for multi-CPU debugging) |
| `fa` / `ta` | Audio freeze / thaw |
| `x` | Exit monitor and resume all CPUs |

Addresses accept `$hex`, `0xhex`, bare hex, `#decimal`, or expressions like `pc+$20`.

Full documentation: [iemon.md](docs/iemon.md)

# 2. Architecture

## 2.1 Unified Memory

All CPU cores (IE32, IE64, M68K, Z80, 6502, x86) share the same memory space through the MachineBus. This unified architecture ensures that:

- **Program data** loaded by any CPU is immediately visible to all peripherals
- **Audio synthesis** responds instantly to register writes from any CPU
- **DMA operations** (blitter, copper, file players) can access any memory location
- **Memory-mapped I/O** works consistently across all CPU types
- **Video compositing** blends multiple video sources (VideoChip, VGA) into final output

```
┌─────────────────────────────────────────────────────────────────┐
│                       MachineBus Memory                         │
│                          (32MB Shared)                          │
├─────────────────────────────────────────────────────────────────┤
│  0x000000 - 0x000FFF  │  System Vectors                         │
│  0x001000 - 0x09FFFF  │  Program Space (code + data)            │
│  0x0A0000 - 0x0AFFFF  │  VGA VRAM Window (64KB)                 │
│  0x0B8000 - 0x0BFFFF  │  VGA Text Buffer (32KB)                 │
│  0x0F0000 - 0x0F0FFF  │  Video/Audio I/O Registers              │
│  0x0F1000 - 0x0F13FF  │  VGA Registers                          │
│  0x100000 - 0x4FFFFF  │  Video RAM (4MB, chunky RGBA)           │
│  0x500000 - 0x1FFFFFF │  Extended RAM                           │
└─────────────────────────────────────────────────────────────────┘
        │                       │                      │
        ▼                       ▼                      ▼
   ┌─────────┐            ┌────────────────┐     ┌────────────────┐
   │   CPU   │            │  Video System  │     │  Audio System  │
   │ (IE32/  │            │  ────────────  │     │  ──────────────│
   │  IE64/  │            │  VideoChip     │     │  Custom Synth  │
   │  M68K/  │            │  VGA Engine    │     │  PSG/POKEY/SID │
   │  Z80/   │            │  Compositor    │     │  File Players  │
   │  6502/  │            │  Blitter/Copper│     └────────────────┘
   │  x86)   │
   └─────────┘
```

The video compositor blends output from the VideoChip (layer 0) and VGA (layer 10) into a single display, enabling mixed-mode effects. The copper coprocessor can target both video systems via SETBASE for per-scanline raster effects.

The custom audio synthesizer is the core of the sound system. PSG, POKEY, and SID registers are mapped to the custom synth, providing authentic register-level compatibility with high-quality 44.1kHz output. File players (.ym, .ay, .vgm/vgz, .sid, .sap, etc.) execute embedded CPU code that writes to these mapped registers.

### Bus Layers

- **MachineBus** (`Bus32`/`Bus64` interfaces): The global 32MB RAM + MMIO backbone shared by all CPUs and peripherals.
- **CPU bus interfaces** (`Z80Bus`, `X86Bus`, `Bus6502`): Per-CPU contract shapes that define the read/write/port operations each CPU core expects.
- **CPU bus adapters** (`Z80BusAdapter`, `X86BusAdapter`, `Bus6502Adapter`): Translate 8/16-bit CPU address spaces and port I/O into MachineBus calls, handling bank switching and sign extension.
- **Playback buses** (`SAPPlaybackBus6502`, `SIDPlaybackBus6502`, `TEDPlaybackBus6502`, `sndhPlaybackBus68K`, `ayPlaybackBusZ80`): Standalone lightweight bus implementations for music file playback — provide just enough memory and I/O to run embedded CPU code that drives audio register writes.

## 2.2 Hardware I/O

All hardware is accessed through memory-mapped registers in the `$F0000-$FFFFF` range:

| Subsystem | Address Range | Description |
|-----------|---------------|-------------|
| Video | `$F0000-$F0058` | Display control, copper, blitter, raster |
| Timer | `$F0800-$F080C` | System timer with interrupt support |
| Custom Audio | `$F0820-$F0B3F` | Filter, channels, effects, flex synth |
| PSG | `$F0C00-$F0C1C` | AY-3-8910 registers and file playback |
| POKEY | `$F0D00-$F0D1D` | Atari POKEY registers and SAP playback |
| SID | `$F0E00-$F0E2D` | MOS 6581 registers and SID playback |
| Banking | `$F700-$F7F0` | Bank window control (Z80/6502 only) |
| VGA | `$F1000-$F13FF` | VGA mode, DAC, sequencer, CRTC, palette |
| File I/O | `$F2200-$F221F` | Host filesystem access (LOAD/SAVE) |
| Coprocessor | `$F2340-$F237F` | Worker CPU lifecycle and async RPC |

Additionally, VGA uses legacy PC-compatible memory windows:
- `$A0000-$AFFFF`: VGA VRAM (64KB graphics memory)
- `$B8000-$BFFFF`: VGA Text Buffer (32KB, char+attr pairs)

For 8-bit CPUs (Z80, 6502), addresses are mapped to the 16-bit range `$F000-$FFFF` or accessed via I/O ports.

# 3. Memory Map & Hardware Registers (Detailed)

The system's memory layout is designed to provide efficient access to both program space and hardware features while maintaining clear separation between different memory regions.

**Address Notation:** This document uses both `0x` prefix (C-style) and `$` prefix (assembly-style) for hexadecimal addresses. The notation varies to match each CPU's assembly dialect: `0x` for IE32/IE64/general discussion, `$` for M68K and 6502 assembly examples.

## Memory Map Overview

```
0x000000 - 0x000FFF: System vectors (including interrupt vector)
0x001000 - 0x0EFFFF: Program space
0x0F0000 - 0x0F0077: Video registers (copper, blitter, raster control, Mode7)
0x0F0700 - 0x0F07FF: Terminal/Serial output
0x0F0800 - 0x0F080C: Timer registers
0x0F0820 - 0x0F0834: Filter registers
0x0F0900 - 0x0F0A6F: Legacy synth registers (square/triangle/sine/noise/saw)
0x0F0A80 - 0x0F0B7F: Flexible 4-channel synth registers (preferred)
0x0F0C00 - 0x0F0C0D: PSG registers (AY/YM synthesis)
0x0F0C0E:            PSG+ control register
0x0F0C10 - 0x0F0C1C: PSG playback control (AY/YM/VGM/SNDH)
0x0F0D00 - 0x0F0D08: POKEY registers (Atari 8-bit audio)
0x0F0D09:            POKEY+ control register
0x0F0D10 - 0x0F0D1D: SAP playback control
0x0F0E00 - 0x0F0E18: SID registers (C64 audio synthesis)
0x0F0E19:            SID+ control register
0x0F0E1A - 0x0F0E1C: SID read-only registers (OSC3, ENV3)
0x0F0E20 - 0x0F0E2D: SID playback control (.SID file playback)
0x0F0F00 - 0x0F0F05: TED registers (Plus/4 audio)
0x0F0F10 - 0x0F0F1C: TED playback control
0x0F1000 - 0x0F13FF: VGA registers (IBM VGA emulation)
0x0F2200 - 0x0F221F: File I/O registers
0x0F2340 - 0x0F237F: Coprocessor MMIO registers
0x0A0000 - 0x0AFFFF: VGA VRAM window (Mode 13h/12h)
0x0B8000 - 0x0BFFFF: VGA text buffer
0x100000 - 0x4FFFFF: Video RAM (VRAM_START to VRAM_START + VRAM_SIZE)
0x200000 - 0x27FFFF: Coprocessor worker region (IE32, 512KB)
0x280000 - 0x2FFFFF: Coprocessor worker region (M68K, 512KB)
0x300000 - 0x30FFFF: Coprocessor worker region (6502, 64KB)
0x310000 - 0x31FFFF: Coprocessor worker region (Z80, 64KB)
0x320000 - 0x39FFFF: Coprocessor worker region (x86, 512KB)
0x400000 - 0x7FFFFF: User data buffers (coprocessor request/response data)
0x820000 - 0x820FFF: Coprocessor mailbox shared RAM (4KB, ring buffers)
```

## 3.1 System Vector Table (0x000000 - 0x000FFF)

The first 4KB of memory is reserved for system vectors. The most important vector is:

```
0x0000 - 0x0003: Interrupt Service Routine (ISR) vector
```

When interrupts are enabled via the SEI instruction, the CPU reads this vector to determine the ISR address. Programs must initialise this vector before enabling interrupts:

```assembly
.org 0x0000
    .word isr_address    ; Store ISR location in vector
```

## 3.2 Program Space (0x001000 - onwards)

Programs begin loading at 0x1000, providing:

- Protection of low memory from accidental overwrites
- Clear separation from system areas
- Space for program code and data

## 3.3 Video Registers (0x0F0000 - 0x0F0077)

```
0x0F0000: VIDEO_CTRL   - Video system control (0 = disabled, 1 = enabled)
0x0F0004: VIDEO_MODE   - Display mode selection
0x0F0008: VIDEO_STATUS - Video status (read-only, lock-free)
                         bit0 = hasContent (frame has been drawn to)
                         bit1 = inVBlank (safe to draw without flicker)
0x0F000C: COPPER_CTRL  - Copper control (bit0=enable, bit1=reset/rewind)
0x0F0010: COPPER_PTR   - Copper list base address (32-bit)
0x0F0014: COPPER_PC    - Copper program counter (read-only)
0x0F0018: COPPER_STATUS- Copper status (bit0=running, bit1=waiting, bit2=halted)
0x0F001C: BLT_CTRL     - Blitter control (bit0=start, bit1=busy, bit2=irq enable)
0x0F0020: BLT_OP       - Blitter op (copy/fill/line/masked copy/alpha/mode7)
0x0F0024: BLT_SRC      - Blitter source address (32-bit)
0x0F0028: BLT_DST      - Blitter dest address (32-bit)
0x0F002C: BLT_WIDTH    - Blit width (pixels)
0x0F0030: BLT_HEIGHT   - Blit height (pixels)
0x0F0034: BLT_SRC_STRIDE - Source stride (bytes/row)
0x0F0038: BLT_DST_STRIDE - Dest stride (bytes/row)
0x0F003C: BLT_COLOR    - Fill/line color (RGBA)
0x0F0040: BLT_MASK     - Mask address for masked copy (1-bit/pixel)
0x0F0044: BLT_STATUS   - Blitter status (bit0=error)
0x0F0048: VIDEO_RASTER_Y - Raster band start Y
0x0F004C: VIDEO_RASTER_HEIGHT - Raster band height (pixels)
0x0F0050: VIDEO_RASTER_COLOR - Raster band color (RGBA)
0x0F0054: VIDEO_RASTER_CTRL - Raster band control (bit0=draw)
0x0F0058: BLT_MODE7_U0 - Mode7 start U (signed 16.16)
0x0F005C: BLT_MODE7_V0 - Mode7 start V (signed 16.16)
0x0F0060: BLT_MODE7_DU_COL - Mode7 U delta per column pixel (signed 16.16)
0x0F0064: BLT_MODE7_DV_COL - Mode7 V delta per column pixel (signed 16.16)
0x0F0068: BLT_MODE7_DU_ROW - Mode7 U delta per row (signed 16.16)
0x0F006C: BLT_MODE7_DV_ROW - Mode7 V delta per row (signed 16.16)
0x0F0070: BLT_MODE7_TEX_W - Mode7 texture width mask (2^n-1)
0x0F0074: BLT_MODE7_TEX_H - Mode7 texture height mask (2^n-1)

Available Video Modes:
MODE_640x480  = 0x00
MODE_800x600  = 0x01
MODE_1024x768 = 0x02
```

**Video Compositor:** These registers control the VideoChip, which renders as layer 0 in the compositor. The VGA chip (section 3.11) renders as layer 10 on top. Both sources are composited together for final display output. The copper coprocessor can write to either device using the SETBASE instruction (see section 12.6).

Copper lists are stored as little-endian 32-bit words in RAM. The list format is:
- `WAIT`: `(0<<30) | (y<<12) | x` (wait until raster Y/X reached)
- `MOVE`: `(1<<30) | (regIndex<<16)` followed by a 32-bit value word
- `END`: `(3<<30)`

`regIndex` is `(register_address - VIDEO_REG_BASE) / 4`, where `VIDEO_REG_BASE = 0x0F0000`.
`COPPER_PTR` is latched on enable/reset; 8-bit CPUs should write bytes to `COPPER_PTR+0..3`.

Example (mid-frame mode switch):
```assembly
; Copper list in RAM
    .long (0 << 30) | (100 << 12) | 0          ; WAIT y=100, x=0
    .long (1 << 30) | (1 << 16)                ; MOVE VIDEO_MODE (index 1)
    .long 0x01                                 ; MODE_800x600
    .long (3 << 30)                            ; END
```

## 3.4 Timer Registers (0x0F0800 - 0x0F080C)

```
0x0F0800: TIMER_CTRL   - Timer control (0 = disabled, 1 = enabled)
0x0F0804: TIMER_COUNT  - Current timer value (decrements automatically)
0x0F0808: TIMER_PERIOD - Timer reload value
0x0F080C: TIMER_STATUS - Timer status (read-only)
```

The timer generates an interrupt when TIMER_COUNT reaches zero and automatically reloads from TIMER_PERIOD.

## 3.5 Sound Registers

### Filter Registers (0x0F0820 - 0x0F0834)

```
0x0F0820: FILTER_CUTOFF     - Filter cutoff frequency (0-255)
0x0F0824: FILTER_RESONANCE  - Filter resonance (0-255)
0x0F0828: FILTER_TYPE       - Filter type selection
0x0F082C: FILTER_MOD_SOURCE - Filter modulation source
0x0F0830: FILTER_MOD_AMOUNT - Modulation depth (0-255)
```

### Legacy Synth Block (0x0F0900 - 0x0F0A6F)

#### Square Wave Channel (0x0F0900 - 0x0F093F)
```
0x0F0900: SQUARE_FREQ     - Frequency (16.8 fixed-point Hz, value = Hz * 256)
0x0F0904: SQUARE_VOL      - Volume (0-255)
0x0F0908: SQUARE_CTRL     - Channel control
0x0F090C: SQUARE_DUTY     - Duty cycle control
0x0F0910: SQUARE_SWEEP    - Frequency sweep control
0x0F0922: SQUARE_PWM_CTRL - PWM modulation control
0x0F0930: SQUARE_ATK      - Attack time (ms)
0x0F0934: SQUARE_DEC      - Decay time (ms)
0x0F0938: SQUARE_SUS      - Sustain level (0-255)
0x0F093C: SQUARE_REL      - Release time (ms)
```

#### Triangle Wave Channel (0x0F0940 - 0x0F097F)
```
0x0F0940: TRI_FREQ  - Frequency (16.8 fixed-point Hz, value = Hz * 256)
0x0F0944: TRI_VOL   - Volume control
0x0F0948: TRI_CTRL  - Channel control
0x0F0914: TRI_SWEEP - Frequency sweep control
0x0F0960: TRI_ATK   - Attack time
0x0F0964: TRI_DEC   - Decay time
0x0F0968: TRI_SUS   - Sustain level
0x0F096C: TRI_REL   - Release time
```

#### Sine Wave Channel (0x0F0980 - 0x0F09BF)
```
0x0F0980: SINE_FREQ  - Frequency (16.8 fixed-point Hz, value = Hz * 256)
0x0F0984: SINE_VOL   - Volume control
0x0F0988: SINE_CTRL  - Channel control
0x0F0918: SINE_SWEEP - Frequency sweep control
0x0F0990: SINE_ATK   - Attack time
0x0F0994: SINE_DEC   - Decay time
0x0F0998: SINE_SUS   - Sustain level
0x0F099C: SINE_REL   - Release time
```

#### Noise Channel (0x0F09C0 - 0x0F09FF)
```
0x0F09C0: NOISE_FREQ  - Frequency (16.8 fixed-point Hz, value = Hz * 256)
0x0F09C4: NOISE_VOL   - Volume control
0x0F09C8: NOISE_CTRL  - Channel control
0x0F09D0: NOISE_ATK   - Attack time
0x0F09D4: NOISE_DEC   - Decay time
0x0F09D8: NOISE_SUS   - Sustain level
0x0F09DC: NOISE_REL   - Release time
0x0F09E0: NOISE_MODE  - Noise generation mode

Noise Modes:
NOISE_MODE_WHITE    = 0 // Standard LFSR noise
NOISE_MODE_PERIODIC = 1 // Periodic/looping noise
NOISE_MODE_METALLIC = 2 // "Metallic" noise variant
```

#### Sawtooth Wave Channel (0x0F0A20 - 0x0F0A3F)
```
0x0F0A20: SAW_FREQ  - Frequency (16.8 fixed-point Hz, value = Hz * 256)
0x0F0A24: SAW_VOL   - Volume control
0x0F0A28: SAW_CTRL  - Channel control
0x0F0A2C: SAW_SWEEP - Frequency sweep control
0x0F0A30: SAW_ATK   - Attack time
0x0F0A34: SAW_DEC   - Decay time
0x0F0A38: SAW_SUS   - Sustain level
0x0F0A3C: SAW_REL   - Release time
```

#### Channel Modulation Controls (0x0F0A00 - 0x0F0A6F)
```
0x0F0A00: SYNC_SOURCE_CH0 - Sync source for channel 0
0x0F0A04: SYNC_SOURCE_CH1 - Sync source for channel 1
0x0F0A08: SYNC_SOURCE_CH2 - Sync source for channel 2
0x0F0A0C: SYNC_SOURCE_CH3 - Sync source for channel 3

0x0F0A10: RING_MOD_SOURCE_CH0 - Ring mod source for channel 0
0x0F0A14: RING_MOD_SOURCE_CH1 - Ring mod source for channel 1
0x0F0A18: RING_MOD_SOURCE_CH2 - Ring mod source for channel 2
0x0F0A1C: RING_MOD_SOURCE_CH3 - Ring mod source for channel 3

0x0F0A60: SYNC_SOURCE_CH4     - Sync source for sawtooth channel
0x0F0A64: RING_MOD_SOURCE_CH4 - Ring mod source for sawtooth channel
```

#### Global Sound Effects (0x0F0A40 - 0x0F0A54)
```
0x0F0A40: OVERDRIVE_CTRL - Drive amount (0-255)
0x0F0A50: REVERB_MIX     - Dry/wet mix (0-255)
0x0F0A54: REVERB_DECAY   - Decay time (0-255)
```

### Flexible 4-Channel Synth Block (0x0F0A80 - 0x0F0B7F)

The flexible synth block provides a modern, uniform interface for all four synthesis channels. Each channel occupies 0x40 bytes (64 bytes), supporting any waveform type.

```
Channel Base Addresses:
  Channel 0: 0x0F0A80
  Channel 1: 0x0F0AC0
  Channel 2: 0x0F0B00
  Channel 3: 0x0F0B40

Per-Channel Register Offsets:
  +0x00: FREQ       - Frequency (16.8 fixed-point Hz, value = Hz * 256)
  +0x04: VOL        - Volume (0-255)
  +0x08: CTRL       - Channel control (bit0=enable, bit1=gate)
  +0x0C: DUTY       - Duty cycle for square/pulse waves
  +0x10: SWEEP      - Frequency sweep control
  +0x14: ATK        - Attack time (ms)
  +0x18: DEC        - Decay time (ms)
  +0x1C: SUS        - Sustain level (0-255)
  +0x20: REL        - Release time (ms)
  +0x24: WAVE_TYPE  - Waveform selection (0=square, 1=triangle, 2=sine, 3=noise, 4=saw)
  +0x28: PWM_CTRL   - PWM modulation control
  +0x2C: NOISEMODE  - Noise mode (for noise waveform)
  +0x30: PHASE      - Reset phase position
  +0x34: RINGMOD    - Ring modulation source (bit7=enable, bits0-2=source channel)
  +0x38: SYNC       - Hard sync source (bit7=enable, bits0-2=source channel)
```

Example: Configure channel 1 as a sawtooth wave at 440Hz:
```assembly
; Using flexible synth registers
; Frequency is 16.8 fixed-point: 440 Hz * 256 = 112640
LOAD A, #112640
STORE A, @0x0F0AB0      ; CH1 FREQ
LOAD A, #200
STORE A, @0x0F0AB4      ; CH1 VOL
LOAD A, #4
STORE A, @0x0F0AD4      ; CH1 WAVE_TYPE = sawtooth
LOAD A, #1
STORE A, @0x0F0AB8      ; CH1 CTRL = enable
```

## 3.6 PSG Sound Chip Registers (0x0F0C00 - 0x0F0C1C)

```
0x0F0C00: PSG_REG_SELECT   - Register select
0x0F0C01: PSG_REG_DATA     - Register data
0x0F0C02-0x0F0C0D: PSG registers (direct access)
0x0F0C0E: PSG_PLUS_CTRL    - PSG+ mode (0=standard, 1=enhanced)

PSG Playback Control:
0x0F0C10: PSG_PLAY_PTR    - Pointer to PSG data (32-bit)
0x0F0C14: PSG_PLAY_LEN    - Length of PSG data (32-bit)
0x0F0C18: PSG_PLAY_CTRL   - Control (bit0=start, bit1=stop, bit2=loop)
0x0F0C1C: PSG_PLAY_STATUS - Status (bit0=busy, bit1=error)
```

## 3.7 POKEY Sound Chip Registers (0x0F0D00 - 0x0F0D1D)

```
0x0F0D00: POKEY_AUDF1    - Channel 1 frequency divider
0x0F0D01: POKEY_AUDC1    - Channel 1 control (distortion + volume)
0x0F0D02: POKEY_AUDF2    - Channel 2 frequency divider
0x0F0D03: POKEY_AUDC2    - Channel 2 control
0x0F0D04: POKEY_AUDF3    - Channel 3 frequency divider
0x0F0D05: POKEY_AUDC3    - Channel 3 control
0x0F0D06: POKEY_AUDF4    - Channel 4 frequency divider
0x0F0D07: POKEY_AUDC4    - Channel 4 control
0x0F0D08: POKEY_AUDCTL   - Master audio control
0x0F0D09: POKEY_PLUS_CTRL - POKEY+ mode (0=standard, 1=enhanced)

AUDCTL Bit Masks:
bit 0: Use 15kHz base clock (else 64kHz)
bit 1: High-pass filter ch1 by ch3
bit 2: High-pass filter ch2 by ch4
bit 3: Ch4 clocked by ch3 (16-bit mode)
bit 4: Ch2 clocked by ch1 (16-bit mode)
bit 5: Ch3 uses 1.79MHz clock
bit 6: Ch1 uses 1.79MHz clock
bit 7: Use 9-bit poly instead of 17-bit

AUDC Distortion Modes (bits 5-7):
0x00: 17-bit + 5-bit poly
0x20: 5-bit poly only
0x40: 17-bit + 4-bit poly (most metallic)
0x60: 5-bit + 4-bit poly
0x80: 17-bit poly only (white noise)
0xA0: Pure square wave
0xC0: 4-bit poly only (buzzy)
0xE0: 17-bit + pulse

SAP Player Registers (0x0F0D10 - 0x0F0D1D):
0x0F0D10: SAP_PLAY_PTR    - Pointer to SAP data (32-bit)
0x0F0D14: SAP_PLAY_LEN    - Length of SAP data (32-bit)
0x0F0D18: SAP_PLAY_CTRL   - Control (bit0=start, bit1=stop, bit2=loop)
0x0F0D1C: SAP_PLAY_STATUS - Status (bit0=busy, bit1=error)
0x0F0D1D: SAP_SUBSONG     - Subsong selection (0-255)
```

## 3.8 SID Sound Chip Registers (0x0F0E00 - 0x0F0E2D)

```
Voice 1 (0x0F0E00 - 0x0F0E06):
0x0F0E00: SID_V1_FREQ_LO  - Frequency low byte
0x0F0E01: SID_V1_FREQ_HI  - Frequency high byte
0x0F0E02: SID_V1_PW_LO    - Pulse width low byte
0x0F0E03: SID_V1_PW_HI    - Pulse width high (bits 0-3)
0x0F0E04: SID_V1_CTRL     - Control register
0x0F0E05: SID_V1_AD       - Attack/Decay
0x0F0E06: SID_V1_SR       - Sustain/Release

Voice 2 (0x0F0E07 - 0x0F0E0D):
0x0F0E07-0x0F0E0D: Same layout as Voice 1

Voice 3 (0x0F0E0E - 0x0F0E14):
0x0F0E0E-0x0F0E14: Same layout as Voice 1

Filter and Volume:
0x0F0E15: SID_FC_LO       - Filter cutoff low (bits 0-2)
0x0F0E16: SID_FC_HI       - Filter cutoff high byte
0x0F0E17: SID_RES_FILT    - Resonance (bits 4-7) + routing (bits 0-3)
0x0F0E18: SID_MODE_VOL    - Volume (bits 0-3) + filter mode (bits 4-7)
0x0F0E19: SID_PLUS_CTRL   - SID+ mode (0=standard, 1=enhanced)
0x0F0E1A: SID_OSC3        - Voice 3 oscillator output (read-only)
0x0F0E1B: SID_ENV3        - Voice 3 envelope output (read-only)
0x0F0E1C: (reserved)

Voice Control Register Bits:
bit 0: Gate (trigger envelope)
bit 1: Sync with previous voice
bit 2: Ring modulation
bit 3: Test bit (resets oscillator)
bit 4: Triangle waveform
bit 5: Sawtooth waveform
bit 6: Pulse waveform
bit 7: Noise waveform

Filter Mode Bits (SID_MODE_VOL bits 4-7):
bit 4: Low-pass filter
bit 5: Band-pass filter
bit 6: High-pass filter
bit 7: Disconnect voice 3 from output

SID Player Registers (0x0F0E20 - 0x0F0E2D):
0x0F0E20: SID_PLAY_PTR    - Pointer to .SID data (32-bit)
0x0F0E24: SID_PLAY_LEN    - Length of .SID data (32-bit)
0x0F0E28: SID_PLAY_CTRL   - Control (bit0=start, bit1=stop, bit2=loop)
0x0F0E2C: SID_PLAY_STATUS - Status (bit0=busy, bit1=error)
0x0F0E2D: SID_SUBSONG     - Subsong selection (0-255)
```

These registers allow CPU code to trigger .SID file playback from RAM, similar to the PSG and SAP player registers. The embedded 6502 code in the SID file is executed by the internal 6502 emulator at the correct frame rate (50Hz PAL or ~60Hz NTSC).

## 3.9 TED Sound Chip Registers (0x0F0F00 - 0x0F0F1F)

The TED (Text Editing Device) chip from the Commodore Plus/4 provides simple 2-voice square wave synthesis:

```
TED Sound Registers (0x0F0F00 - 0x0F0F05):
0x0F0F00: TED_FREQ1_LO   - Voice 1 frequency low byte
0x0F0F01: TED_FREQ2_LO   - Voice 2 frequency low byte
0x0F0F02: TED_FREQ2_HI   - Voice 2 frequency high (bits 0-1)
0x0F0F03: TED_SND_CTRL   - Control (DA/noise/ch2on/ch1on/volume)
0x0F0F04: TED_FREQ1_HI   - Voice 1 frequency high (bits 0-1)
0x0F0F05: TED_PLUS_CTRL  - TED+ enhanced audio mode (0=standard, 1=enhanced)

TED_SND_CTRL bits:
  Bit 7: D/A mode
  Bit 6: Voice 2 noise enable (replaces square wave with white noise)
  Bit 5: Voice 2 enable
  Bit 4: Voice 1 enable
  Bits 0-3: Volume (0-8, where 8 is maximum)

TED Player Registers (0x0F0F10 - 0x0F0F1C):
0x0F0F10: TED_PLAY_PTR    - Pointer to .TED data (32-bit)
0x0F0F14: TED_PLAY_LEN    - Length of .TED data (32-bit)
0x0F0F18: TED_PLAY_CTRL   - Control (bit0=start, bit1=stop, bit2=loop)
0x0F0F1C: TED_PLAY_STATUS - Status (bit0=busy, bit1=error)
```

Frequency formula: `freq_hz = clock/8 / (1024 - register_value)`
Clock rates: 886724 Hz (PAL), 894886 Hz (NTSC)

These registers allow CPU code to trigger .TED file playback from RAM. The embedded 6502 code in the TED file is executed by the internal 6502 emulator at 50Hz (PAL).

## 3.10 TED Video Chip Registers (0x0F0F20 - 0x0F0F5F)

The TED chip also provides video capabilities from the Commodore Plus/4:

- 40x25 text mode (8x8 character cells)
- 320x200 pixel resolution (384x272 with border)
- 121 colors (16 hues × 8 luminances)
- Hardware cursor support
- Compositor layer 12 (between VGA=10 and ULA=15)

```
TED Video Registers (0x0F0F20 - 0x0F0F5F, 4-byte aligned for copper):
0x0F0F20: TED_V_CTRL1      - Control 1 (ECM/BMM/DEN/RSEL/YSCROLL)
0x0F0F24: TED_V_CTRL2      - Control 2 (RES/MCM/CSEL/XSCROLL)
0x0F0F28: TED_V_CHAR_BASE  - Character/bitmap base address
0x0F0F2C: TED_V_VIDEO_BASE - Video matrix base address
0x0F0F30: TED_V_BG_COLOR0  - Background color 0
0x0F0F34: TED_V_BG_COLOR1  - Background color 1 (multicolor)
0x0F0F38: TED_V_BG_COLOR2  - Background color 2 (multicolor)
0x0F0F3C: TED_V_BG_COLOR3  - Background color 3 (multicolor)
0x0F0F40: TED_V_BORDER     - Border color
0x0F0F44: TED_V_CURSOR_HI  - Cursor position high byte
0x0F0F48: TED_V_CURSOR_LO  - Cursor position low byte
0x0F0F4C: TED_V_CURSOR_CLR - Cursor color
0x0F0F50: TED_V_RASTER_LO  - Raster line low (read-only)
0x0F0F54: TED_V_RASTER_HI  - Raster line high (read-only)
0x0F0F58: TED_V_ENABLE     - Video enable (bit 0)
0x0F0F5C: TED_V_STATUS     - Status (bit 0 = VBlank)

TED_V_CTRL1 bits:
  Bit 6: ECM - Extended Color Mode
  Bit 5: BMM - Bitmap Mode
  Bit 4: DEN - Display Enable
  Bit 3: RSEL - Row Select (0=24 rows, 1=25 rows)
  Bits 0-2: YSCROLL - Vertical scroll (0-7)

TED_V_CTRL2 bits:
  Bit 5: RES - Reset
  Bit 4: MCM - Multicolor Mode
  Bit 3: CSEL - Column Select (0=38 cols, 1=40 cols)
  Bits 0-2: XSCROLL - Horizontal scroll (0-7)

Color byte format: Bits 4-6 = luminance (0-7), Bits 0-3 = hue (0-15)
```

TED Video CPU Mappings:
- IE32/IE64/M68K: Direct access at 0x0F0F20-0x0F0F5F
- 6502: Memory-mapped at $D620-$D62F
- Z80: Port I/O via 0xF2/0xF3, indices 0x20-0x2F

## 3.11 AHX Module Player Registers (0x0F0B80 - 0x0F0B91)

The AHX engine provides Amiga AHX/THX module playback with 4-channel waveform synthesis:

```
AHX Control Registers (0x0F0B80):
0x0F0B80: AHX_PLUS_CTRL  - AHX+ enhanced audio mode (0=standard, 1=enhanced)

AHX Player Registers (0x0F0B84 - 0x0F0B91):
0x0F0B84: AHX_PLAY_PTR    - Pointer to .AHX data (32-bit)
0x0F0B88: AHX_PLAY_LEN    - Length of .AHX data (32-bit)
0x0F0B8C: AHX_PLAY_CTRL   - Control (bit0=start, bit1=stop, bit2=loop)
0x0F0B90: AHX_PLAY_STATUS - Status (bit0=busy, bit1=error)
0x0F0B91: AHX_SUBSONG     - Subsong selection (0-255)
```

AHX+ mode provides enhanced audio processing:
- 4x oversampling for cleaner waveforms
- Second-order Butterworth lowpass (biquad, -12dB/oct anti-alias)
- Subtle saturation (drive 0.16) for analog warmth
- Allpass diffuser room ambience (mix 0.09, delay 120 samples)
- Authentic Amiga stereo panning (L-R-R-L pattern)
- Hardware PWM mapping SquarePos to duty cycle

## 3.12 Hardware I/O Memory Map by CPU

All sound and video chips are accessible from all CPU architectures at different address ranges:

### Sound Chips

| Chip  | IE32/IE64/M68K         | Z80 Ports | x86 Ports | 6502        | Notes |
|-------|-------------------|-----------|-----------|-------------|-------|
| PSG   | 0x0F0C00-0x0F0C0D | 0xF0/0xF1 | 0xF0/0xF1 | $D400-$D40D | AY-3-8910/YM2149 compatible |
| POKEY | 0x0F0D00-0x0F0D09 | 0xD0/0xD1 | 0xD0-0xD3* | $D200-$D209 | Atari 8-bit compatible |
| SID   | 0x0F0E00-0x0F0E1C | 0xE0/0xE1 | 0xE0/0xE1 | $D500-$D51C | MOS 6581/8580 compatible |
| TED   | 0x0F0F00-0x0F0F05 | 0xF2/0xF3 | 0xF2/0xF3 | $D600-$D605 | Plus/4 compatible |
| AHX   | 0x0F0B80-0x0F0B91 | —         | —         | $FB80-$FB91 | Amiga AHX/THX module player |

\* x86 POKEY uses ports 0xD0-0xD3 and 0xD8-0xDF (0xD4-0xD7 reserved for ANTIC/GTIA)

### Video Chips

| Chip      | IE32/IE64/M68K         | Z80 Ports   | x86 Ports     | 6502        | Notes |
|-----------|-------------------|-------------|---------------|-------------|-------|
| VideoChip | 0x0F0000-0x0F0077 | Memory      | Memory        | $F000-$F077 | Custom copper/blitter |
| TED Video | 0x0F0F20-0x0F0F5F | 0xF2/0xF3   | 0xF2/0xF3     | $D620-$D62F | Plus/4 (idx 0x20-0x2F) |
| VGA       | 0x0F1000-0x0F13FF | 0xA0-0xAC   | 0x3C4-0x3DA   | $D700-$D70A | IBM VGA compatible |
| Voodoo    | 0x0F4000-0x0F43FF | Memory      | Memory        | Memory      | 3DFX SST-1 3D accelerator |
| ANTIC     | 0x0F2100-0x0F213F | 0xD4/0xD5   | 0xD4/0xD5     | $D400-$D40F | Atari 8-bit video |
| GTIA      | 0x0F2140-0x0F21B7 | 0xD6/0xD7   | 0xD6/0xD7     | $D000-$D01F | Atari 8-bit color + P/M |
| ULA       | 0x0F2000-0x0F200B | 0xFE        | 0xFE          | $D800-$D80B | ZX Spectrum compatible |

Note: 6502 has PSG at $D400 which overlaps with ANTIC's authentic Atari address. Use M68K/Z80/x86 for ANTIC access when PSG is in use.

### Access Methods

**Z80 Port I/O:** The first port selects the register index, the second reads/writes data.
Example: `OUT (0xF0),A` selects PSG register, `OUT (0xF1),A` writes data.

**6502 Memory-Mapped:** Direct memory access following C64/Atari/Plus4 conventions.
Example: `STA $D400` writes to PSG register 0.

**IE32/IE64/M68K Direct:** Full 32-bit address space access.
Example: `MOVE.B D0,($F0C00).L` writes to PSG register 0.

## 3.13 VGA Video Chip (0x0F1000 - 0x0F13FF)

The VGA chip provides IBM PC-compatible graphics modes, allowing classic PC demo effects and games:

### Supported Modes

| Mode | Resolution | Colors | Type | Description |
|------|------------|--------|------|-------------|
| 0x03 | 80×25 | 16 | Text | Standard VGA text mode with 8×16 font |
| 0x12 | 640×480 | 16 | Planar | High-res 4-plane graphics |
| 0x13 | 320×200 | 256 | Linear | Classic "Mode 13h" for demos/games |
| 0x14 | 320×240 | 256 | Mode-X | Unchained planar for page flipping |

### Register Map

```
VGA Control Registers (0x0F1000 - 0x0F100F):
0x0F1000: VGA_MODE        - Mode select (0x03/0x12/0x13/0x14)
0x0F1004: VGA_STATUS      - Status (bit 0=vsync, bit 3=retrace)
0x0F1008: VGA_CTRL        - Control (bit 0=enable)

Sequencer Registers (0x0F1010 - 0x0F101F):
0x0F1010: VGA_SEQ_INDEX   - Sequencer register index
0x0F1014: VGA_SEQ_DATA    - Sequencer register data
0x0F1018: VGA_SEQ_MAPMASK - Plane write mask (direct access)

CRTC Registers (0x0F1020 - 0x0F102F):
0x0F1020: VGA_CRTC_INDEX  - CRTC register index
0x0F1024: VGA_CRTC_DATA   - CRTC register data
0x0F1028: VGA_CRTC_STARTHI - Display start address high
0x0F102C: VGA_CRTC_STARTLO - Display start address low

Graphics Controller (0x0F1030 - 0x0F103F):
0x0F1030: VGA_GC_INDEX    - Graphics controller index
0x0F1034: VGA_GC_DATA     - Graphics controller data
0x0F1038: VGA_GC_READMAP  - Read plane select
0x0F103C: VGA_GC_BITMASK  - Bit mask for write operations

Attribute Controller (0x0F1040 - 0x0F104F):
0x0F1040: VGA_ATTR_INDEX  - Attribute index/data
0x0F1044: VGA_ATTR_DATA   - Attribute read

DAC/Palette (0x0F1050 - 0x0F105F):
0x0F1050: VGA_DAC_MASK    - Pixel mask (default 0xFF)
0x0F1054: VGA_DAC_RINDEX  - Read index
0x0F1058: VGA_DAC_WINDEX  - Write index
0x0F105C: VGA_DAC_DATA    - DAC data (R,G,B sequence, 6-bit values)

Palette RAM (0x0F1100 - 0x0F13FF):
0x0F1100: VGA_PALETTE     - 256 palette entries × 3 bytes (768 bytes)
```

### VRAM Windows

```
0x0A0000 - 0x0AFFFF: VGA VRAM (64KB graphics memory)
0x0B8000 - 0x0BFFFF: VGA Text Buffer (32KB, char+attr pairs)
```

### CPU Address Mappings

| CPU | VGA Registers | VGA VRAM | DAC Access |
|-----|---------------|----------|------------|
| IE32/IE64/M68K | 0x0F1000-0x0F13FF | 0x0A0000-0x0AFFFF | Memory-mapped |
| Z80 | Ports 0xA0-0xAC | Memory 0xA000 (banked) | Port I/O |
| 6502 | $D700-$D70D | $A000 (banked) | Memory-mapped |

### Mode 13h Example (M68K)

```asm
    include "ie68.inc"

    ; Set Mode 13h (320x200x256)
    vga_setmode VGA_MODE_13H

    ; Set palette entry 1 to bright red
    move.b  #1,VGA_DAC_WINDEX    ; Select color 1
    move.b  #63,VGA_DAC_DATA     ; R = 63 (max)
    move.b  #0,VGA_DAC_DATA      ; G = 0
    move.b  #0,VGA_DAC_DATA      ; B = 0

    ; Draw pixel at (100, 50) = offset 100 + 50*320 = 16100
    move.b  #1,$A0000+16100      ; Write color 1
```

### Mode 12h Planar Example (M68K)

```asm
    include "ie68.inc"

    ; Set Mode 12h (640x480x16 planar)
    vga_setmode VGA_MODE_12H

    ; Enable plane 0 only for writing
    move.b  #1,VGA_SEQ_MAPMASK

    ; Write to VRAM (affects only plane 0)
    move.b  #$FF,$A0000          ; Set 8 pixels in plane 0
```

### Video Compositor Integration

The VGA chip integrates with the video compositor as a separate layer (layer 10) that renders on top of the VideoChip (layer 0). Both sources are blended together for the final display output.

**Per-Scanline Rendering:** The VGA supports scanline-aware rendering, meaning the copper coprocessor can modify VGA palette registers on a per-scanline basis. When the copper executes a `SETBASE` to target VGA DAC registers followed by palette MOVE operations, those changes affect VGA rendering from that scanline onward. This enables classic PC demo effects like raster bars and gradient backgrounds.

**VSync Timing:** The VGA provides time-based vsync status through `VGA_STATUS`. The status bit automatically calculates whether the display is in vertical retrace based on a 60Hz refresh cycle, requiring no explicit signaling from the compositor.

See section 12.9 (Video Compositor) for details on the compositing architecture and section 12.6 (Copper List Executor) for examples of copper-driven VGA palette manipulation.

## 3.14 ULA Video Chip - ZX Spectrum (0x0F2000 - 0x0F200B)

The ULA chip provides authentic ZX Spectrum video output, enabling classic Spectrum demos and games:

### Display Specifications

| Feature | Value |
|---------|-------|
| Resolution | 256×192 pixels |
| Border | 32 pixels each side (320×256 total) |
| Color Cells | 32×24 (8×8 pixels per cell) |
| Colors | 15 unique (8 base + 8 bright, black can't brighten) |
| VRAM | 6912 bytes (6144 bitmap + 768 attributes) |
| Flash Rate | ~1.6Hz (toggle every 32 frames at 50Hz) |

### Register Map

```
ULA Control Registers (0x0F2000 - 0x0F200B):
0x0F2000: ULA_BORDER      - Border color (bits 0-2, values 0-7)
0x0F2004: ULA_CTRL        - Control (bit 0=enable)
0x0F2008: ULA_STATUS      - Status (bit 0=vblank)
```

### VRAM Layout

The ULA uses the authentic ZX Spectrum memory layout at 0x4000:

```
0x4000 - 0x57FF: Bitmap (6144 bytes, non-linear Y addressing)
0x5800 - 0x5AFF: Attributes (768 bytes, 32×24 cells)
```

### Non-Linear Bitmap Addressing

The ZX Spectrum uses a peculiar addressing formula for the bitmap. The screen is divided into three 64-line sections, with each section having interleaved line ordering:

```
Address = ((y & 0xC0) << 5) + ((y & 0x07) << 8) + ((y & 0x38) << 2) + (x >> 3)
```

| Y Range | Address Range | Description |
|---------|---------------|-------------|
| 0-63 | 0x4000-0x47FF | Top third of screen |
| 64-127 | 0x4800-0x4FFF | Middle third |
| 128-191 | 0x5000-0x57FF | Bottom third |

### Attribute Format

Each attribute byte controls an 8×8 pixel cell:

```
Bit 7: FLASH   - Swap INK/PAPER at ~1.6Hz
Bit 6: BRIGHT  - Intensify both INK and PAPER
Bits 5-3: PAPER (background color, 0-7)
Bits 2-0: INK   (foreground color, 0-7)
```

### Color Palette

| Index | Normal RGB | Bright RGB | Color |
|-------|-----------|-----------|-------|
| 0 | (0,0,0) | (0,0,0) | Black |
| 1 | (0,0,205) | (0,0,255) | Blue |
| 2 | (205,0,0) | (255,0,0) | Red |
| 3 | (205,0,205) | (255,0,255) | Magenta |
| 4 | (0,205,0) | (0,255,0) | Green |
| 5 | (0,205,205) | (0,255,255) | Cyan |
| 6 | (205,205,0) | (255,255,0) | Yellow |
| 7 | (205,205,205) | (255,255,255) | White |

### CPU Address Mappings

| CPU | ULA Registers | ULA VRAM | Notes |
|-----|---------------|----------|-------|
| IE32/IE64/M68K | 0x0F2000-0x0F200B | 0x4000-0x5AFF | Direct 32-bit |
| Z80 | Port 0xFE | 0x4000-0x5AFF | Authentic Spectrum |
| 6502 | $D800-$D80F | $4000 (banked) | Memory-mapped |

### Example: Drawing a Pixel (M68K)

```asm
    include "ie68.inc"

    ; Draw white pixel at (128, 96) with bright attribute
    ; First calculate bitmap address for y=96, x=128
    ; ((96 & $C0) << 5) + ((96 & $07) << 8) + ((96 & $38) << 2) + (128 >> 3)
    ; = ($40 << 5) + ($00 << 8) + ($00 << 2) + 16
    ; = $800 + $0 + $0 + $10 = $810

    move.b  #$80,ULA_VRAM+$810   ; Set bit 7 (leftmost pixel in byte)

    ; Set attribute for cell at (16, 12) = offset 12*32+16 = 400 = $190
    ; Attribute: BRIGHT=1, PAPER=0 (black), INK=7 (white) = $47
    move.b  #$47,ULA_VRAM+ULA_ATTR_OFFSET+$190

    ; Set border to blue
    ula_border 1
```

### Example: Clear Screen (Z80)

```asm
    include "ie80.inc"

    ; Clear bitmap to zeros (all paper color)
    ld hl,ULA_VRAM
    ld de,ULA_VRAM+1
    ld bc,ULA_BITMAP_SIZE-1
    ld (hl),0
    ldir

    ; Set all attributes to white ink on blue paper
    ; Attribute: BRIGHT=0, PAPER=1 (blue), INK=7 (white) = $0F
    ld hl,ULA_ATTR_BASE
    ld de,ULA_ATTR_BASE+1
    ld bc,ULA_ATTR_SIZE-1
    ld (hl),$0F
    ldir

    ; Set border to blue
    ULA_SET_BORDER 1
```

### Video Compositor Integration

The ULA integrates with the video compositor as layer 15, rendering above both the VideoChip (layer 0) and VGA (layer 10). This allows ZX Spectrum graphics to overlay other video sources.

The ULA provides its own frame timing through `SignalVSync()`, which handles the FLASH attribute timing (toggling every 32 frames). When disabled via `ULA_CTRL`, the chip returns nil frames to the compositor.

## 3.15 ANTIC Video Chip - Atari 8-bit (0x0F2100 - 0x0F213F)

The ANTIC (Alphanumeric Television Interface Controller) provides authentic Atari 8-bit computer video output, enabling classic Atari demos and games:

### Display Specifications

| Feature | Value |
|---------|-------|
| Resolution | 320×192 pixels |
| Border | 32 pixels horizontal, 24 pixels vertical (384×240 total) |
| Colors | 128 (16 hues × 8 luminances) |
| Display Modes | 14 (text and graphics modes via display list) |
| Fine Scrolling | 4-bit horizontal/vertical (0-15 pixels) |

### Register Map (IE32/IE64/M68K/x86)

All registers are 4-byte aligned for copper coprocessor compatibility:

```
ANTIC Registers (0x0F2100 - 0x0F213F):
0x0F2100: ANTIC_DMACTL    - DMA control (playfield width, DMA enables)
0x0F2104: ANTIC_CHACTL    - Character control (inverse, reflect)
0x0F2108: ANTIC_DLISTL    - Display list pointer low byte
0x0F210C: ANTIC_DLISTH    - Display list pointer high byte
0x0F2110: ANTIC_HSCROL    - Horizontal scroll (0-15)
0x0F2114: ANTIC_VSCROL    - Vertical scroll (0-15)
0x0F2118: ANTIC_PMBASE    - Player-missile base address (high byte)
0x0F211C: ANTIC_CHBASE    - Character set base address (high byte)
0x0F2120: ANTIC_WSYNC     - Wait for horizontal sync (write only)
0x0F2124: ANTIC_VCOUNT    - Vertical line counter (read only, /2)
0x0F2128: ANTIC_PENH      - Light pen horizontal (read only)
0x0F212C: ANTIC_PENV      - Light pen vertical (read only)
0x0F2130: ANTIC_NMIEN     - NMI enable register
0x0F2134: ANTIC_NMIST     - NMI status (read) / NMIRES (write)
0x0F2138: ANTIC_ENABLE    - Video enable (IE-specific)
0x0F213C: ANTIC_STATUS    - Status (VBlank flag, IE-specific)
```

### 6502 Register Map (Atari Authentic)

For 6502 compatibility, ANTIC uses authentic Atari addresses at 0xD400:

```
0xD400: DMACTL    0xD401: CHACTL    0xD402: DLISTL    0xD403: DLISTH
0xD404: HSCROL    0xD405: VSCROL    0xD407: PMBASE    0xD409: CHBASE
0xD40A: WSYNC     0xD40B: VCOUNT    0xD40C: PENH      0xD40D: PENV
0xD40E: NMIEN     0xD40F: NMIST
```

### DMACTL Register Bits

| Bit | Name | Description |
|-----|------|-------------|
| 0-1 | Playfield | 00=off, 01=narrow, 10=normal, 11=wide |
| 2 | Missile DMA | Enable missile graphics DMA |
| 3 | Player DMA | Enable player graphics DMA |
| 4 | PM Resolution | 0=double-line, 1=single-line |
| 5 | DL Enable | Enable display list DMA |

### Color System

ANTIC uses a 128-color palette with 16 hues and 8 luminance levels:

| Hue | Color | Hue | Color |
|-----|-------|-----|-------|
| 0 | Gray | 8 | Blue-2 |
| 1 | Gold | 9 | Light Blue |
| 2 | Orange | A | Turquoise |
| 3 | Red-Orange | B | Green-Blue |
| 4 | Pink | C | Green |
| 5 | Purple | D | Yellow-Green |
| 6 | Purple-Blue | E | Orange-Green |
| 7 | Blue | F | Light Orange |

Color format: `HHHHLLLL` where `HHHH` = hue (0-15), `LLLL` = luminance (0-15, but only 0-F used).

### Display List

ANTIC is driven by a display list - a program that specifies what to render on each scanline. Similar to the Amiga's copper coprocessor, the display list is a sequence of instructions that control video output.

#### Blank Line Instructions

| Opcode | Constant | Scanlines |
|--------|----------|-----------|
| 0x00 | DL_BLANK1 | 1 blank scanline |
| 0x10 | DL_BLANK2 | 2 blank scanlines |
| 0x20 | DL_BLANK3 | 3 blank scanlines |
| 0x30 | DL_BLANK4 | 4 blank scanlines |
| 0x40 | DL_BLANK5 | 5 blank scanlines |
| 0x50 | DL_BLANK6 | 6 blank scanlines |
| 0x60 | DL_BLANK7 | 7 blank scanlines |
| 0x70 | DL_BLANK8 | 8 blank scanlines |

#### Jump Instructions

| Opcode | Constant | Description |
|--------|----------|-------------|
| 0x01 | DL_JMP | Jump to address (2 bytes follow) |
| 0x41 | DL_JVB | Jump and wait for Vertical Blank |

#### Graphics Mode Instructions

| Opcode | Constant | Description |
|--------|----------|-------------|
| 0x02 | DL_MODE2 | 40 column text, 8 scanlines/row |
| 0x03 | DL_MODE3 | 40 column text, 10 scanlines/row |
| 0x04 | DL_MODE4 | 40 column text, 8 scanlines, multicolor |
| 0x05 | DL_MODE5 | 40 column text, 16 scanlines, multicolor |
| 0x06 | DL_MODE6 | 20 column text, 8 scanlines |
| 0x07 | DL_MODE7 | 20 column text, 16 scanlines |
| 0x08 | DL_MODE8 | 40 pixels, 8 scanlines/row (GRAPHICS 3) |
| 0x09 | DL_MODE9 | 80 pixels, 4 scanlines (GRAPHICS 4) |
| 0x0A | DL_MODE10 | 80 pixels, 2 scanlines (GRAPHICS 5) |
| 0x0B | DL_MODE11 | 160 pixels, 1 scanline (GRAPHICS 6) |
| 0x0C | DL_MODE12 | 160 pixels, 1 scanline (GRAPHICS 6+) |
| 0x0D | DL_MODE13 | 160 pixels, 2 scanlines (GRAPHICS 7) |
| 0x0E | DL_MODE14 | 160 pixels, 1 scanline, 4 colors |
| 0x0F | DL_MODE15 | 320 pixels, 1 scanline (GRAPHICS 8) |

#### Instruction Modifiers (OR with mode)

| Value | Constant | Description |
|-------|----------|-------------|
| 0x40 | DL_LMS | Load Memory Scan (2 address bytes follow) |
| 0x80 | DL_DLI | Display List Interrupt at end of line |
| 0x10 | DL_HSCROL | Enable horizontal fine scrolling |
| 0x20 | DL_VSCROL | Enable vertical fine scrolling |

#### Example Display List (x86)

```asm
display_list:
    db DL_BLANK8                    ; 8 blank lines (top border)
    db DL_BLANK8                    ; 8 more blank lines
    db DL_BLANK8                    ; 8 more blank lines
    db DL_MODE2 | DL_LMS            ; Mode 2 text with LMS
    dw screen_memory                ; Screen memory address
    times 23 db DL_MODE2            ; 23 more mode 2 lines
    db DL_JVB                       ; Jump and wait for VBlank
    dw display_list                 ; Loop back to start
```

### Video Compositor Integration

ANTIC integrates with the video compositor as layer 13, positioned between TED (layer 12) and ULA (layer 15). All copper commands can target ANTIC registers via SETBASE for per-scanline effects.

### Example: Basic Setup (M68K)

```asm
    include "ie68.inc"

    ; Enable ANTIC with normal playfield and display list DMA
    move.b  #ANTIC_DMA_NORMAL|ANTIC_DMA_DL,ANTIC_DMACTL

    ; Point to display list
    lea     my_dlist,a0
    move.b  a0,ANTIC_DLISTL
    lsr.w   #8,d0
    move.b  d0,ANTIC_DLISTH

    ; Enable ANTIC video output
    antic_enable

    ; Wait for VBlank
    antic_wait_vblank
```

### Example: WSYNC Raster Effect (6502)

```asm
    include "ie65.inc"

    ; Create color bars using WSYNC timing
    ldx #0
loop:
    stx GTIA_COLBK      ; Set background color via GTIA
    sta ANTIC_WSYNC     ; Wait for horizontal sync
    inx
    bne loop
```

## 3.16 GTIA Color Control (0x0F2140 - 0x0F216F)

The GTIA (Graphics Television Interface Adapter) companion chip handles color generation and player-missile graphics for Atari 8-bit systems. While ANTIC controls display timing and the display list, GTIA controls all color output.

### Register Map (IE32/IE64/M68K/x86)

All registers are 4-byte aligned for copper coprocessor compatibility:

```
GTIA Registers (0x0F2140 - 0x0F21B7):
0x0F2140: GTIA_COLPF0   - Playfield color 0
0x0F2144: GTIA_COLPF1   - Playfield color 1
0x0F2148: GTIA_COLPF2   - Playfield color 2
0x0F214C: GTIA_COLPF3   - Playfield color 3
0x0F2150: GTIA_COLBK    - Background/border color
0x0F2154: GTIA_COLPM0   - Player/missile 0 color
0x0F2158: GTIA_COLPM1   - Player/missile 1 color
0x0F215C: GTIA_COLPM2   - Player/missile 2 color
0x0F2160: GTIA_COLPM3   - Player/missile 3 color
0x0F2164: GTIA_PRIOR    - Priority and GTIA modes
0x0F2168: GTIA_GRACTL   - Graphics control (bit 1=players, bit 0=missiles)
0x0F216C: GTIA_CONSOL   - Console switches (read only)
0x0F2170: GTIA_HPOSP0   - Player 0 horizontal position
0x0F2174: GTIA_HPOSP1   - Player 1 horizontal position
0x0F2178: GTIA_HPOSP2   - Player 2 horizontal position
0x0F217C: GTIA_HPOSP3   - Player 3 horizontal position
0x0F2180: GTIA_HPOSM0   - Missile 0 horizontal position
0x0F2184: GTIA_HPOSM1   - Missile 1 horizontal position
0x0F2188: GTIA_HPOSM2   - Missile 2 horizontal position
0x0F218C: GTIA_HPOSM3   - Missile 3 horizontal position
0x0F2190: GTIA_SIZEP0   - Player 0 size (0=normal, 1=double, 3=quad)
0x0F2194: GTIA_SIZEP1   - Player 1 size
0x0F2198: GTIA_SIZEP2   - Player 2 size
0x0F219C: GTIA_SIZEP3   - Player 3 size
0x0F21A0: GTIA_SIZEM    - Missile sizes (2 bits each)
0x0F21A4: GTIA_GRAFP0   - Player 0 graphics (8 pixels)
0x0F21A8: GTIA_GRAFP1   - Player 1 graphics
0x0F21AC: GTIA_GRAFP2   - Player 2 graphics
0x0F21B0: GTIA_GRAFP3   - Player 3 graphics
0x0F21B4: GTIA_GRAFM    - Missile graphics (2 bits each)
```

### 6502 Register Map (Atari Authentic)

For 6502 compatibility, GTIA uses authentic Atari addresses at 0xD000:

```
Player/Missile Position and Size:
0xD000: HPOSP0    0xD001: HPOSP1    0xD002: HPOSP2    0xD003: HPOSP3
0xD004: HPOSM0    0xD005: HPOSM1    0xD006: HPOSM2    0xD007: HPOSM3
0xD008: SIZEP0    0xD009: SIZEP1    0xD00A: SIZEP2    0xD00B: SIZEP3
0xD00C: SIZEM     0xD00D: GRAFP0    0xD00E: GRAFP1    0xD00F: GRAFP2
0xD010: GRAFP3    0xD011: GRAFM

Color and Control:
0xD012: COLPM0    0xD013: COLPM1    0xD014: COLPM2    0xD015: COLPM3
0xD016: COLPF0    0xD017: COLPF1    0xD018: COLPF2    0xD019: COLPF3
0xD01A: COLBK     0xD01B: PRIOR     0xD01D: GRACTL    0xD01F: CONSOL
```

### Color Format

Colors use the ANTIC 128-color palette format:

| Bits | Field | Description |
|------|-------|-------------|
| 7-4 | Hue | 16 hues (0=gray, 1-15=chromatic) |
| 3-0 | Luminance | 8 levels (only even values 0-14 used) |

### PRIOR Register Bits

The PRIOR register controls display priority and special GTIA modes:

| Bit | Name | Description |
|-----|------|-------------|
| 0-3 | Priority | Player/playfield priority selection |
| 4 | Multicolor | Enable 5th player (missiles as single player) |
| 5 | Fifth | Enable multicolor players |
| 6-7 | GTIA Mode | 00=normal, 01=16 lum, 10=9 color, 11=16 hue |

### Example: Raster Bars (x86)

```asm
    %include "ie86.inc"

    ; Create smooth rainbow raster bars
    mov ecx, 192            ; 192 visible lines
.raster_loop:
    mov byte [ANTIC_WSYNC], 0   ; Wait for HSYNC
    mov eax, ecx
    shl eax, 4              ; Scale line to hue
    and eax, 0xF0           ; Mask hue bits
    or eax, 0x08            ; Medium luminance
    mov byte [GTIA_COLBK], al
    loop .raster_loop
```

### Example: Set Playfield Colors (M68K)

```asm
    include "ie68.inc"

    ; Set up a typical Atari display palette
    move.b  #$94,GTIA_COLPF0    ; Light blue
    move.b  #$0F,GTIA_COLPF1    ; White
    move.b  #$C6,GTIA_COLPF2    ; Green
    move.b  #$46,GTIA_COLPF3    ; Red
    move.b  #$00,GTIA_COLBK     ; Black background
```

## 3.17 Coprocessor Subsystem (0x0F2340 - 0x0F237F)

The coprocessor subsystem allows any CPU to launch worker CPUs (IE32, 6502, M68K, Z80, x86) that run service binaries. Workers poll a shared mailbox ring buffer for requests, process them, and write results. The caller manages worker lifecycle and request routing via MMIO registers. Available in all CPU modes.

### MMIO Registers

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| 0xF2340 | COPROC_CMD | W | Command register (triggers action on byte-0 write) |
| 0xF2344 | COPROC_CPU_TYPE | W | Target CPU type (1=IE32, 2=IE64, 3=6502, 4=M68K, 5=Z80, 6=x86) |
| 0xF2348 | COPROC_CMD_STATUS | R | 0=ok, 1=error |
| 0xF234C | COPROC_CMD_ERROR | R | Error code (see below) |
| 0xF2350 | COPROC_TICKET | R/W | Ticket ID |
| 0xF2354 | COPROC_TICKET_STATUS | R | Per-ticket status |
| 0xF2358 | COPROC_OP | W | Operation code |
| 0xF235C | COPROC_REQ_PTR | W | Request data pointer |
| 0xF2360 | COPROC_REQ_LEN | W | Request data length |
| 0xF2364 | COPROC_RESP_PTR | W | Response buffer pointer |
| 0xF2368 | COPROC_RESP_CAP | W | Response buffer capacity |
| 0xF236C | COPROC_TIMEOUT | W | Timeout in ms |
| 0xF2370 | COPROC_NAME_PTR | W | Service filename pointer |
| 0xF2374 | COPROC_WORKER_STATE | R | Bitmask of running workers |

### Byte-Level MMIO Access

All registers support byte-level reads and writes. This allows 8-bit CPUs to program 32-bit registers using four single-byte writes. Registers are aligned to 4-byte boundaries: sub-register byte offsets are computed as `addr & 3`. Writes to bytes 1-3 of a register perform read-modify-write on the shadow register. Command dispatch only fires when byte 0 of COPROC_CMD is written — writes to bytes 1-3 of COPROC_CMD do not trigger dispatch. This means the CMD register must be written last in any sequence.

### 16-bit CPU Gateway (0xF200 - 0xF23F)

Z80 and 6502 CPUs have 16-bit address spaces and cannot directly reach `0xF2340`. A gateway window at `0xF200-0xF23F` transparently redirects reads and writes to the coprocessor MMIO:

| Gateway Address | Maps To | Register |
|----------------|---------|----------|
| 0xF200 | 0xF2340 | COPROC_CMD |
| 0xF204 | 0xF2344 | COPROC_CPU_TYPE |
| 0xF208 | 0xF2348 | COPROC_CMD_STATUS |
| 0xF20C | 0xF234C | COPROC_CMD_ERROR |
| 0xF210 | 0xF2350 | COPROC_TICKET |
| 0xF214 | 0xF2354 | COPROC_TICKET_STATUS |
| 0xF218 | 0xF2358 | COPROC_OP |
| 0xF21C | 0xF235C | COPROC_REQ_PTR |
| 0xF220 | 0xF2360 | COPROC_REQ_LEN |
| 0xF224 | 0xF2364 | COPROC_RESP_PTR |
| 0xF228 | 0xF2368 | COPROC_RESP_CAP |
| 0xF22C | 0xF236C | COPROC_TIMEOUT |
| 0xF230 | 0xF2370 | COPROC_NAME_PTR |
| 0xF234 | 0xF2374 | COPROC_WORKER_STATE |

IE32, IE64, M68K, and x86 CPUs access `0xF2340` directly (no gateway needed).

### Commands (COPROC_CMD)

| Value | Name | Description |
|-------|------|-------------|
| 1 | START | Load and start a worker from service binary file |
| 2 | STOP | Stop a running worker |
| 3 | ENQUEUE | Submit async request, returns ticket in COPROC_TICKET |
| 4 | POLL | Check ticket status (non-blocking) |
| 5 | WAIT | Block until ticket completes or timeout |

### Ticket Status (COPROC_TICKET_STATUS)

| Value | Meaning |
|-------|---------|
| 0 | Pending |
| 1 | Running |
| 2 | OK (completed successfully) |
| 3 | Error |
| 4 | Timeout |
| 5 | Worker down |

### Shared Mailbox RAM (0x820000 - 0x820FFF)

4KB ring buffer region shared between the Go manager and all worker CPUs. Contains 5 rings (one per supported CPU type), each 768 bytes with 16 request/response descriptor slots. Workers are loaded into dedicated, non-overlapping memory regions (0x200000-0x39FFFF). User data buffers for request/response payloads should be placed at 0x400000-0x7FFFFF.

### Access by CPU Type

| CPU | Register Addresses | Write Method | Include File |
|-----|-------------------|--------------|-------------|
| IE32 | `0xF2340` direct | 32-bit `STORE` | `ie32.inc` |
| IE64 | `0xF2340` direct | 32-bit `store.l` | `ie64.inc` |
| M68K | `0xF2340` direct | 32-bit `move.l` | `ie68.inc` |
| x86 | `0xF2340` direct | 32-bit `mov dword` | `ie86.inc` |
| Z80 | `0xF200` gateway | 4x byte `ld (addr),a` | `ie80.inc` |
| 6502 | `$F200` gateway | 4x byte `sta addr` | `ie65.inc` |

### ASM Helper Macros

All include files except `ie32.inc` (constants only, no macro support) provide coprocessor helper macros with identical semantics:

| Macro | Arguments | Description |
|-------|-----------|-------------|
| `coproc_start` | cpuType, namePtr | Start a worker from service binary |
| `coproc_stop` | cpuType | Stop a running worker |
| `coproc_enqueue` | cpuType, op, reqPtr, reqLen, respPtr, respCap | Enqueue async request (ticket in COPROC_TICKET) |
| `coproc_poll` | ticket | Poll ticket status (non-blocking) |
| `coproc_wait` | ticket, timeoutMs | Block until ticket completes or timeout |

For 16-bit CPUs (Z80/6502), macros use `STORE32` internally to compose 32-bit values from 4 byte writes through the gateway.

**Example (x86 — native 32-bit):**
```nasm
%include "ie86.inc"
    coproc_start COPROC_CPU_IE32, 0x400000    ; start IE32 worker
    coproc_enqueue COPROC_CPU_IE32, 1, 0x410000, 8, 0x410100, 4
    mov ebx, [COPROC_TICKET]                  ; save ticket
    coproc_poll ebx                           ; poll status
    mov eax, [COPROC_TICKET_STATUS]           ; read result
```

**Example (Z80 — gateway + byte writes):**
```z80
    .include "ie80.inc"
    coproc_start COPROC_CPU_M68K 0x400000     ; start M68K worker
    coproc_enqueue COPROC_CPU_M68K 1 0x410000 8 0x410100 4
    ld a,(COPROC_TICKET)                      ; read ticket byte 0
```

### EhBASIC Interface

From BASIC, use the high-level commands instead of direct MMIO access:

```basic
COSTART 3, "svc_6502.ie65"         ' Start 6502 worker
T = COCALL(3, 1, &H1000, 8, &H2000, 16)  ' Async RPC (op=1, add)
COWAIT T, 5000                      ' Wait up to 5 seconds
IF COSTATUS(T) = 2 THEN PRINT "OK" ' Check result
COSTOP 3                            ' Stop worker
```

Full reference: [ehbasic_ie64.md](docs/ehbasic_ie64.md)

### Caller Examples

Complete caller examples are provided for all CPU architectures:

| File | Caller CPU | Worker CPU | Description |
|------|-----------|------------|-------------|
| `assembler/coproc_caller_ie32.asm` | IE32 | IE32 | Native 32-bit register access |
| `assembler/coproc_caller_68k.asm` | M68K | IE32 | Uses `coproc_start`/`coproc_enqueue` macros |
| `assembler/coproc_caller_x86.asm` | x86 | IE32 | Uses NASM `%macro` helpers |
| `assembler/coproc_caller_z80.asm` | Z80 | M68K | Gateway access via `STORE32` macros |
| `assembler/coproc_caller_65.asm` | 6502 | IE32 | Gateway access via `STORE32` macros |

## 3.18 Voodoo 3D Graphics (0x0F4000 - 0x0F43FF)

The Voodoo chip emulates a 3DFX SST-1 graphics accelerator using High-Level Emulation (HLE). Instead of software rasterization, register writes are translated to GPU draw calls for hardware-accelerated 3D rendering with Vulkan (or software fallback).

**Important:** The Voodoo is disabled by default to allow per-scanline rendering for copper effects. Programs must explicitly enable it by writing 1 to `VOODOO_ENABLE` (0x0F4004) before using the 3D accelerator.

### Features

- Voodoo SST-1 register-compatible interface
- Gouraud shaded triangles with per-vertex color interpolation
- Z-buffering with all 8 depth compare functions (never, less, equal, lessequal, greater, notequal, greaterequal, always)
- Alpha testing with 8 comparison functions and configurable reference value
- Chroma key transparency (discard fragments matching key color)
- Configurable alpha blending with 9 blend factors per source/dest
- Texture mapping with per-vertex UV coordinates and color modulation
- Color combine modes (iterated, texture, modulate, add, decal) via fbzColorPath
- Depth-based fog with configurable fog color (linear blend based on vertex Z)
- Ordered dithering with 4x4 or 2x2 Bayer matrices for reduced banding
- Point sampling with wrap/clamp addressing modes
- Dynamic pipeline state with automatic pipeline caching for performance
- Scissor clipping
- 640x480 default, up to 800x600
- Compositor layer 20 (renders on top of all 2D chips)

### Register Map

```
Status/Control (0x0F4000 - 0x0F4007):
0x0F4000: VOODOO_STATUS      - Status (busy, vsync, fifo state) [read-only]
0x0F4004: VOODOO_ENABLE      - Write 1 to enable, 0 to disable (disabled by default)

Vertex Coordinates (0x0F4008 - 0x0F401F, 12.4 fixed-point):
0x0F4008: VOODOO_VERTEX_AX   - Vertex A X coordinate
0x0F400C: VOODOO_VERTEX_AY   - Vertex A Y coordinate
0x0F4010: VOODOO_VERTEX_BX   - Vertex B X coordinate
0x0F4014: VOODOO_VERTEX_BY   - Vertex B Y coordinate
0x0F4018: VOODOO_VERTEX_CX   - Vertex C X coordinate
0x0F401C: VOODOO_VERTEX_CY   - Vertex C Y coordinate

Vertex Attributes (0x0F4020 - 0x0F403F, 12.12 fixed-point):
0x0F4020: VOODOO_START_R     - Start red (1.0 = 0x1000)
0x0F4024: VOODOO_START_G     - Start green
0x0F4028: VOODOO_START_B     - Start blue
0x0F402C: VOODOO_START_Z     - Start Z depth (20.12 fixed-point)
0x0F4030: VOODOO_START_A     - Start alpha
0x0F4034: VOODOO_START_S     - Start S texture coord (14.18)
0x0F4038: VOODOO_START_T     - Start T texture coord (14.18)
0x0F403C: VOODOO_START_W     - Start W (perspective, 2.30)

Command Registers:
0x0F4080: VOODOO_TRIANGLE_CMD    - Submit triangle for rendering
0x0F4088: VOODOO_COLOR_SELECT    - Select vertex (0/1/2) for Gouraud shading
0x0F4104: VOODOO_FBZCOLOR_PATH   - Color combine mode configuration
0x0F4108: VOODOO_FOG_MODE        - Fog mode configuration
0x0F410C: VOODOO_ALPHA_MODE      - Alpha test/blend configuration
0x0F4110: VOODOO_FBZ_MODE        - Depth test/write/dither configuration
0x0F4118: VOODOO_CLIP_LEFT_RIGHT - Scissor rectangle X bounds
0x0F411C: VOODOO_CLIP_LOW_Y_HIGH - Scissor rectangle Y bounds
0x0F4124: VOODOO_FAST_FILL_CMD   - Clear framebuffer with COLOR0
0x0F4128: VOODOO_SWAP_BUFFER_CMD - Swap front/back buffers

Configuration:
0x0F41CC: VOODOO_CHROMA_KEY  - Chroma key color (0x00RRGGBB)
0x0F41D0: VOODOO_FOG_COLOR   - Fog color (0x00RRGGBB)
0x0F41D8: VOODOO_COLOR0      - Fill color for FAST_FILL_CMD (ARGB)
0x0F4214: VOODOO_VIDEO_DIM   - Video dimensions (width<<16 | height)

Texture Mapping (0x0F4300 - 0x0F433F):
0x0F4300: VOODOO_TEXTURE_MODE - Texture mode configuration
0x0F430C: VOODOO_TEX_BASE0    - Texture base address (LOD 0)
0x0F4330: VOODOO_TEX_WIDTH    - Texture width for upload (IE extension)
0x0F4334: VOODOO_TEX_HEIGHT   - Texture height for upload (IE extension)
0x0F4338: VOODOO_TEX_UPLOAD   - Write to trigger texture upload (IE extension)

Texture Memory (0x0F5000 - 0x0F5FFF):
0x0F5000: VOODOO_TEXMEM_BASE  - Texture memory base (64KB)
                              Write RGBA pixel data here, then trigger upload
```

### Voodoo Access by CPU Type

**x86 32-bit flat mode:** Direct memory access to 0xF4000-0xF43FF works:

```nasm
; x86 32-bit - direct access to Voodoo registers
mov dword [0xF4100], (640 << 16) | 480    ; VOODOO_VIDEO_DIM
mov dword [0xF4110], 0x0310               ; VOODOO_FBZ_MODE
mov dword [0xF4080], 0                    ; VOODOO_TRIANGLE_CMD
```

**Z80 / x86 real mode:** Cannot directly address 0xF4xxx. Use I/O ports 0xB0-0xB7:

| Port | Name | Description |
|------|------|-------------|
| 0xB0 | ADDR_LO | Register offset low byte (from VOODOO_BASE) |
| 0xB1 | ADDR_HI | Register offset high byte |
| 0xB2 | DATA0 | Data byte 0 (bits 0-7) |
| 0xB3 | DATA1 | Data byte 1 (bits 8-15) |
| 0xB4 | DATA2 | Data byte 2 (bits 16-23) |
| 0xB5 | DATA3 | Data byte 3 (bits 24-31) - triggers 32-bit write |
| 0xB6 | TEXSRC_LO | Texture source address low (RAM) |
| 0xB7 | TEXSRC_HI | Texture source address high (RAM) |

**I/O Port Usage:**
1. Set register offset (from 0xF4000) via ports 0xB0-0xB1
2. Write 4 data bytes to ports 0xB2-0xB5 (little-endian)
3. Writing to port 0xB5 triggers the 32-bit write to Voodoo

**Texture Upload via I/O Ports:**
1. Set texture dimensions via TEX_WIDTH/TEX_HEIGHT registers
2. Set source address in RAM via ports 0xB6-0xB7
3. Trigger upload via TEX_UPLOAD register - emulator copies from CPU RAM

```z80
; Z80 Example: Write 640x480 to VOODOO_VIDEO_DIM (offset 0x100)
    ld a, 0x00
    out (0xB0), a           ; Offset low = 0x00
    ld a, 0x01
    out (0xB1), a           ; Offset high = 0x01 (offset = 0x100)
    ld a, 0xE0
    out (0xB2), a           ; Height low (480 & 0xFF)
    ld a, 0x01
    out (0xB3), a           ; Height high (480 >> 8)
    ld a, 0x80
    out (0xB4), a           ; Width low (640 & 0xFF)
    ld a, 0x02
    out (0xB5), a           ; Width high - triggers write
```

### Fixed-Point Formats

| Format | Shift | Range | Usage |
|--------|-------|-------|-------|
| 12.4 | 4 | -2048.0 to 2047.9375 | Vertex coordinates |
| 12.12 | 12 | -2048.0 to 2047.999 | Colors (0.0-1.0 range: 0x0000-0x1000) |
| 20.12 | 12 | Large range | Z depth |
| 14.18 | 18 | 0.0 to 16383.999 | Texture coordinates |

### fbzMode Bits

| Bit | Name | Description |
|-----|------|-------------|
| 0 | CLIPPING | Enable scissor clipping |
| 1 | CHROMAKEY | Enable chroma key transparency |
| 4 | DEPTH_ENABLE | Enable depth buffer test |
| 5-7 | DEPTH_FUNC | Depth compare function (see table below) |
| 8 | DITHER | Enable ordered dithering (4x4 Bayer matrix) |
| 9 | RGB_WRITE | Enable RGB buffer write |
| 10 | DEPTH_WRITE | Enable depth buffer write |
| 11 | DITHER_2X2 | Use 2x2 Bayer matrix instead of 4x4 |

### Depth Compare Functions

The depth function (bits 5-7 of fbzMode) controls Z-buffer testing. Shift these values left by 5 to position in fbzMode.

| Value | Name | Description |
|-------|------|-------------|
| 0 | NEVER | Never pass (always discard fragment) |
| 1 | LESS | Pass if new Z < buffer Z |
| 2 | EQUAL | Pass if new Z == buffer Z |
| 3 | LESSEQUAL | Pass if new Z <= buffer Z |
| 4 | GREATER | Pass if new Z > buffer Z |
| 5 | NOTEQUAL | Pass if new Z != buffer Z |
| 6 | GREATEREQUAL | Pass if new Z >= buffer Z |
| 7 | ALWAYS | Always pass (disable depth test) |

### alphaMode Bits

| Bit | Name | Description |
|-----|------|-------------|
| 0 | ALPHA_TEST_EN | Enable alpha test |
| 1-3 | ALPHA_FUNC | Alpha test function (see table below) |
| 4 | ALPHA_BLEND_EN | Enable alpha blending |
| 8-11 | SRC_BLEND | Source blend factor |
| 12-15 | DST_BLEND | Destination blend factor |
| 24-31 | ALPHA_REF | Alpha reference value (0-255) |

### Alpha Test Functions

The alpha test function (bits 1-3 of alphaMode) compares fragment alpha against the reference value (bits 24-31). If the test fails, the fragment is discarded. Shift function values left by 1 to position in alphaMode.

| Value | Name | Description |
|-------|------|-------------|
| 0 | NEVER | Never pass (always discard fragment) |
| 1 | LESS | Pass if alpha < reference |
| 2 | EQUAL | Pass if alpha == reference |
| 3 | LESSEQUAL | Pass if alpha <= reference |
| 4 | GREATER | Pass if alpha > reference |
| 5 | NOTEQUAL | Pass if alpha != reference |
| 6 | GREATEREQUAL | Pass if alpha >= reference |
| 7 | ALWAYS | Always pass (alpha test disabled) |

Example: Discard fragments with alpha < 0.5 (reference=128):
```asm
    ; Enable alpha test with LESS function, reference = 128
    move.l  #(VOODOO_ALPHA_TEST_EN|(VOODOO_ALPHA_GREATER<<1)|(128<<24)),VOODOO_ALPHA_MODE
```

### Alpha Blend Factors

Use these values in bits 8-11 (source) and 12-15 (destination) of alphaMode. The final color is computed as: `result = src * srcFactor + dst * dstFactor`

| Value | Name | Description |
|-------|------|-------------|
| 0 | ZERO | Factor = 0 |
| 1 | SRC_ALPHA | Factor = source alpha |
| 2 | COLOR | Factor = constant color |
| 3 | DST_ALPHA | Factor = destination alpha |
| 4 | ONE | Factor = 1 |
| 5 | INV_SRC_A | Factor = 1 - source alpha |
| 6 | INV_COLOR | Factor = 1 - constant color |
| 7 | INV_DST_A | Factor = 1 - destination alpha |
| 15 | SATURATE | Factor = min(srcA, 1-dstA) |

Common blending modes:
- **Standard alpha blend**: srcFactor=SRC_ALPHA (1), dstFactor=INV_SRC_A (5)
- **Additive blend**: srcFactor=ONE (4), dstFactor=ONE (4)
- **Pre-multiplied alpha**: srcFactor=ONE (4), dstFactor=INV_SRC_A (5)

### Chroma Key

Chroma keying discards fragments that match a specific color, creating transparency without alpha blending. This is useful for sprite-based rendering where a specific color represents "transparent."

To enable chroma keying:
1. Set the key color in `VOODOO_CHROMA_KEY` (format: 0x00RRGGBB)
2. Enable chroma keying by setting bit 1 (CHROMAKEY) in `VOODOO_FBZ_MODE`

When enabled, any fragment whose RGB color matches the chroma key color will be discarded.

Example: Use magenta (255, 0, 255) as transparent color:
```asm
    ; Set chroma key to magenta
    move.l  #$00FF00FF,VOODOO_CHROMA_KEY

    ; Enable chroma keying in fbzMode
    move.l  #(VOODOO_FBZ_CHROMAKEY|VOODOO_FBZ_RGB_WRITE),VOODOO_FBZ_MODE
```

### textureMode Bits

| Bit | Name | Description |
|-----|------|-------------|
| 0 | TEX_ENABLE | Enable texture mapping |
| 1-3 | TEX_MINIFY | Minification filter (0=point) |
| 4 | TEX_MAGNIFY | Magnification filter (0=point, 1=bilinear) |
| 5 | TEX_CLAMP_S | Clamp S (U) coordinate (vs wrap) |
| 6 | TEX_CLAMP_T | Clamp T (V) coordinate (vs wrap) |
| 8-11 | TEX_FORMAT | Texture format (see table below) |

### Texture Formats

| Value | Name | Description |
|-------|------|-------------|
| 0 | 8BIT_PALETTE | 8-bit paletted texture |
| 5 | P8 | 8-bit palette (alternative) |
| 8 | ARGB1555 | 16-bit ARGB 1555 |
| 9 | ARGB4444 | 16-bit ARGB 4444 |
| 10 | ARGB8888 | 32-bit ARGB 8888 (default) |

### Texture Coordinates

Texture coordinates (S and T) use 14.18 fixed-point format. Values represent positions in texture space where 1.0 (0x40000) equals the texture width/height. Coordinates outside 0.0-1.0 wrap or clamp depending on TEX_CLAMP_S/T bits.

Per-vertex texture coordinates work like per-vertex colors: use `VOODOO_COLOR_SELECT` to select the vertex (0/1/2), then write to `VOODOO_START_S` and `VOODOO_START_T`.

### fbzColorPath (Color Combine)

The `VOODOO_FBZCOLOR_PATH` register (0x0F4104) controls how texture and vertex (iterated) colors are combined. This allows for various rendering effects from simple flat colors to complex texture blending.

#### fbzColorPath Bits

| Bit | Name | Description |
|-----|------|-------------|
| 0-1 | RGB_SELECT | RGB source select (see table below) |
| 2-3 | A_SELECT | Alpha source select (see table below) |
| 4-6 | CC_MSELECT | Color combine function mode |
| 27 | TEXTURE_ENABLE | Enable texture in color path |

#### Color Source Select Values

| Value | Name | Description |
|-------|------|-------------|
| 0 | ITERATED | Use iterated (vertex) color |
| 1 | TEXTURE | Use texture color |
| 2 | COLOR1 | Use constant color1 |
| 3 | LFB | Use linear framebuffer color |

#### Color Combine Functions (CC_MSELECT)

| Value | Name | Description |
|-------|------|-------------|
| 0 | ZERO | Output zero (black) |
| 1 | CSUB_CL | cother - clocal (subtract) |
| 2 | ALOCAL | clocal * alocal (modulate by local alpha) |
| 3 | AOTHER | clocal * aother (modulate by other alpha) |
| 4 | CLOCAL | clocal only (pass through) |
| 5 | ALOCAL_T | alocal * texture |
| 6 | CLOC_MUL | clocal * cother (multiply/modulate) |
| 7 | AOTHER_T | aother * texture |

#### Simplified Combine Modes

For convenience, pre-computed values are provided for common operations:

| Value | Name | Description |
|-------|------|-------------|
| 0x00 | COMBINE_ITERATED | Vertex color only (default when no texture) |
| 0x01 | COMBINE_TEXTURE | Texture color only |
| 0x61 | COMBINE_MODULATE | Texture × vertex color (most common for textured geometry) |
| 0x81 | COMBINE_ADD | Texture + vertex color (clamped, for glow effects) |
| 0x41 | COMBINE_DECAL | Texture with vertex alpha |

Example: Enable texture modulation (texture color multiplied by vertex color):
```asm
    ; Set color combine to MODULATE mode (tex * vert)
    move.l  #VOODOO_COMBINE_MODULATE,VOODOO_FBZCOLOR_PATH
```

### fogMode (Depth-Based Fog)

The `VOODOO_FOG_MODE` register (0x0F4108) enables depth-based fog blending. When enabled, fragment colors are linearly blended with the fog color based on the vertex Z coordinate (depth). Objects further from the camera appear more "fogged".

#### fogMode Bits

| Bit | Name | Description |
|-----|------|-------------|
| 0 | FOG_ENABLE | Enable fog processing |
| 1 | FOG_ADD | Add fog color to output (vs. blend) |
| 2 | FOG_MULT | Multiply fog factor by alpha |
| 3 | FOG_ZALPHA | Use Z alpha for fog (vs. iterated) |
| 4 | FOG_CONSTANT | Use constant fog alpha |
| 5 | FOG_DITHER | Apply dithering to fog |
| 6 | FOG_ZONES | Enable fog zones (table-based fog) |

The fog color is set in `VOODOO_FOG_COLOR` (0x0F41D0) using the format 0x00RRGGBB.

Fog blending formula: `output.rgb = mix(color.rgb, fogColor.rgb, fogFactor)`

Where `fogFactor` is derived from the vertex Z coordinate (0.0 = near/no fog, 1.0 = far/full fog).

Example: Enable gray fog for distance fade effect:
```asm
    ; Set fog color to gray
    move.l  #$00808080,VOODOO_FOG_COLOR

    ; Enable fog
    move.l  #VOODOO_FOG_ENABLE,VOODOO_FOG_MODE
```

### Dithering

Ordered dithering reduces color banding artifacts by applying a threshold pattern to pixel colors. The Voodoo supports two dither modes controlled by fbzMode bits:

- **4x4 Bayer matrix** (default): Higher quality, 16 threshold levels
- **2x2 Bayer matrix**: Faster, 4 threshold levels

Enable dithering by setting bit 8 (DITHER) in `VOODOO_FBZ_MODE`. For 2x2 mode, also set bit 11 (DITHER_2X2).

Example: Enable 4x4 dithering:
```asm
    ; Enable depth test, RGB write, and 4x4 dithering
    move.l  #(VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DITHER|(VOODOO_DEPTH_LESS<<5)),VOODOO_FBZ_MODE
```

Example: Fog with dithering for smooth distance fade:
```asm
    ; Set fog color
    move.l  #$00404040,VOODOO_FOG_COLOR

    ; Enable fog
    move.l  #VOODOO_FOG_ENABLE,VOODOO_FOG_MODE

    ; Enable depth, RGB write, and dithering
    move.l  #(VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DITHER|(VOODOO_DEPTH_LESS<<5)),VOODOO_FBZ_MODE
```

### Gouraud Shading

The Voodoo supports per-vertex colors for smooth Gouraud shading. Use `VOODOO_COLOR_SELECT` to select which vertex (0, 1, or 2 for vertices A, B, C) will receive subsequent writes to the `VOODOO_START_*` attribute registers:

1. Write vertex index (0/1/2) to `VOODOO_COLOR_SELECT`
2. Write R/G/B/A values to `VOODOO_START_R/G/B/A` - these are stored for the selected vertex
3. Repeat for each vertex with different colors
4. Submit triangle - colors will be smoothly interpolated across the surface

When `VOODOO_COLOR_SELECT` is not used (or set to 0), flat shading is applied using the last written color values.

### Example: Flat Shaded Triangle (M68K)

```asm
    include "ie68.inc"

    ; Enable the Voodoo graphics card (disabled by default)
    move.l  #1,VOODOO_ENABLE

    ; Clear screen to black
    move.l  #$FF000000,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; Set up depth test (less-than, write enabled)
    move.l  #(VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_WRITE|(VOODOO_DEPTH_LESS<<5)),VOODOO_FBZ_MODE

    ; Define triangle vertices (12.4 fixed-point: value << 4)
    move.l  #(320<<4),VOODOO_VERTEX_AX   ; Top center (320, 100)
    move.l  #(100<<4),VOODOO_VERTEX_AY
    move.l  #(420<<4),VOODOO_VERTEX_BX   ; Bottom right (420, 300)
    move.l  #(300<<4),VOODOO_VERTEX_BY
    move.l  #(220<<4),VOODOO_VERTEX_CX   ; Bottom left (220, 300)
    move.l  #(300<<4),VOODOO_VERTEX_CY

    ; Set red color (12.12 fixed-point: 1.0 = $1000)
    move.l  #$1000,VOODOO_START_R        ; R = 1.0
    move.l  #$0000,VOODOO_START_G        ; G = 0.0
    move.l  #$0000,VOODOO_START_B        ; B = 0.0
    move.l  #$1000,VOODOO_START_A        ; A = 1.0 (opaque)
    move.l  #$800000,VOODOO_START_Z      ; Z = 0.5

    ; Submit triangle
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Present frame
    move.l  #0,VOODOO_SWAP_BUFFER_CMD
```

### Example: Gouraud Shaded Triangle (M68K)

Per-vertex colors are set using `VOODOO_COLOR_SELECT` to specify which vertex (0/1/2) receives the subsequent `VOODOO_START_*` writes. The colors are smoothly interpolated across the triangle.

```asm
    include "ie68.inc"

    ; Enable the Voodoo graphics card (disabled by default)
    move.l  #1,VOODOO_ENABLE

    ; Clear screen to black
    move.l  #$FF000000,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; Enable depth test and RGB write
    move.l  #(VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_WRITE|(VOODOO_DEPTH_LESS<<5)),VOODOO_FBZ_MODE

    ; Define triangle vertices (12.4 fixed-point)
    move.l  #(320<<4),VOODOO_VERTEX_AX   ; Top center (320, 100)
    move.l  #(100<<4),VOODOO_VERTEX_AY
    move.l  #(420<<4),VOODOO_VERTEX_BX   ; Bottom right (420, 300)
    move.l  #(300<<4),VOODOO_VERTEX_BY
    move.l  #(220<<4),VOODOO_VERTEX_CX   ; Bottom left (220, 300)
    move.l  #(300<<4),VOODOO_VERTEX_CY

    ; Set vertex 0 (A) to RED
    move.l  #0,VOODOO_COLOR_SELECT       ; Select vertex 0
    move.l  #$1000,VOODOO_START_R        ; R = 1.0
    move.l  #$0000,VOODOO_START_G        ; G = 0.0
    move.l  #$0000,VOODOO_START_B        ; B = 0.0
    move.l  #$1000,VOODOO_START_A        ; A = 1.0

    ; Set vertex 1 (B) to GREEN
    move.l  #1,VOODOO_COLOR_SELECT       ; Select vertex 1
    move.l  #$0000,VOODOO_START_R        ; R = 0.0
    move.l  #$1000,VOODOO_START_G        ; G = 1.0
    move.l  #$0000,VOODOO_START_B        ; B = 0.0
    move.l  #$1000,VOODOO_START_A        ; A = 1.0

    ; Set vertex 2 (C) to BLUE
    move.l  #2,VOODOO_COLOR_SELECT       ; Select vertex 2
    move.l  #$0000,VOODOO_START_R        ; R = 0.0
    move.l  #$0000,VOODOO_START_G        ; G = 0.0
    move.l  #$1000,VOODOO_START_B        ; B = 1.0
    move.l  #$1000,VOODOO_START_A        ; A = 1.0

    ; Submit triangle (colors will interpolate smoothly)
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Present frame
    move.l  #0,VOODOO_SWAP_BUFFER_CMD
```

### Example: Alpha Blending (M68K)

This example demonstrates configuring alpha blending with source alpha and inverse source alpha factors (standard transparency).

```asm
    include "ie68.inc"

    ; Enable alpha blending: src*srcA + dst*(1-srcA)
    move.l  #(VOODOO_ALPHA_BLEND_EN|(VOODOO_BLEND_SRC_ALPHA<<8)|(VOODOO_BLEND_INV_SRC_A<<12)),VOODOO_ALPHA_MODE

    ; Draw opaque background triangle first
    ; ... (set vertices and full alpha color)
    move.l  #$1000,VOODOO_START_A        ; Alpha = 1.0 (opaque)
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Draw semi-transparent overlay triangle
    move.l  #$0800,VOODOO_START_A        ; Alpha = 0.5 (50% transparent)
    move.l  #$1000,VOODOO_START_R        ; Red
    move.l  #0,VOODOO_START_G
    move.l  #0,VOODOO_START_B
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Present frame
    move.l  #0,VOODOO_SWAP_BUFFER_CMD
```

### Example: Rotating Cube with Z-Buffer (M68K)

```asm
    include "ie68.inc"

    ; Main render loop
.frame_loop:
    ; Clear framebuffer
    move.l  #$FF000000,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; Draw 12 triangles (6 faces x 2 triangles each)
    ; Front face - red
    bsr     draw_face_front

    ; Back face - blue (will be Z-rejected when behind front)
    bsr     draw_face_back

    ; Present frame
    move.l  #VOODOO_SWAP_VSYNC,VOODOO_SWAP_BUFFER_CMD

    ; Update rotation angle
    add.w   #2,rotation_angle
    bra     .frame_loop
```

### Example: Textured Triangle (M68K)

This example demonstrates texture-mapped rendering with per-vertex UV coordinates.

```asm
    include "ie68.inc"

    ; Enable texturing with point sampling and wrap mode
    move.l  #VOODOO_TEX_ENABLE,VOODOO_TEXTURE_MODE

    ; Set up depth test and RGB write
    move.l  #(VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_WRITE|(VOODOO_DEPTH_LESS<<5)),VOODOO_FBZ_MODE

    ; Define triangle vertices (12.4 fixed-point)
    move.l  #(320<<4),VOODOO_VERTEX_AX   ; Top center
    move.l  #(100<<4),VOODOO_VERTEX_AY
    move.l  #(420<<4),VOODOO_VERTEX_BX   ; Bottom right
    move.l  #(300<<4),VOODOO_VERTEX_BY
    move.l  #(220<<4),VOODOO_VERTEX_CX   ; Bottom left
    move.l  #(300<<4),VOODOO_VERTEX_CY

    ; Vertex 0 (A): UV = (0.5, 0.0) - top center of texture
    move.l  #0,VOODOO_COLOR_SELECT
    move.l  #$1000,VOODOO_START_R        ; White color modulation
    move.l  #$1000,VOODOO_START_G
    move.l  #$1000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$20000,VOODOO_START_S       ; S = 0.5 (14.18 format: 0.5 * 0x40000)
    move.l  #$00000,VOODOO_START_T       ; T = 0.0

    ; Vertex 1 (B): UV = (1.0, 1.0) - bottom right
    move.l  #1,VOODOO_COLOR_SELECT
    move.l  #$1000,VOODOO_START_R
    move.l  #$1000,VOODOO_START_G
    move.l  #$1000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$40000,VOODOO_START_S       ; S = 1.0
    move.l  #$40000,VOODOO_START_T       ; T = 1.0

    ; Vertex 2 (C): UV = (0.0, 1.0) - bottom left
    move.l  #2,VOODOO_COLOR_SELECT
    move.l  #$1000,VOODOO_START_R
    move.l  #$1000,VOODOO_START_G
    move.l  #$1000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$00000,VOODOO_START_S       ; S = 0.0
    move.l  #$40000,VOODOO_START_T       ; T = 1.0

    ; Submit triangle
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Present frame
    move.l  #0,VOODOO_SWAP_BUFFER_CMD
```

# 4. IE32 CPU Architecture

The IE32 implements a 32-bit RISC-like architecture with fixed-width instructions and a clean, orthogonal instruction set.

## 4.1 Register Set

The CPU provides 16 general-purpose 32-bit registers organised in two logical banks:

**First Bank (A-H):**
```
A - Primary accumulator/general purpose
B - General purpose
C - General purpose
D - General purpose
E - General purpose
F - General purpose
G - General purpose
H - General purpose
```

**Second Bank (S-Z):**
```
S - General purpose/stack operations
T - General purpose
U - General purpose
V - General purpose
W - General purpose
X - General purpose/index
Y - General purpose/index
Z - General purpose/index
```

While the register naming suggests traditional roles (like X/Y/Z for indexing), all registers are fully general-purpose and can be used interchangeably.

**Special Registers:**
```
PC - Program Counter (32-bit)
SP - Stack Pointer (32-bit, initialised to 0xE0000)
```

## 4.2 Status Flags

The IE32 uses implicit status flags based on the result of the last operation:
- Operations that produce a zero result set an internal zero flag
- Comparison instructions (used by conditional jumps) compare register values directly

## 4.3 Addressing Modes

The system supports five addressing modes:

**Immediate (ADDR_IMMEDIATE = 0x00)**
```assembly
LOAD A, #42        ; Load value 42 into register A
```

**Register (ADDR_REGISTER = 0x01)**
```assembly
ADD A, X           ; Add X register to A register
```

**Register Indirect (ADDR_REG_IND = 0x02)**
```assembly
LOAD A, [X]        ; Load from address in X
LOAD A, [X+4]      ; Load from address in X plus 4
```

**Memory Indirect (ADDR_MEM_IND = 0x03)**
```assembly
LOAD A, [0x1000]   ; Load from address stored at memory location 0x1000
```

**Direct (ADDR_DIRECT = 0x04)**
```assembly
STORE A, @0xF0900  ; Store A's value directly to memory address 0xF0900
LOAD A, @0x1000    ; Load value directly from memory address 0x1000
```

The direct addressing mode is used for memory-mapped I/O operations, providing efficient access to hardware registers without double indirection.

## 4.4 Instruction Format

Every instruction is exactly 8 bytes long, providing a consistent and easy-to-decode format:

```
Byte 0: Opcode
Byte 1: Register specifier
Byte 2: Addressing mode
Byte 3: Reserved (must be 0)
Bytes 4-7: 32-bit operand value
```

## 4.5 Instruction Set

### Data Movement Instructions
```assembly
; Traditional load/store
LOAD  (0x01) ; Load value into register
STORE (0x02) ; Store register to memory

; Register-specific loads
LDA (0x20) ; Load to A
LDB (0x3A) ; Load to B
LDC (0x3B) ; Load to C
LDD (0x3C) ; Load to D
LDE (0x3D) ; Load to E
LDF (0x3E) ; Load to F
LDG (0x3F) ; Load to G
LDH (0x4C) ; Load to H
LDS (0x4D) ; Load to S
LDT (0x4E) ; Load to T
LDU (0x40) ; Load to U
LDV (0x41) ; Load to V
LDW (0x42) ; Load to W
LDX (0x21) ; Load to X
LDY (0x22) ; Load to Y
LDZ (0x23) ; Load to Z

; Register-specific stores
STA (0x24) ; Store from A
STB (0x43) ; Store from B
STC (0x44) ; Store from C
STD (0x45) ; Store from D
STE (0x46) ; Store from E
STF (0x47) ; Store from F
STG (0x48) ; Store from G
STH (0x4F) ; Store from H
STS (0x50) ; Store from S
STT (0x51) ; Store from T
STU (0x49) ; Store from U
STV (0x4A) ; Store from V
STW (0x4B) ; Store from W
STX (0x25) ; Store from X
STY (0x26) ; Store from Y
STZ (0x27) ; Store from Z

; Increment/Decrement
INC (0x28) ; Increment
DEC (0x29) ; Decrement

; Stack operations
PUSH (0x12) ; Push register to stack
POP  (0x13) ; Pop from stack to register
```

### Arithmetic Instructions
```assembly
ADD (0x03) ; Add
SUB (0x04) ; Subtract
MUL (0x14) ; Multiply
DIV (0x15) ; Divide
MOD (0x16) ; Modulus
```

### Logical Instructions
```assembly
AND (0x05) ; Bitwise AND
OR  (0x09) ; Bitwise OR
XOR (0x0A) ; Bitwise XOR
NOT (0x0D) ; Bitwise NOT
SHL (0x0B) ; Shift left
SHR (0x0C) ; Shift right
```

### Control Flow Instructions
```assembly
JMP (0x06) ; Unconditional jump
JNZ (0x07) ; Jump if not zero
JZ  (0x08) ; Jump if zero
JGT (0x0E) ; Jump if greater than
JGE (0x0F) ; Jump if greater or equal
JLT (0x10) ; Jump if less than
JLE (0x11) ; Jump if less or equal
JSR (0x18) ; Jump to subroutine
RTS (0x19) ; Return from subroutine
```

### Interrupt Management Instructions
```assembly
SEI (0x1A) ; Set Enable Interrupts
CLI (0x1B) ; Clear Interrupt Enable
RTI (0x1C) ; Return from Interrupt
```

### System Control Instructions
```assembly
WAIT (0x17) ; Wait for specified cycles
NOP  (0xEE) ; No operation
HALT (0xFF) ; Stop execution
```

## 4.6 Memory and I/O Integration

- The IE32 uses the shared 32MB system bus
- All memory-mapped devices (video, audio, PSG/POKEY/SID, terminal) are accessible
- I/O region: 0x0F0000 - 0x0FFFFF
- VRAM access: 0x100000 - 0x4FFFFF (direct 32-bit addressing)
- Stack grows downward from 0xE0000

## 4.7 Interrupt Handling

The system implements a simple but effective interrupt system:

1. **Interrupt Vector**
    - Located at address 0x0000
    - Contains the address of the interrupt service routine (ISR)
    - Must be initialised before enabling interrupts

2. **Interrupt Control**
    - SEI enables interrupts
    - CLI disables interrupts
    - Interrupts automatically disabled during ISR execution

3. **Interrupt Processing**
   When an interrupt occurs:
    - Current PC is pushed onto stack
    - CPU jumps to ISR address from vector
    - Interrupts are disabled until RTI

4. **Timer Interrupts**
   The system timer can generate periodic interrupts:

```assembly
; Configure timer interrupt
LOAD A, #44100     ; Set period (1 second at 44.1kHz)
STORE A, @0xF0808  ; Write to TIMER_PERIOD
LOAD A, #1
STORE A, @0xF0800  ; Enable timer
SEI                ; Enable interrupts
```

## 4.8 Compatibility Notes

- Little-endian byte order for memory bus operations
- Fixed 8-byte instruction size
- All registers are 32-bit
- Word-aligned memory access recommended for performance

# 5. MOS 6502 CPU

The Intuition Engine includes an NMOS 6502 core for running raw 8-bit binaries. The 6502 shares the same memory-mapped I/O and device map as IE32 and M68K, so hardware registers behave identically across CPU modes.

## 5.1 Register Set

The 6502 core exposes the classic NMOS register file:

```
A  - Accumulator (8-bit) for arithmetic and logic
X  - Index register X (8-bit)
Y  - Index register Y (8-bit)
SP - Stack Pointer (8-bit), stack page fixed at 0x0100-0x01FF
PC - Program Counter (16-bit)
SR - Status Register (8-bit flags)
```

## 5.2 Status Flags

The status register follows NMOS 6502 semantics:

```
Bit 7: N - Negative flag
Bit 6: V - Overflow flag
Bit 5: - - Unused (always 1 when pushed)
Bit 4: B - Break command
Bit 3: D - Decimal mode
Bit 2: I - IRQ Disable
Bit 1: Z - Zero flag
Bit 0: C - Carry flag
```

## 5.3 Addressing Modes

Supported addressing modes match the NMOS 6502 set used by common assemblers:

| Mode | Syntax | Description |
|------|--------|-------------|
| Immediate | #$nn | Operand is the byte following the opcode |
| Zero Page | $nn | 8-bit address in page zero |
| Zero Page,X | $nn,X | Zero page indexed by X |
| Zero Page,Y | $nn,Y | Zero page indexed by Y |
| Absolute | $nnnn | 16-bit address |
| Absolute,X | $nnnn,X | Absolute indexed by X |
| Absolute,Y | $nnnn,Y | Absolute indexed by Y |
| Indirect | ($nnnn) | Indirect (JMP only) |
| (Indirect,X) | ($nn,X) | Indexed indirect |
| (Indirect),Y | ($nn),Y | Indirect indexed |
| Relative | $nn | Signed offset for branches |
| Accumulator | A | Operand is accumulator |
| Implied | | Operand implied by instruction |

## 5.4 Instruction Set

The 6502 implements all 56 documented NMOS instructions plus unofficial opcodes:

**Load/Store:** LDA, LDX, LDY, STA, STX, STY
**Transfer:** TAX, TAY, TSX, TXA, TXS, TYA
**Stack:** PHA, PHP, PLA, PLP
**Arithmetic:** ADC, SBC, INC, INX, INY, DEC, DEX, DEY
**Logical:** AND, EOR, ORA, BIT
**Shift/Rotate:** ASL, LSR, ROL, ROR
**Compare:** CMP, CPX, CPY
**Branch:** BCC, BCS, BEQ, BMI, BNE, BPL, BVC, BVS
**Jump/Return:** JMP, JSR, RTS, RTI, BRK
**Flags:** CLC, CLD, CLI, CLV, SEC, SED, SEI
**No-op:** NOP

## 5.5 Memory and I/O Integration

- The 6502 uses the shared system bus and all memory-mapped devices
- Native 16-bit address space (0x0000-0xFFFF)
- VRAM access via banking at 0x8000-0xBFFF (16KB window)
- Bank select register at 0xF7F0
- VRAM banking is disabled until the first write to the bank register
- Extended bank windows for IE65:
  - Bank 1: 0x2000-0x3FFF (8KB, sprite data)
  - Bank 2: 0x4000-0x5FFF (8KB, font data)
  - Bank 3: 0x6000-0x7FFF (8KB, general/AY data)
- Bank control registers: 0xF700-0xF705

**Audio Chip Access (6502 native addresses):**
| Chip  | Address Range |
|-------|---------------|
| PSG   | $D400-$D40D   |
| POKEY | $D200-$D209   |
| SID   | $D500-$D51C   |

## 5.6 Interrupts and Vectors

Vector locations follow standard 6502 layout:
```
0xFFFA-0xFFFB: NMI vector
0xFFFC-0xFFFD: RESET vector
0xFFFE-0xFFFF: IRQ/BRK vector
```

The loader seeds these vectors for raw binaries; custom binaries may overwrite them.

## 5.7 Compatibility Notes

- **NMOS 6502 only**: 65C02 opcodes are not supported
- Decimal mode is fully implemented
- Cycle-accurate instruction timing
- Use Klaus tests to validate D-flag behavior
- Use `-m6502` flag to run 6502 binaries
- `--load-addr` sets the load address (default 0x0600)
- `--entry` sets the entry address (defaults to load address)

# 6. Zilog Z80 CPU

The Intuition Engine includes a Z80 core for running raw 8-bit binaries. It shares the same memory map and device registers as the other CPU modes.

## 6.1 Register Set

The Z80 provides a rich register set with shadow registers:

**Main Registers:**
```
A  - Accumulator (8-bit)
F  - Flags (8-bit)
B, C - General purpose (8-bit each, BC as 16-bit pair)
D, E - General purpose (8-bit each, DE as 16-bit pair)
H, L - General purpose (8-bit each, HL as 16-bit pair)
```

**Shadow Registers:**
```
A', F', B', C', D', E', H', L' - Alternate register set
```

**Index Registers:**
```
IX - Index register X (16-bit)
IY - Index register Y (16-bit)
```

**Special Registers:**
```
SP - Stack Pointer (16-bit)
PC - Program Counter (16-bit)
I  - Interrupt Vector (8-bit)
R  - Refresh Counter (8-bit)
```

## 6.2 Status Flags

The F register contains:
```
Bit 7: S  - Sign flag
Bit 6: Z  - Zero flag
Bit 5: Y  - Undocumented (copy of bit 5 of result)
Bit 4: H  - Half-carry flag
Bit 3: X  - Undocumented (copy of bit 3 of result)
Bit 2: P/V - Parity/Overflow flag
Bit 1: N  - Add/Subtract flag
Bit 0: C  - Carry flag
```

## 6.3 Addressing Modes

| Mode | Syntax | Description |
|------|--------|-------------|
| Immediate | n | 8-bit immediate value |
| Immediate Extended | nn | 16-bit immediate value |
| Register | r | Single register |
| Register Pair | rr | 16-bit register pair (BC, DE, HL, SP) |
| Indirect | (HL) | Memory at address in HL |
| Indexed | (IX+d), (IY+d) | Indexed with signed displacement |
| Extended | (nn) | Direct 16-bit address |
| Relative | e | Signed 8-bit offset for jumps |
| Bit | b | Bit number (0-7) |

## 6.4 Instruction Set

The Z80 implements a comprehensive instruction set including:

**8-bit Load:** LD r,r' / LD r,n / LD r,(HL) / LD (HL),r
**16-bit Load:** LD rr,nn / LD (nn),HL / LD HL,(nn) / PUSH/POP
**Exchange:** EX DE,HL / EX AF,AF' / EXX
**Arithmetic:** ADD, ADC, SUB, SBC, AND, OR, XOR, CP, INC, DEC
**16-bit Arithmetic:** ADD HL,rr / ADC HL,rr / SBC HL,rr / INC/DEC rr
**Rotate/Shift:** RLCA, RRCA, RLA, RRA, RLC, RRC, RL, RR, SLA, SRA, SRL
**Bit Operations:** BIT, SET, RES
**Jump:** JP, JR, DJNZ
**Call/Return:** CALL, RET, RETI, RETN, RST
**Input/Output:** IN, OUT
**Block Transfer:** LDI, LDIR, LDD, LDDR
**Block Search:** CPI, CPIR, CPD, CPDR
**Block I/O:** INI, INIR, IND, INDR, OUTI, OTIR, OUTD, OTDR

## 6.5 Memory and I/O Integration

- The Z80 uses the shared system bus
- Native 16-bit address space (0x0000-0xFFFF)
- Z80 `IN/OUT` ports map to the 16-bit address space as memory-mapped registers
- VRAM access via banking similar to 6502

**Port-Based Audio Chip Access:**
| Chip  | Ports   |
|-------|---------|
| PSG   | 0xF0-0xF1 |
| POKEY | 0xD0-0xD1 |
| SID   | 0xE0-0xE1 |

First port selects the register, second port reads/writes data.

## 6.6 Interrupts

The Z80 supports three interrupt modes:

**Mode 0:** External device places instruction on data bus (typically RST)
**Mode 1:** Jump to fixed address 0x0038
**Mode 2:** Vectored interrupts using I register as high byte

Interrupt control:
```assembly
DI      ; Disable interrupts
EI      ; Enable interrupts
IM 0/1/2 ; Set interrupt mode
```

## 6.7 Compatibility Notes

- Full Z80 instruction set including undocumented opcodes
- Use `-z80` flag to run Z80 binaries
- `--load-addr` sets the load address (default 0x0000)
- `--entry` sets the entry address (defaults to load address)
- Shadow registers fully implemented
- Block transfer and search instructions implemented
- All interrupt modes supported

# 7. Motorola 68020 CPU with FPU

In addition to the IE32 instruction set, the Intuition Engine includes a complete Motorola 68020 CPU emulator with 68881/68882 FPU (Floating Point Unit) support.

## 7.1 Register Set

**Data Registers:**
```
D0-D7 - Eight 32-bit data registers
        Can be used as byte (.B), word (.W), or long (.L)
```

**Address Registers:**
```
A0-A6 - Seven 32-bit address registers
A7    - Stack pointer (SSP in supervisor mode, USP in user mode)
```

**Special Registers:**
```
PC   - Program Counter (32-bit)
SR   - Status Register (16-bit)
CCR  - Condition Code Register (low byte of SR)
USP  - User Stack Pointer
SSP  - Supervisor Stack Pointer
VBR  - Vector Base Register
SFC  - Source Function Code
DFC  - Destination Function Code
CACR - Cache Control Register
CAAR - Cache Address Register
```

**FPU Registers (68881/68882):**
```
FP0-FP7 - Eight 80-bit floating-point registers
FPCR    - Floating-Point Control Register
FPSR    - Floating-Point Status Register
FPIAR   - Floating-Point Instruction Address Register
```

## 7.2 Status Flags

**Condition Code Register (CCR):**
```
Bit 4: X - Extend (copy of carry for multi-precision)
Bit 3: N - Negative
Bit 2: Z - Zero
Bit 1: V - Overflow
Bit 0: C - Carry
```

**System Byte:**
```
Bit 15: T1 - Trace enable
Bit 14: T0 - Trace enable
Bit 13: S  - Supervisor state
Bits 10-8: IPL - Interrupt priority level mask
```

## 7.3 Addressing Modes

The 68020 supports 12 basic addressing modes plus extensions:

| Mode | Syntax | Description |
|------|--------|-------------|
| Data Register Direct | Dn | Data in register |
| Address Register Direct | An | Address in register |
| Address Register Indirect | (An) | Memory at address in An |
| Address Indirect Postincrement | (An)+ | Indirect, then increment An |
| Address Indirect Predecrement | -(An) | Decrement An, then indirect |
| Address Indirect with Displacement | (d16,An) | An + signed 16-bit offset |
| Address Indirect with Index | (d8,An,Xn) | An + Xn + signed 8-bit offset |
| Absolute Short | (xxx).W | 16-bit address, sign-extended |
| Absolute Long | (xxx).L | Full 32-bit address |
| PC with Displacement | (d16,PC) | PC + signed 16-bit offset |
| PC with Index | (d8,PC,Xn) | PC + Xn + signed 8-bit offset |
| Immediate | #<data> | Immediate value |

**68020-Specific Extensions:**
| Mode | Syntax | Description |
|------|--------|-------------|
| Memory Indirect Preindexed | ([bd,An,Xn],od) | Double indirection with preindex |
| Memory Indirect Postindexed | ([bd,An],Xn,od) | Double indirection with postindex |
| PC Memory Indirect | ([bd,PC,Xn],od) | PC-relative indirect |
| Scaled Indexing | (d8,An,Xn*scale) | Scale factor ×1, ×2, ×4, or ×8 |

## 7.4 Instruction Set

**Data Movement:**
MOVE, MOVEA, MOVEM, MOVEQ, MOVEP, LEA, PEA, EXG, SWAP, LINK, UNLK

**Arithmetic:**
ADD, ADDA, ADDI, ADDQ, ADDX
SUB, SUBA, SUBI, SUBQ, SUBX
MULU, MULS, DIVU, DIVS, DIVUL, DIVSL
NEG, NEGX, CLR, CMP, CMPA, CMPI, CMPM
TST, EXT, EXTB

**Logical:**
AND, ANDI, OR, ORI, EOR, EORI, NOT

**Shift and Rotate:**
ASL, ASR, LSL, LSR, ROL, ROR, ROXL, ROXR

**Bit Manipulation:**
BTST, BCHG, BCLR, BSET

**Bit Field (68020):**
BFTST, BFEXTU, BFEXTS, BFCHG, BFCLR, BFSET, BFFFO, BFINS

**BCD Arithmetic:**
ABCD, SBCD, NBCD, PACK, UNPK

**Program Control:**
Bcc (14 conditions), DBcc, Scc, JMP, JSR, RTS, RTE, RTR, RTD, TRAP, TRAPV, CHK, CHK2, TAS

**System Control:**
MOVE to/from SR, MOVE USP, MOVEC, MOVES, RESET, STOP, NOP, ILLEGAL, ORI/ANDI/EORI to CCR/SR

**Atomic Operations (68020):**
CAS, CAS2

## 7.5 FPU (68881/68882) Features

### Data Types:
- **80-bit extended precision** (IEEE 754 compliant)
- 8 floating-point registers (FP0-FP7)
- Control registers: FPCR, FPSR, FPIAR

### Basic Operations:
| Instruction | Description |
|-------------|-------------|
| FMOVE | Move floating-point data |
| FADD | Add |
| FSUB | Subtract |
| FMUL | Multiply |
| FDIV | Divide |
| FNEG | Negate |
| FABS | Absolute value |
| FCMP | Compare |
| FTST | Test |
| FSQRT | Square root |
| FINT | Integer part |
| FINTRZ | Integer part (round to zero) |

### Transcendental Functions:
| Instruction | Description |
|-------------|-------------|
| FSIN | Sine |
| FCOS | Cosine |
| FTAN | Tangent |
| FASIN | Arc sine |
| FACOS | Arc cosine |
| FATAN | Arc tangent |
| FSINH | Hyperbolic sine |
| FCOSH | Hyperbolic cosine |
| FTANH | Hyperbolic tangent |
| FLOG10 | Base-10 logarithm |
| FLOGN | Natural logarithm |
| FLOG2 | Base-2 logarithm |
| FETOX | e^x |
| FTWOTOX | 2^x |
| FTENTOX | 10^x |

### ROM Constants (FMOVECR):
The FPU provides built-in constants:
- Pi (π)
- e (Euler's number)
- log₂(e), log₁₀(e)
- ln(2), ln(10)
- Powers of 10 (10⁰ through 10⁴)

### FPU Condition Codes:
- N (Negative) - Result is negative
- Z (Zero) - Result is zero
- I (Infinity) - Result is infinite
- NAN - Result is Not a Number

## 7.6 Memory and I/O Integration

- Uses shared 32MB system bus
- 24-bit address mask (16MB accessible via address bus)
- Big-endian byte order
- I/O region: 0x00F00000 - 0x00FFFFFF
- VRAM: 0x00100000 - 0x004FFFFF (direct access)
- Exception vector table: 0x00000000 (relocatable via VBR)
- Default stack: 0x00FF0000

## 7.7 Interrupts and Exceptions

**Exception Vector Table (256 vectors):**
```
Vector 0: Initial SSP
Vector 1: Initial PC (reset)
Vector 2: Bus Error
Vector 3: Address Error
Vector 4: Illegal Instruction
Vector 5: Zero Divide
Vector 6: CHK/CHK2 Instruction
Vector 7: TRAPcc, TRAPV, cpTRAPcc
Vector 8: Privilege Violation
Vector 9: Trace
Vector 10: Line-A Emulator
Vector 11: Line-F Emulator (FPU)
Vectors 24-31: Spurious + Auto-vectored Interrupts
Vectors 32-47: TRAP #0-15
Vectors 48-63: FPU Exceptions
Vectors 64-255: User Defined
```

**Interrupt Priorities:**
- Level 7: Non-maskable
- Levels 1-6: Maskable (compared against SR IPL)
- Level 0: No interrupt

## 7.8 Compatibility Notes

- 95%+ instruction coverage (68020 + 68881/68882)
- 68EC020 variant (no MMU)
- Big-endian byte order (converted from host)
- F-line opcodes route to FPU when present
- Use `-m68k` flag to run M68K binaries
- File extension: `.ie68`

**Not Implemented:**
- MMU and address translation
- Coprocessor interface (beyond FPU)
- Instruction cache emulation
- Trace mode (T0/T1 bits defined but not enforced)
- Dynamic bus sizing

# 8. Intel x86 CPU (32-bit)

The Intuition Engine includes an x86 core implementing the 8086 instruction set with 32-bit register extensions in a flat memory model. This provides 32-bit programming without the complexity of protected mode, segment descriptors, or paging.

## 8.1 Register Set

The x86 provides 32-bit general purpose registers with 16-bit and 8-bit access:

**General Purpose Registers:**
```
EAX (AX, AH, AL) - Accumulator
EBX (BX, BH, BL) - Base
ECX (CX, CH, CL) - Counter
EDX (DX, DH, DL) - Data
ESI (SI)         - Source Index
EDI (DI)         - Destination Index
EBP (BP)         - Base Pointer
ESP (SP)         - Stack Pointer
```

**Special Registers:**
```
EIP - Instruction Pointer (32-bit)
EFLAGS - Status flags (32-bit)
```

**Segment Registers (for 8086 compatibility):**
```
CS - Code Segment
DS - Data Segment
ES - Extra Segment
SS - Stack Segment
FS, GS - Additional segments (386+)
```

## 8.2 Status Flags

The EFLAGS register contains:
```
Bit 0:  CF - Carry flag
Bit 2:  PF - Parity flag
Bit 4:  AF - Auxiliary carry flag
Bit 6:  ZF - Zero flag
Bit 7:  SF - Sign flag
Bit 8:  TF - Trap flag
Bit 9:  IF - Interrupt enable flag
Bit 10: DF - Direction flag
Bit 11: OF - Overflow flag
```

## 8.3 Addressing Modes

| Mode | Syntax | Description |
|------|--------|-------------|
| Immediate | imm8/imm16/imm32 | Immediate value |
| Register | reg | Register operand |
| Direct | [addr] | Direct memory address |
| Register Indirect | [reg] | Memory at register address |
| Base+Displacement | [reg+disp] | Base register + offset |
| SIB | [base+index*scale+disp] | Full 386 addressing |

## 8.4 Instruction Set

The x86 core implements the 8086 instruction set with 32-bit register support:

**Data Transfer:** MOV, PUSH, POP, XCHG, LEA, LES, LDS
**Arithmetic:** ADD, ADC, SUB, SBB, MUL, IMUL, DIV, IDIV, INC, DEC, CMP, NEG
**Logical:** AND, OR, XOR, NOT, TEST
**Shift/Rotate:** SHL, SHR, SAL, SAR, ROL, ROR, RCL, RCR
**Control Flow:** JMP, Jcc, CALL, RET, LOOP, LOOPE, LOOPNE
**String:** MOVS, STOS, LODS, CMPS, SCAS with REP/REPE/REPNE prefixes
**I/O:** IN, OUT (port-based I/O for audio chips)
**Flag Control:** CLC, STC, CMC, CLD, STD, CLI, STI
**BCD:** DAA, DAS, AAA, AAS, AAM, AAD

**32-bit Register Extensions:**
- 32-bit register operations (EAX, EBX, etc.)
- Operand size prefix (0x66) for 16/32-bit switching
- Address size prefix (0x67)
- SIB byte addressing for complex memory operands

**Additional Instructions:**
- MOVZX, MOVSX (zero/sign extend)
- SETcc (conditional byte set)
- Bit test: BT, BTS, BTR, BTC, BSF, BSR
- SHLD, SHRD (double-precision shifts)

## 8.5 Memory and I/O Integration

- Full 32-bit flat address space (32MB system RAM)
- VGA VRAM at standard PC address 0xA0000-0xAFFFF
- Hardware registers memory-mapped at 0xF0000+
- Separate I/O port space for audio chips

**Port-Based Audio Chip Access:**
| Chip  | Ports     | Description |
|-------|-----------|-------------|
| PSG   | 0xF0-0xF1 | Register select, data |
| POKEY | 0xD0-0xDF | Direct register access |
| SID   | 0xE0-0xE1 | Register select, data |
| TED   | 0xF2-0xF3 | Register select, data |

**Standard VGA Ports:**
| Port  | Description |
|-------|-------------|
| 0x3C4-0x3C5 | Sequencer index/data |
| 0x3C6-0x3C9 | DAC mask, read/write index, data |
| 0x3CE-0x3CF | Graphics controller index/data |
| 0x3D4-0x3D5 | CRTC index/data |
| 0x3DA | Input status (VSync) |

## 8.6 Interrupts

The x86 supports software interrupts:
```assembly
INT n     ; Call interrupt n
INT 3     ; Breakpoint
INTO      ; Overflow interrupt
IRET      ; Return from interrupt
```

## 8.7 Compatibility Notes

- Use `-x86` flag to run x86 binaries
- File extension: `.ie86`
- Use NASM or FASM for assembly with `ie86.inc` include file
- `--load-addr` sets the load address (default 0x00000000)
- `--entry` sets the entry point (defaults to load address)

**Memory Model:**

The x86 core uses a simplified flat memory model:
- Segment registers exist but are ignored for address calculation
- All memory accesses use the 32-bit offset directly (no segment:offset)
- Full 32-bit address space accessible without segment arithmetic
- This is neither true real mode (1MB limit) nor protected mode

**x87 FPU (387 scope):**
- x87 escape opcodes `D8`-`DF` are implemented with stack-based floating-point operations
- Data movement, arithmetic, compares, control ops, and ENV/SAVE/RESTORE paths are supported
- Deliberate exclusions: `FCMOV*`, `FCOMI/FCOMIP`, `FUCOMI/FUCOMIP`, and SSE-family instructions
- Deliberate deviations: large-argument trig reduction always completes (`C2=0`), `FPREM/FPREM1` complete in one step (`C2=0`), and ENV/SAVE always use the 32-bit layout

**Not Implemented:**
- Real mode segment:offset addressing
- Protected mode (descriptor tables, privilege levels)
- Virtual 8086 mode
- Paging and virtual memory
- Task switching

# 9. IE64 CPU Architecture

The IE64 is a custom 64-bit RISC processor designed for the Intuition Engine. It uses a clean load-store architecture with compare-and-branch semantics (no flags register), 32 general-purpose registers, and fixed 8-byte instructions.

## 9.1 Register Set

| Register | Width | Description |
|----------|-------|-------------|
| R0 | 64-bit | Hardwired zero (writes ignored) |
| R1–R30 | 64-bit | General-purpose |
| R31 (SP) | 64-bit | Stack pointer |
| PC | 64-bit | Program counter (masked to 25 bits / 32MB) |

**No flags register.** Conditional branches embed a register comparison directly (e.g., `BEQ Rs, Rt, offset`).

**Initial State:**
- PC = `$1000` (program start)
- SP (R31) = `$9F000` (top of stack, grows downward)
- All other registers = 0

## 9.2 Instruction Format

All instructions are 8 bytes (64 bits), little-endian:

```
Byte 0:   Opcode      (8 bits)
Byte 1:   Rd[4:0]     (5 bits) | Size[1:0] (2 bits) | X (1 bit)
Byte 2:   Rs[4:0]     (5 bits) | unused    (3 bits)
Byte 3:   Rt[4:0]     (5 bits) | unused    (3 bits)
Bytes 4-7: imm32      (32-bit LE immediate)
```

**Size codes:** `.B` (8-bit), `.W` (16-bit), `.L` (32-bit), `.Q` (64-bit, default)

**X bit:** 0 = third operand is register (Rt), 1 = third operand is immediate (imm32)

## 9.3 Addressing Modes

| Mode | Syntax | Description | Example |
|------|--------|-------------|---------|
| Immediate | `#value` | Constant value | `move.l r1, #42` |
| Register | `Rn` | Register contents | `add r1, r2, r3` |
| Register-indirect (data) | `(Rs)` | Memory at address in Rs | `load.l r1, (r2)` |
| Register-indirect (control) | `(Rs)` | Transfer control to address in Rs | `jmp (r5)` |
| Displacement | `offset(Rs)` | Memory/control at Rs + offset | `load.l r1, 16(r2)` |
| PC-relative | `label` | Branch target relative to PC | `bra loop` |

## 9.4 Instruction Set

### Data Movement

| Mnemonic | Description | Example |
|----------|-------------|---------|
| `MOVE` | Move register/immediate to register | `move.l r1, #100` |
| `MOVT` | Move to upper 32 bits | `movt r1, #$DEAD` |
| `MOVEQ` | Move with sign-extend 32→64 | `moveq r1, #-1` |
| `LEA` | Load effective address | `lea r1, offset(r2)` |

### Load/Store

| Mnemonic | Description | Example |
|----------|-------------|---------|
| `LOAD.x` | Load from memory (size suffix) | `load.l r1, (r2)` |
| `STORE.x` | Store to memory (size suffix) | `store.l r1, (r2)` |

Size suffixes: `.b` (8-bit), `.w` (16-bit), `.l` (32-bit), `.q` (64-bit)

### Arithmetic

| Mnemonic | Description |
|----------|-------------|
| `ADD` | Add (register or immediate) |
| `SUB` | Subtract |
| `MULU` | Multiply unsigned |
| `MULS` | Multiply signed |
| `DIVU` | Divide unsigned |
| `DIVS` | Divide signed |
| `MOD` | Modulo |
| `NEG` | Negate |

### Logical and Shifts

| Mnemonic | Description |
|----------|-------------|
| `AND` | Bitwise AND |
| `OR` | Bitwise OR |
| `EOR` | Bitwise exclusive OR |
| `NOT` | Bitwise NOT |
| `LSL` | Logical shift left |
| `LSR` | Logical shift right |
| `ASR` | Arithmetic shift right |

### Branches (Compare-and-Branch)

All conditional branches compare two registers directly — no flags register:

| Mnemonic | Condition | Example |
|----------|-----------|---------|
| `BRA` | Always | `bra loop` |
| `BEQ` | Rs == Rt | `beq r1, r2, equal` |
| `BNE` | Rs != Rt | `bne r1, r0, nonzero` |
| `BLT` | Rs < Rt (signed) | `blt r1, r2, less` |
| `BGE` | Rs >= Rt (signed) | `bge r1, r2, greater_eq` |
| `BGT` | Rs > Rt (signed) | `bgt r1, r2, greater` |
| `BLE` | Rs <= Rt (signed) | `ble r1, r2, less_eq` |
| `BHI` | Rs > Rt (unsigned) | `bhi r1, r2, higher` |
| `BLS` | Rs <= Rt (unsigned) | `bls r1, r2, lower_same` |
| `JMP` | Register-indirect | `jmp (r5)` / `jmp 16(r5)` |

### Subroutine and Stack

| Mnemonic | Description |
|----------|-------------|
| `JSR` | Jump to subroutine — PC-relative (`jsr label`) or register-indirect (`jsr (r5)`) |
| `RTS` | Return from subroutine |
| `PUSH` | Push register onto stack |
| `POP` | Pop from stack to register |

### System

| Mnemonic | Description |
|----------|-------------|
| `NOP` | No operation |
| `HALT` | Halt processor |
| `SEI` | Set (enable) interrupts |
| `CLI` | Clear (disable) interrupts |
| `RTI` | Return from interrupt |
| `WAIT` | Wait for specified microseconds |

### Pseudo-Instructions

The `ie64asm` assembler provides these convenience pseudo-instructions:

| Pseudo | Expansion | Description |
|--------|-----------|-------------|
| `la Rd, addr` | `lea Rd, addr(r0)` | Load address into register |
| `li Rd, #imm32` | `move.l Rd, #imm32` | Load 32-bit immediate |
| `li Rd, #imm64` | `move.l Rd, #lo32` + `movt Rd, #hi32` | Load full 64-bit immediate |
| `beqz Rs, label` | `beq Rs, r0, label` | Branch if zero |
| `bnez Rs, label` | `bne Rs, r0, label` | Branch if not zero |
| `bltz Rs, label` | `blt Rs, r0, label` | Branch if less than zero |
| `bgez Rs, label` | `bge Rs, r0, label` | Branch if greater or equal to zero |
| `bgtz Rs, label` | `bgt Rs, r0, label` | Branch if greater than zero |
| `blez Rs, label` | `ble Rs, r0, label` | Branch if less or equal to zero |

### Common Patterns

```assembly
    include "ie64.inc"

start:
    ; Set up video
    la   r1, VIDEO_CTRL
    move.l r2, #1
    store.l r2, (r1)

    ; Loop with counter
    move.l r10, #0              ; counter
    move.l r11, #100            ; limit
.loop:
    add.l  r10, r10, #1
    blt    r10, r11, .loop      ; compare-and-branch

    ; Subroutine call
    jsr    my_func
    halt

my_func:
    push   r5
    ; ... work ...
    pop    r5
    rts
```

## 9.5 FPU Logic

The IE64 includes a dedicated hardware Floating-Point Unit (FPU) for IEEE-754 single-precision arithmetic.

### FPU Register File
- **F0–F15**: 16 dedicated 32-bit registers for floating-point bit patterns.
- **FPSR**: Status register containing overwritten condition codes (N, Z, I, NaN) and sticky exception flags (IO, DZ, OE, UE).
- **FPCR**: Control register for setting the rounding mode (Nearest, Zero, Floor, Ceil).

### Native FPU Instructions (29)
- **Arithmetic**: FADD, FSUB, FMUL, FDIV, FMOD, FABS, FNEG, FSQRT, FINT
- **Transcendentals**: FSIN, FCOS, FTAN, FATAN, FLOG, FEXP, FPOW
- **Movement/Conversion**: FMOV, FLOAD, FSTORE, FCVTIF (int→float), FCVTFI (float→int), FMOVI, FMOVO (bitwise reinterpret)
- **Status/Constants**: FMOVSR, FMOVCR, FMOVSC, FMOVCC, FMOVECR (load ROM Pi, e, etc.)

FPU instructions are strictly 32-bit; the assembler rejects size suffixes on these mnemonics.

## 9.6 Memory and I/O Integration

The IE64 shares the same 32MB system bus and memory-mapped device address space as all other Intuition Engine CPUs:

- All hardware registers at `$F0000–$FFFFF` are accessible via `LOAD`/`STORE`
- VGA VRAM at `$A0000–$AFFFF`, Video RAM at `$100000+`
- VRAM direct-write fast path for stores to Video RAM (`$100000+`, attached VRAM window)
- 64-bit bus operations (`Read64`/`Write64`) with I/O region split semantics for device registers

## 9.7 Interrupt Handling

The IE64 has an integrated timer and interrupt system:

- **Interrupt vector**: Internal `interruptVector` field (set to 0 on reset)
- **Timer**: Integrated CPU timer, decremented every 44100 cycles
- **Timer state**: Internal CPU fields (not memory-mapped timer registers)

**Interrupt flow:**
1. Timer counts down; when it reaches zero, an interrupt fires
2. If `interruptEnabled` is true and not already in an ISR, CPU sets `inInterrupt=true`
3. CPU pushes PC to stack and jumps to `interruptVector`
4. ISR executes and returns via `RTI` (restores PC, clears `inInterrupt`)

**Instructions:**
- `SEI` — Enable interrupts
- `CLI` — Disable interrupts
- `RTI` — Return from interrupt (pops PC, clears `inInterrupt`)

Note: The interrupt vector is currently set internally. Assembly-level vector programming is reserved for a future update.

## 9.8 Compatibility Notes

- Use `-ie64` flag to run IE64 binaries
- File extension: `.ie64`
- Use `ie64asm` assembler with `ie64.inc` include file
- Little-endian byte order
- Compare-and-branch model (no flags register — unlike IE32, M68K, Z80, 6502, x86)
- R0 is hardwired to zero (reads always return 0, writes are silently ignored)
- `.l` operations zero-mask to 32 bits; use `.q` for full 64-bit arithmetic
- Full ISA reference: [IE64_ISA.md](docs/IE64_ISA.md)
- Assembly cookbook: [IE64_COOKBOOK.md](docs/IE64_COOKBOOK.md)

## 9.9 EhBASIC IE64

The Intuition Engine includes a full port of Lee Davison's Enhanced BASIC (EhBASIC) for the IE64 CPU. The interpreter is a ground-up reimplementation in IE64 assembly, using IEEE 754 single-precision (FP32) arithmetic and providing direct access to all hardware subsystems from BASIC.

### Running

```bash
# Run with embedded BASIC image (requires 'make basic' build)
./bin/IntuitionEngine -basic

# Run with a custom BASIC binary
./bin/IntuitionEngine -basic-image path/to/custom.ie64
```

### Features

- **Core language**: PRINT, LET, IF/THEN/ELSE, FOR/NEXT/STEP, WHILE/WEND, DO/LOOP, GOTO, GOSUB/RETURN, DATA/READ, INPUT, DIM (multi-dimensional arrays), string variables and operations
- **Built-in functions**: ABS, INT, SQR, RND, SGN, SIN, COS, TAN, ATN, LOG, EXP, LEN, ASC, VAL, CHR$, LEFT$, RIGHT$, MID$, STR$, HEX$, BIN$, PEEK, POKE, USR, MAX, MIN, BITTST, FRE
- **Video commands**: SCREEN, CLS, PLOT, LINE, CIRCLE, BOX, PALETTE, LOCATE, COLOR, SCROLL, VSYNC, plus ULA, TED, ANTIC/GTIA, and full Voodoo 3D pipeline (vertices, triangles, textures, Z-buffer, alpha blending, fog)
- **Audio commands**: SOUND, ENVELOPE, GATE, WAVE, FILTER, REVERB, OVERDRIVE, SWEEP, SYNC, RINGMOD, plus PSG/SID/POKEY/TED/AHX playback and STATUS queries
- **System commands**: CALL (machine code subroutine), USR (call with return value), POKE8/PEEK8, DOKE/DEEK, WAIT, BLIT, COPPER, TRON/TROFF (trace mode)
- **Coprocessor commands**: COSTART, COSTOP, COWAIT (worker lifecycle); COCALL(), COSTATUS() (async cross-CPU RPC to IE32/6502/M68K/Z80/x86 workers)
- **Machine code interface**: CALL and USR use register-indirect JSR to invoke IE64 assembly routines; R8 carries return values
- **Terminal editor**: Insert mode with character shifting, key repeat, Ctrl shortcuts (A/E/K/U/L), Ctrl+Arrow word movement, command history (Ctrl+Up/Down), Page Up/Down and mouse wheel scrollback navigation

### Common REPL Commands

- `SOUND PLAY "music.ext" [,subsong]` starts music playback and auto-detects format by extension: `.sid`, `.ym`, `.ay`, `.sndh`, `.ted`, `.prg`, `.ahx` (stops any currently playing track first)
- `SOUND STOP` stops current music playback (`SOUND PLAY STOP` is also accepted)
- `RUN` executes the current BASIC program; `RUN "program.ie64"` (or `.iex`/`.ie68`/`.ie86`/`.ie80`/`.bin`) loads and launches an external binary

### Example

```basic
10 SCREEN &H13
20 FOR I = 0 TO 199
30   PLOT 160, I, I AND 255
40 NEXT
50 VSYNC
```

Full reference: [ehbasic_ie64.md](docs/ehbasic_ie64.md)

# 10. Assembly Language Reference

This section documents the IE32 assembly language used with the `ie32asm` assembler. For IE64, 6502, Z80, M68K, and x86 programming, use their respective assemblers (`ie64asm`, ca65, vasmz80_std, vasmm68k_mot, NASM/FASM) with the include files documented in Section 13.4.

The Intuition Engine assembly language provides a straightforward way to program the system while maintaining access to all hardware features.

## 10.1 Basic Program Structure

Every assembly program follows this basic structure:

```assembly
; Program header with description
; Example: Simple counter program
.equ TIMER_CTRL, 0xF0800    ; Define hardware constants
.equ TIMER_PERIOD, 0xF0808  ; using symbolic names

start:                     ; Main entry point
    LOAD A, #0             ; Initialise counter
    JSR setup_timer        ; Call timer setup
main_loop:
    JSR check_timer        ; Check timer status
    JMP main_loop          ; Continue main loop

; Subroutines follow main program
setup_timer:
    LOAD A, #44100         ; ~1Hz timer period
    STORE A, @TIMER_PERIOD
    LOAD A, #1
    STORE A, @TIMER_CTRL
    RTS
```

## 10.2 Assembler Directives

The assembler supports these directives:

```assembly
.equ SYMBOL, VALUE   ; Define a constant
.word VALUE          ; Define a 32-bit word
.byte VALUE          ; Define an 8-bit byte
.space SIZE          ; Reserve bytes of space
.org ADDRESS         ; Set assembly address
```

The .org directive provides control over code placement:

```assembly
; Example memory organisation
.org 0x0000               ; Start at vector table
    .word isr_handler     ; Set up interrupt vector

.org 0x1000               ; Place main program
start:
    JSR init
    SEI
    JMP main
```

## 10.3 Memory Access Patterns

When working with memory, consider alignment and efficiency:

```assembly
; Efficient memory copy
copy_memory:
    LOAD X, #0           ; Source index
    LOAD Y, #0           ; Destination index
    LOAD Z, #100         ; Word count
copy_loop:
    LOAD A, [X]          ; Load from source
    STORE A, [Y]         ; Store to destination
    ADD X, #4            ; Next word (32-bit aligned)
    ADD Y, #4
    SUB Z, #1
    JNZ Z, copy_loop
    RTS
```

## 10.4 Stack Usage

The stack is essential for subroutines and temporary storage:

```assembly
calculate:
    PUSH A              ; Save registers
    PUSH X

    ; Perform calculation
    LOAD X, #0
    ADD X, A

    POP X               ; Restore registers
    POP A               ; in reverse order
    RTS
```

## 10.5 Interrupt Handlers

Interrupt handlers must preserve register state:

```assembly
isr_handler:
    PUSH A              ; Save registers
    PUSH X

    LOAD A, @TIMER_COUNT
    JSR process_timer  ; Handle timer event

    POP X              ; Restore registers
    POP A
    RTI                ; Return from interrupt
```

# 11. Sound System

The Intuition Engine provides a powerful custom audio synthesizer alongside three emulated classic sound chips. The custom audio chip offers modern synthesis capabilities while maintaining the retro aesthetic.

## Custom Audio Chip Overview

The custom audio chip is a 4-channel synthesizer with advanced features:

- **5 Dedicated Waveform Channels**: Square, Triangle, Sine, Noise, Sawtooth
- **4 Flexible Synth Channels**: Any waveform type per channel
- **Per-Voice ADSR Envelopes**: 16-bit attack/decay/release times, 8-bit sustain level
- **Pulse Width Modulation**: Variable duty cycle with automatic LFO
- **Frequency Sweep**: Portamento and pitch bend effects
- **Hard Sync**: Slave oscillator phase reset by master
- **Ring Modulation**: Amplitude modulation between channels
- **Global Resonant Filter**: Low-pass, high-pass, band-pass with resonance
- **Effects**: Overdrive distortion and reverb

### Signal Flow

```
Oscillators → Envelopes → Mix → Filter → Overdrive → Reverb → Output
```

Sample rate: 44.1kHz with 32-bit floating-point internal processing

### Memory Map

| Register Block | IE32/IE64/M68K Address | Z80/6502 Address | Description |
|----------------|-------------------|------------------|-------------|
| Global Control | `$F0800-$F0807` | `$F800-$F807` | Master audio control, envelope shape |
| Filter | `$F0820-$F0833` | `$F820-$F833` | Cutoff, resonance, type, modulation |
| Square Channel | `$F0900-$F093F` | `$F900-$F93F` | Frequency, volume, ADSR, duty, PWM, sweep |
| Triangle Channel | `$F0940-$F097F` | `$F940-$F97F` | Frequency, volume, ADSR, sweep |
| Sine Channel | `$F0980-$F09BF` | `$F980-$F9BF` | Frequency, volume, ADSR, sweep |
| Noise Channel | `$F09C0-$F09FF` | `$F9C0-$F9FF` | Frequency, volume, ADSR, noise mode |
| Sync Sources | `$F0A00-$F0A0F` | `$FA00-$FA0F` | Hard sync source per channel |
| Ring Mod Sources | `$F0A10-$F0A1F` | `$FA10-$FA1F` | Ring modulation source per channel |
| Sawtooth Channel | `$F0A20-$F0A5F` | `$FA20-$FA5F` | Frequency, volume, ADSR, sweep |
| Overdrive | `$F0A40-$F0A43` | `$FA40-$FA43` | Drive amount (0-255) |
| Reverb | `$F0A50-$F0A57` | `$FA50-$FA57` | Mix level and decay time |
| Flex Channel 0 | `$F0A80-$F0ABF` | `$FA80-$FABF` | Configurable waveform channel |
| Flex Channel 1 | `$F0AC0-$F0AFF` | `$FAC0-$FAFF` | Configurable waveform channel |
| Flex Channel 2 | `$F0B00-$F0B3F` | `$FB00-$FB3F` | Configurable waveform channel |
| Flex Channel 3 | `$F0B40-$F0B7F` | `$FB40-$FB7F` | Configurable waveform channel |

### Register Reference

#### Global Registers

| Offset | IE32 Address | Name | Description |
|--------|--------------|------|-------------|
| +$00 | `$F0800` | AUDIO_CTRL | Master audio control |
| +$04 | `$F0804` | ENV_SHAPE | Global envelope shape (0=ADSR, 1=Saw Up, 2=Saw Down, 3=Loop, 4=SID-style) |

#### Filter Registers

| Offset | IE32 Address | Name | Range | Description |
|--------|--------------|------|-------|-------------|
| +$00 | `$F0820` | FILTER_CUTOFF | 0-65535 | Cutoff frequency (exponential 20Hz-20kHz) |
| +$04 | `$F0824` | FILTER_RESONANCE | 0-255 | Resonance/Q factor |
| +$08 | `$F0828` | FILTER_TYPE | 0-3 | 0=Off, 1=Low-pass, 2=High-pass, 3=Band-pass |
| +$0C | `$F082C` | FILTER_MOD_SOURCE | 0-3 | Modulation source channel |
| +$10 | `$F0830` | FILTER_MOD_AMOUNT | 0-255 | Modulation depth |

#### Dedicated Channel Registers (Square/Triangle/Sine/Noise/Sawtooth)

Each dedicated channel has a similar register layout:

| Offset | Name | Description |
|--------|------|-------------|
| +$00 | FREQ | Frequency (16.8 fixed-point Hz, value = Hz * 256) |
| +$04 | VOL | Volume 0-255 (32-bit, only low byte used) |
| +$08 | CTRL | Control bits (see below) |
| +$0C | ATTACK | Attack time in ms (16-bit) |
| +$10 | DECAY | Decay time in ms (16-bit) |
| +$14 | SUSTAIN | Sustain level 0-255 (16-bit) |
| +$18 | RELEASE | Release time in ms (16-bit) |
| +$1C | DUTY | Duty cycle 0-65535 (square only, 32768=50%) |
| +$20 | PWM_RATE | PWM oscillation rate (square only) |
| +$24 | PWM_DEPTH | PWM depth 0-65535 (square only) |
| +$28 | SWEEP_RATE | Frequency sweep rate |
| +$2C | SWEEP_DIR | Sweep direction (0=down, 1=up) |
| +$30 | SWEEP_AMT | Sweep amount per step |
| +$34 | TARGET | Target frequency for sweep |

**Noise channel additional register:**
| Offset | Name | Values | Description |
|--------|------|--------|-------------|
| +$1C | NOISE_MODE | 0-3 | 0=White, 1=Periodic, 2=Metallic, 3=PSG-style |

#### Control Register Bits

| Bit | Name | Description |
|-----|------|-------------|
| 0 | GATE | Trigger envelope (1=attack, 0=release) |
| 1 | PWM_EN | Enable pulse width modulation |
| 2 | SWEEP_EN | Enable frequency sweep |
| 3 | SYNC_EN | Enable hard sync to source channel |
| 4 | RING_EN | Enable ring modulation from source |
| 5 | FILTER_EN | Route channel through global filter |

#### Flexible Channel Registers

Each flexible channel is 48 bytes ($30) with full synthesis control:

| Offset | Name | Description |
|--------|------|-------------|
| +$00 | FREQ | Frequency (16.8 fixed-point Hz, value = Hz * 256) |
| +$04 | VOL | Volume 0-255 (32-bit) |
| +$08 | WAVE | Waveform type (see below) |
| +$0C | CTRL | Control bits (same as dedicated channels) |
| +$10 | ATTACK | Attack time in ms (16-bit) |
| +$14 | DECAY | Decay time in ms (16-bit) |
| +$18 | SUSTAIN | Sustain level 0-255 (16-bit) |
| +$1C | RELEASE | Release time in ms (16-bit) |
| +$20 | DUTY | Duty cycle for square wave |
| +$24 | PAN | Stereo pan (-128 to 127, signed) |

**Waveform Types:**
| Value | Name | Description |
|-------|------|-------------|
| 0 | WAVE_SQUARE | Square/pulse wave with PWM support |
| 1 | WAVE_TRIANGLE | Triangle wave |
| 2 | WAVE_SINE | Pure sine wave |
| 3 | WAVE_NOISE | Noise generator |
| 4 | WAVE_SAWTOOTH | Sawtooth wave |

## 11.1 Sound Channel Types

Each channel offers different synthesis capabilities:

### Square Wave Channel

Features:
- Variable duty cycle control
- PWM modulation
- Frequency sweep
- Ring modulation support
- ADSR envelope

### Configuration Example:

**IE32:**
```assembly
setup_square:
    LOAD A, #112640         ; 440 Hz (16.8 fixed-point: 440*256)
    STORE A, @SQUARE_FREQ
    LOAD A, #128            ; 50% duty cycle
    STORE A, @SQUARE_DUTY
    LOAD A, #1              ; Enable PWM
    STORE A, @SQUARE_PWM_CTRL
    LOAD A, #10             ; 10ms attack
    STORE A, @SQUARE_ATK
    LOAD A, #20             ; 20ms decay
    STORE A, @SQUARE_DEC
    LOAD A, #192            ; 75% sustain
    STORE A, @SQUARE_SUS
    LOAD A, #100            ; 100ms release
    STORE A, @SQUARE_REL
    RTS
```

**M68K:**
```assembly
setup_square:
    move.l  #112640,SQUARE_FREQ.l    ; 440 Hz (16.8 fixed-point: 440*256)
    move.l  #128,SQUARE_DUTY.l       ; 50% duty cycle
    move.l  #1,SQUARE_PWM_CTRL.l     ; Enable PWM
    move.l  #10,SQUARE_ATK.l         ; Attack
    move.l  #20,SQUARE_DEC.l         ; Decay
    move.l  #192,SQUARE_SUS.l        ; Sustain
    move.l  #100,SQUARE_REL.l        ; Release
    rts
```

**Z80:**
```assembly
setup_square:
    STORE32 SQUARE_FREQ,112640       ; 440 Hz (16.8 fixed-point: 440*256)
    STORE32 SQUARE_DUTY,128          ; 50% duty cycle
    STORE32 SQUARE_PWM_CTRL,1        ; Enable PWM
    STORE32 SQUARE_ATK,10            ; Attack
    STORE32 SQUARE_DEC,20            ; Decay
    STORE32 SQUARE_SUS,192           ; Sustain
    STORE32 SQUARE_REL,100           ; Release
    ret
```

**6502:**
```assembly
setup_square:
    STORE32 SQUARE_FREQ, 112640      ; 440 Hz (16.8 fixed-point: 440*256)
    STORE32 SQUARE_DUTY, 128         ; 50% duty cycle
    STORE32 SQUARE_PWM_CTRL, 1       ; Enable PWM
    STORE32 SQUARE_ATK, 10           ; Attack
    STORE32 SQUARE_DEC, 20           ; Decay
    STORE32 SQUARE_SUS, 192          ; Sustain
    STORE32 SQUARE_REL, 100          ; Release
    rts
```

### Triangle Wave Channel

Features:
- Pure harmonic content
- Frequency sweep
- Ring modulation support
- ADSR envelope

### Sine Wave Channel

Features:
- Clean tonal output
- Frequency sweep
- Ring modulation support
- ADSR envelope

### Noise Channel

Features:
- Three noise types:
    - White noise (LFSR-based)
    - Periodic noise
    - Metallic noise
- Frequency sweep
- ADSR envelope

### Sawtooth Wave Channel

Features:
- Classic sawtooth waveform (ramps from -1 to +1)
- polyBLEP anti-aliasing for cleaner high-frequency output
- Frequency sweep
- Ring modulation support
- ADSR envelope

### Configuration Example:

**IE32:**
```assembly
setup_sawtooth:
    LOAD A, #112640         ; 440 Hz (16.8 fixed-point: 440*256)
    STORE A, @SAW_FREQ
    LOAD A, #192            ; 75% volume
    STORE A, @SAW_VOL
    LOAD A, #10             ; Attack
    STORE A, @SAW_ATK
    LOAD A, #20             ; Decay
    STORE A, @SAW_DEC
    LOAD A, #192            ; Sustain
    STORE A, @SAW_SUS
    LOAD A, #100            ; Release
    STORE A, @SAW_REL
    LOAD A, #1              ; Enable
    STORE A, @SAW_CTRL
    RTS
```

**M68K:**
```assembly
setup_sawtooth:
    move.l  #112640,SAW_FREQ.l        ; 440 Hz (16.8 fixed-point: 440*256)
    move.l  #192,SAW_VOL.l
    move.l  #10,SAW_ATK.l
    move.l  #20,SAW_DEC.l
    move.l  #192,SAW_SUS.l
    move.l  #100,SAW_REL.l
    move.l  #1,SAW_CTRL.l
    rts
```

**Z80:**
```assembly
setup_sawtooth:
    STORE32 SAW_FREQ,112640           ; 440 Hz (16.8 fixed-point: 440*256)
    STORE32 SAW_VOL,192
    STORE32 SAW_ATK,10
    STORE32 SAW_DEC,20
    STORE32 SAW_SUS,192
    STORE32 SAW_REL,100
    STORE32 SAW_CTRL,1
    ret
```

**6502:**
```assembly
setup_sawtooth:
    STORE32 SAW_FREQ, 112640          ; 440 Hz (16.8 fixed-point: 440*256)
    STORE32 SAW_VOL, 192
    STORE32 SAW_ATK, 10
    STORE32 SAW_DEC, 20
    STORE32 SAW_SUS, 192
    STORE32 SAW_REL, 100
    STORE32 SAW_CTRL, 1
    rts
```

## 11.2 Modulation System

The sound system supports complex inter-channel modulation for creating rich, evolving timbres.

### Hard Sync

Hard sync forces a slave oscillator to reset its phase whenever the master oscillator completes a cycle. This creates complex harmonic content that changes with the frequency ratio between oscillators.

**Sync Source Registers:**
| Register | IE32 Address | Description |
|----------|--------------|-------------|
| SYNC_SOURCE_CH0 | `$F0A00` | Sync source for channel 0 |
| SYNC_SOURCE_CH1 | `$F0A04` | Sync source for channel 1 |
| SYNC_SOURCE_CH2 | `$F0A08` | Sync source for channel 2 |
| SYNC_SOURCE_CH3 | `$F0A0C` | Sync source for channel 3 |

Set source to channel number (0-3) or `$FF` to disable.

**IE32:**
```assembly
; Sync channel 0 to channel 1 (ch1 is master)
    LOAD A, #1
    STORE A, @SYNC_SOURCE_CH0
    LOAD A, #CTRL_GATE | CTRL_SYNC_EN   ; Enable sync in control
    STORE A, @SQUARE_CTRL
```

**M68K:**
```assembly
    move.l  #1,SYNC_SOURCE_CH0.l
    move.l  #CTRL_GATE|CTRL_SYNC_EN,SQUARE_CTRL.l
```

### Ring Modulation

Ring modulation multiplies two signals together, producing sum and difference frequencies (sidebands). This creates metallic, bell-like tones.

**Ring Mod Source Registers:**
| Register | IE32 Address | Description |
|----------|--------------|-------------|
| RING_MOD_SOURCE_CH0 | `$F0A10` | Ring mod source for channel 0 |
| RING_MOD_SOURCE_CH1 | `$F0A14` | Ring mod source for channel 1 |
| RING_MOD_SOURCE_CH2 | `$F0A18` | Ring mod source for channel 2 |
| RING_MOD_SOURCE_CH3 | `$F0A1C` | Ring mod source for channel 3 |

**IE32:**
```assembly
; Ring modulate channel 0 with channel 1
    LOAD A, #1
    STORE A, @RING_MOD_SOURCE_CH0
    LOAD A, #CTRL_GATE | CTRL_RING_EN
    STORE A, @SQUARE_CTRL
```

**M68K:**
```assembly
    move.l  #1,RING_MOD_SOURCE_CH0.l
    move.l  #CTRL_GATE|CTRL_RING_EN,SQUARE_CTRL.l
```

### Frequency Sweep

Automatic frequency changes for pitch bend, portamento, and laser effects.

**Sweep Registers (per channel):**
| Offset | Name | Description |
|--------|------|-------------|
| SWEEP_RATE | Rate of frequency change |
| SWEEP_DIR | Direction: 0=down, 1=up |
| SWEEP_AMT | Amount per step |
| TARGET | Target frequency (sweep stops here) |

**IE32:**
```assembly
; Sweep square channel up from 220Hz to 880Hz
    LOAD A, #220
    STORE A, @SQUARE_FREQ        ; Start frequency
    LOAD A, #880
    STORE A, @SQUARE_TARGET      ; End frequency
    LOAD A, #1
    STORE A, @SQUARE_SWEEP_DIR   ; Sweep up
    LOAD A, #10
    STORE A, @SQUARE_SWEEP_RATE  ; Sweep speed
    LOAD A, #CTRL_GATE | CTRL_SWEEP_EN
    STORE A, @SQUARE_CTRL        ; Enable sweep
```

### Pulse Width Modulation (PWM)

For square wave channels, PWM automatically varies the duty cycle for a rich, animated sound.

**IE32:**
```assembly
; Enable PWM on square channel
    LOAD A, #32768              ; 50% base duty cycle
    STORE A, @SQUARE_DUTY
    LOAD A, #512                ; PWM rate (Hz * 256)
    STORE A, @SQUARE_PWM_RATE
    LOAD A, #16384              ; PWM depth
    STORE A, @SQUARE_PWM_DEPTH
    LOAD A, #CTRL_GATE | CTRL_PWM_EN
    STORE A, @SQUARE_CTRL
```

## 11.3 Global Effects

The system provides global audio processing applied after channel mixing.

### Filter System

A resonant state-variable filter with three modes:

| Type Value | Mode | Description |
|------------|------|-------------|
| 0 | Off | Filter bypassed |
| 1 | Low-pass | Attenuates frequencies above cutoff |
| 2 | High-pass | Attenuates frequencies below cutoff |
| 3 | Band-pass | Passes frequencies around cutoff |

**Filter Registers:**
| Register | IE32 Address | Range | Description |
|----------|--------------|-------|-------------|
| FILTER_CUTOFF | `$F0820` | 0-65535 | Cutoff frequency (exponential mapping) |
| FILTER_RESONANCE | `$F0824` | 0-255 | Resonance/Q (higher = more emphasis) |
| FILTER_TYPE | `$F0828` | 0-3 | Filter mode |
| FILTER_MOD_SOURCE | `$F082C` | 0-3 | Channel to modulate cutoff |
| FILTER_MOD_AMOUNT | `$F0830` | 0-255 | Modulation depth |

The cutoff uses exponential mapping for musical control (human hearing is logarithmic). Value 0 maps to 20Hz, 65535 maps to 20kHz.

To route a channel through the filter, set bit 5 (CTRL_FILTER_EN) in its control register.

**IE32:**
```assembly
    LOAD A, #32768          ; Cutoff (~1kHz)
    STORE A, @FILTER_CUTOFF
    LOAD A, #128            ; Medium resonance
    STORE A, @FILTER_RESONANCE
    LOAD A, #1              ; Low-pass mode
    STORE A, @FILTER_TYPE
    ; Route square channel through filter
    LOAD A, #CTRL_GATE | CTRL_FILTER_EN
    STORE A, @SQUARE_CTRL
```

**M68K:**
```assembly
    move.l  #32768,FILTER_CUTOFF.l
    move.l  #128,FILTER_RESONANCE.l
    move.l  #1,FILTER_TYPE.l
    move.l  #CTRL_GATE|CTRL_FILTER_EN,SQUARE_CTRL.l
```

**Z80:**
```assembly
    SET_FILTER FILT_LOWPASS,32768,128
    ; Enable filter on square channel
    FILTER_SQ_ON
```

**6502:**
```assembly
    SET_FILTER FILT_LOWPASS, 32768, 128
    FILTER_SQ_ON
```

### Overdrive

Soft-clipping distortion for adding grit and harmonics to the output.

| Register | IE32 Address | Range | Description |
|----------|--------------|-------|-------------|
| OVERDRIVE_CTRL | `$F0A40` | 0-255 | 0=off, 1-255=distortion amount |

Higher values produce more aggressive clipping. Values around 32-64 add subtle warmth, while 128+ creates heavy distortion.

**IE32:**
```assembly
    LOAD A, #64             ; Moderate overdrive
    STORE A, @OVERDRIVE_CTRL
```

**M68K:**
```assembly
    move.l  #64,OVERDRIVE_CTRL.l
```

**Z80:**
```assembly
    SET_OVERDRIVE 64
```

**6502:**
```assembly
    SET_OVERDRIVE 64
```

### Reverb System

Stereo reverb with adjustable mix and decay time.

| Register | IE32 Address | Range | Description |
|----------|--------------|-------|-------------|
| REVERB_MIX | `$F0A50` | 0-255 | Dry/wet mix (0=dry, 255=wet) |
| REVERB_DECAY | `$F0A54` | 0-65535 | Decay time in ms |

**IE32:**
```assembly
    LOAD A, #128            ; 50% wet/dry mix
    STORE A, @REVERB_MIX
    LOAD A, #2000           ; 2 second decay
    STORE A, @REVERB_DECAY
```

**M68K:**
```assembly
    move.l  #128,REVERB_MIX.l
    move.l  #2000,REVERB_DECAY.l
```

**Z80:**
```assembly
    SET_REVERB 128,2000
```

**6502:**
```assembly
    SET_REVERB 128, 2000
```

## 11.4 PSG Sound Chip (AY-3-8910/YM2149)

The PSG chip emulates the General Instrument AY-3-8910 and Yamaha YM2149, providing three channels of square wave synthesis with noise and envelope capabilities. This chip powered the sound in countless 8-bit computers including the ZX Spectrum 128, Amstrad CPC, Atari ST, and MSX.

### Features:
- Three independent square wave tone generators
- One noise generator (shared across channels)
- Hardware envelope generator with 8 shape patterns
- Per-channel mixer control (tone/noise enable)
- 4-bit volume control per channel (or envelope)
- PSG+ enhanced audio processing mode
- Support for .YM, .AY, .VGM, .VGZ, and .SNDH file playback

### Tone Generation:
Each channel has a 12-bit frequency divider:
- Frequency = Clock / (16 × TP) where TP is the tone period (1-4095)
- Channel A: Registers 0-1 (fine/coarse tune)
- Channel B: Registers 2-3 (fine/coarse tune)
- Channel C: Registers 4-5 (fine/coarse tune)

### Noise Generator:
- 5-bit noise period (Register 6)
- Pseudo-random output from 17-bit LFSR
- Can be mixed with any tone channel

### Mixer Control (Register 7):
```
Bit 0: Channel A tone enable (0=on, 1=off)
Bit 1: Channel B tone enable
Bit 2: Channel C tone enable
Bit 3: Channel A noise enable (0=on, 1=off)
Bit 4: Channel B noise enable
Bit 5: Channel C noise enable
Bits 6-7: I/O port direction (directly mapped only)
```

### Volume and Envelope:
- Registers 8-10: Channel A/B/C amplitude (0-15, or bit 4 set for envelope)
- Registers 11-12: Envelope period (16-bit)
- Register 13: Envelope shape (8 patterns)

### Envelope Shapes:
| Value | Shape | Description |
|-------|-------|-------------|
| 0-3   | ╲____ | Decay to zero, hold |
| 4-7   | /‾‾‾‾ | Attack to max, hold |
| 8     | ╲╲╲╲╲ | Repeating decay (sawtooth down) |
| 9     | ╲____ | Decay to zero, hold |
| 10    | ╲/╲/╲ | Repeating decay-attack (triangle) |
| 11    | ╲‾‾‾‾ | Decay to zero, then hold max |
| 12    | ///// | Repeating attack (sawtooth up) |
| 13    | /‾‾‾‾ | Attack to max, hold |
| 14    | /╲/╲/ | Repeating attack-decay (triangle) |
| 15    | /_____ | Attack to max, then hold zero |

### Configuration Example:

Configure PSG channel A for a 440Hz tone with envelope:

**IE32:**
```assembly
; Configure PSG channel A for a 440Hz tone with envelope
LOAD A, #0xFE          ; Tone period low byte (440Hz approx)
STORE A, @0x0F0C00     ; Register 0: Channel A fine tune
LOAD A, #0x00          ; Tone period high byte
STORE A, @0x0F0C01     ; Register 1: Channel A coarse tune
LOAD A, #0x3E          ; Enable tone A, disable noise
STORE A, @0x0F0C07     ; Register 7: Mixer
LOAD A, #0x10          ; Use envelope for volume
STORE A, @0x0F0C08     ; Register 8: Channel A amplitude
LOAD A, #0x00          ; Envelope period low
STORE A, @0x0F0C0B     ; Register 11: Envelope fine
LOAD A, #0x10          ; Envelope period high
STORE A, @0x0F0C0C     ; Register 12: Envelope coarse
LOAD A, #0x0E          ; Triangle envelope shape
STORE A, @0x0F0C0D     ; Register 13: Envelope shape
```

**M68K:**
```assembly
; Configure PSG channel A for a 440Hz tone with envelope
    move.b  #$FE,$F0C00.l       ; Register 0: Channel A fine tune
    move.b  #$00,$F0C01.l       ; Register 1: Channel A coarse tune
    move.b  #$3E,$F0C07.l       ; Register 7: Mixer (tone A on)
    move.b  #$10,$F0C08.l       ; Register 8: Envelope mode
    move.b  #$00,$F0C0B.l       ; Register 11: Envelope fine
    move.b  #$10,$F0C0C.l       ; Register 12: Envelope coarse
    move.b  #$0E,$F0C0D.l       ; Register 13: Triangle shape
```

**Z80:**
```assembly
; Configure PSG channel A for a 440Hz tone with envelope
; Z80 uses port I/O: port $F0 = register select, port $F1 = data
    ld   a,0               ; Select register 0 (fine tune)
    out  ($F0),a
    ld   a,$FE             ; Tone period low byte
    out  ($F1),a
    ld   a,1               ; Select register 1 (coarse tune)
    out  ($F0),a
    ld   a,$00             ; Tone period high byte
    out  ($F1),a
    ld   a,7               ; Select register 7 (mixer)
    out  ($F0),a
    ld   a,$3E             ; Enable tone A
    out  ($F1),a
    ld   a,8               ; Select register 8 (amplitude)
    out  ($F0),a
    ld   a,$10             ; Envelope mode
    out  ($F1),a
    ld   a,11              ; Select register 11 (envelope fine)
    out  ($F0),a
    ld   a,$00
    out  ($F1),a
    ld   a,12              ; Select register 12 (envelope coarse)
    out  ($F0),a
    ld   a,$10
    out  ($F1),a
    ld   a,13              ; Select register 13 (shape)
    out  ($F0),a
    ld   a,$0E             ; Triangle shape
    out  ($F1),a
```

**6502:**
```assembly
; Configure PSG channel A for a 440Hz tone with envelope
; 6502 uses memory-mapped I/O at $D400-$D40D
    lda  #$FE
    sta  $D400             ; Register 0: Channel A fine tune
    lda  #$00
    sta  $D401             ; Register 1: Channel A coarse tune
    lda  #$3E
    sta  $D407             ; Register 7: Mixer (tone A on)
    lda  #$10
    sta  $D408             ; Register 8: Envelope mode
    lda  #$00
    sta  $D40B             ; Register 11: Envelope fine
    lda  #$10
    sta  $D40C             ; Register 12: Envelope coarse
    lda  #$0E
    sta  $D40D             ; Register 13: Triangle shape
```

### File Playback

The PSG player supports multiple music file formats with automatic detection:
- **.ym** - YM2149 register dump frames (50Hz playback)
- **.ay** - ZX Spectrum format with embedded Z80 code
- **.sndh** - Atari ST format with embedded M68K code
- **.vgm** - Video Game Music format with timed PSG events

To play a file, embed the data in your program and set the player registers:

**IE32:**
```assembly
; Play a .ym file with looping
    LOAD A, #1
    STORE A, @PSG_PLUS_CTRL      ; Enable PSG+ enhanced audio
    LOAD A, #music_data          ; Address of embedded music
    STORE A, @PSG_PLAY_PTR
    LOAD A, #music_data_end - music_data
    STORE A, @PSG_PLAY_LEN
    LOAD A, #5                   ; bit0=start, bit2=loop
    STORE A, @PSG_PLAY_CTRL

; Embedded music data
music_data:
    .incbin "music.ym"
music_data_end:
```

**M68K:**
```assembly
; Play a .ym file with looping
    move.b  #1,PSG_PLUS_CTRL.l   ; Enable PSG+ enhanced audio
    lea     music_data,a0
    move.l  a0,PSG_PLAY_PTR.l
    move.l  #music_data_end-music_data,PSG_PLAY_LEN.l
    move.l  #5,PSG_PLAY_CTRL.l   ; bit0=start, bit2=loop

music_data:
    incbin  "music.ym"
music_data_end:
```

**Z80:**
```assembly
; Play a .ym file with looping
    ld   a,1
    ld   (PSG_PLUS_CTRL),a       ; Enable PSG+ enhanced audio
    SET_PSG_PTR music_data
    SET_PSG_LEN (music_data_end-music_data)
    ld   a,5                     ; bit0=start, bit2=loop
    ld   (PSG_PLAY_CTRL),a

music_data:
    incbin  "music.ym"
music_data_end:
```

**6502:**
```assembly
; Play a .ym file with looping
    lda  #1
    sta  PSG_PLUS_CTRL           ; Enable PSG+ enhanced audio
    STORE32 PSG_PLAY_PTR_0, music_data
    STORE32 PSG_PLAY_LEN_0, (music_data_end-music_data)
    lda  #5                      ; bit0=start, bit2=loop
    sta  PSG_PLAY_CTRL

music_data:
    .incbin "music.ym"
music_data_end:
```

**Playback Control:**
- Write `1` to PSG_PLAY_CTRL to start playback
- Write `2` to PSG_PLAY_CTRL to stop playback
- Write `5` to PSG_PLAY_CTRL to start with looping (bit0 + bit2)
- Read PSG_PLAY_STATUS bit 0 to check if playing (1=busy, 0=stopped)
- Read PSG_PLAY_STATUS bit 1 to check for errors

## 11.5 POKEY Sound Chip

The POKEY chip emulates the Atari 8-bit computer's sound hardware, providing four channels of distinctive 8-bit audio with polynomial-based distortion.

### Features:
- Four independent frequency channels
- Multiple distortion modes using polynomial counters (4-bit, 5-bit, 9-bit, 17-bit)
- 16-bit channel linking for extended frequency range
- High-pass filter clocking between channels
- Volume-only mode for sample playback
- POKEY+ enhanced audio processing mode

### Distortion Modes:
The POKEY's signature sound comes from its polynomial-based distortion:
- **Pure Tone (0xA0)**: Clean square wave
- **Poly5 (0x20)**: 5-bit polynomial for buzzy tones
- **Poly4 (0xC0)**: 4-bit polynomial for harsh buzzy sounds
- **Poly17/Poly5 (0x00)**: Combined for complex timbres
- **Poly17 (0x80)**: White noise

### 16-bit Mode:
For higher frequency resolution, channels can be linked:
- Ch1+Ch2 linked via AUDCTL bit 4
- Ch3+Ch4 linked via AUDCTL bit 3

### Configuration Example:

Configure POKEY for pure tone on channel 1:

**IE32:**
```assembly
; Configure POKEY for pure tone on channel 1
LOAD A, #0x50          ; Frequency divider
STORE A, @0xF0D00      ; AUDF1
LOAD A, #0xAF          ; Pure tone + volume 15
STORE A, @0xF0D01      ; AUDC1
```

**M68K:**
```assembly
; Configure POKEY for pure tone on channel 1
    move.b  #$50,$F0D00.l      ; AUDF1: Frequency divider
    move.b  #$AF,$F0D01.l      ; AUDC1: Pure tone + volume 15
```

**Z80:**
```assembly
; Configure POKEY for pure tone on channel 1
; Z80 uses port I/O: port $D0 = register select, port $D1 = data
    ld   a,0               ; Select AUDF1
    out  ($D0),a
    ld   a,$50             ; Frequency divider
    out  ($D1),a
    ld   a,1               ; Select AUDC1
    out  ($D0),a
    ld   a,$AF             ; Pure tone + volume 15
    out  ($D1),a
```

**6502:**
```assembly
; Configure POKEY for pure tone on channel 1
; 6502 uses memory-mapped I/O at $D200-$D209
    lda  #$50
    sta  $D200             ; AUDF1: Frequency divider
    lda  #$AF
    sta  $D201             ; AUDC1: Pure tone + volume 15
```

### SAP File Playback

The POKEY player supports Atari 8-bit music files (.sap) which contain embedded 6502 code that drives the POKEY chip. SAP TYPE B files are supported where INIT is called once and PLAYER is called each frame.

**IE32:**
```assembly
; Play a .sap file with looping
    LOAD A, #1
    STORE A, @POKEY_PLUS_CTRL    ; Enable POKEY+ enhanced audio
    LOAD A, #sap_data            ; Address of embedded SAP file
    STORE A, @SAP_PLAY_PTR
    LOAD A, #sap_data_end - sap_data
    STORE A, @SAP_PLAY_LEN
    LOAD A, #0
    STORE A, @SAP_SUBSONG        ; Select subsong 0
    LOAD A, #5                   ; bit0=start, bit2=loop
    STORE A, @SAP_PLAY_CTRL

sap_data:
    .incbin "music.sap"
sap_data_end:
```

**M68K:**
```assembly
; Play a .sap file with looping
    move.b  #1,POKEY_PLUS_CTRL.l
    lea     sap_data,a0
    move.l  a0,SAP_PLAY_PTR.l
    move.l  #sap_data_end-sap_data,SAP_PLAY_LEN.l
    move.b  #0,SAP_SUBSONG.l     ; Subsong 0
    move.l  #5,SAP_PLAY_CTRL.l   ; bit0=start, bit2=loop

sap_data:
    incbin  "music.sap"
sap_data_end:
```

**Z80:**
```assembly
; Play a .sap file with looping
    ld   a,1
    ld   (POKEY_PLUS_CTRL),a
    SET_SAP_PTR sap_data
    SET_SAP_LEN (sap_data_end-sap_data)
    xor  a
    ld   (SAP_SUBSONG),a         ; Subsong 0
    ld   a,5                     ; bit0=start, bit2=loop
    ld   (SAP_PLAY_CTRL),a

sap_data:
    incbin  "music.sap"
sap_data_end:
```

**6502:**
```assembly
; Play a .sap file with looping
    lda  #1
    sta  POKEY_PLUS_CTRL
    STORE32 SAP_PLAY_PTR_0, sap_data
    STORE32 SAP_PLAY_LEN_0, (sap_data_end-sap_data)
    lda  #0
    sta  SAP_SUBSONG             ; Subsong 0
    lda  #5                      ; bit0=start, bit2=loop
    sta  SAP_PLAY_CTRL

sap_data:
    .incbin "music.sap"
sap_data_end:
```

**Playback Control:**
- Write `1` to SAP_PLAY_CTRL to start, `2` to stop, `5` to start with loop
- Set SAP_SUBSONG before starting to select a specific subsong (0-255)
- Read SAP_PLAY_STATUS for busy/error flags

## 11.6 SID Sound Chip

The SID chip emulates the legendary MOS 6581/8580 from the Commodore 64, providing three voices of analog-style synthesis with the distinctive warm sound that defined a generation of computer music.

### Features:
- Three independent voices with full ADSR envelopes
- Four waveforms per voice: triangle, sawtooth, pulse (with variable width), noise
- Combined waveforms (AND-style mixing when multiple waveform bits set)
- Ring modulation between voices
- Hard sync for complex timbres (accurate sync timing)
- Test bit support (resets oscillator phase and holds output)
- OSC3 and ENV3 register readback (oscillator and envelope output)
- Programmable resonant filter (low-pass, band-pass, high-pass, notch)
- Rate counter ADSR with exponential decay curve
- SID+ enhanced audio processing mode
- .SID file playback with embedded 6502 code execution

### Waveform Selection:
Each voice can output one waveform at a time via the control register:
- **Triangle (0x10)**: Smooth, flute-like tone
- **Sawtooth (0x20)**: Bright, brassy tone with rich harmonics
- **Pulse (0x40)**: Square wave with variable duty cycle (PWM capable)
- **Noise (0x80)**: White noise for percussion and effects

### ADSR Envelope:
Each voice has a dedicated ADSR envelope generator:
- Attack: 2ms to 8 seconds (16 rates)
- Decay: 6ms to 24 seconds (16 rates)
- Sustain: 16 levels (0-15)
- Release: 6ms to 24 seconds (16 rates)

### Filter:
The SID's resonant filter can process any combination of voices:
- 11-bit cutoff frequency control
- 4-bit resonance control
- Selectable low-pass, band-pass, high-pass modes (combinable for notch)

### Configuration Example:

Configure SID voice 1 for a pulse wave with filter:

**IE32:**
```assembly
; Configure SID voice 1 for a pulse wave with filter
LOAD A, #0x00
STORE A, @0xF0E00      ; Freq low
LOAD A, #0x1C          ; ~440Hz (A4)
STORE A, @0xF0E01      ; Freq high
LOAD A, #0x00
STORE A, @0xF0E02      ; Pulse width low
LOAD A, #0x08          ; 50% duty
STORE A, @0xF0E03      ; Pulse width high
LOAD A, #0x41          ; Pulse waveform + gate
STORE A, @0xF0E04      ; Control
LOAD A, #0x00          ; Fast attack, no decay
STORE A, @0xF0E05      ; Attack/Decay
LOAD A, #0xF0          ; Full sustain, no release
STORE A, @0xF0E06      ; Sustain/Release
LOAD A, #0x1F          ; Max volume + low-pass
STORE A, @0xF0E18      ; Mode/Volume
```

**M68K:**
```assembly
; Configure SID voice 1 for a pulse wave with filter
    move.b  #$00,$F0E00.l      ; Freq low
    move.b  #$1C,$F0E01.l      ; Freq high (~440Hz)
    move.b  #$00,$F0E02.l      ; Pulse width low
    move.b  #$08,$F0E03.l      ; Pulse width high (50% duty)
    move.b  #$41,$F0E04.l      ; Pulse waveform + gate
    move.b  #$00,$F0E05.l      ; Attack/Decay (fast attack)
    move.b  #$F0,$F0E06.l      ; Sustain/Release (full sustain)
    move.b  #$1F,$F0E18.l      ; Mode/Volume (max + low-pass)
```

**Z80:**
```assembly
; Configure SID voice 1 for a pulse wave with filter
; Z80 uses port I/O: port $E0 = register select, port $E1 = data
    ld   a,0               ; Select freq low register
    out  ($E0),a
    ld   a,$00
    out  ($E1),a
    ld   a,1               ; Select freq high register
    out  ($E0),a
    ld   a,$1C             ; ~440Hz
    out  ($E1),a
    ld   a,2               ; Select pulse width low
    out  ($E0),a
    ld   a,$00
    out  ($E1),a
    ld   a,3               ; Select pulse width high
    out  ($E0),a
    ld   a,$08             ; 50% duty
    out  ($E1),a
    ld   a,4               ; Select control register
    out  ($E0),a
    ld   a,$41             ; Pulse waveform + gate
    out  ($E1),a
    ld   a,5               ; Select attack/decay
    out  ($E0),a
    ld   a,$00             ; Fast attack
    out  ($E1),a
    ld   a,6               ; Select sustain/release
    out  ($E0),a
    ld   a,$F0             ; Full sustain
    out  ($E1),a
    ld   a,$18             ; Select mode/volume
    out  ($E0),a
    ld   a,$1F             ; Max volume + low-pass
    out  ($E1),a
```

**6502:**
```assembly
; Configure SID voice 1 for a pulse wave with filter
; 6502 uses memory-mapped I/O at $D500-$D51C (native SID location)
    lda  #$00
    sta  $D500             ; Freq low
    lda  #$1C
    sta  $D501             ; Freq high (~440Hz)
    lda  #$00
    sta  $D502             ; Pulse width low
    lda  #$08
    sta  $D503             ; Pulse width high (50% duty)
    lda  #$41
    sta  $D504             ; Pulse waveform + gate
    lda  #$00
    sta  $D505             ; Attack/Decay (fast attack)
    lda  #$F0
    sta  $D506             ; Sustain/Release (full sustain)
    lda  #$1F
    sta  $D518             ; Mode/Volume (max + low-pass)
```

### SID File Playback

The SID player handles Commodore 64 music files (.sid) which contain embedded 6502 code that drives the SID sound chip. The player executes the 6502 init routine once, then calls the play routine each frame at the correct rate.

**IE32:**
```assembly
; Play a .sid file with looping
    LOAD A, #1
    STORE A, @SID_PLUS_CTRL      ; Enable SID+ enhanced audio
    LOAD A, #sid_data            ; Address of embedded SID file
    STORE A, @SID_PLAY_PTR
    LOAD A, #sid_data_end - sid_data
    STORE A, @SID_PLAY_LEN
    LOAD A, #0
    STORE A, @SID_SUBSONG        ; Select subsong 0
    LOAD A, #5                   ; bit0=start, bit2=loop
    STORE A, @SID_PLAY_CTRL

sid_data:
    .incbin "music.sid"
sid_data_end:
```

**M68K:**
```assembly
; Play a .sid file with looping
    move.b  #1,SID_PLUS_CTRL.l
    lea     sid_data,a0
    move.l  a0,SID_PLAY_PTR.l
    move.l  #sid_data_end-sid_data,SID_PLAY_LEN.l
    move.b  #0,SID_SUBSONG.l     ; Subsong 0
    move.l  #5,SID_PLAY_CTRL.l   ; bit0=start, bit2=loop

sid_data:
    incbin  "music.sid"
sid_data_end:
```

**Z80:**
```assembly
; Play a .sid file with looping
    ld   a,1
    ld   (SID_PLUS_CTRL),a
    SET_SID_PTR sid_data
    SET_SID_LEN (sid_data_end-sid_data)
    SET_SID_SUBSONG 0
    START_SID_LOOP               ; Macro: start with looping

sid_data:
    incbin  "music.sid"
sid_data_end:
```

**6502:**
```assembly
; Play a .sid file with looping
    lda  #1
    sta  SID_PLUS_CTRL
    STORE32 SID_PLAY_PTR_0, sid_data
    STORE32 SID_PLAY_LEN_0, (sid_data_end-sid_data)
    lda  #0
    sta  SID_SUBSONG             ; Subsong 0
    lda  #5                      ; bit0=start, bit2=loop
    sta  SID_PLAY_CTRL

sid_data:
    .incbin "music.sid"
sid_data_end:
```

**Playback Control:**
- Write `1` to SID_PLAY_CTRL to start, `2` to stop, `5` to start with loop
- Set SID_SUBSONG before starting to select a specific subsong (0-255)
- Read SID_PLAY_STATUS for busy/error flags
- Many SID files contain multiple subsongs (tunes) - check the SID header for count

## 11.7 TED Sound Chip

The TED (Text Editing Device) chip emulates the sound capabilities of the Commodore Plus/4 and C16, providing simple 2-voice square wave synthesis. While simpler than the SID, the TED has a distinctive lo-fi character valued by demoscene musicians.

### Features:
- Two independent square wave voices
- 10-bit frequency control per voice (0-1023)
- Voice 2 can optionally produce white noise
- Global 4-bit volume control (0-8)
- TED+ enhanced audio processing mode
- .TED file playback with embedded 6502 code execution

### Voice Configuration:
Each voice outputs a square wave with 10-bit frequency resolution:
- **Frequency range**: ~107 Hz to ~110 kHz (at PAL clock)
- **Formula**: `freq_hz = clock/8 / (1024 - register_value)`
- **Clock**: 886724 Hz (PAL), 894886 Hz (NTSC)

### Control Register:
The TED_SND_CTRL register controls all audio output:
- **Bit 7**: D/A mode (direct audio output)
- **Bit 6**: Voice 2 noise enable (white noise instead of square)
- **Bit 5**: Voice 2 enable
- **Bit 4**: Voice 1 enable
- **Bits 0-3**: Volume (0-8, where 8 is maximum)

### Configuration Example:

Configure TED for two square wave voices:

**IE32:**
```assembly
; Configure TED voice 1 at ~440Hz (A4)
LOAD A, #0xE3          ; Low byte of frequency
STORE A, @0xF0F00      ; TED_FREQ1_LO
LOAD A, #0x01          ; High byte (bits 0-1)
STORE A, @0xF0F04      ; TED_FREQ1_HI

; Configure TED voice 2 at ~880Hz (A5)
LOAD A, #0xF1          ; Low byte of frequency
STORE A, @0xF0F01      ; TED_FREQ2_LO
LOAD A, #0x02          ; High byte
STORE A, @0xF0F02      ; TED_FREQ2_HI

; Enable both voices, volume 8
LOAD A, #0x38          ; ch1on + ch2on + volume 8
STORE A, @0xF0F03      ; TED_SND_CTRL
```

**M68K:**
```assembly
; Configure TED voice 1 at ~440Hz
    move.b  #$E3,$F0F00.l      ; Freq1 low
    move.b  #$01,$F0F04.l      ; Freq1 high

; Configure TED voice 2 at ~880Hz
    move.b  #$F1,$F0F01.l      ; Freq2 low
    move.b  #$02,$F0F02.l      ; Freq2 high

; Enable both voices, volume 8
    move.b  #$38,$F0F03.l      ; Control: ch1on + ch2on + vol8
```

**Z80:**
```assembly
; Configure TED voice 1 at ~440Hz
; Z80 uses port I/O: port $F2 = register select, port $F3 = data
    ld   a,0               ; Select TED_FREQ1_LO
    out  ($F2),a
    ld   a,$E3             ; Low byte
    out  ($F3),a
    ld   a,4               ; Select TED_FREQ1_HI
    out  ($F2),a
    ld   a,$01             ; High byte
    out  ($F3),a

; Enable both voices
    ld   a,3               ; Select TED_SND_CTRL
    out  ($F2),a
    ld   a,$38             ; ch1on + ch2on + volume 8
    out  ($F3),a
```

**6502:**
```assembly
; Configure TED voice 1 at ~440Hz
    lda  #$E3
    sta  TED_FREQ1_LO
    lda  #$01
    sta  TED_FREQ1_HI

; Configure TED voice 2 at ~880Hz
    lda  #$F1
    sta  TED_FREQ2_LO
    lda  #$02
    sta  TED_FREQ2_HI

; Enable both voices, volume 8
    lda  #$38              ; ch1on + ch2on + volume 8
    sta  TED_SND_CTRL
```

### .TED File Playback:

Play Commodore Plus/4 music files (.ted format from HVTC collection):

**IE32:**
```assembly
; Play a .ted file with TED+ enhancement
LOAD A, #1
STORE A, @0xF0F05          ; Enable TED+ mode
LOAD A, @ted_data          ; Point to embedded data
STORE A, @0xF0F10          ; TED_PLAY_PTR
LOAD A, @(ted_data_end - ted_data)
STORE A, @0xF0F14          ; TED_PLAY_LEN
LOAD A, #5                 ; Start with looping
STORE A, @0xF0F18          ; TED_PLAY_CTRL

ted_data:
    INCBIN "music.ted"
ted_data_end:
```

**6502:**
```assembly
; Play a .ted file with looping
    lda  #1
    sta  TED_PLUS_CTRL
    STORE32 TED_PLAY_PTR_0, ted_data
    STORE32 TED_PLAY_LEN_0, (ted_data_end-ted_data)
    lda  #5                      ; bit0=start, bit2=loop
    sta  TED_PLAY_CTRL

ted_data:
    .incbin "music.ted"
ted_data_end:
```

**Playback Control:**
- Write `1` to TED_PLAY_CTRL to start, `2` to stop, `5` to start with loop
- Read TED_PLAY_STATUS for busy/error flags

## 11.8 AHX Sound Chip

The AHX engine provides Amiga AHX/THX module playback with 4-channel waveform synthesis. AHX modules use procedural synthesis rather than samples, creating rich sounds from simple waveforms (triangle, sawtooth, square, noise) with modulation.

### Features:
- 4-channel synthesis matching original Amiga hardware
- Triangle, sawtooth, square, and noise waveforms
- Per-voice filter modulation and square pulse width modulation
- Vibrato, portamento, and envelope effects
- AHX+ enhanced mode with stereo spread and audio processing
- Subsong support for multi-tune modules

### .AHX File Playback:

**M68K:**
```assembly
; Play an .ahx file with looping and AHX+ enhancement
    PLAY_AHX_PLUS_LOOP ahx_data,ahx_data_end-ahx_data

ahx_data:
    incbin  "music.ahx"
ahx_data_end:
```

**Z80:**
```assembly
; Play an .ahx file with looping
    ENABLE_AHX_PLUS              ; Enable enhanced mode
    SET_AHX_PTR ahx_data
    SET_AHX_LEN (ahx_data_end-ahx_data)
    START_AHX_LOOP               ; Start with looping

ahx_data:
    incbin  "music.ahx"
ahx_data_end:
```

**6502:**
```assembly
; Play an .ahx file with looping
    lda  #1
    sta  AHX_PLUS_CTRL           ; Enable AHX+ mode
    STORE32 AHX_PLAY_PTR_0, ahx_data
    STORE32 AHX_PLAY_LEN_0, (ahx_data_end-ahx_data)
    lda  #5                      ; bit0=start, bit2=loop
    sta  AHX_PLAY_CTRL

ahx_data:
    .incbin "music.ahx"
ahx_data_end:
```

**Playback Control:**
- Write `1` to AHX_PLAY_CTRL to start, `2` to stop, `5` to start with loop
- Set AHX_SUBSONG before starting to select a specific subsong (0-255)
- Read AHX_PLAY_STATUS for busy/error flags
- Speed 0 command in the module signals end of song

# 12. Video System

The video system provides flexible graphics output through a memory-mapped framebuffer design.

## 12.1 Display Modes

Three resolution modes are available:
- 640x480 (MODE_640x480)
- 800x600 (MODE_800x600)
- 1024x768 (MODE_1024x768)

### Setting display mode:

**IE32:**
```assembly
init_display:
    LOAD A, #0          ; MODE_640x480
    STORE A, @VIDEO_MODE
    LOAD A, #1          ; Enable display
    STORE A, @VIDEO_CTRL
    RTS
```

**M68K:**
```assembly
init_display:
    move.l  #0,VIDEO_MODE.l        ; MODE_640x480
    move.l  #1,VIDEO_CTRL.l        ; Enable display
    rts
```

**Z80:**
```assembly
init_display:
    xor  a                         ; MODE_640x480
    ld   (VIDEO_MODE),a
    ld   a,1                       ; Enable display
    ld   (VIDEO_CTRL),a
    ret
```

**6502:**
```assembly
init_display:
    lda  #0
    sta  VIDEO_MODE                ; MODE_640x480
    lda  #1
    sta  VIDEO_CTRL                ; Enable display
    rts
```

## 12.2 Framebuffer Organisation

The framebuffer uses 32-bit RGBA colour format:
- Start address: 0x100000 (VRAM_START)
- Each pixel: 4 bytes (R,G,B,A)
- Linear layout: y * width + x

### Pixel address calculation:

```
Address = 0x100000 + (y * width + x) * 4
```

## 12.3 Dirty Rectangle Tracking

The system tracks changes in 32x32 pixel blocks:
- Automatically marks modified regions
- Updates only changed areas
- Improves rendering performance

## 12.4 Double Buffering and VBlank Synchronisation

Video output uses double buffering to prevent tearing. The system provides a VBlank status bit for flicker-free animation:

- `VIDEO_STATUS` (0x0F0008) bit 1 indicates VBlank period (safe to draw)
- VBlank is read lock-free - no mutex contention during polling
- Write to back buffer during VBlank to avoid screen flicker
- Buffers swap automatically

### VBlank Timing

The VBlank flag follows this timing:
1. `inVBlank = false` when frame processing starts (active scan)
2. `inVBlank = true` after frame is sent to display (CPU can safely draw)

### Frame Synchronisation Pattern

For smooth animation, wait for a complete frame boundary (VBlank transition):

**IE32:**
```assembly
.equ VIDEO_STATUS   0xF0008
.equ STATUS_VBLANK  2           ; bit 1

; Wait for exactly one frame - prevents animation running too fast
wait_frame:
    ; First wait for VBlank to END (active scan period)
.wait_not_vblank:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JNZ A, .wait_not_vblank

    ; Then wait for VBlank to START (new frame)
.wait_vblank_start:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JZ A, .wait_vblank_start
    RTS
```

**M68K:**
```assembly
; Wait for exactly one frame
wait_frame:
.wait_not_vblank:
    move.l  VIDEO_STATUS.l,d0
    and.l   #STATUS_VBLANK,d0
    bne.s   .wait_not_vblank
.wait_vblank_start:
    move.l  VIDEO_STATUS.l,d0
    and.l   #STATUS_VBLANK,d0
    beq.s   .wait_vblank_start
    rts
```

**Z80:**
```assembly
; Wait for exactly one frame
wait_frame:
.wait_not_vblank:
    ld   a,(VIDEO_STATUS)
    and  STATUS_VBLANK
    jr   nz,.wait_not_vblank
.wait_vblank_start:
    ld   a,(VIDEO_STATUS)
    and  STATUS_VBLANK
    jr   z,.wait_vblank_start
    ret
```

**6502:**
```assembly
; Wait for exactly one frame
wait_frame:
    ; Wait for VBlank to END
@wait_not_vblank:
    lda  VIDEO_STATUS
    and  #STATUS_VBLANK
    bne  @wait_not_vblank
    ; Wait for VBlank to START
@wait_vblank_start:
    lda  VIDEO_STATUS
    and  #STATUS_VBLANK
    beq  @wait_vblank_start
    rts
```

### Animation Loop Example

```assembly
main_loop:
    JSR wait_frame      ; Wait for frame boundary (60 FPS)
    JSR erase_sprite    ; Erase old sprite position
    JSR update_position ; Calculate new position
    JSR draw_sprite     ; Draw at new position
    JMP main_loop
```

The `wait_frame` pattern ensures exactly one frame per loop iteration, giving smooth 60 FPS animation regardless of how fast the CPU runs.

## 12.5 Direct VRAM Access Mode

For fullscreen effects such as plasma, fire, or tunnel demos where every pixel is updated each frame, the system provides a direct VRAM access mode with lock-free dirty tracking that bypasses the standard memory bus. This delivers approximately **4.5x video throughput** compared to bus-based access.

### Performance Comparison

| Mode | Writes/sec | Approx FPS | Use Case |
|------|------------|------------|----------|
| Bus-based | ~1.4M | ~9 | Partial screen updates, sprites |
| Direct VRAM + Lock-free | ~6.3M | ~41 | Fullscreen effects, demoscene |

### How It Works

Direct VRAM mode eliminates per-pixel overhead by:
- Bypassing CPU and bus mutex locks
- Skipping I/O region mapping lookups
- Using lock-free atomic bitmap for dirty tile tracking
- Employing compare-and-swap operations instead of mutex locks

### API Usage (Go)

```go
// Enable direct VRAM mode and get buffer pointer
vramBuffer := videoChip.EnableDirectMode()

// Attach buffer to CPU for fast writes
cpu.AttachDirectVRAM(vramBuffer, VRAM_START, VRAM_START+uint32(len(vramBuffer)))

// ... run demo ...

// Cleanup
cpu.DetachDirectVRAM()
videoChip.DisableDirectMode()
```

### When to Use

- **Use direct mode** for fullscreen effects where most pixels change every frame
- **Use bus mode** for partial updates, sprites, or when dirty region tracking is beneficial

Direct VRAM mode is ideal for demoscene-style effects, real-time visualisations, and any application that redraws the entire screen each frame.

## 12.6 Copper List Executor

The video subsystem includes a simple copper-like list executor for mid-frame register updates. Copper lists are stored in RAM as little-endian 32-bit words and can WAIT on raster positions, MOVE values into video registers, and END the list. The copper restarts each frame while enabled.

Registers:
- `COPPER_CTRL` (0x0F000C): bit0=enable, bit1=reset/rewind
- `COPPER_PTR`  (0x0F0010): list base address (latched on enable/reset)
- `COPPER_PC`   (0x0F0014): current list address (read-only)
- `COPPER_STATUS` (0x0F0018): bit0=running, bit1=waiting, bit2=halted

List words:
- `WAIT`: `(0<<30) | (y<<12) | x` - Wait until raster reaches Y/X position
- `MOVE`: `(1<<30) | (regIndex<<16)` followed by a 32-bit value - Write to register
- `SETBASE`: `(2<<30) | (addr>>2)` - Change base address for MOVE operations
- `END`: `(3<<30)` - Halt copper for this frame

### SETBASE Instruction

The `SETBASE` instruction allows the copper to write to any memory-mapped I/O device by changing the base address for subsequent MOVE operations. This enables cross-device effects like modifying VGA palette registers from the copper.

**SETBASE Format:**
- Bits 30-31: Opcode (2)
- Bits 0-23: Target address right-shifted by 2 (addresses are 4-byte aligned)

**Pre-defined SETBASE values:**
- `COP_SETBASE_VIDEO` (0x8003C000) - IE video registers (default)
- `COP_SETBASE_VGA` (0x8003C400) - VGA registers
- `COP_SETBASE_VGA_DAC` (0x8003C416) - VGA DAC for palette writes

The base is reset to VIDEO_REG_BASE at the start of each frame.

### Per-Scanline Execution

The copper executes synchronously with the video compositor's scanline rendering. When the compositor renders each scanline:

1. The copper advances, executing instructions until it reaches a WAIT for a future scanline
2. MOVE instructions take effect immediately for the current scanline
3. VGA renders its scanline using the current palette state

This means copper-driven palette changes affect only the scanlines rendered after the change, enabling classic raster effects like:
- Gradient backgrounds (changing palette entries per scanline)
- Split-screen color schemes
- Plasma and interference patterns

### Cross-Device Copper Example (VGA Palette + IE Raster Bars)

**IE32:**
```assembly
.include "ie32.inc"

copper_list:
    ; Switch to VGA DAC registers
    .long COP_SETBASE_VGA_DAC

    ; Set palette entry 32 to red
    .long COP_MOVE_VGA_WINDEX
    .long 32                          ; Palette index
    .long COP_MOVE_VGA_DATA
    .long 63                          ; R (6-bit VGA)
    .long COP_MOVE_VGA_DATA
    .long 0                           ; G
    .long COP_MOVE_VGA_DATA
    .long 0                           ; B

    ; Switch back to IE video chip
    .long COP_SETBASE_VIDEO

    ; Draw raster bar at Y=100
    .long COP_WAIT_MASK | (100 * COP_WAIT_SCALE)
    .long COP_MOVE_RASTER_Y
    .long 100
    .long COP_MOVE_RASTER_H
    .long 8
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF00FFFF                  ; Cyan (RGBA)
    .long COP_MOVE_RASTER_CTRL
    .long 1

    .long COP_END
```

## 12.7 DMA Blitter

The DMA blitter performs rectangle copy/fill and line drawing. Registers are written via memory-mapped I/O, and the blitter operates on VRAM addresses (RGBA, 4 bytes/pixel).

**Synchronous Execution**: The blitter runs immediately when `BLT_CTRL` is written with bit0 set. This ensures blitter operations complete before the CPU continues, allowing safe drawing during VBlank without race conditions.

Operations (`BLT_OP`):
- `0`: COPY
- `1`: FILL
- `2`: LINE (coordinates packed into `BLT_SRC`/`BLT_DST`)
- `3`: MASKED COPY (1-bit mask, LSB-first per byte)
- `4`: ALPHA (alpha-aware copy with source alpha blending)
- `5`: MODE7 (affine texture mapping with 16.16 UV coordinates)

Line coordinates:
- `BLT_SRC`: x0 (low 16 bits), y0 (high 16 bits)
- `BLT_DST`: x1 (low 16 bits), y1 (high 16 bits)

### Mode7 Parameters

`BLT_OP=5` uses these registers in addition to the normal blitter source/destination/size:
- `BLT_MODE7_U0`, `BLT_MODE7_V0`: start coordinates in signed 16.16 fixed-point.
- `BLT_MODE7_DU_COL`, `BLT_MODE7_DV_COL`: UV deltas per destination X pixel.
- `BLT_MODE7_DU_ROW`, `BLT_MODE7_DV_ROW`: UV deltas applied when moving to next destination row.
- `BLT_MODE7_TEX_W`, `BLT_MODE7_TEX_H`: wrap masks (must be `2^n-1`, for example `255` for 256).
- `BLT_SRC_STRIDE`: source row stride bytes. `0` means auto `((texWMask+1)*4)`.
- `BLT_DST_STRIDE`: destination row stride bytes. `0` keeps normal auto behavior.

Example UV mapping values:
- Identity sampling: `duCol=0x00010000`, `dvCol=0`, `duRow=0`, `dvRow=0x00010000`.
- Rotation/zoom: precompute sine/cosine in software and write fixed-point deltas once per frame.

### Blitter Example (fill a 16x16 block):

**IE32:**
```assembly
    LOAD A, #1              ; BLT_OP_FILL
    STORE A, @BLT_OP
    LOAD A, #0x100000       ; VRAM_START
    STORE A, @BLT_DST
    LOAD A, #16
    STORE A, @BLT_WIDTH
    LOAD A, #16
    STORE A, @BLT_HEIGHT
    LOAD A, #0xFF00FF00     ; green (RGBA)
    STORE A, @BLT_COLOR
    LOAD A, #1
    STORE A, @BLT_CTRL      ; start
```

**M68K:**
```assembly
    move.l  #BLT_OP_FILL,BLT_OP.l
    move.l  #VRAM_START,BLT_DST.l
    move.l  #16,BLT_WIDTH.l
    move.l  #16,BLT_HEIGHT.l
    move.l  #$FF00FF00,BLT_COLOR.l   ; green (RGBA)
    move.l  #1,BLT_CTRL.l            ; start
```

**Z80:**
```assembly
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST VRAM_START
    SET_BLT_WIDTH 16
    SET_BLT_HEIGHT 16
    SET_BLT_COLOR 0xFF00FF00         ; green (RGBA)
    START_BLIT
```

**6502:**
```assembly
    lda  #BLT_OP_FILL
    sta  BLT_OP
    STORE32 BLT_DST_0, VRAM_START
    SET_BLT_WIDTH 16
    SET_BLT_HEIGHT 16
    SET_BLT_COLOR $FF00FF00          ; green (RGBA)
    START_BLIT
```

The blitter defaults `BLT_SRC_STRIDE`/`BLT_DST_STRIDE` to the current mode row bytes when the address is in VRAM, otherwise it uses `width*4`. If an unaligned VRAM address is used, `BLT_STATUS.bit0` is set.

## 12.8 Raster Band Fill

`VIDEO_RASTER_*` registers draw a full-width horizontal band directly into the framebuffer. This is useful for copper-driven raster bars without adding a palette system.

### Raster Band Example (draw 4-pixel band at Y=100):

**IE32:**
```assembly
    LOAD A, #100
    STORE A, @VIDEO_RASTER_Y
    LOAD A, #4
    STORE A, @VIDEO_RASTER_HEIGHT
    LOAD A, #0xFFFF0000           ; blue (RGBA)
    STORE A, @VIDEO_RASTER_COLOR
    LOAD A, #1
    STORE A, @VIDEO_RASTER_CTRL
```

**M68K:**
```assembly
    move.l  #100,VIDEO_RASTER_Y.l
    move.l  #4,VIDEO_RASTER_HEIGHT.l
    move.l  #$FFFF0000,VIDEO_RASTER_COLOR.l  ; blue (RGBA)
    move.l  #1,VIDEO_RASTER_CTRL.l
```

**Z80:**
```assembly
    STORE32 VIDEO_RASTER_Y_LO,100
    STORE32 VIDEO_RASTER_HEIGHT_LO,4
    STORE32 VIDEO_RASTER_COLOR_0,0xFFFF0000  ; blue (RGBA)
    ld   a,1
    ld   (VIDEO_RASTER_CTRL),a
```

**6502:**
```assembly
    STORE32 VIDEO_RASTER_Y_LO, 100
    STORE32 VIDEO_RASTER_HEIGHT_LO, 4
    STORE32 VIDEO_RASTER_COLOR_0, $FFFF0000  ; blue (RGBA)
    lda  #1
    sta  VIDEO_RASTER_CTRL
```

## 12.9 Video Compositor

The Intuition Engine uses a video compositor to blend multiple video sources into a single display output. This architecture enables layered rendering where different video devices (VideoChip, VGA) can contribute to the final frame.

### Architecture

```
                    +-------------+
  CPU -> VGA VRAM -> |   VGAEngine | --+
                    +-------------+   |     +-------------+     +---------+
                                      +---> | Compositor  | --> | Display |
                    +-------------+   |     +-------------+     +---------+
  CPU -> Chip VRAM -> |  VideoChip  | --+
                    +-------------+
```

### Layer Ordering

Each video source has a layer number that determines compositing order (higher layers render on top):

| Source | Layer | Description |
|--------|-------|-------------|
| VideoChip | 0 | Base layer with copper coprocessor |
| VGA | 10 | Overlays on top of VideoChip |

### Per-Scanline Rendering

The compositor supports two rendering modes:

**Full-Frame Mode:** Each source renders its complete frame, then frames are composited. Simple but copper effects only affect video registers, not VGA palette.

**Scanline-Aware Mode:** Sources that implement the `ScanlineAware` interface render one scanline at a time. This enables:
- Copper MOVE operations to affect the current scanline immediately
- Per-scanline VGA palette changes via SETBASE
- Classic demoscene raster effects (color cycling, plasma bars)

The compositor automatically uses scanline-aware rendering when all enabled sources support it. The render sequence per scanline is:
1. VideoChip processes copper list up to current Y position
2. VideoChip renders its scanline
3. VGA renders its scanline using current palette state
4. Scanlines are composited in layer order

### Copper + VGA Integration

The copper's SETBASE instruction enables per-scanline VGA palette manipulation:

```assembly
; Create a raster bar effect by changing VGA palette per scanline
copper_list:
    ; Wait for scanline 50
    .long COP_WAIT_MASK | (50 * COP_WAIT_SCALE)

    ; Switch to VGA DAC registers
    .long COP_SETBASE_VGA_DAC

    ; Set color 0 to red for this scanline
    .long COP_MOVE_VGA_WINDEX
    .long 0
    .long COP_MOVE_VGA_DATA
    .long 63                          ; R
    .long COP_MOVE_VGA_DATA
    .long 0                           ; G
    .long COP_MOVE_VGA_DATA
    .long 0                           ; B

    ; Wait for scanline 60
    .long COP_WAIT_MASK | (60 * COP_WAIT_SCALE)

    ; Set color 0 to blue
    .long COP_MOVE_VGA_WINDEX
    .long 0
    .long COP_MOVE_VGA_DATA
    .long 0                           ; R
    .long COP_MOVE_VGA_DATA
    .long 0                           ; G
    .long COP_MOVE_VGA_DATA
    .long 63                          ; B

    ; Return to video chip registers
    .long COP_SETBASE_VIDEO

    .long COP_END
```

This creates a horizontal band where the VGA background color (palette entry 0) changes from red to blue at specific scanlines.

# 13. Developer's Guide

## 13.1 Development Environment Setup

To develop for the Intuition Engine, you'll need to set up your development environment with several components:

1. Install the Go programming language (version 1.21 or later)
2. Install required development libraries:
   - None for default runtime path (Ebiten + Oto)

Create a project directory structure:

```bash
my_project/
├── src/             # Assembly source files
├── bin/             # Compiled binaries
└── tools/           # Development tools
```

## 13.2 Building the System

The build process uses the provided Makefile:

```bash
# Build both VM and assembler
make

# Build only the VM
make intuition-engine

# Build only the IE32 assembler
make ie32asm

# Build only the IE64 assembler
make ie64asm

# Install to /usr/local/bin
make install

# Create AppImage package
make appimage

# Clean build artifacts
make clean
```

This creates:
```
./bin/IntuitionEngine   # The virtual machine
./bin/ie32asm           # The IE32 assembler
./bin/ie64asm           # The IE64 assembler
```

The build automatically injects version metadata (version, git commit, build date) via ldflags. Use `./bin/IntuitionEngine -version` to verify.

Available make targets:
```
all              - Build Intuition Engine, ie32asm, and ie64asm (default)
intuition-engine - Build only the Intuition Engine VM
ie32asm          - Build only the IE32 assembler
ie64asm          - Build only the IE64 assembler
ie64dis          - Build only the IE64 disassembler
basic            - Build with embedded EhBASIC interpreter
novulkan         - Build without Vulkan (software Voodoo only)
headless         - Build without display/audio (CI/testing)
headless-novulkan - Fully portable CGO_ENABLED=0 build
appimage         - Build AppImage package for Linux distributions
install          - Install binaries to $(INSTALL_BIN_DIR)
uninstall        - Remove installed binaries from $(INSTALL_BIN_DIR)
clean            - Remove all build artifacts
list             - List compiled binaries with sizes
help             - Show this help message
```

## 13.3 Development Workflow

A typical development cycle involves:

1. Write assembly code in your preferred text editor
2. Assemble the code:
```bash
./bin/ie32asm program.asm
```
3. Run the resulting program:

**IE32 programs:**
```bash
./bin/IntuitionEngine -ie32 program.iex
```

**M68K programs:**
```bash
./bin/IntuitionEngine -m68k program.ie68
```

**Z80 programs:**
```bash
./bin/IntuitionEngine -z80 program.ie80
```

**6502 programs:**
```bash
./bin/IntuitionEngine -m6502 program.bin
./bin/IntuitionEngine -m6502 --load-addr 0x0600 --entry 0x0600 program.bin
```

**PSG music playback:**
```bash
./bin/IntuitionEngine -psg track.ym
./bin/IntuitionEngine -psg track.ay
./bin/IntuitionEngine -psg track.vgm
./bin/IntuitionEngine -psg track.vgz
./bin/IntuitionEngine -psg track.sndh
./bin/IntuitionEngine -psg+ track.ym   # Enhanced audio
```

**POKEY/SAP music playback (Atari 8-bit):**
```bash
./bin/IntuitionEngine -pokey track.sap
./bin/IntuitionEngine -pokey+ track.sap  # Enhanced audio
```

**SID music playback (C64 PSID):**
```bash
./bin/IntuitionEngine -sid tune.sid
./bin/IntuitionEngine -sid+ tune.sid     # Enhanced audio
./bin/IntuitionEngine -sid-pal tune.sid
./bin/IntuitionEngine -sid-ntsc tune.sid
```

**AHX music playback (Amiga AHX/THX modules):**
```bash
./bin/IntuitionEngine -ahx module.ahx
./bin/IntuitionEngine -ahx+ module.ahx   # Enhanced audio with stereo spread
```

**Notes:**
- `.ym` files are Atari ST YM format
- `.vgm/.vgz` are VGM streams (including MSX PSG logs)
- `.ay` ZXAYEMUL files with embedded Z80 players are supported
- `.sndh` files are Atari ST SNDH format with embedded M68K code
- PSID only for SID; RSID is rejected
- Single-SID playback at $D400; multi-SID not yet implemented

**Enhanced Audio Modes (PSG+/POKEY+/SID+/TED+/AHX+):**
These modes provide 4x oversampling, second-order Butterworth lowpass filtering, subtle drive saturation, and allpass diffuser room ambience for richer sound while preserving pitch and timing. SID+ additionally preserves per-channel filter sweeps through the enhanced path. AHX+ additionally provides authentic Amiga stereo panning (L-R-R-L pattern) and hardware PWM for square wave modulation.

## 13.4 Assembler Include Files

The `assembler/` directory provides hardware definition include files for each CPU architecture. These files are essential for writing portable Intuition Engine programs.

| File | CPU | Assembler | Description |
|------|-----|-----------|-------------|
| `ie32.inc` | IE32 | ie32asm | Hardware constants and register definitions |
| `ie68.inc` | M68K | vasmm68k_mot | Hardware constants with M68K macros |
| `ie65.inc` | 6502 | ca65 | Hardware constants, macros, and zero page allocation |
| `ie80.inc` | Z80 | vasmz80_std | Hardware constants with Z80 macros |
| `ie86.inc` | x86 | NASM/FASM | Hardware constants, port I/O, VGA registers |
| `ie64.inc` | IE64 | ie64asm | Hardware constants and macros |
| `ie64_fp.inc` | IE64 | ie64asm | IEEE 754 FP32 math wrappers backed by IE64 hardware FPU |
| `ehbasic_ie64.asm` | IE64 | ie64asm | EhBASIC interpreter (entry point + REPL) |
| `ehbasic_*.inc` | IE64 | ie64asm | EhBASIC modules (tokeniser, executor, expressions, variables, strings, I/O, hardware commands) |

### Contents Overview

All include files provide:

- **Video Registers**: VIDEO_CTRL, VIDEO_MODE, VIDEO_STATUS, blitter, copper, raster band
- **Audio Registers**: PSG (raw + player), POKEY (raw + SAP player), SID (raw + SID player)
- **Memory Constants**: VRAM_START, SCREEN_W/H, LINE_BYTES
- **Blitter Operations**: BLT_OP_COPY, BLT_OP_FILL, BLT_OP_LINE, BLT_OP_MASKED, BLT_OP_ALPHA, BLT_OP_MODE7
- **Copper Opcodes**: COP_WAIT_MASK, COP_MOVE_RASTER_*, COP_END
- **Timer Registers**: TIMER_CTRL, TIMER_COUNT, TIMER_RELOAD

### ie32.inc (IE32 CPU)

The IE32 include file provides constants using `.equ` directives:

```assembly
.include "ie32.inc"

start:
    LOAD A, #1
    STORE A, @VIDEO_CTRL        ; Use constant instead of raw address
    LOAD A, #BLT_OP_FILL
    STORE A, @BLT_OP
```

### ie68.inc (M68K CPU)

The M68K include file provides constants using `equ` and helper macros:

```assembly
    include "ie68.inc"

start:
    move.l  #1,VIDEO_CTRL.l
    wait_vblank                 ; Macro: wait for vertical blank
    set_blt_color $FF00FF00     ; Macro: set blitter fill color
    start_blit                  ; Macro: trigger blitter
```

**Macros provided:**
- `wait_vblank` - Wait for VBlank period
- `wait_blit` - Wait for blitter to complete
- `start_blit` - Trigger blitter operation
- `set_blt_color`, `set_blt_src`, `set_blt_dst`, `set_blt_size`, `set_blt_strides`
- `set_copper_ptr`, `enable_copper`, `disable_copper`
- `set_psg_play`, `start_psg_play`, `stop_psg_play`, `enable_psg_plus`
- `set_sid_play`, `start_sid_play`, `start_sid_loop`, `stop_sid_play`, `enable_sid_plus`
- `set_sap_play`, `start_sap_play`, `stop_sap_play`
- `PLAY_AHX`, `PLAY_AHX_LOOP`, `PLAY_AHX_PLUS`, `PLAY_AHX_PLUS_LOOP`, `STOP_AHX`
- `coproc_start`, `coproc_stop`, `coproc_enqueue`, `coproc_poll`, `coproc_wait` - Coprocessor helpers

### ie65.inc (6502 CPU)

The 6502 include file is the most comprehensive, providing constants, macros, and zero page allocation:

```assembly
.include "ie65.inc"

.segment "CODE"
start:
    lda  #1
    sta  VIDEO_CTRL             ; Memory-mapped at $F000
    WAIT_VBLANK                 ; Macro: wait for VBlank
    SET_BLT_OP BLT_OP_FILL      ; Macro: set blitter operation
    SET_BLT_WIDTH 16            ; Macro: set 16-bit width
    SET_BLT_HEIGHT 16           ; Macro: set 16-bit height
    SET_BLT_COLOR $FF00FF00     ; Macro: set 32-bit color
    START_BLIT                  ; Macro: trigger blitter
```

**Macros provided:**
- `SET_BANK1`, `SET_BANK2`, `SET_BANK3`, `SET_VRAM_BANK` - Bank switching
- `STORE16`, `STORE32`, `STORE32_ZP` - Multi-byte stores
- `WAIT_VBLANK`, `WAIT_BLIT`, `START_BLIT`
- `SET_BLT_OP`, `SET_BLT_WIDTH`, `SET_BLT_HEIGHT`, `SET_BLT_COLOR`
- `SET_SRC_STRIDE`, `SET_DST_STRIDE`
- `ADD16`, `INC16`, `CMP16` - 16-bit arithmetic helpers
- `SET_AHX_PTR`, `SET_AHX_LEN`, `START_AHX_PLAY`, `START_AHX_LOOP`, `STOP_AHX_PLAY`, `ENABLE_AHX_PLUS`
- `COPROC_START`, `COPROC_STOP`, `COPROC_ENQUEUE`, `COPROC_POLL`, `COPROC_WAIT` - Coprocessor helpers (via gateway `$F200`)

**Zero page allocation:**
```assembly
.zeropage
    zp_ptr0:    .res 2          ; General purpose pointer 0
    zp_ptr1:    .res 2          ; General purpose pointer 1
    zp_tmp0:    .res 4          ; 32-bit temporary 0
    zp_frame:   .res 2          ; Frame counter
    zp_scratch: .res 8          ; Scratch space
```

### ie80.inc (Z80 CPU)

The Z80 include file provides constants using `.set` and comprehensive macros:

```assembly
    .include "ie80.inc"

start:
    ld   sp,STACK_TOP
    ld   a,1
    ld   (VIDEO_CTRL),a
    WAIT_VBLANK                  ; Macro: wait for VBlank
    SET_BLT_OP BLT_OP_FILL       ; Macro: set blitter operation
    SET_BLT_DST VRAM_START       ; Macro: set 32-bit dest address
    SET_BLT_WIDTH 16
    SET_BLT_HEIGHT 16
    SET_BLT_COLOR 0xFF00FF00
    START_BLIT
```

**Macros provided:**
- `SET_BANK1`, `SET_BANK2`, `SET_BANK3`, `SET_VRAM_BANK` - Bank switching
- `STORE16`, `STORE32` - Multi-byte stores
- `WAIT_VBLANK`, `WAIT_BLIT`, `START_BLIT`
- `SET_BLT_OP`, `SET_BLT_SRC`, `SET_BLT_DST`, `SET_BLT_WIDTH`, `SET_BLT_HEIGHT`
- `SET_BLT_COLOR`, `SET_BLT_MASK`, `SET_SRC_STRIDE`, `SET_DST_STRIDE`
- `SET_COPPER_PTR`
- `SET_PSG_PTR`, `SET_PSG_LEN` - PSG player setup
- `SET_SID_PTR`, `SET_SID_LEN`, `SET_SID_SUBSONG`, `START_SID_PLAY`, `START_SID_LOOP`, `STOP_SID_PLAY`
- `SET_SAP_PTR`, `SET_SAP_LEN` - SAP player setup
- `SET_AHX_PTR`, `SET_AHX_LEN`, `START_AHX_PLAY`, `START_AHX_LOOP`, `STOP_AHX_PLAY`, `ENABLE_AHX_PLUS`
- `SID_WRITE reg,val` - Write SID register via port I/O
- `ADD_HL_IMM`, `CP_HL_IMM`, `INC16` - Utility macros
- `coproc_start`, `coproc_stop`, `coproc_enqueue`, `coproc_poll`, `coproc_wait` - Coprocessor helpers (via gateway `0xF200`)

### ie86.inc (x86 CPU)

The x86 include file provides constants using `equ` and macros for NASM/FASM:

```assembly
%include "ie86.inc"

section .text
start:
    mov     eax, 1
    mov     [VIDEO_CTRL], eax
    wait_vblank                 ; Macro: wait for vertical blank

    ; PSG port I/O
    psg_write PSG_REG_MIXER, 0x38    ; Enable channels A, B, C
    psg_write PSG_REG_VOL_A, 15      ; Max volume channel A

    ; VGA port I/O
    vga_wait_vsync              ; Wait for vertical retrace
```

**Memory-mapped addresses (32-bit):**
- Full 32-bit flat address space
- VGA VRAM at standard PC address 0xA0000-0xAFFFF
- Hardware I/O at 0xF0000+

**Port I/O for audio chips:**
- PSG: ports 0xF0-0xF1 (register select, data)
- POKEY: ports 0xD0-0xDF (direct access)
- SID: ports 0xE0-0xE1 (register select, data)
- TED: ports 0xF2-0xF3 (register select, data)

**VGA standard PC ports:**
- 0x3C4-0x3C5: Sequencer index/data
- 0x3C6-0x3C9: DAC mask, indices, data
- 0x3CE-0x3CF: Graphics controller
- 0x3D4-0x3D5: CRTC index/data
- 0x3DA: Input status (bit 3 = VSync)

**Macros provided:**
- `wait_vblank` - Wait for vertical blank via memory-mapped status
- `vga_wait_vsync` - Wait for VSync via VGA port I/O
- `psg_write reg, val` - Write PSG register via port I/O
- `sid_write reg, val` - Write SID register via port I/O
- `pokey_write reg, val` - Write POKEY register via port I/O
- `coproc_start cpuType, namePtr`, `coproc_stop`, `coproc_enqueue`, `coproc_poll`, `coproc_wait` - Coprocessor helpers

### ie64.inc (IE64 CPU)

The IE64 include file provides constants using `equ` and comprehensive macros:

```assembly
    include "ie64.inc"

start:
    la   r1, VRAM_START
    move.l r2, #1
    store.l r2, VIDEO_CTRL(r0)
    wait_vblank                     ; Macro: wait for VBlank
    set_blt_color $FF00FF00         ; Macro: set blitter fill color
    start_blit                      ; Macro: trigger blitter
```

**Macros provided:**
- `wait_vblank`, `wait_blit`, `start_blit`
- `set_blt_color`, `set_blt_src`, `set_blt_dst`, `set_blt_size`, `set_blt_strides`
- `set_copper_ptr`, `enable_copper`, `disable_copper`
- `vga_setmode`, `vga_enable`, `vga_setpalette`, `vga_palette_rgb`, `vga_wait_vsync`, `vga_mapmask`, `vga_readmap`
- `set_ula_border`, `ula_enable`, `ula_disable`
- `set_ted_v_enable`, `ted_v_disable`, `set_ted_v_border`, `ted_v_bgcolor`, `ted_v_wait_vblank`
- `set_antic_enable`, `antic_disable`, `antic_wait_vblank`, `antic_set_dlist`
- `gtia_setbk`, `gtia_setpf0`–`gtia_setpf3`
- PSG: `enable_psg_plus`, `set_psg_play`, `start_psg_play`, `stop_psg_play`
- SID: `enable_sid_plus`, `set_sid_play`, `set_sid_subsong`, `start_sid_play`, `start_sid_loop`, `stop_sid_play`
- SAP: `set_sap_play`, `set_sap_subsong`, `start_sap_play`, `start_sap_loop`, `stop_sap_play`
- AHX: `PLAY_AHX`, `PLAY_AHX_LOOP`, `PLAY_AHX_PLUS`, `PLAY_AHX_PLUS_LOOP`, `STOP_AHX`
- POKEY: `enable_pokey_plus`, `set_pokey_audctl`, `set_pokey_ch`
- Audio channels: `set_ch_freq`, `set_ch_vol`, `set_ch_wave`, `set_ch_env`, `gate_ch_on`, `gate_ch_off`, `set_filter`, `set_reverb`
- Voodoo: `voodoo_vertex_a/b/c`, `voodoo_color_rgb`, `voodoo_triangle`, `voodoo_swap`, `voodoo_clear`
- Coprocessor: `coproc_start`, `coproc_stop`, `coproc_enqueue`, `coproc_poll`, `coproc_wait`

### 8-Bit CPU Banking System

The 6502 and Z80 use a banking system to access the full 32MB address space:

| Window | Address Range | Purpose | Bank Register |
|--------|---------------|---------|---------------|
| Bank 1 | $2000-$3FFF | Sprite data | BANK1_REG_LO/HI |
| Bank 2 | $4000-$5FFF | Font data | BANK2_REG_LO/HI |
| Bank 3 | $6000-$7FFF | General data | BANK3_REG_LO/HI |
| VRAM | $8000-$BFFF | Video memory (16KB) | VRAM_BANK_REG |

**Banking example (6502):**
```assembly
    ; Switch bank 1 to sprite data at offset $10000
    SET_BANK1 $10000>>13        ; Bank number = address / 8KB
    ; Now access sprite data via $2000-$3FFF
    lda  BANK1_WINDOW           ; Read first byte
```

**Banking example (Z80):**
```assembly
    ; Switch bank 1 to sprite data at offset $10000
    SET_BANK1 (0x10000>>13)     ; Bank number = address / 8KB
    ; Now access sprite data via 0x2000-0x3FFF
    ld   a,(BANK1_WINDOW)       ; Read first byte
```

## 13.5 Debugging Techniques

### Console Output

Write values to the debug output register to display information during execution:

**IE32:**
```assembly
debug_print:
    STORE A, @0xF0700       ; Output register A value to console
    RTS
```

**M68K:**
```assembly
debug_print:
    move.l  d0,$F0700.l     ; Output D0 to console
    rts
```

**Z80:**
```assembly
debug_print:
    ld   a,l
    ld   ($F700),a          ; Output L to console
    ret
```

**6502:**
```assembly
debug_print:
    sta  $F700              ; Output A to console
    rts
```

### Memory Dumps

Dump a range of memory to inspect state:

**IE32:**
```assembly
; Dump 4 words (16 bytes) starting at address in B
dump_memory:
    LOAD C, #4              ; Counter (4 words = 16 bytes)
.loop:
    LOAD A, [B]             ; Read 32-bit word
    STORE A, @0xF0700       ; Output to console
    ADD B, #4               ; Next word address
    SUB C, #1
    JNZ C, .loop
    RTS
```

**M68K:**
```assembly
; Dump 16 bytes from address in A0
dump_memory:
    moveq   #15,d1          ; Counter (16 bytes)
.loop:
    move.b  (a0)+,d0        ; Read byte, increment
    move.l  d0,$F0700.l     ; Output
    dbra    d1,.loop
    rts
```

### Breakpoint Simulation

Insert breakpoints by halting execution:

**IE32:**
```assembly
breakpoint:
    HALT                    ; Stop CPU
```

**M68K:**
```assembly
breakpoint:
    illegal                 ; Trigger illegal instruction exception
```

**Z80:**
```assembly
breakpoint:
    halt                    ; Stop CPU
```

**6502:**
```assembly
breakpoint:
    brk                     ; Trigger break interrupt
```

### Register State Inspection

Save and display all registers at a checkpoint:

**M68K:**
```assembly
checkpoint:
    movem.l d0-d7/a0-a6,-(sp)   ; Save all registers
    ; ... inspect state ...
    movem.l (sp)+,d0-d7/a0-a6   ; Restore
    rts
```

### Hardware Status Monitoring

Check hardware state during debugging:

| Register | Address | Purpose |
|----------|---------|---------|
| VIDEO_STATUS | `$F0008` | VBlank flag (bit 1) |
| BLT_STATUS | `$F0044` | Blitter busy (bit 1) |
| PSG_PLAY_STATUS | `$F0C1C` | PSG player status |
| SID_PLAY_STATUS | `$F0E2C` | SID player status |
| SAP_PLAY_STATUS | `$F0D1C` | SAP player status |

**IE32:**
```assembly
wait_debug:
    LOAD A, @VIDEO_STATUS
    AND A, #2               ; Check VBlank bit
    BEQ wait_debug          ; Loop until set
    RTS
```

### Stack Inspection

Monitor stack usage to detect overflow:

**IE32:**
```assembly
check_stack:
    LOAD A, SP              ; Get stack pointer
    CMP A, #0x1000          ; Compare against limit
    BLT stack_overflow      ; Branch if too low
    RTS
stack_overflow:
    HALT                    ; Stop on overflow
```

**M68K:**
```assembly
check_stack:
    cmpa.l  #$1000,sp       ; Check stack limit
    blt     stack_overflow
    rts
stack_overflow:
    illegal
```

### Tips for Effective Debugging

1. **Use incremental testing**: Test small code sections before combining
2. **Monitor VBlank timing**: Ensure frame updates complete within VBlank
3. **Check memory alignment**: M68K requires word/longword alignment for 16/32-bit access
4. **Verify bank switching**: Ensure correct bank is selected before accessing windowed memory
5. **Watch for signed/unsigned issues**: Know when operations treat values as signed
6. **Trace interrupt handlers**: Ensure registers are saved/restored properly

# 14. Implementation Details

This section describes how the Intuition Engine emulates its hardware components.

## 14.1 CPU Emulation

### IE32 Custom RISC CPU

The IE32 is a custom 32-bit RISC processor designed for simplicity and performance:

- **16 general-purpose registers**: A (accumulator), B-H, S-W, X-Z (all 32-bit, orthogonal access)
- **Fixed 8-byte instruction format**: Simplifies fetch/decode
- **Load/store architecture**: Memory access only via LOAD/STORE
- **Little-endian byte order**: Matches system bus convention
- **Hardware interrupt support**: Single vector at address 0

Execution cycle: Fetch (8 bytes) → Decode → Execute → Update PC → Check interrupts

### Motorola 68EC020

The 68020 emulation provides 95%+ instruction coverage:

- **8 data registers** (D0-D7) and **8 address registers** (A0-A7)
- **Full addressing mode support**: All 18 68020 addressing modes
- **Supervisor/user modes**: Privilege separation
- **Exception handling**: Bus error, address error, illegal instruction
- **FPU emulation** (68881/68882): Floating-point operations

Variable instruction length (2-22 bytes) with complex decode logic.

### Zilog Z80

The Z80 emulation provides complete instruction set support:

- **Main and alternate register sets**: AF, BC, DE, HL with shadows
- **Index registers**: IX, IY with displacement addressing
- **Interrupt modes**: IM 0, IM 1, IM 2
- **I/O port access**: IN/OUT instructions mapped to hardware

### MOS 6502

The 6502 emulation covers the original NMOS instruction set:

- **Accumulator, X, Y registers**: 8-bit operations
- **Zero page optimisation**: Fast access to first 256 bytes
- **Stack at $0100-$01FF**: Hardware stack page
- **Status flags**: N, V, B, D, I, Z, C

### Intel x86 (32-bit)

The x86 emulation provides a 32-bit flat memory model:

- **8 general-purpose registers**: EAX, EBX, ECX, EDX, ESI, EDI, EBP, ESP
- **Segment-free flat model**: Direct 32-bit addressing
- **8086 core instruction set**: Integer instructions with 32-bit register extensions
- **I/O port access**: IN/OUT instructions mapped to hardware

### IE64 Custom 64-bit RISC CPU

The IE64 is a 64-bit RISC processor with a clean load-store architecture:

- **32 general-purpose registers**: R0 (hardwired zero) through R31 (stack pointer), all 64-bit
- **Fixed 8-byte instruction format**: Consistent fetch/decode
- **Compare-and-branch model**: No flags register; conditional branches embed register comparison
- **Size-polymorphic operations**: .B/.W/.L/.Q suffixes on most instructions
- **64-bit bus support**: Read64/Write64 with I/O region split semantics

Execution cycle: Fetch (8 bytes) → Decode → Execute → Update PC → Check timer/interrupts

## 14.2 Memory Architecture

### Unified Memory Bus

All CPUs share a unified 32MB address space through the MachineBus:

- **Direct memory array**: Fast non-I/O access via cached pointer
- **Page-based I/O callbacks**: Handlers for memory-mapped registers
- **Bank switching**: 8KB/16KB windows for 8-bit CPUs to access full memory

### Memory-Mapped I/O Regions

| Region | Address Range | Description |
|--------|---------------|-------------|
| System | `$000000-$000FFF` | Vectors, system data |
| Program | `$001000-$0EFFFF` | User program space |
| I/O | `$0F0000-$0FFFFF` | Hardware registers |
| VRAM | `$100000-$1FFFFF` | Video RAM |

### Bank Windows (8-bit CPUs)

Z80 and 6502 access the full address space through bank windows:

| Window | CPU Address | Size | Control Register |
|--------|-------------|------|------------------|
| Bank 1 | `$2000` | 8KB | `$F700-$F701` |
| Bank 2 | `$4000` | 8KB | `$F702-$F703` |
| Bank 3 | `$6000` | 8KB | `$F704-$F705` |
| VRAM | `$8000` | 16KB | `$F7F0` |

## 14.3 Audio System Architecture

### Sample Generation

Audio is synthesised at 44.1kHz with 32-bit floating-point precision:

1. **Oscillators**: Generate raw waveforms (square, triangle, sine, noise, sawtooth)
2. **Envelopes**: Shape amplitude over time (ADSR)
3. **Modulation**: Apply sync, ring mod, PWM
4. **Mixing**: Combine all channels
5. **Effects**: Apply filter, overdrive, reverb
6. **Output**: Convert to 16-bit stereo PCM

### Anti-Aliasing

PolyBLEP (polynomial bandlimited step) anti-aliasing is applied to square and sawtooth waveforms to reduce high-frequency aliasing artifacts.

### PSG/POKEY/SID Register Mapping

The classic sound chips are implemented via register mapping to the custom audio synthesizer:

- **PSG (AY-3-8910)**: Registers at `$F0C00-$F0C0D` map to square wave channels with hardware envelope emulation
- **POKEY**: Registers at `$F0D00-$F0D09` map to channels with polynomial counter and distortion emulation
- **SID (6581/8580)**: Registers at `$F0E00-$F0E1C` map to channels with filter, ring mod, and sync

This approach provides accurate register-level compatibility while leveraging the custom synth's high-quality output (44.1kHz, anti-aliased waveforms, 32-bit processing).

File playback (.ym, .ay, .sndh, .vgm, .sap, .sid) executes embedded CPU code that writes to the mapped registers, driving the synthesis in real-time.

# 15. Platform Support

The default runtime path is Ebiten (video) + Oto (audio), with no separate control-window frontend.

## 15.1 Supported Platforms

| Platform | Graphics | Audio |
|----------|----------|-------|
| Linux | Ebiten | Oto |
| macOS | Ebiten | Oto |
| Windows | Ebiten | Oto |

## 15.2 Graphics Backends

### Ebiten (Primary)

The default graphics backend providing:
- Hardware-accelerated rendering via OpenGL/Metal/DirectX
- Automatic display scaling
- VSync synchronisation
- Cross-platform window management

### Headless Mode

For testing and batch processing without a display:
```bash
go test -tags headless ./...
```

## 15.3 Audio Backends

### Oto (Primary)

Cross-platform audio output with:
- Low-latency playback (~20ms)
- Automatic sample rate conversion
- 44.1kHz stereo output

## 15.4 Runtime UI

The runtime is display-only and uses the Ebiten output window directly.
There is no separate control window frontend.

# 16. Running Demonstrations

The Intuition Engine includes visual and audio demonstrations that showcase system capabilities.

**Tutorial:** For a hands-on guide to building a complete demoscene-style demo (blitter sprites, copper effects, PSG+ music, scrolltext), see **[TUTORIAL.md](docs/TUTORIAL.md)**. It includes full implementations for multiple CPU architectures.

## 16.1 Quick Start

Run all short tests:
```bash
go test -v
```

Run with headless mode (no GUI/audio):
```bash
go test -v -tags headless
```

## 16.2 Audio Demonstrations

Long-running audio demos require the `audiolong` tag:

```bash
# Sine wave generation
go test -v -tags audiolong -run TestSineWave_BasicWaveforms

# Square wave with PWM
go test -v -tags audiolong -run TestSquareWave_PWM

# Filter sweep demonstration
go test -v -tags audiolong -run TestFilterSweep
```

## 16.3 Visual Demonstrations

Long-running visual demos require the `videolong` tag:

```bash
# Fire effect (cellular automata)
go test -v -tags videolong -run TestFireEffect

# Plasma waves
go test -v -tags videolong -run TestPlasmaWaves

# 3D starfield
go test -v -tags videolong -run TestStarfield

# Rotating cube
go test -v -tags videolong -run TestRotatingCube

# Mandelbrot set
go test -v -tags videolong -run TestMandelbrot
```

## 16.4 CPU Test Suites

### M68K Tests
```bash
go test -v -tags m68k ./...
```

### 6502 Klaus Functional Tests
```bash
KLAUS_FUNCTIONAL=1 KLAUS_INTERRUPT_SUCCESS_PC=0x06F5 go test -v -run '^Test6502'
```

### Z80 Tests
```bash
go test -v -run TestZ80
```

## 16.5 Available Demonstrations

| Category | Test Name | Description |
|----------|-----------|-------------|
| Audio | TestSineWave_BasicWaveforms | Pure sine wave generation |
| Audio | TestSquareWave_DutyCycle | Variable duty cycle |
| Audio | TestNoiseTypes | White, periodic, metallic noise |
| Audio | TestADSR_Envelope | Envelope timing accuracy |
| Audio | TestFilterModes | LP/HP/BP filter demonstration |
| Video | TestFireEffect | Cellular automata fire |
| Video | TestPlasmaWaves | Dynamic colour plasma |
| Video | TestMetaballs | Organic blob rendering |
| Video | TestTunnelEffect | Texture-mapped tunnel |
| Video | TestRotozoom | Rotation and zoom effect |
| Video | TestStarfield | 3D star simulation |
| Video | TestMandelbrot | Fractal visualisation |
| Video | TestParticles | Physics-based particles |
| Video | rotozoomer_ie64 | IE64 rotozoomer with blitter, copper, and fixed-point math |

# 17. Building from Source

## 17.1 Prerequisites

- Go 1.26 or later
- `sstrip` and `upx` (for binary optimization; modify Makefile to skip if unavailable)
- No extra system packages are required for the default runtime window/audio path.

Optional advanced features may still use CGO/toolchain libraries:
- Voodoo Vulkan path: requires Vulkan-capable system/driver/toolchain support.
- Linux LHA decompression path: uses `liblhasa` (see `lhasa_linux.go`).

**macOS:**
```bash
# No extra system packages required for the runtime window path.
```

## 17.2 Build Commands

```bash
# Build everything (VM and assembler)
make

# Build only the VM
make intuition-engine

# Build only the IE32 assembler
make ie32asm

# Build only the IE64 assembler
make ie64asm

# Build with embedded EhBASIC interpreter
make basic

# Build without Vulkan (software Voodoo only)
make novulkan

# Build without display/audio (CI/testing)
make headless

# Fully portable CGO_ENABLED=0 build (cross-compile safe)
make headless-novulkan

# Install to /usr/local/bin
make install

# Create Linux AppImage
make appimage

# Clean build artifacts
make clean
```

## 17.3 Build Tags

| Tag | Effect |
|-----|--------|
| `headless` | Disable GUI/audio/video backends |
| `novulkan` | Disable Vulkan backend (software Voodoo only) |
| `embed_basic` | Embed assembled EhBASIC binary for `-basic` flag |
| `m68k` | Enable M68K-specific tests |
| `audiolong` | Enable long-running audio demos |
| `videolong` | Enable long-running video demos |

## 17.4 Development Workflow

1. Edit source files
2. Run `make` to build
3. Test with `go test -v`
4. Run demos to verify changes
5. Use `./bin/IntuitionEngine -ie32 program.iex` to test programs

## 17.5 Creating New Demonstrations

When adding new test demonstrations:

1. Use descriptive names that indicate what capability is being showcased. The demonstration should tell a story about the system's capabilities.

2. Include detailed logging that explains:
    - What effects or sounds the user should expect
    - The technical aspects being demonstrated
    - Any interesting parameters or variations being shown

3. Structure demonstrations to:
    - Start with basic capabilities
    - Progress to more complex effects
    - Show interesting combinations of features
    - Clean up resources properly when complete

4. Add informative comments that explain:
    - How the effects are achieved
    - Key algorithms and techniques being used
    - Important implementation details

# 18. SDK Developer Package

The `sdk/` directory contains a curated developer package with example programs, include files, and build scripts for all supported CPU architectures. See [sdk/README.md](sdk/README.md) for the full SDK documentation, including:

- Ready-to-build example programs for IE32, IE64, M68K, 6502, and Z80
- Reusable include files (register definitions, macros, helper routines)
- Build scripts and Makefiles for each architecture
- SID multi-chip support details and VGM chip coverage matrix
