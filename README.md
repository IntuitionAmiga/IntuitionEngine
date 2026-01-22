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

# Table of Contents

1. [System Overview](#1-system-overview)
2. [Architecture Design](#2-architecture-design)
   - 2.1 Core Components
   - 2.2 Unified Memory Architecture
3. [Memory Map & Hardware Registers](#3-memory-map--hardware-registers-detailed)
   - 3.1 System Vector Table
   - 3.2 Program Space
   - 3.3 Video Registers
   - 3.4 Timer Registers
   - 3.5 Sound Registers (Legacy and Flexible)
   - 3.6 PSG Registers
   - 3.7 POKEY Registers
   - 3.8 SID Registers
   - 3.9 Audio Chip Memory Map by CPU
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
8. [Assembly Language Reference](#8-assembly-language-reference)
   - 8.1 Basic Program Structure
   - 8.2 Assembler Directives
   - 8.3 Memory Access Patterns
   - 8.4 Stack Usage
   - 8.5 Interrupt Handlers
9. [Sound System](#9-sound-system)
   - 9.1 Sound Channel Types
   - 9.2 Modulation System
   - 9.3 Global Effects
   - 9.4 PSG Sound Chip (AY-3-8910/YM2149)
   - 9.5 POKEY Sound Chip
   - 9.6 SID Sound Chip
10. [Video System](#10-video-system)
    - 10.1 Display Modes
    - 10.2 Framebuffer Organisation
    - 10.3 Dirty Rectangle Tracking
    - 10.4 Double Buffering and VBlank Synchronisation
    - 10.5 Direct VRAM Access Mode
    - 10.6 Copper List Executor
    - 10.7 DMA Blitter
    - 10.8 Raster Band Fill
11. [Developer's Guide](#11-developers-guide)
    - 11.1 Development Environment Setup
    - 11.2 Building the System
    - 11.3 Development Workflow
    - 11.4 Assembler Include Files
    - 11.5 Debugging Techniques
12. [Implementation Details](#12-implementation-details)
    - 12.1 CPU Implementation
    - 12.2 Memory Bus Architecture
    - 12.3 Sound System Implementation
13. [Platform Support & Backend Systems](#13-platform-support--backend-systems)
    - 13.1 Graphics Backend Architecture
    - 13.2 Audio Backend Systems
    - 13.3 GUI Backend Systems
14. [Hardware Interface Architecture](#14-hardware-interface-architecture)
    - 14.1 Interface Design
    - 14.2 Hardware Abstraction
    - 14.3 Device Communication
    - 14.4 Future Extensibility
15. [Testing & Demonstration Framework](#15-testing--demonstration-framework)
    - 15.1 Testing Architecture
    - 15.2 Audio Synthesis Testing
    - 15.3 Visual System Testing
    - 15.4 Integration Testing
    - 15.5 Technical Demonstrations
    - 15.6 Demonstration Development

# 1. System Overview

This virtual machine implements a complete computer system with multiple CPU architectures, sound synthesis capabilities, video output, and interrupt handling. The system is designed to be both educational and practical, offering a balance between simplicity and capability.

## Key Features

### CPU Options:
- **IE32**: 32-bit RISC-like CPU architecture with 16 general-purpose registers
- **MOS 6502 (NMOS)**: 8-bit CPU emulator for raw binaries (no 65C02 opcodes)
- **Zilog Z80**: 8-bit CPU core with full instruction set
- **Motorola 68020**: Full 32-bit CISC emulation with 95%+ instruction coverage
- **68881/68882 FPU**: Complete floating-point coprocessor with transcendental functions

### Audio Chip Emulation:
- **AY-3-8910/YM2149**: PSG sound chip with PSG+ enhanced audio mode
- **POKEY**: Atari 8-bit sound chip with POKEY+ enhanced audio mode
- **SID (MOS 6581/8580)**: Commodore 64 sound chip with SID+ enhanced audio mode

### Core Features:
- Memory-mapped I/O for peripherals
- Four-channel sound synthesis with advanced features:
    - Multiple waveform types (square, triangle, sine, noise, sawtooth)
    - polyBLEP anti-aliasing for cleaner high-frequency output
    - ADSR envelope system with multiple envelope shapes
    - Ring modulation capabilities
    - PWM for square waves
    - Global filter system with exponential cutoff mapping (20Hz-20kHz) and resonance
    - Reverb effects processing
- Configurable video output:
    - Multiple resolution support (640x480, 800x600, 1024x768)
    - Double-buffered output with dirty rectangle tracking
    - 32-bit RGBA colour support
- Hardware timer with interrupt support
- Dual GUI frontend support (GTK4 and FLTK)

## System Requirements

The VM is implemented in Go and requires:

- Go 1.21 or later
- One of the supported GUI toolkits:
    - GTK4 development libraries
    - FLTK development libraries
- Audio support:
    - Linux: ALSA development libraries
    - All platforms: Oto audio library (primary backend)
- For video output:
    - Ebiten graphics library (primary backend)
    - X11 client-side library (development headers)
    - X11 XFree86 video mode extension library (development headers)
    - OpenGL development libraries (optional/in development)
- For LHA archive support (Linux):
    - lhasa development libraries
- For building the system:
    - UPX compression utility (optional) https://github.com/upx/upx
    - SuperStrip utility (optional) https://github.com/aunali1/super-strip

# 2. Architecture Design

## 2.1 Core Components

The system consists of five main subsystems that work together:

1. **CPU Core**
    - Supports four CPU architectures (IE32, 6502, Z80, 68020)
    - Manages program execution
    - Hardware interrupt support via vector table
    - Timer system synchronised to audio rate

2. **Sound System**
    - Four independent synthesis channels
    - Multiple waveform generators:
        - Square wave with PWM capability and polyBLEP anti-aliasing
        - Triangle wave
        - Sine wave
        - Noise generator with multiple modes
        - Sawtooth wave with polyBLEP anti-aliasing
    - ADSR envelope system with multiple envelope shapes
    - Ring modulation between channels
    - Global filter system with exponential cutoff mapping and resonance control
    - Reverb effects processing
    - 44.1kHz sample rate

3. **Video System**
    - Multiple resolution support:
        - 640x480
        - 800x600
        - 1024x768
    - Double-buffered framebuffer
    - 32x32 pixel dirty rectangle tracking
    - 32-bit RGBA colour depth
    - Copper list executor for mid-frame register updates
    - DMA blitter for copy/fill/line operations
    - Ebiten primary backend
    - OpenGL backend in development

4. **GUI System**
    - Abstract frontend interface
    - GTK4 implementation
    - FLTK implementation
    - Common event handling
    - File loading dialog support
    - Basic debugging interface

5. **Memory Management**
    - 32MB address space
    - Memory-mapped I/O
    - Protected memory regions
    - Double-buffered video memory

## 2.2 Unified Memory Architecture

All CPU cores (IE32, M68K, Z80, 6502) share the same memory space through the SystemBus. This unified architecture ensures that:

- **Program data** loaded by any CPU is immediately visible to all peripherals
- **DMA operations** (blitter, copper, PSG/POKEY/SID streaming) can access any memory location
- **Memory-mapped I/O** works consistently across all CPU types

```
┌─────────────────────────────────────────────────────────────────┐
│                        SystemBus Memory                         │
│                          (32MB Shared)                          │
├─────────────────────────────────────────────────────────────────┤
│  0x000000 - 0x000FFF  │  System Vectors                         │
│  0x001000 - 0x0EFFFF  │  Program Space (code + data)            │
│  0x0F0000 - 0x0FFFFF  │  Hardware I/O Registers                 │
│  0x100000 - 0x4FFFFF  │  Video RAM (4MB)                        │
│  0x500000 - 0x1FFFFFF │  Extended RAM                           │
└─────────────────────────────────────────────────────────────────┘
        │                       │                      │
        ▼                       ▼                      ▼
   ┌─────────┐            ┌──────────┐           ┌────────────────┐
   │   CPU   │            │  Blitter │           │   Audio DMA    │
   │ (IE32/  │            │  Copper  │           │ PSG/POKEY/SID  │
   │  M68K/  │            │          │           │   Players      │
   │  Z80/   │            └──────────┘           └────────────────┘
   │  6502)  │
   └─────────┘
```

When a program is loaded, its embedded data (sprites, copper lists, audio files) is placed in shared memory and immediately accessible to hardware DMA engines.

# 3. Memory Map & Hardware Registers (Detailed)

The system's memory layout is designed to provide efficient access to both program space and hardware features while maintaining clear separation between different memory regions.

## Memory Map Overview

```
0x000000 - 0x000FFF: System vectors (including interrupt vector)
0x001000 - 0x0EFFFF: Program space
0x0F0000 - 0x0F0058: Video registers (copper, blitter, raster control)
0x0F0800 - 0x0F080C: Timer registers
0x0F0820 - 0x0F0834: Filter registers
0x0F0900 - 0x0F0A6F: Legacy synth registers (square/triangle/sine/noise/saw)
0x0F0A80 - 0x0F0B3F: Flexible 4-channel synth registers (preferred)
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
0x0F0900: SQUARE_FREQ     - Frequency control
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
0x0F0940: TRI_FREQ  - Frequency control
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
0x0F0980: SINE_FREQ  - Frequency control
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
0x0F09C0: NOISE_FREQ  - Frequency control
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
0x0F0A20: SAW_FREQ  - Frequency control
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

### Flexible 4-Channel Synth Block (0x0F0A80 - 0x0F0B3F)

The flexible synth block provides a modern, uniform interface for all four synthesis channels. Each channel occupies 0x30 bytes (48 bytes), supporting any waveform type.

```
Channel Base Addresses:
  Channel 0: 0x0F0A80
  Channel 1: 0x0F0AB0
  Channel 2: 0x0F0AE0
  Channel 3: 0x0F0B10

Per-Channel Register Offsets:
  +0x00: FREQ       - Frequency control (32-bit)
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
LOAD A, #440
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

## 3.9 Audio Chip Memory Map by CPU

The three sound chips (PSG, POKEY, SID) are accessible from all CPU architectures at different address ranges:

| Chip  | IE32/M68K         | Z80 Ports | 6502        |
|-------|-------------------|-----------|-------------|
| PSG   | 0x0F0C00-0x0F0C0D | 0xF0-0xF1 | $D400-$D40D |
| POKEY | 0x0F0D00-0x0F0D09 | 0xD0-0xD1 | $D200-$D209 |
| SID   | 0x0F0E00-0x0F0E1C | 0xE0-0xE1 | $D500-$D51C |

Z80 uses port-based I/O: the first port selects the register, the second reads/writes data.
6502 uses memory-mapped I/O in its native address space, following C64/Atari conventions.

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

# 8. Assembly Language Reference

The Intuition Engine assembly language provides a straightforward way to program the system while maintaining access to all hardware features.

## 8.1 Basic Program Structure

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

## 8.2 Assembler Directives

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

## 8.3 Memory Access Patterns

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

## 8.4 Stack Usage

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

## 8.5 Interrupt Handlers

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

# 9. Sound System

The sound system provides sophisticated synthesis capabilities through four independent channels and global effects processing.

## 9.1 Sound Channel Types

Each channel offers different synthesis capabilities:

### Square Wave Channel

Features:
- Variable duty cycle control
- PWM modulation
- Frequency sweep
- Ring modulation support
- ADSR envelope

Configuration example:
```assembly
setup_square:
    ; Set frequency
    LOAD A, #440        ; Base frequency
    STORE A, @SQUARE_FREQ

    ; Configure PWM
    LOAD A, #128        ; 50% duty cycle
    STORE A, @SQUARE_DUTY
    LOAD A, #1          ; Enable PWM
    STORE A, @SQUARE_PWM_CTRL

    ; Set envelope
    LOAD A, #10         ; 10ms attack
    STORE A, @SQUARE_ATK
    LOAD A, #20         ; 20ms decay
    STORE A, @SQUARE_DEC
    LOAD A, #192        ; 75% sustain
    STORE A, @SQUARE_SUS
    LOAD A, #100        ; 100ms release
    STORE A, @SQUARE_REL
    RTS
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

Configuration example:
```assembly
setup_sawtooth:
    ; Set frequency
    LOAD A, #440        ; Base frequency
    STORE A, @SAW_FREQ

    ; Set volume
    LOAD A, #192        ; 75% volume
    STORE A, @SAW_VOL

    ; Set envelope
    LOAD A, #10         ; 10ms attack
    STORE A, @SAW_ATK
    LOAD A, #20         ; 20ms decay
    STORE A, @SAW_DEC
    LOAD A, #192        ; 75% sustain
    STORE A, @SAW_SUS
    LOAD A, #100        ; 100ms release
    STORE A, @SAW_REL

    ; Enable channel
    LOAD A, #1
    STORE A, @SAW_CTRL
    RTS
```

## 9.2 Modulation System

The sound system supports complex modulation:

### Ring Modulation

Connect channels for amplitude modulation:
```assembly
; Set channel 1 to modulate channel 0
LOAD A, #1             ; Use channel 1 as source
STORE A, @RING_MOD_SOURCE_CH0
```

### Frequency Sweep

Configure automatic frequency changes:
```assembly
; Set up frequency sweep
LOAD A, #0x87          ; Enable sweep up
STORE A, @SQUARE_SWEEP
```

## 9.3 Global Effects

The system provides global audio processing:

### Filter System

- Variable cutoff frequency with exponential mapping (20Hz-20kHz)
- Resonance control
- Multiple filter types:
    - Low-pass
    - High-pass
    - Band-pass
- Modulation support

The filter cutoff uses exponential mapping for more musical control, as human hearing is logarithmic. A cutoff value of 0 maps to 20Hz, while 255 maps to 20kHz.

```assembly
; Configure filter
LOAD A, #128           ; Set cutoff
STORE A, @FILTER_CUTOFF
LOAD A, #64            ; Set resonance
STORE A, @FILTER_RESONANCE
LOAD A, #1             ; Low-pass mode
STORE A, @FILTER_TYPE
```

### Reverb System

- Adjustable mix level
- Variable decay time
- Multiple delay lines

```assembly
; Configure reverb
LOAD A, #128           ; 50% wet/dry
STORE A, @REVERB_MIX
LOAD A, #192           ; Long decay
STORE A, @REVERB_DECAY
```

## 9.4 PSG Sound Chip (AY-3-8910/YM2149)

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

## 9.5 POKEY Sound Chip

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

Configuration example:
```assembly
; Configure POKEY for pure tone on channel 1
LOAD A, #0x50          ; Frequency divider
STORE A, @0xF0D00      ; AUDF1
LOAD A, #0xAF          ; Pure tone + volume 15
STORE A, @0xF0D01      ; AUDC1
```

## 9.6 SID Sound Chip

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

Configuration example:
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

# 10. Video System

The video system provides flexible graphics output through a memory-mapped framebuffer design.

## 10.1 Display Modes

Three resolution modes are available:
- 640x480 (MODE_640x480)
- 800x600 (MODE_800x600)
- 1024x768 (MODE_1024x768)

### Setting display mode:

```assembly
init_display:
    LOAD A, #0          ; MODE_640x480
    STORE A, @VIDEO_MODE
    LOAD A, #1          ; Enable display
    STORE A, @VIDEO_CTRL
    RTS
```

## 10.2 Framebuffer Organisation

The framebuffer uses 32-bit RGBA colour format:
- Start address: 0x100000 (VRAM_START)
- Each pixel: 4 bytes (R,G,B,A)
- Linear layout: y * width + x

### Pixel address calculation:

```
Address = 0x100000 + (y * width + x) * 4
```

## 10.3 Dirty Rectangle Tracking

The system tracks changes in 32x32 pixel blocks:
- Automatically marks modified regions
- Updates only changed areas
- Improves rendering performance

## 10.4 Double Buffering and VBlank Synchronisation

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

; Simple VBlank wait (use when not already in VBlank)
wait_vblank:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JZ A, wait_vblank
    RTS
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

## 10.5 Direct VRAM Access Mode

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

## 10.6 Copper List Executor

The video subsystem includes a simple copper-like list executor for mid-frame register updates. Copper lists are stored in RAM as little-endian 32-bit words and can WAIT on raster positions, MOVE values into video registers, and END the list. The copper restarts each frame while enabled.

Registers:
- `COPPER_CTRL` (0x0F000C): bit0=enable, bit1=reset/rewind
- `COPPER_PTR`  (0x0F0010): list base address (latched on enable/reset)
- `COPPER_PC`   (0x0F0014): current list address (read-only)
- `COPPER_STATUS` (0x0F0018): bit0=running, bit1=waiting, bit2=halted

List words:
- `WAIT`: `(0<<30) | (y<<12) | x`
- `MOVE`: `(1<<30) | (regIndex<<16)` followed by a 32-bit value
- `END`: `(3<<30)`

## 10.7 DMA Blitter

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

Example (fill a 16x16 block):
```assembly
    LOAD A, #1              ; BLT_OP_FILL
    STORE A, @BLT_OP
    LOAD A, #0x100000        ; VRAM_START
    STORE A, @BLT_DST
    LOAD A, #16
    STORE A, @BLT_WIDTH
    LOAD A, #16
    STORE A, @BLT_HEIGHT
    LOAD A, #0xFF00FF00      ; green
    STORE A, @BLT_COLOR
    LOAD A, #1
    STORE A, @BLT_CTRL       ; start
```

The blitter defaults `BLT_SRC_STRIDE`/`BLT_DST_STRIDE` to the current mode row bytes when the address is in VRAM, otherwise it uses `width*4`. If an unaligned VRAM address is used, `BLT_STATUS.bit0` is set.

## 10.8 Raster Band Fill

`VIDEO_RASTER_*` registers draw a full-width horizontal band directly into the framebuffer. This is useful for copper-driven raster bars without adding a palette system.

Example (draw 4-pixel band at Y=100):
```assembly
    LOAD A, #100
    STORE A, @VIDEO_RASTER_Y
    LOAD A, #4
    STORE A, @VIDEO_RASTER_HEIGHT
    LOAD A, #0xFF0000FF
    STORE A, @VIDEO_RASTER_COLOR
    LOAD A, #1
    STORE A, @VIDEO_RASTER_CTRL
```

# 11. Developer's Guide

## 11.1 Development Environment Setup

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

## 11.2 Building the System

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

## 11.3 Development Workflow

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

**Notes:**
- `.ym` files are Atari ST YM format
- `.vgm/.vgz` are VGM streams (including MSX PSG logs)
- `.ay` ZXAYEMUL files with embedded Z80 players are supported
- `.sndh` files are Atari ST SNDH format with embedded M68K code
- PSID only for SID; RSID is rejected
- Single-SID playback at $D400; multi-SID not yet implemented

**Enhanced Audio Modes (PSG+/POKEY+/SID+):**
These modes provide oversampling, gentle low-pass smoothing, subtle saturation, and a tiny room/width effect for richer sound while preserving pitch and timing.

## 11.4 Assembler Include Files

The `assembler/` directory provides hardware definition include files for each CPU architecture:

| File | CPU | Assembler | Description |
|------|-----|-----------|-------------|
| `ie32.inc` | IE32 | ie32asm | All hardware constants for IE32 assembly |
| `ie68.inc` | M68K | vasmm68k_mot | All hardware constants for 68020 assembly |
| `ie65.inc` | 6502 | ca65 | All hardware constants for 6502 assembly |
| `ie80.inc` | Z80 | z80asm | All hardware constants for Z80 assembly |

These files define:
- Memory map addresses (VRAM, bank windows, I/O regions)
- Video registers (VIDEO_CTRL, VIDEO_MODE, VIDEO_STATUS, blitter, copper, raster)
- Audio registers (PSG, POKEY, SID, and player registers)
- Timer registers
- Blitter operations (BLT_OP_COPY, BLT_OP_FILL, BLT_OP_LINE, BLT_OP_MASKED, BLT_OP_ALPHA)
- Copper opcodes (COP_WAIT, COP_MOVE, COP_END)

Usage example (IE32):
```assembly
.include "ie32.inc"

    LDA #1
    STA @VIDEO_CTRL         ; Enable video using defined constant
    LDA #BLT_OP_FILL
    STA @BLT_OP             ; Set blitter to fill mode
```

Usage example (M68K with vasm):
```assembly
    include "ie68.inc"

    move.l  #1,VIDEO_CTRL   ; Enable video
    move.l  #BLT_OP_ALPHA,BLT_OP
```

## 11.5 Debugging Techniques

The system provides several debugging methods:

### Register State Display

```assembly
debug_point:
    PUSH A
    STORE A, @0xF0700   ; Debug output register
    POP A
    RTS
```

### Memory Inspection

- Use the debug interface in the GUI
- Monitor memory-mapped registers
- Track stack usage

### Hardware State Monitoring

- Video status register
- Audio channel states
- Timer operation

# 12. Implementation Details

## 12.1 CPU Implementation

The CPU implementation prioritises clarity and correctness:

```go
type CPU struct {
    // Registers
    A, X, Y, Z uint32
    B, C, D, E uint32
    F, G, H    uint32
    S, T, U    uint32
    V, W       uint32

    // System state
    PC            uint32
    SP            uint32
    Running       bool
    Debug         bool
    InterruptVector  uint32
    InterruptEnabled bool
    InInterrupt     bool

    // System bus interface
    bus MemoryBus
}
```

The instruction execution cycle:

1. Fetch instruction (8 bytes)
2. Decode opcode and addressing mode
3. Execute instruction
4. Update program counter
5. Check for interrupts

## 12.2 Memory Bus Architecture

The memory bus provides a flexible interface for memory access:

```go
type MemoryBus interface {
    Read32(addr uint32) uint32
    Write32(addr uint32, value uint32)
    Reset()
}
```

Memory operations handle:

- Memory-mapped I/O
- Alignment requirements
- Access protection
- Multiple device mappings

## 12.3 Sound System Implementation

The sound system uses a sophisticated multi-channel architecture:

```go
type Channel struct {
    waveType      int
    frequency     float32
    volume        float32
    enabled       bool
    phase         float32
    envelopePhase int
    envelopeLevel float32

    // Advanced features
    dutyCycle     float32
    pwmEnabled    bool
    pwmRate       float32
    pwmDepth      float32
    pwmPhase      float32

    // Modulation
    ringModulate  bool
    ringModSource *Channel
    prevRawSample float32
}
```

Audio processing occurs in real-time at 44.1kHz, with features like:

- Sample-accurate timing
- Efficient waveform generation
- Real-time parameter updates
- Multiple effect processors

# 13. Platform Support & Backend Systems

## 13.1 Graphics Backend Architecture

The system supports multiple graphics backends through a common interface:

```go
type VideoOutput interface {
    Start() error
    Stop() error
    Close() error
    IsStarted() bool
    SetDisplayConfig(config DisplayConfig) error
    GetDisplayConfig() DisplayConfig
    UpdateFrame(buffer []byte) error
}
```

### Ebiten Backend

The primary graphics backend uses Ebiten for:

- Cross-platform compatibility
- Hardware acceleration
- Automatic scaling
- VSync support

### OpenGL Backend (In Development)

The OpenGL backend (when completed) will provide:

- Direct hardware access
- Custom shader support
- Additional texture features
- Platform-specific optimisations

## 13.2 Audio Backend Systems

Audio output supports two backends:

### Oto Backend

The primary audio backend uses Oto for:

- Cross-platform support
- Low-latency output
- Automatic buffer management
- Sample-accurate timing

### ALSA Backend

On Linux systems, ALSA provides:

- Native audio support
- Lower latency
- Direct hardware access
- Better integration with system audio

## 13.3 GUI Backend Systems

Two GUI implementations are available:

### GTK4 Frontend

The GTK4 implementation provides:

- Modern widget toolkit
- Native look and feel
- File dialogs
- Debug interface

### FLTK Frontend

The FLTK implementation offers:

- Lightweight alternative
- Cross-platform support
- Basic UI functionality
- Simple file selection

# 14. Hardware Interface Architecture

## 14.1 Interface Design

The system uses a layered interface approach:

```go
// Core interfaces
type MemoryBus interface { ... }
type VideoOutput interface { ... }
type AudioOutput interface { ... }
type GUIFrontend interface { ... }

// Optional enhancement interfaces
type PaletteCapable interface { ... }
type TextureCapable interface { ... }
type SpriteCapable interface { ... }
```

## 14.2 Hardware Abstraction

Each hardware component provides:

- Memory-mapped registers
- Status reporting
- Configuration interface
- Event handling

## 14.3 Device Communication

Hardware interaction occurs through:

- Memory-mapped I/O
- Direct register access
- Interrupt system
- Status polling

## 14.4 Future Extensibility

The interface architecture supports:

- New hardware features
- Additional backends
- Extended capabilities
- Platform-specific optimisations
- Platform-specific GUIs

# 15. Testing & Demonstration Framework

The Intuition Engine includes a comprehensive testing and demonstration framework that verifies system functionality while showcasing its capabilities through interactive demos and visual effects.

## 15.1 Testing Architecture

The testing framework is built on Go's native testing package and provides:

- Automated functional verification of all subsystems
- Real-time audio synthesis demonstrations
- Interactive visual effect demonstrations
- Performance benchmarking capabilities
- Cross-platform compatibility testing

## 15.2 Audio Synthesis Testing

### Basic Waveform Tests

The system verifies the accuracy and quality of fundamental waveform generation:

- Square wave synthesis with variable duty cycle control
- Triangle wave generation with pristine harmonic content
- Pure sine wave generation with perfect frequency accuracy
- Multiple noise generation algorithms (white, periodic, metallic)

### Advanced Synthesis Features

Comprehensive testing of advanced sound synthesis capabilities:

- PWM (Pulse Width Modulation) with dynamic width control
- Frequency sweep effects with configurable parameters
- Ring modulation between multiple oscillators
- Hard sync effects across oscillator channels
- Complex noise shaping and filtering

### Envelope System

Verification of the ADSR (Attack, Decay, Sustain, Release) envelope system:

- Precise timing accuracy for all envelope stages
- Linear and exponential envelope shapes
- Complex envelope interactions with modulation
- Multi-channel envelope synchronisation

### Audio Effects Processing

Testing of the global audio effects processing chain:

- Multi-mode filter system with resonance control
- Overdrive and saturation effects
- Stereo reverb processing
- Cross-modulation effects between channels

## 15.3 Visual System Testing

### Fundamental Operations

Basic video system functionality verification:

- Resolution mode switching (640x480, 800x600, 1024x768)
- Frame buffer operations and memory access
- Colour depth and format handling
- VSync and timing verification

### Visual Effect Demonstrations

The test suite includes several real-time visual demonstrations:

1. **Colour Palette Test**
    - Full RGB colour space visualisation
    - Colour gradient accuracy verification
    - Alpha channel blending tests

2. **3D Graphics**
    - Rotating wireframe cube demonstration
    - 3D perspective projection
    - Real-time rotation and transformation

3. **Particle Systems**
    - Dynamic particle emission and physics
    - Colour and alpha blending
    - Performance optimisation testing

4. **Special Effects**
    - Fire simulation using cellular automata
    - Plasma wave generation
    - Metaball rendering system
    - Scrolling sine-wave text effects
    - Real-time tunnel effect
    - Rotozoom transformation
    - 3D starfield simulation
    - Mandelbrot set visualisation

### Performance Testing

- Frame rate monitoring and performance profiling
- Memory bandwidth utilisation measurement
- CPU load analysis during complex effects
- Optimisation verification for critical paths

## 15.4 Integration Testing

The framework includes tests that verify the interaction between different subsystems:

- Audio-visual synchronisation
- Interrupt handling and timing accuracy
- Memory access patterns and conflicts
- Resource sharing and management

## 15.5 Technical Demonstrations

The system uses Go's testing framework as a convenient way to organise and run technical demonstrations. Some tests are short unit/integration checks, while long-running audio/video demos are gated by build tags.

To run the default test suite:

```bash
go test -v
```

To run a specific audio demonstration (long-running):

```bash
go test -v -tags audiolong -run TestNameOfDemo
```

For example:

### Demonstrate pure sine wave generation with zero harmonic distortion
```bash
go test -v -tags audiolong -run TestSineWave_BasicWaveforms
```

### Show dynamic fire simulation using cellular automata
```bash
go test -v -tags videolong -run TestFireEffect
```

### Show real-time plasma wave generation with dynamic colour patterns
```bash
go test -v -tags videolong -run TestPlasmaWaves
```

Each demonstration includes thorough logging output that explains what is being demonstrated and what effects or sounds you should observe. The demonstrations typically run for a set duration (ranging from 2 to 10 seconds) before automatically proceeding to the next test.

To run the M68K test suite:

```bash
go test -v -tags m68k ./...
```

To run the 6502 Klaus test suite:

```bash
KLAUS_FUNCTIONAL=1 KLAUS_INTERRUPT_SUCCESS_PC=0x06F5 go test -v -run '^Test6502'
```

Use `-tags headless` to skip GUI/audio/video backends when you only need CPU tests or do not have native dependencies installed.

## 15.6 Demonstration Development

When creating new demonstrations:

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
