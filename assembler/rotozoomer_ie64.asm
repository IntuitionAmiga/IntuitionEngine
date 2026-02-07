; rotozoomer_ie64.asm - Optimized IE64 Rotozoomer
;
; Classic demoscene rotozoomer effect rendering a rotating and zooming
; checkerboard texture at 640x480. Ported from IE32 with IE64 optimizations.
;
; =============================================================================
; IE64 OPTIMIZATIONS OVER IE32
; =============================================================================
;
; 1. 3-OPERAND INSTRUCTIONS
;    - Eliminates 4 accumulator-copy instructions per pixel
;    - Inner loop: 13 instructions/pixel vs IE32's 17 (24% reduction)
;
; 2. NATIVE SIGNED MULTIPLY (MULS)
;    - Replaces 7-9 instruction sign-check+negate+MUL+negate patterns
;    - DU/DV table generation dramatically simplified
;
; 3. NATIVE ARITHMETIC SHIFT RIGHT (ASR)
;    - Replaces 5-7 instruction sign-check shift patterns
;    - Single instruction: asr.l r1, r1, #N
;
; 4. COMPARE-AND-BRANCH
;    - BNE/BEQ/BLT etc. compare two registers and branch in one instruction
;    - Replaces separate SUB+JNZ patterns (saves 1 instruction per test)
;
; 5. EXACT CENTER OFFSET (MULS replaces shift approximation)
;    - IE32: 320*x >> 8 approximated as x + (x >> 2) with sign handling (~10 instr)
;    - IE64: muls.l + asr.l (2 instructions, exact result)
;
; =============================================================================
; REGISTER ALLOCATION (Inner Loop)
; =============================================================================
;
;   Reg | Contents                  | IE32 equiv
;   ----+---------------------------+-----------
;    r0 | zero (hardwired)          | -
;   r1-r4| scratch                  | A, B, F
;   r16 | du_dx (U incr per pixel)  | C
;   r17 | dv_dx (V incr per pixel)  | D
;   r18 | TEXTURE_BASE address      | E
;   r19 | 255 (TEX_MASK)            | G
;   r20 | 4 (pixel stride)          | H
;   r21 | du_dy (U incr per row)    | S
;   r22 | current V coord           | T
;   r23 | current U coord           | U
;   r24 | dv_dy (V incr per row)    | V
;   r25 | LINE_BYTES (2560)         | W
;   r26 | VRAM write pointer        | X
;   r27 | row counter (0-479)       | Y
;   r28 | column counter            | Z
;   r29 | RENDER_H (480)            | - (IE64 extra)
;   r30 | 40 (col loop limit)       | - (IE64 extra)
;
; =============================================================================
; PERFORMANCE SUMMARY
; =============================================================================
;
; Per-pixel: 13 instructions (vs IE32's 17)
; Loop overhead: 0.125 instructions/pixel (2 per 16 pixels)
; Total per frame: ~4.04 million instructions (vs ~5.27M)
; Reduction: 23%
;
; =============================================================================
; USAGE
; =============================================================================
;
; Assemble: ./bin/ie64asm assembler/rotozoomer_ie64.asm
; Run:      ./bin/IntuitionEngine -ie64 -perf assembler/rotozoomer_ie64.ie64

include "ie64.inc"

; =============================================================================
; CONSTANTS
; =============================================================================

RENDER_W       equ 640
RENDER_H       equ 480
CENTER_X       equ 320
CENTER_Y       equ 240

DISPLAY_W      equ 640
DISPLAY_H      equ 480
FRAME_SIZE     equ 0x12C000

BACK_BUFFER    equ 0x22C000

TEX_SIZE       equ 256
TEX_MASK       equ 255
TEX_ROW_SHIFT  equ 10

FP_SHIFT       equ 8
FP_ONE         equ 256

; Memory layout
TEXTURE_BASE   equ 0x4000
SINE_TABLE     equ 0x44000
RECIP_TABLE    equ 0x44400
DU_TABLE       equ 0x44420
DV_TABLE       equ 0x45820
VAR_BASE       equ 0x46C20

; Variables
VAR_ANGLE      equ 0x46C20
VAR_SCALE_IDX  equ 0x46C24
VAR_SCALE_SEL  equ 0x46C28
VAR_DU_DX      equ 0x46C2C
VAR_DV_DX      equ 0x46C30
VAR_DU_DY      equ 0x46C34
VAR_DV_DY      equ 0x46C38
VAR_ROW_U      equ 0x46C3C
VAR_ROW_V      equ 0x46C40
VAR_START_U    equ 0x46C44
VAR_START_V    equ 0x46C48
VAR_VRAM_ROW   equ 0x46C54

; =============================================================================
; ENTRY POINT
; =============================================================================
org 0x1000

start:
    ; Enable video
    la      r1, VIDEO_CTRL
    li      r2, #1
    store.l r2, (r1)

    ; Init variables
    la      r1, VAR_ANGLE
    store.l r0, (r1)                    ; angle = 0
    la      r1, VAR_SCALE_IDX
    li      r2, #192                    ; start at sine minimum = most zoomed in
    store.l r2, (r1)

    jsr     generate_texture
    jsr     generate_sine_table
    jsr     generate_recip_table
    jsr     generate_dudv_tables

main_loop:
    jsr     update_animation
    jsr     render_rotozoomer
    jsr     wait_vsync
    jsr     blit_to_front
    bra     main_loop

; =============================================================================
; WAIT FOR VSYNC
; =============================================================================
wait_vsync:
    la      r1, VIDEO_STATUS
.wait:
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK
    beqz    r2, .wait
    rts

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard)
; =============================================================================
generate_texture:
    la      r5, TEXTURE_BASE            ; r5 = write pointer
    li      r6, #0                      ; r6 = row (Y)
    li      r7, #TEX_SIZE               ; r7 = 256 (limit)

.row:
    li      r8, #0                      ; r8 = col (X)

.col:
    eor.l   r1, r8, r6                  ; X ^ Y
    and.l   r1, r1, #128               ; & 128
    bnez    r1, .dark

    li      r2, #0xFFFFFFFF             ; white
    bra     .store

.dark:
    li      r2, #0xFF000000             ; black

.store:
    store.l r2, (r5)
    add.l   r5, r5, #4

    add.l   r8, r8, #1
    bne     r8, r7, .col               ; loop while col != 256

    add.l   r6, r6, #1
    bne     r6, r7, .row               ; loop while row != 256
    rts

; =============================================================================
; GENERATE SINE TABLE
; 256 entries, 8.8 fixed-point, piecewise linear approximation
; =============================================================================
generate_sine_table:
    la      r5, SINE_TABLE              ; r5 = write pointer
    li      r6, #0                      ; r6 = index
    li      r7, #256                    ; r7 = limit

.loop:
    ; t = index & 63
    and.l   r1, r6, #63

    ; If index & 64, t = 64 - t
    and.l   r2, r6, #64
    beqz    r2, .rising
    li      r3, #64
    sub.l   r1, r3, r1

.rising:
    ; value = t * 4 (shift left 2)
    lsl.l   r1, r1, #2

    ; Clamp to 256 max
    li      r3, #256
    bls     r1, r3, .clamp_ok
    li      r1, #256
.clamp_ok:

    ; If index & 128, negate
    and.l   r2, r6, #128
    beqz    r2, .positive
    neg.l   r1, r1

.positive:
    store.l r1, (r5)
    add.l   r5, r5, #4

    add.l   r6, r6, #1
    bne     r6, r7, .loop
    rts

; =============================================================================
; GENERATE RECIPROCAL TABLE
; recip_table[x] = 1536 / x for x = 0..7
; =============================================================================
generate_recip_table:
    la      r5, RECIP_TABLE

    li      r1, #0
    store.l r1, 0(r5)                   ; [0] = 0

    li      r1, #1536
    store.l r1, 4(r5)                   ; [1] = 1536

    li      r1, #768
    store.l r1, 8(r5)                   ; [2] = 768

    li      r1, #512
    store.l r1, 12(r5)                  ; [3] = 512

    li      r1, #384
    store.l r1, 16(r5)                  ; [4] = 384

    li      r1, #307
    store.l r1, 20(r5)                  ; [5] = 307

    li      r1, #256
    store.l r1, 24(r5)                  ; [6] = 256

    li      r1, #219
    store.l r1, 28(r5)                  ; [7] = 219

    rts

; =============================================================================
; GENERATE DU/DV TABLES
; DU_TABLE[scale * 256 + angle] = recip[scale+2] * cos[angle] >> 8
; DV_TABLE[scale * 256 + angle] = recip[scale+2] * sin[angle] >> 8
; scale = 0..4 (maps to divisor 2..6)
; =============================================================================
generate_dudv_tables:
    li      r16, #0                     ; r16 = scale index (0..4)

.scale_loop:
    ; Get recip value: recip_table[scale + 2]
    add.l   r1, r16, #2                ; divisor index
    lsl.l   r1, r1, #2                 ; * 4 bytes
    la      r2, RECIP_TABLE
    add.l   r1, r2, r1
    load.l  r17, (r1)                   ; r17 = scale_inv

    ; DU table base = DU_TABLE + scale * 1024
    lsl.l   r1, r16, #10
    la      r2, DU_TABLE
    add.l   r18, r2, r1                ; r18 = DU table ptr for this scale

    ; DV table base = DV_TABLE + scale * 1024
    la      r2, DV_TABLE
    add.l   r19, r2, r1                ; r19 = DV table ptr for this scale

    li      r20, #0                     ; r20 = angle index (0..255)
    li      r21, #256                   ; r21 = limit

.angle_loop:
    ; cos[angle] = sine[(angle + 64) & 255]
    add.l   r1, r20, #64
    and.l   r1, r1, #255
    lsl.l   r1, r1, #2
    la      r2, SINE_TABLE
    add.l   r1, r2, r1
    load.l  r8, (r1)                    ; r8 = cos[angle] (signed)

    ; du = (scale_inv * cos) >> 8  (signed multiply + arithmetic shift)
    muls.l  r1, r17, r8
    asr.l   r1, r1, #8
    store.l r1, (r18)                   ; store DU

    ; sin[angle]
    lsl.l   r1, r20, #2
    la      r2, SINE_TABLE
    add.l   r1, r2, r1
    load.l  r8, (r1)                    ; r8 = sin[angle] (signed)

    ; dv = (scale_inv * sin) >> 8
    muls.l  r1, r17, r8
    asr.l   r1, r1, #8
    store.l r1, (r19)                   ; store DV

    ; Advance pointers and angle
    add.l   r18, r18, #4
    add.l   r19, r19, #4
    add.l   r20, r20, #1
    bne     r20, r21, .angle_loop

    ; Next scale
    add.l   r16, r16, #1
    li      r1, #5
    bne     r16, r1, .scale_loop

    rts

; =============================================================================
; UPDATE ANIMATION
; =============================================================================
update_animation:
    ; Increment angle
    la      r5, VAR_ANGLE
    load.l  r1, (r5)
    add.l   r1, r1, #1
    and.l   r1, r1, #TEX_MASK
    store.l r1, (r5)
    move.l  r6, r1                      ; r6 = angle

    ; Increment scale index
    la      r5, VAR_SCALE_IDX
    load.l  r1, (r5)
    add.l   r1, r1, #2
    and.l   r1, r1, #TEX_MASK
    store.l r1, (r5)

    ; Calculate scale selector (0-4) from scale value
    ; scale = 205 + (sin[scale_idx] >> 1)
    lsl.l   r2, r1, #2
    la      r3, SINE_TABLE
    add.l   r2, r3, r2
    load.l  r1, (r2)                    ; r1 = sin[scale_idx]
    asr.l   r1, r1, #1                 ; signed shift right by 1
    add.l   r1, r1, #205               ; scale in 8.8

    ; Clamp to positive
    bltz    r1, .clamp_pos
    bra     .scale_ok
.clamp_pos:
    li      r1, #77
.scale_ok:
    ; Convert to scale selector: (scale >> 6) - 1, clamped to 0-4
    lsr.l   r1, r1, #6                 ; 1-5
    sub.l   r1, r1, #1                 ; 0-4

    ; Clamp low
    bltz    r1, .clamp_low
    ; Clamp high
    li      r2, #4
    bgt     r1, r2, .clamp_high
    bra     .scale_sel_done
.clamp_low:
    li      r1, #0
    bra     .scale_sel_done
.clamp_high:
    li      r1, #4
.scale_sel_done:
    la      r5, VAR_SCALE_SEL
    store.l r1, (r5)
    move.l  r7, r1                      ; r7 = scale_sel

    ; Look up du_dx from precomputed table
    ; DU_TABLE[scale_sel * 256 + angle]
    lsl.l   r1, r7, #10                ; * 1024
    lsl.l   r2, r6, #2                 ; angle * 4
    add.l   r1, r1, r2
    la      r3, DU_TABLE
    add.l   r1, r3, r1
    load.l  r8, (r1)                    ; r8 = du_dx
    la      r5, VAR_DU_DX
    store.l r8, (r5)

    ; Look up dv_dx
    lsl.l   r1, r7, #10
    lsl.l   r2, r6, #2
    add.l   r1, r1, r2
    la      r3, DV_TABLE
    add.l   r1, r3, r1
    load.l  r9, (r1)                    ; r9 = dv_dx
    la      r5, VAR_DV_DX
    store.l r9, (r5)

    ; du_dy = -dv_dx
    neg.l   r1, r9
    la      r5, VAR_DU_DY
    store.l r1, (r5)

    ; dv_dy = du_dx
    la      r5, VAR_DV_DY
    store.l r8, (r5)

    ; Set texture center
    la      r5, VAR_START_U
    li      r1, #0x8000
    store.l r1, (r5)
    la      r5, VAR_START_V
    store.l r1, (r5)

    rts

; =============================================================================
; RENDER ROTOZOOMER
; 16x loop unrolling, 13 instructions per pixel
; =============================================================================
render_rotozoomer:
    ; Load variables for center offset calculation
    la      r5, VAR_DU_DX
    load.l  r8, (r5)                    ; r8 = du_dx
    la      r5, VAR_DV_DX
    load.l  r9, (r5)                    ; r9 = dv_dx
    la      r5, VAR_DU_DY
    load.l  r10, (r5)                   ; r10 = du_dy
    la      r5, VAR_DV_DY
    load.l  r11, (r5)                   ; r11 = dv_dy

    ; Calculate starting row U,V using MULS (exact, 2 instr vs ~10)
    ; row_u = start_u - (320 * du_dx >> 8) - (240 * du_dy >> 8)
    ; row_v = start_v - (320 * dv_dx >> 8) - (240 * dv_dy >> 8)

    ; 320 * du_dx >> 8
    muls.l  r1, r8, #320
    asr.l   r1, r1, #8                 ; r1 = 320 * du_dx >> 8

    ; 240 * du_dy >> 8
    muls.l  r2, r10, #240
    asr.l   r2, r2, #8                 ; r2 = 240 * du_dy >> 8

    add.l   r1, r1, r2                 ; r1 = total U offset

    la      r5, VAR_START_U
    load.l  r3, (r5)
    sub.l   r3, r3, r1                 ; row_u = start_u - offset
    la      r5, VAR_ROW_U
    store.l r3, (r5)

    ; 320 * dv_dx >> 8
    muls.l  r1, r9, #320
    asr.l   r1, r1, #8

    ; 240 * dv_dy >> 8
    muls.l  r2, r11, #240
    asr.l   r2, r2, #8

    add.l   r1, r1, r2                 ; r1 = total V offset

    la      r5, VAR_START_V
    load.l  r3, (r5)
    sub.l   r3, r3, r1                 ; row_v = start_v - offset
    la      r5, VAR_ROW_V
    store.l r3, (r5)

    ; Init VRAM pointer
    la      r5, VAR_VRAM_ROW
    li      r1, #BACK_BUFFER
    store.l r1, (r5)

    ; Load constants into registers for inner loop
    la      r5, VAR_DU_DX
    load.l  r16, (r5)                   ; r16 = du_dx
    la      r5, VAR_DV_DX
    load.l  r17, (r5)                   ; r17 = dv_dx
    li      r18, #TEXTURE_BASE          ; r18 = texture base
    li      r19, #TEX_MASK              ; r19 = 255
    li      r20, #4                     ; r20 = pixel stride
    la      r5, VAR_DU_DY
    load.l  r21, (r5)                   ; r21 = du_dy
    la      r5, VAR_DV_DY
    load.l  r24, (r5)                   ; r24 = dv_dy
    li      r25, #LINE_BYTES            ; r25 = 2560
    li      r29, #RENDER_H              ; r29 = 480 (loop limit)
    li      r30, #40                    ; r30 = 40 (col iterations)

    li      r27, #0                     ; r27 = row counter

.row_loop:
    ; Load per-row state
    la      r5, VAR_VRAM_ROW
    load.l  r26, (r5)                   ; r26 = VRAM pointer
    la      r5, VAR_ROW_U
    load.l  r23, (r5)                   ; r23 = current U
    la      r5, VAR_ROW_V
    load.l  r22, (r5)                   ; r22 = current V

    ; 16x unrolled: 40 iterations for 640 pixels
    li      r28, #0                     ; r28 = col counter

.col_loop:
    ; ===== PIXEL 0 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 1 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 2 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 3 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 4 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 5 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 6 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 7 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 8 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 9 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 10 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 11 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 12 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 13 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 14 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 15 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; Loop: 40 iterations for 640 pixels
    add.l   r28, r28, #1
    bne     r28, r30, .col_loop

    ; Advance to next row
    la      r5, VAR_VRAM_ROW
    load.l  r1, (r5)
    add.l   r1, r1, r25                ; + LINE_BYTES
    store.l r1, (r5)

    la      r5, VAR_ROW_U
    load.l  r1, (r5)
    add.l   r1, r1, r21                ; + du_dy
    store.l r1, (r5)

    la      r5, VAR_ROW_V
    load.l  r1, (r5)
    add.l   r1, r1, r24                ; + dv_dy
    store.l r1, (r5)

    add.l   r27, r27, #1
    bne     r27, r29, .row_loop         ; loop while row != 480

    rts

; =============================================================================
; BLIT BACK BUFFER TO FRONT BUFFER
; =============================================================================
blit_to_front:
    la      r5, BLT_OP
    li      r1, #BLT_OP_COPY
    store.l r1, (r5)

    la      r5, BLT_SRC
    li      r1, #BACK_BUFFER
    store.l r1, (r5)

    la      r5, BLT_DST
    li      r1, #VRAM_START
    store.l r1, (r5)

    la      r5, BLT_WIDTH
    li      r1, #DISPLAY_W
    store.l r1, (r5)

    la      r5, BLT_HEIGHT
    li      r1, #DISPLAY_H
    store.l r1, (r5)

    la      r5, BLT_SRC_STRIDE
    li      r1, #LINE_BYTES
    store.l r1, (r5)

    la      r5, BLT_DST_STRIDE
    store.l r1, (r5)

    la      r5, BLT_CTRL
    li      r1, #1
    store.l r1, (r5)

    ; Wait for blit to finish
    la      r5, BLT_STATUS
.wait_blit:
    load.l  r1, (r5)
    and.l   r1, r1, #2
    bnez    r1, .wait_blit

    rts

; =============================================================================
; EOF
; =============================================================================
