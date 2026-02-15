; ============================================================================
; ROTATING CUBE + COPPER RAINBOW - 3D WIREFRAME WITH RASTER BAR BACKGROUND
; M68020 assembly for IntuitionEngine - VGA 320x200, copper coprocessor
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Assembler:    vasmm68k_mot -Fbin -m68020 -devpac -o rotating_cube_copper_68k.ie68 rotating_cube_copper_68k.asm
; Run:          IntuitionEngine -m68k rotating_cube_copper_68k.ie68
; Video mode:   VGA Mode 13h (320x200, 256 colours, linear framebuffer)
; Audio:        SID music via embedded 6502 CPU core
; Copper:       100-line animated rainbow gradient behind the scene
; CPU:          Motorola 68020 (32-bit, big-endian)
; Include:      ie68.inc (hardware register and macro definitions)
;
; === WHAT THIS DEMO DOES ===
; Combines four classic demoscene effects into a single production:
;   1. A wireframe 3D cube rotating in real time on X and Y axes
;   2. A circular scrolltext orbiting the screen centre like a clock face
;   3. Animated rainbow raster bars using the copper coprocessor
;   4. SID music ("Edge of Disgrace" by Goto80) via the 6502 core
;
; === WHY 3D WIREFRAME RENDERING ===
; Real-time 3D wireframe rendering was one of the most impressive feats of
; early computer graphics. Elite (1984) on the BBC Micro rendered an entire
; 3D universe with just a 2 MHz 6502. The technique uses:
;
;   - Pre-computed sine/cosine lookup tables (avoiding slow trig functions)
;   - Fixed-point arithmetic (8.8 format) for fractional precision without FPU
;   - 3x3 rotation matrices decomposed into sequential Y-axis then X-axis turns
;   - Perspective projection: screen_x = (x * focal_length) / z + centre_x
;   - Line drawing via Bresenham's algorithm (integer arithmetic only)
;
; === WHY COPPER RASTER BARS ===
; On the Amiga (1985), the "copper" coprocessor could change hardware
; registers synchronised to the electron beam position. This enabled
; effects impossible with normal VGA hardware -- changing the background
; colour on every scanline to create smooth gradients and "raster bars."
;
; The Intuition Engine emulates this capability. Our copper program runs
; in parallel with the CPU, issuing WAIT/MOVE commands that modify palette
; entry #1 at specific scanlines. Since the screen is filled with colour
; index 1, different scanlines display different colours -- creating the
; animated rainbow tube effect.
;
; The colour calculation uses phase-shifted sine waves for RGB components
; (red = sin(hue), green = sin(hue + 120 degrees), blue = sin(hue + 240 degrees)),
; modulated by a brightness sine wave that creates the illusion of a 3D
; tube surface.
;
; === M68K-SPECIFIC NOTES ===
; - The IE VideoChip must be enabled (VIDEO_CTRL=1) for the copper to work,
;   even when using VGA for display -- a common gotcha
; - The copper list is built dynamically and updated each frame
; - Copper list memory layout: line 0 has no WAIT (36 bytes), subsequent
;   lines include WAIT + SETBASE overhead (44 bytes each)
; - The screen is filled with colour index 1 (not 0!) so the copper's
;   palette changes are visible
;
; === ARCHITECTURE OVERVIEW ===
;
;   M68020 (main CPU)              Copper (runs in parallel)
;   +--------------------+        +------------------------+
;   | Init VGA + copper  |        |                        |
;   | Start SID music    |        | For each scanline:     |
;   |                    |        |   WAIT for beam pos    |
;   | Main loop:         |        |   SETBASE VGA_DAC      |
;   |   Wait vsync       |        |   MOVE palette idx 1   |
;   |   Clear to idx 1   |        |   MOVE R, G, B values  |
;   |   Draw scroller    |        |                        |
;   |   Rotate + project |        | (restarted each vsync) |
;   |   Draw cube edges  |        +------------------------+
;   |   Update copper    |
;   +--------------------+        6502 (audio coprocessor)
;                                 +------------------------+
;                                 | Execute SID player     |
;                                 | Write SID regs -> synth|
;                                 +------------------------+
;
; === MEMORY MAP ===
;   $001000        Code entry point (PROGRAM_START)
;   $FF0000        Stack top (STACK_TOP, grows downward)
;   VGA_VRAM       Linear framebuffer (64000 bytes)
;   copper_list    Dynamic copper program (~4800 bytes)
;   sid_data       Embedded .SID file (6502 code + music data)
;
; === BUILD AND RUN ===
;   vasmm68k_mot -Fbin -m68020 -devpac -o rotating_cube_copper_68k.ie68 \
;       sdk/examples/asm/rotating_cube_copper_68k.asm
;   IntuitionEngine -m68k rotating_cube_copper_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

                include "ie68.inc"

                org     PROGRAM_START

; ----------------------------------------------------------------------------
; Screen geometry -- Mode 13h constants
; ----------------------------------------------------------------------------
SCR_W           equ     320
SCR_H           equ     200
CENTER_X        equ     160
CENTER_Y        equ     100

; ----------------------------------------------------------------------------
; 3D cube parameters
; ----------------------------------------------------------------------------
NUM_VERTICES    equ     8
NUM_EDGES       equ     12
CUBE_SIZE       equ     40              ; Smaller cube to leave room for scroller
DISTANCE        equ     256             ; Perspective focal length

; ----------------------------------------------------------------------------
; Fixed-point format (8.8)
; ----------------------------------------------------------------------------
FP_SHIFT        equ     8
FP_ONE          equ     256

; ----------------------------------------------------------------------------
; Circular scroller parameters
; Characters orbit around screen centre like numbers on a clock face.
; Polar coordinates (angle + radius) are converted to Cartesian (x, y).
; ----------------------------------------------------------------------------
SCROLL_RADIUS   equ     75              ; Distance from centre to text
SCROLL_CENTER_X equ     160
SCROLL_CENTER_Y equ     100
NUM_VISIBLE     equ     12              ; Characters visible at once
CHAR_SPACING    equ     21              ; Angular spacing (256/12 ~ 21)
DRAW_CHAR_SIZE  equ     8               ; 8x8 pixel font
SCROLL_SPEED    equ     1               ; Rotation speed per frame

; ----------------------------------------------------------------------------
; Copper raster bar configuration
; 100 colour changes x 2 scanlines each = 200 lines (full screen coverage)
; ----------------------------------------------------------------------------
NUM_COPPER_LINES equ    100
LINES_PER_CHANGE equ    2

; ============================================================================
; ENTRY POINT - HARDWARE INITIALISATION
; ============================================================================
start:
                move.l  #STACK_TOP,sp

                ; --- Enable IE VideoChip (required for copper coprocessor) ---
                ; WHY: The copper is part of the IE video system, not the VGA.
                ; Without this, copper instructions silently do nothing.
                move.l  #1,VIDEO_CTRL

                ; --- Configure VGA ---
                move.b  #VGA_MODE_13H,VGA_MODE
                move.b  #VGA_CTRL_ENABLE,VGA_CTRL

                ; --- Build and activate copper program ---
                bsr     build_copper_list
                move.l  #copper_list,COPPER_PTR
                move.l  #1,COPPER_CTRL

                ; --- Start SID music ---
                move.l  #sid_data,SID_PLAY_PTR
                move.l  #sid_data_end-sid_data,SID_PLAY_LEN
                start_sid_loop

                ; --- Initialise animation state ---
                clr.l   angle_x
                clr.l   angle_y
                clr.l   scroll_angle
                clr.l   scroll_char_offset
                clr.l   raster_phase

; ============================================================================
; MAIN LOOP - ONE ITERATION PER FRAME
; ============================================================================
; Order matters: clear first, draw scroller (behind), then cube (on top),
; then update copper colours for the next frame's raster bars.
; ----------------------------------------------------------------------------
main_loop:
                ; --- VSync synchronisation (two-stage wait) ---
.wait_not_vb:   move.b  VGA_STATUS,d0
                andi.b  #VGA_STATUS_VSYNC,d0
                bne.s   .wait_not_vb

.wait_vb:       move.b  VGA_STATUS,d0
                andi.b  #VGA_STATUS_VSYNC,d0
                beq.s   .wait_vb

                ; --- Clear screen to colour index 1 ---
                ; WHY index 1: The copper modifies palette entry 1 per scanline.
                ; Pixels with index 1 show the copper's current colour, creating
                ; the rainbow gradient. Index 0 would not be affected.
                bsr     clear_screen

                ; --- Draw circular scroller (behind cube) ---
                bsr     draw_circular_scroll

                ; --- 3D cube rendering pipeline ---
                bsr     calc_rotation
                bsr     transform_vertices
                bsr     draw_edges

                ; --- Advance cube rotation ---
                addq.l  #2,angle_x
                addq.l  #3,angle_y
                andi.l  #$FF,angle_x
                andi.l  #$FF,angle_y

                ; --- Advance scroller rotation ---
                move.l  scroll_angle,d0
                addq.l  #SCROLL_SPEED,d0
                andi.l  #$FF,d0
                move.l  d0,scroll_angle

                ; --- Advance copper colour animation ---
                ; The phase offset makes colours scroll vertically through the bars
                move.l  raster_phase,d0
                addq.l  #3,d0
                andi.l  #$FF,d0
                move.l  d0,raster_phase
                bsr     update_copper_colors

                bra     main_loop

; ============================================================================
; CLEAR SCREEN TO COLOUR INDEX 1
; ============================================================================
; WHY UNROLLED: Writing 32 bytes per iteration reduces loop overhead by 32x.
; 64000 bytes / 32 bytes per iteration = 2000 iterations.
; ----------------------------------------------------------------------------
clear_screen:
                movem.l d0/a0,-(sp)
                lea     VGA_VRAM,a0
                move.w  #2000-1,d0

.clear_loop:
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                move.b  #1,(a0)+
                dbf     d0,.clear_loop

                movem.l (sp)+,d0/a0
                rts

; ============================================================================
; DRAW CIRCULAR SCROLLER
; ============================================================================
; WHY: Characters orbit the screen centre using polar-to-Cartesian conversion:
;   x = centre_x + radius * cos(angle)
;   y = centre_y + radius * sin(angle)
; Each of the 12 visible characters is offset by CHAR_SPACING (21 angle units)
; from its neighbour, evenly distributing them around the circle.
; ----------------------------------------------------------------------------
draw_circular_scroll:
                movem.l d0-d7/a0-a4,-(sp)

                moveq   #0,d7                   ; Visible character index (0..11)

.char_loop:
                ; --- Calculate angular position for this character ---
                move.l  scroll_angle,d0
                move.l  d7,d1
                mulu.w  #CHAR_SPACING,d1
                add.l   d1,d0
                andi.l  #$FF,d0                 ; Wrap to full circle

                ; --- Look up sin and cos from table ---
                lea     sin_table,a0
                move.l  d0,d1
                add.w   d1,d1                   ; *2 for word access
                move.w  (a0,d1.w),d2            ; sin(angle)

                move.l  d0,d1
                addi.l  #64,d1                  ; cos = sin(angle + 64)
                andi.l  #$FF,d1
                add.w   d1,d1
                move.w  (a0,d1.w),d3            ; cos(angle)

                ; --- Convert polar to Cartesian ---
                move.w  d3,d4
                ext.l   d4
                muls.w  #SCROLL_RADIUS,d4
                asr.l   #FP_SHIFT,d4
                addi.l  #SCROLL_CENTER_X,d4
                subi.l  #DRAW_CHAR_SIZE/2,d4    ; Centre character on point

                move.w  d2,d5
                ext.l   d5
                muls.w  #SCROLL_RADIUS,d5
                asr.l   #FP_SHIFT,d5
                addi.l  #SCROLL_CENTER_Y,d5
                subi.l  #DRAW_CHAR_SIZE/2,d5

                ; --- Get character from message (with wrapping) ---
                move.l  scroll_char_offset,d6
                add.l   d7,d6

                lea     scroll_message,a1
                lea     scroll_msg_end,a2
                move.l  a2,d0
                sub.l   a1,d0
                subq.l  #1,d0                   ; Message length (excluding null)

.wrap_check:
                cmp.l   d0,d6
                blt.s   .no_wrap
                sub.l   d0,d6
                bra.s   .wrap_check
.no_wrap:
                move.b  (a1,d6.l),d6
                andi.l  #$7F,d6                 ; Mask to 7-bit ASCII

                ; --- Draw the character ---
                bsr     draw_scroll_char

                ; --- Next visible character ---
                addq.l  #1,d7
                cmpi.l  #NUM_VISIBLE,d7
                blt     .char_loop

                ; --- Advance message offset periodically ---
                ; Every 32 angle steps, shift the message by one character
                move.l  scroll_angle,d0
                andi.l  #$1F,d0
                bne.s   .no_advance

                lea     scroll_message,a1
                lea     scroll_msg_end,a2
                move.l  a2,d0
                sub.l   a1,d0
                subq.l  #1,d0

                move.l  scroll_char_offset,d1
                addq.l  #1,d1
                cmp.l   d0,d1
                blt.s   .no_wrap_offset
                moveq   #0,d1
.no_wrap_offset:
                move.l  d1,scroll_char_offset
.no_advance:

                movem.l (sp)+,d0-d7/a0-a4
                rts

; ============================================================================
; DRAW SCROLL CHARACTER (8x8 BITMAP FONT)
; ============================================================================
; Input: d4=x, d5=y, d6=ASCII code
; Renders a single character from the bitmap font with animated colour
; based on scroll_angle and pixel position.
; ----------------------------------------------------------------------------
draw_scroll_char:
                movem.l d0-d7/a0-a2,-(sp)

                ; --- Bounds check ---
                tst.l   d4
                bmi     .char_done
                cmpi.l  #SCR_W-8,d4
                bgt     .char_done
                tst.l   d5
                bmi     .char_done
                cmpi.l  #SCR_H-8,d5
                bgt     .char_done

                ; --- Look up character in font ---
                lea     simple_font,a0
                subi.l  #32,d6                  ; ASCII space = font index 0
                bmi     .char_done
                cmpi.l  #95,d6                  ; Font covers ASCII 32-126
                bge     .char_done

                lsl.l   #3,d6                   ; *8 bytes per character
                add.l   d6,a0

                ; --- Calculate VRAM destination ---
                lea     VGA_VRAM,a1
                move.l  d5,d0
                mulu.w  #SCR_W,d0
                add.l   d4,d0
                add.l   d0,a1

                ; --- Render 8 rows of 8 pixels ---
                moveq   #7,d7

.row_loop:
                move.b  (a0)+,d0                ; Row bitmap (8 pixels as bits)
                moveq   #7,d3

.col_loop:
                btst    d3,d0                   ; Test pixel bit
                beq.s   .skip_pixel

                ; Animated colour based on position and time
                move.l  scroll_angle,d1
                add.l   d7,d1
                add.l   d3,d1
                andi.l  #$3F,d1
                addi.l  #64,d1                  ; Use palette entries 64-127
                move.b  d1,(a1)

.skip_pixel:
                addq.l  #1,a1
                dbf     d3,.col_loop

                ; Advance to next VRAM row (320 - 8 pixels already advanced)
                lea     SCR_W-8(a1),a1
                dbf     d7,.row_loop

.char_done:
                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; CALCULATE ROTATION MATRIX COMPONENTS
; ============================================================================
; Pre-computes sin/cos for both rotation angles from the lookup table.
; ----------------------------------------------------------------------------
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
; TRANSFORM AND PROJECT ALL VERTICES
; ============================================================================
; Applies Y-rotation, X-rotation, then perspective projection to each vertex.
; See rotating_cube_68k.asm for detailed mathematical commentary on the
; rotation matrix decomposition and fixed-point arithmetic.
; ----------------------------------------------------------------------------
transform_vertices:
                movem.l d0-d7/a0-a2,-(sp)

                lea     cube_vertices,a0
                lea     projected_x,a1
                lea     projected_y,a2
                moveq   #NUM_VERTICES-1,d7

.vertex_loop:
                move.w  (a0)+,d0                ; x
                move.w  (a0)+,d1                ; y
                move.w  (a0)+,d2                ; z
                ext.l   d0
                ext.l   d1
                ext.l   d2

                ; --- Y-axis rotation ---
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

                ; --- X-axis rotation ---
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

                ; --- Perspective projection ---
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
; DRAW ALL EDGES
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

                ; Edge colour varies by index
                move.l  d7,d2
                andi.l  #7,d2
                add.l   #40,d2
                lsl.l   #4,d2
                addi.l  #15,d2
                andi.l  #$FF,d2

                bsr     draw_line

                dbf     d7,.edge_loop

                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; DRAW LINE - BRESENHAM'S ALGORITHM
; ============================================================================
; Input: d3=x1, d4=y1, d5=x2, d6=y2, d2=colour
; Uses fast-reject visibility test plus per-pixel bounds checking.
; ----------------------------------------------------------------------------
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
; PLOT PIXEL WITH BOUNDS CHECKING
; ============================================================================
; Input: d3=x, d4=y, d2=colour
; ----------------------------------------------------------------------------
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
; BUILD COPPER LIST
; ============================================================================
; WHY: Creates the copper program that executes during each frame. For each
; of 100 scanlines, the copper WAITs for the beam to reach that position,
; then writes new R/G/B values into palette entry 1. The actual colour
; values are placeholders here -- update_copper_colors fills them in each
; frame for smooth animation.
;
; Copper instructions used:
;   SETBASE  - Select target device (VIDEO for WAIT, VGA_DAC for palette)
;   WAIT     - Pause until beam reaches specified scanline
;   MOVE     - Write a value to the selected device's register
;   END      - Stop execution (restarted automatically at next vsync)
; ----------------------------------------------------------------------------
build_copper_list:
                movem.l d0-d7/a0,-(sp)

                lea     copper_list,a0
                moveq   #0,d7

.build_loop:
                move.l  d7,d0
                mulu.w  #LINES_PER_CHANGE,d0

                cmpi.l  #SCR_H,d0
                bge.s   .build_end

                ; First line needs no WAIT (starts at top of screen)
                tst.l   d7
                beq.s   .no_wait

                ; WAIT for beam to reach this scanline
                move.l  #COP_SETBASE_VIDEO,(a0)+
                lsl.l   #8,d0
                lsl.l   #4,d0                   ; scanline * 4096
                move.l  d0,(a0)+

.no_wait:
                ; Write palette entry 1 with new R/G/B
                move.l  #COP_SETBASE_VGA_DAC,(a0)+
                move.l  #COP_MOVE_VGA_WINDEX,(a0)+
                move.l  #1,(a0)+                ; Palette index 1
                move.l  #COP_MOVE_VGA_DATA,(a0)+
                move.l  #0,(a0)+                ; R (placeholder)
                move.l  #COP_MOVE_VGA_DATA,(a0)+
                move.l  #0,(a0)+                ; G (placeholder)
                move.l  #COP_MOVE_VGA_DATA,(a0)+
                move.l  #0,(a0)+                ; B (placeholder)

                addq.l  #1,d7
                cmpi.l  #NUM_COPPER_LINES,d7
                blt.s   .build_loop

.build_end:
                move.l  #COP_END,(a0)+

                movem.l (sp)+,d0-d7/a0
                rts

; ============================================================================
; UPDATE COPPER COLOURS
; ============================================================================
; WHY: Modifies the R/G/B values in the copper list each frame to animate
; the rainbow gradient. The effect creates the illusion of a 3D "tube"
; surface using:
;
;   BRIGHTNESS: A sine wave modulated by scanline position, creating bright
;               centres and dark edges for each band
;   HUE:        Determined by colour group + animation phase, scrolling
;               colours through the rainbow over time
;   RGB:        Phase-shifted sine waves approximate HSV-to-RGB conversion:
;               Red = sin(hue), Green = sin(hue + 120 deg), Blue = sin(hue + 240 deg)
;
; Memory layout of copper list:
;   Line 0:  36 bytes (no WAIT instruction)
;   Line 1+: 44 bytes each (includes WAIT + SETBASE overhead)
;   R value offset within a line's palette block: varies by line index
; ----------------------------------------------------------------------------
update_copper_colors:
                movem.l d0-d7/a0-a2,-(sp)

                lea     copper_list,a0
                lea     sin_table,a1
                move.l  raster_phase,d6
                moveq   #0,d7

.update_loop:
                ; --- Calculate pointer to this line's R value ---
                tst.l   d7
                bne.s   .not_line0

                ; Line 0: R at offset 16 (no WAIT)
                lea     16(a0),a2
                bra.s   .calc_color

.not_line0:
                ; Lines 1+: offset = 36 + (line-1)*44 + 24
                move.l  d7,d0
                subq.l  #1,d0
                mulu.w  #44,d0
                addi.l  #36+24,d0
                lea     (a0,d0.l),a2

.calc_color:
                ; --- Colour group (bands of similar colour) ---
                move.l  d7,d0
                lsr.l   #2,d0                   ; line / 4
                andi.l  #$1F,d0                 ; 32 groups

                ; --- Brightness (tube effect via sine wave) ---
                move.l  d7,d1
                lsl.l   #4,d1                   ; line * 16 (faster cycling)
                add.l   d6,d1                   ; Add animation phase
                andi.l  #$FF,d1

                move.l  d1,d2
                add.w   d2,d2                   ; *2 for word access
                move.w  (a1,d2.w),d3            ; sin value (-256..+256)
                bpl.s   .bright_pos
                neg.w   d3                      ; Absolute value for brightness
.bright_pos:

                ; --- Hue (colour group + animation phase) ---
                move.l  d0,d4
                lsl.l   #3,d4                   ; *8 for hue spread
                add.l   d6,d4                   ; Add animation phase
                andi.l  #$FF,d4

                ; --- Red: sin(hue) * brightness ---
                move.l  d4,d1
                add.w   d1,d1
                move.w  (a1,d1.w),d2
                bpl.s   .r_pos
                moveq   #0,d2
                bra.s   .r_scale
.r_pos:
                mulu.w  d3,d2
                lsr.l   #8,d2
                lsr.l   #2,d2                   ; Scale to 0-63 (VGA DAC range)
.r_scale:
                andi.l  #$3F,d2
                move.l  d2,(a2)

                ; --- Green: sin(hue + 120 deg) * brightness ---
                ; 120 deg ~ 85 in 256-unit system
                move.l  d4,d1
                addi.l  #85,d1
                andi.l  #$FF,d1
                add.w   d1,d1
                move.w  (a1,d1.w),d2
                bpl.s   .g_pos
                moveq   #0,d2
                bra.s   .g_scale
.g_pos:
                mulu.w  d3,d2
                lsr.l   #8,d2
                lsr.l   #2,d2
.g_scale:
                andi.l  #$3F,d2
                move.l  d2,8(a2)

                ; --- Blue: sin(hue + 240 deg) * brightness ---
                ; 240 deg ~ 170 in 256-unit system
                move.l  d4,d1
                addi.l  #170,d1
                andi.l  #$FF,d1
                add.w   d1,d1
                move.w  (a1,d1.w),d2
                bpl.s   .b_pos
                moveq   #0,d2
                bra.s   .b_scale
.b_pos:
                mulu.w  d3,d2
                lsr.l   #8,d2
                lsr.l   #2,d2
.b_scale:
                andi.l  #$3F,d2
                move.l  d2,16(a2)

                addq.l  #1,d7
                cmpi.l  #NUM_COPPER_LINES,d7
                blt     .update_loop

                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; DATA SECTION
; ============================================================================

                even

; --- Animation state variables ---
angle_x:        dc.l    0               ; Cube X rotation (0-255)
angle_y:        dc.l    0               ; Cube Y rotation (0-255)
sin_x:          dc.w    0               ; Cached sin(angle_x)
cos_x:          dc.w    0               ; Cached cos(angle_x)
sin_y:          dc.w    0               ; Cached sin(angle_y)
cos_y:          dc.w    0               ; Cached cos(angle_y)
scroll_angle:   dc.l    0               ; Scroller orbit position (0-255)
scroll_char_offset: dc.l 0              ; Current message character offset
raster_phase:   dc.l    0               ; Copper colour animation phase (0-255)

; --- Copper list buffer ---
; Reserved space for the dynamically-built copper program.
; Maximum: 36 + 99*44 + 4 = ~4400 bytes, rounded up to 4800.
                even
copper_list:    ds.l    1200

; --- Cube vertices (x, y, z as signed 16-bit words) ---
; Centred at origin, half-width = CUBE_SIZE.
;
;        7--------6
;       /|       /|
;      / |      / |
;     4--------5  |
;     |  3-----|--2
;     | /      | /
;     |/       |/
;     0--------1
;
cube_vertices:
                dc.w    -CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 0: front-bottom-left
                dc.w     CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 1: front-bottom-right
                dc.w     CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 2: front-top-right
                dc.w    -CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 3: front-top-left
                dc.w    -CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 4: back-bottom-left
                dc.w     CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 5: back-bottom-right
                dc.w     CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 6: back-top-right
                dc.w    -CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 7: back-top-left

; --- Edge list (pairs of vertex indices) ---
edge_list:
                ; Front face
                dc.b    0, 1
                dc.b    1, 2
                dc.b    2, 3
                dc.b    3, 0
                ; Back face
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

; --- Projected 2D coordinates (filled at runtime) ---
projected_x:    ds.w    NUM_VERTICES
projected_y:    ds.w    NUM_VERTICES

; --- Sine table (256 entries, 8.8 fixed-point) ---
; Values: -256 to +256 representing -1.0 to +1.0
; cos(a) = sin(a + 64), where 64 = 90 degrees in 256-unit system
sin_table:
                ; Quadrant 1: 0 to 90 degrees
                dc.w    0, 6, 12, 18, 25, 31, 37, 43
                dc.w    49, 56, 62, 68, 74, 80, 86, 92
                dc.w    97, 103, 109, 115, 120, 126, 131, 136
                dc.w    142, 147, 152, 157, 162, 167, 171, 176
                dc.w    181, 185, 189, 193, 197, 201, 205, 209
                dc.w    212, 216, 219, 222, 225, 228, 231, 234
                dc.w    236, 238, 241, 243, 245, 247, 248, 250
                dc.w    251, 252, 253, 254, 255, 255, 256, 256

                ; Quadrant 2: 90 to 180 degrees
                dc.w    256, 256, 256, 255, 255, 254, 253, 252
                dc.w    251, 250, 248, 247, 245, 243, 241, 238
                dc.w    236, 234, 231, 228, 225, 222, 219, 216
                dc.w    212, 209, 205, 201, 197, 193, 189, 185
                dc.w    181, 176, 171, 167, 162, 157, 152, 147
                dc.w    142, 136, 131, 126, 120, 115, 109, 103
                dc.w    97, 92, 86, 80, 74, 68, 62, 56
                dc.w    49, 43, 37, 31, 25, 18, 12, 6

                ; Quadrant 3: 180 to 270 degrees
                dc.w    0, -6, -12, -18, -25, -31, -37, -43
                dc.w    -49, -56, -62, -68, -74, -80, -86, -92
                dc.w    -97, -103, -109, -115, -120, -126, -131, -136
                dc.w    -142, -147, -152, -157, -162, -167, -171, -176
                dc.w    -181, -185, -189, -193, -197, -201, -205, -209
                dc.w    -212, -216, -219, -222, -225, -228, -231, -234
                dc.w    -236, -238, -241, -243, -245, -247, -248, -250
                dc.w    -251, -252, -253, -254, -255, -255, -256, -256

                ; Quadrant 4: 270 to 360 degrees
                dc.w    -256, -256, -256, -255, -255, -254, -253, -252
                dc.w    -251, -250, -248, -247, -245, -243, -241, -238
                dc.w    -236, -234, -231, -228, -225, -222, -219, -216
                dc.w    -212, -209, -205, -201, -197, -193, -189, -185
                dc.w    -181, -176, -171, -167, -162, -157, -152, -147
                dc.w    -142, -136, -131, -126, -120, -115, -109, -103
                dc.w    -97, -92, -86, -80, -74, -68, -62, -56
                dc.w    -49, -43, -37, -31, -25, -18, -12, -6

; ============================================================================
; BITMAP FONT (8x8 PIXELS, ASCII 32-126)
; ============================================================================
; Each character is 8 bytes. Each byte is one row of 8 pixels, where bit 7
; (MSB) is the leftmost pixel and bit 0 is the rightmost.
; Font address = simple_font + (ASCII - 32) * 8
; ----------------------------------------------------------------------------

                even

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

; --- Scroll message ---
scroll_message:
                dc.b    "   INTUITION ENGINE ... COPPER RASTER BARS - IMPOSSIBLE ON REAL VGA! ... "
                dc.b    "THE COPPER CHANGES PALETTE ENTRIES MID-FRAME FOR THIS RAINBOW EFFECT ... "
                dc.b    "68020 CODE FOR VGA MODE13H 3D CUBE AND 360 DEGREE SCROLLER... "
                dc.b    "6502 CODE FOR EDGE OF DISGRACE BY BOOZE DESIGN FOR SID REMAPPED TO NATIVE SYNTH ... "
                dc.b    "GREETS TO ALL DEMOSCENERS ... "
                dc.b    "   "
scroll_msg_end:
                dc.b    0

                even

; ============================================================================
; SID MUSIC DATA
; ============================================================================
; WHY: The .SID file contains actual 6502 machine code -- the original C64
; music player. The Intuition Engine's 6502 core executes it and intercepts
; SID register writes ($D400-$D418), remapping them to the native synth.
; "Edge of Disgrace" by Booze Design, music by Goto80.
; ----------------------------------------------------------------------------
sid_data:
                incbin  "../assets/music/Edge_of_Disgrace.sid"
sid_data_end:

                end
