; ============================================================================
; ULA ROTATING CUBE WITH AHX MUSIC - Pre-Calculated 3D Animation
; 6502 Assembly for IntuitionEngine - ULA Display (256x192, attribute-based)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    MOS 6502
; Video Chip:    ULA (ZX Spectrum-compatible, 256x192 with 8x8 attribute cells)
; Audio Engine:  AHX (Amiga tracker synthesis)
; Assembler:     ca65/ld65 (cc65 toolchain)
; Build:         make ie65asm SRC=sdk/examples/asm/ula_rotating_cube_65.asm
; Run:           ./bin/IntuitionEngine -6502 ula_rotating_cube_65.ie65
; Porting:       ULA/AHX MMIO is CPU-agnostic. Port effort: rewrite the
;                ULA address calculation and line-drawing loop. Z80 port is
;                straightforward (similar register model). M68K simplifies
;                significantly (hardware multiply, 32-bit registers).
;
; === WHAT THIS DEMO DOES ===
; 1. Displays a wireframe 3D cube rotating smoothly on two axes
; 2. Uses the ZX Spectrum-style ULA display with its unique memory layout
; 3. Plays Amiga AHX tracker music through the audio subsystem
;
; The cube animation uses 32 pre-calculated frames rather than real-time
; trigonometry. This is a classic demoscene technique: trade memory for
; CPU time. 32 frames x 8 vertices x 2 coordinates = 512 bytes for
; butter-smooth rotation, freeing the 6502 to focus on the complex ULA
; address calculations.
;
; === WHY ULA DISPLAY ARCHITECTURE ===
; The ZX Spectrum's ULA (Uncommitted Logic Array) was the custom chip that
; generated video output. Its most distinctive feature was a non-linear VRAM
; layout: consecutive screen rows are NOT at consecutive memory addresses.
; This was a cost-saving measure by Sinclair Research -- it simplified the
; ULA's line-counting logic at the expense of making software more complex.
;
; The attribute system divides the 256x192 pixel screen into 8x8 cells
; (32 columns x 24 rows = 768 cells). Each cell has a single attribute byte
; controlling: INK colour (foreground, 3 bits), PAPER colour (background,
; 3 bits), BRIGHT (1 bit), and FLASH (1 bit). This means each 8x8 cell can
; only display 2 colours, leading to the infamous "attribute clash" that
; defined the Spectrum's visual style.
;
; The IntuitionEngine's ULA emulates this system faithfully, including the
; non-linear addressing.
;
; === 6502-SPECIFIC NOTES ===
; The 6502 has only three 8-bit registers (A, X, Y), no multiply or divide
; instructions, and a 256-byte zero page for fast access. The zero page
; is critical: instructions that access it are one byte shorter and one
; cycle faster than absolute addressing. For a routine like plot_pixel
; called hundreds of times per frame, this adds up significantly:
;   LDA $1234    ; Absolute: 3 bytes, 4 cycles
;   LDA $12      ; Zero page: 2 bytes, 3 cycles
;
; The 16-bit pointer zp_ptr0 (provided by ie65.inc) is used for reaching
; ULA VRAM at $4000 via indirect indexed addressing: (zp_ptr0),Y.
;
; === MEMORY MAP ===
;   $0000-$00FF  Zero page (line vars, frame pointers, animation state)
;   $4000-$57FF  ULA bitmap VRAM (6144 bytes, non-linear layout)
;   $5800-$5AFF  ULA attribute VRAM (768 bytes, 32x24 cells)
;   $D800        ULA registers (border, control, status)
;   RODATA       Pre-calculated vertex tables, edge list, bit masks
;   BINDATA      Embedded AHX tracker module (~15KB)
;
; === BUILD AND RUN ===
;   make ie65asm SRC=sdk/examples/asm/ula_rotating_cube_65.asm
;   ./bin/IntuitionEngine -6502 ula_rotating_cube_65.ie65
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie65.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Music ---
MUSIC_SIZE = 14687

; --- Display geometry ---
SCR_W       = 256           ; Screen width in pixels
SCR_H       = 192           ; Screen height in pixels
CENTER_X    = 128           ; Screen centre X (256/2)
CENTER_Y    = 96            ; Screen centre Y (192/2)

; --- 3D cube topology ---
NUM_VERTICES = 8
NUM_EDGES    = 12
NUM_FRAMES   = 32           ; Pre-calculated animation frames

; ============================================================================
; ZERO PAGE VARIABLES
; ============================================================================
.segment "ZEROPAGE"

; --- Line drawing state ---
line_x0:        .res 1      ; Current X position (also starting X)
line_y0:        .res 1      ; Current Y position (also starting Y)
line_x1:        .res 1      ; Destination X position
line_y1:        .res 1      ; Destination Y position
line_dx:        .res 1      ; Absolute delta X (|x1 - x0|)
line_dy:        .res 1      ; Absolute delta Y (|y1 - y0|)
line_sx:        .res 1      ; X step direction ($01 or $FF)
line_sy:        .res 1      ; Y step direction ($01 or $FF)
line_err:       .res 2      ; Error accumulator

; --- Animation state ---
edge_idx:       .res 1      ; Current edge being drawn (0-11)
curr_frame:     .res 1      ; Current animation frame (0-31)

; --- Frame data pointers ---
; 16-bit pointers to the current frame's vertex coordinate arrays.
frame_ptr_x:    .res 2      ; Pointer to X coordinates for current frame
frame_ptr_y:    .res 2      ; Pointer to Y coordinates for current frame

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

; ============================================================================
; ENTRY POINT
; ============================================================================
.proc start
    ; --- Initialise ULA display ---
    ; Border colour 1 = blue (classic Spectrum look).
    lda #1
    sta ULA_BORDER

    ; Enable the ULA video output circuitry.
    lda #ULA_CTRL_ENABLE
    sta ULA_CTRL

    ; --- Start AHX music playback ---
    ; Music runs autonomously once started -- no CPU overhead per frame.
    jsr init_music

    ; --- Initialise animation state ---
    lda #0
    sta curr_frame

; ============================================================================
; MAIN LOOP
; ============================================================================
; Each iteration draws one frame of the 32-frame rotation sequence.
; We wait for two vertical blanks per frame (~25 FPS), giving the 6502
; enough time for the complex ULA address calculations and providing
; a deliberate, "chunky" aesthetic that matches many original Spectrum demos.
; ============================================================================
main_loop:
    jsr wait_vblank
    jsr clear_screen
    jsr set_attributes
    jsr setup_frame_ptrs
    jsr draw_cube

    ; Wait for a second vblank to halve the frame rate.
    jsr wait_vblank

    ; --- Advance to next frame, wrapping at 32 ---
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
; Busy-waiting is the simplest synchronisation method on the 6502.
; ============================================================================
.proc wait_vblank
@wait:
    lda ULA_STATUS          ; Read status register
    and #ULA_STATUS_VBLANK  ; Isolate VBLANK bit
    beq @wait               ; Loop until bit is set
    rts
.endproc

; ============================================================================
; INITIALISE AHX MUSIC PLAYBACK
; ============================================================================
; Configures the AHX audio engine with a pointer to the embedded tracker
; module and starts looped playback.
;
; === WHY AHX? ===
; AHX (Abyss' Highest eXperience) is a tracker format from the Amiga
; demoscene that synthesises all sounds algorithmically from waveform
; definitions -- no PCM samples needed. This achieves remarkable compression:
; a full multi-channel composition in under 16KB. Once started, the
; IntuitionEngine's AHX player runs in the audio subsystem independently
; of the CPU.
;
; === REGISTER INTERFACE ===
;   AHX_PLAY_PTR_0-3: 32-bit pointer to AHX data (little-endian)
;   AHX_PLAY_LEN_0-3: 32-bit length of AHX data (little-endian)
;   AHX_PLAY_CTRL:    Control register (bit 0=start, bit 2=loop)
; ============================================================================
.proc init_music
    ; Set 32-bit pointer to music data (little-endian).
    lda #<music_data        ; Low byte of address
    sta AHX_PLAY_PTR_0
    lda #>music_data        ; High byte of address
    sta AHX_PLAY_PTR_1
    lda #0                  ; Upper 16 bits are zero
    sta AHX_PLAY_PTR_2      ; (6502 has 16-bit address space)
    sta AHX_PLAY_PTR_3

    ; Set 32-bit data length.
    lda #<MUSIC_SIZE        ; Low byte of length
    sta AHX_PLAY_LEN_0
    lda #>MUSIC_SIZE        ; High byte of length
    sta AHX_PLAY_LEN_1
    lda #0                  ; Upper bytes zero (file < 64KB)
    sta AHX_PLAY_LEN_2
    sta AHX_PLAY_LEN_3

    ; Start playback with looping ($05 = start + loop).
    lda #$05
    sta AHX_PLAY_CTRL

    rts
.endproc

; ============================================================================
; SET UP FRAME POINTERS
; ============================================================================
; Calculates pointers to the current frame's pre-computed vertex coordinates.
; Each frame has 8 X values and 8 Y values (8 bytes each). Frame N's data
; starts at: all_vertex_x + (N * 8) and all_vertex_y + (N * 8).
;
; We use ASL (shift left) instead of multiplication:
;   ASL A three times = multiply by 8
; ============================================================================
.proc setup_frame_ptrs
    ; --- Calculate X pointer ---
    lda curr_frame
    asl a                   ; * 2
    asl a                   ; * 4
    asl a                   ; * 8

    clc
    adc #<all_vertex_x      ; Add low byte of base
    sta frame_ptr_x
    lda #>all_vertex_x      ; Start with high byte of base
    adc #0                  ; Add carry from low byte addition
    sta frame_ptr_x+1

    ; --- Calculate Y pointer ---
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
; CLEAR SCREEN (BITMAP AREA)
; ============================================================================
; Fills all 6144 bytes of the ULA bitmap with zero (all pixels off).
; Uses page-at-a-time clearing: 24 pages x 256 bytes = 6144 bytes.
; ============================================================================
.proc clear_screen
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
; Fills the 768-byte attribute area with white INK on black PAPER ($07).
;
; === WHY UNIFORM ATTRIBUTES? ===
; Each attribute byte controls an 8x8 pixel cell:
;   Bits 0-2: INK colour   (foreground)  -- 7 = white
;   Bits 3-5: PAPER colour (background)  -- 0 = black
;   Bit 6:    BRIGHT flag
;   Bit 7:    FLASH flag
;
; Using the same attribute everywhere avoids the infamous "attribute clash"
; where overlapping colours in the same 8x8 cell cause visual artefacts.
; ============================================================================
.proc set_attributes
    lda #<(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0
    lda #>(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0+1

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
;        7--------6
;       /|       /|
;      / |      / |
;     4--------5  |
;     |  3-----|--2
;     | /      | /
;     |/       |/
;     0--------1
;
; Edges 0-3: front face, 4-7: back face, 8-11: connecting edges.
; For each edge, we look up the two vertex indices, fetch their screen
; coordinates from the current frame's tables, and draw a line.
; ============================================================================
.proc draw_cube
    lda #0
    sta edge_idx            ; Start with edge 0

@edge_loop:
    ; --- Calculate edge table offset (2 bytes per edge) ---
    lda edge_idx
    asl a                   ; * 2
    tax                     ; X = offset into edge table

    ; --- Get first vertex coordinates ---
    lda cube_edges,x        ; First vertex index
    tay                     ; Y = vertex index (for indirect addressing)
    lda (frame_ptr_x),y     ; Load X coordinate
    sta line_x0
    lda (frame_ptr_y),y     ; Load Y coordinate
    sta line_y0

    ; --- Get second vertex coordinates ---
    inx                     ; Move to second vertex index in edge table
    lda cube_edges,x
    tay
    lda (frame_ptr_x),y
    sta line_x1
    lda (frame_ptr_y),y
    sta line_y1

    ; --- Draw the edge ---
    jsr draw_line

    ; --- Next edge ---
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
; Bresenham's line algorithm (1962) is ideal for integer-only hardware:
; it uses only addition, subtraction, and bit shifts -- no multiplication
; or division required. For the 6502 with no MUL instruction, this is
; essential.
;
; The algorithm steps along the major axis one pixel at a time, accumulating
; an error term that determines when to step on the minor axis. We split
; into X-major and Y-major loops because parameterising the axis on the
; 6502 would be slower than duplicating the loop.
;
; === SIGNED DIRECTION ===
; The 6502 has no signed compare. We compute |dx| and |dy| as unsigned
; values, storing the step direction separately: $01 for +1, $FF for -1.
; ============================================================================
.proc draw_line
    ; Always draw the starting pixel.
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    ; Single-point check.
    lda line_x0
    cmp line_x1
    bne @not_point
    lda line_y0
    cmp line_y1
    bne @not_point
    rts                     ; Start equals end, nothing more to draw

@not_point:
    ; --- Calculate |dx| and X direction ---
    sec
    lda line_x1
    sbc line_x0             ; A = x1 - x0 (signed result)
    bcs @dx_pos             ; Branch if result >= 0 (no borrow)

    ; dx is negative: negate and set sx = -1
    eor #$FF                ; One's complement
    clc
    adc #1                  ; Two's complement = negate
    sta line_dx
    lda #$FF                ; -1 as unsigned byte
    sta line_sx
    jmp @calc_dy

@dx_pos:
    sta line_dx
    lda #$01
    sta line_sx

@calc_dy:
    ; --- Calculate |dy| and Y direction ---
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
    sta line_dy
    lda #$01
    sta line_sy

@start:
    ; --- Choose major axis ---
    lda line_dx
    cmp line_dy
    bcc @y_major            ; Branch if dx < dy (Y is major axis)

; --- X-MAJOR LOOP ---
@x_major:
    lda line_dx
    beq @done               ; Safety check
    sta line_err            ; Initialise error to dx
    lsr line_err            ; error = dx / 2 (Bresenham's midpoint)

@x_loop:
    clc
    lda line_x0
    adc line_sx             ; Step X in the appropriate direction
    sta line_x0

    clc
    lda line_err
    adc line_dy             ; Accumulate error
    sta line_err

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
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    lda line_x0
    cmp line_x1
    bne @x_loop             ; Continue if not at destination X
    rts

; --- Y-MAJOR LOOP ---
@y_major:
    lda line_dy
    beq @done               ; Safety check
    sta line_err
    lsr line_err            ; error = dy / 2

@y_loop:
    clc
    lda line_y0
    adc line_sy
    sta line_y0

    clc
    lda line_err
    adc line_dx
    sta line_err

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
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    lda line_y0
    cmp line_y1
    bne @y_loop

@done:
    rts
.endproc

; ============================================================================
; PLOT PIXEL (ULA NON-LINEAR ADDRESSING)
; ============================================================================
; Sets a single pixel at coordinates (A=X, X=Y).
;
; === THE INFAMOUS ULA MEMORY LAYOUT ===
;
; The ZX Spectrum's ULA addressed video memory using a simplified circuit
; that produced a highly non-linear layout. The Y coordinate (0-191) is
; decomposed into three bit-fields:
;
;   Bits 7-6: Screen third (0-2)     --> high byte bits 4-3
;   Bits 2-0: Pixel row in cell      --> high byte bits 2-0
;   Bits 5-3: Character row in third --> low byte bits 7-5
;
; This creates the bizarre interleaving where Y=0 is at $4000, Y=1 is at
; $4100 (not $4020!), and Y=8 is at $4020. Consecutive scanlines are
; 256 bytes apart until you cross a character cell boundary.
;
; Address formula:
;   High byte: %010[Y7][Y6][Y2][Y1][Y0]  (+ ULA_VRAM base)
;   Low byte:  %[Y5][Y4][Y3][X7][X6][X5][X4][X3]
;
; The X coordinate's low 3 bits select which bit within the byte to set.
; Bit 7 is leftmost, bit 0 is rightmost.
;
; === WHY THIS LAYOUT? ===
; The ULA could generate these addresses with very few logic gates. A
; linear layout would have required a binary adder circuit. In 1982 every
; transistor counted, so Sinclair accepted this programmer-hostile scheme
; to keep manufacturing costs down.
; ============================================================================
.proc plot_pixel
    pha                     ; Save X coordinate on stack

    ; --- High byte: screen third + pixel row within character ---
    txa                     ; A = Y coordinate
    and #$C0                ; Isolate bits 7-6 (screen third)
    lsr a                   ; >> 1
    lsr a                   ; >> 2
    lsr a                   ; >> 3: bits now at 4,3
    sta zp_ptr0+1           ; Store partial high byte

    txa
    and #$07                ; Isolate bits 2-0 (pixel row in cell)
    clc
    adc zp_ptr0+1           ; Combine with screen third
    sta zp_ptr0+1

    ; --- Low byte: character row + byte column ---
    txa
    and #$38                ; Isolate bits 5-3 (character row)
    asl a                   ; << 1
    asl a                   ; << 2: bits now at 7-5
    sta zp_ptr0             ; Store partial low byte

    pla                     ; Retrieve X coordinate
    pha                     ; Keep it on stack (needed for bit mask)
    lsr a                   ; >> 1
    lsr a                   ; >> 2
    lsr a                   ; >> 3: X/8 = byte column
    clc
    adc zp_ptr0             ; Combine with character row
    sta zp_ptr0
    bcc @nc                 ; Check for carry into high byte
    inc zp_ptr0+1
@nc:
    ; --- Add ULA VRAM base address ---
    clc
    lda zp_ptr0+1
    adc #>ULA_VRAM          ; Add high byte of base address
    sta zp_ptr0+1

    ; --- Set the pixel bit ---
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
; READ-ONLY DATA
; ============================================================================
.segment "RODATA"

; ============================================================================
; BIT MASKS TABLE
; ============================================================================
; Index 0 = leftmost pixel (bit 7), index 7 = rightmost pixel (bit 0).
; ============================================================================
bit_masks:
    .byte $80, $40, $20, $10, $08, $04, $02, $01

; ============================================================================
; CUBE EDGE LIST
; ============================================================================
; 12 edges as pairs of vertex indices (24 bytes total).
;
;        7--------6
;       /|       /|
;      / |      / |
;     4--------5  |
;     |  3-----|--2
;     | /      | /
;     |/       |/
;     0--------1
; ============================================================================
cube_edges:
    ; Front face edges
    .byte 0, 1              ; Bottom edge
    .byte 1, 2              ; Right edge
    .byte 2, 3              ; Top edge
    .byte 3, 0              ; Left edge

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
; 32 frames of animation, 8 vertices per frame.
;
; === WHY PRE-CALCULATE? ===
; Real-time 3D rotation requires sin/cos lookups and matrix multiplication.
; The 6502 has no multiply instruction, and the ULA's non-linear addressing
; already taxes the CPU budget. Instead, all 32 frames were computed offline:
;   32 frames x 8 vertices x 2 coords = 512 bytes
; This is a classic demoscene trade-off: memory for CPU time.
;
; Cube centred at (128, 96). Combined X + Y axis rotation. Full 360-degree
; Y rotation and 180-degree X rotation across the 32-frame sequence.
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
; EMBEDDED AHX MUSIC DATA
; ============================================================================
; AHX tracker module -- synthesises all sounds algorithmically from waveform
; definitions (no PCM samples). The IntuitionEngine's AHX player parses the
; file header, steps through patterns at the song's tempo, synthesises
; waveforms, applies envelopes and effects, and mixes to stereo output --
; all in the audio subsystem, completely independent of the 6502.
; ============================================================================
.segment "BINDATA"

music_data:
    .incbin "../assets/music/chopper.ahx"
