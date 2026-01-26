; ============================================================================
; ULA ROTATING CUBE DEMO - IE65 (6502)
;
; A wireframe rotating cube rendered on the ZX Spectrum ULA video chip.
; Demonstrates:
; - ULA bitmap/attribute addressing (the famous non-linear layout)
; - 8.8 fixed-point 3D rotation math
; - Bresenham line drawing algorithm
; - 64-entry sine/cosine lookup tables
;
; Display: 256x192 pixels, white wireframe on black background
; Border: Blue (color 1)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie65.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; Cube size (half-width in pixels, used for vertex coordinates)
CUBE_SIZE       = 50

; Screen center for projection
CENTER_X        = 128           ; 256/2
CENTER_Y        = 96            ; 192/2

; Projection distance (larger = less perspective)
PROJ_DIST       = 200

; Rotation speed (added to angle each frame)
ROT_SPEED_X     = 2
ROT_SPEED_Y     = 3

; Number of cube vertices and edges
NUM_VERTICES    = 8
NUM_EDGES       = 12

; ULA colors
INK_WHITE       = 7
PAPER_BLACK     = 0
ATTR_VALUE      = (PAPER_BLACK << 3) | INK_WHITE    ; $07

; Border color
BORDER_BLUE     = 1

; ============================================================================
; ZERO PAGE VARIABLES
; ============================================================================
.segment "ZEROPAGE"

; Frame counter
frame_count:    .res 2

; Rotation angles (0-255 maps to 0-360 degrees)
angle_x:        .res 1
angle_y:        .res 1

; Temporary variables for 3D math (8.8 fixed point)
temp_x:         .res 2          ; Intermediate X coordinate
temp_y:         .res 2          ; Intermediate Y coordinate
temp_z:         .res 2          ; Intermediate Z coordinate
sin_val:        .res 2          ; Current sine value (signed 8.8)
cos_val:        .res 2          ; Current cosine value (signed 8.8)

; Projected 2D coordinates for all 8 vertices
proj_x:         .res 8          ; Screen X coordinates
proj_y:         .res 8          ; Screen Y coordinates

; Line drawing variables
line_x0:        .res 1
line_y0:        .res 1
line_x1:        .res 1
line_y1:        .res 1
line_dx:        .res 1
line_dy:        .res 1
line_sx:        .res 1          ; Step X: 1 or -1
line_sy:        .res 1          ; Step Y: 1 or -1
line_err:       .res 2          ; Error term (signed)
line_e2:        .res 2          ; 2 * error

; Temporary storage for multiplication
mul_a:          .res 2          ; Multiplicand (signed 8.8)
mul_b:          .res 2          ; Multiplier (signed 8.8)
mul_result:     .res 4          ; Result (signed 16.16, we use middle 16 bits)

; General temporaries
temp0:          .res 2
temp1:          .res 2

; Current vertex being processed
curr_vertex:    .res 1

; ============================================================================
; BSS SEGMENT - Uninitialized RAM
; ============================================================================
.segment "BSS"

; Rotated 3D coordinates (8.8 fixed point, signed)
rot_x:          .res 16         ; 8 vertices * 2 bytes
rot_y:          .res 16
rot_z:          .res 16

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

; ----------------------------------------------------------------------------
; Entry Point
; ----------------------------------------------------------------------------
.proc start
    ; Initialize ULA
    jsr init_ula

    ; Clear screen
    jsr clear_screen

    ; Set attributes to white on black for entire screen
    jsr set_attributes

    ; Initialize rotation angles
    lda #0
    sta angle_x
    sta angle_y
    sta frame_count
    sta frame_count+1

    ; Main loop
main_loop:
    ; Wait for VBlank (check VIDEO_STATUS for compatibility)
    jsr wait_vblank

    ; Clear the bitmap area
    jsr clear_bitmap

    ; Rotate and project vertices
    jsr rotate_vertices
    jsr project_vertices

    ; Draw all edges
    jsr draw_cube

    ; Update rotation angles
    clc
    lda angle_x
    adc #ROT_SPEED_X
    sta angle_x

    clc
    lda angle_y
    adc #ROT_SPEED_Y
    sta angle_y

    ; Increment frame counter
    inc frame_count
    bne :+
    inc frame_count+1
:

    jmp main_loop
.endproc

; ----------------------------------------------------------------------------
; Initialize ULA video chip
; ----------------------------------------------------------------------------
.proc init_ula
    ; Set border color to blue
    lda #BORDER_BLUE
    sta ULA_BORDER

    ; Enable ULA
    lda #ULA_CTRL_ENABLE
    sta ULA_CTRL

    rts
.endproc

; ----------------------------------------------------------------------------
; Clear entire screen (bitmap + attributes)
; ----------------------------------------------------------------------------
.proc clear_screen
    ; Clear bitmap (6144 bytes at $4000)
    lda #<ULA_VRAM
    sta zp_ptr0
    lda #>ULA_VRAM
    sta zp_ptr0+1

    lda #0
    ldy #0
    ldx #24                     ; 24 pages = 6144 bytes

@clear_loop:
    sta (zp_ptr0),y
    iny
    bne @clear_loop
    inc zp_ptr0+1
    dex
    bne @clear_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Clear just the bitmap area (for each frame)
; ----------------------------------------------------------------------------
.proc clear_bitmap
    ; Same as clear_screen but only bitmap
    lda #<ULA_VRAM
    sta zp_ptr0
    lda #>ULA_VRAM
    sta zp_ptr0+1

    lda #0
    ldy #0
    ldx #24                     ; 24 pages = 6144 bytes

@clear_loop:
    sta (zp_ptr0),y
    iny
    bne @clear_loop
    inc zp_ptr0+1
    dex
    bne @clear_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Set attributes to white ink on black paper for entire screen
; ----------------------------------------------------------------------------
.proc set_attributes
    ; Attributes start at $5800 (768 bytes for 32x24 cells)
    lda #<(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0
    lda #>(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0+1

    lda #ATTR_VALUE             ; White on black
    ldy #0
    ldx #3                      ; 3 pages = 768 bytes

@attr_loop:
    sta (zp_ptr0),y
    iny
    bne @attr_loop
    inc zp_ptr0+1
    dex
    bne @attr_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Wait for VBlank
; ----------------------------------------------------------------------------
.proc wait_vblank
    ; Wait for VBlank flag to be set
    ; On the Intuition Engine, use VIDEO_STATUS
:   lda VIDEO_STATUS
    and #STATUS_VBLANK
    beq :-

    ; Wait for VBlank to end (next frame)
:   lda VIDEO_STATUS
    and #STATUS_VBLANK
    bne :-

    rts
.endproc

; ----------------------------------------------------------------------------
; Rotate all cube vertices around X and Y axes
; Input: angle_x, angle_y
; Output: rot_x[], rot_y[], rot_z[] (8.8 fixed point)
; ----------------------------------------------------------------------------
.proc rotate_vertices
    lda #0
    sta curr_vertex

@vertex_loop:
    ; Get original vertex coordinates (already in 8.8 format in ROM)
    lda curr_vertex
    asl a                       ; * 2 for word index
    tax

    ; Load original coordinates
    lda cube_vertices,x
    sta temp_x
    lda cube_vertices+1,x
    sta temp_x+1

    lda cube_vertices+16,x      ; Y coordinates are offset by 16 bytes
    sta temp_y
    lda cube_vertices+17,x
    sta temp_y+1

    lda cube_vertices+32,x      ; Z coordinates are offset by 32 bytes
    sta temp_z
    lda cube_vertices+33,x
    sta temp_z+1

    ; ----- Rotate around Y axis -----
    ; new_x = x * cos(y) + z * sin(y)
    ; new_z = z * cos(y) - x * sin(y)

    ; Get sin/cos for Y angle
    ldx angle_y
    lda sin_table,x
    sta sin_val
    lda sin_table_hi,x
    sta sin_val+1

    lda cos_table,x
    sta cos_val
    lda cos_table_hi,x
    sta cos_val+1

    ; Calculate x * cos(y)
    lda temp_x
    sta mul_a
    lda temp_x+1
    sta mul_a+1
    lda cos_val
    sta mul_b
    lda cos_val+1
    sta mul_b+1
    jsr multiply_signed
    ; Result in mul_result (use middle 16 bits: mul_result+1, mul_result+2)
    lda mul_result+1
    sta temp0
    lda mul_result+2
    sta temp0+1

    ; Calculate z * sin(y)
    lda temp_z
    sta mul_a
    lda temp_z+1
    sta mul_a+1
    lda sin_val
    sta mul_b
    lda sin_val+1
    sta mul_b+1
    jsr multiply_signed

    ; new_x = x*cos(y) + z*sin(y)
    clc
    lda temp0
    adc mul_result+1
    sta temp0
    lda temp0+1
    adc mul_result+2
    sta temp0+1

    ; Save new_x temporarily
    lda temp0
    pha
    lda temp0+1
    pha

    ; Calculate z * cos(y)
    lda temp_z
    sta mul_a
    lda temp_z+1
    sta mul_a+1
    lda cos_val
    sta mul_b
    lda cos_val+1
    sta mul_b+1
    jsr multiply_signed
    lda mul_result+1
    sta temp0
    lda mul_result+2
    sta temp0+1

    ; Calculate x * sin(y)
    lda temp_x
    sta mul_a
    lda temp_x+1
    sta mul_a+1
    lda sin_val
    sta mul_b
    lda sin_val+1
    sta mul_b+1
    jsr multiply_signed

    ; new_z = z*cos(y) - x*sin(y)
    sec
    lda temp0
    sbc mul_result+1
    sta temp_z
    lda temp0+1
    sbc mul_result+2
    sta temp_z+1

    ; Restore new_x
    pla
    sta temp_x+1
    pla
    sta temp_x

    ; ----- Rotate around X axis -----
    ; new_y = y * cos(x) - z * sin(x)
    ; new_z = y * sin(x) + z * cos(x)

    ; Get sin/cos for X angle
    ldx angle_x
    lda sin_table,x
    sta sin_val
    lda sin_table_hi,x
    sta sin_val+1

    lda cos_table,x
    sta cos_val
    lda cos_table_hi,x
    sta cos_val+1

    ; Calculate y * cos(x)
    lda temp_y
    sta mul_a
    lda temp_y+1
    sta mul_a+1
    lda cos_val
    sta mul_b
    lda cos_val+1
    sta mul_b+1
    jsr multiply_signed
    lda mul_result+1
    sta temp0
    lda mul_result+2
    sta temp0+1

    ; Calculate z * sin(x)
    lda temp_z
    sta mul_a
    lda temp_z+1
    sta mul_a+1
    lda sin_val
    sta mul_b
    lda sin_val+1
    sta mul_b+1
    jsr multiply_signed

    ; new_y = y*cos(x) - z*sin(x)
    sec
    lda temp0
    sbc mul_result+1
    sta temp0
    lda temp0+1
    sbc mul_result+2
    sta temp0+1

    ; Save new_y
    lda temp0
    pha
    lda temp0+1
    pha

    ; Calculate y * sin(x)
    lda temp_y
    sta mul_a
    lda temp_y+1
    sta mul_a+1
    lda sin_val
    sta mul_b
    lda sin_val+1
    sta mul_b+1
    jsr multiply_signed
    lda mul_result+1
    sta temp0
    lda mul_result+2
    sta temp0+1

    ; Calculate z * cos(x)
    lda temp_z
    sta mul_a
    lda temp_z+1
    sta mul_a+1
    lda cos_val
    sta mul_b
    lda cos_val+1
    sta mul_b+1
    jsr multiply_signed

    ; new_z = y*sin(x) + z*cos(x)
    clc
    lda temp0
    adc mul_result+1
    sta temp_z
    lda temp0+1
    adc mul_result+2
    sta temp_z+1

    ; Restore new_y
    pla
    sta temp_y+1
    pla
    sta temp_y

    ; Store rotated coordinates
    lda curr_vertex
    asl a
    tax

    lda temp_x
    sta rot_x,x
    lda temp_x+1
    sta rot_x+1,x

    lda temp_y
    sta rot_y,x
    lda temp_y+1
    sta rot_y+1,x

    lda temp_z
    sta rot_z,x
    lda temp_z+1
    sta rot_z+1,x

    ; Next vertex
    inc curr_vertex
    lda curr_vertex
    cmp #NUM_VERTICES
    beq @done
    jmp @vertex_loop            ; Use JMP for long branch

@done:
    rts
.endproc

; ----------------------------------------------------------------------------
; Project 3D vertices to 2D screen coordinates
; Simple orthographic projection (no perspective)
; screen_x = rot_x_integer + CENTER_X
; screen_y = rot_y_integer + CENTER_Y
; Input: rot_x[], rot_y[], rot_z[]
; Output: proj_x[], proj_y[]
; ----------------------------------------------------------------------------
.proc project_vertices
    lda #0
    sta curr_vertex

@vertex_loop:
    ; Get rotated coordinates (word index = vertex * 2)
    lda curr_vertex
    asl a
    tax

    ; Get X coordinate high byte (integer part of 8.8 fixed point)
    ; The value is signed, range roughly -50 to +50
    lda rot_x+1,x

    ; Add center (128) - this handles signed conversion automatically
    ; For positive: 50 + 128 = 178
    ; For negative: -50 + 128 = 78 (wraps correctly)
    clc
    adc #CENTER_X

    ; Store projected X
    ldy curr_vertex
    sta proj_x,y

    ; Get Y coordinate high byte
    lda rot_y+1,x

    ; Add center (96)
    clc
    adc #CENTER_Y

    ; Clamp Y to valid range (0-191)
    cmp #192
    bcc @y_ok
    ; If >= 192, could be either too large or wrapped negative
    ; Check if it's a wrapped negative (> 200 means was negative)
    cmp #220
    bcs @y_neg
    lda #191                    ; Clamp to max
    jmp @y_ok
@y_neg:
    lda #0                      ; Clamp to min
@y_ok:
    sta proj_y,y

    ; Next vertex
    inc curr_vertex
    lda curr_vertex
    cmp #NUM_VERTICES
    bne @vertex_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Draw all cube edges
; ----------------------------------------------------------------------------
.proc draw_cube
    ldx #0
@edge_loop:
    ; Get vertex indices for this edge
    lda cube_edges,x            ; First vertex
    tay
    lda proj_x,y
    sta line_x0
    lda proj_y,y
    sta line_y0

    inx
    lda cube_edges,x            ; Second vertex
    tay
    lda proj_x,y
    sta line_x1
    lda proj_y,y
    sta line_y1

    ; Save edge index
    txa
    pha

    ; Draw the line
    jsr draw_line

    ; Restore edge index
    pla
    tax

    ; Next edge (2 bytes per edge)
    inx
    cpx #(NUM_EDGES * 2)
    bne @edge_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Draw a line using DDA algorithm
; Input: line_x0, line_y0, line_x1, line_y1
; ----------------------------------------------------------------------------
.proc draw_line
    ; First, plot the starting point
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    ; Check if it's a single point
    lda line_x0
    cmp line_x1
    bne @not_point
    lda line_y0
    cmp line_y1
    bne @not_point
    rts                         ; Single point, done

@not_point:
    ; Calculate dx and sx
    sec
    lda line_x1
    sbc line_x0
    bcs @dx_positive
    ; dx is negative
    eor #$FF
    clc
    adc #1
    sta line_dx
    lda #$FF
    sta line_sx
    jmp @calc_dy
@dx_positive:
    sta line_dx
    lda #$01
    sta line_sx

@calc_dy:
    ; Calculate dy and sy
    sec
    lda line_y1
    sbc line_y0
    bcs @dy_positive
    ; dy is negative
    eor #$FF
    clc
    adc #1
    sta line_dy
    lda #$FF
    sta line_sy
    jmp @start_loop
@dy_positive:
    sta line_dy
    lda #$01
    sta line_sy

@start_loop:
    ; Determine if we step primarily in X or Y
    lda line_dx
    cmp line_dy
    bcc @y_major                ; dy > dx, step in Y

@x_major:
    ; Step along X axis
    lda line_dx
    beq @done                   ; dx=0, done
    sta line_err                ; err = dx / 2
    lsr line_err

@x_loop:
    ; Step X
    clc
    lda line_x0
    adc line_sx
    sta line_x0

    ; Accumulate error
    clc
    lda line_err
    adc line_dy
    sta line_err

    ; Check if we need to step Y
    cmp line_dx
    bcc @x_no_y_step
    ; Step Y and subtract dx from error
    sec
    sbc line_dx
    sta line_err
    clc
    lda line_y0
    adc line_sy
    sta line_y0

@x_no_y_step:
    ; Plot pixel
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    ; Check if done
    lda line_x0
    cmp line_x1
    bne @x_loop
    rts

@y_major:
    ; Step along Y axis
    lda line_dy
    beq @done                   ; dy=0, done
    sta line_err                ; err = dy / 2
    lsr line_err

@y_loop:
    ; Step Y
    clc
    lda line_y0
    adc line_sy
    sta line_y0

    ; Accumulate error
    clc
    lda line_err
    adc line_dx
    sta line_err

    ; Check if we need to step X
    cmp line_dy
    bcc @y_no_x_step
    ; Step X and subtract dy from error
    sec
    sbc line_dy
    sta line_err
    clc
    lda line_x0
    adc line_sx
    sta line_x0

@y_no_x_step:
    ; Plot pixel
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    ; Check if done
    lda line_y0
    cmp line_y1
    bne @y_loop

@done:
    rts
.endproc

; ----------------------------------------------------------------------------
; Plot a single pixel using ULA non-linear addressing
; Input: A = X coordinate (0-255), X = Y coordinate (0-191)
; Uses: zp_ptr0
; ----------------------------------------------------------------------------
.proc plot_pixel
    ; Save X coordinate
    pha

    ; Calculate bitmap address using ULA formula:
    ; addr = ((y & 0xC0) << 5) + ((y & 0x07) << 8) + ((y & 0x38) << 2) + (x >> 3)

    ; First calculate the Y component
    txa                         ; A = Y
    and #$C0                    ; A = y & 0xC0
    ; Shift left 5: equivalent to (y & 0xC0) << 5
    ; But we need to be careful - this can overflow
    ; Actually: (y & 0xC0) << 5 = (y >> 6) << 11 = (y >> 6) * 2048
    ; For y=0-63: result=0, y=64-127: result=2048, y=128-191: result=4096
    lsr a
    lsr a
    lsr a                       ; A = (y & 0xC0) >> 3 = high byte contribution
    sta zp_ptr0+1               ; Store high byte

    ; Now add (y & 0x07) << 8 - this is just (y & 0x07) in the high byte
    txa                         ; A = Y
    and #$07
    clc
    adc zp_ptr0+1
    sta zp_ptr0+1

    ; Now add (y & 0x38) << 2 - this affects both bytes
    txa                         ; A = Y
    and #$38
    asl a
    asl a                       ; A = (y & 0x38) << 2
    sta zp_ptr0                 ; Low byte

    ; Now add x >> 3
    pla                         ; Get X coordinate back
    pha                         ; Save it again for bit mask
    lsr a
    lsr a
    lsr a                       ; A = x >> 3
    clc
    adc zp_ptr0
    sta zp_ptr0
    bcc @no_carry
    inc zp_ptr0+1
@no_carry:

    ; Add ULA VRAM base address ($4000)
    clc
    lda zp_ptr0+1
    adc #>ULA_VRAM
    sta zp_ptr0+1

    ; Now calculate bit mask: bit 7 - (x & 7)
    pla                         ; Get X coordinate
    and #$07                    ; A = x & 7
    tax
    lda bit_masks,x             ; Get bit mask

    ; OR the pixel into the bitmap
    ldy #0
    ora (zp_ptr0),y
    sta (zp_ptr0),y

    rts
.endproc

; ----------------------------------------------------------------------------
; Signed 16-bit multiplication (8.8 fixed point)
; Input: mul_a, mul_b (signed 16-bit)
; Output: mul_result (32-bit, use middle 16 bits for 8.8 result)
; ----------------------------------------------------------------------------
.proc multiply_signed
    ; Clear result
    lda #0
    sta mul_result
    sta mul_result+1
    sta mul_result+2
    sta mul_result+3

    ; Check signs and make operands positive
    lda mul_a+1
    eor mul_b+1
    php                         ; Save sign of result

    ; Make mul_a positive
    lda mul_a+1
    bpl @a_positive
    sec
    lda #0
    sbc mul_a
    sta mul_a
    lda #0
    sbc mul_a+1
    sta mul_a+1
@a_positive:

    ; Make mul_b positive
    lda mul_b+1
    bpl @b_positive
    sec
    lda #0
    sbc mul_b
    sta mul_b
    lda #0
    sbc mul_b+1
    sta mul_b+1
@b_positive:

    ; Unsigned multiply: mul_result = mul_a * mul_b
    ; Using shift-and-add
    ldx #16                     ; 16 bits to process
@mul_loop:
    ; Shift result right
    lsr mul_result+3
    ror mul_result+2
    ror mul_result+1
    ror mul_result

    ; Shift multiplier right, check bit
    lsr mul_b+1
    ror mul_b
    bcc @no_add

    ; Add multiplicand to high bytes of result
    clc
    lda mul_result+2
    adc mul_a
    sta mul_result+2
    lda mul_result+3
    adc mul_a+1
    sta mul_result+3

@no_add:
    dex
    bne @mul_loop

    ; Apply sign to result
    plp
    bpl @done

    ; Negate result
    sec
    lda #0
    sbc mul_result
    sta mul_result
    lda #0
    sbc mul_result+1
    sta mul_result+1
    lda #0
    sbc mul_result+2
    sta mul_result+2
    lda #0
    sbc mul_result+3
    sta mul_result+3

@done:
    rts
.endproc

; ============================================================================
; READ-ONLY DATA
; ============================================================================
.segment "RODATA"

; Bit masks for pixel plotting (MSB = leftmost pixel)
bit_masks:
    .byte $80, $40, $20, $10, $08, $04, $02, $01

; Cube vertices in 8.8 fixed-point format (16-bit signed)
; Stored as: X coords (8 words), Y coords (8 words), Z coords (8 words)
; Vertices are at corners of a cube centered at origin
; -50 * 256 = -12800 = $CE00 (two's complement)
; +50 * 256 = +12800 = $3200
VERT_NEG = $CE00                ; -50 in 8.8 format
VERT_POS = $3200                ; +50 in 8.8 format

cube_vertices:
    ; X coordinates: -CUBE_SIZE or +CUBE_SIZE
    .word VERT_NEG              ; Vertex 0: -,-,-
    .word VERT_POS              ; Vertex 1: +,-,-
    .word VERT_POS              ; Vertex 2: +,+,-
    .word VERT_NEG              ; Vertex 3: -,+,-
    .word VERT_NEG              ; Vertex 4: -,-,+
    .word VERT_POS              ; Vertex 5: +,-,+
    .word VERT_POS              ; Vertex 6: +,+,+
    .word VERT_NEG              ; Vertex 7: -,+,+

    ; Y coordinates
    .word VERT_NEG              ; Vertex 0
    .word VERT_NEG              ; Vertex 1
    .word VERT_POS              ; Vertex 2
    .word VERT_POS              ; Vertex 3
    .word VERT_NEG              ; Vertex 4
    .word VERT_NEG              ; Vertex 5
    .word VERT_POS              ; Vertex 6
    .word VERT_POS              ; Vertex 7

    ; Z coordinates
    .word VERT_NEG              ; Vertex 0
    .word VERT_NEG              ; Vertex 1
    .word VERT_NEG              ; Vertex 2
    .word VERT_NEG              ; Vertex 3
    .word VERT_POS              ; Vertex 4
    .word VERT_POS              ; Vertex 5
    .word VERT_POS              ; Vertex 6
    .word VERT_POS              ; Vertex 7

; Cube edges: pairs of vertex indices
cube_edges:
    ; Front face
    .byte 0, 1
    .byte 1, 2
    .byte 2, 3
    .byte 3, 0
    ; Back face
    .byte 4, 5
    .byte 5, 6
    .byte 6, 7
    .byte 7, 4
    ; Connecting edges
    .byte 0, 4
    .byte 1, 5
    .byte 2, 6
    .byte 3, 7

; Sine table (256 entries, 8.8 fixed-point signed)
; Values: sin(i * 2 * PI / 256) * 256 (gives 8.8 format where 256 = 1.0)
; Range: -256 to +255 (approximately -1.0 to +1.0)
sin_table:
    .byte $00, $06, $0C, $12, $19, $1F, $25, $2B
    .byte $31, $38, $3E, $44, $4A, $50, $56, $5C
    .byte $61, $67, $6D, $73, $78, $7E, $83, $88
    .byte $8E, $93, $98, $9D, $A2, $A7, $AB, $B0
    .byte $B5, $B9, $BD, $C1, $C5, $C9, $CD, $D1
    .byte $D4, $D8, $DB, $DE, $E1, $E4, $E7, $EA
    .byte $EC, $EE, $F1, $F3, $F4, $F6, $F8, $F9
    .byte $FB, $FC, $FD, $FE, $FE, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FE, $FE, $FD, $FC
    .byte $FB, $F9, $F8, $F6, $F4, $F3, $F1, $EE
    .byte $EC, $EA, $E7, $E4, $E1, $DE, $DB, $D8
    .byte $D4, $D1, $CD, $C9, $C5, $C1, $BD, $B9
    .byte $B5, $B0, $AB, $A7, $A2, $9D, $98, $93
    .byte $8E, $88, $83, $7E, $78, $73, $6D, $67
    .byte $61, $5C, $56, $50, $4A, $44, $3E, $38
    .byte $31, $2B, $25, $1F, $19, $12, $0C, $06
    .byte $00, $FA, $F4, $EE, $E7, $E1, $DB, $D5
    .byte $CF, $C8, $C2, $BC, $B6, $B0, $AA, $A4
    .byte $9F, $99, $93, $8D, $88, $82, $7D, $78
    .byte $72, $6D, $68, $63, $5E, $59, $55, $50
    .byte $4B, $47, $43, $3F, $3B, $37, $33, $2F
    .byte $2C, $28, $25, $22, $1F, $1C, $19, $16
    .byte $14, $12, $0F, $0D, $0C, $0A, $08, $07
    .byte $05, $04, $03, $02, $02, $01, $01, $01
    .byte $01, $01, $01, $01, $02, $02, $03, $04
    .byte $05, $07, $08, $0A, $0C, $0D, $0F, $12
    .byte $14, $16, $19, $1C, $1F, $22, $25, $28
    .byte $2C, $2F, $33, $37, $3B, $3F, $43, $47
    .byte $4B, $50, $55, $59, $5E, $63, $68, $6D
    .byte $72, $78, $7D, $82, $88, $8D, $93, $99
    .byte $9F, $A4, $AA, $B0, $B6, $BC, $C2, $C8
    .byte $CF, $D5, $DB, $E1, $E7, $EE, $F4, $FA

; High bytes for sine table (sign extension)
sin_table_hi:
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF

; Cosine table (256 entries, 8.8 fixed-point signed)
; cos(x) = sin(x + 64), so we just offset into the sine table
cos_table:
    .byte $FF, $FF, $FF, $FF, $FE, $FE, $FD, $FC
    .byte $FB, $F9, $F8, $F6, $F4, $F3, $F1, $EE
    .byte $EC, $EA, $E7, $E4, $E1, $DE, $DB, $D8
    .byte $D4, $D1, $CD, $C9, $C5, $C1, $BD, $B9
    .byte $B5, $B0, $AB, $A7, $A2, $9D, $98, $93
    .byte $8E, $88, $83, $7E, $78, $73, $6D, $67
    .byte $61, $5C, $56, $50, $4A, $44, $3E, $38
    .byte $31, $2B, $25, $1F, $19, $12, $0C, $06
    .byte $00, $FA, $F4, $EE, $E7, $E1, $DB, $D5
    .byte $CF, $C8, $C2, $BC, $B6, $B0, $AA, $A4
    .byte $9F, $99, $93, $8D, $88, $82, $7D, $78
    .byte $72, $6D, $68, $63, $5E, $59, $55, $50
    .byte $4B, $47, $43, $3F, $3B, $37, $33, $2F
    .byte $2C, $28, $25, $22, $1F, $1C, $19, $16
    .byte $14, $12, $0F, $0D, $0C, $0A, $08, $07
    .byte $05, $04, $03, $02, $02, $01, $01, $01
    .byte $01, $01, $01, $01, $02, $02, $03, $04
    .byte $05, $07, $08, $0A, $0C, $0D, $0F, $12
    .byte $14, $16, $19, $1C, $1F, $22, $25, $28
    .byte $2C, $2F, $33, $37, $3B, $3F, $43, $47
    .byte $4B, $50, $55, $59, $5E, $63, $68, $6D
    .byte $72, $78, $7D, $82, $88, $8D, $93, $99
    .byte $9F, $A4, $AA, $B0, $B6, $BC, $C2, $C8
    .byte $CF, $D5, $DB, $E1, $E7, $EE, $F4, $FA
    .byte $00, $06, $0C, $12, $19, $1F, $25, $2B
    .byte $31, $38, $3E, $44, $4A, $50, $56, $5C
    .byte $61, $67, $6D, $73, $78, $7E, $83, $88
    .byte $8E, $93, $98, $9D, $A2, $A7, $AB, $B0
    .byte $B5, $B9, $BD, $C1, $C5, $C9, $CD, $D1
    .byte $D4, $D8, $DB, $DE, $E1, $E4, $E7, $EA
    .byte $EC, $EE, $F1, $F3, $F4, $F6, $F8, $F9
    .byte $FB, $FC, $FD, $FE, $FE, $FF, $FF, $FF

; High bytes for cosine table
cos_table_hi:
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $FF, $FF, $FF, $FF, $FF, $FF, $FF, $FF
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00
    .byte $00, $00, $00, $00, $00, $00, $00, $00

; Entry point is at 'start' label
