; ============================================================================
; HARDWARE-ACCELERATED ROTOZOOMER DEMO
; IE64 Assembly for IntuitionEngine - VideoChip Mode 0 (640x480x32)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE64 (custom 64-bit RISC)
; Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true colour)
; Audio Engine:  SAP/POKEY (Atari 8-bit music format)
; Assembler:     ie64asm (built-in IE64 assembler)
; Build:         sdk/bin/ie64asm sdk/examples/asm/rotozoomer_ie64.asm
; Run:           ./bin/IntuitionEngine -ie64 rotozoomer_ie64.ie64
; Porting:       VideoChip/blitter MMIO is CPU-agnostic. See rotozoomer.asm (IE32),
;                rotozoomer_68k.asm (M68K), rotozoomer_z80.asm (Z80) for other ports.
;
; SDK REFERENCE IMPLEMENTATION - HARDWARE MODE7 BLITTER TUTORIAL
; This file is heavily commented to teach Mode7 affine texture mapping,
; fixed-point animation, and IE64-specific programming patterns.
;
; === WHAT THIS DEMO DOES ===
; Renders a full-screen (640x480) rotating and zooming checkerboard pattern
; using the Mode7 hardware blitter. The texture tiles infinitely, creating
; the classic "rotozoomer" effect seen in countless demoscene productions.
; SAP music (Atari 8-bit POKEY format) plays in the background.
;
; === WHY MODE7 HARDWARE BLITTER (NOT SOFTWARE RENDERING) ===
; A 640x480 framebuffer is 307,200 pixels. Computing affine texture
; coordinates per-pixel in software would require two multiplies, two adds,
; and a texture fetch for EACH pixel -- over 1.8 million arithmetic ops per
; frame. Instead, we delegate the entire transformation to the Mode7 blitter
; hardware: the CPU computes just 6 parameters (u0, v0, du_col, dv_col,
; du_row, dv_row), and the blitter handles all 307,200 pixels in parallel.
;
; This is directly analogous to the SNES Mode7 hardware (F-Zero, Super Mario
; Kart, Pilotwings), where the PPU performed per-scanline affine transforms
; on a single background layer. The key insight is the same: affine
; transforms are defined by a 2x2 matrix plus an origin, so the hardware
; only needs to perform incremental addition per pixel.
;
; === THE AFFINE TRANSFORM (MATHEMATICAL FOUNDATION) ===
;
; For each output pixel at screen position (col, row), the texture
; coordinates are:
;
;   u(col,row) = u0 + col * du_col + row * du_row
;   v(col,row) = v0 + col * dv_col + row * dv_row
;
; Where the matrix [[du_col, du_row], [dv_col, dv_row]] encodes rotation
; and scale, and (u0, v0) is the texture origin mapped to screen (0,0).
;
; For pure rotation by angle A with zoom factor Z:
;
;   du_col =  cos(A) * Z     (column step in U)
;   dv_col =  sin(A) * Z     (column step in V)
;   du_row = -sin(A) * Z     (row step in U = -dv_col, 90-degree rotation)
;   dv_row =  cos(A) * Z     (row step in V =  du_col)
;
; The matrix [[CA, -SA], [SA, CA]] is a standard 2D rotation matrix.
; Multiplying by the zoom factor Z scales the sampling stride.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +---------------------------------------------------------------------+
;   |                     MAIN LOOP (~60 FPS)                             |
;   |                                                                     |
;   |  +--------------+    +--------------+    +-----------------+        |
;   |  |  COMPUTE     |    |  RENDER      |    |  BLIT TO FRONT  |        |
;   |  |  FRAME PARAMS|--->|  MODE7       |--->|  (double buffer)|        |
;   |  |  (6 values)  |    |  (blitter)   |    |  BLIT COPY      |        |
;   |  +--------------+    +--------------+    +-----------------+        |
;   |        |                                        |                   |
;   |        v                                        v                   |
;   |  +--------------+                        +--------------+           |
;   |  |  ADVANCE     |                        |  WAIT VSYNC  |           |
;   |  |  ANIMATION   |<-----------------------|  (2-phase)   |           |
;   |  |  accumulators|                        +--------------+           |
;   |  +--------------+                                                   |
;   +---------------------------------------------------------------------+
;
;   +---------------------------------------------------------------------+
;   |              AUDIO SUBSYSTEM (runs independently)                   |
;   |                                                                     |
;   |  SAP music data -> POKEY Plus engine -> audio output                |
;   |  CPU sets pointer + length + ctrl=5 at startup, then forgets it.    |
;   +---------------------------------------------------------------------+
;
; === MEMORY MAP ===
;
;   Address     Size     Purpose
;   -------     ----     -------
;   0x001000    ~2KB     Program code (this file)
;   0x09F000    ---      Stack top (grows downward)
;   0x100000    1.2MB    VRAM front buffer (640x480x4 bytes)
;   0x500000    256KB    Texture data (256x256 BGRA, stride 1024)
;   0x600000    1.2MB    Back buffer for Mode7 render
;   0xF0000+    ---      Hardware I/O registers (video, blitter, audio)
;
;   Texture layout (256x256 pixels, 4 bytes/pixel, stride = 1024 bytes):
;   +----------+----------+
;   |  WHITE   |  BLACK   |   <- top half: rows 0-127
;   | 128x128  | 128x128  |
;   +----------+----------+
;   |  BLACK   |  WHITE   |   <- bottom half: rows 128-255
;   | 128x128  | 128x128  |
;   +----------+----------+
;   Each quadrant is filled by a single BLIT FILL operation.
;
; === BUILD AND RUN ===
;
;   Assemble:  sdk/bin/ie64asm assembler/rotozoomer_ie64.asm
;   Run:       ./bin/IntuitionEngine -ie64 assembler/rotozoomer_ie64.ie64
;
; === DEMOSCENE CONTEXT ===
; The rotozoomer is one of the most recognizable demoscene effects, dating
; back to the early 1990s on Amiga and Atari ST. It demonstrates real-time
; affine texture mapping -- rotating and scaling a texture in a single pass.
; Classic examples include Future Crew's "Second Reality" (1993) and
; Sanity's "Interference" (1995). On hardware without Mode7, this required
; hand-optimised inner loops. Here, the blitter does the heavy lifting,
; letting us achieve the effect at 640x480 with minimal CPU cost.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

include "ie64.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Texture Memory Layout ---
; The texture lives at 0x500000 -- above the 1MB VRAM region (0x100000) so
; there is no overlap between texture data and the display framebuffer.
; If the texture were placed inside VRAM, Mode7 reads and display reads
; would compete, and writing the checkerboard would corrupt the display.
TEXTURE_BASE    equ 0x500000

; --- Double-Buffer Back Buffer ---
; Mode7 renders into this off-screen buffer, then we BLIT COPY it to VRAM.
; Why double buffer? Without it, the Mode7 blitter would write directly
; into the VRAM that the display is actively scanning out, causing visible
; tearing artefacts (top half shows the new frame, bottom half the old).
; By rendering to 0x600000 first, we can copy the completed frame to VRAM
; atomically (during vblank) for tear-free display.
BACK_BUFFER     equ 0x600000

; --- Output Resolution ---
; VideoChip Mode 0 is 640x480 at 32 bits per pixel (BGRA).
; These match the VideoChip's native resolution. The back buffer and VRAM
; are both sized for this: 640 * 480 * 4 = 1,228,800 bytes per frame.
RENDER_W        equ 640
RENDER_H        equ 480

; --- Texture Stride ---
; Stride = width * bytes_per_pixel = 256 * 4 = 1024 bytes per row.
; The blitter needs this to compute row offsets within the texture.
; The texture is 256x256 pixels, giving TEX_W_MASK = TEX_H_MASK = 255 for
; coordinate wrapping (bitwise AND). This enables infinite tiling: any
; texture coordinate automatically wraps to the 256x256 range, so the
; checkerboard repeats seamlessly in all directions.
TEX_STRIDE      equ 1024

; --- Animation Accumulator Increments (8.8 Fixed-Point) ---
; These control how fast the rotation angle and zoom scale advance per frame.
; The accumulators are 16-bit values in 8.8 fixed-point format:
;   - High byte (bits 15-8) = integer part = table index (0-255)
;   - Low byte (bits 7-0)   = fractional part = sub-index precision
;
; This fractional accumulator pattern is the key to smooth non-integer
; stepping. If we simply incremented the table index by 1 each frame,
; we'd be locked to 256 possible speeds (one entry per frame). With 8.8
; accumulators, we get 65536 possible speeds with sub-pixel precision.
;
; ANGLE_INC = 313:
;   This matches the BASIC version's `A += 0.03` (radians per frame).
;   Derivation: 0.03 radians * (256 entries / 2*pi radians) * 256 scale
;             = 0.03 * 40.74 * 256 = 313.1 -> 313
;   At 313/256 = 1.22 entries per frame, the texture completes a full
;   rotation every 256/1.22 = ~210 frames (~3.5 seconds at 60 fps).
;
; SCALE_INC = 104:
;   This matches the BASIC version's `SI += 0.01` (radians per frame).
;   Derivation: 0.01 * (256 / 2*pi) * 256 = 104.4 -> 104
;   The 3:1 ratio between angle and scale speeds means the rotation
;   completes 3 full cycles for every 1 zoom cycle, creating a visually
;   pleasing Lissajous-like pattern where the motion never exactly repeats.
ANGLE_INC       equ 313
SCALE_INC       equ 104

; ============================================================================
; PROGRAM ENTRY POINT
; ============================================================================
; The IE64 CPU begins execution at the address specified by `org`.
; We must initialise the stack pointer before any subroutine calls (JSR),
; enable the video hardware, generate the texture, start audio, then
; enter the main loop.
; ============================================================================

org 0x1000

start:
    ; --- Initialise Stack Pointer ---
    ; IE64 uses a descending stack (grows toward lower addresses), with R31
    ; as the stack pointer. STACK_TOP (0x09F000) is defined in ie64.inc.
    ; This MUST be set before any JSR/RTS, as JSR pushes the return address
    ; onto the stack.
    ; Why `la` (load address)? IE64's `la` pseudo-instruction safely loads
    ; symbolic addresses. Unlike `li` (load immediate), which resolves
    ; symbols to 0 during pseudo-expansion (before equates are defined),
    ; `la` wraps as `lea rd, addr(r0)` and correctly resolves at link time.
    ; This is a critical IE64 gotcha: always use `la` for symbolic addresses.
    la      r31, STACK_TOP

    ; --- Enable VideoChip ---
    ; VIDEO_CTRL = 1 enables the VideoChip display engine.
    ; VIDEO_MODE = 0 selects Mode 0 (640x480, 32-bit BGRA, linear framebuffer).
    ; The VideoChip reads from VRAM at 0x100000 and displays it on screen.
    ; Writing 0 to VIDEO_MODE is explicit even though 0 is the default,
    ; because other code (or a warm restart) may have changed the mode.
    la      r1, VIDEO_CTRL
    li      r2, #1
    store.l r2, (r1)
    la      r1, VIDEO_MODE
    store.l r0, (r1)

    ; --- Generate Checkerboard Texture ---
    ; Build a 256x256 checkerboard at TEXTURE_BASE using the blitter.
    ; Why a checkerboard? It provides high contrast and clear visual
    ; feedback for rotation/zoom -- you can immediately see if the
    ; transform is wrong. It also tiles seamlessly (opposite corners match),
    ; which is essential for the infinite-scrolling rotozoomer illusion.
    jsr     generate_texture

    ; --- Clear Animation Accumulators ---
    ; Both accumulators start at 0, meaning angle_idx = 0 (no rotation)
    ; and scale_idx = 0 (base zoom level). They will advance each frame.
    ; R0 is hardwired to 0 on IE64 (like MIPS), so store.l r0 writes zero.
    la      r1, angle_accum
    store.l r0, (r1)
    la      r1, scale_accum
    store.l r0, (r1)

    ; --- Start SAP Music Playback (Looping) ---
    ; SAP (Slight Atari Player) is the Atari 8-bit music format. Each demo
    ; in the IntuitionEngine SDK showcases a different audio chip:
    ;   - M68K cube demo: SID (Commodore 64)
    ;   - Z80 demo: AY/PSG (ZX Spectrum)
    ;   - IE64 rotozoomer: SAP/POKEY (Atari 800)
    ;
    ; POKEY_PLUS_CTRL = 1 enables the enhanced POKEY playback engine, which
    ; supports the SAP format's advanced features (stereo POKEY, AUDCTL modes).
    ;
    ; SAP_PLAY_CTRL = 5 = bit 0 (start playback) | bit 2 (loop when finished).
    ; Without the loop bit, the music would play once and stop. With it,
    ; the POKEY engine automatically resets to the beginning when the song
    ; ends, providing continuous background music without CPU intervention.
    ;
    ; The sequence matters: set POKEY+ mode first, then load the data pointer
    ; and length, then trigger playback. Setting CTRL before the pointer is
    ; loaded would start playback from an undefined address.
    la      r1, POKEY_PLUS_CTRL
    li      r2, #1
    store.b r2, (r1)
    la      r1, SAP_PLAY_PTR
    la      r2, sap_data
    store.l r2, (r1)
    la      r1, SAP_PLAY_LEN
    la      r2, sap_data_end
    la      r3, sap_data
    sub.l   r2, r2, r3
    store.l r2, (r1)
    la      r1, SAP_PLAY_CTRL
    li      r2, #5
    store.l r2, (r1)

; ============================================================================
; MAIN LOOP
; ============================================================================
; Runs once per frame at ~60 fps (synchronized to vsync).
;
; The loop has 5 stages:
;   1. compute_frame:     Calculate the 6 Mode7 affine parameters from
;                         the current animation state (angle + zoom).
;   2. render_mode7:      Program the blitter with those parameters and
;                         trigger the Mode7 blit into the back buffer.
;   3. blit_to_front:     Copy the completed back buffer to VRAM (front
;                         buffer) so the display shows the new frame.
;   4. wait_vsync:        Wait for the next vertical blank interval to
;                         prevent tearing and maintain consistent timing.
;   5. advance_animation: Increment the angle and scale accumulators for
;                         the next frame.
;
; Why this order? We compute and render BEFORE vsync, so the frame is
; ready in the back buffer. Then we blit to front during/after vsync
; (the display is blanked, so no tearing). Finally we advance the
; animation state so the NEXT iteration computes different parameters.
; ============================================================================
main_loop:
    jsr     compute_frame
    jsr     render_mode7
    jsr     blit_to_front
    jsr     wait_vsync
    jsr     advance_animation
    bra     main_loop

; =============================================================================
; WAIT FOR VSYNC (TWO-PHASE EDGE DETECTION)
; =============================================================================
; Synchronises the main loop to the 60 Hz display refresh.
;
; Why two-phase? A single "wait for vblank" check has a race condition:
; if we happen to check during an ongoing vblank, we'd return immediately
; and potentially render multiple frames during a single vblank period.
;
; The two-phase approach guarantees we catch exactly one vblank edge:
;   Phase 1 (.wait_end):   Wait until we are NOT in vblank.
;                          This handles the case where we enter this
;                          routine while vblank is still active.
;   Phase 2 (.wait_start): Wait until vblank BEGINS.
;                          This catches the leading edge of the next
;                          vblank, ensuring exactly one frame per loop.
;
; STATUS_VBLANK (bit 1, value 2) in VIDEO_STATUS is set when the display
; is in the vertical blanking interval and cleared during active display.
; =============================================================================
wait_vsync:
    la      r1, VIDEO_STATUS
.wait_end:
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK
    bnez    r2, .wait_end
.wait_start:
    load.l  r2, (r1)
    and.l   r2, r2, #STATUS_VBLANK
    beqz    r2, .wait_start
    rts

; =============================================================================
; GENERATE TEXTURE (256x256 CHECKERBOARD VIA 4x BLIT FILL)
; =============================================================================
; Creates a 256x256 checkerboard texture at TEXTURE_BASE using four
; 128x128 BLIT FILL operations -- one per quadrant.
;
; Why 4x BLIT FILL instead of a software XOR pattern?
;   1. Simpler: Each fill is a single blitter command (set dst, w, h,
;      colour, stride, go). No loops, no conditionals, no XOR logic.
;   2. Faster: The blitter fills memory at hardware speed. A software
;      loop writing 256*256 = 65536 pixels would take thousands of cycles.
;   3. Clearer: The intent is obvious -- fill four rectangles. A software
;      XOR pattern requires understanding `(x^y) & 128` or similar tricks.
;
; Quadrant layout and memory offsets:
;
;   +--------------------+--------------------+
;   | Top-Left (WHITE)   | Top-Right (BLACK)  |
;   | offset = 0         | offset = 128*4     |
;   | 0x500000           | = +512 = 0x500200  |
;   +--------------------+--------------------+
;   | Bottom-Left (BLACK)| Bottom-Right (WHITE)|
;   | offset = 128*1024  | offset = 128*1024  |
;   | = +131072          |   + 128*4           |
;   | = 0x520000         | = +131584           |
;   |                    | = 0x520200          |
;   +--------------------+--------------------+
;
;   Each row is TEX_STRIDE (1024) bytes apart (256 pixels * 4 bytes).
;   Horizontal offset for right column: 128 pixels * 4 bytes = 512.
;   Vertical offset for bottom row: 128 rows * 1024 bytes/row = 131072.
;   Bottom-right = 131072 + 512 = 131584.
;
; The white colour is 0xFFFFFFFF (fully opaque white in BGRA).
; The black colour is 0xFF000000 (fully opaque black -- note alpha is still
; 0xFF; only the RGB channels are zeroed).
;
; After each BLIT FILL, we poll BLT_STATUS bit 1 (busy flag) to wait for
; completion before issuing the next fill. This is essential because the
; blitter is asynchronous -- writing BLT_CTRL=1 starts the operation and
; returns immediately. If we started the next fill while the previous one
; was still running, the registers would be overwritten mid-operation.
; =============================================================================
generate_texture:
    ; --- Top-Left 128x128: WHITE ---
    la      r1, BLT_OP
    move.l  r2, #BLT_OP_FILL
    store.l r2, (r1)
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE
    store.l r2, (r1)
    la      r1, BLT_WIDTH
    li      r2, #128
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    store.l r2, (r1)
    la      r1, BLT_COLOR
    li      r2, #0xFFFFFFFF
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #TEX_STRIDE
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
    la      r3, BLT_STATUS
.w1:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w1

    ; --- Top-Right 128x128: BLACK ---
    ; Offset = 128 pixels * 4 bytes = 512 bytes from TEXTURE_BASE.
    ; Width, height, stride carry over from the previous fill -- only
    ; DST and COLOUR need updating. (BLT_OP also carries over as FILL.)
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE+512
    store.l r2, (r1)
    la      r1, BLT_COLOR
    li      r2, #0xFF000000
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
.w2:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w2

    ; --- Bottom-Left 128x128: BLACK ---
    ; Offset = 128 rows * 1024 bytes/row = 131072 bytes from TEXTURE_BASE.
    ; Colour carries over as BLACK from the top-right fill.
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE+131072
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
.w3:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w3

    ; --- Bottom-Right 128x128: WHITE ---
    ; Offset = 131072 + 512 = 131584 bytes from TEXTURE_BASE.
    la      r1, BLT_DST
    move.l  r2, #TEXTURE_BASE+131584
    store.l r2, (r1)
    la      r1, BLT_COLOR
    li      r2, #0xFFFFFFFF
    store.l r2, (r1)
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)
.w4:
    load.l  r4, (r3)
    and.l   r4, r4, #2
    bnez    r4, .w4

    rts

; =============================================================================
; COMPUTE FRAME - Calculate Mode7 Parameters from Animation State
; =============================================================================
; This is the CPU-intensive part of the demo. We compute the 6 values that
; define the affine transform for this frame:
;
;   CA (cosine * zoom)  -- column delta for U texture coordinate
;   SA (sine * zoom)    -- column delta for V texture coordinate
;   u0 (texture origin U) -- starting U at screen pixel (0,0)
;   v0 (texture origin V) -- starting V at screen pixel (0,0)
;   du_row = -SA        -- row delta for U (derived from SA)
;   dv_row =  CA        -- row delta for V (same as CA)
;
; === ANIMATION STATE: TWO 8.8 ACCUMULATORS ===
;
; angle_accum and scale_accum are 16-bit values in 8.8 fixed-point:
;   - High byte = table index (0-255), used to look up sine/recip values
;   - Low byte  = fractional part, providing sub-entry precision
;
; Each frame, we add ANGLE_INC (313) or SCALE_INC (104) and mask to 16 bits,
; so the accumulators wrap smoothly through all 256 table entries.
;
; === TABLE LOOKUPS ===
;
; sine_table[i] = round(sin(i * 2*pi / 256) * 256)
;   - 256 entries, signed 16-bit (dc.w), range -256 to +256
;   - Represents sine values in 8.8 fixed-point (-1.0 to +1.0)
;   - cos(i) = sin((i + 64) & 255)  (quarter-wave offset = 90 degrees)
;   - Using 256 entries for a full revolution gives ~1.4-degree resolution,
;     which is more than sufficient for smooth visual rotation.
;
; recip_table[i] = round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
;   - 256 entries, unsigned 16-bit (dc.w), range 320 to 1280
;   - This encodes a smoothly oscillating zoom factor.
;   - The 0.5 base ensures the denominator is always positive (never zero),
;     so the zoom factor is always well-defined.
;   - The 0.3 amplitude gives a zoom range of ~1.25x to ~5x
;     (256/0.8=320 to 256/0.2=1280), providing dramatic but not extreme
;     zoom variation.
;   - Pre-computing avoids runtime division, which is expensive on IE64
;     (no hardware divide instruction).
;
; === THE CORE COMPUTATION ===
;
; After loading cos_val, sin_val, and recip:
;
;   CA = cos_val * recip     (signed * unsigned -> signed, 16.16 FP)
;   SA = sin_val * recip     (signed * unsigned -> signed, 16.16 FP)
;
; CA and SA encode both rotation (from sin/cos) and zoom (from recip)
; in a single multiply. The blitter interprets them as 16.16 fixed-point
; per-pixel deltas.
;
; === ORIGIN CENTERING (u0 / v0) ===
;
; Without centreing, the rotation pivot would be at screen corner (0,0),
; causing the texture to swing wildly around the top-left. We want the
; pivot at screen centre (320, 240).
;
; The formula to centre rotation at screen (cx, cy) is:
;   u0 = tex_centre - CA*cx + SA*cy
;   v0 = tex_centre - SA*cx - CA*cy
;
; Where tex_centre = 128 << 16 = 8388608 (0x800000) centres on the
; 256x256 texture's midpoint in 16.16 fixed-point.
;
; We compute CA*320 using shifts: 320 = 256 + 64, so CA*320 = CA<<8 + CA<<6.
; Similarly, SA*240 using: 240 = 256 - 16, so SA*240 = SA<<8 - SA<<4.
; This avoids a multiply instruction and is a classic demoscene trick.
;
; === IE64-SPECIFIC: WHY SIGN EXTENSION IS REQUIRED ===
;
; IE64's `load.l` instruction ZERO-extends the loaded 32-bit value into
; the 64-bit register. This is correct for unsigned values (like the
; reciprocal table, whose entries are always positive 320-1280).
;
; However, the sine table contains SIGNED 16-bit values (-256 to +256)
; stored as dc.w. After loading, we must:
;   1. Mask to 16 bits:    and.l r, r, #0xFFFF
;   2. Sign-extend to 64:  lsl.q r, r, #48; asr.q r, r, #48
;
; Without sign extension, a sine value of -256 (0xFF00 as uint16) would
; be treated as 65280 (positive), causing cos_val and sin_val to be
; wildly wrong for angles 128-255 (the negative half of the sine wave).
; The rotation would appear to "jump" at 180 degrees.
;
; === IE64-SPECIFIC: WHY .q OPERATIONS FOR SHIFTS ===
;
; After `muls.l`, CA and SA are 32-bit signed values in 64-bit registers.
; Before performing the *320 and *240 decomposition via shifts, we must
; sign-extend them to full 64-bit width with `lsl.q/asr.q #32`.
; Then all subsequent shifts (<<8, <<6, <<4) must use `.q` (64-bit) ops.
;
; If we used `.l` (32-bit) shifts instead, the shift would zero the upper
; 32 bits, losing the sign bit. For negative CA/SA values, this would
; corrupt the centreing math: instead of subtracting (e.g., -CA*320),
; we'd add a large positive number, placing the texture origin far
; off-screen.
; =============================================================================
compute_frame:
    ; --- Extract angle table index from 8.8 accumulator ---
    ; Right-shift by 8 discards the fractional byte, leaving the integer
    ; table index in bits 7-0. Mask to 255 ensures wrapping.
    la      r1, angle_accum
    load.l  r5, (r1)
    lsr.l   r5, r5, #8
    and.l   r5, r5, #255                    ; r5 = angle_idx (0-255)

    ; --- Extract scale table index from 8.8 accumulator ---
    la      r1, scale_accum
    load.l  r6, (r1)
    lsr.l   r6, r6, #8
    and.l   r6, r6, #255                    ; r6 = scale_idx (0-255)

    ; --- Look up cos_val = sine_table[(angle_idx + 64) & 255] ---
    ; cos(x) = sin(x + 90 degrees). In our 256-entry table, 90 degrees = 64.
    ; The table uses dc.w (16-bit words), so we multiply the index by 2 for
    ; byte offset. We load 32 bits (load.l) because IE64 has no load.w,
    ; then mask to 16 bits and sign-extend.
    add.l   r1, r5, #64
    and.l   r1, r1, #255
    lsl.l   r1, r1, #1                     ; *2 for word access
    la      r2, sine_table
    add.l   r1, r2, r1
    load.l  r7, (r1)
    and.l   r7, r7, #0xFFFF                ; mask to 16 bits (zero upper bits)
    lsl.q   r7, r7, #48
    asr.q   r7, r7, #48                    ; r7 = cos_val sign-extended to 64-bit

    ; --- Look up sin_val = sine_table[angle_idx] ---
    lsl.l   r1, r5, #1                     ; *2 for word access
    la      r2, sine_table
    add.l   r1, r2, r1
    load.l  r8, (r1)
    and.l   r8, r8, #0xFFFF
    lsl.q   r8, r8, #48
    asr.q   r8, r8, #48                    ; r8 = sin_val sign-extended to 64-bit

    ; --- Look up recip = recip_table[scale_idx] ---
    ; Reciprocal values are always positive (320-1280), so zero-extension
    ; from load.l is correct here -- no sign extension needed.
    lsl.l   r1, r6, #1                     ; *2 for word access
    la      r2, recip_table
    add.l   r1, r2, r1
    load.l  r9, (r1)
    and.l   r9, r9, #0xFFFF                ; r9 = recip (unsigned, always positive)

    ; --- Compute CA = cos_val * recip ---
    ; This single multiply encodes BOTH rotation angle (from cos_val) and
    ; zoom level (from recip). The result is in 16.16 fixed-point because
    ; cos_val is 8.8 and recip is ~8.8, and 8+8 = 16 fractional bits.
    ; muls.l does a signed 32-bit multiply; since recip is positive and
    ; fits in 16 bits, the result is correct.
    muls.l  r10, r7, r9                    ; r10 = CA (16.16 FP signed)

    ; --- Compute SA = sin_val * recip ---
    muls.l  r11, r8, r9                    ; r11 = SA (16.16 FP signed)

    ; --- Store CA and SA for use by render_mode7 ---
    la      r1, var_ca
    store.l r10, (r1)
    la      r1, var_sa
    store.l r11, (r1)

    ; =========================================================================
    ; COMPUTE u0 = 8388608 - CA*320 + SA*240
    ; =========================================================================
    ; 8388608 = 128 << 16 = texture centre (128 pixels) in 16.16 FP.
    ; CA*320 offsets by half the screen width (640/2 = 320).
    ; SA*240 offsets by half the screen height (480/2 = 240).
    ; Together, these centre the rotation pivot at screen centre.

    ; First, sign-extend CA and SA from 32-bit to 64-bit.
    ; After muls.l, the results are 32-bit signed values sitting in 64-bit
    ; registers. The upper 32 bits may contain garbage. We must sign-extend
    ; before doing 64-bit shifts, otherwise negative values get corrupted.
    lsl.q   r10, r10, #32
    asr.q   r10, r10, #32                  ; sign-extend CA to 64-bit
    lsl.q   r11, r11, #32
    asr.q   r11, r11, #32                  ; sign-extend SA to 64-bit

    ; CA * 320 = CA * (256 + 64) = (CA << 8) + (CA << 6)
    ; Why shifts instead of multiply? Two shifts + add is faster than muls
    ; and avoids using extra registers. This decomposition works because
    ; 320 = 2^8 + 2^6. It's a classic strength-reduction optimization.
    lsl.q   r1, r10, #8                    ; CA * 256
    lsl.q   r2, r10, #6                    ; CA * 64
    add.q   r1, r1, r2                     ; r1 = CA * 320

    ; SA * 240 = SA * (256 - 16) = (SA << 8) - (SA << 4)
    ; 240 = 2^8 - 2^4. Same strength-reduction trick.
    lsl.q   r2, r11, #8                    ; SA * 256
    lsl.q   r3, r11, #4                    ; SA * 16
    sub.q   r2, r2, r3                     ; r2 = SA * 240

    ; u0 = tex_centre - CA*320 + SA*240
    ; Sign-extend the constant 0x800000 too, for correct 64-bit arithmetic.
    move.l  r3, #0x800000                  ; 8388608 = 128 << 16
    lsl.q   r3, r3, #32
    asr.q   r3, r3, #32                    ; sign-extend to 64-bit
    sub.q   r3, r3, r1                     ; - CA*320
    add.q   r3, r3, r2                     ; + SA*240
    la      r1, var_u0
    store.l r3, (r1)

    ; =========================================================================
    ; COMPUTE v0 = 8388608 - SA*320 - CA*240
    ; =========================================================================
    ; Same decomposition, but with SA and CA swapped and both subtracted.
    ; This comes from the rotation matrix:
    ;   u0 = centre - CA*cx + SA*cy
    ;   v0 = centre - SA*cx - CA*cy
    ; The sign pattern (-, +) vs (-, -) is because the rotation matrix
    ; [[CA, -SA], [SA, CA]] inverts differently for u vs v.

    ; SA * 320 = (SA << 8) + (SA << 6)
    lsl.q   r1, r11, #8                    ; SA * 256
    lsl.q   r2, r11, #6                    ; SA * 64
    add.q   r1, r1, r2                     ; r1 = SA * 320

    ; CA * 240 = (CA << 8) - (CA << 4)
    lsl.q   r2, r10, #8                    ; CA * 256
    lsl.q   r3, r10, #4                    ; CA * 16
    sub.q   r2, r2, r3                     ; r2 = CA * 240

    ; v0 = tex_centre - SA*320 - CA*240
    move.l  r3, #0x800000
    lsl.q   r3, r3, #32
    asr.q   r3, r3, #32
    sub.q   r3, r3, r1                     ; - SA*320
    sub.q   r3, r3, r2                     ; - CA*240
    la      r1, var_v0
    store.l r3, (r1)

    rts

; =============================================================================
; RENDER MODE7 - Configure and Trigger Mode7 Hardware Blit
; =============================================================================
; Programs the blitter's Mode7 registers with the 6 affine parameters
; computed by compute_frame, then triggers the blit.
;
; The blitter performs the following for each output pixel at (col, row):
;   u = u0 + col * du_col + row * du_row
;   v = v0 + col * dv_col + row * dv_row
;   tex_x = (u >> 16) & TEX_W_MASK    (wrap to texture size)
;   tex_y = (v >> 16) & TEX_H_MASK    (wrap to texture size)
;   dst[row][col] = src[tex_y][tex_x]
;
; The 6 parameters form the complete affine transform:
;
;   Parameter     Value     Meaning
;   ----------    -----     ----------------------------------
;   u0            var_u0    Texture U at screen pixel (0,0)
;   v0            var_v0    Texture V at screen pixel (0,0)
;   du_col (CA)   var_ca    U change per column (cosine * zoom)
;   dv_col (SA)   var_sa    V change per column (sine * zoom)
;   du_row (-SA)  -var_sa   U change per row (-sine * zoom)
;   dv_row (CA)   var_ca    V change per row (cosine * zoom)
;
; The relationship du_row = -dv_col and dv_row = du_col is what makes
; this a pure rotation (no shear). The rotation matrix:
;   [[du_col, du_row],   =  [[ CA, -SA],
;    [dv_col, dv_row]]       [ SA,  CA]]
;
; TEX_W_MASK and TEX_H_MASK are both 255 (for a 256x256 texture).
; The blitter uses these for bitwise-AND wrapping, which is equivalent
; to modulo 256 but much faster. This only works when the texture
; dimensions are powers of 2 -- a deliberate design choice.
;
; The blit renders to BACK_BUFFER (0x600000), not directly to VRAM.
; See the double-buffering discussion in the constants section.
; =============================================================================
render_mode7:
    ; --- Set blitter operation to Mode7 ---
    la      r1, BLT_OP
    move.l  r2, #BLT_OP_MODE7
    store.l r2, (r1)

    ; --- Set source (texture) and destination (back buffer) ---
    la      r1, BLT_SRC
    move.l  r2, #TEXTURE_BASE
    store.l r2, (r1)
    la      r1, BLT_DST
    move.l  r2, #BACK_BUFFER
    store.l r2, (r1)

    ; --- Set output dimensions ---
    ; These define how many pixels the blitter will produce.
    ; 640 columns * 480 rows = 307,200 pixels.
    la      r1, BLT_WIDTH
    move.l  r2, #RENDER_W
    store.l r2, (r1)
    la      r1, BLT_HEIGHT
    move.l  r2, #RENDER_H
    store.l r2, (r1)

    ; --- Set source and destination strides ---
    ; Source stride = texture row pitch in bytes (256 * 4 = 1024).
    ; Dest stride   = output row pitch in bytes (640 * 4 = 2560 = LINE_BYTES).
    ; The blitter uses these to advance to the next row after processing
    ; each scanline.
    la      r1, BLT_SRC_STRIDE
    move.l  r2, #TEX_STRIDE
    store.l r2, (r1)
    la      r1, BLT_DST_STRIDE
    move.l  r2, #LINE_BYTES
    store.l r2, (r1)

    ; --- Set texture coordinate wrapping masks ---
    ; TEX_W_MASK = TEX_H_MASK = 255 (for 256x256 texture).
    ; The blitter ANDs texture coordinates with these masks, implementing
    ; wrap-around: any coordinate automatically maps to 0-255, creating
    ; infinite seamless tiling in both dimensions.
    la      r1, BLT_MODE7_TEX_W
    li      r2, #255
    store.l r2, (r1)
    la      r1, BLT_MODE7_TEX_H
    store.l r2, (r1)

    ; --- Load the 6 affine parameters ---
    ; u0, v0: texture origin mapped to screen (0,0), centreed for pivot
    la      r3, var_u0
    load.l  r2, (r3)
    la      r1, BLT_MODE7_U0
    store.l r2, (r1)

    la      r3, var_v0
    load.l  r2, (r3)
    la      r1, BLT_MODE7_V0
    store.l r2, (r1)

    ; du_col = CA: how much U changes per column step (horizontal)
    la      r3, var_ca
    load.l  r2, (r3)
    la      r1, BLT_MODE7_DU_COL
    store.l r2, (r1)                        ; du_col = CA

    ; dv_col = SA: how much V changes per column step
    la      r3, var_sa
    load.l  r2, (r3)
    la      r1, BLT_MODE7_DV_COL
    store.l r2, (r1)                        ; dv_col = SA

    ; du_row = -SA: how much U changes per row step (vertical)
    ; Negating SA gives the perpendicular direction, which is what makes
    ; rows advance at 90 degrees to columns -- essential for a pure
    ; rotation without shear.
    neg.l   r2, r2                          ; -SA
    la      r1, BLT_MODE7_DU_ROW
    store.l r2, (r1)                        ; du_row = -SA

    ; dv_row = CA: how much V changes per row step
    ; This reuses the CA value, completing the rotation matrix.
    la      r3, var_ca
    load.l  r2, (r3)
    la      r1, BLT_MODE7_DV_ROW
    store.l r2, (r1)                        ; dv_row = CA

    ; --- Trigger the Mode7 blit ---
    ; Writing 1 to BLT_CTRL starts the blitter. It runs asynchronously
    ; (the CPU continues executing), so we must poll BLT_STATUS to know
    ; when it finishes. Bit 1 of BLT_STATUS = busy.
    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)

    ; --- Wait for Mode7 blit to complete ---
    ; We must wait because the next step (blit_to_front) will reconfigure
    ; the blitter for a COPY operation. If Mode7 is still running, we'd
    ; corrupt its registers mid-blit.
    la      r1, BLT_STATUS
.wait:
    load.l  r2, (r1)
    and.l   r2, r2, #2
    bnez    r2, .wait

    rts

; =============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM) - DOUBLE BUFFER PRESENT
; =============================================================================
; Copies the completed Mode7 render from the back buffer to the VRAM
; front buffer using a hardware BLIT COPY.
;
; Why BLIT COPY instead of pointer swap?
; On hardware with page flipping (Amiga, modern GPUs), you'd swap the
; display pointer to avoid the copy entirely. The IntuitionEngine's
; VideoChip always displays from VRAM_START (0x100000), so we must
; physically copy the pixels. The blitter does this at hardware speed --
; much faster than a CPU memcpy loop.
;
; The copy uses the same dimensions and stride as the Mode7 render.
; Source stride = destination stride = LINE_BYTES (2560), because both
; buffers use the same 640-pixel-wide layout.
; =============================================================================
blit_to_front:
    la      r1, BLT_OP
    move.l  r2, #BLT_OP_COPY
    store.l r2, (r1)

    la      r1, BLT_SRC
    move.l  r2, #BACK_BUFFER
    store.l r2, (r1)

    la      r1, BLT_DST
    move.l  r2, #VRAM_START
    store.l r2, (r1)

    la      r1, BLT_WIDTH
    move.l  r2, #RENDER_W
    store.l r2, (r1)

    la      r1, BLT_HEIGHT
    move.l  r2, #RENDER_H
    store.l r2, (r1)

    la      r1, BLT_SRC_STRIDE
    move.l  r2, #LINE_BYTES
    store.l r2, (r1)

    la      r1, BLT_DST_STRIDE
    store.l r2, (r1)

    la      r1, BLT_CTRL
    li      r2, #1
    store.l r2, (r1)

    ; Wait for copy completion before returning to main loop.
    ; If we didn't wait, the next frame's Mode7 render could start
    ; writing to the back buffer while the copy is still reading from it.
    la      r1, BLT_STATUS
.wait:
    load.l  r2, (r1)
    and.l   r2, r2, #2
    bnez    r2, .wait

    rts

; =============================================================================
; ADVANCE ANIMATION - Increment 8.8 Fixed-Point Accumulators
; =============================================================================
; Adds the per-frame increment to each accumulator and wraps to 16 bits.
;
; The 16-bit mask (0xFFFF) implements modular arithmetic:
;   - angle_accum wraps from 65535 back to 0 seamlessly
;   - This gives 256 complete revolutions of the angle index before
;     the fractional part repeats exactly (65536 / 256 = 256 cycles)
;   - In practice, the non-integer increment (313) means the fractional
;     offset is different each revolution, so the animation never
;     visibly "clicks" to grid points
;
; The 3:1 ratio (313 vs 104) between angle and scale increments creates
; a quasi-periodic Lissajous pattern: the rotation is 3x faster than
; the zoom oscillation, so the visual combination takes a long time to
; repeat (LCM of their periods), keeping the effect visually interesting
; over extended viewing.
; =============================================================================
advance_animation:
    la      r1, angle_accum
    load.l  r2, (r1)
    add.l   r2, r2, #ANGLE_INC
    and.l   r2, r2, #0xFFFF
    store.l r2, (r1)

    la      r1, scale_accum
    load.l  r2, (r1)
    add.l   r2, r2, #SCALE_INC
    and.l   r2, r2, #0xFFFF
    store.l r2, (r1)

    rts

; =============================================================================
; VARIABLES (RUNTIME STATE)
; =============================================================================
; These are modified every frame by compute_frame and advance_animation.
; They are stored in the code segment (no separate BSS on IE64) and
; initialised to 0. Using dc.l ensures 32-bit alignment.
;
;   angle_accum: 8.8 FP accumulator for rotation angle (bits 15-8 = table index)
;   scale_accum: 8.8 FP accumulator for zoom scale  (bits 15-8 = table index)
;   var_ca:      CA = cos(angle) * recip(scale)    -- per-frame affine param
;   var_sa:      SA = sin(angle) * recip(scale)    -- per-frame affine param
;   var_u0:      Texture U origin at screen (0,0)  -- centreed for pivot
;   var_v0:      Texture V origin at screen (0,0)  -- centreed for pivot
; =============================================================================
angle_accum:    dc.l    0
scale_accum:    dc.l    0
var_ca:         dc.l    0
var_sa:         dc.l    0
var_u0:         dc.l    0
var_v0:         dc.l    0

; =============================================================================
; SINE TABLE - 256 Entries, Signed 16-Bit
; =============================================================================
; Pre-computed sine values: round(sin(i * 2*pi / 256) * 256)
;
; === WHY PRE-COMPUTED SINE? ===
; Computing sin() at runtime requires Taylor series expansion, CORDIC, or
; polynomial approximation -- all expensive multi-instruction sequences.
; A 512-byte lookup table (256 entries * 2 bytes each) gives instant O(1)
; results with perfect reproducibility. The memory cost is trivial
; compared to the 1.2MB framebuffer.
;
; === TABLE FORMAT ===
;   - 256 entries cover one full revolution (0 to 2*pi radians)
;   - Each entry is a signed 16-bit word (dc.w)
;   - Value range: -256 to +256 (representing -1.0 to +1.0 in 8.8 FP)
;   - Index 0 = sin(0) = 0, Index 64 = sin(90deg) = 256 = 1.0
;   - cos(i) = sine_table[(i + 64) & 255]  (90-degree phase shift)
;
; === WHY 256 ENTRIES? ===
; 256 is a power of 2, so wrapping via AND with 255 is a single
; instruction (no division/modulo needed). 256 entries give ~1.4-degree
; resolution, which is imperceptible at 60 fps -- the eye cannot
; distinguish sub-2-degree rotation steps at animation speed.
;
; === IE64 LOAD GOTCHA ===
; These are 16-bit signed values, but IE64 has no `load.w` instruction.
; We use `load.l` (which reads 32 bits and zero-extends) followed by
; masking and sign extension. See compute_frame for the pattern.
; =============================================================================
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

; =============================================================================
; RECIPROCAL TABLE - 256 Entries, Unsigned 16-Bit
; =============================================================================
; Pre-computed zoom factors: round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
;
; === WHY A RECIPROCAL TABLE? ===
; We want the zoom level to oscillate smoothly over time. Using sine for
; the zoom directly (zoom = sin(scale_idx)) would include zero crossings,
; where the zoom hits 0 and the image collapses to a point. The reciprocal
; formulation ensures the zoom factor is ALWAYS positive and bounded.
;
; === THE FORMULA: 256 / (0.5 + sin(x) * 0.3) ===
;
; The denominator oscillates between:
;   - Maximum: 0.5 + 0.3 = 0.8 -> zoom = 256/0.8 = 320  (zoomed in, 1.25x)
;   - Minimum: 0.5 - 0.3 = 0.2 -> zoom = 256/0.2 = 1280 (zoomed out, 5x)
;
; The 0.5 base keeps the denominator always positive (since |sin| <= 1.0
; and 0.5 > 0.3, the denominator ranges from 0.2 to 0.8, never zero).
; The 0.3 amplitude controls the zoom contrast:
;   - Larger amplitude = more dramatic zoom range but risks small denominator
;   - Smaller amplitude = subtler zoom, always safe
;   - 0.3 was chosen empirically for a visually pleasing 4:1 zoom ratio
;
; === TABLE FORMAT ===
;   - 256 entries, unsigned 16-bit (dc.w)
;   - Value range: 320 to 1280
;   - All values are positive, so zero-extension from load.l is correct
;     (no sign extension needed, unlike the sine table)
;   - Indexed by scale_accum >> 8, same as the sine table
;
; === WHY PRE-COMPUTE INSTEAD OF RUNTIME DIVISION? ===
; IE64 has no hardware divide instruction. Software division requires
; an iterative shift-and-subtract loop (~30-50 cycles per division).
; A table lookup is a single load instruction. Since we only need 256
; possible zoom values (one per table entry), pre-computation is both
; faster and simpler.
; =============================================================================
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

; =============================================================================
; MUSIC DATA - SAP FORMAT (ATARI 8-BIT POKEY)
; =============================================================================
; SAP (Slight Atari Player) files contain music data for the POKEY sound
; chip, the audio hardware of the Atari 400/800/XL/XE computers (1979-1992).
;
; === WHY SAP/POKEY FOR THIS DEMO? ===
; Each IntuitionEngine SDK demo showcases a different audio subsystem:
;   - M68K rotating cube:   SID  (Commodore 64, via 6502 CPU core)
;   - Z80 tunnel demo:      AY   (ZX Spectrum / Amstrad CPC)
;   - 6502 demo:            SID  (native architecture match)
;   - IE64 rotozoomer:      SAP  (Atari 8-bit POKEY)
;
; The POKEY chip had 4 channels with unique "distortion" modes that could
; produce a wide range of timbres beyond simple square/triangle waves,
; giving Atari music a distinctive crunchy character quite different from
; the SID or AY.
;
; === HOW SAP PLAYBACK WORKS ===
; Unlike SID files (which contain executable 6502 code), SAP files are
; data-driven: the POKEY Plus engine in the IntuitionEngine interprets
; the SAP format directly and programs the emulated POKEY registers.
; The CPU's only job is to point the engine at the data and press "play".
;
; === THE PLAYBACK REGISTERS ===
;   SAP_PLAY_PTR  = pointer to SAP data in memory
;   SAP_PLAY_LEN  = byte length of SAP data
;   SAP_PLAY_CTRL = control bits:
;     Bit 0 (1): Start playback
;     Bit 2 (4): Loop when finished
;     Combined: 5 = start + loop = continuous background music
;
; The POKEY Plus mode (POKEY_PLUS_CTRL = 1) must be enabled first. This
; activates the enhanced playback engine that supports SAP format features
; beyond raw register writes.
; =============================================================================
sap_data:
    incbin  "../assets/music/Chromaluma.sap"
sap_data_end:
