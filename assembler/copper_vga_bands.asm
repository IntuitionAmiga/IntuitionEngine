; copper_vga_bands.asm - Per-Scanline VGA Palette Demo
;
; This demo clearly demonstrates copper per-scanline effects on VGA:
; - Fills the entire screen with a single color index (1)
; - Copper changes palette entry 1 at different scanlines
; - Result: horizontal color bands without CPU intervention
;
; This is the classic Amiga copper effect - more colors than palette allows!
;
; Assemble: ./bin/ie32asm assembler/copper_vga_bands.asm
; Run: ./bin/IntuitionEngine -ie32 assembler/copper_vga_bands.iex

.include "ie32.inc"

.equ WIDTH          320
.equ HEIGHT         200
.equ SCREEN_SIZE    64000

.org 0x1000

start:
    ; Enable IE VideoChip (required for copper)
    LDA #1
    STA @VIDEO_CTRL

    ; Enable VGA
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    ; Set Mode 13h (320x200x256)
    LDA #VGA_MODE_13H
    STA @VGA_MODE

    ; Fill entire screen with color index 1
    JSR fill_screen

    ; Setup and enable copper
    LDA #copper_list
    STA @COPPER_PTR
    LDA #1
    STA @COPPER_CTRL

main_loop:
    ; Just wait for vsync - copper does all the work
    JSR wait_vsync
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
; fill_screen - Fill entire screen with color index 1
; -----------------------------------------------------------------------------
fill_screen:
    LDX #VGA_VRAM
    LDY #SCREEN_SIZE

.fill_loop:
    LDA #1                  ; Use color index 1 everywhere
    STA [X]
    ADD X, #1
    SUB Y, #1
    JNZ Y, .fill_loop
    RTS

; =============================================================================
; COPPER LIST - Changes palette entry 1 at different scanlines
;
; Each section:
; 1. Switches to IE video (for WAIT instruction)
; 2. WAITs for specific scanline
; 3. Switches to VGA DAC
; 4. Sets palette entry 1 to a new color
;
; Result: Horizontal bands of different colors!
; =============================================================================

copper_list:
    ; -------------------------------------------------------------------------
    ; Scanline 0: RED
    ; -------------------------------------------------------------------------
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000                ; MOVE to DAC write index
    .word 1                         ; Palette entry 1
    .word 0x40010000                ; MOVE to DAC data (R)
    .word 63                        ; R = 63 (bright)
    .word 0x40010000                ; MOVE to DAC data (G)
    .word 0                         ; G = 0
    .word 0x40010000                ; MOVE to DAC data (B)
    .word 0                         ; B = 0

    ; -------------------------------------------------------------------------
    ; Scanline 25: ORANGE
    ; -------------------------------------------------------------------------
    .word 0x8003C000                ; SETBASE to VIDEO (for WAIT)
    .word 0x00019000                ; WAIT y=25
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000                ; MOVE to DAC write index
    .word 1
    .word 0x40010000                ; R
    .word 63
    .word 0x40010000                ; G
    .word 32
    .word 0x40010000                ; B
    .word 0

    ; -------------------------------------------------------------------------
    ; Scanline 50: YELLOW
    ; -------------------------------------------------------------------------
    .word 0x8003C000                ; SETBASE to VIDEO
    .word 0x00032000                ; WAIT y=50
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000
    .word 1
    .word 0x40010000                ; R
    .word 63
    .word 0x40010000                ; G
    .word 63
    .word 0x40010000                ; B
    .word 0

    ; -------------------------------------------------------------------------
    ; Scanline 75: GREEN
    ; -------------------------------------------------------------------------
    .word 0x8003C000                ; SETBASE to VIDEO
    .word 0x0004B000                ; WAIT y=75
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000
    .word 1
    .word 0x40010000                ; R
    .word 0
    .word 0x40010000                ; G
    .word 63
    .word 0x40010000                ; B
    .word 0

    ; -------------------------------------------------------------------------
    ; Scanline 100: CYAN
    ; -------------------------------------------------------------------------
    .word 0x8003C000                ; SETBASE to VIDEO
    .word 0x00064000                ; WAIT y=100
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000
    .word 1
    .word 0x40010000                ; R
    .word 0
    .word 0x40010000                ; G
    .word 63
    .word 0x40010000                ; B
    .word 63

    ; -------------------------------------------------------------------------
    ; Scanline 125: BLUE
    ; -------------------------------------------------------------------------
    .word 0x8003C000                ; SETBASE to VIDEO
    .word 0x0007D000                ; WAIT y=125
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000
    .word 1
    .word 0x40010000                ; R
    .word 0
    .word 0x40010000                ; G
    .word 0
    .word 0x40010000                ; B
    .word 63

    ; -------------------------------------------------------------------------
    ; Scanline 150: MAGENTA
    ; -------------------------------------------------------------------------
    .word 0x8003C000                ; SETBASE to VIDEO
    .word 0x00096000                ; WAIT y=150
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000
    .word 1
    .word 0x40010000                ; R
    .word 63
    .word 0x40010000                ; G
    .word 0
    .word 0x40010000                ; B
    .word 63

    ; -------------------------------------------------------------------------
    ; Scanline 175: WHITE
    ; -------------------------------------------------------------------------
    .word 0x8003C000                ; SETBASE to VIDEO
    .word 0x000AF000                ; WAIT y=175
    .word 0x8003C416                ; SETBASE to VGA_DAC_WINDEX
    .word 0x40000000
    .word 1
    .word 0x40010000                ; R
    .word 63
    .word 0x40010000                ; G
    .word 63
    .word 0x40010000                ; B
    .word 63

    ; -------------------------------------------------------------------------
    ; End copper list
    ; -------------------------------------------------------------------------
    .word 0xC0000000                ; COP_END
