; voodoo_cube_68k.asm - 3D Rotating Cube Demo for Voodoo Graphics
;
; Demonstrates the 3DFX Voodoo SST-1 emulation with:
; - Z-buffered triangle rendering
; - Gouraud shading with per-face colors
; - Rotating 3D cube with depth sorting
;
; Build: vasmm68k_mot -Fbin -m68020 -o voodoo_cube_68k.ie68 voodoo_cube_68k.asm
; Run:   ./IntuitionEngine -68k voodoo_cube_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    include "ie68.inc"

; ============================================================================
; Constants
; ============================================================================

; SCREEN_W and SCREEN_H are defined in ie68.inc (640x480)
CENTER_X        equ SCREEN_W/2      ; 320
CENTER_Y        equ SCREEN_H/2      ; 240

; Cube size (in pixels)
CUBE_SIZE       equ 100

; Fixed-point scale factors
FP_12_4         equ 4               ; Shift for 12.4 format
FP_12_12        equ 12              ; Shift for 12.12 format

; Sin/cos table size (256 entries = full circle)
SIN_TABLE_SIZE  equ 256
SIN_SCALE       equ 128             ; Sin values scaled to -128..+127

; ============================================================================
; Entry Point
; ============================================================================

    org     $1000

start:
    ; Initialize stack (use high memory, not program area)
    lea     $FF0000,sp

    ; Set video dimensions
    move.l  #(SCREEN_W<<16)|SCREEN_H,VOODOO_VIDEO_DIM

    ; Enable depth testing with LESS function
    move.l  #$0310,VOODOO_FBZ_MODE  ; depth_enable | rgb_write | depth_write | depth_less<<5

    ; Initialize rotation angles
    clr.w   angle_x
    clr.w   angle_y
    clr.w   angle_z

; ============================================================================
; Main Loop
; ============================================================================

main_loop:
    ; Clear framebuffer to dark blue
    move.l  #$FF000040,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; Transform and draw cube faces
    bsr     draw_cube

    ; Swap buffers (wait for vsync)
    move.l  #1,VOODOO_SWAP_BUFFER_CMD

    ; Update rotation angles
    add.w   #1,angle_x
    add.w   #2,angle_y
    add.w   #1,angle_z

    bra     main_loop

; ============================================================================
; Draw Cube - Draws all 6 faces as 12 triangles
; ============================================================================

draw_cube:
    movem.l d0-d7/a0-a6,-(sp)

    ; Transform all 8 vertices
    bsr     transform_vertices

    ; Draw 6 faces (2 triangles each)
    ; Face order matters for z-buffering visual correctness

    ; Front face (z = +CUBE_SIZE) - Red
    move.l  #$1000,face_r
    move.l  #$0000,face_g
    move.l  #$0000,face_b
    lea     face_front,a0
    bsr     draw_face

    ; Back face (z = -CUBE_SIZE) - Cyan
    move.l  #$0000,face_r
    move.l  #$1000,face_g
    move.l  #$1000,face_b
    lea     face_back,a0
    bsr     draw_face

    ; Top face (y = -CUBE_SIZE) - Green
    move.l  #$0000,face_r
    move.l  #$1000,face_g
    move.l  #$0000,face_b
    lea     face_top,a0
    bsr     draw_face

    ; Bottom face (y = +CUBE_SIZE) - Magenta
    move.l  #$1000,face_r
    move.l  #$0000,face_g
    move.l  #$1000,face_b
    lea     face_bottom,a0
    bsr     draw_face

    ; Left face (x = -CUBE_SIZE) - Blue
    move.l  #$0000,face_r
    move.l  #$0000,face_g
    move.l  #$1000,face_b
    lea     face_left,a0
    bsr     draw_face

    ; Right face (x = +CUBE_SIZE) - Yellow
    move.l  #$1000,face_r
    move.l  #$1000,face_g
    move.l  #$0000,face_b
    lea     face_right,a0
    bsr     draw_face

    movem.l (sp)+,d0-d7/a0-a6
    rts

; ============================================================================
; Draw Face - Draws a quad as 2 triangles
; Input: A0 = pointer to face indices (4 bytes: v0, v1, v2, v3)
; ============================================================================

draw_face:
    movem.l d0-d7/a0-a2,-(sp)

    ; Get vertex indices
    moveq   #0,d0
    moveq   #0,d1
    moveq   #0,d2
    moveq   #0,d3
    move.b  (a0)+,d0            ; v0
    move.b  (a0)+,d1            ; v1
    move.b  (a0)+,d2            ; v2
    move.b  (a0)+,d3            ; v3

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
; Draw Triangle
; Input: D4 = vertex index 0, D5 = vertex index 1, D6 = vertex index 2
; ============================================================================

draw_triangle:
    movem.l d0-d7/a0,-(sp)

    lea     transformed_verts,a0

    ; Get vertex A (D4)
    move.l  d4,d0
    mulu    #12,d0              ; 12 bytes per vertex (x,y,z as longs)
    move.l  0(a0,d0.l),d1       ; X
    move.l  4(a0,d0.l),d2       ; Y
    move.l  8(a0,d0.l),d3       ; Z

    ; Convert to screen coordinates and 12.4 fixed-point
    add.l   #CENTER_X,d1
    add.l   #CENTER_Y,d2
    lsl.l   #FP_12_4,d1
    lsl.l   #FP_12_4,d2
    move.l  d1,VOODOO_VERTEX_AX
    move.l  d2,VOODOO_VERTEX_AY

    ; Get vertex B (D5)
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

    ; Get vertex C (D6)
    move.l  d6,d0
    mulu    #12,d0
    move.l  0(a0,d0.l),d1
    move.l  4(a0,d0.l),d2
    move.l  8(a0,d0.l),d7       ; Keep Z for depth
    add.l   #CENTER_X,d1
    add.l   #CENTER_Y,d2
    lsl.l   #FP_12_4,d1
    lsl.l   #FP_12_4,d2
    move.l  d1,VOODOO_VERTEX_CX
    move.l  d2,VOODOO_VERTEX_CY

    ; Set color (from face_r/g/b)
    move.l  face_r,VOODOO_START_R
    move.l  face_g,VOODOO_START_G
    move.l  face_b,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A

    ; Set Z depth (convert d7 to 20.12 format, offset to positive range)
    add.l   #CUBE_SIZE*2,d7     ; Make positive
    lsl.l   #8,d7               ; Scale for 20.12
    move.l  d7,VOODOO_START_Z

    ; Submit triangle
    move.l  #0,VOODOO_TRIANGLE_CMD

    movem.l (sp)+,d0-d7/a0
    rts

; ============================================================================
; Transform Vertices - Apply rotation matrices to cube vertices
; ============================================================================

transform_vertices:
    movem.l d0-d7/a0-a2,-(sp)

    lea     cube_vertices,a0
    lea     transformed_verts,a1

    moveq   #7,d7               ; 8 vertices

.transform_loop:
    ; Load vertex (x, y, z)
    move.w  (a0)+,d0            ; x
    move.w  (a0)+,d1            ; y
    move.w  (a0)+,d2            ; z
    ext.l   d0
    ext.l   d1
    ext.l   d2

    ; Rotate around Y axis
    move.w  angle_y,d3
    bsr     rotate_y

    ; Rotate around X axis
    move.w  angle_x,d3
    bsr     rotate_x

    ; Rotate around Z axis
    move.w  angle_z,d3
    bsr     rotate_z

    ; Store transformed vertex
    move.l  d0,(a1)+            ; x
    move.l  d1,(a1)+            ; y
    move.l  d2,(a1)+            ; z

    dbf     d7,.transform_loop

    movem.l (sp)+,d0-d7/a0-a2
    rts

; ============================================================================
; Rotation Subroutines
; Input: D0=x, D1=y, D2=z, D3=angle (0-255)
; Output: D0=x', D1=y', D2=z'
; ============================================================================

rotate_x:
    ; Rotate around X: y' = y*cos - z*sin, z' = y*sin + z*cos
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    ; Get sin and cos
    lea     sin_table,a2
    move.b  0(a2,d3.w),d4       ; sin(angle)
    ext.w   d4
    add.w   #64,d3              ; cos = sin(angle + 90)
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5       ; cos(angle)
    ext.w   d5

    ; y' = y*cos - z*sin
    move.l  d1,d6
    muls    d5,d6               ; y*cos
    move.l  d2,-(sp)
    muls    d4,d2               ; z*sin
    sub.l   d2,d6
    asr.l   #7,d6               ; Scale back
    move.l  (sp)+,d2

    ; z' = y*sin + z*cos
    muls    d4,d1               ; y*sin
    muls    d5,d2               ; z*cos
    add.l   d1,d2
    asr.l   #7,d2

    move.l  d6,d1               ; y' -> d1

    movem.l (sp)+,d4-d6
    rts

rotate_y:
    ; Rotate around Y: x' = x*cos + z*sin, z' = -x*sin + z*cos
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    lea     sin_table,a2
    move.b  0(a2,d3.w),d4       ; sin
    ext.w   d4
    add.w   #64,d3
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5       ; cos
    ext.w   d5

    ; x' = x*cos + z*sin
    move.l  d0,d6
    muls    d5,d6               ; x*cos
    move.l  d2,-(sp)
    muls    d4,d2               ; z*sin
    add.l   d2,d6
    asr.l   #7,d6
    move.l  (sp)+,d2

    ; z' = -x*sin + z*cos
    muls    d4,d0               ; x*sin
    neg.l   d0
    muls    d5,d2               ; z*cos
    add.l   d0,d2
    asr.l   #7,d2

    move.l  d6,d0               ; x' -> d0

    movem.l (sp)+,d4-d6
    rts

rotate_z:
    ; Rotate around Z: x' = x*cos - y*sin, y' = x*sin + y*cos
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    lea     sin_table,a2
    move.b  0(a2,d3.w),d4       ; sin
    ext.w   d4
    add.w   #64,d3
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5       ; cos
    ext.w   d5

    ; x' = x*cos - y*sin
    move.l  d0,d6
    muls    d5,d6               ; x*cos
    move.l  d1,-(sp)
    muls    d4,d1               ; y*sin
    sub.l   d1,d6
    asr.l   #7,d6
    move.l  (sp)+,d1

    ; y' = x*sin + y*cos
    muls    d4,d0               ; x*sin
    muls    d5,d1               ; y*cos
    add.l   d0,d1
    asr.l   #7,d1

    move.l  d6,d0               ; x' -> d0

    movem.l (sp)+,d4-d6
    rts

; ============================================================================
; Data
; ============================================================================

    even

; Rotation angles (0-255 maps to 0-360 degrees)
angle_x:    dc.w    0
angle_y:    dc.w    0
angle_z:    dc.w    0

; Current face color (12.12 fixed-point)
face_r:     dc.l    0
face_g:     dc.l    0
face_b:     dc.l    0

; Cube vertices (8 corners)
; Format: x, y, z (16-bit signed values)
cube_vertices:
    dc.w    -CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 0: back-top-left
    dc.w     CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 1: back-top-right
    dc.w     CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 2: back-bottom-right
    dc.w    -CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 3: back-bottom-left
    dc.w    -CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 4: front-top-left
    dc.w     CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 5: front-top-right
    dc.w     CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 6: front-bottom-right
    dc.w    -CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 7: front-bottom-left

; Face definitions (4 vertex indices per face, CCW winding)
face_front:     dc.b    4, 5, 6, 7      ; Front face (+Z)
face_back:      dc.b    1, 0, 3, 2      ; Back face (-Z)
face_top:       dc.b    0, 1, 5, 4      ; Top face (-Y)
face_bottom:    dc.b    7, 6, 2, 3      ; Bottom face (+Y)
face_left:      dc.b    0, 4, 7, 3      ; Left face (-X)
face_right:     dc.b    5, 1, 2, 6      ; Right face (+X)

    even

; Transformed vertices (8 vertices x 3 longs = 96 bytes)
transformed_verts:
    ds.l    24

; Sin table (256 entries, values -128 to +127)
sin_table:
    dc.b      0,   3,   6,   9,  12,  16,  19,  22
    dc.b     25,  28,  31,  34,  37,  40,  43,  46
    dc.b     49,  51,  54,  57,  60,  63,  65,  68
    dc.b     71,  73,  76,  78,  81,  83,  85,  88
    dc.b     90,  92,  94,  96,  98, 100, 102, 104
    dc.b    106, 107, 109, 111, 112, 113, 115, 116
    dc.b    117, 118, 120, 121, 122, 122, 123, 124
    dc.b    125, 125, 126, 126, 126, 127, 127, 127
    dc.b    127, 127, 127, 127, 126, 126, 126, 125
    dc.b    125, 124, 123, 122, 122, 121, 120, 118
    dc.b    117, 116, 115, 113, 112, 111, 109, 107
    dc.b    106, 104, 102, 100,  98,  96,  94,  92
    dc.b     90,  88,  85,  83,  81,  78,  76,  73
    dc.b     71,  68,  65,  63,  60,  57,  54,  51
    dc.b     49,  46,  43,  40,  37,  34,  31,  28
    dc.b     25,  22,  19,  16,  12,   9,   6,   3
    dc.b      0,  -3,  -6,  -9, -12, -16, -19, -22
    dc.b    -25, -28, -31, -34, -37, -40, -43, -46
    dc.b    -49, -51, -54, -57, -60, -63, -65, -68
    dc.b    -71, -73, -76, -78, -81, -83, -85, -88
    dc.b    -90, -92, -94, -96, -98,-100,-102,-104
    dc.b   -106,-107,-109,-111,-112,-113,-115,-116
    dc.b   -117,-118,-120,-121,-122,-122,-123,-124
    dc.b   -125,-125,-126,-126,-126,-127,-127,-127
    dc.b   -127,-127,-127,-127,-126,-126,-126,-125
    dc.b   -125,-124,-123,-122,-122,-121,-120,-118
    dc.b   -117,-116,-115,-113,-112,-111,-109,-107
    dc.b   -106,-104,-102,-100, -98, -96, -94, -92
    dc.b    -90, -88, -85, -83, -81, -78, -76, -73
    dc.b    -71, -68, -65, -63, -60, -57, -54, -51
    dc.b    -49, -46, -43, -40, -37, -34, -31, -28
    dc.b    -25, -22, -19, -16, -12,  -9,  -6,  -3

    end
