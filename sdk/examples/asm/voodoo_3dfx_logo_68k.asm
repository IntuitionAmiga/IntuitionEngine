; ============================================================================
; 3DFX BOOT LOGO - Animated Starburst Logo Recreation
; M68K (68020) for IntuitionEngine - Voodoo 3D (640x480, triangle rasteriser)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:     Motorola 68020
; Video chip:     3DFX Voodoo SST-1 (hardware 3D, Z-buffer, triangle rasteriser)
; Audio engine:   None
; Assembler:      vasmm68k_mot (VASM M68K, Motorola syntax)
; Include file:   ie68.inc (Voodoo MMIO register definitions)
;
; === WHAT THIS DEMO DOES ===
; Recreates the iconic 3dfx Interactive boot logo that appeared when PCs with
; Voodoo graphics cards powered on.  Features:
;   - Rotating starburst background (blue outer, yellow middle, white centre)
;   - Green metallic "3D" text built from Voodoo triangles
;   - Black italic "fx" text in front
;   - Zoom-in animation from small to full size
;   - Rotation speed shifts from fast (during zoom) to slow (at full size)
;
; === WHY THE 3DFX LOGO ===
; 3dfx Interactive (1994-2002) was founded by former Silicon Graphics engineers
; with a mission to bring SGI-quality 3D to consumer PCs.  Their Voodoo
; Graphics card (1996) was the first widely successful consumer 3D accelerator,
; delivering 50 million textured pixels per second for under $300.
;
; Before the Voodoo, PC 3D games relied on slow software rendering (Doom,
; Quake in software mode) or expensive workstation hardware ($10,000+).
; The Voodoo brought hardware triangle rasterisation, Z-buffering, bilinear
; texture filtering, and Gouraud shading to the mass market, powering titles
; like GLQuake (1997), Tomb Raider (1996), Unreal (1998), and Half-Life (1998).
;
; When you turned on a PC with a Voodoo card, this distinctive starburst logo
; appeared before the OS loaded -- a symbol that you had real 3D hardware.
; 3dfx was acquired by NVIDIA in December 2000, but their innovations live on
; in every modern GPU.
;
; This demo renders the logo entirely through the Voodoo's triangle pipeline:
; the starburst spikes are triangles in polar coordinates, the text characters
; are composed of triangle pairs forming rectangular bars, and Z-buffering
; ensures correct layering (blue behind yellow behind white behind text).
;
; === STARBURST ALGORITHM ===
;
;   The starburst is drawn in three passes (back to front):
;     1. Blue outer spikes  (16 triangles, Z = $F000)
;     2. Yellow middle spikes (16 triangles, Z = $E000)
;     3. White centre circle  (16 triangles, Z = $D000)
;
;   Each spike is positioned using polar coordinates (angle + radius),
;   converted to Cartesian via a 256-entry sine table:
;     x = cos(angle) * radius / 128
;     y = sin(angle) * radius / 128
;
;   All radii are scaled by zoom_scale (20 -> 256) to animate the zoom-in.
;
; === MEMORY MAP ===
;   $001000         Program code (org)
;   zoom_scale      Current zoom level (16-bit word, 20-256)
;   rotation_angle  Current starburst rotation (16-bit word)
;   sin_table       256-byte signed sine table (-127 to +127)
;   $0F0000+        Voodoo MMIO registers (ie68.inc)
;   $FF0000         Stack (grows downward)
;
; === BUILD AND RUN ===
;   vasmm68k_mot -Fbin -m68020 -devpac -I. -o voodoo_3dfx_logo_68k.ie68 voodoo_3dfx_logo_68k.asm
;   ./bin/IntuitionEngine -m68k voodoo_3dfx_logo_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

; ----------------------------------------------------------------------------
; Constants
; ----------------------------------------------------------------------------

CENTER_X        equ SCREEN_W/2          ; 320
CENTER_Y        equ SCREEN_H/2          ; 240

; Voodoo fixed-point shifts
FP_SHIFT        equ 4                   ; 12.4 vertex coordinates (pixel * 16)
FP_COLOR        equ 12                  ; 4.12 colour format (not used here)

; Starburst geometry
NUM_SPIKES      equ 16                  ; 16 spikes (22.5 degrees apart)
SPIKE_INNER     equ 60                  ; White centre radius
SPIKE_MID       equ 90                  ; Yellow/blue boundary radius
SPIKE_OUTER     equ 200                 ; Blue spike tip radius

; Zoom animation
ZOOM_START      equ 20                  ; Initial scale (small/far)
ZOOM_END        equ 256                 ; Final scale (full size)
ZOOM_SPEED      equ 3                   ; Scale increment per frame

; ============================================================================
; Program entry
; ============================================================================

    org     $1000

start:
    ; --- Initialise stack and Voodoo hardware ---
    lea     $FF0000,sp

    move.l  #1,VOODOO_ENABLE
    move.l  #(SCREEN_W<<16)|SCREEN_H,VOODOO_VIDEO_DIM

    ; fbzMode: depth_enable | depth_less | rgb_write | depth_write
    move.l  #$0630,VOODOO_FBZ_MODE

    ; --- Initialise animation state ---
    move.w  #ZOOM_START,zoom_scale
    clr.w   rotation_angle

; ============================================================================
; Main loop -- clear, draw starburst + text, swap buffers, animate
; ============================================================================

main_loop:
    ; --- Clear framebuffer to black ---
    move.l  #$FF000000,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; --- Draw the three visual layers (back to front) ---
    bsr     draw_starburst
    bsr     draw_3d_text
    bsr     draw_fx_text

    ; --- Swap buffers ---
    move.l  #1,VOODOO_SWAP_BUFFER_CMD

    ; --- Advance zoom (clamp at ZOOM_END) ---
    move.w  zoom_scale,d0
    cmp.w   #ZOOM_END,d0
    bge.s   .no_zoom
    add.w   #ZOOM_SPEED,d0
    move.w  d0,zoom_scale
.no_zoom:

    ; --- Advance rotation (fast during zoom, slow when done) ---
    move.w  zoom_scale,d0
    cmp.w   #ZOOM_END-20,d0
    blt.s   .fast_rotate
    addq.w  #1,rotation_angle
    bra.s   .rotate_done
.fast_rotate:
    add.w   #4,rotation_angle
.rotate_done:

    bra     main_loop

; ============================================================================
; draw_starburst -- three concentric layers of triangles
; ============================================================================
;
; WHY three layers: the starburst creates depth through colour and Z-value.
; Blue spikes (farthest) suggest the deep background energy.  Yellow spikes
; (middle) provide warmth and transition.  The white centre (nearest) is the
; bright focal point.  Z-buffering ensures correct occlusion even though we
; draw back-to-front for clarity.

draw_starburst:
    movem.l d0-d7/a0-a2,-(sp)

    move.w  zoom_scale,d7               ; D7 = current zoom scale

    ; ======================================================================
    ; Pass 1: Blue outer spikes (SPIKE_MID to SPIKE_OUTER, Z = $F000)
    ; ======================================================================
    moveq   #NUM_SPIKES-1,d6

.spike_loop:
    ; Spike centre angle = spike_index * 16 + rotation
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   rotation_angle,d0
    and.w   #255,d0

    ; Right and left edges of the spike (+-8 = +-11.25 degrees)
    move.w  d0,d1
    add.w   #8,d1
    and.w   #255,d1

    move.w  d0,d2
    sub.w   #8,d2
    and.w   #255,d2

    ; Vertex A: inner edge at SPIKE_MID radius
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_MID,d4
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)
    move.l  d1,-(sp)

    ; Vertex B: outer left
    move.w  d2,d3
    move.w  d7,d4
    mulu    #SPIKE_OUTER,d4
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)
    move.l  d1,-(sp)

    ; Vertex C: outer right
    move.w  8(sp),d3
    add.w   #16,d3
    and.w   #255,d3
    move.w  d7,d4
    mulu    #SPIKE_OUTER,d4
    lsr.l   #8,d4
    bsr     polar_to_cart

    ; Retrieve saved vertices
    move.l  (sp)+,d5                    ; outer1_y
    move.l  (sp)+,d4                    ; outer1_x
    move.l  (sp)+,d3                    ; centre_y
    move.l  (sp)+,d2                    ; centre_x

    ; Dark blue colour
    move.l  #$0200,VOODOO_START_R
    move.l  #$0400,VOODOO_START_G
    move.l  #$0C00,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$F000,VOODOO_START_Z

    ; Submit triangle (convert to screen coords + 12.4)
    add.l   #CENTER_X,d2
    add.l   #CENTER_Y,d3
    lsl.l   #FP_SHIFT,d2
    lsl.l   #FP_SHIFT,d3
    move.l  d2,VOODOO_VERTEX_AX
    move.l  d3,VOODOO_VERTEX_AY

    add.l   #CENTER_X,d4
    add.l   #CENTER_Y,d5
    lsl.l   #FP_SHIFT,d4
    lsl.l   #FP_SHIFT,d5
    move.l  d4,VOODOO_VERTEX_BX
    move.l  d5,VOODOO_VERTEX_BY

    add.l   #CENTER_X,d0
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY

    move.l  #0,VOODOO_TRIANGLE_CMD

    dbf     d6,.spike_loop

    ; ======================================================================
    ; Pass 2: Yellow middle spikes (SPIKE_INNER to SPIKE_MID, Z = $E000)
    ; ======================================================================
    moveq   #NUM_SPIKES-1,d6

.yellow_loop:
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   rotation_angle,d0
    and.w   #255,d0

    ; Vertex A: inner point
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER-5,d4
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)
    move.l  d1,-(sp)

    ; Vertex B: outer tip
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_MID+10,d4
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)
    move.l  d1,-(sp)

    ; Vertex C: side point
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   #8,d0
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER+10,d4
    lsr.l   #8,d4
    bsr     polar_to_cart

    ; Yellow-gold colour
    move.l  #$1000,VOODOO_START_R
    move.l  #$0D00,VOODOO_START_G
    move.l  #$0000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$E000,VOODOO_START_Z

    move.l  (sp)+,d5
    move.l  (sp)+,d4
    move.l  (sp)+,d3
    move.l  (sp)+,d2

    add.l   #CENTER_X,d2
    add.l   #CENTER_Y,d3
    lsl.l   #FP_SHIFT,d2
    lsl.l   #FP_SHIFT,d3
    move.l  d2,VOODOO_VERTEX_AX
    move.l  d3,VOODOO_VERTEX_AY

    add.l   #CENTER_X,d4
    add.l   #CENTER_Y,d5
    lsl.l   #FP_SHIFT,d4
    lsl.l   #FP_SHIFT,d5
    move.l  d4,VOODOO_VERTEX_BX
    move.l  d5,VOODOO_VERTEX_BY

    add.l   #CENTER_X,d0
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY

    move.l  #0,VOODOO_TRIANGLE_CMD

    dbf     d6,.yellow_loop

    ; ======================================================================
    ; Pass 3: White centre circle (16 fan triangles, Z = $D000)
    ; ======================================================================
    ; WHY a triangle fan: a filled circle is approximated by radiating
    ; triangles from the centre point to successive edge points.  With 16
    ; segments the result looks smooth enough at the starburst scale.
    ; ======================================================================
    moveq   #NUM_SPIKES-1,d6

.white_loop:
    ; Edge point 1
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER,d4
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)
    move.l  d1,-(sp)

    ; Edge point 2 (next segment)
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   #16,d0
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER,d4
    lsr.l   #8,d4
    bsr     polar_to_cart

    ; White colour
    move.l  #$1000,VOODOO_START_R
    move.l  #$1000,VOODOO_START_G
    move.l  #$1000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$D000,VOODOO_START_Z

    ; Vertex A: screen centre
    move.l  #CENTER_X<<FP_SHIFT,VOODOO_VERTEX_AX
    move.l  #CENTER_Y<<FP_SHIFT,VOODOO_VERTEX_AY

    ; Vertex B: edge point 1
    move.l  (sp)+,d3
    move.l  (sp)+,d2
    add.l   #CENTER_X,d2
    add.l   #CENTER_Y,d3
    lsl.l   #FP_SHIFT,d2
    lsl.l   #FP_SHIFT,d3
    move.l  d2,VOODOO_VERTEX_BX
    move.l  d3,VOODOO_VERTEX_BY

    ; Vertex C: edge point 2
    add.l   #CENTER_X,d0
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY

    move.l  #0,VOODOO_TRIANGLE_CMD

    dbf     d6,.white_loop

    movem.l (sp)+,d0-d7/a0-a2
    rts

; ============================================================================
; polar_to_cart -- convert (angle, radius) to (x, y)
; ============================================================================
; Input:  D3 = angle (0-255), D4 = radius
; Output: D0 = x = cos(angle) * radius / 128
;         D1 = y = sin(angle) * radius / 128

polar_to_cart:
    movem.l d2-d4/a0,-(sp)
    lea     sin_table,a0

    and.w   #255,d3

    ; cos(angle) = sin(angle + 64)
    move.w  d3,d2
    add.w   #64,d2
    and.w   #255,d2

    move.b  0(a0,d2.w),d0
    move.b  0(a0,d3.w),d1
    ext.w   d0
    ext.w   d1
    ext.l   d0
    ext.l   d1

    muls    d4,d0
    muls    d4,d1
    asr.l   #7,d0
    asr.l   #7,d1

    movem.l (sp)+,d2-d4/a0
    rts

; ============================================================================
; draw_3d_text -- green metallic "3D" in Voodoo triangles
; ============================================================================
;
; WHY: the "3D" text is the brand identity -- teal/green metallic, positioned
; left of centre.  Each character bar is a rectangle built from two triangles.
; All coordinates are scaled by zoom_scale/256 for the zoom-in animation.
; The text only appears once the zoom has reached scale 80 (enough to be
; legible).
;
; "3" structure: three horizontal bars (top, middle indented, bottom).
; "D" structure: vertical bar + two curve triangles for the rounded side.

draw_3d_text:
    movem.l d0-d7/a0-a2,-(sp)

    move.w  zoom_scale,d7

    ; Skip drawing until zoomed in enough to be visible
    cmp.w   #80,d7
    blt     .done

    ; --- Teal/green colour for "3" ---
    move.l  #$0000,VOODOO_START_R
    move.l  #$0A00,VOODOO_START_G
    move.l  #$0800,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$4000,VOODOO_START_Z

    ; === "3" TOP BAR (x=-80..-30, y=-50..-35) ===

    ; Triangle 1: top-left, top-right, bottom-right
    move.w  d7,d0
    muls    #-80,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-35,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2: top-left, bottom-right, bottom-left
    move.w  d7,d0
    muls    #-80,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-35,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-80,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-35,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; === "3" MIDDLE BAR (indented: x=-60..-30, y=-10..+10) ===

    ; Triangle 1
    move.w  d7,d0
    muls    #-60,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2
    move.w  d7,d0
    muls    #-60,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-60,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; === "3" BOTTOM BAR (x=-80..-30, y=+35..+50) ===

    ; Triangle 1
    move.w  d7,d0
    muls    #-80,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #35,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #35,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2
    move.w  d7,d0
    muls    #-80,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #35,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #-30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-80,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; === "D" CHARACTER (lighter green highlight) ===
    move.l  #$0200,VOODOO_START_R
    move.l  #$0C00,VOODOO_START_G
    move.l  #$0A00,VOODOO_START_B

    ; --- "D" vertical bar (x=-20..0, y=-50..+50) ---

    ; Triangle 1
    move.w  d7,d0
    muls    #-20,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #0,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #0,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2
    move.w  d7,d0
    muls    #-20,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #0,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-20,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; --- "D" curve: upper triangle ---
    move.w  d7,d0
    muls    #0,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-30,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #30,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; --- "D" curve: lower triangle ---
    move.w  d7,d0
    muls    #0,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #30,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #30,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #0,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

.done:
    movem.l (sp)+,d0-d7/a0-a2
    rts

; ============================================================================
; draw_fx_text -- black italic "fx" in front of the "3D"
; ============================================================================
;
; WHY: "fx" stands for "effects" -- the company's full name was
; "3DFX Interactive".  The lowercase italic style adds visual hierarchy
; against the bold green "3D".  Drawn at Z = $2000 (in front of everything).

draw_fx_text:
    movem.l d0-d7/a0-a2,-(sp)

    move.w  zoom_scale,d7

    ; Only draw when zoomed in enough (text is smaller than "3D")
    cmp.w   #100,d7
    blt     .done

    ; Near-black colour (slight tint to distinguish from pure black background)
    move.l  #$0100,VOODOO_START_R
    move.l  #$0100,VOODOO_START_G
    move.l  #$0100,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A
    move.l  #$2000,VOODOO_START_Z

    ; === "f" VERTICAL STEM (italic slant) ===

    ; Triangle 1
    move.w  d7,d0
    muls    #45,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-30,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #55,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-30,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #50,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2
    move.w  d7,d0
    muls    #45,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-30,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #50,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #40,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; === "f" CROSSBAR ===

    ; Triangle 1
    move.w  d7,d0
    muls    #40,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #0,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #60,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #0,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #58,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2
    move.w  d7,d0
    muls    #40,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #0,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #58,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #38,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #10,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; === "x" FIRST DIAGONAL (top-left to bottom-right) ===

    ; Triangle 1
    move.w  d7,d0
    muls    #65,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-20,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #75,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-20,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #95,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2
    move.w  d7,d0
    muls    #65,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-20,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #95,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #85,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; === "x" SECOND DIAGONAL (top-right to bottom-left) ===

    ; Triangle 1
    move.w  d7,d0
    muls    #90,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-20,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #100,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-20,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #75,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2
    move.w  d7,d0
    muls    #90,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-20,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #75,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #65,d0
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #50,d1
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

.done:
    movem.l (sp)+,d0-d7/a0-a2
    rts

; ============================================================================
; Data section -- animation state and sine table
; ============================================================================

zoom_scale:     dc.w    ZOOM_START      ; Current zoom level (20-256)
rotation_angle: dc.w    0               ; Current starburst rotation

; ----------------------------------------------------------------------------
; 256-entry signed sine table (-127 to +127)
; cos(angle) = sin_table[(angle + 64) & 255]
; ----------------------------------------------------------------------------

sin_table:
    ; Quadrant 0: 0 to 90 degrees
    dc.b    0,3,6,9,12,15,18,21,24,27,30,33,36,39,42,45
    dc.b    48,51,54,57,59,62,65,67,70,73,75,78,80,82,85,87
    dc.b    89,91,94,96,98,100,102,103,105,107,108,110,112,113,114,116
    dc.b    117,118,119,120,121,122,123,123,124,125,125,126,126,126,127,127

    ; Quadrant 1: 90 to 180 degrees
    dc.b    127,127,127,126,126,126,125,125,124,123,123,122,121,120,119,118
    dc.b    117,116,114,113,112,110,108,107,105,103,102,100,98,96,94,91
    dc.b    89,87,85,82,80,78,75,73,70,67,65,62,59,57,54,51
    dc.b    48,45,42,39,36,33,30,27,24,21,18,15,12,9,6,3

    ; Quadrant 2: 180 to 270 degrees
    dc.b    0,-3,-6,-9,-12,-15,-18,-21,-24,-27,-30,-33,-36,-39,-42,-45
    dc.b    -48,-51,-54,-57,-59,-62,-65,-67,-70,-73,-75,-78,-80,-82,-85,-87
    dc.b    -89,-91,-94,-96,-98,-100,-102,-103,-105,-107,-108,-110,-112,-113,-114,-116
    dc.b    -117,-118,-119,-120,-121,-122,-123,-123,-124,-125,-125,-126,-126,-126,-127,-127

    ; Quadrant 3: 270 to 360 degrees
    dc.b    -127,-127,-127,-126,-126,-126,-125,-125,-124,-123,-123,-122,-121,-120,-119,-118
    dc.b    -117,-116,-114,-113,-112,-110,-108,-107,-105,-103,-102,-100,-98,-96,-94,-91
    dc.b    -89,-87,-85,-82,-80,-78,-75,-73,-70,-67,-65,-62,-59,-57,-54,-51
    dc.b    -48,-45,-42,-39,-36,-33,-30,-27,-24,-21,-18,-15,-12,-9,-6,-3

    end
