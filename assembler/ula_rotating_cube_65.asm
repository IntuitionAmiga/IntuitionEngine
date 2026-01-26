; ============================================================================
; ULA ROTATING CUBE DEMO - IE65 (6502)
;
; Rotating wireframe cube with dual-axis rotation (X and Y).
; 32 frames for smooth animation.
; ============================================================================

.include "ie65.inc"

; ============================================================================
; ZERO PAGE VARIABLES
; ============================================================================
.segment "ZEROPAGE"

line_x0:        .res 1
line_y0:        .res 1
line_x1:        .res 1
line_y1:        .res 1
line_dx:        .res 1
line_dy:        .res 1
line_sx:        .res 1
line_sy:        .res 1
line_err:       .res 2
edge_idx:       .res 1
curr_frame:     .res 1
frame_ptr_x:    .res 2
frame_ptr_y:    .res 2

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

.proc start
    lda #1
    sta ULA_BORDER

    lda #ULA_CTRL_ENABLE
    sta ULA_CTRL

    lda #0
    sta curr_frame

main_loop:
    ; Wait for VBlank before drawing (sync to display)
    jsr wait_vblank

    jsr clear_screen
    jsr set_attributes
    jsr setup_frame_ptrs
    jsr draw_cube

    ; Wait another VBlank for ~2 frames per animation step
    jsr wait_vblank

    ; Next frame
    inc curr_frame
    lda curr_frame
    cmp #32             ; 32 frames for full rotation
    bne main_loop
    lda #0
    sta curr_frame
    jmp main_loop
.endproc

; ----------------------------------------------------------------------------
; Wait for VBlank - reads ULA_STATUS until VBlank bit is set
; The VBlank bit is cleared on read, so we poll until it becomes set
; ----------------------------------------------------------------------------
.proc wait_vblank
@wait:
    lda ULA_STATUS
    and #ULA_STATUS_VBLANK
    beq @wait
    rts
.endproc

; ----------------------------------------------------------------------------
; Set up frame pointers
; ----------------------------------------------------------------------------
.proc setup_frame_ptrs
    lda curr_frame
    asl a
    asl a
    asl a               ; *8

    clc
    adc #<all_vertex_x
    sta frame_ptr_x
    lda #>all_vertex_x
    adc #0
    sta frame_ptr_x+1

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

; ----------------------------------------------------------------------------
; Clear screen
; ----------------------------------------------------------------------------
.proc clear_screen
    lda #<ULA_VRAM
    sta zp_ptr0
    lda #>ULA_VRAM
    sta zp_ptr0+1

    lda #0
    ldy #0
    ldx #24

@loop:
    sta (zp_ptr0),y
    iny
    bne @loop
    inc zp_ptr0+1
    dex
    bne @loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Set attributes
; ----------------------------------------------------------------------------
.proc set_attributes
    lda #<(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0
    lda #>(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0+1

    lda #$07
    ldy #0
    ldx #3

@loop:
    sta (zp_ptr0),y
    iny
    bne @loop
    inc zp_ptr0+1
    dex
    bne @loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Draw cube
; ----------------------------------------------------------------------------
.proc draw_cube
    lda #0
    sta edge_idx

@edge_loop:
    lda edge_idx
    asl a
    tax

    lda cube_edges,x
    tay
    lda (frame_ptr_x),y
    sta line_x0
    lda (frame_ptr_y),y
    sta line_y0

    inx
    lda cube_edges,x
    tay
    lda (frame_ptr_x),y
    sta line_x1
    lda (frame_ptr_y),y
    sta line_y1

    jsr draw_line

    inc edge_idx
    lda edge_idx
    cmp #12
    bne @edge_loop

    rts
.endproc

; ----------------------------------------------------------------------------
; Draw line (DDA)
; ----------------------------------------------------------------------------
.proc draw_line
    lda line_x0
    ldx line_y0
    jsr plot_pixel

    lda line_x0
    cmp line_x1
    bne @not_point
    lda line_y0
    cmp line_y1
    bne @not_point
    rts

@not_point:
    sec
    lda line_x1
    sbc line_x0
    bcs @dx_pos
    eor #$FF
    clc
    adc #1
    sta line_dx
    lda #$FF
    sta line_sx
    jmp @calc_dy
@dx_pos:
    sta line_dx
    lda #$01
    sta line_sx

@calc_dy:
    sec
    lda line_y1
    sbc line_y0
    bcs @dy_pos
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
    bcc @x_noy
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
    bcc @y_nox
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

; ----------------------------------------------------------------------------
; Plot pixel
; ----------------------------------------------------------------------------
.proc plot_pixel
    pha
    txa
    and #$C0
    lsr a
    lsr a
    lsr a
    sta zp_ptr0+1
    txa
    and #$07
    clc
    adc zp_ptr0+1
    sta zp_ptr0+1
    txa
    and #$38
    asl a
    asl a
    sta zp_ptr0
    pla
    pha
    lsr a
    lsr a
    lsr a
    clc
    adc zp_ptr0
    sta zp_ptr0
    bcc @nc
    inc zp_ptr0+1
@nc:
    clc
    lda zp_ptr0+1
    adc #>ULA_VRAM
    sta zp_ptr0+1
    pla
    and #$07
    tax
    lda bit_masks,x
    ldy #0
    ora (zp_ptr0),y
    sta (zp_ptr0),y
    rts
.endproc

; ============================================================================
; DATA - 32 frames of dual-axis rotation (X and Y combined)
; ============================================================================
.segment "RODATA"

bit_masks:
    .byte $80, $40, $20, $10, $08, $04, $02, $01

cube_edges:
    .byte 0, 1, 1, 2, 2, 3, 3, 0     ; Front face
    .byte 4, 5, 5, 6, 6, 7, 7, 4     ; Back face
    .byte 0, 4, 1, 5, 2, 6, 3, 7     ; Connecting edges

; Pre-calculated vertices for 32 frames of X+Y rotation
; Cube centered at (128, 96), size 80 pixels (±40 from center)
; Full Y rotation + half X rotation for tumbling effect

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
    .byte 35, 85, 146, 97, 45, 94, 156, 106      ; Frame 7
    .byte 39, 96, 152, 96, 39, 96, 152, 96        ; Frame 8
    .byte 46, 106, 157, 97, 34, 94, 145, 85      ; Frame 9
    .byte 55, 117, 161, 100, 30, 91, 136, 74      ; Frame 10
    .byte 67, 126, 163, 105, 28, 86, 124, 65      ; Frame 11
    .byte 80, 132, 163, 111, 28, 80, 111, 59      ; Frame 12
    .byte 94, 137, 160, 118, 31, 73, 97, 54      ; Frame 13
    .byte 109, 139, 155, 125, 36, 66, 82, 52      ; Frame 14
    .byte 123, 138, 146, 131, 45, 60, 68, 53      ; Frame 15
    .byte 136, 136, 136, 136, 55, 56, 56, 55      ; Frame 16
    .byte 146, 131, 123, 138, 68, 53, 45, 60      ; Frame 17
    .byte 155, 125, 109, 139, 82, 52, 36, 66      ; Frame 18
    .byte 160, 118, 94, 137, 97, 54, 31, 73      ; Frame 19
    .byte 163, 111, 80, 132, 111, 59, 28, 80      ; Frame 20
    .byte 163, 105, 67, 126, 124, 65, 28, 86      ; Frame 21
    .byte 161, 100, 55, 117, 136, 74, 30, 91      ; Frame 22
    .byte 157, 97, 46, 106, 145, 85, 34, 94      ; Frame 23
    .byte 152, 96, 39, 96, 152, 95, 39, 96        ; Frame 24
    .byte 146, 97, 35, 85, 156, 106, 45, 94      ; Frame 25
    .byte 141, 100, 33, 74, 158, 117, 50, 91      ; Frame 26
    .byte 136, 105, 34, 65, 157, 126, 55, 86      ; Frame 27
    .byte 132, 111, 37, 59, 154, 132, 59, 80      ; Frame 28
    .byte 131, 118, 41, 54, 150, 137, 60, 73      ; Frame 29
    .byte 131, 125, 46, 52, 145, 139, 60, 66      ; Frame 30
    .byte 132, 131, 51, 53, 140, 138, 59, 60      ; Frame 31
