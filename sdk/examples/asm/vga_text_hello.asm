; ============================================================================
; VGA TEXT MODE "HELLO WORLD" - SIMPLEST POSSIBLE IE32 DEMO
; IE32 Assembly for IntuitionEngine - VGA Mode 03h (80x25 Text)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    VGA Mode 03h (80x25 text, 16 foreground + 8 background colours)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm sdk/examples/asm/vga_text_hello.asm
; Run:           ./bin/IntuitionEngine -ie32 vga_text_hello.iex
; Porting:       VGA MMIO is CPU-agnostic. Any CPU core can drive VGA text mode
;                by writing to the same register addresses.
;
; === WHAT THIS DEMO DOES ===
; Displays coloured text on an 80x25 VGA text mode screen using IBM PC-style
; character/attribute pairs.  A title banner, a 16-colour palette display, a
; welcome message, and an animated rainbow bar that cycles across row 20.
; This is the simplest possible graphical demo and a good starting point for
; learning IE32 assembly.
;
; === WHY VGA TEXT MODE (MODE 03h) ===
; VGA text mode dates back to the original IBM PC's CGA adapter (1981) and
; has remained essentially unchanged through EGA, VGA, and even modern BIOS
; implementations.  It is a character-based display, typically 80 columns by
; 25 rows, where each cell is stored as two bytes in a 4000-byte buffer at
; physical address 0xB8000.
;
; The first byte is the ASCII character code (0-255, using the CP437 character
; set on IBM PCs).  The second byte is the attribute, which encodes the
; foreground colour in bits 0-3 (16 colours) and the background colour in
; bits 4-6 (8 colours, or 16 if blink is disabled).  Bit 7 controls blinking
; or serves as a high-intensity background bit.
;
; This two-byte-per-cell layout made text mode extremely memory-efficient --
; a full 80x25 screen used just 4000 bytes, crucial in the era of 16KB-64KB
; video RAM.  Despite its simplicity, creative programmers used text mode for
; everything from Norton Commander's iconic blue panels to ANSI art bulletin
; boards to demoscene text-mode competitions.
;
; === MEMORY MAP ===
;   0x1000          Program code entry point
;   0x8800          VAR_CURSOR_X - current cursor column
;   0x8804          VAR_CURSOR_Y - current cursor row
;   0x8808          VAR_ATTR - current character attribute byte
;   0x880C          VAR_FRAME - animation frame counter
;   0xB8000         VGA text buffer (4000 bytes, 80x25 x 2 bytes per cell)
;
; === BUILD AND RUN ===
;   sdk/bin/ie32asm sdk/examples/asm/vga_text_hello.asm
;   ./bin/IntuitionEngine -ie32 vga_text_hello.iex
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie32.inc"

; ---------------------------------------------------------------------------
; Text mode geometry and buffer address
; ---------------------------------------------------------------------------
.equ TEXT_COLS      80
.equ TEXT_ROWS      25
.equ TEXT_BUFFER    0xB8000

; ---------------------------------------------------------------------------
; Attribute byte constants
; ---------------------------------------------------------------------------
; Format: foreground (bits 0-3) | background (bits 4-6) << 4
; Colour indices follow the standard CGA/VGA palette:
;   0=Black  1=Blue  2=Green  3=Cyan  4=Red  5=Magenta  6=Brown  7=LightGrey
;   8=DarkGrey  9=LightBlue  10=LightGreen  11=LightCyan  12=LightRed
;   13=LightMagenta  14=Yellow  15=White
.equ ATTR_WHITE     0x0F
.equ ATTR_YELLOW    0x0E
.equ ATTR_CYAN      0x0B
.equ ATTR_GREEN     0x0A
.equ ATTR_RED       0x0C
.equ ATTR_BLUE_BG   0x1F

; ---------------------------------------------------------------------------
; Variables in scratch RAM
; ---------------------------------------------------------------------------
.equ VAR_CURSOR_X   0x8800
.equ VAR_CURSOR_Y   0x8804
.equ VAR_ATTR       0x8808
.equ VAR_FRAME      0x880C

.org 0x1000

; ============================================================================
; ENTRY POINT
; ============================================================================
; Initialise VGA in text mode, draw the static screen elements (title bar,
; colour palette, welcome message), then enter the animation loop.
; ============================================================================
start:
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    LDA #VGA_MODE_TEXT
    STA @VGA_MODE

    LDA #0
    STA @VAR_FRAME

    JSR clear_screen

    JSR draw_title

    JSR draw_palette

    JSR draw_message

; ============================================================================
; MAIN LOOP
; ============================================================================
; Each frame: synchronise to vertical blank, then update the rainbow
; animation on row 20.  The static elements (title, palette, message)
; are drawn once at startup and persist in the text buffer.
; ============================================================================
main_loop:
    JSR wait_vsync

    JSR animate_rainbow

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
; CLEAR SCREEN
; ============================================================================
; Fills all 2000 character cells with spaces on a blue background.  Each
; cell gets two writes: 0x20 (space) for the character byte and 0x1F
; (white-on-blue) for the attribute byte.
; ============================================================================
clear_screen:
    LDX #TEXT_BUFFER
    LDY #2000

.clr:
    LDA #0x20
    STA [X]
    ADD X, #1
    LDA #ATTR_BLUE_BG
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .clr
    RTS

; ============================================================================
; DRAW TITLE BAR
; ============================================================================
; Positions the cursor at row 1, column 25 and prints the title string
; in yellow on the blue background.
; ============================================================================
draw_title:
    LDA #1
    STA @VAR_CURSOR_Y
    LDA #25
    STA @VAR_CURSOR_X
    LDA #ATTR_YELLOW
    STA @VAR_ATTR

    LDX #title_str
    JSR print_string
    RTS

; ============================================================================
; DRAW 16-COLOUR PALETTE DISPLAY
; ============================================================================
; Shows all 16 CGA/VGA foreground colours as pairs of full-block characters
; (CP437 character 0xDB) separated by spaces.  This demonstrates the
; complete text-mode colour range available via the attribute byte.
; ============================================================================
draw_palette:
    LDA #5
    STA @VAR_CURSOR_Y
    LDA #10
    STA @VAR_CURSOR_X

    LDY #0

.pal_loop:
    ; Set foreground colour to the current palette index
    LDA Y
    STA @VAR_ATTR

    ; Print two full-block characters to form a visible colour swatch
    LDA #0xDB
    JSR print_char
    LDA #0xDB
    JSR print_char

    ; Print a white space separator between swatches
    LDA #ATTR_WHITE
    STA @VAR_ATTR
    LDA #0x20
    JSR print_char

    ADD Y, #1
    LDA #16
    SUB A, Y
    JNZ A, .pal_loop

    RTS

; ============================================================================
; DRAW WELCOME MESSAGE
; ============================================================================
; Three lines of text at different vertical positions, each in a distinct
; colour, providing a visual hierarchy typical of DOS-era "about" screens.
; ============================================================================
draw_message:
    ; Line 1: white
    LDA #10
    STA @VAR_CURSOR_Y
    LDA #20
    STA @VAR_CURSOR_X
    LDA #ATTR_WHITE
    STA @VAR_ATTR
    LDX #msg1_str
    JSR print_string

    ; Line 2: cyan
    LDA #12
    STA @VAR_CURSOR_Y
    LDA #15
    STA @VAR_CURSOR_X
    LDA #ATTR_CYAN
    STA @VAR_ATTR
    LDX #msg2_str
    JSR print_string

    ; Line 3: green
    LDA #14
    STA @VAR_CURSOR_Y
    LDA #22
    STA @VAR_CURSOR_X
    LDA #ATTR_GREEN
    STA @VAR_ATTR
    LDX #msg3_str
    JSR print_string

    RTS

; ============================================================================
; ANIMATE RAINBOW BAR
; ============================================================================
; Fills row 20 with double-line horizontal characters (CP437 0xCD), each
; coloured with a different foreground that cycles based on column + frame
; counter.  This creates a sweeping rainbow animation across the full width
; of the screen -- a simple but visually striking effect.
;
; === WHY THIS WORKS ===
; By adding the frame counter to each column's index before masking to
; the 16-colour range, the entire colour pattern shifts left by one palette
; entry each frame.  At 60 FPS this produces a smooth, continuous motion.
; ============================================================================
animate_rainbow:
    LDA #20
    STA @VAR_CURSOR_Y
    LDA #0
    STA @VAR_CURSOR_X

    LDX #0

.anim_loop:
    ; Calculate cycling colour: (column + frame) mod 16, avoiding black (0)
    LDA X
    ADD A, @VAR_FRAME
    AND A, #0x0F
    ADD A, #1
    STA @VAR_ATTR

    ; Calculate the screen buffer address for this cell directly
    LDA @VAR_CURSOR_Y
    MUL A, #TEXT_COLS
    ADD A, @VAR_CURSOR_X
    MUL A, #2
    ADD A, #TEXT_BUFFER
    LDF A

    ; Write the character and attribute bytes
    LDA #0xCD
    STA [F]
    ADD F, #1
    LDA @VAR_ATTR
    STA [F]

    ; Advance to the next column
    LDA @VAR_CURSOR_X
    ADD A, #1
    STA @VAR_CURSOR_X
    ADD X, #1
    LDA #TEXT_COLS
    SUB A, X
    JNZ A, .anim_loop

    RTS

; ============================================================================
; PRINT A SINGLE CHARACTER AT THE CURSOR POSITION
; ============================================================================
; Input: A = ASCII character code
;
; Calculates the text buffer address from VAR_CURSOR_X/Y, writes the
; character and its attribute byte, then advances the cursor one column
; to the right.
; ============================================================================
print_char:
    PUSH B
    PUSH F
    LDB A

    LDA @VAR_CURSOR_Y
    MUL A, #TEXT_COLS
    ADD A, @VAR_CURSOR_X
    MUL A, #2
    ADD A, #TEXT_BUFFER
    LDF A

    LDA B
    STA [F]

    ADD F, #1
    LDA @VAR_ATTR
    STA [F]

    LDA @VAR_CURSOR_X
    ADD A, #1
    STA @VAR_CURSOR_X

    POP F
    POP B
    RTS

; ============================================================================
; PRINT NULL-TERMINATED STRING
; ============================================================================
; Input: X = address of null-terminated string
;
; Reads bytes from the string one at a time, printing each via print_char
; until a zero byte (null terminator) is encountered.
; ============================================================================
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

; ============================================================================
; STRING DATA
; ============================================================================
title_str:
.ascii "VGA TEXT MODE DEMO"
.byte 0

msg1_str:
.ascii "Welcome to Intuition Engine!"
.byte 0

msg2_str:
.ascii "VGA Mode 3 - 80x25 Text with 16 Colours"
.byte 0

msg3_str:
.ascii "IE32 Assembly Language"
.byte 0
