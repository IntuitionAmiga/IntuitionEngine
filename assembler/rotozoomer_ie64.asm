; =============================================================================
; ROTOZOOMER DEMO
; IE64 Assembly for IntuitionEngine - 640x480 True-Color Display
; =============================================================================
;
; REFERENCE IMPLEMENTATION FOR DEMOSCENE TECHNIQUES
; This file is heavily commented to teach demo programming concepts.
;
; === WHAT THIS DEMO DOES ===
; 1. Renders a 256x256 checkerboard texture that rotates and zooms smoothly
; 2. Fills the entire 640x480 display at true color (32-bit BGRA per pixel)
; 3. Uses double buffering to prevent visible tearing
; 4. Demonstrates the IE64 CPU's performance advantages over IE32
;
; === WHY THIS EFFECT MATTERS (HISTORICAL CONTEXT) ===
;
; THE ROTOZOOMER:
; The rotozoomer is one of the most iconic effects in the demoscene, appearing
; in demos from the early 1990s onward. It takes a small texture (typically a
; checkerboard, logo, or photograph) and maps it onto the screen with rotation
; and zoom applied simultaneously -- hence "roto" (rotation) + "zoomer" (zoom).
;
; What makes rotozoomers fascinating is that they achieve a visually complex
; result using remarkably simple math: the entire inner loop is just addition.
; The rotation and scaling are encoded into per-pixel "step" values that are
; precomputed once per frame, then the inner loop simply walks through texture
; space by adding these steps repeatedly.
;
; Classic demoscene rotozoomers appeared on platforms like the Amiga (68000),
; Atari ST, PC (386/486), and even the SNES. They were often combined with
; music, scrolltexts, and other effects in demo competitions.
;
; THE IE64 CPU:
; The IE64 is a custom 64-bit RISC processor designed for the Intuition Engine.
; It extends the IE32 architecture with features that dramatically simplify
; graphics programming:
;
;   - 3-operand instructions: "add.l r3, r1, r2" instead of accumulator style
;   - 32 general-purpose 64-bit registers (r0-r31, r0 hardwired to zero)
;   - Native signed multiply (MULS) and arithmetic shift right (ASR)
;   - Compare-and-branch in a single instruction (BNE, BEQ, BLT, etc.)
;   - Both .l (32-bit) and .q (64-bit) operation sizes
;
; These features eliminate the tedious sign-handling, accumulator-shuffling,
; and multi-instruction patterns needed on simpler architectures.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +-------------------------------------------------------------+
;   |                      INITIALIZATION                         |
;   |                                                             |
;   |  +-----------+    +-----------+    +-----------+            |
;   |  |  ENABLE   |--->| GENERATE  |--->| GENERATE  |            |
;   |  |   VIDEO   |    |  TEXTURE  |    | SINE TABLE|            |
;   |  +-----------+    +-----------+    +-----------+            |
;   |                                          |                  |
;   |  +-----------+    +-----------+          |                  |
;   |  | GENERATE  |<---| GENERATE  |<---------+                  |
;   |  | DU/DV TBL |    | RECIP TBL |                             |
;   |  +-----------+    +-----------+                             |
;   +-------------------------------------------------------------+
;              |
;              v
;   +-------------------------------------------------------------+
;   |                    MAIN LOOP (~60 FPS)                      |
;   |                                                             |
;   |  +-----------+    +-----------+    +-----------+            |
;   |  |  UPDATE   |--->|  RENDER   |--->| WAIT FOR  |            |
;   |  | ANIMATION |    | ROTOZOOMER|    |   VSYNC   |            |
;   |  +-----------+    +-----------+    +-----------+            |
;   |       ^                                  |                  |
;   |       |           +-----------+          |                  |
;   |       +-----------| BLIT TO   |<---------+                  |
;   |                   |   FRONT   |                             |
;   |                   +-----------+                             |
;   +-------------------------------------------------------------+
;
; === THE INTUITION ENGINE'S HARDWARE ===
;
; This demo uses the IE's VideoChip with its built-in blitter. The display
; is a 640x480 framebuffer at 32 bits per pixel (BGRA format). The blitter
; provides hardware-accelerated memory copies, which we use to transfer
; our completed frame from the back buffer to visible VRAM.
;
;   +----------+     +---------+     +----------+
;   |  IE64    |---->|  Back   |---->| Blitter  |---->| VRAM    |
;   |   CPU    |     |  Buffer |     | (HW DMA) |     | (Front) |
;   +----------+     +---------+     +----------+     +---------+
;        |                                                 |
;        |                                                 v
;        |                                           +-----------+
;        |                                           | VideoChip |
;        |                                           |  Display  |
;        +------ writes pixels here                  +-----------+
;
; === DOUBLE BUFFERING ===
;
; Without double buffering, the CPU would write directly to the visible
; framebuffer. If the display scans out a frame while we're mid-render,
; the top half shows the new frame and the bottom half shows the old one --
; a visible "tear" across the screen.
;
; Double buffering solves this:
;   1. CPU renders the entire frame to an off-screen "back buffer"
;   2. Wait for vertical blank (display is between frames)
;   3. Blit (copy) the completed back buffer to the visible front buffer
;   4. Repeat with the next frame
;
; The audience never sees a partially-rendered frame.
;
; =============================================================================
; IE64 OPTIMIZATIONS OVER IE32
; =============================================================================
;
; This demo was ported from an IE32 implementation. The IE64's richer ISA
; yields a 23% instruction count reduction in the critical inner loop.
; Here's what changed and WHY each IE64 feature helps:
;
; 1. 3-OPERAND INSTRUCTIONS
;    IE32: move A, r1 / add A, r2 / move r3, A   (accumulator style)
;    IE64: add.l r3, r1, r2                        (direct 3-operand)
;    Eliminates 4 accumulator-copy instructions per pixel.
;    Inner loop: 13 instructions/pixel vs IE32's 17 (24% reduction).
;
; 2. NATIVE SIGNED MULTIPLY (MULS)
;    IE32 has unsigned MUL only. Signed multiply required checking signs,
;    negating operands, multiplying, then conditionally negating the result.
;    That's 7-9 instructions replaced by a single MULS.
;    The DU/DV table generation benefits the most from this.
;
; 3. NATIVE ARITHMETIC SHIFT RIGHT (ASR)
;    Arithmetic shift preserves the sign bit (fills with copies of bit 31).
;    IE32's logical shift right fills with zeros, corrupting negative values.
;    On IE32, signed right shift required: check sign, shift, conditionally
;    OR in high bits. That's 5-7 instructions replaced by one ASR.
;
; 4. COMPARE-AND-BRANCH
;    IE32: sub r1, r2 / jnz label   (subtract sets flags, then branch)
;    IE64: bne r1, r2, label         (compare + branch in one instruction)
;    Saves 1 instruction per loop iteration.
;
; 5. EXACT CENTER OFFSET (MULS replaces shift approximation)
;    To center the rotation, we need: offset = 320 * du_dx / 256
;    IE32 approximated 320/256 as (1 + 1/4) with sign handling (~10 instr)
;    IE64 computes it exactly: muls.l + asr.l (2 instructions)
;
; =============================================================================
; HOW A ROTOZOOMER WORKS (THE MATH)
; =============================================================================
;
; === THE CORE IDEA ===
;
; A rotozoomer maps each screen pixel (sx, sy) to a texture coordinate
; (u, v) using a 2D rotation + scale matrix:
;
;   u = sx * cos(angle) / scale - sy * sin(angle) / scale + center_u
;   v = sx * sin(angle) / scale + sy * cos(angle) / scale + center_v
;
; This is an AFFINE transformation: it preserves parallel lines and
; evenly-spaced points. The texture wraps via modulo (u & 255, v & 255),
; creating the infinite tiling effect.
;
; === THE INCREMENTAL TRICK ===
;
; Computing the full formula for every pixel would be expensive.
; But notice: moving one pixel to the right (sx + 1) changes u and v by
; CONSTANT amounts:
;
;   du_dx = cos(angle) / scale     (change in u when sx increases by 1)
;   dv_dx = sin(angle) / scale     (change in v when sx increases by 1)
;
; Similarly, moving one row down (sy + 1):
;
;   du_dy = -sin(angle) / scale    (change in u when sy increases by 1)
;   dv_dy =  cos(angle) / scale    (change in v when sy increases by 1)
;
; So the inner loop just ADDS these constants to running u,v accumulators:
;
;   for each row:
;       u = row_start_u
;       v = row_start_v
;       for each column:
;           plot texture[v][u]     ; look up texel, write to screen
;           u += du_dx             ; step u by one pixel horizontally
;           v += dv_dx             ; step v by one pixel horizontally
;       row_start_u += du_dy       ; step to next row
;       row_start_v += dv_dy       ; step to next row
;
; The multiply is gone! Only additions remain in the inner loop.
; This is why rotozoomers are fast even on weak hardware.
;
; === FIXED-POINT ARITHMETIC ===
;
; The step values (du_dx, etc.) are usually fractional -- a rotation of
; 10 degrees at 2x zoom means du_dx = cos(10) / 2 = 0.4924...
;
; Computers work with integers, so we use FIXED-POINT representation:
; multiply the value by 256 (shift left 8 bits) and store the integer.
; The low 8 bits are the fractional part, the upper bits are the integer.
;
;   Fixed-point value = real_value * 256
;   0.4924 * 256 = 126  (stored as integer 126)
;
; To extract the integer part for texture lookup: value >> 8
; To mask to texture size: (value >> 8) & 255
;
; This "8.8 fixed-point" format gives us 1/256 precision (~0.004) which
; is more than enough for smooth texture mapping.
;
; === WHY du_dy = -dv_dx AND dv_dy = du_dx ===
;
; The rotation matrix is:
;
;   | cos(a)  -sin(a) |
;   | sin(a)   cos(a) |
;
; Moving right (increasing sx) uses column 1: (cos, sin)
; Moving down (increasing sy) uses column 2: (-sin, cos)
;
; So du_dy = -sin/scale = -dv_dx, and dv_dy = cos/scale = du_dx.
; We exploit this: compute du_dx and dv_dx, then derive the others
; with a single negation. No extra trig lookups needed!
;
; =============================================================================
; REGISTER ALLOCATION (Inner Loop)
; =============================================================================
;
; The inner loop processes 640 pixels per row. Every register access costs
; zero extra cycles if the value is already in a register, vs several
; instructions to load from memory. We dedicate 15 registers to loop state:
;
;   Reg | Contents                  | Why in a register
;   ----+---------------------------+----------------------------------
;    r0 | zero (hardwired)          | IE64: r0 is always 0 by design
;   r1-r4| scratch temporaries      | address calculation workspace
;   r16 | du_dx (U step per pixel)  | added EVERY pixel (640x480 times)
;   r17 | dv_dx (V step per pixel)  | added EVERY pixel
;   r18 | TEXTURE_BASE address      | used in EVERY pixel address calc
;   r19 | 255 (TEX_MASK)            | texture coord masking, EVERY pixel
;   r20 | 4 (pixel byte stride)     | VRAM pointer advance, EVERY pixel
;   r21 | du_dy (U step per row)    | added once per row (480 times)
;   r22 | current V coordinate      | running V accumulator
;   r23 | current U coordinate      | running U accumulator
;   r24 | dv_dy (V step per row)    | added once per row
;   r25 | LINE_BYTES (2560)         | VRAM row stride
;   r26 | VRAM write pointer        | incremented EVERY pixel
;   r27 | row counter (0-479)       | outer loop control
;   r28 | column counter            | inner loop control (unrolled)
;   r29 | RENDER_H (480)            | row loop termination constant
;   r30 | 40 (unrolled iterations)  | column loop termination constant
;
; Registers r5-r15 are free for the setup/animation code.
; This allocation minimizes memory traffic in the hottest code path.
;
; =============================================================================
; PERFORMANCE SUMMARY
; =============================================================================
;
; Per-pixel:       13 instructions (vs IE32's 17, a 24% reduction)
; Loop overhead:   0.125 instructions/pixel (2 per 16 unrolled pixels)
; Pixels per frame: 640 * 480 = 307,200
; Total per frame: ~4.04 million instructions (vs IE32's ~5.27M)
;
; =============================================================================
; MEMORY MAP
; =============================================================================
;
;   Address     | Size     | Contents
;   ------------+----------+-----------------------------------
;   0x001000    | ~3KB     | Program code (this file)
;   0x004000    | 256KB    | 256x256 checkerboard texture (BGRA)
;   0x044000    | 1KB      | Sine table (256 entries x 4 bytes)
;   0x044400    | 32B      | Reciprocal table (8 entries x 4)
;   0x044420    | 5KB      | DU lookup table (5 scales x 256)
;   0x045820    | 5KB      | DV lookup table (5 scales x 256)
;   0x046C20    | ~64B     | Animation variables
;   0x100000    | 1.2MB    | VRAM (front buffer, 640x480x4)
;   0x22C000    | 1.2MB    | Back buffer (off-screen render)
;
; =============================================================================
; USAGE
; =============================================================================
;
; Assemble: ./bin/ie64asm assembler/rotozoomer_ie64.asm
; Run:      ./bin/IntuitionEngine -ie64 -perf assembler/rotozoomer_ie64.ie64
;
; =============================================================================

include "ie64.inc"

; =============================================================================
; CONSTANTS
; =============================================================================

; --- Display Geometry ---
; The IE VideoChip outputs 640x480 at 32 bits per pixel (BGRA).
; Each pixel is 4 bytes, so each scanline is 640 * 4 = 2560 bytes.
; The full framebuffer is 640 * 480 * 4 = 1,228,800 bytes (~1.2 MB).
RENDER_W       equ 640
RENDER_H       equ 480
CENTER_X       equ 320           ; Screen center X (640/2)
CENTER_Y       equ 240           ; Screen center Y (480/2)

DISPLAY_W      equ 640
DISPLAY_H      equ 480
FRAME_SIZE     equ 0x12C000      ; 1,228,800 bytes (640*480*4)

; Back buffer lives above VRAM so we can render without tearing.
; VRAM_START (front buffer) is at 0x100000 (defined in ie64.inc).
; BACK_BUFFER is placed immediately after one full 640x480x4 front frame.
BACK_BUFFER    equ 0x22C000

; --- Texture Dimensions ---
; A 256x256 texture is ideal for rotozooming:
;   - Power-of-2 size allows masking instead of modulo (fast!)
;   - 256 = 2^8, matching our 8.8 fixed-point fractional precision
;   - Wraps seamlessly via (coord >> 8) & 255
;
; Each texel is 4 bytes (BGRA), so one row = 256 * 4 = 1024 = 2^10 bytes.
; TEX_ROW_SHIFT = 10 means we can compute row offset with a single shift.
TEX_SIZE       equ 256
TEX_MASK       equ 255           ; = TEX_SIZE - 1, for wrapping coords
TEX_ROW_SHIFT  equ 10            ; log2(256 * 4) = log2(1024)

; --- Fixed-Point Format (8.8) ---
; We use 8 fractional bits throughout. This means:
;   - Real value 1.0 is stored as integer 256
;   - Real value 0.5 is stored as integer 128
;   - To convert fixed-point to integer: value >> 8 (or value / 256)
;   - Precision: 1/256 = 0.00390625 (plenty for smooth animation)
FP_SHIFT       equ 8             ; Number of fractional bits
FP_ONE         equ 256           ; 1.0 in 8.8 fixed-point

; --- Memory Layout ---
; All data tables are placed above the program code in RAM.
; See the MEMORY MAP section in the file header for the full picture.
TEXTURE_BASE   equ 0x4000        ; 256x256 BGRA texture (256KB)
SINE_TABLE     equ 0x44000       ; 256-entry sine lookup (1KB)
RECIP_TABLE    equ 0x44400       ; 8-entry reciprocal lookup (32B)
DU_TABLE       equ 0x44420       ; DU[scale][angle] table (5KB)
DV_TABLE       equ 0x45820       ; DV[scale][angle] table (5KB)
VAR_BASE       equ 0x46C20       ; Animation state variables

; --- Animation State Variables ---
; These are the per-frame parameters that control rotation and zoom.
; They live in a contiguous block for easy access.
VAR_ANGLE      equ 0x46C20       ; Current rotation angle (0-255)
VAR_SCALE_IDX  equ 0x46C24       ; Scale oscillator index (0-255)
VAR_SCALE_SEL  equ 0x46C28       ; Current scale selector (0-4)
VAR_DU_DX      equ 0x46C2C       ; U step per pixel (horizontal)
VAR_DV_DX      equ 0x46C30       ; V step per pixel (horizontal)
VAR_DU_DY      equ 0x46C34       ; U step per row (vertical)
VAR_DV_DY      equ 0x46C38       ; V step per row (vertical)
VAR_ROW_U      equ 0x46C3C       ; U coord at start of current row
VAR_ROW_V      equ 0x46C40       ; V coord at start of current row
VAR_START_U    equ 0x46C44       ; U coord at texture center
VAR_START_V    equ 0x46C48       ; V coord at texture center
VAR_VRAM_ROW   equ 0x46C54       ; VRAM pointer at start of current row

; =============================================================================
; ENTRY POINT
; =============================================================================
; The IE64 begins execution at the address specified by "org".
; We initialize hardware and generate all lookup tables before entering
; the main animation loop.
;
; Table generation runs once at startup and takes a fraction of a second.
; It's a classic demoscene trade-off: spend a little time at startup to
; build lookup tables that make the per-frame rendering blazing fast.
; =============================================================================
org 0x1000

start:
    ; === ENABLE VIDEO OUTPUT ===
    ; Writing 1 to VIDEO_CTRL activates the IE VideoChip.
    ; Without this, the display stays black.
    la      r1, VIDEO_CTRL
    li      r2, #1
    store.l r2, (r1)

    ; === INITIALIZE ANIMATION STATE ===
    ; angle starts at 0 (no rotation initially).
    ; scale_idx starts at 192 (the sine table's minimum), which produces
    ; the most zoomed-in view. As the sine oscillator sweeps, the zoom
    ; breathes in and out smoothly.
    la      r1, VAR_ANGLE
    store.l r0, (r1)                    ; angle = 0 (r0 is hardwired zero)
    la      r1, VAR_SCALE_IDX
    li      r2, #192                    ; sine minimum = most zoomed in
    store.l r2, (r1)

    ; === GENERATE LOOKUP TABLES ===
    ; Build all tables in dependency order:
    ;   1. Texture -- the image we'll be rotating and zooming
    ;   2. Sine table -- needed by the DU/DV table generator
    ;   3. Reciprocal table -- also needed by DU/DV generator
    ;   4. DU/DV tables -- precomputed rotation+scale step values
    jsr     generate_texture
    jsr     generate_sine_table
    jsr     generate_recip_table
    jsr     generate_dudv_tables

; =============================================================================
; MAIN LOOP
; =============================================================================
; This loop runs forever, producing one frame per iteration.
;
; The order of operations matters:
;   1. update_animation:   advance angle and zoom, compute step values
;   2. render_rotozoomer:  fill the back buffer with the rotated texture
;   3. wait_vsync:         wait until the display is between frames
;   4. blit_to_front:      copy back buffer to visible VRAM via blitter
;
; We render BEFORE waiting for vsync so the CPU isn't idle during rendering.
; Then we wait for vsync and blit, ensuring the audience sees a clean frame.
; =============================================================================
main_loop:
    jsr     update_animation
    jsr     render_rotozoomer
    jsr     wait_vsync
    jsr     blit_to_front
    bra     main_loop

; =============================================================================
; WAIT FOR VERTICAL SYNC
; =============================================================================
; Polls the VIDEO_STATUS register until the VBLANK bit is set.
;
; WHY VSYNC?
; The display scans out the framebuffer 60 times per second (60 Hz).
; During active scanout (~93% of the time), reading VRAM is safe but
; WRITING would cause tearing. The vertical blank period (~7%) is when
; the display beam returns to the top -- this is our window to swap buffers.
;
; WHY POLL?
; The IE64 has no interrupt system in this configuration. Busy-waiting
; is simple and effective -- the CPU has nothing else to do while waiting.
; In a more complex demo with audio, we could do useful work here instead.
;
; VIDEO_STATUS BIT LAYOUT:
;   Bit 1 (STATUS_VBLANK): Set during vertical blank period
; =============================================================================
wait_vsync:
    la      r1, VIDEO_STATUS
.wait:
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK      ; isolate VBLANK bit
    beqz    r2, .wait                    ; loop until bit is set
    rts

; =============================================================================
; GENERATE TEXTURE (256x256 Checkerboard)
; =============================================================================
; Creates a 256x256 pixel checkerboard pattern in memory.
;
; === WHY A CHECKERBOARD? ===
;
; The checkerboard is the classic rotozoomer texture because:
;   1. High contrast makes rotation/zoom clearly visible
;   2. The regular grid pattern reveals the mathematical precision
;      (or imprecision!) of the texture mapping
;   3. Seamless tiling -- the pattern wraps perfectly at all edges
;   4. Trivial to generate (no external asset files needed)
;
; === HOW THE PATTERN WORKS ===
;
; For each pixel (x, y), we compute: (x ^ y) & 128
;
; XOR creates a pattern that alternates between the X and Y axes.
; Masking with 128 (bit 7) divides the space into 128-pixel blocks.
; The result: squares that are 128 pixels wide (half the texture).
;
; If we masked with 64, we'd get 64-pixel squares (4x4 grid).
; With 32, we'd get 32-pixel squares (8x8 grid), and so on.
;
; === PIXEL FORMAT ===
;
; Each pixel is stored as a 32-bit BGRA value:
;   Byte 0: Blue    (0x00 or 0xFF)
;   Byte 1: Green   (0x00 or 0xFF)
;   Byte 2: Red     (0x00 or 0xFF)
;   Byte 3: Alpha   (0xFF = fully opaque)
;
;   White = 0xFFFFFFFF (all channels max, full alpha)
;   Black = 0xFF000000 (alpha only, RGB all zero)
;
; === REGISTER USAGE ===
;   r5 = write pointer (advances through texture memory)
;   r6 = row counter (Y = 0..255)
;   r7 = loop limit (256)
;   r8 = column counter (X = 0..255)
; =============================================================================
generate_texture:
    la      r5, TEXTURE_BASE            ; r5 = write pointer
    li      r6, #0                      ; r6 = row (Y)
    move.l  r7, #TEX_SIZE               ; r7 = 256 (limit)

.row:
    li      r8, #0                      ; r8 = col (X)

.col:
    ; Determine black or white: XOR column with row, check bit 7.
    ; This creates the alternating checkerboard pattern.
    eor.l   r1, r8, r6                  ; r1 = X ^ Y
    and.l   r1, r1, #128               ; isolate bit 7 (128-pixel blocks)
    bnez    r1, .dark                   ; if bit set, dark square

    li      r2, #0xFFFFFFFF             ; white (BGRA: all 0xFF)
    bra     .store

.dark:
    li      r2, #0xFF000000             ; black (BGRA: alpha only)

.store:
    store.l r2, (r5)                    ; write 4-byte pixel to texture
    add.l   r5, r5, #4                  ; advance write pointer by 4 bytes

    add.l   r8, r8, #1                  ; next column
    bne     r8, r7, .col               ; loop while col != 256

    add.l   r6, r6, #1                  ; next row
    bne     r6, r7, .row               ; loop while row != 256
    rts

; =============================================================================
; GENERATE SINE TABLE
; =============================================================================
; Builds a 256-entry sine lookup table using piecewise linear approximation.
; Each entry is a signed 32-bit value in 8.8 fixed-point format.
;
; === WHY NOT USE A "REAL" SINE FUNCTION? ===
;
; The IE64 has no floating-point unit and no built-in trig functions.
; Even on platforms that do, a lookup table is faster than computing sine
; for every access. Demoscene coders have used table-driven trig since
; the 1980s -- it's a fundamental technique.
;
; === THE APPROXIMATION ===
;
; Real sine varies smoothly from -1 to +1 over 360 degrees.
; We approximate it with straight line segments:
;
;            +256    ___
;           /    \  /    \              <-- piecewise linear peaks
;          /      \/      \
;   ------/---0---index---255\------
;          \      /\      /
;           \    /  \    /
;            ---     ---
;           -256
;
; We divide the 256-entry cycle into four quadrants of 64 entries each:
;
;   Quadrant 0 (index   0..63):  rises from 0 to +256  (ascending)
;   Quadrant 1 (index  64..127): falls from +256 to 0  (descending)
;   Quadrant 2 (index 128..191): falls from 0 to -256  (ascending, negated)
;   Quadrant 3 (index 192..255): rises from -256 to 0  (descending, negated)
;
; The algorithm:
;   1. t = index & 63                    (position within quadrant, 0..63)
;   2. If index & 64: t = 64 - t        (mirror for descending quadrants)
;   3. value = t * 4                     (scale to 0..256 range)
;   4. Clamp value to 256               (prevent overshoot at peak)
;   5. If index & 128: value = -value   (negate for bottom half)
;
; This gives us sine values in the range [-256, +256], which is [-1.0, +1.0]
; in our 8.8 fixed-point format (since FP_ONE = 256).
;
; === COSINE FOR FREE ===
;
; Cosine is just sine shifted by 90 degrees. With 256 entries per cycle:
;   cos[angle] = sine[(angle + 64) & 255]
;
; We use this trick throughout the code instead of building a separate table.
;
; === REGISTER USAGE ===
;   r5 = write pointer into sine table
;   r6 = index counter (0..255)
;   r7 = loop limit (256)
;   r1 = working value (triangle wave, then final sine value)
; =============================================================================
generate_sine_table:
    la      r5, SINE_TABLE              ; r5 = write pointer
    li      r6, #0                      ; r6 = index
    li      r7, #256                    ; r7 = limit

.loop:
    ; Step 1: Get position within quadrant (0..63)
    and.l   r1, r6, #63                ; t = index & 63

    ; Step 2: Mirror for descending quadrants (64..127, 192..255)
    ; If bit 6 is set, we're in a descending quadrant: t = 64 - t
    and.l   r2, r6, #64
    beqz    r2, .rising                 ; skip if ascending quadrant
    li      r3, #64
    sub.l   r1, r3, r1                  ; t = 64 - t (mirror)

.rising:
    ; Step 3: Scale to sine amplitude range
    ; t ranges 0..64, we want 0..256. Multiply by 4 (shift left 2).
    lsl.l   r1, r1, #2                 ; value = t * 4

    ; Step 4: Clamp to 256 maximum
    ; At the peak (t=64), value = 256. Due to mirroring, t can be 64
    ; which gives exactly 256. Values above 256 shouldn't occur but
    ; we clamp defensively.
    li      r3, #256
    bls     r1, r3, .clamp_ok          ; branch if value <= 256
    li      r1, #256                    ; clamp to maximum
.clamp_ok:

    ; Step 5: Negate for bottom half of sine wave
    ; If bit 7 is set (index 128..255), the sine is negative.
    and.l   r2, r6, #128
    beqz    r2, .positive               ; skip if positive half
    neg.l   r1, r1                      ; value = -value

.positive:
    ; Store the computed sine value
    store.l r1, (r5)
    add.l   r5, r5, #4                  ; advance pointer by 4 bytes

    add.l   r6, r6, #1                  ; next index
    bne     r6, r7, .loop               ; loop while index != 256
    rts

; =============================================================================
; GENERATE RECIPROCAL TABLE
; =============================================================================
; Stores precomputed values of 1536 / x for x = 0..7.
;
; === WHY RECIPROCALS? ===
;
; Division is expensive on most CPUs and impossible on many retro platforms.
; IE64 does provide DIVU/DIVS/MOD, but table lookup + multiply keeps this
; setup path simple and mirrors classic demoscene precompute style:
; a/b = a * (1/b).
;
; === WHY 1536? ===
;
; The magic number 1536 comes from the desired zoom range. The scale factor
; determines how many texture pixels span each screen pixel:
;
;   step_size = 1536 / divisor
;
; Where divisor ranges from 2 to 6, giving step sizes from 768 down to 256.
; In 8.8 fixed-point, step size 256 = 1.0 (1:1 mapping, no zoom).
; Step size 768 = 3.0 (each screen pixel covers 3 texture pixels = zoomed out).
;
; The values stored here (using the table indexed by divisor):
;   recip[0] =    0  (unused, divisor=0 is invalid)
;   recip[1] = 1536  (unused, 1536/1 = very zoomed out)
;   recip[2] =  768  (1536/2 = 3.0x zoom out)  <-- scale_sel=0 uses this
;   recip[3] =  512  (1536/3 = 2.0x zoom out)  <-- scale_sel=1
;   recip[4] =  384  (1536/4 = 1.5x zoom out)  <-- scale_sel=2
;   recip[5] =  307  (1536/5 = ~1.2x zoom out) <-- scale_sel=3
;   recip[6] =  256  (1536/6 = 1.0x, no zoom)  <-- scale_sel=4
;   recip[7] =  219  (1536/7 = ~0.86x zoom in) <-- not currently used
;
; === WHY HARDCODE INSTEAD OF COMPUTE? ===
;
; With only 8 entries, hardcoding is simpler and avoids needing a division
; routine. This is a common demoscene approach for small tables.
;
; === REGISTER USAGE ===
;   r5 = base pointer to reciprocal table
;   r1 = value to store
; =============================================================================
generate_recip_table:
    la      r5, RECIP_TABLE

    li      r1, #0
    store.l r1, 0(r5)                   ; [0] = 0    (div by 0 guard)

    li      r1, #1536
    store.l r1, 4(r5)                   ; [1] = 1536 (1536/1)

    li      r1, #768
    store.l r1, 8(r5)                   ; [2] = 768  (1536/2)

    li      r1, #512
    store.l r1, 12(r5)                  ; [3] = 512  (1536/3)

    li      r1, #384
    store.l r1, 16(r5)                  ; [4] = 384  (1536/4)

    li      r1, #307
    store.l r1, 20(r5)                  ; [5] = 307  (1536/5)

    li      r1, #256
    store.l r1, 24(r5)                  ; [6] = 256  (1536/6)

    li      r1, #219
    store.l r1, 28(r5)                  ; [7] = 219  (1536/7)

    rts

; =============================================================================
; GENERATE DU/DV TABLES
; =============================================================================
; Precomputes per-pixel texture coordinate steps for all combinations of
; scale level (0..4) and rotation angle (0..255).
;
; === WHAT THESE TABLES CONTAIN ===
;
; For each (scale, angle) pair:
;   DU_TABLE[scale * 256 + angle] = recip[scale+2] * cos(angle) >> 8
;   DV_TABLE[scale * 256 + angle] = recip[scale+2] * sin(angle) >> 8
;
; These are the du_dx and dv_dx values from the rotozoomer math section.
; Precomputing them means update_animation just does a table lookup instead
; of multiply + shift per frame.
;
; === TABLE DIMENSIONS ===
;
;   5 scale levels * 256 angles = 1,280 entries per table
;   Each entry is 4 bytes (32-bit signed integer)
;   Total: 1,280 * 4 * 2 tables = 10,240 bytes (~10 KB)
;
; Scale levels 0..4 map to reciprocal table entries 2..6:
;   scale_sel=0 -> recip[2] = 768 (most zoomed out)
;   scale_sel=4 -> recip[6] = 256 (1:1 zoom, no magnification)
;
; === WHY PRECOMPUTE? ===
;
; Computing du_dx at runtime requires: multiply, shift, sign extension.
; That's ~5 instructions per frame. With 5 scales * 256 angles = 1280
; combinations, precomputing costs 1280 * ~10 instructions at startup
; but saves per-frame work and keeps update_animation simple.
;
; More importantly, it demonstrates the precomputation pattern:
; a startup cost of ~13,000 instructions to save work across thousands
; of frames is an excellent trade-off.
;
; === IE64 ADVANTAGE: MULS ===
;
; The multiply here MUST be signed because sine values range [-256, +256].
; On IE32, unsigned MUL required sign-checking both operands, conditionally
; negating, multiplying, then conditionally negating the result (7+ instr).
; IE64's MULS handles it in one instruction.
;
; === IE64 GOTCHA: SIGN EXTENSION ===
;
; load.l zero-extends: it loads 32 bits into the low half of a 64-bit
; register and fills the upper 32 bits with zeros. But muls.l reads the
; full 64-bit register value for its operands.
;
; If sine[angle] = -128, it's stored as 0xFFFFFF80 in memory (32-bit).
; After load.l, the register holds 0x00000000_FFFFFF80 -- a POSITIVE
; number in 64-bit! The muls.l would give the wrong sign.
;
; Fix: sign-extend from 32 to 64 bits after loading:
;   lsl.q r8, r8, #32   ; shift to upper 32 bits, filling low with 0
;   asr.q r8, r8, #32   ; arithmetic shift back, filling upper with sign
;
; === REGISTER USAGE ===
;   r16 = scale index (0..4)
;   r17 = current reciprocal value (scale_inv)
;   r18 = DU table write pointer for current scale
;   r19 = DV table write pointer for current scale
;   r20 = angle index (0..255)
;   r21 = angle loop limit (256)
;   r8  = loaded trig value (sign-extended for MULS)
; =============================================================================
generate_dudv_tables:
    li      r16, #0                     ; r16 = scale index (0..4)

.scale_loop:
    ; === LOOK UP RECIPROCAL VALUE FOR THIS SCALE LEVEL ===
    ; scale_sel 0..4 maps to recip_table entries 2..6.
    ; Array indexing: address = base + (scale+2) * 4
    add.l   r1, r16, #2                ; divisor index = scale + 2
    lsl.l   r1, r1, #2                 ; * 4 bytes per entry
    la      r2, RECIP_TABLE
    add.l   r1, r2, r1                 ; r1 = address of recip[scale+2]
    load.l  r17, (r1)                   ; r17 = scale_inv

    ; === COMPUTE TABLE BASE POINTERS FOR THIS SCALE ===
    ; Each scale level occupies 256 entries * 4 bytes = 1024 bytes.
    ; scale * 1024 = scale << 10
    lsl.l   r1, r16, #10               ; offset = scale * 1024
    la      r2, DU_TABLE
    add.l   r18, r2, r1                ; r18 = DU table ptr for this scale

    la      r2, DV_TABLE
    add.l   r19, r2, r1                ; r19 = DV table ptr for this scale

    li      r20, #0                     ; r20 = angle index (0..255)
    li      r21, #256                   ; r21 = limit

.angle_loop:
    ; === COMPUTE DU = scale_inv * cos(angle) >> 8 ===

    ; Cosine via phase-shifted sine: cos(a) = sin(a + 64)
    ; With 256 entries per full cycle, 64 = 90 degrees.
    add.l   r1, r20, #64               ; a + 64
    and.l   r1, r1, #255               ; wrap to 0..255
    lsl.l   r1, r1, #2                 ; * 4 bytes per entry
    la      r2, SINE_TABLE
    add.l   r1, r2, r1                 ; address of sine[a+64]
    load.l  r8, (r1)                    ; r8 = cos[angle] (32-bit signed)

    ; Sign-extend from 32 to 64 bits (see IE64 GOTCHA above)
    lsl.q   r8, r8, #32
    asr.q   r8, r8, #32

    ; Signed multiply + fixed-point shift:
    ; du = (scale_inv * cos) / 256 = (scale_inv * cos) >> 8
    muls.l  r1, r17, r8                ; r1 = scale_inv * cos (signed)
    asr.l   r1, r1, #8                 ; convert from 8.8 to integer
    store.l r1, (r18)                   ; store DU for this (scale, angle)

    ; === COMPUTE DV = scale_inv * sin(angle) >> 8 ===

    ; Direct sine lookup (no phase shift needed)
    lsl.l   r1, r20, #2                ; angle * 4 bytes per entry
    la      r2, SINE_TABLE
    add.l   r1, r2, r1                 ; address of sine[angle]
    load.l  r8, (r1)                    ; r8 = sin[angle] (32-bit signed)

    ; Sign-extend from 32 to 64 bits
    lsl.q   r8, r8, #32
    asr.q   r8, r8, #32

    ; Signed multiply + fixed-point shift
    muls.l  r1, r17, r8                ; r1 = scale_inv * sin (signed)
    asr.l   r1, r1, #8                 ; convert from 8.8 to integer
    store.l r1, (r19)                   ; store DV for this (scale, angle)

    ; === ADVANCE TO NEXT ANGLE ===
    add.l   r18, r18, #4               ; next DU entry
    add.l   r19, r19, #4               ; next DV entry
    add.l   r20, r20, #1               ; next angle
    bne     r20, r21, .angle_loop       ; loop while angle != 256

    ; === NEXT SCALE LEVEL ===
    add.l   r16, r16, #1
    li      r1, #5
    bne     r16, r1, .scale_loop        ; loop while scale != 5

    rts

; =============================================================================
; UPDATE ANIMATION
; =============================================================================
; Advances the rotation angle and zoom level, then looks up the precomputed
; du_dx and dv_dx values from the DU/DV tables.
;
; === ANIMATION PARAMETERS ===
;
; Two independent oscillators drive the effect:
;
;   1. ROTATION: angle increments by 1 each frame, wrapping 0..255.
;      With 256 steps per full rotation and ~60 FPS, one full rotation
;      takes about 4.3 seconds -- fast enough to be visually dramatic.
;
;   2. ZOOM: scale_idx increments by 2 each frame, also wrapping 0..255.
;      This index looks up a sine value to smoothly oscillate the zoom
;      level. The sine makes the zoom "breathe" -- smoothly easing between
;      zoomed in and zoomed out with no sudden jumps.
;      Speed is 2 per frame (vs angle's 1), so zoom cycles ~2x faster.
;
; The combination of independent rotation and zoom creates the hypnotic,
; ever-changing visual pattern that defines the rotozoomer effect.
;
; === SCALE CALCULATION ===
;
; The zoom level follows this pipeline:
;   1. Look up sin(scale_idx) from the sine table         (range: -256..+256)
;   2. Halve it: sin >> 1                                  (range: -128..+128)
;   3. Add 205: scale = 205 + (sin >> 1)                   (range: 77..333)
;   4. Convert to scale selector: (scale >> 6) - 1         (range: 0..4)
;   5. Clamp to [0, 4]                                     (safety bounds)
;
; The base value 205 centers the oscillation at scale ~3.2 in fixed-point.
; The sine adds ±128, sweeping from 77 (zoomed out) to 333 (zoomed in).
;
; === DERIVING du_dy AND dv_dy ===
;
; From the rotation matrix properties (see "HOW A ROTOZOOMER WORKS"):
;   du_dy = -dv_dx    (negation only -- no extra trig lookup)
;   dv_dy =  du_dx    (direct copy -- no computation at all)
;
; This exploits the orthogonality of the rotation matrix columns.
; =============================================================================
update_animation:
    ; === ADVANCE ROTATION ANGLE ===
    ; Increment by 1, wrap at 255 using bitwise AND with TEX_MASK.
    ; This is faster than a compare-and-reset and works because 256
    ; is a power of 2.
    la      r5, VAR_ANGLE
    load.l  r1, (r5)
    add.l   r1, r1, #1
    and.l   r1, r1, #TEX_MASK          ; wrap: 255+1 -> 0
    store.l r1, (r5)
    move.l  r6, r1                      ; r6 = angle (saved for later)

    ; === ADVANCE ZOOM OSCILLATOR ===
    ; Increment by 2 (faster cycle than rotation), wrap at 255.
    la      r5, VAR_SCALE_IDX
    load.l  r1, (r5)
    add.l   r1, r1, #2
    and.l   r1, r1, #TEX_MASK          ; wrap to 0..255
    store.l r1, (r5)

    ; === COMPUTE ZOOM LEVEL FROM SINE OSCILLATOR ===
    ; Look up sine[scale_idx] to get smooth oscillation
    lsl.l   r2, r1, #2                 ; scale_idx * 4 (byte offset)
    la      r3, SINE_TABLE
    add.l   r2, r3, r2                 ; address of sine[scale_idx]
    load.l  r1, (r2)                    ; r1 = sin[scale_idx] (-256..+256)
    asr.l   r1, r1, #1                 ; halve: -128..+128
    add.l   r1, r1, #205               ; center: 77..333

    ; Clamp to positive (safety for edge case near sine minimum)
    bltz    r1, .clamp_pos
    bra     .scale_ok
.clamp_pos:
    li      r1, #77                     ; floor at 77 (lowest valid scale)
.scale_ok:

    ; Convert continuous scale to discrete selector (0..4)
    ; scale >> 6 maps 77..333 to ~1..5, then subtract 1 for 0..4
    lsr.l   r1, r1, #6                 ; 1..5
    sub.l   r1, r1, #1                 ; 0..4

    ; Sign-extend for signed comparison (IE64 gotcha: .l ops zero-mask
    ; to 32 bits, but branch instructions compare full 64-bit values)
    lsl.q   r1, r1, #32
    asr.q   r1, r1, #32

    ; Clamp to valid range [0, 4]
    bltz    r1, .clamp_low
    li      r2, #4
    bgt     r1, r2, .clamp_high
    bra     .scale_sel_done
.clamp_low:
    li      r1, #0
    bra     .scale_sel_done
.clamp_high:
    li      r1, #4
.scale_sel_done:
    la      r5, VAR_SCALE_SEL
    store.l r1, (r5)
    move.l  r7, r1                      ; r7 = scale_sel (saved for lookup)

    ; === LOOK UP PRECOMPUTED STEP VALUES ===
    ; Index into the DU/DV tables: offset = scale_sel * 1024 + angle * 4
    ; (1024 = 256 entries * 4 bytes per entry)

    ; du_dx = DU_TABLE[scale_sel * 256 + angle]
    lsl.l   r1, r7, #10                ; scale_sel * 1024
    lsl.l   r2, r6, #2                 ; angle * 4
    add.l   r1, r1, r2                 ; combined offset
    la      r3, DU_TABLE
    add.l   r1, r3, r1                 ; address of DU entry
    load.l  r8, (r1)                    ; r8 = du_dx
    la      r5, VAR_DU_DX
    store.l r8, (r5)

    ; dv_dx = DV_TABLE[scale_sel * 256 + angle]
    lsl.l   r1, r7, #10
    lsl.l   r2, r6, #2
    add.l   r1, r1, r2
    la      r3, DV_TABLE
    add.l   r1, r3, r1
    load.l  r9, (r1)                    ; r9 = dv_dx
    la      r5, VAR_DV_DX
    store.l r9, (r5)

    ; === DERIVE PERPENDICULAR STEP VALUES ===
    ; From rotation matrix orthogonality:
    ;   du_dy = -dv_dx  (rotate step vector 90 degrees)
    ;   dv_dy =  du_dx
    neg.l   r1, r9                      ; du_dy = -dv_dx
    la      r5, VAR_DU_DY
    store.l r1, (r5)

    la      r5, VAR_DV_DY               ; dv_dy = du_dx
    store.l r8, (r5)

    ; === SET TEXTURE CENTER POINT ===
    ; The center of rotation in texture space. We fix it at (128, 128)
    ; in 8.8 fixed-point, which is 0x8000.
    ;   128.0 * 256 = 32768 = 0x8000
    ; This puts the rotation center at the middle of our 256x256 texture.
    la      r5, VAR_START_U
    li      r1, #0x8000                 ; 128.0 in 8.8 fixed-point
    store.l r1, (r5)
    la      r5, VAR_START_V
    store.l r1, (r5)                    ; same center for both U and V

    rts

; =============================================================================
; RENDER ROTOZOOMER
; =============================================================================
; The main rendering routine. Fills the entire 640x480 back buffer with
; the rotated/zoomed texture.
;
; This is where all the math from the theory section comes to life.
; The routine has three parts:
;
;   1. SETUP: Compute the starting (u,v) for the top-left pixel by
;      offsetting from the texture center by half the screen dimensions.
;
;   2. OUTER LOOP (rows): For each of 480 rows, load the row's starting
;      (u,v) and enter the inner loop.
;
;   3. INNER LOOP (pixels): For each pixel, compute a texture address from
;      the current (u,v), read the texel, write it to VRAM, then advance
;      (u,v) by (du_dx, dv_dx). Unrolled 16x for performance.
;
; === CENTERING THE ROTATION ===
;
; The rotation center should appear at screen center (320, 240).
; But we start drawing from the top-left corner (0, 0). So we need to
; compute what texture coordinate maps to the top-left pixel.
;
; From the math section, for screen pixel (sx, sy):
;   u = start_u + sx * du_dx + sy * du_dy
;   v = start_v + sx * dv_dx + sy * dv_dy
;
; At the screen center (320, 240), we want u = start_u, v = start_v.
; So at (0, 0):
;   row_u = start_u - 320 * du_dx - 240 * du_dy
;   row_v = start_v - 320 * dv_dx - 240 * dv_dy
;
; The multiplies by 320 and 240 are in fixed-point, so we shift right
; by FP_SHIFT (8) after multiplying.
;
; === IE64 ADVANTAGE: EXACT CENTER OFFSET ===
;
; On IE32, the multiplication 320 * du_dx had to be approximated because
; there was no signed multiply. The approximation used:
;   320/256 = 1.25 ≈ 1 + 1/4
;   320 * x >> 8 ≈ x + (x >> 2)
;
; This introduced a small centering error. On IE64, MULS gives us the
; exact result in just 2 instructions (muls.l + asr.l).
;
; === LOOP UNROLLING (16x) ===
;
; The inner loop processes 16 pixels per iteration. This means:
;   640 pixels / 16 pixels per iteration = 40 iterations per row
;
; Why unroll? Each iteration has 2 instructions of overhead (increment
; counter + branch). Without unrolling, that's 640 * 2 = 1280 overhead
; instructions per row. With 16x unrolling, it's only 40 * 2 = 80.
; That's a saving of 1200 instructions per row, or 576,000 per frame!
;
; The trade-off is code size: 16 copies of the 13-instruction pixel
; block = 208 instructions of code. But instruction memory is plentiful
; on the IE64, while per-frame instruction count directly impacts framerate.
;
; === THE 13-INSTRUCTION PIXEL PIPELINE ===
;
; Each pixel is rendered by this sequence:
;
;   lsr.l   r1, r22, #8         ; 1. Extract V integer part from fixed-point
;   and.l   r1, r1, r19         ; 2. Wrap V to 0..255 (mask with TEX_MASK)
;   lsl.l   r1, r1, #TEX_ROW_SHIFT ; 3. V * 1024 = byte offset to texture row
;   add.l   r3, r1, r18         ; 4. Add texture base = row start address
;   lsr.l   r1, r23, #8         ; 5. Extract U integer part from fixed-point
;   and.l   r1, r1, r19         ; 6. Wrap U to 0..255
;   lsl.l   r1, r1, #2          ; 7. U * 4 = byte offset within row (BGRA)
;   add.l   r1, r3, r1          ; 8. Row address + column offset = texel addr
;   load.l  r2, (r1)            ; 9. Load 32-bit BGRA texel from texture
;   store.l r2, (r26)           ; 10. Write texel to VRAM (back buffer)
;   add.l   r26, r26, r20       ; 11. Advance VRAM pointer by 4 bytes
;   add.l   r23, r23, r16       ; 12. u += du_dx (step U horizontally)
;   add.l   r22, r22, r17       ; 13. v += dv_dx (step V horizontally)
;
; Instructions 1-8 compute the texture address. This is the "mapping":
; converting screen position (implicit in the running u,v values) to a
; texture coordinate, then to a memory address.
;
; Instructions 9-10 are the actual pixel transfer: read texel, write pixel.
;
; Instructions 11-13 advance to the next pixel: move the VRAM pointer and
; step through texture space by adding the per-pixel increments.
;
; ALL register operands are pre-loaded constants (r16-r20, r18, r19) or
; running accumulators (r22, r23, r26). Zero memory loads for constants!
; =============================================================================
render_rotozoomer:
    ; === PART 1: COMPUTE STARTING (U,V) FOR TOP-LEFT PIXEL ===

    ; Load the per-pixel step values and sign-extend for MULS.
    ; Sign extension is needed because load.l zero-extends but MULS
    ; reads full 64-bit register values (see generate_dudv_tables comments).
    la      r5, VAR_DU_DX
    load.l  r8, (r5)                    ; r8 = du_dx
    lsl.q   r8, r8, #32                ; sign-extend 32->64
    asr.q   r8, r8, #32
    la      r5, VAR_DV_DX
    load.l  r9, (r5)                    ; r9 = dv_dx
    lsl.q   r9, r9, #32
    asr.q   r9, r9, #32
    la      r5, VAR_DU_DY
    load.l  r10, (r5)                   ; r10 = du_dy
    lsl.q   r10, r10, #32
    asr.q   r10, r10, #32
    la      r5, VAR_DV_DY
    load.l  r11, (r5)                   ; r11 = dv_dy
    lsl.q   r11, r11, #32
    asr.q   r11, r11, #32

    ; Compute: row_u = start_u - (320 * du_dx >> 8) - (240 * du_dy >> 8)
    ; This offsets from screen center to top-left corner in texture space.

    ; 320 * du_dx >> 8  (horizontal offset due to X distance from center)
    muls.l  r1, r8, #320
    asr.l   r1, r1, #8                 ; r1 = 320 * du_dx >> 8

    ; 240 * du_dy >> 8  (vertical offset due to Y distance from center)
    muls.l  r2, r10, #240
    asr.l   r2, r2, #8                 ; r2 = 240 * du_dy >> 8

    add.l   r1, r1, r2                 ; r1 = total U offset from center

    la      r5, VAR_START_U
    load.l  r3, (r5)
    sub.l   r3, r3, r1                 ; row_u = start_u - total_offset
    la      r5, VAR_ROW_U
    store.l r3, (r5)

    ; Same calculation for V: row_v = start_v - (320*dv_dx + 240*dv_dy) >> 8
    muls.l  r1, r9, #320               ; 320 * dv_dx
    asr.l   r1, r1, #8

    muls.l  r2, r11, #240              ; 240 * dv_dy
    asr.l   r2, r2, #8

    add.l   r1, r1, r2                 ; total V offset from center

    la      r5, VAR_START_V
    load.l  r3, (r5)
    sub.l   r3, r3, r1                 ; row_v = start_v - total_offset
    la      r5, VAR_ROW_V
    store.l r3, (r5)

    ; Initialize VRAM write pointer to start of back buffer
    la      r5, VAR_VRAM_ROW
    move.l  r1, #BACK_BUFFER
    store.l r1, (r5)

    ; === PART 2: LOAD ALL CONSTANTS INTO REGISTERS ===
    ; We front-load every loop-invariant value into a dedicated register.
    ; This eliminates ALL memory loads for constants during the inner loop.
    ; The IE64's generous register file (32 registers) makes this possible.

    la      r5, VAR_DU_DX
    load.l  r16, (r5)                   ; r16 = du_dx (U step per pixel)
    la      r5, VAR_DV_DX
    load.l  r17, (r5)                   ; r17 = dv_dx (V step per pixel)
    move.l  r18, #TEXTURE_BASE          ; r18 = texture base address
    move.l  r19, #TEX_MASK              ; r19 = 255 (texture coord mask)
    li      r20, #4                     ; r20 = 4 (BGRA pixel stride)
    la      r5, VAR_DU_DY
    load.l  r21, (r5)                   ; r21 = du_dy (U step per row)
    la      r5, VAR_DV_DY
    load.l  r24, (r5)                   ; r24 = dv_dy (V step per row)
    move.l  r25, #LINE_BYTES            ; r25 = 2560 (bytes per scanline)
    move.l  r29, #RENDER_H              ; r29 = 480 (row loop limit)
    li      r30, #40                    ; r30 = 40 (column loop iterations)

    li      r27, #0                     ; r27 = row counter (starts at 0)

    ; === PART 3: ROW LOOP (480 rows) ===
.row_loop:
    ; Load per-row state from memory.
    ; We reload these each row because the row-end advancement writes
    ; updated values back to memory (VAR_VRAM_ROW, VAR_ROW_U, VAR_ROW_V).
    la      r5, VAR_VRAM_ROW
    load.l  r26, (r5)                   ; r26 = VRAM write pointer for this row
    la      r5, VAR_ROW_U
    load.l  r23, (r5)                   ; r23 = U coord at row start
    la      r5, VAR_ROW_V
    load.l  r22, (r5)                   ; r22 = V coord at row start

    ; Inner loop: 40 iterations * 16 pixels = 640 pixels per row
    li      r28, #0                     ; r28 = column iteration counter

    ; === PART 4: INNER LOOP (16 pixels per iteration, 40 iterations) ===
    ; See "THE 13-INSTRUCTION PIXEL PIPELINE" in the header for a
    ; line-by-line breakdown of what each instruction does.
    ;
    ; The 16 pixel blocks below are identical -- each processes one pixel
    ; and advances the texture coordinates. The repetition IS the unrolling.

.col_loop:
    ; ===== PIXEL 0 =====
    lsr.l   r1, r22, #8                ; V integer = V_fixed >> 8
    and.l   r1, r1, r19                ; wrap V to 0..255
    lsl.l   r1, r1, #TEX_ROW_SHIFT     ; V * 1024 (row offset in bytes)
    add.l   r3, r1, r18                ; row_addr = texture_base + row_offset
    lsr.l   r1, r23, #8                ; U integer = U_fixed >> 8
    and.l   r1, r1, r19                ; wrap U to 0..255
    lsl.l   r1, r1, #2                 ; U * 4 (column offset in bytes)
    add.l   r1, r3, r1                 ; texel_addr = row_addr + col_offset
    load.l  r2, (r1)                    ; read BGRA texel from texture
    store.l r2, (r26)                   ; write texel to back buffer
    add.l   r26, r26, r20              ; advance VRAM pointer (+=4 bytes)
    add.l   r23, r23, r16              ; u += du_dx
    add.l   r22, r22, r17              ; v += dv_dx

    ; ===== PIXEL 1 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 2 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 3 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 4 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 5 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 6 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 7 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 8 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 9 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 10 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 11 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 12 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 13 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 14 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; ===== PIXEL 15 =====
    lsr.l   r1, r22, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #TEX_ROW_SHIFT
    add.l   r3, r1, r18
    lsr.l   r1, r23, #8
    and.l   r1, r1, r19
    lsl.l   r1, r1, #2
    add.l   r1, r3, r1
    load.l  r2, (r1)
    store.l r2, (r26)
    add.l   r26, r26, r20
    add.l   r23, r23, r16
    add.l   r22, r22, r17

    ; === INNER LOOP CONTROL ===
    ; 40 iterations * 16 pixels = 640 pixels per row
    add.l   r28, r28, #1
    bne     r28, r30, .col_loop         ; loop while iterations != 40

    ; === ADVANCE TO NEXT ROW ===
    ; Three things change between rows:
    ;   1. VRAM pointer jumps forward by one scanline (LINE_BYTES = 2560)
    ;   2. U coordinate advances by du_dy (vertical texture step)
    ;   3. V coordinate advances by dv_dy (vertical texture step)

    la      r5, VAR_VRAM_ROW
    load.l  r1, (r5)
    add.l   r1, r1, r25                ; VRAM_ptr += LINE_BYTES (2560)
    store.l r1, (r5)

    la      r5, VAR_ROW_U
    load.l  r1, (r5)
    add.l   r1, r1, r21                ; row_u += du_dy
    store.l r1, (r5)

    la      r5, VAR_ROW_V
    load.l  r1, (r5)
    add.l   r1, r1, r24                ; row_v += dv_dy
    store.l r1, (r5)

    add.l   r27, r27, #1               ; row counter++
    bne     r27, r29, .row_loop         ; loop while row != 480

    rts

; =============================================================================
; BLIT BACK BUFFER TO FRONT BUFFER
; =============================================================================
; Copies the completed frame from the off-screen back buffer to visible VRAM
; using the IE VideoChip's hardware blitter.
;
; === WHY USE THE BLITTER? ===
;
; The blitter is a hardware copy engine built into the VideoChip.
; In the current IntuitionEngine implementation, BLT_CTRL start runs the copy
; synchronously before returning. So this routine still waits for completion,
; but using blitter registers keeps the code aligned with the hardware model.
;
; For a 640x480x4 = 1,228,800 byte copy, the blitter is significantly
; faster than a CPU copy loop.
;
; === BLITTER REGISTER INTERFACE ===
;
; The blitter uses memory-mapped registers:
;   BLT_OP:         Operation type (copy, fill, line, masked, alpha)
;   BLT_SRC:        Source address (32-bit)
;   BLT_DST:        Destination address (32-bit)
;   BLT_WIDTH:      Width in pixels
;   BLT_HEIGHT:     Height in pixels
;   BLT_SRC_STRIDE: Source bytes per row
;   BLT_DST_STRIDE: Destination bytes per row
;   BLT_CTRL:       Bit 0=start, bit 1=busy, bit 2=IRQ enable
;   BLT_STATUS:     Bit 0=error
;
; After configuring all parameters, writing 1 to BLT_CTRL triggers the copy.
; We poll BLT_CTRL bit 1 (busy) until it clears.
; =============================================================================
blit_to_front:
    ; Configure blitter for a simple copy operation
    la      r5, BLT_OP
    move.l  r1, #BLT_OP_COPY           ; operation = copy
    store.l r1, (r5)

    la      r5, BLT_SRC
    move.l  r1, #BACK_BUFFER           ; source = back buffer
    store.l r1, (r5)

    la      r5, BLT_DST
    move.l  r1, #VRAM_START             ; destination = visible VRAM
    store.l r1, (r5)

    la      r5, BLT_WIDTH
    move.l  r1, #DISPLAY_W              ; width = 640 pixels
    store.l r1, (r5)

    la      r5, BLT_HEIGHT
    move.l  r1, #DISPLAY_H              ; height = 480 pixels
    store.l r1, (r5)

    la      r5, BLT_SRC_STRIDE
    move.l  r1, #LINE_BYTES             ; source stride = 2560 bytes/row
    store.l r1, (r5)

    la      r5, BLT_DST_STRIDE
    store.l r1, (r5)                    ; dest stride = same (2560)

    ; Trigger the blit operation
    la      r5, BLT_CTRL
    li      r1, #1                      ; write 1 to start
    store.l r1, (r5)

    ; Wait for blit to complete (poll BLT_CTRL busy bit 1)
    la      r5, BLT_CTRL
.wait_blit:
    load.l  r1, (r5)
    and.l   r1, r1, #2                  ; isolate busy bit
    bnez    r1, .wait_blit              ; loop while busy

    rts

; =============================================================================
; EOF
; =============================================================================
