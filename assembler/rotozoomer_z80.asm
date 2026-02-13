; rotozoomer_z80.asm - Mode7 Blitter Rotozoomer
;
; Z80 rotozoomer using hardware Mode7 affine texture mapping.
; Proper sine tables, smooth 256-level zoom, fractional animation accumulators.
; Self-contained: no external binary blobs.
;
; Assemble: vasmz80_std -Fbin -I. -o assembler/rotozoomer_z80.ie80 assembler/rotozoomer_z80.asm
; Run:      ./bin/IntuitionEngine -z80 assembler/rotozoomer_z80.ie80

    .include "ie80.inc"

.set TEXTURE_BASE,0x500000
.set BACK_BUFFER,0x600000
.set RENDER_W,640
.set RENDER_H,480
.set TEX_STRIDE,1024
.set LINE_BYTES,2560
.set VRAM_START,0x100000
.set BLT_OP_FILL,1
.set BLT_OP_COPY,0
.set BLT_OP_MODE7,5

; Animation accumulator increments
.set ANGLE_INC,313
.set SCALE_INC,104

    .org 0x0000

start:
    ld sp,STACK_TOP

    ; Enable VideoChip
    ld a,1
    ld (VIDEO_CTRL),a
    xor a
    ld (VIDEO_MODE),a

    ; Generate checkerboard texture via 4x BLIT FILL
    call generate_texture

    ; Init animation accumulators to 0
    ld hl,0
    ld (angle_accum),hl
    ld (scale_accum),hl

    ; Start SID music playback (looping)
    ; SID player registers at physical 0xF0E20+ map to Z80 0xFE20+
    ; (translateIO8Bit: 0xFE20 - 0xF000 + 0xF0000 = 0xF0E20)
    ; SET_SID_PTR/LEN/LOOP macros use 20-bit addresses which vasm
    ; truncates to 16-bit, so we write the 16-bit aliases directly.
    ld a,sid_data & 0xFF
    ld (0xFE20),a
    ld a,(sid_data >> 8) & 0xFF
    ld (0xFE21),a
    ld a,(sid_data >> 16) & 0xFF
    ld (0xFE22),a
    ld a,(sid_data >> 24) & 0xFF
    ld (0xFE23),a
    ld a,(sid_data_end-sid_data) & 0xFF
    ld (0xFE24),a
    ld a,((sid_data_end-sid_data) >> 8) & 0xFF
    ld (0xFE25),a
    ld a,((sid_data_end-sid_data) >> 16) & 0xFF
    ld (0xFE26),a
    ld a,((sid_data_end-sid_data) >> 24) & 0xFF
    ld (0xFE27),a
    ld a,5
    ld (0xFE28),a

main_loop:
    call compute_frame
    call render_mode7
    call blit_to_front
    WAIT_VBLANK
    call advance_animation
    jp main_loop

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; =============================================================================
generate_texture:
    ; Top-left 128x128 white
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFFFFFFFF
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; Top-right 128x128 black
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE+512
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFF000000
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; Bottom-left 128x128 black
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE+131072
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFF000000
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; Bottom-right 128x128 white
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE+131584
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFFFFFFFF
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ret

; =============================================================================
; COMPUTE FRAME
; =============================================================================
compute_frame:
    ; angle_idx = angle_accum >> 8 (high byte of 16-bit accum)
    ld a,(angle_accum+1)
    ld (var_angle_idx),a

    ; scale_idx = scale_accum >> 8
    ld a,(scale_accum+1)
    ld (var_scale_idx),a

    ; cos_val = sine_table[(angle_idx + 64) & 255]
    ld a,(var_angle_idx)
    add a,64                ; wraps naturally in 8 bits
    call lookup_sine        ; HL = cos_val (signed 16-bit)
    ld (var_cos),hl

    ; sin_val = sine_table[angle_idx]
    ld a,(var_angle_idx)
    call lookup_sine        ; HL = sin_val
    ld (var_sin),hl

    ; recip = recip_table[scale_idx]
    ld a,(var_scale_idx)
    call lookup_recip       ; HL = recip (unsigned 16-bit)
    ld (var_recip),hl

    ; CA = cos_val * recip (signed 16 x 16 -> signed 32)
    ld hl,(var_cos)
    ld de,(var_recip)
    call mul16_signed       ; DEHL = result (signed 32-bit)
    ld (var_ca),hl
    ld (var_ca+2),de

    ; SA = sin_val * recip
    ld hl,(var_sin)
    ld de,(var_recip)
    call mul16_signed
    ld (var_sa),hl
    ld (var_sa+2),de

    ; u0 = 8388608 - CA*320 + SA*240
    ; CA*320 = (CA<<8) + (CA<<6)
    call compute_ca_320     ; result in var_tmp1
    call compute_sa_240     ; result in var_tmp2

    ; u0 = 0x800000 - tmp1 + tmp2
    ld hl,0x0000           ; low word of 0x800000
    ld de,(var_tmp1)
    or a
    sbc hl,de
    ld (var_u0),hl
    ld hl,0x0080           ; high word of 0x800000
    ld de,(var_tmp1+2)
    sbc hl,de
    ld (var_u0+2),hl
    ; + SA*240
    ld hl,(var_u0)
    ld de,(var_tmp2)
    add hl,de
    ld (var_u0),hl
    ld hl,(var_u0+2)
    ld de,(var_tmp2+2)
    adc hl,de
    ld (var_u0+2),hl

    ; v0 = 8388608 - SA*320 - CA*240
    call compute_sa_320     ; result in var_tmp1
    call compute_ca_240     ; result in var_tmp2

    ld hl,0x0000
    ld de,(var_tmp1)
    or a
    sbc hl,de
    ld (var_v0),hl
    ld hl,0x0080
    ld de,(var_tmp1+2)
    sbc hl,de
    ld (var_v0+2),hl
    ; - CA*240
    ld hl,(var_v0)
    ld de,(var_tmp2)
    or a
    sbc hl,de
    ld (var_v0),hl
    ld hl,(var_v0+2)
    ld de,(var_tmp2+2)
    sbc hl,de
    ld (var_v0+2),hl

    ret

; --- Helper: CA*320 -> var_tmp1 ---
compute_ca_320:
    ; CA*256 (shift left 8 = byte shift)
    ld a,(var_ca)
    ld (var_tmp3+1),a
    ld a,(var_ca+1)
    ld (var_tmp3+2),a
    ld a,(var_ca+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a
    ; Sign extend byte 3 from bit 7 of var_ca+2
    ld a,(var_ca+3)
    ; Actually CA is 32-bit, CA*256 could overflow. But CA<<8:
    ; byte 0 = 0, byte 1 = CA[0], byte 2 = CA[1], byte 3 = CA[2]
    ; We lose CA[3], but CA fits in ~20 bits so this is fine.

    ; CA*64 (shift left 6)
    ld hl,(var_ca)
    ld de,(var_ca+2)
    call shift_left_6

    ; CA*320 = CA*256 + CA*64
    ld bc,(var_tmp3)
    add hl,bc
    ld (var_tmp1),hl
    ex de,hl
    ld bc,(var_tmp3+2)
    adc hl,bc
    ld (var_tmp1+2),hl
    ret

; --- Helper: SA*320 -> var_tmp1 ---
compute_sa_320:
    ld a,(var_sa)
    ld (var_tmp3+1),a
    ld a,(var_sa+1)
    ld (var_tmp3+2),a
    ld a,(var_sa+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a

    ld hl,(var_sa)
    ld de,(var_sa+2)
    call shift_left_6

    ld bc,(var_tmp3)
    add hl,bc
    ld (var_tmp1),hl
    ex de,hl
    ld bc,(var_tmp3+2)
    adc hl,bc
    ld (var_tmp1+2),hl
    ret

; --- Helper: SA*240 -> var_tmp2 ---
compute_sa_240:
    ; SA*256
    ld a,(var_sa)
    ld (var_tmp3+1),a
    ld a,(var_sa+1)
    ld (var_tmp3+2),a
    ld a,(var_sa+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a

    ; SA*16
    ld hl,(var_sa)
    ld de,(var_sa+2)
    call shift_left_4

    ; SA*240 = SA*256 - SA*16
    push de
    push hl
    ld hl,(var_tmp3)
    pop de
    or a
    sbc hl,de
    ld (var_tmp2),hl
    ld hl,(var_tmp3+2)
    pop de
    sbc hl,de
    ld (var_tmp2+2),hl
    ret

; --- Helper: CA*240 -> var_tmp2 ---
compute_ca_240:
    ; CA*256
    ld a,(var_ca)
    ld (var_tmp3+1),a
    ld a,(var_ca+1)
    ld (var_tmp3+2),a
    ld a,(var_ca+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a

    ; CA*16
    ld hl,(var_ca)
    ld de,(var_ca+2)
    call shift_left_4

    ; CA*240 = CA*256 - CA*16
    push de
    push hl
    ld hl,(var_tmp3)
    pop de
    or a
    sbc hl,de
    ld (var_tmp2),hl
    ld hl,(var_tmp3+2)
    pop de
    sbc hl,de
    ld (var_tmp2+2),hl
    ret

; =============================================================================
; LOOKUP SINE: A = table index -> HL = signed 16-bit value
; =============================================================================
lookup_sine:
    ld l,a
    ld h,0
    add hl,hl              ; *2 for word access
    ld de,sine_table
    add hl,de
    ld e,(hl)
    inc hl
    ld d,(hl)
    ex de,hl               ; HL = value
    ret

; =============================================================================
; LOOKUP RECIP: A = table index -> HL = unsigned 16-bit value
; =============================================================================
lookup_recip:
    ld l,a
    ld h,0
    add hl,hl              ; *2 for word access
    ld de,recip_table
    add hl,de
    ld e,(hl)
    inc hl
    ld d,(hl)
    ex de,hl
    ret

; =============================================================================
; MUL16_SIGNED: DEHL = HL * DE (signed 16-bit -> signed 32-bit)
; =============================================================================
mul16_signed:
    ; Check sign of HL
    bit 7,h
    jr z,.hl_pos
    ; HL negative: negate, multiply unsigned, negate result
    push de
    xor a
    sub l
    ld l,a
    ld a,0
    sbc a,h
    ld h,a                 ; HL = |HL|
    pop de
    call mul16u
    ; Negate 32-bit DEHL
    ld a,l
    cpl
    ld l,a
    ld a,h
    cpl
    ld h,a
    ld a,e
    cpl
    ld e,a
    ld a,d
    cpl
    ld d,a
    ; Add 1 (two's complement)
    inc hl
    ld a,h
    or l
    ret nz
    inc de
    ret
.hl_pos:
    jp mul16u

; =============================================================================
; MUL16U: DEHL = HL * DE (unsigned 16x16 -> 32)
; Shift-and-add: shifts multiplicand (BC) out MSB-first, adds DE to accumulator
; =============================================================================
mul16u:
    push bc
    ld b,h
    ld c,l                 ; BC = multiplicand (shifts out)
    ; DE = multiplier (added when bit set)
    ; Accumulator = DEHL -> use (sp):HL for 32-bit, but DE is multiplier...
    ; Use IX for high word to avoid stack issues
    push ix
    ld ix,0                ; IX = high word of result
    ld hl,0                ; HL = low word of result

    ld a,16
.mul_loop:
    ; Shift accumulator left by 1
    add hl,hl              ; shift low word
    push hl
    push ix
    pop hl                 ; HL = IX (high word)
    adc hl,hl              ; shift high word with carry
    push hl
    pop ix                 ; IX = shifted high word
    pop hl                 ; HL = shifted low word

    ; Shift BC left, test top bit
    sla c
    rl b
    jr nc,.no_add

    ; Add DE to low word
    add hl,de
    jr nc,.no_add
    ; Carry to high word
    inc ix
.no_add:
    dec a
    jr nz,.mul_loop

    ; Result: IX = high word, HL = low word
    push ix
    pop de                 ; DE = high word

    pop ix                 ; restore IX
    pop bc                 ; restore BC
    ret

; =============================================================================
; MUL_A_L: HL = A * L (8x8 -> 16 unsigned)
; =============================================================================
mul_a_l:
    ld h,0
    ld d,0
    ld e,l                 ; DE = L
    ld l,h                 ; HL = 0
    ld b,8
.loop:
    add hl,hl
    rla
    jr nc,.no_add2
    add hl,de
.no_add2:
    djnz .loop
    ret

; =============================================================================
; SHIFT LEFT helpers: DEHL <<= N (32-bit)
; =============================================================================
shift_left_4:
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    ret

shift_left_6:
    call shift_left_4
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    ret

; =============================================================================
; WRITE 32-BIT VALUE: copy 4 bytes from (HL) to (DE) - MMIO write
; =============================================================================
write_mode7_32:
    ld a,(hl)
    ld (de),a
    inc hl
    inc de
    ld a,(hl)
    ld (de),a
    inc hl
    inc de
    ld a,(hl)
    ld (de),a
    inc hl
    inc de
    ld a,(hl)
    ld (de),a
    ret

; =============================================================================
; NEG32: negate 32-bit value at (HL) in-place
; =============================================================================
neg32:
    push bc
    ; Load 4 bytes, complement, add 1
    ld c,(hl)
    inc hl
    ld b,(hl)
    inc hl
    push hl
    ld a,(hl)
    inc hl
    ld h,(hl)
    ld l,a                 ; HL = high word, BC = low word

    ; Complement all
    ld a,c
    cpl
    ld c,a
    ld a,b
    cpl
    ld b,a
    ld a,l
    cpl
    ld l,a
    ld a,h
    cpl
    ld h,a

    ; Add 1
    inc bc
    ld a,b
    or c
    jr nz,.no_carry
    inc hl
.no_carry:
    ; Store back
    pop de                 ; DE = pointer to byte 2
    push de
    ; We need original pointer. It was HL at entry, now modified.
    ; DE points to byte 2
    dec de
    dec de                 ; DE = original pointer
    ex de,hl               ; HL = pointer, DE = high word
    ld (hl),c
    inc hl
    ld (hl),b
    inc hl
    ld (hl),e
    inc hl
    ld (hl),d
    pop de                 ; clean up extra push
    pop bc
    ret

; =============================================================================
; RENDER MODE7
; =============================================================================
render_mode7:
    SET_BLT_OP BLT_OP_MODE7
    SET_BLT_SRC TEXTURE_BASE
    SET_BLT_DST BACK_BUFFER
    SET_BLT_WIDTH RENDER_W
    SET_BLT_HEIGHT RENDER_H
    SET_SRC_STRIDE TEX_STRIDE
    SET_DST_STRIDE LINE_BYTES

    STORE32 BLT_MODE7_TEX_W_0, 255
    STORE32 BLT_MODE7_TEX_H_0, 255

    ; Write Mode7 parameters
    ld hl,var_u0
    ld de,BLT_MODE7_U0_0
    call write_mode7_32

    ld hl,var_v0
    ld de,BLT_MODE7_V0_0
    call write_mode7_32

    ld hl,var_ca
    ld de,BLT_MODE7_DU_COL_0
    call write_mode7_32         ; du_col = CA

    ld hl,var_sa
    ld de,BLT_MODE7_DV_COL_0
    call write_mode7_32         ; dv_col = SA

    ; du_row = -SA: copy and negate
    ld hl,(var_sa)
    ld (var_neg_sa),hl
    ld hl,(var_sa+2)
    ld (var_neg_sa+2),hl
    ld hl,var_neg_sa
    call neg32
    ld hl,var_neg_sa
    ld de,BLT_MODE7_DU_ROW_0
    call write_mode7_32

    ld hl,var_ca
    ld de,BLT_MODE7_DV_ROW_0
    call write_mode7_32         ; dv_row = CA

    START_BLIT
    WAIT_BLIT

    ret

; =============================================================================
; BLIT BACK BUFFER TO FRONT
; =============================================================================
blit_to_front:
    SET_BLT_OP BLT_OP_COPY
    SET_BLT_SRC BACK_BUFFER
    SET_BLT_DST VRAM_START
    SET_BLT_WIDTH RENDER_W
    SET_BLT_HEIGHT RENDER_H
    SET_SRC_STRIDE LINE_BYTES
    SET_DST_STRIDE LINE_BYTES
    START_BLIT
    WAIT_BLIT
    ret

; =============================================================================
; ADVANCE ANIMATION
; =============================================================================
advance_animation:
    ld hl,(angle_accum)
    ld de,ANGLE_INC
    add hl,de
    ld (angle_accum),hl

    ld hl,(scale_accum)
    ld de,SCALE_INC
    add hl,de
    ld (scale_accum),hl

    ret

; =============================================================================
; VARIABLES
; =============================================================================
angle_accum:    .word 0
scale_accum:    .word 0
var_angle_idx:  .byte 0
var_scale_idx:  .byte 0
var_cos:        .word 0
var_sin:        .word 0
var_recip:      .word 0
var_ca:         .byte 0,0,0,0
var_sa:         .byte 0,0,0,0
var_neg_sa:     .byte 0,0,0,0
var_u0:         .byte 0,0,0,0
var_v0:         .byte 0,0,0,0
var_tmp1:       .byte 0,0,0,0
var_tmp2:       .byte 0,0,0,0
var_tmp3:       .byte 0,0,0,0

; =============================================================================
; SINE TABLE - 256 entries, signed 16-bit LE
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
    .word 0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
    .word -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
    .word -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
    .word -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256
    .word -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
    .word -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
    .word -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
    .word -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

; =============================================================================
; RECIPROCAL TABLE - 256 entries, unsigned 16-bit LE
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
sid_data:
    .incbin "Circus_Attractions.sid"
sid_data_end:
