; rotozoomer_65.asm - IE65 (6502) rotozoomer port
;
; Primary path uses Mode7 blitter (fast).
; Secondary path keeps a CPU fallback rasterizer for parity/debug.
;
; Build: make ie65asm SRC=assembler/rotozoomer_65.asm
; Run:   ./bin/IntuitionEngine -6502 assembler/rotozoomer_65.ie65

.include "ie65.inc"

; ---------------------------------------------------------------------------
; Constants
; ---------------------------------------------------------------------------
RENDER_W        = 640
RENDER_H        = 480

VRAM_START_ADDR = $100000

; Table bank layout matches the Z80 rotozoomer table generator.
TABLES_BANK_BASE = $0007
TABLES_BANKS     = 3
TABLES_BANK_SIZE = $2000

; Small procedural checker texture for runtime generation.
; 256x256x4 = 262144 bytes -> 32 banks.
TEXTURE_BANK_BASE = $000A
TEXTURE_BANKS     = 32
TEXTURE_BASE_ADDR = $014000
TEXTURE_W_MASK    = 255
TEXTURE_H_MASK    = 255
TEXTURE_ROW_BYTES = 1024

; Table offsets in bank 0.
OFF_SCALE_SEL    = $0420
OFF_SCALE0       = $1000

; Scale block offsets (4KB)
OFF_DU           = $0000
OFF_DV           = $0400
OFF_ROW_U        = $0800
OFF_ROW_V        = $0C00

OFF_SCALE1       = $0000
OFF_SCALE2       = $1000
OFF_SCALE3       = $0000
OFF_SCALE4       = $1000

ZOOM_PHASE_STEP  = 1

; ---------------------------------------------------------------------------
; Zero page
; ---------------------------------------------------------------------------
.segment "ZEROPAGE"
var_angle:           .res 1
var_scale_idx:       .res 1
var_scale_sel:       .res 1
var_scale_bank:      .res 1

var_scale_off:       .res 2
var_temp_offset:     .res 2

src_ptr:             .res 2
dst_ptr:             .res 2
base_ptr:            .res 2

row_count:           .res 2
page_count:          .res 1
bank_count:          .res 1

run_off_lo:          .res 1
run_off_hi:          .res 1
run_bank:            .res 1

y_bank_ptr:          .res 2
y_off_lo_ptr:        .res 2
y_off_hi_ptr:        .res 2

curr_tex_bank:       .res 1
curr_y:              .res 1
row_in_bank:         .res 1

tmp0:                .res 1
tmp1:                .res 1
render_path:         .res 1
zoom_phase_lo:       .res 1
zoom_phase_hi:       .res 1

; ---------------------------------------------------------------------------
; BSS
; ---------------------------------------------------------------------------
.segment "BSS"
var_du_dx:           .res 4
var_dv_dx:           .res 4
var_du_dy:           .res 4
var_dv_dy:           .res 4
var_row_u:           .res 4
var_row_v:           .res 4
var_u:               .res 4
var_v:               .res 4

y_bank:              .res 480
y_off_lo:            .res 480
y_off_hi:            .res 480

; ---------------------------------------------------------------------------
; Macros
; ---------------------------------------------------------------------------
.macro COPY32 src, dst
    lda src
    sta dst
    lda src+1
    sta dst+1
    lda src+2
    sta dst+2
    lda src+3
    sta dst+3
.endmacro

.macro ADD32 dst, src
    clc
    lda dst
    adc src
    sta dst
    lda dst+1
    adc src+1
    sta dst+1
    lda dst+2
    adc src+2
    sta dst+2
    lda dst+3
    adc src+3
    sta dst+3
.endmacro

.macro NEG32 src, dst
    sec
    lda #0
    sbc src
    sta dst
    lda #0
    sbc src+1
    sta dst+1
    lda #0
    sbc src+2
    sta dst+2
    lda #0
    sbc src+3
    sta dst+3
.endmacro

.macro WRITE_MODE7_SHIFT8 src, reg
    lda #0
    sta reg
    lda src
    sta reg+1
    lda src+1
    sta reg+2
    lda src+2
    sta reg+3
.endmacro

; ---------------------------------------------------------------------------
; Code
; ---------------------------------------------------------------------------
.segment "CODE"

.proc start
    jsr init_video
    jsr init_y_addr
    jsr init_texture

    lda #0
    sta var_angle
    lda #192
    sta var_scale_idx
    lda #1
    sta render_path
    lda #0
    sta zoom_phase_lo
    sta zoom_phase_hi

main_loop:
    jsr wait_vsync_edge
    jsr update_animation
    lda render_path
    beq @cpu_path
    jsr render_mode7
    jmp @frame_done
@cpu_path:
    jsr render_rotozoomer
@frame_done:
    jmp main_loop
.endproc

; Wait for a full vblank edge so we step exactly once per display frame.
.proc wait_vsync_edge
@wait_not_vblank:
    lda VIDEO_STATUS
    and #STATUS_VBLANK
    bne @wait_not_vblank

@wait_vblank:
    lda VIDEO_STATUS
    and #STATUS_VBLANK
    beq @wait_vblank
    rts
.endproc

.proc init_video
    lda #0
    sta VIDEO_MODE
    sta VIDEO_MODE+1
    sta VIDEO_MODE+2
    sta VIDEO_MODE+3

    lda #1
    sta VIDEO_CTRL
    rts
.endproc

; Build (bank, offset) for each screen row in back/front VRAM addressing.
.proc init_y_addr
    lda #<y_bank
    sta y_bank_ptr
    lda #>y_bank
    sta y_bank_ptr+1

    lda #<y_off_lo
    sta y_off_lo_ptr
    lda #>y_off_lo
    sta y_off_lo_ptr+1

    lda #<y_off_hi
    sta y_off_hi_ptr
    lda #>y_off_hi
    sta y_off_hi_ptr+1

    lda #0
    sta run_off_lo
    sta run_off_hi
    lda #$8B                    ; BACK_BUFFER bank from Z80 reference
    sta run_bank

    lda #<RENDER_H
    sta row_count
    lda #>RENDER_H
    sta row_count+1

@row_loop:
    ldy #0
    lda run_bank
    sta (y_bank_ptr),y
    lda run_off_lo
    sta (y_off_lo_ptr),y
    lda run_off_hi
    sta (y_off_hi_ptr),y

    inc y_bank_ptr
    bne :+
    inc y_bank_ptr+1
:
    inc y_off_lo_ptr
    bne :+
    inc y_off_lo_ptr+1
:
    inc y_off_hi_ptr
    bne :+
    inc y_off_hi_ptr+1
:

    clc
    lda run_off_lo
    adc #<LINE_BYTES
    sta run_off_lo
    lda run_off_hi
    adc #>LINE_BYTES
    sta run_off_hi

    lda run_off_hi
    cmp #$40
    bcc :+
    sec
    sbc #$40
    sta run_off_hi
    inc run_bank
:

    sec
    lda row_count
    sbc #1
    sta row_count
    lda row_count+1
    sbc #0
    sta row_count+1
    lda row_count
    ora row_count+1
    bne @row_loop
    rts
.endproc

; Copy embedded 24KB rotozoomer tables into banked table RAM (BANK3 banks 7-9).
.proc init_tables
    ; Legacy no-op kept for compatibility with earlier revisions.
    rts
.endproc

; Generate a 256x256 checker texture into BANK1 banks [TEXTURE_BANK_BASE..+31].
.proc init_texture
    lda #TEXTURE_BANK_BASE
    sta curr_tex_bank
    sta BANK1_REG_LO
    lda #0
    sta BANK1_REG_HI

    lda #0
    sta curr_y

    lda #<BANK1_WINDOW
    sta dst_ptr
    lda #>BANK1_WINDOW
    sta dst_ptr+1

@row_loop:
    ldx #0
@pixel_loop:
    txa
    eor curr_y
    and #$40                    ; 64px checker cells to match ShaderToy SQUARE_SIZE
    bne @black

    ; white pixel BGRA = FF FF FF FF
    ldy #0
    lda #$FF
    sta (dst_ptr),y
    iny
    sta (dst_ptr),y
    iny
    sta (dst_ptr),y
    iny
    sta (dst_ptr),y
    jmp @next_pixel

@black:
    ; black pixel BGRA = 00 00 00 FF
    ldy #0
    lda #$00
    sta (dst_ptr),y
    iny
    sta (dst_ptr),y
    iny
    sta (dst_ptr),y
    iny
    lda #$FF
    sta (dst_ptr),y

@next_pixel:
    clc
    lda dst_ptr
    adc #4
    sta dst_ptr
    bcc :+
    inc dst_ptr+1
:

    ; BANK1 window wrap at $4000 (8KB window).
    lda dst_ptr+1
    cmp #$40
    bcc :+
    sec
    sbc #$20
    sta dst_ptr+1
    inc curr_tex_bank
    lda curr_tex_bank
    sta BANK1_REG_LO
:

    inx
    bne @pixel_loop             ; 256 pixels per row (X wraps)

    inc curr_y
    bne @row_loop               ; 256 rows (curr_y wraps)

    rts
.endproc

; Animation update reuses precomputed du/dv/row tables copied into banked RAM.
.proc update_animation
    inc var_angle

    ; Frame phase advances zoom dynamics.
    clc
    lda zoom_phase_lo
    adc #<ZOOM_PHASE_STEP
    sta zoom_phase_lo
    lda zoom_phase_hi
    adc #>ZOOM_PHASE_STEP
    sta zoom_phase_hi

    ; idx = phase low byte (0..255)
    lda zoom_phase_lo
    sta var_scale_idx

    ; src_ptr = mode7_params + idx * 24
    ; Build idx*8 into src_ptr and idx*16 into dst_ptr, then add.
    lda var_scale_idx
    sta src_ptr
    lda #0
    sta src_ptr+1

    asl src_ptr
    rol src_ptr+1
    asl src_ptr
    rol src_ptr+1
    asl src_ptr
    rol src_ptr+1                 ; src_ptr = idx*8

    lda var_scale_idx
    sta dst_ptr
    lda #0
    sta dst_ptr+1
    asl dst_ptr
    rol dst_ptr+1
    asl dst_ptr
    rol dst_ptr+1
    asl dst_ptr
    rol dst_ptr+1
    asl dst_ptr
    rol dst_ptr+1                 ; dst_ptr = idx*16

    clc
    lda src_ptr
    adc dst_ptr
    sta src_ptr
    lda src_ptr+1
    adc dst_ptr+1
    sta src_ptr+1

    clc
    lda src_ptr
    adc #<mode7_params
    sta src_ptr
    lda src_ptr+1
    adc #>mode7_params
    sta src_ptr+1

    ; Packed frame params:
    ; rowU,rowV,duDx,dvDx,duDy,dvDy
    lda #<var_row_u
    sta dst_ptr
    lda #>var_row_u
    sta dst_ptr+1
    jsr load32_from_src_advance

    lda #<var_row_v
    sta dst_ptr
    lda #>var_row_v
    sta dst_ptr+1
    jsr load32_from_src_advance

    lda #<var_du_dx
    sta dst_ptr
    lda #>var_du_dx
    sta dst_ptr+1
    jsr load32_from_src_advance

    lda #<var_dv_dx
    sta dst_ptr
    lda #>var_dv_dx
    sta dst_ptr+1
    jsr load32_from_src_advance

    lda #<var_du_dy
    sta dst_ptr
    lda #>var_du_dy
    sta dst_ptr+1
    jsr load32_from_src_advance

    lda #<var_dv_dy
    sta dst_ptr
    lda #>var_dv_dy
    sta dst_ptr+1
    jsr load32_from_src_advance

    rts
.endproc

.proc load32_from_src_advance
    ldy #0
    lda (src_ptr),y
    sta (dst_ptr),y
    iny
    lda (src_ptr),y
    sta (dst_ptr),y
    iny
    lda (src_ptr),y
    sta (dst_ptr),y
    iny
    lda (src_ptr),y
    sta (dst_ptr),y
    clc
    lda src_ptr
    adc #4
    sta src_ptr
    bcc :+
    inc src_ptr+1
:
    rts
.endproc

.proc render_mode7
    STORE32 BLT_OP, BLT_OP_MODE7
    STORE32 BLT_SRC_0, TEXTURE_BASE_ADDR
    STORE32 BLT_DST_0, VRAM_START_ADDR
    STORE32 BLT_WIDTH_LO, RENDER_W
    STORE32 BLT_HEIGHT_LO, RENDER_H
    STORE32 BLT_SRC_STRIDE_LO, TEXTURE_ROW_BYTES
    STORE32 BLT_DST_STRIDE_LO, LINE_BYTES
    STORE32 BLT_MODE7_TEX_W_0, TEXTURE_W_MASK
    STORE32 BLT_MODE7_TEX_H_0, TEXTURE_H_MASK

    WRITE_MODE7_SHIFT8 var_row_u, BLT_MODE7_U0_0
    WRITE_MODE7_SHIFT8 var_row_v, BLT_MODE7_V0_0
    WRITE_MODE7_SHIFT8 var_du_dx, BLT_MODE7_DU_COL_0
    WRITE_MODE7_SHIFT8 var_dv_dx, BLT_MODE7_DV_COL_0
    WRITE_MODE7_SHIFT8 var_du_dy, BLT_MODE7_DU_ROW_0
    WRITE_MODE7_SHIFT8 var_dv_dy, BLT_MODE7_DV_ROW_0

    START_BLIT
    WAIT_BLIT
    rts
.endproc

; CPU fallback path retained for parity/debugging.
; Uses the same affine increments as Mode7 but does per-pixel sampling in CPU.
.proc render_rotozoomer
    lda #TABLES_BANK_BASE
    sta BANK3_REG_LO
    lda #0
    sta BANK3_REG_HI

    lda #$FF
    sta curr_tex_bank

    lda #<y_bank
    sta y_bank_ptr
    lda #>y_bank
    sta y_bank_ptr+1

    lda #<y_off_lo
    sta y_off_lo_ptr
    lda #>y_off_lo
    sta y_off_lo_ptr+1

    lda #<y_off_hi
    sta y_off_hi_ptr
    lda #>y_off_hi
    sta y_off_hi_ptr+1

    lda #<RENDER_H
    sta row_count
    lda #>RENDER_H
    sta row_count+1

@row_loop:
    ; select destination VRAM bank and row pointer
    ldy #0
    lda (y_bank_ptr),y
    sta VRAM_BANK_REG

    lda (y_off_lo_ptr),y
    sta dst_ptr
    lda (y_off_hi_ptr),y
    clc
    adc #>VRAM_WINDOW
    sta dst_ptr+1

    COPY32 var_row_u, var_u
    COPY32 var_row_v, var_v

    ; 640 pixels
    lda #<RENDER_W
    sta tmp0
    lda #>RENDER_W
    sta tmp1

@col_loop:
    ; texY = (v >> 8) & 255
    lda var_v+1
    and #TEXTURE_H_MASK
    sta row_in_bank             ; reuse as texY

    ; texture bank = base + (texY >> 3)
    lda row_in_bank
    lsr
    lsr
    lsr
    clc
    adc #TEXTURE_BANK_BASE
    cmp curr_tex_bank
    beq :+
    sta curr_tex_bank
    sta BANK1_REG_LO
    lda #0
    sta BANK1_REG_HI
:

    ; tex row offset = (texY & 7) * 1024
    lda row_in_bank
    and #7
    asl
    asl
    clc
    adc #>BANK1_WINDOW
    sta tmp1

    ; texX = (u >> 8) & 255; byte offset = texX * 4 (16-bit)
    lda var_u+1
    and #$FF
    asl
    sta src_ptr
    lda #0
    rol
    sta tmp0
    asl
    rol tmp0

    clc
    lda tmp1
    adc tmp0
    sta src_ptr+1

    ; copy BGRA pixel
    ldy #0
    lda (src_ptr),y
    sta (dst_ptr),y
    iny
    lda (src_ptr),y
    sta (dst_ptr),y
    iny
    lda (src_ptr),y
    sta (dst_ptr),y
    iny
    lda (src_ptr),y
    sta (dst_ptr),y

    ; dst += 4
    clc
    lda dst_ptr
    adc #4
    sta dst_ptr
    bcc :+
    inc dst_ptr+1
:

    ; bank wrap if dst >= $C000
    lda dst_ptr+1
    cmp #$C0
    bcc :+
    sec
    lda dst_ptr+1
    sbc #$40
    sta dst_ptr+1
    lda VRAM_BANK_REG
    clc
    adc #1
    sta VRAM_BANK_REG
:

    ADD32 var_u, var_du_dx
    ADD32 var_v, var_dv_dx

    sec
    lda tmp0
    sbc #1
    sta tmp0
    lda tmp1
    sbc #0
    sta tmp1
    lda tmp0
    ora tmp1
    beq :+
    jmp @col_loop
:

    ADD32 var_row_u, var_du_dy
    ADD32 var_row_v, var_dv_dy

    inc y_bank_ptr
    bne :+
    inc y_bank_ptr+1
:
    inc y_off_lo_ptr
    bne :+
    inc y_off_lo_ptr+1
:
    inc y_off_hi_ptr
    bne :+
    inc y_off_hi_ptr+1
:

    sec
    lda row_count
    sbc #1
    sta row_count
    lda row_count+1
    sbc #0
    sta row_count+1
    lda row_count
    ora row_count+1
    beq :+
    jmp @row_loop
:

    rts
.endproc

; ---------------------------------------------------------------------------
; Embedded data
; ---------------------------------------------------------------------------
.segment "RODATA"
mode7_params:
    .incbin "rotozoomer_65_mode7_params.bin"
