; ============================================================================
; ROBOCOP INTRO (M68K PORT) - Blitter Sprite, Copper Rasterbars and PSG Music
; Motorola 68020 assembly for IntuitionEngine - VideoChip + Copper + PSG audio
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Motorola 68020 (32-bit)
; Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true colour)
; Audio Engine:  PSG (AY-3-8910 compatible, PSG+ enhanced mode)
; Assembler:     vasmm68k_mot (Motorola syntax)
; Build:         vasmm68k_mot -Fbin -m68020 -devpac -o robocop_intro_68k.ie68 robocop_intro_68k.asm
; Run:           ./IntuitionEngine -m68k robocop_intro_68k.ie68
; Porting:       See robocop_intro.asm (IE32 reference), robocop_intro_65.asm
;                (6502), robocop_intro_z80.asm (Z80)
;
; === WHAT THIS DEMO DOES ===
; 1. Clears the screen to solid black using a blitter fill operation
; 2. Loads and plays an AY-format music file (Robocop theme) via PSG+
; 3. Programmes a 16-bar copper list for animated rasterbar colour cycling
; 4. Moves a masked Robocop sprite along a sine/cosine Lissajous path
; 5. Renders a sine-wave scrolltext along the bottom of the screen
; 6. Animates copper bar colours each frame using a scrolling gradient
;
; === WHY BLITTER IMAGE DISPLAY + COPPER EFFECTS ===
; This demo recreates the style of classic 8-bit and 16-bit game intro
; screens -- specifically inspired by the Robocop (1988) home computer
; ports by Ocean Software. On machines like the ZX Spectrum and Amstrad
; CPC, the loading screen was often the player's first impression of a
; game, and developers used every hardware trick available to make it
; memorable.
;
; The copper coprocessor is analogous to the Amiga's copper -- a simple
; programmable display coprocessor that can modify video registers at
; specific scanline positions. This enables effects like colour gradient
; bars, split-screen palettes, and per-scanline colour changes without
; any CPU intervention. The 16 rainbow bars here cycle their colours each
; frame, producing a flowing gradient wave reminiscent of demoscene
; rasterbar effects from the late 1980s.
;
; The hardware blitter handles all pixel operations: clearing the previous
; sprite position, drawing the masked sprite at its new location, and
; rendering scrolltext characters. This frees the CPU to focus on
; animation logic (sine table lookups, copper list updates) rather than
; pushing individual pixels -- exactly the division of labour that made
; the Amiga and Atari ST so effective for games and demos.
;
; === M68K-SPECIFIC NOTES ===
; The 68020 is a 32-bit processor with a flat address space, making this
; port the most straightforward of the four. All hardware registers are
; accessed directly via absolute long addressing. The 68020's indexed
; addressing modes (e.g. move.l (a0,d1.l),d2) make sine table lookups
; particularly clean. Register allocation: d0-d5 for temporaries, d6/d7
; for previous/new VRAM addresses, a0-a3 for data pointers. The movem
; instruction saves and restores register sets efficiently around the
; scrolltext blitter calls. Hardware constants are defined in ie68.inc.
;
; === MEMORY MAP ===
; $001000 onwards     Program code (loaded at PROGRAM_START)
; After code          Sine/cosine tables, palette, copper list (32-bit aligned)
; After tables        Sprite RGBA, mask, AY music, font and scrolltext data
; $008800-$00880F     Runtime variables (frame counter, positions, scroll)
; $0F0000             I/O registers (video, blitter, copper, PSG)
; $100000             VRAM start (640x480x4 = 1,228,800 bytes)
;
; === BUILD AND RUN ===
; Build:  vasmm68k_mot -Fbin -m68020 -devpac -o robocop_intro_68k.ie68 robocop_intro_68k.asm
; Run:    ./IntuitionEngine -m68k robocop_intro_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

; ----------------------------------------------------------------------------
; DEMO-SPECIFIC CONSTANTS
; ----------------------------------------------------------------------------
SPRITE_W        equ 240
SPRITE_H        equ 180
SPRITE_STRIDE   equ 960             ; 240 pixels x 4 bytes per pixel
CENTER_X        equ 200             ; Horizontal centre of Lissajous path
CENTER_Y        equ 150             ; Vertical centre of Lissajous path

; Copper bar layout constants
BAR_COUNT       equ 16
BAR_STRIDE      equ 36              ; Bytes per bar entry in copper list
BAR_WAIT_OFFSET equ 0
BAR_Y_OFFSET    equ 8
BAR_HEIGHT_OFFSET equ 16
BAR_COLOR_OFFSET  equ 24            ; Offset to colour value within each bar
BAR_HEIGHT      equ 12
BAR_SPACING     equ 20

; Runtime variable addresses (scratch RAM)
VAR_FRAME_ADDR  equ $8800
VAR_PREV_X_ADDR equ $8804
VAR_PREV_Y_ADDR equ $8808
VAR_SCROLL_X    equ $880C

ROBOCOP_AY_LEN  equ 24525

; Scrolltext constants
SCROLL_Y        equ 430             ; Baseline Y position for scrolltext
SCROLL_SPEED    equ 4               ; Pixels per frame horizontal scroll
CHAR_WIDTH      equ 32
CHAR_HEIGHT     equ 32
FONT_STRIDE     equ 1280            ; 320 pixels x 4 bytes (10 chars/row)

; ============================================================================
; CODE SECTION
; ============================================================================
    section code
    org     $001000              ; M68K programmes load at $1000

; ============================================================================
; INITIALISATION
; ============================================================================
start:
    ; Set 640x480 true colour mode and enable the display
    moveq   #0,d0
    move.l  d0,VIDEO_MODE
    moveq   #1,d0
    move.l  d0,VIDEO_CTRL

    ; --- Clear screen to black using a blitter fill ---
    jsr     wait_blit
    move.l  #BLT_OP_FILL,d0
    move.l  d0,BLT_OP
    move.l  #VRAM_START,d0
    move.l  d0,BLT_DST
    move.l  #SCREEN_W,d0
    move.l  d0,BLT_WIDTH
    move.l  #SCREEN_H,d0
    move.l  d0,BLT_HEIGHT
    move.l  #BACKGROUND,d0
    move.l  d0,BLT_COLOR
    move.l  #LINE_BYTES,d0
    move.l  d0,BLT_DST_STRIDE
    moveq   #1,d0
    move.l  d0,BLT_CTRL
    jsr     wait_blit

    ; --- Start PSG+ music playback ---
    ; Enable PSG+ enhanced mode, point to the embedded AY file,
    ; and start looped playback (bit 0 = start, bit 2 = loop)
    moveq   #1,d0
    move.l  d0,PSG_PLUS_CTRL
    lea     data_robocop_ay,a0
    move.l  a0,d0
    move.l  d0,PSG_PLAY_PTR
    move.l  #ROBOCOP_AY_LEN,d0
    move.l  d0,PSG_PLAY_LEN
    moveq   #5,d0                   ; bit0=start, bit2=loop
    move.l  d0,PSG_PLAY_CTRL

    ; --- Programme the copper coprocessor ---
    ; Disable copper, set the list pointer, then re-enable
    moveq   #2,d0
    move.l  d0,COPPER_CTRL
    lea     data_copper_list,a0
    move.l  a0,d0
    move.l  d0,COPPER_PTR
    moveq   #1,d0
    move.l  d0,COPPER_CTRL

    ; --- Initialise runtime state ---
    moveq   #0,d0
    move.l  d0,VAR_FRAME_ADDR
    jsr     compute_xy              ; d2=X, d3=Y
    move.l  d2,VAR_PREV_X_ADDR
    move.l  d3,VAR_PREV_Y_ADDR

    ; Initialise scrolltext horizontal position
    moveq   #0,d0
    move.l  d0,VAR_SCROLL_X

; ============================================================================
; MAIN LOOP
; Each iteration: advance frame, update copper colours, move sprite,
; synchronise to VBlank, then draw.
; ============================================================================
main_loop:
    ; Advance frame counter
    move.l  VAR_FRAME_ADDR,d0
    addq.l  #1,d0
    move.l  d0,VAR_FRAME_ADDR

    ; Update copper bar colours with scrolling gradient effect
    move.l  VAR_FRAME_ADDR,d0
    jsr     update_bars

    ; Compute new sprite position from sine/cosine tables (d2=x, d3=y)
    move.l  VAR_FRAME_ADDR,d0
    jsr     compute_xy

    ; Calculate VRAM address of previous sprite position -> d6
    move.l  VAR_PREV_Y_ADDR,d4
    move.l  #LINE_BYTES,d5
    mulu.w  d5,d4
    move.l  VAR_PREV_X_ADDR,d5
    lsl.l   #2,d5
    add.l   d5,d4
    addi.l  #VRAM_START,d4
    move.l  d4,d6                   ; d6 = prev address

    ; Calculate VRAM address of new sprite position -> d7
    move.l  d3,d4
    move.l  #LINE_BYTES,d5
    mulu.w  d5,d4
    move.l  d2,d5
    lsl.l   #2,d5
    add.l   d5,d4
    addi.l  #VRAM_START,d4
    move.l  d4,d7                   ; d7 = new address

    ; Save current position for next frame's erase
    move.l  d2,VAR_PREV_X_ADDR
    move.l  d3,VAR_PREV_Y_ADDR

    ; --- Synchronise to vertical blank before drawing ---
    jsr     wait_frame

    ; --- Erase previous sprite position ---
    ; Fill the old rectangle with the background colour
    jsr     wait_blit
    move.l  #BLT_OP_FILL,d0
    move.l  d0,BLT_OP
    move.l  d6,BLT_DST
    move.l  #SPRITE_W,d0
    move.l  d0,BLT_WIDTH
    move.l  #SPRITE_H,d0
    move.l  d0,BLT_HEIGHT
    move.l  #BACKGROUND,d0
    move.l  d0,BLT_COLOR
    move.l  #LINE_BYTES,d0
    move.l  d0,BLT_DST_STRIDE
    moveq   #1,d0
    move.l  d0,BLT_CTRL
    jsr     wait_blit

    ; --- Draw sprite at new position using masked blit ---
    ; The mask ensures transparent pixels are not written to VRAM,
    ; preserving the copper rasterbar effect behind the sprite
    move.l  #BLT_OP_MASKED,d0
    move.l  d0,BLT_OP
    lea     data_robocop_rgba,a0
    move.l  a0,BLT_SRC
    lea     data_robocop_mask,a0
    move.l  a0,BLT_MASK
    move.l  d7,BLT_DST
    move.l  #SPRITE_W,d0
    move.l  d0,BLT_WIDTH
    move.l  #SPRITE_H,d0
    move.l  d0,BLT_HEIGHT
    move.l  #SPRITE_STRIDE,d0
    move.l  d0,BLT_SRC_STRIDE
    move.l  #LINE_BYTES,d0
    move.l  d0,BLT_DST_STRIDE
    moveq   #1,d0
    move.l  d0,BLT_CTRL

    ; --- Render scrolltext ---
    jsr     clear_scroll_area
    jsr     draw_scrolltext
    move.l  VAR_SCROLL_X,d0
    addq.l  #SCROLL_SPEED,d0
    move.l  d0,VAR_SCROLL_X

    bra     main_loop

; ============================================================================
; SUBROUTINES
; ============================================================================

; ----------------------------------------------------------------------------
; wait_blit - Poll the blitter busy flag until the current operation completes
; ----------------------------------------------------------------------------
wait_blit:
    move.l  BLT_CTRL,d0
    andi.l  #2,d0
    bne     wait_blit
    rts

; ----------------------------------------------------------------------------
; wait_vblank - Wait until the vertical blanking interval begins
; ----------------------------------------------------------------------------
wait_vblank:
    move.l  VIDEO_STATUS,d0
    andi.l  #STATUS_VBLANK,d0
    beq     wait_vblank
    rts

; ----------------------------------------------------------------------------
; wait_frame - Wait for exactly one complete frame boundary
; First waits for VBlank to END (active scan period begins), then waits
; for VBlank to START again (new frame). This ensures exactly one frame
; elapses per call.
; ----------------------------------------------------------------------------
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

; ----------------------------------------------------------------------------
; update_bars - Animate copper bar colours with a scrolling sine gradient
;
; Each frame, every bar's colour is recalculated as:
;   colour_index = (bar_index + sine_scroll_offset + frame/4) mod 16
; This produces a flowing rainbow wave that shifts across all 16 bars.
;
; Input:  d0 = frame counter
; Clobbers: d0-d2, d6-d7, a0-a2
; ----------------------------------------------------------------------------
update_bars:
    moveq   #0,d1                       ; d1 = bar index (0-15)
    lea     data_copper_list,a0
    lea     BAR_COLOR_OFFSET(a0),a0     ; point to first colour value
    lea     data_palette,a1             ; a1 = palette base

    ; Calculate global scroll offset from sine table
    move.l  d0,d2                       ; d2 = frame
    lsl.l   #1,d2                       ; faster scroll
    andi.l  #$FF,d2                     ; wrap to 256 entries
    lsl.l   #2,d2                       ; * 4 bytes per entry
    lea     data_sin_x,a2
    move.l  (a2,d2.l),d6                ; d6 = sine offset (-200 to +200)
    addi.l  #200,d6                     ; d6 = 0 to 400
    lsr.l   #4,d6                       ; d6 = 0 to 25 (scroll offset)

.bar_loop:
    ; Colour index = bar_index + scroll_offset + frame/4
    move.l  d0,d2                       ; d2 = frame
    lsr.l   #2,d2                       ; slow down colour cycling
    add.l   d1,d2                       ; + bar_index
    add.l   d6,d2                       ; + sine scroll offset
    andi.l  #$0F,d2                     ; wrap to 16 colours
    lsl.l   #2,d2                       ; * 4 bytes per colour
    move.l  (a1,d2.l),d7                ; d7 = colour value
    move.l  d7,(a0)                     ; store colour in copper list

    ; Advance to next bar
    lea     BAR_STRIDE(a0),a0
    addq.l  #1,d1
    cmpi.l  #BAR_COUNT,d1
    blt     .bar_loop
    rts

; ----------------------------------------------------------------------------
; compute_xy - Calculate sprite position from sine/cosine tables
;
; The sprite follows a Lissajous curve: X uses sin(frame), Y uses
; cos(frame*2). The doubled frequency on Y creates an infinity-symbol
; (figure-8) motion path.
;
; Input:  d0 = frame counter
; Output: d2 = X position, d3 = Y position
; Clobbers: d1, a0
; ----------------------------------------------------------------------------
compute_xy:
    move.l  d0,d1
    andi.l  #$FF,d1
    lsl.l   #2,d1
    lea     data_sin_x,a0
    move.l  (a0,d1.l),d2
    addi.l  #CENTER_X,d2

    move.l  d0,d1
    lsl.l   #1,d1
    andi.l  #$FF,d1
    lsl.l   #2,d1
    lea     data_cos_y,a0
    move.l  (a0,d1.l),d3
    addi.l  #CENTER_Y,d3
    rts

; ----------------------------------------------------------------------------
; clear_scroll_area - Erase the bottom 90 scanlines for scrolltext
; Uses the blitter fill operation to clear Y=390..479 to black.
; ----------------------------------------------------------------------------
clear_scroll_area:
    jsr     wait_blit
    move.l  #BLT_OP_FILL,d0
    move.l  d0,BLT_OP
    move.l  #390,d0
    mulu.w  #LINE_BYTES,d0
    addi.l  #VRAM_START,d0
    move.l  d0,BLT_DST
    move.l  #SCREEN_W,d0
    move.l  d0,BLT_WIDTH
    moveq   #90,d0
    move.l  d0,BLT_HEIGHT
    move.l  #LINE_BYTES,d0
    move.l  d0,BLT_DST_STRIDE
    move.l  #BACKGROUND,d0
    move.l  d0,BLT_COLOR
    moveq   #1,d0
    move.l  d0,BLT_CTRL
    rts

; ----------------------------------------------------------------------------
; draw_scrolltext - Render sine-wave scrolling text using the blitter
;
; Characters are drawn from a pre-rendered RGBA font bitmap. Each
; character's Y position is offset by a sine wave lookup, creating the
; classic demoscene "bouncing scrolltext" effect. The 68020's indexed
; addressing modes make the table lookups particularly efficient.
;
; Register usage:
;   d0 = temp/accumulator
;   d1 = scroll_x value
;   d2 = char index in message
;   d3 = screen X position
;   d4 = char counter
;   d5 = character value (ASCII)
;   d6 = source address (font)
;   d7 = dest address (VRAM)
;   a0 = pointer to message
;   a1 = pointer to char table
;   a2 = pointer to sine table
;   a3 = pointer to font data
; ----------------------------------------------------------------------------
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
    move.b  (a0,d2.l),d5                ; d5 = ASCII character (byte)
    andi.l  #$FF,d5
    tst.l   d5
    beq     .scroll_wrap                ; null terminator -- wrap

    ; Skip if off-screen left
    tst.l   d3
    bmi     .scroll_next

    ; Skip if off-screen right
    cmpi.l  #608,d3
    bge     .scroll_done

    ; Preserve loop state across blitter call
    movem.l d1-d4,-(sp)

    ; Look up character glyph offset in the font atlas
    lea     scroll_char_table,a1
    move.l  d5,d0
    lsl.l   #2,d0
    move.l  (a1,d0.l),d0                ; font offset
    lea     scroll_font_data,a3
    lea     (a3,d0.l),a3
    move.l  a3,d6                       ; d6 = source address

    ; Calculate Y with sine offset for wave motion
    move.l  d3,d0
    add.l   VAR_FRAME_ADDR,d0           ; + frame counter for animated wave
    andi.l  #$FF,d0
    lsl.l   #2,d0
    lea     scroll_sine_table,a2
    move.l  (a2,d0.l),d7                ; d7 = sine offset
    addi.l  #SCROLL_Y,d7                ; d7 = final Y

    ; Calculate destination VRAM address
    move.l  d7,d0
    mulu.w  #LINE_BYTES,d0
    addi.l  #VRAM_START,d0
    move.l  d3,d5
    lsl.l   #2,d5
    add.l   d5,d0
    move.l  d0,d7                       ; d7 = dest address

    ; Blit the character glyph with alpha blending
    jsr     wait_blit
    move.l  #BLT_OP_ALPHA,d0
    move.l  d0,BLT_OP
    move.l  d6,BLT_SRC
    move.l  d7,BLT_DST
    move.l  #CHAR_WIDTH,d0
    move.l  d0,BLT_WIDTH
    move.l  #CHAR_HEIGHT,d0
    move.l  d0,BLT_HEIGHT
    move.l  #FONT_STRIDE,d0
    move.l  d0,BLT_SRC_STRIDE
    move.l  #LINE_BYTES,d0
    move.l  d0,BLT_DST_STRIDE
    moveq   #1,d0
    move.l  d0,BLT_CTRL

    movem.l (sp)+,d1-d4

.scroll_next:
    addq.l  #1,d2                       ; next char in message
    addi.l  #CHAR_WIDTH,d3              ; next screen position
    addq.l  #1,d4
    cmpi.l  #21,d4
    blt     .scroll_loop

.scroll_done:
    rts

.scroll_wrap:
    move.l  VAR_SCROLL_X,d0
    andi.l  #$1F,d0
    move.l  d0,VAR_SCROLL_X
    moveq   #0,d2
    bra     .scroll_loop

; ============================================================================
; DATA SECTION
; Placed after code, referenced by labels. The 68020's flat address space
; means all data is directly accessible without banking.
; ============================================================================

    even

; ----------------------------------------------------------------------------
; Sine table for X movement
; 256 entries, 32-bit signed values scaled to +/-200 pixels
; Pre-computed: sin(i * 2pi / 256) * 200
; ----------------------------------------------------------------------------
data_sin_x:
    dc.l    $00000000
    dc.l    $00000005
    dc.l    $0000000A
    dc.l    $0000000F
    dc.l    $00000014
    dc.l    $00000018
    dc.l    $0000001D
    dc.l    $00000022
    dc.l    $00000027
    dc.l    $0000002C
    dc.l    $00000031
    dc.l    $00000035
    dc.l    $0000003A
    dc.l    $0000003F
    dc.l    $00000043
    dc.l    $00000048
    dc.l    $0000004D
    dc.l    $00000051
    dc.l    $00000056
    dc.l    $0000005A
    dc.l    $0000005E
    dc.l    $00000063
    dc.l    $00000067
    dc.l    $0000006B
    dc.l    $0000006F
    dc.l    $00000073
    dc.l    $00000077
    dc.l    $0000007B
    dc.l    $0000007F
    dc.l    $00000083
    dc.l    $00000086
    dc.l    $0000008A
    dc.l    $0000008D
    dc.l    $00000091
    dc.l    $00000094
    dc.l    $00000097
    dc.l    $0000009B
    dc.l    $0000009E
    dc.l    $000000A1
    dc.l    $000000A4
    dc.l    $000000A6
    dc.l    $000000A9
    dc.l    $000000AC
    dc.l    $000000AE
    dc.l    $000000B0
    dc.l    $000000B3
    dc.l    $000000B5
    dc.l    $000000B7
    dc.l    $000000B9
    dc.l    $000000BB
    dc.l    $000000BC
    dc.l    $000000BE
    dc.l    $000000BF
    dc.l    $000000C1
    dc.l    $000000C2
    dc.l    $000000C3
    dc.l    $000000C4
    dc.l    $000000C5
    dc.l    $000000C6
    dc.l    $000000C6
    dc.l    $000000C7
    dc.l    $000000C7
    dc.l    $000000C8
    dc.l    $000000C8
    dc.l    $000000C8
    dc.l    $000000C8
    dc.l    $000000C8
    dc.l    $000000C7
    dc.l    $000000C7
    dc.l    $000000C6
    dc.l    $000000C6
    dc.l    $000000C5
    dc.l    $000000C4
    dc.l    $000000C3
    dc.l    $000000C2
    dc.l    $000000C1
    dc.l    $000000BF
    dc.l    $000000BE
    dc.l    $000000BC
    dc.l    $000000BB
    dc.l    $000000B9
    dc.l    $000000B7
    dc.l    $000000B5
    dc.l    $000000B3
    dc.l    $000000B0
    dc.l    $000000AE
    dc.l    $000000AC
    dc.l    $000000A9
    dc.l    $000000A6
    dc.l    $000000A4
    dc.l    $000000A1
    dc.l    $0000009E
    dc.l    $0000009B
    dc.l    $00000097
    dc.l    $00000094
    dc.l    $00000091
    dc.l    $0000008D
    dc.l    $0000008A
    dc.l    $00000086
    dc.l    $00000083
    dc.l    $0000007F
    dc.l    $0000007B
    dc.l    $00000077
    dc.l    $00000073
    dc.l    $0000006F
    dc.l    $0000006B
    dc.l    $00000067
    dc.l    $00000063
    dc.l    $0000005E
    dc.l    $0000005A
    dc.l    $00000056
    dc.l    $00000051
    dc.l    $0000004D
    dc.l    $00000048
    dc.l    $00000043
    dc.l    $0000003F
    dc.l    $0000003A
    dc.l    $00000035
    dc.l    $00000031
    dc.l    $0000002C
    dc.l    $00000027
    dc.l    $00000022
    dc.l    $0000001D
    dc.l    $00000018
    dc.l    $00000014
    dc.l    $0000000F
    dc.l    $0000000A
    dc.l    $00000005
    dc.l    $00000000
    dc.l    $FFFFFFFB
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFEC
    dc.l    $FFFFFFE8
    dc.l    $FFFFFFE3
    dc.l    $FFFFFFDE
    dc.l    $FFFFFFD9
    dc.l    $FFFFFFD4
    dc.l    $FFFFFFCF
    dc.l    $FFFFFFCB
    dc.l    $FFFFFFC6
    dc.l    $FFFFFFC1
    dc.l    $FFFFFFBD
    dc.l    $FFFFFFB8
    dc.l    $FFFFFFB3
    dc.l    $FFFFFFAF
    dc.l    $FFFFFFAA
    dc.l    $FFFFFFA6
    dc.l    $FFFFFFA2
    dc.l    $FFFFFF9D
    dc.l    $FFFFFF99
    dc.l    $FFFFFF95
    dc.l    $FFFFFF91
    dc.l    $FFFFFF8D
    dc.l    $FFFFFF89
    dc.l    $FFFFFF85
    dc.l    $FFFFFF81
    dc.l    $FFFFFF7D
    dc.l    $FFFFFF7A
    dc.l    $FFFFFF76
    dc.l    $FFFFFF73
    dc.l    $FFFFFF6F
    dc.l    $FFFFFF6C
    dc.l    $FFFFFF69
    dc.l    $FFFFFF65
    dc.l    $FFFFFF62
    dc.l    $FFFFFF5F
    dc.l    $FFFFFF5C
    dc.l    $FFFFFF5A
    dc.l    $FFFFFF57
    dc.l    $FFFFFF54
    dc.l    $FFFFFF52
    dc.l    $FFFFFF50
    dc.l    $FFFFFF4D
    dc.l    $FFFFFF4B
    dc.l    $FFFFFF49
    dc.l    $FFFFFF47
    dc.l    $FFFFFF45
    dc.l    $FFFFFF44
    dc.l    $FFFFFF42
    dc.l    $FFFFFF41
    dc.l    $FFFFFF3F
    dc.l    $FFFFFF3E
    dc.l    $FFFFFF3D
    dc.l    $FFFFFF3C
    dc.l    $FFFFFF3B
    dc.l    $FFFFFF3A
    dc.l    $FFFFFF3A
    dc.l    $FFFFFF39
    dc.l    $FFFFFF39
    dc.l    $FFFFFF38
    dc.l    $FFFFFF38
    dc.l    $FFFFFF38
    dc.l    $FFFFFF38
    dc.l    $FFFFFF38
    dc.l    $FFFFFF39
    dc.l    $FFFFFF39
    dc.l    $FFFFFF3A
    dc.l    $FFFFFF3A
    dc.l    $FFFFFF3B
    dc.l    $FFFFFF3C
    dc.l    $FFFFFF3D
    dc.l    $FFFFFF3E
    dc.l    $FFFFFF3F
    dc.l    $FFFFFF41
    dc.l    $FFFFFF42
    dc.l    $FFFFFF44
    dc.l    $FFFFFF45
    dc.l    $FFFFFF47
    dc.l    $FFFFFF49
    dc.l    $FFFFFF4B
    dc.l    $FFFFFF4D
    dc.l    $FFFFFF50
    dc.l    $FFFFFF52
    dc.l    $FFFFFF54
    dc.l    $FFFFFF57
    dc.l    $FFFFFF5A
    dc.l    $FFFFFF5C
    dc.l    $FFFFFF5F
    dc.l    $FFFFFF62
    dc.l    $FFFFFF65
    dc.l    $FFFFFF69
    dc.l    $FFFFFF6C
    dc.l    $FFFFFF6F
    dc.l    $FFFFFF73
    dc.l    $FFFFFF76
    dc.l    $FFFFFF7A
    dc.l    $FFFFFF7D
    dc.l    $FFFFFF81
    dc.l    $FFFFFF85
    dc.l    $FFFFFF89
    dc.l    $FFFFFF8D
    dc.l    $FFFFFF91
    dc.l    $FFFFFF95
    dc.l    $FFFFFF99
    dc.l    $FFFFFF9D
    dc.l    $FFFFFFA2
    dc.l    $FFFFFFA6
    dc.l    $FFFFFFAA
    dc.l    $FFFFFFAF
    dc.l    $FFFFFFB3
    dc.l    $FFFFFFB8
    dc.l    $FFFFFFBD
    dc.l    $FFFFFFC1
    dc.l    $FFFFFFC6
    dc.l    $FFFFFFCB
    dc.l    $FFFFFFCF
    dc.l    $FFFFFFD4
    dc.l    $FFFFFFD9
    dc.l    $FFFFFFDE
    dc.l    $FFFFFFE3
    dc.l    $FFFFFFE8
    dc.l    $FFFFFFEC
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFFB

; ----------------------------------------------------------------------------
; Cosine table for Y movement
; 256 entries, 32-bit signed values scaled to +/-150 pixels
; Pre-computed: cos(i * 2pi / 256) * 150
; ----------------------------------------------------------------------------
data_cos_y:
    dc.l    $00000096
    dc.l    $00000096
    dc.l    $00000096
    dc.l    $00000096
    dc.l    $00000095
    dc.l    $00000095
    dc.l    $00000094
    dc.l    $00000094
    dc.l    $00000093
    dc.l    $00000092
    dc.l    $00000092
    dc.l    $00000091
    dc.l    $00000090
    dc.l    $0000008E
    dc.l    $0000008D
    dc.l    $0000008C
    dc.l    $0000008B
    dc.l    $00000089
    dc.l    $00000088
    dc.l    $00000086
    dc.l    $00000084
    dc.l    $00000083
    dc.l    $00000081
    dc.l    $0000007F
    dc.l    $0000007D
    dc.l    $0000007B
    dc.l    $00000078
    dc.l    $00000076
    dc.l    $00000074
    dc.l    $00000072
    dc.l    $0000006F
    dc.l    $0000006D
    dc.l    $0000006A
    dc.l    $00000067
    dc.l    $00000065
    dc.l    $00000062
    dc.l    $0000005F
    dc.l    $0000005C
    dc.l    $00000059
    dc.l    $00000056
    dc.l    $00000053
    dc.l    $00000050
    dc.l    $0000004D
    dc.l    $0000004A
    dc.l    $00000047
    dc.l    $00000043
    dc.l    $00000040
    dc.l    $0000003D
    dc.l    $00000039
    dc.l    $00000036
    dc.l    $00000033
    dc.l    $0000002F
    dc.l    $0000002C
    dc.l    $00000028
    dc.l    $00000024
    dc.l    $00000021
    dc.l    $0000001D
    dc.l    $0000001A
    dc.l    $00000016
    dc.l    $00000012
    dc.l    $0000000F
    dc.l    $0000000B
    dc.l    $00000007
    dc.l    $00000004
    dc.l    $00000000
    dc.l    $FFFFFFFC
    dc.l    $FFFFFFF9
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEA
    dc.l    $FFFFFFE6
    dc.l    $FFFFFFE3
    dc.l    $FFFFFFDF
    dc.l    $FFFFFFDC
    dc.l    $FFFFFFD8
    dc.l    $FFFFFFD4
    dc.l    $FFFFFFD1
    dc.l    $FFFFFFCD
    dc.l    $FFFFFFCA
    dc.l    $FFFFFFC7
    dc.l    $FFFFFFC3
    dc.l    $FFFFFFC0
    dc.l    $FFFFFFBD
    dc.l    $FFFFFFB9
    dc.l    $FFFFFFB6
    dc.l    $FFFFFFB3
    dc.l    $FFFFFFB0
    dc.l    $FFFFFFAD
    dc.l    $FFFFFFAA
    dc.l    $FFFFFFA7
    dc.l    $FFFFFFA4
    dc.l    $FFFFFFA1
    dc.l    $FFFFFF9E
    dc.l    $FFFFFF9B
    dc.l    $FFFFFF99
    dc.l    $FFFFFF96
    dc.l    $FFFFFF93
    dc.l    $FFFFFF91
    dc.l    $FFFFFF8E
    dc.l    $FFFFFF8C
    dc.l    $FFFFFF8A
    dc.l    $FFFFFF88
    dc.l    $FFFFFF85
    dc.l    $FFFFFF83
    dc.l    $FFFFFF81
    dc.l    $FFFFFF7F
    dc.l    $FFFFFF7D
    dc.l    $FFFFFF7C
    dc.l    $FFFFFF7A
    dc.l    $FFFFFF78
    dc.l    $FFFFFF77
    dc.l    $FFFFFF75
    dc.l    $FFFFFF74
    dc.l    $FFFFFF73
    dc.l    $FFFFFF72
    dc.l    $FFFFFF70
    dc.l    $FFFFFF6F
    dc.l    $FFFFFF6E
    dc.l    $FFFFFF6E
    dc.l    $FFFFFF6D
    dc.l    $FFFFFF6C
    dc.l    $FFFFFF6C
    dc.l    $FFFFFF6B
    dc.l    $FFFFFF6B
    dc.l    $FFFFFF6A
    dc.l    $FFFFFF6A
    dc.l    $FFFFFF6A
    dc.l    $FFFFFF6A
    dc.l    $FFFFFF6A
    dc.l    $FFFFFF6A
    dc.l    $FFFFFF6A
    dc.l    $FFFFFF6B
    dc.l    $FFFFFF6B
    dc.l    $FFFFFF6C
    dc.l    $FFFFFF6C
    dc.l    $FFFFFF6D
    dc.l    $FFFFFF6E
    dc.l    $FFFFFF6E
    dc.l    $FFFFFF6F
    dc.l    $FFFFFF70
    dc.l    $FFFFFF72
    dc.l    $FFFFFF73
    dc.l    $FFFFFF74
    dc.l    $FFFFFF75
    dc.l    $FFFFFF77
    dc.l    $FFFFFF78
    dc.l    $FFFFFF7A
    dc.l    $FFFFFF7C
    dc.l    $FFFFFF7D
    dc.l    $FFFFFF7F
    dc.l    $FFFFFF81
    dc.l    $FFFFFF83
    dc.l    $FFFFFF85
    dc.l    $FFFFFF88
    dc.l    $FFFFFF8A
    dc.l    $FFFFFF8C
    dc.l    $FFFFFF8E
    dc.l    $FFFFFF91
    dc.l    $FFFFFF93
    dc.l    $FFFFFF96
    dc.l    $FFFFFF99
    dc.l    $FFFFFF9B
    dc.l    $FFFFFF9E
    dc.l    $FFFFFFA1
    dc.l    $FFFFFFA4
    dc.l    $FFFFFFA7
    dc.l    $FFFFFFAA
    dc.l    $FFFFFFAD
    dc.l    $FFFFFFB0
    dc.l    $FFFFFFB3
    dc.l    $FFFFFFB6
    dc.l    $FFFFFFB9
    dc.l    $FFFFFFBD
    dc.l    $FFFFFFC0
    dc.l    $FFFFFFC3
    dc.l    $FFFFFFC7
    dc.l    $FFFFFFCA
    dc.l    $FFFFFFCD
    dc.l    $FFFFFFD1
    dc.l    $FFFFFFD4
    dc.l    $FFFFFFD8
    dc.l    $FFFFFFDC
    dc.l    $FFFFFFDF
    dc.l    $FFFFFFE3
    dc.l    $FFFFFFE6
    dc.l    $FFFFFFEA
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF9
    dc.l    $FFFFFFFC
    dc.l    $00000000
    dc.l    $00000004
    dc.l    $00000007
    dc.l    $0000000B
    dc.l    $0000000F
    dc.l    $00000012
    dc.l    $00000016
    dc.l    $0000001A
    dc.l    $0000001D
    dc.l    $00000021
    dc.l    $00000024
    dc.l    $00000028
    dc.l    $0000002C
    dc.l    $0000002F
    dc.l    $00000033
    dc.l    $00000036
    dc.l    $00000039
    dc.l    $0000003D
    dc.l    $00000040
    dc.l    $00000043
    dc.l    $00000047
    dc.l    $0000004A
    dc.l    $0000004D
    dc.l    $00000050
    dc.l    $00000053
    dc.l    $00000056
    dc.l    $00000059
    dc.l    $0000005C
    dc.l    $0000005F
    dc.l    $00000062
    dc.l    $00000065
    dc.l    $00000067
    dc.l    $0000006A
    dc.l    $0000006D
    dc.l    $0000006F
    dc.l    $00000072
    dc.l    $00000074
    dc.l    $00000076
    dc.l    $00000078
    dc.l    $0000007B
    dc.l    $0000007D
    dc.l    $0000007F
    dc.l    $00000081
    dc.l    $00000083
    dc.l    $00000084
    dc.l    $00000086
    dc.l    $00000088
    dc.l    $00000089
    dc.l    $0000008B
    dc.l    $0000008C
    dc.l    $0000008D
    dc.l    $0000008E
    dc.l    $00000090
    dc.l    $00000091
    dc.l    $00000092
    dc.l    $00000092
    dc.l    $00000093
    dc.l    $00000094
    dc.l    $00000094
    dc.l    $00000095
    dc.l    $00000095
    dc.l    $00000096
    dc.l    $00000096
    dc.l    $00000096

; ----------------------------------------------------------------------------
; Colour palette for copper bars (16 entries, BGRA format)
; These colours form a rainbow gradient that the update_bars routine
; cycles through each frame.
; ----------------------------------------------------------------------------
data_palette:
    dc.l    $FF0000FF
    dc.l    $FF0040FF
    dc.l    $FF0080FF
    dc.l    $FF00C0FF
    dc.l    $FF00FF80
    dc.l    $FF00FF00
    dc.l    $FF40FF00
    dc.l    $FF80FF00
    dc.l    $FFFFFF00
    dc.l    $FFFFC000
    dc.l    $FFFF8000
    dc.l    $FFFF4000
    dc.l    $FFFF0000
    dc.l    $FFFF00FF
    dc.l    $FF8000FF
    dc.l    $FF4000FF

; ----------------------------------------------------------------------------
; Copper list - 16 horizontal raster bars
; Each bar consists of 9 longwords:
;   WAIT scanline, MOVE raster_y, MOVE raster_height, MOVE colour, MOVE ctrl
; The colour field at offset 24 is updated dynamically each frame.
; ----------------------------------------------------------------------------
data_copper_list:
    dc.l    40*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    40
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF0000FF
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    64*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    64
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF0040FF
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    88*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    88
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF0080FF
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    112*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    112
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF00C0FF
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    136*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    136
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF00FF80
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    160*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    160
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF00FF00
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    184*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    184
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF40FF00
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    208*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    208
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF80FF00
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    232*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    232
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FFFFFF00
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    256*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    256
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FFFFC000
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    280*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    280
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FFFF8000
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    304*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    304
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FFFF4000
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    328*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    328
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FFFF0000
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    352*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    352
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FFFF00FF
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    376*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    376
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF8000FF
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    400*COP_WAIT_SCALE
    dc.l    COP_MOVE_RASTER_Y
    dc.l    400
    dc.l    COP_MOVE_RASTER_H
    dc.l    12
    dc.l    COP_MOVE_RASTER_COLOR
    dc.l    $FF4000FF
    dc.l    COP_MOVE_RASTER_CTRL
    dc.l    1
    dc.l    COP_END

; ----------------------------------------------------------------------------
; Embedded binary data
; Sprite RGBA (240x180x4 = 172,800 bytes), mask, AY music file
; ----------------------------------------------------------------------------
data_robocop_rgba:
    incbin  "../assets/robocop_rgba.bin"

data_robocop_mask:
    incbin  "../assets/robocop_mask.bin"

data_robocop_ay:
    incbin  "../assets/music/Robocop1.ay"

; ============================================================================
; SCROLLTEXT DATA
; ============================================================================

    even                ; Ensure proper alignment for 32-bit table access

; ----------------------------------------------------------------------------
; Scrolltext sine table
; 256 entries of pre-calculated Y offsets: sin(i * 2pi / 256) * 20
; Range: -20 to +20 pixels. No runtime division needed.
; ----------------------------------------------------------------------------
scroll_sine_table:
    dc.l    $00000000
    dc.l    $00000000
    dc.l    $00000001
    dc.l    $00000001
    dc.l    $00000002
    dc.l    $00000002
    dc.l    $00000003
    dc.l    $00000003
    dc.l    $00000004
    dc.l    $00000004
    dc.l    $00000005
    dc.l    $00000005
    dc.l    $00000006
    dc.l    $00000006
    dc.l    $00000006
    dc.l    $00000007
    dc.l    $00000007
    dc.l    $00000008
    dc.l    $00000008
    dc.l    $00000009
    dc.l    $00000009
    dc.l    $00000009
    dc.l    $0000000A
    dc.l    $0000000A
    dc.l    $0000000A
    dc.l    $0000000B
    dc.l    $0000000B
    dc.l    $0000000B
    dc.l    $0000000C
    dc.l    $0000000C
    dc.l    $0000000C
    dc.l    $0000000D
    dc.l    $0000000D
    dc.l    $0000000D
    dc.l    $0000000D
    dc.l    $0000000E
    dc.l    $0000000E
    dc.l    $0000000E
    dc.l    $0000000E
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000012
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000011
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $00000010
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $0000000F
    dc.l    $0000000E
    dc.l    $0000000E
    dc.l    $0000000E
    dc.l    $0000000E
    dc.l    $0000000D
    dc.l    $0000000D
    dc.l    $0000000D
    dc.l    $0000000D
    dc.l    $0000000C
    dc.l    $0000000C
    dc.l    $0000000C
    dc.l    $0000000B
    dc.l    $0000000B
    dc.l    $0000000B
    dc.l    $0000000A
    dc.l    $0000000A
    dc.l    $0000000A
    dc.l    $00000009
    dc.l    $00000009
    dc.l    $00000009
    dc.l    $00000008
    dc.l    $00000008
    dc.l    $00000007
    dc.l    $00000007
    dc.l    $00000006
    dc.l    $00000006
    dc.l    $00000006
    dc.l    $00000005
    dc.l    $00000005
    dc.l    $00000004
    dc.l    $00000004
    dc.l    $00000003
    dc.l    $00000003
    dc.l    $00000002
    dc.l    $00000002
    dc.l    $00000001
    dc.l    $00000001
    dc.l    $00000000
    dc.l    $00000000
    dc.l    $FFFFFFFF
    dc.l    $FFFFFFFF
    dc.l    $FFFFFFFE
    dc.l    $FFFFFFFE
    dc.l    $FFFFFFFD
    dc.l    $FFFFFFFD
    dc.l    $FFFFFFFC
    dc.l    $FFFFFFFC
    dc.l    $FFFFFFFB
    dc.l    $FFFFFFFB
    dc.l    $FFFFFFFA
    dc.l    $FFFFFFFA
    dc.l    $FFFFFFFA
    dc.l    $FFFFFFF9
    dc.l    $FFFFFFF9
    dc.l    $FFFFFFF8
    dc.l    $FFFFFFF8
    dc.l    $FFFFFFF7
    dc.l    $FFFFFFF7
    dc.l    $FFFFFFF7
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF4
    dc.l    $FFFFFFF4
    dc.l    $FFFFFFF4
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEE
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFEF
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF0
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF1
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF2
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF3
    dc.l    $FFFFFFF4
    dc.l    $FFFFFFF4
    dc.l    $FFFFFFF4
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF5
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFF6
    dc.l    $FFFFFFF7
    dc.l    $FFFFFFF7
    dc.l    $FFFFFFF7
    dc.l    $FFFFFFF8
    dc.l    $FFFFFFF8
    dc.l    $FFFFFFF9
    dc.l    $FFFFFFF9
    dc.l    $FFFFFFFA
    dc.l    $FFFFFFFA
    dc.l    $FFFFFFFA
    dc.l    $FFFFFFFB
    dc.l    $FFFFFFFB
    dc.l    $FFFFFFFC
    dc.l    $FFFFFFFC
    dc.l    $FFFFFFFD
    dc.l    $FFFFFFFD
    dc.l    $FFFFFFFE
    dc.l    $FFFFFFFE
    dc.l    $FFFFFFFF
    dc.l    $FFFFFFFF

; ----------------------------------------------------------------------------
; Character lookup table
; Maps ASCII codes (0-127) to byte offsets into the font RGBA bitmap.
; Each entry is a 32-bit offset. Zero means no glyph available.
; ----------------------------------------------------------------------------
scroll_char_table:
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    128
    dc.l    256
    dc.l    0
    dc.l    0
    dc.l    0
    dc.l    40960
    dc.l    896
    dc.l    1024
    dc.l    1152
    dc.l    512
    dc.l    41600
    dc.l    41216
    dc.l    41344
    dc.l    41472
    dc.l    0
    dc.l    41728
    dc.l    41856
    dc.l    41984
    dc.l    42112
    dc.l    81920
    dc.l    82048
    dc.l    82176
    dc.l    82304
    dc.l    82432
    dc.l    82560
    dc.l    82688
    dc.l    82816
    dc.l    0
    dc.l    82944
    dc.l    0
    dc.l    122880
    dc.l    384
    dc.l    123264
    dc.l    123392
    dc.l    123520
    dc.l    123648
    dc.l    123776
    dc.l    123904
    dc.l    124032
    dc.l    163840
    dc.l    163968
    dc.l    164096
    dc.l    164224
    dc.l    164352
    dc.l    164480
    dc.l    164608
    dc.l    164736
    dc.l    164864
    dc.l    164992
    dc.l    204800
    dc.l    204928
    dc.l    205056
    dc.l    205184
    dc.l    205312
    dc.l    205440
    dc.l    205568
    dc.l    205696
    dc.l    205824
    dc.l    83072
    dc.l    0
    dc.l    122752
    dc.l    768
    dc.l    0
    dc.l    205952
    dc.l    123264
    dc.l    123392
    dc.l    123520
    dc.l    123648
    dc.l    123776
    dc.l    123904
    dc.l    124032
    dc.l    163840
    dc.l    163968
    dc.l    164096
    dc.l    164224
    dc.l    164352
    dc.l    164480
    dc.l    164608
    dc.l    164736
    dc.l    164864
    dc.l    164992
    dc.l    204800
    dc.l    204928
    dc.l    205056
    dc.l    205184
    dc.l    205312
    dc.l    205440
    dc.l    205568
    dc.l    205696
    dc.l    205824
    dc.l    123008
    dc.l    0
    dc.l    0
    dc.l    41088
    dc.l    0

; ----------------------------------------------------------------------------
; Font RGBA bitmap (pre-rendered 32x32 character glyphs, 10 chars per row)
; ----------------------------------------------------------------------------
scroll_font_data:
    incbin  "../assets/font_rgba.bin"

; ----------------------------------------------------------------------------
; Scroll message (null-terminated ASCII)
; ----------------------------------------------------------------------------
scroll_message:
    dc.b    "    ...ROBOCOP DUAL CPU 68020 AND Z80 INTRO FOR THE INTUITION ENGINE... ...100 PERCENT ASM CODE... ...68020 ASM FOR DEMO EFFECTS... ...Z80 ASM FOR MUSIC REPLAY ROUTINE... ...ALL CODE BY INTUITON...  MUSIC BY JONATHAN DUNN FROM THE 1987 ZX SPECTRUM GAME ROBOCOP BY OCEAN SOFTWARE... ...AY REGISTERS ARE REMAPPED TO THE INTUITON ENGINE SYNTH FOR SUPERIOR SOUND QUALITY... ...GREETS TO ...GADGETMASTER... ...KARLOS... ...BLOODLINE... ...VISIT INTUITIONSUBSYNTH.COM......................."
    dc.b    0
    even
