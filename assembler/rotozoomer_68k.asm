; ============================================================================
; ROTOZOOMER DEMO - HARDWARE MODE7 AFFINE TEXTURE MAPPING
; M68020 Assembly for IntuitionEngine - VideoChip Mode 0 (640x480x32bpp)
; ============================================================================
;
; SDK REFERENCE IMPLEMENTATION - HARDWARE-ACCELERATED TEXTURE ROTATION
; This file is heavily commented to teach Mode7 blitter programming concepts.
;
; === WHAT THIS DEMO DOES ===
; Displays a 256x256 checkerboard texture that continuously rotates and zooms,
; filling the entire 640x480 screen. The texture wraps infinitely in all
; directions, creating the classic "rotozoomer" effect seen in countless
; demoscene productions since the early 1990s.
;
; === WHY THE ROTOZOOMER MATTERS (HISTORICAL CONTEXT) ===
; The rotozoomer (rotating + zooming texture mapper) became a demoscene
; staple because it demonstrates real-time affine texture mapping -- the same
; mathematical operation that powered SNES Mode 7 in games like F-Zero,
; Super Mario Kart, and Pilotwings. On the SNES, Mode 7 was implemented in
; dedicated silicon. On home computers like the Amiga and Atari ST, coders
; had to compute the transformation entirely in software, making it a
; benchmark for optimization skill.
;
; The Intuition Engine provides a hardware Mode7 blitter, allowing us to
; achieve the effect with minimal CPU work: we compute just 6 parameters per
; frame, and the blitter handles all 307,200 pixels (640x480).
;
; === KEY TECHNIQUES DEMONSTRATED ===
;
; 1. Mode7 hardware blitter for affine texture mapping
; 2. Pre-computed sine and reciprocal lookup tables
; 3. 8.8 fixed-point fractional animation accumulators
; 4. Double buffering with BLIT COPY for tear-free display
; 5. Two-phase vsync synchronization
; 6. Checkerboard texture generation via hardware BLIT FILL
; 7. TED music playback (Commodore Plus/4 sound chip)
;
; === ARCHITECTURE OVERVIEW ===
;
;   +---------------------------------------------------------------+
;   |                    MAIN LOOP (60 FPS)                         |
;   |                                                               |
;   |  +-------------+    +-------------+    +----------------+     |
;   |  | compute     |--->| render      |--->| blit to front  |     |
;   |  | frame params|    | Mode7       |    | (double buffer)|     |
;   |  +-------------+    +-------------+    +----------------+     |
;   |        ^                                       |              |
;   |        |            +-------------+            |              |
;   |        +------------| advance     |<-----------+              |
;   |                     | animation   |   +-----------+           |
;   |                     +-------------+<--| wait      |           |
;   |                                       | vsync     |           |
;   |                                       +-----------+           |
;   +---------------------------------------------------------------+
;
;   CPU work per frame: ~50 instructions (table lookups + shifts)
;   Blitter work per frame: 307,200 pixels (affine-mapped) + 307,200 pixels (copy)
;   Audio: TED music runs independently via player subsystem
;
; === MEMORY MAP ===
;
;   Address      Size      Purpose
;   ---------    --------  ------------------------------------------
;   $001000      ~2 KB     Program code + data + tables
;   $100000      1,228,800 VRAM (front buffer, 640x480x4 bytes)
;   $500000      262,144   Texture (256x256x4 bytes, stride 1024)
;   $600000      1,228,800 Back buffer (640x480x4 bytes)
;   $FF0000      (stack)   Stack (grows downward)
;
;   Why these addresses?
;   - VRAM at $100000: Fixed by hardware (VideoChip framebuffer)
;   - Texture at $500000: Placed above VRAM end (~$22C000) with margin,
;     below back buffer. Avoids any overlap with display memory.
;   - Back buffer at $600000: Well above texture end (~$540000).
;     Mode7 renders here off-screen, then BLIT COPY transfers the
;     completed frame to VRAM atomically -- preventing visible tearing.
;
; === AFFINE TRANSFORMATION MATH ===
;
; The Mode7 blitter maps screen pixels to texture coordinates using:
;
;   u(x,y) = u0 + du_col * x + du_row * y
;   v(x,y) = v0 + dv_col * x + dv_row * y
;
; Where the 2x2 matrix [[du_col, du_row], [dv_col, dv_row]] is a
; rotation-and-scale matrix:
;
;   | du_col  du_row |   |  CA  -SA |
;   |                | = |          |
;   | dv_col  dv_row |   |  SA   CA |
;
;   CA = cos(angle) * reciprocal(scale)
;   SA = sin(angle) * reciprocal(scale)
;
; The (u0, v0) origin is offset to centre the rotation pivot at
; the middle of the screen. All values are 16.16 fixed-point.
;
; === BUILD AND RUN ===
;
; Assemble:
;   vasmm68k_mot -Fbin -m68020 -devpac -o assembler/rotozoomer_68k.ie68 assembler/rotozoomer_68k.asm
;
; Run:
;   ./bin/IntuitionEngine -m68k assembler/rotozoomer_68k.ie68
;
; ============================================================================

                include "ie68.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Texture Memory Layout ---
; The texture lives at $500000, above VRAM ($100000-~$22C000) and below
; the back buffer ($600000). This keeps all three memory regions separate
; so the blitter can read the texture while writing the back buffer without
; any aliasing conflicts.
TEXTURE_BASE    equ     $500000

; --- Double Buffer ---
; We render to an off-screen back buffer and then copy the completed frame
; to VRAM in one fast blit. Without this, the Mode7 blit would write directly
; to visible VRAM, and the user would see partially-rendered frames ("tearing").
; The BLIT COPY from back buffer to VRAM is fast enough to complete within
; the vertical blanking interval.
BACK_BUFFER     equ     $600000

; --- Screen Dimensions ---
; VideoChip Mode 0 is 640x480 pixels at 32 bits per pixel (BGRA).
; This is the native resolution of the Intuition Engine's VideoChip,
; giving us a large canvas for the rotozoomer effect.
RENDER_W        equ     640
RENDER_H        equ     480

; --- Texture Stride ---
; The texture is 256 pixels wide, and each pixel is 4 bytes (BGRA).
; Stride = 256 * 4 = 1024 bytes per row. The Mode7 blitter needs this
; to correctly step from one texture row to the next.
TEX_STRIDE      equ     1024

; --- Animation Accumulator Increments (8.8 Fixed-Point) ---
;
; WHY 8.8 FIXED-POINT ACCUMULATORS?
; We want smooth, non-integer stepping through lookup tables. BASIC uses
; floating-point increments (A+=0.03 for rotation, SI+=0.01 for zoom),
; but M68K integer code needs a fixed-point equivalent.
;
; The conversion from BASIC's radians-per-frame to table-entries-per-frame:
;   - Rotation: 0.03 * 256 / (2*pi) = ~1.2225 entries/frame
;   - Zoom:     0.01 * 256 / (2*pi) = ~0.4075 entries/frame
;
; With 8.8 fixed-point (multiply by 256 for sub-entry precision):
;   - ANGLE_INC = 1.2225 * 256 = ~313
;   - SCALE_INC = 0.4075 * 256 = ~104
;
; This gives us a 3:1 ratio between rotation and zoom speed, matching
; the BASIC version. The high byte of the accumulator is the table index,
; the low byte is fractional precision that accumulates over many frames.
; This produces smooth animation without any floating-point hardware.
ANGLE_INC       equ     313
SCALE_INC       equ     104

; ============================================================================
; ENTRY POINT
; ============================================================================
; The CPU begins execution here after loading the program.
; We initialize the video hardware, generate the texture, set up animation
; state, start music playback, and then enter the main loop.
; ============================================================================

                org     PROGRAM_START

start:
                ; Initialize stack pointer. M68K uses a descending stack
                ; (grows toward lower addresses). Must be set before any
                ; BSR/JSR calls.
                move.l  #STACK_TOP,sp

                ; --- Enable VideoChip ---
                ; The VideoChip must be enabled before any VRAM writes become
                ; visible. Mode 0 is the default 640x480x32bpp mode.
                ; CRITICAL: The blitter is part of the VideoChip subsystem,
                ; so it won't function until VIDEO_CTRL is set to 1.
                move.l  #1,VIDEO_CTRL
                move.l  #0,VIDEO_MODE

                ; --- Generate Checkerboard Texture ---
                ; WHY A CHECKERBOARD?
                ; The high-contrast black/white pattern makes the rotation
                ; and zoom clearly visible. When the texture is static, it's
                ; hard to perceive the effect. The checkerboard's sharp edges
                ; and alternating colours make even subtle rotations obvious.
                ; It's also the simplest non-trivial texture to generate
                ; procedurally -- just 4 BLIT FILL calls.
                bsr     generate_texture

                ; --- Initialize Animation Accumulators ---
                ; Both accumulators start at zero. They will be incremented
                ; by ANGLE_INC and SCALE_INC each frame, with the high byte
                ; used as the table index and the low byte as fractional
                ; sub-index precision.
                clr.l   angle_accum
                clr.l   scale_accum

                ; --- Start TED Music Playback ---
                ; WHY TED?
                ; Each Intuition Engine demo showcases a different audio chip.
                ; The M68K rotozoomer uses the TED (Commodore Plus/4's sound
                ; chip, the MOS 7360/8360). The TED has 2 square-wave channels
                ; and provides a distinctive 8-bit sound. Other demos use SID
                ; (C64), PSG (ZX Spectrum/MSX), or POKEY (Atari 800).
                ;
                ; TED_PLAY_CTRL value of 5 = bit 0 (start) + bit 2 (loop).
                ; This tells the player subsystem to begin playback and
                ; automatically restart when the song ends.
                move.l  #ted_data,TED_PLAY_PTR
                move.l  #ted_data_end-ted_data,TED_PLAY_LEN
                move.l  #5,TED_PLAY_CTRL

; ============================================================================
; MAIN LOOP
; ============================================================================
; This loop runs once per frame (~60 FPS with vsync).
;
; The order of operations is deliberate:
;   1. compute_frame:      Calculate the 6 Mode7 parameters for this frame
;   2. render_mode7:       Trigger the blitter to render into the back buffer
;   3. blit_to_front:      Copy completed back buffer to visible VRAM
;   4. wait_vsync:         Synchronize to vertical blank (prevents tearing)
;   5. advance_animation:  Update fractional accumulators for next frame
;
; We render BEFORE vsync so the blit-to-front happens as close to the
; blanking interval as possible, minimizing visible tearing.
; ============================================================================
main_loop:
                bsr     compute_frame
                bsr     render_mode7
                bsr     blit_to_front
                bsr     wait_vsync
                bsr     advance_animation
                bra     main_loop

; ============================================================================
; WAIT FOR VSYNC (Two-Phase Edge Detection)
; ============================================================================
; Synchronizes to the start of the vertical blanking interval.
;
; WHY TWO PHASES?
; A naive "wait until vblank bit is set" approach has a race condition:
; if we check during an ongoing vblank, we'll proceed immediately without
; waiting for the NEXT frame. Two-phase detection solves this:
;
;   Phase 1: Wait for vblank to END (bit becomes 0)
;            This ensures we're in active display, not mid-vblank.
;
;   Phase 2: Wait for vblank to START (bit becomes 1)
;            This catches the rising edge of the next blanking interval.
;
; This guarantees exactly one frame between successive calls, regardless
; of how long or short the rendering took.
;
; The STATUS_VBLANK bit (value 2) in VIDEO_STATUS indicates whether the
; VideoChip is currently in the vertical blanking period.
; ============================================================================
wait_vsync:
                ; Phase 1: If currently in vblank, wait for it to end
.wait_end:      move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                bne.s   .wait_end
                ; Phase 2: Now wait for the next vblank to begin
.wait_start:    move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                beq.s   .wait_start
                rts

; ============================================================================
; GENERATE TEXTURE (256x256 Checkerboard via 4x BLIT FILL)
; ============================================================================
; Creates a 256x256 checkerboard texture at TEXTURE_BASE using 4 hardware
; BLIT FILL operations -- one for each 128x128 quadrant.
;
; WHY BLIT FILL INSTEAD OF SOFTWARE?
; The BLIT FILL hardware fills memory at full bus bandwidth, much faster
; than a CPU store loop. Four 128x128 fills complete nearly instantly
; compared to a software loop over 65,536 pixels.
;
; WHY A CHECKERBOARD?
; The 2x2 pattern of alternating white and black 128x128 blocks creates
; maximum visual contrast. When rotated, the diagonal boundaries between
; colours make the rotation angle obvious at any zoom level. The XOR
; alternative (pixel[x][y] = ((x^y) & 128) ? white : black) would
; produce the same result but requires per-pixel CPU computation.
;
; TEXTURE MEMORY LAYOUT (256 pixels wide, 4 bytes/pixel, stride=1024):
;
;   +----------+----------+
;   |  WHITE   |  BLACK   |  Row 0-127
;   | $FFFFFFFF| $FF000000|  (128 pixels each)
;   |          |          |
;   +----------+----------+
;   |  BLACK   |  WHITE   |  Row 128-255
;   | $FF000000| $FFFFFFFF|  (128 pixels each)
;   |          |          |
;   +----------+----------+
;
;   Base address offsets:
;     Top-left:     TEXTURE_BASE + 0        = $500000
;     Top-right:    TEXTURE_BASE + 512      = $500200  (128 pixels * 4 bytes)
;     Bottom-left:  TEXTURE_BASE + 131072   = $520000  (128 rows * 1024 stride)
;     Bottom-right: TEXTURE_BASE + 131584   = $520200  (128*1024 + 128*4)
;
;   Color format is BGRA (32-bit):
;     $FFFFFFFF = white (B=FF, G=FF, R=FF, A=FF)
;     $FF000000 = black (B=00, G=00, R=00, A=FF)
;
; Each BLIT FILL writes to BLT_STATUS bit 1 (busy) while in progress.
; We poll-wait for completion before starting the next fill to avoid
; clobbering the blitter's state mid-operation.
; ============================================================================
generate_texture:
                ; --- Top-left quadrant: 128x128 white ---
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FFFFFFFF,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w1:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w1

                ; --- Top-right quadrant: 128x128 black ---
                ; Offset = 128 pixels * 4 bytes/pixel = 512 bytes from row start
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+512,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FF000000,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w2:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w2

                ; --- Bottom-left quadrant: 128x128 black ---
                ; Offset = 128 rows * 1024 bytes/row = 131072 bytes
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+131072,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FF000000,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w3:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w3

                ; --- Bottom-right quadrant: 128x128 white ---
                ; Offset = 131072 (128 rows) + 512 (128 pixels) = 131584
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+131584,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FFFFFFFF,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w4:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w4

                rts

; ============================================================================
; COMPUTE FRAME - Calculate Mode7 Parameters from Animation State
; ============================================================================
; This is the CPU's only per-frame work: convert the current rotation angle
; and zoom scale into the 6 Mode7 blitter parameters (u0, v0, CA, SA, -SA, CA).
;
; === ALGORITHM ===
;
; 1. Extract table indices from 8.8 accumulators (high byte = index)
; 2. Look up cos(angle), sin(angle) from sine table
; 3. Look up reciprocal zoom factor from reciprocal table
; 4. Compute CA = cos * recip, SA = sin * recip  (via MULS.W)
; 5. Compute u0, v0 centreing offsets
; 6. Store all 6 parameters for render_mode7 to use
;
; === WHY MULS.W IS SUFFICIENT ===
; M68K's MULS.W gives signed 16x16 -> 32 in a single instruction.
; Our sine values range from -256 to +256 (fits in 16 bits signed).
; Our reciprocal values range from 320 to 1280 (fits in 16 bits unsigned).
; The product is at most 256*1280 = 327,680 which fits in 32 bits.
; The result is naturally in 16.16 fixed-point format because:
;   sine (8.8) * reciprocal (8.8) = product (16.16)
; This is exactly the format the Mode7 blitter expects.
;
; === THE u0/v0 CENTERING FORMULA ===
;
; The Mode7 blitter starts sampling at (u0, v0) for screen pixel (0,0).
; Without centreing, the top-left screen pixel maps to the texture origin,
; and rotation happens around the top-left corner -- visually wrong.
;
; To rotate around screen centre (320, 240), we need:
;
;   u0 = tex_centre - CA * screen_half_w + SA * screen_half_h
;   v0 = tex_centre - SA * screen_half_w - CA * screen_half_h
;
; Where:
;   tex_centre = 128 << 16 = 8388608  (centre of 256-texel map, in 16.16 FP)
;   screen_half_w = 320  (640 / 2)
;   screen_half_h = 240  (480 / 2)
;
; The signs come from inverting the affine transformation: we want the
; screen centre to map to the texture centre, then work backwards to find
; where screen pixel (0,0) maps.
;
; The multiplications by 320 and 240 are done via shift decomposition
; to avoid needing a multiply instruction for large values:
;   320 = 256 + 64  -> (val << 8) + (val << 6)
;   240 = 256 - 16  -> (val << 8) - (val << 4)
; ============================================================================
compute_frame:
                movem.l d0-d7,-(sp)

                ; --- Extract Table Indices from 8.8 Accumulators ---
                ; The accumulator is a 16-bit value where:
                ;   bits 15-8 = integer part (table index, 0-255)
                ;   bits 7-0  = fractional part (sub-index precision)
                ; We shift right 8 to get the integer index, then mask to 0-255.
                move.l  angle_accum,d0
                lsr.l   #8,d0
                andi.l  #255,d0                 ; angle_idx (0-255, covers full rotation)

                move.l  scale_accum,d1
                lsr.l   #8,d1
                andi.l  #255,d1                 ; scale_idx (0-255, covers one zoom cycle)

                ; --- Look Up Cosine ---
                ; cos(angle) = sin(angle + 90 degrees)
                ; In our 256-entry system, 90 degrees = 64 entries (256/4).
                ; The AND with 255 wraps the index for table safety.
                move.l  d0,d2
                addi.l  #64,d2
                andi.l  #255,d2
                add.l   d2,d2                   ; *2 for word (16-bit) table access
                lea     sine_table(pc),a0
                move.w  (a0,d2.l),d3            ; d3 = cos_val (signed, range -256..+256)
                ext.l   d3                      ; sign-extend to 32-bit for arithmetic

                ; --- Look Up Sine ---
                move.l  d0,d2
                add.l   d2,d2                   ; *2 for word access
                move.w  (a0,d2.l),d4            ; d4 = sin_val (signed, range -256..+256)
                ext.l   d4

                ; --- Look Up Reciprocal Zoom Factor ---
                ; WHY A RECIPROCAL TABLE?
                ; The zoom effect uses: recip = round(256 / (0.5 + sin(i*2pi/256) * 0.3))
                ;
                ; The 0.5 base keeps the denominator always positive (never zero or
                ; negative, which would cause division-by-zero or inverted textures).
                ;
                ; The 0.3 amplitude gives smooth oscillation:
                ;   When sin=0:    recip = 256/0.5 = 512  (~2x zoom out)
                ;   When sin=+1:   recip = 256/0.8 = 320  (~1.25x zoom out, minimum)
                ;   When sin=-1:   recip = 256/0.2 = 1280 (~5x zoom out, maximum)
                ;
                ; Pre-computing avoids expensive runtime division. The table has
                ; 256 entries covering one full sine cycle of zoom oscillation.
                move.l  d1,d2
                add.l   d2,d2                   ; *2 for word access
                lea     recip_table(pc),a1
                move.w  (a1,d2.l),d5            ; d5 = recip (unsigned, range 320..1280)
                andi.l  #$FFFF,d5               ; zero-extend (values are all positive)

                ; --- Compute CA = cos_val * recip ---
                ; This is the core of the affine transformation.
                ; cos_val is signed (-256..+256), recip is unsigned (320..1280).
                ; MULS.W treats both operands as signed 16-bit, giving a signed
                ; 32-bit result. Since recip fits in signed 16-bit range (max 1280),
                ; this works correctly.
                ; The result is naturally 16.16 fixed-point:
                ;   cos_val is 8.8 (256 = 1.0) * recip is 8.8 (256 = 1.0)
                ;   product is 16.16 (65536 = 1.0)
                move.l  d3,d6
                muls.w  d5,d6                   ; d6 = CA (signed 32-bit, 16.16 FP)

                ; --- Compute SA = sin_val * recip ---
                move.l  d4,d7
                muls.w  d5,d7                   ; d7 = SA (signed 32-bit, 16.16 FP)

                ; Store CA and SA for use in render_mode7 and centreing below
                move.l  d6,var_ca
                move.l  d7,var_sa

                ; --- Compute u0 (texture U origin for screen pixel 0,0) ---
                ;
                ; u0 = 8388608 - CA*320 + SA*240
                ;
                ; Decompose the multiplications using shifts to avoid overflow:
                ;   CA * 320 = CA * 256 + CA * 64 = (CA << 8) + (CA << 6)
                ;   SA * 240 = SA * 256 - SA * 16 = (SA << 8) - (SA << 4)
                ;
                ; WHY SHIFTS INSTEAD OF MULS?
                ; CA and SA are 32-bit values (16.16 FP). MULS.W only handles
                ; 16-bit operands, and the 32-bit values could exceed 16 bits.
                ; Shift-and-add decomposition works on full 32-bit values.
                move.l  d6,d0                   ; d0 = CA
                move.l  d0,d1
                lsl.l   #8,d0                   ; CA * 256
                lsl.l   #6,d1                   ; CA * 64
                add.l   d1,d0                   ; d0 = CA * 320

                move.l  d7,d1                   ; d1 = SA
                move.l  d1,d2
                lsl.l   #8,d1                   ; SA * 256
                lsl.l   #4,d2                   ; SA * 16
                sub.l   d2,d1                   ; d1 = SA * 240

                move.l  #$800000,d3             ; 8388608 = 128 << 16 (texture centre in 16.16 FP)
                sub.l   d0,d3                   ; - CA*320 (undo horizontal offset)
                add.l   d1,d3                   ; + SA*240 (undo vertical offset, rotated)
                move.l  d3,var_u0

                ; --- Compute v0 (texture V origin for screen pixel 0,0) ---
                ;
                ; v0 = 8388608 - SA*320 - CA*240
                ;
                ; Same decomposition via shifts.
                ; The signs differ from u0 because the V axis uses the
                ; complementary rotation terms (SA for horizontal, CA for vertical).
                move.l  d7,d0                   ; SA
                move.l  d0,d1
                lsl.l   #8,d0                   ; SA * 256
                lsl.l   #6,d1                   ; SA * 64
                add.l   d1,d0                   ; d0 = SA * 320

                move.l  d6,d1                   ; CA
                move.l  d1,d2
                lsl.l   #8,d1                   ; CA * 256
                lsl.l   #4,d2                   ; CA * 16
                sub.l   d2,d1                   ; d1 = CA * 240

                move.l  #$800000,d3             ; 8388608 = texture centre in 16.16 FP
                sub.l   d0,d3                   ; - SA*320
                sub.l   d1,d3                   ; - CA*240
                move.l  d3,var_v0

                movem.l (sp)+,d0-d7
                rts

; ============================================================================
; RENDER MODE7 - Configure and Trigger the Mode7 Blitter
; ============================================================================
; Programs the blitter with the 6 affine parameters computed by
; compute_frame, then triggers the Mode7 blit operation.
;
; === MODE7 BLITTER REGISTER MATRIX ===
;
; The blitter implements the affine mapping:
;
;   For each screen pixel (x, y):
;     tex_u = u0 + du_col * x + du_row * y
;     tex_v = v0 + dv_col * x + dv_row * y
;     output_pixel = texture[(tex_v >> 16) & TEX_H_MASK]
;                           [(tex_u >> 16) & TEX_W_MASK]
;
; Our rotation matrix assigns:
;     du_col =  CA    (U changes by +CA per column step)
;     dv_col =  SA    (V changes by +SA per column step)
;     du_row = -SA    (U changes by -SA per row step)
;     dv_row =  CA    (V changes by +CA per row step)
;
; This is a standard 2D rotation matrix [[cos, -sin], [sin, cos]]
; scaled by the reciprocal zoom factor (already baked into CA and SA).
;
; The TEX_W_MASK and TEX_H_MASK (both 255) cause automatic wrapping
; of texture coordinates, creating infinite tiling in all directions.
; This is why we see the checkerboard repeating endlessly as we zoom out.
;
; All parameters are 16.16 fixed-point (high 16 bits = integer texels,
; low 16 bits = sub-texel fraction for smooth interpolation).
;
; === WHY RENDER TO BACK BUFFER? ===
; The Mode7 blit writes 307,200 pixels sequentially. If we wrote directly
; to VRAM ($100000), the display would show the blit in progress (top half
; new frame, bottom half old frame). By rendering to $600000 and then
; doing a fast BLIT COPY to VRAM, we get atomic frame updates.
; ============================================================================
render_mode7:
                ; Set blitter to Mode7 affine texture mapping operation
                move.l  #BLT_OP_MODE7,BLT_OP

                ; Source = texture at $500000, Destination = back buffer at $600000
                move.l  #TEXTURE_BASE,BLT_SRC
                move.l  #BACK_BUFFER,BLT_DST

                ; Output dimensions: full 640x480 screen
                move.l  #RENDER_W,BLT_WIDTH
                move.l  #RENDER_H,BLT_HEIGHT

                ; Source stride = texture row pitch (256 pixels * 4 bytes = 1024)
                ; Dest stride = screen row pitch (640 pixels * 4 bytes = 2560)
                move.l  #TEX_STRIDE,BLT_SRC_STRIDE
                move.l  #LINE_BYTES,BLT_DST_STRIDE

                ; Texture wrap masks: 255 means the texture is 256 texels wide/tall.
                ; The blitter ANDs texture coordinates with these masks, causing
                ; seamless wrapping. A 256x256 texture tiles infinitely in both axes.
                move.l  #255,BLT_MODE7_TEX_W
                move.l  #255,BLT_MODE7_TEX_H

                ; --- Write the 6 affine parameters (all 16.16 fixed-point) ---

                ; u0, v0: texture coordinate origin for screen pixel (0,0)
                move.l  var_u0,d0
                move.l  d0,BLT_MODE7_U0

                move.l  var_v0,d0
                move.l  d0,BLT_MODE7_V0

                ; du_col = CA: texture U delta per screen column (horizontal step)
                move.l  var_ca,d0
                move.l  d0,BLT_MODE7_DU_COL       ; du_col = CA

                ; dv_col = SA: texture V delta per screen column
                move.l  var_sa,d0
                move.l  d0,BLT_MODE7_DV_COL       ; dv_col = SA

                ; du_row = -SA: texture U delta per screen row (vertical step)
                ; Negating SA gives the rotation matrix's off-diagonal sign
                neg.l   d0                          ; -SA
                move.l  d0,BLT_MODE7_DU_ROW       ; du_row = -SA

                ; dv_row = CA: texture V delta per screen row
                move.l  var_ca,d0
                move.l  d0,BLT_MODE7_DV_ROW       ; dv_row = CA

                ; --- Trigger the blit ---
                ; Writing 1 to BLT_CTRL starts the blitter. It will process
                ; all 640*480 = 307,200 pixels, looking up each one from the
                ; texture via the affine formula.
                move.l  #1,BLT_CTRL

                ; --- Wait for completion ---
                ; BLT_STATUS bit 1 (value 2) = busy. Poll until clear.
                ; The CPU is idle during this time. In a more complex demo,
                ; you could use this time for non-blitter work (AI, physics, etc).
.wait:          move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .wait

                rts

; ============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM) - Double Buffer Flip
; ============================================================================
; Copies the completed frame from the off-screen back buffer ($600000)
; to visible VRAM ($100000) using the hardware BLIT COPY operation.
;
; WHY NOT JUST SWAP BUFFER POINTERS?
; The VideoChip always reads from VRAM at $100000 (the base address is not
; programmable like on the Amiga). So we must physically copy the pixels.
; The BLIT COPY hardware does this at full bus bandwidth, much faster than
; a CPU copy loop. For 640x480x4 = 1,228,800 bytes, the hardware blitter
; completes in a fraction of the frame time.
;
; The copy happens right before vsync, so by the time the display starts
; scanning the next frame, VRAM contains the complete new image.
; ============================================================================
blit_to_front:
                move.l  #BLT_OP_COPY,BLT_OP
                move.l  #BACK_BUFFER,BLT_SRC
                move.l  #VRAM_START,BLT_DST
                move.l  #RENDER_W,BLT_WIDTH
                move.l  #RENDER_H,BLT_HEIGHT
                move.l  #LINE_BYTES,BLT_SRC_STRIDE
                move.l  #LINE_BYTES,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL

                ; Poll-wait for blit completion
.wait:          move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .wait

                rts

; ============================================================================
; ADVANCE ANIMATION - Update Fractional Accumulators
; ============================================================================
; Increments both 8.8 fixed-point accumulators and wraps them to 16 bits.
;
; WHY 16-BIT WRAP (ANDI #$FFFF)?
; The accumulator is conceptually 8.8:
;   - Bits 15-8: integer part (table index 0-255)
;   - Bits 7-0:  fractional part (sub-index precision)
;
; Wrapping at $FFFF means the integer part wraps at 255->0, which
; naturally cycles through the full 256-entry lookup table.
; The fractional part provides smooth sub-entry stepping: with
; ANGLE_INC=313 (=$139), the index advances by 1.22 entries per frame,
; giving rotation speed that isn't locked to integer table steps.
;
; Without fractional accumulation, rotation would be either 1 entry/frame
; (too slow) or 2 entries/frame (too fast), with no in-between.
; ============================================================================
advance_animation:
                move.l  angle_accum,d0
                addi.l  #ANGLE_INC,d0
                andi.l  #$FFFF,d0
                move.l  d0,angle_accum

                move.l  scale_accum,d0
                addi.l  #SCALE_INC,d0
                andi.l  #$FFFF,d0
                move.l  d0,scale_accum

                rts

; ============================================================================
; VARIABLES (Runtime State)
; ============================================================================
; These are modified each frame by compute_frame and advance_animation.
;
; angle_accum / scale_accum:
;   8.8 fixed-point accumulators. The high byte indexes into the
;   sine_table and recip_table respectively. The low byte provides
;   fractional precision for smooth non-integer stepping.
;
; var_ca / var_sa:
;   Cached results of cos*recip and sin*recip (16.16 fixed-point).
;   These are the diagonal and off-diagonal elements of the affine
;   rotation-scale matrix. Stored between compute_frame and render_mode7
;   to avoid recomputation.
;
; var_u0 / var_v0:
;   Texture coordinate origin for screen pixel (0,0), in 16.16 FP.
;   Pre-offset so that the rotation pivot is at screen centre.
; ============================================================================
angle_accum:    dc.l    0
scale_accum:    dc.l    0
var_ca:         dc.l    0
var_sa:         dc.l    0
var_u0:         dc.l    0
var_v0:         dc.l    0

; ============================================================================
; SINE TABLE - 256 Entries, Signed 16-bit
; ============================================================================
; Pre-computed sine values: round(sin(i * 2*pi / 256) * 256)
;
; WHY PROPER SINE TABLES (NOT TRIANGLE-WAVE APPROXIMATION)?
; Earlier versions of this demo used a triangle-wave approximation:
; fast to compute but up to 29% error at 45 degrees, causing visible
; "diamond" distortion in the rotation (the texture appeared to deform
; into a rhombus at certain angles instead of rotating cleanly).
;
; These tables use true sinusoidal values, giving perfectly circular
; rotation. The 256-entry resolution covers a full 360-degree revolution
; with ~1.4-degree steps, which is smooth enough that individual angle
; increments are imperceptible at 60 FPS.
;
; FORMAT:
;   - 256 entries, each a signed 16-bit word (dc.w)
;   - Value range: -256 to +256 (representing -1.0 to +1.0 in 8.8 FP)
;   - Index 0 = sin(0) = 0
;   - Index 64 = sin(90 degrees) = 256 (maximum)
;   - Index 128 = sin(180 degrees) = 0
;   - Index 192 = sin(270 degrees) = -256 (minimum)
;
; COSINE ACCESS:
;   cos(angle) = sin(angle + 64), because 64/256 of a circle = 90 degrees.
;   No separate cosine table needed -- just offset the index.
;
; TABLE SIZE:
;   256 entries * 2 bytes = 512 bytes. A tiny memory cost for instant
;   trigonometry. Computing sin() at runtime would require either Taylor
;   series (many multiplies and divides per call) or CORDIC (iterative
;   shift-and-add), both dramatically slower than a table lookup.
; ============================================================================
sine_table:
                dc.w    0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
                dc.w    98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
                dc.w    181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
                dc.w    237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
                dc.w    256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
                dc.w    237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
                dc.w    181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
                dc.w    98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
                dc.w    0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
                dc.w    -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
                dc.w    -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
                dc.w    -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256
                dc.w    -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
                dc.w    -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
                dc.w    -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
                dc.w    -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

; ============================================================================
; RECIPROCAL TABLE - 256 Entries, Unsigned 16-bit
; ============================================================================
; Pre-computed zoom factors: round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
;
; WHY THIS FORMULA?
; The zoom effect oscillates smoothly between zoomed-in and zoomed-out.
; The formula creates a reciprocal of a sine-modulated base:
;
;   denominator = 0.5 + sin(i * 2*pi/256) * 0.3
;
;   - The 0.5 base keeps the denominator ALWAYS POSITIVE:
;     minimum = 0.5 - 0.3 = 0.2 (when sin = -1)
;     maximum = 0.5 + 0.3 = 0.8 (when sin = +1)
;     This prevents division-by-zero and negative zoom (texture inversion).
;
;   - The 0.3 amplitude gives a pleasant zoom range:
;     When sin = -1: 256/0.2 = 1280 (~5x zoom out, texture appears small)
;     When sin =  0: 256/0.5 = 512  (~2x zoom out, neutral)
;     When sin = +1: 256/0.8 = 320  (~1.25x zoom out, texture appears large)
;
;   - The 256 numerator normalizes to 8.8 fixed-point scale where 256 = 1.0
;
; VALUE RANGE: 320 to 1280 (always fits in unsigned 16-bit)
;
; WHY PRE-COMPUTE?
; Division is one of the most expensive operations on M68K (and most CPUs).
; DIVU.W takes ~140 cycles on 68000, compared to ~4 cycles for a table
; lookup. With 256 entries * 2 bytes = 512 bytes, the table is tiny.
;
; The zoom oscillation is driven by the scale_accum 8.8 accumulator,
; cycling through this table independently of the rotation angle.
; The SCALE_INC of 104 (vs ANGLE_INC of 313) means the zoom cycles
; roughly 3x slower than the rotation, creating visually interesting
; non-repeating combinations.
; ============================================================================
recip_table:
                dc.w    512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
                dc.w    416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
                dc.w    359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
                dc.w    329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
                dc.w    320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
                dc.w    329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
                dc.w    359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
                dc.w    416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
                dc.w    512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
                dc.w    665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
                dc.w    889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
                dc.w    1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
                dc.w    1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
                dc.w    1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
                dc.w    889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
                dc.w    665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; ============================================================================
; MUSIC DATA - TED (MOS 7360/8360, Commodore Plus/4)
; ============================================================================
; The TED chip provides 2 square-wave tone channels. It was used in the
; Commodore Plus/4 and C16 computers (1984). While less capable than the
; SID chip (C64), TED has a distinctive bright, buzzy sound that became
; part of the Plus/4 demoscene's identity.
;
; The .ted file is a register dump format: pre-recorded TED register
; writes at 50Hz (one frame of register values per video frame). The
; player subsystem writes these values to the TED emulation core
; automatically, producing music without any CPU intervention.
;
; The "even" directive ensures 16-bit alignment for the incbin data,
; preventing potential bus alignment issues on M68K.
; ============================================================================
                even
ted_data:
                incbin  "chromatic_admiration.ted"
ted_data_end:
