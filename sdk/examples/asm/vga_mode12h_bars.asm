; ============================================================================
; VGA MODE 12h COLOUR BARS - PLANAR GRAPHICS FUNDAMENTALS
; IE32 Assembly for IntuitionEngine - VGA Mode 12h (640x480x16 Planar)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    VGA Mode 12h (640x480, 16-colour planar framebuffer)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm sdk/examples/asm/vga_mode12h_bars.asm
; Run:           ./bin/IntuitionEngine -ie32 vga_mode12h_bars.iex
; Porting:       VGA MMIO is CPU-agnostic. The planar colour bar algorithm
;                works on any CPU core, though the bit-plane manipulation
;                concepts are VGA-specific.
;
; === WHAT THIS DEMO DOES ===
; Draws 16 vertical colour bars across a 640x480 VGA Mode 12h display, one
; bar for each of the 16 available palette colours. An animated white
; diagonal line sweeps across the screen, demonstrating how to set individual
; pixels in planar mode. The bars fill the entire screen height and each bar
; is 40 pixels wide (640 / 16 = 40).
;
; === WHY VGA MODE 12h (640x480x16 PLANAR) ===
; Mode 12h is IBM VGA's high-resolution graphics mode, introduced with the
; PS/2 in 1987. Unlike Mode 13h's simple linear framebuffer, Mode 12h
; organises VRAM into four separate bit planes. Each pixel's 4-bit colour
; index is spread across these planes: bit 0 in plane 0, bit 1 in plane 1,
; and so on. To write a pixel, you must select which planes to enable via
; the Sequence Controller's Map Mask register (port 0x3C4, index 2).
;
; This planar architecture was inherited from EGA and allowed the VGA to
; display 640x480 pixels in just 150KB of VRAM (4 planes x 38,400 bytes).
; It was the standard mode for DOS CAD applications, business graphics, and
; Windows 3.x. Demosceners also exploited it for high-resolution effects,
; though Mode 13h and Mode X were far more popular for demos and games due
; to their simpler addressing and richer colour depth.
;
; Key concept: each byte in VRAM represents 8 horizontal pixels in a single
; plane. Writing 0xFF to a byte with all planes enabled sets 8 consecutive
; pixels to colour 15 (white). The Map Mask register controls which planes
; receive the write, effectively selecting the colour.
;
; === MEMORY MAP ===
;   0x1000          Program code entry point
;   0x8800          VAR_FRAME - animation frame counter
;   0x8804          VAR_LINE_Y - diagonal line Y offset
;   VGA_VRAM        640x480 planar VRAM (4 planes, 38400 bytes each)
;
; === BUILD AND RUN ===
;   sdk/bin/ie32asm sdk/examples/asm/vga_mode12h_bars.asm
;   ./bin/IntuitionEngine -ie32 vga_mode12h_bars.iex
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie32.inc"

; ---------------------------------------------------------------------------
; Screen geometry for Mode 12h
; ---------------------------------------------------------------------------
; 640 pixels wide / 8 bits per byte = 80 bytes per scanline in each plane.
; 480 scanlines total.  Each pixel's colour is assembled from 4 bit planes.
.equ WIDTH          640
.equ HEIGHT         480
.equ BYTES_PER_LINE 80

; ---------------------------------------------------------------------------
; Variables in scratch RAM
; ---------------------------------------------------------------------------
.equ VAR_FRAME      0x8800
.equ VAR_LINE_Y     0x8804

.org 0x1000

; ============================================================================
; ENTRY POINT
; ============================================================================
; Initialise VGA hardware, select Mode 12h, then enter the main loop.
; ============================================================================
start:
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    LDA #VGA_MODE_12H
    STA @VGA_MODE

    LDA #0
    STA @VAR_FRAME

; ============================================================================
; MAIN LOOP
; ============================================================================
; Each frame: synchronise to vertical blank, redraw the 16 colour bars,
; then overlay an animated diagonal line that sweeps via the frame counter.
; ============================================================================
main_loop:
    JSR wait_vsync

    JSR draw_bars

    JSR draw_line

    LDA @VAR_FRAME
    ADD A, #1
    STA @VAR_FRAME

    JMP main_loop

; ============================================================================
; WAIT FOR VSYNC
; ============================================================================
; Polls the VGA status register until the vertical sync bit is set.
; This prevents tearing by ensuring we only update VRAM between frames.
; ============================================================================
wait_vsync:
.wait:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JZ A, .wait
    RTS

; ============================================================================
; DRAW 16 VERTICAL COLOUR BARS
; ============================================================================
; Iterates through colours 0-15.  For each colour, the Map Mask register is
; set to the colour's 4-bit value, which enables writes to the corresponding
; combination of bit planes.  Five bytes of 0xFF are written per scanline
; (5 bytes x 8 pixels = 40 pixels), filling a 40-pixel-wide column for all
; 480 rows.
;
; === WHY MAP MASK SELECTS COLOUR ===
; In planar mode, each plane contributes one bit to the final colour index.
; Setting Map Mask to 0x0F enables all four planes, so writing 0xFF sets
; all 8 pixels to colour 15 (1111 binary = white).  Setting it to 0x04
; enables only plane 2, so writing 0xFF sets pixels to colour 4 (0100
; binary = red in the default VGA palette).
; ============================================================================
draw_bars:
    LDY #0

.color_loop:
    ; --- Calculate the starting X pixel for this colour bar ---
    ; Each bar is 40 pixels wide: X_start = colour_index * 40
    LDA Y
    MUL A, #40
    LDX A

    LDT #0

.row_loop:
    ; --- Select bit planes matching this colour index ---
    LDA Y
    AND A, #0x0F
    STA @VGA_SEQ_MAPMASK

    ; --- Calculate VRAM byte address ---
    ; address = row * 80 + (x_pixel / 8) + VGA_VRAM base
    LDA T
    MUL A, #BYTES_PER_LINE
    LDB X
    SHR B, #3
    ADD A, B
    ADD A, #VGA_VRAM
    LDF A

    ; --- Write 5 consecutive bytes (40 pixels) of solid fill ---
    LDA #0xFF
    LDC #5

.byte_loop:
    STA [F]
    ADD F, #1
    SUB C, #1
    JNZ C, .byte_loop

    ; --- Next scanline ---
    ADD T, #1
    LDA #HEIGHT
    SUB A, T
    JNZ A, .row_loop

    ; --- Next colour ---
    ADD Y, #1
    LDA #16
    SUB A, Y
    JNZ A, .color_loop

    RTS

; ============================================================================
; DRAW ANIMATED DIAGONAL LINE
; ============================================================================
; Draws a white diagonal line across the screen.  The starting Y position
; is derived from the frame counter, so the line scrolls downward over time.
; All four bit planes are enabled (Map Mask = 0x0F) to produce colour 15
; (white).
;
; === WHY INDIVIDUAL PIXEL PLOTTING IS HARDER IN PLANAR MODE ===
; In Mode 13h, setting a pixel is a single byte write.  In Mode 12h, each
; byte covers 8 horizontal pixels, so setting one pixel requires a
; read-modify-write: read the existing byte, OR in the correct bit for the
; target pixel, then write it back.  The bit position is determined by
; (7 - (x mod 8)), since bit 7 is the leftmost pixel in the byte.
; ============================================================================
draw_line:
    ; Enable all planes for a white line (colour 15)
    LDA #0x0F
    STA @VGA_SEQ_MAPMASK

    ; Use the low 8 bits of the frame counter as the Y offset
    LDA @VAR_FRAME
    AND A, #0xFF
    STA @VAR_LINE_Y

    LDX #0

.line_loop:
    ; --- Calculate Y = (frame_offset + X) mod 480 ---
    LDA @VAR_LINE_Y
    ADD A, X
    LDB #480
.mod_loop:
    SUB A, B
    AND A, #0x80000000
    JZ A, .mod_loop
    ADD A, B

    ; --- Calculate VRAM byte address ---
    MUL A, #BYTES_PER_LINE
    LDB X
    SHR B, #3
    ADD A, B
    ADD A, #VGA_VRAM
    LDF A

    ; --- Calculate the bit mask for this pixel's position within the byte ---
    ; Bit 7 = leftmost pixel, so shift 0x80 right by (X mod 8)
    LDA X
    AND A, #7
    LDB #0x80
    LDC A
.shift_loop:
    JZ C, .shift_done
    SHR B, #1
    SUB C, #1
    JMP .shift_loop
.shift_done:

    ; --- Set the pixel bit via read-modify-write ---
    LDA [F]
    OR A, B
    STA [F]

    ; --- Next pixel along the diagonal ---
    ADD X, #1
    LDA #480
    SUB A, X
    JNZ A, .line_loop

    RTS
