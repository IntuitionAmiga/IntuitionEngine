; ============================================================================
; 3DFX BOOT LOGO - shaded Voodoo vector logo
; M68K (68020) for IntuitionEngine - Voodoo 3D
; ============================================================================
;
; The previous version drew the logo as many hand-written, flat-colour bars.
; This version keeps the same Voodoo triangle-pipeline target, but uses compact
; data-driven vector geometry:
;   - 32-spike rotating starburst with Gouraud shading
;   - translucent centre glow
;   - vector "3Dfx" logo with drop shadow, bevel shading, and highlights
;   - VSync swap, scissor clipping, and ordered dithering
;
; The hot path now shares one scaled-quad renderer instead of repeating the
; same coordinate scaling code for every triangle.
;
; Build:
;   vasmm68k_mot -Fbin -m68020 -devpac -I ../../include -o voodoo_3dfx_logo_68k.ie68 voodoo_3dfx_logo_68k.asm
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

SCREEN_W        set 640
SCREEN_H        set 480

CENTER_X        equ SCREEN_W/2
CENTER_Y        equ SCREEN_H/2
FP_SHIFT        equ 4

NUM_SPIKES      equ 32
SPIKE_INNER     equ 46
SPIKE_MID       equ 96
SPIKE_OUTER_A   equ 242
SPIKE_OUTER_B   equ 204

ZOOM_START      equ 28
ZOOM_END        equ 286
ZOOM_SPEED      equ 4

LOGO_MAIN_COUNT equ 14
LOGO_HI_COUNT   equ 7

FBZ_MAIN        equ VOODOO_FBZ_CLIPPING|VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_WRITE|VOODOO_FBZ_DITHER|(VOODOO_DEPTH_LESS<<5)
ALPHA_BLEND     equ VOODOO_ALPHA_BLEND_EN|(VOODOO_BLEND_SRC_ALPHA<<8)|(VOODOO_BLEND_INV_SRC_A<<12)

    org     $1000

start:
    lea     STACK_TOP,sp

    move.l  #1,VOODOO_ENABLE
    move.l  #(SCREEN_W<<16)|SCREEN_H,VOODOO_VIDEO_DIM
    move.l  #FBZ_MAIN,VOODOO_FBZ_MODE
    move.l  #(0<<16)|SCREEN_W,VOODOO_CLIP_LEFT_RIGHT
    move.l  #(0<<16)|SCREEN_H,VOODOO_CLIP_LOW_Y_HIGH
    move.l  #VOODOO_COMBINE_ITERATED,VOODOO_FBZCOLOR_PATH
    clr.l   VOODOO_ALPHA_MODE
    clr.l   VOODOO_FOG_MODE

    move.w  #ZOOM_START,zoom_scale
    clr.w   rotation_angle

main_loop:
    move.l  #$FF02030C,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    bsr     draw_starburst
    bsr     draw_logo

    move.l  #VOODOO_SWAP_VSYNC,VOODOO_SWAP_BUFFER_CMD
    bsr     advance_animation
    bra     main_loop

advance_animation:
    move.w  zoom_scale,d0
    cmp.w   #ZOOM_END,d0
    bge.s   .zoom_done
    add.w   #ZOOM_SPEED,d0
    cmp.w   #ZOOM_END,d0
    ble.s   .store_zoom
    move.w  #ZOOM_END,d0
.store_zoom:
    move.w  d0,zoom_scale
.zoom_done:

    cmp.w   #ZOOM_END-30,d0
    blt.s   .fast
    addq.w  #1,rotation_angle
    rts
.fast:
    addq.w  #5,rotation_angle
    rts

; ============================================================================
; Starburst and glow
; ============================================================================

draw_starburst:
    movem.l d0-d7/a0-a2,-(sp)

    bsr     draw_centre_glow

    move.l  #$0300,cur_r
    move.l  #$0600,cur_g
    move.l  #$1000,cur_b
    move.l  #$1000,cur_a
    move.l  #$E800,cur_z
    move.l  #$0400,cur_hi
    move.l  #$0300,cur_lo

    moveq   #NUM_SPIKES-1,d7
.outer_loop:
    move.w  d7,d0
    lsl.w   #3,d0
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d6

    move.w  zoom_scale,d1
    mulu    #SPIKE_MID,d1
    lsr.l   #8,d1
    bsr     polar_screen
    move.l  d0,p0x
    move.l  d1,p0y

    move.w  d6,d0
    subq.w  #4,d0
    and.w   #255,d0
    btst    #0,d7
    beq.s   .outer_a
    move.w  #SPIKE_OUTER_B,d1
    bra.s   .have_outer
.outer_a:
    move.w  #SPIKE_OUTER_A,d1
.have_outer:
    move.w  zoom_scale,d2
    mulu    d1,d2
    lsr.l   #8,d2
    move.w  d2,d1
    bsr     polar_screen
    move.l  d0,p1x
    move.l  d1,p1y

    move.w  d6,d0
    addq.w  #4,d0
    and.w   #255,d0
    move.w  d2,d1
    bsr     polar_screen
    move.l  d0,p2x
    move.l  d1,p2y

    move.l  p0x,d0
    move.l  p0y,d1
    move.l  p1x,d2
    move.l  p1y,d3
    move.l  p2x,d4
    move.l  p2y,d5
    bsr     submit_triangle_gouraud

    dbf     d7,.outer_loop

    move.l  #$1000,cur_r
    move.l  #$0D00,cur_g
    move.l  #$0200,cur_b
    move.l  #$1000,cur_a
    move.l  #$D800,cur_z
    move.l  #$0200,cur_hi
    move.l  #$0400,cur_lo

    moveq   #NUM_SPIKES-1,d7
.inner_loop:
    move.w  d7,d0
    lsl.w   #3,d0
    add.w   rotation_angle,d0
    addq.w  #2,d0
    and.w   #255,d0
    move.w  d0,d6

    move.w  zoom_scale,d1
    mulu    #SPIKE_INNER,d1
    lsr.l   #8,d1
    bsr     polar_screen
    move.l  d0,p0x
    move.l  d1,p0y

    move.w  d6,d0
    subq.w  #4,d0
    and.w   #255,d0
    move.w  zoom_scale,d1
    mulu    #SPIKE_MID,d1
    lsr.l   #8,d1
    bsr     polar_screen
    move.l  d0,p1x
    move.l  d1,p1y

    move.w  d6,d0
    addq.w  #4,d0
    and.w   #255,d0
    move.w  zoom_scale,d1
    mulu    #SPIKE_MID,d1
    lsr.l   #8,d1
    bsr     polar_screen
    move.l  d0,p2x
    move.l  d1,p2y

    move.l  p0x,d0
    move.l  p0y,d1
    move.l  p1x,d2
    move.l  p1y,d3
    move.l  p2x,d4
    move.l  p2y,d5
    bsr     submit_triangle_gouraud

    dbf     d7,.inner_loop

    movem.l (sp)+,d0-d7/a0-a2
    rts

draw_centre_glow:
    move.l  #ALPHA_BLEND,VOODOO_ALPHA_MODE
    move.l  #$1000,cur_r
    move.l  #$0F00,cur_g
    move.l  #$0600,cur_b
    move.l  #$0800,cur_a
    move.l  #$F400,cur_z
    move.l  #$0000,cur_hi
    move.l  #$0800,cur_lo

    moveq   #NUM_SPIKES-1,d7
.glow_loop:
    move.l  #(CENTER_X<<FP_SHIFT),p0x
    move.l  #(CENTER_Y<<FP_SHIFT),p0y

    move.w  d7,d0
    lsl.w   #3,d0
    and.w   #255,d0
    move.w  zoom_scale,d1
    mulu    #150,d1
    lsr.l   #8,d1
    bsr     polar_screen
    move.l  d0,p1x
    move.l  d1,p1y

    move.w  d7,d0
    lsl.w   #3,d0
    addq.w  #8,d0
    and.w   #255,d0
    move.w  zoom_scale,d1
    mulu    #150,d1
    lsr.l   #8,d1
    bsr     polar_screen
    move.l  d0,p2x
    move.l  d1,p2y

    move.l  p0x,d0
    move.l  p0y,d1
    move.l  p1x,d2
    move.l  p1y,d3
    move.l  p2x,d4
    move.l  p2y,d5
    bsr     submit_triangle_gouraud

    dbf     d7,.glow_loop

    clr.l   VOODOO_ALPHA_MODE
    rts

polar_screen:
    ; Input: D0 = angle, D1 = radius.  Output: D0/D1 = screen 12.4 coords.
    bsr     polar_to_cart
    add.l   #CENTER_X,d0
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    rts

polar_to_cart:
    lea     sin_table,a1
    and.w   #255,d0
    move.w  d0,d2
    add.w   #64,d2
    and.w   #255,d2

    move.b  0(a1,d2.w),d2
    move.b  0(a1,d0.w),d3
    ext.w   d2
    ext.w   d3
    ext.l   d2
    ext.l   d3
    muls    d1,d2
    muls    d1,d3
    asr.l   #7,d2
    asr.l   #7,d3
    move.l  d2,d0
    move.l  d3,d1
    rts

; ============================================================================
; Logo renderer
; ============================================================================

draw_logo:
    movem.l d0-d7/a0-a3,-(sp)

    move.w  zoom_scale,d0
    cmp.w   #80,d0
    blt     .done

    move.l  #ALPHA_BLEND,VOODOO_ALPHA_MODE
    move.w  #14,draw_xoff
    move.w  #16,draw_yoff
    move.w  d0,draw_scale
    move.l  #$0000,cur_r
    move.l  #$0000,cur_g
    move.l  #$0000,cur_b
    move.l  #$0900,cur_a
    move.l  #$7000,cur_z
    move.l  #$0000,cur_hi
    move.l  #$0000,cur_lo
    lea     logo_main_quads,a0
    move.w  #LOGO_MAIN_COUNT,quad_count
    bsr     draw_shadow_table

    clr.l   VOODOO_ALPHA_MODE
    clr.w   draw_xoff
    clr.w   draw_yoff
    move.w  zoom_scale,draw_scale
    lea     logo_main_quads,a0
    move.w  #LOGO_MAIN_COUNT,quad_count
    bsr     draw_quad_table

    lea     logo_highlight_quads,a0
    move.w  #LOGO_HI_COUNT,quad_count
    bsr     draw_quad_table

.done:
    movem.l (sp)+,d0-d7/a0-a3
    rts

draw_shadow_table:
    move.l  a0,a2
.loop:
    tst.w   quad_count
    beq.s   .done
    bsr     load_quad_points
    adda.w  #10,a2
    bsr     draw_loaded_quad
    subq.w  #1,quad_count
    bra.s   .loop
.done:
    rts

draw_quad_table:
    move.l  a0,a2
.loop:
    tst.w   quad_count
    beq.s   .done
    bsr     load_quad_points
    bsr     load_quad_colour
    bsr     draw_loaded_quad
    subq.w  #1,quad_count
    bra.s   .loop
.done:
    rts

load_quad_points:
    move.w  (a2)+,d0
    move.w  (a2)+,d1
    bsr     transform_logo_point
    move.l  d0,p0x
    move.l  d1,p0y

    move.w  (a2)+,d0
    move.w  (a2)+,d1
    bsr     transform_logo_point
    move.l  d0,p1x
    move.l  d1,p1y

    move.w  (a2)+,d0
    move.w  (a2)+,d1
    bsr     transform_logo_point
    move.l  d0,p2x
    move.l  d1,p2y

    move.w  (a2)+,d0
    move.w  (a2)+,d1
    bsr     transform_logo_point
    move.l  d0,p3x
    move.l  d1,p3y
    rts

load_quad_colour:
    move.w  (a2)+,d0
    and.l   #$FFFF,d0
    move.l  d0,cur_r
    move.w  (a2)+,d0
    and.l   #$FFFF,d0
    move.l  d0,cur_g
    move.w  (a2)+,d0
    and.l   #$FFFF,d0
    move.l  d0,cur_b
    move.w  (a2)+,d0
    and.l   #$FFFF,d0
    move.l  d0,cur_a
    move.w  (a2)+,d0
    and.l   #$FFFF,d0
    move.l  d0,cur_z
    move.l  #$0180,cur_hi
    move.l  #$0200,cur_lo
    rts

transform_logo_point:
    ext.l   d0
    ext.l   d1
    move.w  draw_scale,d2
    muls    d2,d0
    muls    d2,d1
    asr.l   #8,d0
    asr.l   #8,d1
    move.w  draw_xoff,d2
    ext.l   d2
    add.l   d2,d0
    move.w  draw_yoff,d2
    ext.l   d2
    add.l   d2,d1
    add.l   #CENTER_X,d0
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    rts

draw_loaded_quad:
    move.l  p0x,d0
    move.l  p0y,d1
    move.l  p1x,d2
    move.l  p1y,d3
    move.l  p2x,d4
    move.l  p2y,d5
    bsr     submit_triangle_gouraud

    move.l  p0x,d0
    move.l  p0y,d1
    move.l  p2x,d2
    move.l  p2y,d3
    move.l  p3x,d4
    move.l  p3y,d5
    bsr     submit_triangle_gouraud
    rts

submit_triangle_gouraud:
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY
    move.l  d2,VOODOO_VERTEX_BX
    move.l  d3,VOODOO_VERTEX_BY
    move.l  d4,VOODOO_VERTEX_CX
    move.l  d5,VOODOO_VERTEX_CY

    move.l  #0,VOODOO_COLOR_SELECT
    move.l  cur_r,d6
    add.l   cur_hi,d6
    move.l  d6,VOODOO_START_R
    move.l  cur_g,d6
    add.l   cur_hi,d6
    move.l  d6,VOODOO_START_G
    move.l  cur_b,d6
    add.l   cur_hi,d6
    move.l  d6,VOODOO_START_B
    move.l  cur_a,VOODOO_START_A
    move.l  cur_z,VOODOO_START_Z

    move.l  #1,VOODOO_COLOR_SELECT
    move.l  cur_r,VOODOO_START_R
    move.l  cur_g,VOODOO_START_G
    move.l  cur_b,VOODOO_START_B
    move.l  cur_a,VOODOO_START_A
    move.l  cur_z,VOODOO_START_Z

    move.l  #2,VOODOO_COLOR_SELECT
    move.l  cur_r,d6
    sub.l   cur_lo,d6
    bpl.s   .r_ok
    clr.l   d6
.r_ok:
    move.l  d6,VOODOO_START_R
    move.l  cur_g,d6
    sub.l   cur_lo,d6
    bpl.s   .g_ok
    clr.l   d6
.g_ok:
    move.l  d6,VOODOO_START_G
    move.l  cur_b,d6
    sub.l   cur_lo,d6
    bpl.s   .b_ok
    clr.l   d6
.b_ok:
    move.l  d6,VOODOO_START_B
    move.l  cur_a,VOODOO_START_A
    move.l  cur_z,VOODOO_START_Z

    move.l  #0,VOODOO_TRIANGLE_CMD
    rts

; ============================================================================
; Data
; ============================================================================

zoom_scale:     dc.w    ZOOM_START
rotation_angle: dc.w    0
draw_scale:     dc.w    ZOOM_START
draw_xoff:      dc.w    0
draw_yoff:      dc.w    0
quad_count:     dc.w    0

cur_r:          dc.l    0
cur_g:          dc.l    0
cur_b:          dc.l    0
cur_a:          dc.l    $1000
cur_z:          dc.l    0
cur_hi:         dc.l    $0100
cur_lo:         dc.l    $0100

p0x:            dc.l    0
p0y:            dc.l    0
p1x:            dc.l    0
p1y:            dc.l    0
p2x:            dc.l    0
p2y:            dc.l    0
p3x:            dc.l    0
p3y:            dc.l    0

; Quad format:
;   x0,y0, x1,y1, x2,y2, x3,y3, r,g,b,a,z

logo_main_quads:
    ; "3"
    dc.w    -170,-86,  -56,-86,  -70,-61,  -184,-61,  $0180,$0A80,$0880,$1000,$4200
    dc.w     -72,-61,  -38,-49,  -55, -8,   -88,-17,  $0100,$0C00,$0980,$1000,$4000
    dc.w    -145,-21,  -50,-21,  -61,  5,  -156,  5,  $0080,$0E00,$0B00,$1000,$3F00
    dc.w     -64,  4,  -39, 18,  -76, 75,  -104, 59,  $0080,$0B80,$0900,$1000,$4000
    dc.w    -176, 57,  -75, 57,  -89, 84,  -190, 84,  $0180,$0A80,$0880,$1000,$4200

    ; "D"
    dc.w     -34,-82,   -7,-78,  -12, 79,   -43, 83,  $0200,$0D00,$0A00,$1000,$3A00
    dc.w      -6,-78,   53,-61,   82,-28,    18,-31,  $0180,$0F00,$0C00,$1000,$3900
    dc.w      18,-31,   87,-27,   90, 28,    26, 45,  $0100,$0C80,$0A00,$1000,$3A00
    dc.w      26, 45,   82, 30,   47, 70,   -12, 79,  $0100,$0B00,$0900,$1000,$3B00

    ; "fx"
    dc.w      96,-58,  119,-52,   92, 84,    69, 80,  $0100,$0100,$0120,$1000,$2600
    dc.w      71, -8,  137, -2,  130, 19,    66, 13,  $0100,$0100,$0120,$1000,$2600
    dc.w     143,-34,  164,-27,  205, 73,   181, 68,  $0100,$0100,$0120,$1000,$2500
    dc.w     198,-35,  220,-31,  164, 75,   141, 70,  $0100,$0100,$0120,$1000,$2500
    dc.w     174,  4,  197,  8,  184, 31,   162, 27,  $0200,$0200,$0240,$1000,$2400

logo_highlight_quads:
    dc.w    -158,-78,  -65,-78,  -72,-71,  -165,-71,  $0800,$1000,$0E00,$1000,$2200
    dc.w    -137,-12,  -58,-12,  -64, -4,  -143, -4,  $0800,$1000,$0E00,$1000,$2200
    dc.w    -164, 65,  -83, 65,  -90, 73,  -171, 73,  $0800,$1000,$0E00,$1000,$2200
    dc.w     -25,-72,   -9,-69,  -13, 71,   -29, 74,  $0900,$1000,$0F00,$1000,$2100
    dc.w       5,-69,   48,-54,   60,-42,    12,-45,  $0900,$1000,$0F00,$1000,$2100
    dc.w     105,-50,  113,-48,   86, 75,    78, 74,  $0500,$0500,$0600,$1000,$2000
    dc.w     151,-27,  159,-24,  198, 66,   190, 65,  $0500,$0500,$0600,$1000,$2000

sin_table:
    dc.b    0,3,6,9,12,15,18,21,24,27,30,33,36,39,42,45
    dc.b    48,51,54,57,59,62,65,67,70,73,75,78,80,82,85,87
    dc.b    89,91,94,96,98,100,102,103,105,107,108,110,112,113,114,116
    dc.b    117,118,119,120,121,122,123,123,124,125,125,126,126,126,127,127
    dc.b    127,127,127,126,126,126,125,125,124,123,123,122,121,120,119,118
    dc.b    117,116,114,113,112,110,108,107,105,103,102,100,98,96,94,91
    dc.b    89,87,85,82,80,78,75,73,70,67,65,62,59,57,54,51
    dc.b    48,45,42,39,36,33,30,27,24,21,18,15,12,9,6,3
    dc.b    0,-3,-6,-9,-12,-15,-18,-21,-24,-27,-30,-33,-36,-39,-42,-45
    dc.b    -48,-51,-54,-57,-59,-62,-65,-67,-70,-73,-75,-78,-80,-82,-85,-87
    dc.b    -89,-91,-94,-96,-98,-100,-102,-103,-105,-107,-108,-110,-112,-113,-114,-116
    dc.b    -117,-118,-119,-120,-121,-122,-123,-123,-124,-125,-125,-126,-126,-126,-127,-127
    dc.b    -127,-127,-127,-126,-126,-126,-125,-125,-124,-123,-123,-122,-121,-120,-119,-118
    dc.b    -117,-116,-114,-113,-112,-110,-108,-107,-105,-103,-102,-100,-98,-96,-94,-91
    dc.b    -89,-87,-85,-82,-80,-78,-75,-73,-70,-67,-65,-62,-59,-57,-54,-51
    dc.b    -48,-45,-42,-39,-36,-33,-30,-27,-24,-21,-18,-15,-12,-9,-6,-3

    end
