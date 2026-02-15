; ============================================================================
; VGA MODE 13h FIRE EFFECT - CLASSIC DOS DEMOSCENE TECHNIQUE
; IE32 Assembly for IntuitionEngine - VGA Mode 13h (320x200x256)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    VGA Mode 13h (320x200, 256-colour linear framebuffer)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm sdk/examples/asm/vga_mode13h_fire.asm
; Run:           ./bin/IntuitionEngine -ie32 vga_mode13h_fire.iex
; Porting:       VGA MMIO is CPU-agnostic. The fire algorithm is simple enough
;                to run on any CPU core, even 8-bit (6502/Z80).
;
; === WHAT THIS DEMO DOES ===
; Implements the classic DOS-era fire effect in VGA Mode 13h (320x200, 256
; colours). The bottom row is seeded with random hot pixels, and each frame
; propagates heat upward with cooling, creating a realistic flame effect.
; A custom fire palette maps colour indices to black -> red -> yellow -> white.
;
; === WHY VGA MODE 13h (320x200x256 LINEAR) ===
; Mode 13h is the legendary "MCGA mode" -- the single most important graphics
; mode in PC gaming and demoscene history.  Introduced with the IBM PS/2's
; MCGA adapter in 1987 and adopted by VGA, it provides a dead-simple linear
; framebuffer: one byte per pixel, 256 colours chosen from a programmable
; 18-bit palette.  The pixel at screen position (x, y) lives at VRAM address
; 0xA0000 + y*320 + x.  No planes, no bank switching, no tricks required.
;
; This simplicity made Mode 13h THE standard for DOS games (Doom, Duke
; Nukem 3D, Quake's software renderer, countless shareware titles) and
; demoscene productions throughout the late 1980s and 1990s.  The fire
; effect shown here was one of the most popular demo effects of that era,
; appearing in hundreds of demos, intros, and cracktros.  Its appeal lay
; in how a tiny amount of code -- seed the bottom row with random values,
; average neighbours upward with decay -- produced surprisingly convincing
; flames when paired with a carefully crafted palette.
;
; === MEMORY MAP ===
;   0x1000          Program code entry point
;   0x8800          VAR_SEED - PRNG state for bottom-row seeding
;   VGA_VRAM        320x200 linear VRAM (64000 bytes, one byte per pixel)
;
; === BUILD AND RUN ===
;   sdk/bin/ie32asm sdk/examples/asm/vga_mode13h_fire.asm
;   ./bin/IntuitionEngine -ie32 vga_mode13h_fire.iex
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie32.inc"

; ---------------------------------------------------------------------------
; Screen geometry for Mode 13h
; ---------------------------------------------------------------------------
; 320 x 200 = 64000 bytes total.  One byte per pixel, linear layout.
.equ WIDTH          320
.equ HEIGHT         200
.equ SCREEN_SIZE    64000

; ---------------------------------------------------------------------------
; Variables in scratch RAM
; ---------------------------------------------------------------------------
.equ VAR_SEED       0x8800

.org 0x1000

; ============================================================================
; ENTRY POINT
; ============================================================================
; Initialise VGA, select Mode 13h, set up the fire palette, clear the
; framebuffer, then enter the main loop.
; ============================================================================
start:
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    LDA #VGA_MODE_13H
    STA @VGA_MODE

    LDA #12345678
    STA @VAR_SEED

    JSR setup_palette

    JSR clear_vram

; ============================================================================
; MAIN LOOP
; ============================================================================
; Each frame: synchronise to vertical blank, seed the bottom row with fresh
; random "heat" values, then propagate the fire upward with averaging and
; decay.  The palette does the rest -- colour index 0 is black (cool) and
; higher indices progress through red, orange, yellow, and white (hot).
; ============================================================================
main_loop:
    JSR wait_vsync

    JSR fire_source

    JSR fire_propagate

    JMP main_loop

; ============================================================================
; WAIT FOR VSYNC
; ============================================================================
wait_vsync:
.wait:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JZ A, .wait
    RTS

; ============================================================================
; CLEAR VRAM TO BLACK
; ============================================================================
; Zeroes all 64000 bytes of the Mode 13h framebuffer.  Colour index 0
; is black in our fire palette.
; ============================================================================
clear_vram:
    LDX #VGA_VRAM
    LDY #SCREEN_SIZE
.clr:
    LDA #0
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .clr
    RTS

; ============================================================================
; SET UP FIRE PALETTE
; ============================================================================
; Programmes the VGA DAC with a fire gradient across all 256 colour indices.
; The gradient ramps through three phases:
;
;   Indices  0-15:  Black to red     (R ramps, G=0, B=0)
;   Indices 16-47:  Red to yellow    (R=max, G ramps, B=0)
;   Indices 48-63:  Yellow to white  (R=max, G=max, B ramps)
;   Indices 64+:    White            (R=max, G=max, B=max)
;
; === WHY PALETTE DESIGN MATTERS ===
; The fire effect only writes colour indices (0-255) into VRAM.  All visual
; warmth comes from the palette mapping those indices to RGB values.  A
; carefully tuned gradient is essential: too abrupt and the flames look
; banded, too gradual and they appear washed out.
; ============================================================================
setup_palette:
    LDX #0

.pal_loop:
    STA @VGA_DAC_WINDEX
    LDA X
    STA @VGA_DAC_WINDEX

    ; --- Red channel: ramps up first, clamped at 63 ---
    LDA X
    MUL A, #4
    LDB #63
    SUB B, A
    AND B, #0x80000000
    JNZ B, .r_clamp
    JMP .r_ok
.r_clamp:
    LDA #63
.r_ok:
    STA @VGA_DAC_DATA

    ; --- Green channel: begins ramping after index 16 ---
    LDA X
    SUB A, #16
    AND A, #0x80000000
    JNZ A, .g_zero
    LDA X
    SUB A, #16
    MUL A, #4
    LDB #63
    SUB B, A
    AND B, #0x80000000
    JNZ B, .g_clamp
    JMP .g_ok
.g_zero:
    LDA #0
    JMP .g_ok
.g_clamp:
    LDA #63
.g_ok:
    STA @VGA_DAC_DATA

    ; --- Blue channel: begins ramping after index 48 ---
    LDA X
    SUB A, #48
    AND A, #0x80000000
    JNZ A, .b_zero
    LDA X
    SUB A, #48
    MUL A, #4
    LDB #63
    SUB B, A
    AND B, #0x80000000
    JNZ B, .b_clamp
    JMP .b_ok
.b_zero:
    LDA #0
    JMP .b_ok
.b_clamp:
    LDA #63
.b_ok:
    STA @VGA_DAC_DATA

    ADD X, #1
    LDA #256
    SUB A, X
    JNZ A, .pal_loop

    RTS

; ============================================================================
; FIRE SOURCE - SEED THE BOTTOM ROW
; ============================================================================
; Fills the bottom scanline (row 199) with random colour index values using
; an XOR-shift pseudo-random number generator.  Values above 128 are biased
; brighter (OR'd with 0xC0) to ensure a strong flame base.  This row acts
; as the "fuel" that the propagation step draws heat from.
;
; === WHY XOR-SHIFT PRNG ===
; XOR-shift generators are ideal for demoscene effects: they are fast
; (three XOR + shift operations), require minimal state (a single 32-bit
; seed), and produce visually convincing randomness.  Perfect distribution
; is not needed here -- we just want chaotic flame bases.
; ============================================================================
fire_source:
    LDX #VGA_VRAM
    LDA #HEIGHT
    SUB A, #1
    MUL A, #WIDTH
    ADD X, A

    LDY #WIDTH

.src_loop:
    ; --- XOR-shift PRNG (period ~2^32) ---
    LDA @VAR_SEED
    LDB A
    SHL B, #3
    XOR A, B
    LDB A
    SHR B, #5
    XOR A, B
    LDB A
    SHL B, #4
    XOR A, B
    STA @VAR_SEED

    ; --- Extract low byte as colour index ---
    AND A, #0xFF

    ; --- Bias values >= 128 toward hot colours ---
    LDB A
    SUB B, #128
    AND B, #0x80000000
    JNZ B, .dim
    OR A, #0xC0
.dim:
    STA [X]

    ADD X, #1
    SUB Y, #1
    JNZ Y, .src_loop

    RTS

; ============================================================================
; FIRE PROPAGATION - AVERAGE AND DECAY UPWARD
; ============================================================================
; Processes every pixel from row HEIGHT-2 up to row 0.  Each pixel's new
; value is the average of its three neighbours on the row below (centre,
; left, right), divided by 4 and decremented by 1.  This produces a natural
; upward spread with gradual cooling -- the essence of the fire effect.
;
; === WHY BOTTOM-TO-TOP PROCESSING ===
; Processing rows from bottom to top means each row reads from the row
; below (which has already been written this frame), allowing heat to
; propagate multiple rows upward within a single frame.  This makes the
; flames appear to rise more quickly and naturally.
;
; === WHY DIVIDE BY 4 INSTEAD OF 3 ===
; Dividing by 4 (a single right-shift by 2) is much faster than dividing
; by 3 on the IE32.  The slight extra cooling from dividing by 4 instead
; of 3 is compensated by the -1 decay constant.  Edge pixels only sample
; 2 neighbours, but the /4 keeps them from being disproportionately bright.
; ============================================================================
fire_propagate:
    LDY #HEIGHT
    SUB Y, #2

.row_loop:
    LDA Y
    AND A, #0x80000000
    JNZ A, .done

    LDX #0

.col_loop:
    ; --- Address of current pixel (destination) ---
    LDA Y
    MUL A, #WIDTH
    ADD A, X
    ADD A, #VGA_VRAM
    LDF A

    ; --- Address of pixel directly below (source centre) ---
    LDA Y
    ADD A, #1
    MUL A, #WIDTH
    ADD A, X
    ADD A, #VGA_VRAM
    LDT A

    ; --- Start sum with centre-below value ---
    LDA [T]
    AND A, #0xFF
    LDB A

    ; --- Add left-below neighbour (skip if at left edge) ---
    LDA X
    JZ A, .no_left
    LDA T
    SUB A, #1
    LDC A
    LDA [C]
    AND A, #0xFF
    ADD B, A
.no_left:

    ; --- Add right-below neighbour (skip if at right edge) ---
    LDA X
    LDC #WIDTH
    SUB C, #1
    SUB A, C
    JZ A, .no_right
    LDA T
    ADD A, #1
    LDC A
    LDA [C]
    AND A, #0xFF
    ADD B, A
.no_right:

    ; --- Average: sum / 4 (fast shift), then subtract 1 for cooling ---
    LDA B
    SHR A, #2

    SUB A, #1

    ; --- Clamp to 0 on underflow (prevents wrap-around to bright colours) ---
    LDB A
    AND B, #0x80000000
    JZ B, .no_clamp
    LDA #0
.no_clamp:

    ; --- Write cooled value to current pixel ---
    STA [F]

    ADD X, #1
    LDA #WIDTH
    SUB A, X
    JNZ A, .col_loop

    ; --- Move up one row ---
    SUB Y, #1
    JMP .row_loop

.done:
    RTS
