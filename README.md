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

**See also: [TUTORIAL.md](TUTORIAL.md)** - Step-by-step guide to building a complete demoscene intro with all four CPU architectures.

# Table of Contents

1. [System Overview](#1-system-overview)
   - CPU Options
   - Audio Capabilities
   - Video System
   - Quick Start
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
9. [Assembly Language Reference](#9-assembly-language-reference)
   - 9.1 Basic Program Structure
   - 9.2 Assembler Directives
   - 9.3 Memory Access Patterns
   - 9.4 Stack Usage
   - 9.5 Interrupt Handlers
10. [Sound System](#10-sound-system)
    - Custom Audio Chip Overview
    - 10.1 Sound Channel Types
    - 10.2 Modulation System
    - 10.3 Global Effects
    - 10.4 PSG Sound Chip (AY-3-8910/YM2149)
    - 10.5 POKEY Sound Chip
    - 10.6 SID Sound Chip
    - 10.7 TED Sound Chip
    - 10.8 AHX Sound Chip
11. [Video System](#11-video-system)
    - 11.1 Display Modes
    - 11.2 Framebuffer Organisation
    - 11.3 Dirty Rectangle Tracking
    - 11.4 Double Buffering and VBlank Synchronisation
    - 11.5 Direct VRAM Access Mode
    - 11.6 Copper List Executor
    - 11.7 DMA Blitter
    - 11.8 Raster Band Fill
    - 11.9 Video Compositor
12. [Developer's Guide](#12-developers-guide)
    - 12.1 Development Environment Setup
    - 12.2 Building the System
    - 12.3 Development Workflow
    - 12.4 Assembler Include Files
    - 12.5 Debugging Techniques
13. [Implementation Details](#13-implementation-details)
    - 13.1 CPU Emulation
    - 13.2 Memory Architecture
    - 13.3 Audio System Architecture
14. [Platform Support](#14-platform-support)
    - 14.1 Supported Platforms
    - 14.2 Graphics Backends
    - 14.3 Audio Backends
    - 14.4 GUI Frontends
15. [Running Demonstrations](#15-running-demonstrations)
    - 15.1 Quick Start
    - 15.2 Audio Demonstrations
    - 15.3 Visual Demonstrations
    - 15.4 CPU Test Suites
    - 15.5 Available Demonstrations
16. [Building from Source](#16-building-from-source)
    - 16.1 Prerequisites
    - 16.2 Build Commands
    - 16.3 Build Tags
    - 16.4 Development Workflow
    - 16.5 Creating New Demonstrations

# 1. System Overview

The Intuition Engine is a virtual machine that emulates a complete retro-style computer system. It provides a platform for learning assembly language, developing demoscene-style effects, and playing classic music formats from the 8-bit and 16-bit era.

## CPU Options

| CPU | Architecture | Registers | Features |
|-----|--------------|-----------|----------|
| **IE32** | 32-bit RISC | 16 general-purpose (A-H, S-W, X-Z) | Fixed 8-byte instructions, simple and fast |
| **M68K** | 32-bit CISC | 8 data (D0-D7), 8 address (A0-A7) | 95%+ instruction coverage, FPU support |
| **Z80** | 8-bit | AF, BC, DE, HL + alternates, IX, IY | Full instruction set, interrupt modes |
| **6502** | 8-bit | A, X, Y | NMOS instruction set, zero page optimisation |
| **x86** | 32-bit | EAX-EDX, ESI, EDI, EBP, ESP | 8086 base + 386 extensions, flat memory model |

## Audio Capabilities

**Custom Synthesizer:**
- 5 dedicated waveform channels (square, triangle, sine, noise, sawtooth)
- 4 flexible channels with selectable waveforms
- ADSR envelopes, PWM, frequency sweep, hard sync, ring modulation
- Global filter (LP/HP/BP), overdrive, reverb
- 44.1kHz, 32-bit floating-point processing

**Classic Sound Chips (register-mapped to custom synth):**
- **PSG** (AY-3-8910/YM2149) - Supports .ym, .ay, .vgm, .sndh playback
- **POKEY** (Atari) - Supports .sap playback
- **SID** (6581/8580) - Supports .sid playback

## Video System

- Resolutions: 640×480, 800×600, 1024×768
- 32-bit RGBA framebuffer with double buffering
- Copper coprocessor for raster effects
- DMA blitter for fast copy/fill/line operations
- Dirty rectangle tracking for efficient updates

## Quick Start

```bash
# Run IE32 program
./bin/IntuitionEngine -ie32 program.iex

# Run M68K program
./bin/IntuitionEngine -m68k program.ie68

# Run x86 program
./bin/IntuitionEngine -x86 program.ie86

# Play PSG music
./bin/IntuitionEngine -psg music.ym

# Play SID music
./bin/IntuitionEngine -sid music.sid
```

# 2. Architecture

## 2.1 Unified Memory

All CPU cores (IE32, M68K, Z80, 6502) share the same memory space through the SystemBus. This unified architecture ensures that:

- **Program data** loaded by any CPU is immediately visible to all peripherals
- **Audio synthesis** responds instantly to register writes from any CPU
- **DMA operations** (blitter, copper, file players) can access any memory location
- **Memory-mapped I/O** works consistently across all CPU types
- **Video compositing** blends multiple video sources (VideoChip, VGA) into final output

```
┌─────────────────────────────────────────────────────────────────┐
│                        SystemBus Memory                         │
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
   │  M68K/  │            │  VideoChip     │     │  Custom Synth  │
   │  Z80/   │            │  VGA Engine    │     │  PSG/POKEY/SID │
   │  6502)  │            │  Compositor    │     │  File Players  │
   └─────────┘            │  Blitter/Copper│     └────────────────┘
                          └────────────────┘
```

The video compositor blends output from the VideoChip (layer 0) and VGA (layer 10) into a single display, enabling mixed-mode effects. The copper coprocessor can target both video systems via SETBASE for per-scanline raster effects.

The custom audio synthesizer is the core of the sound system. PSG, POKEY, and SID registers are mapped to the custom synth, providing authentic register-level compatibility with high-quality 44.1kHz output. File players (.ym, .ay, .vgm/vgz, .sid, .sap, etc.) execute embedded CPU code that writes to these mapped registers.

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

Additionally, VGA uses legacy PC-compatible memory windows:
- `$A0000-$AFFFF`: VGA VRAM (64KB graphics memory)
- `$B8000-$BFFFF`: VGA Text Buffer (32KB, char+attr pairs)

For 8-bit CPUs (Z80, 6502), addresses are mapped to the 16-bit range `$F000-$FFFF` or accessed via I/O ports.

# 3. Memory Map & Hardware Registers (Detailed)

The system's memory layout is designed to provide efficient access to both program space and hardware features while maintaining clear separation between different memory regions.

**Address Notation:** This document uses both `0x` prefix (C-style) and `$` prefix (assembly-style) for hexadecimal addresses. The notation varies to match each CPU's assembly dialect: `0x` for IE32/general discussion, `$` for M68K and 6502 assembly examples.

## Memory Map Overview

```
0x000000 - 0x000FFF: System vectors (including interrupt vector)
0x001000 - 0x0EFFFF: Program space
0x0F0000 - 0x0F0058: Video registers (copper, blitter, raster control)
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
0x0A0000 - 0x0AFFFF: VGA VRAM window (Mode 13h/12h)
0x0B8000 - 0x0BFFFF: VGA text buffer
0x100000 - 0x4FFFFF: Video RAM (VRAM_START to VRAM_START + VRAM_SIZE)
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

## 3.3 Video Registers (0x0F0000 - 0x0F0058)

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
0x0F0020: BLT_OP       - Blitter op (copy/fill/line/masked copy)
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

Available Video Modes:
MODE_640x480  = 0x00
MODE_800x600  = 0x01
MODE_1024x768 = 0x02
```

**Video Compositor:** These registers control the VideoChip, which renders as layer 0 in the compositor. The VGA chip (section 3.11) renders as layer 10 on top. Both sources are composited together for final display output. The copper coprocessor can write to either device using the SETBASE instruction (see section 10.6).

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
- IE32/M68K: Direct access at 0x0F0F20-0x0F0F5F
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
- Soft low-pass filtering (alpha 0.11)
- Subtle saturation (drive 0.16) for analog warmth
- Room reverb (mix 0.09, delay 120 samples)
- Authentic Amiga stereo panning (L-R-R-L pattern)
- Hardware PWM mapping SquarePos to duty cycle

## 3.12 Hardware I/O Memory Map by CPU

All sound and video chips are accessible from all four CPU architectures at different address ranges:

### Sound Chips

| Chip  | IE32/M68K         | Z80 Ports | 6502        | Notes |
|-------|-------------------|-----------|-------------|-------|
| PSG   | 0x0F0C00-0x0F0C0D | 0xF0-0xF1 | $D400-$D40D | AY-3-8910/YM2149 compatible |
| POKEY | 0x0F0D00-0x0F0D09 | 0xD0-0xD1 | $D200-$D209 | Atari 8-bit compatible |
| SID   | 0x0F0E00-0x0F0E1C | 0xE0-0xE1 | $D500-$D51C | MOS 6581/8580 compatible |
| TED   | 0x0F0F00-0x0F0F05 | 0xF2-0xF3 | $D600-$D605 | Plus/4 compatible |
| AHX   | 0x0F0B80-0x0F0B91 | —         | $FB80-$FB91 | Amiga AHX/THX module player |

### Video Chips

| Chip      | IE32/M68K         | Z80 Ports | 6502        | Notes |
|-----------|-------------------|-----------|-------------|-------|
| VideoChip | 0x0F0000-0x0F0058 | 0xF000+   | $F000-$F058 | Custom copper/blitter |
| TED Video | 0x0F0F20-0x0F0F5F | 0xF2-0xF3 | $D620-$D62F | Plus/4 compatible (idx 0x20-0x2F) |
| VGA       | 0x0F1000-0x0F13FF | 0xA0-0xAC | $D700-$D70A | IBM VGA compatible |
| ULA       | 0x0F2000-0x0F200B | 0xFE      | $D800-$D80B | ZX Spectrum compatible |

### Access Methods

**Z80 Port I/O:** The first port selects the register index, the second reads/writes data.
Example: `OUT (0xF0),A` selects PSG register, `OUT (0xF1),A` writes data.

**6502 Memory-Mapped:** Direct memory access following C64/Atari/Plus4 conventions.
Example: `STA $D400` writes to PSG register 0.

**IE32/M68K Direct:** Full 32-bit address space access.
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
| IE32/M68K | 0x0F1000-0x0F13FF | 0x0A0000-0x0AFFFF | Memory-mapped |
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

See section 10.9 (Video Compositor) for details on the compositing architecture and section 10.6 (Copper List Executor) for examples of copper-driven VGA palette manipulation.

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
| IE32/M68K | 0x0F2000-0x0F200B | 0x4000-0x5AFF | Direct 32-bit |
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

The Intuition Engine includes an x86 core implementing the 8086 instruction set with 386 32-bit extensions, using a simplified flat memory model.

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

The x86 core implements the 8086 instruction set plus 386 32-bit extensions:

**Data Transfer:** MOV, PUSH, POP, XCHG, LEA, LES, LDS
**Arithmetic:** ADD, ADC, SUB, SBB, MUL, IMUL, DIV, IDIV, INC, DEC, CMP, NEG
**Logical:** AND, OR, XOR, NOT, TEST
**Shift/Rotate:** SHL, SHR, SAL, SAR, ROL, ROR, RCL, RCR
**Control Flow:** JMP, Jcc, CALL, RET, LOOP, LOOPE, LOOPNE
**String:** MOVS, STOS, LODS, CMPS, SCAS with REP/REPE/REPNE prefixes
**I/O:** IN, OUT (port-based I/O for audio chips)
**Flag Control:** CLC, STC, CMC, CLD, STD, CLI, STI
**BCD:** DAA, DAS, AAA, AAS, AAM, AAD

**386 Extensions:**
- 32-bit register operations (EAX, EBX, etc.)
- Operand size prefix (0x66)
- Address size prefix (0x67)
- SIB byte addressing
- MOVZX, MOVSX
- SETcc instructions
- Bit test: BT, BTS, BTR, BTC, BSF, BSR
- SHLD, SHRD

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
- Flat memory model (segments ignored for addressing)
- Use NASM or FASM for assembly with `ie86.inc` include file
- `--load-addr` sets the load address (default 0x00000000)
- `--entry` sets the entry point (defaults to load address)

**Not Implemented:**
- Protected mode
- Virtual 8086 mode
- Paging
- Task switching
- x87 FPU

# 9. Assembly Language Reference

This section documents the IE32 assembly language used with the `ie32asm` assembler. For 6502, Z80, M68K, and x86 programming, use their respective standard assemblers (ca65, vasmz80_std, vasmm68k_mot, NASM/FASM) with the include files documented in Section 12.4.

The Intuition Engine assembly language provides a straightforward way to program the system while maintaining access to all hardware features.

## 9.1 Basic Program Structure

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

## 9.2 Assembler Directives

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

## 9.3 Memory Access Patterns

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

## 9.4 Stack Usage

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

## 9.5 Interrupt Handlers

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

# 10. Sound System

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

| Register Block | IE32/M68K Address | Z80/6502 Address | Description |
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

## 10.1 Sound Channel Types

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

## 10.2 Modulation System

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

## 10.3 Global Effects

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

## 10.4 PSG Sound Chip (AY-3-8910/YM2149)

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

## 10.5 POKEY Sound Chip

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

## 10.6 SID Sound Chip

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

## 10.7 TED Sound Chip

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

## 10.8 AHX Sound Chip

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

# 11. Video System

The video system provides flexible graphics output through a memory-mapped framebuffer design.

## 11.1 Display Modes

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

## 11.2 Framebuffer Organisation

The framebuffer uses 32-bit RGBA colour format:
- Start address: 0x100000 (VRAM_START)
- Each pixel: 4 bytes (R,G,B,A)
- Linear layout: y * width + x

### Pixel address calculation:

```
Address = 0x100000 + (y * width + x) * 4
```

## 11.3 Dirty Rectangle Tracking

The system tracks changes in 32x32 pixel blocks:
- Automatically marks modified regions
- Updates only changed areas
- Improves rendering performance

## 11.4 Double Buffering and VBlank Synchronisation

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

## 11.5 Direct VRAM Access Mode

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

## 11.6 Copper List Executor

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

## 11.7 DMA Blitter

The DMA blitter performs rectangle copy/fill and line drawing. Registers are written via memory-mapped I/O, and the blitter operates on VRAM addresses (RGBA, 4 bytes/pixel).

**Synchronous Execution**: The blitter runs immediately when `BLT_CTRL` is written with bit0 set. This ensures blitter operations complete before the CPU continues, allowing safe drawing during VBlank without race conditions.

Operations (`BLT_OP`):
- `0`: COPY
- `1`: FILL
- `2`: LINE (coordinates packed into `BLT_SRC`/`BLT_DST`)
- `3`: MASKED COPY (1-bit mask, LSB-first per byte)
- `4`: ALPHA (alpha-aware copy with source alpha blending)

Line coordinates:
- `BLT_SRC`: x0 (low 16 bits), y0 (high 16 bits)
- `BLT_DST`: x1 (low 16 bits), y1 (high 16 bits)

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

## 11.8 Raster Band Fill

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

## 11.9 Video Compositor

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

# 12. Developer's Guide

## 12.1 Development Environment Setup

To develop for the Intuition Engine, you'll need to set up your development environment with several components:

1. Install the Go programming language (version 1.21 or later)
2. Install required development libraries:
   - GTK4 or FLTK development files
   - ALSA development files (Linux only)
   - OpenGL development files (optional)

Create a project directory structure:

```bash
my_project/
├── src/             # Assembly source files
├── bin/             # Compiled binaries
└── tools/           # Development tools
```

## 12.2 Building the System

The build process uses the provided Makefile:

```bash
# Build both VM and assembler
make

# Build only the VM
make intuition-engine

# Build only the IE32 assembler
make ie32asm

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
./bin/ie32asm           # The assembler
```

Available make targets:
```
all              - Build both Intuition Engine and ie32asm (default)
intuition-engine - Build only the Intuition Engine VM
ie32asm          - Build only the IE32 assembler
appimage         - Build AppImage package for Linux distributions
install          - Install binaries to $(INSTALL_BIN_DIR)
uninstall        - Remove installed binaries from $(INSTALL_BIN_DIR)
clean            - Remove all build artifacts
list             - List compiled binaries with sizes
help             - Show this help message
```

## 12.3 Development Workflow

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
These modes provide oversampling, gentle low-pass smoothing, subtle saturation, and a tiny room/width effect for richer sound while preserving pitch and timing. AHX+ additionally provides authentic Amiga stereo panning (L-R-R-L pattern) and hardware PWM for square wave modulation.

## 12.4 Assembler Include Files

The `assembler/` directory provides hardware definition include files for each CPU architecture. These files are essential for writing portable Intuition Engine programs.

| File | CPU | Assembler | Description |
|------|-----|-----------|-------------|
| `ie32.inc` | IE32 | ie32asm | Hardware constants and register definitions |
| `ie68.inc` | M68K | vasmm68k_mot | Hardware constants with M68K macros |
| `ie65.inc` | 6502 | ca65 | Hardware constants, macros, and zero page allocation |
| `ie80.inc` | Z80 | vasmz80_std | Hardware constants with Z80 macros |
| `ie86.inc` | x86 | NASM/FASM | Hardware constants, port I/O, VGA registers |

### Contents Overview

All include files provide:

- **Video Registers**: VIDEO_CTRL, VIDEO_MODE, VIDEO_STATUS, blitter, copper, raster band
- **Audio Registers**: PSG (raw + player), POKEY (raw + SAP player), SID (raw + SID player)
- **Memory Constants**: VRAM_START, SCREEN_W/H, LINE_BYTES
- **Blitter Operations**: BLT_OP_COPY, BLT_OP_FILL, BLT_OP_LINE, BLT_OP_MASKED, BLT_OP_ALPHA
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

## 12.5 Debugging Techniques

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

# 13. Implementation Details

This section describes how the Intuition Engine emulates its hardware components.

## 13.1 CPU Emulation

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

## 13.2 Memory Architecture

### Unified Memory Bus

All CPUs share a unified 32MB address space through the SystemBus:

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

## 13.3 Audio System Architecture

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

# 14. Platform Support

The Intuition Engine supports multiple platforms through abstracted backend systems for graphics, audio, and GUI.

## 14.1 Supported Platforms

| Platform | Graphics | Audio | GUI |
|----------|----------|-------|-----|
| Linux | Ebiten | Oto, ALSA | GTK4, FLTK |
| macOS | Ebiten | Oto | GTK4, FLTK |
| Windows | Ebiten | Oto | GTK4, FLTK |

## 14.2 Graphics Backends

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

## 14.3 Audio Backends

### Oto (Primary)

Cross-platform audio output with:
- Low-latency playback (~20ms)
- Automatic sample rate conversion
- 44.1kHz stereo output

### ALSA (Linux)

Native Linux audio for:
- Lower latency (~10ms)
- Direct hardware access
- System audio integration

## 14.4 GUI Frontends

### GTK4

Modern desktop interface with:
- Native look and feel
- File browser dialogs
- Debug/inspector windows
- Menu bar integration

### FLTK

Lightweight alternative offering:
- Minimal dependencies
- Fast startup
- Basic file selection
- Simple controls

# 15. Running Demonstrations

The Intuition Engine includes visual and audio demonstrations that showcase system capabilities.

**Tutorial:** For a hands-on guide to building a complete demoscene-style demo (blitter sprites, copper effects, PSG+ music, scrolltext), see **[TUTORIAL.md](TUTORIAL.md)**. It includes full implementations for all four CPU architectures.

## 15.1 Quick Start

Run all short tests:
```bash
go test -v
```

Run with headless mode (no GUI/audio):
```bash
go test -v -tags headless
```

## 15.2 Audio Demonstrations

Long-running audio demos require the `audiolong` tag:

```bash
# Sine wave generation
go test -v -tags audiolong -run TestSineWave_BasicWaveforms

# Square wave with PWM
go test -v -tags audiolong -run TestSquareWave_PWM

# Filter sweep demonstration
go test -v -tags audiolong -run TestFilterSweep
```

## 15.3 Visual Demonstrations

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

## 15.4 CPU Test Suites

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

## 15.5 Available Demonstrations

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

# 16. Building from Source

## 16.1 Prerequisites

- Go 1.21 or later
- C compiler (for CGO dependencies)
- `sstrip` and `upx` (for binary optimization; modify Makefile to skip if unavailable)
- Platform-specific libraries:

**Linux (Debian/Ubuntu):**
```bash
sudo apt install libgtk-4-dev libasound2-dev libgl1-mesa-dev xorg-dev
```

**Linux (Fedora):**
```bash
sudo dnf install gtk4-devel alsa-lib-devel mesa-libGL-devel libX11-devel
```

**macOS:**
```bash
brew install gtk4
```

## 16.2 Build Commands

```bash
# Build everything (VM and assembler)
make

# Build only the VM
make intuition-engine

# Build only the IE32 assembler
make ie32asm

# Install to /usr/local/bin
make install

# Create Linux AppImage
make appimage

# Clean build artifacts
make clean
```

## 16.3 Build Tags

| Tag | Effect |
|-----|--------|
| `headless` | Disable GUI/audio/video backends |
| `m68k` | Enable M68K-specific tests |
| `audiolong` | Enable long-running audio demos |
| `videolong` | Enable long-running video demos |

## 16.4 Development Workflow

1. Edit source files
2. Run `make` to build
3. Test with `go test -v`
4. Run demos to verify changes
5. Use `./bin/IntuitionEngine -ie32 program.iex` to test programs

## 16.5 Creating New Demonstrations

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
