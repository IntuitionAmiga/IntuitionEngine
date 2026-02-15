; ============================================================================
; VOODOO MEGA DEMO - Twisting Starfield Tunnel with Rainbow Scrolltext
; IE32 for IntuitionEngine - Voodoo 3D (640x480, triangle rasteriser)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:     IE32 (custom 32-bit RISC, unsigned arithmetic only)
; Video chip:     3DFX Voodoo SST-1 (hardware 3D, Z-buffer, triangle rasteriser)
; Audio engine:   SID (6502 CPU core + SID synthesiser)
; Assembler:      ie32asm (IntuitionEngine native assembler)
; Include file:   ie32.inc (Voodoo + SID player MMIO definitions)
;
; === WHAT THIS DEMO DOES ===
;   1. TWISTING STARFIELD TUNNEL
;      256 stars distributed in a cylindrical pattern rush toward the camera.
;      The tunnel centre oscillates via sine waves (Lissajous-like motion).
;      Stars are coloured by depth: white (close), cyan (medium), blue (far).
;
;   2. RAINBOW BITMAP SCROLLTEXT
;      Classic 5x7 bitmap font rendered as coloured Voodoo quads.
;      Horizontal scrolling with Y-axis sine-wave wobble.
;      Per-character rainbow colour cycling via phase-shifted sine waves.
;
;   3. SID MUSIC PLAYBACK
;      "Reggae 2" by Djinn/Fraction (1998) -- authentic 6502 code executed
;      on the IntuitionEngine's internal 6502 CPU core, driving the SID
;      synthesiser for genuine Commodore 64 sound.
;
; === WHY THESE EFFECTS ===
; The starfield tunnel, scrolltext, and chiptune music are the three
; pillars of the demoscene tradition that began on the Commodore 64 and
; Amiga in the mid-1980s.  Nearly every demo combined these elements:
;
;   - STARFIELD: evolved from 2D dots (1985) to 3D perspective (1988) to
;     tunnel effects (1992) to hardware-accelerated geometry (1996+).
;     This demo uses the Voodoo to render each star as a depth-scaled
;     triangle, a technique that became possible with consumer 3D hardware.
;
;   - SCROLLTEXT: the standard way to display credits and greetings.
;     A 5x7 bitmap font (448 bytes for 64 characters) was the classic
;     "small" format -- readable, memory-efficient, and fast to render.
;     The sine-wave wobble and rainbow colouring are signature demoscene
;     embellishments.
;
;   - SID MUSIC: the MOS 6581 Sound Interface Device in the Commodore 64
;     produced a distinctive sound that defined an entire genre.  A .SID
;     file contains actual 6502 machine code that drives the chip's
;     registers.  The IntuitionEngine runs this code on its real 6502 core,
;     with SID register writes ($D400-$D41C) remapped to the native
;     synthesiser -- authentic playback without cycle-accurate emulation.
;
; === THE UNSIGNED ARITHMETIC CHALLENGE ===
; The IE32 CPU has only unsigned 32-bit arithmetic.  For 3D graphics where
; coordinates can be negative (left of centre, above centre), we use an
; offset-based technique: add WORLD_OFFSET (600) to all coordinates before
; calculation, keeping everything positive.  After perspective projection,
; we subtract the projected offset to recover the true screen position:
;
;   screen_x = proj_x - offset_proj + CENTER_X
;
; This works because projection is linear:
;   proj(value + offset) = proj(value) + proj(offset)
;
; The sine table also uses an unsigned encoding: values 0-254 where
; 127 represents zero, 254 represents +1, and 0 represents -1.
;
; === MEMORY MAP ===
;   $0000-$0FFF     Stack / reserved
;   $1000-$87FF     Program code
;   $8800-$88FF     Runtime variables (angles, star temps, scroll state)
;   $10000-$103FF   Sine lookup table (256 entries * 4 bytes)
;   $20000-$20FFF   Star data array (256 stars * 16 bytes)
;   $0F0000+        Voodoo / SID player MMIO registers
;
; === BUILD AND RUN ===
;   sdk/bin/ie32asm sdk/examples/asm/voodoo_mega_demo.asm
;   ./bin/IntuitionEngine -ie32 sdk/examples/asm/voodoo_mega_demo.iex
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie32.inc"

; ============================================================================
; Constants
; ============================================================================

; --- Display ---
.equ SCREEN_W       640
.equ SCREEN_H       480
.equ CENTER_X       320
.equ CENTER_Y       240

; --- Fixed-point ---
.equ FP_SHIFT       4               ; 12.4 vertex format (pixel * 16)

; --- Starfield ---
.equ NUM_STARS      256
.equ TUNNEL_RADIUS  150             ; Max distance from tunnel centre axis
.equ NEAR_PLANE     80              ; Respawn threshold
.equ FAR_PLANE      1000            ; Spawn distance
.equ FOCAL_LENGTH   200             ; Perspective projection focal length
.equ TWIST_AMP      120             ; Tunnel centre oscillation amplitude
.equ WORLD_OFFSET   600             ; Keeps all unsigned coords positive

; --- Scrolltext ---
.equ SCROLL_SPEED   2               ; Pixels per frame
.equ CHAR_WIDTH     6               ; 5 content + 1 spacing
.equ CHAR_HEIGHT    7
.equ PIXEL_SIZE     4               ; Screen pixels per font pixel
.equ SCROLL_Y_POS   340             ; Base Y position on screen
.equ WOBBLE_AMP     50              ; Sine-wave Y amplitude
.equ MAX_CHARS      24              ; Max visible characters

; --- Runtime variables ---
.equ frame_counter  0x8800
.equ rand_seed      0x8804
.equ twist_x        0x8808
.equ twist_y        0x880C
.equ cur_z          0x8810
.equ offset_proj    0x8814

; --- Scrolltext variables ---
.equ scroll_offset    0x8880
.equ scroll_char_num  0x8884
.equ scroll_char_code 0x8888
.equ scroll_pixel_x   0x888C
.equ scroll_pixel_y   0x8890
.equ scroll_screen_x  0x8894
.equ scroll_screen_y  0x8898
.equ scroll_font_ptr  0x889C
.equ scroll_font_row  0x88A0
.equ scroll_base_x    0x88A4
.equ scroll_base_y    0x88A8
.equ scroll_msg_len   0x88AC

; --- Lookup tables and star data ---
.equ sin_table      0x10000         ; 256 entries (unsigned: 0=min, 127=zero, 254=max)
.equ star_array     0x20000         ; 16 bytes per star: angle, radius, z, speed

; --- Embedded SID file size ---
.equ sid_data_size  4790

; ============================================================================
; Program entry -- initialise Voodoo, sine table, stars, SID music
; ============================================================================

.org 0x1000

start:
    ; --- Enable Voodoo, set 640x480, configure Z-buffer ---
    LDA #1
    STA @VOODOO_ENABLE

    LDA #0x02800000
    OR A, #0x1E0
    STA @VOODOO_VIDEO_DIM

    LDA #0x0670
    STA @VOODOO_FBZ_MODE

    ; --- Zero frame counter and scroll offset ---
    LDA #0
    STA @frame_counter
    STA @scroll_offset

    LDA #54321
    STA @rand_seed

    ; --- Build the 256-entry sine table at runtime ---
    JSR build_sin_table

    ; --- Scatter 256 stars randomly through the tunnel ---
    JSR init_stars

    ; --- Start SID music: "Reggae 2" by Djinn/Fraction (1998) ---
    ; WHY: a demo without music is incomplete.  The SID player loads the
    ; embedded .SID file into the 6502 CPU core, which executes the original
    ; C64 music player code ~50 times per second to drive the synthesiser.
    LDA #sid_data
    STA @SID_PLAY_PTR

    LDA #sid_data_size
    STA @SID_PLAY_LEN

    LDA #0x05                       ; Start (bit 0) + Loop (bit 2)
    STA @SID_PLAY_CTRL

; ============================================================================
; Main loop -- one iteration per frame (~60 FPS)
; ============================================================================

main_loop:
    ; --- Clear framebuffer to near-black with blue tint ---
    LDA #0xFF040410
    STA @VOODOO_COLOR0

    LDA #0
    STA @VOODOO_FAST_FILL_CMD

    ; --- Calculate tunnel twist (X axis) ---
    ; WHY different frequencies: if both axes used the same sine, the tunnel
    ; would move in a circle.  Different rates create a Lissajous figure.
    LDA @frame_counter
    AND A, #255
    JSR get_sin
    MUL A, #TWIST_AMP
    SHR A, #6
    SUB A, #238
    STA @twist_x

    ; --- Calculate tunnel twist (Y axis, different frequency + phase) ---
    LDA @frame_counter
    MUL A, #3
    SHR A, #1
    ADD A, #64                      ; 90-degree phase offset
    AND A, #255
    JSR get_sin
    MUL A, #TWIST_AMP
    SHR A, #6
    SUB A, #238
    STA @twist_y

    ; --- Process all 256 stars ---
    LDY #0

star_loop:
    ; Calculate star array address: base + index * 16
    LDA Y
    SHL A, #4
    LDX #star_array
    ADD X, A

    ; Load star data into working variables
    LDA [X]
    STA @0x8820                     ; angle

    PUSH X
    ADD X, #4
    LDA [X]
    STA @0x8824                     ; radius

    ADD X, #4
    LDA [X]
    STA @0x8828                     ; z

    ADD X, #4
    LDA [X]
    STA @0x882C                     ; speed
    POP X

    ; --- Move star toward the camera ---
    LDA @0x8828
    SUB A, @0x882C
    STA @0x8828

    ; --- Respawn if star has passed the near plane ---
    LDA @0x8828
    SUB A, #NEAR_PLANE
    JGT A, no_respawn

    JSR random
    AND A, #255
    STA @0x8820                     ; new angle

    JSR random
    AND A, #255
    MUL A, #TUNNEL_RADIUS
    SHR A, #8
    ADD A, #15
    STA @0x8824                     ; new radius

    JSR random
    AND A, #511
    ADD A, #FAR_PLANE
    STA @0x8828                     ; new z

    JSR random
    AND A, #7
    ADD A, #2
    STA @0x882C                     ; new speed (2-9)

no_respawn:
    ; --- Write updated star data back ---
    LDA @0x8820
    STA [X]
    ADD X, #4
    LDA @0x8824
    STA [X]
    ADD X, #4
    LDA @0x8828
    STA [X]
    ADD X, #4
    LDA @0x882C
    STA [X]

    ; === 3D to 2D projection ===
    ; WHY the offset technique: cos/sin values are 0-254 (127 = zero).
    ; Multiplying by radius gives an unsigned result that cannot represent
    ; negative coordinates.  Adding WORLD_OFFSET keeps everything positive
    ; through division; we subtract the projected offset afterwards.

    ; Cosine = sin(angle + 64)
    LDA @0x8820
    ADD A, #64
    AND A, #255
    JSR get_sin
    STA @0x8830                     ; cos_val

    LDA @0x8820
    JSR get_sin
    STA @0x8834                     ; sin_val

    ; local_x = cos * radius / 64 + WORLD_OFFSET
    LDA @0x8830
    MUL A, @0x8824
    SHR A, #6
    ADD A, #WORLD_OFFSET
    STA @0x8838

    ; local_y = sin * radius / 64 + WORLD_OFFSET
    LDA @0x8834
    MUL A, @0x8824
    SHR A, #6
    ADD A, #WORLD_OFFSET
    STA @0x883C

    ; Apply twist offset
    LDA @0x8838
    ADD A, @twist_x
    ADD A, #128
    STA @0x8838

    LDA @0x883C
    ADD A, @twist_y
    ADD A, #128
    STA @0x883C

    ; Validate Z (skip if too close for safe division)
    LDA @0x8828
    STA @cur_z

    LDB #10
    SUB B, A
    JGT B, skip_star

    ; Perspective projection: proj = world * FOCAL / z
    LDA @0x8838
    MUL A, #FOCAL_LENGTH
    DIV A, @cur_z
    STA @0x8848                     ; proj_x

    LDA @0x883C
    MUL A, #FOCAL_LENGTH
    DIV A, @cur_z
    STA @0x884C                     ; proj_y

    ; Subtract projected offset to recover true screen position
    LDA #127
    MUL A, @0x8824
    SHR A, #6
    ADD A, #WORLD_OFFSET
    ADD A, #128
    MUL A, #FOCAL_LENGTH
    DIV A, @cur_z
    STA @offset_proj

    LDA @0x8848
    SUB A, @offset_proj
    ADD A, #CENTER_X
    STA @0x8850                     ; screen_x

    LDA @0x884C
    SUB A, @offset_proj
    ADD A, #CENTER_Y
    STA @0x8854                     ; screen_y

    ; --- Screen bounds check (sign-bit trick for unsigned "negative") ---
    LDA @0x8850
    AND A, #0x80000000
    JNZ A, skip_star

    LDA @0x8850
    SUB A, #SCREEN_W
    AND A, #0x80000000
    JZ A, skip_star

    LDA @0x8854
    AND A, #0x80000000
    JNZ A, skip_star

    LDA @0x8854
    SUB A, #SCREEN_H
    AND A, #0x80000000
    JZ A, skip_star

    ; --- Star size = 5000 / z (inverse depth) ---
    LDA #5000
    DIV A, @cur_z
    STA @0x8858

    ; Clamp to 12..96
    LDA @0x8858
    SUB A, #12
    JGT A, size_min_ok
    LDA #12
    STA @0x8858
size_min_ok:

    LDA @0x8858
    SUB A, #96
    JLT A, size_max_ok
    LDA #96
    STA @0x8858
size_max_ok:

    ; --- Depth-based colour (atmospheric perspective) ---
    ; WHY: close stars are white-hot, medium stars are cyan, far stars
    ; fade to deep blue.  This creates the illusion of depth without
    ; fog hardware.

    LDA @cur_z
    SUB A, #200
    JLT A, color_white

    LDA @cur_z
    SUB A, #500
    JLT A, color_cyan

    ; Far (z >= 500): deep blue
    LDA #0x0500
    STA @VOODOO_START_R
    LDA #0x0800
    STA @VOODOO_START_G
    LDA #0x1000
    STA @VOODOO_START_B
    JMP color_ok

color_cyan:
    LDA #0x0800
    STA @VOODOO_START_R
    LDA #0x0E00
    STA @VOODOO_START_G
    LDA #0x1000
    STA @VOODOO_START_B
    JMP color_ok

color_white:
    LDA #0x1000
    STA @VOODOO_START_R
    STA @VOODOO_START_G
    STA @VOODOO_START_B

color_ok:
    LDA #0x1000
    STA @VOODOO_START_A
    LDA #0x8000
    STA @VOODOO_START_Z

    ; --- Convert to 12.4 fixed-point ---
    LDA @0x8850
    SHL A, #4
    STA @0x8850

    LDA @0x8854
    SHL A, #4
    STA @0x8854

    ; --- Draw star as upward-pointing triangle ---
    ;       A (top)
    ;      / \
    ;     B---C (bottom)

    ; Vertex A (top centre)
    LDA @0x8850
    STA @VOODOO_VERTEX_AX
    LDA @0x8854
    SUB A, @0x8858
    STA @VOODOO_VERTEX_AY

    ; Vertex B (bottom left)
    LDA @0x8850
    SUB A, @0x8858
    STA @VOODOO_VERTEX_BX
    LDA @0x8854
    ADD A, @0x8858
    STA @VOODOO_VERTEX_BY

    ; Vertex C (bottom right)
    LDA @0x8850
    ADD A, @0x8858
    STA @VOODOO_VERTEX_CX
    LDA @0x8854
    ADD A, @0x8858
    STA @VOODOO_VERTEX_CY

    LDA #0
    STA @VOODOO_TRIANGLE_CMD

skip_star:
    ADD Y, #1
    LDB #NUM_STARS
    SUB B, Y
    JNZ B, star_loop

    ; --- Draw scrolltext ---
    JSR draw_scrolltext

    ; --- Swap buffers ---
    LDA #1
    STA @VOODOO_SWAP_BUFFER_CMD

    ; --- Advance animation ---
    LDA @frame_counter
    ADD A, #1
    STA @frame_counter

    LDA @scroll_offset
    ADD A, #SCROLL_SPEED
    STA @scroll_offset

    JMP main_loop

; ============================================================================
; Subroutines
; ============================================================================

; ----------------------------------------------------------------------------
; get_sin -- look up unsigned sine value from table
; Input:  A = angle (0-255)
; Output: A = sine value (0-254, where 127 = zero)
; ----------------------------------------------------------------------------

get_sin:
    AND A, #255
    SHL A, #2
    LDX #sin_table
    ADD X, A
    LDA [X]
    RTS

; ----------------------------------------------------------------------------
; random -- linear congruential generator (Numerical Recipes constants)
; Output: A = pseudo-random 32-bit value
; ----------------------------------------------------------------------------

random:
    LDA @rand_seed
    MUL A, #1664525
    ADD A, #1013904223
    STA @rand_seed
    RTS

; ============================================================================
; draw_scrolltext -- render bitmap font text with rainbow colours and wobble
; ============================================================================
;
; WHY quads instead of pixels: each "on" pixel in the 5x7 font is drawn as
; a 4x4 screen-pixel quad (two Voodoo triangles).  This makes the text large
; enough to read at 640x480 while keeping the charming bitmap aesthetic.
;
; The rainbow colour uses phase-shifted sine waves for R, G, B:
;   Red   = sin(angle + 0 degrees)
;   Green = sin(angle + 120 degrees)
;   Blue  = sin(angle + 240 degrees)

draw_scrolltext:
    PUSH Y

    ; Fixed alpha and Z for all scrolltext
    LDA #0x1000
    STA @VOODOO_START_A
    LDA #0x2000
    STA @VOODOO_START_Z

    ; Calculate first visible character index
    LDA @scroll_offset
    SHR A, #5
    STA @scroll_char_num

    LDY #0

scroll_char_loop:
    LDA Y
    SUB A, #MAX_CHARS
    JGE A, scroll_text_done

    ; Modulo wraparound for looping message (218 characters)
    LDA @scroll_char_num
    ADD A, Y
    LDX #scroll_message
scroll_mod:
    SUB A, #218
    JGE A, scroll_mod
    ADD A, #218

    ; Fetch ASCII character from message
    LDX #scroll_message
    ADD X, A
    LDA [X]
    AND A, #0xFF
    STA @scroll_char_code

    JZ A, scroll_text_done

    ; --- Character X position ---
    LDA Y
    MUL A, #24
    STA @scroll_base_x

    LDB @scroll_offset
    AND B, #31
    LDA @scroll_base_x
    SUB A, B
    STA @scroll_base_x

    ; --- Character Y position (with sine-wave wobble) ---
    LDA @scroll_base_x
    ADD A, @frame_counter
    SHR A, #3
    AND A, #255
    JSR get_sin
    MUL A, #WOBBLE_AMP
    SHR A, #7
    ADD A, #SCROLL_Y_POS
    SUB A, #20
    STA @scroll_base_y

    ; --- Font data pointer ---
    LDA @scroll_char_code
    SUB A, #32
    STA @scroll_font_ptr

    ; Bounds check (ASCII 32-95 only)
    LDA @scroll_font_ptr
    AND A, #0x80000000
    JNZ A, scroll_next_chr

    LDA @scroll_font_ptr
    SUB A, #64
    AND A, #0x80000000
    JZ A, scroll_next_chr

    LDA @scroll_font_ptr
    MUL A, #7
    STA @scroll_font_ptr
    LDA #font_data
    ADD A, @scroll_font_ptr
    STA @scroll_font_ptr

    ; --- Rainbow colour (phase-shifted sine waves) ---

    ; Red (phase 0 degrees)
    LDA Y
    SHL A, #5
    ADD A, @frame_counter
    SHL A, #1
    AND A, #255
    JSR get_sin
    SHR A, #4
    SHL A, #8
    ADD A, #0x0400
    STA @VOODOO_START_R

    ; Green (phase 120 degrees = 85)
    LDA Y
    SHL A, #5
    ADD A, @frame_counter
    SHL A, #1
    ADD A, #85
    AND A, #255
    JSR get_sin
    SHR A, #4
    SHL A, #8
    ADD A, #0x0400
    STA @VOODOO_START_G

    ; Blue (phase 240 degrees = 170)
    LDA Y
    SHL A, #5
    ADD A, @frame_counter
    SHL A, #1
    ADD A, #170
    AND A, #255
    JSR get_sin
    SHR A, #4
    SHL A, #8
    ADD A, #0x0400
    STA @VOODOO_START_B

    ; --- Render 5x7 character bitmap ---
    PUSH Y
    LDY #0                          ; Row counter

scroll_row_loop:
    LDA Y
    SUB A, #7
    JGE A, scroll_row_done

    ; Load font row byte
    LDA @scroll_font_ptr
    LDX A
    ADD X, Y
    LDA [X]
    AND A, #0xFF
    STA @scroll_font_row

    LDX #0                          ; Column counter

scroll_pixel_loop:
    LDA X
    SUB A, #5
    JGE A, scroll_pixel_done

    ; Build bit mask for column X (bit 4 = leftmost)
    LDA #16
    PUSH X
scroll_shift:
    LDB X
    JZ B, scroll_shift_done
    SHR A, #1
    SUB X, #1
    JMP scroll_shift
scroll_shift_done:
    POP X

    ; Test if pixel is set
    LDB @scroll_font_row
    AND B, A
    JZ B, scroll_skip_pixel

    ; Screen position for this pixel
    LDA X
    MUL A, #PIXEL_SIZE
    ADD A, @scroll_base_x
    STA @scroll_screen_x

    PUSH Y
    LDA Y
    MUL A, #PIXEL_SIZE
    ADD A, @scroll_base_y
    STA @scroll_screen_y
    POP Y

    ; Bounds check
    LDA @scroll_screen_x
    AND A, #0x80000000
    JNZ A, scroll_skip_pixel

    LDA @scroll_screen_x
    SUB A, #SCREEN_W
    JGE A, scroll_skip_pixel

    LDA @scroll_screen_y
    AND A, #0x80000000
    JNZ A, scroll_skip_pixel

    LDA @scroll_screen_y
    SUB A, #SCREEN_H
    JGE A, scroll_skip_pixel

    ; Convert to 12.4
    LDA @scroll_screen_x
    SHL A, #4
    STA @scroll_screen_x
    LDA @scroll_screen_y
    SHL A, #4
    STA @scroll_screen_y

    ; --- Draw pixel as quad (2 triangles, 4x4 screen pixels = 64 in 12.4) ---

    ; Triangle 1: top-left, top-right, bottom-left
    LDA @scroll_screen_x
    STA @VOODOO_VERTEX_AX
    LDA @scroll_screen_y
    STA @VOODOO_VERTEX_AY

    LDA @scroll_screen_x
    ADD A, #64
    STA @VOODOO_VERTEX_BX
    LDA @scroll_screen_y
    STA @VOODOO_VERTEX_BY

    LDA @scroll_screen_x
    STA @VOODOO_VERTEX_CX
    LDA @scroll_screen_y
    ADD A, #64
    STA @VOODOO_VERTEX_CY

    LDA #0
    STA @VOODOO_TRIANGLE_CMD

    ; Triangle 2: top-right, bottom-right, bottom-left
    LDA @scroll_screen_x
    ADD A, #64
    STA @VOODOO_VERTEX_AX
    LDA @scroll_screen_y
    STA @VOODOO_VERTEX_AY

    LDA @scroll_screen_x
    ADD A, #64
    STA @VOODOO_VERTEX_BX
    LDA @scroll_screen_y
    ADD A, #64
    STA @VOODOO_VERTEX_BY

    LDA @scroll_screen_x
    STA @VOODOO_VERTEX_CX
    LDA @scroll_screen_y
    ADD A, #64
    STA @VOODOO_VERTEX_CY

    LDA #0
    STA @VOODOO_TRIANGLE_CMD

scroll_skip_pixel:
    ADD X, #1
    JMP scroll_pixel_loop

scroll_pixel_done:
    ADD Y, #1
    JMP scroll_row_loop

scroll_row_done:
    POP Y

scroll_next_chr:
    ADD Y, #1
    JMP scroll_char_loop

scroll_text_done:
    POP Y
    RTS

; ============================================================================
; build_sin_table -- generate 256-entry unsigned sine table using quarter-wave
; ============================================================================
; WHY quarter-wave: a full sine has mirror symmetry.  We store only one
; quarter (64 values in quarter_sin) and derive the other three quadrants
; at runtime, saving ROM space and demonstrating the technique.

build_sin_table:
    LDY #0
    LDX #sin_table

sin_loop:
    ; Determine quadrant (bits 7-6)
    LDA Y
    SHR A, #6
    STA @0x8870

    ; Index within quadrant (bits 5-0)
    LDA Y
    AND A, #63

    ; Mirror index for odd quadrants (1, 3)
    LDB @0x8870
    AND B, #1
    JZ B, no_mirror
    LDB #63
    SUB B, A
    LDA B
no_mirror:

    ; Look up quarter-sine value
    SHL A, #2
    PUSH X
    LDX #quarter_sin
    ADD X, A
    LDA [X]
    POP X

    ; Quadrants 2-3 are the negative half: value = 127 - quarter_sin
    ; Quadrants 0-1 are the positive half: value = quarter_sin + 127
    LDB @0x8870
    AND B, #2
    JZ B, sin_pos
    LDB #127
    SUB B, A
    LDA B
    JMP store_sin

sin_pos:
    ADD A, #127

store_sin:
    STA [X]
    ADD X, #4
    ADD Y, #1
    LDB #256
    SUB B, Y
    JNZ B, sin_loop
    RTS

; ============================================================================
; init_stars -- scatter 256 stars randomly through the tunnel volume
; ============================================================================

init_stars:
    LDY #0
    LDX #star_array

init_loop:
    JSR random
    AND A, #255
    STA [X]                         ; angle
    ADD X, #4

    JSR random
    AND A, #255
    MUL A, #TUNNEL_RADIUS
    SHR A, #8
    ADD A, #15
    STA [X]                         ; radius
    ADD X, #4

    JSR random
    AND A, #2047
    ADD A, #NEAR_PLANE
    STA [X]                         ; z (wider initial spread)
    ADD X, #4

    JSR random
    AND A, #7
    ADD A, #2
    STA [X]                         ; speed (2-9)
    ADD X, #4

    ADD Y, #1
    LDB #NUM_STARS
    SUB B, Y
    JNZ B, init_loop
    RTS

; ============================================================================
; Data section
; ============================================================================

; ----------------------------------------------------------------------------
; Quarter-sine table (64 entries, 0-127)
; Full table is built at runtime by build_sin_table using symmetry.
; ----------------------------------------------------------------------------

quarter_sin:
    .word   0,   3,   6,  10,  13,  16,  19,  22
    .word  25,  28,  31,  34,  37,  40,  43,  46
    .word  49,  51,  54,  57,  60,  62,  65,  68
    .word  70,  73,  75,  78,  80,  82,  85,  87
    .word  89,  91,  94,  96,  98, 100, 102, 103
    .word 105, 107, 108, 110, 112, 113, 114, 116
    .word 117, 118, 119, 120, 121, 122, 123, 124
    .word 124, 125, 125, 126, 126, 127, 127, 127

; ----------------------------------------------------------------------------
; Scroll message (218 characters, loops continuously)
; ----------------------------------------------------------------------------

scroll_message:
    .ascii "     INTUITION ENGINE     3DFX VOODOO TWISTING STARFIELD TUNNEL     CODE: IE32 RISC ASM BY INTUITION      MUSIC: REGGAE 2 BY DJINN (6502 + SID)     GREETINGS TO ALL DEMOSCENERS...     VISIT INTUITIONSUBSYNTH.COM      "
    .byte 0

; ----------------------------------------------------------------------------
; 5x7 bitmap font data (ASCII 32-95, 64 characters, 448 bytes)
; Each character is 7 bytes (one per row), bits 4-0 = columns left to right.
; ----------------------------------------------------------------------------

font_data:
    ; Space (32)
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    ; ! (33)
    .byte 0x04, 0x04, 0x04, 0x04, 0x00, 0x04, 0x00
    ; " (34)
    .byte 0x0A, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x00
    ; # (35)
    .byte 0x0A, 0x1F, 0x0A, 0x0A, 0x1F, 0x0A, 0x00
    ; $ (36)
    .byte 0x04, 0x0F, 0x14, 0x0E, 0x05, 0x1E, 0x04
    ; % (37)
    .byte 0x18, 0x19, 0x02, 0x04, 0x08, 0x13, 0x03
    ; & (38)
    .byte 0x08, 0x14, 0x14, 0x08, 0x15, 0x12, 0x0D
    ; ' (39)
    .byte 0x04, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00
    ; ( (40)
    .byte 0x02, 0x04, 0x08, 0x08, 0x08, 0x04, 0x02
    ; ) (41)
    .byte 0x08, 0x04, 0x02, 0x02, 0x02, 0x04, 0x08
    ; * (42)
    .byte 0x00, 0x04, 0x15, 0x0E, 0x15, 0x04, 0x00
    ; + (43)
    .byte 0x00, 0x04, 0x04, 0x1F, 0x04, 0x04, 0x00
    ; , (44)
    .byte 0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x08
    ; - (45)
    .byte 0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00
    ; . (46)
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00
    ; / (47)
    .byte 0x01, 0x02, 0x02, 0x04, 0x08, 0x08, 0x10
    ; 0 (48)
    .byte 0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E
    ; 1 (49)
    .byte 0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E
    ; 2 (50)
    .byte 0x0E, 0x11, 0x01, 0x06, 0x08, 0x10, 0x1F
    ; 3 (51)
    .byte 0x0E, 0x11, 0x01, 0x06, 0x01, 0x11, 0x0E
    ; 4 (52)
    .byte 0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02
    ; 5 (53)
    .byte 0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E
    ; 6 (54)
    .byte 0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E
    ; 7 (55)
    .byte 0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08
    ; 8 (56)
    .byte 0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E
    ; 9 (57)
    .byte 0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C
    ; : (58)
    .byte 0x00, 0x00, 0x04, 0x00, 0x04, 0x00, 0x00
    ; ; (59)
    .byte 0x00, 0x00, 0x04, 0x00, 0x04, 0x04, 0x08
    ; < (60)
    .byte 0x02, 0x04, 0x08, 0x10, 0x08, 0x04, 0x02
    ; = (61)
    .byte 0x00, 0x00, 0x1F, 0x00, 0x1F, 0x00, 0x00
    ; > (62)
    .byte 0x08, 0x04, 0x02, 0x01, 0x02, 0x04, 0x08
    ; ? (63)
    .byte 0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04
    ; @ (64)
    .byte 0x0E, 0x11, 0x17, 0x15, 0x17, 0x10, 0x0E
    ; A (65)
    .byte 0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11
    ; B (66)
    .byte 0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E
    ; C (67)
    .byte 0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E
    ; D (68)
    .byte 0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E
    ; E (69)
    .byte 0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F
    ; F (70)
    .byte 0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10
    ; G (71)
    .byte 0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0E
    ; H (72)
    .byte 0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11
    ; I (73)
    .byte 0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E
    ; J (74)
    .byte 0x01, 0x01, 0x01, 0x01, 0x11, 0x11, 0x0E
    ; K (75)
    .byte 0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11
    ; L (76)
    .byte 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F
    ; M (77)
    .byte 0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11
    ; N (78)
    .byte 0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11
    ; O (79)
    .byte 0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E
    ; P (80)
    .byte 0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10
    ; Q (81)
    .byte 0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D
    ; R (82)
    .byte 0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11
    ; S (83)
    .byte 0x0E, 0x11, 0x10, 0x0E, 0x01, 0x11, 0x0E
    ; T (84)
    .byte 0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04
    ; U (85)
    .byte 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E
    ; V (86)
    .byte 0x11, 0x11, 0x11, 0x11, 0x0A, 0x0A, 0x04
    ; W (87)
    .byte 0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11
    ; X (88)
    .byte 0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11
    ; Y (89)
    .byte 0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04
    ; Z (90)
    .byte 0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F
    ; [ (91)
    .byte 0x0E, 0x08, 0x08, 0x08, 0x08, 0x08, 0x0E
    ; \ (92)
    .byte 0x10, 0x08, 0x08, 0x04, 0x02, 0x02, 0x01
    ; ] (93)
    .byte 0x0E, 0x02, 0x02, 0x02, 0x02, 0x02, 0x0E
    ; ^ (94)
    .byte 0x04, 0x0A, 0x11, 0x00, 0x00, 0x00, 0x00
    ; _ (95)
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1F

; ============================================================================
; SID music data -- "Reggae 2" by Kamil Degorski (Djinn) / Fraction (1998)
; ============================================================================
; WHY embedded: the .SID file contains both the 6502 player code and the
; pattern data.  The IntuitionEngine's 6502 core executes this code directly,
; with SID register writes ($D400-$D41C) remapped to the native synthesiser.

sid_data:
    .incbin "../assets/music/Reggae_2.sid"
sid_data_end:
