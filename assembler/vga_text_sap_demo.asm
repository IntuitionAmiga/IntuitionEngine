; ============================================================================
; VGA TEXT MODE DEMO WITH SAP MUSIC
; Z80 Assembly for IntuitionEngine - VGA Mode 03h (80x25x16 colors)
; ============================================================================
;
; REFERENCE IMPLEMENTATION FOR DEMOSCENE TECHNIQUES
; This file is heavily commented to teach demo programming concepts.
;
; === WHAT THIS DEMO DOES ===
; 1. Displays a large "INTUITION" ASCII art logo using CP437 shade characters
; 2. Creates animated rainbow raster bars sweeping across all rows
; 3. Cycles the logo through bright color palette (classic demoscene effect)
; 4. Shows four info text lines that bounce independently via sine wave
; 5. Scrolls a long message across the bottom of the screen
; 6. Plays Atari SAP music through the POKEY+ enhanced audio engine
;
; === WHY THESE EFFECTS MATTER (HISTORICAL CONTEXT) ===
;
; VGA TEXT MODE (Mode 03h):
; Before graphical user interfaces, text mode was THE display mode for PCs.
; Mode 03h (80x25 characters, 16 colors) dates back to the IBM CGA/EGA/VGA
; era (1981-1987). Each character cell has two bytes: an ASCII code and an
; attribute byte encoding foreground (4 bits) and background (4 bits) colors.
;
; Demosceners discovered that even in text mode, impressive effects were
; possible by rapidly changing colors per-row (fake raster bars), animating
; custom characters, and using the extended CP437 character set's shade
; blocks (░▒▓█) to create pseudo-graphics.
;
; THE Z80 CPU:
; The Zilog Z80 (1976) powered countless home computers: ZX Spectrum, MSX,
; Amstrad CPC, TRS-80, and arcade machines like Pac-Man. With its extended
; instruction set (IX/IY index registers, block operations, bit manipulation),
; it was more capable than the contemporary 6502, though sharing the 8-bit era's
; emphasis on clever coding to overcome limited registers and speed.
;
; POKEY SOUND CHIP & SAP FILES:
; The POKEY (Pot Keyboard Integrated Circuit) was Atari's signature sound
; chip in the 400/800/XL/XE computers. Its four voices with polynomial
; counters and high-pass filters created the distinctive "Atari sound."
; SAP (Slight Atari Player) is a music file format preserving the 6502
; player code and POKEY register data from thousands of Atari chiptunes.
;
; The Intuition Engine's POKEY+ mode enhances classic SAP playback with
; improved audio quality while maintaining authentic Atari character.
;
; CP437 CHARACTER SET:
; Code Page 437 was the original IBM PC character set, containing ASCII
; plus 128 extended characters including box-drawing symbols and the
; famous "shade blocks" (chars 176-178, 219-223) used extensively in
; DOS-era UI design and text-mode art. These characters enable pseudo-
; graphical effects like our logo's gradient shading.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +-------------------------------------------------------------+
;   |                    MAIN LOOP (60 FPS)                       |
;   |                                                             |
;   |  +-----------+    +-----------+    +-----------+            |
;   |  | WAIT FOR  |--->|  RASTER   |--->|   LOGO    |            |
;   |  |  VSYNC    |    |   BARS    |    |  COLORS   |            |
;   |  +-----------+    +-----------+    +-----------+            |
;   |                                          |                  |
;   |  +-----------+    +-----------+          |                  |
;   |  |   NEXT    |<---|  SCROLLER |<---------+                  |
;   |  |   FRAME   |    | + BOUNCE  |                             |
;   |  +-----------+    +-----------+                             |
;   +-------------------------------------------------------------+
;
;   +-------------------------------------------------------------+
;   |              POKEY+ AUDIO ENGINE (runs in parallel)         |
;   |                                                             |
;   |  The SAP player renders Atari 8-bit music data to POKEY     |
;   |  register writes, which the POKEY emulator converts to      |
;   |  audio samples. Music plays autonomously once triggered.    |
;   +-------------------------------------------------------------+
;
; === MEMORY MAP (Z80 64KB Address Space) ===
;
;   +-------------+---------------------------+
;   | 0x0000-0x0FFF | Program code (~4KB)    |
;   +-------------+---------------------------+
;   | 0x8000-0xBFFF | VRAM Bank Window (16KB)|  <- Text buffer access
;   +-------------+---------------------------+
;   | 0xC100-0xC1FF | Sine table (256 bytes) |
;   +-------------+---------------------------+
;   | 0xE000-0xEFFF | SAP music data (~4KB)  |
;   +-------------+---------------------------+
;   | 0xF000-0xFFFF | I/O region             |
;   +-------------+---------------------------+
;
; === VGA TEXT BUFFER MEMORY LAYOUT ===
;
; VGA text mode stores 80 columns x 25 rows of character cells.
; Each cell is 2 bytes (4000 bytes total for full screen):
;
;   Byte 0: ASCII character code (0-255)
;   Byte 1: Attribute byte
;           Bits 7-4: Background color (0-15)
;           Bits 3-0: Foreground color (0-15)
;
; Screen position (col, row) maps to offset: (row * 160) + (col * 2)
;
; The text buffer lives at physical address 0xB8000. Since the Z80
; can only address 64KB, we use a VRAM bank window at 0x8000-0xBFFF
; to access it. Bank 0x2E (46) maps this window to the text buffer.
;
; ============================================================================

    .include "ie80.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- VGA Text Mode Memory Access ---
; The text buffer is at physical address 0xB8000, beyond Z80's 64KB reach.
; We use bank 0x2E (46) to map the 0x8000 bank window to this address.
; Bank calculation: 0xB8000 / 0x4000 = 46 (0x2E)
.set TEXT_BANK,0x2E
.set TEXT_WINDOW,0x8000         ; Z80 address of VRAM bank window

; --- Stack Configuration ---
; Z80 stack grows downward. We place it just below the I/O region
; to avoid conflicts with VRAM window and data areas.
.set STACK_TOP,0xEFF0

; --- SAP Music Data ---
; SAP file is embedded at 0xE000. Length calculated automatically
; by assembler from label difference.
.set SAP_DATA_LEN,sap_data_end-sap_data

; --- SAP Player I/O Registers (Z80 Addresses) ---
; The SAP player uses memory-mapped registers in the 0xFD00 range.
; Z80 address 0xFDxx maps to physical address 0xF0Dxx via I/O translation.
;
; SAP_PLAY_PTR: 32-bit pointer to SAP file data in memory
; SAP_PLAY_LEN: 32-bit length of SAP file in bytes
; SAP_PLAY_CTRL: Control register (bit 0=start, bit 2=loop)
; POKEY_PLUS: Enable enhanced POKEY mode for better audio quality
.set Z80_SAP_PTR_0,0xFD10       ; Pointer byte 0 (bits 0-7)
.set Z80_SAP_PTR_1,0xFD11       ; Pointer byte 1 (bits 8-15)
.set Z80_SAP_PTR_2,0xFD12       ; Pointer byte 2 (bits 16-23)
.set Z80_SAP_PTR_3,0xFD13       ; Pointer byte 3 (bits 24-31)
.set Z80_SAP_LEN_0,0xFD14       ; Length byte 0 (bits 0-7)
.set Z80_SAP_LEN_1,0xFD15       ; Length byte 1 (bits 8-15)
.set Z80_SAP_LEN_2,0xFD16       ; Length byte 2 (bits 16-23)
.set Z80_SAP_LEN_3,0xFD17       ; Length byte 3 (bits 24-31)
.set Z80_SAP_CTRL,0xFD18        ; Control: bit0=play, bit1=stop, bit2=loop
.set Z80_POKEY_PLUS,0xFD09      ; POKEY+ mode enable

; ============================================================================
; ENTRY POINT
; ============================================================================
; The Z80 begins execution here after loading the program.
; We initialize all hardware subsystems before entering the main loop.
; ============================================================================
    .org 0x0000

start:
    ; === DISABLE INTERRUPTS ===
    ; We don't use interrupts in this demo, so disable them to prevent
    ; any unexpected behavior. The DI instruction clears the interrupt
    ; flip-flop, blocking maskable interrupts (NMIs can still occur).
    di

    ; === INITIALIZE STACK POINTER ===
    ; The Z80 stack grows downward (toward lower addresses).
    ; SP must be set before any CALL/PUSH/POP operations.
    ld sp,STACK_TOP

    ; === CONFIGURE VGA HARDWARE ===
    ; The VGA chip is controlled via I/O ports (IN/OUT instructions).
    ; VGA_PORT_CTRL enables/disables the VGA output.
    ; VGA_PORT_MODE selects the video mode (text or graphics).
    ;
    ; Mode 03h (VGA_MODE_TEXT) is the classic 80x25 color text mode.
    ; This mode has been standard since the IBM CGA (1981).
    ld a,VGA_CTRL_ENABLE
    out (VGA_PORT_CTRL),a       ; Turn on VGA output
    ld a,VGA_MODE_TEXT
    out (VGA_PORT_MODE),a       ; Select mode 03h (80x25 text)

    ; === SET UP VRAM BANK FOR TEXT BUFFER ACCESS ===
    ; The VGA text buffer at 0xB8000 is outside Z80's 64KB address space.
    ; The VRAM_BANK_REG register selects which 16KB chunk appears at 0x8000.
    ; Bank 0x2E (46) maps 0x8000-0xBFFF to physical 0xB8000-0xBBFFF.
    ld a,TEXT_BANK
    ld (VRAM_BANK_REG),a

    ; === START SAP MUSIC PLAYBACK ===
    ; Initialize the POKEY+ audio engine and begin playing the embedded
    ; SAP file. Music plays autonomously - no per-frame CPU involvement.
    call init_sap

    ; === PREPARE DISPLAY ===
    ; Clear screen to black, draw static elements (logo, info text).
    call clear_screen
    call draw_logo
    call draw_info

    ; === INITIALIZE ANIMATION STATE ===
    ; All animation counters start at zero.
    ; XOR A is the fastest way to load 0 into A (1 byte, 4 T-states).
    xor a
    ld (frame_lo),a             ; Frame counter low byte
    ld (frame_hi),a             ; Frame counter high byte
    ld (scroll_pos),a           ; Scroll message position low byte
    ld (scroll_pos+1),a         ; Scroll message position high byte
    ld (scroll_wait),a          ; Scroll timing counter
    ld (scroll_cnt),a           ; (unused, kept for compatibility)

; ============================================================================
; MAIN LOOP
; ============================================================================
; This loop executes once per video frame (60 times per second with VSYNC).
; Each iteration performs all visual effects and animation updates.
;
; ORDER OF OPERATIONS:
; 1. Wait for VSYNC - prevents screen tearing
; 2. Increment frame counter - drives all animation timing
; 3. Raster bars - changes background colors per row (sine wave pattern)
; 4. Logo colors - cycles foreground through bright colors
; 5. Bouncing text - repositions info lines using sine table
; 6. Scroller - draws and advances scrolling message at bottom
; ============================================================================
main_loop:
    ; === VSYNC SYNCHRONIZATION ===
    ; We wait for vertical blank to prevent "tearing" artifacts.
    ; Without VSYNC, the screen update could happen while the display
    ; is being drawn, causing visible flickering or split images.
    call wait_vsync

    ; === INCREMENT FRAME COUNTER ===
    ; The 16-bit frame counter (frame_lo, frame_hi) wraps at 65536.
    ; Various effects use different bits for timing:
    ;   - frame_lo bit 0-2: fast effects (8 speeds per second)
    ;   - frame_lo bit 3-7: slow effects (logo color, scroll)
    ;   - frame_hi: very slow effects (not used in this demo)
    ld hl,(frame_lo)            ; Load 16-bit counter into HL
    inc hl                      ; Increment
    ld (frame_lo),hl            ; Store back

    ; === EXECUTE VISUAL EFFECTS ===
    ; Each effect is a separate subroutine for clarity.
    ; They execute in order, each modifying parts of the text buffer.
    call do_raster_bars         ; Effect 1: Animated background colors
    call do_logo_colors         ; Effect 2: Cycling logo foreground
    call do_bounce_text         ; Effect 3: Bouncing info text lines
    call do_scroller            ; Effect 4: Horizontal scrolling message

    ; === LOOP FOREVER ===
    ; The demo runs indefinitely until the user stops it.
    jp main_loop

; ============================================================================
; SAP MUSIC INITIALIZATION
; ============================================================================
; Sets up the POKEY+ audio engine and begins SAP file playback.
;
; SAP FILE FORMAT:
; SAP files contain a header with metadata (author, title, etc.) followed
; by 6502 machine code that writes POKEY registers to create music.
; The Intuition Engine's SAP player executes this code, converting the
; POKEY writes to audio samples.
;
; POKEY+ MODE:
; The standard POKEY emulation is cycle-accurate to the original hardware.
; POKEY+ mode adds enhanced audio quality (better filtering, interpolation)
; while maintaining compatibility with original SAP files.
;
; CONTROL REGISTER BITS:
;   Bit 0: Start playback (set to begin playing)
;   Bit 1: Stop playback (set to halt)
;   Bit 2: Loop mode (set to repeat when song ends)
;
; We use 0x05 = binary 00000101 = Start + Loop
; ============================================================================
init_sap:
    ; === ENABLE POKEY+ ENHANCED MODE ===
    ; This improves audio quality without changing compatibility.
    ld a,1
    ld (Z80_POKEY_PLUS),a

    ; === SET SAP DATA POINTER (32-bit) ===
    ; The pointer must be the physical memory address where the SAP
    ; data is located. Since Z80 addresses are 16-bit, the upper
    ; two bytes are always 0x00 for data in the first 64KB.
    ;
    ; NOTE: The assembler calculates 'sap_data' as a 16-bit address.
    ; We split it into 4 bytes for the 32-bit pointer register.
    ld a,sap_data & 0xFF        ; Byte 0: bits 0-7
    ld (Z80_SAP_PTR_0),a
    ld a,(sap_data >> 8) & 0xFF ; Byte 1: bits 8-15
    ld (Z80_SAP_PTR_1),a
    ld a,(sap_data >> 16) & 0xFF ; Byte 2: bits 16-23 (always 0)
    ld (Z80_SAP_PTR_2),a
    ld a,(sap_data >> 24) & 0xFF ; Byte 3: bits 24-31 (always 0)
    ld (Z80_SAP_PTR_3),a

    ; === SET SAP DATA LENGTH (32-bit) ===
    ; Length is calculated by the assembler as the difference between
    ; the end and start labels of the embedded SAP data.
    ld a,SAP_DATA_LEN & 0xFF
    ld (Z80_SAP_LEN_0),a
    ld a,(SAP_DATA_LEN >> 8) & 0xFF
    ld (Z80_SAP_LEN_1),a
    ld a,(SAP_DATA_LEN >> 16) & 0xFF
    ld (Z80_SAP_LEN_2),a
    ld a,(SAP_DATA_LEN >> 24) & 0xFF
    ld (Z80_SAP_LEN_3),a

    ; === START PLAYBACK WITH LOOPING ===
    ; 0x05 = bit 0 (start) + bit 2 (loop)
    ; The music will play continuously, restarting when it ends.
    ld a,0x05
    ld (Z80_SAP_CTRL),a
    ret

; ============================================================================
; VSYNC SYNCHRONIZATION
; ============================================================================
; Waits for the vertical blanking interval to begin.
;
; WHY VSYNC MATTERS:
; The VGA display is drawn line by line, top to bottom, 60 times per second.
; If we modify the display buffer while it's being scanned out, the viewer
; may see part of the old frame and part of the new frame (tearing).
;
; By waiting for VSYNC (when the electron beam returns from bottom to top),
; we ensure our updates happen "between frames" and appear instantaneous.
;
; TWO-STAGE WAIT:
; 1. Wait while VSYNC is active (in case we caught the end of one)
; 2. Wait until VSYNC becomes active (catch the START of the new interval)
; This ensures we're at the beginning of VBLANK, maximizing time for drawing.
; ============================================================================
wait_vsync:
    ; === WAIT FOR VSYNC TO END (if already active) ===
    ; Loop while VGA_STATUS_VSYNC bit is set
vs_wait_end:
    in a,(VGA_PORT_STATUS)      ; Read VGA status register
    and VGA_STATUS_VSYNC        ; Isolate VSYNC bit
    jr nz,vs_wait_end           ; Loop while bit is set

    ; === WAIT FOR VSYNC TO START ===
    ; Loop until VGA_STATUS_VSYNC bit becomes set
vs_wait_start:
    in a,(VGA_PORT_STATUS)
    and VGA_STATUS_VSYNC
    jr z,vs_wait_start          ; Loop while bit is clear
    ret

; ============================================================================
; CLEAR SCREEN
; ============================================================================
; Fills the entire screen with space characters and black-on-black attributes.
; This creates a blank black screen as the canvas for our effects.
;
; IMPLEMENTATION:
; We write 80 columns x 25 rows = 2000 character cells.
; Each cell is 2 bytes, so we write 4000 bytes total.
; Space character (0x20) with attribute 0x00 (black on black).
; ============================================================================
clear_screen:
    ld hl,TEXT_WINDOW           ; Start of text buffer (via bank window)
    ld bc,80*25                 ; 2000 character cells
clr_loop:
    ld (hl),' '                 ; Space character
    inc hl
    ld (hl),0x00                ; Attribute: black on black
    inc hl
    dec bc                      ; Decrement counter
    ld a,b
    or c                        ; Check if BC = 0
    jr nz,clr_loop              ; Continue until all cells cleared
    ret

; ============================================================================
; DRAW LOGO
; ============================================================================
; Draws the "INTUITION" ASCII art logo at the top of the screen.
;
; LOGO DESIGN:
; The logo uses CP437 extended characters to create a gradient shading effect:
;   Character 219 (0xDB) = █ Full block (solid)
;   Character 178 (0xB2) = ▓ Dark shade (75% fill)
;   Character 177 (0xB1) = ▒ Medium shade (50% fill)
;   Character 176 (0xB0) = ░ Light shade (25% fill)
;
; These characters were invented for IBM's original PC character set (1981)
; and became icons of DOS-era computing, used in everything from Norton
; Commander to ASCII art to demoscene productions.
;
; LOGO DIMENSIONS:
; 72 characters wide x 9 rows tall, centered at column 4 (leaving 8 chars margin)
; ============================================================================
draw_logo:
    ld hl,TEXT_WINDOW + 0*160 + 4*2    ; Row 0, column 4 (centered)
    ld de,logo_data                     ; Source: logo character data
    ld b,9                              ; 9 rows to draw
logo_row:
    push bc
    push hl
    ld b,72                             ; 72 characters per row
logo_chr:
    ld a,(de)                           ; Get character from logo data
    ld (hl),a                           ; Write to screen
    inc hl
    ld a,0x0F                           ; White foreground (will be color cycled)
    ld (hl),a                           ; Write attribute
    inc hl
    inc de                              ; Next source character
    djnz logo_chr                       ; Loop for all 72 characters
    pop hl
    ld bc,160                           ; 80 chars * 2 bytes = 160 bytes per row
    add hl,bc                           ; Move to next row
    pop bc
    djnz logo_row                       ; Loop for all 9 rows
    ret

; ============================================================================
; DRAW INFO TEXT
; ============================================================================
; Draws static information text in the center of the screen.
; These lines will be animated (bounced) by do_bounce_text each frame.
;
; INITIAL POSITIONS:
;   Row 11: Music credit (white)
;   Row 13: System info (cyan)
;   Row 15: Demo title (yellow)
;   Row 18: Greetings (magenta)
;
; The colors were chosen to create visual hierarchy and reference classic
; demoscene color schemes (cyan for tech, yellow for titles, magenta for greets).
; ============================================================================
draw_info:
    ; --- Row 11: Music Credit ---
    ld hl,TEXT_WINDOW + 11*160 + 16*2   ; Row 11, column 16
    ld de,str_music
    ld c,0x0F                           ; White text
    call print_str

    ; --- Row 13: System Info ---
    ld hl,TEXT_WINDOW + 13*160 + 20*2   ; Row 13, column 20
    ld de,str_system
    ld c,0x0B                           ; Bright cyan
    call print_str

    ; --- Row 15: Demo Title ---
    ld hl,TEXT_WINDOW + 15*160 + 22*2   ; Row 15, column 22
    ld de,str_demo
    ld c,0x0E                           ; Yellow
    call print_str

    ; --- Row 18: Scene Greetings ---
    ld hl,TEXT_WINDOW + 18*160 + 24*2   ; Row 18, column 24
    ld de,str_greets
    ld c,0x0D                           ; Bright magenta
    call print_str
    ret

; ============================================================================
; PRINT NULL-TERMINATED STRING
; ============================================================================
; Prints a string to the screen at the specified position with given color.
;
; INPUT:
;   HL = Screen position (pointer into text buffer)
;   DE = String address (null-terminated)
;   C  = Attribute byte (color)
;
; The routine stops when it encounters a null byte (0x00) in the string.
; ============================================================================
print_str:
    ld a,(de)                   ; Get character from string
    or a                        ; Check for null terminator
    ret z                       ; Return if end of string
    ld (hl),a                   ; Write character to screen
    inc hl
    ld (hl),c                   ; Write attribute
    inc hl
    inc de                      ; Next character in string
    jr print_str                ; Loop

; ============================================================================
; EFFECT 1: RASTER BARS
; ============================================================================
; Creates animated horizontal color bands across the entire screen.
;
; RASTER BAR HISTORY:
; "Raster bars" or "raster splits" were a signature effect on the Amiga
; and C64, where the hardware could change palette colors mid-scanline.
; By changing the background color at precise moments during the CRT
; beam's scan, coders created gradients impossible with static palettes.
;
; In text mode, we simulate this by changing the background color of each
; row. While not as fine-grained as true raster effects, it captures the
; aesthetic with simpler implementation.
;
; IMPLEMENTATION:
; For each row (0-24), we calculate a color using the sine table:
;   1. row_index * 8 + frame_counter = sine table offset
;   2. Look up sine value (0-15)
;   3. Mask to 0-7 (darker colors only, for readability)
;   4. Shift to upper nibble (background color position)
;   5. Apply to all 80 character attributes in that row
;
; The frame_counter offset creates the animation - colors appear to
; wave downward as the sine pattern scrolls through the rows.
; ============================================================================
do_raster_bars:
    ld a,(frame_lo)
    ld c,a                      ; C = animation phase (0-255)

    ; Start at first attribute byte (offset 1 from buffer start)
    ld hl,TEXT_WINDOW + 1
    ld b,25                     ; 25 rows to process

raster_row:
    push bc
    push hl

    ; === CALCULATE ROW COLOR ===
    ; Formula: sine_table[(row_index * 8 + phase) & 0xFF]
    ; The *8 spreads the sine wave across rows, phase animates it.
    ld a,25
    sub b                       ; A = row number (0-24)
    sla a
    sla a
    sla a                       ; A = row * 8
    add a,c                     ; A = row*8 + phase (wraps at 256)

    ; Look up in sine table
    ld e,a
    ld d,0
    push hl
    ld hl,sine_table
    add hl,de
    ld a,(hl)                   ; A = sine value (0-15)
    pop hl

    ; === CONVERT TO BACKGROUND COLOR ===
    ; Keep only 0-7 (darker colors) so text remains readable
    ; Then shift to upper nibble (bits 4-7 = background)
    and 0x07                    ; Mask to 0-7
    sla a
    sla a
    sla a
    sla a                       ; Shift to upper nibble
    ld d,a                      ; D = background color in position

    ; === APPLY TO ALL COLUMNS ===
    ; Update attribute byte of each character, preserving foreground
    ld b,80                     ; 80 columns per row
raster_col:
    ld a,(hl)                   ; Get current attribute
    and 0x0F                    ; Keep foreground (lower nibble)
    or d                        ; Add new background (upper nibble)
    ld (hl),a                   ; Write back
    inc hl
    inc hl                      ; Skip to next attribute (every 2 bytes)
    djnz raster_col             ; Loop for all 80 columns

    ; === MOVE TO NEXT ROW ===
    pop hl
    push de
    ld de,160                   ; 160 bytes per row
    add hl,de                   ; HL now points to next row's first attr
    pop de
    pop bc
    djnz raster_row             ; Loop for all 25 rows
    ret

; ============================================================================
; EFFECT 2: LOGO COLOR CYCLING
; ============================================================================
; Cycles the logo foreground through bright colors (8-15).
;
; COLOR CYCLING HISTORY:
; Before GPUs could display millions of colors, palette cycling was THE way
; to create animation without redrawing pixels. By rotating palette indices,
; entire screens could appear to animate - waterfalls flowing, fire burning,
; plasma pulsing - all without touching the actual pixel data.
;
; This technique was essential in games like Monkey Island and demos that
; needed to animate complex patterns within CPU constraints.
;
; IMPLEMENTATION:
; We use the frame counter divided by 8 (slowing the cycle) to index into
; the bright color range (8-15). Dark gray (8) is skipped for visibility.
; The color is applied to all 72x9 characters of the logo while preserving
; the raster bar background colors.
; ============================================================================
do_logo_colors:
    ; === CALCULATE CYCLING COLOR ===
    ; Divide frame counter by 8 for slower color changes
    ; (one color change every 8 frames = ~7.5 changes per second)
    ld a,(frame_lo)
    srl a                       ; Divide by 2
    srl a                       ; Divide by 4
    srl a                       ; Divide by 8
    and 0x0F                    ; Mask to 0-15
    or 0x08                     ; Force bright colors (8-15)
    cp 0x08                     ; Is it dark gray (8)?
    jr nz,lc_color_ok
    ld a,0x0F                   ; Use white instead of dark gray
lc_color_ok:
    ld c,a                      ; C = cycling foreground color

    ; === APPLY TO LOGO AREA ===
    ; Logo is at row 0-8, columns 4-75 (72 chars wide)
    ; We access attribute bytes (odd offsets)
    ld hl,TEXT_WINDOW + 0*160 + 4*2 + 1  ; First attr of logo
    ld b,9                      ; 9 rows

lc_row:
    push bc
    push hl

    ld b,72                     ; 72 characters per logo row
lc_char:
    ld a,(hl)                   ; Get current attribute
    and 0xF0                    ; Keep background (raster bars)
    or c                        ; Add cycling foreground
    ld (hl),a
    inc hl
    inc hl                      ; Skip to next attribute
    djnz lc_char                ; Loop for all characters

    pop hl
    push de
    ld de,160                   ; Next row
    add hl,de
    pop de
    pop bc
    djnz lc_row                 ; Loop for all logo rows
    ret

; ============================================================================
; EFFECT 3: BOUNCING TEXT
; ============================================================================
; Makes the four info text lines bounce left and right independently.
;
; SINE WAVE ANIMATION:
; The sine table contains precomputed values for smooth oscillation.
; By indexing into it with (frame_counter + phase_offset), each line
; gets a different position in the wave, creating an organic, flowing
; motion where lines move independently but harmoniously.
;
; PHASE OFFSETS:
;   Line 1 (Music):  0   - Reference position
;   Line 2 (System): 64  - 1/4 wave behind
;   Line 3 (Demo):   128 - 1/2 wave behind (opposite phase)
;   Line 4 (Greets): 192 - 3/4 wave behind
;
; This creates a "wave" effect where the motion ripples down the lines.
;
; IMPLEMENTATION:
; Each line is cleared, then redrawn at a new column position calculated
; from the sine table. The sine values (0-15) add horizontal offset to
; each line's base column position.
; ============================================================================
do_bounce_text:
    ; === LINE 1: MUSIC CREDIT (phase 0) ===
    ld a,(frame_lo)
    call get_bounce_offset      ; Get sine value into (bounce_off)
    ld hl,TEXT_WINDOW + 11*160
    call clear_text_row         ; Clear row (spaces, keep bg colors)
    ld a,(bounce_off)
    add a,12                    ; Base column 12 + bounce (0-15)
    ld b,a
    ld hl,TEXT_WINDOW + 11*160
    call set_text_col           ; Adjust HL to column B
    ld de,str_music
    ld c,0x0F                   ; White
    call print_str

    ; === LINE 2: SYSTEM INFO (phase 64 = 1/4 wave) ===
    ld a,(frame_lo)
    add a,64                    ; Quarter wave offset
    call get_bounce_offset
    ld hl,TEXT_WINDOW + 13*160
    call clear_text_row
    ld a,(bounce_off)
    add a,17                    ; Base column 17
    ld b,a
    ld hl,TEXT_WINDOW + 13*160
    call set_text_col
    ld de,str_system
    ld c,0x0B                   ; Cyan
    call print_str

    ; === LINE 3: DEMO TITLE (phase 128 = 1/2 wave) ===
    ld a,(frame_lo)
    add a,128                   ; Half wave offset (opposite phase)
    call get_bounce_offset
    ld hl,TEXT_WINDOW + 15*160
    call clear_text_row
    ld a,(bounce_off)
    add a,21                    ; Base column 21
    ld b,a
    ld hl,TEXT_WINDOW + 15*160
    call set_text_col
    ld de,str_demo
    ld c,0x0E                   ; Yellow
    call print_str

    ; === LINE 4: GREETINGS (phase 192 = 3/4 wave) ===
    ld a,(frame_lo)
    add a,192                   ; Three-quarter wave offset
    call get_bounce_offset
    ld hl,TEXT_WINDOW + 18*160
    call clear_text_row
    ld a,(bounce_off)
    add a,21                    ; Base column 21
    ld b,a
    ld hl,TEXT_WINDOW + 18*160
    call set_text_col
    ld de,str_greets
    ld c,0x0D                   ; Magenta
    call print_str
    ret

; ============================================================================
; GET BOUNCE OFFSET FROM SINE TABLE
; ============================================================================
; Looks up a value in the sine table and stores it for bounce calculation.
;
; INPUT: A = index into sine table (0-255)
; OUTPUT: (bounce_off) = sine value (0-15)
;
; The sine table is 256 entries, giving full 360-degree coverage.
; Values range from 0 to 15, perfect for text column offsets.
; ============================================================================
get_bounce_offset:
    ld e,a
    ld d,0
    ld hl,sine_table
    add hl,de                   ; HL = sine_table + index
    ld a,(hl)                   ; A = sine value (0-15)
    ld (bounce_off),a           ; Store for caller
    ret

; ============================================================================
; CLEAR TEXT ROW
; ============================================================================
; Fills a row with space characters while preserving attribute backgrounds.
;
; INPUT: HL = start of row in text buffer
;
; This allows raster bar colors to show through even when text is erased.
; ============================================================================
clear_text_row:
    ld b,80                     ; 80 columns
ctr_loop:
    ld (hl),' '                 ; Space character
    inc hl
    inc hl                      ; Skip attribute (preserve raster bg)
    djnz ctr_loop
    ret

; ============================================================================
; SET TEXT COLUMN
; ============================================================================
; Adjusts HL to point to the specified column within a row.
;
; INPUT: HL = start of row, B = column number (0-79)
; OUTPUT: HL = position of character at column B
; ============================================================================
set_text_col:
    ld a,b
    add a,a                     ; A = column * 2 (2 bytes per cell)
    ld e,a
    ld d,0
    add hl,de                   ; HL += column * 2
    ret

; ============================================================================
; EFFECT 4: SCROLLING TEXT
; ============================================================================
; Scrolls a message horizontally across the bottom of the screen.
;
; SCROLLERS IN DEMOSCENE:
; The horizontal scroller is perhaps THE most iconic demoscene effect.
; From the earliest C64 demos to modern PC productions, scrollers
; conveyed messages, credits, greetings ("greets"), and manifestos.
; They were often enhanced with wave motion, zooming, or rotation.
;
; IMPLEMENTATION:
; The scroll_pos variable tracks which character of the message should
; appear at the leftmost screen column. Each frame (after scroll_wait
; frames for timing), we increment scroll_pos and wrap at message end.
;
; We draw 80 characters starting from scroll_pos, wrapping to the
; message start when we hit the null terminator. This creates seamless
; looping without visible seams.
;
; SCROLL SPEED:
; scroll_wait counts frames between advances. At 8 frames per advance
; with 60 FPS display, we get ~7.5 characters per second - readable
; but engaging, matching classic demoscene scroll speeds.
; ============================================================================
do_scroller:
    ; === CHECK SCROLL TIMING ===
    ; Only advance scroll position every 8 frames
    ld a,(scroll_wait)
    inc a
    ld (scroll_wait),a
    cp 8                        ; Time to advance?
    jr c,sc_no_advance          ; Not yet, skip to drawing

    ; === ADVANCE SCROLL POSITION ===
    xor a
    ld (scroll_wait),a          ; Reset wait counter

    ld hl,(scroll_pos)
    inc hl                      ; Advance by one character

    ; Check for wrap at end of message
    ld de,scroll_end - scroll_msg
    or a                        ; Clear carry
    sbc hl,de                   ; HL = pos - length
    jr c,sc_no_wrap             ; If pos < length, no wrap needed
    ld hl,0                     ; Wrap to beginning
    jr sc_save_pos
sc_no_wrap:
    add hl,de                   ; Restore HL (undo subtraction)
sc_save_pos:
    ld (scroll_pos),hl

sc_no_advance:
    ; === DRAW VISIBLE PORTION ===
    ; DE = current position in message
    ; HL = screen position (row 23)
    ld hl,(scroll_pos)
    ld de,scroll_msg
    add hl,de                   ; HL = scroll_msg + scroll_pos
    ex de,hl                    ; DE = pointer into message

    ld hl,TEXT_WINDOW + 23*160  ; Row 23 (near bottom)
    ld b,80                     ; Draw 80 characters

sc_draw:
    ; Get character (handle wrap at null terminator)
    ld a,(de)
    or a                        ; Check for null
    jr nz,sc_not_end
    ; Hit end of message, wrap to start
    push hl
    ld hl,scroll_msg
    ex de,hl
    ld a,(de)                   ; Get first char of message
    pop hl
sc_not_end:
    ld (hl),a                   ; Write character
    inc hl
    ld a,0x0F                   ; White on (raster bar background)
    ld (hl),a
    inc hl                      ; Next screen position

    inc de                      ; Next message character
    djnz sc_draw
    ret

; ============================================================================
; VARIABLES
; ============================================================================
; Runtime state for animation and effects.
; Placed immediately after code to keep memory layout contiguous.
; ============================================================================

frame_lo:       .byte 0         ; Frame counter low byte (increments each frame)
frame_hi:       .byte 0         ; Frame counter high byte (for very slow effects)
scroll_pos:     .word 0         ; Current scroll offset in message
scroll_wait:    .byte 0         ; Frame counter for scroll timing
temp_row:       .byte 0         ; Temporary storage (unused)
temp_var:       .byte 0         ; Temporary storage (unused)
scroll_cnt:     .byte 0         ; Scroll counter (unused, kept for compatibility)
bounce_off:     .byte 0         ; Current bounce offset from sine table

; ============================================================================
; SINE TABLE
; ============================================================================
; 256-entry lookup table for smooth sine wave animation.
;
; WHY LOOKUP TABLES:
; Computing sine/cosine in real-time requires floating-point math or
; expensive fixed-point approximations. On 8-bit CPUs, a simple table
; lookup (one indexed memory read) is FAR faster than any calculation.
;
; This table contains one complete sine cycle scaled to 0-15:
;   Index 0:   sine(0)   = 0.0  -> 8 (midpoint)
;   Index 64:  sine(90)  = 1.0  -> 15 (maximum)
;   Index 128: sine(180) = 0.0  -> 8 (midpoint)
;   Index 192: sine(270) = -1.0 -> 0 (minimum)
;
; The values are offset so the range is 0-15 (no negative numbers),
; suitable for color indices and column offsets.
;
; TABLE GENERATION (pseudocode):
;   for i in 0..255:
;       table[i] = round(sin(i * 2 * PI / 256) * 7.5 + 7.5)
; ============================================================================
    .org 0xC100

sine_table:
    .byte  8, 8, 8, 9, 9, 9, 9,10,10,10,10,11,11,11,11,12
    .byte 12,12,12,12,13,13,13,13,13,14,14,14,14,14,14,14
    .byte 15,15,15,15,15,15,15,15,15,15,15,15,15,15,15,14
    .byte 14,14,14,14,14,14,13,13,13,13,13,12,12,12,12,12
    .byte 11,11,11,11,10,10,10,10, 9, 9, 9, 9, 8, 8, 8, 8
    .byte  7, 7, 7, 6, 6, 6, 6, 5, 5, 5, 5, 4, 4, 4, 4, 3
    .byte  3, 3, 3, 3, 2, 2, 2, 2, 2, 1, 1, 1, 1, 1, 1, 1
    .byte  0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
    .byte  1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3
    .byte  4, 4, 4, 4, 5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7
    .byte  8, 8, 8, 9, 9, 9, 9,10,10,10,10,11,11,11,11,12
    .byte 12,12,12,12,13,13,13,13,13,14,14,14,14,14,14,14
    .byte 15,15,15,15,15,15,15,15,15,15,15,15,15,15,15,14
    .byte 14,14,14,14,14,14,13,13,13,13,13,12,12,12,12,12
    .byte 11,11,11,11,10,10,10,10, 9, 9, 9, 9, 8, 8, 8, 8
    .byte  7, 7, 7, 6, 6, 6, 6, 5, 5, 5, 5, 4, 4, 4, 4, 3

; ============================================================================
; STRING DATA
; ============================================================================
; Text content displayed in the demo.
;
; GREETINGS TRADITION:
; "Greets" or "greetings" are shout-outs to other demoscene groups,
; musicians, artists, and friends. This tradition dates back to the
; earliest cracktros and demos, fostering community bonds across
; continents and decades. Even in 2026, demos include greets sections.
; ============================================================================

; --- Logo Data (72 chars x 9 rows) ---
; Uses CP437 extended characters for gradient shading:
;   219 (0xDB) = Full block    █
;   178 (0xB2) = Dark shade    ▓
;   177 (0xB1) = Medium shade  ▒
;   176 (0xB0) = Light shade   ░
;   223 (0xDF) = Upper half    ▀
;   220 (0xDC) = Lower half    ▄
;   222 (0xDE) = Right half    ▐
;
; This creates a 3D beveled effect with light appearing from upper-left.
logo_data:
    ; Row 1: " ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █ "
    .byte 32,219,219,178,32,219,219,219,220,32,32,32,32,219,32,220,220,220,219,219,219,219,219,178,32,219,32,32,32,32,219,219,32,32,219,219,178,220,220,220,219,219,219,219,219,178,32,219,219,178,32,177,219,219,219,219,219,32,32,32,219,219,219,220,32,32,32,32,219,32,32,32
    ; Row 2: "▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █ "
    .byte 178,219,219,177,32,219,219,32,223,219,32,32,32,219,32,178,32,32,219,219,177,32,178,177,32,219,219,32,32,178,219,219,177,178,219,219,177,178,32,32,219,219,177,32,178,177,178,219,219,177,177,219,219,177,32,32,219,219,177,32,219,219,32,223,219,32,32,32,219,32,32,32
    ; Row 3: "▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒"
    .byte 177,219,219,177,178,219,219,32,32,223,219,32,219,219,177,177,32,178,219,219,176,32,177,176,178,219,219,32,32,177,219,219,176,177,219,219,177,177,32,178,219,219,176,32,177,176,177,219,219,177,177,219,219,176,32,32,219,219,177,178,219,219,32,32,223,219,32,219,219,177,32,32
    ; Row 4: "░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒"
    .byte 176,219,219,176,178,219,219,177,32,32,222,32,219,219,177,176,32,178,219,219,178,32,176,32,178,178,219,32,32,176,219,219,176,176,219,219,176,176,32,178,219,219,178,32,176,32,176,219,219,176,177,219,219,32,32,32,219,219,176,178,219,219,177,32,32,222,32,219,219,177,32,32
    ; Row 5: "░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░"
    .byte 176,219,219,176,177,219,219,176,32,32,32,178,219,219,176,32,32,177,219,219,177,32,176,32,177,177,219,219,219,219,219,178,32,176,219,219,176,32,32,177,219,219,177,32,176,32,176,219,219,176,176,32,219,219,219,219,178,177,176,177,219,219,176,32,32,32,178,219,219,176,32,32
    ; Row 6: "░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒ "
    .byte 176,178,32,32,176,32,177,176,32,32,32,177,32,177,32,32,32,177,32,176,176,32,32,32,176,177,178,177,32,177,32,177,32,176,178,32,32,32,32,177,32,176,176,32,32,32,176,178,32,32,176,32,177,176,177,176,177,176,32,176,32,177,176,32,32,32,177,32,177,32,32,32
    ; Row 7: " ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░"
    .byte 32,177,32,176,176,32,176,176,32,32,32,176,32,177,176,32,32,32,32,176,32,32,32,32,176,176,177,176,32,176,32,176,32,32,177,32,176,32,32,32,32,176,32,32,32,32,32,177,32,176,32,32,176,32,177,32,177,176,32,176,32,176,176,32,32,32,176,32,177,176,32,32
    ; Row 8: " ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░ "
    .byte 32,177,32,176,32,32,32,176,32,32,32,176,32,176,32,32,32,176,32,32,32,32,32,32,32,176,176,176,32,176,32,176,32,32,177,32,176,32,32,176,32,32,32,32,32,32,32,177,32,176,176,32,176,32,176,32,177,32,32,32,32,32,176,32,32,32,176,32,176,32,32,32
    ; Row 9: " ░           ░             ░      ░            ░      ░ ░           ░ "
    .byte 32,176,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32,32,176,32,32,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32,32,176,32,176,32,32,32,32,32,32,32,32,32,32,32,176,32,32,32,32,32

; --- Info Text Strings ---
str_music:
    .ascii "MUSIC: WOOBOODOO BY KWIATEK & SYCHOWICZ",0
str_system:
    .ascii "Z80 CODE + VGA MODE 13H + ATARI POKEY AUDIO",0
str_demo:
    .ascii "INTUITION ENGINE 2026",0
str_greets:
    .ascii "GREETS TO THE SCENE!",0

; --- Scroll Message ---
; The message is padded with spaces at start and end for clean looping.
; This creates a pause before the message begins and after it ends,
; mimicking classic demo scrollers.
scroll_msg:
    .ascii "                                        "
    .ascii "WELCOME TO THE INTUITION ENGINE...   "
    .ascii "FEATURING IE32 RISC, Z80, 6502, AND 68020 CPU EMULATION   "
    .ascii "WITH VGA GRAPHICS AND PSG (AY/YM), SID, POKEY, TED SOUND CHIPS!   "
    .ascii "KEEP THE DEMOSCENE SPIRIT ALIVE!!!   "
    .ascii "VISIT WWW.INTUITIONSUBSYNTH.COM   "
    .ascii "                                        "
scroll_end:

; ============================================================================
; EMBEDDED SAP MUSIC DATA
; ============================================================================
; WooBooDoo by Grzegorz Kwiatek & Lukasz Sychowicz
;
; SAP FILE FORMAT:
; SAP files begin with a text header containing metadata:
;   SAP\r\n
;   AUTHOR "name"\r\n
;   NAME "title"\r\n
;   TYPE B/C/D/S\r\n
;   etc.
;
; Followed by 0xFF 0xFF marker and binary data containing:
;   - 6502 machine code for the player routine
;   - Music data (patterns, instruments, sequences)
;
; The Intuition Engine's SAP player parses this format and executes
; the embedded 6502 code, converting POKEY register writes to audio.
; ============================================================================
    .org 0xE000

sap_data:
    .incbin "../WooBooDoo.sap"
sap_data_end:

; ============================================================================
; END OF DEMO
; ============================================================================
; This demo demonstrates:
;   - Z80 assembly programming on the Intuition Engine
;   - VGA text mode (Mode 03h) graphics techniques
;   - POKEY+ audio playback for authentic Atari sound
;   - Classic demoscene effects: raster bars, color cycling, scrollers
;   - Sine table animation for smooth motion
;   - CP437 character art using shade blocks
;
; Keep the demoscene spirit alive - code something creative today!
; ============================================================================
