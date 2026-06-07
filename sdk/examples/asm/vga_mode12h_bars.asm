; ============================================================================
; VGA MODE 12h PLANAR TUNNEL PLASMA
; IE32 Assembly for IntuitionEngine, VGA 640x480x16 planar graphics
; ============================================================================
;
; Target CPU:    IE32
; Video Chip:    VGA Mode 12h, 640x480, 16-colour planar framebuffer
; Audio Engine:  None
; Assembler:     ie32asm
; Build:         sdk/bin/ie32asm sdk/examples/asm/vga_mode12h_bars.asm
; Run:           ./bin/IntuitionEngine -ie32 vga_mode12h_bars.iex
;
; This replaces the old static colour bars with a full-screen animated tunnel
; plasma.  The renderer fills all 480 scanlines every frame, one plane at a
; time, and writes explicit bit patterns into each plane.  That gives the
; effect sub-byte detail while staying inside genuine Mode 12h planar rules.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie32.inc"

; ---------------------------------------------------------------------------
; Mode 12h geometry
; ---------------------------------------------------------------------------
.equ WIDTH_BYTES     80
.equ HEIGHT          480
.equ PLANE_BYTES     38400

; ---------------------------------------------------------------------------
; Scratch variables
; ---------------------------------------------------------------------------
.equ VAR_FRAME       0x8800
.equ VAR_PLANEMASK   0x8804
.equ VAR_PLANE_SHIFT 0x8808
.equ VAR_ROW         0x880C
.equ VAR_XBYTE       0x8810
.equ VAR_ROW_BASE    0x8814
.equ VAR_COLOUR      0x8818

.org 0x1000

; ============================================================================
; ENTRY POINT
; ============================================================================
start:
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    LDA #VGA_MODE_12H
    STA @VGA_MODE

    JSR setup_palette

    LDA #0
    STA @VAR_FRAME

; ============================================================================
; MAIN LOOP
; ============================================================================
main_loop:
    JSR wait_vsync
    JSR draw_tunnel

    LDA @VAR_FRAME
    ADD A, #1
    STA @VAR_FRAME

    JMP main_loop

; ============================================================================
; WAIT FOR VERTICAL BLANK
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
; SET UP A 16-COLOUR NEON PALETTE
; ============================================================================
setup_palette:
    LDA #0
    STA @VGA_DAC_WINDEX

    ; 0: black
    LDA #0
    STA @VGA_DAC_DATA
    STA @VGA_DAC_DATA
    STA @VGA_DAC_DATA

    ; 1: deep blue
    LDA #2
    STA @VGA_DAC_DATA
    LDA #2
    STA @VGA_DAC_DATA
    LDA #22
    STA @VGA_DAC_DATA

    ; 2: violet
    LDA #18
    STA @VGA_DAC_DATA
    LDA #4
    STA @VGA_DAC_DATA
    LDA #34
    STA @VGA_DAC_DATA

    ; 3: magenta
    LDA #38
    STA @VGA_DAC_DATA
    LDA #6
    STA @VGA_DAC_DATA
    LDA #42
    STA @VGA_DAC_DATA

    ; 4: hot pink
    LDA #58
    STA @VGA_DAC_DATA
    LDA #8
    STA @VGA_DAC_DATA
    LDA #36
    STA @VGA_DAC_DATA

    ; 5: red
    LDA #63
    STA @VGA_DAC_DATA
    LDA #8
    STA @VGA_DAC_DATA
    LDA #8
    STA @VGA_DAC_DATA

    ; 6: orange
    LDA #63
    STA @VGA_DAC_DATA
    LDA #28
    STA @VGA_DAC_DATA
    LDA #4
    STA @VGA_DAC_DATA

    ; 7: amber
    LDA #63
    STA @VGA_DAC_DATA
    LDA #48
    STA @VGA_DAC_DATA
    LDA #6
    STA @VGA_DAC_DATA

    ; 8: yellow
    LDA #63
    STA @VGA_DAC_DATA
    LDA #62
    STA @VGA_DAC_DATA
    LDA #16
    STA @VGA_DAC_DATA

    ; 9: lime
    LDA #34
    STA @VGA_DAC_DATA
    LDA #63
    STA @VGA_DAC_DATA
    LDA #18
    STA @VGA_DAC_DATA

    ; 10: green
    LDA #8
    STA @VGA_DAC_DATA
    LDA #54
    STA @VGA_DAC_DATA
    LDA #22
    STA @VGA_DAC_DATA

    ; 11: teal
    LDA #5
    STA @VGA_DAC_DATA
    LDA #50
    STA @VGA_DAC_DATA
    LDA #46
    STA @VGA_DAC_DATA

    ; 12: cyan
    LDA #10
    STA @VGA_DAC_DATA
    LDA #58
    STA @VGA_DAC_DATA
    LDA #63
    STA @VGA_DAC_DATA

    ; 13: sky blue
    LDA #24
    STA @VGA_DAC_DATA
    LDA #42
    STA @VGA_DAC_DATA
    LDA #63
    STA @VGA_DAC_DATA

    ; 14: pale blue
    LDA #42
    STA @VGA_DAC_DATA
    LDA #52
    STA @VGA_DAC_DATA
    LDA #63
    STA @VGA_DAC_DATA

    ; 15: white
    LDA #63
    STA @VGA_DAC_DATA
    STA @VGA_DAC_DATA
    STA @VGA_DAC_DATA

    RTS

; ============================================================================
; DRAW TUNNEL PLASMA
; ============================================================================
; The same colour field is rendered four times, once per bit plane.  Instead
; of tiled byte patterns, the field is based on distance from the screen
; centre.  Highlight passes add per-byte masks while ordinary colour writes
; stay solid and stable.
; ============================================================================
draw_tunnel:
    LDA #1
    STA @VAR_PLANEMASK
    LDA #0
    STA @VAR_PLANE_SHIFT

.plane_loop:
    LDA @VAR_PLANEMASK
    STA @VGA_SEQ_MAPMASK

    LDA #0
    STA @VAR_ROW
    LDF #VGA_VRAM

.row_loop:
    LDA F
    STA @VAR_ROW_BASE

    LDA #0
    STA @VAR_XBYTE

.x_loop:
    JSR tunnel_plane_pattern
    STA [F]

    ADD F, #1
    LDA @VAR_XBYTE
    ADD A, #1
    STA @VAR_XBYTE
    LDB #WIDTH_BYTES
    SUB B, A
    JNZ B, .x_loop

    LDA @VAR_ROW_BASE
    ADD A, #WIDTH_BYTES
    LDF A

    LDA @VAR_ROW
    ADD A, #1
    STA @VAR_ROW
    LDB #HEIGHT
    SUB B, A
    JNZ B, .row_loop

    LDA @VAR_PLANEMASK
    SHL A, #1
    STA @VAR_PLANEMASK
    LDA @VAR_PLANE_SHIFT
    ADD A, #1
    STA @VAR_PLANE_SHIFT
    LDB #4
    SUB B, A
    JNZ B, .plane_loop

    RTS

; ============================================================================
; TUNNEL PLANE PATTERN
; ============================================================================
; Returns the byte pattern to write for the currently selected plane.  The
; colour comes only from distance to the screen centre.  There are no tiled
; X/Y shade terms here: the image is a single centre-origin tunnel.
; ============================================================================
tunnel_plane_pattern:
    ; Distance from the 640x480 screen centre, using byte X as x*8.
    LDB @VAR_XBYTE
    SHL B, #3
    SUB B, #320
    JGE B, .dx_abs_done
    XOR B, #0xFFFFFFFF
    ADD B, #1
.dx_abs_done:
    LDC @VAR_ROW
    SUB C, #240
    JGE C, .dy_abs_done
    XOR C, #0xFFFFFFFF
    ADD C, #1
.dy_abs_done:
    ADD B, C

    ; Move the bands inward over time, then classify one 128-unit ring cycle.
    LDA B
    LDC @VAR_FRAME
    SHL C, #2
    ADD A, C
    AND A, #127
    LDB A

    SUB B, #4
    JLT B, .band_white
    ADD B, #4
    SUB B, #9
    JLT B, .band_cyan
    ADD B, #9
    SUB B, #15
    JLT B, .band_blue
    ADD B, #15
    SUB B, #24
    JLT B, .band_violet
    ADD B, #24
    SUB B, #36
    JLT B, .band_red
    ADD B, #36
    SUB B, #52
    JLT B, .band_amber
    LDA #0
    STA @VAR_COLOUR
    JMP .emit_colour

.band_white:
    LDA #15
    STA @VAR_COLOUR
    JMP .emit_colour

.band_cyan:
    LDA #12
    STA @VAR_COLOUR
    JMP .emit_colour

.band_blue:
    LDA #13
    STA @VAR_COLOUR
    JMP .emit_colour

.band_violet:
    LDA #3
    STA @VAR_COLOUR
    JMP .emit_colour

.band_red:
    LDA #5
    STA @VAR_COLOUR
    JMP .emit_colour

.band_amber:
    LDA #7
    STA @VAR_COLOUR
    JMP .emit_colour

.emit_colour:
    LDA @VAR_COLOUR
    LDB @VAR_PLANEMASK
    AND A, B
    JZ A, .plane_off
    LDA #0xFF
    RTS

.plane_off:
    LDA #0
    RTS
