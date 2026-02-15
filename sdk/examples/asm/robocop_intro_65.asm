; ============================================================================
; ROBOCOP INTRO (6502 PORT) - Blitter Sprite, Copper Rasterbars and PSG Music
; cc65 assembly for IntuitionEngine - VideoChip + Copper + PSG audio
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    MOS 6502 (8-bit, extended banking)
; Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true colour)
; Audio Engine:  PSG (AY-3-8910 compatible, PSG+ enhanced mode)
; Assembler:     ca65/ld65 (cc65 toolchain)
; Build:         make ie65asm SRC=sdk/examples/asm/robocop_intro_65.asm
; Run:           ./IntuitionEngine -6502 robocop_intro_65.ie65
; Porting:       See robocop_intro.asm (IE32 reference), robocop_intro_68k.asm
;                (M68K), robocop_intro_z80.asm (Z80)
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
; === 6502-SPECIFIC NOTES ===
; The 6502 is an 8-bit processor with a 16-bit address bus (64KB). All
; 32-bit hardware register writes must be done one byte at a time. The
; Intuition Engine extends the 6502's address space using a banking system:
; data larger than 64KB (sprite, mask, music, font) is placed in extended
; banks and referenced by 32-bit addresses that the blitter can access
; directly. Sine/cosine tables are split into separate low-byte and
; high-byte arrays for efficient 8-bit indexed lookup. A pre-computed
; Y-address table avoids expensive runtime multiplication (y * 2560).
; Helper macros from ie65.inc (SET_BLT_OP, SET_BLT_WIDTH, etc.) handle
; the multi-byte register writes.
;
; === MEMORY MAP ===
; Zero page ($00-$FF)    Working variables (frame counter, positions, temps)
; BSS segment            Y-address lookup tables (480 entries x 3 bytes)
; CODE segment           Program code
; RODATA segment         Sine/cosine tables, palette, copper list, char table
; BINDATA segment        Embedded sprite RGBA, mask, AY music, font data
;
; === BUILD AND RUN ===
; Build:  make ie65asm SRC=sdk/examples/asm/robocop_intro_65.asm
; Run:    ./IntuitionEngine -6502 robocop_intro_65.ie65
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie65.inc"

; ============================================================================
; DATA ORGANISATION
; ============================================================================
; Due to the 6502's 64KB address limit, data is organised into banks:
;
; Bank 0-21 ($00000-$2BFFF):   Sprite RGBA data (172KB = 22 banks)
; Bank 22-27 ($2C000-$37FFF):  Sprite mask (43KB = 6 banks)
; Bank 28-30 ($38000-$3DFFF):  AY music data (24KB = 3 banks)
; Bank 31-56 ($3E000-$71FFF):  Font RGBA data (200KB = 26 banks)
;
; Lookup tables and copper list are kept in main memory for fast access.
; ============================================================================

; Sprite constants
SPRITE_W        = 240
SPRITE_H        = 180
SPRITE_STRIDE   = 960           ; 240 * 4 bytes per pixel
CENTER_X        = 200
CENTER_Y        = 150

; Copper bar constants
BAR_COUNT       = 16
BAR_STRIDE      = 36            ; Bytes per copper bar entry

; Scrolltext constants
SCROLL_Y        = 430
SCROLL_SPEED    = 2             ; Pixels per frame (slower = smoother)
CHAR_WIDTH      = 32
CHAR_HEIGHT     = 32
FONT_STRIDE     = 1280          ; 320 * 4 bytes per row (10 chars wide)

; Bank assignments for data (8KB bank numbers)
SPRITE_BANK_START  = 0          ; Sprite RGBA starts at bank 0
MASK_BANK_START    = 22         ; Mask starts at bank 22
AY_BANK_START      = 28         ; AY music at bank 28
FONT_BANK_START    = 31         ; Font data at bank 31

; AY music length
ROBOCOP_AY_LEN     = 24525

; ============================================================================
; ZERO PAGE VARIABLES
; ============================================================================
.segment "ZEROPAGE"

frame_lo:       .res 1          ; Frame counter (low byte)
frame_hi:       .res 1          ; Frame counter (high byte)
prev_x:         .res 2          ; Previous sprite X position
prev_y:         .res 2          ; Previous sprite Y position
curr_x:         .res 2          ; Current sprite X position
curr_y:         .res 2          ; Current sprite Y position
scroll_x_lo:    .res 1          ; Scroll X position (low)
scroll_x_hi:    .res 1          ; Scroll X position (high)
temp0:          .res 4          ; Temporary 32-bit value
temp1:          .res 4          ; Temporary 32-bit value
copper_ptr:     .res 2          ; Copper list pointer
palette_idx:    .res 1          ; Palette index for bar update
bar_idx:        .res 1          ; Current bar index
dest_addr:      .res 4          ; Destination VRAM address
; Scrolltext variables
char_idx:       .res 2          ; Current character index in message (16-bit)
char_x:         .res 2          ; Current character X position (signed)
char_count:     .res 1          ; Characters drawn counter
font_offset:    .res 4          ; Font source address offset
scroll_y:       .res 2          ; Y position with sine offset

; ============================================================================
; BSS SEGMENT - Uninitialised data in RAM
; ============================================================================
.segment "BSS"

; Y address lookup table: y_addr_tbl[y] = y * 2560 (line offset in VRAM)
; Extended to 480 entries to cover full screen (scrolltext at Y=430+)
y_addr_lo:      .res 480        ; Low byte of line offset
y_addr_hi:      .res 480        ; High byte of line offset
y_addr_bank:    .res 480        ; VRAM bank for each line

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

; ============================================================================
; ENTRY POINT AND INITIALISATION
; ============================================================================
.proc start
    jsr init_video
    jsr init_tables
    jsr init_psg
    jsr init_copper

    ; Initialise frame counter and scroll position to zero
    lda #0
    sta frame_lo
    sta frame_hi
    sta scroll_x_lo
    sta scroll_x_hi

    ; Compute initial sprite position and save as "previous"
    jsr compute_xy
    lda curr_x
    sta prev_x
    lda curr_x+1
    sta prev_x+1
    lda curr_y
    sta prev_y
    lda curr_y+1
    sta prev_y+1

; ----------------------------------------------------------------------------
; Main Loop
; Each iteration: advance frame, update copper colours, move sprite,
; synchronise to VBlank, then draw.
; ----------------------------------------------------------------------------
main_loop:
    ; Increment 16-bit frame counter
    inc frame_lo
    bne :+
    inc frame_hi
:

    ; Update copper bar colours with scrolling gradient
    jsr update_bars

    ; Compute new sprite position from sine/cosine tables
    jsr compute_xy

    ; Synchronise to vertical blank before drawing
    jsr wait_frame

    ; Erase previous sprite position with a black fill
    jsr clear_prev_sprite

    ; Draw sprite at new position with mask
    jsr draw_sprite

    ; Save current position for next frame's erase
    lda curr_x
    sta prev_x
    lda curr_x+1
    sta prev_x+1
    lda curr_y
    sta prev_y
    lda curr_y+1
    sta prev_y+1

    ; Clear scroll area and render scrolltext
    jsr clear_scroll_area
    jsr draw_scrolltext

    ; Wait for all blitter operations to complete before next frame
    jsr wait_blit

    ; Advance horizontal scroll position
    clc
    lda scroll_x_lo
    adc #SCROLL_SPEED
    sta scroll_x_lo
    lda scroll_x_hi
    adc #0
    sta scroll_x_hi

    jmp main_loop
.endproc

; ============================================================================
; HARDWARE INITIALISATION
; ============================================================================

; ----------------------------------------------------------------------------
; init_video - Set 640x480 true colour mode, enable display, clear screen
; ----------------------------------------------------------------------------
.proc init_video
    ; Set 640x480 mode
    lda #0
    sta VIDEO_MODE
    sta VIDEO_MODE+1
    sta VIDEO_MODE+2
    sta VIDEO_MODE+3

    ; Enable video
    lda #1
    sta VIDEO_CTRL

    ; Clear screen to black using blitter fill
    jsr wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Set destination to VRAM start ($100000)
    lda #$00
    sta BLT_DST_0
    sta BLT_DST_1
    lda #$10                    ; $100000 = VRAM_START
    sta BLT_DST_2
    lda #$00
    sta BLT_DST_3

    SET_BLT_WIDTH SCREEN_W
    SET_BLT_HEIGHT SCREEN_H
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR $FF000000

    START_BLIT
    jsr wait_blit

    rts
.endproc

; ----------------------------------------------------------------------------
; init_tables - Build Y-address lookup table
; Pre-computes y * 2560 for every scanline (0-479) to avoid runtime
; multiplication. Each entry stores low byte, high byte, and bank number.
; ----------------------------------------------------------------------------
.proc init_tables
    lda #0
    sta temp0                   ; Running total low
    sta temp0+1                 ; Running total high
    sta temp0+2                 ; Bank
    sta temp0+3                 ; Loop counter high byte

    ldx #0                      ; Loop counter low byte (0-479)

@loop:
    ; Store current values using split indexing for 16-bit range
    lda temp0+3
    bne @high_index

    ; Low index (0-255): direct indexing
    lda temp0
    sta y_addr_lo,x
    lda temp0+1
    sta y_addr_hi,x
    lda temp0+2
    sta y_addr_bank,x
    jmp @add_line_bytes

@high_index:
    ; High index (256-479): offset indexing
    lda temp0
    sta y_addr_lo + 256,x
    lda temp0+1
    sta y_addr_hi + 256,x
    lda temp0+2
    sta y_addr_bank + 256,x

@add_line_bytes:
    ; Add LINE_BYTES (2560 = $0A00) to running total
    clc
    lda temp0
    adc #$00                    ; Low byte of $0A00
    sta temp0
    lda temp0+1
    adc #$0A                    ; High byte of $0A00
    sta temp0+1
    bcc :+
    inc temp0+2                 ; Carry into bank
:

    ; Increment 16-bit loop counter
    inx
    bne :+
    inc temp0+3                 ; X wrapped, increment high byte
:
    ; Check if we have done 480 iterations (256 + 224)
    lda temp0+3
    cmp #1
    bne @loop                   ; High byte < 1, continue
    cpx #224                    ; 256 + 224 = 480
    bne @loop

    rts
.endproc

; ----------------------------------------------------------------------------
; init_psg - Enable PSG+ and start looped AY music playback
; ----------------------------------------------------------------------------
.proc init_psg
    ; Enable PSG+ mode
    lda #1
    sta PSG_PLUS_CTRL

    ; Set play pointer to embedded AY data (32-bit address)
    STORE32 PSG_PLAY_PTR_0, data_robocop_ay

    ; Set play length
    lda #<ROBOCOP_AY_LEN
    sta PSG_PLAY_LEN_0
    lda #>ROBOCOP_AY_LEN
    sta PSG_PLAY_LEN_1
    lda #0
    sta PSG_PLAY_LEN_2
    sta PSG_PLAY_LEN_3

    ; Start playback with loop (bit 0 = start, bit 2 = loop)
    lda #5
    sta PSG_PLAY_CTRL

    rts
.endproc

; ----------------------------------------------------------------------------
; init_copper - Programme the copper coprocessor with the rasterbar list
; ----------------------------------------------------------------------------
.proc init_copper
    ; Disable copper, set list pointer, then re-enable
    lda #2
    sta COPPER_CTRL

    lda #<copper_list
    sta COPPER_PTR_0
    lda #>copper_list
    sta COPPER_PTR_1
    lda #0
    sta COPPER_PTR_2
    sta COPPER_PTR_3

    lda #1
    sta COPPER_CTRL

    rts
.endproc

; ============================================================================
; SYNCHRONISATION AND BLITTER HELPERS
; ============================================================================

; ----------------------------------------------------------------------------
; wait_blit - Poll the blitter busy flag until the current operation completes
; ----------------------------------------------------------------------------
.proc wait_blit
:   lda BLT_CTRL
    and #2
    bne :-
    rts
.endproc

; ----------------------------------------------------------------------------
; wait_frame - Wait for exactly one complete frame boundary
; First waits for VBlank to END (active scan), then waits for VBlank to
; START (new frame). This ensures exactly one frame per iteration.
; ----------------------------------------------------------------------------
.proc wait_frame
    ; Wait for VBlank to end (active scan)
:   lda VIDEO_STATUS
    and #STATUS_VBLANK
    bne :-

    ; Wait for VBlank to start (new frame)
:   lda VIDEO_STATUS
    and #STATUS_VBLANK
    beq :-

    rts
.endproc

; ============================================================================
; SPRITE ANIMATION
; ============================================================================

; ----------------------------------------------------------------------------
; compute_xy - Calculate sprite position from sine/cosine tables
; The sprite follows a Lissajous curve: X uses sin(frame), Y uses
; cos(frame*2). Results stored in curr_x, curr_y (16-bit each).
; ----------------------------------------------------------------------------
.proc compute_xy
    ; X = sin_table[frame & 0xFF] + CENTER_X
    lda frame_lo
    and #$FF
    tax
    lda sin_x_lo,x
    clc
    adc #<CENTER_X
    sta curr_x
    lda sin_x_hi,x
    adc #>CENTER_X
    sta curr_x+1

    ; Y = cos_table[(frame * 2) & 0xFF] + CENTER_Y
    lda frame_lo
    asl a                       ; * 2
    and #$FF
    tax
    lda cos_y_lo,x
    clc
    adc #<CENTER_Y
    sta curr_y
    lda cos_y_hi,x
    adc #>CENTER_Y
    sta curr_y+1

    rts
.endproc

; ----------------------------------------------------------------------------
; update_bars - Animate copper bar colours with a scrolling sine gradient
; Each bar's colour index = (bar_idx + scroll_offset + frame/4) mod 16
; ----------------------------------------------------------------------------
.proc update_bars
    ; Calculate scroll offset from sine table
    lda frame_lo
    asl a                       ; Faster scroll
    and #$FF
    tax
    lda sin_x_lo,x
    clc
    adc #200                    ; Offset to 0-400 range
    lsr a
    lsr a
    lsr a
    lsr a                       ; / 16, now 0-25
    sta temp0                   ; Scroll offset

    ; Update each bar's colour in the copper list
    lda #0
    sta bar_idx
    lda #<(copper_list + 24)    ; Offset to first colour in copper list
    sta copper_ptr
    lda #>(copper_list + 24)
    sta copper_ptr+1

@bar_loop:
    ; Colour index = (bar_idx + scroll_offset + frame/4) & 0x0F
    lda frame_lo
    lsr a
    lsr a                       ; / 4
    clc
    adc bar_idx
    adc temp0
    and #$0F                    ; Wrap to 16 colours

    ; Get colour from palette (4 bytes per entry)
    asl a
    asl a                       ; * 4
    tax

    ; Copy 4 colour bytes to copper list
    ldy #0
    lda palette,x
    sta (copper_ptr),y
    iny
    lda palette+1,x
    sta (copper_ptr),y
    iny
    lda palette+2,x
    sta (copper_ptr),y
    iny
    lda palette+3,x
    sta (copper_ptr),y

    ; Advance to next bar in copper list
    clc
    lda copper_ptr
    adc #BAR_STRIDE
    sta copper_ptr
    lda copper_ptr+1
    adc #0
    sta copper_ptr+1

    ; Next bar
    inc bar_idx
    lda bar_idx
    cmp #BAR_COUNT
    bne @bar_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; clear_prev_sprite - Erase previous sprite position with a black fill
; ----------------------------------------------------------------------------
.proc clear_prev_sprite
    jsr wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Calculate VRAM address from previous position
    jsr calc_vram_addr_prev

    ; Store destination address
    lda dest_addr
    sta BLT_DST_0
    lda dest_addr+1
    sta BLT_DST_1
    lda dest_addr+2
    sta BLT_DST_2
    lda dest_addr+3
    sta BLT_DST_3

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR $FF000000

    START_BLIT

    rts
.endproc

; ----------------------------------------------------------------------------
; draw_sprite - Draw sprite at current position using masked blit
; The mask ensures transparent pixels preserve the background.
; ----------------------------------------------------------------------------
.proc draw_sprite
    jsr wait_blit

    SET_BLT_OP BLT_OP_MASKED

    ; Set source to embedded sprite RGBA data (32-bit address)
    STORE32 BLT_SRC_0, data_robocop_rgba

    ; Set mask to embedded sprite mask data (32-bit address)
    STORE32 BLT_MASK_0, data_robocop_mask

    ; Calculate destination VRAM address from current position
    jsr calc_vram_addr_curr

    lda dest_addr
    sta BLT_DST_0
    lda dest_addr+1
    sta BLT_DST_1
    lda dest_addr+2
    sta BLT_DST_2
    lda dest_addr+3
    sta BLT_DST_3

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_SRC_STRIDE SPRITE_STRIDE
    SET_DST_STRIDE LINE_BYTES

    START_BLIT

    rts
.endproc

; ============================================================================
; VRAM ADDRESS CALCULATION
; ============================================================================

; ----------------------------------------------------------------------------
; calc_vram_addr_prev - Calculate 32-bit VRAM address for previous position
; Uses the pre-computed Y lookup table to avoid runtime multiplication.
; Result in dest_addr (4 bytes).
; ----------------------------------------------------------------------------
.proc calc_vram_addr_prev
    ; Look up Y * LINE_BYTES from table, handling Y >= 256
    lda prev_y+1
    bne @high_y

    ; Y < 256: direct indexing
    ldy prev_y
    lda y_addr_lo,y
    sta dest_addr
    lda y_addr_hi,y
    sta dest_addr+1
    lda y_addr_bank,y
    jmp @add_vram_base

@high_y:
    ; Y >= 256: offset indexing (low byte is index into second half)
    ldy prev_y
    lda y_addr_lo + 256,y
    sta dest_addr
    lda y_addr_hi + 256,y
    sta dest_addr+1
    lda y_addr_bank + 256,y

@add_vram_base:
    ; Add VRAM_START ($100000) -- bank byte + $10
    clc
    adc #$10
    sta dest_addr+2
    lda #$00
    sta dest_addr+3

    ; Add X * 4 (16-bit multiply via shift)
    lda prev_x
    asl a                       ; low * 2, C = bit 7
    sta temp0
    lda prev_x+1
    rol a                       ; high * 2 + carry
    sta temp0+1

    asl temp0                   ; low * 4, C = bit 7
    rol temp0+1                 ; high * 4 + carry

    ; Add X*4 to dest_addr
    clc
    lda temp0
    adc dest_addr
    sta dest_addr
    lda temp0+1
    adc dest_addr+1
    sta dest_addr+1
    bcc :+
    inc dest_addr+2
:

    rts
.endproc

; ----------------------------------------------------------------------------
; calc_vram_addr_curr - Calculate 32-bit VRAM address for current position
; Same algorithm as calc_vram_addr_prev but reads from curr_x/curr_y.
; ----------------------------------------------------------------------------
.proc calc_vram_addr_curr
    lda curr_y+1
    bne @high_y

    ; Y < 256: direct indexing
    ldy curr_y
    lda y_addr_lo,y
    sta dest_addr
    lda y_addr_hi,y
    sta dest_addr+1
    lda y_addr_bank,y
    jmp @add_vram_base

@high_y:
    ; Y >= 256: offset indexing
    ldy curr_y
    lda y_addr_lo + 256,y
    sta dest_addr
    lda y_addr_hi + 256,y
    sta dest_addr+1
    lda y_addr_bank + 256,y

@add_vram_base:
    ; Add VRAM_START ($100000)
    clc
    adc #$10
    sta dest_addr+2
    lda #$00
    sta dest_addr+3

    ; Add X * 4 (16-bit multiply via shift)
    lda curr_x
    asl a                       ; low * 2, C = bit 7
    sta temp0
    lda curr_x+1
    rol a                       ; high * 2 + carry
    sta temp0+1

    asl temp0                   ; low * 4, C = bit 7
    rol temp0+1                 ; high * 4 + carry

    ; Add X*4 to dest_addr
    clc
    lda temp0
    adc dest_addr
    sta dest_addr
    lda temp0+1
    adc dest_addr+1
    sta dest_addr+1
    bcc :+
    inc dest_addr+2
:

    rts
.endproc

; ============================================================================
; SCROLLTEXT
; ============================================================================

; ----------------------------------------------------------------------------
; clear_scroll_area - Erase the bottom 90 scanlines for scrolltext
; Uses the blitter to fill Y=390..479 with black.
; ----------------------------------------------------------------------------
.proc clear_scroll_area
    jsr wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Destination: VRAM_START + 390 * LINE_BYTES = $1F3C00
    lda #$00
    sta BLT_DST_0
    lda #$3C
    sta BLT_DST_1
    lda #$1F
    sta BLT_DST_2
    lda #$00
    sta BLT_DST_3

    SET_BLT_WIDTH SCREEN_W
    lda #90
    sta BLT_HEIGHT_LO
    lda #0
    sta BLT_HEIGHT_HI
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR $FF000000

    START_BLIT

    rts
.endproc

; ----------------------------------------------------------------------------
; draw_scrolltext - Render sine-wave scrolling text using the blitter
;
; Characters are drawn from a pre-rendered RGBA font bitmap. Each
; character's Y position is offset by a sine wave lookup, creating the
; classic demoscene "bouncing scrolltext" effect. The 6502 version uses
; split high/low sine tables and 16-bit arithmetic throughout.
; ----------------------------------------------------------------------------
.proc draw_scrolltext
    ; Calculate starting character index: char_idx = scroll_x >> 5 (16-bit)
    lda scroll_x_lo
    lsr a
    lsr a
    lsr a
    lsr a
    lsr a                       ; A = scroll_x_lo >> 5 (top 3 bits)
    sta char_idx
    lda scroll_x_hi
    asl a
    asl a
    asl a                       ; A = scroll_x_hi << 3
    ora char_idx
    sta char_idx                ; char_idx low = (scroll_x_lo >> 5) | (scroll_x_hi << 3)
    lda scroll_x_hi
    lsr a
    lsr a
    lsr a
    lsr a
    lsr a                       ; A = scroll_x_hi >> 5
    sta char_idx+1              ; char_idx high

    ; Set up message pointer: zp_ptr0 = scroll_message + char_idx
    clc
    lda #<scroll_message
    adc char_idx
    sta zp_ptr0
    lda #>scroll_message
    adc char_idx+1
    sta zp_ptr0+1

    ; Calculate pixel offset: char_x = -(scroll_x & 0x1F)
    lda scroll_x_lo
    and #$1F                    ; scroll_x & 31
    beq @char_x_zero            ; If zero, result is zero (not -256)
    eor #$FF                    ; Negate (two's complement)
    clc
    adc #1
    sta char_x
    lda #$FF                    ; Sign extend to 16-bit (negative)
    sta char_x+1
    jmp @char_x_done
@char_x_zero:
    sta char_x                  ; char_x = 0
    sta char_x+1                ; char_x+1 = 0 (positive)
@char_x_done:

    ; Initialise character counter
    lda #0
    sta char_count

@char_loop:
    ; Get character from message using pointer
    ldy #0
    lda (zp_ptr0),y
    bne @got_char               ; Not null, continue
    jmp @wrap_scroll            ; Null terminator -- wrap
@got_char:

    ; Save character for later glyph lookup
    sta temp0

    ; Check if off-screen left (char_x < 0 and char_x+32 <= 0)
    lda char_x+1
    bpl @check_right            ; If positive, check right edge
    ; char_x is negative -- check if any part is visible
    lda char_x
    clc
    adc #32                     ; char_x + 32
    lda char_x+1
    adc #0
    bpl @visible                ; char_x + 32 > 0, partially visible
    jmp @next_char              ; Still negative, skip this char

@check_right:
    ; Check if off-screen right (char_x >= 608)
    lda char_x+1
    cmp #>608
    bcc @visible                ; High byte < 2, definitely visible
    beq @check_low_right        ; High byte == 2, check low byte
    jmp @done                   ; High byte > 2, we are done
@check_low_right:
    lda char_x
    cmp #<608
    bcc @visible                ; char_x < 608
    jmp @done                   ; char_x >= 608, we are done

@visible:
    ; Look up character in table to get font offset
    lda temp0                   ; Get character back
    sec
    sbc #32                     ; ASCII offset (space = 0)
    bpl @valid_lo               ; >= 0
    jmp @next_char              ; Invalid character (< 32)
@valid_lo:
    cmp #96
    bcc @valid_range            ; < 96, valid
    jmp @next_char              ; Beyond printable range
@valid_range:

    ; Multiply by 4 for table index (4 bytes per entry)
    asl a
    asl a
    tay

    ; Get 32-bit font offset from character table
    lda scroll_char_tbl,y
    sta font_offset
    lda scroll_char_tbl+1,y
    sta font_offset+1
    lda scroll_char_tbl+2,y
    sta font_offset+2
    lda scroll_char_tbl+3,y
    sta font_offset+3

    ; Check if valid glyph (offset != 0)
    lda font_offset
    ora font_offset+1
    ora font_offset+2
    ora font_offset+3
    bne @has_glyph
    jmp @next_char              ; No glyph for this character
@has_glyph:

    ; Calculate Y position with sine offset for wave motion
    lda char_count
    asl a                       ; * 2
    asl a                       ; * 4
    asl a                       ; * 8
    asl a                       ; * 16
    asl a                       ; * 32
    clc
    adc scroll_x_lo             ; Add scroll position for animation
    tay                         ; Y = sine table index

    ; Get signed 16-bit sine offset and add to baseline Y
    lda scroll_sine_lo,y
    clc
    adc #<SCROLL_Y
    sta scroll_y
    lda scroll_sine_hi,y
    adc #>SCROLL_Y
    sta scroll_y+1

    ; Set up blitter for character
    jsr wait_blit

    SET_BLT_OP BLT_OP_COPY

    ; Source = data_font_rgba + font_offset (32-bit addition)
    clc
    lda #<data_font_rgba
    adc font_offset
    sta BLT_SRC_0
    lda #>data_font_rgba
    adc font_offset+1
    sta BLT_SRC_1
    lda #^data_font_rgba
    adc font_offset+2
    sta BLT_SRC_2
    lda #0
    adc font_offset+3
    sta BLT_SRC_3

    ; Calculate destination VRAM address using Y lookup table
    lda scroll_y+1
    beq @y_low                  ; scroll_y < 256
    ; scroll_y >= 256: use offset indexing
    ldx scroll_y
    lda y_addr_lo + 256,x
    sta dest_addr
    lda y_addr_hi + 256,x
    sta dest_addr+1
    lda y_addr_bank + 256,x
    jmp @y_done
@y_low:
    ldy scroll_y
    lda y_addr_lo,y
    sta dest_addr
    lda y_addr_hi,y
    sta dest_addr+1
    lda y_addr_bank,y
@y_done:
    clc
    adc #$10                    ; Add VRAM_START base ($100000)
    sta dest_addr+2
    lda #0
    sta dest_addr+3

    ; Add char_x * 4 to destination
    lda char_x
    asl a
    sta temp1
    lda char_x+1
    rol a
    sta temp1+1
    asl temp1
    rol temp1+1

    clc
    lda dest_addr
    adc temp1
    sta BLT_DST_0
    lda dest_addr+1
    adc temp1+1
    sta BLT_DST_1
    lda dest_addr+2
    adc #0
    sta BLT_DST_2
    lda #0
    sta BLT_DST_3

    ; Set dimensions and strides
    SET_BLT_WIDTH CHAR_WIDTH
    SET_BLT_HEIGHT CHAR_HEIGHT
    SET_SRC_STRIDE FONT_STRIDE
    SET_DST_STRIDE LINE_BYTES

    START_BLIT

@next_char:
    ; Advance message pointer
    inc zp_ptr0
    bne :+
    inc zp_ptr0+1
:

    ; Advance X position by CHAR_WIDTH
    clc
    lda char_x
    adc #CHAR_WIDTH
    sta char_x
    lda char_x+1
    adc #0
    sta char_x+1

    ; Increment counter and check limit (max 21 characters)
    inc char_count
    lda char_count
    cmp #21
    bcs @done                   ; >= 21, we are done
    jmp @char_loop

@done:
    rts

@wrap_scroll:
    ; Wrap scroll position when we hit end of message
    lda scroll_x_lo
    and #$1F
    sta scroll_x_lo
    lda #0
    sta scroll_x_hi
    sta char_idx
    sta char_idx+1
    ; Reset pointer to start of message
    lda #<scroll_message
    sta zp_ptr0
    lda #>scroll_message
    sta zp_ptr0+1
    sta char_idx
    jmp @char_loop
.endproc

; ============================================================================
; READ-ONLY DATA SEGMENT
; ============================================================================
.segment "RODATA"

; ----------------------------------------------------------------------------
; Sine table for X movement (256 entries, 16-bit signed, scaled to +/-200)
; Split into separate low-byte and high-byte arrays for efficient 6502
; indexed lookup. Pre-computed: sin(i * 2pi / 256) * 200
; ----------------------------------------------------------------------------
sin_x_lo:
    .byte $00, $05, $0A, $0F, $14, $18, $1D, $22
    .byte $27, $2C, $31, $35, $3A, $3F, $43, $48
    .byte $4D, $51, $56, $5A, $5E, $63, $67, $6B
    .byte $6F, $73, $77, $7B, $7F, $83, $86, $8A
    .byte $8D, $91, $94, $97, $9B, $9E, $A1, $A4
    .byte $A6, $A9, $AC, $AE, $B0, $B3, $B5, $B7
    .byte $B9, $BB, $BC, $BE, $BF, $C1, $C2, $C3
    .byte $C4, $C5, $C6, $C6, $C7, $C7, $C8, $C8
    .byte $C8, $C8, $C8, $C7, $C7, $C6, $C6, $C5
    .byte $C4, $C3, $C2, $C1, $BF, $BE, $BC, $BB
    .byte $B9, $B7, $B5, $B3, $B0, $AE, $AC, $A9
    .byte $A6, $A4, $A1, $9E, $9B, $97, $94, $91
    .byte $8D, $8A, $86, $83, $7F, $7B, $77, $73
    .byte $6F, $6B, $67, $63, $5E, $5A, $56, $51
    .byte $4D, $48, $43, $3F, $3A, $35, $31, $2C
    .byte $27, $22, $1D, $18, $14, $0F, $0A, $05
    .byte $00, $FB, $F6, $F1, $EC, $E8, $E3, $DE
    .byte $D9, $D4, $CF, $CB, $C6, $C1, $BD, $B8
    .byte $B3, $AF, $AA, $A6, $A2, $9D, $99, $95
    .byte $91, $8D, $89, $85, $81, $7D, $7A, $76
    .byte $73, $6F, $6C, $69, $65, $62, $5F, $5C
    .byte $5A, $57, $54, $52, $50, $4D, $4B, $49
    .byte $47, $45, $44, $42, $41, $3F, $3E, $3D
    .byte $3C, $3B, $3A, $3A, $39, $39, $38, $38
    .byte $38, $38, $38, $39, $39, $3A, $3A, $3B
    .byte $3C, $3D, $3E, $3F, $41, $42, $44, $45
    .byte $47, $49, $4B, $4D, $50, $52, $54, $57
    .byte $5A, $5C, $5F, $62, $65, $69, $6C, $6F
    .byte $73, $76, $7A, $7D, $81, $85, $89, $8D
    .byte $91, $95, $99, $9D, $A2, $A6, $AA, $AF
    .byte $B3, $B8, $BD, $C1, $C6, $CB, $CF, $D4
    .byte $D9, $DE, $E3, $E8, $EC, $F1, $F6, $FB

sin_x_hi:
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF

; ----------------------------------------------------------------------------
; Cosine table for Y movement (256 entries, 16-bit signed, scaled to +/-150)
; Pre-computed: cos(i * 2pi / 256) * 150
; ----------------------------------------------------------------------------
cos_y_lo:
    .byte $96, $96, $96, $96, $95, $95, $94, $94
    .byte $93, $92, $92, $91, $90, $8E, $8D, $8C
    .byte $8B, $89, $88, $86, $84, $83, $81, $7F
    .byte $7D, $7B, $78, $76, $74, $72, $6F, $6D
    .byte $6A, $67, $65, $62, $5F, $5C, $59, $56
    .byte $53, $50, $4D, $4A, $47, $43, $40, $3D
    .byte $39, $36, $33, $2F, $2C, $28, $24, $21
    .byte $1D, $1A, $16, $12, $0F, $0B, $07, $04
    .byte $00, $FC, $F9, $F5, $F1, $EE, $EA, $E6
    .byte $E3, $DF, $DC, $D8, $D4, $D1, $CD, $CA
    .byte $C7, $C3, $C0, $BD, $B9, $B6, $B3, $B0
    .byte $AD, $AA, $A7, $A4, $A1, $9E, $9B, $99
    .byte $96, $93, $91, $8E, $8C, $8A, $88, $85
    .byte $83, $81, $7F, $7D, $7C, $7A, $78, $77
    .byte $75, $74, $73, $72, $70, $6F, $6E, $6E
    .byte $6D, $6C, $6C, $6B, $6B, $6A, $6A, $6A
    .byte $6A, $6A, $6A, $6A, $6B, $6B, $6C, $6C
    .byte $6D, $6E, $6E, $6F, $70, $72, $73, $74
    .byte $75, $77, $78, $7A, $7C, $7D, $7F, $81
    .byte $83, $85, $88, $8A, $8C, $8E, $91, $93
    .byte $96, $99, $9B, $9E, $A1, $A4, $A7, $AA
    .byte $AD, $B0, $B3, $B6, $B9, $BD, $C0, $C3
    .byte $C7, $CA, $CD, $D1, $D4, $D8, $DC, $DF
    .byte $E3, $E6, $EA, $EE, $F1, $F5, $F9, $FC
    .byte $00, $04, $07, $0B, $0F, $12, $16, $1A
    .byte $1D, $21, $24, $28, $2C, $2F, $33, $36
    .byte $39, $3D, $40, $43, $47, $4A, $4D, $50
    .byte $53, $56, $59, $5C, $5F, $62, $65, $67
    .byte $6A, $6D, $6F, $72, $74, $76, $78, $7B
    .byte $7D, $7F, $81, $83, $84, $86, $88, $89
    .byte $8B, $8C, $8D, $8E, $90, $91, $92, $92
    .byte $93, $94, $94, $95, $95, $96, $96, $96

cos_y_hi:
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00

; ----------------------------------------------------------------------------
; Colour palette for copper bars (16 entries, BGRA byte order)
; These colours form a rainbow gradient that cycles each frame.
; ----------------------------------------------------------------------------
palette:
    .byte $FF, $00, $00, $FF    ; Blue
    .byte $FF, $40, $00, $FF    ; Blue-cyan
    .byte $FF, $80, $00, $FF    ; Cyan
    .byte $FF, $C0, $00, $FF    ; Cyan-green
    .byte $80, $FF, $00, $FF    ; Green-yellow
    .byte $00, $FF, $00, $FF    ; Green
    .byte $00, $FF, $40, $FF    ; Green-yellow
    .byte $00, $FF, $80, $FF    ; Yellow
    .byte $00, $FF, $FF, $FF    ; Yellow
    .byte $00, $C0, $FF, $FF    ; Orange
    .byte $00, $80, $FF, $FF    ; Orange-red
    .byte $00, $40, $FF, $FF    ; Red
    .byte $00, $00, $FF, $FF    ; Red
    .byte $FF, $00, $FF, $FF    ; Magenta
    .byte $FF, $00, $80, $FF    ; Purple
    .byte $FF, $00, $40, $FF    ; Blue-purple

; ----------------------------------------------------------------------------
; Copper list for 16 horizontal raster bars
; Each bar: WAIT scanline, MOVE raster_y, MOVE raster_height, MOVE colour,
; MOVE ctrl. The colour field is updated dynamically each frame.
; ----------------------------------------------------------------------------
copper_list:
    ; Bar 0 at Y=40
    .dword 40*COP_WAIT_SCALE     ; WAIT
    .dword COP_MOVE_RASTER_Y    ; MOVE RASTER_Y
    .dword 40
    .dword COP_MOVE_RASTER_H    ; MOVE RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR ; MOVE COLOUR
    .dword $FF0000FF            ; (will be updated)
    .dword COP_MOVE_RASTER_CTRL ; MOVE CTRL
    .dword 1

    ; Bar 1 at Y=64
    .dword 64*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 64
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF0040FF
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 2 at Y=88
    .dword 88*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 88
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF0080FF
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 3 at Y=112
    .dword 112*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 112
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF00C0FF
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 4 at Y=136
    .dword 136*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 136
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF00FF80
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 5 at Y=160
    .dword 160*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 160
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF00FF00
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 6 at Y=184
    .dword 184*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 184
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF40FF00
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 7 at Y=208
    .dword 208*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 208
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF80FF00
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 8 at Y=232
    .dword 232*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 232
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FFFFFF00
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 9 at Y=256
    .dword 256*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 256
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FFFFC000
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 10 at Y=280
    .dword 280*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 280
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FFFF8000
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 11 at Y=304
    .dword 304*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 304
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FFFF4000
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 12 at Y=328
    .dword 328*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 328
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FFFF0000
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 13 at Y=352
    .dword 352*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 352
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FFFF00FF
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 14 at Y=376
    .dword 376*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 376
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF8000FF
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; Bar 15 at Y=400
    .dword 400*COP_WAIT_SCALE
    .dword COP_MOVE_RASTER_Y
    .dword 400
    .dword COP_MOVE_RASTER_H
    .dword 12
    .dword COP_MOVE_RASTER_COLOR
    .dword $FF4000FF
    .dword COP_MOVE_RASTER_CTRL
    .dword 1

    ; END
    .dword COP_END

; ----------------------------------------------------------------------------
; Scrolltext sine table for Y wave effect (256 entries, +/-20 pixels)
; Split into low/high byte arrays. Pre-computed: sin(i * 2pi / 256) * 20
; ----------------------------------------------------------------------------
scroll_sine_lo:
    .byte $00, $00, $01, $01, $02, $02, $03, $03
    .byte $04, $04, $05, $05, $06, $06, $07, $07
    .byte $08, $08, $09, $09, $0A, $0A, $0A, $0B
    .byte $0B, $0C, $0C, $0C, $0D, $0D, $0D, $0E
    .byte $0E, $0E, $0E, $0F, $0F, $0F, $0F, $0F
    .byte $10, $10, $10, $10, $10, $10, $10, $10
    .byte $10, $10, $10, $10, $10, $10, $10, $10
    .byte $10, $0F, $0F, $0F, $0F, $0F, $0E, $0E
    .byte $0E, $0E, $0D, $0D, $0D, $0C, $0C, $0C
    .byte $0B, $0B, $0A, $0A, $0A, $09, $09, $08
    .byte $08, $07, $07, $06, $06, $05, $05, $04
    .byte $04, $03, $03, $02, $02, $01, $01, $00
    .byte $00, $00, $FF, $FF, $FE, $FE, $FD, $FD
    .byte $FC, $FC, $FB, $FB, $FA, $FA, $F9, $F9
    .byte $F8, $F8, $F7, $F7, $F6, $F6, $F6, $F5
    .byte $F5, $F4, $F4, $F4, $F3, $F3, $F3, $F2
    .byte $F2, $F2, $F2, $F1, $F1, $F1, $F1, $F1
    .byte $F0, $F0, $F0, $F0, $F0, $F0, $F0, $F0
    .byte $F0, $F0, $F0, $F0, $F0, $F0, $F0, $F0
    .byte $F0, $F1, $F1, $F1, $F1, $F1, $F2, $F2
    .byte $F2, $F2, $F3, $F3, $F3, $F4, $F4, $F4
    .byte $F5, $F5, $F6, $F6, $F6, $F7, $F7, $F8
    .byte $F8, $F9, $F9, $FA, $FA, $FB, $FB, $FC
    .byte $FC, $FD, $FD, $FE, $FE, $FF, $FF, $00
    .byte $00, $00, $01, $01, $02, $02, $03, $03
    .byte $04, $04, $05, $05, $06, $06, $07, $07
    .byte $08, $08, $09, $09, $0A, $0A, $0A, $0B
    .byte $0B, $0C, $0C, $0C, $0D, $0D, $0D, $0E
    .byte $0E, $0E, $0E, $0F, $0F, $0F, $0F, $0F
    .byte $10, $10, $10, $10, $10, $10, $10, $10
    .byte $10, $10, $10, $10, $10, $10, $10, $10
    .byte $10, $0F, $0F, $0F, $0F, $0F, $0E, $0E

scroll_sine_hi:
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00

; ----------------------------------------------------------------------------
; Character lookup table - maps ASCII 32-127 to font byte offsets
; Each entry is a 32-bit offset into the font RGBA bitmap.
; Zero means no glyph available for that character.
; ----------------------------------------------------------------------------
scroll_char_tbl:
    ; ASCII 32-47: space and punctuation
    .dword 0                    ; 32 ' ' space
    .dword 128                  ; 33 '!'
    .dword 256                  ; 34 '"'
    .dword 0                    ; 35 '#'
    .dword 0                    ; 36 '$'
    .dword 0                    ; 37 '%'
    .dword 40960                ; 38 '&'
    .dword 896                  ; 39 '''
    .dword 1024                 ; 40 '('
    .dword 1152                 ; 41 ')'
    .dword 512                  ; 42 '*'
    .dword 41600                ; 43 '+'
    .dword 41216                ; 44 ','
    .dword 41344                ; 45 '-'
    .dword 41472                ; 46 '.'
    .dword 0                    ; 47 '/'

    ; ASCII 48-57: digits 0-9
    .dword 41728                ; 48 '0'
    .dword 41856                ; 49 '1'
    .dword 41984                ; 50 '2'
    .dword 42112                ; 51 '3'
    .dword 81920                ; 52 '4'
    .dword 82048                ; 53 '5'
    .dword 82176                ; 54 '6'
    .dword 82304                ; 55 '7'
    .dword 82432                ; 56 '8'
    .dword 82560                ; 57 '9'

    ; ASCII 58-64: punctuation
    .dword 82688                ; 58 ':'
    .dword 82816                ; 59 ';'
    .dword 0                    ; 60 '<'
    .dword 82944                ; 61 '='
    .dword 0                    ; 62 '>'
    .dword 122880               ; 63 '?'
    .dword 384                  ; 64 '@'

    ; ASCII 65-90: uppercase A-Z
    .dword 123264               ; 65 'A'
    .dword 123392               ; 66 'B'
    .dword 123520               ; 67 'C'
    .dword 123648               ; 68 'D'
    .dword 123776               ; 69 'E'
    .dword 123904               ; 70 'F'
    .dword 124032               ; 71 'G'
    .dword 163840               ; 72 'H'
    .dword 163968               ; 73 'I'
    .dword 164096               ; 74 'J'
    .dword 164224               ; 75 'K'
    .dword 164352               ; 76 'L'
    .dword 164480               ; 77 'M'
    .dword 164608               ; 78 'N'
    .dword 164736               ; 79 'O'
    .dword 164864               ; 80 'P'
    .dword 164992               ; 81 'Q'
    .dword 204800               ; 82 'R'
    .dword 204928               ; 83 'S'
    .dword 205056               ; 84 'T'
    .dword 205184               ; 85 'U'
    .dword 205312               ; 86 'V'
    .dword 205440               ; 87 'W'
    .dword 205568               ; 88 'X'
    .dword 205696               ; 89 'Y'
    .dword 205824               ; 90 'Z'

    ; ASCII 91-96: brackets and miscellaneous
    .dword 83072                ; 91 '['
    .dword 0                    ; 92 '\'
    .dword 122752               ; 93 ']'
    .dword 768                  ; 94 '^'
    .dword 0                    ; 95 '_'
    .dword 205952               ; 96 '`'

    ; ASCII 97-122: lowercase a-z (mapped to same glyphs as uppercase)
    .dword 123264               ; 97 'a'
    .dword 123392               ; 98 'b'
    .dword 123520               ; 99 'c'
    .dword 123648               ; 100 'd'
    .dword 123776               ; 101 'e'
    .dword 123904               ; 102 'f'
    .dword 124032               ; 103 'g'
    .dword 163840               ; 104 'h'
    .dword 163968               ; 105 'i'
    .dword 164096               ; 106 'j'
    .dword 164224               ; 107 'k'
    .dword 164352               ; 108 'l'
    .dword 164480               ; 109 'm'
    .dword 164608               ; 110 'n'
    .dword 164736               ; 111 'o'
    .dword 164864               ; 112 'p'
    .dword 164992               ; 113 'q'
    .dword 204800               ; 114 'r'
    .dword 204928               ; 115 's'
    .dword 205056               ; 116 't'
    .dword 205184               ; 117 'u'
    .dword 205312               ; 118 'v'
    .dword 205440               ; 119 'w'
    .dword 205568               ; 120 'x'
    .dword 205696               ; 121 'y'
    .dword 205824               ; 122 'z'

    ; ASCII 123-127: braces and miscellaneous
    .dword 123008               ; 123 '{'
    .dword 0                    ; 124 '|'
    .dword 0                    ; 125 '}'
    .dword 41088                ; 126 '~'
    .dword 0                    ; 127 DEL

; ----------------------------------------------------------------------------
; Scroll message text (null-terminated ASCII)
; ----------------------------------------------------------------------------
scroll_message:
    .byte "    ...ROBOCOP DUAL CPU 6502 AND Z80 INTRO FOR THE INTUITION ENGINE... "
    .byte "...100 PERCENT ASM CODE... "
    .byte "...6502 ASM FOR DEMO EFFECTS... "
    .byte "...Z80 ASM FOR MUSIC REPLAY ROUTINE... "
    .byte "...ALL CODE BY INTUITON...  "
    .byte "MUSIC BY JONATHAN DUNN FROM THE 1987 ZX SPECTRUM GAME ROBOCOP BY OCEAN SOFTWARE... "
    .byte "...AY REGISTERS ARE REMAPPED TO THE INTUITON ENGINE SYNTH FOR SUPERIOR SOUND QUALITY... "
    .byte "...GREETS TO ...GADGETMASTER... ...KARLOS... ...BLOODLINE... "
    .byte "...VISIT INTUITIONSUBSYNTH.COM......................."
    .byte 0

; ============================================================================
; EMBEDDED BINARY DATA
; ============================================================================
; All graphics, mask, and audio data is embedded here. The blitter accesses
; this data via 32-bit addresses regardless of the 6502's 16-bit bus.
; ============================================================================
.segment "BINDATA"

; Sprite RGBA data (240x180x4 = 172,800 bytes)
data_robocop_rgba:
.incbin "../assets/robocop_rgba.bin"

; Sprite mask data (240x180/8 = 5,400 bytes, 1 bit per pixel)
data_robocop_mask:
.incbin "../assets/robocop_mask.bin"

; AY music data (24,525 bytes)
data_robocop_ay:
.incbin "../assets/music/Robocop1.ay"

; Font RGBA data (256,000 bytes)
data_font_rgba:
.incbin "../assets/font_rgba.bin"

; Entry point is at 'start' label - loader handles vectors
