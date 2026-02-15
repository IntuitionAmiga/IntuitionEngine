; ============================================================================
; VGA TEXT MODE DEMO WITH SAP MUSIC - CLASSIC DEMOSCENE PRODUCTION
; Z80 Assembly for IntuitionEngine - VGA Mode 03h (80x25 Text)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Z80 (Zilog Z80, 8-bit)
; Video Chip:    VGA Mode 03h (80x25 text, 16 foreground + 16 background colours)
; Audio Engine:  POKEY+ (enhanced Atari POKEY emulation via SAP playback)
; Assembler:     vasmz80_std (VASM Z80 assembler, standard syntax)
; Build:         vasmz80_std -Fbin -o vga_text_sap_demo.ie80 vga_text_sap_demo.asm
; Run:           ./bin/IntuitionEngine -z80 vga_text_sap_demo.ie80
; Porting:       VGA text buffer is CPU-agnostic (same 0xB8000 layout).
;                Z80-specific aspects are the I/O port access (IN/OUT)
;                and 16KB bank window for reaching VRAM beyond 64KB.
;
; === WHAT THIS DEMO DOES ===
; 1. Displays a large "INTUITION" ASCII art logo using CP437 shade characters
; 2. Creates animated rainbow raster bars sweeping across all 25 rows
; 3. Cycles the logo through bright colours (classic demoscene effect)
; 4. Shows four info text lines that bounce independently via sine wave
; 5. Scrolls a long message across the bottom of the screen
; 6. Plays Atari SAP music through the POKEY+ enhanced audio engine
;
; === WHY VGA TEXT MODE (MODE 03h) ===
; Before graphical user interfaces, text mode was THE display mode for PCs.
; Mode 03h (80x25 characters, 16 colours) dates back to the IBM CGA/EGA/VGA
; era (1981-1987). Each character cell has two bytes: an ASCII code and an
; attribute byte encoding foreground (4 bits) and background (4 bits) colours.
;
; Demosceners discovered that even in text mode, impressive effects were
; possible by rapidly changing colours per-row (fake raster bars), animating
; custom characters, and using the extended CP437 character set's shade
; blocks (chars 176-178, 219: light/medium/dark shade and full block) to
; create pseudo-graphics.
;
; === WHY THE Z80 CPU ===
; The Zilog Z80 (1976) powered countless home computers: ZX Spectrum, MSX,
; Amstrad CPC, TRS-80, and arcade machines like Pac-Man.  With its extended
; instruction set (IX/IY index registers, block operations, bit manipulation),
; it was more capable than the contemporary 6502, though sharing the 8-bit
; era's emphasis on clever coding to overcome limited registers and speed.
;
; === WHY POKEY SOUND AND SAP FILES ===
; The POKEY (Pot Keyboard Integrated Circuit) was Atari's signature sound
; chip in the 400/800/XL/XE computers.  Its four voices with polynomial
; counters and high-pass filters created the distinctive "Atari sound."
; SAP (Slight Atari Player) is a music file format preserving the 6502
; player code and POKEY register data from thousands of Atari chiptunes.
;
; The Intuition Engine's POKEY+ mode enhances classic SAP playback with
; improved audio quality while maintaining authentic Atari character.
;
; === WHY CP437 SHADE CHARACTERS ===
; Code Page 437 was the original IBM PC character set, containing ASCII
; plus 128 extended characters including box-drawing symbols and the
; famous "shade blocks" (chars 176-178, 219-223) used extensively in
; DOS-era UI design and text-mode art.  These characters enable pseudo-
; graphical effects like our logo's gradient shading.
;
; === MEMORY MAP (Z80 64KB Address Space) ===
;   0x0000-0x0FFF   Program code (~4KB)
;   0x8000-0xBFFF   VRAM bank window (16KB) - text buffer access
;   0xC100-0xC1FF   Sine table (256 bytes)
;   0xE000-0xEFFF   SAP music data (~4KB)
;   0xEFF0          Stack top (grows downward)
;   0xF000-0xFFFF   I/O region
;
; === VGA TEXT BUFFER LAYOUT ===
; Each of the 2000 character cells (80 columns x 25 rows) is 2 bytes:
;   Byte 0: ASCII character code (0-255, CP437)
;   Byte 1: Attribute byte
;            Bits 7-4: Background colour (0-15)
;            Bits 3-0: Foreground colour (0-15)
;
; Screen position (col, row) maps to offset: (row * 160) + (col * 2)
;
; The text buffer lives at physical address 0xB8000.  Since the Z80 can
; only address 64KB, we use a VRAM bank window at 0x8000-0xBFFF.
; Bank 0x2E (46) maps this window to the text buffer:
;   Bank calculation: 0xB8000 / 0x4000 = 46 (0x2E)
;
; === BUILD AND RUN ===
;   vasmz80_std -Fbin -o vga_text_sap_demo.ie80 vga_text_sap_demo.asm
;   ./bin/IntuitionEngine -z80 vga_text_sap_demo.ie80
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie80.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; ---------------------------------------------------------------------------
; VGA text mode memory access
; ---------------------------------------------------------------------------
; The text buffer at physical 0xB8000 is beyond the Z80's 64KB reach.
; Bank 0x2E maps the 0x8000 window to this address.
.set TEXT_BANK,0x2E
.set TEXT_WINDOW,0x8000

; ---------------------------------------------------------------------------
; Stack configuration
; ---------------------------------------------------------------------------
; Z80 stack grows downward; placed below I/O region to avoid conflicts.
.set STACK_TOP,0xEFF0

; ---------------------------------------------------------------------------
; SAP music data
; ---------------------------------------------------------------------------
; Length calculated automatically from label difference.
.set SAP_DATA_LEN,sap_data_end-sap_data

; ---------------------------------------------------------------------------
; SAP player I/O registers (Z80 addresses)
; ---------------------------------------------------------------------------
; Memory-mapped registers in the 0xFD00 range.
; Z80 address 0xFDxx maps to physical 0xF0Dxx via I/O translation.
.set Z80_SAP_PTR_0,0xFD10
.set Z80_SAP_PTR_1,0xFD11
.set Z80_SAP_PTR_2,0xFD12
.set Z80_SAP_PTR_3,0xFD13
.set Z80_SAP_LEN_0,0xFD14
.set Z80_SAP_LEN_1,0xFD15
.set Z80_SAP_LEN_2,0xFD16
.set Z80_SAP_LEN_3,0xFD17
.set Z80_SAP_CTRL,0xFD18
.set Z80_POKEY_PLUS,0xFD09

; ============================================================================
; ENTRY POINT
; ============================================================================
; Initialise all hardware subsystems, draw static screen elements, then
; enter the main animation loop.
; ============================================================================
    .org 0x0000

start:
    di

    ld sp,STACK_TOP

    ; --- Configure VGA hardware ---
    ; Mode 03h is the classic 80x25 colour text mode, standard since the
    ; IBM CGA (1981).
    ld a,VGA_CTRL_ENABLE
    out (VGA_PORT_CTRL),a
    ld a,VGA_MODE_TEXT
    out (VGA_PORT_MODE),a

    ; --- Set up VRAM bank for text buffer access ---
    ld a,TEXT_BANK
    ld (VRAM_BANK_REG),a

    ; --- Start SAP music playback ---
    call init_sap

    ; --- Draw static screen elements ---
    call clear_screen
    call draw_logo
    call draw_info

    ; --- Initialise animation state ---
    ; XOR A is the fastest way to zero A on Z80 (1 byte, 4 T-states).
    xor a
    ld (frame_lo),a
    ld (frame_hi),a
    ld (scroll_pos),a
    ld (scroll_pos+1),a
    ld (scroll_wait),a
    ld (scroll_cnt),a

; ============================================================================
; MAIN LOOP (60 FPS)
; ============================================================================
; Executes once per video frame.  Order of operations:
;   1. Wait for vsync - prevents screen tearing
;   2. Increment frame counter - drives all animation timing
;   3. Raster bars - animated background colours per row
;   4. Logo colours - cycling foreground through bright palette
;   5. Bouncing text - repositions info lines using sine table
;   6. Scroller - horizontal scrolling message at bottom
; ============================================================================
main_loop:
    call wait_vsync

    ; --- Increment 16-bit frame counter ---
    ; Various effects use different bits for timing:
    ;   frame_lo bits 0-2: fast effects (~8 changes/second)
    ;   frame_lo bits 3-7: slow effects (logo colour, scroll)
    ld hl,(frame_lo)
    inc hl
    ld (frame_lo),hl

    ; --- Execute visual effects ---
    call do_raster_bars
    call do_logo_colors
    call do_bounce_text
    call do_scroller

    jp main_loop

; ============================================================================
; SAP MUSIC INITIALISATION
; ============================================================================
; Sets up the POKEY+ audio engine and begins SAP file playback.
;
; SAP files contain a text header with metadata followed by a 0xFF 0xFF
; marker and binary data containing 6502 machine code and music data.
; The Intuition Engine's SAP player executes this code, converting POKEY
; register writes to audio samples.
;
; POKEY+ mode adds enhanced audio quality (better filtering, interpolation)
; while maintaining full compatibility with original SAP files.
;
; Control register bits:
;   Bit 0: Start playback
;   Bit 1: Stop playback
;   Bit 2: Loop mode (repeat when song ends)
; We use 0x05 = Start + Loop.
; ============================================================================
init_sap:
    ; --- Enable POKEY+ enhanced mode ---
    ld a,1
    ld (Z80_POKEY_PLUS),a

    ; --- Set 32-bit SAP data pointer ---
    ; Z80 addresses are 16-bit, so upper two bytes are always 0x00.
    ld a,sap_data & 0xFF
    ld (Z80_SAP_PTR_0),a
    ld a,(sap_data >> 8) & 0xFF
    ld (Z80_SAP_PTR_1),a
    ld a,(sap_data >> 16) & 0xFF
    ld (Z80_SAP_PTR_2),a
    ld a,(sap_data >> 24) & 0xFF
    ld (Z80_SAP_PTR_3),a

    ; --- Set 32-bit SAP data length ---
    ld a,SAP_DATA_LEN & 0xFF
    ld (Z80_SAP_LEN_0),a
    ld a,(SAP_DATA_LEN >> 8) & 0xFF
    ld (Z80_SAP_LEN_1),a
    ld a,(SAP_DATA_LEN >> 16) & 0xFF
    ld (Z80_SAP_LEN_2),a
    ld a,(SAP_DATA_LEN >> 24) & 0xFF
    ld (Z80_SAP_LEN_3),a

    ; --- Start playback with looping ---
    ld a,0x05
    ld (Z80_SAP_CTRL),a
    ret

; ============================================================================
; VSYNC SYNCHRONISATION
; ============================================================================
; Two-stage wait to catch the START of the vertical blanking interval:
;   1. Wait while vsync is active (in case we caught the tail end of one)
;   2. Wait until vsync becomes active (the start of the new interval)
; This maximises the time available for drawing before the next scanout.
;
; === WHY VSYNC MATTERS ===
; The VGA display is drawn line by line, top to bottom, 60 times per second.
; Modifying the text buffer mid-scanout causes "tearing" (part old frame,
; part new frame visible simultaneously).  By waiting for vertical blank,
; all our updates happen "between frames" and appear instantaneous.
; ============================================================================
wait_vsync:
vs_wait_end:
    in a,(VGA_PORT_STATUS)
    and VGA_STATUS_VSYNC
    jr nz,vs_wait_end

vs_wait_start:
    in a,(VGA_PORT_STATUS)
    and VGA_STATUS_VSYNC
    jr z,vs_wait_start
    ret

; ============================================================================
; CLEAR SCREEN
; ============================================================================
; Fills all 2000 character cells with spaces and black-on-black attributes,
; creating a blank canvas for the demo effects.
; ============================================================================
clear_screen:
    ld hl,TEXT_WINDOW
    ld bc,80*25
clr_loop:
    ld (hl),' '
    inc hl
    ld (hl),0x00
    inc hl
    dec bc
    ld a,b
    or c
    jr nz,clr_loop
    ret

; ============================================================================
; DRAW LOGO
; ============================================================================
; Draws the "INTUITION" ASCII art logo across the top 9 rows of the screen.
;
; The logo uses CP437 extended characters for gradient shading:
;   219 (0xDB) = Full block    (solid)
;   178 (0xB2) = Dark shade    (75% fill)
;   177 (0xB1) = Medium shade  (50% fill)
;   176 (0xB0) = Light shade   (25% fill)
;
; These characters were invented for IBM's original PC character set (1981)
; and became icons of DOS-era computing, used in everything from Norton
; Commander to ASCII art to demoscene productions.
;
; Logo dimensions: 72 characters wide x 9 rows, centred at column 4.
; ============================================================================
draw_logo:
    ld hl,TEXT_WINDOW + 0*160 + 4*2
    ld de,logo_data
    ld b,9
logo_row:
    push bc
    push hl
    ld b,72
logo_chr:
    ld a,(de)
    ld (hl),a
    inc hl
    ld a,0x0F
    ld (hl),a
    inc hl
    inc de
    djnz logo_chr
    pop hl
    ld bc,160
    add hl,bc
    pop bc
    djnz logo_row
    ret

; ============================================================================
; DRAW INFO TEXT
; ============================================================================
; Draws four lines of static information text.  These will be animated
; (bounced) by do_bounce_text each frame.
;
; Colours chosen for visual hierarchy and classic demoscene palette:
;   White (0x0F)   - Music credit (neutral, informational)
;   Cyan (0x0B)    - System info (technical, cool)
;   Yellow (0x0E)  - Demo title (prominent, warm)
;   Magenta (0x0D) - Greetings (friendly, scene tradition)
; ============================================================================
draw_info:
    ld hl,TEXT_WINDOW + 11*160 + 16*2
    ld de,str_music
    ld c,0x0F
    call print_str

    ld hl,TEXT_WINDOW + 13*160 + 20*2
    ld de,str_system
    ld c,0x0B
    call print_str

    ld hl,TEXT_WINDOW + 15*160 + 22*2
    ld de,str_demo
    ld c,0x0E
    call print_str

    ld hl,TEXT_WINDOW + 18*160 + 24*2
    ld de,str_greets
    ld c,0x0D
    call print_str
    ret

; ============================================================================
; PRINT NULL-TERMINATED STRING
; ============================================================================
; Input: HL = screen position, DE = string address, C = attribute byte
; ============================================================================
print_str:
    ld a,(de)
    or a
    ret z
    ld (hl),a
    inc hl
    ld (hl),c
    inc hl
    inc de
    jr print_str

; ============================================================================
; EFFECT 1: RASTER BARS
; ============================================================================
; Creates animated horizontal colour bands sweeping across all 25 rows.
;
; === WHY RASTER BARS ===
; "Raster bars" were a signature effect on the Amiga and C64, where hardware
; could change palette colours mid-scanline.  By altering the background
; colour at precise moments during the CRT beam's scan, coders created
; gradients impossible with static palettes.  In text mode, we simulate this
; by changing each row's background colour -- less fine-grained than true
; raster effects, but capturing the same aesthetic.
;
; For each row, we calculate a background colour from the sine table:
;   sine_table[(row * 8 + frame_counter) & 0xFF] -> masked to 0-7,
;   shifted to the upper nibble (background position in the attribute byte).
; The frame_counter offset creates the animation -- colours appear to
; wave downward as the sine pattern scrolls through the rows.
; ============================================================================
do_raster_bars:
    ld a,(frame_lo)
    ld c,a

    ld hl,TEXT_WINDOW + 1
    ld b,25

raster_row:
    push bc
    push hl

    ; --- Calculate row colour from sine table ---
    ld a,25
    sub b
    sla a
    sla a
    sla a
    add a,c

    ld e,a
    ld d,0
    push hl
    ld hl,sine_table
    add hl,de
    ld a,(hl)
    pop hl

    ; --- Convert to background colour (upper nibble, darker range 0-7) ---
    and 0x07
    sla a
    sla a
    sla a
    sla a
    ld d,a

    ; --- Apply to all 80 columns, preserving foreground colours ---
    ld b,80
raster_col:
    ld a,(hl)
    and 0x0F
    or d
    ld (hl),a
    inc hl
    inc hl
    djnz raster_col

    ; --- Advance to next row ---
    pop hl
    push de
    ld de,160
    add hl,de
    pop de
    pop bc
    djnz raster_row
    ret

; ============================================================================
; EFFECT 2: LOGO COLOUR CYCLING
; ============================================================================
; Cycles the logo's foreground through bright colours (indices 8-15).
;
; === WHY COLOUR CYCLING ===
; Before GPUs could display millions of colours, palette cycling was THE way
; to create animation without redrawing pixels.  By rotating palette indices,
; entire screens could appear to animate -- waterfalls flowing, fire burning,
; plasma pulsing -- all without touching the actual pixel data.  This
; technique was essential in games like Monkey Island and demos that needed
; to animate complex patterns within CPU constraints.
;
; We divide the frame counter by 8 for slower cycling (~7.5 changes/second).
; Dark grey (index 8) is replaced with white for visibility.  The colour is
; applied to all 72x9 characters of the logo while preserving the raster
; bar background colours set by the previous effect.
; ============================================================================
do_logo_colors:
    ; --- Calculate cycling colour (bright range 8-15) ---
    ld a,(frame_lo)
    srl a
    srl a
    srl a
    and 0x0F
    or 0x08
    cp 0x08
    jr nz,lc_color_ok
    ld a,0x0F
lc_color_ok:
    ld c,a

    ; --- Apply to logo area (rows 0-8, columns 4-75) ---
    ld hl,TEXT_WINDOW + 0*160 + 4*2 + 1
    ld b,9

lc_row:
    push bc
    push hl

    ld b,72
lc_char:
    ld a,(hl)
    and 0xF0
    or c
    ld (hl),a
    inc hl
    inc hl
    djnz lc_char

    pop hl
    push de
    ld de,160
    add hl,de
    pop de
    pop bc
    djnz lc_row
    ret

; ============================================================================
; EFFECT 3: BOUNCING TEXT
; ============================================================================
; Makes the four info text lines bounce left and right independently using
; sine wave animation.
;
; === WHY SINE TABLE ANIMATION ===
; Computing sine in real-time requires floating-point maths or expensive
; fixed-point approximations.  On 8-bit CPUs, a simple table lookup (one
; indexed memory read) is far faster than any calculation.  By indexing
; into the table with (frame_counter + phase_offset), each line gets a
; different position in the wave, creating organic, flowing motion where
; lines move independently but harmoniously.
;
; Phase offsets: 0, 64, 128, 192 (quarter-wave increments)
; This creates a "wave" effect rippling down through the lines.
; ============================================================================
do_bounce_text:
    ; --- Line 1: Music credit (phase 0) ---
    ld a,(frame_lo)
    call get_bounce_offset
    ld hl,TEXT_WINDOW + 11*160
    call clear_text_row
    ld a,(bounce_off)
    add a,12
    ld b,a
    ld hl,TEXT_WINDOW + 11*160
    call set_text_col
    ld de,str_music
    ld c,0x0F
    call print_str

    ; --- Line 2: System info (phase 64 = 1/4 wave) ---
    ld a,(frame_lo)
    add a,64
    call get_bounce_offset
    ld hl,TEXT_WINDOW + 13*160
    call clear_text_row
    ld a,(bounce_off)
    add a,17
    ld b,a
    ld hl,TEXT_WINDOW + 13*160
    call set_text_col
    ld de,str_system
    ld c,0x0B
    call print_str

    ; --- Line 3: Demo title (phase 128 = 1/2 wave, opposite phase) ---
    ld a,(frame_lo)
    add a,128
    call get_bounce_offset
    ld hl,TEXT_WINDOW + 15*160
    call clear_text_row
    ld a,(bounce_off)
    add a,21
    ld b,a
    ld hl,TEXT_WINDOW + 15*160
    call set_text_col
    ld de,str_demo
    ld c,0x0E
    call print_str

    ; --- Line 4: Greetings (phase 192 = 3/4 wave) ---
    ld a,(frame_lo)
    add a,192
    call get_bounce_offset
    ld hl,TEXT_WINDOW + 18*160
    call clear_text_row
    ld a,(bounce_off)
    add a,21
    ld b,a
    ld hl,TEXT_WINDOW + 18*160
    call set_text_col
    ld de,str_greets
    ld c,0x0D
    call print_str
    ret

; ============================================================================
; GET BOUNCE OFFSET FROM SINE TABLE
; ============================================================================
; Input: A = index into sine table (0-255)
; Output: (bounce_off) = sine value (0-15)
; ============================================================================
get_bounce_offset:
    ld e,a
    ld d,0
    ld hl,sine_table
    add hl,de
    ld a,(hl)
    ld (bounce_off),a
    ret

; ============================================================================
; CLEAR TEXT ROW
; ============================================================================
; Fills a row with space characters while preserving attribute backgrounds
; (so raster bar colours show through even when text is erased).
;
; Input: HL = start of row in text buffer
; ============================================================================
clear_text_row:
    ld b,80
ctr_loop:
    ld (hl),' '
    inc hl
    inc hl
    djnz ctr_loop
    ret

; ============================================================================
; SET TEXT COLUMN
; ============================================================================
; Adjusts HL to point to the specified column within a row.
;
; Input: HL = start of row, B = column number (0-79)
; Output: HL = position of character at column B
; ============================================================================
set_text_col:
    ld a,b
    add a,a
    ld e,a
    ld d,0
    add hl,de
    ret

; ============================================================================
; EFFECT 4: SCROLLING TEXT
; ============================================================================
; Scrolls a message horizontally across the bottom of the screen.
;
; === WHY SCROLLERS ===
; The horizontal scroller is perhaps THE most iconic demoscene effect.
; From the earliest C64 demos to modern PC productions, scrollers conveyed
; messages, credits, greetings ("greets"), and manifestos.  They were often
; enhanced with wave motion, zooming, or rotation.  Even in 2026, demos
; include scrollers as a nod to tradition.
;
; Scroll speed: 8 frames per character advance at 60 FPS = ~7.5 chars/second.
; The message wraps seamlessly from end to start for continuous looping.
; ============================================================================
do_scroller:
    ; --- Check scroll timing (advance every 8 frames) ---
    ld a,(scroll_wait)
    inc a
    ld (scroll_wait),a
    cp 8
    jr c,sc_no_advance

    ; --- Advance scroll position ---
    xor a
    ld (scroll_wait),a

    ld hl,(scroll_pos)
    inc hl

    ; --- Wrap at end of message ---
    ld de,scroll_end - scroll_msg
    or a
    sbc hl,de
    jr c,sc_no_wrap
    ld hl,0
    jr sc_save_pos
sc_no_wrap:
    add hl,de
sc_save_pos:
    ld (scroll_pos),hl

sc_no_advance:
    ; --- Draw 80 visible characters starting from scroll_pos ---
    ld hl,(scroll_pos)
    ld de,scroll_msg
    add hl,de
    ex de,hl

    ld hl,TEXT_WINDOW + 23*160
    ld b,80

sc_draw:
    ld a,(de)
    or a
    jr nz,sc_not_end
    ; Wrap to message start on null terminator
    push hl
    ld hl,scroll_msg
    ex de,hl
    ld a,(de)
    pop hl
sc_not_end:
    ld (hl),a
    inc hl
    ld a,0x0F
    ld (hl),a
    inc hl

    inc de
    djnz sc_draw
    ret

; ============================================================================
; VARIABLES
; ============================================================================
; Runtime state for animation and effects.
; ============================================================================

frame_lo:       .byte 0
frame_hi:       .byte 0
scroll_pos:     .word 0
scroll_wait:    .byte 0
temp_row:       .byte 0
temp_var:       .byte 0
scroll_cnt:     .byte 0
bounce_off:     .byte 0

; ============================================================================
; SINE TABLE
; ============================================================================
; 256-entry lookup table for smooth sine wave animation, scaled to 0-15.
;
; === WHY LOOKUP TABLES ===
; Computing sine in real-time requires floating-point maths or expensive
; fixed-point approximations.  On 8-bit CPUs like the Z80, a simple table
; lookup (one indexed memory read) is far faster than any calculation.
; This was the standard approach in virtually all 8-bit and 16-bit era
; demos and games.
;
; Table generation (pseudocode):
;   for i in 0..255:
;       table[i] = round(sin(i * 2 * PI / 256) * 7.5 + 7.5)
; ============================================================================
    .org 0xC100

sine_table:
    .byte  8, 8, 8, 9, 9, 9, 9,10,10,10,10,11,11,11,11,12
    .byte 12,12,12,12,13,13,13,13,13,14,14,14,14,14,14,14
    .byte 15,15,15,15,15,15,15,15,15,15,15,15,15,15,15,14
    .byte 14,14,14,14,14,14,13,13,13,13,13,12,12,12,12,12
    .byte 11,11,11,11,10,10,10,10, 9, 9, 9, 9, 8, 8, 8, 8
    .byte  7, 7, 7, 6, 6, 6, 6, 5, 5, 5, 5, 4, 4, 4, 4, 3
    .byte  3, 3, 3, 3, 2, 2, 2, 2, 2, 1, 1, 1, 1, 1, 1, 1
    .byte  0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
    .byte  1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3
    .byte  4, 4, 4, 4, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7
    .byte  8, 8, 8, 9, 9, 9, 9,10,10,10,10,11,11,11,11,12
    .byte 12,12,12,12,13,13,13,13,13,14,14,14,14,14,14,14
    .byte 15,15,15,15,15,15,15,15,15,15,15,15,15,15,15,14
    .byte 14,14,14,14,14,14,13,13,13,13,13,12,12,12,12,12
    .byte 11,11,11,11,10,10,10,10, 9, 9, 9, 9, 8, 8, 8, 8
    .byte  7, 7, 7, 6, 6, 6, 6, 5, 5, 5, 5, 4, 4, 4, 4, 3

; ============================================================================
; STRING DATA
; ============================================================================

; --- Logo data (72 chars x 9 rows) ---
; Uses CP437 extended characters for gradient shading:
;   219 (0xDB) = Full block, 178 (0xB2) = Dark shade
;   177 (0xB1) = Medium shade, 176 (0xB0) = Light shade
;   223 (0xDF) = Upper half, 220 (0xDC) = Lower half, 222 (0xDE) = Right half
logo_data:
    .byte 32,219,219,178,32,219,219,219,220,32,32,32,32,219,32,220,220,220,219,219,219,219,219,178,32,219,32,32,32,32,219,219,32,32,219,219,178,220,220,220,219,219,219,219,219,178,32,219,219,178,32,177,219,219,219,219,219,32,32,32,219,219,219,220,32,32,32,32,219,32,32,32
    .byte 178,219,219,177,32,219,219,32,223,219,32,32,32,219,32,178,32,32,219,219,177,32,178,177,32,219,219,32,32,178,219,219,177,178,219,219,177,178,32,32,219,219,177,32,178,177,178,219,219,177,177,219,219,177,32,32,219,219,177,32,219,219,32,223,219,32,32,32,219,32,32,32
    .byte 177,219,219,177,178,219,219,32,32,223,219,32,219,219,177,177,32,178,219,219,176,32,177,176,178,219,219,32,32,177,219,219,176,177,219,219,177,177,32,178,219,219,176,32,177,176,177,219,219,177,177,219,219,176,32,32,219,219,177,178,219,219,32,32,223,219,32,219,219,177,32,32
    .byte 176,219,219,176,178,219,219,177,32,32,222,32,219,219,177,176,32,178,219,219,178,32,176,32,178,178,219,32,32,176,219,219,176,176,219,219,176,176,32,178,219,219,178,32,176,32,176,219,219,176,177,219,219,32,32,32,219,219,176,178,219,219,177,32,32,222,32,219,219,177,32,32
    .byte 176,219,219,176,177,219,219,176,32,32,32,178,219,219,176,32,32,177,219,219,177,32,176,32,177,177,219,219,219,219,219,178,32,176,219,219,176,32,32,177,219,219,177,32,176,32,176,219,219,176,176,32,219,219,219,219,178,177,176,177,219,219,176,32,32,32,178,219,219,176,32,32
    .byte 176,178,32,32,176,32,177,176,32,32,32,177,32,177,32,32,32,177,32,176,176,32,32,32,176,177,178,177,32,177,32,177,32,176,178,32,32,32,32,177,32,176,176,32,32,32,176,178,32,32,176,32,177,176,177,176,177,176,32,176,32,177,176,32,32,32,177,32,177,32,32,32
    .byte 32,177,32,176,176,32,176,176,32,32,32,176,32,177,176,32,32,32,32,176,32,32,32,32,176,176,177,176,32,176,32,176,32,32,177,32,176,32,32,32,32,176,32,32,32,32,32,177,32,176,32,32,176,32,177,32,177,176,32,176,32,176,176,32,32,32,176,32,177,176,32,32
    .byte 32,177,32,176,32,32,32,176,32,32,32,176,32,176,32,32,32,176,32,32,32,32,32,32,32,176,176,176,32,176,32,176,32,32,177,32,176,32,32,176,32,32,32,32,32,32,32,177,32,176,176,32,176,32,176,32,177,32,32,32,32,32,176,32,32,32,176,32,176,32,32,32
    .byte 32,176,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32,32,176,32,32,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32,32,176,32,176,32,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32

; --- Info text strings ---
str_music:
    .ascii "MUSIC: WOOBOODOO BY KWIATEK & SYCHOWICZ",0
str_system:
    .ascii "Z80 CODE + VGA MODE 13H + ATARI POKEY AUDIO",0
str_demo:
    .ascii "INTUITION ENGINE 2026",0
str_greets:
    .ascii "GREETS TO THE SCENE!",0

; --- Scroll message ---
; Padded with spaces at start and end for clean looping, creating a pause
; before the message begins and after it ends.
scroll_msg:
    .ascii "                                        "
    .ascii "WELCOME TO THE INTUITION ENGINE...   "
    .ascii "FEATURING IE32 RISC, Z80, 6502, AND 68020 CPU EMULATION   "
    .ascii "WITH VGA GRAPHICS AND PSG (AY/YM), SID, POKEY, TED SOUND CHIPS!   "
    .ascii "KEEP THE DEMOSCENE SPIRIT ALIVE!!!   "
    .ascii "VISIT WWW.INTUITIONSUBSYNTH.COM   "
    .ascii "                                        "
scroll_end:

; ============================================================================
; EMBEDDED SAP MUSIC DATA
; ============================================================================
; WooBooDoo by Grzegorz Kwiatek & Lukasz Sychowicz
;
; SAP files begin with a text header (AUTHOR, NAME, TYPE, etc.) followed
; by a 0xFF 0xFF marker and binary data containing 6502 machine code for
; the player routine and music data (patterns, instruments, sequences).
; ============================================================================
    .org 0xE000

sap_data:
    .incbin "../assets/music/WooBooDoo.sap"
sap_data_end:

; ============================================================================
; END OF DEMO
; ============================================================================
