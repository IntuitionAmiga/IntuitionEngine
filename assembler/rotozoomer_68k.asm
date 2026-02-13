; rotozoomer_68k.asm - Mode7 Blitter Rotozoomer
;
; M68020 rotozoomer using hardware Mode7 affine texture mapping.
; Proper sine tables, smooth 256-level zoom, fractional animation accumulators.
;
; Assemble: vasmm68k_mot -Fbin -m68020 -devpac -o assembler/rotozoomer_68k.ie68 assembler/rotozoomer_68k.asm
; Run:      ./bin/IntuitionEngine -m68k assembler/rotozoomer_68k.ie68

                include "ie68.inc"

TEXTURE_BASE    equ     $500000
BACK_BUFFER     equ     $600000
RENDER_W        equ     640
RENDER_H        equ     480
TEX_STRIDE      equ     1024

; Animation accumulator increments (8.8 fixed-point, 16-bit)
ANGLE_INC       equ     313
SCALE_INC       equ     104

                org     PROGRAM_START

start:
                move.l  #STACK_TOP,sp

                ; Enable VideoChip, mode 0
                move.l  #1,VIDEO_CTRL
                move.l  #0,VIDEO_MODE

                ; Generate 256x256 checkerboard texture via 4x BLIT FILL
                bsr     generate_texture

                ; Init animation accumulators
                clr.l   angle_accum
                clr.l   scale_accum

                ; Start TED music playback (looping)
                move.l  #ted_data,TED_PLAY_PTR
                move.l  #ted_data_end-ted_data,TED_PLAY_LEN
                move.l  #5,TED_PLAY_CTRL

main_loop:
                bsr     compute_frame
                bsr     render_mode7
                bsr     blit_to_front
                bsr     wait_vsync
                bsr     advance_animation
                bra     main_loop

; =============================================================================
; WAIT FOR VSYNC
; =============================================================================
wait_vsync:
.wait_end:      move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                bne.s   .wait_end
.wait_start:    move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                beq.s   .wait_start
                rts

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; =============================================================================
generate_texture:
                ; Top-left 128x128 white
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FFFFFFFF,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w1:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w1

                ; Top-right 128x128 black
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+512,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FF000000,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w2:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w2

                ; Bottom-left 128x128 black
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+131072,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FF000000,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w3:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w3

                ; Bottom-right 128x128 white
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+131584,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FFFFFFFF,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w4:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w4

                rts

; =============================================================================
; COMPUTE FRAME - calculate Mode7 parameters from animation state
; =============================================================================
compute_frame:
                movem.l d0-d7,-(sp)

                ; Get table indices from accumulators
                move.l  angle_accum,d0
                lsr.l   #8,d0
                andi.l  #255,d0                 ; angle_idx

                move.l  scale_accum,d1
                lsr.l   #8,d1
                andi.l  #255,d1                 ; scale_idx

                ; Look up cos = sine_table[(angle_idx + 64) & 255]
                move.l  d0,d2
                addi.l  #64,d2
                andi.l  #255,d2
                add.l   d2,d2                   ; *2 for word access
                lea     sine_table(pc),a0
                move.w  (a0,d2.l),d3            ; d3 = cos_val (signed 16-bit)
                ext.l   d3                      ; sign-extend to 32-bit

                ; Look up sin = sine_table[angle_idx]
                move.l  d0,d2
                add.l   d2,d2                   ; *2 for word access
                move.w  (a0,d2.l),d4            ; d4 = sin_val (signed 16-bit)
                ext.l   d4

                ; Look up recip = recip_table[scale_idx]
                move.l  d1,d2
                add.l   d2,d2                   ; *2 for word access
                lea     recip_table(pc),a1
                move.w  (a1,d2.l),d5            ; d5 = recip (unsigned 16-bit)
                andi.l  #$FFFF,d5               ; zero-extend

                ; CA = cos_val * recip (signed 16 x unsigned 16 -> 32)
                ; Result is 16.16 fixed-point
                move.l  d3,d6
                muls.w  d5,d6                   ; d6 = CA (signed 32-bit, 16.16 FP)

                ; SA = sin_val * recip
                move.l  d4,d7
                muls.w  d5,d7                   ; d7 = SA (signed 32-bit, 16.16 FP)

                ; Store CA, SA for Mode7 register writes
                move.l  d6,var_ca
                move.l  d7,var_sa

                ; u0 = 8388608 - (CA*320 + CA*64) + (SA*256 - SA*16)
                ;    = 8388608 - CA*(256+64) + SA*(256-16)
                ; Using shifts: CA<<8 = CA*256, CA<<6 = CA*64, etc.
                ; But CA is already 16.16, so we shift the RESULT
                ; u0 = 8388608 - (CA<<8 + CA<<6) + (SA<<8 - SA<<4)
                ;    = 8388608 - CA*320 + SA*240
                ; Actually the plan says:
                ; u0 = 8388608 - (CA<<8 + CA<<6) + (SA<<8 - SA<<4)
                ; where shifts are on the 16.16 value, meaning multiply by 320/240

                ; u0 = 8388608 - CA*320 + SA*240
                ; Compute CA*320 via shifts: CA*256 + CA*64 = (CA<<8) + (CA<<6)
                move.l  d6,d0                   ; CA
                asr.l   #2,d0                   ; CA>>2 (for *64 relative to *256)
                ; Actually let's just use the shift approach from the plan directly:
                ; CA<<8 = CA * 256, CA<<6 = CA * 64
                ; But CA is already the multiply result, those shifts would overflow.
                ; Better: just multiply CA * 320 directly.
                ; M68K MULS.W is 16x16->32, but CA might be >16 bits.
                ; Use shift decomposition: 320 = 256 + 64
                move.l  d6,d0                   ; d0 = CA
                move.l  d0,d1
                lsl.l   #8,d0                   ; CA * 256
                lsl.l   #6,d1                   ; CA * 64
                add.l   d1,d0                   ; d0 = CA * 320

                move.l  d7,d1                   ; d1 = SA
                move.l  d1,d2
                lsl.l   #8,d1                   ; SA * 256
                lsl.l   #4,d2                   ; SA * 16
                sub.l   d2,d1                   ; d1 = SA * 240

                move.l  #$800000,d3             ; 8388608 = 128 << 16
                sub.l   d0,d3                   ; - CA*320
                add.l   d1,d3                   ; + SA*240
                move.l  d3,var_u0

                ; v0 = 8388608 - (SA<<8 + SA<<6) - (CA<<8 - CA<<4)
                ;    = 8388608 - SA*320 - CA*240
                move.l  d7,d0                   ; SA
                move.l  d0,d1
                lsl.l   #8,d0                   ; SA * 256
                lsl.l   #6,d1                   ; SA * 64
                add.l   d1,d0                   ; d0 = SA * 320

                move.l  d6,d1                   ; CA
                move.l  d1,d2
                lsl.l   #8,d1                   ; CA * 256
                lsl.l   #4,d2                   ; CA * 16
                sub.l   d2,d1                   ; d1 = CA * 240

                move.l  #$800000,d3             ; 8388608
                sub.l   d0,d3                   ; - SA*320
                sub.l   d1,d3                   ; - CA*240
                move.l  d3,var_v0

                movem.l (sp)+,d0-d7
                rts

; =============================================================================
; RENDER MODE7 - configure and trigger Mode7 blit
; =============================================================================
render_mode7:
                ; Set blitter operation
                move.l  #BLT_OP_MODE7,BLT_OP

                ; Source = texture, Dest = back buffer
                move.l  #TEXTURE_BASE,BLT_SRC
                move.l  #BACK_BUFFER,BLT_DST

                ; Dimensions
                move.l  #RENDER_W,BLT_WIDTH
                move.l  #RENDER_H,BLT_HEIGHT

                ; Strides
                move.l  #TEX_STRIDE,BLT_SRC_STRIDE
                move.l  #LINE_BYTES,BLT_DST_STRIDE

                ; Texture masks
                move.l  #255,BLT_MODE7_TEX_W
                move.l  #255,BLT_MODE7_TEX_H

                ; Mode7 parameters (all 16.16 fixed-point, stored as uint32)
                move.l  var_u0,d0
                move.l  d0,BLT_MODE7_U0

                move.l  var_v0,d0
                move.l  d0,BLT_MODE7_V0

                move.l  var_ca,d0
                move.l  d0,BLT_MODE7_DU_COL       ; du_col = CA

                move.l  var_sa,d0
                move.l  d0,BLT_MODE7_DV_COL       ; dv_col = SA

                neg.l   d0                          ; -SA
                move.l  d0,BLT_MODE7_DU_ROW       ; du_row = -SA

                move.l  var_ca,d0
                move.l  d0,BLT_MODE7_DV_ROW       ; dv_row = CA

                ; Trigger blit
                move.l  #1,BLT_CTRL

                ; Wait for completion
.wait:          move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .wait

                rts

; =============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM)
; =============================================================================
blit_to_front:
                move.l  #BLT_OP_COPY,BLT_OP
                move.l  #BACK_BUFFER,BLT_SRC
                move.l  #VRAM_START,BLT_DST
                move.l  #RENDER_W,BLT_WIDTH
                move.l  #RENDER_H,BLT_HEIGHT
                move.l  #LINE_BYTES,BLT_SRC_STRIDE
                move.l  #LINE_BYTES,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL

.wait:          move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .wait

                rts

; =============================================================================
; ADVANCE ANIMATION - update fractional accumulators
; =============================================================================
advance_animation:
                move.l  angle_accum,d0
                addi.l  #ANGLE_INC,d0
                andi.l  #$FFFF,d0
                move.l  d0,angle_accum

                move.l  scale_accum,d0
                addi.l  #SCALE_INC,d0
                andi.l  #$FFFF,d0
                move.l  d0,scale_accum

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
                even
ted_data:
                incbin  "chromatic_admiration.ted"
ted_data_end:
