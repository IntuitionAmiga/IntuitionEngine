; ============================================================================
; ROBOCOP INTRO (Z80 PORT) - Blitter Sprite, Copper Rasterbars and PSG Music
; Zilog Z80 assembly for IntuitionEngine - VideoChip + Copper + PSG audio
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Zilog Z80 (8-bit, 16-bit address bus)
; Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true colour)
; Audio Engine:  PSG (AY-3-8910 compatible, PSG+ enhanced mode)
; Assembler:     vasmz80_std
; Build:         vasmz80_std -Fbin -o robocop_intro_z80.ie80 robocop_intro_z80.asm
; Run:           ./IntuitionEngine -z80 robocop_intro_z80.ie80
; Porting:       See robocop_intro.asm (IE32 reference), robocop_intro_65.asm
;                (6502), robocop_intro_68k.asm (M68K)
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
; === Z80-SPECIFIC NOTES ===
; The Z80 has only a 16-bit address bus (64KB), so all data beyond the
; code space must be accessed through the Intuition Engine's banking
; system. Sprite, mask, font, and music data are assigned to numbered
; 8KB banks, with three bank windows at 0x2000, 0x4000, and 0x6000
; providing sliding views into extended memory. VRAM is accessed through
; a 16KB bank window at 0x8000.
;
; Sine/cosine tables are page-aligned (256-byte boundaries) so that the
; low byte of the index can be loaded directly into L while the high
; byte of the table base is loaded into H -- a single `ld h,>table_page`
; gives instant indexed access with no addition required. Split high/low
; byte tables avoid the need for 16-bit arithmetic on each lookup. The
; IX and IY index registers are used during table construction. All
; runtime variables and lookup tables are kept in main memory (0xC000+)
; for fast access without bank switching.
;
; === MEMORY MAP ===
; 0x0000-0x00FF   Entry point
; 0x0100-0x1FFF   Programme code (~8KB)
; 0x2000-0x3FFF   Bank 1 window (8KB) - sprite RGBA data
; 0x4000-0x5FFF   Bank 2 window (8KB) - font RGBA data
; 0x6000-0x7FFF   Bank 3 window (8KB) - AY music / general
; 0x8000-0xBFFF   VRAM bank window (16KB)
; 0xC000-0xCFFF   Variables and lookup tables
; 0xD000-0xDFFF   Copper list
; 0xE000-0xEFFF   Stack
; 0xF000-0xFFFF   I/O region (mapped to 0xF0000-0xF0FFF)
;
; === BUILD AND RUN ===
; Build:  vasmz80_std -Fbin -o robocop_intro_z80.ie80 robocop_intro_z80.asm
; Run:    ./IntuitionEngine -z80 robocop_intro_z80.ie80
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie80.inc"

; ----------------------------------------------------------------------------
; DATA ORGANISATION
; ----------------------------------------------------------------------------
; The Z80 can only directly address 64KB, so all binary data (sprite,
; mask, font, music) is placed in extended memory and accessed through
; numbered 8KB banks. The bank layout below keeps related data together
; and minimises bank-switching overhead during rendering.
;
; Bank 0-21  (0x00000-0x2BFFF):  Sprite RGBA data (172KB = 22 banks)
; Bank 22-27 (0x2C000-0x37FFF):  Sprite mask (43KB = 6 banks)
; Bank 28-30 (0x38000-0x3DFFF):  AY music data (24KB = 3 banks)
; Bank 31-56 (0x3E000-0x71FFF):  Font RGBA data (200KB = 26 banks)
;
; Lookup tables and the copper list remain in main memory (0xC000+) so
; they can be read without any bank switching.
; ----------------------------------------------------------------------------

; Sprite constants
.set SPRITE_W,240
.set SPRITE_H,180
.set SPRITE_STRIDE,960        ; 240 pixels x 4 bytes per pixel
.set CENTER_X,200             ; Horizontal centre of Lissajous path
.set CENTER_Y,150             ; Vertical centre of Lissajous path

; Copper bar layout constants
.set BAR_COUNT,16
.set BAR_STRIDE,36            ; Bytes per bar entry in copper list

; Scrolltext constants
.set SCROLL_Y,430             ; Vertical position of scrolltext
.set SCROLL_SPEED,2           ; Pixels advanced per frame
.set CHAR_WIDTH,32            ; Glyph width in pixels
.set CHAR_HEIGHT,32           ; Glyph height in pixels
.set FONT_STRIDE,1280         ; 320 pixels x 4 bytes per row (10 chars wide)

; Bank assignments for embedded binary data (8KB bank numbers)
.set SPRITE_BANK_START,0      ; Sprite RGBA starts at bank 0
.set MASK_BANK_START,22       ; Sprite mask starts at bank 22
.set AY_BANK_START,28         ; AY music at bank 28
.set FONT_BANK_START,31       ; Font RGBA at bank 31

; AY music file length
.set ROBOCOP_AY_LEN,24525

; ============================================================================
; ENTRY POINT
; ============================================================================

    .org 0x0000

start:
    di                          ; Disable interrupts during initialisation
    ld sp,STACK_TOP             ; Set up stack pointer

    call init_video
    call init_tables
    call init_psg
    call init_copper

    ; Clear all runtime state to zero
    xor a
    ld (frame_lo),a
    ld (frame_hi),a
    ld (scroll_x_lo),a
    ld (scroll_x_hi),a

    ; Seed the previous position with the initial computed position so
    ; the first frame does not clear a garbage rectangle
    call compute_xy
    ld hl,(curr_x)
    ld (prev_x),hl
    ld hl,(curr_y)
    ld (prev_y),hl

; ----------------------------------------------------------------------------
; MAIN LOOP
; Each iteration: advance frame counter, update copper colours, compute new
; sprite position, synchronise to vertical blank, clear old sprite, draw new
; sprite, update scrolltext, and advance the scroll position.
; ----------------------------------------------------------------------------
main_loop:
    ; Increment 16-bit frame counter
    ld hl,(frame_lo)
    inc hl
    ld (frame_lo),hl

    ; Cycle copper bar colours for this frame
    call update_bars

    ; Compute new sprite position from sine/cosine tables
    call compute_xy

    ; Synchronise to vertical blank before any drawing
    call wait_frame

    ; Erase the sprite at its previous position (black fill)
    call clear_prev_sprite

    ; Draw the masked sprite at its new position
    call draw_sprite

    ; Save current position as previous for next frame's erase
    ld hl,(curr_x)
    ld (prev_x),hl
    ld hl,(curr_y)
    ld (prev_y),hl

    ; Clear the scrolltext strip and render current characters
    call clear_scroll_area
    call draw_scrolltext

    ; Ensure all blitter operations complete before modifying state
    call wait_blit

    ; Advance horizontal scroll position
    ld hl,(scroll_x_lo)
    ld bc,SCROLL_SPEED
    add hl,bc
    ld (scroll_x_lo),hl

    jp main_loop

; ============================================================================
; SUBROUTINES
; ============================================================================

; ----------------------------------------------------------------------------
; init_video -- Set 640x480 mode and clear the screen to black
; ----------------------------------------------------------------------------
init_video:
    ; Select mode 0 (640x480, 32bpp true colour)
    xor a
    ld (VIDEO_MODE),a
    ld (VIDEO_MODE+1),a
    ld (VIDEO_MODE+2),a
    ld (VIDEO_MODE+3),a

    ; Enable the video output
    ld a,1
    ld (VIDEO_CTRL),a

    ; Use the blitter to fill the entire framebuffer with opaque black
    call wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Destination: VRAM start at 0x100000
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
; init_tables -- Build the Y-address lookup table
; Pre-computes y_addr[y] = y * 2560 (LINE_BYTES) for all 480 scanlines.
; The result is split into three tables (lo, hi, bank) so VRAM address
; calculation only requires table lookups and a small addition for X.
; ----------------------------------------------------------------------------
init_tables:
    ld hl,0                     ; Running 16-bit offset (wraps on overflow)
    ld de,0                     ; DE tracks bank number (high bits)
    ld ix,y_addr_lo             ; Pointer into low-byte table
    ld iy,y_addr_hi             ; Pointer into high-byte table
    ld bc,480                   ; Loop counter: one entry per scanline

.init_loop:
    ; Store low byte of current offset
    ld a,l
    ld (ix+0),a

    ; Store high byte of current offset
    ld a,h
    ld (iy+0),a

    ; Store bank number at y_addr_bank[index]
    push hl
    push bc
    push de

    ; Calculate index = 480 - remaining count
    ld hl,480
    or a                        ; Clear carry for SBC
    sbc hl,bc                   ; HL = current index
    ex de,hl                    ; DE = index
    ld hl,y_addr_bank
    add hl,de                   ; HL points into bank table
    pop de
    ld a,e                      ; Bank number from DE
    ld (hl),a

    pop bc
    pop hl

    ; Advance table pointers
    inc ix
    inc iy

    ; Add LINE_BYTES (2560 = 0x0A00) to running offset
    push bc
    ld bc,0x0A00
    add hl,bc
    jr nc,.no_bank_inc
    inc de                      ; Carry means we crossed a 64KB boundary
.no_bank_inc:
    pop bc

    dec bc
    ld a,b
    or c
    jr nz,.init_loop

    ret

; ----------------------------------------------------------------------------
; init_psg -- Enable PSG+ enhanced mode and start music playback
; PSG+ remaps the original AY-3-8910 register writes through the Intuition
; Engine's synthesiser for higher-fidelity sound reproduction.
; ----------------------------------------------------------------------------
init_psg:
    ; Enable PSG+ enhanced mode
    ld a,1
    ld (PSG_PLUS_CTRL),a

    ; Point the PSG player at the embedded AY music data
    SET_PSG_PTR data_robocop_ay

    ; Set the total length of the AY file
    SET_PSG_LEN ROBOCOP_AY_LEN

    ; Start playback with looping (bit 0 = start, bit 2 = loop)
    ld a,5
    ld (PSG_PLAY_CTRL),a

    ret

; ----------------------------------------------------------------------------
; init_copper -- Programme the copper list and enable the coprocessor
; The copper list is pre-built in memory at copper_list; this routine
; simply points the coprocessor at it and enables execution.
; ----------------------------------------------------------------------------
init_copper:
    ; Disable copper before changing the list pointer
    ld a,2
    ld (COPPER_CTRL),a

    ; Set copper list base address
    SET_COPPER_PTR copper_list

    ; Enable copper execution
    ld a,1
    ld (COPPER_CTRL),a

    ret

; ----------------------------------------------------------------------------
; wait_blit -- Spin until the blitter signals idle
; Bit 1 of BLT_CTRL is the busy flag; we poll until it clears.
; ----------------------------------------------------------------------------
wait_blit:
    ld a,(BLT_CTRL)
    and 2
    jr nz,wait_blit
    ret

; ----------------------------------------------------------------------------
; wait_frame -- Synchronise to the start of vertical blank
; Two-phase wait: first wait for active scan (vblank flag clear), then
; wait for the vblank flag to assert, ensuring we catch a fresh edge.
; ----------------------------------------------------------------------------
wait_frame:
    ; Phase 1: wait until we are NOT in vblank (active scan)
.wait_not_vblank:
    ld a,(VIDEO_STATUS)
    and STATUS_VBLANK
    jr nz,.wait_not_vblank

    ; Phase 2: wait until vblank begins
.wait_vblank:
    ld a,(VIDEO_STATUS)
    and STATUS_VBLANK
    jr z,.wait_vblank

    ret

; ----------------------------------------------------------------------------
; compute_xy -- Calculate sprite position from the frame counter
; X follows a sine curve, Y follows a cosine curve at double speed,
; producing a Lissajous figure. The split lo/hi tables provide 16-bit
; signed values via page-aligned lookup: ld h,>table_page is all that
; is needed to switch between the low and high byte halves.
; ----------------------------------------------------------------------------
compute_xy:
    ; X = sin_table[frame & 0xFF] + CENTER_X
    ld a,(frame_lo)
    ld l,a
    ld h,>sin_x_lo          ; Page-aligned table base (high byte only)
    ld e,(hl)                   ; Low byte of sine value
    ld h,>sin_x_hi
    ld d,(hl)                   ; High byte (sign extension)

    ld hl,CENTER_X
    add hl,de
    ld (curr_x),hl

    ; Y = cos_table[(frame * 2) & 0xFF] + CENTER_Y
    ld a,(frame_lo)
    add a,a                     ; Double the index for the cosine curve
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
; update_bars -- Cycle copper bar colours using a scrolling gradient
; Each of the 16 bars picks a colour from a 16-entry palette, offset by
; the frame counter and a sine-derived scroll value, producing a smooth
; rainbow wave effect.
; ----------------------------------------------------------------------------
update_bars:
    ; Derive a scroll offset from the sine table
    ld a,(frame_lo)
    add a,a                     ; Faster scroll rate
    ld l,a
    ld h,>sin_x_lo
    ld a,(hl)
    add a,200                   ; Shift into positive range
    rrca
    rrca
    rrca
    rrca                        ; Divide by 16
    and 0x0F
    ld (scroll_offset),a

    ; Iterate over all 16 bars, updating each colour entry
    ld b,BAR_COUNT
    xor a
    ld (bar_idx),a

    ld hl,copper_list + 24      ; Offset to first colour value in copper list

.bar_loop:
    push hl
    push bc

    ; Colour index = (bar_idx + scroll_offset + frame/4) & 0x0F
    ld a,(frame_lo)
    srl a
    srl a                       ; frame / 4
    ld b,a
    ld a,(bar_idx)
    add a,b
    ld b,a
    ld a,(scroll_offset)
    add a,b
    and 0x0F                    ; Wrap to 16 palette entries

    ; Index into the 16-colour palette (4 bytes per entry)
    add a,a
    add a,a                     ; index * 4
    ld e,a
    ld d,0
    ld hl,palette
    add hl,de                   ; HL = palette + colour_index * 4

    ; Calculate the copper list destination for this bar's colour field:
    ; copper_list + 24 + bar_idx * BAR_STRIDE
    pop bc
    push bc

    push hl                     ; Save palette source pointer
    ld hl,copper_list + 24
    ld a,(bar_idx)
.mul_stride:
    or a
    jr z,.mul_done
    push bc
    ld bc,BAR_STRIDE
    add hl,bc               ; Advance by one BAR_STRIDE per iteration
    pop bc
    dec a
    jr .mul_stride
.mul_done:
    ld de,0                     ; Clear DE before exchange
    ex de,hl                    ; DE = copper list colour address
    pop hl                      ; HL = palette source

    ; Copy 4 colour bytes (BGRA) into the copper list
    ldi
    ldi
    ldi
    ldi

    ; Advance bar index
    ld a,(bar_idx)
    inc a
    ld (bar_idx),a

    pop bc
    pop hl

    ; Advance the copper list pointer by BAR_STRIDE for next iteration
    push bc
    ld bc,BAR_STRIDE
    add hl,bc
    pop bc

    djnz .bar_loop

    ret

; ----------------------------------------------------------------------------
; clear_prev_sprite -- Erase the sprite at its previous position
; Fills a SPRITE_W x SPRITE_H rectangle with opaque black.
; ----------------------------------------------------------------------------
clear_prev_sprite:
    call wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Compute VRAM address from previous X,Y
    call calc_vram_addr_prev

    ; Write the 32-bit destination address to blitter registers
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
; draw_sprite -- Draw the Robocop sprite at its current position
; Uses a masked blit: the mask data determines which pixels are
; transparent, allowing the sprite to appear over the background.
; ----------------------------------------------------------------------------
draw_sprite:
    call wait_blit

    SET_BLT_OP BLT_OP_MASKED

    ; Source: embedded sprite RGBA pixel data
    SET_BLT_SRC data_robocop_rgba

    ; Mask: 1-bit-per-pixel transparency mask
    SET_BLT_MASK data_robocop_mask

    ; Compute VRAM address from current X,Y
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
; calc_vram_addr_prev -- Compute 32-bit VRAM address for previous position
; Looks up the Y offset from the pre-computed table, adds the VRAM base
; (0x100000), then adds X * 4 for the horizontal pixel offset.
; Result is stored in the 4-byte dest_addr variable.
; ----------------------------------------------------------------------------
calc_vram_addr_prev:
    ; Look up Y * LINE_BYTES from the pre-computed table
    ld hl,(prev_y)

    ; The table is split at index 256, so branch on high byte
    ld a,h
    or a
    jr nz,.high_y

    ; Y < 256: index directly into the first 256 entries
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
    jr .add_vram_base

.high_y:
    ; Y >= 256: offset into the second half of each table
    ld hl,(prev_y)
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

.add_vram_base:
    ; Add VRAM_START high byte (0x10) to bank value
    add a,0x10
    ld (dest_addr+2),a
    xor a
    ld (dest_addr+3),a

    ; Add X * 4 (4 bytes per pixel) to the base offset
    ld hl,(prev_x)
    add hl,hl                   ; * 2
    add hl,hl                   ; * 4

    ; 16-bit addition into the lower 16 bits of dest_addr
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
; calc_vram_addr_curr -- Compute 32-bit VRAM address for current position
; Same algorithm as calc_vram_addr_prev but reads curr_x/curr_y instead.
; ----------------------------------------------------------------------------
calc_vram_addr_curr:
    ld hl,(curr_y)

    ld a,h
    or a
    jr nz,.curr_high_y

    ; Y < 256
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
    ; Y >= 256
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
; clear_scroll_area -- Fill the scrolltext strip with opaque black
; Clears a 640 x 90 pixel band starting at scanline 390, covering the
; region where scrolltext characters will be drawn.
; ----------------------------------------------------------------------------
clear_scroll_area:
    call wait_blit

    SET_BLT_OP BLT_OP_FILL

    ; Destination: VRAM_START + 390 * LINE_BYTES
    ; 390 * 2560 = 998400 = 0x0F3C00, plus VRAM base 0x100000 = 0x1F3C00
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
; draw_scrolltext -- Render scrolling text with a sine-wave vertical offset
; Characters are drawn left to right across the screen. Each character's
; vertical position is perturbed by a sine lookup to create the classic
; demoscene wave effect. Characters are blitted from the font sheet.
; ----------------------------------------------------------------------------
draw_scrolltext:
    ; Calculate the starting character index: char_idx = scroll_x >> 5
    ; Each character is 32 pixels wide, so dividing by 32 gives the
    ; index of the first visible character in the scroll message.
    ld hl,(scroll_x_lo)
    ; Shift right 5 bits: combine low and high bytes
    ld a,l
    rrca
    rrca
    rrca
    rrca
    rrca                        ; A = L >> 5 (top 3 bits of L)
    and 0x07                    ; Keep only bottom 3 bits
    ld c,a
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

    ; Calculate sub-character pixel offset: char_x = -(scroll_x & 0x1F)
    ; This is the fractional part that shifts characters smoothly between
    ; whole-character boundaries.
    ld a,(scroll_x_lo)
    and 0x1F                    ; scroll_x modulo 32
    jr z,.char_x_zero
    neg                         ; Negate for leftward shift
    ld (char_x),a
    ld a,0xFF                   ; Sign-extend to 16 bits
    ld (char_x+1),a
    jr .char_x_done
.char_x_zero:
    xor a
    ld (char_x),a
    ld (char_x+1),a
.char_x_done:

    ; Reset character counter
    xor a
    ld (char_count),a

; --- Character rendering loop ---
.char_loop:
    ; Fetch the next character from the scroll message
    ld hl,(msg_ptr)
    ld a,(hl)
    or a
    jr nz,.got_char
    jp .wrap_scroll             ; Null terminator: wrap back to start
.got_char:
    ld (curr_char),a

    ; Clip: skip characters entirely off the left edge
    ld a,(char_x+1)
    bit 7,a
    jr z,.check_right           ; Positive X: check right edge instead
    ld hl,(char_x)
    ld bc,32
    add hl,bc                   ; char_x + CHAR_WIDTH
    bit 7,h
    jr z,.visible               ; Some part is visible
    jp .next_char               ; Entirely off-screen left

.check_right:
    ; Clip: stop once characters exceed the right edge (X >= 608)
    ld hl,(char_x)
    ld bc,-608
    add hl,bc
    jr nc,.visible
    jp .done                    ; Past right edge: finished

.visible:
    ; Validate ASCII range (printable characters 32-127)
    ld a,(curr_char)
    sub 32                      ; ASCII offset (space = index 0)
    jp m,.next_char             ; Below space: skip
    cp 96
    jp nc,.next_char            ; Above 127: skip

    ; Look up the font sheet offset from the character table
    ld l,a
    ld h,0
    add hl,hl                   ; * 2
    add hl,hl                   ; * 4 (4 bytes per table entry)
    ld de,scroll_char_tbl
    add hl,de                   ; HL = scroll_char_tbl + index * 4

    ; Read the 32-bit font offset
    ld e,(hl)
    inc hl
    ld d,(hl)
    inc hl
    ld (font_offset),de
    ld e,(hl)
    inc hl
    ld d,(hl)
    ld (font_offset+2),de

    ; Skip characters with no glyph (offset == 0)
    ld a,(font_offset)
    ld hl,font_offset+1
    or (hl)
    inc hl
    or (hl)
    inc hl
    or (hl)
    jp z,.next_char

    ; Calculate the sine-wave Y offset for this character
    ; sine_index = (char_count * 32 + scroll_x) & 0xFF
    ld a,(char_count)
    rlca
    rlca
    rlca
    rlca
    rlca                        ; char_count * 32
    ld hl,(scroll_x_lo)
    add a,l                     ; Add scroll position
    ld l,a
    ld h,>scroll_sine_lo        ; Page-aligned sine table
    ld e,(hl)
    ld h,>scroll_sine_hi
    ld d,(hl)                   ; DE = sine offset (16-bit signed)

    ld hl,SCROLL_Y
    add hl,de
    ld (scroll_y),hl            ; Final Y position for this character

    ; Set up the blitter to copy one character glyph
    call wait_blit
    SET_BLT_OP BLT_OP_COPY

    ; Source = data_font_rgba + font_offset (32-bit addition)
    ld hl,data_font_rgba & 0xFFFF
    ld de,(font_offset)
    add hl,de
    ld a,l
    ld (BLT_SRC_0),a
    ld a,h
    ld (BLT_SRC_1),a
    ; High byte: font base bank + font_offset[2] + carry from low addition
    ld a,(font_offset+2)
    adc a,(data_font_rgba >> 16) & 0xFF
    ld (BLT_SRC_2),a
    xor a
    ld (BLT_SRC_3),a

    ; Calculate destination VRAM address using the Y lookup table
    ld hl,(scroll_y)
    ld a,h
    or a
    jr z,.y_low
    ; scroll_y >= 256: use second half of tables
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
    ; scroll_y < 256: direct index
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

    ; Add char_x * 4 (pixel offset to byte offset)
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

    ; Set glyph dimensions and strides
    SET_BLT_WIDTH CHAR_WIDTH
    SET_BLT_HEIGHT CHAR_HEIGHT
    SET_SRC_STRIDE FONT_STRIDE
    SET_DST_STRIDE LINE_BYTES

    START_BLIT

.next_char:
    ; Advance to next character in the message
    ld hl,(msg_ptr)
    inc hl
    ld (msg_ptr),hl

    ; Move X position right by one character width
    ld hl,(char_x)
    ld bc,CHAR_WIDTH
    add hl,bc
    ld (char_x),hl

    ; Loop until we have drawn enough characters to fill the screen
    ld a,(char_count)
    inc a
    ld (char_count),a
    cp 21
    jp c,.char_loop

.done:
    ret

.wrap_scroll:
    ; Wrap scroll position back to the start of the message
    ld a,(scroll_x_lo)
    and 0x1F
    ld (scroll_x_lo),a
    xor a
    ld (scroll_x_hi),a
    ld (char_idx),a
    ld (char_idx+1),a
    ; Reset message pointer to beginning
    ld hl,scroll_message
    ld (msg_ptr),hl
    jp .char_loop

; ============================================================================
; VARIABLES (in RAM at 0xC000)
; ============================================================================
    .org 0xC000

frame_lo:       .word 0         ; Frame counter (low 16 bits)
frame_hi:       .word 0         ; Frame counter (high 16 bits, extended)
prev_x:         .word 0         ; Previous sprite X position
prev_y:         .word 0         ; Previous sprite Y position
curr_x:         .word 0         ; Current sprite X position
curr_y:         .word 0         ; Current sprite Y position
scroll_x_lo:    .word 0         ; Horizontal scroll position (low)
scroll_x_hi:    .word 0         ; Horizontal scroll position (high)
dest_addr:      .space 4        ; 32-bit VRAM destination address (scratch)
scroll_offset:  .byte 0         ; Copper bar scroll offset
bar_idx:        .byte 0         ; Current bar index during update_bars
char_idx:       .word 0         ; Scrolltext starting character index
char_x:         .word 0         ; Scrolltext current X pixel position
char_count:     .byte 0         ; Characters drawn so far this frame
font_offset:    .space 4        ; 32-bit font glyph offset (scratch)
scroll_y:       .word 0         ; Scrolltext Y position with sine offset
msg_ptr:        .word 0         ; Pointer into scroll_message
curr_char:      .byte 0         ; Current character being rendered

; ----------------------------------------------------------------------------
; Y-ADDRESS LOOKUP TABLES
; Pre-computed Y * LINE_BYTES split into three byte-wide tables for
; efficient 32-bit address reconstruction on an 8-bit CPU.
; ----------------------------------------------------------------------------
y_addr_lo:      .space 480      ; Low byte of Y * LINE_BYTES
y_addr_hi:      .space 480      ; High byte of Y * LINE_BYTES
y_addr_bank:    .space 480      ; Bank byte of Y * LINE_BYTES

; ============================================================================
; COPPER LIST (at 0xD200)
; ============================================================================
; 16 horizontal colour bars spaced 24 scanlines apart, starting at Y=40.
; Each bar entry consists of: WAIT (scanline trigger), MOVE RASTER_Y
; (bar position), MOVE RASTER_H (bar height), MOVE RASTER_COLOR (colour),
; and MOVE RASTER_CTRL (enable). The colour values are overwritten each
; frame by update_bars to produce the scrolling rainbow gradient.
; ============================================================================
    .org 0xD200

copper_list:
    ; --- Bar 0 at Y=40 ---
    .long 40*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 40
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF0000FF            ; Updated dynamically by update_bars
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 1 at Y=64 ---
    .long 64*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 64
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF0040FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 2 at Y=88 ---
    .long 88*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 88
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF0080FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 3 at Y=112 ---
    .long 112*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 112
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF00C0FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 4 at Y=136 ---
    .long 136*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 136
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF00FF80
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 5 at Y=160 ---
    .long 160*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 160
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF00FF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 6 at Y=184 ---
    .long 184*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 184
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF40FF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 7 at Y=208 ---
    .long 208*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 208
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF80FF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 8 at Y=232 ---
    .long 232*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 232
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFFFF00
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 9 at Y=256 ---
    .long 256*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 256
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFFC000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 10 at Y=280 ---
    .long 280*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 280
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF8000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 11 at Y=304 ---
    .long 304*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 304
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF4000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 12 at Y=328 ---
    .long 328*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 328
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF0000
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 13 at Y=352 ---
    .long 352*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 352
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFFFF00FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 14 at Y=376 ---
    .long 376*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 376
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF8000FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; --- Bar 15 at Y=400 ---
    .long 400*COP_WAIT_SCALE
    .long COP_MOVE_RASTER_Y
    .long 400
    .long COP_MOVE_RASTER_H
    .long 12
    .long COP_MOVE_RASTER_COLOR
    .long 0xFF4000FF
    .long COP_MOVE_RASTER_CTRL
    .long 1

    ; Terminate the copper list
    .long COP_END

; ============================================================================
; SINE AND COSINE TABLES (page-aligned for fast indexed lookup)
; ============================================================================
; All tables are aligned to 256-byte boundaries so that the table base
; address occupies the high byte of a 16-bit pointer. This allows the
; Z80 to index into a table with just:
;     ld l,<index>
;     ld h,>table_page
;     ld a,(hl)
; ...avoiding any 16-bit addition. Each waveform is split into separate
; low-byte and high-byte tables to reconstruct 16-bit signed values.
; ============================================================================
    .org 0xC800

; --- X sine table (256 entries, 16-bit signed, scaled to +-200 pixels) ---
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

; --- X sine high bytes (sign extension: 0x00 for positive, 0xFF for negative) ---
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

; --- Y cosine table (256 entries, 16-bit signed, scaled to +-150 pixels) ---
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

; --- Y cosine high bytes (sign extension) ---
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

; --- Scroll sine table (256 entries, 16-bit signed, +-16 pixels) ---
; Used to apply a vertical sine-wave offset to each scrolltext character.
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

; --- Scroll sine high bytes (sign extension) ---
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

; ============================================================================
; COLOUR PALETTE AND CHARACTER DATA
; ============================================================================

; Copper bar colour palette (16 entries, BGRA format)
; These colours form the rainbow gradient that scrolls across the bars.
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

; Scroll character table (96 entries x 4 bytes = 384 bytes)
; Maps ASCII 32-127 to byte offsets within the font sheet. The font is
; arranged as a 10-character-wide grid of 32x32 pixel glyphs at 4 bytes
; per pixel. Offset = ((char - 32) % 10) * 128 + ((char - 32) / 10) * 40960.
; A zero offset means no glyph is available for that character.
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
    ; ASCII 91-96: brackets and miscellaneous
    .long 83072                 ; 91 '['
    .long 0                     ; 92 '\'
    .long 122752                ; 93 ']'
    .long 768                   ; 94 '^'
    .long 0                     ; 95 '_'
    .long 205952                ; 96 '`'
    ; ASCII 97-122: lowercase mapped to uppercase glyphs
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
    ; ASCII 123-127: braces and miscellaneous
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
; All graphics, mask, and audio data is embedded contiguously starting at
; 0xE000. The blitter accesses this data via 32-bit addresses through the
; banking system, so placement in the Z80's address space is irrelevant --
; what matters is that the data appears at known offsets in the binary.
; ============================================================================

    .org 0xE000

; Sprite RGBA pixel data (240 x 180 x 4 = 172,800 bytes)
data_robocop_rgba:
    .incbin "../assets/robocop_rgba.bin"

; Sprite transparency mask (240 x 180 / 8 = 5,400 bytes, 1 bit per pixel)
data_robocop_mask:
    .incbin "../assets/robocop_mask.bin"

; AY music file (24,525 bytes)
data_robocop_ay:
    .incbin "../assets/music/Robocop1.ay"

; Font RGBA sheet (256,000 bytes)
data_font_rgba:
    .incbin "../assets/font_rgba.bin"

; ============================================================================
; END OF FILE
; ============================================================================
