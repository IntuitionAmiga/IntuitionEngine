; rotozoomer.asm - Mode7 Blitter Rotozoomer
;
; IE32 rotozoomer using hardware Mode7 affine texture mapping.
; Proper sine tables, smooth 256-level zoom, fractional animation accumulators.
;
; Assemble: ./bin/ie32asm assembler/rotozoomer.asm
; Run:      ./bin/IntuitionEngine -ie32 assembler/rotozoomer.iex

.include "ie32.inc"

.equ TEXTURE_BASE   0x500000
.equ TEX_TR         0x500200
.equ TEX_BL         0x520000
.equ TEX_BR         0x520200
.equ BACK_BUFFER    0x600000
.equ RENDER_W       640
.equ RENDER_H       480
.equ TEX_STRIDE     1024
.equ LINE_BYTES_V   2560

; Animation accumulator increments (8.8 fixed-point)
.equ ANGLE_INC      313
.equ SCALE_INC      104

; Variable addresses
.equ VAR_BASE       0x46C20
.equ VAR_ANGLE_ACC  0x46C20
.equ VAR_SCALE_ACC  0x46C24
.equ VAR_CA         0x46C28
.equ VAR_SA         0x46C2C
.equ VAR_U0         0x46C30
.equ VAR_V0         0x46C34

.org 0x1000

start:
    ; Enable VideoChip, mode 0
    LDA #1
    STA @VIDEO_CTRL
    LDA #0
    STA @VIDEO_MODE

    ; Generate 256x256 checkerboard texture via 4x BLIT FILL
    JSR generate_texture

    ; Init animation accumulators
    LDA #0
    STA @VAR_ANGLE_ACC
    STA @VAR_SCALE_ACC

    ; Start AHX music playback (looping)
    LDA #ahx_data
    STA @AHX_PLAY_PTR
    LDA #ahx_data_end
    LDB #ahx_data
    SUB A, B
    STA @AHX_PLAY_LEN
    LDA #5
    STA @AHX_PLAY_CTRL

main_loop:
    JSR compute_frame
    JSR render_mode7
    JSR blit_to_front
    JSR wait_vsync
    JSR advance_animation
    JMP main_loop

; =============================================================================
; WAIT FOR VSYNC
; =============================================================================
wait_vsync:
wait_end:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JNZ A, wait_end
wait_start:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JZ A, wait_start
    RTS

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; =============================================================================
generate_texture:
    ; Top-left 128x128 white
    LDA #1
    STA @BLT_OP
    LDA #TEXTURE_BASE
    STA @BLT_DST
    LDA #128
    STA @BLT_WIDTH
    STA @BLT_HEIGHT
    LDA #0xFFFFFFFF
    STA @BLT_COLOR
    LDA #TEX_STRIDE
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
gt_w1:
    LDA @BLT_STATUS
    AND A, #2
    JNZ A, gt_w1

    ; Top-right 128x128 black
    LDA #1
    STA @BLT_OP
    LDA #TEX_TR
    STA @BLT_DST
    LDA #128
    STA @BLT_WIDTH
    STA @BLT_HEIGHT
    LDA #0xFF000000
    STA @BLT_COLOR
    LDA #TEX_STRIDE
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
gt_w2:
    LDA @BLT_STATUS
    AND A, #2
    JNZ A, gt_w2

    ; Bottom-left 128x128 black
    LDA #1
    STA @BLT_OP
    LDA #TEX_BL
    STA @BLT_DST
    LDA #128
    STA @BLT_WIDTH
    STA @BLT_HEIGHT
    LDA #0xFF000000
    STA @BLT_COLOR
    LDA #TEX_STRIDE
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
gt_w3:
    LDA @BLT_STATUS
    AND A, #2
    JNZ A, gt_w3

    ; Bottom-right 128x128 white
    LDA #1
    STA @BLT_OP
    LDA #TEX_BR
    STA @BLT_DST
    LDA #128
    STA @BLT_WIDTH
    STA @BLT_HEIGHT
    LDA #0xFFFFFFFF
    STA @BLT_COLOR
    LDA #TEX_STRIDE
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
gt_w4:
    LDA @BLT_STATUS
    AND A, #2
    JNZ A, gt_w4

    RTS

; =============================================================================
; COMPUTE FRAME - calculate Mode7 parameters from animation state
; Register usage: D=angle_idx, E=scale_idx, F=cos_val, G=sin_val,
; H=recip, S=CA, T=SA; A,B,C=scratch (used by mul helpers)
; =============================================================================
compute_frame:
    ; angle_idx = (angle_accum >> 8) & 255
    LDA @VAR_ANGLE_ACC
    SHR A, #8
    AND A, #255
    LDD A                ; D = angle_idx

    ; scale_idx = (scale_accum >> 8) & 255
    LDA @VAR_SCALE_ACC
    SHR A, #8
    AND A, #255
    LDE A                ; E = scale_idx

    ; cos_val = sine_table[(angle_idx + 64) & 255]
    LDA D
    ADD A, #64
    AND A, #255
    SHL A, #2            ; *4 for 32-bit entries
    ADD A, #sine_table
    LDA [A]              ; A = cos_val (signed, already sign-extended in table)
    LDF A                ; F = cos_val

    ; sin_val = sine_table[angle_idx]
    LDA D
    SHL A, #2
    ADD A, #sine_table
    LDA [A]
    LDG A                ; G = sin_val

    ; recip = recip_table[scale_idx]
    LDA E
    SHL A, #2
    ADD A, #recip_table
    LDA [A]
    LDH A                ; H = recip (unsigned)

    ; CA = cos_val * recip (signed 16-bit * unsigned 16-bit)
    LDA F
    LDB H
    JSR signed_mul       ; A = CA
    STA @VAR_CA
    LDS A                ; S = CA

    ; SA = sin_val * recip
    LDA G
    LDB H
    JSR signed_mul       ; A = SA
    STA @VAR_SA
    LDT A                ; T = SA

    ; u0 = 8388608 - CA*320 + SA*240
    LDA S
    JSR mul_320           ; A = CA*320
    LDU A                ; U = CA*320
    LDA T
    JSR mul_240           ; A = SA*240
    LDB #0x800000
    SUB B, U             ; B = 0x800000 - CA*320
    ADD B, A             ; B += SA*240
    STB @VAR_U0

    ; v0 = 8388608 - SA*320 - CA*240
    LDA T
    JSR mul_320           ; A = SA*320
    LDU A                ; U = SA*320
    LDA S
    JSR mul_240           ; A = CA*240
    LDB #0x800000
    SUB B, U             ; B = 0x800000 - SA*320
    SUB B, A             ; B -= CA*240
    STB @VAR_V0

    RTS

; =============================================================================
; SIGNED MULTIPLY: A = A * B (signed result)
; IE32 MUL is unsigned -> check signs, abs, MUL, conditionally negate
; Clobbers: A, B, C (C is saved/restored)
; =============================================================================
signed_mul:
    PUSH C
    LDC #0               ; C = sign flag (0=positive, 1=negative)

    ; Check sign of A (JLT tests int32 < 0)
    JGE A, sm_a_pos
    XOR A, #0xFFFFFFFF
    ADD A, #1            ; A = -A (two's complement negate)
    XOR C, #1
sm_a_pos:
    ; Check sign of B
    JGE B, sm_b_pos
    XOR B, #0xFFFFFFFF
    ADD B, #1
    XOR C, #1
sm_b_pos:
    MUL A, B             ; unsigned multiply
    ; If sign flag set, negate result
    JZ C, sm_done
    XOR A, #0xFFFFFFFF
    ADD A, #1
sm_done:
    POP C
    RTS

; =============================================================================
; MUL_320: A = A * 320 (signed) using shifts: 320 = 256 + 64
; Clobbers: A, B, C (B, C saved/restored)
; =============================================================================
mul_320:
    PUSH B
    PUSH C
    LDC #0
    JGE A, m320_pos
    XOR A, #0xFFFFFFFF
    ADD A, #1
    LDC #1
m320_pos:
    LDB A
    SHL A, #8            ; A*256
    SHL B, #6            ; B*64
    ADD A, B             ; A*320
    JZ C, m320_done
    XOR A, #0xFFFFFFFF
    ADD A, #1
m320_done:
    POP C
    POP B
    RTS

; =============================================================================
; MUL_240: A = A * 240 (signed) using shifts: 240 = 256 - 16
; Clobbers: A, B, C (B, C saved/restored)
; =============================================================================
mul_240:
    PUSH B
    PUSH C
    LDC #0
    JGE A, m240_pos
    XOR A, #0xFFFFFFFF
    ADD A, #1
    LDC #1
m240_pos:
    LDB A
    SHL A, #8            ; A*256
    SHL B, #4            ; B*16
    SUB A, B             ; A*240
    JZ C, m240_done
    XOR A, #0xFFFFFFFF
    ADD A, #1
m240_done:
    POP C
    POP B
    RTS

; =============================================================================
; RENDER MODE7 - configure and trigger Mode7 blit
; =============================================================================
render_mode7:
    LDA #5
    STA @BLT_OP

    LDA #TEXTURE_BASE
    STA @BLT_SRC
    LDA #BACK_BUFFER
    STA @BLT_DST

    LDA #RENDER_W
    STA @BLT_WIDTH
    LDA #RENDER_H
    STA @BLT_HEIGHT

    LDA #TEX_STRIDE
    STA @BLT_SRC_STRIDE
    LDA #LINE_BYTES_V
    STA @BLT_DST_STRIDE

    LDA #255
    STA @BLT_MODE7_TEX_W
    STA @BLT_MODE7_TEX_H

    ; Mode7 parameters
    LDA @VAR_U0
    STA @BLT_MODE7_U0

    LDA @VAR_V0
    STA @BLT_MODE7_V0

    LDA @VAR_CA
    STA @BLT_MODE7_DU_COL       ; du_col = CA

    LDA @VAR_SA
    STA @BLT_MODE7_DV_COL       ; dv_col = SA

    XOR A, #0xFFFFFFFF
    ADD A, #1                     ; -SA (two's complement negate)
    STA @BLT_MODE7_DU_ROW       ; du_row = -SA

    LDA @VAR_CA
    STA @BLT_MODE7_DV_ROW       ; dv_row = CA

    ; Trigger blit
    LDA #1
    STA @BLT_CTRL

rm7_wait:
    LDA @BLT_STATUS
    AND A, #2
    JNZ A, rm7_wait

    RTS

; =============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM)
; =============================================================================
blit_to_front:
    LDA #0
    STA @BLT_OP
    LDA #BACK_BUFFER
    STA @BLT_SRC
    LDA #VRAM_START
    STA @BLT_DST
    LDA #RENDER_W
    STA @BLT_WIDTH
    LDA #RENDER_H
    STA @BLT_HEIGHT
    LDA #LINE_BYTES_V
    STA @BLT_SRC_STRIDE
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL

btf_wait:
    LDA @BLT_STATUS
    AND A, #2
    JNZ A, btf_wait

    RTS

; =============================================================================
; ADVANCE ANIMATION
; =============================================================================
advance_animation:
    LDA @VAR_ANGLE_ACC
    ADD A, #ANGLE_INC
    AND A, #0xFFFF
    STA @VAR_ANGLE_ACC

    LDA @VAR_SCALE_ACC
    ADD A, #SCALE_INC
    AND A, #0xFFFF
    STA @VAR_SCALE_ACC

    RTS

; =============================================================================
; SINE TABLE - 256 entries, 32-bit sign-extended (IE32 .word = 32-bit)
; round(sin(i*2pi/256)*256)
; =============================================================================
sine_table:
    .word 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
    .word 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
    .word 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
    .word 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
    .word 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
    .word 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
    .word 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
    .word 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
    .word 0,0xFFFFFFFA,0xFFFFFFF3,0xFFFFFFED,0xFFFFFFE7,0xFFFFFFE1,0xFFFFFFDA,0xFFFFFFD4,0xFFFFFFCE,0xFFFFFFC8,0xFFFFFFC2,0xFFFFFFBC,0xFFFFFFB6,0xFFFFFFB0,0xFFFFFFAA,0xFFFFFFA4
    .word 0xFFFFFF9E,0xFFFFFF98,0xFFFFFF93,0xFFFFFF8D,0xFFFFFF87,0xFFFFFF82,0xFFFFFF7C,0xFFFFFF77,0xFFFFFF72,0xFFFFFF6D,0xFFFFFF68,0xFFFFFF63,0xFFFFFF5E,0xFFFFFF59,0xFFFFFF54,0xFFFFFF4F
    .word 0xFFFFFF4B,0xFFFFFF47,0xFFFFFF42,0xFFFFFF3E,0xFFFFFF3A,0xFFFFFF36,0xFFFFFF32,0xFFFFFF2F,0xFFFFFF2B,0xFFFFFF28,0xFFFFFF24,0xFFFFFF21,0xFFFFFF1E,0xFFFFFF1B,0xFFFFFF19,0xFFFFFF16
    .word 0xFFFFFF13,0xFFFFFF11,0xFFFFFF0F,0xFFFFFF0D,0xFFFFFF0B,0xFFFFFF09,0xFFFFFF08,0xFFFFFF06,0xFFFFFF05,0xFFFFFF04,0xFFFFFF03,0xFFFFFF02,0xFFFFFF01,0xFFFFFF01,0xFFFFFF00,0xFFFFFF00
    .word 0xFFFFFF00,0xFFFFFF00,0xFFFFFF00,0xFFFFFF01,0xFFFFFF01,0xFFFFFF02,0xFFFFFF03,0xFFFFFF04,0xFFFFFF05,0xFFFFFF06,0xFFFFFF08,0xFFFFFF09,0xFFFFFF0B,0xFFFFFF0D,0xFFFFFF0F,0xFFFFFF11
    .word 0xFFFFFF13,0xFFFFFF16,0xFFFFFF19,0xFFFFFF1B,0xFFFFFF1E,0xFFFFFF21,0xFFFFFF24,0xFFFFFF28,0xFFFFFF2B,0xFFFFFF2F,0xFFFFFF32,0xFFFFFF36,0xFFFFFF3A,0xFFFFFF3E,0xFFFFFF42,0xFFFFFF47
    .word 0xFFFFFF4B,0xFFFFFF4F,0xFFFFFF54,0xFFFFFF59,0xFFFFFF5E,0xFFFFFF63,0xFFFFFF68,0xFFFFFF6D,0xFFFFFF72,0xFFFFFF77,0xFFFFFF7C,0xFFFFFF82,0xFFFFFF87,0xFFFFFF8D,0xFFFFFF93,0xFFFFFF98
    .word 0xFFFFFF9E,0xFFFFFFA4,0xFFFFFFAA,0xFFFFFFB0,0xFFFFFFB6,0xFFFFFFBC,0xFFFFFFC2,0xFFFFFFC8,0xFFFFFFCE,0xFFFFFFD4,0xFFFFFFDA,0xFFFFFFE1,0xFFFFFFE7,0xFFFFFFED,0xFFFFFFF3,0xFFFFFFFA

; =============================================================================
; RECIPROCAL TABLE - 256 entries, 32-bit unsigned (IE32 .word = 32-bit)
; round(256/(0.5+sin(i*2pi/256)*0.3))
; =============================================================================
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

; =============================================================================
; MUSIC DATA
; =============================================================================
ahx_data:
.incbin "Fairlightz.ahx"
ahx_data_end:
