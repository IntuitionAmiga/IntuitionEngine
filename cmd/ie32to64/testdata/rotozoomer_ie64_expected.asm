; ============================================================================
; MODE7 BLITTER ROTOZOOMER WITH AHX MUSIC
; IE32 Assembly for IntuitionEngine - VideoChip Mode 0 (640x480x32bpp)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true colour)
; Audio Engine:  AHX (Amiga tracker synthesis)
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm sdk/examples/asm/rotozoomer.asm
; Run:           ./bin/IntuitionEngine -ie32 rotozoomer.iex
; Porting:       VideoChip/blitter MMIO is CPU-agnostic. See rotozoomer_68k.asm,
;                rotozoomer_z80.asm, rotozoomer_65.asm, rotozoomer_x86.asm for
;                ports to other CPU cores.
;
; REFERENCE IMPLEMENTATION FOR THE INTUITION ENGINE SDK
; This file is heavily commented to teach demo programming concepts on the
; IE32 custom CPU, with particular attention to IE32-specific idioms that
; differ from conventional architectures.
;
; === WHAT THIS DEMO DOES ===
; 1. Generates a 256x256 checkerboard texture via 4 hardware blitter fills
; 2. Computes per-frame affine transformation parameters (rotation + zoom)
; 3. Delegates the full-screen affine warp to the Mode7 hardware blitter
; 4. Double-buffers the result into VRAM for tear-free display
; 5. Plays AHX music (Amiga tracker format) through the audio subsystem
;
; === WHY MODE7 HARDWARE BLITTER ===
; The classic "rotozoomer" effect rotates and zooms a texture across the
; entire screen. A naive software implementation must compute texture
; coordinates for every single pixel:
;
;   for each pixel (px, py):
;       u = u0 + px*du_col + py*du_row
;       v = v0 + px*dv_col + py*dv_row
;       colour = texture[u & mask][v & mask]
;
; At 640x480 = 307,200 pixels per frame, this is extremely expensive on
; the CPU. The Mode7 hardware blitter (BLT_OP=5) accepts just 6 parameters
; per frame -- the starting UV and the per-column/per-row UV deltas -- and
; renders the entire screen in hardware. The CPU's job is reduced to
; computing those 6 parameters from the current rotation angle and zoom
; factor, which involves only a handful of multiplies per frame.
;
; This is directly analogous to the SNES Mode7 coprocessor, which enabled
; effects like the F-Zero and Mario Kart ground planes using the same
; affine parameter approach. The key insight is the same: separate the
; "what to render" (CPU computes parameters) from "how to render" (hardware
; iterates pixels).
;
; === THE MODE7 AFFINE MATRIX ===
;
; The 2x2 affine matrix for combined rotation + zoom is:
;
;                  [ cos(angle)  -sin(angle) ]
;   M = scale  *   [                          ]
;                  [ sin(angle)   cos(angle) ]
;
; We decompose this into the blitter's six parameters:
;
;   CA = cos(angle) * reciprocal(scale_index)    -- scaled cosine
;   SA = sin(angle) * reciprocal(scale_index)    -- scaled sine
;
;   du_col =  CA     (texture U change per screen column)
;   dv_col =  SA     (texture V change per screen column)
;   du_row = -SA     (texture U change per screen row)
;   dv_row =  CA     (texture V change per screen row)
;
;   u0 = centre - CA*half_width + SA*half_height  (starting U)
;   v0 = centre - SA*half_width - CA*half_height  (starting V)
;
; The u0/v0 formulas ensure the rotation pivots around the centre of
; both the screen and the texture, not the top-left corner.
;
; === WHY AHX MUSIC ===
; Each demo in the Intuition Engine SDK showcases a different audio chip.
; The rotating cube demo (M68K) uses SID; the ULA demos use PSG.
; This IE32 demo uses AHX (Abyss' Highest eXperience), an Amiga-heritage
; tracker format with waveform synthesis. AHX_PLAY_CTRL=5 means bits 0+2
; are set: start playback (bit 0) with looping enabled (bit 2).
;
; === IE32 CPU CHARACTERISTICS ===
; The IE32 is a custom 32-bit RISC-style CPU with some important quirks
; that this demo must work around:
;
;   - NO SIGNED MULTIPLY: MUL is unsigned only. For signed math (needed
;     because sine values range -256 to +256), we must manually handle
;     signs using the "sign-magnitude multiply" pattern.
;
;   - NO NEG INSTRUCTION: To negate a value we use two's complement
;     identity: -x = ~x + 1, implemented as XOR #0xFFFFFFFF + ADD #1.
;
;   - 32-BIT .word DIRECTIVE: Unlike most assemblers where .word is 16-bit,
;     IE32's .word emits 32-bit values. This means our sine table entries
;     are already sign-extended (e.g., -256 stored as 0xFFFFFF00), avoiding
;     any sign-extension at load time.
;
;   - REGISTER-RICH: 20 general-purpose registers (A-T, plus U) allow
;     complex computations without excessive stack traffic.
;
;   - @ PREFIX FOR MMIO: Absolute memory-mapped I/O uses the @ prefix
;     (e.g., STA @VIDEO_CTRL), while [A] dereferences register A as a
;     pointer.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +-----------------------------------------------------------------+
;   |                    MAIN LOOP (60 FPS)                           |
;   |                                                                 |
;   |  +-------------+   +-------------+   +---------------+         |
;   |  | compute_    |-->| render_     |-->| blit_to_      |         |
;   |  | frame       |   | mode7       |   | front         |         |
;   |  | (6 params)  |   | (HW blit)   |   | (dbl buffer)  |         |
;   |  +-------------+   +-------------+   +---------------+         |
;   |        |                                      |                 |
;   |        v                                      v                 |
;   |  +-------------+                      +---------------+         |
;   |  | advance_    |<---------------------| wait_vsync    |         |
;   |  | animation   |                      | (tear-free)   |         |
;   |  +-------------+                      +---------------+         |
;   +-----------------------------------------------------------------+
;
;   +----------------------------+    +-----------------------------+
;   |  AHX AUDIO (background)   |    |  MODE7 BLITTER (hardware)   |
;   |                            |    |                             |
;   |  AHX player runs in the   |    |  Iterates 307,200 pixels    |
;   |  audio subsystem. Once    |    |  per frame using the 6      |
;   |  started, it plays        |    |  affine parameters. CPU     |
;   |  independently of the     |    |  only waits for completion  |
;   |  main CPU -- no per-frame |    |  via BLT_STATUS polling.    |
;   |  overhead for music.      |    |                             |
;   +----------------------------+    +-----------------------------+
;
; === MEMORY MAP ===
;
;   Address       Size    Purpose
;   ------------- ------- ------------------------------------------
;   0x001000      ~4 KB   Program code (.org 0x1000)
;   ~0x002000     2 KB    Sine table (256 entries x 4 bytes)
;   ~0x002800     2 KB    Reciprocal table (256 entries x 4 bytes)
;   ~0x003000     varies  AHX music data (.incbin)
;   0x046C20      24 B    Runtime variables (angle, scale, CA, SA, u0, v0)
;   0x100000      1.2 MB  VRAM front buffer (640x480x4 bytes)
;   0x500000      256 KB  Texture (256x256, stride 1024 bytes)
;   0x600000      1.2 MB  Back buffer (Mode7 render target)
;
; === BUILD AND RUN ===
;
;   Assemble:  sdk/bin/ie32asm assembler/rotozoomer.asm
;   Run:       ./bin/IntuitionEngine -ie32 assembler/rotozoomer.iex
;
; ============================================================================

include "ie64.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Texture Memory Layout ---
;
; The texture is a 256x256 pixel checkerboard placed at 0x500000, which is
; above VRAM (0x100000-0x2E0000 for 640x480x4). We use a stride of 1024
; bytes (256 pixels * 4 bytes/pixel) so that each row is contiguous.
;
; The checkerboard is built from four 128x128 quadrants using BLIT FILL:
;
;   +---+---+
;   | W | B |   W = white (0xFFFFFFFF)   B = black (0xFF000000)
;   +---+---+   Each quadrant is 128x128 pixels
;   | B | W |   Stride = 1024 bytes per row (256 * 4)
;   +---+---+
;
; The quadrant addresses are computed from TEXTURE_BASE + column offset
; and TEXTURE_BASE + row offset:
;   Top-left  = 0x500000                (row 0, col 0)
;   Top-right = 0x500200                (row 0, col 128 * 4 = +0x200)
;   Bot-left  = 0x520000                (row 128 * 1024 = +0x20000)
;   Bot-right = 0x520200                (row 128 * 1024 + col 128 * 4)
;
; WHY 0x500000? This address is safely above the front buffer VRAM
; (0x100000 + 640*480*4 = ~0x2E0000) and below the back buffer (0x600000).
; Placing the texture in this gap means all three memory regions (front
; buffer, texture, back buffer) are non-overlapping and can be accessed
; simultaneously by the blitter without conflicts.
;
; WHY STRIDE 1024? The texture is 256 pixels wide at 4 bytes/pixel =
; 1024 bytes per row. The blitter needs this stride to know how to
; advance from one row to the next in the texture source during Mode7
; rendering. The Mode7 mask (255) wraps texture coordinates to 0-255,
; creating the infinite-tiling illusion as the texture rotates.
TEXTURE_BASE equ 0x500000
TEX_TR equ 0x500200
TEX_BL equ 0x520000
TEX_BR equ 0x520200
BACK_BUFFER equ 0x600000
RENDER_W equ 640
RENDER_H equ 480
TEX_STRIDE equ 1024
LINE_BYTES_V equ 2560

; --- Animation Accumulator Increments (8.8 Fixed-Point) ---
;
; Animation uses 16-bit 8.8 fixed-point accumulators that wrap at 0xFFFF.
; The upper 8 bits index into the 256-entry sine/reciprocal tables, while
; the lower 8 bits provide sub-index fractional precision for smooth motion.
;
; ANGLE_INC=313 and SCALE_INC=104 maintain a 3:1 ratio, matching the
; original BASIC rotozoomer's `A+=0.03, SI+=0.01`. In 8.8 fixed-point:
;   0.03 * (256 / 2*pi) * 256 ~= 313
;   0.01 * (256 / 2*pi) * 256 ~= 104
;
; The 3:1 ratio means the rotation completes 3 full cycles for every 1
; zoom cycle, creating a visually interesting Lissajous-like pattern where
; the same combination of angle+zoom never exactly repeats for a long time.
;
; WHY 8.8 AND NOT PLAIN INTEGERS?
; If we incremented the table index directly by 1 each frame, the rotation
; would complete 256/60 ~ 4.3 seconds per revolution, which is too fast.
; Using 8.8 with an increment of 313 means the index advances by ~1.22
; per frame (313/256), giving ~3.5 seconds per revolution -- a more
; visually pleasing speed. The fractional accumulation also means the
; effective table index changes smoothly rather than in integer jumps.
ANGLE_INC equ 313
SCALE_INC equ 104

; --- Variable Addresses ---
;
; WHY 0x46C20? The IE32 program starts at .org 0x1000 and can grow large
; (code + sine table + reciprocal table + AHX music data). Variables are
; placed at 0x46C20 to guarantee they don't overlap with any of the
; program's code or data sections. This address is in the gap between the
; program area and VRAM (which starts at 0x100000), providing ample room.
;
; Each variable is 4 bytes (32-bit), totaling 24 bytes for the 6 runtime
; state variables that drive the animation:
;
;   ANGLE_ACC  -- 8.8 FP accumulator for rotation angle
;   SCALE_ACC  -- 8.8 FP accumulator for zoom oscillation
;   CA         -- cos(angle) * recip(scale), the scaled cosine
;   SA         -- sin(angle) * recip(scale), the scaled sine
;   U0         -- starting texture U coordinate (16.16 FP)
;   V0         -- starting texture V coordinate (16.16 FP)
VAR_BASE equ 0x46C20
VAR_ANGLE_ACC equ 0x46C20
VAR_SCALE_ACC equ 0x46C24
VAR_CA equ 0x46C28
VAR_SA equ 0x46C2C
VAR_U0 equ 0x46C30
VAR_V0 equ 0x46C34

; ============================================================================
; PROGRAM ENTRY POINT
; ============================================================================
; The IE32 CPU begins execution at .org 0x1000 after loading the binary.
; We must initialise video, generate the texture, set up animation state,
; and start music before entering the main loop.
; ============================================================================

org 0x1000

start:
    ; --- Enable VideoChip ---
    ; The VideoChip is the IE custom video system (distinct from VGA/ULA/etc).
    ; Mode 0 = 640x480 truecolour (32-bit BGRA). The blitter and VRAM are
    ; part of this subsystem, so it MUST be enabled before any blit operations.
    ; VIDEO_CTRL=1 enables the chip; VIDEO_MODE=0 selects Mode 0.
    move.l r1, #1
    la r17, VIDEO_CTRL
    store.l r1, (r17)
    move.l r1, #0
    la r17, VIDEO_MODE
    store.l r1, (r17)

    ; --- Generate Texture ---
    ; Build the 256x256 checkerboard using 4 hardware blitter FILL operations.
    ; We do this once at startup; the texture persists in memory at 0x500000
    ; for the entire duration of the demo.
    jsr generate_texture

    ; --- Initialise Animation State ---
    ; Both accumulators start at zero. The angle accumulator determines
    ; rotation; the scale accumulator determines zoom oscillation.
    ; Starting both at zero means the demo begins with no rotation and
    ; the zoom at the midpoint of its oscillation range.
    move.l r1, #0
    la r17, VAR_ANGLE_ACC
    store.l r1, (r17)
    la r17, VAR_SCALE_ACC
    store.l r1, (r17)

    ; --- Start AHX Music Playback ---
    ; AHX (Abyss' Highest eXperience) is an Amiga-heritage tracker format.
    ; The audio subsystem handles playback entirely in the background --
    ; once started, it requires zero CPU involvement per frame.
    ;
    ; Setup: point the player at the embedded AHX data and tell it the size.
    ; The size is computed as (end label - start label) at assemble time.
    ;
    ; AHX_PLAY_CTRL = 5 = bit 0 (start) | bit 2 (loop).
    ; Looping ensures the music repeats seamlessly when it reaches the end,
    ; which is essential for a demo that runs indefinitely.
    move.l r1, #ahx_data
    la r17, AHX_PLAY_PTR
    store.l r1, (r17)
    move.l r1, #ahx_data_end
    move.l r5, #ahx_data
    sub.l r1, r1, r5
    la r17, AHX_PLAY_LEN
    store.l r1, (r17)
    move.l r1, #5
    la r17, AHX_PLAY_CTRL
    store.l r1, (r17)

; ============================================================================
; MAIN LOOP
; ============================================================================
; Runs once per frame (~60 FPS with vsync). The order of operations is:
;
; 1. compute_frame:      Calculate the 6 Mode7 affine parameters from the
;                        current angle and scale accumulators.
; 2. render_mode7:       Program the blitter with those parameters and
;                        trigger a full-screen affine warp into the back
;                        buffer at 0x600000.
; 3. blit_to_front:      Copy the completed back buffer to VRAM (0x100000)
;                        so the display shows the new frame.
; 4. wait_vsync:         Synchronise with the vertical blank to prevent
;                        visual tearing.
; 5. advance_animation:  Increment the angle and scale accumulators for
;                        the next frame.
;
; WHY THIS ORDER?
; We compute+render+copy BEFORE waiting for vsync. This means all the
; heavy work happens during the active display period (while the previous
; frame is being shown). The vsync wait then ensures the completed blit
; becomes visible at the start of the next refresh cycle. This maximises
; the time available for rendering without introducing tearing.
; ============================================================================
main_loop:
    jsr compute_frame
    jsr render_mode7
    jsr blit_to_front
    jsr wait_vsync
    jsr advance_animation
    bra main_loop

; ============================================================================
; WAIT FOR VSYNC (Two-Phase Synchronization)
; ============================================================================
; Ensures exactly one frame passes between iterations of the main loop.
;
; WHY TWO PHASES?
; A single "wait for vblank" is ambiguous -- if we're already IN vblank
; when we check, we'd return immediately and potentially run multiple
; frames within the same vblank period. The two-phase approach:
;
;   Phase 1 (wait_end):   Spin while vblank is ACTIVE. This drains any
;                         remaining vblank time from the previous frame.
;   Phase 2 (wait_start): Spin until vblank BEGINS. This catches the
;                         rising edge of the next vblank.
;
; Together, these guarantee we synchronise to exactly one vblank boundary
; per main loop iteration, giving a stable 60 FPS frame rate.
;
; STATUS_VBLANK is bit 1 (value 2) of the VIDEO_STATUS register.
; ============================================================================
wait_vsync:
wait_end:
    la r17, VIDEO_STATUS
    load.l r1, (r17)
    and.l r1, r1, #STATUS_VBLANK
    bnez r1, wait_end
wait_start:
    la r17, VIDEO_STATUS
    load.l r1, (r17)
    and.l r1, r1, #STATUS_VBLANK
    beqz r1, wait_start
    rts

; ============================================================================
; GENERATE TEXTURE (256x256 Checkerboard via 4x Hardware BLIT FILL)
; ============================================================================
; Creates the texture used by the Mode7 blitter for the rotozoomer effect.
;
; WHY 4x BLIT FILL?
; The blitter's FILL operation (BLT_OP=1) fills a rectangular region with
; a solid colour. A 2x2 checkerboard requires 4 filled quadrants:
; two white (0xFFFFFFFF = opaque white in BGRA) and two black (0xFF000000
; = opaque black). Each quadrant is 128x128 pixels.
;
; WHY NOT SOFTWARE FILL?
; Filling 256*256*4 = 262,144 bytes in software would require a large
; loop. The hardware blitter does it in a single operation per quadrant,
; running in parallel with the CPU. We just need to wait for each fill
; to complete (BLT_STATUS bit 1 = busy) before starting the next one,
; since they share the same blitter registers.
;
; BLITTER REGISTER SETUP:
;   BLT_OP         = 1 (FILL operation)
;   BLT_DST        = destination address for this quadrant
;   BLT_WIDTH      = 128 (pixels per row in this quadrant)
;   BLT_HEIGHT     = 128 (rows in this quadrant)
;   BLT_COLOR      = fill colour (32-bit BGRA)
;   BLT_DST_STRIDE = 1024 (bytes per row in the FULL texture, not the
;                    quadrant -- the blitter advances the destination
;                    pointer by this amount after each row)
;   BLT_CTRL       = 1 (trigger the blit)
; ============================================================================
generate_texture:
    ; --- Top-left 128x128: WHITE ---
    ; This is the first quadrant. After triggering, we poll BLT_STATUS
    ; bit 1 (mask=2) until it clears, indicating the blit is complete.
    move.l r1, #1
    la r17, BLT_OP
    store.l r1, (r17)
    move.l r1, #TEXTURE_BASE
    la r17, BLT_DST
    store.l r1, (r17)
    move.l r1, #128
    la r17, BLT_WIDTH
    store.l r1, (r17)
    la r17, BLT_HEIGHT
    store.l r1, (r17)
    move.l r1, #0xFFFFFFFF
    la r17, BLT_COLOR
    store.l r1, (r17)
    move.l r1, #TEX_STRIDE
    la r17, BLT_DST_STRIDE
    store.l r1, (r17)
    move.l r1, #1
    la r17, BLT_CTRL
    store.l r1, (r17)
gt_w1:
    la r17, BLT_STATUS
    load.l r1, (r17)
    and.l r1, r1, #2
    bnez r1, gt_w1

    ; --- Top-right 128x128: BLACK ---
    ; TEX_TR = TEXTURE_BASE + 128*4 = TEXTURE_BASE + 0x200
    ; The 0x200 offset skips 128 pixels (each 4 bytes) to reach column 128.
    move.l r1, #1
    la r17, BLT_OP
    store.l r1, (r17)
    move.l r1, #TEX_TR
    la r17, BLT_DST
    store.l r1, (r17)
    move.l r1, #128
    la r17, BLT_WIDTH
    store.l r1, (r17)
    la r17, BLT_HEIGHT
    store.l r1, (r17)
    move.l r1, #0xFF000000
    la r17, BLT_COLOR
    store.l r1, (r17)
    move.l r1, #TEX_STRIDE
    la r17, BLT_DST_STRIDE
    store.l r1, (r17)
    move.l r1, #1
    la r17, BLT_CTRL
    store.l r1, (r17)
gt_w2:
    la r17, BLT_STATUS
    load.l r1, (r17)
    and.l r1, r1, #2
    bnez r1, gt_w2

    ; --- Bottom-left 128x128: BLACK ---
    ; TEX_BL = TEXTURE_BASE + 128*1024 = TEXTURE_BASE + 0x20000
    ; The 0x20000 offset skips 128 rows (each 1024 bytes) to reach row 128.
    move.l r1, #1
    la r17, BLT_OP
    store.l r1, (r17)
    move.l r1, #TEX_BL
    la r17, BLT_DST
    store.l r1, (r17)
    move.l r1, #128
    la r17, BLT_WIDTH
    store.l r1, (r17)
    la r17, BLT_HEIGHT
    store.l r1, (r17)
    move.l r1, #0xFF000000
    la r17, BLT_COLOR
    store.l r1, (r17)
    move.l r1, #TEX_STRIDE
    la r17, BLT_DST_STRIDE
    store.l r1, (r17)
    move.l r1, #1
    la r17, BLT_CTRL
    store.l r1, (r17)
gt_w3:
    la r17, BLT_STATUS
    load.l r1, (r17)
    and.l r1, r1, #2
    bnez r1, gt_w3

    ; --- Bottom-right 128x128: WHITE ---
    ; TEX_BR = TEXTURE_BASE + 0x20000 + 0x200
    ; This completes the checkerboard: white diagonals, black diagonals.
    move.l r1, #1
    la r17, BLT_OP
    store.l r1, (r17)
    move.l r1, #TEX_BR
    la r17, BLT_DST
    store.l r1, (r17)
    move.l r1, #128
    la r17, BLT_WIDTH
    store.l r1, (r17)
    la r17, BLT_HEIGHT
    store.l r1, (r17)
    move.l r1, #0xFFFFFFFF
    la r17, BLT_COLOR
    store.l r1, (r17)
    move.l r1, #TEX_STRIDE
    la r17, BLT_DST_STRIDE
    store.l r1, (r17)
    move.l r1, #1
    la r17, BLT_CTRL
    store.l r1, (r17)
gt_w4:
    la r17, BLT_STATUS
    load.l r1, (r17)
    and.l r1, r1, #2
    bnez r1, gt_w4

    rts

; ============================================================================
; COMPUTE FRAME - Calculate Mode7 Parameters from Animation State
; ============================================================================
; This is the mathematical heart of the rotozoomer. From two animation
; accumulators (angle and scale), we derive the 6 parameters the Mode7
; blitter needs: u0, v0, du_col, dv_col, du_row, dv_row.
;
; === ALGORITHM ===
;
; 1. Extract table indices from the 8.8 accumulators:
;      angle_idx = (angle_accum >> 8) & 255
;      scale_idx = (scale_accum >> 8) & 255
;
; 2. Look up trigonometric values from the sine table:
;      cos_val = sine_table[(angle_idx + 64) & 255]   (cosine = sine + 90 degrees)
;      sin_val = sine_table[angle_idx]
;
; 3. Look up zoom factor from the reciprocal table:
;      recip = recip_table[scale_idx]
;
; 4. Compute scaled cosine/sine (these ARE the column deltas):
;      CA = cos_val * recip   (signed multiply)
;      SA = sin_val * recip   (signed multiply)
;
; 5. Compute starting UV coordinates (16.16 fixed-point):
;      u0 = 8388608 - CA*320 + SA*240
;      v0 = 8388608 - SA*320 - CA*240
;
;    WHERE 8388608 = 128 << 16 = centre of a 256-pixel texture in 16.16 FP.
;    The CA*320 and SA*240 terms are half-screen offsets (640/2 and 480/2)
;    that shift the rotation pivot to the screen centre.
;
; === REGISTER ALLOCATION ===
; We use IE32's rich register set to avoid stack pressure:
;   D = angle_idx        E = scale_idx
;   F = cos_val          G = sin_val
;   H = recip            S = CA (scaled cosine)
;   T = SA (scaled sine) U = scratch for intermediate results
;   A, B, C = scratch (used extensively by multiply helpers)
;
; === WHY PROPER SINE TABLES ===
; The values are computed as round(sin(i * 2*pi / 256) * 256), giving
; true circular rotation. A cheap approximation like a triangle wave
; would produce ~29% error at 45 degrees, causing visible distortion
; (the rotation would appear to "speed up" and "slow down" within each
; quadrant). With 256 entries, we get ~1.4 degree resolution, which is
; imperceptible at 60 FPS animation speed.
;
; === WHY RECIPROCAL TABLE ===
; The reciprocal table provides zoom modulation:
;   recip[i] = round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
;
; The 0.5 baseline ensures the zoom factor is always positive (no
; inversion). The 0.3 amplitude gives smooth oscillation between
; zoom levels ~320 (zoomed in) and ~1280 (zoomed out). Pre-computing
; avoids per-frame division, which IE32 lacks entirely (no DIV instruction).
; ============================================================================
compute_frame:
    ; --- Extract angle table index ---
    ; The accumulator is 8.8 fixed-point. Shifting right by 8 extracts
    ; the integer part, which directly indexes the 256-entry sine table.
    ; The AND #255 wraps the index, though it should already be in range
    ; since the accumulator is masked to 16 bits in advance_animation.
    la r17, VAR_ANGLE_ACC
    load.l r1, (r17)
    lsr.l r1, r1, #8
    and.l r1, r1, #255
    move.l r7, r1    ; D = angle_idx (preserved across subroutine calls)

    ; --- Extract scale table index ---
    ; Same extraction for the scale accumulator. This index drives the
    ; reciprocal table, which controls the zoom oscillation.
    la r17, VAR_SCALE_ACC
    load.l r1, (r17)
    lsr.l r1, r1, #8
    and.l r1, r1, #255
    move.l r8, r1    ; E = scale_idx

    ; --- Look up cos(angle) ---
    ; Cosine is sine phase-shifted by 90 degrees. In our 256-unit angle
    ; system, 90 degrees = 64 units (256/4). We add 64 to the angle index
    ; and wrap with AND #255 to get the cosine.
    ;
    ; WHY SHL #2? IE32's .word directive emits 32-bit values, so each
    ; table entry is 4 bytes. Multiplying the index by 4 converts it to
    ; a byte offset for the table lookup.
    move.l r1, r7
    add.l r1, r1, #64
    and.l r1, r1, #255
    lsl.l r1, r1, #2    ; *4 for 32-bit entries
    add.l r1, r1, #sine_table
    load.l r1, (r1)    ; A = cos_val (signed, already sign-extended in table)
    move.l r9, r1    ; F = cos_val

    ; --- Look up sin(angle) ---
    ; Direct table lookup using the angle index.
    move.l r1, r7
    lsl.l r1, r1, #2
    add.l r1, r1, #sine_table
    load.l r1, (r1)
    move.l r10, r1    ; G = sin_val

    ; --- Look up reciprocal (zoom factor) ---
    ; The reciprocal table is indexed by scale_idx. Values are unsigned
    ; (always positive) and range from 320 to 1280.
    move.l r1, r8
    lsl.l r1, r1, #2
    add.l r1, r1, #recip_table
    load.l r1, (r1)
    move.l r11, r1    ; H = recip (unsigned)

    ; --- Compute CA = cos_val * recip ---
    ; This is a signed * unsigned multiply. The sine values range -256 to
    ; +256 (signed), while the reciprocal is always positive (unsigned).
    ; We use the signed_mul helper which handles sign-magnitude conversion.
    ; CA is the per-column texture U delta and also used in the row delta.
    move.l r1, r9
    move.l r5, r11
    jsr signed_mul    ; A = CA
    la r17, VAR_CA
    store.l r1, (r17)
    move.l r12, r1    ; S = CA (preserved for u0/v0 computation)

    ; --- Compute SA = sin_val * recip ---
    ; Same signed multiply for the sine component.
    ; SA is the per-column texture V delta and (negated) the row U delta.
    move.l r1, r10
    move.l r5, r11
    jsr signed_mul    ; A = SA
    la r17, VAR_SA
    store.l r1, (r17)
    move.l r13, r1    ; T = SA

    ; --- Compute u0 = 8388608 - CA*320 + SA*240 ---
    ;
    ; WHY 8388608?
    ; 8388608 = 128 << 16 = 0x800000. This is the centre of the 256-pixel
    ; texture in 16.16 fixed-point representation. The texture coordinates
    ; wrap at 255 (the Mode7 mask), so 128.0 in 16.16 FP places the
    ; origin at the texture centre.
    ;
    ; WHY -CA*320 + SA*240?
    ; The screen is 640x480. Half-width = 320, half-height = 240.
    ; These terms offset from the texture centre by the rotated half-screen
    ; dimensions, ensuring the rotation pivots around the screen centre
    ; rather than the top-left corner. Without this centreing, the
    ; texture would rotate around pixel (0,0) and most of the visible
    ; area would show off-centre content.
    ;
    ; The formula comes from evaluating the affine transformation at the
    ; screen centre (320, 240) and solving for the origin:
    ;   u(320,240) should map to texture centre (128.0)
    ;   u0 = 128.0 - du_col*320 - du_row*240
    ;   u0 = 128.0 - CA*320 - (-SA)*240
    ;   u0 = 128.0 - CA*320 + SA*240
    move.l r1, r12
    jsr mul_320    ; A = CA*320
    move.l r14, r1    ; U = CA*320
    move.l r1, r13
    jsr mul_240    ; A = SA*240
    move.l r5, #0x800000
    sub.l r5, r5, r14    ; B = 0x800000 - CA*320
    add.l r5, r5, r1    ; B += SA*240
    la r17, VAR_U0
    store.l r5, (r17)

    ; --- Compute v0 = 8388608 - SA*320 - CA*240 ---
    ;
    ; Same centreing logic for the V coordinate:
    ;   v(320,240) should map to texture centre (128.0)
    ;   v0 = 128.0 - dv_col*320 - dv_row*240
    ;   v0 = 128.0 - SA*320 - CA*240
    move.l r1, r13
    jsr mul_320    ; A = SA*320
    move.l r14, r1    ; U = SA*320
    move.l r1, r12
    jsr mul_240    ; A = CA*240
    move.l r5, #0x800000
    sub.l r5, r5, r14    ; B = 0x800000 - SA*320
    sub.l r5, r5, r1    ; B -= CA*240
    la r17, VAR_V0
    store.l r5, (r17)

    rts

; ============================================================================
; SIGNED MULTIPLY: A = A * B (signed result)
; ============================================================================
; IE32-SPECIFIC: WHY THIS EXISTS
;
; The IE32 CPU's MUL instruction is UNSIGNED only. It treats both operands
; as unsigned 32-bit integers. But our sine table contains signed values
; (-256 to +256), so we need signed multiplication for the core math.
;
; This subroutine implements the classic "sign-magnitude multiply" pattern
; used on CPUs without IMUL (like early MIPS and some microcontrollers):
;
;   1. Track the result sign: XOR of input signs
;      (positive*positive=positive, positive*negative=negative, etc.)
;   2. Take absolute values of both operands
;   3. Perform unsigned multiply
;   4. If result should be negative, negate it
;
; IE32-SPECIFIC: WHY XOR/ADD FOR NEGATE
; IE32 has no NEG instruction. We use the two's complement identity:
;   -x = ~x + 1
; Implemented as:
;   XOR A, #0xFFFFFFFF    ; bitwise NOT (flip all bits)
;   ADD A, #1             ; add one
; This is the standard two's complement negate, equivalent to NEG on CPUs
; that have it. The XOR flips all bits (one's complement), and the +1
; converts from one's complement to two's complement.
;
; REGISTER USAGE:
;   Input:  A = first operand (signed), B = second operand (signed)
;   Output: A = A * B (signed result)
;   Clobbers: A, B, C (C is saved/restored via PUSH/POP)
;
; JGE A, label -- tests the int32 sign bit. If A >= 0 (sign bit clear),
; the operand is already positive and needs no negation.
; ============================================================================
signed_mul:
    push r6
    move.l r6, #0    ; C = sign flag (0=positive result, 1=negative result)

    ; --- Check sign of A ---
    ; If A is negative (int32 < 0), negate it and toggle the sign flag.
    bgez r1, sm_a_pos
    eor.l r1, r1, #0xFFFFFFFF
    add.l r1, r1, #1    ; A = -A (two's complement negate)
    eor.l r6, r6, #1    ; Toggle sign flag
sm_a_pos:
    ; --- Check sign of B ---
    ; Same treatment for the second operand.
    bgez r5, sm_b_pos
    eor.l r5, r5, #0xFFFFFFFF
    add.l r5, r5, #1
    eor.l r6, r6, #1    ; Toggle again (two negatives make positive)
sm_b_pos:
    mulu.l r1, r1, r5    ; Unsigned multiply of absolute values
    ; --- Conditionally negate result ---
    ; If the sign flag is set (operands had different signs), the result
    ; should be negative. Apply two's complement negate.
    beqz r6, sm_done
    eor.l r1, r1, #0xFFFFFFFF
    add.l r1, r1, #1
sm_done:
    pop r6
    rts

; ============================================================================
; MUL_320: A = A * 320 (signed) using shift decomposition
; ============================================================================
; IE32-SPECIFIC: WHY SHIFT DECOMPOSITION WITH SIGN HANDLING
;
; Multiplying by 320 using MUL would work, but shift decomposition is a
; common optimization that avoids the multiply unit:
;   320 = 256 + 64 = (1 << 8) + (1 << 6)
; So: A * 320 = (A << 8) + (A << 6)
;
; HOWEVER, left shifts on IE32 are unsigned operations -- they don't
; preserve the sign bit in a meaningful way for signed arithmetic. Since
; CA and SA can be negative (signed 16.16 fixed-point values from the
; Mode7 matrix), we must:
;   1. Save the sign of the input
;   2. Take the absolute value
;   3. Perform the unsigned shift-and-add
;   4. Restore the original sign
;
; This is the same negate-before/negate-after pattern as signed_mul,
; applied to shift operations instead of MUL.
;
; REGISTER USAGE:
;   Input:  A = signed value to multiply by 320
;   Output: A = A * 320 (signed)
;   Clobbers: A, B, C (B and C are saved/restored)
; ============================================================================
mul_320:
    push r5
    push r6
    move.l r6, #0    ; C = sign flag
    bgez r1, m320_pos
    eor.l r1, r1, #0xFFFFFFFF
    add.l r1, r1, #1    ; Negate to get absolute value
    move.l r6, #1    ; Remember that result needs negation
m320_pos:
    move.l r5, r1    ; B = |A| (copy for the second shift)
    lsl.l r1, r1, #8    ; A = |A| * 256
    lsl.l r5, r5, #6    ; B = |A| * 64
    add.l r1, r1, r5    ; A = |A| * 320
    beqz r6, m320_done    ; If input was positive, we're done
    eor.l r1, r1, #0xFFFFFFFF
    add.l r1, r1, #1    ; Negate back to restore sign
m320_done:
    pop r6
    pop r5
    rts

; ============================================================================
; MUL_240: A = A * 240 (signed) using shift decomposition
; ============================================================================
; Same pattern as mul_320 but with different shift decomposition:
;   240 = 256 - 16 = (1 << 8) - (1 << 4)
; So: A * 240 = (A << 8) - (A << 4)
;
; Note the SUBTRACTION here (unlike mul_320's addition). This is because
; 240 = 256 - 16, not 256 + anything. The shift decomposition exploits
; the fact that 240 is close to a power of 2, making it expressible as
; a difference of two powers.
;
; REGISTER USAGE:
;   Input:  A = signed value to multiply by 240
;   Output: A = A * 240 (signed)
;   Clobbers: A, B, C (B and C are saved/restored)
; ============================================================================
mul_240:
    push r5
    push r6
    move.l r6, #0
    bgez r1, m240_pos
    eor.l r1, r1, #0xFFFFFFFF
    add.l r1, r1, #1
    move.l r6, #1
m240_pos:
    move.l r5, r1
    lsl.l r1, r1, #8    ; A = |A| * 256
    lsl.l r5, r5, #4    ; B = |A| * 16
    sub.l r1, r1, r5    ; A = |A| * (256 - 16) = |A| * 240
    beqz r6, m240_done
    eor.l r1, r1, #0xFFFFFFFF
    add.l r1, r1, #1
m240_done:
    pop r6
    pop r5
    rts

; ============================================================================
; RENDER MODE7 - Configure and Trigger the Mode7 Affine Texture Blit
; ============================================================================
; Programs the hardware blitter with all parameters for a full-screen
; affine texture warp, then triggers the blit and waits for completion.
;
; WHY DOUBLE BUFFERING?
; The Mode7 blit writes to the BACK BUFFER at 0x600000, NOT directly to
; VRAM (0x100000). If we wrote directly to VRAM, the display would show
; a partially-rendered frame (tearing) because the blit takes multiple
; scanline periods to complete. By rendering to an off-screen buffer
; and then copying the result to VRAM in a single fast blit (blit_to_front),
; we ensure the display always shows a complete frame.
;
; === BLITTER PARAMETER SETUP ===
;
;   BLT_OP = 5                    Mode7 affine texture mapping operation
;   BLT_SRC = TEXTURE_BASE        Source texture at 0x500000
;   BLT_DST = BACK_BUFFER         Destination at 0x600000
;   BLT_WIDTH = 640               Output width in pixels
;   BLT_HEIGHT = 480              Output height in pixels
;   BLT_SRC_STRIDE = 1024         Texture row stride (256 px * 4 bytes)
;   BLT_DST_STRIDE = 2560         Output row stride (640 px * 4 bytes)
;   BLT_MODE7_TEX_W = 255         Texture width MASK (wraps U to 0-255)
;   BLT_MODE7_TEX_H = 255         Texture height MASK (wraps V to 0-255)
;
; === MODE7 AFFINE PARAMETERS ===
;
; These define the affine transformation. For each screen pixel (px, py):
;   u = u0 + px * du_col + py * du_row
;   v = v0 + px * dv_col + py * dv_row
;   output[py][px] = texture[(u >> 16) & tex_w][(v >> 16) & tex_h]
;
;   u0, v0:           Starting texture coordinates (16.16 fixed-point)
;   du_col, dv_col:   UV change per column (per pixel horizontally)
;   du_row, dv_row:   UV change per row (per pixel vertically)
;
; From the rotation matrix [[CA, -SA], [SA, CA]]:
;   du_col =  CA       dv_col =  SA
;   du_row = -SA       dv_row =  CA
;
; The -SA for du_row is computed inline using IE32's XOR+ADD negate pattern.
;
; WHY MASKS = 255 (NOT 256)?
; The Mode7 blitter uses bitwise AND with these masks to wrap texture
; coordinates: (u >> 16) & 255 maps any coordinate to 0-255, creating
; the infinite tiling effect. This is a standard power-of-2 texture
; wrapping trick used in hardware since the SNES.
; ============================================================================
render_mode7:
    move.l r1, #5
    la r17, BLT_OP
    store.l r1, (r17)

    move.l r1, #TEXTURE_BASE
    la r17, BLT_SRC
    store.l r1, (r17)
    move.l r1, #BACK_BUFFER
    la r17, BLT_DST
    store.l r1, (r17)

    move.l r1, #RENDER_W
    la r17, BLT_WIDTH
    store.l r1, (r17)
    move.l r1, #RENDER_H
    la r17, BLT_HEIGHT
    store.l r1, (r17)

    move.l r1, #TEX_STRIDE
    la r17, BLT_SRC_STRIDE
    store.l r1, (r17)
    move.l r1, #LINE_BYTES_V
    la r17, BLT_DST_STRIDE
    store.l r1, (r17)

    move.l r1, #255
    la r17, BLT_MODE7_TEX_W
    store.l r1, (r17)
    la r17, BLT_MODE7_TEX_H
    store.l r1, (r17)

    ; --- Load pre-computed affine parameters ---
    la r17, VAR_U0
    load.l r1, (r17)
    la r17, BLT_MODE7_U0
    store.l r1, (r17)

    la r17, VAR_V0
    load.l r1, (r17)
    la r17, BLT_MODE7_V0
    store.l r1, (r17)

    la r17, VAR_CA
    load.l r1, (r17)
    la r17, BLT_MODE7_DU_COL    ; du_col = CA
    store.l r1, (r17)

    la r17, VAR_SA
    load.l r1, (r17)
    la r17, BLT_MODE7_DV_COL    ; dv_col = SA
    store.l r1, (r17)

    ; du_row = -SA (negate SA using XOR+ADD)
    ; The rotation matrix requires du_row to be the negative of SA.
    ; Since IE32 has no NEG instruction, we apply two's complement negate.
    eor.l r1, r1, #0xFFFFFFFF
    add.l r1, r1, #1    ; A = -SA
    la r17, BLT_MODE7_DU_ROW    ; du_row = -SA
    store.l r1, (r17)

    la r17, VAR_CA
    load.l r1, (r17)
    la r17, BLT_MODE7_DV_ROW    ; dv_row = CA
    store.l r1, (r17)

    ; --- Trigger the blit and wait for completion ---
    ; BLT_CTRL = 1 starts the operation. We then poll BLT_STATUS bit 1
    ; (busy flag) until it clears. The Mode7 blit processes all 307,200
    ; pixels in hardware, so this is much faster than software rendering.
    move.l r1, #1
    la r17, BLT_CTRL
    store.l r1, (r17)

rm7_wait:
    la r17, BLT_STATUS
    load.l r1, (r17)
    and.l r1, r1, #2
    bnez r1, rm7_wait

    rts

; ============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM)
; ============================================================================
; Copies the completed Mode7 render from the back buffer (0x600000)
; to VRAM (0x100000) using the blitter's COPY operation (BLT_OP=0).
;
; WHY NOT JUST RENDER DIRECTLY TO VRAM?
; Direct rendering causes tearing: the display refreshes at 60 Hz and
; reads VRAM continuously. If we write to VRAM while it's being scanned
; out, the top half of the screen might show the new frame while the
; bottom still shows the old frame. Double buffering eliminates this by
; completing the full render off-screen, then swapping instantaneously.
;
; The copy blit itself is fast enough to complete within the vblank
; interval, so the buffer swap appears atomic to the viewer.
; ============================================================================
blit_to_front:
    move.l r1, #0
    la r17, BLT_OP
    store.l r1, (r17)
    move.l r1, #BACK_BUFFER
    la r17, BLT_SRC
    store.l r1, (r17)
    move.l r1, #VRAM_START
    la r17, BLT_DST
    store.l r1, (r17)
    move.l r1, #RENDER_W
    la r17, BLT_WIDTH
    store.l r1, (r17)
    move.l r1, #RENDER_H
    la r17, BLT_HEIGHT
    store.l r1, (r17)
    move.l r1, #LINE_BYTES_V
    la r17, BLT_SRC_STRIDE
    store.l r1, (r17)
    la r17, BLT_DST_STRIDE
    store.l r1, (r17)
    move.l r1, #1
    la r17, BLT_CTRL
    store.l r1, (r17)

btf_wait:
    la r17, BLT_STATUS
    load.l r1, (r17)
    and.l r1, r1, #2
    bnez r1, btf_wait

    rts

; ============================================================================
; ADVANCE ANIMATION
; ============================================================================
; Increments the angle and scale accumulators by their respective rates,
; then wraps both to 16 bits (0x0000-0xFFFF).
;
; The 8.8 fixed-point format means:
;   - Upper 8 bits: table index (0-255)
;   - Lower 8 bits: fractional part (sub-index precision)
;
; ANGLE_INC=313 advances the angle by ~1.22 table entries per frame,
; completing a full rotation every ~210 frames (~3.5 seconds at 60 FPS).
;
; SCALE_INC=104 advances the zoom by ~0.41 table entries per frame,
; completing a full zoom cycle every ~630 frames (~10.5 seconds).
;
; The AND #0xFFFF wraps the accumulator to 16 bits, ensuring the upper
; byte (table index) naturally wraps from 255 back to 0 via overflow.
; ============================================================================
advance_animation:
    la r17, VAR_ANGLE_ACC
    load.l r1, (r17)
    add.l r1, r1, #ANGLE_INC
    and.l r1, r1, #0xFFFF
    la r17, VAR_ANGLE_ACC
    store.l r1, (r17)

    la r17, VAR_SCALE_ACC
    load.l r1, (r17)
    add.l r1, r1, #SCALE_INC
    and.l r1, r1, #0xFFFF
    la r17, VAR_SCALE_ACC
    store.l r1, (r17)

    rts

; ============================================================================
; SINE TABLE - 256 Entries, 32-bit Sign-Extended
; ============================================================================
; Precomputed sine values for angles 0-255 (representing 0 to 2*pi).
; Each entry is round(sin(i * 2*pi / 256) * 256).
; Values range from -256 to +256 (representing -1.0 to +1.0 in 8.8 FP).
;
; WHY 256 ENTRIES?
; 256 = 2^8, so the table index is exactly the upper byte of our 8.8
; fixed-point angle accumulator. No additional masking or scaling is
; needed beyond shifting right by 8 and masking to 8 bits. 256 entries
; give ~1.4 degree resolution, which is far below the perceptible
; threshold for smooth rotation animation at 60 FPS.
;
; WHY TRUE SINE (NOT TRIANGLE WAVE)?
; A triangle wave approximation would be cheaper to compute (and wouldn't
; need a table at all), but it introduces ~29% error at 45 degrees. This
; error manifests as visible speed variation -- the rotation appears to
; accelerate and decelerate within each quadrant, destroying the smooth
; circular motion. True sine gives perfect circular rotation with constant
; angular velocity.
;
; IE32-SPECIFIC: WHY 32-BIT ENTRIES?
; IE32's .word directive emits 32-bit values (unlike most assemblers where
; .word is 16-bit). This means negative values are stored already sign-
; extended to 32 bits: -6 is stored as 0xFFFFFFFA, -256 as 0xFFFFFF00.
; When loaded with LDA [A], no sign-extension is needed -- the value is
; immediately usable as a signed 32-bit integer. This eliminates what
; would otherwise be a separate sign-extension step on every table lookup.
;
; TABLE LAYOUT:
;   Index   0-63:   Quadrant 1 (0 to +256)    ascending
;   Index  64-127:  Quadrant 2 (+256 to 0)     descending
;   Index 128-191:  Quadrant 3 (0 to -256)     descending
;   Index 192-255:  Quadrant 4 (-256 to 0)     ascending
;
; COSINE TRICK: cos(angle) = sin(angle + 64)
; Since 64 entries = 90 degrees, adding 64 to the sine table index gives
; cosine. This avoids needing a separate cosine table.
;
; MEMORY FOOTPRINT: 256 entries * 4 bytes = 1024 bytes (1 KB)
; ============================================================================
sine_table:
    ; Quadrant 1: indices 0-63, values rise from 0 to +256
    dc.l 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
    dc.l 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
    dc.l 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
    dc.l 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
    ; Quadrant 2: indices 64-127, values fall from +256 to 0
    dc.l 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
    dc.l 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
    dc.l 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
    dc.l 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
    ; Quadrant 3: indices 128-191, values fall from 0 to -256
    ; Negative values are stored as 32-bit two's complement (sign-extended).
    ; Example: -6 = 0xFFFFFFFA, -256 = 0xFFFFFF00
    dc.l 0,0xFFFFFFFA,0xFFFFFFF3,0xFFFFFFED,0xFFFFFFE7,0xFFFFFFE1,0xFFFFFFDA,0xFFFFFFD4,0xFFFFFFCE,0xFFFFFFC8,0xFFFFFFC2,0xFFFFFFBC,0xFFFFFFB6,0xFFFFFFB0,0xFFFFFFAA,0xFFFFFFA4
    dc.l 0xFFFFFF9E,0xFFFFFF98,0xFFFFFF93,0xFFFFFF8D,0xFFFFFF87,0xFFFFFF82,0xFFFFFF7C,0xFFFFFF77,0xFFFFFF72,0xFFFFFF6D,0xFFFFFF68,0xFFFFFF63,0xFFFFFF5E,0xFFFFFF59,0xFFFFFF54,0xFFFFFF4F
    dc.l 0xFFFFFF4B,0xFFFFFF47,0xFFFFFF42,0xFFFFFF3E,0xFFFFFF3A,0xFFFFFF36,0xFFFFFF32,0xFFFFFF2F,0xFFFFFF2B,0xFFFFFF28,0xFFFFFF24,0xFFFFFF21,0xFFFFFF1E,0xFFFFFF1B,0xFFFFFF19,0xFFFFFF16
    dc.l 0xFFFFFF13,0xFFFFFF11,0xFFFFFF0F,0xFFFFFF0D,0xFFFFFF0B,0xFFFFFF09,0xFFFFFF08,0xFFFFFF06,0xFFFFFF05,0xFFFFFF04,0xFFFFFF03,0xFFFFFF02,0xFFFFFF01,0xFFFFFF01,0xFFFFFF00,0xFFFFFF00
    ; Quadrant 4: indices 192-255, values rise from -256 to 0
    dc.l 0xFFFFFF00,0xFFFFFF00,0xFFFFFF00,0xFFFFFF01,0xFFFFFF01,0xFFFFFF02,0xFFFFFF03,0xFFFFFF04,0xFFFFFF05,0xFFFFFF06,0xFFFFFF08,0xFFFFFF09,0xFFFFFF0B,0xFFFFFF0D,0xFFFFFF0F,0xFFFFFF11
    dc.l 0xFFFFFF13,0xFFFFFF16,0xFFFFFF19,0xFFFFFF1B,0xFFFFFF1E,0xFFFFFF21,0xFFFFFF24,0xFFFFFF28,0xFFFFFF2B,0xFFFFFF2F,0xFFFFFF32,0xFFFFFF36,0xFFFFFF3A,0xFFFFFF3E,0xFFFFFF42,0xFFFFFF47
    dc.l 0xFFFFFF4B,0xFFFFFF4F,0xFFFFFF54,0xFFFFFF59,0xFFFFFF5E,0xFFFFFF63,0xFFFFFF68,0xFFFFFF6D,0xFFFFFF72,0xFFFFFF77,0xFFFFFF7C,0xFFFFFF82,0xFFFFFF87,0xFFFFFF8D,0xFFFFFF93,0xFFFFFF98
    dc.l 0xFFFFFF9E,0xFFFFFFA4,0xFFFFFFAA,0xFFFFFFB0,0xFFFFFFB6,0xFFFFFFBC,0xFFFFFFC2,0xFFFFFFC8,0xFFFFFFCE,0xFFFFFFD4,0xFFFFFFDA,0xFFFFFFE1,0xFFFFFFE7,0xFFFFFFED,0xFFFFFFF3,0xFFFFFFFA

; ============================================================================
; RECIPROCAL TABLE - 256 Entries, 32-bit Unsigned
; ============================================================================
; Precomputed zoom factors for the Mode7 affine transformation.
; Each entry is round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3)).
;
; WHY THIS FORMULA?
; The expression (0.5 + sin(i) * 0.3) oscillates between 0.2 and 0.8.
; Taking the reciprocal and scaling by 256 gives values from ~320 to ~1280.
;
;   - The 0.5 baseline keeps the denominator always positive (minimum 0.2),
;     preventing division by zero or negative zoom (which would flip the
;     texture -- visually jarring). Without this offset, when sin(i) = -1,
;     the denominator would be -0.3, causing an inverted and extremely
;     zoomed-in frame.
;
;   - The 0.3 amplitude controls how dramatically the zoom oscillates.
;     Smaller values (e.g., 0.1) would give subtle zoom; larger values
;     (e.g., 0.5) would range from extreme close-up to very far away.
;     0.3 gives a visually balanced range: the texture is always
;     recognizable but clearly zooming in and out.
;
;   - The 256 numerator scales the result into a useful integer range for
;     the signed multiply with sine/cosine values (which are also scaled
;     by 256). The product CA = cos * recip is therefore in 16.16-ish
;     fixed-point, suitable for the blitter's 16.16 FP parameter format.
;
; WHY PRE-COMPUTE?
; IE32 has no division instruction at all. Computing reciprocals at
; runtime would require a software division routine -- expensive and
; unnecessary when the values are deterministic (indexed by a known
; accumulator). A 1 KB table eliminates all runtime division.
;
; TABLE SHAPE (zoom over one full cycle):
;   Index 0:     512 (baseline zoom -- texture at 1:2 scale)
;   Index 64:    320 (most zoomed in -- texture appears larger)
;   Index 128:   512 (back to baseline)
;   Index 192:  1280 (most zoomed out -- texture appears smaller)
;   Index 255:   520 (approaching baseline again)
;
; MEMORY FOOTPRINT: 256 entries * 4 bytes = 1024 bytes (1 KB)
;
; IE32-SPECIFIC: All values are positive, so no sign-extension concerns.
; They are stored as plain unsigned 32-bit integers.
; ============================================================================
recip_table:
    dc.l 512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
    dc.l 416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
    dc.l 359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
    dc.l 329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
    dc.l 320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
    dc.l 329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
    dc.l 359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
    dc.l 416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
    dc.l 512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
    dc.l 665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
    dc.l 889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
    dc.l 1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
    dc.l 1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
    dc.l 1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
    dc.l 889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
    dc.l 665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; ============================================================================
; MUSIC DATA
; ============================================================================
; Embedded AHX music file (Amiga tracker format).
;
; AHX (Abyss' Highest eXperience) is a 4-channel waveform synthesis tracker
; format from the Amiga demoscene. Unlike SID (which uses a 6502 CPU core)
; or PSG (which uses a Z80 core), AHX playback is handled natively by the
; Intuition Engine's AHX audio engine -- no secondary CPU is involved.
;
; The file is included verbatim using .incbin. The audio subsystem parses
; the AHX header, extracts instrument definitions and pattern data, and
; plays the music autonomously. The main CPU has zero per-frame overhead
; for audio after the initial AHX_PLAY_CTRL write.
;
; The ahx_data_end label immediately after the .incbin allows us to compute
; the file size at assemble time: size = ahx_data_end - ahx_data.
; ============================================================================
ahx_data:
incbin "../assets/music/Fairlightz.ahx"
ahx_data_end:
