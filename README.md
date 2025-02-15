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
## (c) 2024 - 2025 Zayn Otley
## https://github.com/intuitionamiga/IntuitionEngine
## License: GPLv3 or later
## https://www.youtube.com/@IntuitionAmiga/
[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/M4M61AHEFR)

# Table of Contents

1. System Overview
2. Architecture Design
3. Memory Map & Hardware Registers
4. CPU Architecture
5. Assembly Language Reference
6. Sound System
7. Video System
8. Developer's Guide
9. Implementation Details
10. Platform Support & Backend Systems
11. Hardware Interface Architecture

# 1. System Overview

This virtual machine implements a complete computer system with a custom CPU architecture, sound synthesis capabilities, video output, and interrupt handling. The system is designed to be both educational and practical, offering a balance between simplicity and capability.

## Key Features

- 32-bit RISC-like CPU architecture
- 16 general-purpose registers (A through H and S through Y)
- Memory-mapped I/O for peripherals
- Four-channel sound synthesis with advanced features:
    - Multiple waveform types (square, triangle, sine, noise)
    - ADSR envelope system with multiple envelope shapes
    - Ring modulation capabilities
    - PWM for square waves
    - Global filter system with resonance
    - Reverb effects processing
- Configurable video output:
    - Multiple resolution support (640x480, 800x600, 1024x768)
    - Double-buffered output with dirty rectangle tracking
    - 32-bit RGBA color support
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
- For building the system:
    - UPX compression utility (optional) https://github.com/upx/upx
    - SuperStrip utility (optional) https://github.com/aunali1/super-strip

# 2. Architecture Design

## Core Components

The system consists of five main subsystems that work together:

1. CPU Core
    - Implements the IE32 instruction set
    - Manages program execution
    - Hardware interrupt support via vector table
    - Timer system synchronized to audio rate
    - 16 general-purpose registers

2. Sound System
    - Four independent synthesis channels
    - Multiple waveform generators:
        - Square wave with PWM capability
        - Triangle wave
        - Sine wave
        - Noise generator with multiple modes
    - ADSR envelope system with multiple envelope shapes
    - Ring modulation between channels
    - Global filter system with resonance control
    - Reverb effects processing
    - 44.1kHz sample rate

3. Video System
    - Multiple resolution support:
        - 640x480
        - 800x600
        - 1024x768
    - Double-buffered framebuffer
    - 32x32 pixel dirty rectangle tracking
    - 32-bit RGBA color depth
    - Ebiten primary backend
    - OpenGL backend in development

4. GUI System
    - Abstract frontend interface
    - GTK4 implementation
    - FLTK implementation
    - Common event handling
    - File loading dialog support
    - Basic debugging interface

5. Memory Management
    - 16MB address space
    - Memory-mapped I/O
    - Protected memory regions
    - Double-buffered video memory

## Memory Map

The system's memory is organized as follows:

```
0x000000 - 0x000FFF: System vectors (including interrupt vector)
0x001000 - 0x001FFF: Program start
0x100000 - 0x4FFFFF: Video RAM (VRAM_START to VRAM_START + VRAM_SIZE)
0x00F000 - 0x00F008: Video registers
0x00F800 - 0x00F80C: Timer registers
0x00F900 - 0x00F97C: Sound registers
```
Key memory-mapped hardware registers are logically grouped to facilitate system programming and hardware access. Each subsystem has a dedicated register block for configuration and control.

# 3. Memory Map & Hardware Registers (Detailed)

The system's memory layout is designed to provide efficient access to both program space and hardware features while maintaining clear separation between different memory regions.

## System Vector Table (0x000000 - 0x000FFF)
The first 4KB of memory is reserved for system vectors. The most important vector is:

```
0x0000 - 0x0003: Interrupt Service Routine (ISR) vector
```

When interrupts are enabled via the SEI instruction, the CPU reads this vector to determine the ISR address. Programs must initialize this vector before enabling interrupts:

```assembly
.org 0x0000
    .word isr_address    ; Store ISR location in vector
```

## Program Space (0x001000 - onwards)
Programs begin loading at 0x1000, providing:

- Protection of low memory from accidental overwrites
- Clear separation from system areas
- Space for program code and data

## Hardware Registers (0xF000 - 0xF9FF)

### Video Registers (0xF000 - 0xF008)
```
0xF000: VIDEO_CTRL   - Video system control (0 = disabled, 1 = enabled)
0xF004: VIDEO_MODE   - Display mode selection
0xF008: VIDEO_STATUS - Current video status (read-only)

Available Video Modes:
MODE_640x480  = 0x00
MODE_800x600  = 0x01
MODE_1024x768 = 0x02
```

### Timer Registers (0xF800 - 0xF80C)
```
0xF800: TIMER_CTRL   - Timer control (0 = disabled, 1 = enabled)
0xF804: TIMER_COUNT  - Current timer value (decrements automatically)
0xF808: TIMER_PERIOD - Timer reload value
```

The timer generates an interrupt when TIMER_COUNT reaches zero and automatically reloads from TIMER_PERIOD.

### Sound Registers (0xF900 - 0xF97C)

Each sound channel has its own register block. Here's the layout for each channel type:

#### Square Wave Channel (0xF900 - 0xF93F)
```
0xF900: SQUARE_FREQ     - Frequency control
0xF904: SQUARE_VOL      - Volume (0-255)
0xF908: SQUARE_CTRL     - Channel control
0xF90C: SQUARE_DUTY     - Duty cycle control
0xF910: SQUARE_SWEEP    - Frequency sweep control
0xF922: SQUARE_PWM_CTRL - PWM modulation control
0xF930: SQUARE_ATK      - Attack time (ms)
0xF934: SQUARE_DEC      - Decay time (ms)
0xF938: SQUARE_SUS      - Sustain level (0-255)
0xF93C: SQUARE_REL      - Release time (ms)
```

#### Triangle Wave Channel (0xF940 - 0xF97F)
```
0xF940: TRI_FREQ  - Frequency control
0xF944: TRI_VOL   - Volume control
0xF948: TRI_CTRL  - Channel control
0xF960: TRI_ATK   - Attack time
0xF964: TRI_DEC   - Decay time
0xF968: TRI_SUS   - Sustain level
0xF96C: TRI_REL   - Release time
0xF914: TRI_SWEEP - Frequency sweep control
```

#### Sine Wave Channel (0xF980 - 0xF9BF)
```
0xF980: SINE_FREQ  - Frequency control
0xF984: SINE_VOL   - Volume control
0xF988: SINE_CTRL  - Channel control
0xF990: SINE_ATK   - Attack time
0xF994: SINE_DEC   - Decay time
0xF998: SINE_SUS   - Sustain level
0xF99C: SINE_REL   - Release time
0xF918: SINE_SWEEP - Frequency sweep control
```

#### Noise Channel (0xF9C0 - 0xF9FF)
```
0xF9C0: NOISE_FREQ  - Frequency control
0xF9C4: NOISE_VOL   - Volume control
0xF9C8: NOISE_CTRL  - Channel control
0xF9D0: NOISE_ATK   - Attack time
0xF9D4: NOISE_DEC   - Decay time
0xF9D8: NOISE_SUS   - Sustain level
0xF9DC: NOISE_REL   - Release time
0xF9E0: NOISE_MODE  - Noise generation mode
0xF91C: NOISE_SWEEP - Frequency sweep control

Noise Modes:
NOISE_MODE_WHITE    = 0 // Standard LFSR noise
NOISE_MODE_PERIODIC = 1 // Periodic/looping noise
NOISE_MODE_METALLIC = 2 // "Metallic" noise variant
```

#### Channel Modulation Controls (0xFA00 - 0xFA1C)
```
0xFA00: SYNC_SOURCE_CH0 - Sync source for channel 0
0xFA04: SYNC_SOURCE_CH1 - Sync source for channel 1
0xFA08: SYNC_SOURCE_CH2 - Sync source for channel 2
0xFA0C: SYNC_SOURCE_CH3 - Sync source for channel 3

0xFA10: RING_MOD_SOURCE_CH0 - Ring mod source for channel 0
0xFA14: RING_MOD_SOURCE_CH1 - Ring mod source for channel 1
0xFA18: RING_MOD_SOURCE_CH2 - Ring mod source for channel 2
0xFA1C: RING_MOD_SOURCE_CH3 - Ring mod source for channel 3
```

#### Global Sound Effects (0xFA40 - 0xFA54)
```
0xF820: FILTER_CUTOFF     - Filter cutoff frequency (0-255)
0xF824: FILTER_RESONANCE  - Filter resonance (0-255)
0xF828: FILTER_TYPE       - Filter type selection
0xF82C: FILTER_MOD_SOURCE - Filter modulation source
0xF830: FILTER_MOD_AMOUNT - Modulation depth (0-255)

0xFA40: OVERDRIVE_CTRL - Drive amount (0-255)
0xFA50: REVERB_MIX     - Dry/wet mix (0-255)
0xFA54: REVERB_DECAY   - Decay time (0-255)
```

# 4. CPU Architecture

The CPU implements a 32-bit RISC-like architecture with fixed-width instructions and a clean, orthogonal instruction set.

## 4.1 Register Set

The CPU provides 16 general-purpose 32-bit registers organized in two logical banks:

First Bank (A-H):

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

Second Bank (S-Z):

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

## 4.2 Instruction Format

Every instruction is exactly 8 bytes long, providing a consistent and easy-to-decode format:

```
Byte 0: Opcode
Byte 1: Register specifier
Byte 2: Addressing mode
Byte 3: Reserved (must be 0)
Bytes 4-7: 32-bit operand value
```

## 4.3 Addressing Modes

The system supports four addressing modes:

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
   STORE A, @1000     ; Store A's value at memory address 1000
   ```

## 4.4 Instruction Set

The CPU provides these instruction categories:

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

## 4.5 Interrupt Handling

The system implements a simple but effective interrupt system:

1. **Interrupt Vector**
    - Located at address 0x0000
    - Contains the address of the interrupt service routine (ISR)
    - Must be initialized before enabling interrupts

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
   STORE A, @0xF808   ; Write to TIMER_PERIOD
   LOAD A, #1
   STORE A, @0xF800   ; Enable timer
   SEI                ; Enable interrupts
   ```

# 5. Assembly Language Reference

The Intuition Engine assembly language provides a straightforward way to program the system while maintaining access to all hardware features.

## 5.1 Basic Program Structure

Every assembly program follows this basic structure:

```assembly
; Program header with description
; Example: Simple counter program
.equ TIMER_CTRL, 0xF800    ; Define hardware constants
.equ TIMER_PERIOD, 0xF808  ; using symbolic names

start:                     ; Main entry point
    LOAD A, #0             ; Initialize counter
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

## 5.2 Assembler Directives

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
; Example memory organization
.org 0x0000               ; Start at vector table
    .word isr_handler     ; Set up interrupt vector

.org 0x1000               ; Place main program
start:
    JSR init
    SEI
    JMP main
```

## 5.3 Memory Access Patterns

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

## 5.4 Stack Usage

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

## 5.5 Interrupt Handlers

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

# 6. Sound System

The sound system provides sophisticated synthesis capabilities through four independent channels and global effects processing.

## 6.1 Sound Channel Types

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

## 6.2 Modulation System

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

## 6.3 Global Effects

The system provides global audio processing:

### Filter System
- Variable cutoff frequency
- Resonance control
- Multiple filter types:
    - Low-pass
    - High-pass
    - Band-pass
- Modulation support

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

# 7. Video System

The video system provides flexible graphics output through a memory-mapped framebuffer design.

## 7.1 Display Modes

Three resolution modes are available:
- 640x480 (MODE_640x480)
- 800x600 (MODE_800x600)
- 1024x768 (MODE_1024x768)

###Setting display mode:

```assembly
init_display:
    LOAD A, #0          ; MODE_640x480
    STORE A, @VIDEO_MODE
    LOAD A, #1          ; Enable display
    STORE A, @VIDEO_CTRL
    RTS
```

## 7.2 Framebuffer Organization

The framebuffer uses 32-bit RGBA color format:
- Start address: 0x100000 (VRAM_START)
- Each pixel: 4 bytes (R,G,B,A)
- Linear layout: y * width + x

###Pixel address calculation:

```
Address = 0x100000 + (y * width + x) * 4
```

## 7.3 Dirty Rectangle Tracking

The system tracks changes in 32x32 pixel blocks:
- Automatically marks modified regions
- Updates only changed areas
- Improves rendering performance

## 7.4 Double Buffering

Video output uses double buffering to prevent tearing:
- Write to back buffer
- Wait for VSync
- Buffers swap automatically

Example frame rendering:

```assembly
draw_frame:
    JSR clear_buffer    ; Clear back buffer
    JSR draw_graphics   ; Draw new frame
    JSR wait_vsync      ; Wait for sync
    RTS

wait_vsync:
    LOAD A, @VIDEO_STATUS
    AND A, #1           ; Check vsync bit
    JZ A, wait_vsync
    RTS
```

# 8. Developer's Guide

## 8.1 Development Environment Setup

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

## 8.2 Building the System

The build process uses the provided build script:

```bash
# Build both VM and assembler
make
```
This creates:
```
./bin/IntuitionEngine   # The virtual machine
./bin/ie32asm           # The assembler
```
```bash
# Build and install both VM and assembler
make
make install
```
This creates:
```
/usr/local/bin/IntuitionEngine   # The virtual machine
/usr/local/bin/ie32asm           # The assembler
```
```bash
# Build AppImage package
make appimage
```
This creates:
```
./IntuitionEngine-1.0.0-<CPU_ARCH>.AppImage
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

The Makefile handles all necessary compilation flags and optimizations automatically. It uses:
- Compiler optimization flags for performance
- SuperStrip and UPX LZMA compression for binary size reduction
- Parallel compilation where possible
- AppImage packaging for Linux distribution

## 8.3 Development Workflow

A typical development cycle involves:

1. Write assembly code in your preferred text editor
2. Assemble the code:
```bash
./bin/ie32asm program.asm
```
3. Run the resulting program:
```bash
./bin/IntuitionEngine program.iex
```

The assembler provides error messages for common issues like:
- Undefined labels
- Invalid addressing modes
- Misaligned memory access
- Invalid instruction formats

## 8.4 Debugging Techniques

The system provides several debugging methods:

###Register State Display

```assembly
debug_point:
    PUSH A
    STORE A, @0xF700    ; Debug output register
    POP A
    RTS
```

###Memory Inspection
    - Use the debug interface in the GUI
    - Monitor memory-mapped registers
    - Track stack usage

###Hardware State Monitoring
    - Video status register
    - Audio channel states
    - Timer operation

# 9. Implementation Details

## 9.1 CPU Implementation

The CPU implementation prioritizes clarity and correctness:

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

## 9.2 Memory Bus Architecture

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

## 9.3 Sound System Implementation

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

# 10. Platform Support & Backend Systems

## 10.1 Graphics Backend Architecture

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

### 10.1.1 Ebiten Backend

The primary graphics backend uses Ebiten for:

- Cross-platform compatibility
- Hardware acceleration
- Automatic scaling
- VSync support

### 10.1.2 OpenGL Backend (In Development)

The OpenGL backend (when completed) will provide:

- Direct hardware access
- Custom shader support
- Additional texture features
- Platform-specific optimizations

## 10.2 Audio Backend Systems

Audio output supports two backends:

### 10.2.1 Oto Backend

The primary audio backend uses Oto for:

- Cross-platform support
- Low-latency output
- Automatic buffer management
- Sample-accurate timing

### 10.2.2 ALSA Backend

On Linux systems, ALSA provides:

- Native audio support
- Lower latency
- Direct hardware access
- Better integration with system audio

## 10.3 GUI Backend Systems

Two GUI implementations are available:

### 10.3.1 GTK4 Frontend

The GTK4 implementation provides:

- Modern widget toolkit
- Native look and feel
- File dialogs
- Debug interface

### 10.3.2 FLTK Frontend

The FLTK implementation offers:

- Lightweight alternative
- Cross-platform support
- Basic UI functionality
- Simple file selection

# 11. Hardware Interface Architecture

## 11.1 Interface Design

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

## 11.2 Hardware Abstraction

Each hardware component provides:

- Memory-mapped registers
- Status reporting
- Configuration interface
- Event handling

## 11.3 Device Communication

Hardware interaction occurs through:

- Memory-mapped I/O
- Direct register access
- Interrupt system
- Status polling

## 11.4 Future Extensibility

The interface architecture supports:

- New hardware features
- Additional backends
- Extended capabilities
- Platform-specific optimizations
- Platform-specific GUIs

# 12. Testing & Demonstration Framework

The Intuition Engine includes a comprehensive testing and demonstration framework that verifies system functionality while showcasing its capabilities through interactive demos and visual effects.

## 12.1 Testing Architecture

The testing framework is built on Go's native testing package and provides:

- Automated functional verification of all subsystems
- Real-time audio synthesis demonstrations
- Interactive visual effect demonstrations
- Performance benchmarking capabilities
- Cross-platform compatibility testing

## 12.2 Audio Synthesis Testing

### 12.2.1 Basic Waveform Tests
The system verifies the accuracy and quality of fundamental waveform generation:

- Square wave synthesis with variable duty cycle control
- Triangle wave generation with pristine harmonic content
- Pure sine wave generation with perfect frequency accuracy
- Multiple noise generation algorithms (white, periodic, metallic)

### 12.2.2 Advanced Synthesis Features
Comprehensive testing of advanced sound synthesis capabilities:

- PWM (Pulse Width Modulation) with dynamic width control
- Frequency sweep effects with configurable parameters
- Ring modulation between multiple oscillators
- Hard sync effects across oscillator channels
- Complex noise shaping and filtering

### 12.2.3 Envelope System
Verification of the ADSR (Attack, Decay, Sustain, Release) envelope system:

- Precise timing accuracy for all envelope stages
- Linear and exponential envelope shapes
- Complex envelope interactions with modulation
- Multi-channel envelope synchronization

### 12.2.4 Audio Effects Processing
Testing of the global audio effects processing chain:

- Multi-mode filter system with resonance control
- Overdrive and saturation effects
- Stereo reverb processing
- Cross-modulation effects between channels

## 12.3 Visual System Testing

### 12.3.1 Fundamental Operations
Basic video system functionality verification:

- Resolution mode switching (640x480, 800x600, 1024x768)
- Frame buffer operations and memory access
- Color depth and format handling
- VSync and timing verification

### 12.3.2 Visual Effect Demonstrations
The test suite includes several real-time visual demonstrations:

1. **Color Palette Test**
    - Full RGB color space visualization
    - Color gradient accuracy verification
    - Alpha channel blending tests

2. **3D Graphics**
    - Rotating wireframe cube demonstration
    - 3D perspective projection
    - Real-time rotation and transformation

3. **Particle Systems**
    - Dynamic particle emission and physics
    - Color and alpha blending
    - Performance optimization testing

4. **Special Effects**
    - Fire simulation using cellular automata
    - Plasma wave generation
    - Metaball rendering system
    - Scrolling sine-wave text effects
    - Real-time tunnel effect
    - Rotozoom transformation
    - 3D starfield simulation
    - Mandelbrot set visualization

### 12.3.3 Performance Testing
- Frame rate monitoring and performance profiling
- Memory bandwidth utilization measurement
- CPU load analysis during complex effects
- Optimization verification for critical paths

## 12.4 Integration Testing

The framework includes tests that verify the interaction between different subsystems:

- Audio-visual synchronization
- Interrupt handling and timing accuracy
- Memory access patterns and conflicts
- Resource sharing and management

## 12.5 Technical Demonstrations

The system uses Go's testing framework as a convenient way to organise and run technical demonstrations. These are not traditional unit tests, but rather interactive demonstrations that showcase the system's capabilities.

To run the demonstrations:

```bash
go test -v
```

To run a specific demonstration:

```bash
go test -v -run TestNameOfDemo
```

For example:

###Demonstrate pure sine wave generation with zero harmonic distortion
```bash
go test -v -run TestSineWave_BasicWaveforms
```
###Show dynamic fire simulation using cellular automata

```bash
go test -v -run TestFireEffect
```
###Show real-time plasma wave generation with dynamic colour patterns
```bash
go test -v -run TestPlasmaWaves
```

Each demonstration includes thorough logging output that explains what is being demonstrated and what effects or sounds you should observe. The demonstrations typically run for a set duration (ranging from 2 to 10 seconds) before automatically proceeding to the next test.

## 12.6 Demonstration Development

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