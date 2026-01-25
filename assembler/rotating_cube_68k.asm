; ============================================================================
; Rotating 3D Cube Demo - VGA Mode 13h (320x200x256)
; M68020 Assembly for IntuitionEngine
; Rotates on X and Y axes with VSync synchronization
; ============================================================================

                include "ie68.inc"

                org     PROGRAM_START

; --- Mode 13h Screen Constants ---
SCR_W           equ     320
SCR_H           equ     200
CENTER_X        equ     160
CENTER_Y        equ     100

; --- 3D Constants ---
NUM_VERTICES    equ     8
NUM_EDGES       equ     12
CUBE_SIZE       equ     60              ; Half-size of cube
DISTANCE        equ     256             ; Perspective distance

; --- Fixed Point (8.8 format) ---
FP_SHIFT        equ     8
FP_ONE          equ     256

; ============================================================================
; Entry Point
; ============================================================================
start:
                move.l  #STACK_TOP,sp   ; Initialize stack

                ; Set Mode 13h (320x200, 256 colors)
                move.b  #VGA_MODE_13H,VGA_MODE

                ; Enable VGA
                move.b  #VGA_CTRL_ENABLE,VGA_CTRL

                ; Initialize angles
                clr.l   angle_x
                clr.l   angle_y

; ============================================================================
; Main Loop
; ============================================================================
main_loop:
                ; Wait for vsync (wait until NOT in vblank first)
.wait_not_vb:   move.b  VGA_STATUS,d0
                andi.b  #VGA_STATUS_VSYNC,d0
                bne.s   .wait_not_vb

                ; Now wait for vblank to start
.wait_vb:       move.b  VGA_STATUS,d0
                andi.b  #VGA_STATUS_VSYNC,d0
                beq.s   .wait_vb

                ; Clear screen
                bsr     clear_screen

                ; Calculate sin/cos for current angles
                bsr     calc_rotation

                ; Transform and project all vertices
                bsr     transform_vertices

                ; Draw all edges
                bsr     draw_edges

                ; Update rotation angles
                addq.l  #2,angle_x      ; X rotation speed
                addq.l  #3,angle_y      ; Y rotation speed (slightly faster)

                ; Wrap angles (0-255 for table lookup)
                andi.l  #$FF,angle_x
                andi.l  #$FF,angle_y

                bra     main_loop

; ============================================================================
; Clear Screen - Fill with black (must use byte writes for VGA)
; ============================================================================
clear_screen:
                movem.l d0-d1/a0,-(sp)
                lea     VGA_VRAM,a0
                move.l  #SCR_W*SCR_H,d1
                moveq   #0,d0           ; Color 0 (black)
.clear_loop:
                move.b  d0,(a0)+
                subq.l  #1,d1
                bne.s   .clear_loop
                movem.l (sp)+,d0-d1/a0
                rts

; ============================================================================
; Calculate Rotation Matrix Components
; Uses angle_x and angle_y to compute sin/cos values
; ============================================================================
calc_rotation:
                movem.l d0-d1/a0,-(sp)

                ; Get sin/cos for X angle
                lea     sin_table,a0
                move.l  angle_x,d0
                andi.l  #$FF,d0
                move.w  (a0,d0.w*2),sin_x
                addi.w  #64,d0          ; cos = sin(angle + 64)
                andi.w  #$FF,d0
                move.w  (a0,d0.w*2),cos_x

                ; Get sin/cos for Y angle
                move.l  angle_y,d0
                andi.l  #$FF,d0
                move.w  (a0,d0.w*2),sin_y
                addi.w  #64,d0
                andi.w  #$FF,d0
                move.w  (a0,d0.w*2),cos_y

                movem.l (sp)+,d0-d1/a0
                rts

; ============================================================================
; Transform and Project All Vertices
; Applies Y rotation, then X rotation, then perspective projection
; ============================================================================
transform_vertices:
                movem.l d0-d7/a0-a2,-(sp)

                lea     cube_vertices,a0
                lea     projected_x,a1
                lea     projected_y,a2
                moveq   #NUM_VERTICES-1,d7

.vertex_loop:
                ; Load original vertex (x, y, z)
                move.w  (a0)+,d0        ; x
                move.w  (a0)+,d1        ; y
                move.w  (a0)+,d2        ; z
                ext.l   d0
                ext.l   d1
                ext.l   d2

                ; --- Y-axis rotation (around vertical axis) ---
                ; x' = x*cos_y - z*sin_y
                ; z' = x*sin_y + z*cos_y
                move.l  d0,d3           ; x
                move.l  d2,d4           ; z

                ; x' = x*cos_y - z*sin_y
                move.w  cos_y,d5
                ext.l   d5
                muls.w  d5,d3           ; x * cos_y
                move.w  sin_y,d5
                ext.l   d5
                muls.w  d5,d4           ; z * sin_y
                sub.l   d4,d3           ; x' = x*cos_y - z*sin_y
                asr.l   #FP_SHIFT,d3    ; Scale back

                ; z' = x*sin_y + z*cos_y
                move.l  d0,d4           ; x (original)
                move.l  d2,d6           ; z (original)
                move.w  sin_y,d5
                ext.l   d5
                muls.w  d5,d4           ; x * sin_y
                move.w  cos_y,d5
                ext.l   d5
                muls.w  d5,d6           ; z * cos_y
                add.l   d4,d6           ; z' = x*sin_y + z*cos_y
                asr.l   #FP_SHIFT,d6    ; Scale back

                move.l  d3,d0           ; x = x'
                move.l  d6,d2           ; z = z'

                ; --- X-axis rotation (around horizontal axis) ---
                ; y' = y*cos_x - z*sin_x
                ; z' = y*sin_x + z*cos_x
                move.l  d1,d3           ; y
                move.l  d2,d4           ; z

                ; y' = y*cos_x - z*sin_x
                move.w  cos_x,d5
                ext.l   d5
                muls.w  d5,d3           ; y * cos_x
                move.w  sin_x,d5
                ext.l   d5
                muls.w  d5,d4           ; z * sin_x
                sub.l   d4,d3           ; y' = y*cos_x - z*sin_x
                asr.l   #FP_SHIFT,d3    ; Scale back

                ; z' = y*sin_x + z*cos_x
                move.l  d1,d4           ; y (original)
                move.l  d2,d6           ; z (original)
                move.w  sin_x,d5
                ext.l   d5
                muls.w  d5,d4           ; y * sin_x
                move.w  cos_x,d5
                ext.l   d5
                muls.w  d5,d6           ; z * cos_x
                add.l   d4,d6           ; z' = y*sin_x + z*cos_x
                asr.l   #FP_SHIFT,d6    ; Scale back

                move.l  d3,d1           ; y = y'
                move.l  d6,d2           ; z = z'

                ; --- Perspective Projection ---
                ; screen_x = CENTER_X + (x * DISTANCE) / (z + DISTANCE)
                ; screen_y = CENTER_Y + (y * DISTANCE) / (z + DISTANCE)

                add.l   #DISTANCE,d2    ; z + DISTANCE (avoid div by zero)
                beq.s   .skip_div       ; Safety check
                bmi.s   .skip_div       ; Don't draw if behind camera

                ; Project X
                move.l  d0,d3
                muls.w  #DISTANCE,d3
                divs.w  d2,d3
                ext.l   d3
                add.l   #CENTER_X,d3
                move.w  d3,(a1)+        ; Store projected X

                ; Project Y
                move.l  d1,d3
                muls.w  #DISTANCE,d3
                divs.w  d2,d3
                ext.l   d3
                add.l   #CENTER_Y,d3
                move.w  d3,(a2)+        ; Store projected Y

                dbf     d7,.vertex_loop
                bra.s   .done

.skip_div:
                ; Vertex behind camera - store off-screen
                move.w  #-1000,(a1)+
                move.w  #-1000,(a2)+
                dbf     d7,.vertex_loop

.done:
                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; Draw All Edges
; ============================================================================
draw_edges:
                movem.l d0-d7/a0-a2,-(sp)

                lea     edge_list,a0
                moveq   #NUM_EDGES-1,d7

.edge_loop:
                ; Get vertex indices for this edge
                move.b  (a0)+,d0        ; Start vertex
                move.b  (a0)+,d1        ; End vertex
                ext.w   d0
                ext.w   d1

                ; Get projected coordinates
                lea     projected_x,a1
                lea     projected_y,a2

                ; Start point
                move.w  d0,d2
                add.w   d2,d2           ; *2 for word index
                move.w  (a1,d2.w),d3    ; x1
                move.w  (a2,d2.w),d4    ; y1

                ; End point
                move.w  d1,d2
                add.w   d2,d2           ; *2 for word index
                move.w  (a1,d2.w),d5    ; x2
                move.w  (a2,d2.w),d6    ; y2

                ; Calculate edge color based on index (for variety)
                move.l  d7,d2
                andi.l  #7,d2
                add.l   #40,d2          ; Colors 40-47 (greens in default palette)
                lsl.l   #4,d2           ; Brighter colors
                addi.l  #15,d2

                ; Draw the line
                ; d3=x1, d4=y1, d5=x2, d6=y2, d2=color
                bsr     draw_line

                dbf     d7,.edge_loop

                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; Draw Line (Bresenham's Algorithm)
; Input: d3=x1, d4=y1, d5=x2, d6=y2, d2=color
; ============================================================================
draw_line:
                movem.l d0-d7/a0,-(sp)

                ; Clip check - skip if obviously off-screen
                cmpi.w  #0,d3
                blt.s   .check_x2
                cmpi.w  #SCR_W,d3
                bge.s   .check_x2
                cmpi.w  #0,d4
                blt.s   .check_x2
                cmpi.w  #SCR_H,d4
                bge.s   .check_x2
                bra.s   .start_line

.check_x2:
                cmpi.w  #0,d5
                blt     .line_done
                cmpi.w  #SCR_W,d5
                bge     .line_done
                cmpi.w  #0,d6
                blt     .line_done
                cmpi.w  #SCR_H,d6
                bge     .line_done

.start_line:
                ; Calculate deltas
                move.w  d5,d0           ; dx = x2 - x1
                sub.w   d3,d0
                move.w  d6,d1           ; dy = y2 - y1
                sub.w   d4,d1

                ; Absolute values and step directions
                moveq   #1,d7           ; sx = 1
                tst.w   d0
                bge.s   .dx_pos
                neg.w   d0              ; dx = abs(dx)
                moveq   #-1,d7          ; sx = -1
.dx_pos:
                move.w  d7,-(sp)        ; Save sx

                moveq   #1,d7           ; sy = 1
                tst.w   d1
                bge.s   .dy_pos
                neg.w   d1              ; dy = abs(dy)
                moveq   #-1,d7          ; sy = -1
.dy_pos:
                move.w  d7,-(sp)        ; Save sy

                ; Determine major axis
                cmp.w   d1,d0           ; dx > dy?
                bge.s   .x_major

                ; Y-major line
                move.w  d1,d7           ; err = dy / 2
                lsr.w   #1,d7

.y_loop:
                bsr     plot_pixel
                cmp.w   d6,d4           ; y1 == y2?
                beq.s   .line_done_pop
                add.w   (sp),d4         ; y1 += sy
                sub.w   d0,d7           ; err -= dx
                bge.s   .y_loop
                add.w   d1,d7           ; err += dy
                add.w   2(sp),d3        ; x1 += sx
                bra.s   .y_loop

.x_major:
                ; X-major line
                move.w  d0,d7           ; err = dx / 2
                lsr.w   #1,d7

.x_loop:
                bsr     plot_pixel
                cmp.w   d5,d3           ; x1 == x2?
                beq.s   .line_done_pop
                add.w   2(sp),d3        ; x1 += sx
                sub.w   d1,d7           ; err -= dy
                bge.s   .x_loop
                add.w   d0,d7           ; err += dx
                add.w   (sp),d4         ; y1 += sy
                bra.s   .x_loop

.line_done_pop:
                addq.l  #4,sp           ; Clean up sx, sy
.line_done:
                movem.l (sp)+,d0-d7/a0
                rts

; ============================================================================
; Plot Pixel with Clipping
; Input: d3=x, d4=y, d2=color
; ============================================================================
plot_pixel:
                ; Bounds check
                tst.w   d3
                bmi.s   .skip
                cmpi.w  #SCR_W,d3
                bge.s   .skip
                tst.w   d4
                bmi.s   .skip
                cmpi.w  #SCR_H,d4
                bge.s   .skip

                ; Calculate address: VGA_VRAM + y * 320 + x
                movem.l d0-d1/a0,-(sp)
                lea     VGA_VRAM,a0
                move.w  d4,d0
                mulu    #SCR_W,d0    ; y * 320
                add.w   d3,d0           ; + x
                move.b  d2,(a0,d0.l)    ; Write pixel
                movem.l (sp)+,d0-d1/a0
.skip:
                rts

; ============================================================================
; Data Section
; ============================================================================

                even

; --- Rotation State ---
angle_x:        dc.l    0
angle_y:        dc.l    0
sin_x:          dc.w    0
cos_x:          dc.w    0
sin_y:          dc.w    0
cos_y:          dc.w    0

; --- Cube Vertices (x, y, z) ---
; Unit cube centered at origin, scaled by CUBE_SIZE
cube_vertices:
                dc.w    -CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE   ; 0: back-bottom-left
                dc.w     CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE   ; 1: back-bottom-right
                dc.w     CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE   ; 2: back-top-right
                dc.w    -CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE   ; 3: back-top-left
                dc.w    -CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE   ; 4: front-bottom-left
                dc.w     CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE   ; 5: front-bottom-right
                dc.w     CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE   ; 6: front-top-right
                dc.w    -CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE   ; 7: front-top-left

; --- Edge List (vertex index pairs) ---
edge_list:
                ; Back face
                dc.b    0, 1
                dc.b    1, 2
                dc.b    2, 3
                dc.b    3, 0
                ; Front face
                dc.b    4, 5
                dc.b    5, 6
                dc.b    6, 7
                dc.b    7, 4
                ; Connecting edges
                dc.b    0, 4
                dc.b    1, 5
                dc.b    2, 6
                dc.b    3, 7

                even

; --- Projected Coordinates (filled at runtime) ---
projected_x:    ds.w    NUM_VERTICES
projected_y:    ds.w    NUM_VERTICES

; --- Sin Table (256 entries, 8.8 fixed point) ---
; sin(i * 360 / 256) * 256, where i = 0..255
sin_table:
                dc.w    0, 6, 12, 18, 25, 31, 37, 43
                dc.w    49, 56, 62, 68, 74, 80, 86, 92
                dc.w    97, 103, 109, 115, 120, 126, 131, 136
                dc.w    142, 147, 152, 157, 162, 167, 171, 176
                dc.w    181, 185, 189, 193, 197, 201, 205, 209
                dc.w    212, 216, 219, 222, 225, 228, 231, 234
                dc.w    236, 238, 241, 243, 245, 247, 248, 250
                dc.w    251, 252, 253, 254, 255, 255, 256, 256
                dc.w    256, 256, 256, 255, 255, 254, 253, 252
                dc.w    251, 250, 248, 247, 245, 243, 241, 238
                dc.w    236, 234, 231, 228, 225, 222, 219, 216
                dc.w    212, 209, 205, 201, 197, 193, 189, 185
                dc.w    181, 176, 171, 167, 162, 157, 152, 147
                dc.w    142, 136, 131, 126, 120, 115, 109, 103
                dc.w    97, 92, 86, 80, 74, 68, 62, 56
                dc.w    49, 43, 37, 31, 25, 18, 12, 6
                dc.w    0, -6, -12, -18, -25, -31, -37, -43
                dc.w    -49, -56, -62, -68, -74, -80, -86, -92
                dc.w    -97, -103, -109, -115, -120, -126, -131, -136
                dc.w    -142, -147, -152, -157, -162, -167, -171, -176
                dc.w    -181, -185, -189, -193, -197, -201, -205, -209
                dc.w    -212, -216, -219, -222, -225, -228, -231, -234
                dc.w    -236, -238, -241, -243, -245, -247, -248, -250
                dc.w    -251, -252, -253, -254, -255, -255, -256, -256
                dc.w    -256, -256, -256, -255, -255, -254, -253, -252
                dc.w    -251, -250, -248, -247, -245, -243, -241, -238
                dc.w    -236, -234, -231, -228, -225, -222, -219, -216
                dc.w    -212, -209, -205, -201, -197, -193, -189, -185
                dc.w    -181, -176, -171, -167, -162, -157, -152, -147
                dc.w    -142, -136, -131, -126, -120, -115, -109, -103
                dc.w    -97, -92, -86, -80, -74, -68, -62, -56
                dc.w    -49, -43, -37, -31, -25, -18, -12, -6

                end
