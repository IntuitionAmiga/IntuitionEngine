# Intuition Engine Tutorial
## Building a Demoscene Intro: Robocop Demo
## (c) 2024-2026 Zayn Otley - GPLv3 or later

This tutorial walks through building a complete demoscene-style intro featuring:
- Animated sprite with masked blitter
- Copper list RGB colour bars
- PSG+ music playback
- Sine-wave scrolling text

The demo is implemented in all four CPU architectures supported by the Intuition Engine: IE32, M68K (68020), Z80, and 6502.

---

# Table of Contents

1. [Introduction](#1-introduction)
2. [Prerequisites](#2-prerequisites)
3. [Part 1: Video Initialization and VBlank Sync](#3-part-1-video-initialization-and-vblank-sync)
4. [Part 2: Blitter Sprite Movement](#4-part-2-blitter-sprite-movement)
5. [Part 3: Copper List RGB Bars](#5-part-3-copper-list-rgb-bars)
6. [Part 4: PSG+ Music Playback](#6-part-4-psg-music-playback)
7. [Part 5: Sine-Wave Scrolltext](#7-part-5-sine-wave-scrolltext)
8. [Complete Source Listings](#8-complete-source-listings)
9. [Building and Running](#9-building-and-running)

---

# 1. Introduction

The Robocop Intro demo showcases the Intuition Engine's hardware capabilities through a classic demoscene-style presentation:

```
┌─────────────────────────────────────────────────────┐
│                                                     │
│     ╔═══════════════════════════════════════╗       │
│     ║   Animated RGB colour bars (copper)   ║       │
│     ╚═══════════════════════════════════════╝       │
│                                                     │
│              ┌─────────────────┐                    │
│              │                 │                    │
│              │    ROBOCOP      │  ← Masked sprite   │
│              │    SPRITE       │    moving in       │
│              │   (240x180)     │    Lissajous       │
│              │                 │    pattern         │
│              └─────────────────┘                    │
│                                                     │
│  ~~~~ HELLO WORLD FROM INTUITION ENGINE ~~~~       │
│     ↑ Sine-wave scrolling text                      │
└─────────────────────────────────────────────────────┘
```

**Features demonstrated:**
- Video mode setup and screen clearing
- VBlank synchronisation for flicker-free animation
- DMA blitter operations (fill, masked copy)
- Copper coprocessor for mid-frame register changes
- PSG+ enhanced audio playback
- Sine lookup tables for smooth animation
- Multi-CPU architecture support

---

# 2. Prerequisites

## Tools Required

| CPU | Assembler | Command |
|-----|-----------|---------|
| IE32 | ie32asm | `./bin/ie32asm program.asm` |
| M68K | vasmm68k_mot | `vasmm68k_mot -Fbin -m68020 -devpac -o out.ie68 program.asm` |
| Z80 | vasmz80_std | `vasmz80_std -Fbin -o out.ie80 program.asm` |
| 6502 | ca65/ld65 | `ca65 program.asm && ld65 -C ie65.cfg -o out.bin program.o` |

## Include Files

Each CPU architecture has a corresponding include file in the `assembler/` directory:

| File | CPU | Contents |
|------|-----|----------|
| `ie32.inc` | IE32 | Hardware constants using `.equ` |
| `ie68.inc` | M68K | Hardware constants and helper macros |
| `ie80.inc` | Z80 | Hardware constants and macros for 32-bit operations |
| `ie65.inc` | 6502 | Hardware constants, macros, and zero page allocation |

These files define all hardware register addresses, blitter operations, copper opcodes, and provide helper macros for common operations.

## Data Files

The demo requires embedded binary data:
- `robocop_sprite.rgba` - 240×180 pixel sprite (172,800 bytes)
- `robocop_mask.bin` - 1-bit mask for the sprite (5,400 bytes)
- `robocop.ay` - AY music file (~24KB)
- `font.rgba` - 32×32 pixel character set
- Pre-calculated sine/cosine tables

---

# 3. Part 1: Video Initialization and VBlank Sync

Every demo starts with setting up the display and establishing a stable frame rate.

## Video Mode Setup

The Intuition Engine supports three resolutions. We'll use 640×480 (mode 0):

**IE32:**
```assembly
    ; Set 640x480 mode
    LDA #0
    STA @VIDEO_MODE
    ; Enable video output
    LDA #1
    STA @VIDEO_CTRL
```

**M68K:**
```assembly
    ; Set 640x480 mode
    moveq   #0,d0
    move.l  d0,VIDEO_MODE
    ; Enable video output
    moveq   #1,d0
    move.l  d0,VIDEO_CTRL
```

**Z80:**
```assembly
    ; Set 640x480 mode (need to write 4 bytes)
    xor  a
    ld   (VIDEO_MODE),a
    ld   (VIDEO_MODE+1),a
    ld   (VIDEO_MODE+2),a
    ld   (VIDEO_MODE+3),a
    ; Enable video output
    ld   a,1
    ld   (VIDEO_CTRL),a
```

**6502:**
```assembly
    ; Set 640x480 mode
    lda  #0
    sta  VIDEO_MODE
    sta  VIDEO_MODE+1
    sta  VIDEO_MODE+2
    sta  VIDEO_MODE+3
    ; Enable video output
    lda  #1
    sta  VIDEO_CTRL
```

## Screen Clearing with Blitter

Use the blitter to fill the screen with a background colour:

**IE32:**
```assembly
    JSR wait_blit           ; Wait for blitter to be ready
    LDA #BLT_OP_FILL
    STA @BLT_OP
    LDA #VRAM_START         ; 0x100000
    STA @BLT_DST
    LDA #SCREEN_W           ; 640
    STA @BLT_WIDTH
    LDA #SCREEN_H           ; 480
    STA @BLT_HEIGHT
    LDA #0xFF000000         ; Black with full alpha (RGBA)
    STA @BLT_COLOR
    LDA #LINE_BYTES         ; 2560 (640 * 4)
    STA @BLT_DST_STRIDE
    LDA #1                  ; Start blitter
    STA @BLT_CTRL
    JSR wait_blit           ; Wait for completion
```

**M68K:**
```assembly
    jsr     wait_blit
    move.l  #BLT_OP_FILL,BLT_OP
    move.l  #VRAM_START,BLT_DST
    move.l  #SCREEN_W,BLT_WIDTH
    move.l  #SCREEN_H,BLT_HEIGHT
    move.l  #$FF000000,BLT_COLOR
    move.l  #LINE_BYTES,BLT_DST_STRIDE
    moveq   #1,d0
    move.l  d0,BLT_CTRL
    jsr     wait_blit
```

## VBlank Synchronisation

For smooth 60 FPS animation, we need to synchronise with the display's vertical blank period. The `wait_frame` routine ensures exactly one frame per loop iteration:

**IE32:**
```assembly
wait_frame:
    ; First wait for VBlank to END (active scan period)
.wait_not_vblank:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK   ; Bit 1 = VBlank flag
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
wait_frame:
.wait_not_vblank:
    move.l  VIDEO_STATUS,d0
    andi.l  #STATUS_VBLANK,d0
    bne     .wait_not_vblank
.wait_vblank_start:
    move.l  VIDEO_STATUS,d0
    andi.l  #STATUS_VBLANK,d0
    beq     .wait_vblank_start
    rts
```

**Z80:**
```assembly
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
.proc wait_frame
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
.endproc
```

## Wait for Blitter

The blitter runs asynchronously. Check the busy flag before starting a new operation:

**IE32:**
```assembly
wait_blit:
    LDA @BLT_CTRL
    AND A, #2               ; Bit 1 = busy
    JNZ A, wait_blit
    RTS
```

**M68K:**
```assembly
wait_blit:
    move.l  BLT_CTRL,d0
    andi.l  #2,d0
    bne     wait_blit
    rts
```

---

# 4. Part 2: Blitter Sprite Movement

The sprite moves in a Lissajous curve pattern using pre-calculated sine and cosine tables.

## Position Calculation

The sprite's X and Y coordinates are calculated from sine/cosine lookup tables indexed by the frame counter:

**IE32:**
```assembly
compute_xy:
    ; X = sin_table[frame & 0xFF] + CENTER_X
    LDB A
    AND B, #0xFF            ; Wrap to 256 entries
    SHL B, #2               ; * 4 bytes per entry
    ADD B, #SIN_X_ADDR      ; Table address
    LDC [B]                 ; Load sine value
    LDE #CENTER_X           ; 200
    ADD C, E                ; C = X position

    ; Y = cos_table[(frame*2) & 0xFF] + CENTER_Y
    LDB A
    SHL B, #1               ; Different frequency for Y
    AND B, #0xFF
    SHL B, #2
    ADD B, #COS_Y_ADDR
    LDD [B]
    LDE #CENTER_Y           ; 150
    ADD D, E                ; D = Y position
    RTS
```

**M68K:**
```assembly
compute_xy:
    ; Input: d0 = frame
    ; Output: d2 = X position, d3 = Y position
    move.l  d0,d1
    andi.l  #$FF,d1
    lsl.l   #2,d1
    lea     data_sin_x,a0
    move.l  (a0,d1.l),d2
    addi.l  #CENTER_X,d2

    move.l  d0,d1
    lsl.l   #1,d1           ; Different frequency for Y
    andi.l  #$FF,d1
    lsl.l   #2,d1
    lea     data_cos_y,a0
    move.l  (a0,d1.l),d3
    addi.l  #CENTER_Y,d3
    rts
```

## VRAM Address Calculation

Convert (X, Y) screen coordinates to a VRAM address:

```
Address = VRAM_START + (Y × LINE_BYTES) + (X × 4)
```

**IE32:**
```assembly
    ; Compute VRAM address from X (in C) and Y (in D)
    LDE D
    LDF #LINE_BYTES         ; 2560
    MUL E, F                ; E = Y * LINE_BYTES
    LDF C
    SHL F, #2               ; F = X * 4
    ADD E, F
    ADD E, #VRAM_START      ; E = final address
```

## Masked Sprite Blit

The blitter's masked copy operation (BLT_OP_MASKED = 3) uses a 1-bit mask to control which pixels are drawn:

**IE32:**
```assembly
    ; Erase previous position first (fill with background)
    JSR wait_blit
    LDA #BLT_OP_FILL
    STA @BLT_OP
    LDA T                   ; T = previous VRAM address
    STA @BLT_DST
    LDA #SPRITE_W
    STA @BLT_WIDTH
    LDA #SPRITE_H
    STA @BLT_HEIGHT
    LDA #BACKGROUND
    STA @BLT_COLOR
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
    JSR wait_blit

    ; Blit sprite with mask at new position
    LDA #BLT_OP_MASKED
    STA @BLT_OP
    LDA #ROBOCOP_RGBA_ADDR  ; Sprite pixel data
    STA @BLT_SRC
    LDA #ROBOCOP_MASK_ADDR  ; 1-bit mask
    STA @BLT_MASK
    LDA U                   ; U = new VRAM address
    STA @BLT_DST
    LDA #SPRITE_W           ; 240
    STA @BLT_WIDTH
    LDA #SPRITE_H           ; 180
    STA @BLT_HEIGHT
    LDA #SPRITE_STRIDE      ; 960 (240 * 4)
    STA @BLT_SRC_STRIDE
    LDA #LINE_BYTES         ; 2560
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
```

**M68K:**
```assembly
    ; Clear previous position
    jsr     wait_blit
    move.l  #BLT_OP_FILL,BLT_OP
    move.l  d6,BLT_DST              ; d6 = previous address
    move.l  #SPRITE_W,BLT_WIDTH
    move.l  #SPRITE_H,BLT_HEIGHT
    move.l  #BACKGROUND,BLT_COLOR
    move.l  #LINE_BYTES,BLT_DST_STRIDE
    moveq   #1,d0
    move.l  d0,BLT_CTRL
    jsr     wait_blit

    ; Masked blit at new position
    move.l  #BLT_OP_MASKED,BLT_OP
    lea     data_robocop_rgba,a0
    move.l  a0,BLT_SRC
    lea     data_robocop_mask,a0
    move.l  a0,BLT_MASK
    move.l  d7,BLT_DST              ; d7 = new address
    move.l  #SPRITE_W,BLT_WIDTH
    move.l  #SPRITE_H,BLT_HEIGHT
    move.l  #SPRITE_STRIDE,BLT_SRC_STRIDE
    move.l  #LINE_BYTES,BLT_DST_STRIDE
    moveq   #1,d0
    move.l  d0,BLT_CTRL
```

---

# 5. Part 3: Copper List RGB Bars

The copper coprocessor executes a list of commands synchronised to the raster beam, allowing mid-frame register changes.

## Copper List Structure

Each copper bar entry consists of:
1. **WAIT** - Wait for specific raster Y position
2. **MOVE** - Set raster band Y position
3. **MOVE** - Set raster band height
4. **MOVE** - Set raster band colour
5. **MOVE** - Trigger raster band draw

```
Copper List Entry (36 bytes per bar):
Offset  Contents
0x00    WAIT instruction (wait for Y position)
0x04    Padding (wait Y value embedded in instruction)
0x08    MOVE RASTER_Y + value
0x0C    (value for Y)
0x10    MOVE RASTER_HEIGHT + value
0x14    (value for height)
0x18    MOVE RASTER_COLOR + value
0x1C    (value for colour) ← This is animated!
0x20    MOVE RASTER_CTRL
```

## Initializing the Copper

**IE32:**
```assembly
    ; Reset and set copper list pointer
    LDA #2                  ; Bit 1 = reset
    STA @COPPER_CTRL
    LDA #COPPER_LIST_ADDR
    STA @COPPER_PTR
    LDA #1                  ; Bit 0 = enable
    STA @COPPER_CTRL
```

**M68K:**
```assembly
    moveq   #2,d0
    move.l  d0,COPPER_CTRL
    lea     data_copper_list,a0
    move.l  a0,COPPER_PTR
    moveq   #1,d0
    move.l  d0,COPPER_CTRL
```

## Animating Bar Colours

Each frame, we update the colour values in the copper list to create a flowing gradient effect:

**IE32:**
```assembly
update_bars:
    ; A = frame counter
    LDB #0                      ; B = bar index (0-15)
    LDE #COPPER_LIST_ADDR
    ADD E, #BAR_COLOR_OFFSET    ; Point to first colour
    LDF #PALETTE_ADDR           ; 16-colour gradient palette

    ; Calculate scroll offset from sine table
    LDC A
    SHL C, #1                   ; Faster scroll
    AND C, #0xFF
    SHL C, #2
    ADD C, #SIN_X_ADDR
    LDT [C]                     ; T = sine offset
    ADD T, #200
    SHR T, #4                   ; T = 0-25 scroll offset

bar_loop:
    ; Colour index = bar_index + scroll_offset + frame/4
    LDC A
    SHR C, #2                   ; Slow colour cycling
    ADD C, B                    ; + bar index
    ADD C, T                    ; + sine scroll
    AND C, #0x0F                ; Wrap to 16 colours
    SHL C, #2                   ; * 4 bytes per colour
    ADD C, F
    LDU [C]                     ; Load colour from palette
    STU [E]                     ; Store in copper list

    ADD E, #BAR_STRIDE          ; Next bar entry
    ADD B, #1
    LDC #BAR_COUNT
    SUB C, B
    JNZ C, bar_loop
    RTS
```

**M68K:**
```assembly
update_bars:
    ; d0 = frame counter
    moveq   #0,d1                       ; d1 = bar index
    lea     data_copper_list,a0
    lea     BAR_COLOR_OFFSET(a0),a0     ; Point to first colour
    lea     data_palette,a1             ; Palette base

    ; Calculate sine scroll offset
    move.l  d0,d2
    lsl.l   #1,d2
    andi.l  #$FF,d2
    lsl.l   #2,d2
    lea     data_sin_x,a2
    move.l  (a2,d2.l),d6
    addi.l  #200,d6
    lsr.l   #4,d6                       ; d6 = scroll offset

.bar_loop:
    move.l  d0,d2
    lsr.l   #2,d2
    add.l   d1,d2
    add.l   d6,d2
    andi.l  #$0F,d2
    lsl.l   #2,d2
    move.l  (a1,d2.l),d7
    move.l  d7,(a0)

    lea     BAR_STRIDE(a0),a0
    addq.l  #1,d1
    cmpi.l  #BAR_COUNT,d1
    blt     .bar_loop
    rts
```

---

# 6. Part 4: PSG+ Music Playback

The PSG player supports AY-3-8910/YM2149 music files with enhanced audio processing.

## Starting Playback

**IE32:**
```assembly
    ; Enable PSG+ enhanced audio mode
    LDA #1
    STA @PSG_PLUS_CTRL

    ; Set pointer to embedded AY data
    LDA #ROBOCOP_AY_ADDR
    STA @PSG_PLAY_PTR

    ; Set data length
    LDA #ROBOCOP_AY_LEN     ; 24525 bytes
    STA @PSG_PLAY_LEN

    ; Start playback with looping
    LDA #5                  ; bit0=start, bit2=loop
    STA @PSG_PLAY_CTRL
```

**M68K:**
```assembly
    ; Enable PSG+
    moveq   #1,d0
    move.l  d0,PSG_PLUS_CTRL

    ; Set data pointer
    lea     data_robocop_ay,a0
    move.l  a0,PSG_PLAY_PTR

    ; Set length
    move.l  #ROBOCOP_AY_LEN,PSG_PLAY_LEN

    ; Start with loop
    moveq   #5,d0
    move.l  d0,PSG_PLAY_CTRL
```

**Z80:**
```assembly
    ; Enable PSG+
    ld   a,1
    ld   (PSG_PLUS_CTRL),a

    ; Set pointer (need to write 32-bit address byte by byte)
    ; AY data at bank 28 = 0x38000
    SET_PSG_PTR 0x38000
    SET_PSG_LEN ROBOCOP_AY_LEN

    ; Start with loop
    ld   a,5
    ld   (PSG_PLAY_CTRL),a
```

**6502:**
```assembly
    ; Enable PSG+
    lda  #1
    sta  PSG_PLUS_CTRL

    ; Set pointer and length (32-bit writes via macros)
    STORE32 PSG_PLAY_PTR, $38000
    STORE32 PSG_PLAY_LEN, ROBOCOP_AY_LEN

    ; Start with loop
    lda  #5
    sta  PSG_PLAY_CTRL
```

## PSG+ Enhanced Audio

PSG+ mode provides:
- 4× oversampling for smoother waveforms
- Gentle low-pass filtering
- Subtle saturation for warmth
- Stereo width enhancement

This makes 8-bit chip music sound richer while preserving the authentic character.

---

# 7. Part 5: Sine-Wave Scrolltext

The scrolling text at the bottom of the screen uses a sine wave lookup table to create a wavy effect.

## Scrolltext Data

The scrolltext system requires:
- **Character table** - Maps ASCII to font offsets
- **Font data** - 32×32 pixel characters in RGBA format
- **Sine table** - Pre-calculated Y offsets for the wave effect
- **Message string** - Null-terminated text to display

## Drawing Characters

For each visible character:
1. Look up the character's font offset
2. Calculate Y position from sine table (based on X + frame)
3. Calculate VRAM destination address
4. Blit the character with the blitter

**IE32:**
```assembly
draw_scrolltext:
    LDB @VAR_SCROLL_X
    LDC B
    SHR C, #5               ; C = char index (scroll / 32)
    LDD B
    AND D, #0x1F            ; D = pixel offset within char
    LDF #0
    SUB F, D
    LDD F                   ; D = -offset (start X position)
    LDE #0                  ; E = char counter

.scroll_loop:
    ; Get character from message
    LDF #SCROLL_MSG_ADDR
    ADD F, C
    LDT [F]
    AND T, #0xFF
    JZ T, .scroll_wrap      ; Null terminator

    ; Skip if off-screen left (negative X)
    LDA D
    AND A, #0x80000000
    JNZ A, .scroll_next

    ; Skip if off-screen right (X > 608)
    LDA #608
    SUB A, D
    AND A, #0x80000000
    JNZ A, .scroll_done

    ; Look up font offset for this character
    PUSH C
    PUSH D
    PUSH E

    LDF #SCROLL_CHAR_ADDR
    LDA T
    SHL A, #2
    ADD F, A
    LDA [F]                 ; A = font offset

    ; Calculate Y with sine wave
    LDF D                   ; F = X position
    ADD F, @VAR_FRAME_ADDR  ; + frame for animation
    AND F, #0xFF
    SHL F, #2
    ADD F, #SCROLL_SINE_ADDR
    LDB [F]                 ; B = sine offset
    ADD B, #SCROLL_Y        ; B = final Y position

    ; Calculate VRAM destination
    ; ... (address calculation)

    ; Blit character
    JSR blit_char

    POP E
    POP D
    POP C

.scroll_next:
    ADD D, #CHAR_WIDTH      ; Next X position
    ADD C, #1               ; Next char index
    ADD E, #1
    JMP .scroll_loop

.scroll_wrap:
    LDC #0                  ; Wrap to start of message
    JMP .scroll_loop

.scroll_done:
    RTS
```

## Sine Table Generation

The sine table contains pre-calculated Y offsets scaled for the wave amplitude:

```c
// Generate 256-entry sine table with amplitude of ±20 pixels
for (int i = 0; i < 256; i++) {
    sine_table[i] = (int)(sin(i * 2 * PI / 256) * 20);
}
```

---

# 8. Complete Source Listings

The complete source files are located in the `assembler/` directory:

| File | CPU | Lines | Description |
|------|-----|-------|-------------|
| `robocop_intro.asm` | IE32 | 1562 | Native IE32 implementation |
| `robocop_intro_68k.asm` | M68K | 1544 | Motorola 68020 port |
| `robocop_intro_z80.asm` | Z80 | 1556 | Zilog Z80 port with banking |
| `robocop_intro_65.asm` | 6502 | 1558 | MOS 6502 port with banking |

## Key Differences Between Ports

### IE32 and M68K (32-bit)
- Direct access to full 32MB address space
- Native 32-bit register operations
- Data embedded directly in program memory

### Z80 and 6502 (8-bit)
- 64KB address space requires banking system
- Data split across multiple 8KB banks:
  - Banks 0-21: Sprite RGBA data
  - Banks 22-27: Sprite mask
  - Banks 28-30: AY music
  - Banks 31-56: Font data
- 32-bit operations require multi-byte sequences
- Lookup tables for Y address calculation (16-bit × 480 entries)

---

# 9. Building and Running

## IE32

```bash
# Assemble
./bin/ie32asm assembler/robocop_intro.asm

# Run
./bin/IntuitionEngine -ie32 assembler/robocop_intro.iex
```

## M68K

```bash
# Assemble (requires vasm)
vasmm68k_mot -Fbin -m68020 -devpac \
    -o assembler/robocop_intro_68k.ie68 \
    assembler/robocop_intro_68k.asm

# Run
./bin/IntuitionEngine -m68k assembler/robocop_intro_68k.ie68
```

## Z80

```bash
# Assemble (requires vasm)
vasmz80_std -Fbin \
    -o assembler/robocop_intro_z80.ie80 \
    assembler/robocop_intro_z80.asm

# Run
./bin/IntuitionEngine -z80 assembler/robocop_intro_z80.ie80
```

## 6502

```bash
# Assemble (requires cc65 suite)
cd assembler
ca65 robocop_intro_65.asm -o robocop_intro_65.o
ld65 -C ie65.cfg -o robocop_intro_65.bin robocop_intro_65.o

# Run
cd ..
./bin/IntuitionEngine -m6502 assembler/robocop_intro_65.bin
```

## Expected Output

When running successfully, you should see:
1. Black screen clears
2. PSG+ music starts playing
3. Animated RGB colour bars appear (copper effect)
4. Robocop sprite moves in a smooth Lissajous pattern
5. Wavy scrolling text at the bottom

The demo runs at 60 FPS with VBlank synchronisation.

---

# Further Reading

- **README.md** - Complete system reference documentation
- **assembler/ie32.inc** - IE32 hardware definitions
- **assembler/ie68.inc** - M68K hardware definitions and macros
- **assembler/ie80.inc** - Z80 hardware definitions and macros
- **assembler/ie65.inc** - 6502 hardware definitions and macros

For questions or issues, visit: https://github.com/intuitionamiga/IntuitionEngine
