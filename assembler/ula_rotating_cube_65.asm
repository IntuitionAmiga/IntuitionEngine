; ============================================================================
; ROTATING 3D CUBE DEMO WITH AHX MUSIC
; 6502 Assembly for IntuitionEngine - ZX Spectrum ULA Display Mode
; ============================================================================
;
; REFERENCE IMPLEMENTATION FOR DEMOSCENE TECHNIQUES
; This file is heavily commented to teach demo programming concepts.
;
; === WHAT THIS DEMO DOES ===
; 1. Displays a wireframe 3D cube rotating smoothly on two axes
; 2. Uses the ZX Spectrum-style ULA display with its unique memory layout
; 3. Plays Amiga AHX tracker music through the audio subsystem
;
; === WHY THESE EFFECTS MATTER (HISTORICAL CONTEXT) ===
;
; THE ZX SPECTRUM ULA:
; The ZX Spectrum (1982) used a custom ULA (Uncommitted Logic Array) chip
; to generate video output. Its display had a notorious memory layout where
; scanlines were NOT stored sequentially - a design decision made to simplify
; the ULA's address generation logic at the cost of programmer headaches.
;
; The Intuition Engine faithfully emulates this quirky addressing scheme,
; allowing authentic Spectrum-style effects and teaching why this hardware
; was both beloved and frustrating to program.
;
; THE 6502 CPU:
; The 6502 was the heart of the Apple II, Commodore 64, BBC Micro, and
; (via the Z80A variant) influenced the Spectrum. With only three 8-bit
; registers (A, X, Y), a 256-byte "zero page" for fast access, and no
; multiply or divide instructions, it forced programmers to be creative.
;
; AHX TRACKER MUSIC:
; AHX (Abyss' Highest eXperience) is a tracker format from the Amiga
; demoscene, known for creating complex sounds from simple waveforms
; using software synthesis. Unlike sampled music (MOD/XM), AHX generates
; sounds algorithmically, achieving impressive results in tiny file sizes.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +-------------------------------------------------------------+
;   |                    MAIN LOOP (~30 FPS)                      |
;   |                                                             |
;   |  +-----------+    +-----------+    +-----------+            |
;   |  | WAIT FOR  |--->|   CLEAR   |--->|    SET    |            |
;   |  |  VBLANK   |    |  SCREEN   |    | ATTRIBUTES|            |
;   |  +-----------+    +-----------+    +-----------+            |
;   |                                          |                  |
;   |  +-----------+    +-----------+          |                  |
;   |  |   NEXT    |<---|   DRAW    |<---------+                  |
;   |  |   FRAME   |    |   CUBE    |                             |
;   |  +-----------+    +-----------+                             |
;   +-------------------------------------------------------------+
;
;   +-------------------------------------------------------------+
;   |              AHX AUDIO ENGINE (runs in parallel)            |
;   |                                                             |
;   |  The AHX synthesizer generates Amiga-style music from the   |
;   |  embedded tracker data. It runs independently of the CPU,   |
;   |  freeing the 6502 to focus entirely on graphics.            |
;   +-------------------------------------------------------------+
;
; === THE INTUITION ENGINE'S MULTI-CPU ARCHITECTURE ===
;
; This demo runs on the 6502 CPU core, but the Intuition Engine can
; also run M68020, Z80, and IE32 code. The AHX audio engine is separate
; from all CPUs - once started, it synthesizes music autonomously.
;
;   +----------+  +----------+  +----------+  +----------+
;   |  M68020  |  |   6502   |  |   Z80    |  |   IE32   |
;   | (unused) |  | (active) |  | (unused) |  | (unused) |
;   +----------+  +----------+  +----------+  +----------+
;        |             |             |             |
;        +------+------+------+------+------+------+
;               |                    |
;          +---------+         +-----------+
;          |   ULA   |         |    AHX    |
;          | Display |         |   Synth   |
;          +---------+         +-----------+
;
; The IE32 is a custom 32-bit RISC CPU designed specifically for the
; Intuition Engine, offering modern performance with retro aesthetics.
;
; ============================================================================

.include "ie65.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Music Configuration ---
; AHX files are compact tracker modules. This one is under 15KB but contains
; a full multi-channel composition with instrument definitions.
MUSIC_SIZE = 14687

; --- Display Geometry ---
; The ULA display is 256x192 pixels, but organized in a complex way.
; We'll explain the memory layout in detail in the plot_pixel routine.
SCR_W       = 256           ; Screen width in pixels
SCR_H       = 192           ; Screen height in pixels
CENTER_X    = 128           ; Screen center X (256/2)
CENTER_Y    = 96            ; Screen center Y (192/2)

; --- 3D Cube Geometry ---
; A cube has 8 vertices and 12 edges. We pre-calculate all vertex positions
; for 32 frames of animation (explained in the data section).
NUM_VERTICES = 8
NUM_EDGES    = 12
NUM_FRAMES   = 32           ; Animation frames for smooth rotation

; ============================================================================
; ZERO PAGE VARIABLES
; ============================================================================
; The 6502's zero page ($00-$FF) is special: instructions that access it
; are one byte shorter and one cycle faster than normal memory access.
; We use it for frequently-accessed variables, especially pointers.
;
; WHY ZERO PAGE MATTERS:
;   LDA $1234    ; Absolute addressing: 3 bytes, 4 cycles
;   LDA $12      ; Zero page addressing: 2 bytes, 3 cycles
;
; For a routine called thousands of times per frame (like plot_pixel),
; this difference adds up significantly.
; ============================================================================
.segment "ZEROPAGE"

; --- Line Drawing Variables ---
; Bresenham's algorithm needs to track current position, deltas, and error.
; All are single bytes because our screen is 256x192 (fits in 8 bits).
line_x0:        .res 1      ; Current X position (also starting X)
line_y0:        .res 1      ; Current Y position (also starting Y)
line_x1:        .res 1      ; Destination X position
line_y1:        .res 1      ; Destination Y position
line_dx:        .res 1      ; Absolute delta X (|x1 - x0|)
line_dy:        .res 1      ; Absolute delta Y (|y1 - y0|)
line_sx:        .res 1      ; X step direction (+1 or -1, stored as $01 or $FF)
line_sy:        .res 1      ; Y step direction (+1 or -1, stored as $01 or $FF)
line_err:       .res 2      ; Error accumulator (needs 16 bits for safety)

; --- Animation State ---
edge_idx:       .res 1      ; Current edge being drawn (0-11)
curr_frame:     .res 1      ; Current animation frame (0-31)

; --- Frame Data Pointers ---
; These point to the current frame's vertex coordinate arrays.
; 16-bit pointers because the data tables are outside zero page.
frame_ptr_x:    .res 2      ; Pointer to X coordinates for current frame
frame_ptr_y:    .res 2      ; Pointer to Y coordinates for current frame

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

; ============================================================================
; ENTRY POINT
; ============================================================================
; The 6502 begins execution here after loading the program.
; We initialize the display hardware and audio, then enter the main loop.
; ============================================================================
.proc start
    ; === CONFIGURE ULA DISPLAY ===
    ; The ULA has a border area surrounding the main display.
    ; Border color 1 = blue (classic Spectrum look).
    ; The border provides visual framing and was often used for loading
    ; stripes or simple effects on the original hardware.
    lda #1
    sta ULA_BORDER

    ; Enable the ULA display. Without this, we'd see nothing!
    ; ULA_CTRL_ENABLE activates the video output circuitry.
    lda #ULA_CTRL_ENABLE
    sta ULA_CTRL

    ; === START MUSIC PLAYBACK ===
    ; Initialize the AHX audio engine with our tracker module.
    ; Music plays autonomously once started - no CPU overhead.
    jsr init_music

    ; === INITIALIZE ANIMATION STATE ===
    ; Start at frame 0 of the 32-frame rotation sequence.
    lda #0
    sta curr_frame

; ============================================================================
; MAIN LOOP
; ============================================================================
; This loop runs continuously, drawing one animation frame per iteration.
; We synchronize to the display's vertical blank (VBLANK) to prevent tearing.
;
; FRAME TIMING:
; The ULA runs at 50Hz (PAL) or 60Hz (NTSC). We wait for TWO vblanks per
; frame, giving us ~25-30 FPS animation. This is intentional:
;   1. Gives more CPU time for drawing (the ULA addressing is slow)
;   2. Creates a more deliberate, "chunky" aesthetic
;   3. Matches the feel of many original Spectrum demos
; ============================================================================
main_loop:
    ; === WAIT FOR VERTICAL BLANK ===
    ; Drawing during active display causes "tearing" - visible artifacts
    ; where part of the old frame and part of the new frame are both shown.
    ; By waiting for VBLANK, we ensure all drawing happens while the
    ; electron beam is returning to the top of the screen.
    jsr wait_vblank

    ; === RENDER CURRENT FRAME ===
    ; Order matters here:
    ;   1. Clear old pixels (blank slate)
    ;   2. Set attributes (colors for the whole screen)
    ;   3. Set up pointers to this frame's vertex data
    ;   4. Draw the wireframe cube
    jsr clear_screen
    jsr set_attributes
    jsr setup_frame_ptrs
    jsr draw_cube

    ; === WAIT FOR SECOND VBLANK ===
    ; This extra wait slows animation to ~25 FPS, giving a smoother
    ; visual result and more time for the complex ULA addressing math.
    jsr wait_vblank

    ; === ADVANCE ANIMATION ===
    ; Move to next frame. After frame 31, wrap back to frame 0.
    ; This creates a seamless looping rotation.
    inc curr_frame
    lda curr_frame
    cmp #NUM_FRAMES         ; 32 frames for full rotation
    bne main_loop           ; Not at end, continue
    lda #0                  ; Wrap to frame 0
    sta curr_frame
    jmp main_loop           ; Loop forever
.endproc

; ============================================================================
; WAIT FOR VERTICAL BLANK
; ============================================================================
; Polls the ULA status register until the VBLANK bit is set.
;
; WHY POLL?
; The 6502 has no interrupt controller in this implementation.
; We busy-wait, which "wastes" CPU cycles but keeps the code simple.
; In a more complex demo, we might use this time for audio or calculations.
;
; ULA_STATUS BIT LAYOUT:
;   Bit 0: VBLANK - Set when display is in vertical blank period
;   Other bits reserved for future use
; ============================================================================
.proc wait_vblank
@wait:
    lda ULA_STATUS          ; Read status register
    and #ULA_STATUS_VBLANK  ; Isolate VBLANK bit
    beq @wait               ; Loop until bit is set
    rts
.endproc

; ============================================================================
; INITIALIZE AND START MUSIC PLAYBACK
; ============================================================================
; Configures the AHX audio engine to play our embedded tracker module.
;
; === WHAT IS AHX? ===
;
; AHX (Abyss' Highest eXperience) is a tracker music format created by
; Dexter and Pink of Abyss for the Amiga demoscene in 1998. It synthesizes
; all sounds in real-time using mathematical waveforms, rather than playing
; back sampled audio like MOD or XM files.
;
; This gives AHX files remarkable compression - a full song in under 16KB!
;
; === HOW AHX SYNTHESIS WORKS ===
;
;   +------------------+     +------------------+     +-------------+
;   | Tracker Pattern  |---->| Instrument Def   |---->| Oscillator  |
;   | (notes, effects) |     | (ADSR, waveform) |     | (sine, saw) |
;   +------------------+     +------------------+     +-------------+
;                                                           |
;   +------------------+     +------------------+           v
;   |   Audio Output   |<----| Filter/Effects   |<----------+
;   +------------------+     +------------------+
;
; Each "instrument" is a program that modulates basic waveforms over time,
; controlled by the tracker pattern data. The Intuition Engine's AHX player
; interprets this data and generates audio samples in real-time.
;
; === REGISTER INTERFACE ===
;
; The AHX player uses memory-mapped registers:
;   AHX_PLAY_PTR_0-3: 32-bit pointer to AHX data (little-endian)
;   AHX_PLAY_LEN_0-3: 32-bit length of AHX data (little-endian)
;   AHX_PLAY_CTRL:    Control register (bit 0=start, bit 2=loop)
;
; Once started, the player runs autonomously in the audio subsystem.
; The 6502 doesn't need to do anything else for music playback!
; ============================================================================
.proc init_music
    ; === SET POINTER TO MUSIC DATA ===
    ; The AHX player needs to know where the tracker data lives in memory.
    ; We use a 32-bit pointer (4 bytes) stored in little-endian order.
    ; On 6502, the < operator gives the low byte, > gives the high byte.
    ;
    ; For a 16-bit address like $8000, we'd store:
    ;   PTR_0 = $00 (bits 0-7)
    ;   PTR_1 = $80 (bits 8-15)
    ;   PTR_2 = $00 (bits 16-23, always 0 on 6502)
    ;   PTR_3 = $00 (bits 24-31, always 0 on 6502)
    lda #<music_data        ; Low byte of address
    sta AHX_PLAY_PTR_0
    lda #>music_data        ; High byte of address
    sta AHX_PLAY_PTR_1
    lda #0                  ; Upper 16 bits are zero
    sta AHX_PLAY_PTR_2      ; (6502 has 16-bit address space)
    sta AHX_PLAY_PTR_3

    ; === SET MUSIC LENGTH ===
    ; The player needs to know the file size to parse the AHX header
    ; and locate the pattern data within.
    lda #<MUSIC_SIZE        ; Low byte of length
    sta AHX_PLAY_LEN_0
    lda #>MUSIC_SIZE        ; High byte of length
    sta AHX_PLAY_LEN_1
    lda #0                  ; Upper bytes zero (file < 64KB)
    sta AHX_PLAY_LEN_2
    sta AHX_PLAY_LEN_3

    ; === START PLAYBACK ===
    ; Control register bits:
    ;   Bit 0 (value 1): Start playback
    ;   Bit 1 (value 2): Stop playback
    ;   Bit 2 (value 4): Enable looping
    ;
    ; We use $05 = %00000101 = start + loop
    ; This makes the music play continuously, perfect for a demo.
    lda #$05                ; Start playback with looping
    sta AHX_PLAY_CTRL

    rts
.endproc

; ============================================================================
; SET UP FRAME POINTERS
; ============================================================================
; Calculates pointers to the current frame's pre-computed vertex coordinates.
;
; === WHY PRE-COMPUTE? ===
;
; Real-time 3D rotation requires trigonometry (sin/cos) and matrix math.
; The 6502 has no multiply instruction, let alone floating-point!
; While we COULD compute rotations using lookup tables and shifts,
; the ULA's complex addressing already taxes our CPU budget.
;
; Instead, we pre-computed all 32 frames of animation offline:
;   - Python script calculates rotation matrices
;   - Projects 3D vertices to 2D screen coordinates
;   - Stores results as simple byte arrays
;
; This is a classic demoscene technique: trade memory for CPU time.
; 32 frames * 8 vertices * 2 coords = 512 bytes for butter-smooth rotation.
;
; === POINTER CALCULATION ===
;
; Each frame has 8 vertex X values and 8 vertex Y values (8 bytes each).
; Frame N's X data starts at: all_vertex_x + (N * 8)
; Frame N's Y data starts at: all_vertex_y + (N * 8)
;
; We use shifts instead of multiplication:
;   ASL A (shift left) = multiply by 2
;   Three shifts = multiply by 8
; ============================================================================
.proc setup_frame_ptrs
    ; === CALCULATE X POINTER ===
    ; frame_ptr_x = all_vertex_x + (curr_frame * 8)
    lda curr_frame
    asl a                   ; * 2
    asl a                   ; * 4
    asl a                   ; * 8

    ; Add base address (16-bit addition)
    clc
    adc #<all_vertex_x      ; Add low byte of base
    sta frame_ptr_x
    lda #>all_vertex_x      ; Start with high byte of base
    adc #0                  ; Add carry from low byte addition
    sta frame_ptr_x+1

    ; === CALCULATE Y POINTER ===
    ; Same calculation for Y coordinates
    lda curr_frame
    asl a
    asl a
    asl a
    clc
    adc #<all_vertex_y
    sta frame_ptr_y
    lda #>all_vertex_y
    adc #0
    sta frame_ptr_y+1

    rts
.endproc

; ============================================================================
; CLEAR SCREEN
; ============================================================================
; Fills the entire ULA bitmap area with zeros (all pixels off).
;
; === ULA MEMORY LAYOUT OVERVIEW ===
;
; The ULA display memory starts at ULA_VRAM and occupies 6912 bytes:
;   - 6144 bytes for pixel bitmap (256x192 / 8 bits per byte)
;   - 768 bytes for color attributes (32x24 character cells)
;
; Unlike modern linear framebuffers, the ULA bitmap has a complex layout
; that we'll explain in detail in the plot_pixel routine.
;
; === CLEARING STRATEGY ===
;
; We need to clear 6144 bytes ($1800) of bitmap data.
; The inner loop clears 256 bytes (one "page"), then we increment the
; high byte of our pointer and repeat for all 24 pages.
;
; This is faster than a single 6144-iteration loop because:
;   - INY is faster than 16-bit pointer increment
;   - The branch (BNE) predicts well when Y wraps from 255 to 0
; ============================================================================
.proc clear_screen
    ; Set up pointer to start of video RAM
    lda #<ULA_VRAM
    sta zp_ptr0
    lda #>ULA_VRAM
    sta zp_ptr0+1

    lda #0                  ; Value to write (all pixels off)
    ldy #0                  ; Index within current page
    ldx #24                 ; Number of 256-byte pages to clear

@loop:
    sta (zp_ptr0),y         ; Clear byte at ptr+Y
    iny                     ; Next byte
    bne @loop               ; Loop until Y wraps (256 iterations)
    inc zp_ptr0+1           ; Move to next page
    dex                     ; Decrement page counter
    bne @loop               ; Continue if pages remain

    rts
.endproc

; ============================================================================
; SET ATTRIBUTES
; ============================================================================
; Fills the attribute area with a uniform color (white on black).
;
; === ULA COLOR ATTRIBUTES ===
;
; The Spectrum ULA uses a clever memory-saving trick: instead of storing
; color per-pixel, it stores color per 8x8 CHARACTER CELL. The attribute
; area is a 32x24 grid (one byte per cell) covering the 256x192 display.
;
; Each attribute byte encodes:
;   Bits 0-2: INK color (foreground, 0-7)
;   Bits 3-5: PAPER color (background, 0-7)
;   Bit 6:    BRIGHT flag (makes colors brighter)
;   Bit 7:    FLASH flag (swaps ink/paper periodically)
;
; Color palette (normal/bright):
;   0: Black      / Black
;   1: Blue       / Bright Blue
;   2: Red        / Bright Red
;   3: Magenta    / Bright Magenta
;   4: Green      / Bright Green
;   5: Cyan       / Bright Cyan
;   6: Yellow     / Bright Yellow
;   7: White      / Bright White
;
; === ATTRIBUTE CLASH ===
;
; Because colors apply to entire 8x8 cells, it's impossible to have more
; than 2 colors in any cell. This "attribute clash" was a defining visual
; characteristic of Spectrum graphics - and a challenge for artists!
;
; In this demo, we simply use white ink on black paper ($07) everywhere,
; avoiding clash entirely at the cost of colorful graphics.
; ============================================================================
.proc set_attributes
    ; Attribute area starts after the bitmap
    ; ULA_ATTR_OFFSET = 6144 bytes ($1800)
    lda #<(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0
    lda #>(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0+1

    ; Attribute value: $07 = white ink (7), black paper (0), no flash/bright
    lda #$07
    ldy #0
    ldx #3                  ; 768 bytes = 3 pages of 256

@loop:
    sta (zp_ptr0),y
    iny
    bne @loop
    inc zp_ptr0+1
    dex
    bne @loop

    rts
.endproc

; ============================================================================
; DRAW CUBE
; ============================================================================
; Renders the wireframe cube by drawing all 12 edges.
;
; === CUBE TOPOLOGY ===
;
; A cube has 8 vertices (corners) and 12 edges:
;   - 4 edges on the front face
;   - 4 edges on the back face
;   - 4 edges connecting front to back
;
; We store edges as pairs of vertex indices in the cube_edges table.
;
;        7--------6
;       /|       /|
;      / |      / |
;     4--------5  |
;     |  3-----|--2
;     | /      | /
;     |/       |/
;     0--------1
;
; === RENDERING APPROACH ===
;
; For each edge:
;   1. Look up the two vertex indices
;   2. Fetch their screen coordinates from the current frame's tables
;   3. Draw a line between them using Bresenham's algorithm
;
; The Y register is used for indirect indexed addressing: (ptr),Y
; This lets us efficiently access the vertex coordinate arrays.
; ============================================================================
.proc draw_cube
    lda #0
    sta edge_idx            ; Start with edge 0

@edge_loop:
    ; === CALCULATE EDGE TABLE OFFSET ===
    ; Each edge is 2 bytes (two vertex indices), so offset = edge_idx * 2
    lda edge_idx
    asl a                   ; * 2
    tax                     ; X = offset into edge table

    ; === GET FIRST VERTEX COORDINATES ===
    ; Load first vertex index, use it to fetch X and Y from frame data
    lda cube_edges,x        ; First vertex index
    tay                     ; Y = vertex index (for indirect addressing)
    lda (frame_ptr_x),y     ; Load X coordinate
    sta line_x0
    lda (frame_ptr_y),y     ; Load Y coordinate
    sta line_y0

    ; === GET SECOND VERTEX COORDINATES ===
    inx                     ; Move to second vertex index in edge table
    lda cube_edges,x
    tay
    lda (frame_ptr_x),y
    sta line_x1
    lda (frame_ptr_y),y
    sta line_y1

    ; === DRAW THE EDGE ===
    jsr draw_line

    ; === NEXT EDGE ===
    inc edge_idx
    lda edge_idx
    cmp #NUM_EDGES          ; 12 edges total
    bne @edge_loop

    rts
.endproc

; ============================================================================
; DRAW LINE (BRESENHAM'S ALGORITHM)
; ============================================================================
; Draws a line from (line_x0, line_y0) to (line_x1, line_y1).
;
; === WHY BRESENHAM? ===
;
; Bresenham's line algorithm (1962) is ideal for integer-only hardware:
;   - Uses only addition, subtraction, and bit shifts
;   - No multiplication or division required
;   - Perfectly accurate (no floating-point drift)
;
; For the 6502 with no MUL instruction, this is essential.
;
; === ALGORITHM OVERVIEW ===
;
; The key insight is that we can track "error" as we step along the major
; axis, deciding when to step on the minor axis:
;
;   1. Determine which axis has the larger delta (the "major" axis)
;   2. Step along the major axis one pixel at a time
;   3. Accumulate error (the minor axis delta) each step
;   4. When error exceeds half the major delta, step on minor axis
;   5. Repeat until we reach the destination
;
; === X-MAJOR vs Y-MAJOR ===
;
; A line is "X-major" if |dx| >= |dy| (more horizontal than vertical).
; For X-major lines, we step X every iteration and sometimes step Y.
; Y-major lines are the opposite.
;
; We need separate loops because the 6502 can't easily parameterize
; which variable to step - the code would be slower than duplicating it.
;
; === SIGNED ARITHMETIC ON 6502 ===
;
; The 6502 has no signed compare instruction. We handle negative deltas
; by computing the absolute value and storing the direction separately:
;   - line_dx, line_dy: absolute delta values
;   - line_sx, line_sy: step directions ($01 for +1, $FF for -1)
;
; Using $FF for -1 works because signed addition treats it as -1:
;   CLC; ADC #$FF is equivalent to subtracting 1 (with potential carry)
; ============================================================================
.proc draw_line
    ; === DRAW STARTING POINT ===
    ; Always draw at least the first pixel
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    ; === CHECK FOR SINGLE-POINT LINE ===
    ; If start == end, we're done (avoids division by zero in error calc)
    lda line_x0
    cmp line_x1
    bne @not_point
    lda line_y0
    cmp line_y1
    bne @not_point
    rts                     ; Start equals end, nothing more to draw

@not_point:
    ; === CALCULATE DX AND DETERMINE X DIRECTION ===
    ; dx = x1 - x0 (signed)
    ; We need |dx| and the sign separately
    sec
    lda line_x1
    sbc line_x0             ; A = x1 - x0 (signed result)
    bcs @dx_pos             ; Branch if result >= 0 (no borrow)

    ; dx is negative: negate it and set sx = -1
    eor #$FF                ; One's complement
    clc
    adc #1                  ; Two's complement = negate
    sta line_dx
    lda #$FF                ; -1 as unsigned byte
    sta line_sx
    jmp @calc_dy

@dx_pos:
    ; dx is positive: store it and set sx = +1
    sta line_dx
    lda #$01
    sta line_sx

@calc_dy:
    ; === CALCULATE DY AND DETERMINE Y DIRECTION ===
    ; Same process for the Y axis
    sec
    lda line_y1
    sbc line_y0
    bcs @dy_pos

    ; dy is negative
    eor #$FF
    clc
    adc #1
    sta line_dy
    lda #$FF
    sta line_sy
    jmp @start

@dy_pos:
    ; dy is positive
    sta line_dy
    lda #$01
    sta line_sy

@start:
    ; === CHOOSE MAJOR AXIS ===
    ; Compare |dx| and |dy| to determine line orientation
    lda line_dx
    cmp line_dy
    bcc @y_major            ; Branch if dx < dy (Y is major axis)

; ============================================================================
; X-MAJOR LINE DRAWING LOOP
; ============================================================================
; For lines where |dx| >= |dy|, we step X every iteration.
; The error term tracks when to step Y.
; ============================================================================
@x_major:
    lda line_dx
    beq @done               ; Safety check: if dx=0, we're done
    sta line_err            ; Initialize error to dx
    lsr line_err            ; error = dx / 2 (Bresenham's midpoint)

@x_loop:
    ; Step X in the appropriate direction
    clc
    lda line_x0
    adc line_sx             ; Add +1 or -1
    sta line_x0

    ; Accumulate error (add dy each step)
    clc
    lda line_err
    adc line_dy
    sta line_err

    ; Check if error exceeds threshold (dx)
    cmp line_dx
    bcc @x_noy              ; Branch if error < dx

    ; Error exceeded: step Y and reduce error
    sec
    sbc line_dx
    sta line_err
    clc
    lda line_y0
    adc line_sy
    sta line_y0

@x_noy:
    ; Draw current pixel
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    ; Check if we've reached the destination X
    lda line_x0
    cmp line_x1
    bne @x_loop             ; Continue if not there yet
    rts

; ============================================================================
; Y-MAJOR LINE DRAWING LOOP
; ============================================================================
; For lines where |dy| > |dx|, we step Y every iteration.
; ============================================================================
@y_major:
    lda line_dy
    beq @done               ; Safety check
    sta line_err
    lsr line_err            ; error = dy / 2

@y_loop:
    ; Step Y
    clc
    lda line_y0
    adc line_sy
    sta line_y0

    ; Accumulate error (add dx each step)
    clc
    lda line_err
    adc line_dx
    sta line_err

    ; Check if error exceeds threshold (dy)
    cmp line_dy
    bcc @y_nox

    ; Error exceeded: step X
    sec
    sbc line_dy
    sta line_err
    clc
    lda line_x0
    adc line_sx
    sta line_x0

@y_nox:
    ; Draw current pixel
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    ; Check if we've reached the destination Y
    lda line_y0
    cmp line_y1
    bne @y_loop

@done:
    rts
.endproc

; ============================================================================
; PLOT PIXEL
; ============================================================================
; Sets a single pixel at coordinates (A=X, X=Y).
;
; Input: A = X coordinate (0-255)
;        X = Y coordinate (0-191)
;
; === THE INFAMOUS ULA MEMORY LAYOUT ===
;
; The ZX Spectrum's ULA chip generated video addresses using a simplified
; circuit that resulted in a highly non-linear memory layout. Understanding
; this is essential for Spectrum programming!
;
; === SCREEN ADDRESS CALCULATION ===
;
; For a pixel at coordinates (X, Y), the byte address is calculated from
; the Y coordinate alone (X determines which BIT within that byte).
;
; The Y coordinate (0-191) is split into three parts:
;   - Bits 7-6: Third of screen (0-2), contributes to high address
;   - Bits 5-3: Character row (0-7), contributes to low address
;   - Bits 2-0: Pixel row within character (0-7), contributes to high address
;
; This creates a bizarre interleaving where consecutive scanlines are
; NOT consecutive in memory!
;
; Screen layout (simplified):
;   Y=0   -> $4000    First pixel row of first character row
;   Y=1   -> $4100    First pixel row of NINTH character row (!)
;   Y=2   -> $4200    ...
;   ...
;   Y=8   -> $4020    SECOND pixel row of first character row
;   ...
;
; === ADDRESS FORMULA ===
;
;   High byte: %010[Y7][Y6][Y2][Y1][Y0]
;   Low byte:  %[Y5][Y4][Y3][X7][X6][X5][X4][X3]
;
; Where:
;   - 010 = base address ($4000 = %0100 0000 0000 0000)
;   - Y7,Y6 = screen third (0, 1, or 2)
;   - Y2,Y1,Y0 = pixel row within character cell
;   - Y5,Y4,Y3 = character row within third
;   - X7..X3 = character column (X / 8)
;
; === BIT WITHIN BYTE ===
;
; The X coordinate's low 3 bits (X2,X1,X0) determine which bit to set.
; Bit 7 is leftmost, bit 0 is rightmost:
;   X & 7 = 0 -> bit 7 ($80)
;   X & 7 = 1 -> bit 6 ($40)
;   ...
;   X & 7 = 7 -> bit 0 ($01)
;
; We use a lookup table (bit_masks) for this conversion.
;
; === WHY THIS MADNESS? ===
;
; The ULA's address generator could compute this layout with very few gates.
; Linear addressing would have required an adder circuit. In 1982, every
; transistor counted, so this "programmer-hostile" layout was acceptable.
;
; Many Spectrum programmers memorized this calculation!
; ============================================================================
.proc plot_pixel
    pha                     ; Save X coordinate on stack

    ; === CALCULATE HIGH BYTE OF ADDRESS ===
    ; High byte = %010[Y7][Y6][Y2][Y1][Y0]

    ; Get screen third from Y (bits 7-6)
    ; Shift right 3 times: Y7,Y6 move to bits 4,3
    txa                     ; A = Y coordinate
    and #$C0                ; Isolate bits 7-6 (screen third)
    lsr a                   ; >> 1: bits now at 6,5
    lsr a                   ; >> 2: bits now at 5,4
    lsr a                   ; >> 3: bits now at 4,3
    sta zp_ptr0+1           ; Store partial high byte

    ; Get pixel row within character from Y (bits 2-0)
    ; These go directly to bits 2,1,0 of high byte
    txa
    and #$07                ; Isolate bits 2-0
    clc
    adc zp_ptr0+1           ; Combine with screen third
    sta zp_ptr0+1           ; High byte now has: %000[Y7][Y6][Y2][Y1][Y0]

    ; === CALCULATE LOW BYTE OF ADDRESS ===
    ; Low byte = %[Y5][Y4][Y3][X7][X6][X5][X4][X3]

    ; Get character row from Y (bits 5-3)
    ; Shift left 2 times to move to bits 7-5
    txa
    and #$38                ; Isolate bits 5-3 (%00111000)
    asl a                   ; << 1: bits now at 6-4
    asl a                   ; << 2: bits now at 7-5
    sta zp_ptr0             ; Store partial low byte

    ; Get character column from X (X / 8 = bits 7-3)
    pla                     ; Retrieve X coordinate
    pha                     ; Keep it on stack (we need it again)
    lsr a                   ; >> 1
    lsr a                   ; >> 2
    lsr a                   ; >> 3: X/8 now in bits 4-0
    clc
    adc zp_ptr0             ; Combine with character row
    sta zp_ptr0             ; Low byte complete
    bcc @nc                 ; Check for carry into high byte
    inc zp_ptr0+1
@nc:
    ; === ADD BASE ADDRESS ===
    ; ULA_VRAM is the base address of video memory
    ; We add only the high byte since low byte is already complete
    clc
    lda zp_ptr0+1
    adc #>ULA_VRAM          ; Add high byte of base address
    sta zp_ptr0+1

    ; === SET THE PIXEL BIT ===
    ; X coordinate's low 3 bits determine which bit in the byte
    pla                     ; Retrieve X coordinate
    and #$07                ; Isolate low 3 bits (bit position)
    tax                     ; Use as index into bit_masks table
    lda bit_masks,x         ; Get the bit mask
    ldy #0
    ora (zp_ptr0),y         ; OR with existing byte (preserve other pixels)
    sta (zp_ptr0),y         ; Write back

    rts
.endproc

; ============================================================================
; READ-ONLY DATA SEGMENT
; ============================================================================
.segment "RODATA"

; ============================================================================
; BIT MASKS TABLE
; ============================================================================
; Lookup table for pixel bit positions within a byte.
; Index 0 = leftmost pixel (bit 7), index 7 = rightmost pixel (bit 0).
; ============================================================================
bit_masks:
    .byte $80, $40, $20, $10, $08, $04, $02, $01

; ============================================================================
; CUBE EDGE LIST
; ============================================================================
; Each edge is defined by two vertex indices.
; The cube_edges table contains 12 edges * 2 indices = 24 bytes.
;
; Edge numbering:
;   Edges 0-3:  Front face (vertices 0,1,2,3)
;   Edges 4-7:  Back face (vertices 4,5,6,7)
;   Edges 8-11: Connecting edges (front to back)
; ============================================================================
cube_edges:
    ; Front face edges
    .byte 0, 1              ; Bottom edge: vertex 0 to vertex 1
    .byte 1, 2              ; Right edge:  vertex 1 to vertex 2
    .byte 2, 3              ; Top edge:    vertex 2 to vertex 3
    .byte 3, 0              ; Left edge:   vertex 3 to vertex 0

    ; Back face edges
    .byte 4, 5              ; Bottom edge
    .byte 5, 6              ; Right edge
    .byte 6, 7              ; Top edge
    .byte 7, 4              ; Left edge

    ; Connecting edges (front to back)
    .byte 0, 4              ; Bottom-left
    .byte 1, 5              ; Bottom-right
    .byte 2, 6              ; Top-right
    .byte 3, 7              ; Top-left

; ============================================================================
; PRE-CALCULATED VERTEX COORDINATES
; ============================================================================
; 32 frames of animation data, with 8 vertices per frame.
;
; === WHY PRE-CALCULATE? ===
;
; Real-time 3D rotation requires:
;   1. Sine/cosine lookups (or calculations)
;   2. Matrix multiplication (9 multiplies, 6 adds per vertex)
;   3. Perspective division (divide by Z)
;
; On a 6502 without multiply/divide instructions, this would be VERY slow.
; Instead, we pre-compute everything offline and store the results.
;
; === ANIMATION PARAMETERS ===
;
; Cube centered at (128, 96) - the screen center
; Cube size: 80 pixels (±40 from center)
; Rotation: Combined X and Y axis rotation
;   - Full 360° rotation on Y axis across 32 frames
;   - Half rotation (180°) on X axis for tumbling effect
;
; === VERTEX NUMBERING ===
;
;        7--------6
;       /|       /|
;      / |      / |
;     4--------5  |
;     |  3-----|--2
;     | /      | /
;     |/       |/
;     0--------1
;
; Vertices 0-3 form the front face, 4-7 the back face.
; ============================================================================

all_vertex_x:
    .byte 88, 168, 168, 88, 88, 168, 168, 88      ; Frame 0
    .byte 96, 175, 175, 96, 80, 159, 159, 80      ; Frame 1
    .byte 106, 180, 180, 106, 75, 149, 149, 75    ; Frame 2
    .byte 116, 183, 183, 116, 72, 139, 139, 72    ; Frame 3
    .byte 128, 184, 184, 128, 71, 128, 128, 71    ; Frame 4
    .byte 139, 183, 183, 139, 72, 116, 116, 72    ; Frame 5
    .byte 149, 180, 180, 149, 75, 106, 106, 75    ; Frame 6
    .byte 159, 175, 175, 159, 80, 96, 96, 80      ; Frame 7
    .byte 168, 168, 168, 168, 88, 88, 88, 88      ; Frame 8
    .byte 175, 159, 159, 175, 96, 80, 80, 96      ; Frame 9
    .byte 180, 149, 149, 180, 106, 75, 75, 106    ; Frame 10
    .byte 183, 139, 139, 183, 116, 72, 72, 116    ; Frame 11
    .byte 184, 128, 128, 184, 128, 71, 71, 128    ; Frame 12
    .byte 183, 116, 116, 183, 139, 72, 72, 139    ; Frame 13
    .byte 180, 106, 106, 180, 149, 75, 75, 149    ; Frame 14
    .byte 175, 96, 96, 175, 159, 80, 80, 159      ; Frame 15
    .byte 168, 88, 88, 168, 168, 88, 88, 168      ; Frame 16
    .byte 159, 80, 80, 159, 175, 96, 96, 175      ; Frame 17
    .byte 149, 75, 75, 149, 180, 106, 106, 180    ; Frame 18
    .byte 139, 72, 72, 139, 183, 116, 116, 183    ; Frame 19
    .byte 128, 71, 71, 128, 184, 127, 127, 184    ; Frame 20
    .byte 116, 72, 72, 116, 183, 139, 139, 183    ; Frame 21
    .byte 106, 75, 75, 106, 180, 149, 149, 180    ; Frame 22
    .byte 96, 80, 80, 96, 175, 159, 159, 175      ; Frame 23
    .byte 88, 88, 88, 88, 168, 168, 168, 168      ; Frame 24
    .byte 80, 96, 96, 80, 159, 175, 175, 159      ; Frame 25
    .byte 75, 106, 106, 75, 149, 180, 180, 149    ; Frame 26
    .byte 72, 116, 116, 72, 139, 183, 183, 139    ; Frame 27
    .byte 71, 127, 127, 71, 128, 184, 184, 128    ; Frame 28
    .byte 72, 139, 139, 72, 116, 183, 183, 116    ; Frame 29
    .byte 75, 149, 149, 75, 106, 180, 180, 106    ; Frame 30
    .byte 80, 159, 159, 80, 96, 175, 175, 96      ; Frame 31

all_vertex_y:
    .byte 56, 56, 136, 136, 56, 56, 136, 136      ; Frame 0
    .byte 51, 53, 132, 131, 59, 60, 140, 138      ; Frame 1
    .byte 46, 52, 131, 125, 60, 66, 145, 139      ; Frame 2
    .byte 41, 54, 131, 118, 60, 73, 150, 137      ; Frame 3
    .byte 37, 59, 132, 111, 59, 80, 154, 132      ; Frame 4
    .byte 34, 65, 136, 105, 55, 86, 157, 126      ; Frame 5
    .byte 33, 74, 141, 100, 50, 91, 158, 117      ; Frame 6
    .byte 35, 85, 146, 97, 45, 94, 156, 106       ; Frame 7
    .byte 39, 96, 152, 96, 39, 96, 152, 96        ; Frame 8
    .byte 46, 106, 157, 97, 34, 94, 145, 85       ; Frame 9
    .byte 55, 117, 161, 100, 30, 91, 136, 74      ; Frame 10
    .byte 67, 126, 163, 105, 28, 86, 124, 65      ; Frame 11
    .byte 80, 132, 163, 111, 28, 80, 111, 59      ; Frame 12
    .byte 94, 137, 160, 118, 31, 73, 97, 54       ; Frame 13
    .byte 109, 139, 155, 125, 36, 66, 82, 52      ; Frame 14
    .byte 123, 138, 146, 131, 45, 60, 68, 53      ; Frame 15
    .byte 136, 136, 136, 136, 55, 56, 56, 55      ; Frame 16
    .byte 146, 131, 123, 138, 68, 53, 45, 60      ; Frame 17
    .byte 155, 125, 109, 139, 82, 52, 36, 66      ; Frame 18
    .byte 160, 118, 94, 137, 97, 54, 31, 73       ; Frame 19
    .byte 163, 111, 80, 132, 111, 59, 28, 80      ; Frame 20
    .byte 163, 105, 67, 126, 124, 65, 28, 86      ; Frame 21
    .byte 161, 100, 55, 117, 136, 74, 30, 91      ; Frame 22
    .byte 157, 97, 46, 106, 145, 85, 34, 94       ; Frame 23
    .byte 152, 96, 39, 96, 152, 95, 39, 96        ; Frame 24
    .byte 146, 97, 35, 85, 156, 106, 45, 94       ; Frame 25
    .byte 141, 100, 33, 74, 158, 117, 50, 91      ; Frame 26
    .byte 136, 105, 34, 65, 157, 126, 55, 86      ; Frame 27
    .byte 132, 111, 37, 59, 154, 132, 59, 80      ; Frame 28
    .byte 131, 118, 41, 54, 150, 137, 60, 73      ; Frame 29
    .byte 131, 125, 46, 52, 145, 139, 60, 66      ; Frame 30
    .byte 132, 131, 51, 53, 140, 138, 59, 60      ; Frame 31

; ============================================================================
; MUSIC DATA
; ============================================================================
; Embedded AHX tracker module.
;
; === WHAT'S IN AN AHX FILE? ===
;
; An AHX file contains:
;   1. Header with song metadata (name, speed, pattern count, etc.)
;   2. Instrument definitions (waveform generators, ADSR envelopes)
;   3. Pattern sequence (order of patterns to play)
;   4. Pattern data (notes and effects for each channel)
;
; Unlike MOD/XM files which contain PCM samples, AHX generates ALL sound
; through synthesis. This is why it achieves such small file sizes.
;
; === THE AUDIO ENGINE ===
;
; The Intuition Engine's AHX player:
;   1. Parses the file header to find song structure
;   2. Steps through patterns at the song's tempo
;   3. Synthesizes waveforms based on instrument definitions
;   4. Applies envelopes, filters, and effects
;   5. Mixes all channels to stereo output
;
; All of this happens in the audio subsystem - the 6502 is completely
; free to focus on graphics once playback is started.
;
; "Chopper" is a driving, energetic track typical of the Amiga demoscene.
; ============================================================================
.segment "BINDATA"

music_data:
    .incbin "../chopper.ahx"
