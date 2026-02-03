; rotozoomer_68k.asm - Maximum Optimized Rotozoomer
;
; M68020 port of the classic demoscene rotozoomer effect.
; Renders a rotating and zooming checkerboard texture at 640x480.
;
; =============================================================================
; OPTIMIZATIONS APPLIED
; =============================================================================
;
; 1. INCREMENTAL TEXTURE COORDINATE CALCULATION
;    - Instead of per-pixel: texX = dx*cosA - dy*sinA (4 multiplies)
;    - We use: texU += du_dx, texV += dv_dx (2 additions)
;    - Saves ~1.2 million multiplications per frame
;
; 2. DOUBLE BUFFERING WITH HARDWARE BLITTER
;    - Render to back buffer (0x22C000) while front is displayed
;    - Hardware blitter copies back->front during vblank
;    - Eliminates tearing artifacts
;
; 3. REGISTER ALLOCATION (Inner Loop)
;    - Hot loop constants cached in registers (no memory reads)
;    - d1 = du_dx, d2 = dv_dx, d3 = scratch, d4 = texV
;    - d5 = texU, d6 = TEX_MASK (255), d7 = loop counter
;    - a0 = TEXTURE_BASE, a1 = VRAM ptr, a2 = scratch
;
; 4. 16x LOOP UNROLLING
;    - Process 16 pixels per iteration (40 iterations for 640 pixels)
;    - Reduces loop overhead significantly
;
; 5. PRECOMPUTED DU/DV TABLES
;    - du_table[scale][angle] = scale_inv[scale] * cos[angle]
;    - dv_table[scale][angle] = scale_inv[scale] * sin[angle]
;    - 5 scale values x 256 angles x 2 tables = 2560 entries (10KB)
;    - Eliminates ALL MUL operations in render path
;
; =============================================================================
; USAGE
; =============================================================================
;
; Assemble: vasmm68k_mot -Fbin -m68020 -devpac -o assembler/rotozoomer_68k.ie68 assembler/rotozoomer_68k.asm
; Run:      ./bin/IntuitionEngine -68k assembler/rotozoomer_68k.ie68
;

                include "ie68.inc"

; =============================================================================
; CONSTANTS
; =============================================================================

RENDER_W        equ     640
RENDER_H        equ     480
CENTER_X        equ     320
CENTER_Y        equ     240

DISPLAY_W       equ     640
DISPLAY_H       equ     480
FRAME_SIZE      equ     $12C000

BACK_BUFFER     equ     $22C000

TEX_SIZE        equ     256
TEX_MASK        equ     255
TEX_ROW_SHIFT   equ     10

FP_SHIFT        equ     8
FP_ONE          equ     256

; Memory layout
TEXTURE_BASE    equ     $4000
SINE_TABLE      equ     $44000
RECIP_TABLE     equ     $44400
DU_TABLE        equ     $44420
DV_TABLE        equ     $45820
VAR_BASE        equ     $46C20

; =============================================================================
; ENTRY POINT
; =============================================================================
                org     PROGRAM_START

start:
                move.l  #STACK_TOP,sp           ; Initialize stack

                ; Enable video
                move.l  #1,VIDEO_CTRL

                ; Initialize variables
                clr.l   var_angle
                move.l  #192,var_scale_idx      ; Start at sine minimum = most zoomed in

                bsr     generate_texture
                bsr     generate_sine_table
                bsr     generate_recip_table
                bsr     generate_dudv_tables

main_loop:
                bsr     update_animation
                bsr     render_rotozoomer
                bsr     wait_vsync
                bsr     blit_to_front
                bra     main_loop

; =============================================================================
; WAIT FOR VSYNC
; Proper vsync: wait for vblank to END first (if active), then wait for START
; This ensures exactly one frame per vsync cycle
; =============================================================================
wait_vsync:
                ; First, wait for vblank to END (if we're already in vblank)
.wait_end:      move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                bne.s   .wait_end
                ; Now wait for vblank to START
.wait_start:    move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                beq.s   .wait_start
                rts

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard)
; =============================================================================
generate_texture:
                movem.l d0-d3/a0,-(sp)
                lea     TEXTURE_BASE,a0
                moveq   #0,d1                   ; y counter

.row:           moveq   #0,d2                   ; x counter

.col:           move.l  d2,d0
                eor.l   d1,d0
                andi.l  #128,d0
                bne.s   .dark

                move.l  #$FFFFFFFF,d3
                bra.s   .store

.dark:          move.l  #$FF000000,d3

.store:         move.l  d3,(a0)+

                addq.l  #1,d2
                cmpi.l  #TEX_SIZE,d2
                bne.s   .col

                addq.l  #1,d1
                cmpi.l  #TEX_SIZE,d1
                bne.s   .row

                movem.l (sp)+,d0-d3/a0
                rts

; =============================================================================
; GENERATE SINE TABLE
; 256 entries, 8.8 fixed-point: -256 to +256 (-1.0 to +1.0)
; =============================================================================
generate_sine_table:
                movem.l d0-d4/a0,-(sp)
                lea     SINE_TABLE,a0
                moveq   #0,d1                   ; index 0-255

.loop:          move.l  d1,d0
                andi.l  #63,d0                  ; d0 = index & 63

                move.l  d1,d2
                andi.l  #64,d2
                beq.s   .rising

                ; Falling edge: 64 - (index & 63)
                moveq   #64,d3
                sub.l   d0,d3
                move.l  d3,d0

.rising:        ; d0 = ramp value 0-64
                lsl.l   #2,d0                   ; d0 * 4, range 0-256

                ; Clamp to 256
                cmpi.l  #256,d0
                ble.s   .clamp_ok
                move.l  #256,d0
.clamp_ok:
                ; Check if negative half (index & 128)
                move.l  d1,d2
                andi.l  #128,d2
                beq.s   .positive

                neg.l   d0                      ; Negate for second half

.positive:
                move.l  d0,(a0)+

                addq.l  #1,d1
                cmpi.l  #256,d1
                bne.s   .loop

                movem.l (sp)+,d0-d4/a0
                rts

; =============================================================================
; GENERATE RECIPROCAL TABLE
; recip_table[x] = 1536 / x for x = 0..7
; =============================================================================
generate_recip_table:
                movem.l a0,-(sp)
                lea     RECIP_TABLE,a0

                move.l  #0,(a0)+                ; 1536/0 = 0 (undefined)
                move.l  #1536,(a0)+             ; 1536/1 = 1536
                move.l  #768,(a0)+              ; 1536/2 = 768
                move.l  #512,(a0)+              ; 1536/3 = 512
                move.l  #384,(a0)+              ; 1536/4 = 384
                move.l  #307,(a0)+              ; 1536/5 = 307
                move.l  #256,(a0)+              ; 1536/6 = 256
                move.l  #219,(a0)               ; 1536/7 = 219

                movem.l (sp)+,a0
                rts

; =============================================================================
; GENERATE DU/DV TABLES
; Precompute scale_inv * cos/sin for all scale and angle combinations
; DU_TABLE[scale * 256 + angle] = recip[scale+2] * cos[angle] >> 8
; DV_TABLE[scale * 256 + angle] = recip[scale+2] * sin[angle] >> 8
; scale = 0..4 (maps to divisor 2..6)
; =============================================================================
generate_dudv_tables:
                movem.l d0-d7/a0-a4,-(sp)

                ; a3 = sine table base
                lea     SINE_TABLE,a3

                ; For each scale (0-4)
                moveq   #0,d7                   ; d7 = scale index

.scale_loop:
                ; Get reciprocal value for this scale
                ; recip_table[scale + 2]
                move.l  d7,d0
                addq.l  #2,d0                   ; divisor index
                lsl.l   #2,d0                   ; *4 for long access
                lea     RECIP_TABLE,a0
                move.l  (a0,d0.l),d6            ; d6 = scale_inv for this scale

                ; Calculate table base for this scale
                ; DU base = DU_TABLE + scale * 1024 (256 entries * 4 bytes)
                move.l  d7,d0
                lsl.l   #8,d0
                lsl.l   #2,d0                   ; *1024 (split: 8+2=10)
                lea     DU_TABLE,a1
                adda.l  d0,a1                   ; a1 = DU table ptr for this scale

                lea     DV_TABLE,a2
                adda.l  d0,a2                   ; a2 = DV table ptr for this scale

                ; For each angle (0-255)
                moveq   #0,d5                   ; d5 = angle

.angle_loop:
                ; Get cos[angle] = sin[angle + 64]
                move.l  d5,d0
                addi.l  #64,d0
                andi.l  #255,d0
                lsl.l   #2,d0                   ; *4 for long
                move.l  (a3,d0.l),d0            ; d0 = cos[angle]

                ; Multiply by scale_inv (signed)
                ; Result = (scale_inv * cosA) >> 8
                move.l  d0,d1
                bpl.s   .cos_pos

                ; Negative: negate, multiply, negate
                neg.l   d1
                mulu.w  d6,d1
                lsr.l   #8,d1
                neg.l   d1
                bra.s   .store_du

.cos_pos:       mulu.w  d6,d1
                lsr.l   #8,d1

.store_du:      move.l  d1,(a1)+

                ; Get sin[angle]
                move.l  d5,d0
                lsl.l   #2,d0
                move.l  (a3,d0.l),d0            ; d0 = sin[angle]

                ; Multiply by scale_inv (signed)
                move.l  d0,d1
                bpl.s   .sin_pos

                neg.l   d1
                mulu.w  d6,d1
                lsr.l   #8,d1
                neg.l   d1
                bra.s   .store_dv

.sin_pos:       mulu.w  d6,d1
                lsr.l   #8,d1

.store_dv:      move.l  d1,(a2)+

                addq.l  #1,d5
                cmpi.l  #256,d5
                bne.s   .angle_loop

                ; Next scale
                addq.l  #1,d7
                cmpi.l  #5,d7
                bne     .scale_loop

                movem.l (sp)+,d0-d7/a0-a4
                rts

; =============================================================================
; UPDATE ANIMATION
; =============================================================================
update_animation:
                movem.l d0-d4/a0-a1,-(sp)

                ; Increment angle
                move.l  var_angle,d0
                addq.l  #1,d0
                andi.l  #TEX_MASK,d0
                move.l  d0,var_angle

                ; Increment scale index
                move.l  var_scale_idx,d0
                addq.l  #2,d0
                andi.l  #TEX_MASK,d0
                move.l  d0,var_scale_idx

                ; Calculate scale selector (0-4) from scale value
                ; scale = 205 + (sin[scale_idx] >> 1), range ~77-333
                ; scale >> 6 gives 1-5, so (scale >> 6) - 1 gives 0-4
                lsl.l   #2,d0                   ; *4 for long access
                lea     SINE_TABLE,a0
                move.l  (a0,d0.l),d0            ; sin value

                ; Signed shift right by 1
                asr.l   #1,d0
                addi.l  #205,d0                 ; scale in 8.8

                ; Clamp to positive
                tst.l   d0
                bpl.s   .scale_ok
                moveq   #77,d0
.scale_ok:
                ; Convert to scale selector: (scale >> 6) - 1, clamped to 0-4
                lsr.l   #6,d0                   ; 1-5
                subq.l  #1,d0                   ; 0-4
                bpl.s   .check_high
                moveq   #0,d0
                bra.s   .scale_sel_done
.check_high:    cmpi.l  #4,d0
                ble.s   .scale_sel_done
                moveq   #4,d0
.scale_sel_done:
                move.l  d0,var_scale_sel

                ; Look up du_dx from precomputed table
                ; DU_TABLE[scale_sel * 256 + angle]
                lsl.l   #8,d0
                lsl.l   #2,d0                   ; * 1024 (256 entries * 4 bytes, split: 8+2=10)
                move.l  var_angle,d1
                lsl.l   #2,d1                   ; * 4
                add.l   d1,d0
                lea     DU_TABLE,a0
                move.l  (a0,d0.l),d2
                move.l  d2,var_du_dx

                ; Look up dv_dx from precomputed table
                move.l  var_scale_sel,d0
                lsl.l   #8,d0
                lsl.l   #2,d0                   ; *1024 (split: 8+2=10)
                add.l   d1,d0
                lea     DV_TABLE,a0
                move.l  (a0,d0.l),d3
                move.l  d3,var_dv_dx

                ; du_dy = -dv_dx
                neg.l   d3
                move.l  d3,var_du_dy

                ; dv_dy = du_dx
                move.l  d2,var_dv_dy

                ; Set texture center
                move.l  #$8000,var_start_u
                move.l  #$8000,var_start_v

                movem.l (sp)+,d0-d4/a0-a1
                rts

; =============================================================================
; RENDER ROTOZOOMER
; 16x loop unrolling, all arithmetic inlined
; =============================================================================
render_rotozoomer:
                movem.l d0-d7/a0-a6,-(sp)

                ; Calculate starting row U,V using shifts (no MUL)
                ; 320 * x >> 8 = x + (x >> 2)   [320/256 = 1.25]
                ; 240 * x >> 8 = x - (x >> 4)   [240/256 = 0.9375]

                ; === 320 * du_dx >> 8 ===
                move.l  var_du_dx,d0
                bpl.s   .pos_320_dudx

                neg.l   d0
                move.l  d0,d1
                lsr.l   #2,d1
                add.l   d1,d0
                neg.l   d0
                bra.s   .done_320_dudx

.pos_320_dudx:  move.l  d0,d1
                lsr.l   #2,d1
                add.l   d1,d0

.done_320_dudx: move.l  d0,-(sp)                ; Save temp

                ; === 240 * du_dy >> 8 ===
                move.l  var_du_dy,d0
                bpl.s   .pos_240_dudy

                neg.l   d0
                move.l  d0,d1
                lsr.l   #4,d1
                sub.l   d1,d0
                neg.l   d0
                bra.s   .done_240_dudy

.pos_240_dudy:  move.l  d0,d1
                lsr.l   #4,d1
                sub.l   d1,d0

.done_240_dudy: add.l   (sp)+,d0                ; Add 320*du_dx

                move.l  var_start_u,d1
                sub.l   d0,d1
                move.l  d1,var_row_u

                ; === 320 * dv_dx >> 8 ===
                move.l  var_dv_dx,d0
                bpl.s   .pos_320_dvdx

                neg.l   d0
                move.l  d0,d1
                lsr.l   #2,d1
                add.l   d1,d0
                neg.l   d0
                bra.s   .done_320_dvdx

.pos_320_dvdx:  move.l  d0,d1
                lsr.l   #2,d1
                add.l   d1,d0

.done_320_dvdx: move.l  d0,-(sp)                ; Save temp

                ; === 240 * dv_dy >> 8 ===
                move.l  var_dv_dy,d0
                bpl.s   .pos_240_dvdy

                neg.l   d0
                move.l  d0,d1
                lsr.l   #4,d1
                sub.l   d1,d0
                neg.l   d0
                bra.s   .done_240_dvdy

.pos_240_dvdy:  move.l  d0,d1
                lsr.l   #4,d1
                sub.l   d1,d0

.done_240_dvdy: add.l   (sp)+,d0                ; Add 320*dv_dx

                move.l  var_start_v,d1
                sub.l   d0,d1
                move.l  d1,var_row_v

                ; Initialize VRAM pointer
                move.l  #BACK_BUFFER,var_vram_row

                ; Load constants into registers for inner loop
                ; d1 = du_dx, d2 = dv_dx, d6 = TEX_MASK (255)
                ; a0 = TEXTURE_BASE
                move.l  var_du_dx,d1
                move.l  var_dv_dx,d2
                moveq   #0,d6
                move.b  #TEX_MASK,d6            ; d6 = 255
                lea     TEXTURE_BASE,a0

                ; a3 = du_dy, a4 = dv_dy for row updates
                move.l  var_du_dy,a3
                move.l  var_dv_dy,a4

                ; Row counter
                move.l  #RENDER_H-1,d7

.row_loop:
                move.l  var_vram_row,a1         ; VRAM pointer
                move.l  var_row_u,d5            ; texU
                move.l  var_row_v,d4            ; texV

                ; Column loop: 40 iterations x 16 pixels = 640 pixels
                ; Use a5 for col counter since d0 is used as scratch in pixel calcs
                move.l  #40-1,a5

.col_loop:
                ; ===== PIXEL 0 =====
                move.l  d4,d3                   ; texV
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3                   ; row * 1024
                move.l  d5,d0                   ; texU
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0                   ; col * 4
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5                   ; texU += du_dx
                add.l   d2,d4                   ; texV += dv_dx

                ; ===== PIXEL 1 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 2 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 3 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 4 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 5 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 6 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 7 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 8 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 9 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 10 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 11 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 12 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 13 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 14 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; ===== PIXEL 15 =====
                move.l  d4,d3
                lsr.l   #8,d3
                and.l   d6,d3
                lsl.l   #8,d3
                lsl.l   #2,d3
                move.l  d5,d0
                lsr.l   #8,d0
                and.l   d6,d0
                lsl.l   #2,d0
                add.l   d3,d0
                move.l  (a0,d0.l),(a1)+
                add.l   d1,d5
                add.l   d2,d4

                ; Decrement col counter (in a5) and loop
                subq.l  #1,a5
                move.l  a5,d0
                bpl     .col_loop

                ; Advance to next row
                move.l  var_vram_row,d0
                addi.l  #LINE_BYTES,d0
                move.l  d0,var_vram_row

                move.l  var_row_u,d0
                add.l   a3,d0                   ; row_u += du_dy
                move.l  d0,var_row_u

                move.l  var_row_v,d0
                add.l   a4,d0                   ; row_v += dv_dy
                move.l  d0,var_row_v

                dbf     d7,.row_loop

                movem.l (sp)+,d0-d7/a0-a6
                rts

; =============================================================================
; BLIT BACK BUFFER TO FRONT BUFFER
; =============================================================================
blit_to_front:
                move.l  #BLT_OP_COPY,BLT_OP
                move.l  #BACK_BUFFER,BLT_SRC
                move.l  #VRAM_START,BLT_DST
                move.l  #DISPLAY_W,BLT_WIDTH
                move.l  #DISPLAY_H,BLT_HEIGHT
                move.l  #LINE_BYTES,BLT_SRC_STRIDE
                move.l  #LINE_BYTES,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL

.wait_blit:     move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .wait_blit

                rts

; =============================================================================
; VARIABLE ADDRESSES (using fixed RAM locations like IE32)
; Located after the precomputed tables at VAR_BASE (0x46C20)
; =============================================================================
var_angle       equ     VAR_BASE+$00
var_scale_idx   equ     VAR_BASE+$04
var_scale_sel   equ     VAR_BASE+$08
var_du_dx       equ     VAR_BASE+$0C
var_dv_dx       equ     VAR_BASE+$10
var_du_dy       equ     VAR_BASE+$14
var_dv_dy       equ     VAR_BASE+$18
var_row_u       equ     VAR_BASE+$1C
var_row_v       equ     VAR_BASE+$20
var_start_u     equ     VAR_BASE+$24
var_start_v     equ     VAR_BASE+$28
var_vram_row    equ     VAR_BASE+$2C

; =============================================================================
; EOF
; =============================================================================
