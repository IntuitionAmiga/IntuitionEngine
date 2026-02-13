; rotozoomer_65.asm - Mode7 Blitter Rotozoomer
;
; 6502 rotozoomer using hardware Mode7 affine texture mapping.
; Proper sine tables, smooth 256-level zoom, fractional animation accumulators.
; Self-contained: no external binary blobs.
;
; Build: make ie65asm SRC=assembler/rotozoomer_65.asm
; Run:   ./bin/IntuitionEngine -m6502 assembler/rotozoomer_65.ie65

.include "ie65.inc"

RENDER_W         = 640
RENDER_H         = 480
TEXTURE_BASE_LO  = $00
TEXTURE_BASE_MI  = $00
TEXTURE_BASE_HI  = $50
TEXTURE_BASE     = $500000
BACK_BUFFER      = $600000
TEX_STRIDE       = 1024
TEX_W_MASK       = 255
TEX_H_MASK       = 255

; Animation increments
ANGLE_INC_LO     = <313          ; 313 = $0139
ANGLE_INC_HI     = >313
SCALE_INC_LO     = <104          ; 104 = $0068
SCALE_INC_HI     = >104

; ---------------------------------------------------------------------------
; Zero page
; ---------------------------------------------------------------------------
.segment "ZEROPAGE"
angle_accum:     .res 2          ; 16-bit fractional accumulator
scale_accum:     .res 2
var_ca:          .res 4          ; 32-bit signed (16.16 FP)
var_sa:          .res 4
var_u0:          .res 4
var_v0:          .res 4
mul_a:           .res 2          ; 16-bit multiply operands
mul_b:           .res 2
mul_result:      .res 4          ; 32-bit result
tmp32_a:         .res 4          ; 32-bit temporaries
tmp32_b:         .res 4
tmp32_c:         .res 4
sign_flag:       .res 1

; ---------------------------------------------------------------------------
.segment "CODE"
; ---------------------------------------------------------------------------

.proc main
    ; Enable VideoChip
    lda #1
    sta VIDEO_CTRL
    lda #0
    sta VIDEO_MODE

    ; Generate checkerboard texture
    jsr generate_texture

    ; Init accumulators to 0
    lda #0
    sta angle_accum
    sta angle_accum+1
    sta scale_accum
    sta scale_accum+1

loop:
    jsr compute_frame
    jsr render_mode7
    jsr blit_to_front
    WAIT_VBLANK
    jsr advance_animation
    jmp loop
.endproc

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; =============================================================================
.proc generate_texture
    ; Top-left 128x128 white
    STORE32 BLT_OP, 1
    STORE32 BLT_DST_0, TEXTURE_BASE
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FFFFFFFF
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; Top-right 128x128 black
    STORE32 BLT_OP, 1
    STORE32 BLT_DST_0, TEXTURE_BASE+512
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FF000000
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; Bottom-left 128x128 black
    STORE32 BLT_OP, 1
    STORE32 BLT_DST_0, TEXTURE_BASE+131072
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FF000000
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; Bottom-right 128x128 white
    STORE32 BLT_OP, 1
    STORE32 BLT_DST_0, TEXTURE_BASE+131584
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FFFFFFFF
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    rts
.endproc

; =============================================================================
; COMPUTE FRAME
; =============================================================================
.proc compute_frame
    ; angle_idx = angle_accum >> 8 = high byte
    ldx angle_accum+1      ; X = angle_idx

    ; scale_idx = scale_accum >> 8
    ldy scale_accum+1      ; Y = scale_idx

    ; cos_val = sine_table[(angle_idx + 64) & 255]
    txa
    clc
    adc #64                 ; A = (angle_idx + 64) & 0xFF (natural wrap)
    asl a                   ; *2 for word index
    tax                     ; save in X
    lda sine_table,x
    sta mul_a
    lda sine_table+1,x
    sta mul_a+1             ; mul_a = cos_val (signed 16-bit)

    ; recip = recip_table[scale_idx]
    tya
    asl a
    tax
    lda recip_table,x
    sta mul_b
    lda recip_table+1,x
    sta mul_b+1             ; mul_b = recip (unsigned 16-bit)

    ; CA = cos_val * recip
    jsr mul16_signed
    lda mul_result
    sta var_ca
    lda mul_result+1
    sta var_ca+1
    lda mul_result+2
    sta var_ca+2
    lda mul_result+3
    sta var_ca+3

    ; sin_val = sine_table[angle_idx]
    lda angle_accum+1
    asl a
    tax
    lda sine_table,x
    sta mul_a
    lda sine_table+1,x
    sta mul_a+1             ; mul_a = sin_val

    ; recip already in mul_b
    lda scale_accum+1
    asl a
    tax
    lda recip_table,x
    sta mul_b
    lda recip_table+1,x
    sta mul_b+1

    ; SA = sin_val * recip
    jsr mul16_signed
    lda mul_result
    sta var_sa
    lda mul_result+1
    sta var_sa+1
    lda mul_result+2
    sta var_sa+2
    lda mul_result+3
    sta var_sa+3

    ; u0 = 0x800000 - CA*320 + SA*240
    ; CA*320 = CA*256 + CA*64 = (CA<<8) + (CA<<6)
    jsr compute_ca_320      ; result in tmp32_a
    jsr compute_sa_240      ; result in tmp32_b

    ; u0 = 0x800000 - tmp32_a + tmp32_b
    ; Start with 0x800000
    lda #$00
    sta var_u0
    sta var_u0+1
    lda #$80
    sta var_u0+2
    lda #$00
    sta var_u0+3

    ; Subtract CA*320
    sec
    lda var_u0
    sbc tmp32_a
    sta var_u0
    lda var_u0+1
    sbc tmp32_a+1
    sta var_u0+1
    lda var_u0+2
    sbc tmp32_a+2
    sta var_u0+2
    lda var_u0+3
    sbc tmp32_a+3
    sta var_u0+3

    ; Add SA*240
    clc
    lda var_u0
    adc tmp32_b
    sta var_u0
    lda var_u0+1
    adc tmp32_b+1
    sta var_u0+1
    lda var_u0+2
    adc tmp32_b+2
    sta var_u0+2
    lda var_u0+3
    adc tmp32_b+3
    sta var_u0+3

    ; v0 = 0x800000 - SA*320 - CA*240
    jsr compute_sa_320      ; result in tmp32_a
    jsr compute_ca_240      ; result in tmp32_b

    lda #$00
    sta var_v0
    sta var_v0+1
    lda #$80
    sta var_v0+2
    lda #$00
    sta var_v0+3

    sec
    lda var_v0
    sbc tmp32_a
    sta var_v0
    lda var_v0+1
    sbc tmp32_a+1
    sta var_v0+1
    lda var_v0+2
    sbc tmp32_a+2
    sta var_v0+2
    lda var_v0+3
    sbc tmp32_a+3
    sta var_v0+3

    sec
    lda var_v0
    sbc tmp32_b
    sta var_v0
    lda var_v0+1
    sbc tmp32_b+1
    sta var_v0+1
    lda var_v0+2
    sbc tmp32_b+2
    sta var_v0+2
    lda var_v0+3
    sbc tmp32_b+3
    sta var_v0+3

    rts
.endproc

; --- CA*320 -> tmp32_a ---
.proc compute_ca_320
    ; CA*256 (byte shift: 0, ca[0], ca[1], ca[2])
    lda #0
    sta tmp32_c
    lda var_ca
    sta tmp32_c+1
    lda var_ca+1
    sta tmp32_c+2
    lda var_ca+2
    sta tmp32_c+3

    ; CA*64 (shift left 6)
    lda var_ca
    sta tmp32_a
    lda var_ca+1
    sta tmp32_a+1
    lda var_ca+2
    sta tmp32_a+2
    lda var_ca+3
    sta tmp32_a+3
    ldx #6
:   asl tmp32_a
    rol tmp32_a+1
    rol tmp32_a+2
    rol tmp32_a+3
    dex
    bne :-

    ; CA*320 = tmp32_c + tmp32_a
    clc
    lda tmp32_c
    adc tmp32_a
    sta tmp32_a
    lda tmp32_c+1
    adc tmp32_a+1
    sta tmp32_a+1
    lda tmp32_c+2
    adc tmp32_a+2
    sta tmp32_a+2
    lda tmp32_c+3
    adc tmp32_a+3
    sta tmp32_a+3
    rts
.endproc

; --- SA*320 -> tmp32_a ---
.proc compute_sa_320
    lda #0
    sta tmp32_c
    lda var_sa
    sta tmp32_c+1
    lda var_sa+1
    sta tmp32_c+2
    lda var_sa+2
    sta tmp32_c+3

    lda var_sa
    sta tmp32_a
    lda var_sa+1
    sta tmp32_a+1
    lda var_sa+2
    sta tmp32_a+2
    lda var_sa+3
    sta tmp32_a+3
    ldx #6
:   asl tmp32_a
    rol tmp32_a+1
    rol tmp32_a+2
    rol tmp32_a+3
    dex
    bne :-

    clc
    lda tmp32_c
    adc tmp32_a
    sta tmp32_a
    lda tmp32_c+1
    adc tmp32_a+1
    sta tmp32_a+1
    lda tmp32_c+2
    adc tmp32_a+2
    sta tmp32_a+2
    lda tmp32_c+3
    adc tmp32_a+3
    sta tmp32_a+3
    rts
.endproc

; --- SA*240 -> tmp32_b ---
.proc compute_sa_240
    ; SA*256
    lda #0
    sta tmp32_c
    lda var_sa
    sta tmp32_c+1
    lda var_sa+1
    sta tmp32_c+2
    lda var_sa+2
    sta tmp32_c+3

    ; SA*16 (shift left 4)
    lda var_sa
    sta tmp32_b
    lda var_sa+1
    sta tmp32_b+1
    lda var_sa+2
    sta tmp32_b+2
    lda var_sa+3
    sta tmp32_b+3
    ldx #4
:   asl tmp32_b
    rol tmp32_b+1
    rol tmp32_b+2
    rol tmp32_b+3
    dex
    bne :-

    ; SA*240 = SA*256 - SA*16
    sec
    lda tmp32_c
    sbc tmp32_b
    sta tmp32_b
    lda tmp32_c+1
    sbc tmp32_b+1
    sta tmp32_b+1
    lda tmp32_c+2
    sbc tmp32_b+2
    sta tmp32_b+2
    lda tmp32_c+3
    sbc tmp32_b+3
    sta tmp32_b+3
    rts
.endproc

; --- CA*240 -> tmp32_b ---
.proc compute_ca_240
    lda #0
    sta tmp32_c
    lda var_ca
    sta tmp32_c+1
    lda var_ca+1
    sta tmp32_c+2
    lda var_ca+2
    sta tmp32_c+3

    lda var_ca
    sta tmp32_b
    lda var_ca+1
    sta tmp32_b+1
    lda var_ca+2
    sta tmp32_b+2
    lda var_ca+3
    sta tmp32_b+3
    ldx #4
:   asl tmp32_b
    rol tmp32_b+1
    rol tmp32_b+2
    rol tmp32_b+3
    dex
    bne :-

    sec
    lda tmp32_c
    sbc tmp32_b
    sta tmp32_b
    lda tmp32_c+1
    sbc tmp32_b+1
    sta tmp32_b+1
    lda tmp32_c+2
    sbc tmp32_b+2
    sta tmp32_b+2
    lda tmp32_c+3
    sbc tmp32_b+3
    sta tmp32_b+3
    rts
.endproc

; =============================================================================
; MUL16_SIGNED: mul_result (32-bit) = mul_a * mul_b (signed 16 x 16)
; =============================================================================
.proc mul16_signed
    lda #0
    sta sign_flag

    ; Check sign of mul_a
    lda mul_a+1
    bpl @a_pos
    ; Negate mul_a
    sec
    lda #0
    sbc mul_a
    sta mul_a
    lda #0
    sbc mul_a+1
    sta mul_a+1
    inc sign_flag
@a_pos:
    ; Check sign of mul_b
    lda mul_b+1
    bpl @b_pos
    sec
    lda #0
    sbc mul_b
    sta mul_b
    lda #0
    sbc mul_b+1
    sta mul_b+1
    inc sign_flag
@b_pos:
    ; Unsigned 16x16 multiply
    jsr mul16u

    ; If sign_flag is odd, negate result
    lda sign_flag
    and #1
    beq @done
    ; Negate 32-bit result
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

; =============================================================================
; MUL16U: mul_result (32-bit) = mul_a * mul_b (unsigned 16x16 -> 32)
; Shift-and-add algorithm
; =============================================================================
.proc mul16u
    ; Clear result
    lda #0
    sta mul_result
    sta mul_result+1
    sta mul_result+2
    sta mul_result+3

    ldx #16                 ; 16 bits
@loop:
    ; Shift result left
    asl mul_result
    rol mul_result+1
    rol mul_result+2
    rol mul_result+3

    ; Shift mul_a left, test top bit
    asl mul_a
    rol mul_a+1
    bcc @no_add

    ; Add mul_b to high word of result
    clc
    lda mul_result+2
    adc mul_b
    sta mul_result+2
    lda mul_result+3
    adc mul_b+1
    sta mul_result+3
@no_add:
    dex
    bne @loop
    rts
.endproc

; =============================================================================
; RENDER MODE7
; =============================================================================
.proc render_mode7
    STORE32 BLT_OP, BLT_OP_MODE7_OP
    STORE32 BLT_SRC_0, TEXTURE_BASE
    STORE32 BLT_DST_0, BACK_BUFFER
    STORE32 BLT_WIDTH_LO, RENDER_W
    STORE32 BLT_HEIGHT_LO, RENDER_H
    STORE32 BLT_SRC_STRIDE_LO, TEX_STRIDE
    STORE32 BLT_DST_STRIDE_LO, LINE_BYTES
    STORE32 BLT_MODE7_TEX_W_0, TEX_W_MASK
    STORE32 BLT_MODE7_TEX_H_0, TEX_H_MASK

    ; u0
    lda var_u0
    sta BLT_MODE7_U0_0
    lda var_u0+1
    sta BLT_MODE7_U0_1
    lda var_u0+2
    sta BLT_MODE7_U0_2
    lda var_u0+3
    sta BLT_MODE7_U0_3

    ; v0
    lda var_v0
    sta BLT_MODE7_V0_0
    lda var_v0+1
    sta BLT_MODE7_V0_1
    lda var_v0+2
    sta BLT_MODE7_V0_2
    lda var_v0+3
    sta BLT_MODE7_V0_3

    ; du_col = CA
    lda var_ca
    sta BLT_MODE7_DU_COL_0
    lda var_ca+1
    sta BLT_MODE7_DU_COL_1
    lda var_ca+2
    sta BLT_MODE7_DU_COL_2
    lda var_ca+3
    sta BLT_MODE7_DU_COL_3

    ; dv_col = SA
    lda var_sa
    sta BLT_MODE7_DV_COL_0
    lda var_sa+1
    sta BLT_MODE7_DV_COL_1
    lda var_sa+2
    sta BLT_MODE7_DV_COL_2
    lda var_sa+3
    sta BLT_MODE7_DV_COL_3

    ; du_row = -SA (negate)
    sec
    lda #0
    sbc var_sa
    sta BLT_MODE7_DU_ROW_0
    lda #0
    sbc var_sa+1
    sta BLT_MODE7_DU_ROW_1
    lda #0
    sbc var_sa+2
    sta BLT_MODE7_DU_ROW_2
    lda #0
    sbc var_sa+3
    sta BLT_MODE7_DU_ROW_3

    ; dv_row = CA
    lda var_ca
    sta BLT_MODE7_DV_ROW_0
    lda var_ca+1
    sta BLT_MODE7_DV_ROW_1
    lda var_ca+2
    sta BLT_MODE7_DV_ROW_2
    lda var_ca+3
    sta BLT_MODE7_DV_ROW_3

    START_BLIT
    WAIT_BLIT
    rts
.endproc

; =============================================================================
; BLIT BACK BUFFER TO FRONT
; =============================================================================
.proc blit_to_front
    STORE32 BLT_OP, 0
    STORE32 BLT_SRC_0, BACK_BUFFER
    STORE32 BLT_DST_0, VRAM_START
    STORE32 BLT_WIDTH_LO, RENDER_W
    STORE32 BLT_HEIGHT_LO, RENDER_H
    STORE32 BLT_SRC_STRIDE_LO, LINE_BYTES
    STORE32 BLT_DST_STRIDE_LO, LINE_BYTES
    START_BLIT
    WAIT_BLIT
    rts
.endproc

; =============================================================================
; ADVANCE ANIMATION
; =============================================================================
.proc advance_animation
    clc
    lda angle_accum
    adc #ANGLE_INC_LO
    sta angle_accum
    lda angle_accum+1
    adc #ANGLE_INC_HI
    sta angle_accum+1       ; natural 16-bit wrap

    clc
    lda scale_accum
    adc #SCALE_INC_LO
    sta scale_accum
    lda scale_accum+1
    adc #SCALE_INC_HI
    sta scale_accum+1

    rts
.endproc

; =============================================================================
; DATA TABLES
; =============================================================================
.segment "RODATA"

; SINE TABLE - 256 entries, signed 16-bit LE, round(sin(i*2pi/256)*256)
sine_table:
    .word 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
    .word 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
    .word 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
    .word 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
    .word 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
    .word 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
    .word 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
    .word 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
    .word 0,65530,65523,65517,65511,65505,65498,65492,65486,65480,65474,65468,65462,65456,65450,65444
    .word 65438,65432,65427,65421,65415,65410,65404,65399,65394,65389,65384,65379,65374,65369,65364,65359
    .word 65355,65351,65346,65342,65338,65334,65330,65327,65323,65320,65316,65313,65310,65307,65305,65302
    .word 65299,65297,65295,65293,65291,65289,65288,65286,65285,65284,65283,65282,65281,65281,65280,65280
    .word 65280,65280,65280,65281,65281,65282,65283,65284,65285,65286,65288,65289,65291,65293,65295,65297
    .word 65299,65302,65305,65307,65310,65313,65316,65320,65323,65327,65330,65334,65338,65342,65346,65351
    .word 65355,65359,65364,65369,65374,65379,65384,65389,65394,65399,65404,65410,65415,65421,65427,65432
    .word 65438,65444,65450,65456,65462,65468,65474,65480,65486,65492,65498,65505,65511,65517,65523,65530

; RECIPROCAL TABLE - 256 entries, unsigned 16-bit LE
recip_table:
    .word 512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
    .word 416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
    .word 359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
    .word 329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
    .word 320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
    .word 329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
    .word 359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
    .word 416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
    .word 512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
    .word 665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
    .word 889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
    .word 1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
    .word 1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
    .word 1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
    .word 889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
    .word 665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520
