; ============================================================================
; VGA TEXT MODE "HELLO WORLD" - SIMPLEST POSSIBLE IE32 DEMO
; IE32 Assembly for IntuitionEngine - VGA Mode 03h (80x25 Text)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    VGA Mode 03h (80x25 text, 16 foreground + 8 background colors)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         ./bin/ie32asm sdk/examples/asm/vga_text_hello.asm
; Run:           ./bin/IntuitionEngine -ie32 vga_text_hello.iex
; Porting:       VGA MMIO is CPU-agnostic. Any CPU core can drive VGA text mode
;                by writing to the same register addresses.
;
; === WHAT THIS DEMO DOES ===
; Displays colored text on an 80x25 VGA text mode screen using IBM PC-style
; character/attribute pairs. This is the simplest possible graphical demo and
; a good starting point for learning IE32 assembly.
;
; === VGA TEXT MODE EXPLAINED ===
; VGA text mode uses a 4000-byte buffer at 0xB8000 (same as IBM PC). Each
; character cell is 2 bytes: [ASCII char] [attribute]. The attribute byte
; encodes foreground color (bits 0-3) and background color (bits 4-6).
; ============================================================================

; Include VGA definitions
.include "ie32.inc"

; Text mode dimensions
.equ TEXT_COLS      80
.equ TEXT_ROWS      25
.equ TEXT_BUFFER    0xB8000     ; VGA text buffer address

; Attribute colors (foreground | (background << 4))
.equ ATTR_WHITE     0x0F        ; White on black
.equ ATTR_YELLOW    0x0E        ; Yellow on black
.equ ATTR_CYAN      0x0B        ; Cyan on black
.equ ATTR_GREEN     0x0A        ; Green on black
.equ ATTR_RED       0x0C        ; Red on black
.equ ATTR_BLUE_BG   0x1F        ; White on blue

; Variables
.equ VAR_CURSOR_X   0x8800
.equ VAR_CURSOR_Y   0x8804
.equ VAR_ATTR       0x8808
.equ VAR_FRAME      0x880C

.org 0x1000

start:
    ; Enable VGA
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    ; Set text mode (80x25)
    LDA #VGA_MODE_TEXT
    STA @VGA_MODE

    ; Initialize frame counter
    LDA #0
    STA @VAR_FRAME

    ; Clear screen with blue background
    JSR clear_screen

    ; Draw title bar
    JSR draw_title

    ; Draw color palette display
    JSR draw_palette

    ; Draw message
    JSR draw_message

main_loop:
    ; Wait for vsync
    JSR wait_vsync

    ; Animate rainbow text
    JSR animate_rainbow

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
; clear_screen - Fill screen with spaces on blue background
; -----------------------------------------------------------------------------
clear_screen:
    LDX #TEXT_BUFFER
    LDY #2000           ; 80 * 25 = 2000 characters

.clr:
    LDA #0x20           ; Space character
    STA [X]
    ADD X, #1
    LDA #ATTR_BLUE_BG   ; White on blue
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .clr
    RTS

; -----------------------------------------------------------------------------
; draw_title - Draw title at top of screen
; -----------------------------------------------------------------------------
draw_title:
    ; Position at row 1, column 25
    LDA #1
    STA @VAR_CURSOR_Y
    LDA #25
    STA @VAR_CURSOR_X
    LDA #ATTR_YELLOW
    STA @VAR_ATTR

    ; Draw title string
    LDX #title_str
    JSR print_string
    RTS

; -----------------------------------------------------------------------------
; draw_palette - Show all 16 colors
; -----------------------------------------------------------------------------
draw_palette:
    ; Position at row 5
    LDA #5
    STA @VAR_CURSOR_Y
    LDA #10
    STA @VAR_CURSOR_X

    LDY #0              ; Color counter

.pal_loop:
    ; Set attribute: color Y as foreground on black
    LDA Y
    STA @VAR_ATTR

    ; Print block character
    LDA #0xDB           ; Full block
    JSR print_char
    LDA #0xDB
    JSR print_char

    ; Print space
    LDA #ATTR_WHITE
    STA @VAR_ATTR
    LDA #0x20
    JSR print_char

    ADD Y, #1
    LDA #16
    SUB A, Y
    JNZ A, .pal_loop

    RTS

; -----------------------------------------------------------------------------
; draw_message - Draw welcome message
; -----------------------------------------------------------------------------
draw_message:
    ; Line 1
    LDA #10
    STA @VAR_CURSOR_Y
    LDA #20
    STA @VAR_CURSOR_X
    LDA #ATTR_WHITE
    STA @VAR_ATTR
    LDX #msg1_str
    JSR print_string

    ; Line 2
    LDA #12
    STA @VAR_CURSOR_Y
    LDA #15
    STA @VAR_CURSOR_X
    LDA #ATTR_CYAN
    STA @VAR_ATTR
    LDX #msg2_str
    JSR print_string

    ; Line 3
    LDA #14
    STA @VAR_CURSOR_Y
    LDA #22
    STA @VAR_CURSOR_X
    LDA #ATTR_GREEN
    STA @VAR_ATTR
    LDX #msg3_str
    JSR print_string

    RTS

; -----------------------------------------------------------------------------
; animate_rainbow - Cycle colors on bottom row
; -----------------------------------------------------------------------------
animate_rainbow:
    ; Position at row 20
    LDA #20
    STA @VAR_CURSOR_Y
    LDA #0
    STA @VAR_CURSOR_X

    LDX #0              ; Column counter

.anim_loop:
    ; Calculate color based on column + frame
    LDA X
    ADD A, @VAR_FRAME
    AND A, #0x0F        ; Wrap at 16
    ADD A, #1           ; Avoid black (color 0)
    STA @VAR_ATTR

    ; Calculate screen position
    LDA @VAR_CURSOR_Y
    MUL A, #TEXT_COLS
    ADD A, @VAR_CURSOR_X
    MUL A, #2           ; 2 bytes per character
    ADD A, #TEXT_BUFFER
    LDF A

    ; Write character and attribute
    LDA #0xCD           ; Double line horizontal
    STA [F]
    ADD F, #1
    LDA @VAR_ATTR
    STA [F]

    ; Next column
    LDA @VAR_CURSOR_X
    ADD A, #1
    STA @VAR_CURSOR_X
    ADD X, #1
    LDA #TEXT_COLS
    SUB A, X
    JNZ A, .anim_loop

    RTS

; -----------------------------------------------------------------------------
; print_char - Print character at cursor position
; Input: A = character
; -----------------------------------------------------------------------------
print_char:
    PUSH B
    PUSH F
    LDB A               ; Save character in B

    ; Calculate screen position
    LDA @VAR_CURSOR_Y
    MUL A, #TEXT_COLS
    ADD A, @VAR_CURSOR_X
    MUL A, #2           ; 2 bytes per character
    ADD A, #TEXT_BUFFER
    LDF A

    ; Write character
    LDA B
    STA [F]

    ; Write attribute
    ADD F, #1
    LDA @VAR_ATTR
    STA [F]

    ; Advance cursor
    LDA @VAR_CURSOR_X
    ADD A, #1
    STA @VAR_CURSOR_X

    POP F
    POP B
    RTS

; -----------------------------------------------------------------------------
; print_string - Print null-terminated string
; Input: X = string address
; -----------------------------------------------------------------------------
print_string:
    PUSH B
.ps_loop:
    LDA [X]
    AND A, #0xFF
    JZ A, .ps_done
    JSR print_char
    ADD X, #1
    JMP .ps_loop
.ps_done:
    POP B
    RTS

; -----------------------------------------------------------------------------
; String Data
; -----------------------------------------------------------------------------
title_str:
.ascii "VGA TEXT MODE DEMO"
.byte 0

msg1_str:
.ascii "Welcome to Intuition Engine!"
.byte 0

msg2_str:
.ascii "VGA Mode 3 - 80x25 Text with 16 Colors"
.byte 0

msg3_str:
.ascii "IE32 Assembly Language"
.byte 0
