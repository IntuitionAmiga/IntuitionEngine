; ============================================================================
; ROBOCOP INTRO - 68020
; Moves robocop sprite with the blitter, animated copper RGB bars, PSG+ AY.
; Port from IE32 assembly to Motorola 68020
; ============================================================================
; Assemble with: vasmm68k_mot -Fbin -m68020 -devpac -o robocop_intro_68k.ie68 robocop_intro_68k.asm
; Run with: ./IntuitionEngine -m68k robocop_intro_68k.ie68

; ----------------------------------------------------------------------------
; HARDWARE REGISTERS (I/O region at $F0000)
; ----------------------------------------------------------------------------
VIDEO_CTRL      equ $F0000
VIDEO_MODE      equ $F0004
VIDEO_STATUS    equ $F0008
STATUS_VBLANK   equ 2
COPPER_CTRL     equ $F000C
COPPER_PTR      equ $F0010
BLT_CTRL        equ $F001C
BLT_OP          equ $F0020
BLT_SRC         equ $F0024
BLT_DST         equ $F0028
BLT_WIDTH       equ $F002C
BLT_HEIGHT      equ $F0030
BLT_SRC_STRIDE  equ $F0034
BLT_DST_STRIDE  equ $F0038
BLT_COLOR       equ $F003C
BLT_MASK        equ $F0040
BLT_STATUS      equ $F0044
VIDEO_RASTER_Y      equ $F0048
VIDEO_RASTER_HEIGHT equ $F004C
VIDEO_RASTER_COLOR  equ $F0050
VIDEO_RASTER_CTRL   equ $F0054

PSG_PLUS_CTRL   equ $F0C0E
PSG_PLAY_PTR    equ $F0C10
PSG_PLAY_LEN    equ $F0C14
PSG_PLAY_CTRL   equ $F0C18

; ----------------------------------------------------------------------------
; CONSTANTS
; ----------------------------------------------------------------------------
VRAM_START      equ $100000
SCREEN_W        equ 640
SCREEN_H        equ 480
LINE_BYTES      equ 2560
SPRITE_W        equ 240
SPRITE_H        equ 180
SPRITE_STRIDE   equ 960
CENTER_X        equ 200
CENTER_Y        equ 150
BACKGROUND      equ $FF000000

BLT_OP_COPY     equ 0
BLT_OP_FILL     equ 1
BLT_OP_LINE     equ 2
BLT_OP_MASKED   equ 3
BLT_OP_ALPHA    equ 4

BAR_COUNT       equ 16
BAR_STRIDE      equ 36
BAR_WAIT_OFFSET equ 0
BAR_Y_OFFSET    equ 8
BAR_HEIGHT_OFFSET equ 16
BAR_COLOR_OFFSET  equ 24
BAR_HEIGHT      equ 12
BAR_SPACING     equ 20

VAR_FRAME_ADDR  equ $8800
VAR_PREV_X_ADDR equ $8804
VAR_PREV_Y_ADDR equ $8808
VAR_SCROLL_X    equ $880C

ROBOCOP_AY_LEN  equ 24525

; Scrolltext constants
SCROLL_Y        equ 430
SCROLL_SPEED    equ 4
CHAR_WIDTH      equ 32
CHAR_HEIGHT     equ 32
FONT_STRIDE     equ 1280

; ----------------------------------------------------------------------------
; CODE SECTION
; ----------------------------------------------------------------------------
    section code
    org     $001000              ; M68K programs load at $1000

start:
    ; 640x480 mode, enable video
    moveq   #0,d0
    move.l  d0,VIDEO_MODE
    moveq   #1,d0
    move.l  d0,VIDEO_CTRL

    ; Clear screen once
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

    ; Enable PSG+ and start AY playback with looping
    moveq   #1,d0
    move.l  d0,PSG_PLUS_CTRL
    lea     data_robocop_ay,a0
    move.l  a0,d0
    move.l  d0,PSG_PLAY_PTR
    move.l  #ROBOCOP_AY_LEN,d0
    move.l  d0,PSG_PLAY_LEN
    moveq   #5,d0                   ; bit0=start, bit2=loop
    move.l  d0,PSG_PLAY_CTRL

    ; Setup copper list
    moveq   #2,d0
    move.l  d0,COPPER_CTRL
    lea     data_copper_list,a0
    move.l  a0,d0
    move.l  d0,COPPER_PTR
    moveq   #1,d0
    move.l  d0,COPPER_CTRL

    ; Init frame counter and prev position
    moveq   #0,d0
    move.l  d0,VAR_FRAME_ADDR
    jsr     compute_xy              ; d2=X, d3=Y
    move.l  d2,VAR_PREV_X_ADDR
    move.l  d3,VAR_PREV_Y_ADDR

    ; Init scrolltext
    moveq   #0,d0
    move.l  d0,VAR_SCROLL_X

main_loop:
    ; Advance frame
    move.l  VAR_FRAME_ADDR,d0
    addq.l  #1,d0
    move.l  d0,VAR_FRAME_ADDR

    ; Update copper bar colors
    move.l  VAR_FRAME_ADDR,d0
    jsr     update_bars

    ; Compute new sprite position (d2=x, d3=y)
    move.l  VAR_FRAME_ADDR,d0
    jsr     compute_xy

    ; Compute previous address -> d6
    move.l  VAR_PREV_Y_ADDR,d4
    move.l  #LINE_BYTES,d5
    mulu.w  d5,d4
    move.l  VAR_PREV_X_ADDR,d5
    lsl.l   #2,d5
    add.l   d5,d4
    addi.l  #VRAM_START,d4
    move.l  d4,d6                   ; d6 = prev address

    ; Compute new address -> d7
    move.l  d3,d4
    move.l  #LINE_BYTES,d5
    mulu.w  d5,d4
    move.l  d2,d5
    lsl.l   #2,d5
    add.l   d5,d4
    addi.l  #VRAM_START,d4
    move.l  d4,d7                   ; d7 = new address

    ; Store current position as previous
    move.l  d2,VAR_PREV_X_ADDR
    move.l  d3,VAR_PREV_Y_ADDR

    ; Wait for next frame (VBlank transition) before drawing
    jsr     wait_frame

    ; Clear previous sprite rect
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

    ; Blit sprite with mask
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

    ; Scrolltext
    jsr     clear_scroll_area
    jsr     draw_scrolltext
    move.l  VAR_SCROLL_X,d0
    addq.l  #SCROLL_SPEED,d0
    move.l  d0,VAR_SCROLL_X

    bra     main_loop

; ----------------------------------------------------------------------------
; Subroutines
; ----------------------------------------------------------------------------
wait_blit:
    move.l  BLT_CTRL,d0
    andi.l  #2,d0
    bne     wait_blit
    rts

wait_vblank:
    ; Wait for VBlank to start (bit 1 of VIDEO_STATUS)
    move.l  VIDEO_STATUS,d0
    andi.l  #STATUS_VBLANK,d0
    beq     wait_vblank
    rts

wait_frame:
    ; Wait for a complete frame boundary (VBlank transition)
    ; First wait for VBlank to END (active scan period)
    ; Then wait for VBlank to START (new frame)
.wait_not_vblank:
    move.l  VIDEO_STATUS,d0
    andi.l  #STATUS_VBLANK,d0
    bne     .wait_not_vblank
.wait_vblank_start:
    move.l  VIDEO_STATUS,d0
    andi.l  #STATUS_VBLANK,d0
    beq     .wait_vblank_start
    rts

update_bars:
    ; d0 = frame counter
    ; Scrolling gradient effect: colors flow through bars like a wave
    moveq   #0,d1                       ; d1 = bar index (0-15)
    lea     data_copper_list,a0
    lea     BAR_COLOR_OFFSET(a0),a0     ; point to first color value
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
    ; Color index = bar_index + scroll_offset + frame/4
    move.l  d0,d2                       ; d2 = frame
    lsr.l   #2,d2                       ; slow down color cycling
    add.l   d1,d2                       ; + bar_index
    add.l   d6,d2                       ; + sine scroll offset
    andi.l  #$0F,d2                     ; wrap to 16 colors
    lsl.l   #2,d2                       ; * 4 bytes per color
    move.l  (a1,d2.l),d7                ; d7 = color value
    move.l  d7,(a0)                     ; store color in copper list

    ; Next bar
    lea     BAR_STRIDE(a0),a0
    addq.l  #1,d1
    cmpi.l  #BAR_COUNT,d1
    blt     .bar_loop
    rts

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
    lsl.l   #1,d1
    andi.l  #$FF,d1
    lsl.l   #2,d1
    lea     data_cos_y,a0
    move.l  (a0,d1.l),d3
    addi.l  #CENTER_Y,d3
    rts

; ----------------------------------------------------------------------------
; clear_scroll_area - Clear bottom of screen for scrolltext
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
; draw_scrolltext - Render sine wave scrolling text
; ----------------------------------------------------------------------------
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

    ; Calculate Y with sine (pre-calculated offsets)
    move.l  d3,d0
    add.l   VAR_FRAME_ADDR,d0           ; + frame counter for animated wave
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

    ; Blit char
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

; ----------------------------------------------------------------------------
; DATA SECTION
; Placed after code, referenced by labels
; ----------------------------------------------------------------------------

    even

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

data_copper_list:
; 16 bars: height=12, spacing=24, Y from 40 to 400
; Each bar: WAIT, MOVE_Y, y_val, MOVE_H, height, MOVE_COLOR, color, MOVE_CTRL, 1
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

data_robocop_rgba:
    incbin  "../robocop_rgba.bin"

data_robocop_mask:
    incbin  "../robocop_mask.bin"

data_robocop_ay:
    incbin  "../Robocop1.ay"

; ============================================================================
; SCROLLTEXT DATA
; ============================================================================

    even                ; Ensure proper alignment for 32-bit table access

; Pre-calculated Y offsets: sin(i * 2pi / 256) * 20, range -20 to +20
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

scroll_font_data:
    incbin  "font_rgba.bin"

scroll_message:
    dc.b    "    ...ROBOCOP DUAL CPU 68020 AND Z80 INTRO FOR THE INTUITION ENGINE... ...100 PERCENT ASM CODE... ...68020 ASM FOR DEMO EFFECTS... ...Z80 ASM FOR MUSIC REPLAY ROUTINE... ...ALL CODE BY INTUITON...  MUSIC BY JONATHAN DUNN FROM THE 1987 ZX SPECTRUM GAME ROBOCOP BY OCEAN SOFTWARE... ...AY REGISTERS ARE REMAPPED TO THE INTUITON ENGINE SYNTH FOR SUPERIOR SOUND QUALITY... ...GREETS TO ...GADGETMASTER... ...KARLOS... ...BLOODLINE... ...VISIT INTUITIONSUBSYNTH.COM......................."
    dc.b    0
    even
