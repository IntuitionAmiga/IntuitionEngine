; copper_vga_bands.asm - Per-Scanline VGA Palette Demo
;
; WHAT THIS DEMONSTRATES:
; This is a classic Amiga-style copper effect running on VGA hardware.
; The copper coprocessor changes a palette entry at different scanlines,
; creating the illusion of more colours than the hardware actually supports.
;
; HOW IT WORKS:
; 1. The entire screen is filled with a single colour index (1)
; 2. The copper list runs synchronized with the video beam
; 3. As the beam reaches each scanline threshold, the copper changes
;    what colour palette entry 1 actually looks like
; 4. Result: 8 different coloured horizontal bands, all using the same
;    palette index but appearing different due to mid-frame palette changes
;
; WHY?
; This technique was pioneered on the Amiga in the 1980s and allowed
; programmers to display far more colours than the hardware palette allowed.
; Here we demonstrate that the Intuition Engine's copper can control
; external devices (VGA) via the SETBASE opcode, not just the built-in
; video chip.
;
; ARCHITECTURE NOTE:
; The copper and VGA are completely separate chips that communicate only
; through the system bus. The copper uses SETBASE to redirect its MOVE
; writes to VGA DAC registers instead of the default IE video registers.
;
; Assemble: ./bin/ie32asm assembler/copper_vga_bands.asm
; Run: ./bin/IntuitionEngine -ie32 assembler/copper_vga_bands.iex

.include "ie32.inc"

; Screen dimensions for VGA Mode 13h (320x200, 256 colours)
.equ WIDTH          320
.equ HEIGHT         200
.equ SCREEN_SIZE    64000       ; WIDTH * HEIGHT bytes

; Pre-computed WAIT values for copper synchronization
; Format: (Y_position << 12) | X_position
; The copper compares these against the current beam position
; We use X=0 so the colour change happens at the start of each scanline
.equ COP_WAIT_Y25   0x00019000  ; Y=25:  25 * 0x1000 = 0x19000
.equ COP_WAIT_Y50   0x00032000  ; Y=50:  50 * 0x1000 = 0x32000
.equ COP_WAIT_Y75   0x0004B000  ; Y=75:  75 * 0x1000 = 0x4B000
.equ COP_WAIT_Y100  0x00064000  ; Y=100: 100 * 0x1000 = 0x64000
.equ COP_WAIT_Y125  0x0007D000  ; Y=125: 125 * 0x1000 = 0x7D000
.equ COP_WAIT_Y150  0x00096000  ; Y=150: 150 * 0x1000 = 0x96000
.equ COP_WAIT_Y175  0x000AF000  ; Y=175: 175 * 0x1000 = 0xAF000

.org 0x1000

start:
    ; ---------------------------------------------------------------------------
    ; INITIALIZATION
    ; We must enable BOTH the IE VideoChip (which contains the copper) AND
    ; the VGA chip (which we're controlling). The copper lives in VideoChip
    ; but will write to VGA registers via bus routing.
    ; ---------------------------------------------------------------------------

    ; Enable IE VideoChip - this activates the copper coprocessor
    ; Without this, the copper list would never execute
    LDA #1
    STA @VIDEO_CTRL

    ; Enable VGA output - this makes VGA visible in the compositor
    ; The VGA chip renders independently; we're just changing its palette
    LDA #VGA_CTRL_ENABLE
    STA @VGA_CTRL

    ; Set VGA to Mode 13h: 320x200 resolution, 256 colours, chunky framebuffer
    ; This is the classic "demoscene" mode - simple and fast
    LDA #VGA_MODE_13H
    STA @VGA_MODE

    ; Fill the screen with colour index 1
    ; We use a single colour so palette changes affect the ENTIRE display
    ; If we used multiple colours, only pixels using index 1 would change
    JSR fill_screen

    ; ---------------------------------------------------------------------------
    ; COPPER SETUP
    ; Point the copper at our instruction list and enable it
    ; The copper will automatically restart from this address each frame
    ; ---------------------------------------------------------------------------
    LDA #copper_list
    STA @COPPER_PTR

    ; Enable copper execution - it will start running on next frame
    LDA #1
    STA @COPPER_CTRL

; ---------------------------------------------------------------------------
; MAIN LOOP
; The CPU has nothing to do! The copper handles all the visual effects
; automatically, synchronized with the video beam. We just wait for vsync
; to keep the loop from spinning too fast.
; ---------------------------------------------------------------------------
main_loop:
    JSR wait_vsync
    JMP main_loop

; ---------------------------------------------------------------------------
; wait_vsync - Wait for vertical blanking interval
;
; We poll the VGA status register for the vsync flag. This ensures we
; don't waste CPU cycles and provides a natural 60Hz timing reference.
; In a real demo, you'd use this time to update animations.
; ---------------------------------------------------------------------------
wait_vsync:
.wait:
    LDA @VGA_STATUS
    AND A, #VGA_STATUS_VSYNC
    JZ A, .wait
    RTS

; ---------------------------------------------------------------------------
; fill_screen - Fill entire VGA framebuffer with colour index 1
;
; We write directly to VGA VRAM at 0xA0000. In Mode 13h, this is a simple
; linear framebuffer - one byte per pixel, 320x200 = 64000 bytes.
;
; Using a single colour index everywhere means the copper's palette changes
; will affect every pixel on screen, creating solid horizontal bands.
; ---------------------------------------------------------------------------
fill_screen:
    LDX #VGA_VRAM           ; Start of VGA framebuffer
    LDY #SCREEN_SIZE        ; 64000 bytes to fill

.fill_loop:
    LDA #1                  ; Colour index 1 - our "magic" colour
    STA [X]                 ; Write to VRAM
    ADD X, #1
    SUB Y, #1
    JNZ Y, .fill_loop
    RTS

; =============================================================================
; COPPER LIST - The heart of the effect
;
; This list executes once per frame, synchronized with the video beam.
; Each section does three things:
;   1. SETBASE to VIDEO - so WAIT instruction uses IE video timing
;   2. WAIT for target scanline - copper pauses until beam reaches Y
;   3. SETBASE to VGA_DAC - redirect writes to VGA palette registers
;   4. MOVE commands - change palette entry 1 to new RGB values
;
; WHY SETBASE SWITCHING?
; The WAIT instruction only works with IE VideoChip coordinates (it's
; part of that chip). But we want to write to VGA registers. So we:
;   - Switch to VIDEO base for WAIT
;   - Switch to VGA_DAC base for palette writes
;
; VGA DAC PROTOCOL:
; The VGA DAC expects writes in a specific sequence:
;   1. Write palette index to DAC_WINDEX (which entry to modify)
;   2. Write R value (6-bit, 0-63) to DAC_DATA
;   3. Write G value to DAC_DATA
;   4. Write B value to DAC_DATA
; The DAC auto-advances after each RGB triplet, but we reset the index
; each time for clarity.
; =============================================================================

copper_list:
    ; =========================================================================
    ; SCANLINE 0-24: RED
    ; This executes immediately at frame start (no WAIT needed)
    ; Sets palette entry 1 to bright red before any pixels are drawn
    ; =========================================================================
    .word COP_SETBASE_VGA_DAC   ; Target VGA DAC registers
    .word COP_MOVE_VGA_WINDEX   ; Select which palette entry to modify
    .word 1                     ; Palette entry 1
    .word COP_MOVE_VGA_DATA     ; Write R component
    .word 63                    ; R = 63 (maximum brightness)
    .word COP_MOVE_VGA_DATA     ; Write G component
    .word 0                     ; G = 0
    .word COP_MOVE_VGA_DATA     ; Write B component
    .word 0                     ; B = 0 (pure red)

    ; =========================================================================
    ; SCANLINE 25-49: ORANGE
    ; Wait until beam reaches line 25, then change palette to orange
    ; =========================================================================
    .word COP_SETBASE_VIDEO     ; Switch to VIDEO for WAIT timing
    .word COP_WAIT_Y25          ; Pause until scanline 25
    .word COP_SETBASE_VGA_DAC   ; Switch back to VGA DAC
    .word COP_MOVE_VGA_WINDEX
    .word 1
    .word COP_MOVE_VGA_DATA
    .word 63                    ; R = 63
    .word COP_MOVE_VGA_DATA
    .word 32                    ; G = 32 (half brightness for orange)
    .word COP_MOVE_VGA_DATA
    .word 0                     ; B = 0

    ; =========================================================================
    ; SCANLINE 50-74: YELLOW
    ; =========================================================================
    .word COP_SETBASE_VIDEO
    .word COP_WAIT_Y50
    .word COP_SETBASE_VGA_DAC
    .word COP_MOVE_VGA_WINDEX
    .word 1
    .word COP_MOVE_VGA_DATA
    .word 63                    ; R = 63
    .word COP_MOVE_VGA_DATA
    .word 63                    ; G = 63 (R+G = yellow)
    .word COP_MOVE_VGA_DATA
    .word 0                     ; B = 0

    ; =========================================================================
    ; SCANLINE 75-99: GREEN
    ; =========================================================================
    .word COP_SETBASE_VIDEO
    .word COP_WAIT_Y75
    .word COP_SETBASE_VGA_DAC
    .word COP_MOVE_VGA_WINDEX
    .word 1
    .word COP_MOVE_VGA_DATA
    .word 0                     ; R = 0
    .word COP_MOVE_VGA_DATA
    .word 63                    ; G = 63 (pure green)
    .word COP_MOVE_VGA_DATA
    .word 0                     ; B = 0

    ; =========================================================================
    ; SCANLINE 100-124: CYAN
    ; =========================================================================
    .word COP_SETBASE_VIDEO
    .word COP_WAIT_Y100
    .word COP_SETBASE_VGA_DAC
    .word COP_MOVE_VGA_WINDEX
    .word 1
    .word COP_MOVE_VGA_DATA
    .word 0                     ; R = 0
    .word COP_MOVE_VGA_DATA
    .word 63                    ; G = 63
    .word COP_MOVE_VGA_DATA
    .word 63                    ; B = 63 (G+B = cyan)

    ; =========================================================================
    ; SCANLINE 125-149: BLUE
    ; =========================================================================
    .word COP_SETBASE_VIDEO
    .word COP_WAIT_Y125
    .word COP_SETBASE_VGA_DAC
    .word COP_MOVE_VGA_WINDEX
    .word 1
    .word COP_MOVE_VGA_DATA
    .word 0                     ; R = 0
    .word COP_MOVE_VGA_DATA
    .word 0                     ; G = 0
    .word COP_MOVE_VGA_DATA
    .word 63                    ; B = 63 (pure blue)

    ; =========================================================================
    ; SCANLINE 150-174: MAGENTA
    ; =========================================================================
    .word COP_SETBASE_VIDEO
    .word COP_WAIT_Y150
    .word COP_SETBASE_VGA_DAC
    .word COP_MOVE_VGA_WINDEX
    .word 1
    .word COP_MOVE_VGA_DATA
    .word 63                    ; R = 63
    .word COP_MOVE_VGA_DATA
    .word 0                     ; G = 0
    .word COP_MOVE_VGA_DATA
    .word 63                    ; B = 63 (R+B = magenta)

    ; =========================================================================
    ; SCANLINE 175-199: WHITE
    ; =========================================================================
    .word COP_SETBASE_VIDEO
    .word COP_WAIT_Y175
    .word COP_SETBASE_VGA_DAC
    .word COP_MOVE_VGA_WINDEX
    .word 1
    .word COP_MOVE_VGA_DATA
    .word 63                    ; R = 63
    .word COP_MOVE_VGA_DATA
    .word 63                    ; G = 63
    .word COP_MOVE_VGA_DATA
    .word 63                    ; B = 63 (full white)

    ; =========================================================================
    ; END OF COPPER LIST
    ; Copper halts here until next frame, when it restarts from copper_list
    ; =========================================================================
    .word COP_END
