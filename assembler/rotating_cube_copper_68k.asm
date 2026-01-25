; ============================================================================
; ROTATING 3D CUBE DEMO WITH COPPER RAINBOW RASTER BARS
; M68020 Assembly for IntuitionEngine - VGA Mode 13h (320x200x256)
; ============================================================================
;
; REFERENCE IMPLEMENTATION FOR DEMOSCENE TECHNIQUES
; This file is heavily commented to teach demo programming concepts.
;
; === WHAT THIS DEMO DOES ===
; 1. Displays a wireframe 3D cube rotating in real-time
; 2. Shows a circular scrolltext orbiting the screen center
; 3. Creates animated rainbow raster bars using the copper coprocessor
; 4. Plays SID music through the audio subsystem
;
; === WHY THESE EFFECTS MATTER (HISTORICAL CONTEXT) ===
; In the demoscene, coders compete to create visually impressive effects
; within hardware constraints. This demo showcases several classic techniques:
;
; - COPPER RASTER BARS: On the Amiga, the "copper" coprocessor could change
;   hardware registers mid-frame, synchronized to the electron beam. This
;   allowed effects IMPOSSIBLE with normal VGA hardware, like changing the
;   background color on every scanline to create gradients. The Intuition
;   Engine emulates this capability, letting us recreate Amiga-style effects.
;
; - 3D ROTATION: Real-time 3D was a "holy grail" effect in the 1980s-90s.
;   Without floating-point hardware, we use fixed-point math (integers that
;   represent fractions) and lookup tables for trigonometry.
;
; - CIRCULAR SCROLLER: Text effects were a demoscene staple. A circular
;   scroller (text orbiting in a circle) shows mastery of polar coordinates
;   and timing.
;
; === ARCHITECTURE OVERVIEW ===
;
;   ┌─────────────────────────────────────────────────────────────┐
;   │                    MAIN LOOP (60 FPS)                       │
;   │                                                             │
;   │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐      │
;   │  │  WAIT FOR   │───►│   CLEAR     │───►│    DRAW     │      │
;   │  │   VSYNC     │    │   SCREEN    │    │  SCROLLER   │      │
;   │  └─────────────┘    └─────────────┘    └─────────────┘      │
;   │                                              │              │
;   │  ┌─────────────┐    ┌─────────────┐          │              │
;   │  │   UPDATE    │◄───│    DRAW     │◄─────────┘              │
;   │  │   COPPER    │    │    CUBE     │                         │
;   │  └─────────────┘    └─────────────┘                         │
;   └─────────────────────────────────────────────────────────────┘
;
;   ┌─────────────────────────────────────────────────────────────┐
;   │              COPPER COPROCESSOR (runs in parallel)          │
;   │                                                             │
;   │  The copper executes its program list during each frame,    │
;   │  changing palette entry #1 at specific scanlines to create  │
;   │  the rainbow gradient effect in the background.             │
;   └─────────────────────────────────────────────────────────────┘
;
; ============================================================================

                include "ie68.inc"

                org     PROGRAM_START

; ============================================================================
; CONSTANTS
; ============================================================================

; --- VGA Mode 13h Screen Geometry ---
; Mode 13h is the classic "MCGA" mode: 320x200 pixels, 256 colors.
; Each pixel is one byte in video memory (linear layout, no bit planes).
; This mode was ubiquitous in DOS games and demos because:
; 1. Simple linear framebuffer (pixel address = y*320 + x)
; 2. 256 colors from an 18-bit palette (262,144 possible colors)
; 3. Fits in 64KB, allowing fast clearing and double-buffering
SCR_W           equ     320             ; Horizontal resolution
SCR_H           equ     200             ; Vertical resolution
CENTER_X        equ     160             ; Screen center X (320/2)
CENTER_Y        equ     100             ; Screen center Y (200/2)

; --- 3D Cube Geometry ---
; A cube has 8 vertices (corners) and 12 edges connecting them.
; We define vertices as 3D coordinates and edges as vertex index pairs.
NUM_VERTICES    equ     8
NUM_EDGES       equ     12
CUBE_SIZE       equ     40              ; Half-width of cube (±40 units from center)
                                        ; Kept small to leave room for scroller

; --- Perspective Projection ---
; DISTANCE controls the "focal length" of our virtual camera.
; Larger values = less perspective distortion (more orthographic)
; Smaller values = more extreme perspective (objects shrink faster with depth)
; 256 is chosen because it's a power of 2, making division efficient.
DISTANCE        equ     256

; --- Fixed-Point Arithmetic (8.8 Format) ---
; Without a floating-point unit, we simulate decimals using integers.
; In 8.8 format, the lower 8 bits represent the fractional part:
;   - 256 ($100) = 1.0
;   - 128 ($80)  = 0.5
;   - 64  ($40)  = 0.25
; To convert: multiply by 256, then shift right 8 bits after operations.
; This gives us ~0.004 precision (1/256) which is sufficient for smooth animation.
FP_SHIFT        equ     8               ; Number of fractional bits
FP_ONE          equ     256             ; 1.0 in fixed-point

; --- Circular Scroller Geometry ---
; Characters orbit around the screen center like numbers on a clock face.
; We use polar coordinates (angle + radius) converted to Cartesian (x, y).
SCROLL_RADIUS   equ     75              ; Distance from center to text
SCROLL_CENTER_X equ     160             ; Orbit center X
SCROLL_CENTER_Y equ     100             ; Orbit center Y
NUM_VISIBLE     equ     12              ; How many characters visible at once
                                        ; (like 12 hours on a clock)
CHAR_SPACING    equ     21              ; Angular spacing between chars
                                        ; 256/12 ≈ 21 (256 = full circle in our angle system)
DRAW_CHAR_SIZE  equ     8               ; Character size in pixels (8x8 font)
SCROLL_SPEED    equ     1               ; Rotation speed (1 = smooth, higher = faster)

; --- Copper Raster Bar Configuration ---
; The copper changes the background color multiple times per frame.
; More changes = smoother gradient, but more CPU time to set up.
NUM_COPPER_LINES equ    100             ; Number of color changes per frame
LINES_PER_CHANGE equ    2               ; Scanlines between each change
                                        ; 100 changes * 2 lines = 200 scanlines (full screen)

; ============================================================================
; ENTRY POINT
; ============================================================================
; The CPU begins execution here after loading the program.
; We must initialize all hardware subsystems before entering the main loop.
; ============================================================================
start:
                ; --- Initialize Stack Pointer ---
                ; The M68K uses a descending stack (grows toward lower addresses).
                ; STACK_TOP is defined in ie68.inc and points to high memory.
                ; This must be set before ANY subroutine calls (BSR/JSR).
                move.l  #STACK_TOP,sp

                ; --- Enable IE Video Chip ---
                ; CRITICAL: The copper coprocessor is part of the IE video chip,
                ; NOT the VGA chip. We must enable the IE video system first,
                ; even though we're using VGA for the actual display.
                ; This is a common gotcha - the copper won't work without this!
                move.l  #1,VIDEO_CTRL

                ; --- Configure VGA Hardware ---
                ; VGA_MODE selects the video mode (Mode 13h = $13 = 19 decimal)
                ; VGA_CTRL enables the VGA output
                ; After this, VGA_VRAM becomes our framebuffer.
                move.b  #VGA_MODE_13H,VGA_MODE
                move.b  #VGA_CTRL_ENABLE,VGA_CTRL

                ; --- Build the Copper Program ---
                ; The copper executes a "copper list" - a program of WAIT and MOVE
                ; instructions. We build this list dynamically because the color
                ; values change every frame for animation.
                bsr     build_copper_list

                ; --- Activate Copper Coprocessor ---
                ; COPPER_PTR tells the copper where to find its program.
                ; COPPER_CTRL=1 enables copper execution.
                ; The copper will now run automatically each frame, synchronized
                ; to the vertical blank (VSYNC).
                move.l  #copper_list,COPPER_PTR
                move.l  #1,COPPER_CTRL

                ; --- Start Music Playback ---
                ; SID music files contain both the player code and music data.
                ; The audio subsystem handles playback automatically once started.
                ; This runs in the background - we don't need to call it each frame.
                move.l  #sid_data,SID_PLAY_PTR
                move.l  #sid_data_end-sid_data,SID_PLAY_LEN
                start_sid_loop          ; Macro defined in ie68.inc

                ; --- Initialize Animation State ---
                ; All angles and offsets start at zero.
                ; clr.l is faster than move.l #0 on most M68K variants.
                clr.l   angle_x                 ; Cube X-axis rotation angle
                clr.l   angle_y                 ; Cube Y-axis rotation angle
                clr.l   scroll_angle            ; Scroller rotation position
                clr.l   scroll_char_offset      ; Which character is at position 0
                clr.l   raster_phase            ; Copper color animation phase

; ============================================================================
; MAIN LOOP
; ============================================================================
; This loop runs once per frame (60 times per second with VSYNC).
; The order of operations matters:
; 1. Wait for VSYNC (prevents tearing, ensures consistent timing)
; 2. Clear the screen (erase previous frame)
; 3. Draw all objects (scroller first, then cube on top)
; 4. Update animation state (angles, positions)
; 5. Update copper colors (for next frame's raster bars)
; ============================================================================
main_loop:
                ; === VSYNC SYNCHRONIZATION ===
                ; We wait for vertical blank to prevent "tearing" (seeing a
                ; partially-drawn frame). The electron beam retraces from bottom
                ; to top during VBLANK - this is when we should update the screen.
                ;
                ; The two-stage wait ensures we catch the START of VBLANK:
                ; 1. First, wait while ALREADY in vblank (in case we're mid-vblank)
                ; 2. Then, wait until vblank BEGINS (catch the rising edge)
                ; This guarantees exactly one frame per loop iteration.
.wait_not_vb:   move.b  VGA_STATUS,d0           ; Read VGA status register
                andi.b  #VGA_STATUS_VSYNC,d0    ; Isolate VSYNC bit
                bne.s   .wait_not_vb            ; Loop while in vblank

.wait_vb:       move.b  VGA_STATUS,d0           ; Read VGA status again
                andi.b  #VGA_STATUS_VSYNC,d0    ; Isolate VSYNC bit
                beq.s   .wait_vb                ; Loop until vblank starts

                ; === CLEAR SCREEN ===
                ; We fill the entire screen with color index 1 (not 0!).
                ; Why index 1? Because the copper modifies palette entry 1 on
                ; each scanline. If we used index 0, the raster bars wouldn't show.
                ; The raster effect relies on drawing "background" pixels with a
                ; color index that the copper is actively changing.
                bsr     clear_screen

                ; === DRAW CIRCULAR SCROLLER ===
                ; Drawing order determines layering. The scroller is drawn FIRST
                ; so the cube appears ON TOP of it. This creates depth without
                ; needing a Z-buffer.
                bsr     draw_circular_scroll

                ; === 3D CUBE RENDERING PIPELINE ===
                ; Classic 3D graphics pipeline for wireframe rendering:
                ; 1. calc_rotation: Compute sin/cos values for current angles
                ; 2. transform_vertices: Apply rotation matrices to all 8 vertices
                ; 3. draw_edges: Connect vertices with lines using projected coords
                bsr     calc_rotation           ; Pre-compute trig values
                bsr     transform_vertices      ; Transform 3D → 2D
                bsr     draw_edges              ; Render wireframe

                ; === UPDATE ANIMATION STATE ===
                ; Animation is achieved by incrementing angles each frame.
                ; Different speeds on X and Y axes create interesting tumbling motion.

                ; Update cube rotation (different speeds for interesting motion)
                addq.l  #2,angle_x              ; X rotates slower
                addq.l  #3,angle_y              ; Y rotates faster (prime number = no sync)
                andi.l  #$FF,angle_x            ; Wrap to 0-255 (one full rotation)
                andi.l  #$FF,angle_y            ; Our angle system uses 256 = 360°

                ; Update scroller rotation
                move.l  scroll_angle,d0
                addq.l  #SCROLL_SPEED,d0
                andi.l  #$FF,d0                 ; Wrap angle to 0-255
                move.l  d0,scroll_angle

                ; Update copper raster animation
                ; The phase offset makes colors scroll vertically through the bars
                move.l  raster_phase,d0
                addq.l  #3,d0                   ; Speed of vertical color scrolling
                andi.l  #$FF,d0
                move.l  d0,raster_phase
                bsr     update_copper_colors    ; Modify copper list with new colors

                bra     main_loop               ; Loop forever

; ============================================================================
; CLEAR SCREEN
; ============================================================================
; Fills the entire 64000-byte framebuffer with color index 1.
;
; WHY INDEX 1, NOT 0?
; The copper coprocessor changes palette entry 1 on each scanline.
; Pixels with color index 1 will display the copper's current color.
; This is how raster bars work: the SAME pixel index shows DIFFERENT
; colors at different vertical positions.
;
; WHY UNROLL THE LOOP?
; Loop overhead (dbf, branch) takes CPU cycles. By writing 32 bytes
; per iteration instead of 1, we reduce overhead by 32x.
; 64000 bytes / 32 bytes per iteration = 2000 iterations.
;
; WHY move.b INSTEAD OF move.l?
; VGA VRAM is byte-addressable only in Mode 13h. Long writes would
; require aligned addresses and might not work with all VGA implementations.
; The slight speed loss is acceptable for compatibility.
; ============================================================================
clear_screen:
                movem.l d0/a0,-(sp)             ; Save registers we'll modify
                lea     VGA_VRAM,a0             ; a0 = start of video memory
                move.w  #2000-1,d0              ; Loop counter (dbf needs N-1)

.clear_loop:
                ; Write 32 bytes of color index 1 per iteration
                ; This is a classic "loop unrolling" optimization
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
                dbf     d0,.clear_loop          ; Decrement and branch if not -1

                movem.l (sp)+,d0/a0             ; Restore saved registers
                rts

; ============================================================================
; DRAW CIRCULAR SCROLLER
; ============================================================================
; Characters orbit around the screen center like numbers on a clock.
; This demonstrates conversion between polar and Cartesian coordinates:
;
;   POLAR:     (radius, angle) - distance and direction from center
;   CARTESIAN: (x, y) - horizontal and vertical position
;
; Conversion formulas:
;   x = center_x + radius * cos(angle)
;   y = center_y + radius * sin(angle)
;
; The characters advance through the message as time passes, and each
; character is offset by a fixed angle (CHAR_SPACING) from its neighbors.
; ============================================================================
draw_circular_scroll:
                movem.l d0-d7/a0-a4,-(sp)       ; Save all registers we'll use

                moveq   #0,d7                   ; d7 = current visible char index (0 to 11)

.char_loop:
                ; === CALCULATE CHARACTER'S ANGULAR POSITION ===
                ; Each character's angle = scroll_angle + (index * spacing)
                ; This spreads characters evenly around the circle
                move.l  scroll_angle,d0         ; Base rotation angle
                move.l  d7,d1                   ; Character index (0-11)
                mulu.w  #CHAR_SPACING,d1        ; Multiply by angular spacing
                add.l   d1,d0                   ; Add to base angle
                andi.l  #$FF,d0                 ; Wrap to 0-255 (256 = full circle)

                ; === LOOK UP SIN AND COS FROM TABLE ===
                ; We use a precomputed sine table to avoid expensive trig calculations.
                ; For cosine, we use the identity: cos(θ) = sin(θ + 90°)
                ; In our 256-unit system, 90° = 64 units (256/4)
                lea     sin_table,a0
                move.l  d0,d1
                add.w   d1,d1                   ; Multiply by 2 (table entries are words)
                move.w  (a0,d1.w),d2            ; d2 = sin(angle) in 8.8 fixed-point

                ; cos(angle) = sin(angle + 64), where 64 = 90° in our system
                move.l  d0,d1
                addi.l  #64,d1                  ; Add 90° (quarter turn)
                andi.l  #$FF,d1                 ; Wrap angle
                add.w   d1,d1                   ; Multiply by 2 for word access
                move.w  (a0,d1.w),d3            ; d3 = cos(angle) in 8.8 fixed-point

                ; === CONVERT POLAR TO CARTESIAN COORDINATES ===
                ; x = CENTER_X + (RADIUS * cos) / 256
                ; y = CENTER_Y + (RADIUS * sin) / 256
                ; The division by 256 converts from 8.8 fixed-point to integer.

                ; Calculate X position
                move.w  d3,d4                   ; d4 = cos value
                ext.l   d4                      ; Sign-extend to long (cos can be negative)
                muls.w  #SCROLL_RADIUS,d4       ; Multiply by radius
                asr.l   #FP_SHIFT,d4            ; Divide by 256 (shift right 8 bits)
                addi.l  #SCROLL_CENTER_X,d4    ; Add center offset
                subi.l  #DRAW_CHAR_SIZE/2,d4   ; Center the character on this point

                ; Calculate Y position (same process with sin instead of cos)
                move.w  d2,d5
                ext.l   d5
                muls.w  #SCROLL_RADIUS,d5
                asr.l   #FP_SHIFT,d5
                addi.l  #SCROLL_CENTER_Y,d5
                subi.l  #DRAW_CHAR_SIZE/2,d5

                ; === GET CHARACTER FROM MESSAGE ===
                ; We need to handle wrapping when we reach the end of the message.
                ; scroll_char_offset advances over time, so different parts of
                ; the message become visible.
                move.l  scroll_char_offset,d6   ; Starting character in message
                add.l   d7,d6                   ; Add visible position index

                ; Calculate message length for wrapping
                lea     scroll_message,a1
                lea     scroll_msg_end,a2
                move.l  a2,d0
                sub.l   a1,d0                   ; d0 = message length
                subq.l  #1,d0                   ; Exclude null terminator

                ; Wrap character index if past end of message
.wrap_check:
                cmp.l   d0,d6
                blt.s   .no_wrap
                sub.l   d0,d6                   ; Subtract message length
                bra.s   .wrap_check             ; Check again (may need multiple wraps)
.no_wrap:
                move.b  (a1,d6.l),d6            ; Load ASCII character from message
                andi.l  #$7F,d6                 ; Mask to 7-bit ASCII (safety)

                ; === DRAW THE CHARACTER ===
                ; Parameters: d4=x, d5=y, d6=ASCII character code
                bsr     draw_scroll_char

                ; === NEXT CHARACTER ===
                addq.l  #1,d7
                cmpi.l  #NUM_VISIBLE,d7
                blt     .char_loop              ; Loop for all visible characters

                ; === ADVANCE MESSAGE OFFSET ===
                ; Every 32 angle steps, shift the message by one character.
                ; This makes the text gradually scroll through the orbit.
                ;
                ; NOTE ON OVERFLOW: scroll_char_offset is incremented but we
                ; don't wrap it here - the per-character wrap_check loop handles
                ; arbitrarily large offsets. However, after ~2 billion frames
                ; (~1 year at 60fps), the 32-bit counter would overflow.
                ; For production code, wrap it to message length here:
                move.l  scroll_angle,d0
                andi.l  #$1F,d0                 ; Modulo 32
                bne.s   .no_advance             ; Only advance when angle wraps to 0

                ; Advance and wrap to prevent eventual 32-bit overflow
                lea     scroll_message,a1
                lea     scroll_msg_end,a2
                move.l  a2,d0
                sub.l   a1,d0
                subq.l  #1,d0                   ; d0 = message length

                move.l  scroll_char_offset,d1
                addq.l  #1,d1                   ; Move to next character
                cmp.l   d0,d1                   ; Past end of message?
                blt.s   .no_wrap_offset
                moveq   #0,d1                   ; Wrap back to start
.no_wrap_offset:
                move.l  d1,scroll_char_offset
.no_advance:

                movem.l (sp)+,d0-d7/a0-a4
                rts

; ============================================================================
; DRAW SCROLL CHARACTER
; ============================================================================
; Renders a single 8x8 character from our bitmap font.
;
; Input: d4 = x position, d5 = y position, d6 = ASCII code
;
; The font is stored as a bitmap: 8 bytes per character, each byte is one
; row, each bit is one pixel (MSB = leftmost pixel).
;
; We add color variation based on scroll_angle and pixel position to create
; a shimmering, animated effect on the text.
; ============================================================================
draw_scroll_char:
                movem.l d0-d7/a0-a2,-(sp)

                ; === BOUNDS CHECKING ===
                ; Skip characters that are partially or fully off-screen.
                ; This prevents writing to memory outside the framebuffer.
                tst.l   d4                      ; x < 0?
                bmi     .char_done
                cmpi.l  #SCR_W-8,d4             ; x > 312? (would overflow right edge)
                bgt     .char_done
                tst.l   d5                      ; y < 0?
                bmi     .char_done
                cmpi.l  #SCR_H-8,d5             ; y > 192? (would overflow bottom)
                bgt     .char_done

                ; === LOOK UP CHARACTER IN FONT ===
                ; Font starts at ASCII 32 (space), 8 bytes per character
                lea     simple_font,a0

                ; Convert ASCII to font index (space=0, '!'=1, etc.)
                subi.l  #32,d6                  ; Subtract ASCII code of space
                bmi     .char_done              ; Skip if character < space
                cmpi.l  #95,d6                  ; Font has 95 characters (32-126)
                bge     .char_done              ; Skip if character > '~'

                lsl.l   #3,d6                   ; Multiply by 8 (bytes per char)
                add.l   d6,a0                   ; a0 now points to this char's bitmap

                ; === CALCULATE VRAM DESTINATION ===
                ; Linear address = y * 320 + x
                lea     VGA_VRAM,a1
                move.l  d5,d0
                mulu.w  #SCR_W,d0               ; y * 320
                add.l   d4,d0                   ; + x
                add.l   d0,a1                   ; a1 = destination in VRAM

                ; === RENDER 8 ROWS ===
                moveq   #7,d7                   ; Row counter (7 down to 0)

.row_loop:
                move.b  (a0)+,d0                ; Load row bitmap (8 pixels as bits)
                moveq   #7,d3                   ; Column counter (7 down to 0)

.col_loop:
                btst    d3,d0                   ; Test if this pixel is set
                beq.s   .skip_pixel             ; Skip if bit is 0 (transparent)

                ; === CALCULATE ANIMATED COLOR ===
                ; Color varies based on scroll_angle, row, and column.
                ; This creates a shimmering rainbow effect on the text.
                move.l  scroll_angle,d1
                add.l   d7,d1                   ; Add row position
                add.l   d3,d1                   ; Add column position
                andi.l  #$3F,d1                 ; Limit to 64 colors (0-63)
                addi.l  #64,d1                  ; Use palette entries 64-127
                move.b  d1,(a1)                 ; Write pixel to VRAM

.skip_pixel:
                addq.l  #1,a1                   ; Move to next pixel in VRAM
                dbf     d3,.col_loop            ; Next column

                ; Move to next row in VRAM (add 312 to skip to next line)
                ; We already advanced 8 pixels, so add 320-8=312
                lea     SCR_W-8(a1),a1
                dbf     d7,.row_loop            ; Next row

.char_done:
                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; CALCULATE ROTATION MATRIX COMPONENTS
; ============================================================================
; Pre-computes sin and cos values for the current rotation angles.
; These values are used by transform_vertices for the rotation matrices.
;
; WHY PRE-COMPUTE?
; Each vertex needs sin/cos of BOTH angles. With 8 vertices, that's 32
; table lookups if done inline. Pre-computing reduces this to 4 lookups
; total, then 8 memory reads per axis during transformation.
;
; The sin table contains 256 entries representing a full sine wave in
; 8.8 fixed-point format (-256 to +256 representing -1.0 to +1.0).
; ============================================================================
calc_rotation:
                movem.l d0-d1/a0,-(sp)
                lea     sin_table,a0

                ; --- Compute sin(angle_x) and cos(angle_x) ---
                move.l  angle_x,d0
                andi.l  #$FF,d0                 ; Ensure angle is 0-255
                add.w   d0,d0                   ; Multiply by 2 for word offset
                move.w  (a0,d0.w),sin_x         ; Store sin(angle_x)

                move.l  angle_x,d0
                addi.l  #64,d0                  ; cos = sin(angle + 90°)
                andi.l  #$FF,d0                 ; Wrap angle
                add.w   d0,d0
                move.w  (a0,d0.w),cos_x         ; Store cos(angle_x)

                ; --- Compute sin(angle_y) and cos(angle_y) ---
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
; This is the heart of 3D graphics: transforming 3D world coordinates
; to 2D screen coordinates.
;
; === THE 3D PIPELINE ===
;
; 1. ROTATION: Apply rotation matrices around X and Y axes.
;    Each rotation uses the formula:
;      x' = x*cos(θ) - z*sin(θ)
;      z' = x*sin(θ) + z*cos(θ)    (for Y-axis rotation)
;    Similar formulas for X-axis rotation affecting Y and Z.
;
; 2. PERSPECTIVE PROJECTION: Convert 3D to 2D using:
;      screen_x = (x * distance) / z + center_x
;      screen_y = (y * distance) / z + center_y
;    Objects farther away (larger z) appear smaller.
;
; === WHY THIS ORDER? ===
; We rotate around Y first (left/right tumble), then X (forward/back tilt).
; This creates a pleasing tumbling motion. Different orderings produce
; different visual effects (rotation order matters in 3D!).
;
; === FIXED-POINT MATH (IMPORTANT CLARIFICATION) ===
; Only the TRIG VALUES (sin_x, cos_x, sin_y, cos_y) are in 8.8 fixed-point.
; The VERTEX COORDINATES remain as plain integers throughout!
;
; Here's what happens at each step:
;   1. Load vertex: d0 = -40 (plain integer, e.g., CUBE_SIZE)
;   2. Load trig:   d5 = 256 (8.8 fixed-point for 1.0, i.e., cos(0))
;   3. Multiply:    d3 = -40 * 256 = -10240 (now in 8.8 scale)
;   4. Shift right: d3 = -10240 >> 8 = -40 (back to integer)
;
; This "multiply then shift" pattern keeps coordinates as integers while
; applying fractional trig values. The shift after each multiply prevents
; values from growing unboundedly.
;
; WHY divs.w IS SAFE:
; After rotation, coordinates are still small integers (roughly ±CUBE_SIZE).
; The perspective division (x * DISTANCE / z) uses 16-bit signed division.
; With CUBE_SIZE=40 and DISTANCE=256, the numerator is at most ±10240,
; well within the 16-bit signed range (-32768 to +32767).
; ============================================================================
transform_vertices:
                movem.l d0-d7/a0-a2,-(sp)

                lea     cube_vertices,a0        ; Source: 3D vertex coordinates
                lea     projected_x,a1          ; Destination: 2D X coordinates
                lea     projected_y,a2          ; Destination: 2D Y coordinates
                moveq   #NUM_VERTICES-1,d7      ; Loop counter (8 vertices, 0-indexed)

.vertex_loop:
                ; --- Load 3D vertex coordinates ---
                move.w  (a0)+,d0                ; d0 = original X
                move.w  (a0)+,d1                ; d1 = original Y
                move.w  (a0)+,d2                ; d2 = original Z
                ext.l   d0                      ; Sign-extend to long for math
                ext.l   d1
                ext.l   d2

                ; ===================================================
                ; Y-AXIS ROTATION (rotates around vertical axis)
                ; This makes the cube spin left/right.
                ;
                ; Formula:
                ;   x' = x*cos(θ) - z*sin(θ)
                ;   z' = x*sin(θ) + z*cos(θ)
                ;   y' = y (unchanged)
                ; ===================================================
                move.l  d0,d3                   ; d3 = x (working copy)
                move.l  d2,d4                   ; d4 = z (working copy)

                ; Calculate x' = x*cos - z*sin
                move.w  cos_y,d5
                ext.l   d5
                muls.w  d5,d3                   ; d3 = x * cos_y
                move.w  sin_y,d5
                ext.l   d5
                muls.w  d5,d4                   ; d4 = z * sin_y
                sub.l   d4,d3                   ; d3 = x*cos - z*sin
                asr.l   #FP_SHIFT,d3            ; Convert from 16.16 to 8.8

                ; Calculate z' = x*sin + z*cos
                move.l  d0,d4                   ; Reload x
                move.l  d2,d6                   ; Reload z
                move.w  sin_y,d5
                ext.l   d5
                muls.w  d5,d4                   ; d4 = x * sin_y
                move.w  cos_y,d5
                ext.l   d5
                muls.w  d5,d6                   ; d6 = z * cos_y
                add.l   d4,d6                   ; d6 = x*sin + z*cos
                asr.l   #FP_SHIFT,d6

                move.l  d3,d0                   ; Update x with rotated value
                move.l  d6,d2                   ; Update z with rotated value

                ; ===================================================
                ; X-AXIS ROTATION (rotates around horizontal axis)
                ; This makes the cube tumble forward/backward.
                ;
                ; Formula:
                ;   y' = y*cos(θ) - z*sin(θ)
                ;   z' = y*sin(θ) + z*cos(θ)
                ;   x' = x (unchanged)
                ; ===================================================
                move.l  d1,d3                   ; d3 = y
                move.l  d2,d4                   ; d4 = z

                ; Calculate y' = y*cos - z*sin
                move.w  cos_x,d5
                ext.l   d5
                muls.w  d5,d3
                move.w  sin_x,d5
                ext.l   d5
                muls.w  d5,d4
                sub.l   d4,d3
                asr.l   #FP_SHIFT,d3

                ; Calculate z' = y*sin + z*cos
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

                move.l  d3,d1                   ; Final Y after both rotations
                move.l  d6,d2                   ; Final Z after both rotations

                ; ===================================================
                ; PERSPECTIVE PROJECTION
                ; Convert 3D coordinates to 2D screen coordinates.
                ;
                ; The "pinhole camera" model:
                ;   screen_x = (x * focal_length) / z + center_x
                ;   screen_y = (y * focal_length) / z + center_y
                ;
                ; We add DISTANCE to z to push the cube away from camera
                ; (otherwise z=0 vertices would cause division by zero).
                ; ===================================================
                add.l   #DISTANCE,d2            ; Move cube away from camera
                beq.s   .skip_div               ; Avoid division by zero
                bmi.s   .skip_div               ; Skip if z is behind camera

                ; Project X: screen_x = (x * DISTANCE) / z + CENTER_X
                move.l  d0,d3
                muls.w  #DISTANCE,d3            ; x * focal_length
                divs.w  d2,d3                   ; / z
                ext.l   d3                      ; Clean up result
                add.l   #CENTER_X,d3            ; Center on screen
                move.w  d3,(a1)+                ; Store projected X

                ; Project Y: screen_y = (y * DISTANCE) / z + CENTER_Y
                move.l  d1,d3
                muls.w  #DISTANCE,d3
                divs.w  d2,d3
                ext.l   d3
                add.l   #CENTER_Y,d3
                move.w  d3,(a2)+                ; Store projected Y

                dbf     d7,.vertex_loop
                bra.s   .done

.skip_div:
                ; Vertex is at or behind camera - mark as invalid
                ; -1000 is guaranteed to be off-screen
                move.w  #-1000,(a1)+
                move.w  #-1000,(a2)+
                dbf     d7,.vertex_loop

.done:
                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; DRAW ALL EDGES
; ============================================================================
; Renders the wireframe cube by drawing lines between connected vertices.
;
; The edge list defines which vertex pairs should be connected.
; A cube has 12 edges: 4 on the front face, 4 on the back face,
; and 4 connecting front to back.
;
; Each edge uses a slightly different color based on its index,
; creating visual variety and helping distinguish overlapping edges.
; ============================================================================
draw_edges:
                movem.l d0-d7/a0-a2,-(sp)

                lea     edge_list,a0            ; Pairs of vertex indices
                moveq   #NUM_EDGES-1,d7         ; 12 edges, 0-indexed

.edge_loop:
                ; Load vertex indices for this edge
                move.b  (a0)+,d0                ; First vertex index
                move.b  (a0)+,d1                ; Second vertex index
                ext.w   d0                      ; Sign-extend to word
                ext.w   d1

                ; Look up projected 2D coordinates
                lea     projected_x,a1
                lea     projected_y,a2

                ; Get coordinates of first vertex
                move.w  d0,d2
                add.w   d2,d2                   ; Multiply by 2 for word offset
                move.w  (a1,d2.w),d3            ; d3 = x1
                move.w  (a2,d2.w),d4            ; d4 = y1

                ; Get coordinates of second vertex
                move.w  d1,d2
                add.w   d2,d2
                move.w  (a1,d2.w),d5            ; d5 = x2
                move.w  (a2,d2.w),d6            ; d6 = y2

                ; === CALCULATE EDGE COLOR ===
                ; Each edge gets a different color for visual interest.
                ; We use a formula that spreads colors across the palette.
                move.l  d7,d2                   ; Edge index (0-11)
                andi.l  #7,d2                   ; Reduce to 0-7
                add.l   #40,d2                  ; Base color index
                lsl.l   #4,d2                   ; Shift up for brighter colors
                addi.l  #15,d2                  ; Add offset
                andi.l  #$FF,d2                 ; Keep in valid palette range

                ; Draw the line
                ; Parameters: d3=x1, d4=y1, d5=x2, d6=y2, d2=color
                bsr     draw_line

                dbf     d7,.edge_loop

                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; DRAW LINE (BRESENHAM'S ALGORITHM)
; ============================================================================
; Draws a line between two points using Bresenham's algorithm.
;
; Input: d3=x1, d4=y1, d5=x2, d6=y2, d2=color
;
; === WHY BRESENHAM? ===
; Bresenham's line algorithm uses only integer arithmetic (no division
; or floating point), making it extremely fast on integer-only hardware.
; It works by tracking an "error" term that accumulates as we step along
; the major axis, deciding when to step along the minor axis.
;
; === ALGORITHM OVERVIEW ===
; 1. Calculate dx and dy (deltas)
; 2. Determine major axis (the one with larger delta)
; 3. Step along major axis one pixel at a time
; 4. Accumulate error; when it overflows, step on minor axis
;
; === CLIPPING (SIMPLIFIED - SEE WARNING) ===
; WARNING: This is NOT true line clipping! We use a fast but aggressive
; reject test: if the first endpoint is off-screen, we check if the second
; is ALSO off-screen (in ANY direction) and skip the entire line.
;
; This is INCORRECT for lines that span across the screen, e.g.:
;   - Line from (-10, 100) to (330, 100) crosses the visible area
;   - But our test sees: x1<0 (off-screen), x2>=320 (off-screen) → REJECTED
;
; For a small centered cube this rarely matters, but for larger scenes
; or objects near screen edges, you'd want proper Cohen-Sutherland or
; Liang-Barsky clipping that computes actual intersection points.
;
; We accept this limitation here because:
;   1. The cube is small and centered, so edges rarely span the full screen
;   2. True clipping adds significant complexity
;   3. The per-pixel bounds check in plot_pixel prevents memory corruption
; ============================================================================
draw_line:
                movem.l d0-d7/a0,-(sp)

                ; === BASIC VISIBILITY CHECK ===
                ; If first point is on-screen, we'll try to draw
                cmpi.w  #0,d3
                blt.s   .check_x2               ; x1 < 0, check second point
                cmpi.w  #SCR_W,d3
                bge.s   .check_x2               ; x1 >= 320, check second point
                cmpi.w  #0,d4
                blt.s   .check_x2               ; y1 < 0, check second point
                cmpi.w  #SCR_H,d4
                bge.s   .check_x2               ; y1 >= 200, check second point
                bra.s   .start_line             ; First point is valid, draw line

.check_x2:
                ; First point is off-screen; check if second point is also off
                cmpi.w  #0,d5
                blt     .line_done              ; x2 < 0, skip entirely
                cmpi.w  #SCR_W,d5
                bge     .line_done              ; x2 >= 320, skip entirely
                cmpi.w  #0,d6
                blt     .line_done              ; y2 < 0, skip entirely
                cmpi.w  #SCR_H,d6
                bge     .line_done              ; y2 >= 200, skip entirely

.start_line:
                ; === CALCULATE DELTAS ===
                ; dx = x2 - x1, dy = y2 - y1
                move.w  d5,d0                   ; d0 = x2
                sub.w   d3,d0                   ; d0 = dx = x2 - x1
                move.w  d6,d1                   ; d1 = y2
                sub.w   d4,d1                   ; d1 = dy = y2 - y1

                ; === DETERMINE X STEP DIRECTION ===
                ; sx = 1 if dx > 0, else -1
                moveq   #1,d7
                tst.w   d0
                bge.s   .dx_pos
                neg.w   d0                      ; Make dx positive (absolute value)
                moveq   #-1,d7                  ; Step left instead of right
.dx_pos:
                move.w  d7,-(sp)                ; Save sx on stack

                ; === DETERMINE Y STEP DIRECTION ===
                ; sy = 1 if dy > 0, else -1
                moveq   #1,d7
                tst.w   d1
                bge.s   .dy_pos
                neg.w   d1                      ; Make dy positive
                moveq   #-1,d7                  ; Step up instead of down
.dy_pos:
                move.w  d7,-(sp)                ; Save sy on stack

                ; === CHOOSE MAJOR AXIS ===
                ; If dx >= dy, X is the major axis (more horizontal line)
                ; If dy > dx, Y is the major axis (more vertical line)
                cmp.w   d1,d0
                bge.s   .x_major

                ; --- Y-MAJOR LOOP ---
                ; Step along Y axis, occasionally stepping on X
                move.w  d1,d7
                lsr.w   #1,d7                   ; Error term starts at dy/2

.y_loop:
                bsr     plot_pixel              ; Draw current pixel
                cmp.w   d6,d4                   ; Reached destination Y?
                beq.s   .line_done_pop
                add.w   (sp),d4                 ; Step Y by sy
                sub.w   d0,d7                   ; Subtract dx from error
                bge.s   .y_loop                 ; If error >= 0, continue
                add.w   d1,d7                   ; Add dy to error
                add.w   2(sp),d3                ; Step X by sx
                bra.s   .y_loop

                ; --- X-MAJOR LOOP ---
                ; Step along X axis, occasionally stepping on Y
.x_major:
                move.w  d0,d7
                lsr.w   #1,d7                   ; Error term starts at dx/2

.x_loop:
                bsr     plot_pixel              ; Draw current pixel
                cmp.w   d5,d3                   ; Reached destination X?
                beq.s   .line_done_pop
                add.w   2(sp),d3                ; Step X by sx
                sub.w   d1,d7                   ; Subtract dy from error
                bge.s   .x_loop                 ; If error >= 0, continue
                add.w   d0,d7                   ; Add dx to error
                add.w   (sp),d4                 ; Step Y by sy
                bra.s   .x_loop

.line_done_pop:
                addq.l  #4,sp                   ; Clean up sx/sy from stack
.line_done:
                movem.l (sp)+,d0-d7/a0
                rts

; ============================================================================
; PLOT PIXEL WITH CLIPPING
; ============================================================================
; Draws a single pixel at (d3, d4) with color d2.
; Performs bounds checking to prevent writing outside video memory.
;
; This function is called for every pixel in every line, so it must be
; as fast as possible. We inline the bounds checks rather than calling
; a separate clipping function.
; ============================================================================
plot_pixel:
                ; === FAST BOUNDS CHECK ===
                ; Reject pixels outside the screen rectangle
                tst.w   d3
                bmi.s   .skip                   ; x < 0
                cmpi.w  #SCR_W,d3
                bge.s   .skip                   ; x >= 320
                tst.w   d4
                bmi.s   .skip                   ; y < 0
                cmpi.w  #SCR_H,d4
                bge.s   .skip                   ; y >= 200

                ; === CALCULATE VRAM ADDRESS AND WRITE PIXEL ===
                movem.l d0-d1/a0,-(sp)
                lea     VGA_VRAM,a0
                move.w  d4,d0
                mulu    #SCR_W,d0               ; y * 320
                add.w   d3,d0                   ; + x
                move.b  d2,(a0,d0.l)            ; Write color to VRAM
                movem.l (sp)+,d0-d1/a0
.skip:
                rts

; ============================================================================
; BUILD COPPER LIST
; ============================================================================
; Creates the copper program that will execute during each frame.
;
; === WHAT IS A COPPER? ===
; The copper (coprocessor) is a programmable device that can modify
; hardware registers synchronized to the video beam position. Originally
; from the Amiga, it enabled effects impossible with conventional hardware.
;
; === COPPER INSTRUCTIONS ===
; - WAIT: Pause until beam reaches specified position
; - MOVE: Write a value to a register
; - SETBASE: Select which device to write to (VIDEO, VGA_DAC, etc.)
; - END: Stop copper execution (restarted at next VSYNC)
;
; === THIS COPPER PROGRAM ===
; For each of 100 scanlines, we:
; 1. WAIT for the beam to reach that scanline
; 2. SETBASE to VGA_DAC (palette registers)
; 3. MOVE the palette write index to 1
; 4. MOVE R, G, B values (creating the gradient color)
;
; The actual color values are placeholders here; update_copper_colors
; fills them in each frame for animation.
; ============================================================================
build_copper_list:
                movem.l d0-d7/a0,-(sp)

                lea     copper_list,a0
                moveq   #0,d7                   ; Line counter

.build_loop:
                ; Calculate which scanline this entry targets
                move.l  d7,d0
                mulu.w  #LINES_PER_CHANGE,d0    ; Line = index * spacing

                ; Don't go past the bottom of the screen
                cmpi.l  #SCR_H,d0
                bge.s   .build_end

                ; === FIRST LINE: NO WAIT ===
                ; The first entry starts at the top of screen (no wait needed)
                tst.l   d7
                beq.s   .no_wait

                ; === INSERT WAIT INSTRUCTION ===
                ; SETBASE to VIDEO (for WAIT - waits use video beam position)
                move.l  #COP_SETBASE_VIDEO,(a0)+

                ; WAIT for scanline (format: scanline << 12)
                lsl.l   #8,d0                   ; scanline * 256
                lsl.l   #4,d0                   ; * 16 (total * 4096)
                move.l  d0,(a0)+                ; WAIT instruction

.no_wait:
                ; === INSERT PALETTE WRITE INSTRUCTIONS ===
                ; SETBASE to VGA_DAC (palette registers)
                move.l  #COP_SETBASE_VGA_DAC,(a0)+

                ; Select palette index 1 for writing
                move.l  #COP_MOVE_VGA_WINDEX,(a0)+
                move.l  #1,(a0)+                ; Palette entry 1

                ; Write R, G, B values (placeholders - updated each frame)
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
                ; === TERMINATE COPPER PROGRAM ===
                move.l  #COP_END,(a0)+

                movem.l (sp)+,d0-d7/a0
                rts

; ============================================================================
; UPDATE COPPER COLORS
; ============================================================================
; Modifies the copper list with new color values each frame.
; This creates the animated "tube" effect where colors scroll vertically.
;
; === THE TUBE EFFECT ===
; We use a sine wave to modulate brightness, creating the illusion of a
; 3D tube or cylinder. The center of each "bar" is brightest, fading
; to darkness at the edges.
;
; === HSV-STYLE COLOR GENERATION ===
; Instead of storing RGB values directly, we calculate colors from:
; - HUE: Which color in the rainbow (determined by vertical position + phase)
; - SATURATION: Always full (pure colors)
; - VALUE (brightness): Determined by sine wave for tube effect
;
; The hue-to-RGB conversion uses phase-shifted sine waves:
; - Red   = sin(hue + 0°)   * brightness
; - Green = sin(hue + 120°) * brightness  (120° = 85 in 256-unit system)
; - Blue  = sin(hue + 240°) * brightness  (240° = 170 in 256-unit system)
;
; === MEMORY LAYOUT OF COPPER LIST ===
; Line 0: SETBASE(4) + MOVE_WINDEX(4) + 1(4) + MOVE_DATA(4) + R(4) + MOVE_DATA(4) + G(4) + MOVE_DATA(4) + B(4) = 36 bytes
; Line 1+: SETBASE_VIDEO(4) + WAIT(4) + (same as line 0) = 44 bytes
; ============================================================================
update_copper_colors:
                movem.l d0-d7/a0-a2,-(sp)

                lea     copper_list,a0
                lea     sin_table,a1
                move.l  raster_phase,d6         ; Animation phase (0-255)
                moveq   #0,d7                   ; Line counter

.update_loop:
                ; === CALCULATE POINTER TO R VALUE ===
                ; The copper list has different structure for line 0 vs others
                tst.l   d7
                bne.s   .not_line0

                ; Line 0: R value is at offset 16 (no WAIT instruction)
                lea     16(a0),a2
                bra.s   .calc_color

.not_line0:
                ; Lines 1+: Calculate offset accounting for WAIT instruction
                ; Offset = 36 (line 0 size) + (line-1) * 44 + 24 (R offset after WAIT)
                move.l  d7,d0
                subq.l  #1,d0
                mulu.w  #44,d0                  ; 44 bytes per line (with WAIT)
                addi.l  #36+24,d0               ; Base offset + R offset
                lea     (a0,d0.l),a2

.calc_color:
                ; =======================================================
                ; GRADIENT CALCULATION - THE HEART OF THE RASTER EFFECT
                ; =======================================================

                ; === STEP 1: DETERMINE COLOR GROUP ===
                ; Divide line number by 4 to create bands of similar color
                move.l  d7,d0
                lsr.l   #2,d0                   ; line / 4
                andi.l  #$1F,d0                 ; 32 color groups

                ; === STEP 2: CALCULATE BRIGHTNESS (TUBE EFFECT) ===
                ; Use sine wave to make center of each band bright, edges dark
                ; This creates the illusion of a 3D tube surface
                move.l  d7,d1
                lsl.l   #4,d1                   ; line * 16 (faster cycling)
                add.l   d6,d1                   ; Add animation phase
                andi.l  #$FF,d1                 ; Wrap to 0-255

                ; Look up brightness from sine table
                move.l  d1,d2
                add.w   d2,d2                   ; *2 for word access
                move.w  (a1,d2.w),d3            ; d3 = sine value (-256 to +256)
                bpl.s   .bright_pos
                neg.w   d3                      ; Use absolute value for brightness
.bright_pos:
                ; d3 = brightness (0-256)

                ; === STEP 3: CALCULATE HUE ===
                ; Hue is determined by color group + animation phase
                ; This makes colors scroll through the rainbow
                move.l  d0,d4                   ; Color group (0-31)
                lsl.l   #3,d4                   ; * 8 for hue spread
                add.l   d6,d4                   ; Add animation phase
                andi.l  #$FF,d4                 ; d4 = hue (0-255)

                ; =======================================================
                ; HSV TO RGB CONVERSION
                ; We use phase-shifted sine waves to generate RGB from hue
                ; This is an approximation of the standard HSV formula
                ; =======================================================

                ; === RED: sin(hue) * brightness ===
                move.l  d4,d1
                add.w   d1,d1                   ; *2 for table access
                move.w  (a1,d1.w),d2            ; sin(hue)
                bpl.s   .r_pos
                moveq   #0,d2                   ; Clamp negative to 0
                bra.s   .r_scale
.r_pos:
                mulu.w  d3,d2                   ; sin(hue) * brightness
                lsr.l   #8,d2                   ; / 256
                lsr.l   #2,d2                   ; / 4 (scale to 0-63 for VGA DAC)
.r_scale:
                andi.l  #$3F,d2                 ; Ensure 6-bit value
                move.l  d2,(a2)                 ; Store R

                ; === GREEN: sin(hue + 120°) * brightness ===
                ; 120° = 85.33... ≈ 85 in 256-unit system
                move.l  d4,d1
                addi.l  #85,d1                  ; Phase shift by ~120°
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
                move.l  d2,8(a2)                ; Store G (offset 8 bytes from R)

                ; === BLUE: sin(hue + 240°) * brightness ===
                ; 240° = 170.66... ≈ 170 in 256-unit system
                move.l  d4,d1
                addi.l  #170,d1                 ; Phase shift by ~240°
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
                move.l  d2,16(a2)               ; Store B (offset 16 bytes from R)

                addq.l  #1,d7
                cmpi.l  #NUM_COPPER_LINES,d7
                blt     .update_loop

                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; DATA SECTION
; ============================================================================
; All runtime data and lookup tables.
;
; NOTE: "even" directive ensures word/long alignment. Unaligned access
; on M68K is legal but slower (and can cause bus errors on some variants).
; ============================================================================

                even

; --- Animation State Variables ---
; These are modified each frame to animate the demo.
angle_x:        dc.l    0               ; Cube rotation around X axis (0-255)
angle_y:        dc.l    0               ; Cube rotation around Y axis (0-255)
sin_x:          dc.w    0               ; Precomputed sin(angle_x)
cos_x:          dc.w    0               ; Precomputed cos(angle_x)
sin_y:          dc.w    0               ; Precomputed sin(angle_y)
cos_y:          dc.w    0               ; Precomputed cos(angle_y)
scroll_angle:   dc.l    0               ; Scroller rotation position (0-255)
scroll_char_offset: dc.l 0              ; Which message character is at position 0
raster_phase:   dc.l    0               ; Copper color animation phase (0-255)

; ============================================================================
; COPPER LIST BUFFER
; ============================================================================
; Reserved space for the dynamically-built copper program.
; Maximum size calculation:
;   Line 0: 36 bytes
;   Lines 1-99: 99 * 44 = 4356 bytes
;   END instruction: 4 bytes
;   Total: ~4400 bytes
; We round up to 4800 bytes (1200 longs) for safety.
; ============================================================================
                even
copper_list:    ds.l    1200            ; Reserve 4800 bytes

; ============================================================================
; 3D CUBE GEOMETRY DATA
; ============================================================================
; The cube is defined as 8 vertices (corners) in 3D space.
; Each vertex is three 16-bit signed integers (X, Y, Z).
; The cube is centered at the origin with half-width = CUBE_SIZE.
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
; Vertices are numbered 0-7 as shown above.
; ============================================================================
cube_vertices:
                dc.w    -CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 0: front-bottom-left
                dc.w     CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 1: front-bottom-right
                dc.w     CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 2: front-top-right
                dc.w    -CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 3: front-top-left
                dc.w    -CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 4: back-bottom-left
                dc.w     CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 5: back-bottom-right
                dc.w     CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 6: back-top-right
                dc.w    -CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 7: back-top-left

; ============================================================================
; EDGE LIST
; ============================================================================
; Each edge is a pair of vertex indices to connect with a line.
; 12 edges total: 4 on front face, 4 on back face, 4 connecting front to back.
; ============================================================================
edge_list:
                ; Front face edges (vertices 0-1-2-3)
                dc.b    0, 1            ; Bottom edge
                dc.b    1, 2            ; Right edge
                dc.b    2, 3            ; Top edge
                dc.b    3, 0            ; Left edge

                ; Back face edges (vertices 4-5-6-7)
                dc.b    4, 5            ; Bottom edge
                dc.b    5, 6            ; Right edge
                dc.b    6, 7            ; Top edge
                dc.b    7, 4            ; Left edge

                ; Connecting edges (front to back)
                dc.b    0, 4            ; Bottom-left
                dc.b    1, 5            ; Bottom-right
                dc.b    2, 6            ; Top-right
                dc.b    3, 7            ; Top-left

                even

; --- Projected 2D Coordinates ---
; These are filled in by transform_vertices each frame.
projected_x:    ds.w    NUM_VERTICES    ; Screen X for each vertex
projected_y:    ds.w    NUM_VERTICES    ; Screen Y for each vertex

; ============================================================================
; SINE TABLE (256 ENTRIES, 8.8 FIXED-POINT)
; ============================================================================
; Precomputed sine values for angles 0-255 (representing 0° to 360°).
; Values range from -256 to +256 (representing -1.0 to +1.0).
;
; WHY A LOOKUP TABLE?
; Computing sine requires Taylor series or CORDIC algorithm - expensive!
; A 512-byte table gives instant results with no calculation.
;
; HOW TO USE:
;   angle = 0-255 (where 256 would equal 360°, wrapping to 0)
;   sin_value = sin_table[angle * 2]  (multiply by 2 for word access)
;   cos_value = sin_table[(angle + 64) * 2]  (cosine is sine shifted by 90°)
;
; The values below were generated by: round(sin(i * 2π / 256) * 256)
; ============================================================================
sin_table:
                ; Quadrant 1: 0° to 90° (indices 0-63, values rise 0 to 256)
                dc.w    0, 6, 12, 18, 25, 31, 37, 43
                dc.w    49, 56, 62, 68, 74, 80, 86, 92
                dc.w    97, 103, 109, 115, 120, 126, 131, 136
                dc.w    142, 147, 152, 157, 162, 167, 171, 176
                dc.w    181, 185, 189, 193, 197, 201, 205, 209
                dc.w    212, 216, 219, 222, 225, 228, 231, 234
                dc.w    236, 238, 241, 243, 245, 247, 248, 250
                dc.w    251, 252, 253, 254, 255, 255, 256, 256

                ; Quadrant 2: 90° to 180° (indices 64-127, values fall 256 to 0)
                dc.w    256, 256, 256, 255, 255, 254, 253, 252
                dc.w    251, 250, 248, 247, 245, 243, 241, 238
                dc.w    236, 234, 231, 228, 225, 222, 219, 216
                dc.w    212, 209, 205, 201, 197, 193, 189, 185
                dc.w    181, 176, 171, 167, 162, 157, 152, 147
                dc.w    142, 136, 131, 126, 120, 115, 109, 103
                dc.w    97, 92, 86, 80, 74, 68, 62, 56
                dc.w    49, 43, 37, 31, 25, 18, 12, 6

                ; Quadrant 3: 180° to 270° (indices 128-191, values fall 0 to -256)
                dc.w    0, -6, -12, -18, -25, -31, -37, -43
                dc.w    -49, -56, -62, -68, -74, -80, -86, -92
                dc.w    -97, -103, -109, -115, -120, -126, -131, -136
                dc.w    -142, -147, -152, -157, -162, -167, -171, -176
                dc.w    -181, -185, -189, -193, -197, -201, -205, -209
                dc.w    -212, -216, -219, -222, -225, -228, -231, -234
                dc.w    -236, -238, -241, -243, -245, -247, -248, -250
                dc.w    -251, -252, -253, -254, -255, -255, -256, -256

                ; Quadrant 4: 270° to 360° (indices 192-255, values rise -256 to 0)
                dc.w    -256, -256, -256, -255, -255, -254, -253, -252
                dc.w    -251, -250, -248, -247, -245, -243, -241, -238
                dc.w    -236, -234, -231, -228, -225, -222, -219, -216
                dc.w    -212, -209, -205, -201, -197, -193, -189, -185
                dc.w    -181, -176, -171, -167, -162, -157, -152, -147
                dc.w    -142, -136, -131, -126, -120, -115, -109, -103
                dc.w    -97, -92, -86, -80, -74, -68, -62, -56
                dc.w    -49, -43, -37, -31, -25, -18, -12, -6

; ============================================================================
; BITMAP FONT (8x8 pixels, ASCII 32-126)
; ============================================================================
; Each character is 8 bytes, each byte is one row of 8 pixels.
; Bit 7 (MSB) is the leftmost pixel, bit 0 is the rightmost.
;
; Example: 'A' at ASCII 65 (font index 33)
;   Row 0: $38 = 00111000 =    ***
;   Row 1: $6C = 01101100 =   ** **
;   Row 2: $C6 = 11000110 =  **   **
;   Row 3: $FE = 11111110 =  *******
;   Row 4: $C6 = 11000110 =  **   **
;   Row 5: $C6 = 11000110 =  **   **
;   Row 6: $C6 = 11000110 =  **   **
;   Row 7: $00 = 00000000 =
;
; To find a character: font_address = simple_font + (ASCII - 32) * 8
; ============================================================================

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

; ============================================================================
; SCROLL MESSAGE
; ============================================================================
; The text that orbits around the screen. Padded with spaces for smooth
; wrapping at the beginning/end.
; ============================================================================
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
; SID MUSIC DATA - ACTIVE 6502 CODE EXECUTION
; ============================================================================
;
; === THIS IS NOT JUST DATA - IT'S EXECUTABLE 6502 CODE ===
;
; A .SID file is NOT a simple audio format like WAV or MP3. It contains:
;   1. A small header with metadata (load address, init address, play address)
;   2. ACTUAL 6502 MACHINE CODE - the original C64 music player routine
;   3. Music data (patterns, instruments) that the player code interprets
;
; The Intuition Engine has a REAL 6502 CPU CORE that executes this code!
;
; === HOW SID PLAYBACK WORKS ===
;
;   ┌─────────────────────────────────────────────────────────────┐
;   │                 INTUITION ENGINE ARCHITECTURE               │
;   │                                                             │
;   │  ┌──────────────┐     ┌──────────────┐     ┌────────────┐  │
;   │  │   M68020     │     │    6502      │     │   NATIVE   │  │
;   │  │  (main CPU)  │────►│  CPU CORE    │────►│   SYNTH    │  │
;   │  │              │     │              │     │   ENGINE   │  │
;   │  └──────────────┘     └──────────────┘     └────────────┘  │
;   │        │                    │                    │         │
;   │        │ start_sid_loop     │ Executes real      │ Audio   │
;   │        │ macro triggers     │ 6502 code from     │ output  │
;   │        │ SID playback       │ the .SID file      │         │
;   │        ▼                    ▼                    ▼         │
;   │  Sets SID_PLAY_PTR    Runs init routine    Generates       │
;   │  and SID_PLAY_LEN     then play routine    waveforms       │
;   │                       ~50x per second                      │
;   └─────────────────────────────────────────────────────────────┘
;
; === THE SID REGISTER REMAPPING TRICK ===
;
; On a real C64, the SID chip is memory-mapped at $D400-$D41C.
; When 6502 code writes to these addresses, the SID produces sound.
;
; The Intuition Engine does NOT emulate the SID chip's analog circuitry.
; Instead, it INTERCEPTS writes to SID registers and REMAPS them to our
; native synthesizer engine. This approach has several advantages:
;
;   1. ACCURACY: The original 6502 player code runs unmodified
;   2. COMPATIBILITY: Works with any .SID file from the HVSC collection
;   3. QUALITY: Our synth can exceed original SID fidelity
;   4. EFFICIENCY: No cycle-accurate analog circuit emulation needed
;
; === SID REGISTER TO NATIVE SYNTH MAPPING ===
;
; The SID has 3 voices, each with these registers (repeated at +7 byte offsets):
;
;   $D400 + voice*7 + 0: Frequency Low    → Synth oscillator frequency
;   $D400 + voice*7 + 1: Frequency High   → (combined with low byte)
;   $D400 + voice*7 + 2: Pulse Width Low  → Pulse wave duty cycle
;   $D400 + voice*7 + 3: Pulse Width High → (combined with low byte)
;   $D400 + voice*7 + 4: Control Register → Waveform select, gate, sync, ring
;   $D400 + voice*7 + 5: Attack/Decay     → ADSR envelope generator
;   $D400 + voice*7 + 6: Sustain/Release  → ADSR envelope generator
;
; Global registers:
;   $D415: Filter Cutoff Low   → Native filter cutoff frequency
;   $D416: Filter Cutoff High  → (combined with low byte)
;   $D417: Filter Control      → Resonance, voice routing
;   $D418: Volume/Filter Mode  → Master volume, filter type (LP/BP/HP)
;
; Our native synth interprets these writes and generates equivalent audio:
;
;   SID Waveform Bits    Native Synth Action
;   ─────────────────    ───────────────────────────────────
;   Bit 0: Gate          Start/stop envelope generator
;   Bit 1: Sync          Hard-sync oscillator to voice 3/1/2
;   Bit 2: Ring Mod      Ring modulate with voice 3/1/2
;   Bit 4: Triangle      Generate triangle waveform
;   Bit 5: Sawtooth      Generate sawtooth waveform
;   Bit 6: Pulse         Generate pulse waveform (use duty cycle)
;   Bit 7: Noise         Generate pseudo-random noise (LFSR)
;
; === WHY THIS MATTERS FOR DEMO PROGRAMMERS ===
;
; This architecture means you can:
;   1. Use the massive HVSC (High Voltage SID Collection) library
;   2. Play authentic C64 music without porting player code
;   3. Mix 6502-driven music with M68020/Z80 main code
;   4. The 6502 runs in parallel - no main CPU overhead for music
;
; The 6502 core is clocked independently and the play routine is called
; at approximately 50Hz (PAL) or 60Hz (NTSC) to update the SID registers,
; just like on the original C64 hardware.
;
; === EXAMPLE: WHAT HAPPENS WHEN THIS DEMO RUNS ===
;
; 1. M68020 sets SID_PLAY_PTR to point to sid_data
; 2. M68020 sets SID_PLAY_LEN to the file size
; 3. start_sid_loop macro tells the audio subsystem to begin
; 4. Audio subsystem parses .SID header, finds load/init/play addresses
; 5. 6502 CPU loads the code at the correct address
; 6. 6502 CPU calls the init routine once (sets up music state)
; 7. Every frame, 6502 CPU calls the play routine
; 8. Play routine writes to $D400-$D418 (SID registers)
; 9. Those writes are intercepted and sent to native synth
; 10. Native synth generates audio samples
; 11. Main M68020 code continues running completely unaware of this!
;
; "Edge of Disgrace" by Booze Design is a famous C64 demo with excellent
; music composed by Goto80. The player code is highly optimized 6502.
; ============================================================================
sid_data:
                incbin  "../testdata/sid/Edge_of_Disgrace.sid"
sid_data_end:

                end
