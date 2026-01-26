; ============================================================================
; ULA STATIC CUBE DEMO - IE65 (6502)
;
; A simple static wireframe cube to demonstrate ULA graphics.
; No rotation - just draws a 3D-looking cube using pre-calculated coordinates.
; ============================================================================

.include "ie65.inc"

; ============================================================================
; ZERO PAGE VARIABLES
; ============================================================================
.segment "ZEROPAGE"

; Line drawing variables
line_x0:        .res 1
line_y0:        .res 1
line_x1:        .res 1
line_y1:        .res 1
line_dx:        .res 1
line_dy:        .res 1
line_sx:        .res 1
line_sy:        .res 1
line_err:       .res 2
line_e2:        .res 2
edge_idx:       .res 1

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

; ----------------------------------------------------------------------------
; Entry Point
; ----------------------------------------------------------------------------
.proc start
    ; Set border to blue
    lda #1
    sta ULA_BORDER

    ; Enable ULA
    lda #ULA_CTRL_ENABLE
    sta ULA_CTRL

    ; Clear screen
    jsr clear_screen

    ; Set attributes to white on black
    jsr set_attributes

    ; Draw the cube
    jsr draw_cube

    ; Infinite loop
@done:
    jmp @done
.endproc

; ----------------------------------------------------------------------------
; Clear screen (bitmap area)
; ----------------------------------------------------------------------------
.proc clear_screen
    lda #<ULA_VRAM
    sta zp_ptr0
    lda #>ULA_VRAM
    sta zp_ptr0+1

    lda #0
    ldy #0
    ldx #24                     ; 24 pages = 6144 bytes

@page_loop:
@byte_loop:
    sta (zp_ptr0),y
    iny
    bne @byte_loop
    inc zp_ptr0+1
    dex
    bne @page_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Set attributes to white on black
; ----------------------------------------------------------------------------
.proc set_attributes
    lda #<(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0
    lda #>(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0+1

    lda #$07                    ; White ink, black paper
    ldy #0
    ldx #3                      ; 768 bytes = 3 pages

@page_loop:
@byte_loop:
    sta (zp_ptr0),y
    iny
    bne @byte_loop
    inc zp_ptr0+1
    dex
    bne @page_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Draw the wireframe cube
; ----------------------------------------------------------------------------
.proc draw_cube
    lda #0
    sta edge_idx

@edge_loop:
    ; Get edge index * 2 (each edge is 2 vertex indices)
    lda edge_idx
    asl a
    tax

    ; Get first vertex coordinates
    lda cube_edges,x
    tay
    lda vertex_x,y
    sta line_x0
    lda vertex_y,y
    sta line_y0

    ; Get second vertex coordinates
    inx
    lda cube_edges,x
    tay
    lda vertex_x,y
    sta line_x1
    lda vertex_y,y
    sta line_y1

    ; Draw the edge
    jsr draw_line

    ; Next edge
    inc edge_idx
    lda edge_idx
    cmp #12                     ; 12 edges
    bne @edge_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Draw a line using DDA algorithm
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
    rts

@not_point:
    ; Calculate dx and sx
    sec
    lda line_x1
    sbc line_x0
    bcs @dx_positive
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
    lda line_dx
    cmp line_dy
    bcc @y_major

@x_major:
    lda line_dx
    beq @done
    sta line_err
    lsr line_err

@x_loop:
    clc
    lda line_x0
    adc line_sx
    sta line_x0

    clc
    lda line_err
    adc line_dy
    sta line_err

    cmp line_dx
    bcc @x_no_y_step
    sec
    sbc line_dx
    sta line_err
    clc
    lda line_y0
    adc line_sy
    sta line_y0

@x_no_y_step:
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    lda line_x0
    cmp line_x1
    bne @x_loop
    rts

@y_major:
    lda line_dy
    beq @done
    sta line_err
    lsr line_err

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
    bcc @y_no_x_step
    sec
    sbc line_dy
    sta line_err
    clc
    lda line_x0
    adc line_sx
    sta line_x0

@y_no_x_step:
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    lda line_y0
    cmp line_y1
    bne @y_loop

@done:
    rts
.endproc

; ----------------------------------------------------------------------------
; Plot a single pixel using ULA non-linear addressing
; Input: A = X coordinate (0-255), X = Y coordinate (0-191)
; ----------------------------------------------------------------------------
.proc plot_pixel
    pha

    ; High byte from (y & 0xC0) >> 3
    txa
    and #$C0
    lsr a
    lsr a
    lsr a
    sta zp_ptr0+1

    ; Add (y & 0x07) to high byte
    txa
    and #$07
    clc
    adc zp_ptr0+1
    sta zp_ptr0+1

    ; Low byte from (y & 0x38) << 2
    txa
    and #$38
    asl a
    asl a
    sta zp_ptr0

    ; Add x >> 3 to low byte
    pla
    pha
    lsr a
    lsr a
    lsr a
    clc
    adc zp_ptr0
    sta zp_ptr0
    bcc @no_carry
    inc zp_ptr0+1
@no_carry:

    ; Add ULA VRAM base ($4000)
    clc
    lda zp_ptr0+1
    adc #>ULA_VRAM
    sta zp_ptr0+1

    ; Get bit mask
    pla
    and #$07
    tax
    lda bit_masks,x

    ; OR the pixel into the bitmap
    ldy #0
    ora (zp_ptr0),y
    sta (zp_ptr0),y

    rts
.endproc

; ============================================================================
; DATA
; ============================================================================
.segment "RODATA"

; Pre-calculated vertex positions for a nice-looking cube
; Front face (z=near): vertices 0-3
; Back face (z=far): vertices 4-7
vertex_x:
    .byte 88                    ; 0: front top-left
    .byte 168                   ; 1: front top-right
    .byte 168                   ; 2: front bottom-right
    .byte 88                    ; 3: front bottom-left
    .byte 68                    ; 4: back top-left
    .byte 188                   ; 5: back top-right
    .byte 188                   ; 6: back bottom-right
    .byte 68                    ; 7: back bottom-left

vertex_y:
    .byte 56                    ; 0: front top-left
    .byte 56                    ; 1: front top-right
    .byte 136                   ; 2: front bottom-right
    .byte 136                   ; 3: front bottom-left
    .byte 36                    ; 4: back top-left
    .byte 36                    ; 5: back top-right
    .byte 156                   ; 6: back bottom-right
    .byte 156                   ; 7: back bottom-left

; Cube edges: pairs of vertex indices
cube_edges:
    ; Front face
    .byte 0, 1                  ; top
    .byte 1, 2                  ; right
    .byte 2, 3                  ; bottom
    .byte 3, 0                  ; left
    ; Back face
    .byte 4, 5                  ; top
    .byte 5, 6                  ; right
    .byte 6, 7                  ; bottom
    .byte 7, 4                  ; left
    ; Connecting edges
    .byte 0, 4                  ; front-top-left to back-top-left
    .byte 1, 5                  ; front-top-right to back-top-right
    .byte 2, 6                  ; front-bottom-right to back-bottom-right
    .byte 3, 7                  ; front-bottom-left to back-bottom-left

; Bit masks for pixel plotting
bit_masks:
    .byte $80, $40, $20, $10, $08, $04, $02, $01
