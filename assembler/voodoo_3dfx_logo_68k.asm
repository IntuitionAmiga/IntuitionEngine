; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                 3DFX INTERACTIVE BOOT LOGO ANIMATION                   ==
; ==                                                                        ==
; ==         Motorola 68020 Assembly for IntuitionEngine / Voodoo SST-1     ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
;                            ╭────────────────────╮
;                           ╱ ╲       *        ╱ ╲
;                          ╱   ╲    *   *    ╱   ╲
;                         ╱  *  ╲  ╭─────╮  ╱  *  ╲
;                        ╱   *   ╲│     │╱   *   ╲
;                       ╱    *    │ ███ │    *    ╲
;                      ╱     *   ╱│ ███ │╲   *     ╲
;                     ╱      *  ╱ │ ███ │ ╲  *      ╲
;                    ╱       * ╱  ╰─────╯  ╲ *       ╲
;                   ╱        ╱  3D      fx  ╲        ╲
;                  ╱       ╱        *         ╲       ╲
;                 ╱      ╱     *       *       ╲      ╲
;                ╱     ╱   *             *      ╲     ╲
;               ╲    ╱  *                 *     ╱    ╱
;                ╲  ╱ *         BOOT        * ╱  ╱
;                 ╲╱ *          LOGO         *╱ ╱
;                  ╲*                        *╱
;                   ╲*                      *╱
;                    ╲*                    *╱
;
; ============================================================================
; RECREATION OF THE ICONIC 3DFX INTERACTIVE BOOT LOGO
; ============================================================================
;
; This demo recreates the famous boot logo that appeared when PCs with
; 3DFX Voodoo graphics cards started up. It features:
;
;   - STARBURST BACKGROUND: Rotating spikes radiating from center
;     • White center (clean, bright)
;     • Yellow mid-range spikes (energy)
;     • Blue outer spikes (depth)
;
;   - "3D" TEXT: Metallic green with 3D extrusion effect
;
;   - "fx" TEXT: Black italic letters (the Interactive suffix)
;
;   - ZOOM ANIMATION: Logo zooms in from distance to full screen
;
;   - ROTATION: Spikes rotate slowly for visual interest
;
; Reading time: ~35 minutes for thorough understanding
;
; ============================================================================
; TABLE OF CONTENTS
; ============================================================================
;
;   Line    Section
;   ----    -------
;   ~90     Historical Context (3DFX, the Logo, the Era)
;   ~200    Architecture Overview
;   ~280    Constants
;   ~340    Program Entry Point
;   ~380    Main Loop
;   ~450    Draw Starburst (Background Effect)
;   ~680    Polar to Cartesian Conversion
;   ~720    Draw "3D" Text
;   ~1050   Draw "fx" Text
;   ~1400   Data Section (Sine Table)
;
; ============================================================================
; BUILD AND RUN
; ============================================================================
;
;   ASSEMBLE:
;     vasmm68k_mot -Fbin -m68020 -devpac -I. -o voodoo_3dfx_logo_68k.ie68 voodoo_3dfx_logo_68k.asm
;
;   RUN:
;     ./bin/IntuitionEngine -m68k voodoo_3dfx_logo_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
;
; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                       HISTORICAL CONTEXT                               ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    3DFX INTERACTIVE (1994-2002)                         │
; └─────────────────────────────────────────────────────────────────────────┘
;
; 3DFX Interactive was founded in 1994 by former Silicon Graphics engineers.
; Their mission: bring SGI-quality 3D graphics to consumer PCs at an
; affordable price point.
;
; THE VOODOO GRAPHICS (1996):
; The original Voodoo was the card that started the 3D gaming revolution.
; Before 3DFX, PC games either:
;   - Used slow software rendering
;   - Ran on expensive proprietary hardware (arcade boards)
;   - Required workstation-class machines ($10,000+)
;
; The Voodoo delivered:
;   - 50 million textured pixels per second
;   - 1 million triangles per second
;   - Hardware Z-buffering
;   - Bilinear texture filtering
;   - All for under $300
;
; ICONIC GAMES POWERED BY 3DFX:
;   - Quake (GLQuake, 1997)
;   - Tomb Raider (1996)
;   - Unreal (1998)
;   - Half-Life (1998)
;
; THE BOOT LOGO:
; When you turned on a PC with a Voodoo card, the BIOS would display
; this distinctive logo before handing off to the operating system.
; It became a symbol of gaming capability - if you saw this logo,
; you knew you had real 3D hardware.
;
; THE DEMISE (2000-2002):
; 3DFX was acquired by NVIDIA in December 2000 after financial troubles
; caused by the failed Voodoo 4/5 line. The brand disappeared, but
; their legacy lives on in every GPU today - they invented the
; consumer 3D graphics market.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE LOGO DESIGN                                      │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The 3DFX logo was carefully designed for visual impact:
;
;   STARBURST BACKGROUND:
;   Represents energy, power, and the "explosion" of 3D graphics
;   capability. The layered colors (blue → yellow → white) create
;   depth and a sense of radiating power from the center.
;
;   "3D" IN GREEN:
;   The teal/green metallic color was chosen because:
;     - It stands out against the blue/yellow background
;     - Green implies technology and innovation
;     - The 3D effect reinforces the company's focus
;
;   "fx" IN BLACK:
;   The lowercase italic "fx" adds a stylish, "effects" feel.
;   The contrast with the bold green "3D" creates visual hierarchy.
;   "fx" = "effects" - 3D effects!
;
;   ZOOM ANIMATION:
;   The logo zooms from small (distant) to full-size, creating a
;   dramatic reveal effect. This was common in 90s boot logos.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE STARBURST ALGORITHM                              │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The starburst is composed of 16 spikes arranged radially:
;
;                           spike
;                             /\
;                            /  \
;                           /    \
;                          /      \
;              spike     /   ○○○   \     spike
;                 \     /    ○○○    \     /
;                  \   /     ○○○     \   /
;                   \ /      ○○○      \ /
;                    /       ○○○       \
;             ──────/────────○○○────────\──────
;                  / \       ○○○       / \
;                 /   \      ○○○      /   \
;                /     \     ○○○     /     \
;               /       \    ○○○    /       \
;                        \   ○○○   /
;                         \  ○○○  /
;                          \ ○○○ /
;                           \○○○/
;                            \/
;
;   LAYERS (back to front):
;   1. BLUE SPIKES: Outer layer, extends far from center (z = 0xF000)
;   2. YELLOW SPIKES: Middle layer, shorter than blue (z = 0xE000)
;   3. WHITE CENTER: Front layer, filled circle (z = 0xD000)
;
;   POLAR COORDINATES:
;   Each spike is positioned using polar coordinates (angle, radius).
;   We convert to Cartesian (x, y) using:
;     x = cos(angle) × radius
;     y = sin(angle) × radius
;
;   SCALING:
;   The zoom_scale variable (20 → 256) is multiplied with all radii
;   to create the zoom-in effect.
;
; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                      ARCHITECTURE OVERVIEW                             ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    MAIN LOOP FLOW                                       │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   ╔═══════════════════════════════════════════════════════════════════╗
;   ║                        FRAME START                                ║
;   ╚═══════════════════════════════════════════════════════════════════╝
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 1. CLEAR FRAMEBUFFER TO BLACK                                     │
;   │    (Background is purely from the starburst triangles)            │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 2. DRAW STARBURST BACKGROUND                                      │
;   │    ├─ 16 blue outer spikes (back layer)                           │
;   │    ├─ 16 yellow middle spikes                                     │
;   │    └─ White center circle (16 triangles)                          │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 3. DRAW "3D" TEXT (if zoomed in enough)                           │
;   │    ├─ "3" character: three horizontal bars                        │
;   │    └─ "D" character: vertical bar + curved section                │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 4. DRAW "fx" TEXT (if zoomed in enough)                           │
;   │    ├─ "f" character: vertical stem + crossbar                     │
;   │    └─ "x" character: two crossing diagonal bars                   │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 5. SWAP BUFFERS                                                   │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 6. UPDATE ANIMATION                                               │
;   │    ├─ Increment zoom_scale (20 → 256)                             │
;   │    └─ Increment rotation_angle (continuous)                       │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ╔═══════════════════════════════════════════════════════════════════╗
;   ║                    LOOP TO FRAME START                            ║
;   ╚═══════════════════════════════════════════════════════════════════╝
;
; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                         CONSTANTS                                      ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

    include "ie68.inc"

; ============================================================================
; SCREEN CONFIGURATION
; ============================================================================

CENTER_X        equ SCREEN_W/2          ; 320 - horizontal center
CENTER_Y        equ SCREEN_H/2          ; 240 - vertical center

; ============================================================================
; FIXED-POINT FORMATS
; ============================================================================
; Voodoo uses 12.4 for vertex coordinates, 4.12 for colors.
; See the cube demo for detailed explanation.

FP_SHIFT        equ 4                   ; Shift for 12.4 format (× 16)
FP_COLOR        equ 12                  ; Shift for 4.12 format (not used here)

; ============================================================================
; STARBURST PARAMETERS
; ============================================================================
; The starburst has three concentric zones with different colors.
; Radii are in "design units" that get scaled by zoom_scale.
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │                     STARBURST GEOMETRY                              │
;   │                                                                     │
;   │                          SPIKE_OUTER (200)                          │
;   │                         ↙                                           │
;   │                       ╱                                             │
;   │                     ╱    SPIKE_MID (90)                             │
;   │                   ╱      ↙                                          │
;   │                 ╱      ╱                                            │
;   │               ╱      ╱    SPIKE_INNER (60)                          │
;   │             ╱      ╱      ↙                                         │
;   │           ╱      ╱      ╱                                           │
;   │         ╱      ╱      ╱                                             │
;   │        ●──────●──────●───────────────────────────────> radius       │
;   │      center  inner  mid    outer                                    │
;   │                                                                     │
;   │   Blue spikes: mid → outer                                          │
;   │   Yellow spikes: inner-5 → mid+10                                   │
;   │   White center: 0 → inner (filled)                                  │
;   └─────────────────────────────────────────────────────────────────────┘

NUM_SPIKES      equ 16                  ; Number of spikes around the circle
                                        ; 16 gives good density (22.5° apart)

SPIKE_INNER     equ 60                  ; Inner radius (white center edge)
SPIKE_MID       equ 90                  ; Middle radius (yellow/blue boundary)
SPIKE_OUTER     equ 200                 ; Outer radius (blue spike tips)

; ============================================================================
; ANIMATION PARAMETERS
; ============================================================================
; The logo zooms in from ZOOM_START to ZOOM_END.
; zoom_scale is a multiplier that scales all radii.
; At ZOOM_START=20, the logo is tiny. At ZOOM_END=256, it's full size.

ZOOM_START      equ 20                  ; Initial scale (small/far)
ZOOM_END        equ 256                 ; Final scale (full size)
ZOOM_SPEED      equ 3                   ; Scale increment per frame

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                       PROGRAM ENTRY POINT                              ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

    org     $1000

; ============================================================================
; START - Initialize Hardware and Enter Main Loop
; ============================================================================

start:
    ; ------------------------------------------------------------------------
    ; INITIALIZE STACK AND VOODOO
    ; ------------------------------------------------------------------------
    lea     $FF0000,sp                  ; Stack at high memory

    ; Set video dimensions: (640 << 16) | 480
    move.l  #(SCREEN_W<<16)|SCREEN_H,VOODOO_VIDEO_DIM

    ; Enable depth test and RGB write
    ; $0310 = depth_enable | rgb_write | depth_write | depth_less
    move.l  #$0310,VOODOO_FBZ_MODE

    ; ------------------------------------------------------------------------
    ; INITIALIZE ANIMATION STATE
    ; ------------------------------------------------------------------------
    ; Start zoomed out (small) with no rotation
    move.w  #ZOOM_START,zoom_scale      ; Begin at minimum scale
    clr.w   rotation_angle              ; Begin at angle 0

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                          MAIN LOOP                                     ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

main_loop:
    ; ------------------------------------------------------------------------
    ; CLEAR FRAMEBUFFER TO BLACK
    ; ------------------------------------------------------------------------
    ; Pure black background - all color comes from the starburst
    ; $FF000000 = Alpha=255, R=0, G=0, B=0
    move.l  #$FF000000,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; ------------------------------------------------------------------------
    ; DRAW THE THREE VISUAL LAYERS
    ; ------------------------------------------------------------------------
    ; Order matters for z-buffering, but we use explicit Z values
    ; to ensure proper layering regardless of draw order.

    ; 1. Starburst background (blue spikes → yellow spikes → white center)
    bsr     draw_starburst

    ; 2. "3D" text in green (only when zoomed in enough to be visible)
    bsr     draw_3d_text

    ; 3. "fx" text in black (only when zoomed in more)
    bsr     draw_fx_text

    ; ------------------------------------------------------------------------
    ; SWAP BUFFERS
    ; ------------------------------------------------------------------------
    move.l  #1,VOODOO_SWAP_BUFFER_CMD

    ; ------------------------------------------------------------------------
    ; UPDATE ZOOM ANIMATION
    ; ------------------------------------------------------------------------
    ; Increase scale until we reach full size
    move.w  zoom_scale,d0
    cmp.w   #ZOOM_END,d0
    bge.s   .no_zoom                    ; Already at max
    add.w   #ZOOM_SPEED,d0              ; Zoom in
    move.w  d0,zoom_scale
.no_zoom:

    ; ------------------------------------------------------------------------
    ; UPDATE ROTATION ANIMATION
    ; ------------------------------------------------------------------------
    ; Rotation speed varies:
    ;   - Fast rotation during zoom-in (dramatic reveal)
    ;   - Slow rotation after zoomed in (subtle motion)
    move.w  zoom_scale,d0
    cmp.w   #ZOOM_END-20,d0             ; Near full size?
    blt.s   .fast_rotate
    addq.w  #1,rotation_angle           ; Slow: 1 unit per frame
    bra.s   .rotate_done
.fast_rotate:
    add.w   #4,rotation_angle           ; Fast: 4 units per frame
.rotate_done:

    bra     main_loop                   ; Loop forever

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                   DRAW STARBURST BACKGROUND                            ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; The starburst is drawn in three passes (back to front):
;   1. Blue outer spikes (16 triangles) - Z = $F000 (farthest)
;   2. Yellow middle spikes (16 triangles) - Z = $E000
;   3. White center circle (16 triangles) - Z = $D000 (nearest)
;
; Each spike is a triangle with:
;   - One vertex at a smaller radius
;   - Two vertices at a larger radius (spike tip is between them)
;
; The rotation_angle is added to all angles to create the spin effect.
;
; ============================================================================

draw_starburst:
    movem.l d0-d7/a0-a2,-(sp)           ; Save registers

    move.w  zoom_scale,d7               ; D7 = current scale for all calculations

    ; ========================================================================
    ; PASS 1: BLUE OUTER SPIKES
    ; ========================================================================
    ; These form the outermost layer, extending from SPIKE_MID to SPIKE_OUTER.
    ; Dark blue color creates depth and the "energy burst" feeling.
    ; ========================================================================
    moveq   #NUM_SPIKES-1,d6            ; Loop counter: 16 spikes (15 down to 0)

.spike_loop:
    ; Calculate spike center angle
    ; angle = (spike_index × 16) + rotation_angle
    ; 16 = 256/16 = degrees per spike
    move.w  d6,d0
    lsl.w   #4,d0                       ; × 16 (256/16 = 16 per spike)
    add.w   rotation_angle,d0           ; Add current rotation
    and.w   #255,d0                     ; Wrap to 0-255

    ; Calculate angles for spike sides (±8 from center = ±11.25°)
    move.w  d0,d1
    add.w   #8,d1                       ; Right side of spike
    and.w   #255,d1

    move.w  d0,d2
    sub.w   #8,d2                       ; Left side of spike
    and.w   #255,d2

    ; ------------------------------------------------------------------------
    ; CALCULATE THREE VERTICES OF BLUE SPIKE TRIANGLE
    ; ------------------------------------------------------------------------

    ; Vertex A: Inner edge of spike (at mid radius)
    move.w  d0,d3                       ; Center angle
    move.w  d7,d4                       ; Current zoom scale
    mulu    #SPIKE_MID,d4               ; Scale × SPIKE_MID
    lsr.l   #8,d4                       ; Divide by 256 to normalize
    bsr     polar_to_cart               ; Convert to x,y
    move.l  d0,-(sp)                    ; Push center_x
    move.l  d1,-(sp)                    ; Push center_y

    ; Vertex B: Outer point 1 (left side of spike tip)
    move.w  d2,d3                       ; Left angle
    move.w  d7,d4
    mulu    #SPIKE_OUTER,d4             ; Scale × SPIKE_OUTER
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)                    ; Push outer1_x
    move.l  d1,-(sp)                    ; Push outer1_y

    ; Vertex C: Outer point 2 (right side of spike tip)
    ; Need to recalculate angle from d1 (which was corrupted)
    move.w  8(sp),d3                    ; Reload left angle from stack
    add.w   #16,d3                      ; Add full spike width (left → right)
    and.w   #255,d3
    move.w  d7,d4
    mulu    #SPIKE_OUTER,d4
    lsr.l   #8,d4
    bsr     polar_to_cart
    ; D0,D1 now hold outer2_x, outer2_y

    ; Retrieve saved points from stack
    move.l  (sp)+,d5                    ; outer1_y
    move.l  (sp)+,d4                    ; outer1_x
    move.l  (sp)+,d3                    ; center_y
    move.l  (sp)+,d2                    ; center_x

    ; ------------------------------------------------------------------------
    ; SET BLUE COLOR FOR OUTER SPIKES
    ; ------------------------------------------------------------------------
    ; Dark blue: R=0.125, G=0.25, B=0.75 (in 4.12 format)
    move.l  #$0200,VOODOO_START_R       ; R = 0.125
    move.l  #$0400,VOODOO_START_G       ; G = 0.25
    move.l  #$0C00,VOODOO_START_B       ; B = 0.75
    move.l  #$1000,VOODOO_START_A       ; A = 1.0 (opaque)
    move.l  #$F000,VOODOO_START_Z       ; Z = far back (behind everything)

    ; ------------------------------------------------------------------------
    ; SUBMIT BLUE SPIKE TRIANGLE TO VOODOO
    ; ------------------------------------------------------------------------
    ; Convert all coordinates to screen space and 12.4 format

    ; Vertex A (inner point)
    add.l   #CENTER_X,d2
    add.l   #CENTER_Y,d3
    lsl.l   #FP_SHIFT,d2
    lsl.l   #FP_SHIFT,d3
    move.l  d2,VOODOO_VERTEX_AX
    move.l  d3,VOODOO_VERTEX_AY

    ; Vertex B (outer left)
    add.l   #CENTER_X,d4
    add.l   #CENTER_Y,d5
    lsl.l   #FP_SHIFT,d4
    lsl.l   #FP_SHIFT,d5
    move.l  d4,VOODOO_VERTEX_BX
    move.l  d5,VOODOO_VERTEX_BY

    ; Vertex C (outer right)
    add.l   #CENTER_X,d0
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY

    ; Submit triangle
    move.l  #0,VOODOO_TRIANGLE_CMD

    dbf     d6,.spike_loop              ; Next spike

    ; ========================================================================
    ; PASS 2: YELLOW MIDDLE SPIKES
    ; ========================================================================
    ; These overlay the blue spikes with a warmer color, creating the
    ; transition zone between the blue outer area and white center.
    ; Yellow-gold color: R=1.0, G=0.8, B=0.0
    ; ========================================================================
    moveq   #NUM_SPIKES-1,d6

.yellow_loop:
    ; Calculate spike angle (same formula as blue)
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   rotation_angle,d0
    and.w   #255,d0

    ; Vertex A: Inner point (at inner radius - 5)
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER-5,d4           ; Slightly inside white center
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)                    ; Push inner_x
    move.l  d1,-(sp)                    ; Push inner_y

    ; Vertex B: Outer point (tip of yellow spike)
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_MID+10,d4            ; Just past mid radius
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)                    ; Push outer_x
    move.l  d1,-(sp)                    ; Push outer_y

    ; Vertex C: Side point (for triangle width)
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   #8,d0                       ; Offset for width
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER+10,d4          ; Between inner and mid
    lsr.l   #8,d4
    bsr     polar_to_cart

    ; Set yellow color
    move.l  #$1000,VOODOO_START_R       ; R = 1.0
    move.l  #$0D00,VOODOO_START_G       ; G = 0.8
    move.l  #$0000,VOODOO_START_B       ; B = 0.0
    move.l  #$1000,VOODOO_START_A       ; A = 1.0
    move.l  #$E000,VOODOO_START_Z       ; Z = middle layer

    ; Get points and draw
    move.l  (sp)+,d5                    ; outer_y
    move.l  (sp)+,d4                    ; outer_x
    move.l  (sp)+,d3                    ; inner_y
    move.l  (sp)+,d2                    ; inner_x

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

    ; ========================================================================
    ; PASS 3: WHITE CENTER CIRCLE
    ; ========================================================================
    ; The center is a filled circle made of 16 triangles radiating from
    ; the center point. Pure white for maximum brightness and contrast.
    ;
    ;   Each triangle:
    ;     Vertex A: Center (0, 0)
    ;     Vertex B: Edge at angle N
    ;     Vertex C: Edge at angle N+1
    ; ========================================================================
    moveq   #NUM_SPIKES-1,d6

.white_loop:
    ; First edge point
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER,d4
    lsr.l   #8,d4
    bsr     polar_to_cart
    move.l  d0,-(sp)                    ; Push edge1_x
    move.l  d1,-(sp)                    ; Push edge1_y

    ; Second edge point (next angle)
    move.w  d6,d0
    lsl.w   #4,d0
    add.w   #16,d0                      ; Next spike position
    add.w   rotation_angle,d0
    and.w   #255,d0
    move.w  d0,d3
    move.w  d7,d4
    mulu    #SPIKE_INNER,d4
    lsr.l   #8,d4
    bsr     polar_to_cart

    ; Set white color
    move.l  #$1000,VOODOO_START_R       ; R = 1.0
    move.l  #$1000,VOODOO_START_G       ; G = 1.0
    move.l  #$1000,VOODOO_START_B       ; B = 1.0
    move.l  #$1000,VOODOO_START_A       ; A = 1.0
    move.l  #$D000,VOODOO_START_Z       ; Z = front layer

    ; Vertex A: Center point
    move.l  #CENTER_X<<FP_SHIFT,VOODOO_VERTEX_AX
    move.l  #CENTER_Y<<FP_SHIFT,VOODOO_VERTEX_AY

    ; Vertex B: First edge point
    move.l  (sp)+,d3                    ; edge1_y
    move.l  (sp)+,d2                    ; edge1_x
    add.l   #CENTER_X,d2
    add.l   #CENTER_Y,d3
    lsl.l   #FP_SHIFT,d2
    lsl.l   #FP_SHIFT,d3
    move.l  d2,VOODOO_VERTEX_BX
    move.l  d3,VOODOO_VERTEX_BY

    ; Vertex C: Second edge point
    add.l   #CENTER_X,d0
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY

    move.l  #0,VOODOO_TRIANGLE_CMD

    dbf     d6,.white_loop

    movem.l (sp)+,d0-d7/a0-a2           ; Restore registers
    rts

; ============================================================================
; POLAR TO CARTESIAN - Convert polar coordinates to Cartesian
; ============================================================================
;
; Input:
;   D3 = angle (0-255, representing 0°-360°)
;   D4 = radius
;
; Output:
;   D0 = x = cos(angle) × radius / 128
;   D1 = y = sin(angle) × radius / 128
;
; Uses the sine table with cos(θ) = sin(θ + 64).
;
; ============================================================================

polar_to_cart:
    movem.l d2-d4/a0,-(sp)              ; Save work registers
    lea     sin_table,a0                ; Sine table base address

    ; Ensure angle is in range
    and.w   #255,d3

    ; Calculate cosine index (angle + 64 for 90° phase shift)
    move.w  d3,d2
    add.w   #64,d2
    and.w   #255,d2

    ; Look up cos and sin values
    move.b  0(a0,d2.w),d0               ; cos(angle)
    move.b  0(a0,d3.w),d1               ; sin(angle)
    ext.w   d0                          ; Sign-extend to 16-bit
    ext.w   d1
    ext.l   d0                          ; Sign-extend to 32-bit
    ext.l   d1

    ; Multiply by radius and scale back
    muls    d4,d0                       ; x = cos × radius
    muls    d4,d1                       ; y = sin × radius
    asr.l   #7,d0                       ; Divide by 128 (sin scale factor)
    asr.l   #7,d1

    movem.l (sp)+,d2-d4/a0              ; Restore registers
    rts

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                      DRAW "3D" TEXT                                    ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; The "3D" text is rendered as green metallic polygons.
; Each character is constructed from multiple triangles forming thick bars.
;
; "3" STRUCTURE:
;   ┌─────────┐
;   │ TOP BAR │
;   ├─────────┤
;   │ MID BAR │  (indented)
;   ├─────────┤
;   │ BOT BAR │
;   └─────────┘
;
; "D" STRUCTURE:
;   ┌───┬─────╮
;   │   │      ╲
;   │   │       │  (curved right side)
;   │   │      ╱
;   └───┴─────╯
;
; Coordinates are scaled by zoom_scale / 256 for the zoom effect.
;
; ============================================================================

draw_3d_text:
    movem.l d0-d7/a0-a2,-(sp)

    move.w  zoom_scale,d7

    ; Don't draw until zoomed in enough to be visible
    cmp.w   #80,d7
    blt     .done

    ; ------------------------------------------------------------------------
    ; SET GREEN METALLIC COLOR FOR "3"
    ; ------------------------------------------------------------------------
    ; Teal/green: R=0.0, G=0.625, B=0.5
    ; This creates the distinctive 3DFX green
    move.l  #$0000,VOODOO_START_R       ; R = 0
    move.l  #$0A00,VOODOO_START_G       ; G = 0.625
    move.l  #$0800,VOODOO_START_B       ; B = 0.5
    move.l  #$1000,VOODOO_START_A       ; A = 1.0
    move.l  #$4000,VOODOO_START_Z       ; Z = in front of starburst

    ; ========================================================================
    ; DRAW "3" CHARACTER - TOP BAR
    ; ========================================================================
    ; The "3" is positioned to the left of center (negative X)
    ; Top bar spans from x=-80 to x=-30, y=-50 to y=-35

    ; Triangle 1 of top bar: top-left, top-right, bottom-right
    move.w  d7,d0
    muls    #-80,d0                     ; x = -80 (scaled)
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1                     ; y = -50 (scaled)
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_AX
    move.l  d1,VOODOO_VERTEX_AY

    move.w  d7,d0
    muls    #-30,d0                     ; x = -30
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-50,d1                     ; y = -50
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_BX
    move.l  d1,VOODOO_VERTEX_BY

    move.w  d7,d0
    muls    #-30,d0                     ; x = -30
    asr.l   #8,d0
    add.l   #CENTER_X,d0
    move.w  d7,d1
    muls    #-35,d1                     ; y = -35
    asr.l   #8,d1
    add.l   #CENTER_Y,d1
    lsl.l   #FP_SHIFT,d0
    lsl.l   #FP_SHIFT,d1
    move.l  d0,VOODOO_VERTEX_CX
    move.l  d1,VOODOO_VERTEX_CY
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; Triangle 2 of top bar: top-left, bottom-right, bottom-left
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

    ; ========================================================================
    ; DRAW "3" CHARACTER - MIDDLE BAR
    ; ========================================================================
    ; Middle bar is indented (x=-60 instead of -80)
    ; Spans x=-60 to x=-30, y=-10 to y=+10

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

    ; ========================================================================
    ; DRAW "3" CHARACTER - BOTTOM BAR
    ; ========================================================================
    ; Same width as top bar
    ; Spans x=-80 to x=-30, y=+35 to y=+50

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

    ; ========================================================================
    ; DRAW "D" CHARACTER
    ; ========================================================================
    ; Slightly lighter green for highlight effect
    move.l  #$0200,VOODOO_START_R       ; R = 0.125
    move.l  #$0C00,VOODOO_START_G       ; G = 0.75
    move.l  #$0A00,VOODOO_START_B       ; B = 0.625

    ; D vertical bar (x=-20 to x=0, y=-50 to y=+50)
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

    ; D curve (simplified as two triangles forming the rounded part)
    ; Triangle 1 (upper curve)
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

    ; Triangle 2 (lower curve)
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
; ============================================================================
; ==                                                                        ==
; ==                      DRAW "fx" TEXT                                    ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; The "fx" text is rendered in black, positioned to the right of "3D".
; Lowercase italic style for the "effects" feel.
;
; "f" STRUCTURE:
;   ┌─────┐
;   │stem │───── crossbar
;   │     │
;   └─────┘
;
; "x" STRUCTURE:
;      ╲  ╱
;       ╲╱
;       ╱╲
;      ╱  ╲
;
; ============================================================================

draw_fx_text:
    movem.l d0-d7/a0-a2,-(sp)

    move.w  zoom_scale,d7

    ; Don't draw until zoomed in even more (text is smaller)
    cmp.w   #100,d7
    blt     .done

    ; ------------------------------------------------------------------------
    ; SET BLACK COLOR FOR "fx"
    ; ------------------------------------------------------------------------
    ; Nearly black with just a hint of color to distinguish from pure black BG
    move.l  #$0100,VOODOO_START_R       ; R ≈ 0.06
    move.l  #$0100,VOODOO_START_G       ; G ≈ 0.06
    move.l  #$0100,VOODOO_START_B       ; B ≈ 0.06
    move.l  #$1000,VOODOO_START_A       ; A = 1.0
    move.l  #$2000,VOODOO_START_Z       ; Z = in front of "3D"

    ; ========================================================================
    ; DRAW "f" CHARACTER - VERTICAL STEM
    ; ========================================================================
    ; Stem with slight italic slant (top shifted right)
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

    ; ========================================================================
    ; DRAW "f" CHARACTER - CROSSBAR
    ; ========================================================================
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

    ; ========================================================================
    ; DRAW "x" CHARACTER - FIRST DIAGONAL (top-left to bottom-right)
    ; ========================================================================
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

    ; ========================================================================
    ; DRAW "x" CHARACTER - SECOND DIAGONAL (top-right to bottom-left)
    ; ========================================================================
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
; ============================================================================
; ==                                                                        ==
; ==                         DATA SECTION                                   ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

; ============================================================================
; ANIMATION STATE VARIABLES
; ============================================================================

zoom_scale:     dc.w    ZOOM_START      ; Current zoom level (20-256)
rotation_angle: dc.w    0               ; Current rotation angle (0-65535)

; ============================================================================
; SINE TABLE
; ============================================================================
; 256-entry signed sine table. Values range from -127 to +127.
; Index 0-255 maps to angles 0°-360°.
;
; Used for polar→cartesian conversion in the starburst drawing.
; cos(θ) = sin_table[(θ + 64) & 255]
; ============================================================================
sin_table:
    ; Quadrant 0: 0° to 90° (indices 0-63)
    dc.b    0,3,6,9,12,15,18,21,24,27,30,33,36,39,42,45
    dc.b    48,51,54,57,59,62,65,67,70,73,75,78,80,82,85,87
    dc.b    89,91,94,96,98,100,102,103,105,107,108,110,112,113,114,116
    dc.b    117,118,119,120,121,122,123,123,124,125,125,126,126,126,127,127

    ; Quadrant 1: 90° to 180° (indices 64-127)
    dc.b    127,127,127,126,126,126,125,125,124,123,123,122,121,120,119,118
    dc.b    117,116,114,113,112,110,108,107,105,103,102,100,98,96,94,91
    dc.b    89,87,85,82,80,78,75,73,70,67,65,62,59,57,54,51
    dc.b    48,45,42,39,36,33,30,27,24,21,18,15,12,9,6,3

    ; Quadrant 2: 180° to 270° (indices 128-191)
    dc.b    0,-3,-6,-9,-12,-15,-18,-21,-24,-27,-30,-33,-36,-39,-42,-45
    dc.b    -48,-51,-54,-57,-59,-62,-65,-67,-70,-73,-75,-78,-80,-82,-85,-87
    dc.b    -89,-91,-94,-96,-98,-100,-102,-103,-105,-107,-108,-110,-112,-113,-114,-116
    dc.b    -117,-118,-119,-120,-121,-122,-123,-123,-124,-125,-125,-126,-126,-126,-127,-127

    ; Quadrant 3: 270° to 360° (indices 192-255)
    dc.b    -127,-127,-127,-126,-126,-126,-125,-125,-124,-123,-123,-122,-121,-120,-119,-118
    dc.b    -117,-116,-114,-113,-112,-110,-108,-107,-105,-103,-102,-100,-98,-96,-94,-91
    dc.b    -89,-87,-85,-82,-80,-78,-75,-73,-70,-67,-65,-62,-59,-57,-54,-51
    dc.b    -48,-45,-42,-39,-36,-33,-30,-27,-24,-21,-18,-15,-12,-9,-6,-3

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                         END OF PROGRAM                                 ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    ASSEMBLY STATISTICS                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   Code size:        ~2.5 KB
;   Sine table:       256 bytes
;   Total binary:     ~2.8 KB
;   Triangles/frame:  48 (16×3 starburst + 14 text) + dynamic
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    WHAT YOU SHOULD SEE                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   - Logo zooms in from small to full size
;   - Starburst rotates (fast during zoom, slow when done)
;   - Blue spikes, yellow middle, white center
;   - Green metallic "3D" text
;   - Black italic "fx" text
;   - Continuous slow rotation at full size
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    CUSTOMIZATION IDEAS                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   - Change NUM_SPIKES for more or fewer starburst rays
;   - Modify SPIKE_INNER/MID/OUTER for different proportions
;   - Adjust ZOOM_SPEED for faster/slower zoom-in
;   - Change colors in draw_starburst for different palette
;   - Add pulsing effect (vary colors over time)
;   - Add second zoom-out animation after full zoom
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    HISTORICAL NOTE                                      │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   3DFX Interactive (1994-2002) revolutionized consumer 3D graphics.
;   Their Voodoo cards powered the gaming revolution of the late 1990s.
;   While the company is gone (acquired by NVIDIA in 2000), their
;   innovations live on in every modern GPU.
;
;   This demo is a tribute to that era and those engineers who made
;   3D gaming accessible to everyone.
;
;   "What do you want to do today?" - 3DFX slogan
;
; ============================================================================

    end
