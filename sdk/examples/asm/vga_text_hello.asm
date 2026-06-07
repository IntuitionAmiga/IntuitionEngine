; ============================================================================
; VGA TEXT MODE "HELLO" - IE32 TEXT-MODE PLASMA DEMO
; IE32 Assembly for IntuitionEngine - VGA Mode 03h (80x25 Text)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    VGA Mode 03h (80x25 text, 16 foreground + 8 background colours)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm -I sdk/include sdk/examples/asm/vga_text_hello.asm
; Run:           ./bin/IntuitionEngine -ie32 sdk/examples/prebuilt/vga_text_hello.iex
;
; === WHAT THIS DEMO DOES ===
; Shows that classic VGA text mode can still do more than plain "hello world":
; the whole 80x25 character buffer is redrawn every frame as an animated
; shade-field, then a title, palette strip and scroller are overlaid.  The
; frame loop waits for a full vertical blank edge so animation speed stays
; stable after the current VGA timing changes.
;
; === PERFORMANCE NOTES ===
; The original version recalculated row*80+column and used cursor variables for
; almost every character.  This version streams linearly through 0xB8000, uses
; pointer increments, and only masks small phase counters in the hot loop.  VGA
; text memory still accepts one byte per addressed cell byte, so each character
; cell is written as two low-byte MMIO writes: character then attribute.
;
; === MEMORY MAP ===
;   0x1000          Program code entry point
;   0x8800          VAR_FRAME - animation frame counter
;   0xB8000         VGA text buffer (4000 bytes, 80x25 x 2 bytes per cell)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie32.inc"

; ---------------------------------------------------------------------------
; Text mode geometry and buffer address
; ---------------------------------------------------------------------------
.equ TEXT_COLS      80
.equ TEXT_ROWS      25
.equ TEXT_STRIDE    160
.equ TEXT_BUFFER    0xB8000

; ---------------------------------------------------------------------------
; Attribute byte constants
; ---------------------------------------------------------------------------
.equ ATTR_WHITE     0x0F
.equ ATTR_YELLOW    0x0E
.equ ATTR_CYAN      0x0B
.equ ATTR_GREEN     0x0A
.equ ATTR_MAGENTA   0x0D
.equ ATTR_BLUE_BG   0x1F
.equ ATTR_RED_BG    0x4E

; ---------------------------------------------------------------------------
; Variables in scratch RAM
; ---------------------------------------------------------------------------
.equ VAR_FRAME      0x8800

.org 0x1000

; ============================================================================
; ENTRY POINT
; ============================================================================
start:
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    LDA #VGA_MODE_TEXT
    STA @VGA_MODE

    LDA #0
    STA @VAR_FRAME

; ============================================================================
; MAIN LOOP
; ============================================================================
main_loop:
    JSR wait_frame

    JSR draw_plasma
    JSR draw_static_overlay
    JSR draw_scroller

    LDA @VAR_FRAME
    ADD A, #1
    STA @VAR_FRAME

    JMP main_loop

; ============================================================================
; WAIT FOR A VBLANK EDGE
; ============================================================================
; Waiting only for "vblank is set" can return multiple times inside the same
; blanking interval.  This waits for the current blank to finish, then waits
; for the next one to begin.
; ============================================================================
wait_frame:
.wait_low:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JNZ A, .wait_low

.wait_high:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JZ A, .wait_high
    RTS

; ============================================================================
; DRAW ANIMATED TEXT-MODE PLASMA
; ============================================================================
; X = text buffer pointer
; Y = row counter
; Z = column counter
; B = row phase
; ============================================================================
draw_plasma:
    LDX #TEXT_BUFFER
    LDY #0
    LDB #0

.row_loop:
    LDZ #0

.col_loop:
    ; Character phase: (column + row_phase - frame) & 7.
    LDA Z
    ADD A, B
    SUB A, @VAR_FRAME
    AND A, #0x07
    LDF #shade_chars
    ADD F, A
    LDA [F]
    AND A, #0xFF
    STA [X]
    ADD X, #1

    ; Colour phase: keep foreground colours in the bright 1-15 range.
    LDA Z
    ADD A, B
    SUB A, @VAR_FRAME
    AND A, #0x0F
    ADD A, #1
    STA [X]
    ADD X, #1

    ADD Z, #1
    LDA #TEXT_COLS
    SUB A, Z
    JNZ A, .col_loop

    ADD B, #3
    AND B, #0x0F
    ADD Y, #1
    LDA #TEXT_ROWS
    SUB A, Y
    JNZ A, .row_loop

    RTS

; ============================================================================
; DRAW STATIC OVERLAY
; ============================================================================
draw_static_overlay:
    ; Top and bottom raster bars.
    LDX #TEXT_BUFFER
    LDA #0xDF
    LDB #ATTR_RED_BG
    LDY #TEXT_COLS
    JSR fill_cells

    LDX #(TEXT_BUFFER+(24*TEXT_STRIDE))
    LDA #0xDC
    LDB #ATTR_BLUE_BG
    LDY #TEXT_COLS
    JSR fill_cells

    ; Title and labels.
    LDX #title_str
    LDF #(TEXT_BUFFER+(2*TEXT_STRIDE)+(17*2))
    LDB #ATTR_YELLOW
    JSR print_string_at

    LDX #subtitle_str
    LDF #(TEXT_BUFFER+(4*TEXT_STRIDE)+(12*2))
    LDB #ATTR_CYAN
    JSR print_string_at

    LDX #palette_label
    LDF #(TEXT_BUFFER+(7*TEXT_STRIDE)+(13*2))
    LDB #ATTR_WHITE
    JSR print_string_at

    JSR draw_palette

    LDX #msg1_str
    LDF #(TEXT_BUFFER+(12*TEXT_STRIDE)+(16*2))
    LDB #ATTR_GREEN
    JSR print_string_at

    LDX #msg2_str
    LDF #(TEXT_BUFFER+(14*TEXT_STRIDE)+(10*2))
    LDB #ATTR_MAGENTA
    JSR print_string_at

    RTS

; ============================================================================
; DRAW 16-COLOUR PALETTE STRIP
; ============================================================================
draw_palette:
    LDF #(TEXT_BUFFER+(8*TEXT_STRIDE)+(16*2))
    LDY #1

.pal_loop:
    LDA #0xDB
    STA [F]
    ADD F, #1
    LDA Y
    STA [F]
    ADD F, #1

    LDA #0xDB
    STA [F]
    ADD F, #1
    LDA Y
    STA [F]
    ADD F, #1

    LDA #0x20
    STA [F]
    ADD F, #1
    LDA #ATTR_WHITE
    STA [F]
    ADD F, #1

    ADD Y, #1
    LDA #16
    SUB A, Y
    JNZ A, .pal_loop
    RTS

; ============================================================================
; DRAW SCROLLER
; ============================================================================
; A simple character scroller rendered across row 21.  The source index is
; frame/16.  This keeps the scroll readable while avoiding the crawl of
; frame/32.  The plasma still runs at full frame rate.
; ============================================================================
draw_scroller:
    LDF #(TEXT_BUFFER+(21*TEXT_STRIDE))
    LDZ #0

.scroll_loop:
    LDA @VAR_FRAME
    SHR A, #4
    ADD A, Z
    MOD A, #scroll_len
    LDX #scroll_text
    ADD X, A
    LDA [X]
    AND A, #0xFF
    STA [F]
    ADD F, #1

    LDA Z
    ADD A, @VAR_FRAME
    AND A, #0x0F
    ADD A, #1
    STA [F]
    ADD F, #1

    ADD Z, #1
    LDA #TEXT_COLS
    SUB A, Z
    JNZ A, .scroll_loop

    RTS

; ============================================================================
; FILL CELLS
; ============================================================================
; Input: X = destination, A = character, B = attribute, Y = cell count.
; ============================================================================
fill_cells:
    PUSH C
    LDC A

.fill_loop:
    LDA C
    STA [X]
    ADD X, #1
    LDA B
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .fill_loop

    POP C
    RTS

; ============================================================================
; PRINT NULL-TERMINATED STRING AT ADDRESS
; ============================================================================
; Input: X = string, F = destination cell byte, B = attribute.
; ============================================================================
print_string_at:
    PUSH C

.ps_loop:
    LDA [X]
    AND A, #0xFF
    JZ A, .ps_done

    STA [F]
    ADD F, #1
    LDA B
    STA [F]
    ADD F, #1
    ADD X, #1
    JMP .ps_loop

.ps_done:
    POP C
    RTS

; ============================================================================
; DATA
; ============================================================================
shade_chars:
.byte 0x20, 0xB0, 0xB1, 0xB2, 0xDB, 0xB2, 0xB1, 0xB0

title_str:
.ascii "INTUITION ENGINE VGA TEXTMODE"
.byte 0

subtitle_str:
.ascii "80x25 CP437 PLASMA - BYTE MMIO - VBLANK LOCKED"
.byte 0

palette_label:
.ascii "CGA/VGA 16 COLOUR FOREGROUND STRIP"
.byte 0

msg1_str:
.ascii "NO BITMAP MODE, NO BLITTER, JUST TEXT CELLS"
.byte 0

msg2_str:
.ascii "LINEAR WRITES BEAT CURSOR MATH.  STILL MODE 03H."
.byte 0

scroll_text:
.ascii "   HELLO FROM VGA MODE 03H - NOW WITH A PROPER VSYNC EDGE WAIT, STREAMED TEXT MEMORY, RAINBOW ATTRIBUTES AND A SLIGHTLY LESS EMBARRASSING TEXT-MODE PLASMA   "
scroll_end:
.equ scroll_len scroll_end-scroll_text
