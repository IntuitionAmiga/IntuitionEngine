; ============================================================================
; ULA BOX DEMO - IE65 (6502)
;
; A simple static box to test ULA line drawing
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

    ; Clear screen (all zeros)
    jsr clear_screen

    ; Set attributes to white on black
    jsr set_attributes

    ; Draw a box in the center of the screen
    ; Top edge: (64, 48) to (192, 48)
    lda #64
    sta line_x0
    lda #48
    sta line_y0
    lda #192
    sta line_x1
    lda #48
    sta line_y1
    jsr draw_line

    ; Right edge: (192, 48) to (192, 144)
    lda #192
    sta line_x0
    lda #48
    sta line_y0
    lda #192
    sta line_x1
    lda #144
    sta line_y1
    jsr draw_line

    ; Bottom edge: (192, 144) to (64, 144)
    lda #192
    sta line_x0
    lda #144
    sta line_y0
    lda #64
    sta line_x1
    lda #144
    sta line_y1
    jsr draw_line

    ; Left edge: (64, 144) to (64, 48)
    lda #64
    sta line_x0
    lda #144
    sta line_y0
    lda #64
    sta line_x1
    lda #48
    sta line_y1
    jsr draw_line

    ; Draw diagonal: (64, 48) to (192, 144)
    lda #64
    sta line_x0
    lda #48
    sta line_y0
    lda #192
    sta line_x1
    lda #144
    sta line_y1
    jsr draw_line

    ; Draw other diagonal: (192, 48) to (64, 144)
    lda #192
    sta line_x0
    lda #48
    sta line_y0
    lda #64
    sta line_x1
    lda #144
    sta line_y1
    jsr draw_line

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
; Set attributes to white on black for entire screen
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
; Draw a line using simple DDA algorithm (simpler than Bresenham)
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
    beq @done                   ; dx=0, done (shouldn't happen here)
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
    beq @done                   ; dy=0, done (shouldn't happen here)
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
; ----------------------------------------------------------------------------
.proc plot_pixel
    ; Save X coordinate
    pha

    ; Calculate bitmap address using ULA formula:
    ; addr = ((y & 0xC0) << 5) + ((y & 0x07) << 8) + ((y & 0x38) << 2) + (x >> 3)

    ; High byte from (y & 0xC0) >> 3
    txa                         ; A = Y
    and #$C0
    lsr a
    lsr a
    lsr a                       ; A = (y & 0xC0) >> 3
    sta zp_ptr0+1

    ; Add (y & 0x07) to high byte
    txa                         ; A = Y
    and #$07
    clc
    adc zp_ptr0+1
    sta zp_ptr0+1

    ; Low byte from (y & 0x38) << 2
    txa                         ; A = Y
    and #$38
    asl a
    asl a                       ; A = (y & 0x38) << 2
    sta zp_ptr0

    ; Add x >> 3 to low byte
    pla                         ; Get X coordinate
    pha                         ; Save again for bit mask
    lsr a
    lsr a
    lsr a                       ; A = x >> 3
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

    ; Calculate bit mask (bit 7 - (x & 7))
    pla                         ; Get X coordinate
    and #$07
    tax
    lda bit_masks,x

    ; OR the pixel into the bitmap
    ldy #0
    ora (zp_ptr0),y
    sta (zp_ptr0),y

    rts
.endproc

; ----------------------------------------------------------------------------
; DATA
; ----------------------------------------------------------------------------
.segment "RODATA"

; Bit masks for pixel plotting (MSB = leftmost)
bit_masks:
    .byte $80, $40, $20, $10, $08, $04, $02, $01
