; vga_mode12h_bars.asm - VGA Mode 12h Color Bars Demo (IE32)
; 640x480 16-color planar graphics
;
; Assemble: sdk/bin/ie32asm assembler/vga_mode12h_bars.asm
; Run: ./bin/IntuitionEngine -ie32 assembler/vga_mode12h_bars.iex

; Include VGA definitions
.include "ie32.inc"

; Screen dimensions for Mode 12h
.equ WIDTH          640
.equ HEIGHT         480
.equ BYTES_PER_LINE 80      ; 640/8 = 80 bytes per line

; Variables
.equ VAR_FRAME      0x8800
.equ VAR_LINE_Y     0x8804

.org 0x1000

start:
    ; Enable VGA
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    ; Set Mode 12h (640x480x16 planar)
    LDA #VGA_MODE_12H
    STA @VGA_MODE

    ; Initialize frame counter
    LDA #0
    STA @VAR_FRAME

main_loop:
    ; Wait for vsync
    JSR wait_vsync

    ; Draw color bars
    JSR draw_bars

    ; Draw animated diagonal line
    JSR draw_line

    ; Increment frame
    LDA @VAR_FRAME
    ADD A, #1
    STA @VAR_FRAME

    JMP main_loop

; -----------------------------------------------------------------------------
; wait_vsync - Wait for vertical sync
; -----------------------------------------------------------------------------
wait_vsync:
.wait:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JZ A, .wait
    RTS

; -----------------------------------------------------------------------------
; draw_bars - Draw 16 vertical color bars
; -----------------------------------------------------------------------------
draw_bars:
    LDY #0              ; Current color (0-15)

.color_loop:
    ; Calculate X range for this color bar
    ; Each bar is 40 pixels wide (640/16 = 40)
    LDA Y
    MUL A, #40
    LDX A               ; X start = color * 40

    ; Draw all rows for this bar
    LDT #0              ; Row counter

.row_loop:
    ; Set plane write mask based on color bits
    ; Color bit 0 -> plane 0, etc.
    LDA Y
    AND A, #0x0F
    STA @VGA_SEQ_MAPMASK

    ; Calculate VRAM address = row * 80 + (x / 8)
    LDA T
    MUL A, #BYTES_PER_LINE
    LDB X
    SHR B, #3           ; X / 8
    ADD A, B
    ADD A, #VGA_VRAM
    LDF A               ; F = VRAM address

    ; Calculate bit pattern for 40-pixel wide bar
    ; Simplified: write 5 bytes (40 pixels = 5 bytes of 0xFF)
    LDA #0xFF
    LDC #5              ; 5 bytes

.byte_loop:
    STA [F]
    ADD F, #1
    SUB C, #1
    JNZ C, .byte_loop

    ; Next row
    ADD T, #1
    LDA #HEIGHT
    SUB A, T
    JNZ A, .row_loop

    ; Next color
    ADD Y, #1
    LDA #16
    SUB A, Y
    JNZ A, .color_loop

    RTS

; -----------------------------------------------------------------------------
; draw_line - Draw animated diagonal line
; -----------------------------------------------------------------------------
draw_line:
    ; Enable all planes for white line
    LDA #0x0F
    STA @VGA_SEQ_MAPMASK

    ; Calculate line position based on frame
    LDA @VAR_FRAME
    AND A, #0xFF        ; Wrap at 256

    ; Store as Y offset
    STA @VAR_LINE_Y

    ; Draw diagonal line from (0,offset) to (479,offset+479)
    LDX #0              ; X position (0-479)

.line_loop:
    ; Y = (frame + X) mod 480
    LDA @VAR_LINE_Y
    ADD A, X
    LDB #480
.mod_loop:
    SUB A, B
    AND A, #0x80000000
    JZ A, .mod_loop
    ADD A, B            ; Restore after underflow

    ; Calculate VRAM address = Y * 80 + X/8
    MUL A, #BYTES_PER_LINE
    LDB X
    SHR B, #3
    ADD A, B
    ADD A, #VGA_VRAM
    LDF A

    ; Calculate bit mask for pixel position
    LDA X
    AND A, #7           ; X mod 8
    LDB #0x80
    ; Shift right by (X mod 8)
    LDC A
.shift_loop:
    JZ C, .shift_done
    SHR B, #1
    SUB C, #1
    JMP .shift_loop
.shift_done:

    ; Set bit in VRAM (OR operation)
    LDA [F]
    OR A, B
    STA [F]

    ; Next pixel
    ADD X, #1
    LDA #480
    SUB A, X
    JNZ A, .line_loop

    RTS
