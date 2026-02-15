; ============================================================================
; ROBOCOP INTRO - IE80 (Z80 Port)
; Moves robocop sprite with the blitter, animated copper RGB bars, PSG+ AY.
;
; This is a port of the IE65 (6502) demo to Z80 assembly using the extended
; banking system of the Intuition Engine.
;
; For use with vasmz80_std assembler
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie80.inc"

; ============================================================================
; DATA ORGANIZATION
; ============================================================================
; Due to Z80's 64KB address limit, data is organized into banks:
;
; Bank 0-21 (0x00000-0x2BFFF):   Sprite RGBA data (172KB = 22 banks)
; Bank 22-27 (0x2C000-0x37FFF):  Sprite mask (43KB = 6 banks)
; Bank 28-30 (0x38000-0x3DFFF):  AY music data (24KB = 3 banks)
; Bank 31-56 (0x3E000-0x71FFF):  Font RGBA data (200KB = 26 banks)
;
; Lookup tables and copper list are kept in main memory for fast access.
; ============================================================================

; Sprite constants
.set SPRITE_W,240
.set SPRITE_H,180
.set SPRITE_STRIDE,960        ; 240 * 4 bytes per pixel
.set CENTER_X,200
.set CENTER_Y,150

; Copper bar constants
.set BAR_COUNT,16
.set BAR_STRIDE,36            ; Bytes per copper bar entry

; Scrolltext constants
.set SCROLL_Y,430
.set SCROLL_SPEED,2           ; Pixels per frame
.set CHAR_WIDTH,32
.set CHAR_HEIGHT,32
.set FONT_STRIDE,1280         ; 320 * 4 bytes per row (10 chars wide)

; Bank assignments for data (8KB bank numbers)
.set SPRITE_BANK_START,0      ; Sprite RGBA starts at bank 0
.set MASK_BANK_START,22       ; Mask starts at bank 22
.set AY_BANK_START,28         ; AY music at bank 28
.set FONT_BANK_START,31       ; Font data at bank 31

; AY music length
.set ROBOCOP_AY_LEN,24525

; ============================================================================
; MEMORY LAYOUT
; ============================================================================
; 0x0000-0x00FF   Entry point and interrupt vectors
; 0x0100-0x1FFF   Program code (~8KB)
; 0x2000-0x3FFF   Bank 1 Window (8KB) - Sprite RGBA data
; 0x4000-0x5FFF   Bank 2 Window (8KB) - Font RGBA data
; 0x6000-0x7FFF   Bank 3 Window (8KB) - AY music / general
; 0x8000-0xBFFF   VRAM Bank Window (16KB)
; 0xC000-0xCFFF   Variables and lookup tables
; 0xD000-0xDFFF   Copper list
; 0xE000-0xEFFF   Stack
; 0xF000-0xFFFF   I/O region (mapped to 0xF0000-0xF0FFF)
; ============================================================================

    .org 0x0000

; ============================================================================
; ENTRY POINT
; ============================================================================
start:
    di                          ; Disable interrupts
    ld sp,STACK_TOP             ; Initialize stack

    call init_video
    call init_tables
    call init_psg
    call init_copper

    ; Initialize frame counter
    xor a
    ld (frame_lo),a
    ld (frame_hi),a
    ld (scroll_x_lo),a
    ld (scroll_x_hi),a

    ; Compute initial position
    call compute_xy
    ld hl,(curr_x)
    ld (prev_x),hl
    ld hl,(curr_y)
    ld (prev_y),hl

; ----------------------------------------------------------------------------
; Main Loop
; ----------------------------------------------------------------------------
main_loop:
    ; Increment frame counter
    ld hl,(frame_lo)
    inc hl
    ld (frame_lo),hl

    ; Update copper bar colors
    call update_bars

    ; Compute new sprite position
    call compute_xy

    ; Wait for VBlank before drawing
    call wait_frame

    ; Clear previous sprite position
    call clear_prev_sprite

    ; Draw sprite at new position
    call draw_sprite

    ; Update previous position
    ld hl,(curr_x)
    ld (prev_x),hl
    ld hl,(curr_y)
    ld (prev_y),hl

    ; Clear scroll area and draw scrolltext
    call clear_scroll_area
    call draw_scrolltext

    ; Wait for blitter to complete
    call wait_blit

    ; Advance scroll position
    ld hl,(scroll_x_lo)
    ld bc,SCROLL_SPEED
    add hl,bc
    ld (scroll_x_lo),hl

    jp main_loop

; ============================================================================
; SUBROUTINES
; ============================================================================

; ----------------------------------------------------------------------------
; Initialize video mode
; ----------------------------------------------------------------------------
init_video:
    ; Set 640x480 mode (mode 0)
    xor a
    ld (VIDEO_MODE),a
    ld (VIDEO_MODE+1),a
    ld (VIDEO_MODE+2),a
    ld (VIDEO_MODE+3),a

    ; Enable video
    ld a,1
    ld (VIDEO_CTRL),a

    ; Clear screen with blitter
    call wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Set destination to VRAM start (0x100000)
    xor a
    ld (BLT_DST_0),a
    ld (BLT_DST_1),a
    ld a,0x10
    ld (BLT_DST_2),a
    xor a
    ld (BLT_DST_3),a

    SET_BLT_WIDTH SCREEN_W
    SET_BLT_HEIGHT SCREEN_H
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR 0xFF000000

    START_BLIT
    call wait_blit

    ret

; ----------------------------------------------------------------------------
; Initialize lookup tables
; Build Y address table: y_addr[y] = y * 2560 (LINE_BYTES)
; ----------------------------------------------------------------------------
init_tables:
    ld hl,0                     ; Running total (32-bit in HL:DE -> simplified to 24-bit)
    ld de,0                     ; DE = bank (high bits)
    ld ix,y_addr_lo             ; Table pointers
    ld iy,y_addr_hi
    ld bc,480                   ; Loop counter

.init_loop:
    ; Store current offset (low byte)
    ld a,l
    ld (ix+0),a

    ; Store high byte
    ld a,h
    ld (iy+0),a

    ; Store bank at y_addr_bank
    push hl
    push bc
    push de

    ; Calculate y_addr_bank address = y_addr_bank + loop_index
    ; Index = 480 - BC
    ld hl,480
    or a                        ; Clear carry
    sbc hl,bc                   ; HL = 480 - BC = current index
    ex de,hl                    ; DE = index
    ld hl,y_addr_bank
    add hl,de                   ; HL = y_addr_bank + index
    pop de
    ld a,e                      ; Bank number (from DE saved earlier)
    ld (hl),a

    pop bc
    pop hl

    ; Increment table pointers
    inc ix
    inc iy

    ; Add LINE_BYTES (2560 = 0x0A00)
    push bc
    ld bc,0x0A00
    add hl,bc
    jr nc,.no_bank_inc
    inc de                      ; Increment bank on overflow
.no_bank_inc:
    pop bc

    dec bc
    ld a,b
    or c
    jr nz,.init_loop

    ret

; ----------------------------------------------------------------------------
; Initialize PSG+ and start music playback
; ----------------------------------------------------------------------------
init_psg:
    ; Enable PSG+ mode
    ld a,1
    ld (PSG_PLUS_CTRL),a

    ; Set play pointer to embedded AY data
    SET_PSG_PTR data_robocop_ay

    ; Set play length
    SET_PSG_LEN ROBOCOP_AY_LEN

    ; Start playback with loop (bit0=start, bit2=loop)
    ld a,5
    ld (PSG_PLAY_CTRL),a

    ret

; ----------------------------------------------------------------------------
; Initialize copper list
; ----------------------------------------------------------------------------
init_copper:
    ; Disable copper first
    ld a,2
    ld (COPPER_CTRL),a

    ; Set copper list pointer
    SET_COPPER_PTR copper_list

    ; Enable copper
    ld a,1
    ld (COPPER_CTRL),a

    ret

; ----------------------------------------------------------------------------
; Wait for blitter to finish
; ----------------------------------------------------------------------------
wait_blit:
    ld a,(BLT_CTRL)
    and 2
    jr nz,wait_blit
    ret

; ----------------------------------------------------------------------------
; Wait for complete frame (VBlank transition)
; ----------------------------------------------------------------------------
wait_frame:
    ; Wait for VBlank to end (active scan)
.wait_not_vblank:
    ld a,(VIDEO_STATUS)
    and STATUS_VBLANK
    jr nz,.wait_not_vblank

    ; Wait for VBlank to start (new frame)
.wait_vblank:
    ld a,(VIDEO_STATUS)
    and STATUS_VBLANK
    jr z,.wait_vblank

    ret

; ----------------------------------------------------------------------------
; Compute sprite X,Y from frame counter using sine/cosine tables
; Result in curr_x, curr_y
; ----------------------------------------------------------------------------
compute_xy:
    ; X = sin_table[frame & 0xFF] + CENTER_X
    ld a,(frame_lo)
    ld l,a
    ld h,>sin_x_lo          ; Assumes table is page-aligned
    ld e,(hl)                   ; Low byte of sine value
    ld h,>sin_x_hi
    ld d,(hl)                   ; High byte (sign extension)

    ld hl,CENTER_X
    add hl,de
    ld (curr_x),hl

    ; Y = cos_table[(frame * 2) & 0xFF] + CENTER_Y
    ld a,(frame_lo)
    add a,a                     ; * 2
    ld l,a
    ld h,>cos_y_lo
    ld e,(hl)
    ld h,>cos_y_hi
    ld d,(hl)

    ld hl,CENTER_Y
    add hl,de
    ld (curr_y),hl

    ret

; ----------------------------------------------------------------------------
; Update copper bar colors with scrolling gradient
; ----------------------------------------------------------------------------
update_bars:
    ; Calculate scroll offset from sine table
    ld a,(frame_lo)
    add a,a                     ; Faster scroll
    ld l,a
    ld h,>sin_x_lo
    ld a,(hl)
    add a,200                   ; Offset to 0-400 range
    rrca
    rrca
    rrca
    rrca                        ; / 16, now 0-25
    and 0x0F
    ld (scroll_offset),a        ; Store scroll offset

    ; Update each bar's color
    ld b,BAR_COUNT              ; Loop counter
    xor a
    ld (bar_idx),a

    ld hl,copper_list + 24      ; Offset to first color in copper list

.bar_loop:
    push hl
    push bc

    ; Color index = (bar_idx + scroll_offset + frame/4) & 0x0F
    ld a,(frame_lo)
    srl a
    srl a                       ; / 4
    ld b,a
    ld a,(bar_idx)
    add a,b
    ld b,a
    ld a,(scroll_offset)
    add a,b
    and 0x0F                    ; Wrap to 16 colors

    ; Get color from palette (4 bytes per color)
    add a,a
    add a,a                     ; * 4
    ld e,a
    ld d,0
    ld hl,palette
    add hl,de                   ; HL = palette + index * 4

    ; Copy 4 color bytes to copper list
    pop bc
    push bc

    ; Get copper list pointer back
    ld de,-(24 + BAR_COUNT * BAR_STRIDE)
    pop bc
    push bc
    ld a,BAR_COUNT
    sub b
    ld e,a
    ld d,0
    ; Calculate: copper_list + 24 + bar_idx * BAR_STRIDE
    push hl                     ; Save palette pointer
    ld hl,copper_list + 24
    ld a,(bar_idx)
.mul_stride:
    or a
    jr z,.mul_done
    push bc
    ld bc,BAR_STRIDE
    add hl,bc               ; Add BAR_STRIDE to copper list pointer
    pop bc
    dec a
    jr .mul_stride
.mul_done:
    ld de,0                     ; Reset
    ex de,hl                    ; DE = copper list position
    pop hl                      ; HL = palette pointer

    ; Copy 4 bytes
    ldi
    ldi
    ldi
    ldi

    ; Next bar
    ld a,(bar_idx)
    inc a
    ld (bar_idx),a

    pop bc
    pop hl

    ; Advance copper list pointer by BAR_STRIDE
    push bc
    ld bc,BAR_STRIDE
    add hl,bc
    pop bc

    djnz .bar_loop

    ret

; ----------------------------------------------------------------------------
; Clear previous sprite position
; ----------------------------------------------------------------------------
clear_prev_sprite:
    call wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Calculate VRAM address
    call calc_vram_addr_prev

    ; Store destination address
    ld hl,(dest_addr)
    ld a,l
    ld (BLT_DST_0),a
    ld a,h
    ld (BLT_DST_1),a
    ld hl,(dest_addr+2)
    ld a,l
    ld (BLT_DST_2),a
    ld a,h
    ld (BLT_DST_3),a

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR 0xFF000000

    START_BLIT

    ret

; ----------------------------------------------------------------------------
; Draw sprite at current position
; ----------------------------------------------------------------------------
draw_sprite:
    call wait_blit

    SET_BLT_OP BLT_OP_MASKED

    ; Set source to embedded sprite RGBA data
    SET_BLT_SRC data_robocop_rgba

    ; Set mask to embedded sprite mask data
    SET_BLT_MASK data_robocop_mask

    ; Calculate destination VRAM address
    call calc_vram_addr_curr

    ld hl,(dest_addr)
    ld a,l
    ld (BLT_DST_0),a
    ld a,h
    ld (BLT_DST_1),a
    ld hl,(dest_addr+2)
    ld a,l
    ld (BLT_DST_2),a
    ld a,h
    ld (BLT_DST_3),a

    SET_BLT_WIDTH SPRITE_W
    SET_BLT_HEIGHT SPRITE_H
    SET_SRC_STRIDE SPRITE_STRIDE
    SET_DST_STRIDE LINE_BYTES

    START_BLIT

    ret

; ----------------------------------------------------------------------------
; Calculate VRAM address for previous position
; Result in dest_addr (32-bit)
; ----------------------------------------------------------------------------
calc_vram_addr_prev:
    ; Use lookup table for Y * LINE_BYTES
    ld hl,(prev_y)

    ; Check if Y >= 256
    ld a,h
    or a
    jr nz,.high_y

    ; Y < 256: direct indexing
    ld e,l                      ; E = Y
    ld d,0
    ld hl,y_addr_lo
    add hl,de
    ld a,(hl)
    ld (dest_addr),a

    ld hl,y_addr_hi
    add hl,de
    ld a,(hl)
    ld (dest_addr+1),a

    ld hl,y_addr_bank
    add hl,de
    ld a,(hl)
    jr .add_vram_base

.high_y:
    ; Y >= 256: offset indexing
    ld hl,(prev_y)
    ld e,l                      ; Low byte is the offset
    ld d,0
    ld hl,y_addr_lo + 256
    add hl,de
    ld a,(hl)
    ld (dest_addr),a

    ld hl,y_addr_hi + 256
    add hl,de
    ld a,(hl)
    ld (dest_addr+1),a

    ld hl,y_addr_bank + 256
    add hl,de
    ld a,(hl)

.add_vram_base:
    ; Add VRAM_START (0x100000) - bank already in A
    add a,0x10                  ; Add 0x10 to high byte for VRAM base
    ld (dest_addr+2),a
    xor a
    ld (dest_addr+3),a

    ; Add X * 4
    ld hl,(prev_x)
    add hl,hl                   ; * 2
    add hl,hl                   ; * 4

    ; Add to dest_addr
    ld de,(dest_addr)
    add hl,de
    ld (dest_addr),hl
    jr nc,.no_carry
    ld hl,(dest_addr+2)
    inc hl
    ld (dest_addr+2),hl
.no_carry:

    ret

; ----------------------------------------------------------------------------
; Calculate VRAM address for current position
; Result in dest_addr (32-bit)
; ----------------------------------------------------------------------------
calc_vram_addr_curr:
    ; Use lookup table for Y * LINE_BYTES
    ld hl,(curr_y)

    ; Check if Y >= 256
    ld a,h
    or a
    jr nz,.curr_high_y

    ; Y < 256: direct indexing
    ld e,l
    ld d,0
    ld hl,y_addr_lo
    add hl,de
    ld a,(hl)
    ld (dest_addr),a

    ld hl,y_addr_hi
    add hl,de
    ld a,(hl)
    ld (dest_addr+1),a

    ld hl,y_addr_bank
    add hl,de
    ld a,(hl)
    jr .curr_add_vram_base

.curr_high_y:
    ld hl,(curr_y)
    ld e,l
    ld d,0
    ld hl,y_addr_lo + 256
    add hl,de
    ld a,(hl)
    ld (dest_addr),a

    ld hl,y_addr_hi + 256
    add hl,de
    ld a,(hl)
    ld (dest_addr+1),a

    ld hl,y_addr_bank + 256
    add hl,de
    ld a,(hl)

.curr_add_vram_base:
    add a,0x10
    ld (dest_addr+2),a
    xor a
    ld (dest_addr+3),a

    ; Add X * 4
    ld hl,(curr_x)
    add hl,hl
    add hl,hl

    ld de,(dest_addr)
    add hl,de
    ld (dest_addr),hl
    jr nc,.curr_no_carry
    ld hl,(dest_addr+2)
    inc hl
    ld (dest_addr+2),hl
.curr_no_carry:

    ret

; ----------------------------------------------------------------------------
; Clear scroll text area at bottom of screen
; ----------------------------------------------------------------------------
clear_scroll_area:
    call wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Destination: VRAM_START + 390 * LINE_BYTES
    ; 390 * 2560 = 998400 = 0xF3C00
    ; + 0x100000 = 0x1F3C00
    xor a
    ld (BLT_DST_0),a
    ld a,0x3C
    ld (BLT_DST_1),a
    ld a,0x1F
    ld (BLT_DST_2),a
    xor a
    ld (BLT_DST_3),a

    SET_BLT_WIDTH SCREEN_W
    ld a,90
    ld (BLT_HEIGHT_LO),a
    xor a
    ld (BLT_HEIGHT_HI),a
    SET_DST_STRIDE LINE_BYTES
    SET_BLT_COLOR 0xFF000000

    START_BLIT

    ret

; ----------------------------------------------------------------------------
; Draw scrolling text with sine wave effect
; ----------------------------------------------------------------------------
draw_scrolltext:
    ; Calculate starting character index: char_idx = scroll_x >> 5
    ld hl,(scroll_x_lo)
    ; Shift right 5 bits: (H:L) >> 5
    ld a,l
    rrca
    rrca
    rrca
    rrca
    rrca                        ; A = L >> 5 (top 3 bits of L)
    and 0x07                    ; Keep only bottom 3 bits
    ld c,a                      ; Save in C
    ld a,h
    rlca
    rlca
    rlca                        ; A = H << 3
    or c                        ; A = (L >> 5) | (H << 3)
    ld (char_idx),a
    ld a,h
    rrca
    rrca
    rrca
    rrca
    rrca                        ; A = H >> 5
    and 0x07
    ld (char_idx+1),a

    ; Set up message pointer: msg_ptr = scroll_message + char_idx
    ld hl,scroll_message
    ld de,(char_idx)
    add hl,de
    ld (msg_ptr),hl

    ; Calculate pixel offset: char_x = -(scroll_x & 0x1F)
    ld a,(scroll_x_lo)
    and 0x1F                    ; scroll_x & 31
    jr z,.char_x_zero
    neg                         ; Negate
    ld (char_x),a
    ld a,0xFF                   ; Sign extend
    ld (char_x+1),a
    jr .char_x_done
.char_x_zero:
    xor a
    ld (char_x),a
    ld (char_x+1),a
.char_x_done:

    ; Initialize character counter
    xor a
    ld (char_count),a

.char_loop:
    ; Get character from message
    ld hl,(msg_ptr)
    ld a,(hl)
    or a
    jr nz,.got_char
    jp .wrap_scroll             ; Null terminator - wrap
.got_char:
    ld (curr_char),a            ; Save character

    ; Check if off-screen left (char_x < 0 and char_x+32 <= 0)
    ld a,(char_x+1)
    bit 7,a
    jr z,.check_right           ; If positive, check right edge
    ; char_x is negative - check if any part is visible
    ld hl,(char_x)
    ld bc,32
    add hl,bc                   ; char_x + 32
    bit 7,h
    jr z,.visible               ; char_x + 32 > 0, visible
    jp .next_char               ; Still negative, skip

.check_right:
    ; Check if off-screen right (char_x >= 608)
    ld hl,(char_x)
    ld bc,-608
    add hl,bc
    jr nc,.visible              ; char_x < 608 (no overflow = no carry)
    jp .done                    ; char_x >= 608, we're done

.visible:
    ; Get character and validate
    ld a,(curr_char)
    sub 32                      ; ASCII offset (space = 0)
    jp m,.next_char             ; < 32, invalid
    cp 96
    jp nc,.next_char            ; >= 96, invalid

    ; Multiply by 4 for table index
    ld l,a
    ld h,0
    add hl,hl                   ; * 2
    add hl,hl                   ; * 4
    ld de,scroll_char_tbl
    add hl,de                   ; HL = scroll_char_tbl + index * 4

    ; Get font offset (32-bit)
    ld e,(hl)
    inc hl
    ld d,(hl)
    inc hl
    ld (font_offset),de
    ld e,(hl)
    inc hl
    ld d,(hl)
    ld (font_offset+2),de

    ; Check if valid glyph (offset != 0)
    ld a,(font_offset)
    ld hl,font_offset+1
    or (hl)
    inc hl
    or (hl)
    inc hl
    or (hl)
    jp z,.next_char             ; No glyph

    ; Calculate Y position with sine offset
    ; sine_index = (char_count * 32 + scroll_x) & 0xFF
    ld a,(char_count)
    rlca
    rlca
    rlca
    rlca
    rlca                        ; * 32
    ld hl,(scroll_x_lo)
    add a,l                     ; Add scroll position
    ld l,a
    ld h,>scroll_sine_lo        ; Sine table (page-aligned)
    ld e,(hl)
    ld h,>scroll_sine_hi
    ld d,(hl)                   ; DE = sine offset

    ld hl,SCROLL_Y
    add hl,de
    ld (scroll_y),hl            ; scroll_y = SCROLL_Y + sine

    ; Set up blitter for character
    call wait_blit
    SET_BLT_OP BLT_OP_COPY

    ; Source = data_font_rgba + font_offset (32-bit addition)
    ; Byte 0-1: low 16 bits
    ld hl,data_font_rgba & 0xFFFF
    ld de,(font_offset)
    add hl,de
    ld a,l
    ld (BLT_SRC_0),a
    ld a,h
    ld (BLT_SRC_1),a
    ; Byte 2: data_font_rgba bank + font_offset[2] + carry
    ld a,(font_offset+2)
    adc a,(data_font_rgba >> 16) & 0xFF
    ld (BLT_SRC_2),a
    ; Byte 3: always 0 (addresses < 16MB)
    xor a
    ld (BLT_SRC_3),a

    ; Calculate dest VRAM address using Y lookup table
    ld hl,(scroll_y)
    ld a,h
    or a
    jr z,.y_low
    ; scroll_y >= 256
    ld e,l
    ld d,0
    ld hl,y_addr_lo + 256
    add hl,de
    ld a,(hl)
    ld (dest_addr),a
    ld hl,y_addr_hi + 256
    add hl,de
    ld a,(hl)
    ld (dest_addr+1),a
    ld hl,y_addr_bank + 256
    add hl,de
    ld a,(hl)
    jr .y_done
.y_low:
    ld e,l
    ld d,0
    ld hl,y_addr_lo
    add hl,de
    ld a,(hl)
    ld (dest_addr),a
    ld hl,y_addr_hi
    add hl,de
    ld a,(hl)
    ld (dest_addr+1),a
    ld hl,y_addr_bank
    add hl,de
    ld a,(hl)
.y_done:
    add a,0x10                  ; Add VRAM_START base
    ld (dest_addr+2),a
    xor a
    ld (dest_addr+3),a

    ; Add char_x * 4 to dest
    ld hl,(char_x)
    add hl,hl                   ; * 2
    add hl,hl                   ; * 4
    ld de,(dest_addr)
    add hl,de
    ld a,l
    ld (BLT_DST_0),a
    ld a,h
    ld (BLT_DST_1),a
    ld a,(dest_addr+2)
    jr nc,.no_carry_dst
    inc a
.no_carry_dst:
    ld (BLT_DST_2),a
    xor a
    ld (BLT_DST_3),a

    ; Set dimensions and strides
    SET_BLT_WIDTH CHAR_WIDTH
    SET_BLT_HEIGHT CHAR_HEIGHT
    SET_SRC_STRIDE FONT_STRIDE
    SET_DST_STRIDE LINE_BYTES

    START_BLIT

.next_char:
    ; Advance message pointer
    ld hl,(msg_ptr)
    inc hl
    ld (msg_ptr),hl

    ; Advance X by CHAR_WIDTH
    ld hl,(char_x)
    ld bc,CHAR_WIDTH
    add hl,bc
    ld (char_x),hl

    ; Check counter
    ld a,(char_count)
    inc a
    ld (char_count),a
    cp 21
    jp c,.char_loop

.done:
    ret

.wrap_scroll:
    ; Wrap scroll position
    ld a,(scroll_x_lo)
    and 0x1F
    ld (scroll_x_lo),a
    xor a
    ld (scroll_x_hi),a
    ld (char_idx),a
    ld (char_idx+1),a
    ; Reset pointer
    ld hl,scroll_message
    ld (msg_ptr),hl
    jp .char_loop

; ============================================================================
; VARIABLES (in RAM at 0xC000)
; ============================================================================
    .org 0xC000

frame_lo:       .word 0         ; Frame counter (16-bit)
frame_hi:       .word 0         ; Extended counter
prev_x:         .word 0         ; Previous sprite X
prev_y:         .word 0         ; Previous sprite Y
curr_x:         .word 0         ; Current sprite X
curr_y:         .word 0         ; Current sprite Y
scroll_x_lo:    .word 0         ; Scroll position
scroll_x_hi:    .word 0
dest_addr:      .space 4        ; 32-bit destination address
scroll_offset:  .byte 0         ; Bar scroll offset
bar_idx:        .byte 0         ; Current bar index
char_idx:       .word 0         ; Scrolltext character index
char_x:         .word 0         ; Scrolltext X position
char_count:     .byte 0         ; Character counter
font_offset:    .space 4        ; Font offset (32-bit)
scroll_y:       .word 0         ; Scrolltext Y position
msg_ptr:        .word 0         ; Current message pointer
curr_char:      .byte 0         ; Current character being drawn

; Y address lookup tables
y_addr_lo:      .space 480      ; Low byte of Y * LINE_BYTES
y_addr_hi:      .space 480      ; High byte
y_addr_bank:    .space 480      ; Bank number

; ============================================================================
; COPPER LIST (at 0xD200 - moved to avoid overlap with sine tables)
; ============================================================================
    .org 0xD200

copper_list:
    ; Bar 0 at Y=40
    .long 40*COP_WAIT_SCALE      ; WAIT
    .long COP_MOVE_RASTER_Y     ; MOVE RASTER_Y
    .long 40
    .long COP_MOVE_RASTER_H     ; MOVE RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR ; MOVE COLOR
    .long 0xFF0000FF            ; (updated dynamically)
    .long COP_MOVE_RASTER_CTRL  ; MOVE CTRL
    .long 1

    ; Bar 1 at Y=64
    .long 64*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 64
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF0040FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 2 at Y=88
    .long 88*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 88
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF0080FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 3 at Y=112
    .long 112*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 112
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF00C0FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 4 at Y=136
    .long 136*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 136
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF00FF80
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 5 at Y=160
    .long 160*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 160
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF00FF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 6 at Y=184
    .long 184*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 184
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF40FF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 7 at Y=208
    .long 208*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 208
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF80FF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 8 at Y=232
    .long 232*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 232
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFFFF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 9 at Y=256
    .long 256*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 256
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFFC000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 10 at Y=280
    .long 280*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 280
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF8000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 11 at Y=304
    .long 304*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 304
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF4000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 12 at Y=328
    .long 328*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 328
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF0000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 13 at Y=352
    .long 352*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 352
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF00FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 14 at Y=376
    .long 376*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 376
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF8000FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Bar 15 at Y=400
    .long 400*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 400
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF4000FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; END
    .long COP_END

; ============================================================================
; READ-ONLY DATA - Sine/Cosine Tables (page-aligned for fast lookup)
; ============================================================================
    .org 0xC800                  ; Page-aligned for efficient lookup

; Sine table for X movement (256 entries, 16-bit signed scaled to +-200)
    .align 256
sin_x_lo:
    .byte 0x00, 0x05, 0x0A, 0x0F, 0x14, 0x18, 0x1D, 0x22
    .byte 0x27, 0x2C, 0x31, 0x35, 0x3A, 0x3F, 0x43, 0x48
    .byte 0x4D, 0x51, 0x56, 0x5A, 0x5E, 0x63, 0x67, 0x6B
    .byte 0x6F, 0x73, 0x77, 0x7B, 0x7F, 0x83, 0x86, 0x8A
    .byte 0x8D, 0x91, 0x94, 0x97, 0x9B, 0x9E, 0xA1, 0xA4
    .byte 0xA6, 0xA9, 0xAC, 0xAE, 0xB0, 0xB3, 0xB5, 0xB7
    .byte 0xB9, 0xBB, 0xBC, 0xBE, 0xBF, 0xC1, 0xC2, 0xC3
    .byte 0xC4, 0xC5, 0xC6, 0xC6, 0xC7, 0xC7, 0xC8, 0xC8
    .byte 0xC8, 0xC8, 0xC8, 0xC7, 0xC7, 0xC6, 0xC6, 0xC5
    .byte 0xC4, 0xC3, 0xC2, 0xC1, 0xBF, 0xBE, 0xBC, 0xBB
    .byte 0xB9, 0xB7, 0xB5, 0xB3, 0xB0, 0xAE, 0xAC, 0xA9
    .byte 0xA6, 0xA4, 0xA1, 0x9E, 0x9B, 0x97, 0x94, 0x91
    .byte 0x8D, 0x8A, 0x86, 0x83, 0x7F, 0x7B, 0x77, 0x73
    .byte 0x6F, 0x6B, 0x67, 0x63, 0x5E, 0x5A, 0x56, 0x51
    .byte 0x4D, 0x48, 0x43, 0x3F, 0x3A, 0x35, 0x31, 0x2C
    .byte 0x27, 0x22, 0x1D, 0x18, 0x14, 0x0F, 0x0A, 0x05
    .byte 0x00, 0xFB, 0xF6, 0xF1, 0xEC, 0xE8, 0xE3, 0xDE
    .byte 0xD9, 0xD4, 0xCF, 0xCB, 0xC6, 0xC1, 0xBD, 0xB8
    .byte 0xB3, 0xAF, 0xAA, 0xA6, 0xA2, 0x9D, 0x99, 0x95
    .byte 0x91, 0x8D, 0x89, 0x85, 0x81, 0x7D, 0x7A, 0x76
    .byte 0x73, 0x6F, 0x6C, 0x69, 0x65, 0x62, 0x5F, 0x5C
    .byte 0x5A, 0x57, 0x54, 0x52, 0x50, 0x4D, 0x4B, 0x49
    .byte 0x47, 0x45, 0x44, 0x42, 0x41, 0x3F, 0x3E, 0x3D
    .byte 0x3C, 0x3B, 0x3A, 0x3A, 0x39, 0x39, 0x38, 0x38
    .byte 0x38, 0x38, 0x38, 0x39, 0x39, 0x3A, 0x3A, 0x3B
    .byte 0x3C, 0x3D, 0x3E, 0x3F, 0x41, 0x42, 0x44, 0x45
    .byte 0x47, 0x49, 0x4B, 0x4D, 0x50, 0x52, 0x54, 0x57
    .byte 0x5A, 0x5C, 0x5F, 0x62, 0x65, 0x69, 0x6C, 0x6F
    .byte 0x73, 0x76, 0x7A, 0x7D, 0x81, 0x85, 0x89, 0x8D
    .byte 0x91, 0x95, 0x99, 0x9D, 0xA2, 0xA6, 0xAA, 0xAF
    .byte 0xB3, 0xB8, 0xBD, 0xC1, 0xC6, 0xCB, 0xCF, 0xD4
    .byte 0xD9, 0xDE, 0xE3, 0xE8, 0xEC, 0xF1, 0xF6, 0xFB

    .align 256
sin_x_hi:
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF

    .align 256
cos_y_lo:
    .byte 0x96, 0x96, 0x96, 0x96, 0x95, 0x95, 0x94, 0x94
    .byte 0x93, 0x92, 0x92, 0x91, 0x90, 0x8E, 0x8D, 0x8C
    .byte 0x8B, 0x89, 0x88, 0x86, 0x84, 0x83, 0x81, 0x7F
    .byte 0x7D, 0x7B, 0x78, 0x76, 0x74, 0x72, 0x6F, 0x6D
    .byte 0x6A, 0x67, 0x65, 0x62, 0x5F, 0x5C, 0x59, 0x56
    .byte 0x53, 0x50, 0x4D, 0x4A, 0x47, 0x43, 0x40, 0x3D
    .byte 0x39, 0x36, 0x33, 0x2F, 0x2C, 0x28, 0x24, 0x21
    .byte 0x1D, 0x1A, 0x16, 0x12, 0x0F, 0x0B, 0x07, 0x04
    .byte 0x00, 0xFC, 0xF9, 0xF5, 0xF1, 0xEE, 0xEA, 0xE6
    .byte 0xE3, 0xDF, 0xDC, 0xD8, 0xD4, 0xD1, 0xCD, 0xCA
    .byte 0xC7, 0xC3, 0xC0, 0xBD, 0xB9, 0xB6, 0xB3, 0xB0
    .byte 0xAD, 0xAA, 0xA7, 0xA4, 0xA1, 0x9E, 0x9B, 0x99
    .byte 0x96, 0x93, 0x91, 0x8E, 0x8C, 0x8A, 0x88, 0x85
    .byte 0x83, 0x81, 0x7F, 0x7D, 0x7C, 0x7A, 0x78, 0x77
    .byte 0x75, 0x74, 0x73, 0x72, 0x70, 0x6F, 0x6E, 0x6E
    .byte 0x6D, 0x6C, 0x6C, 0x6B, 0x6B, 0x6A, 0x6A, 0x6A
    .byte 0x6A, 0x6A, 0x6A, 0x6A, 0x6B, 0x6B, 0x6C, 0x6C
    .byte 0x6D, 0x6E, 0x6E, 0x6F, 0x70, 0x72, 0x73, 0x74
    .byte 0x75, 0x77, 0x78, 0x7A, 0x7C, 0x7D, 0x7F, 0x81
    .byte 0x83, 0x85, 0x88, 0x8A, 0x8C, 0x8E, 0x91, 0x93
    .byte 0x96, 0x99, 0x9B, 0x9E, 0xA1, 0xA4, 0xA7, 0xAA
    .byte 0xAD, 0xB0, 0xB3, 0xB6, 0xB9, 0xBD, 0xC0, 0xC3
    .byte 0xC7, 0xCA, 0xCD, 0xD1, 0xD4, 0xD8, 0xDC, 0xDF
    .byte 0xE3, 0xE6, 0xEA, 0xEE, 0xF1, 0xF5, 0xF9, 0xFC
    .byte 0x00, 0x04, 0x07, 0x0B, 0x0F, 0x12, 0x16, 0x1A
    .byte 0x1D, 0x21, 0x24, 0x28, 0x2C, 0x2F, 0x33, 0x36
    .byte 0x39, 0x3D, 0x40, 0x43, 0x47, 0x4A, 0x4D, 0x50
    .byte 0x53, 0x56, 0x59, 0x5C, 0x5F, 0x62, 0x65, 0x67
    .byte 0x6A, 0x6D, 0x6F, 0x72, 0x74, 0x76, 0x78, 0x7B
    .byte 0x7D, 0x7F, 0x81, 0x83, 0x84, 0x86, 0x88, 0x89
    .byte 0x8B, 0x8C, 0x8D, 0x8E, 0x90, 0x91, 0x92, 0x92
    .byte 0x93, 0x94, 0x94, 0x95, 0x95, 0x96, 0x96, 0x96

    .align 256
cos_y_hi:
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00

; Scroll sine table for Y wave effect (256 entries, +-16 pixels)
    .align 256
scroll_sine_lo:
    .byte 0x00, 0x00, 0x01, 0x01, 0x02, 0x02, 0x03, 0x03
    .byte 0x04, 0x04, 0x05, 0x05, 0x06, 0x06, 0x07, 0x07
    .byte 0x08, 0x08, 0x09, 0x09, 0x0A, 0x0A, 0x0A, 0x0B
    .byte 0x0B, 0x0C, 0x0C, 0x0C, 0x0D, 0x0D, 0x0D, 0x0E
    .byte 0x0E, 0x0E, 0x0E, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F
    .byte 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10
    .byte 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10
    .byte 0x10, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0E, 0x0E
    .byte 0x0E, 0x0E, 0x0D, 0x0D, 0x0D, 0x0C, 0x0C, 0x0C
    .byte 0x0B, 0x0B, 0x0A, 0x0A, 0x0A, 0x09, 0x09, 0x08
    .byte 0x08, 0x07, 0x07, 0x06, 0x06, 0x05, 0x05, 0x04
    .byte 0x04, 0x03, 0x03, 0x02, 0x02, 0x01, 0x01, 0x00
    .byte 0x00, 0x00, 0xFF, 0xFF, 0xFE, 0xFE, 0xFD, 0xFD
    .byte 0xFC, 0xFC, 0xFB, 0xFB, 0xFA, 0xFA, 0xF9, 0xF9
    .byte 0xF8, 0xF8, 0xF7, 0xF7, 0xF6, 0xF6, 0xF6, 0xF5
    .byte 0xF5, 0xF4, 0xF4, 0xF4, 0xF3, 0xF3, 0xF3, 0xF2
    .byte 0xF2, 0xF2, 0xF2, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1
    .byte 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0
    .byte 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0
    .byte 0xF0, 0xF1, 0xF1, 0xF1, 0xF1, 0xF1, 0xF2, 0xF2
    .byte 0xF2, 0xF2, 0xF3, 0xF3, 0xF3, 0xF4, 0xF4, 0xF4
    .byte 0xF5, 0xF5, 0xF6, 0xF6, 0xF6, 0xF7, 0xF7, 0xF8
    .byte 0xF8, 0xF9, 0xF9, 0xFA, 0xFA, 0xFB, 0xFB, 0xFC
    .byte 0xFC, 0xFD, 0xFD, 0xFE, 0xFE, 0xFF, 0xFF, 0x00
    .byte 0x00, 0x00, 0x01, 0x01, 0x02, 0x02, 0x03, 0x03
    .byte 0x04, 0x04, 0x05, 0x05, 0x06, 0x06, 0x07, 0x07
    .byte 0x08, 0x08, 0x09, 0x09, 0x0A, 0x0A, 0x0A, 0x0B
    .byte 0x0B, 0x0C, 0x0C, 0x0C, 0x0D, 0x0D, 0x0D, 0x0E
    .byte 0x0E, 0x0E, 0x0E, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F
    .byte 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10
    .byte 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10
    .byte 0x10, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0E, 0x0E

    .align 256
scroll_sine_hi:
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF
    .byte 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00

; Color palette for copper bars (16 colors, BGRA format)
palette:
    .byte 0xFF, 0x00, 0x00, 0xFF        ; Blue
    .byte 0xFF, 0x40, 0x00, 0xFF        ; Blue-cyan
    .byte 0xFF, 0x80, 0x00, 0xFF        ; Cyan
    .byte 0xFF, 0xC0, 0x00, 0xFF        ; Cyan-green
    .byte 0x80, 0xFF, 0x00, 0xFF        ; Green-yellow
    .byte 0x00, 0xFF, 0x00, 0xFF        ; Green
    .byte 0x00, 0xFF, 0x40, 0xFF        ; Green-yellow
    .byte 0x00, 0xFF, 0x80, 0xFF        ; Yellow
    .byte 0x00, 0xFF, 0xFF, 0xFF        ; Yellow
    .byte 0x00, 0xC0, 0xFF, 0xFF        ; Orange
    .byte 0x00, 0x80, 0xFF, 0xFF        ; Orange-red
    .byte 0x00, 0x40, 0xFF, 0xFF        ; Red
    .byte 0x00, 0x00, 0xFF, 0xFF        ; Red
    .byte 0xFF, 0x00, 0xFF, 0xFF        ; Magenta
    .byte 0xFF, 0x00, 0x80, 0xFF        ; Purple
    .byte 0xFF, 0x00, 0x40, 0xFF        ; Blue-purple

; Scroll character table (96 entries * 4 bytes = 384 bytes)
; Maps ASCII 32-127 to font offsets (32x32 chars, 4 bytes/pixel, 10 chars/row)
; Font offset = ((char - 32) % 10) * 128 + ((char - 32) / 10) * 40960
scroll_char_tbl:
    ; ASCII 32-47: space and punctuation
    .long 0                     ; 32 ' ' space
    .long 128                   ; 33 '!'
    .long 256                   ; 34 '"'
    .long 0                     ; 35 '#'
    .long 0                     ; 36 '$'
    .long 0                     ; 37 '%'
    .long 40960                 ; 38 '&'
    .long 896                   ; 39 '''
    .long 1024                  ; 40 '('
    .long 1152                  ; 41 ')'
    .long 512                   ; 42 '*'
    .long 41600                 ; 43 '+'
    .long 41216                 ; 44 ','
    .long 41344                 ; 45 '-'
    .long 41472                 ; 46 '.'
    .long 0                     ; 47 '/'
    ; ASCII 48-57: digits 0-9
    .long 41728                 ; 48 '0'
    .long 41856                 ; 49 '1'
    .long 41984                 ; 50 '2'
    .long 42112                 ; 51 '3'
    .long 81920                 ; 52 '4'
    .long 82048                 ; 53 '5'
    .long 82176                 ; 54 '6'
    .long 82304                 ; 55 '7'
    .long 82432                 ; 56 '8'
    .long 82560                 ; 57 '9'
    ; ASCII 58-64: punctuation
    .long 82688                 ; 58 ':'
    .long 82816                 ; 59 ';'
    .long 0                     ; 60 '<'
    .long 82944                 ; 61 '='
    .long 0                     ; 62 '>'
    .long 122880                ; 63 '?'
    .long 384                   ; 64 '@'
    ; ASCII 65-90: uppercase A-Z
    .long 123264                ; 65 'A'
    .long 123392                ; 66 'B'
    .long 123520                ; 67 'C'
    .long 123648                ; 68 'D'
    .long 123776                ; 69 'E'
    .long 123904                ; 70 'F'
    .long 124032                ; 71 'G'
    .long 163840                ; 72 'H'
    .long 163968                ; 73 'I'
    .long 164096                ; 74 'J'
    .long 164224                ; 75 'K'
    .long 164352                ; 76 'L'
    .long 164480                ; 77 'M'
    .long 164608                ; 78 'N'
    .long 164736                ; 79 'O'
    .long 164864                ; 80 'P'
    .long 164992                ; 81 'Q'
    .long 204800                ; 82 'R'
    .long 204928                ; 83 'S'
    .long 205056                ; 84 'T'
    .long 205184                ; 85 'U'
    .long 205312                ; 86 'V'
    .long 205440                ; 87 'W'
    .long 205568                ; 88 'X'
    .long 205696                ; 89 'Y'
    .long 205824                ; 90 'Z'
    ; ASCII 91-96: brackets and misc
    .long 83072                 ; 91 '['
    .long 0                     ; 92 '\'
    .long 122752                ; 93 ']'
    .long 768                   ; 94 '^'
    .long 0                     ; 95 '_'
    .long 205952                ; 96 '`'
    ; ASCII 97-122: lowercase (same as uppercase)
    .long 123264                ; 97 'a'
    .long 123392                ; 98 'b'
    .long 123520                ; 99 'c'
    .long 123648                ; 100 'd'
    .long 123776                ; 101 'e'
    .long 123904                ; 102 'f'
    .long 124032                ; 103 'g'
    .long 163840                ; 104 'h'
    .long 163968                ; 105 'i'
    .long 164096                ; 106 'j'
    .long 164224                ; 107 'k'
    .long 164352                ; 108 'l'
    .long 164480                ; 109 'm'
    .long 164608                ; 110 'n'
    .long 164736                ; 111 'o'
    .long 164864                ; 112 'p'
    .long 164992                ; 113 'q'
    .long 204800                ; 114 'r'
    .long 204928                ; 115 's'
    .long 205056                ; 116 't'
    .long 205184                ; 117 'u'
    .long 205312                ; 118 'v'
    .long 205440                ; 119 'w'
    .long 205568                ; 120 'x'
    .long 205696                ; 121 'y'
    .long 205824                ; 122 'z'
    ; ASCII 123-127: braces and misc
    .long 123008                ; 123 '{'
    .long 0                     ; 124 '|'
    .long 0                     ; 125 '}'
    .long 41088                 ; 126 '~'
    .long 0                     ; 127 DEL

; Scroll message text
scroll_message:
    .byte "    ...ROBOCOP Z80 INTRO FOR THE INTUITION ENGINE... "
    .byte "...100 PERCENT Z80 ASM CODE... "
    .byte "...SPRITE ANIMATION WITH BLITTER... "
    .byte "...COPPER BARS WITH COLOR CYCLING... "
    .byte "...SINE WAVE SCROLLTEXT... "
    .byte "...ALL CODE BY INTUITION...  "
    .byte "MUSIC BY JONATHAN DUNN FROM THE 1987 ZX SPECTRUM GAME ROBOCOP BY OCEAN SOFTWARE... "
    .byte "...AY REGISTERS ARE REMAPPED TO THE INTUITION ENGINE SYNTH FOR SUPERIOR SOUND QUALITY... "
    .byte "...GREETS TO ...GADGETMASTER... ...KARLOS... ...BLOODLINE... "
    .byte "...VISIT INTUITIONSUBSYNTH.COM......................."
    .byte 0

; ============================================================================
; EMBEDDED BINARY DATA
; ============================================================================
; All graphics, mask, and audio data is embedded here.
; The blitter accesses this data via 32-bit addresses.
; Data is placed contiguously starting at 0xE000.
; ============================================================================

    .org 0xE000                  ; Data starts after stack area

; Sprite RGBA data (240x180x4 = 172800 bytes)
data_robocop_rgba:
    .incbin "../robocop_rgba.bin"

; Sprite mask data (240x180/8 = 5400 bytes, 1 bit per pixel)
data_robocop_mask:
    .incbin "../robocop_mask.bin"

; AY music data (24525 bytes)
data_robocop_ay:
    .incbin "../assets/music/Robocop1.ay"

; Font RGBA data (256000 bytes)
data_font_rgba:
    .incbin "../font_rgba.bin"

; ============================================================================
; END OF FILE
; ============================================================================
