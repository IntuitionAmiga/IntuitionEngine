; ============================================================================
; TED 121-COLOR PLASMA DEMO FOR M68020
; "Luminance Dreams"
; ============================================================================
;
; REFERENCE IMPLEMENTATION FOR DEMOSCENE TECHNIQUES
; This file is heavily commented to teach demo programming concepts.
;
; === WHAT THIS DEMO DOES ===
; 1. Displays a full-screen animated plasma effect using all 121 TED colors
; 2. Renders a rainbow-colored title bar with cycling colors
; 3. Shows a smooth pixel-scrolling message with hardware XSCROLL
; 4. Cycles the border through all 121 unique TED colors
; 5. Plays Spectrum AY tracker music through the PSG+ audio subsystem
;
; === WHY THESE EFFECTS MATTER (HISTORICAL CONTEXT) ===
;
; THE COMMODORE PLUS/4 AND TED CHIP:
; The Commodore Plus/4 (1984) used the TED (Text Editing Device) chip, which
; combined video, sound, and I/O in a single IC. While often overshadowed by
; the C64's SID and VIC-II, the TED had one standout feature: 121 COLORS!
;
; Unlike the C64's 16 fixed colors, TED offered 16 hues × 8 luminance levels,
; giving artists unprecedented color choice for an 8-bit machine. However,
; the Plus/4's 1.76 MHz 6502 CPU was too slow to exploit this potential for
; complex real-time effects.
;
; THE 68020 ADVANTAGE:
; This demo showcases what COULD have been possible with TED if paired with
; a more powerful CPU. The M68020 offers:
;   - 32-bit registers and operations
;   - Hardware multiply (MULU/MULS) - the 6502 has NO multiply instruction!
;   - Faster addressing modes and larger address space
;   - Enough power to update all 1000 screen cells at 60 FPS
;
; PLASMA EFFECTS:
; Plasma is a classic demoscene effect that creates organic, flowing patterns
; by combining multiple sine waves. On 8-bit hardware, it typically required:
;   - Extensive pre-calculation and lookup tables
;   - Careful optimization to update even a fraction of the screen
;   - Often running at 15-30 FPS or lower
;
; This demo updates ALL 1000 character cells EVERY FRAME at 60 FPS - something
; absolutely impossible on the original Plus/4 hardware.
;
; === ARCHITECTURE OVERVIEW ===
;
;   ┌─────────────────────────────────────────────────────────────┐
;   │                    MAIN LOOP (60 FPS)                       │
;   │                                                             │
;   │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐      │
;   │  │  WAIT FOR   │───►│   UPDATE    │───►│   RENDER    │      │
;   │  │   VBLANK    │    │   TIMERS    │    │   PLASMA    │      │
;   │  └─────────────┘    └─────────────┘    └─────────────┘      │
;   │                                              │              │
;   │  ┌─────────────┐    ┌─────────────┐          │              │
;   │  │   UPDATE    │◄───│   UPDATE    │◄─────────┘              │
;   │  │  SCROLLER   │    │   BORDER    │                         │
;   │  └─────────────┘    └─────────────┘                         │
;   │         │                                                   │
;   │         ▼                                                   │
;   │  ┌─────────────┐                                            │
;   │  │   DRAW      │                                            │
;   │  │   TITLE     │                                            │
;   │  └─────────────┘                                            │
;   └─────────────────────────────────────────────────────────────┘
;
;   ┌─────────────────────────────────────────────────────────────┐
;   │              PSG+ AUDIO ENGINE (runs in parallel)           │
;   │                                                             │
;   │  The PSG+ synthesizer plays AY tracker music autonomously.  │
;   │  Once started, it runs in the audio subsystem with no CPU   │
;   │  overhead. The AY chip was used in the ZX Spectrum 128K     │
;   │  and Atari ST - this creates an impossible hybrid machine!  │
;   └─────────────────────────────────────────────────────────────┘
;
; === TED COLOR SYSTEM EXPLAINED ===
;
; TED colors are encoded in a single byte:
;
;   Bits 7-4: Luminance (0-7, only 3 bits used, bit 7 ignored)
;   Bits 3-0: Hue (0-15)
;
;   Color byte = (luminance << 4) | hue
;
; The 16 hues are:
;   0: Black (special - ignores luminance, always black)
;   1: White
;   2: Red
;   3: Cyan
;   4: Purple
;   5: Green
;   6: Blue
;   7: Yellow
;   8: Orange
;   9: Brown
;  10: Yellow-Green
;  11: Pink
;  12: Blue-Green
;  13: Light Blue
;  14: Dark Blue
;  15: Light Green
;
; Since hue 0 is always black regardless of luminance, we get:
;   1 black + (15 hues × 8 luminances) = 1 + 120 = 121 unique colors
;
; (c) 2026 Zayn Otley
; License: GPLv3 or later
; ============================================================================

    include "ie68.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- TED Video RAM Addresses ---
; The TED uses a simple linear framebuffer for text mode:
;   - Character RAM: 1000 bytes (40 columns × 25 rows)
;   - Color RAM: 1000 bytes (one color byte per character cell)
;
; In the Intuition Engine, TED VRAM is mapped to $F3000:
TED_VRAM_BASE    equ $F3000          ; Start of character RAM
TED_COLOR_RAM    equ $F3400          ; Start of color RAM (+$400 = +1024)

; --- Display Geometry ---
; TED text mode is 40×25 characters, each character is 8×8 pixels.
; Total display: 320×200 pixels (same as C64, Amiga OCS, VGA Mode 13h).
SCREEN_COLS      equ 40              ; Horizontal characters
SCREEN_ROWS      equ 25              ; Vertical characters
TOTAL_CELLS      equ 1000            ; 40 × 25 = 1000 character cells

; --- Plasma Character ---
; We fill the screen with a solid block character and animate its COLOR,
; not its shape. This is the classic "color cycling" technique.
; Character $80 in the TED character set is a solid 8×8 block.
SOLID_BLOCK      equ $80

; --- Animation Variables ---
; These are stored in RAM (not in the program's data section) because
; they're modified every frame. We use fixed addresses for simplicity.
;
; The plasma effect uses four independent time counters, each advancing
; at a different rate. This creates the complex, non-repeating patterns
; characteristic of good plasma effects.
frame_count      equ $1000           ; Master frame counter
plasma_time1     equ $1004           ; Time offset for sine wave 1
plasma_time2     equ $1008           ; Time offset for sine wave 2
plasma_time3     equ $100C           ; Time offset for sine wave 3
plasma_time4     equ $1010           ; Time offset for sine wave 4 (radial)
scroll_fine      equ $1014           ; Fine scroll position (0-7)
scroll_char      equ $1018           ; Character index into scroll message

    org PROGRAM_START

; ============================================================================
; ENTRY POINT
; ============================================================================
; The CPU begins execution here after loading the program.
; We initialize all hardware subsystems before entering the main loop.
; ============================================================================
start:
    ; --- Initialize Stack Pointer ---
    ; The M68K uses a descending stack (grows toward lower addresses).
    ; STACK_TOP is defined in ie68.inc and points to high memory.
    ; This MUST be set before any subroutine calls (BSR/JSR).
    move.l  #STACK_TOP,sp

    ; --- Enable TED Video Output ---
    ; The TED chip must be explicitly enabled. Without this, the display
    ; shows nothing. TED_V_ENABLE is the master enable register.
    ; TED_V_ENABLE_VIDEO = 1 (bit 0 enables video output)
    move.b  #TED_V_ENABLE_VIDEO,TED_V_ENABLE

    ; --- Configure TED for 40×25 Text Mode ---
    ; TED_V_CTRL1 controls vertical parameters:
    ;   Bit 4 (DEN): Display Enable (1 = display on)
    ;   Bit 3 (RSEL): Row Select (1 = 25 rows, 0 = 24 rows)
    ;   Bits 2-0: Vertical fine scroll (0-7)
    ; $18 = %00011000 = DEN=1, RSEL=1, YSCROLL=0
    move.b  #$18,TED_V_CTRL1

    ; TED_V_CTRL2 controls horizontal parameters:
    ;   Bit 4 (MCM): Multi-Color Mode (0 = normal)
    ;   Bit 3 (CSEL): Column Select (1 = 40 cols, 0 = 38 cols)
    ;   Bits 2-0: Horizontal fine scroll (0-7) - USED BY SCROLLER!
    ; $08 = %00001000 = MCM=0, CSEL=1, XSCROLL=0
    move.b  #$08,TED_V_CTRL2

    ; --- Set Character and Video Base Addresses ---
    ; These registers select which part of memory holds the character
    ; set and video matrix. $00 selects the default ROM character set
    ; and the default screen memory location.
    move.b  #$00,TED_V_CHAR_BASE
    move.b  #$00,TED_V_VIDEO_BASE

    ; --- Set Background and Border Colors ---
    ; Background color 0 is used for the "paper" behind text.
    ; We start with black background ($00) and white border ($71).
    ; The border color will be animated through all 121 colors.
    move.b  #$00,TED_V_BG_COLOR0
    move.b  #$71,TED_V_BORDER

    ; --- Initialize Animation Variables ---
    ; clr.l (clear long) is faster than move.l #0 on most 68K variants.
    ; Starting all timers at 0 ensures consistent initial pattern.
    clr.l   frame_count
    clr.l   plasma_time1
    clr.l   plasma_time2
    clr.l   plasma_time3
    clr.l   plasma_time4
    clr.l   scroll_fine
    clr.l   scroll_char

    ; --- Initialize Screen Content ---
    ; Fill all 1000 character cells with solid blocks.
    ; The plasma effect will animate the COLOR RAM, not the characters.
    bsr     init_screen

    ; --- Enable IE Video System ---
    ; The Intuition Engine video compositor must be enabled.
    ; This is separate from the TED enable - both are required.
    move.l  #1,VIDEO_CTRL

    ; --- Start Music Playback ---
    ; Initialize the PSG+ audio engine with our AY tracker module.
    ; Music plays autonomously once started - no CPU overhead.
    ;
    ; PSG_PLUS_CTRL = 1: Enable PSG+ enhanced mode
    ; PSG_PLAY_PTR: 32-bit pointer to music data
    ; PSG_PLAY_LEN: 32-bit length of music data
    ; PSG_PLAY_CTRL = 5: Start (bit 0) + Loop (bit 2)
    move.b  #1,PSG_PLUS_CTRL
    move.l  #music_data,PSG_PLAY_PTR
    move.l  #music_data_end-music_data,PSG_PLAY_LEN
    move.l  #5,PSG_PLAY_CTRL         ; Start playback with looping

; ============================================================================
; MAIN LOOP
; ============================================================================
; This loop runs once per frame (60 times per second with VSYNC).
; The order of operations is carefully chosen:
;
; 1. Wait for VSYNC (prevents tearing, ensures consistent timing)
; 2. Update plasma timers (animation parameters)
; 3. Render plasma (THE main CPU-intensive operation)
; 4. Update border color (cosmetic effect)
; 5. Draw title overlay (text on top of plasma)
; 6. Update scroller (smooth horizontal text scroll)
; 7. Increment frame counter
; 8. Repeat forever
;
; Total work per frame: ~1000 color RAM writes + scrolling logic
; This would take ~500,000+ cycles on a 6502 - impossible at 60 FPS!
; ============================================================================
main_loop:
    ; === VSYNC SYNCHRONIZATION ===
    ; We wait for vertical blank to prevent "tearing" (seeing a
    ; partially-drawn frame). The TED updates the display from memory
    ; continuously; by waiting for VBLANK, we ensure our writes complete
    ; before the next frame is displayed.
    bsr     wait_vblank

    ; === UPDATE PLASMA TIMERS ===
    ; Each timer advances at a different rate (prime-ish numbers work best).
    ; Different speeds create complex interference patterns that don't
    ; repeat for a very long time.
    ;
    ; Timer 1: +3 per frame (horizontal wave influence)
    ; Timer 2: +2 per frame (vertical wave influence)
    ; Timer 3: +5 per frame (diagonal wave influence)
    ; Timer 4: +1 per frame (radial wave from center)
    move.l  plasma_time1,d0
    addq.l  #3,d0
    move.l  d0,plasma_time1

    move.l  plasma_time2,d0
    addq.l  #2,d0
    move.l  d0,plasma_time2

    move.l  plasma_time3,d0
    addq.l  #5,d0
    move.l  d0,plasma_time3

    move.l  plasma_time4,d0
    addq.l  #1,d0
    move.l  d0,plasma_time4

    ; === RENDER PLASMA ===
    ; This is the main event - update all 1000 color cells.
    ; See render_plasma for the algorithm details.
    bsr     render_plasma

    ; === UPDATE BORDER COLOR ===
    ; Cycle the border through all 121 TED colors for visual flair.
    bsr     update_border

    ; === DRAW TITLE OVERLAY ===
    ; Write the title text and its rainbow colors over the plasma.
    bsr     draw_title_overlay

    ; === UPDATE SCROLLER ===
    ; Advance the smooth-scrolling message on row 24.
    bsr     update_scroller

    ; === INCREMENT FRAME COUNTER ===
    ; Used for various timing calculations throughout the demo.
    addq.l  #1,frame_count

    ; === LOOP FOREVER ===
    bra     main_loop

; ============================================================================
; WAIT FOR VERTICAL BLANK
; ============================================================================
; Polls the TED status register until the VBLANK period begins.
;
; WHY TWO LOOPS?
; The status bit indicates whether we're currently IN vblank, but we want
; to catch the START of vblank (the transition from active to blank).
;
; 1. First loop: Wait while ALREADY in vblank (handle case where we're
;    still in vblank from the previous frame)
; 2. Second loop: Wait until vblank BEGINS (catch the rising edge)
;
; This ensures exactly one frame per main loop iteration.
;
; TED_V_STATUS bit 0: VBLANK status (1 = in vblank, 0 = active display)
; Reading the status register clears the flag (acknowledge).
; ============================================================================
wait_vblank:
    ; Wait while already in vblank (exit vblank period)
.not_vb:
    move.b  TED_V_STATUS,d0
    andi.b  #TED_V_STATUS_VBLANK,d0
    bne.s   .not_vb

    ; Wait until vblank starts (enter new vblank period)
.wait_vb:
    move.b  TED_V_STATUS,d0
    andi.b  #TED_V_STATUS_VBLANK,d0
    beq.s   .wait_vb
    rts

; ============================================================================
; INITIALIZE SCREEN WITH SOLID BLOCKS
; ============================================================================
; Fills all 1000 character cells with the solid block character ($80).
; This creates a uniform surface where we can animate colors freely.
;
; WHY SOLID BLOCKS?
; By using a solid block character, every pixel in each 8×8 cell shows
; the foreground color. The plasma effect then becomes pure color animation
; without any character shape interference.
;
; This is much faster than updating actual pixel data - we write 1000 bytes
; of color RAM instead of 8000 bytes of bitmap data!
; ============================================================================
init_screen:
    lea     TED_VRAM_BASE,a0        ; a0 = pointer to character RAM
    move.w  #TOTAL_CELLS-1,d0       ; d0 = loop counter (dbf needs N-1)
.fill:
    move.b  #SOLID_BLOCK,(a0)+      ; Write solid block character
    dbf     d0,.fill                ; Decrement and branch if not -1
    rts

; ============================================================================
; RENDER PLASMA EFFECT
; ============================================================================
; The heart of the demo: calculates and writes colors for all 1000 cells.
;
; === THE PLASMA ALGORITHM ===
;
; For each cell at position (x, y), we calculate four sine wave contributions:
;
;   v1 = sin(x*4 + time1)         ; Horizontal wave
;   v2 = sin(y*4 + time2)         ; Vertical wave
;   v3 = sin((x+y)*3 + time3)     ; Diagonal wave
;   v4 = sin(dist(x,y)*2 + time4) ; Radial wave from center
;
; The sum of these waves creates the plasma pattern:
;   plasma_value = v1 + v2 + v3 + v4
;
; This value is then mapped to a TED color by extracting bits for hue
; and luminance.
;
; === WHY FOUR WAVES? ===
;
; - One wave = boring stripes
; - Two waves = simple interference (plaid pattern)
; - Three waves = more complex, but still predictable
; - Four waves = rich, organic patterns
;
; The radial wave (v4) adds a "spotlight" effect centered on the screen,
; breaking up the regular grid pattern of the other three waves.
;
; === MANHATTAN DISTANCE ===
;
; For the radial wave, we use Manhattan distance (|x-cx| + |y-cy|) instead
; of Euclidean distance (sqrt((x-cx)² + (y-cy)²)). This is MUCH faster:
;   - No multiplication (expensive on 68K)
;   - No square root (very expensive everywhere)
;   - Creates diamond-shaped patterns instead of circular
;
; The visual difference is subtle and the speed gain is enormous.
;
; === REGISTER USAGE ===
;
; a0 = Color RAM pointer (advances through all 1000 cells)
; a1 = Sine table base address (constant)
; a2 = plasma_time4 value (we ran out of data registers!)
; d0, d1, d2 = Working registers for calculations
; d3 = X coordinate (0-39)
; d4, d5, d6 = Cached time values (time1, time2, time3)
; d7 = Y coordinate (0-24)
;
; === PERFORMANCE ===
;
; Per cell: ~4 table lookups + ~30 arithmetic operations
; Per frame: 1000 cells × ~50 operations = ~50,000 operations
; At 60 FPS: ~3,000,000 operations/second
;
; On a 1.76 MHz 6502: Would need ~500,000+ cycles per frame
; A 6502 frame is only ~29,000 cycles at 60 FPS - IMPOSSIBLE!
;
; On the 68020: Trivial. This is why the demo exists.
; ============================================================================
render_plasma:
    lea     TED_COLOR_RAM,a0        ; a0 = pointer to color RAM
    lea     sine_table,a1           ; a1 = sine table base

    ; Cache time values in registers for speed
    ; (Memory access is slower than register access)
    move.l  plasma_time1,d4
    move.l  plasma_time2,d5
    move.l  plasma_time3,d6
    move.l  plasma_time4,a2         ; Using a2 as a data register!

    moveq   #0,d7                   ; d7 = Y counter (0 to 24)

.row_loop:
    moveq   #0,d3                   ; d3 = X counter (0 to 39)

.col_loop:
    ; ============================================================
    ; WAVE 1: Horizontal wave - sin(x*4 + time1)
    ; This creates vertical stripes that move horizontally.
    ; Contributes to HUE calculation.
    ; ============================================================
    move.l  d3,d0                   ; d0 = x
    lsl.l   #2,d0                   ; d0 = x * 4 (frequency multiplier)
    add.l   d4,d0                   ; d0 = x*4 + time1 (phase offset)
    andi.l  #$FF,d0                 ; Wrap to table size (0-255)
    move.b  (a1,d0.l),d1            ; Look up sine value
    ext.w   d1                      ; Sign-extend byte to word
    ext.l   d1                      ; Sign-extend word to long
    ; d1 now contains v1 (-128 to +127, sign-extended to 32 bits)

    ; ============================================================
    ; WAVE 2: Vertical wave - sin(y*4 + time2)
    ; This creates horizontal stripes that move vertically.
    ; Combined with wave 1, we get a "plaid" base pattern for HUE.
    ; ============================================================
    move.l  d7,d0                   ; d0 = y
    lsl.l   #2,d0                   ; d0 = y * 4
    add.l   d5,d0                   ; d0 = y*4 + time2
    andi.l  #$FF,d0
    move.b  (a1,d0.l),d2
    ext.w   d2
    ext.l   d2
    add.l   d2,d1                   ; d1 = v1 + v2 (used for HUE)

    ; ============================================================
    ; WAVE 3: Diagonal wave - sin((x+y)*3 + time3)
    ; This creates diagonal stripes at 45 degrees.
    ; NOTE: MULU is a hardware multiply - the 6502 has no equivalent!
    ; Contributes to LUMINANCE calculation (stored in a3).
    ; ============================================================
    move.l  d3,d0                   ; d0 = x
    add.l   d7,d0                   ; d0 = x + y
    mulu.w  #3,d0                   ; d0 = (x+y) * 3 - HARDWARE MULTIPLY!
    add.l   d6,d0                   ; d0 = (x+y)*3 + time3
    andi.l  #$FF,d0
    move.b  (a1,d0.l),d2
    ext.w   d2
    ext.l   d2
    move.l  d2,a3                   ; a3 = v3 (start of luminance sum)

    ; ============================================================
    ; WAVE 4: Radial wave - sin(dist*2 + time4)
    ; This creates expanding rings from the screen center.
    ; Uses Manhattan distance for speed: dist = |x-20| + |y-12|
    ; Contributes to LUMINANCE calculation.
    ; ============================================================
    ; Calculate |x - center_x| where center_x = 20
    move.l  d3,d0                   ; d0 = x
    subi.l  #20,d0                  ; d0 = x - 20
    bpl.s   .pos_x                  ; Branch if positive
    neg.l   d0                      ; Make positive (absolute value)
.pos_x:

    ; Calculate |y - center_y| where center_y = 12
    move.l  d7,d2                   ; d2 = y
    subi.l  #12,d2                  ; d2 = y - 12
    bpl.s   .pos_y
    neg.l   d2
.pos_y:

    ; d0 = Manhattan distance from center
    add.l   d2,d0                   ; d0 = |x-20| + |y-12|
    lsl.l   #1,d0                   ; d0 = dist * 2 (frequency multiplier)
    move.l  a2,d2                   ; d2 = time4 (stored in a2)
    add.l   d2,d0                   ; d0 = dist*2 + time4
    andi.l  #$FF,d0
    move.b  (a1,d0.l),d2
    ext.w   d2
    ext.l   d2
    add.l   a3,d2                   ; d2 = v3 + v4 (LUMINANCE sum)

    ; ============================================================
    ; MAP PLASMA VALUE TO TED COLOR - ALL 121 COLORS!
    ; ============================================================
    ; To use ALL 121 TED colors, hue and luminance must vary
    ; INDEPENDENTLY. We achieve this by using DIFFERENT wave
    ; combinations for each:
    ;
    ;   HUE       = f(v1 + v2) = horizontal + vertical waves
    ;   LUMINANCE = f(v3 + v4) = diagonal + radial waves
    ;
    ; This ensures hue and luminance are mathematically independent,
    ; allowing the full 16 × 8 = 128 color combinations to appear.
    ; (Actually 121 unique due to hue 0 always being black.)

    ; --- Calculate HUE from v1 + v2 ---
    ; d1 contains v1+v2 (range -256 to +256)
    addi.l  #256,d1                 ; Shift to 0-512 range
    lsr.l   #5,d1                   ; Divide by 32 → 0-15 range
    andi.l  #$0F,d1                 ; Ensure 4 bits (hue 0-15)
    move.l  d1,d0                   ; d0 = hue

    ; --- Calculate LUMINANCE from v3 + v4 ---
    ; d2 contains v3+v4 (range -256 to +256)
    addi.l  #256,d2                 ; Shift to 0-512 range
    lsr.l   #6,d2                   ; Divide by 64 → 0-7 range
    andi.l  #$07,d2                 ; Ensure 3 bits (luminance 0-7)

    ; Combine into TED color byte: (luminance << 4) | hue
    lsl.l   #4,d2                   ; Shift luminance to upper nibble
    or.l    d0,d2                   ; Combine with hue

    ; Avoid pure black (hue 0 is special - always black)
    ; Replace with dark white ($11) for better visual
    tst.b   d2
    bne.s   .not_black
    moveq   #$11,d2                 ; Dark white instead of black
.not_black:

    ; Write color to color RAM
    move.b  d2,(a0)+                ; Store and advance pointer

    ; === NEXT COLUMN ===
    addq.l  #1,d3
    cmpi.l  #SCREEN_COLS,d3
    blt     .col_loop

    ; === NEXT ROW ===
    addq.l  #1,d7
    cmpi.l  #SCREEN_ROWS,d7
    blt     .row_loop

    rts

; ============================================================================
; UPDATE BORDER COLOR
; ============================================================================
; Cycles the border through all 121 unique TED colors.
;
; === THE 121 COLOR CYCLING ===
;
; TED has 128 possible color byte values (8 luminances × 16 hues), but
; only 121 are visually unique because hue 0 (black) ignores luminance.
;
; We cycle through all 128 combinations anyway (simpler code), but remap
; the "black at non-zero luminance" cases to white, avoiding duplicate blacks.
;
; Cycling rate: One color change every 8 frames = 128 colors × 8 frames
;             = 1024 frames = ~17 seconds for a complete cycle at 60 FPS.
;
; === COLOR ORDERING ===
;
; Rather than cycling the raw color byte (which would give jerky luminance
; jumps), we cycle HUE first (0-15) then increment LUMINANCE (0-7).
; This creates a smooth rainbow sweep at each brightness level.
; ============================================================================
update_border:
    move.l  frame_count,d0
    lsr.l   #3,d0                   ; Slow down: change every 8 frames
    andi.l  #$7F,d0                 ; Keep in range 0-127

    ; Extract hue (cycles 0-15 for each luminance level)
    move.l  d0,d1
    andi.l  #$0F,d1                 ; d1 = hue (bits 0-3)

    ; Extract luminance (increments every 16 hue cycles)
    move.l  d0,d2
    lsr.l   #4,d2                   ; Shift right 4 bits
    andi.l  #$07,d2                 ; d2 = luminance (bits 0-2)

    ; Combine into TED color byte
    lsl.l   #4,d2                   ; Shift luminance to bits 4-6
    or.l    d1,d2                   ; Combine with hue

    ; Handle the "duplicate black" problem:
    ; Hue 0 is black regardless of luminance, so hue 0 + lum > 0 would
    ; show the same color as hue 0 + lum 0. Replace with white (hue 1).
    tst.b   d1                      ; Is hue 0?
    bne.s   .border_ok
    tst.b   d2                      ; Is combined color 0 (true black)?
    beq.s   .border_ok              ; True black is fine
    ; Hue 0 but luminance > 0: use white hue instead
    moveq   #$01,d1                 ; Hue 1 = white
    andi.l  #$70,d2                 ; Keep luminance bits
    or.l    d1,d2                   ; Combine with white hue
.border_ok:
    move.b  d2,TED_V_BORDER
    rts

; ============================================================================
; DRAW TITLE OVERLAY
; ============================================================================
; Writes the demo title text and applies rainbow color cycling.
;
; The title is drawn AFTER the plasma, so it appears on top.
; Each character gets a different color based on its position and the
; current frame, creating a scrolling rainbow effect.
;
; This overwrites 34 characters starting at column 3 of row 0.
; ============================================================================
draw_title_overlay:
    ; === WRITE TITLE CHARACTERS ===
    ; Copy the title string to VRAM at row 0, column 3
    lea     TED_VRAM_BASE+3,a0      ; Start at column 3
    lea     title_text,a1
.copy_title:
    move.b  (a1)+,d0
    beq.s   .title_done             ; Stop at null terminator
    move.b  d0,(a0)+
    bra.s   .copy_title
.title_done:

    ; === APPLY RAINBOW COLORS ===
    ; Each character gets a color based on (frame_count + position) / 2
    ; This creates a rainbow wave effect moving across the title.
    lea     TED_COLOR_RAM+3,a0
    move.l  frame_count,d1
    moveq   #33,d0                  ; 34 characters (0-33)
.title_color:
    move.l  d1,d2
    add.l   d0,d2                   ; Offset by character position
    lsr.l   #1,d2                   ; Slow down color cycling
    andi.l  #$0F,d2                 ; Hue 0-15
    beq.s   .title_white            ; Avoid hue 0 (black)
    ori.l   #$70,d2                 ; Maximum luminance (7)
    bra.s   .title_set
.title_white:
    moveq   #$71,d2                 ; Bright white instead of black
.title_set:
    move.b  d2,(a0)+
    dbf     d0,.title_color
    rts

; ============================================================================
; SMOOTH PIXEL SCROLLER
; ============================================================================
; Implements a classic demoscene horizontal scroller on row 24.
;
; === HOW SMOOTH SCROLLING WORKS ===
;
; Text scrollers can move at two levels:
;   1. CHARACTER SCROLL: Move text left by whole characters (8 pixels)
;   2. FINE SCROLL: Move text left by individual pixels (0-7)
;
; By combining both, we get smooth pixel-by-pixel movement:
;   - Fine scroll (XSCROLL) handles sub-character movement
;   - When XSCROLL reaches 7 and wraps to 0, we shift all characters
;     left by one position and bring in a new character on the right
;
; === TED XSCROLL REGISTER ===
;
; TED_V_CTRL2 bits 0-2 control horizontal fine scroll (0-7 pixels).
; The display is shifted RIGHT by this many pixels, which visually
; moves content LEFT. So XSCROLL=7 is the leftmost position before
; a character shift is needed.
;
; === SCROLLER IMPLEMENTATION ===
;
; Each frame:
;   1. Increment fine scroll (0-7)
;   2. If fine scroll wrapped past 7:
;      a. Reset fine scroll to 0
;      b. Shift all characters on row 24 left by 1
;      c. Put next message character at rightmost position
;   3. Set TED XSCROLL register
;   4. Apply rainbow colors to row 24
;
; === REGISTER PRESERVATION ===
;
; This routine is called from the main loop, which uses various registers.
; We save and restore all registers we modify to avoid corruption.
; ============================================================================
update_scroller:
    movem.l d0-d3/a0-a2,-(sp)       ; Save registers on stack

    ; === UPDATE FINE SCROLL POSITION ===
    move.l  scroll_fine,d0
    addq.l  #1,d0                   ; Advance by 1 pixel
    cmpi.l  #8,d0                   ; Wrapped past 7?
    blt.s   .no_char_scroll         ; No, just update XSCROLL

    ; === CHARACTER SCROLL NEEDED ===
    ; Fine scroll wrapped from 7 to 8, reset to 0 and shift characters
    clr.l   d0                      ; Reset fine scroll to 0

    ; Shift all 40 characters on row 24 left by one position
    ; This copies character N+1 to position N for N = 0 to 38
    lea     TED_VRAM_BASE+(24*40),a0    ; Row 24, column 0
    lea     1(a0),a1                     ; Source = column 1
    moveq   #38,d1                       ; Copy 39 characters (0-38)
.shift_loop:
    move.b  (a1)+,(a0)+
    dbf     d1,.shift_loop

    ; Bring in next character from scroll message at rightmost position
    move.l  scroll_char,d1
    lea     scroll_text,a1
    move.b  (a1,d1.l),d2            ; Load next character
    bne.s   .have_char              ; Not null? Use it
    ; End of message - wrap to beginning
    clr.l   d1                      ; Reset to start of message
    move.b  (a1),d2                 ; Load first character
.have_char:
    move.b  d2,TED_VRAM_BASE+(24*40)+39  ; Write to rightmost column
    addq.l  #1,d1                   ; Advance message index
    move.l  d1,scroll_char          ; Save for next frame

.no_char_scroll:
    move.l  d0,scroll_fine          ; Store fine scroll position

    ; === SET TED XSCROLL REGISTER ===
    ; XSCROLL value is inverted: 0 = no shift, 7 = max shift
    ; We want: fine=0 → XSCROLL=7 (start), fine=7 → XSCROLL=0 (end)
    move.l  #7,d1
    sub.l   d0,d1                   ; Invert: 7 - fine
    andi.l  #$07,d1                 ; Ensure 3 bits
    ori.l   #$08,d1                 ; Keep CSEL bit set (40 columns)
    move.b  d1,TED_V_CTRL2

    ; === APPLY RAINBOW COLORS TO SCROLLER ROW ===
    ; Same technique as the title bar
    lea     TED_COLOR_RAM+(24*40),a0
    move.l  frame_count,d1
    moveq   #39,d0                  ; 40 columns
.scroll_color:
    move.l  d1,d2
    add.l   d0,d2                   ; Offset by column position
    lsr.l   #1,d2                   ; Slow down cycling
    andi.l  #$0F,d2                 ; Hue 0-15
    beq.s   .scroll_white           ; Avoid hue 0 (black)
    ori.l   #$70,d2                 ; Maximum luminance
    bra.s   .scroll_set
.scroll_white:
    moveq   #$71,d2                 ; Bright white
.scroll_set:
    move.b  d2,(a0)+
    dbf     d0,.scroll_color

    movem.l (sp)+,d0-d3/a0-a2       ; Restore registers
    rts

; ============================================================================
; DATA SECTION
; ============================================================================
; All static data used by the demo.
; ============================================================================

; ============================================================================
; TITLE TEXT
; ============================================================================
; The demo title displayed on row 0.
; 34 characters centered on the 40-column display (3 chars margin each side).
; ============================================================================
title_text:
    dc.b    "=== TED PLASMA - 68020 POWER! ===",0

; ============================================================================
; SCROLL MESSAGE
; ============================================================================
; The text that scrolls across row 24.
;
; === MESSAGE DESIGN ===
;
; - Starts and ends with spaces to allow clean entry/exit
; - Multiple sentences for variety
; - Credits, greetings, and technical explanations (demoscene tradition)
; - Null-terminated for end detection
;
; The scroller wraps seamlessly when it reaches the end.
; ============================================================================
scroll_text:
    dc.b    "       "
    dc.b    "WELCOME TO THE TED 121-COLOUR 68K PLASMA DEMO!   "
    dc.b    "THIS EFFECT WOULD BE IMPOSSIBLE ON THE ORIGINAL PLUS/4 OR C16...   "
    dc.b    "THE 1.76MHZ 6502 COULD NEVER UPDATE 1000 CELLS AT 60FPS!   "
    dc.b    "BUT THE MIGHTY 68020 DOES IT WITH EASE...   "
    dc.b    "DUE TO HARDWARE MULTIPLY (MULU) AND 32-BIT REGISTERS...   "
    dc.b    "SCROLLTEXT ROUTINE USES TED HARDWARE SCROLLING (XSCROLL)...   "
    dc.b    "THE BORDER CYCLES THROUGH ALL 121 COLOURS TOO...   "
    dc.b    "GREETINGS TO ALL DEMOSCENERS!   "
    dc.b    "MUSIC: PLATOON BY ROB HUBBARD (AY VERSION)...   "
    dc.b    "ASM CODE: INTUITION 2026...   "
    dc.b    "VISIT INTUITIONSUBSYNTH.COM   "
    dc.b    "                                        ",0

    even                            ; Ensure word alignment

; ============================================================================
; SINE TABLE (256 ENTRIES, SIGNED 8-BIT VALUES)
; ============================================================================
; Pre-computed sine wave for fast lookup.
;
; === TABLE FORMAT ===
;
; 256 entries representing one complete sine wave cycle.
; Each entry is a signed byte: -127 to +127 (approximately -1.0 to +1.0).
;
; Index 0   = sin(0°)    = 0
; Index 64  = sin(90°)   = 127 (maximum)
; Index 128 = sin(180°)  = 0
; Index 192 = sin(270°)  = -127 (minimum)
; Index 255 = sin(~360°) = -3 (almost back to 0)
;
; === WHY A LOOKUP TABLE? ===
;
; Computing sine requires Taylor series or CORDIC - expensive operations!
; A 256-byte table gives instant results with a single memory read.
; This is the classic demoscene trade-off: memory for speed.
;
; === FORMULA USED ===
;
; For index i (0-255):
;   value = round(sin(i × 2π / 256) × 127)
;
; The values were pre-computed and stored as signed bytes.
; ============================================================================
sine_table:
    ; Quadrant 1: 0° to 90° (indices 0-63, values rise 0 to 127)
    dc.b      0,   3,   6,   9,  12,  15,  18,  21
    dc.b     24,  27,  30,  33,  36,  39,  42,  45
    dc.b     48,  51,  54,  57,  59,  62,  65,  67
    dc.b     70,  73,  75,  78,  80,  82,  85,  87
    dc.b     89,  91,  94,  96,  98, 100, 102, 103
    dc.b    105, 107, 108, 110, 112, 113, 114, 116
    dc.b    117, 118, 119, 120, 121, 122, 123, 123
    dc.b    124, 125, 125, 126, 126, 126, 126, 126

    ; Quadrant 2: 90° to 180° (indices 64-127, values fall 127 to 0)
    dc.b    127, 126, 126, 126, 126, 126, 125, 125
    dc.b    124, 123, 123, 122, 121, 120, 119, 118
    dc.b    117, 116, 114, 113, 112, 110, 108, 107
    dc.b    105, 103, 102, 100,  98,  96,  94,  91
    dc.b     89,  87,  85,  82,  80,  78,  75,  73
    dc.b     70,  67,  65,  62,  59,  57,  54,  51
    dc.b     48,  45,  42,  39,  36,  33,  30,  27
    dc.b     24,  21,  18,  15,  12,   9,   6,   3

    ; Quadrant 3: 180° to 270° (indices 128-191, values fall 0 to -127)
    dc.b      0,  -3,  -6,  -9, -12, -15, -18, -21
    dc.b    -24, -27, -30, -33, -36, -39, -42, -45
    dc.b    -48, -51, -54, -57, -59, -62, -65, -67
    dc.b    -70, -73, -75, -78, -80, -82, -85, -87
    dc.b    -89, -91, -94, -96, -98,-100,-102,-103
    dc.b   -105,-107,-108,-110,-112,-113,-114,-116
    dc.b   -117,-118,-119,-120,-121,-122,-123,-123
    dc.b   -124,-125,-125,-126,-126,-126,-126,-126

    ; Quadrant 4: 270° to 360° (indices 192-255, values rise -127 to 0)
    dc.b   -127,-126,-126,-126,-126,-126,-125,-125
    dc.b   -124,-123,-123,-122,-121,-120,-119,-118
    dc.b   -117,-116,-114,-113,-112,-110,-108,-107
    dc.b   -105,-103,-102,-100, -98, -96, -94, -91
    dc.b    -89, -87, -85, -82, -80, -78, -75, -73
    dc.b    -70, -67, -65, -62, -59, -57, -54, -51
    dc.b    -48, -45, -42, -39, -36, -33, -30, -27
    dc.b    -24, -21, -18, -15, -12,  -9,  -6,  -3

    even                            ; Ensure word alignment

; ============================================================================
; MUSIC DATA - AY TRACKER MODULE
; ============================================================================
; Embedded music file for the PSG+ audio subsystem.
;
; === WHAT IS THIS MUSIC? ===
;
; "Platoon" by Rob Hubbard, originally composed for the Commodore 64
; game of the same name (1987). This is the ZX Spectrum AY chip version.
;
; Rob Hubbard is a legendary C64 musician who created some of the most
; memorable game soundtracks of the 8-bit era. His music pushed the
; SID chip to its limits and influenced an entire generation of musicians.
;
; === THE AY-3-8910 CHIP ===
;
; The AY chip was used in:
;   - ZX Spectrum 128K
;   - Atari ST
;   - Amstrad CPC
;   - MSX computers
;   - Many arcade machines
;
; It has 3 square wave channels plus noise, with hardware envelope
; generators. While simpler than the C64's SID, it has its own
; distinctive sound that's equally beloved by chiptune enthusiasts.
;
; === THE PSG+ ENGINE ===
;
; The Intuition Engine's PSG+ is an enhanced AY emulator that:
;   1. Parses AY tracker file format
;   2. Emulates the original AY-3-8910 registers
;   3. Generates audio samples in real-time
;   4. Runs autonomously - no CPU overhead after starting
;
; This creates an impossible combination: C64-era TED graphics with
; Spectrum-era AY music on a 68020 CPU that never existed in the 8-bit era!
;
; === REGISTER INTERFACE ===
;
; PSG_PLUS_CTRL:    Enable enhanced PSG mode
; PSG_PLAY_PTR:     32-bit pointer to AY file data
; PSG_PLAY_LEN:     32-bit length of file
; PSG_PLAY_CTRL:    Control (bit 0=start, bit 2=loop)
;
; Once started with looping enabled, the music plays forever.
; ============================================================================
music_data:
    incbin  "Platoon.ay"
music_data_end:

    end start
