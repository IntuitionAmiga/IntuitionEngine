; ============================================================================
; VOODOO CUBE - 3D Rotating Cube with Hardware Z-Buffering
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
; A solid-colour cube rotates continuously on all three axes (X, Y, Z) at
; different speeds, creating a smooth tumbling motion.  Each of the six faces
; has a distinct colour: red (front), cyan (back), green (top), magenta
; (bottom), blue (left), yellow (right).  The Voodoo's hardware Z-buffer
; handles hidden-surface removal automatically -- no depth sorting needed.
;
; === WHY A ROTATING CUBE ===
; The rotating 3D cube is the "Hello World" of 3D graphics, dating back to
; the early 1970s wireframe displays by Evans & Sutherland.  It teaches
; every fundamental concept of the 3D pipeline:
;
;   1. Vertex definition in model space (8 corners of a cube)
;   2. Rotation matrices (3x3 for each axis, using sine/cosine lookup)
;   3. Projection (orthographic here; perspective is the natural next step)
;   4. Face decomposition into triangles (each quad = 2 triangles)
;   5. Hidden surface removal via Z-buffering
;
; The 3dfx Voodoo (1996) was the first consumer card to perform steps 4-5
; in hardware.  Before the Voodoo, games like Elite (1984) sorted polygons
; manually (the "painter's algorithm") -- a complex and error-prone process.
; With Z-buffering, the hardware tests each pixel's depth automatically:
; if the new pixel is closer than what is already stored, it is drawn;
; otherwise it is discarded.  This demo relies entirely on this mechanism.
;
; === VOODOO FIXED-POINT FORMATS ===
;   12.4  (vertex coords):  pixel * 16           e.g. 320 -> 5120
;   4.12  (colours):        $1000 = 1.0 (full)   $0000 = 0.0 (off)
;   Z-buffer:               offset + scaled Z     (lower = closer)
;
; === ROTATION MATHEMATICS ===
; Each axis rotation uses the standard 3x3 matrix, implemented via a 256-entry
; signed sine table (values -127 to +127).  After multiplying a coordinate by
; sin or cos, we divide by 128 (ASR #7) to normalise back to pixel scale.
;
;   Rotation around X:  y' = y*cos - z*sin,  z' = y*sin + z*cos
;   Rotation around Y:  x' = x*cos + z*sin,  z' = -x*sin + z*cos
;   Rotation around Z:  x' = x*cos - y*sin,  y' = x*sin + y*cos
;
; === MEMORY MAP ===
;   $001000           Program code (org)
;   angle_x/y/z       Rotation angles (16-bit words)
;   face_r/g/b        Current face colour (32-bit, 4.12 format)
;   cube_vertices     8 vertices * 3 words (model space)
;   face_front..right  Face definitions (4 vertex indices each)
;   transformed_verts  8 vertices * 3 longs (rotated, world space)
;   sin_table          256-byte signed sine table
;   $0F0000+           Voodoo MMIO registers (ie68.inc)
;   $FF0000            Stack (grows downward)
;
; === BUILD AND RUN ===
;   vasmm68k_mot -Fbin -m68020 -devpac -o voodoo_cube_68k.ie68 voodoo_cube_68k.asm
;   ./bin/IntuitionEngine -m68k voodoo_cube_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

; ----------------------------------------------------------------------------
; Constants
; ----------------------------------------------------------------------------

CENTER_X        equ SCREEN_W/2      ; 320
CENTER_Y        equ SCREEN_H/2      ; 240

CUBE_SIZE       equ 100             ; Half-width (centre to face = 100 pixels)

FP_12_4         equ 4               ; Shift for 12.4 vertex format
FP_12_12        equ 12              ; Shift for 12.12 colour format (unused)

SIN_TABLE_SIZE  equ 256
SIN_SCALE       equ 128             ; Sine table amplitude

; ============================================================================
; Program entry -- initialise Voodoo and rotation state
; ============================================================================

    org     $1000

start:
    lea     $FF0000,sp

    move.l  #1,VOODOO_ENABLE
    move.l  #(SCREEN_W<<16)|SCREEN_H,VOODOO_VIDEO_DIM

    ; fbzMode: depth_enable | depth_less | rgb_write | depth_write
    move.l  #$0630,VOODOO_FBZ_MODE

    clr.w   angle_x
    clr.w   angle_y
    clr.w   angle_z

; ============================================================================
; Main loop -- clear, draw cube, swap, advance angles
; ============================================================================

main_loop:
    ; --- Clear to dark blue ($FF000040 ARGB) ---
    move.l  #$FF000040,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    bsr     draw_cube

    ; --- Swap buffers (Voodoo waits for vsync automatically) ---
    move.l  #1,VOODOO_SWAP_BUFFER_CMD

    ; --- Advance rotation: different speeds for tumbling ---
    ; X: +1/frame (slow roll),  Y: +2/frame (primary yaw),  Z: +1/frame
    add.w   #1,angle_x
    add.w   #2,angle_y
    add.w   #1,angle_z

    bra     main_loop

; ============================================================================
; draw_cube -- transform vertices, draw all 6 faces (12 triangles)
; ============================================================================
;
; WHY draw order does not matter: the Z-buffer tests every pixel.  We could
; draw the faces in any sequence and still get correct occlusion.  The face
; colours use complementary pairs (red/cyan, green/magenta, blue/yellow) so
; opposite faces are always distinguishable.

draw_cube:
    movem.l d0-d7/a0-a6,-(sp)

    ; --- Step 1: rotate all 8 vertices ---
    bsr     transform_vertices

    ; --- Step 2: draw each face as a coloured quad (2 triangles) ---

    ; Front (+Z): red
    move.l  #$1000,face_r
    move.l  #$0000,face_g
    move.l  #$0000,face_b
    lea     face_front,a0
    bsr     draw_face

    ; Back (-Z): cyan
    move.l  #$0000,face_r
    move.l  #$1000,face_g
    move.l  #$1000,face_b
    lea     face_back,a0
    bsr     draw_face

    ; Top (-Y): green
    move.l  #$0000,face_r
    move.l  #$1000,face_g
    move.l  #$0000,face_b
    lea     face_top,a0
    bsr     draw_face

    ; Bottom (+Y): magenta
    move.l  #$1000,face_r
    move.l  #$0000,face_g
    move.l  #$1000,face_b
    lea     face_bottom,a0
    bsr     draw_face

    ; Left (-X): blue
    move.l  #$0000,face_r
    move.l  #$0000,face_g
    move.l  #$1000,face_b
    lea     face_left,a0
    bsr     draw_face

    ; Right (+X): yellow
    move.l  #$1000,face_r
    move.l  #$1000,face_g
    move.l  #$0000,face_b
    lea     face_right,a0
    bsr     draw_face

    movem.l (sp)+,d0-d7/a0-a6
    rts

; ============================================================================
; draw_face -- render a quad (4 vertex indices) as 2 triangles
; ============================================================================
; Input: A0 = pointer to 4 bytes (vertex indices v0, v1, v2, v3)
;        face_r/g/b = colour
;
; Splits the quad along the v0-v2 diagonal:
;   Triangle 1: v0, v1, v2
;   Triangle 2: v0, v2, v3

draw_face:
    movem.l d0-d7/a0-a2,-(sp)

    moveq   #0,d0
    moveq   #0,d1
    moveq   #0,d2
    moveq   #0,d3
    move.b  (a0)+,d0
    move.b  (a0)+,d1
    move.b  (a0)+,d2
    move.b  (a0)+,d3

    ; Triangle 1: v0, v1, v2
    move.l  d0,d4
    move.l  d1,d5
    move.l  d2,d6
    bsr     draw_triangle

    ; Triangle 2: v0, v2, v3
    move.l  d0,d4
    move.l  d2,d5
    move.l  d3,d6
    bsr     draw_triangle

    movem.l (sp)+,d0-d7/a0-a2
    rts

; ============================================================================
; draw_triangle -- submit one triangle to the Voodoo rasteriser
; ============================================================================
; Input: D4/D5/D6 = vertex indices for A/B/C
;        face_r/g/b = triangle colour
;
; WHY the Z offset: transformed Z values range from -CUBE_SIZE to +CUBE_SIZE.
; The Z-buffer needs positive values, so we add CUBE_SIZE*2 and scale by 256
; to utilise the full depth precision.

draw_triangle:
    movem.l d0-d7/a0,-(sp)

    lea     transformed_verts,a0

    ; --- Vertex A ---
    move.l  d4,d0
    mulu    #12,d0
    move.l  0(a0,d0.l),d1
    move.l  4(a0,d0.l),d2
    move.l  8(a0,d0.l),d3

    add.l   #CENTER_X,d1
    add.l   #CENTER_Y,d2
    lsl.l   #FP_12_4,d1
    lsl.l   #FP_12_4,d2

    move.l  d1,VOODOO_VERTEX_AX
    move.l  d2,VOODOO_VERTEX_AY

    ; --- Vertex B ---
    move.l  d5,d0
    mulu    #12,d0
    move.l  0(a0,d0.l),d1
    move.l  4(a0,d0.l),d2
    add.l   #CENTER_X,d1
    add.l   #CENTER_Y,d2
    lsl.l   #FP_12_4,d1
    lsl.l   #FP_12_4,d2

    move.l  d1,VOODOO_VERTEX_BX
    move.l  d2,VOODOO_VERTEX_BY

    ; --- Vertex C ---
    move.l  d6,d0
    mulu    #12,d0
    move.l  0(a0,d0.l),d1
    move.l  4(a0,d0.l),d2
    move.l  8(a0,d0.l),d7
    add.l   #CENTER_X,d1
    add.l   #CENTER_Y,d2
    lsl.l   #FP_12_4,d1
    lsl.l   #FP_12_4,d2

    move.l  d1,VOODOO_VERTEX_CX
    move.l  d2,VOODOO_VERTEX_CY

    ; --- Colour ---
    move.l  face_r,VOODOO_START_R
    move.l  face_g,VOODOO_START_G
    move.l  face_b,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A

    ; --- Z depth (shift to positive range and scale) ---
    add.l   #CUBE_SIZE*2,d7
    lsl.l   #8,d7
    move.l  d7,VOODOO_START_Z

    ; --- Submit triangle ---
    move.l  #0,VOODOO_TRIANGLE_CMD

    movem.l (sp)+,d0-d7/a0
    rts

; ============================================================================
; transform_vertices -- apply Y, X, Z rotations to all 8 cube vertices
; ============================================================================
;
; WHY Y-X-Z order: different rotation orders produce different tumbling
; patterns.  Y first (yaw) gives the most natural "spinning top" feel.

transform_vertices:
    movem.l d0-d7/a0-a2,-(sp)

    lea     cube_vertices,a0
    lea     transformed_verts,a1

    moveq   #7,d7

.transform_loop:
    ; Load 16-bit model coordinates, sign-extend to 32-bit
    move.w  (a0)+,d0
    move.w  (a0)+,d1
    move.w  (a0)+,d2
    ext.l   d0
    ext.l   d1
    ext.l   d2

    ; Rotate Y (yaw), then X (pitch), then Z (roll)
    move.w  angle_y,d3
    bsr     rotate_y

    move.w  angle_x,d3
    bsr     rotate_x

    move.w  angle_z,d3
    bsr     rotate_z

    ; Store 32-bit results
    move.l  d0,(a1)+
    move.l  d1,(a1)+
    move.l  d2,(a1)+

    dbf     d7,.transform_loop

    movem.l (sp)+,d0-d7/a0-a2
    rts

; ============================================================================
; Rotation subroutines -- one per axis
; ============================================================================
;
; Each routine:
;   Input:  D0=x, D1=y, D2=z, D3=angle (low 8 bits used)
;   Output: D0=x', D1=y', D2=z' (the unchanged axis is preserved)
;
; The sine table values range -127..+127.  After multiplying, we ASR #7
; to divide by 128 and return to pixel-scale coordinates.

; --- rotate_x: pitch (y and z change, x unchanged) ---
;   y' = y*cos - z*sin
;   z' = y*sin + z*cos

rotate_x:
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    lea     sin_table,a2
    move.b  0(a2,d3.w),d4           ; sin
    ext.w   d4
    add.w   #64,d3
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5           ; cos
    ext.w   d5

    move.l  d1,d6
    muls    d5,d6                   ; y*cos
    move.l  d2,-(sp)
    muls    d4,d2                   ; z*sin
    sub.l   d2,d6                   ; y*cos - z*sin
    asr.l   #7,d6
    move.l  (sp)+,d2

    muls    d4,d1                   ; y*sin
    muls    d5,d2                   ; z*cos
    add.l   d1,d2                   ; y*sin + z*cos
    asr.l   #7,d2

    move.l  d6,d1

    movem.l (sp)+,d4-d6
    rts

; --- rotate_y: yaw (x and z change, y unchanged) ---
;   x' = x*cos + z*sin
;   z' = -x*sin + z*cos

rotate_y:
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    lea     sin_table,a2
    move.b  0(a2,d3.w),d4           ; sin
    ext.w   d4
    add.w   #64,d3
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5           ; cos
    ext.w   d5

    move.l  d0,d6
    muls    d5,d6                   ; x*cos
    move.l  d2,-(sp)
    muls    d4,d2                   ; z*sin
    add.l   d2,d6                   ; x*cos + z*sin
    asr.l   #7,d6
    move.l  (sp)+,d2

    muls    d4,d0                   ; x*sin
    neg.l   d0                      ; -(x*sin)
    muls    d5,d2                   ; z*cos
    add.l   d0,d2                   ; z*cos - x*sin
    asr.l   #7,d2

    move.l  d6,d0

    movem.l (sp)+,d4-d6
    rts

; --- rotate_z: roll (x and y change, z unchanged) ---
;   x' = x*cos - y*sin
;   y' = x*sin + y*cos

rotate_z:
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    lea     sin_table,a2
    move.b  0(a2,d3.w),d4           ; sin
    ext.w   d4
    add.w   #64,d3
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5           ; cos
    ext.w   d5

    move.l  d0,d6
    muls    d5,d6                   ; x*cos
    move.l  d1,-(sp)
    muls    d4,d1                   ; y*sin
    sub.l   d1,d6                   ; x*cos - y*sin
    asr.l   #7,d6
    move.l  (sp)+,d1

    muls    d4,d0                   ; x*sin
    muls    d5,d1                   ; y*cos
    add.l   d0,d1                   ; x*sin + y*cos
    asr.l   #7,d1

    move.l  d6,d0

    movem.l (sp)+,d4-d6
    rts

; ============================================================================
; Data section
; ============================================================================

    even

; --- Runtime variables ---
angle_x:    dc.w    0               ; X rotation (pitch)
angle_y:    dc.w    0               ; Y rotation (yaw)
angle_z:    dc.w    0               ; Z rotation (roll)

face_r:     dc.l    0               ; Current face red   (4.12)
face_g:     dc.l    0               ; Current face green (4.12)
face_b:     dc.l    0               ; Current face blue  (4.12)

; ----------------------------------------------------------------------------
; Cube vertices -- 8 corners in model space (16-bit signed words)
; ----------------------------------------------------------------------------
;
;              0---------1
;             /|        /|         Y (up = negative)
;            / |       / |         |
;           4---------5  |         |    Z (forward = positive)
;           |  |      |  |         |   /
;           |  3------|--2         |  /
;           | /       | /          | /
;           |/        |/           |/
;           7---------6            +---------- X (right = positive)

cube_vertices:
    dc.w    -CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 0: back-top-left
    dc.w     CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 1: back-top-right
    dc.w     CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 2: back-bottom-right
    dc.w    -CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 3: back-bottom-left
    dc.w    -CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 4: front-top-left
    dc.w     CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 5: front-top-right
    dc.w     CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 6: front-bottom-right
    dc.w    -CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 7: front-bottom-left

; ----------------------------------------------------------------------------
; Face definitions -- 4 vertex indices per quad, rendered as 2 triangles
; ----------------------------------------------------------------------------

face_front:     dc.b    4, 5, 6, 7      ; Front  (+Z)
face_back:      dc.b    1, 0, 3, 2      ; Back   (-Z)
face_top:       dc.b    0, 1, 5, 4      ; Top    (-Y)
face_bottom:    dc.b    7, 6, 2, 3      ; Bottom (+Y)
face_left:      dc.b    0, 4, 7, 3      ; Left   (-X)
face_right:     dc.b    5, 1, 2, 6      ; Right  (+X)

    even

; --- Transformed vertex buffer (filled by transform_vertices) ---
transformed_verts:
    ds.l    24                      ; 8 vertices * 3 longs (x, y, z)

; ----------------------------------------------------------------------------
; 256-entry signed sine table (-127 to +127)
; cos(angle) = sin_table[(angle + 64) & $FF]
; ----------------------------------------------------------------------------

sin_table:
    ; Quadrant 0: 0 to 90 degrees (rising)
    dc.b      0,   3,   6,   9,  12,  16,  19,  22
    dc.b     25,  28,  31,  34,  37,  40,  43,  46
    dc.b     49,  51,  54,  57,  60,  63,  65,  68
    dc.b     71,  73,  76,  78,  81,  83,  85,  88
    dc.b     90,  92,  94,  96,  98, 100, 102, 104
    dc.b    106, 107, 109, 111, 112, 113, 115, 116
    dc.b    117, 118, 120, 121, 122, 122, 123, 124
    dc.b    125, 125, 126, 126, 126, 127, 127, 127

    ; Quadrant 1: 90 to 180 degrees (falling)
    dc.b    127, 127, 127, 127, 126, 126, 126, 125
    dc.b    125, 124, 123, 122, 122, 121, 120, 118
    dc.b    117, 116, 115, 113, 112, 111, 109, 107
    dc.b    106, 104, 102, 100,  98,  96,  94,  92
    dc.b     90,  88,  85,  83,  81,  78,  76,  73
    dc.b     71,  68,  65,  63,  60,  57,  54,  51
    dc.b     49,  46,  43,  40,  37,  34,  31,  28
    dc.b     25,  22,  19,  16,  12,   9,   6,   3

    ; Quadrant 2: 180 to 270 degrees (negative, falling)
    dc.b      0,  -3,  -6,  -9, -12, -16, -19, -22
    dc.b    -25, -28, -31, -34, -37, -40, -43, -46
    dc.b    -49, -51, -54, -57, -60, -63, -65, -68
    dc.b    -71, -73, -76, -78, -81, -83, -85, -88
    dc.b    -90, -92, -94, -96, -98,-100,-102,-104
    dc.b   -106,-107,-109,-111,-112,-113,-115,-116
    dc.b   -117,-118,-120,-121,-122,-122,-123,-124
    dc.b   -125,-125,-126,-126,-126,-127,-127,-127

    ; Quadrant 3: 270 to 360 degrees (negative, rising)
    dc.b   -127,-127,-127,-127,-126,-126,-126,-125
    dc.b   -125,-124,-123,-122,-122,-121,-120,-118
    dc.b   -117,-116,-115,-113,-112,-111,-109,-107
    dc.b   -106,-104,-102,-100, -98, -96, -94, -92
    dc.b    -90, -88, -85, -83, -81, -78, -76, -73
    dc.b    -71, -68, -65, -63, -60, -57, -54, -51
    dc.b    -49, -46, -43, -40, -37, -34, -31, -28
    dc.b    -25, -22, -19, -16, -12,  -9,  -6,  -3

    end
