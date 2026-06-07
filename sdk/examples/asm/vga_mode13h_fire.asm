; ============================================================================
; VGA MODE 13h FIRESTORM - PALETTE-CYCLED DOS INTRO EFFECT
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
; Builds a classic Mode 13h fire effect into a fuller intro-style effect.  The
; fire is rendered into a RAM back buffer, fed by turbulent full-width fuel and
; moving burner jets, palette-cycled during vertical blank, then copied into the
; visible 64K VGA aperture.
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
;   0x8800          Runtime variables
;   0x20000         RAM back buffer, 320x200 bytes
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
.equ VAR_FRAME      0x8804
.equ VAR_BURNER1    0x8808
.equ VAR_BURNER2    0x880C
.equ VAR_BURNER3    0x8810
.equ FIRE_BUFFER    0x20000

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

    LDA #0
    STA @VAR_FRAME

    JSR setup_palette

    JSR clear_vram
    JSR clear_buffer

; ============================================================================
; MAIN LOOP
; ============================================================================
; Each frame: update the RAM fire buffer, then wait for vertical blank before
; updating the palette and copying the finished image to VGA.
; ============================================================================
main_loop:
    JSR fire_source

    JSR fire_propagate

    JSR wait_vsync

    JSR setup_palette

    JSR copy_to_vram

    LDA @VAR_FRAME
    ADD A, #1
    STA @VAR_FRAME

    JMP main_loop

; ============================================================================
; WAIT FOR VSYNC
; ============================================================================
wait_vsync:
.wait_end:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JNZ A, .wait_end
.wait_start:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JZ A, .wait_start
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
; CLEAR RAM BACK BUFFER
; ============================================================================
clear_buffer:
    LDX #FIRE_BUFFER
    LDY #SCREEN_SIZE
.buf_clr:
    LDA #0
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .buf_clr
    RTS

; ============================================================================
; COPY BACK BUFFER TO VISIBLE MODE 13h VRAM
; ============================================================================
; Mode 13h exposes a single 64K VGA aperture, so the demo composes each frame
; in normal RAM and performs one linear copy during vertical blank.
; ============================================================================
copy_to_vram:
    LDX #FIRE_BUFFER
    LDF #VGA_VRAM
    LDY #SCREEN_SIZE
.copy:
    LDA [X]
    STA [F]
    ADD X, #1
    ADD F, #1
    SUB Y, #1
    JNZ Y, .copy
    RTS

; ============================================================================
; SET UP FIRE PALETTE
; ============================================================================
; Programmes the VGA DAC with a fire gradient across all 256 colour indices.
; The gradient ramps through three phases:
;
;   Indices   0-63:  Black to deep red
;   Indices  64-179: Red through orange into yellow
;   Indices 180-255: Yellow to white-hot tips
;
; === WHY PALETTE DESIGN MATTERS ===
; The palette is rebuilt each frame with a small phase offset in the green and
; smoke-blue channels.  This gives shimmer without moving extra pixels.
; ============================================================================
setup_palette:
    LDX #0

.pal_loop:
    LDA X
    STA @VGA_DAC_WINDEX

    ; --- Red channel: ramps across the lower half, clamped at 63 ---
    LDA X
    SHR A, #1
    LDB #63
    SUB B, A
    AND B, #0x80000000
    JNZ B, .r_clamp
    JMP .r_ok
.r_clamp:
    LDA #63
.r_ok:
    STA @VGA_DAC_DATA

    ; --- Green channel: starts later, so the base stays red/orange ---
    LDA X
    SUB A, #64
    AND A, #0x80000000
    JNZ A, .g_zero
    LDA X
    SUB A, #64
    SHR A, #1
    LDC @VAR_FRAME
    AND C, #3
    ADD A, C
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

    ; --- Blue channel: faint smoke low down, pale only at the hottest tips ---
    LDA X
    SUB A, #48
    AND A, #0x80000000
    JNZ A, .b_smoke
    LDA X
    SUB A, #180
    AND A, #0x80000000
    JNZ A, .b_zero
    LDA X
    SUB A, #180
    SHR A, #1
    LDB #40
    SUB B, A
    AND B, #0x80000000
    JNZ B, .b_clamp
    JMP .b_ok
.b_zero:
    LDA #0
    JMP .b_ok
.b_smoke:
    LDA X
    SHR A, #3
    LDC @VAR_FRAME
    AND C, #3
    ADD A, C
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
; Fills the bottom scanline with turbulent full-width fuel, then reinforces it
; with three moving burner jets.  The jets shape the flame without leaving dead
; gaps between them.
;
; === WHY XOR-SHIFT PRNG ===
; XOR-shift generators are ideal for demoscene effects: they are fast
; (three XOR + shift operations), require minimal state (a single 32-bit
; seed), and produce visually convincing randomness.  Perfect distribution
; is not needed here -- we just want chaotic flame bases.
; ============================================================================
fire_source:
    LDA @VAR_FRAME
    MUL A, #3
    AND A, #255
    ADD A, #32
    STA @VAR_BURNER1

    LDA @VAR_FRAME
    MUL A, #5
    ADD A, #107
    AND A, #255
    ADD A, #32
    STA @VAR_BURNER2

    LDA @VAR_FRAME
    MUL A, #7
    ADD A, #211
    AND A, #255
    ADD A, #32
    STA @VAR_BURNER3

    LDX #FIRE_BUFFER
    LDA #HEIGHT
    SUB A, #1
    MUL A, #WIDTH
    ADD X, A

    LDY #WIDTH
    LDU #0

.src_loop:
    LDC #0

    LDB @VAR_BURNER1
    JSR burner_heat

    LDB @VAR_BURNER2
    JSR burner_heat

    LDB @VAR_BURNER3
    JSR burner_heat

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

    ; Full-width hot base, with burners taking over where they are stronger.
    AND A, #127
    ADD A, #112
    LDB C
    SUB B, A
    AND B, #0x80000000
    JNZ B, .store_source
    LDA C

.store_source:
    STA [X]

.next_source:
    ADD X, #1
    ADD U, #1
    SUB Y, #1
    JNZ Y, .src_loop

    RTS

; ============================================================================
; BURNER HEAT
; ============================================================================
; Input:  U = current X position, B = burner centre, C = current heat.
; Output: C = max(C, 224 - abs(x-centre)*4) for pixels within 28 columns.
; ============================================================================
burner_heat:
    LDA U
    SUB A, B
    LDE A
    AND E, #0x80000000
    JZ E, .abs_done
    XOR A, #0xFFFFFFFF
    ADD A, #1
.abs_done:
    LDE A
    SUB E, #28
    AND E, #0x80000000
    JZ E, .burner_done

    LDE A
    SHL E, #2
    LDA #224
    SUB A, E

    LDE C
    SUB E, A
    AND E, #0x80000000
    JZ E, .burner_done
    LDC A
.burner_done:
    RTS

; ============================================================================
; FIRE PROPAGATION - AVERAGE AND DECAY UPWARD
; ============================================================================
; Processes every pixel from row HEIGHT-2 up to row 0.  Each pixel's new
; value is the average of four samples from the row below: centre twice, plus
; left and right.  The centre weighting keeps the flames tall while the side
; samples spread them into natural tongues.
;
; === WHY BOTTOM-TO-TOP PROCESSING ===
; Processing rows from bottom to top means each row reads from the row
; below (which has already been written this frame), allowing heat to
; propagate multiple rows upward within a single frame.  This makes the
; flames appear to rise more quickly and naturally.
;
; === WHY CENTRE IS SAMPLED TWICE ===
; Three samples divided by four cools too quickly, making the flame only a few
; pixels high.  Sampling the centre twice gives a true /4 average without
; needing a slow divide by 3.
; ============================================================================
fire_propagate:
    LDY #HEIGHT
    SUB Y, #2

.row_loop:
    LDA Y
    AND A, #0x80000000
    JNZ A, .done

    ; Precompute row bases once per row.
    LDA Y
    MUL A, #WIDTH
    ADD A, #FIRE_BUFFER
    LDE A                              ; E = current row base
    ADD A, #WIDTH
    LDT A                              ; T = row below base

    LDX #0

.col_loop:
    ; --- Address of current pixel (destination) ---
    LDA E
    ADD A, X
    LDF A

    ; --- Address of pixel directly below (source centre) ---
    LDA T
    ADD A, X
    LDC A

    ; --- Start sum with centre-below value ---
    LDA [C]
    AND A, #0xFF
    LDB A
    ADD B, A

    ; --- Add left-below neighbour (skip if at left edge) ---
    LDA X
    JZ A, .no_left
    LDA C
    SUB A, #1
    LDU A
    LDA [U]
    AND A, #0xFF
    ADD B, A
.no_left:

    ; --- Add right-below neighbour (skip if at right edge) ---
    LDA X
    LDU #WIDTH
    SUB U, #1
    SUB A, U
    JZ A, .no_right
    LDA C
    ADD A, #1
    LDU A
    LDA [U]
    AND A, #0xFF
    ADD B, A
.no_right:

    ; --- Average: sum / 4 (fast shift), then subtract 1 for cooling ---
    LDA B
    SHR A, #2

    SUB A, #1

    ; --- Clamp to 0 on underflow (prevents wrap-around to bright colours) ---
    LDU A
    AND U, #0x80000000
    JZ U, .clamp_ok
    LDA #0
.clamp_ok:

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
