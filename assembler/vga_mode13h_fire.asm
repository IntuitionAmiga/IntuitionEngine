; vga_mode13h_fire.asm - VGA Mode 13h Fire Effect Demo (IE32)
; Classic DOS-style 256-color fire effect
;
; Assemble: ./bin/ie32asm assembler/vga_mode13h_fire.asm
; Run: ./bin/IntuitionEngine -ie32 assembler/vga_mode13h_fire.iex

; Include VGA definitions
.include "ie32.inc"

; Screen dimensions for Mode 13h
.equ WIDTH          320
.equ HEIGHT         200
.equ SCREEN_SIZE    64000

; Variables
.equ VAR_SEED       0x8800

.org 0x1000

start:
    ; Enable VGA (standalone video device - no VideoChip required)
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    ; Set Mode 13h (320x200x256)
    LDA #VGA_MODE_13H
    STA @VGA_MODE

    ; Initialize random seed
    LDA #12345678
    STA @VAR_SEED

    ; Setup fire palette
    JSR setup_palette

    ; Clear VRAM
    JSR clear_vram

main_loop:
    ; Wait for vsync
    JSR wait_vsync

    ; Generate fire source (random bottom row)
    JSR fire_source

    ; Propagate fire upward
    JSR fire_propagate

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
; clear_vram - Clear VRAM to black
; -----------------------------------------------------------------------------
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

; -----------------------------------------------------------------------------
; setup_palette - Create fire color gradient (black->red->yellow->white)
; -----------------------------------------------------------------------------
setup_palette:
    LDX #0              ; Palette index

.pal_loop:
    ; Set write index
    STA @VGA_DAC_WINDEX
    LDA X
    STA @VGA_DAC_WINDEX

    ; Calculate R (ramps up first, max at 63)
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
    STA @VGA_DAC_DATA   ; R

    ; Calculate G (ramps up after index 16)
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
    STA @VGA_DAC_DATA   ; G

    ; Calculate B (ramps up after index 48)
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
    STA @VGA_DAC_DATA   ; B

    ADD X, #1
    LDA #256
    SUB A, X
    JNZ A, .pal_loop

    RTS

; -----------------------------------------------------------------------------
; fire_source - Generate random pixels on bottom row
; -----------------------------------------------------------------------------
fire_source:
    ; Point to bottom row
    LDX #VGA_VRAM
    LDA #HEIGHT
    SUB A, #1
    MUL A, #WIDTH
    ADD X, A

    LDY #WIDTH          ; Column counter

.src_loop:
    ; XOR-shift random number generator
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

    ; Use low byte as pixel value
    AND A, #0xFF

    ; Bias toward brighter values
    LDB A
    SUB B, #128
    AND B, #0x80000000
    JNZ B, .dim
    OR A, #0xC0         ; Make bright
.dim:
    STA [X]

    ADD X, #1
    SUB Y, #1
    JNZ Y, .src_loop

    RTS

; -----------------------------------------------------------------------------
; fire_propagate - Average pixels and move fire upward with decay
; -----------------------------------------------------------------------------
fire_propagate:
    ; Process from row HEIGHT-2 down to row 0 (bottom to top)
    ; This allows fire to propagate multiple rows per frame
    ; Each pixel = average of pixel below + left-below + right-below
    LDY #HEIGHT
    SUB Y, #2           ; Start at row 198 (HEIGHT-2)

.row_loop:
    ; Check if we've processed all rows (Y < 0)
    LDA Y
    AND A, #0x80000000
    JNZ A, .done

    LDX #0              ; Current column

.col_loop:
    ; Calculate address of current pixel
    LDA Y
    MUL A, #WIDTH
    ADD A, X
    ADD A, #VGA_VRAM
    LDF A               ; F = current pixel address

    ; Calculate address of pixel below
    LDA Y
    ADD A, #1
    MUL A, #WIDTH
    ADD A, X
    ADD A, #VGA_VRAM
    LDT A               ; T = pixel below address

    ; Sum neighbors from row below with proper averaging
    ; Start sum with center pixel below
    LDA [T]
    AND A, #0xFF
    LDB A               ; B = center value

    ; Add left neighbor (if not at left edge)
    LDA X
    JZ A, .no_left
    LDA T
    SUB A, #1
    LDC A
    LDA [C]
    AND A, #0xFF
    ADD B, A            ; B += left
.no_left:

    ; Add right neighbor (if not at right edge)
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
    ADD B, A            ; B += right
.no_right:

    ; Average and decay: value = (sum / 3) - 1
    ; But edge cases only have 2 samples, so we need simpler approach:
    ; Just use: value = sum / 4 (fast shift) gives good decay
    ; For 3 neighbors of ~200 each: sum=600, /4=150 (slow decay)
    LDA B
    SHR A, #2           ; A = sum / 4

    ; Small extra decay
    SUB A, #1

    ; Clamp to 0 if underflow
    LDB A
    AND B, #0x80000000
    JZ B, .no_clamp
    LDA #0
.no_clamp:

    ; Store result
    STA [F]

    ; Next column
    ADD X, #1
    LDA #WIDTH
    SUB A, X
    JNZ A, .col_loop

    ; Next row (going upward, so decrement)
    SUB Y, #1
    JMP .row_loop

.done:
    RTS
