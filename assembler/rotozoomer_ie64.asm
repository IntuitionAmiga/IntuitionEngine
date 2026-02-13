; rotozoomer_ie64.asm - Mode7 Blitter Rotozoomer
;
; IE64 rotozoomer using hardware Mode7 affine texture mapping.
; Proper sine tables, smooth 256-level zoom, fractional animation accumulators.
;
; Assemble: ./bin/ie64asm assembler/rotozoomer_ie64.asm
; Run:      ./bin/IntuitionEngine -ie64 assembler/rotozoomer_ie64.ie64

include "ie64.inc"

TEXTURE_BASE    equ 0x500000
BACK_BUFFER     equ 0x600000
RENDER_W        equ 640
RENDER_H        equ 480
TEX_STRIDE      equ 1024

; Animation accumulator increments (8.8 fixed-point)
ANGLE_INC       equ 313
SCALE_INC       equ 104

org 0x1000

start:
    ; Init stack pointer
    la      r31, STACK_TOP

    ; Enable VideoChip, mode 0
    la      r1, VIDEO_CTRL
    li      r2, #1
    store.l r2, (r1)
    la      r1, VIDEO_MODE
    store.l r0, (r1)

    ; Generate 256x256 checkerboard texture via 4x BLIT FILL
    jsr     generate_texture

    ; Init animation accumulators to 0
    la      r1, angle_accum
    store.l r0, (r1)
    la      r1, scale_accum
    store.l r0, (r1)

    ; Start SAP music playback (looping)
    la      r1, POKEY_PLUS_CTRL
    li      r2, #1
    store.b r2, (r1)
    la      r1, SAP_PLAY_PTR
    la      r2, sap_data
    store.l r2, (r1)
    la      r1, SAP_PLAY_LEN
    la      r2, sap_data_end
    la      r3, sap_data
    sub.l   r2, r2, r3
    store.l r2, (r1)
    la      r1, SAP_PLAY_CTRL
    li      r2, #5
    store.l r2, (r1)

main_loop:
    jsr     compute_frame
    jsr     render_mode7
    jsr     blit_to_front
    jsr     wait_vsync
    jsr     advance_animation
    bra     main_loop

; =============================================================================
; WAIT FOR VSYNC
; =============================================================================
wait_vsync:
    la      r1, VIDEO_STATUS
.wait_end:
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK
    bnez    r2, .wait_end
.wait_start:
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK
    beqz    r2, .wait_start
    rts

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; =============================================================================
generate_texture:
    ; Top-left 128x128 white
    la      r1, BLT_OP
    move.l  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    li      r2, #128
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    store.l r2, (r1)
    la      r1, BLT_COLOR
    li      r2, #0xFFFFFFFF
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #TEX_STRIDE
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
    la      r3, BLT_STATUS
.w1:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w1

    ; Top-right 128x128 black
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE+512
    store.l r2, (r1)
    la      r1, BLT_COLOR
    li      r2, #0xFF000000
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
.w2:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w2

    ; Bottom-left 128x128 black
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE+131072
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
.w3:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w3

    ; Bottom-right 128x128 white
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE+131584
    store.l r2, (r1)
    la      r1, BLT_COLOR
    li      r2, #0xFFFFFFFF
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
.w4:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w4

    rts

; =============================================================================
; COMPUTE FRAME - calculate Mode7 parameters from animation state
; =============================================================================
compute_frame:
    ; Get angle_idx = angle_accum >> 8
    la      r1, angle_accum
    load.l  r5, (r1)
    lsr.l   r5, r5, #8
    and.l   r5, r5, #255                    ; r5 = angle_idx

    ; Get scale_idx = scale_accum >> 8
    la      r1, scale_accum
    load.l  r6, (r1)
    lsr.l   r6, r6, #8
    and.l   r6, r6, #255                    ; r6 = scale_idx

    ; cos_val = sine_table[(angle_idx + 64) & 255]
    ; Tables are dc.w (16-bit LE). Load 32 bits, mask to 16, sign-extend.
    add.l   r1, r5, #64
    and.l   r1, r1, #255
    lsl.l   r1, r1, #1                     ; *2 for word access
    la      r2, sine_table
    add.l   r1, r2, r1
    load.l  r7, (r1)
    and.l   r7, r7, #0xFFFF                ; mask to 16 bits
    lsl.q   r7, r7, #48
    asr.q   r7, r7, #48                    ; r7 = cos_val sign-extended

    ; sin_val = sine_table[angle_idx]
    lsl.l   r1, r5, #1                     ; *2 for word access
    la      r2, sine_table
    add.l   r1, r2, r1
    load.l  r8, (r1)
    and.l   r8, r8, #0xFFFF
    lsl.q   r8, r8, #48
    asr.q   r8, r8, #48                    ; r8 = sin_val sign-extended

    ; recip = recip_table[scale_idx]
    lsl.l   r1, r6, #1                     ; *2 for word access
    la      r2, recip_table
    add.l   r1, r2, r1
    load.l  r9, (r1)
    and.l   r9, r9, #0xFFFF                ; r9 = recip (unsigned, positive)

    ; CA = cos_val * recip (signed * positive -> signed)
    muls.l  r10, r7, r9                    ; r10 = CA (16.16 FP signed)

    ; SA = sin_val * recip
    muls.l  r11, r8, r9                    ; r11 = SA (16.16 FP signed)

    ; Store CA, SA
    la      r1, var_ca
    store.l r10, (r1)
    la      r1, var_sa
    store.l r11, (r1)

    ; u0 = 8388608 - CA*320 + SA*240
    ; Sign-extend CA and SA for .q shifts
    lsl.q   r10, r10, #32
    asr.q   r10, r10, #32                  ; sign-extend CA
    lsl.q   r11, r11, #32
    asr.q   r11, r11, #32                  ; sign-extend SA

    ; CA*320 = CA*256 + CA*64 = (CA<<8) + (CA<<6)
    lsl.q   r1, r10, #8                    ; CA * 256
    lsl.q   r2, r10, #6                    ; CA * 64
    add.q   r1, r1, r2                     ; r1 = CA * 320

    ; SA*240 = SA*256 - SA*16 = (SA<<8) - (SA<<4)
    lsl.q   r2, r11, #8                    ; SA * 256
    lsl.q   r3, r11, #4                    ; SA * 16
    sub.q   r2, r2, r3                     ; r2 = SA * 240

    move.l  r3, #0x800000                  ; 8388608
    lsl.q   r3, r3, #32
    asr.q   r3, r3, #32                    ; sign-extend
    sub.q   r3, r3, r1                     ; - CA*320
    add.q   r3, r3, r2                     ; + SA*240
    la      r1, var_u0
    store.l r3, (r1)

    ; v0 = 8388608 - SA*320 - CA*240
    lsl.q   r1, r11, #8                    ; SA * 256
    lsl.q   r2, r11, #6                    ; SA * 64
    add.q   r1, r1, r2                     ; r1 = SA * 320

    lsl.q   r2, r10, #8                    ; CA * 256
    lsl.q   r3, r10, #4                    ; CA * 16
    sub.q   r2, r2, r3                     ; r2 = CA * 240

    move.l  r3, #0x800000
    lsl.q   r3, r3, #32
    asr.q   r3, r3, #32
    sub.q   r3, r3, r1                     ; - SA*320
    sub.q   r3, r3, r2                     ; - CA*240
    la      r1, var_v0
    store.l r3, (r1)

    rts

; =============================================================================
; RENDER MODE7 - configure and trigger Mode7 blit
; =============================================================================
render_mode7:
    ; BLT_OP = MODE7
    la      r1, BLT_OP
    move.l  r2, #BLT_OP_MODE7
    store.l r2, (r1)

    ; Source = texture, Dest = back buffer
    la      r1, BLT_SRC
    move.l  r2, #TEXTURE_BASE
    store.l r2, (r1)
    la      r1, BLT_DST
    move.l  r2, #BACK_BUFFER
    store.l r2, (r1)

    ; Dimensions
    la      r1, BLT_WIDTH
    move.l  r2, #RENDER_W
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #RENDER_H
    store.l r2, (r1)

    ; Strides
    la      r1, BLT_SRC_STRIDE
    move.l  r2, #TEX_STRIDE
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #LINE_BYTES
    store.l r2, (r1)

    ; Texture masks
    la      r1, BLT_MODE7_TEX_W
    li      r2, #255
    store.l r2, (r1)
    la      r1, BLT_MODE7_TEX_H
    store.l r2, (r1)

    ; Mode7 parameters
    la      r3, var_u0
    load.l  r2, (r3)
    la      r1, BLT_MODE7_U0
    store.l r2, (r1)

    la      r3, var_v0
    load.l  r2, (r3)
    la      r1, BLT_MODE7_V0
    store.l r2, (r1)

    la      r3, var_ca
    load.l  r2, (r3)
    la      r1, BLT_MODE7_DU_COL
    store.l r2, (r1)                        ; du_col = CA

    la      r3, var_sa
    load.l  r2, (r3)
    la      r1, BLT_MODE7_DV_COL
    store.l r2, (r1)                        ; dv_col = SA

    neg.l   r2, r2                          ; -SA
    la      r1, BLT_MODE7_DU_ROW
    store.l r2, (r1)                        ; du_row = -SA

    la      r3, var_ca
    load.l  r2, (r3)
    la      r1, BLT_MODE7_DV_ROW
    store.l r2, (r1)                        ; dv_row = CA

    ; Trigger blit
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)

    ; Wait for completion
    la      r1, BLT_STATUS
.wait:
    load.l  r2, (r1)
    and.l   r2, r2, #2
    bnez    r2, .wait

    rts

; =============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM)
; =============================================================================
blit_to_front:
    la      r1, BLT_OP
    move.l  r2, #BLT_OP_COPY
    store.l r2, (r1)

    la      r1, BLT_SRC
    move.l  r2, #BACK_BUFFER
    store.l r2, (r1)

    la      r1, BLT_DST
    move.l  r2, #VRAM_START
    store.l r2, (r1)

    la      r1, BLT_WIDTH
    move.l  r2, #RENDER_W
    store.l r2, (r1)

    la      r1, BLT_HEIGHT
    move.l  r2, #RENDER_H
    store.l r2, (r1)

    la      r1, BLT_SRC_STRIDE
    move.l  r2, #LINE_BYTES
    store.l r2, (r1)

    la      r1, BLT_DST_STRIDE
    store.l r2, (r1)

    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)

    la      r1, BLT_STATUS
.wait:
    load.l  r2, (r1)
    and.l   r2, r2, #2
    bnez    r2, .wait

    rts

; =============================================================================
; ADVANCE ANIMATION
; =============================================================================
advance_animation:
    la      r1, angle_accum
    load.l  r2, (r1)
    add.l   r2, r2, #ANGLE_INC
    and.l   r2, r2, #0xFFFF
    store.l r2, (r1)

    la      r1, scale_accum
    load.l  r2, (r1)
    add.l   r2, r2, #SCALE_INC
    and.l   r2, r2, #0xFFFF
    store.l r2, (r1)

    rts

; =============================================================================
; VARIABLES
; =============================================================================
angle_accum:    dc.l    0
scale_accum:    dc.l    0
var_ca:         dc.l    0
var_sa:         dc.l    0
var_u0:         dc.l    0
var_v0:         dc.l    0

; =============================================================================
; SINE TABLE - 256 entries, signed 16-bit, round(sin(i*2pi/256)*256)
; =============================================================================
sine_table:
    dc.w    0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
    dc.w    98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
    dc.w    181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
    dc.w    237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
    dc.w    256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
    dc.w    237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
    dc.w    181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
    dc.w    98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
    dc.w    0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
    dc.w    -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
    dc.w    -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
    dc.w    -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256
    dc.w    -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
    dc.w    -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
    dc.w    -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
    dc.w    -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

; =============================================================================
; RECIPROCAL TABLE - 256 entries, unsigned 16-bit, round(256/(0.5+sin(i*2pi/256)*0.3))
; =============================================================================
recip_table:
    dc.w    512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
    dc.w    416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
    dc.w    359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
    dc.w    329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
    dc.w    320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
    dc.w    329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
    dc.w    359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
    dc.w    416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
    dc.w    512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
    dc.w    665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
    dc.w    889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
    dc.w    1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
    dc.w    1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
    dc.w    1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
    dc.w    889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
    dc.w    665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; =============================================================================
; MUSIC DATA
; =============================================================================
sap_data:
    incbin  "Chromaluma.sap"
sap_data_end:
