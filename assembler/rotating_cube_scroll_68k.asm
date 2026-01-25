; ============================================================================
; Rotating 3D Cube Demo with Circular Scrolltext and SID Music
; M68020 Assembly for IntuitionEngine - VGA Mode 13h (320x200x256)
; Features: 3D cube, 360-degree circular scroller, Edge of Disgrace SID
; ============================================================================

                include "ie68.inc"

                org     PROGRAM_START

; --- Mode 13h Screen Constants ---
SCR_W           equ     320
SCR_H           equ     200
CENTER_X        equ     160
CENTER_Y        equ     100

; --- 3D Cube Constants ---
NUM_VERTICES    equ     8
NUM_EDGES       equ     12
CUBE_SIZE       equ     40              ; Smaller cube to make room for scroller
DISTANCE        equ     256

; --- Fixed Point (8.8 format) ---
FP_SHIFT        equ     8
FP_ONE          equ     256

; --- Circular Scroller Constants ---
SCROLL_RADIUS   equ     75              ; Radius of text circle
SCROLL_CENTER_X equ     160             ; Center of circle
SCROLL_CENTER_Y equ     100             ; Center of circle
NUM_VISIBLE     equ     12              ; Characters visible around circle
CHAR_SPACING    equ     21              ; Angle spacing (256/12 ≈ 21)
FONT_CHAR_W     equ     32              ; Original font char width
FONT_CHAR_H     equ     32              ; Original font char height
FONT_STRIDE     equ     1280            ; Font image stride (bytes per row)
DRAW_CHAR_SIZE  equ     8               ; Draw chars at 8x8 (sample every 4th pixel)
SCROLL_SPEED    equ     1               ; Rotation speed

; ============================================================================
; Entry Point
; ============================================================================
start:
                move.l  #STACK_TOP,sp

                ; Set Mode 13h (320x200, 256 colors)
                move.b  #VGA_MODE_13H,VGA_MODE
                move.b  #VGA_CTRL_ENABLE,VGA_CTRL

                ; Start SID music
                move.l  #sid_data,SID_PLAY_PTR
                move.l  #sid_data_end-sid_data,SID_PLAY_LEN
                start_sid_loop

                ; Initialize variables
                clr.l   angle_x
                clr.l   angle_y
                clr.l   scroll_angle
                clr.l   scroll_char_offset

; ============================================================================
; Main Loop
; ============================================================================
main_loop:
                ; Wait for vsync
.wait_not_vb:   move.b  VGA_STATUS,d0
                andi.b  #VGA_STATUS_VSYNC,d0
                bne.s   .wait_not_vb
.wait_vb:       move.b  VGA_STATUS,d0
                andi.b  #VGA_STATUS_VSYNC,d0
                beq.s   .wait_vb

                ; Clear screen
                bsr     clear_screen

                ; Draw circular scrolltext (behind cube)
                bsr     draw_circular_scroll

                ; Calculate rotation
                bsr     calc_rotation

                ; Transform and project vertices
                bsr     transform_vertices

                ; Draw cube edges
                bsr     draw_edges

                ; Update cube angles
                addq.l  #2,angle_x
                addq.l  #3,angle_y
                andi.l  #$FF,angle_x
                andi.l  #$FF,angle_y

                ; Update scroll angle
                move.l  scroll_angle,d0
                addq.l  #SCROLL_SPEED,d0
                andi.l  #$FF,d0
                move.l  d0,scroll_angle

                bra     main_loop

; ============================================================================
; Clear Screen
; ============================================================================
clear_screen:
                movem.l d0-d1/a0,-(sp)
                lea     VGA_VRAM,a0
                move.l  #SCR_W*SCR_H,d1
                moveq   #0,d0
.clear_loop:
                move.b  d0,(a0)+
                subq.l  #1,d1
                bne.s   .clear_loop
                movem.l (sp)+,d0-d1/a0
                rts

; ============================================================================
; Draw Circular Scrolltext
; Characters orbit around the center like a clock face
; ============================================================================
draw_circular_scroll:
                movem.l d0-d7/a0-a4,-(sp)

                moveq   #0,d7                   ; d7 = visible char index (0 to NUM_VISIBLE-1)

.char_loop:
                ; Calculate angle for this character position
                ; angle = scroll_angle + (char_index * CHAR_SPACING)
                move.l  scroll_angle,d0
                move.l  d7,d1
                mulu.w  #CHAR_SPACING,d1
                add.l   d1,d0
                andi.l  #$FF,d0                 ; Wrap to 0-255

                ; Get sin/cos for position
                lea     sin_table,a0
                move.l  d0,d1
                add.w   d1,d1                   ; *2 for word access
                move.w  (a0,d1.w),d2            ; d2 = sin(angle) (8.8 fixed)

                move.l  d0,d1
                addi.l  #64,d1                  ; cos = sin(angle + 64)
                andi.l  #$FF,d1
                add.w   d1,d1
                move.w  (a0,d1.w),d3            ; d3 = cos(angle) (8.8 fixed)

                ; Calculate screen position
                ; x = CENTER_X + (radius * cos) >> 8
                ; y = CENTER_Y + (radius * sin) >> 8
                move.w  d3,d4
                ext.l   d4
                muls.w  #SCROLL_RADIUS,d4
                asr.l   #FP_SHIFT,d4
                addi.l  #SCROLL_CENTER_X,d4
                subi.l  #DRAW_CHAR_SIZE/2,d4    ; Center the character

                move.w  d2,d5
                ext.l   d5
                muls.w  #SCROLL_RADIUS,d5
                asr.l   #FP_SHIFT,d5
                addi.l  #SCROLL_CENTER_Y,d5
                subi.l  #DRAW_CHAR_SIZE/2,d5    ; Center the character

                ; Get character from message
                ; char_index_in_message = scroll_char_offset + visible_index
                move.l  scroll_char_offset,d6
                add.l   d7,d6

                ; Wrap and get character
                lea     scroll_message,a1
                lea     scroll_msg_end,a2
                move.l  a2,d0
                sub.l   a1,d0                   ; d0 = message length
                subq.l  #1,d0                   ; Exclude null terminator

.wrap_check:
                cmp.l   d0,d6
                blt.s   .no_wrap
                sub.l   d0,d6
                bra.s   .wrap_check
.no_wrap:
                move.b  (a1,d6.l),d6            ; d6 = ASCII character
                andi.l  #$7F,d6

                ; Draw the character at (d4, d5)
                ; d4 = x, d5 = y, d6 = ASCII char
                bsr     draw_scroll_char

                ; Next visible character
                addq.l  #1,d7
                cmpi.l  #NUM_VISIBLE,d7
                blt     .char_loop

                ; Advance scroll offset every few frames
                move.l  scroll_angle,d0
                andi.l  #$1F,d0                 ; Every 32 angle steps
                bne.s   .no_advance
                move.l  scroll_char_offset,d0
                addq.l  #1,d0
                move.l  d0,scroll_char_offset
.no_advance:

                movem.l (sp)+,d0-d7/a0-a4
                rts

; ============================================================================
; Draw Scroll Character (8x8 simple bitmap font)
; Input: d4 = x, d5 = y, d6 = ASCII code
; ============================================================================
draw_scroll_char:
                movem.l d0-d7/a0-a2,-(sp)

                ; Bounds check
                tst.l   d4
                bmi     .char_done
                cmpi.l  #SCR_W-8,d4
                bgt     .char_done
                tst.l   d5
                bmi     .char_done
                cmpi.l  #SCR_H-8,d5
                bgt     .char_done

                ; Get font data for this character
                ; Font is 8x8, 8 bytes per char (1 bit per pixel)
                lea     simple_font,a0

                ; Map ASCII to font index (space=0, !-~ = 1-94)
                subi.l  #32,d6                  ; Convert ASCII to font index
                bmi     .char_done              ; Skip if < space
                cmpi.l  #95,d6
                bge     .char_done              ; Skip if > ~

                lsl.l   #3,d6                   ; *8 bytes per char
                add.l   d6,a0                   ; a0 = font char data

                ; Calculate VRAM destination
                lea     VGA_VRAM,a1
                move.l  d5,d0
                mulu.w  #SCR_W,d0
                add.l   d4,d0
                add.l   d0,a1                   ; a1 = VRAM dest pointer

                ; Draw 8 rows
                moveq   #7,d7

.row_loop:
                move.b  (a0)+,d0                ; Get row bitmap
                moveq   #7,d3                   ; 8 pixels per row

.col_loop:
                btst    d3,d0                   ; Test bit
                beq.s   .skip_pixel

                ; Draw colored pixel based on position
                move.l  scroll_angle,d1
                add.l   d7,d1
                add.l   d3,d1
                andi.l  #$3F,d1
                addi.l  #64,d1                  ; Use colors 64-127
                move.b  d1,(a1)

.skip_pixel:
                addq.l  #1,a1
                dbf     d3,.col_loop

                ; Next VRAM row
                lea     SCR_W-8(a1),a1
                dbf     d7,.row_loop

.char_done:
                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; Calculate Rotation Matrix Components
; ============================================================================
calc_rotation:
                movem.l d0-d1/a0,-(sp)
                lea     sin_table,a0

                move.l  angle_x,d0
                andi.l  #$FF,d0
                add.w   d0,d0
                move.w  (a0,d0.w),sin_x
                move.l  angle_x,d0
                addi.l  #64,d0
                andi.l  #$FF,d0
                add.w   d0,d0
                move.w  (a0,d0.w),cos_x

                move.l  angle_y,d0
                andi.l  #$FF,d0
                add.w   d0,d0
                move.w  (a0,d0.w),sin_y
                move.l  angle_y,d0
                addi.l  #64,d0
                andi.l  #$FF,d0
                add.w   d0,d0
                move.w  (a0,d0.w),cos_y

                movem.l (sp)+,d0-d1/a0
                rts

; ============================================================================
; Transform and Project All Vertices
; ============================================================================
transform_vertices:
                movem.l d0-d7/a0-a2,-(sp)

                lea     cube_vertices,a0
                lea     projected_x,a1
                lea     projected_y,a2
                moveq   #NUM_VERTICES-1,d7

.vertex_loop:
                move.w  (a0)+,d0
                move.w  (a0)+,d1
                move.w  (a0)+,d2
                ext.l   d0
                ext.l   d1
                ext.l   d2

                ; Y-axis rotation
                move.l  d0,d3
                move.l  d2,d4
                move.w  cos_y,d5
                ext.l   d5
                muls.w  d5,d3
                move.w  sin_y,d5
                ext.l   d5
                muls.w  d5,d4
                sub.l   d4,d3
                asr.l   #FP_SHIFT,d3

                move.l  d0,d4
                move.l  d2,d6
                move.w  sin_y,d5
                ext.l   d5
                muls.w  d5,d4
                move.w  cos_y,d5
                ext.l   d5
                muls.w  d5,d6
                add.l   d4,d6
                asr.l   #FP_SHIFT,d6

                move.l  d3,d0
                move.l  d6,d2

                ; X-axis rotation
                move.l  d1,d3
                move.l  d2,d4
                move.w  cos_x,d5
                ext.l   d5
                muls.w  d5,d3
                move.w  sin_x,d5
                ext.l   d5
                muls.w  d5,d4
                sub.l   d4,d3
                asr.l   #FP_SHIFT,d3

                move.l  d1,d4
                move.l  d2,d6
                move.w  sin_x,d5
                ext.l   d5
                muls.w  d5,d4
                move.w  cos_x,d5
                ext.l   d5
                muls.w  d5,d6
                add.l   d4,d6
                asr.l   #FP_SHIFT,d6

                move.l  d3,d1
                move.l  d6,d2

                ; Perspective projection
                add.l   #DISTANCE,d2
                beq.s   .skip_div
                bmi.s   .skip_div

                move.l  d0,d3
                muls.w  #DISTANCE,d3
                divs.w  d2,d3
                ext.l   d3
                add.l   #CENTER_X,d3
                move.w  d3,(a1)+

                move.l  d1,d3
                muls.w  #DISTANCE,d3
                divs.w  d2,d3
                ext.l   d3
                add.l   #CENTER_Y,d3
                move.w  d3,(a2)+

                dbf     d7,.vertex_loop
                bra.s   .done

.skip_div:
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
                move.b  (a0)+,d0
                move.b  (a0)+,d1
                ext.w   d0
                ext.w   d1

                lea     projected_x,a1
                lea     projected_y,a2

                move.w  d0,d2
                add.w   d2,d2
                move.w  (a1,d2.w),d3
                move.w  (a2,d2.w),d4

                move.w  d1,d2
                add.w   d2,d2
                move.w  (a1,d2.w),d5
                move.w  (a2,d2.w),d6

                ; Calculate edge color based on index (for variety)
                move.l  d7,d2
                andi.l  #7,d2
                add.l   #40,d2          ; Colors 40-47 (greens in default palette)
                lsl.l   #4,d2           ; Brighter colors
                addi.l  #15,d2
                andi.l  #$FF,d2

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
                move.w  d5,d0
                sub.w   d3,d0
                move.w  d6,d1
                sub.w   d4,d1

                moveq   #1,d7
                tst.w   d0
                bge.s   .dx_pos
                neg.w   d0
                moveq   #-1,d7
.dx_pos:
                move.w  d7,-(sp)

                moveq   #1,d7
                tst.w   d1
                bge.s   .dy_pos
                neg.w   d1
                moveq   #-1,d7
.dy_pos:
                move.w  d7,-(sp)

                cmp.w   d1,d0
                bge.s   .x_major

                move.w  d1,d7
                lsr.w   #1,d7

.y_loop:
                bsr     plot_pixel
                cmp.w   d6,d4
                beq.s   .line_done_pop
                add.w   (sp),d4
                sub.w   d0,d7
                bge.s   .y_loop
                add.w   d1,d7
                add.w   2(sp),d3
                bra.s   .y_loop

.x_major:
                move.w  d0,d7
                lsr.w   #1,d7

.x_loop:
                bsr     plot_pixel
                cmp.w   d5,d3
                beq.s   .line_done_pop
                add.w   2(sp),d3
                sub.w   d1,d7
                bge.s   .x_loop
                add.w   d0,d7
                add.w   (sp),d4
                bra.s   .x_loop

.line_done_pop:
                addq.l  #4,sp
.line_done:
                movem.l (sp)+,d0-d7/a0
                rts

; ============================================================================
; Plot Pixel with Clipping
; Input: d3=x, d4=y, d2=color
; ============================================================================
plot_pixel:
                tst.w   d3
                bmi.s   .skip
                cmpi.w  #SCR_W,d3
                bge.s   .skip
                tst.w   d4
                bmi.s   .skip
                cmpi.w  #SCR_H,d4
                bge.s   .skip

                movem.l d0-d1/a0,-(sp)
                lea     VGA_VRAM,a0
                move.w  d4,d0
                mulu    #SCR_W,d0
                add.w   d3,d0
                move.b  d2,(a0,d0.l)
                movem.l (sp)+,d0-d1/a0
.skip:
                rts

; ============================================================================
; Data Section
; ============================================================================

                even

; --- Variables ---
angle_x:        dc.l    0
angle_y:        dc.l    0
sin_x:          dc.w    0
cos_x:          dc.w    0
sin_y:          dc.w    0
cos_y:          dc.w    0
scroll_angle:   dc.l    0
scroll_char_offset: dc.l 0

; --- Cube Vertices ---
cube_vertices:
                dc.w    -CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE
                dc.w     CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE
                dc.w     CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE
                dc.w    -CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE
                dc.w    -CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE
                dc.w     CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE
                dc.w     CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE
                dc.w    -CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE

; --- Edge List ---
edge_list:
                dc.b    0, 1
                dc.b    1, 2
                dc.b    2, 3
                dc.b    3, 0
                dc.b    4, 5
                dc.b    5, 6
                dc.b    6, 7
                dc.b    7, 4
                dc.b    0, 4
                dc.b    1, 5
                dc.b    2, 6
                dc.b    3, 7

                even

; --- Projected Coordinates ---
projected_x:    ds.w    NUM_VERTICES
projected_y:    ds.w    NUM_VERTICES

; --- Sin Table (256 entries, 8.8 fixed point) ---
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

; ============================================================================
; Scrolltext Data
; ============================================================================

                even

; Simple 8x8 bitmap font (ASCII 32-126, 8 bytes per character)
; Each byte is a row of 8 pixels, MSB is leftmost pixel
simple_font:
; Space (32)
                dc.b    $00,$00,$00,$00,$00,$00,$00,$00
; ! (33)
                dc.b    $18,$18,$18,$18,$18,$00,$18,$00
; " (34)
                dc.b    $6C,$6C,$6C,$00,$00,$00,$00,$00
; # (35)
                dc.b    $6C,$6C,$FE,$6C,$FE,$6C,$6C,$00
; $ (36)
                dc.b    $18,$3E,$60,$3C,$06,$7C,$18,$00
; % (37)
                dc.b    $00,$C6,$CC,$18,$30,$66,$C6,$00
; & (38)
                dc.b    $38,$6C,$38,$76,$DC,$CC,$76,$00
; ' (39)
                dc.b    $18,$18,$30,$00,$00,$00,$00,$00
; ( (40)
                dc.b    $0C,$18,$30,$30,$30,$18,$0C,$00
; ) (41)
                dc.b    $30,$18,$0C,$0C,$0C,$18,$30,$00
; * (42)
                dc.b    $00,$66,$3C,$FF,$3C,$66,$00,$00
; + (43)
                dc.b    $00,$18,$18,$7E,$18,$18,$00,$00
; , (44)
                dc.b    $00,$00,$00,$00,$00,$18,$18,$30
; - (45)
                dc.b    $00,$00,$00,$7E,$00,$00,$00,$00
; . (46)
                dc.b    $00,$00,$00,$00,$00,$18,$18,$00
; / (47)
                dc.b    $06,$0C,$18,$30,$60,$C0,$80,$00
; 0 (48)
                dc.b    $7C,$C6,$CE,$DE,$F6,$E6,$7C,$00
; 1 (49)
                dc.b    $18,$38,$18,$18,$18,$18,$7E,$00
; 2 (50)
                dc.b    $7C,$C6,$06,$1C,$30,$66,$FE,$00
; 3 (51)
                dc.b    $7C,$C6,$06,$3C,$06,$C6,$7C,$00
; 4 (52)
                dc.b    $1C,$3C,$6C,$CC,$FE,$0C,$1E,$00
; 5 (53)
                dc.b    $FE,$C0,$FC,$06,$06,$C6,$7C,$00
; 6 (54)
                dc.b    $38,$60,$C0,$FC,$C6,$C6,$7C,$00
; 7 (55)
                dc.b    $FE,$C6,$0C,$18,$30,$30,$30,$00
; 8 (56)
                dc.b    $7C,$C6,$C6,$7C,$C6,$C6,$7C,$00
; 9 (57)
                dc.b    $7C,$C6,$C6,$7E,$06,$0C,$78,$00
; : (58)
                dc.b    $00,$18,$18,$00,$00,$18,$18,$00
; ; (59)
                dc.b    $00,$18,$18,$00,$00,$18,$18,$30
; < (60)
                dc.b    $0C,$18,$30,$60,$30,$18,$0C,$00
; = (61)
                dc.b    $00,$00,$7E,$00,$00,$7E,$00,$00
; > (62)
                dc.b    $30,$18,$0C,$06,$0C,$18,$30,$00
; ? (63)
                dc.b    $7C,$C6,$0C,$18,$18,$00,$18,$00
; @ (64)
                dc.b    $7C,$C6,$DE,$DE,$DE,$C0,$78,$00
; A (65)
                dc.b    $38,$6C,$C6,$FE,$C6,$C6,$C6,$00
; B (66)
                dc.b    $FC,$66,$66,$7C,$66,$66,$FC,$00
; C (67)
                dc.b    $3C,$66,$C0,$C0,$C0,$66,$3C,$00
; D (68)
                dc.b    $F8,$6C,$66,$66,$66,$6C,$F8,$00
; E (69)
                dc.b    $FE,$62,$68,$78,$68,$62,$FE,$00
; F (70)
                dc.b    $FE,$62,$68,$78,$68,$60,$F0,$00
; G (71)
                dc.b    $3C,$66,$C0,$C0,$CE,$66,$3A,$00
; H (72)
                dc.b    $C6,$C6,$C6,$FE,$C6,$C6,$C6,$00
; I (73)
                dc.b    $3C,$18,$18,$18,$18,$18,$3C,$00
; J (74)
                dc.b    $1E,$0C,$0C,$0C,$CC,$CC,$78,$00
; K (75)
                dc.b    $E6,$66,$6C,$78,$6C,$66,$E6,$00
; L (76)
                dc.b    $F0,$60,$60,$60,$62,$66,$FE,$00
; M (77)
                dc.b    $C6,$EE,$FE,$FE,$D6,$C6,$C6,$00
; N (78)
                dc.b    $C6,$E6,$F6,$DE,$CE,$C6,$C6,$00
; O (79)
                dc.b    $7C,$C6,$C6,$C6,$C6,$C6,$7C,$00
; P (80)
                dc.b    $FC,$66,$66,$7C,$60,$60,$F0,$00
; Q (81)
                dc.b    $7C,$C6,$C6,$C6,$D6,$DE,$7C,$06
; R (82)
                dc.b    $FC,$66,$66,$7C,$6C,$66,$E6,$00
; S (83)
                dc.b    $7C,$C6,$60,$38,$0C,$C6,$7C,$00
; T (84)
                dc.b    $7E,$5A,$18,$18,$18,$18,$3C,$00
; U (85)
                dc.b    $C6,$C6,$C6,$C6,$C6,$C6,$7C,$00
; V (86)
                dc.b    $C6,$C6,$C6,$C6,$6C,$38,$10,$00
; W (87)
                dc.b    $C6,$C6,$D6,$FE,$FE,$EE,$C6,$00
; X (88)
                dc.b    $C6,$C6,$6C,$38,$6C,$C6,$C6,$00
; Y (89)
                dc.b    $66,$66,$66,$3C,$18,$18,$3C,$00
; Z (90)
                dc.b    $FE,$C6,$8C,$18,$32,$66,$FE,$00
; [ (91)
                dc.b    $3C,$30,$30,$30,$30,$30,$3C,$00
; \ (92)
                dc.b    $C0,$60,$30,$18,$0C,$06,$02,$00
; ] (93)
                dc.b    $3C,$0C,$0C,$0C,$0C,$0C,$3C,$00
; ^ (94)
                dc.b    $10,$38,$6C,$C6,$00,$00,$00,$00
; _ (95)
                dc.b    $00,$00,$00,$00,$00,$00,$00,$FE
; ` (96)
                dc.b    $30,$18,$0C,$00,$00,$00,$00,$00
; a (97)
                dc.b    $00,$00,$78,$0C,$7C,$CC,$76,$00
; b (98)
                dc.b    $E0,$60,$7C,$66,$66,$66,$DC,$00
; c (99)
                dc.b    $00,$00,$7C,$C6,$C0,$C6,$7C,$00
; d (100)
                dc.b    $1C,$0C,$7C,$CC,$CC,$CC,$76,$00
; e (101)
                dc.b    $00,$00,$7C,$C6,$FE,$C0,$7C,$00
; f (102)
                dc.b    $38,$6C,$60,$F8,$60,$60,$F0,$00
; g (103)
                dc.b    $00,$00,$76,$CC,$CC,$7C,$0C,$F8
; h (104)
                dc.b    $E0,$60,$6C,$76,$66,$66,$E6,$00
; i (105)
                dc.b    $18,$00,$38,$18,$18,$18,$3C,$00
; j (106)
                dc.b    $06,$00,$0E,$06,$06,$66,$66,$3C
; k (107)
                dc.b    $E0,$60,$66,$6C,$78,$6C,$E6,$00
; l (108)
                dc.b    $38,$18,$18,$18,$18,$18,$3C,$00
; m (109)
                dc.b    $00,$00,$EC,$FE,$D6,$D6,$C6,$00
; n (110)
                dc.b    $00,$00,$DC,$66,$66,$66,$66,$00
; o (111)
                dc.b    $00,$00,$7C,$C6,$C6,$C6,$7C,$00
; p (112)
                dc.b    $00,$00,$DC,$66,$66,$7C,$60,$F0
; q (113)
                dc.b    $00,$00,$76,$CC,$CC,$7C,$0C,$1E
; r (114)
                dc.b    $00,$00,$DC,$76,$60,$60,$F0,$00
; s (115)
                dc.b    $00,$00,$7E,$C0,$7C,$06,$FC,$00
; t (116)
                dc.b    $30,$30,$FC,$30,$30,$34,$18,$00
; u (117)
                dc.b    $00,$00,$CC,$CC,$CC,$CC,$76,$00
; v (118)
                dc.b    $00,$00,$C6,$C6,$C6,$6C,$38,$00
; w (119)
                dc.b    $00,$00,$C6,$D6,$D6,$FE,$6C,$00
; x (120)
                dc.b    $00,$00,$C6,$6C,$38,$6C,$C6,$00
; y (121)
                dc.b    $00,$00,$C6,$C6,$C6,$7E,$06,$FC
; z (122)
                dc.b    $00,$00,$FE,$8C,$18,$32,$FE,$00
; { (123)
                dc.b    $0E,$18,$18,$70,$18,$18,$0E,$00
; | (124)
                dc.b    $18,$18,$18,$18,$18,$18,$18,$00
; } (125)
                dc.b    $70,$18,$18,$0E,$18,$18,$70,$00
; ~ (126)
                dc.b    $76,$DC,$00,$00,$00,$00,$00,$00

                even

; Scroll message
scroll_message:
                dc.b    "   INTUITION ENGINE ... VGA MODE 13H ROTATING CUBE WITH 360 DEGREE CIRCULAR SCROLLER ... "
                dc.b    "EDGE OF DISGRACE BY BOOZE DESIGN PLAYING ON THE SID EMULATOR ... "
                dc.b    "GREETS TO ALL DEMOSCENE CODERS ... "
                dc.b    "   "
scroll_msg_end:
                dc.b    0

                even

; ============================================================================
; SID Music Data
; ============================================================================
sid_data:
                incbin  "../testdata/sid/Edge_of_Disgrace.sid"
sid_data_end:

                end
