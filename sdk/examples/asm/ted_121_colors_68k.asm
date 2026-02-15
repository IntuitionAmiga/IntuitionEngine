; ============================================================================
; TED 121-COLOUR PLASMA DEMO - FULL PALETTE EXPLORATION
; M68K Assembly for IntuitionEngine - TED Video + PSG+ Audio
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Motorola 68020
; Video Chip:    TED (Commodore Plus/4 - 121 colours)
; Audio Engine:  PSG+ (AY-3-8910 compatible, enhanced mode)
; Assembler:     vasmm68k_mot (VASM M68K, Motorola syntax)
; Build:         vasmm68k_mot -Fbin -m68020 -o ted_121_colors_68k.ie68 ted_121_colors_68k.asm
; Run:           bin/IntuitionEngine -m68k ted_121_colors_68k.ie68
; Porting:       TED/PSG MMIO is CPU-agnostic. Port effort: rewrite sine
;                tables and loop structures for target CPU. 8-bit CPUs will
;                need optimised inner loops to maintain 60fps plasma update.
;
; === WHAT THIS DEMO DOES ===
; 1. Displays a full-screen animated plasma effect using all 121 TED colours
; 2. Renders a rainbow-coloured title bar with cycling colours
; 3. Shows a smooth pixel-scrolling message with hardware XSCROLL
; 4. Cycles the border through all 121 unique TED colours
; 5. Plays ZX Spectrum AY tracker music through the PSG+ audio subsystem
;
; === WHY THE TED CHIP AND ITS 121-COLOUR PALETTE ===
;
; The Commodore Plus/4 (1984) used the TED (Text Editing Device) chip, which
; combined video, sound, and I/O in a single IC. While often overshadowed by
; the C64's SID and VIC-II, the TED had one standout feature: 121 colours.
;
; Unlike the C64's 16 fixed colours, TED offered 16 hues x 8 luminance
; levels, giving artists unprecedented colour choice for an 8-bit machine.
; However, the Plus/4's 1.76 MHz 6502 CPU was too slow to exploit this
; potential for complex real-time effects.
;
; TED colours are encoded in a single byte:
;
;   Bits 7-4: Luminance (0-7, only 3 bits used, bit 7 ignored)
;   Bits 3-0: Hue (0-15)
;
;   Colour byte = (luminance << 4) | hue
;
; The 16 hues are:
;    0: Black (special - ignores luminance, always black)
;    1: White          5: Green          9: Brown         13: Light Blue
;    2: Red            6: Blue          10: Yellow-Green  14: Dark Blue
;    3: Cyan           7: Yellow        11: Pink          15: Light Green
;    4: Purple         8: Orange        12: Blue-Green
;
; Since hue 0 is always black regardless of luminance, we get:
;   1 black + (15 hues x 8 luminances) = 1 + 120 = 121 unique colours
;
; This demo showcases what COULD have been possible with TED if paired with
; a more powerful CPU. The M68020 updates all 1000 colour cells every frame
; at 60 FPS - something absolutely impossible on the original Plus/4.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +-------------------------------------------------------------+
;   |                    MAIN LOOP (60 FPS)                       |
;   |                                                             |
;   |  WAIT VBLANK -> UPDATE TIMERS -> RENDER PLASMA              |
;   |       -> UPDATE BORDER -> DRAW TITLE -> UPDATE SCROLLER     |
;   +-------------------------------------------------------------+
;
;   +-------------------------------------------------------------+
;   |              PSG+ AUDIO ENGINE (runs in parallel)           |
;   |  The AY chip was used in the ZX Spectrum 128K and Atari ST  |
;   |  - this creates an impossible hybrid: TED video + AY audio! |
;   +-------------------------------------------------------------+
;
; === MEMORY MAP ===
;   0x001000-0x1013   Animation variable storage (frame_count, timers, etc.)
;   PROGRAM_START     Programme code (after variables)
;   0x0F3000          TED character RAM (1000 bytes)
;   0x0F3400          TED colour RAM (1000 bytes)
;   0x0F0F20          TED video control registers
;   0x0F0C00          PSG audio registers
;
; === BUILD AND RUN ===
;   vasmm68k_mot -Fbin -m68020 -o ted_121_colors_68k.ie68 ted_121_colors_68k.asm
;   bin/IntuitionEngine -m68k ted_121_colors_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- TED Video RAM addresses ---
; The TED uses a simple linear framebuffer for text mode:
;   - Character RAM: 1000 bytes (40 columns x 25 rows)
;   - Colour RAM: 1000 bytes (one colour byte per character cell)
TED_VRAM_BASE    equ $F3000
TED_COLOR_RAM    equ $F3400

; --- Display geometry ---
; TED text mode is 40x25 characters, each 8x8 pixels.
; Total display: 320x200 pixels (same as C64, Amiga OCS, VGA Mode 13h).
SCREEN_COLS      equ 40
SCREEN_ROWS      equ 25
TOTAL_CELLS      equ 1000              ; 40 x 25

; --- Plasma character ---
; We fill the screen with a solid block character and animate its COLOUR,
; not its shape. Character $80 in the TED character set is a solid 8x8 block.
SOLID_BLOCK      equ $80

; --- Animation variables ---
; Stored in RAM at fixed addresses. The plasma effect uses four independent
; time counters advancing at different rates, creating complex non-repeating
; patterns characteristic of good plasma effects.
frame_count      equ $1000
plasma_time1     equ $1004             ; Horizontal wave phase
plasma_time2     equ $1008             ; Vertical wave phase
plasma_time3     equ $100C             ; Diagonal wave phase
plasma_time4     equ $1010             ; Radial wave phase (Manhattan distance)
scroll_fine      equ $1014             ; Fine scroll position (0-7)
scroll_char      equ $1018             ; Character index into scroll message

    org PROGRAM_START

; ============================================================================
; ENTRY POINT
; ============================================================================

start:
    ; --- Initialise stack pointer ---
    ; The M68K uses a descending stack. STACK_TOP is defined in ie68.inc.
    move.l  #STACK_TOP,sp

    ; --- Enable TED video output ---
    move.b  #TED_V_ENABLE_VIDEO,TED_V_ENABLE

    ; --- Configure TED for 40x25 text mode ---
    ; CTRL1: DEN=1 (display enable), RSEL=1 (25 rows), YSCROLL=0
    move.b  #$18,TED_V_CTRL1
    ; CTRL2: MCM=0 (no multicolour), CSEL=1 (40 columns), XSCROLL=0
    move.b  #$08,TED_V_CTRL2

    ; --- Set character and video base addresses ---
    move.b  #$00,TED_V_CHAR_BASE
    move.b  #$00,TED_V_VIDEO_BASE

    ; --- Set initial colours ---
    move.b  #$00,TED_V_BG_COLOR0   ; Black background
    move.b  #$71,TED_V_BORDER      ; White border

    ; --- Clear animation variables ---
    clr.l   frame_count
    clr.l   plasma_time1
    clr.l   plasma_time2
    clr.l   plasma_time3
    clr.l   plasma_time4
    clr.l   scroll_fine
    clr.l   scroll_char

    ; --- Fill screen with solid block characters ---
    bsr     init_screen

    ; --- Enable the Intuition Engine video compositor ---
    move.l  #1,VIDEO_CTRL

    ; --- Start AY music playback ---
    ; PSG_PLUS_CTRL = 1 enables enhanced PSG mode.
    ; PSG_PLAY_CTRL = 5 means: start (bit 0) + loop (bit 2).
    move.b  #1,PSG_PLUS_CTRL
    move.l  #music_data,PSG_PLAY_PTR
    move.l  #music_data_end-music_data,PSG_PLAY_LEN
    move.l  #5,PSG_PLAY_CTRL

; ============================================================================
; MAIN LOOP - one iteration per frame (60 FPS with vsync)
; ============================================================================
; Order of operations:
;   1. Wait for vblank (prevents tearing)
;   2. Advance plasma timer phases (different prime-ish rates)
;   3. Render plasma colours to all 1000 cells
;   4. Cycle the border colour through all 121 unique TED colours
;   5. Draw the rainbow title text over the plasma
;   6. Advance the hardware-scrolled text message
;   7. Increment frame counter and loop

main_loop:
    bsr     wait_vblank

    ; --- Advance plasma timers at different rates ---
    ; Using different speeds creates complex interference patterns.
    move.l  plasma_time1,d0
    addq.l  #3,d0                   ; +3: horizontal wave influence
    move.l  d0,plasma_time1

    move.l  plasma_time2,d0
    addq.l  #2,d0                   ; +2: vertical wave influence
    move.l  d0,plasma_time2

    move.l  plasma_time3,d0
    addq.l  #5,d0                   ; +5: diagonal wave influence
    move.l  d0,plasma_time3

    move.l  plasma_time4,d0
    addq.l  #1,d0                   ; +1: radial wave influence
    move.l  d0,plasma_time4

    bsr     render_plasma
    bsr     update_border
    bsr     draw_title_overlay
    bsr     update_scroller

    addq.l  #1,frame_count

    bra     main_loop

; ============================================================================
; WAIT FOR VERTICAL BLANK
; ============================================================================
; Two-phase poll: first wait while already in vblank (exit current period),
; then wait until vblank begins (catch the rising edge). This guarantees
; exactly one frame per main loop iteration.

wait_vblank:
.not_vb:
    move.b  TED_V_STATUS,d0
    andi.b  #TED_V_STATUS_VBLANK,d0
    bne.s   .not_vb

.wait_vb:
    move.b  TED_V_STATUS,d0
    andi.b  #TED_V_STATUS_VBLANK,d0
    beq.s   .wait_vb
    rts

; ============================================================================
; INITIALISE SCREEN WITH SOLID BLOCKS
; ============================================================================
; Fills all 1000 character cells with the solid block character ($80).
; The plasma effect animates COLOUR RAM, not character shapes. Writing
; 1000 colour bytes per frame is much faster than 8000 bitmap bytes.

init_screen:
    lea     TED_VRAM_BASE,a0
    move.w  #TOTAL_CELLS-1,d0       ; dbf needs N-1
.fill:
    move.b  #SOLID_BLOCK,(a0)+
    dbf     d0,.fill
    rts

; ============================================================================
; RENDER PLASMA EFFECT
; ============================================================================
; The heart of the demo. For each of the 1000 cells at position (x, y),
; four sine waves are combined:
;
;   v1 = sin(x*4 + time1)           Horizontal stripes (contributes to HUE)
;   v2 = sin(y*4 + time2)           Vertical stripes (contributes to HUE)
;   v3 = sin((x+y)*3 + time3)       Diagonal stripes (contributes to LUMINANCE)
;   v4 = sin(dist*2 + time4)        Radial rings (contributes to LUMINANCE)
;
; Hue and luminance use DIFFERENT wave pairs, ensuring they vary
; independently. This is what allows the full 121-colour palette to
; appear naturally in the plasma pattern.
;
; The radial wave uses Manhattan distance (|x-cx| + |y-cy|) instead of
; Euclidean distance to avoid expensive multiplication and square root.
;
; Register usage:
;   a0 = colour RAM pointer           d3 = X coordinate (0-39)
;   a1 = sine table base              d4 = cached time1
;   a2 = cached time4                 d5 = cached time2
;   a3 = luminance accumulator        d6 = cached time3
;   d0-d2 = working registers         d7 = Y coordinate (0-24)

render_plasma:
    lea     TED_COLOR_RAM,a0
    lea     sine_table,a1

    ; Cache time values in registers (register access is faster than memory)
    move.l  plasma_time1,d4
    move.l  plasma_time2,d5
    move.l  plasma_time3,d6
    move.l  plasma_time4,a2

    moveq   #0,d7                   ; Y = 0

.row_loop:
    moveq   #0,d3                   ; X = 0

.col_loop:
    ; --- WAVE 1: Horizontal wave -> HUE ---
    move.l  d3,d0
    lsl.l   #2,d0                   ; x * 4
    add.l   d4,d0                   ; + time1
    andi.l  #$FF,d0                 ; Wrap to 256-entry table
    move.b  (a1,d0.l),d1
    ext.w   d1
    ext.l   d1                      ; d1 = v1 (signed 32-bit)

    ; --- WAVE 2: Vertical wave -> HUE ---
    move.l  d7,d0
    lsl.l   #2,d0                   ; y * 4
    add.l   d5,d0                   ; + time2
    andi.l  #$FF,d0
    move.b  (a1,d0.l),d2
    ext.w   d2
    ext.l   d2
    add.l   d2,d1                   ; d1 = v1 + v2 (for HUE calculation)

    ; --- WAVE 3: Diagonal wave -> LUMINANCE ---
    move.l  d3,d0
    add.l   d7,d0                   ; x + y
    mulu.w  #3,d0                   ; (x+y) * 3 - hardware multiply!
    add.l   d6,d0                   ; + time3
    andi.l  #$FF,d0
    move.b  (a1,d0.l),d2
    ext.w   d2
    ext.l   d2
    move.l  d2,a3                   ; a3 = v3 (start of luminance sum)

    ; --- WAVE 4: Radial wave (Manhattan distance from centre) -> LUMINANCE ---
    ; |x - 20|
    move.l  d3,d0
    subi.l  #20,d0
    bpl.s   .pos_x
    neg.l   d0
.pos_x:
    ; |y - 12|
    move.l  d7,d2
    subi.l  #12,d2
    bpl.s   .pos_y
    neg.l   d2
.pos_y:
    add.l   d2,d0                   ; Manhattan distance
    lsl.l   #1,d0                   ; * 2
    move.l  a2,d2                   ; time4
    add.l   d2,d0                   ; + time4
    andi.l  #$FF,d0
    move.b  (a1,d0.l),d2
    ext.w   d2
    ext.l   d2
    add.l   a3,d2                   ; d2 = v3 + v4 (LUMINANCE sum)

    ; --- MAP TO TED COLOUR: independent hue and luminance ---
    ; HUE from v1+v2, LUMINANCE from v3+v4
    addi.l  #256,d1                 ; Shift hue sum to 0-512
    lsr.l   #5,d1                   ; / 32 -> 0-15
    andi.l  #$0F,d1
    move.l  d1,d0                   ; d0 = hue (4 bits)

    addi.l  #256,d2                 ; Shift luminance sum to 0-512
    lsr.l   #6,d2                   ; / 64 -> 0-7
    andi.l  #$07,d2

    ; Combine: (luminance << 4) | hue
    lsl.l   #4,d2
    or.l    d0,d2

    ; Avoid pure black (hue 0 always reads as black regardless of luminance)
    tst.b   d2
    bne.s   .not_black
    moveq   #$11,d2                 ; Dark white instead
.not_black:

    ; Write colour to colour RAM
    move.b  d2,(a0)+

    ; --- Next column ---
    addq.l  #1,d3
    cmpi.l  #SCREEN_COLS,d3
    blt     .col_loop

    ; --- Next row ---
    addq.l  #1,d7
    cmpi.l  #SCREEN_ROWS,d7
    blt     .row_loop

    rts

; ============================================================================
; UPDATE BORDER COLOUR
; ============================================================================
; Cycles through all 121 unique TED colours. Hue cycles 0-15 first,
; then luminance increments 0-7. Rate: one change every 8 frames,
; completing a full cycle in ~17 seconds at 60 FPS.

update_border:
    move.l  frame_count,d0
    lsr.l   #3,d0                   ; Change every 8 frames
    andi.l  #$7F,d0                 ; 0-127

    ; Extract hue (0-15) and luminance (0-7)
    move.l  d0,d1
    andi.l  #$0F,d1                 ; d1 = hue

    move.l  d0,d2
    lsr.l   #4,d2
    andi.l  #$07,d2                 ; d2 = luminance

    ; Combine into TED colour byte
    lsl.l   #4,d2
    or.l    d1,d2

    ; Handle duplicate black: hue 0 at any luminance is still black,
    ; so replace with white (hue 1) to avoid wasting display time
    tst.b   d1
    bne.s   .border_ok
    tst.b   d2
    beq.s   .border_ok              ; True black (lum 0, hue 0) is fine
    moveq   #$01,d1                 ; Replace hue 0 with white
    andi.l  #$70,d2
    or.l    d1,d2
.border_ok:
    move.b  d2,TED_V_BORDER
    rts

; ============================================================================
; DRAW TITLE OVERLAY
; ============================================================================
; Writes the demo title across row 0 and applies a rainbow colour effect.
; Each character receives a colour based on (frame_count + position),
; creating a wave of cycling rainbow hues across the title text.

draw_title_overlay:
    ; --- Write title characters to VRAM ---
    lea     TED_VRAM_BASE+3,a0      ; Start at column 3 (centred)
    lea     title_text,a1
.copy_title:
    move.b  (a1)+,d0
    beq.s   .title_done
    move.b  d0,(a0)+
    bra.s   .copy_title
.title_done:

    ; --- Apply rainbow colours ---
    lea     TED_COLOR_RAM+3,a0
    move.l  frame_count,d1
    moveq   #33,d0                  ; 34 characters (0-33)
.title_color:
    move.l  d1,d2
    add.l   d0,d2                   ; Offset by position
    lsr.l   #1,d2                   ; Slow down cycling
    andi.l  #$0F,d2                 ; Hue 0-15
    beq.s   .title_white            ; Avoid hue 0 (black)
    ori.l   #$70,d2                 ; Maximum luminance
    bra.s   .title_set
.title_white:
    moveq   #$71,d2                 ; Bright white
.title_set:
    move.b  d2,(a0)+
    dbf     d0,.title_color
    rts

; ============================================================================
; SMOOTH PIXEL SCROLLER
; ============================================================================
; Implements a classic demoscene horizontal scroller on row 24 using the
; TED's hardware XSCROLL register for sub-character pixel movement.
;
; The technique combines two levels of scrolling:
;   FINE SCROLL (XSCROLL 0-7): moves text by individual pixels
;   CHARACTER SCROLL: shifts all characters left when XSCROLL wraps
;
; Each frame: increment fine scroll. When it wraps past 7, shift all
; 40 characters on row 24 left by one and insert the next message
; character on the right.

update_scroller:
    movem.l d0-d3/a0-a2,-(sp)      ; Save registers

    ; --- Advance fine scroll ---
    move.l  scroll_fine,d0
    addq.l  #1,d0
    cmpi.l  #8,d0
    blt.s   .no_char_scroll

    ; --- Fine scroll wrapped: shift characters ---
    clr.l   d0                      ; Reset fine scroll to 0

    ; Shift row 24 left by one character (copy N+1 to N for 39 characters)
    lea     TED_VRAM_BASE+(24*40),a0
    lea     1(a0),a1
    moveq   #38,d1
.shift_loop:
    move.b  (a1)+,(a0)+
    dbf     d1,.shift_loop

    ; Insert next message character at the rightmost column
    move.l  scroll_char,d1
    lea     scroll_text,a1
    move.b  (a1,d1.l),d2
    bne.s   .have_char
    ; End of message: wrap to beginning
    clr.l   d1
    move.b  (a1),d2
.have_char:
    move.b  d2,TED_VRAM_BASE+(24*40)+39
    addq.l  #1,d1
    move.l  d1,scroll_char

.no_char_scroll:
    move.l  d0,scroll_fine

    ; --- Set TED XSCROLL register ---
    ; XSCROLL value is inverted: 0 = no shift, 7 = maximum shift
    move.l  #7,d1
    sub.l   d0,d1
    andi.l  #$07,d1
    ori.l   #$08,d1                 ; Keep CSEL bit set (40 columns)
    move.b  d1,TED_V_CTRL2

    ; --- Apply rainbow colours to scroller row ---
    lea     TED_COLOR_RAM+(24*40),a0
    move.l  frame_count,d1
    moveq   #39,d0                  ; 40 columns
.scroll_color:
    move.l  d1,d2
    add.l   d0,d2
    lsr.l   #1,d2
    andi.l  #$0F,d2
    beq.s   .scroll_white
    ori.l   #$70,d2                 ; Maximum luminance
    bra.s   .scroll_set
.scroll_white:
    moveq   #$71,d2                 ; Bright white
.scroll_set:
    move.b  d2,(a0)+
    dbf     d0,.scroll_color

    movem.l (sp)+,d0-d3/a0-a2      ; Restore registers
    rts

; ============================================================================
; DATA SECTION
; ============================================================================

; --- Title text (34 characters, centred on 40-column display) ---
title_text:
    dc.b    "=== TED PLASMA - 68020 POWER! ===",0

; --- Scroll message (null-terminated, wraps seamlessly) ---
scroll_text:
    dc.b    "       "
    dc.b    "WELCOME TO THE TED 121-COLOUR 68K PLASMA DEMO!   "
    dc.b    "THIS EFFECT WOULD BE IMPOSSIBLE ON THE ORIGINAL PLUS/4 OR C16...   "
    dc.b    "THE 1.76MHZ 6502 COULD NEVER UPDATE 1000 CELLS AT 60FPS!   "
    dc.b    "BUT THE MIGHTY 68020 DOES IT WITH EASE...   "
    dc.b    "DUE TO HARDWARE MULTIPLY (MULU) AND 32-BIT REGISTERS...   "
    dc.b    "SCROLLTEXT ROUTINE USES TED HARDWARE SCROLLING (XSCROLL)...   "
    dc.b    "THE BORDER CYCLES THROUGH ALL 121 COLOURS TOO...   "
    dc.b    "GREETINGS TO ALL DEMOSCENERS!   "
    dc.b    "MUSIC: PLATOON BY ROB HUBBARD (AY VERSION)...   "
    dc.b    "ASM CODE: INTUITION 2026...   "
    dc.b    "VISIT INTUITIONSUBSYNTH.COM   "
    dc.b    "                                        ",0

    even

; ============================================================================
; SINE TABLE (256 entries, signed 8-bit values: -127 to +127)
; ============================================================================
; Pre-computed sine wave for fast lookup. 256 entries cover one full cycle.
; Using a table avoids expensive Taylor series or CORDIC calculations.
; Formula: value = round(sin(i * 2*pi / 256) * 127)

sine_table:
    ; Quadrant 1: 0 to 90 degrees (indices 0-63, values rise 0 to 127)
    dc.b      0,   3,   6,   9,  12,  15,  18,  21
    dc.b     24,  27,  30,  33,  36,  39,  42,  45
    dc.b     48,  51,  54,  57,  59,  62,  65,  67
    dc.b     70,  73,  75,  78,  80,  82,  85,  87
    dc.b     89,  91,  94,  96,  98, 100, 102, 103
    dc.b    105, 107, 108, 110, 112, 113, 114, 116
    dc.b    117, 118, 119, 120, 121, 122, 123, 123
    dc.b    124, 125, 125, 126, 126, 126, 126, 126

    ; Quadrant 2: 90 to 180 degrees (indices 64-127, values fall 127 to 0)
    dc.b    127, 126, 126, 126, 126, 126, 125, 125
    dc.b    124, 123, 123, 122, 121, 120, 119, 118
    dc.b    117, 116, 114, 113, 112, 110, 108, 107
    dc.b    105, 103, 102, 100,  98,  96,  94,  91
    dc.b     89,  87,  85,  82,  80,  78,  75,  73
    dc.b     70,  67,  65,  62,  59,  57,  54,  51
    dc.b     48,  45,  42,  39,  36,  33,  30,  27
    dc.b     24,  21,  18,  15,  12,   9,   6,   3

    ; Quadrant 3: 180 to 270 degrees (indices 128-191, values fall 0 to -127)
    dc.b      0,  -3,  -6,  -9, -12, -15, -18, -21
    dc.b    -24, -27, -30, -33, -36, -39, -42, -45
    dc.b    -48, -51, -54, -57, -59, -62, -65, -67
    dc.b    -70, -73, -75, -78, -80, -82, -85, -87
    dc.b    -89, -91, -94, -96, -98,-100,-102,-103
    dc.b   -105,-107,-108,-110,-112,-113,-114,-116
    dc.b   -117,-118,-119,-120,-121,-122,-123,-123
    dc.b   -124,-125,-125,-126,-126,-126,-126,-126

    ; Quadrant 4: 270 to 360 degrees (indices 192-255, values rise -127 to 0)
    dc.b   -127,-126,-126,-126,-126,-126,-125,-125
    dc.b   -124,-123,-123,-122,-121,-120,-119,-118
    dc.b   -117,-116,-114,-113,-112,-110,-108,-107
    dc.b   -105,-103,-102,-100, -98, -96, -94, -91
    dc.b    -89, -87, -85, -82, -80, -78, -75, -73
    dc.b    -70, -67, -65, -62, -59, -57, -54, -51
    dc.b    -48, -45, -42, -39, -36, -33, -30, -27
    dc.b    -24, -21, -18, -15, -12,  -9,  -6,  -3

    even

; ============================================================================
; MUSIC DATA - AY TRACKER MODULE
; ============================================================================
; "Platoon" by Rob Hubbard, originally composed for the C64 (1987).
; This is the ZX Spectrum AY chip version. The PSG+ engine parses the
; AY file format and drives 3-channel synthesis autonomously.

music_data:
    incbin  "../assets/music/Platoon.ay"
music_data_end:

    end start
