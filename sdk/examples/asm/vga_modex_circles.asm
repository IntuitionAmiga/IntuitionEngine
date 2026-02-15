; ============================================================================
; VGA MODE X CIRCLES - UNCHAINED PLANAR GRAPHICS WITH PAGE FLIPPING
; IE32 Assembly for IntuitionEngine - VGA Mode X (320x240x256 Unchained)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    VGA Mode X (320x240, 256-colour unchained/planar)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm sdk/examples/asm/vga_modex_circles.asm
; Run:           ./bin/IntuitionEngine -ie32 vga_modex_circles.iex
; Porting:       VGA MMIO is CPU-agnostic. The Mode X plane selection and
;                page-flipping logic applies to any CPU core driving VGA.
;
; === WHAT THIS DEMO DOES ===
; Draws 8 concentric expanding circles at the centre of a 320x240 Mode X
; display.  Each circle uses the midpoint (Bresenham) circle algorithm and
; is rendered in a different cycling colour from a rainbow gradient palette.
; Double buffering via page flipping eliminates flicker: one page is
; displayed while the other is being drawn, then they are swapped each
; frame.
;
; === WHY VGA MODE X (320x240 UNCHAINED) ===
; Mode X was famously discovered and documented by Michael Abrash in his
; 1991 Dr. Dobb's Journal articles (later collected in the "Graphics
; Programming Black Book").  It is not a standard BIOS mode -- it is
; created by reprogramming VGA registers to unchain the four memory planes
; and adjust timing for 240 scanlines instead of 200.
;
; In standard Mode 13h, the VGA chains all four planes together to present
; a simple linear framebuffer (one byte per pixel), but this wastes 3/4 of
; the 256KB VRAM and prevents page flipping.  Mode X unchains the planes,
; giving access to all 256KB.  Each pixel selects its plane via the
; Sequence Controller's Map Mask register (plane = x & 3), and the byte
; offset within VRAM is y * 80 + x / 4.
;
; The key advantages of Mode X over Mode 13h are:
;   - Page flipping: 320x240 / 4 = 19200 bytes per page per plane, so
;     multiple pages fit in 256KB VRAM, enabling tear-free double buffering.
;   - Square pixels: 320x240 has a 4:3 aspect ratio matching the monitor,
;     unlike 320x200 which has slightly rectangular pixels.
;   - Hardware scrolling: the CRTC start address register can pan the
;     visible page without moving any data.
;
; Mode X was used extensively in commercial games (Ultima Underworld,
; X-COM, early Doom prototypes) and became a staple of DOS demoscene
; productions for smooth, flicker-free animation.
;
; === MEMORY MAP ===
;   0x1000          Program code entry point
;   0x8800          VAR_FRAME - animation frame counter
;   0x8804          VAR_PAGE - current front-buffer page (0 or 1)
;   0x8808          VAR_RADIUS - base radius for circle animation
;   VGA_VRAM+0      Page 0 (19200 bytes per plane)
;   VGA_VRAM+19200  Page 1 (19200 bytes per plane)
;
; === BUILD AND RUN ===
;   sdk/bin/ie32asm sdk/examples/asm/vga_modex_circles.asm
;   ./bin/IntuitionEngine -ie32 vga_modex_circles.iex
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie32.inc"

; ---------------------------------------------------------------------------
; Screen geometry for Mode X
; ---------------------------------------------------------------------------
; 320 pixels wide, 240 scanlines.  In unchained mode each plane holds
; 320/4 = 80 bytes per scanline, so one page = 80 * 240 = 19200 bytes.
.equ WIDTH          320
.equ HEIGHT         240
.equ PAGE_SIZE      19200

; ---------------------------------------------------------------------------
; Page offsets within VRAM
; ---------------------------------------------------------------------------
.equ PAGE0          0
.equ PAGE1          19200

; ---------------------------------------------------------------------------
; Screen centre for circle rendering
; ---------------------------------------------------------------------------
.equ CENTER_X       160
.equ CENTER_Y       120

; ---------------------------------------------------------------------------
; Variables in scratch RAM
; ---------------------------------------------------------------------------
.equ VAR_FRAME      0x8800
.equ VAR_PAGE       0x8804
.equ VAR_RADIUS     0x8808

.org 0x1000

; ============================================================================
; ENTRY POINT
; ============================================================================
; Initialise VGA hardware, select Mode X, set up the colour palette, and
; enter the main animation loop.
; ============================================================================
start:
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    LDA #VGA_MODE_X
    STA @VGA_MODE

    JSR setup_palette

    LDA #0
    STA @VAR_FRAME
    STA @VAR_PAGE

; ============================================================================
; MAIN LOOP
; ============================================================================
; Each frame: synchronise to vertical blank, clear the back buffer, draw
; the circles into it, then flip pages so the freshly drawn buffer becomes
; visible.  This double-buffering prevents any partial drawing from being
; seen on screen.
; ============================================================================
main_loop:
    JSR wait_vsync

    JSR clear_page

    JSR draw_circles

    JSR flip_page

    LDA @VAR_FRAME
    ADD A, #1
    STA @VAR_FRAME

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
; SET UP RAINBOW GRADIENT PALETTE
; ============================================================================
; Programmes 256 palette entries with offset triangular waves for R, G, B,
; creating a smooth rainbow gradient.  The offsets (0, 21, 42) space the
; channels roughly 1/3 of 64 apart, producing distinct hues across the
; palette range.
; ============================================================================
setup_palette:
    LDX #0

.pal_loop:
    LDA X
    STA @VGA_DAC_WINDEX

    ; Red channel: triangular wave, phase 0
    LDA X
    AND A, #0x3F
    STA @VGA_DAC_DATA

    ; Green channel: triangular wave, phase offset +21
    LDA X
    ADD A, #21
    AND A, #0x3F
    STA @VGA_DAC_DATA

    ; Blue channel: triangular wave, phase offset +42
    LDA X
    ADD A, #42
    AND A, #0x3F
    STA @VGA_DAC_DATA

    ADD X, #1
    LDA #256
    SUB A, X
    JNZ A, .pal_loop

    RTS

; ============================================================================
; CLEAR BACK BUFFER
; ============================================================================
; Zeroes the back-buffer page in Mode X.  All four planes are enabled via
; Map Mask so a single byte write clears 4 pixels simultaneously, making
; the clear operation 4x faster than clearing plane by plane.
;
; === WHY PAGE SELECTION MATTERS ===
; VAR_PAGE tracks which page is currently displayed (front buffer).
; We clear the OTHER page (back buffer) before drawing into it.
; ============================================================================
clear_page:
    LDA #0x0F
    STA @VGA_SEQ_MAPMASK

    LDA @VAR_PAGE
    JZ A, .clear_page1
    LDX #VGA_VRAM
    ADD X, #19200
    JMP .do_clear
.clear_page1:
    LDX #VGA_VRAM

.do_clear:
    LDY #19200

.clr:
    LDA #0
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .clr
    RTS

; ============================================================================
; DRAW CONCENTRIC EXPANDING CIRCLES
; ============================================================================
; Draws 8 circles, each with a radius offset by 16 pixels from the
; previous one.  The base radius oscillates with the frame counter (masked
; to 0-127), so the circles appear to expand outward from the centre and
; wrap around.  Each circle gets a distinct colour that also cycles with
; the frame counter, producing a kaleidoscopic rainbow animation.
; ============================================================================
draw_circles:
    LDA @VAR_FRAME
    AND A, #0x7F
    STA @VAR_RADIUS

    LDT #0

.circle_loop:
    ; Calculate this circle's radius (base + index * 16, wrapped to 0-127)
    LDA @VAR_RADIUS
    LDB T
    MUL B, #16
    ADD A, B
    AND A, #0x7F

    JZ A, .next_circle

    ; Colour = (circle_index * 32 + frame_counter) mod 256
    LDB T
    MUL B, #32
    ADD B, @VAR_FRAME
    AND B, #0xFF

    JSR draw_circle

.next_circle:
    ADD T, #1
    LDA #8
    SUB A, T
    JNZ A, .circle_loop

    RTS

; ============================================================================
; DRAW A SINGLE CIRCLE (MIDPOINT / BRESENHAM ALGORITHM)
; ============================================================================
; Input: A = radius, B = colour index
;
; The midpoint circle algorithm plots points in one octant and mirrors them
; across all 8 octants of the circle.  It uses only integer addition and
; subtraction (no multiplication or square roots), making it efficient on
; the IE32.  The "decision" variable tracks accumulated error and determines
; whether to step diagonally or horizontally at each iteration.
;
; === WHY BRESENHAM FOR CIRCLES ===
; Jack Bresenham's circle algorithm (1977) was designed for plotters and
; early raster displays where floating-point maths was prohibitively slow.
; It remains the standard approach for drawing circles in software
; renderers and retro-style demos.
; ============================================================================
draw_circle:
    PUSH C
    PUSH D
    PUSH E
    PUSH F
    PUSH T
    PUSH U

    LDC A               ; C = radius
    LDD B               ; D = colour
    LDE #0              ; E = x = 0
    LDF C               ; F = y = radius
    LDT #1
    SUB T, C             ; T = decision = 1 - radius

.dc_loop:
    ; --- Plot all 8 octant reflections of the current point ---

    ; Octant 1: (cx+x, cy+y)
    LDA #CENTER_X
    ADD A, E
    LDB #CENTER_Y
    ADD B, F
    JSR plot_pixel

    ; Octant 2: (cx-x, cy+y)
    LDA #CENTER_X
    SUB A, E
    LDB #CENTER_Y
    ADD B, F
    JSR plot_pixel

    ; Octant 3: (cx+x, cy-y)
    LDA #CENTER_X
    ADD A, E
    LDB #CENTER_Y
    SUB B, F
    JSR plot_pixel

    ; Octant 4: (cx-x, cy-y)
    LDA #CENTER_X
    SUB A, E
    LDB #CENTER_Y
    SUB B, F
    JSR plot_pixel

    ; Octant 5: (cx+y, cy+x)
    LDA #CENTER_X
    ADD A, F
    LDB #CENTER_Y
    ADD B, E
    JSR plot_pixel

    ; Octant 6: (cx-y, cy+x)
    LDA #CENTER_X
    SUB A, F
    LDB #CENTER_Y
    ADD B, E
    JSR plot_pixel

    ; Octant 7: (cx+y, cy-x)
    LDA #CENTER_X
    ADD A, F
    LDB #CENTER_Y
    SUB B, E
    JSR plot_pixel

    ; Octant 8: (cx-y, cy-x)
    LDA #CENTER_X
    SUB A, F
    LDB #CENTER_Y
    SUB B, E
    JSR plot_pixel

    ; --- Update the decision variable and step coordinates ---
    LDA T
    AND A, #0x80000000
    JZ A, .dec_positive

    ; Decision < 0: move east (x only)
    ; decision += 2*x + 3
    LDA E
    MUL A, #2
    ADD A, #3
    ADD T, A
    ADD E, #1
    JMP .dc_check

.dec_positive:
    ; Decision >= 0: move south-east (x and y)
    ; decision += 2*(x - y) + 5
    LDA E
    SUB A, F
    MUL A, #2
    ADD A, #5
    ADD T, A
    ADD E, #1
    SUB F, #1

.dc_check:
    ; Continue while x <= y
    LDA F
    SUB A, E
    AND A, #0x80000000
    JZ A, .dc_loop

    POP U
    POP T
    POP F
    POP E
    POP D
    POP C
    RTS

; ============================================================================
; PLOT A SINGLE PIXEL IN MODE X
; ============================================================================
; Input: A = x coordinate, B = y coordinate, D = colour index
;
; === WHY MODE X PIXEL ADDRESSING IS DIFFERENT ===
; In Mode X, the four VGA memory planes are unchained.  Each pixel is
; stored in one specific plane, determined by (x & 3).  The byte offset
; within that plane is y * 80 + x / 4.  Before writing, we must tell the
; VGA which plane to target by writing (1 << plane) to the Map Mask
; register.  This is more complex than Mode 13h's simple linear addressing
; but unlocks access to all 256KB of VRAM for page flipping.
; ============================================================================
plot_pixel:
    PUSH F
    PUSH E

    ; --- Bounds checking ---
    LDF A
    AND F, #0x80000000
    JNZ F, .pp_skip     ; x < 0
    LDF A
    SUB F, #WIDTH
    AND F, #0x80000000
    JZ F, .pp_skip      ; x >= 320

    LDF B
    AND F, #0x80000000
    JNZ F, .pp_skip     ; y < 0
    LDF B
    SUB F, #HEIGHT
    AND F, #0x80000000
    JZ F, .pp_skip      ; y >= 240

    ; --- Select the correct plane: plane = x & 3, mask = 1 << plane ---
    LDE A
    AND E, #3
    LDF #1
    SHL F, E
    PUSH A
    LDA F
    STA @VGA_SEQ_MAPMASK
    POP A

    ; --- Calculate planar byte offset: y * 80 + x / 4 ---
    LDF B
    MUL F, #80
    LDE A
    SHR E, #2
    ADD F, E

    ; --- Add back-buffer page offset ---
    LDA @VAR_PAGE
    JZ A, .pp_page1
    ADD F, #19200
    JMP .pp_addr
.pp_page1:

.pp_addr:
    ADD F, #VGA_VRAM

    ; --- Write the colour byte ---
    LDA D
    STA [F]

.pp_skip:
    POP E
    POP F
    RTS

; ============================================================================
; FLIP DISPLAY PAGE
; ============================================================================
; Toggles VAR_PAGE between 0 and 1, then programmes the VGA CRTC start
; address registers to make the newly completed page visible.  The page
; that was being displayed becomes the new back buffer for the next frame.
;
; === WHY HARDWARE PAGE FLIPPING ===
; The CRTC start address registers tell the VGA where in VRAM to begin
; scanning out the display.  By changing this address, we can instantly
; switch which page is visible -- no data needs to be copied.  Combined
; with vsync timing, this produces perfectly smooth, tear-free animation.
; ============================================================================
flip_page:
    LDA @VAR_PAGE
    XOR A, #1
    STA @VAR_PAGE

    JZ A, .show_page0
    ; Show page 1
    LDA #PAGE1
    SHR A, #8
    STA @VGA_CRTC_STARTHI
    LDA #PAGE1
    AND A, #0xFF
    STA @VGA_CRTC_STARTLO
    RTS

.show_page0:
    LDA #0
    STA @VGA_CRTC_STARTHI
    STA @VGA_CRTC_STARTLO
    RTS
