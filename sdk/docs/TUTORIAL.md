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
3. [Video Initialization and VBlank Sync](#3-video-initialization-and-vblank-sync)
4. [Blitter Sprite Movement](#4-blitter-sprite-movement)
5. [Copper List RGB Bars](#5-copper-list-rgb-bars)
6. [PSG+ Music Playback](#6-psg-music-playback)
7. [Sine-Wave Scrolltext](#7-sine-wave-scrolltext)
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
| IE32 | ie32asm | `sdk/bin/ie32asm program.asm` |
| M68K | vasmm68k_mot | `vasmm68k_mot -Fbin -m68020 -I. -o out.ie68 program.asm` |
| Z80 | vasmz80_std | `vasmz80_std -Fbin -I. -o out.ie80 program.asm` |
| 6502 | ca65/ld65 | `ca65 program.asm && ld65 -C ie65.cfg -o out.bin program.o` |

## Include Files

Each CPU architecture has a corresponding include file in the `sdk/include/` directory:

| File | CPU | Contents |
|------|-----|----------|
| `ie32.inc` | IE32 | Hardware constants using `.equ` |
| `ie68.inc` | M68K | Hardware constants and helper macros |
| `ie80.inc` | Z80 | Hardware constants and macros for 32-bit operations |
| `ie65.inc` | 6502 | Hardware constants, macros, and zero page allocation |

These files define all hardware register addresses, blitter operations, copper opcodes, and provide helper macros for common operations.

**Note on register naming:** The IE32 CPU uses single-letter register names (A, B, C, D, E, F, T, U) while M68K uses d0-d7 and a0-a7. Z80 uses 8-bit registers (A, B, C, D, E, H, L) and register pairs (BC, DE, HL). The 6502 uses A, X, and Y with zero-page variables for extended storage.

## Data Files

The demo requires binary data files that are embedded directly into the assembled binary using the `incbin` directive. These files are located in the repository and assembled along with the source code:

| File | Size | Description |
|------|------|-------------|
| `robocop_rgba.bin` | 172,800 bytes | 240×180 pixel sprite (RGBA format) |
| `robocop_mask.bin` | 5,400 bytes | 1-bit transparency mask for sprite |
| `Robocop1.ay` | ~24KB | AY-3-8910 music file |
| `font_rgba.bin` | 256,000 bytes | 32×32 pixel font (80 characters × 4096 bytes) |

**Embedding data with incbin:**

The data is embedded at assembly time using the `incbin` directive:

```assembly
; M68K (vasm)
data_robocop_rgba:
    incbin  "../assets/robocop_rgba.bin"

; Z80 (vasm)
data_robocop_rgba:
    .incbin "../assets/robocop_rgba.bin"

; 6502 (ca65)
data_robocop_rgba:
.incbin "../assets/robocop_rgba.bin"
```

Once assembled, DMA engines (blitter, copper, PSG player) can access this data directly through the memory bus using the label addresses.

---

# 3. Video Initialization and VBlank Sync

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

**Z80:**
```assembly
    call wait_blit
    SET_BLT_OP BLT_OP_FILL

    ; Set destination to VRAM start (0x100000)
    xor  a
    ld   (BLT_DST_0),a
    ld   (BLT_DST_1),a
    ld   a,$10
    ld   (BLT_DST_2),a
    xor  a
    ld   (BLT_DST_3),a

    SET_BLT_WIDTH SCREEN_W
    SET_BLT_HEIGHT SCREEN_H
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR $FF000000

    START_BLIT
    call wait_blit
```

**6502:**
```assembly
    jsr  wait_blit
    SET_BLT_OP BLT_OP_FILL

    ; Set destination to VRAM start (0x100000)
    lda  #$00
    sta  BLT_DST_0
    sta  BLT_DST_1
    lda  #$10
    sta  BLT_DST_2
    lda  #$00
    sta  BLT_DST_3

    SET_BLT_WIDTH SCREEN_W
    SET_BLT_HEIGHT SCREEN_H
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR $FF000000

    START_BLIT
    jsr  wait_blit
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

**Z80:**
```assembly
wait_blit:
    ld   a,(BLT_CTRL)
    and  2                      ; Bit 1 = busy
    jr   nz,wait_blit
    ret
```

**6502:**
```assembly
.proc wait_blit
:   lda  BLT_CTRL
    and  #2                     ; Bit 1 = busy
    bne  :-
    rts
.endproc
```

---

# 4. Blitter Sprite Movement

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

**Z80:**
```assembly
; compute_xy - Calculate sprite position from sine tables
; Uses pre-calculated 32-bit sine/cosine tables
; Stores result in sprite_x and sprite_y (32-bit values)
compute_xy:
    ; X = sin_x[frame & 0xFF] + CENTER_X
    ld   a,(frame_lo)
    and  $FF                    ; Wrap to 256 entries
    ld   l,a
    ld   h,0
    add  hl,hl
    add  hl,hl                  ; * 4 bytes per entry
    ld   de,data_sin_x
    add  hl,de
    ; Load 32-bit sine value and add CENTER_X
    ld   e,(hl)
    inc  hl
    ld   d,(hl)                 ; DE = low word
    ; Store X position (simplified: using low 16 bits + CENTER_X)
    ld   hl,CENTER_X
    add  hl,de
    ld   (sprite_x),hl

    ; Y = cos_y[(frame*2) & 0xFF] + CENTER_Y
    ld   a,(frame_lo)
    add  a,a                    ; * 2 for different frequency
    and  $FF
    ld   l,a
    ld   h,0
    add  hl,hl
    add  hl,hl                  ; * 4 bytes per entry
    ld   de,data_cos_y
    add  hl,de
    ld   e,(hl)
    inc  hl
    ld   d,(hl)
    ld   hl,CENTER_Y
    add  hl,de
    ld   (sprite_y),hl
    ret
```

**6502:**
```assembly
; compute_xy - Calculate sprite position from sine tables
; Uses pre-calculated tables with 16-bit values
.proc compute_xy
    ; X = sin_x[frame & 0xFF] + CENTER_X
    lda  frame_lo
    and  #$FF
    asl  a                      ; * 2 for table index
    tay
    lda  data_sin_x,y           ; Load low byte
    clc
    adc  #<CENTER_X
    sta  sprite_x
    lda  data_sin_x+1,y         ; Load high byte
    adc  #>CENTER_X
    sta  sprite_x+1

    ; Y = cos_y[(frame*2) & 0xFF] + CENTER_Y
    lda  frame_lo
    asl  a                      ; * 2 for different frequency
    and  #$FE                   ; Keep even (already * 2)
    tay
    lda  data_cos_y,y
    clc
    adc  #<CENTER_Y
    sta  sprite_y
    lda  data_cos_y+1,y
    adc  #>CENTER_Y
    sta  sprite_y+1
    rts
.endproc
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

**M68K:**
```assembly
    ; Compute VRAM address from d2 (X) and d3 (Y)
    move.l  d3,d0
    mulu.w  #LINE_BYTES,d0      ; d0 = Y * LINE_BYTES
    move.l  d2,d1
    lsl.l   #2,d1               ; d1 = X * 4
    add.l   d1,d0
    addi.l  #VRAM_START,d0      ; d0 = final address
```

**Z80:**
```assembly
    ; Use pre-calculated Y address table for efficiency
    ; y_addr_table[y] = VRAM_START + y * LINE_BYTES
    ld   hl,(sprite_y)
    add  hl,hl                  ; * 2 for 16-bit entries
    ld   de,y_addr_table
    add  hl,de
    ld   e,(hl)
    inc  hl
    ld   d,(hl)                 ; DE = y_addr_table[y] (low 16 bits)
    ; Add X * 4
    ld   hl,(sprite_x)
    add  hl,hl
    add  hl,hl                  ; HL = X * 4
    add  hl,de                  ; HL = final address (low 16 bits)
    ld   (dest_addr),hl
    ; High byte is $10 (VRAM_START = $100000)
    ld   a,$10
    ld   (dest_addr+2),a
```

**6502:**
```assembly
    ; Use pre-calculated Y address table
    ; y_addr_table: .word VRAM_START + 0*2560, VRAM_START + 1*2560, ...
    lda  sprite_y
    asl  a                      ; * 2 for 16-bit table entries
    tay
    lda  y_addr_table,y
    sta  dest_addr
    lda  y_addr_table+1,y
    sta  dest_addr+1
    ; Add X * 4
    lda  sprite_x
    asl  a
    asl  a                      ; A = (sprite_x & $3F) * 4
    clc
    adc  dest_addr
    sta  dest_addr
    lda  #0
    adc  dest_addr+1
    sta  dest_addr+1
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

**Z80:**
```assembly
    ; Clear previous position
    call wait_blit
    SET_BLT_OP BLT_OP_FILL

    ; Set destination to previous address (32-bit)
    ld   hl,(prev_addr)
    ld   a,l
    ld   (BLT_DST_0),a
    ld   a,h
    ld   (BLT_DST_1),a
    ld   a,(prev_addr+2)
    ld   (BLT_DST_2),a
    xor  a
    ld   (BLT_DST_3),a

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_BLT_COLOR BACKGROUND
    SET_DST_STRIDE LINE_BYTES
    START_BLIT
    call wait_blit

    ; Masked blit at new position
    SET_BLT_OP BLT_OP_MASKED
    STORE32 BLT_SRC_0, data_robocop_rgba
    STORE32 BLT_MASK_0, data_robocop_mask

    ; Set destination to new address
    ld   hl,(dest_addr)
    ld   a,l
    ld   (BLT_DST_0),a
    ld   a,h
    ld   (BLT_DST_1),a
    ld   a,(dest_addr+2)
    ld   (BLT_DST_2),a
    xor  a
    ld   (BLT_DST_3),a

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_SRC_STRIDE SPRITE_STRIDE
    SET_DST_STRIDE LINE_BYTES
    START_BLIT
```

**6502:**
```assembly
    ; Clear previous position
    jsr  wait_blit
    SET_BLT_OP BLT_OP_FILL

    ; Set destination to previous address
    lda  prev_addr
    sta  BLT_DST_0
    lda  prev_addr+1
    sta  BLT_DST_1
    lda  prev_addr+2
    sta  BLT_DST_2
    lda  #0
    sta  BLT_DST_3

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_BLT_COLOR BACKGROUND
    SET_DST_STRIDE LINE_BYTES
    START_BLIT
    jsr  wait_blit

    ; Masked blit at new position
    SET_BLT_OP BLT_OP_MASKED
    STORE32 BLT_SRC_0, data_robocop_rgba
    STORE32 BLT_MASK_0, data_robocop_mask

    ; Set destination to new address
    lda  dest_addr
    sta  BLT_DST_0
    lda  dest_addr+1
    sta  BLT_DST_1
    lda  dest_addr+2
    sta  BLT_DST_2
    lda  #0
    sta  BLT_DST_3

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_SRC_STRIDE SPRITE_STRIDE
    SET_DST_STRIDE LINE_BYTES
    START_BLIT
```

---

# 5. Copper List RGB Bars

The copper coprocessor executes a list of commands synchronised to the raster beam, allowing mid-frame register changes.

## Copper List Structure

Each copper bar entry consists of 9 longwords (36 bytes):
1. **WAIT** - Wait for specific raster Y position
2. **MOVE** - Set raster band Y position + value
3. **MOVE** - Set raster band height + value
4. **MOVE** - Set raster band colour + value (animated)
5. **MOVE** - Trigger raster band draw

```
Copper List Entry (36 bytes = 9 longwords per bar):
Offset  Size    Contents
0x00    4       WAIT opcode (Y position encoded as Y * 0x1000)
0x04    4       MOVE opcode for RASTER_Y register
0x08    4       Y position value
0x0C    4       MOVE opcode for RASTER_HEIGHT register
0x10    4       Height value
0x14    4       MOVE opcode for RASTER_COLOR register
0x18    4       Colour value (RGBA) ← This is animated!
0x1C    4       MOVE opcode for RASTER_CTRL register
0x20    4       Trigger value (1 = draw band)
```

Example copper list entry for a bar at Y=40 with height=12:
```assembly
    dc.l    40*COP_WAIT_SCALE    ; WAIT for scanline 40
    dc.l    COP_MOVE_RASTER_Y    ; MOVE to RASTER_Y
    dc.l    40                   ; Y = 40
    dc.l    COP_MOVE_RASTER_H    ; MOVE to RASTER_HEIGHT
    dc.l    12                   ; height = 12
    dc.l    COP_MOVE_RASTER_COLOR ; MOVE to RASTER_COLOR
    dc.l    $FF0000FF            ; colour = red (RGBA)
    dc.l    COP_MOVE_RASTER_CTRL ; MOVE to RASTER_CTRL
    dc.l    1                    ; trigger = 1 (draw)
```

The copper opcode constants are defined in the include files (`ie32.inc`, `ie68.inc`, `ie80.inc`, `ie65.inc`):
- `COP_WAIT_SCALE` = 0x1000 (multiply Y position by this for WAIT)
- `COP_MOVE_RASTER_Y` = 0x40120000
- `COP_MOVE_RASTER_H` = 0x40130000
- `COP_MOVE_RASTER_COLOR` = 0x40140000
- `COP_MOVE_RASTER_CTRL` = 0x40150000
- `COP_END` = 0xC0000000

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

**Z80:**
```assembly
    ; Reset copper
    ld   a,2
    ld   (COPPER_CTRL),a
    ld   (COPPER_CTRL+1),a      ; Write 32-bit (simplified)
    xor  a
    ld   (COPPER_CTRL+2),a
    ld   (COPPER_CTRL+3),a

    ; Set copper list pointer (32-bit address)
    STORE32 COPPER_PTR, data_copper_list

    ; Enable copper
    ld   a,1
    ld   (COPPER_CTRL),a
    xor  a
    ld   (COPPER_CTRL+1),a
    ld   (COPPER_CTRL+2),a
    ld   (COPPER_CTRL+3),a
```

**6502:**
```assembly
    ; Reset copper
    lda  #2
    sta  COPPER_CTRL
    lda  #0
    sta  COPPER_CTRL+1
    sta  COPPER_CTRL+2
    sta  COPPER_CTRL+3

    ; Set copper list pointer
    STORE32 COPPER_PTR, data_copper_list

    ; Enable copper
    lda  #1
    sta  COPPER_CTRL
    lda  #0
    sta  COPPER_CTRL+1
    sta  COPPER_CTRL+2
    sta  COPPER_CTRL+3
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

# 6. PSG+ Music Playback

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

    ; Set pointer to embedded AY data (32-bit address via macro)
    SET_PSG_PTR data_robocop_ay
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
    STORE32 PSG_PLAY_PTR_0, data_robocop_ay
    STORE32 PSG_PLAY_LEN_0, ROBOCOP_AY_LEN

    ; Start with loop
    lda  #5
    sta  PSG_PLAY_CTRL
```

## PSG+ Enhanced Audio

PSG+ mode provides:
- 4× oversampling for smoother waveforms
- Second-order Butterworth lowpass filtering (-12dB/oct anti-alias)
- Subtle drive saturation for warmth
- Allpass diffuser room ambience

This makes 8-bit chip music sound richer while preserving the authentic character.

---

# 7. Sine-Wave Scrolltext

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

    ; Calculate VRAM destination address
    ; Address = VRAM_START + (Y * LINE_BYTES) + (X * 4)
    LDF B                   ; F = Y position
    LDU #LINE_BYTES
    MUL F, U                ; F = Y * LINE_BYTES
    LDU D                   ; U = X position
    SHL U, #2               ; U = X * 4
    ADD F, U
    ADD F, #VRAM_START      ; F = final VRAM address

    ; Blit character (A = font offset, F = dest address)
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

**M68K:**
```assembly
draw_scrolltext:
    move.l  VAR_SCROLL_X,d1             ; d1 = scroll_x
    move.l  d1,d2
    lsr.l   #5,d2                       ; d2 = scroll_x / 32 (char index)
    move.l  d1,d3
    andi.l  #$1F,d3                     ; d3 = scroll_x % 32 (pixel offset)
    neg.l   d3                          ; d3 = -pixel_offset (start position)
    moveq   #0,d4                       ; d4 = char counter

.scroll_loop:
    lea     scroll_message,a0
    move.b  (a0,d2.l),d5                ; d5 = ASCII character
    andi.l  #$FF,d5
    tst.l   d5
    beq     .scroll_wrap                ; null terminator

    ; Skip if off-screen left
    tst.l   d3
    bmi     .scroll_next

    ; Skip if off-screen right
    cmpi.l  #608,d3
    bge     .scroll_done

    ; Save registers
    movem.l d1-d4,-(sp)

    ; Look up char in table
    lea     scroll_char_table,a1
    move.l  d5,d0
    lsl.l   #2,d0
    move.l  (a1,d0.l),d0                ; font offset
    lea     scroll_font_data,a3
    lea     (a3,d0.l),a3
    move.l  a3,d6                       ; d6 = source address

    ; Calculate Y with sine
    move.l  d3,d0
    add.l   VAR_FRAME_ADDR,d0           ; + frame counter
    andi.l  #$FF,d0
    lsl.l   #2,d0
    lea     scroll_sine_table,a2
    move.l  (a2,d0.l),d7                ; d7 = sine offset
    addi.l  #SCROLL_Y,d7                ; d7 = final Y

    ; Calculate dest address
    move.l  d7,d0
    mulu.w  #LINE_BYTES,d0
    addi.l  #VRAM_START,d0
    move.l  d3,d5
    lsl.l   #2,d5
    add.l   d5,d0
    move.l  d0,d7                       ; d7 = dest address

    ; Blit character using blitter
    jsr     blit_char

    movem.l (sp)+,d1-d4

.scroll_next:
    addq.l  #1,d2                       ; next char
    addi.l  #CHAR_WIDTH,d3              ; next X position
    addq.l  #1,d4
    cmpi.l  #21,d4
    blt     .scroll_loop
    bra     .scroll_done

.scroll_wrap:
    moveq   #0,d2                       ; wrap to start
    bra     .scroll_loop

.scroll_done:
    rts
```

**Z80:**
```assembly
draw_scrolltext:
    ; Calculate starting character index: char_idx = scroll_x >> 5
    ld   hl,(scroll_x_lo)
    ; Shift right 5 bits
    ld   a,l
    rrca
    rrca
    rrca
    rrca
    rrca
    and  $07
    ld   c,a
    ld   a,h
    rlca
    rlca
    rlca
    or   c
    ld   (char_idx),a

    ; Calculate pixel offset: char_x = -(scroll_x & 0x1F)
    ld   a,(scroll_x_lo)
    and  $1F
    jr   z,.char_x_zero
    neg
    ld   (char_x),a
    ld   a,$FF                  ; Sign extend
    ld   (char_x+1),a
    jr   .char_x_done
.char_x_zero:
    xor  a
    ld   (char_x),a
    ld   (char_x+1),a
.char_x_done:

    ; Initialize counter
    xor  a
    ld   (char_count),a

.char_loop:
    ; Get character from message
    ld   hl,scroll_message
    ld   de,(char_idx)
    add  hl,de
    ld   a,(hl)
    or   a
    jr   nz,.got_char
    jp   .wrap_scroll
.got_char:
    ld   (curr_char),a

    ; Check X bounds, calculate Y from sine, blit character
    ; ... (similar pattern using lookup tables)
    ; See robocop_intro_z80.asm for complete implementation

    ; Next character
    ld   a,(char_idx)
    inc  a
    ld   (char_idx),a
    ld   hl,(char_x)
    ld   de,CHAR_WIDTH
    add  hl,de
    ld   (char_x),hl
    jp   .char_loop

.wrap_scroll:
    xor  a
    ld   (char_idx),a
    jp   .char_loop

.scroll_done:
    ret
```

**6502:**
```assembly
.proc draw_scrolltext
    ; Calculate starting character index: char_idx = scroll_x >> 5
    lda  scroll_x_lo
    lsr  a
    lsr  a
    lsr  a
    lsr  a
    lsr  a
    sta  char_idx
    lda  scroll_x_hi
    asl  a
    asl  a
    asl  a
    ora  char_idx
    sta  char_idx

    ; Calculate pixel offset: char_x = -(scroll_x & 0x1F)
    lda  scroll_x_lo
    and  #$1F
    beq  @char_x_zero
    eor  #$FF
    clc
    adc  #1
    sta  char_x
    lda  #$FF
    sta  char_x+1
    jmp  @char_x_done
@char_x_zero:
    sta  char_x
    sta  char_x+1
@char_x_done:

    ; Initialize counter
    lda  #0
    sta  char_count

@char_loop:
    ; Get character from message
    clc
    lda  #<scroll_message
    adc  char_idx
    sta  zp_ptr0
    lda  #>scroll_message
    adc  #0
    sta  zp_ptr0+1
    ldy  #0
    lda  (zp_ptr0),y
    bne  @got_char
    jmp  @wrap_scroll
@got_char:
    sta  curr_char

    ; Check X bounds, calculate Y from sine, blit character
    ; ... (similar pattern using lookup tables)
    ; See robocop_intro_65.asm for complete implementation

    ; Next character
    inc  char_idx
    clc
    lda  char_x
    adc  #CHAR_WIDTH
    sta  char_x
    lda  char_x+1
    adc  #0
    sta  char_x+1
    jmp  @char_loop

@wrap_scroll:
    lda  #0
    sta  char_idx
    jmp  @char_loop

@scroll_done:
    rts
.endproc
```

## Sine Table Generation

The sine and cosine tables contain pre-calculated values for smooth animation. These tables are already embedded in the source files as data sections, so you don't need to generate them yourself.

The concept behind the table generation:

```c
// Generate 256-entry sine table with amplitude of ±20 pixels
for (int i = 0; i < 256; i++) {
    sine_table[i] = (int)(sin(i * 2 * PI / 256) * 20);
}
```

**Note:** The demo source files include pre-calculated sine/cosine tables (`data_sin_x`, `data_cos_y`, `scroll_sine_table`) and Y-address lookup tables (`y_addr_table`) already embedded as data sections. These are assembled directly into the binary along with the sprite, mask, font, and music data.

---

# 8. Complete Source Listings

The complete source files are located in the `sdk/examples/asm/` directory:

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
sdk/bin/ie32asm sdk/examples/asm/robocop_intro.asm

# Run
./bin/IntuitionEngine -ie32 robocop_intro.iex
```

## M68K

```bash
# Assemble (requires vasm) - use -I for include and asset paths
vasmm68k_mot -Fbin -m68020 -Isdk/include -Isdk/examples/assets \
    -o robocop_intro_68k.ie68 \
    sdk/examples/asm/robocop_intro_68k.asm

# Run
./bin/IntuitionEngine -m68k robocop_intro_68k.ie68
```

## Z80

```bash
# Assemble (requires vasm) - use -I for include and asset paths
vasmz80_std -Fbin -Isdk/include -Isdk/examples/assets \
    -o robocop_intro_z80.ie80 \
    sdk/examples/asm/robocop_intro_z80.asm

# Run
./bin/IntuitionEngine -z80 robocop_intro_z80.ie80
```

## 6502

```bash
# Assemble (requires cc65 suite)
ca65 -I sdk/include sdk/examples/asm/robocop_intro_65.asm -o robocop_intro_65.o
ld65 -C sdk/include/ie65.cfg -o robocop_intro_65.bin robocop_intro_65.o
rm robocop_intro_65.o

# Run
./bin/IntuitionEngine -m6502 robocop_intro_65.bin
```

## Running from EhBASIC

Any assembled binary can also be launched from the EhBASIC interpreter prompt:

```basic
RUN "sdk/examples/prebuilt/robocop_intro.iex"
RUN "sdk/examples/prebuilt/robocop_intro_68k.ie68"
RUN "sdk/examples/prebuilt/robocop_intro_z80.ie80"
RUN "sdk/examples/prebuilt/robocop_intro_65.bin"
```

The `RUN` command auto-detects the CPU core from the file extension.

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
- **sdk/include/ie32.inc** - IE32 hardware definitions
- **sdk/include/ie68.inc** - M68K hardware definitions and macros
- **sdk/include/ie80.inc** - Z80 hardware definitions and macros
- **sdk/include/ie65.inc** - 6502 hardware definitions and macros

For questions or issues, visit: https://github.com/intuitionamiga/IntuitionEngine
