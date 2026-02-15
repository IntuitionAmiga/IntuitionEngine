; ============================================================================
; MODE 7 HARDWARE BLITTER ROTOZOOMER - x86 (32-bit) REFERENCE IMPLEMENTATION
; NASM Syntax for IntuitionEngine - VideoChip 640x480 True Color
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    x86 (32-bit protected mode)
; Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true color)
; Audio Engine:  PSG (AY-3-8910 compatible)
; Assembler:     nasm (Netwide Assembler)
; Build:         nasm -f bin -o rotozoomer_x86.ie86 rotozoomer_x86.asm
; Run:           ./bin/IntuitionEngine -x86 rotozoomer_x86.ie86
; Porting:       VideoChip/blitter MMIO is CPU-agnostic. Compare with
;                rotozoomer.asm (IE32), rotozoomer_68k.asm (M68K) for other
;                32-bit approaches.
;
; SDK TUTORIAL: HARDWARE-ACCELERATED AFFINE TEXTURE MAPPING
; This file is heavily commented to teach Mode7 programming concepts on x86.
;
; === WHAT THIS DEMO DOES ===
; 1. Generates a 256x256 checkerboard texture via hardware blitter fills
; 2. Performs real-time rotation and zoom of the texture using Mode7 hardware
; 3. Double-buffers the output to prevent visual tearing
; 4. Plays PSG (AY-3-8910) music through the audio subsystem
;
; === WHY MODE7 HARDWARE BLITTER (NOT SOFTWARE RENDERING) ===
; A rotozoomer must transform every pixel on screen through an affine matrix.
; At 640x480 resolution, that is 307,200 pixels per frame. A software
; implementation would need per-pixel: texture coordinate calculation,
; wrapping, texture fetch, and framebuffer write -- easily 10+ instructions
; per pixel, totaling ~3 million instructions per frame.
;
; Instead, we delegate the ENTIRE affine transformation to the Mode7 hardware
; blitter. The CPU computes just 6 parameters per frame (u0, v0, du_col,
; dv_col, du_row, dv_row), then the blitter handles all 307,200 pixels in
; hardware. This is analogous to how the SNES Mode7 chip enabled F-Zero and
; Super Mario Kart -- the CPU sets up the transformation matrix, and
; dedicated hardware rasterizes the result.
;
; === THE MODE7 AFFINE TRANSFORM ===
; Mode7 maps a 2D texture onto the screen using an affine transformation:
;
;   For each screen pixel (px, py):
;     u = u0 + px * du_col + py * du_row
;     v = v0 + px * dv_col + py * dv_row
;     color = texture[u & TEX_W_MASK][v & TEX_H_MASK]
;
; The six parameters encode rotation AND scaling simultaneously:
;
;   Rotation matrix = [[cos(A), -sin(A)],     Scaled by zoom factor S:
;                       [sin(A),  cos(A)]]
;
;   du_col = CA = cos(A) * S    (texture U change per column step)
;   dv_col = SA = sin(A) * S    (texture V change per column step)
;   du_row = -SA                (texture U change per row step)
;   dv_row = CA                 (texture V change per row step)
;   u0, v0 = starting coords   (centreing offsets)
;
; This is all in 16.16 fixed-point: upper 16 bits = integer texture coord,
; lower 16 bits = fractional sub-texel precision.
;
; === x86-SPECIFIC ARCHITECTURAL NOTES ===
; This demo uses NASM (Netwide Assembler) syntax with a 32-bit flat memory
; model (`bits 32`). All MMIO registers are accessed via absolute memory
; addresses in [brackets]. Key x86 advantages exploited here:
;
;   - MOVSX/MOVZX: sign/zero extend in a single instruction (no manual
;     bit manipulation needed as on 6502/Z80)
;   - IMUL r32,r32: signed 32x32->32 multiply in one instruction
;   - SHL/SHR: arbitrary shift amounts for fixed-point arithmetic
;   - Flat 32-bit addressing: no bank switching or segment hassles
;
; === MEMORY MAP ===
;
;   0x000000 +-----------------+
;            | System Vectors  |
;   0x001000 +-----------------+
;            | Program Code    |  <-- This program lives here
;            | (code + data)   |
;            |                 |
;   0x100000 +-----------------+
;            | VRAM (front     |  640x480x4 = 1,228,800 bytes
;            |  buffer)        |  Displayed on screen
;   0x200000 +-----------------+
;            |                 |
;   0x500000 +-----------------+
;            | Texture Buffer  |  256x256x4 = 262,144 bytes (stride 1024)
;            |                 |  Checkerboard pattern
;   0x600000 +-----------------+
;            | Back Buffer     |  640x480x4 = 1,228,800 bytes
;            |                 |  Mode7 renders here, then BLIT COPY to VRAM
;   0xFF0000 +-----------------+
;            | Stack           |  Grows downward
;            +-----------------+
;
; === BUILD AND RUN ===
;
;   Assemble: nasm -f bin -o assembler/rotozoomer_x86.ie86 assembler/rotozoomer_x86.asm
;   Run:      ./bin/IntuitionEngine -x86 assembler/rotozoomer_x86.ie86
;
; ============================================================================

; ============================================================================
; INCLUDE: HARDWARE DEFINITIONS
; ============================================================================
; ie86.inc provides all MMIO register addresses, blitter opcodes, audio
; register definitions, and helper macros for the IntuitionEngine x86 mode.
; Uses NASM %include directive (not MASM's include).
; ============================================================================

%include "ie86.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Texture Configuration ---
; WHY 0x500000: The texture must live ABOVE VRAM (0x100000-0x22C000) and
; above the back buffer (0x600000-0x72C000). 0x500000 sits in a safe gap.
; On systems with banked memory, this would be inaccessible -- but x86's
; flat 32-bit address space lets us place buffers anywhere.
TEXTURE_BASE    equ 0x500000

; WHY 0x600000 FOR BACK BUFFER: Double buffering requires two full-screen
; framebuffers. Mode7 renders to 0x600000 (off-screen), then a BLIT COPY
; transfers the completed frame to VRAM (0x100000) atomically. Without
; double buffering, the viewer would see the Mode7 blitter painting across
; the screen mid-frame, causing visible tearing.
BACK_BUFFER     equ 0x600000

; --- Screen Dimensions ---
; The VideoChip's default mode is 640x480 true color (32-bit BGRA per pixel).
; LINE_BYTES (from ie86.inc) = 640 * 4 = 2560 bytes per scanline.
RENDER_W        equ 640
RENDER_H        equ 480

; WHY STRIDE 1024: Each texture row is 256 pixels x 4 bytes/pixel = 1024
; bytes. The stride tells the blitter how many bytes to skip to reach the
; next row. This is separate from the texture width because textures can
; have padding between rows (though ours doesn't).
TEX_STRIDE      equ 1024

; --- Animation Accumulator Increments (8.8 Fixed-Point) ---
;
; WHY 8.8 FIXED-POINT ACCUMULATORS:
; We want smooth, fractional-step animation without floating point.
; The accumulator is a 16-bit value where:
;   - High byte (bits 15-8) = integer part (index into sine/recip table)
;   - Low byte (bits 7-0)   = fractional part (sub-step precision)
;
; After each frame, we add the increment and mask to 16 bits.
; The high byte wraps through 0-255 = one full revolution.
;
; WHY ANGLE_INC=313, SCALE_INC=104:
; These match the BASIC version's `A += 0.03, SI += 0.01` (a 3:1 ratio).
; Converting from radians-per-frame to 8.8 table increments:
;   0.03 radians * (256 / (2*pi)) * 256 = 312.7 -> 313
;   0.01 radians * (256 / (2*pi)) * 256 = 104.2 -> 104
;
; The 3:1 ratio means the rotation angle advances 3x faster than the zoom
; oscillation, creating a pleasing visual rhythm where the zoom cycles
; roughly once for every 3 rotations. ANGLE_INC=313 gives approximately
; 0.03 radians/frame, completing a full rotation in ~209 frames (~3.5s at
; 60fps). SCALE_INC=104 gives ~0.01 radians/frame for a smooth zoom
; oscillation period of ~628 frames (~10.5s).
ANGLE_INC       equ 313
SCALE_INC       equ 104

; ============================================================================
; PROGRAM START
; ============================================================================
; The IntuitionEngine x86 mode uses NASM's `bits 32` for a 32-bit flat
; memory model and `org 0x0000` for the program origin. The loader places
; code at PROGRAM_START (0x1000 per ie86.inc), but labels are relative to
; the org directive. This is the NASM equivalent of M68K's `org` directive.
; ============================================================================

bits 32
                org 0x0000

; ============================================================================
; ENTRY POINT
; ============================================================================
; The CPU begins execution here after loading the binary.
; We initialize all hardware subsystems in a specific order:
;   1. Stack pointer (MUST be first -- all CALL/RET depend on it)
;   2. VideoChip enable (required before any blitter operations)
;   3. Texture generation (blitter fills to create checkerboard)
;   4. Animation state (zero accumulators)
;   5. Music playback (runs autonomously in background)
; ============================================================================

start:
                ; --- Initialize Stack Pointer ---
                ; x86 uses a descending stack: ESP starts high, PUSH decrements it.
                ; STACK_TOP (0xFF0000 from ie86.inc) provides ample space above
                ; our code and below the MMIO registers (0xF0000+).
                ; This MUST be set before any CALL instruction, which pushes
                ; the return address onto the stack.
                mov esp, STACK_TOP

                ; --- Enable VideoChip ---
                ; The VideoChip (at VIDEO_CTRL / 0xF0000) manages the display and
                ; contains the blitter hardware. Writing 1 enables it; writing 0
                ; to VIDEO_MODE selects the default 640x480 true-color mode.
                ; CRITICAL: The blitter will silently do nothing if the VideoChip
                ; is not enabled. This is a common "nothing appears on screen" bug.
                mov dword [VIDEO_CTRL], 1
                mov dword [VIDEO_MODE], 0

                ; --- Generate Checkerboard Texture ---
                ; Create a 256x256 checkerboard pattern using 4 blitter fill
                ; operations (one per 128x128 quadrant). This texture will be
                ; mapped onto the screen by the Mode7 blitter every frame.
                call generate_texture

                ; --- Initialize Animation Accumulators ---
                ; Both start at zero: angle_accum controls rotation, scale_accum
                ; controls the zoom oscillation. They increment each frame by
                ; ANGLE_INC and SCALE_INC respectively.
                mov dword [angle_accum], 0
                mov dword [scale_accum], 0

                ; --- Start PSG Music Playback ---
                ; WHY PSG (AY-3-8910 / YM2149): Each IntuitionEngine demo
                ; showcases a different audio chip from computing history. The x86
                ; demo uses the PSG -- the sound chip found in the Atari ST,
                ; Amstrad CPC, and ZX Spectrum 128K. The YM music data may be
                ; LHA-compressed inside the .ym file; the engine decompresses it
                ; transparently at load time.
                ;
                ; PSG_PLUS_CTRL=1 enables enhanced PSG mode (extended features
                ; beyond the original AY-3-8910 spec).
                ;
                ; The playback pipeline:
                ;   1. Set PSG_PLUS_CTRL=1 to enable PSG+ features
                ;   2. Set PSG_PLAY_PTR to the start of .ym data in memory
                ;   3. Set PSG_PLAY_LEN to the file size in bytes
                ;   4. Set PSG_PLAY_CTRL=5 (bit 0 = start, bit 2 = loop)
                ;      This starts playback and automatically loops when the
                ;      song reaches its end.
                ;
                ; Once started, PSG playback runs autonomously -- the audio
                ; subsystem calls the YM player ~50 times/second to update
                ; PSG registers, independent of the main x86 CPU loop.
                mov dword [PSG_PLUS_CTRL], 1
                mov dword [PSG_PLAY_PTR], psg_data
                mov dword [PSG_PLAY_LEN], psg_data_end - psg_data
                mov dword [PSG_PLAY_CTRL], 5

; ============================================================================
; MAIN LOOP
; ============================================================================
; Runs once per frame at 60 FPS (synchronized by wait_vsync).
;
; The frame pipeline:
;   1. compute_frame:     CPU work (~6 multiplies, ~20 shifts/adds)
;   2. render_mode7:      Blitter work (307,200 pixels, hardware-accelerated)
;   3. blit_to_front:     Blitter work (307,200 pixel copy to VRAM)
;   4. wait_vsync:        Idle until vertical blanking interval
;   5. advance_animation: Increment accumulators (trivial)
;
; WHY THIS ORDER: We compute and render FIRST, then vsync, then advance.
; This means the frame we just displayed was computed during the PREVIOUS
; vsync interval. The alternative (vsync first, then compute) would also
; work but would make the first frame appear one refresh late.
; ============================================================================

main_loop:
                call compute_frame
                call render_mode7
                call blit_to_front
                call wait_vsync
                call advance_animation
                jmp main_loop

; ============================================================================
; WAIT FOR VSYNC (Two-Phase Vertical Blank Synchronization)
; ============================================================================
; Synchronizes the main loop to the display's 60 Hz refresh rate.
; Without vsync, the main loop would run as fast as possible, causing:
;   - Visual tearing (blit_to_front updating VRAM mid-scanout)
;   - Inconsistent animation speed (varies with CPU speed)
;
; WHY TWO-PHASE VSYNC:
; A single "wait until vblank" has a race condition: if we're ALREADY in
; the vblank interval when we check, we'd proceed immediately, potentially
; running multiple frames within a single vblank period.
;
; The two-phase approach guarantees exactly one frame per refresh:
;   Phase 1 (.wait_end):  Spin while vblank is ACTIVE (wait for it to end)
;   Phase 2 (.wait_start): Spin until vblank BEGINS (catch the leading edge)
;
; This ensures we always synchronize to the START of vblank, regardless
; of where we happen to be in the refresh cycle when we enter this routine.
;
; WHY `test eax, N` FOR BIT CHECKS:
; x86's TEST instruction performs a bitwise AND without modifying the
; destination register, then sets the Zero Flag (ZF) based on the result.
; JNZ/JZ branch on ZF. This is more efficient than using AND (which would
; need to preserve the register separately) or CMP (which would need a
; mask step first). TEST is the idiomatic x86 way to check individual bits.
;
; STATUS_VBLANK (value 2, ie86.inc) is the bit mask for the vblank flag
; in the VIDEO_STATUS register at 0xF0008.
; ============================================================================

wait_vsync:
.wait_end:      mov eax, [VIDEO_STATUS]
                test eax, STATUS_VBLANK
                jnz .wait_end
.wait_start:    mov eax, [VIDEO_STATUS]
                test eax, STATUS_VBLANK
                jz .wait_start
                ret

; ============================================================================
; GENERATE TEXTURE (256x256 Checkerboard via 4x BLIT FILL)
; ============================================================================
; Creates the texture that Mode7 will rotate and zoom. The checkerboard
; pattern is ideal for a rotozoomer because:
;   1. High contrast makes rotation clearly visible
;   2. Regular grid lines show zoom distortion intuitively
;   3. Tiling is seamless (the pattern wraps naturally at 256-pixel boundaries)
;
; WHY 4x BLIT FILL (NOT SOFTWARE):
; The blitter hardware can fill a rectangular region with a solid color in
; a single operation, far faster than a CPU loop writing individual pixels.
; We use 4 fills to create the 2x2 checkerboard pattern:
;
;   +-------+-------+      Each quadrant is 128x128 pixels (128x128x4 = 65536 bytes)
;   | WHITE | BLACK |      Total texture: 256x256x4 = 262,144 bytes
;   | (0,0) |(128,0)|
;   +-------+-------+
;   | BLACK | WHITE |
;   |(0,128)|(128,128)|
;   +-------+-------+
;
; WHY TEXTURE AT 0x500000, STRIDE 1024:
; The texture lives above VRAM (0x100000) to avoid conflicts. Each pixel
; is 4 bytes (BGRA), so a 256-pixel row = 256*4 = 1024 bytes stride.
; The Mode7 blitter uses TEX_W_MASK=255 and TEX_H_MASK=255 to wrap
; texture coordinates, creating infinite tiling from this 256x256 source.
;
; ADDRESS CALCULATIONS:
;   Top-left  (0,0):     TEXTURE_BASE + 0*TEX_STRIDE + 0*4 = 0x500000
;   Top-right (128,0):   TEXTURE_BASE + 0*TEX_STRIDE + 128*4 = 0x500000 + 512 = 0x500200
;   Bottom-left (0,128): TEXTURE_BASE + 128*TEX_STRIDE + 0*4 = 0x500000 + 131072 = 0x520000
;   Bottom-right (128,128): TEXTURE_BASE + 128*1024 + 128*4 = 0x500000 + 131584 = 0x520200
;
; WHY BLT_STATUS BIT 2 (mask value 2):
; Bit 1 of BLT_STATUS indicates the blitter is busy. We poll until it
; clears before issuing the next fill, because the blitter can only
; process one operation at a time.
; ============================================================================

generate_texture:
                ; --- Quadrant 1: Top-left 128x128 WHITE ---
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFFFFFFFF
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w1:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w1

                ; --- Quadrant 2: Top-right 128x128 BLACK ---
                ; Offset = 128 pixels * 4 bytes = 512 bytes from row start
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE+512
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFF000000
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w2:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w2

                ; --- Quadrant 3: Bottom-left 128x128 BLACK ---
                ; Offset = 128 rows * 1024 bytes/row = 131072 bytes from texture base
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE+131072
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFF000000
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w3:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w3

                ; --- Quadrant 4: Bottom-right 128x128 WHITE ---
                ; Offset = 128*1024 + 128*4 = 131072 + 512 = 131584
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE+131584
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFFFFFFFF
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w4:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w4

                ret

; ============================================================================
; COMPUTE FRAME - Calculate Mode7 Parameters from Animation State
; ============================================================================
; This is where the CPU does its per-frame work. The entire function
; computes just 6 values (u0, v0, CA, SA, -SA, CA) that the Mode7 blitter
; will use to transform 307,200 pixels. This extreme asymmetry -- a few
; CPU instructions controlling massive blitter work -- is the key insight
; of hardware-accelerated rendering.
;
; === THE MATH ===
;
; Given animation accumulators angle_accum and scale_accum:
;
;   angle_idx = angle_accum >> 8        (8.8 FP -> integer table index)
;   scale_idx = scale_accum >> 8
;
;   cos_val = sine_table[(angle_idx + 64) & 255]   (cosine via phase shift)
;   sin_val = sine_table[angle_idx]
;   recip   = recip_table[scale_idx]                (zoom factor)
;
;   CA = cos_val * recip                (combined rotation+zoom: column U delta)
;   SA = sin_val * recip                (combined rotation+zoom: column V delta)
;
;   u0 = 8388608 - CA*320 + SA*240     (texture start U, centreing offset)
;   v0 = 8388608 - SA*320 - CA*240     (texture start V, centreing offset)
;
; === WHY PROPER SINE TABLES ===
; Each entry is round(sin(i * 2*pi / 256) * 256), giving true circular
; rotation. Earlier implementations used triangle-wave approximations which
; had 29% error at 45 degrees, causing visible "diamond" distortion --
; the rotation would squash diagonally instead of being truly circular.
; 256 entries cover a full revolution at ~1.4-degree resolution.
;
; === WHY RECIPROCAL TABLE ===
; Each entry is round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3)).
;   - The 0.5 base keeps the zoom factor always positive (avoids inversion)
;   - The 0.3 amplitude gives smooth oscillation between ~1.25x and ~5x zoom
;   - Values range from 320 to 1280 (unsigned 16-bit)
; Pre-computing avoids runtime division, which is very expensive even on x86.
; The sine-based oscillation creates a smooth, organic zoom rhythm.
;
; === WHY u0/v0 CENTERING (8388608 = 128 << 16) ===
; 8388608 in 16.16 fixed-point represents 128.0, which is the centre of
; our 256x256 texture. Without this offset, the rotation pivot would be at
; texture coordinate (0,0) -- the top-left corner -- and the texture would
; orbit around that corner instead of spinning in place.
;
; The CA*320 and SA*240 terms represent half the screen dimensions:
;   320 = 640/2 (half screen width)
;   240 = 480/2 (half screen height)
; These offsets ensure the texture centre maps to the screen centre.
; Combined: the texture rotates around its own centre (128,128), and that
; centre is displayed at screen centre (320,240).
;
; === WHY MULTIPLY BY SHIFTS (CA*320 = CA*256 + CA*64) ===
; 320 is not a power of 2, so we decompose: 320 = 256 + 64 = 2^8 + 2^6.
; Similarly: 240 = 256 - 16 = 2^8 - 2^4.
; Using shifts and adds is faster than a general multiply on many CPUs,
; and demonstrates a classic demoscene optimization. On modern x86, IMUL
; is fast enough that this wouldn't matter, but it's educational to show
; the decomposition.
;
; === x86-SPECIFIC: WHY MOVSX AND MOVZX ===
; MOVSX (Move with Sign Extension) takes a 16-bit value from memory and
; sign-extends it to 32 bits in a register. For sine values that can be
; negative (-256 to +256), this preserves the sign correctly.
;   `movsx ecx, word [addr]` : if memory contains 0xFF00 (-256 as int16),
;   ecx becomes 0xFFFFFF00 (-256 as int32). Without MOVSX, the upper 16
;   bits would be zero, giving 0x0000FF00 = 65280 (wrong!).
;
; MOVZX (Move with Zero Extension) zero-extends to 32 bits. For the
; reciprocal table values that are always positive (320-1280), this is
; correct -- we never want sign extension on an unsigned value.
;
; On Z80 or 6502, these extensions require multiple instructions. x86
; does it in one, which is a significant architectural advantage.
;
; === x86-SPECIFIC: WHY IMUL eax, ebx ===
; x86's `IMUL r32, r32` performs a signed 32x32 -> 32-bit multiply in a
; single instruction. Since our operands fit in 16 bits (sine: -256..+256,
; recip: 320..1280) and the product fits in 32 bits (max magnitude:
; 256 * 1280 = 327,680), the low 32 bits are exact -- no need for the
; 64-bit IMUL variant or multi-precision math. The result is a 16.16
; fixed-point value ready for use as a Mode7 parameter.
; ============================================================================

compute_frame:
                ; --- Extract integer table indices from 8.8 accumulators ---

                ; angle_idx = angle_accum >> 8  (discard fractional bits)
                ; The AND 255 masks to 8 bits, ensuring wrap-around through
                ; the 256-entry sine table (one full revolution).
                mov eax, [angle_accum]
                shr eax, 8
                and eax, 255
                mov esi, eax                    ; esi = angle_idx

                ; scale_idx = scale_accum >> 8
                ; Same extraction for the zoom oscillation index.
                mov eax, [scale_accum]
                shr eax, 8
                and eax, 255
                mov edi, eax                    ; edi = scale_idx

                ; --- Look up trigonometric values ---

                ; cos_val = sine_table[(angle_idx + 64) & 255]
                ; WHY +64: In our 256-entry system, 64 = 256/4 = 90 degrees.
                ; cos(x) = sin(x + 90), so we reuse the sine table for cosine
                ; by adding a quarter-revolution offset.
                mov eax, esi
                add eax, 64
                and eax, 255
                movsx ecx, word [sine_table + eax*2]    ; ecx = cos_val (signed)

                ; sin_val = sine_table[angle_idx]
                ; MOVSX sign-extends the 16-bit table value to 32 bits,
                ; preserving negative values (sine ranges from -256 to +256).
                movsx edx, word [sine_table + esi*2]    ; edx = sin_val (signed)

                ; recip = recip_table[scale_idx]
                ; MOVZX zero-extends because reciprocal values are always
                ; positive (range 320-1280). Using MOVSX here would be wrong
                ; for values >= 32768 (none exist in our table, but MOVZX is
                ; semantically correct for unsigned data).
                movzx ebx, word [recip_table + edi*2]   ; ebx = recip (unsigned)

                ; --- Compute combined rotation+zoom factors ---

                ; CA = cos_val * recip
                ; This single IMUL combines rotation angle AND zoom into one
                ; value. The result is in 16.16 fixed-point: if cos_val=256
                ; (=1.0 in 8.8) and recip=512 (=2.0 in 8.8), then
                ; CA = 256*512 = 131072, which in 16.16 FP = 2.0.
                mov eax, ecx
                imul eax, ebx                   ; eax = CA (16.16 FP)
                mov [var_ca], eax

                ; SA = sin_val * recip
                ; Same combination for the sine component.
                mov eax, edx
                imul eax, ebx                   ; eax = SA (16.16 FP)
                mov [var_sa], eax

                ; --- Compute texture starting coordinates (u0, v0) ---
                ; These offsets ensure the rotation is centreed on both the
                ; texture (at texel 128,128) and the screen (at pixel 320,240).

                ; u0 = 8388608 - CA*320 + SA*240
                ;
                ; CA*320 via shift decomposition:
                ;   320 = 256 + 64, so CA*320 = (CA << 8) + (CA << 6)
                mov eax, [var_ca]
                mov ecx, eax
                shl eax, 8                      ; CA * 256
                shl ecx, 6                      ; CA * 64
                add eax, ecx                    ; eax = CA * 320

                ; SA*240 via shift decomposition:
                ;   240 = 256 - 16, so SA*240 = (SA << 8) - (SA << 4)
                mov ecx, [var_sa]
                mov edx, ecx
                shl ecx, 8                      ; SA * 256
                shl edx, 4                      ; SA * 16
                sub ecx, edx                    ; ecx = SA * 240

                ; Combine: u0 = centre - half_width_offset + half_height_offset
                mov edx, 0x800000               ; 8388608 = 128.0 in 16.16 FP
                sub edx, eax                    ; - CA*320
                add edx, ecx                    ; + SA*240
                mov [var_u0], edx

                ; v0 = 8388608 - SA*320 - CA*240
                ;
                ; SA*320 = (SA << 8) + (SA << 6)
                mov eax, [var_sa]
                mov ecx, eax
                shl eax, 8                      ; SA * 256
                shl ecx, 6                      ; SA * 64
                add eax, ecx                    ; eax = SA * 320

                ; CA*240 = (CA << 8) - (CA << 4)
                mov ecx, [var_ca]
                mov edx, ecx
                shl ecx, 8                      ; CA * 256
                shl edx, 4                      ; CA * 16
                sub ecx, edx                    ; ecx = CA * 240

                ; Combine: v0 = centre - half_width_offset - half_height_offset
                ; Note the MINUS for CA*240 here vs PLUS in u0 -- this comes
                ; from the rotation matrix: the row deltas use [-SA, CA]
                ; (negated sine for U, positive cosine for V).
                mov edx, 0x800000               ; 8388608 = 128.0 in 16.16 FP
                sub edx, eax                    ; - SA*320
                sub edx, ecx                    ; - CA*240
                mov [var_v0], edx

                ret

; ============================================================================
; RENDER MODE7 - Configure and Trigger Mode7 Affine Blit
; ============================================================================
; Programs the blitter's Mode7 registers and executes the affine texture
; mapping operation. This is where the hardware does the heavy lifting.
;
; === MODE7 BLITTER REGISTER SUMMARY ===
;
; The blitter needs to know:
;   BLT_SRC / BLT_SRC_STRIDE: Where the texture lives and its row stride
;   BLT_DST / BLT_DST_STRIDE: Where to write the output and its row stride
;   BLT_WIDTH / BLT_HEIGHT:   Output dimensions in pixels
;   BLT_MODE7_TEX_W/H:        Texture wrap masks (255 for 256x256)
;   BLT_MODE7_U0/V0:          Starting texture coordinates (16.16 FP)
;   BLT_MODE7_DU_COL/DV_COL:  Texture delta per column step (16.16 FP)
;   BLT_MODE7_DU_ROW/DV_ROW:  Texture delta per row step (16.16 FP)
;
; === THE MODE7 MATRIX ===
;
; The six parameters encode a 2x2 affine transformation matrix plus offset:
;
;   [ du_col  du_row ]   [ CA  -SA ]
;   [ dv_col  dv_row ] = [ SA   CA ]
;
; Column deltas (du_col, dv_col): How texture coords change as we move
;   RIGHT across the screen. du_col = CA means "moving right in screen space
;   moves right in texture space (scaled by zoom)". dv_col = SA means
;   "moving right also moves down in texture space (due to rotation)".
;
; Row deltas (du_row, dv_row): How texture coords change as we move
;   DOWN the screen. du_row = -SA, dv_row = CA complete the rotation matrix.
;
; This is a standard 2D rotation matrix [[cos,-sin],[sin,cos]] scaled by
; the zoom factor (already baked into CA and SA from compute_frame).
;
; WHY RENDER TO BACK BUFFER (0x600000) NOT DIRECTLY TO VRAM:
; The Mode7 blit takes time. If we wrote directly to VRAM (which is being
; scanned out to the display), the viewer would see the blit in progress --
; old frame at the bottom, new frame painting from the top. Rendering to an
; off-screen buffer and then copying the complete result prevents this
; tearing artifact.
; ============================================================================

render_mode7:
                mov dword [BLT_OP], BLT_OP_MODE7
                mov dword [BLT_SRC], TEXTURE_BASE
                mov dword [BLT_DST], BACK_BUFFER
                mov dword [BLT_WIDTH], RENDER_W
                mov dword [BLT_HEIGHT], RENDER_H
                mov dword [BLT_SRC_STRIDE], TEX_STRIDE
                mov dword [BLT_DST_STRIDE], LINE_BYTES

                ; Texture wrap masks: 255 for a 256x256 texture.
                ; The blitter computes (u & 255) and (v & 255) to wrap texture
                ; coordinates, creating seamless infinite tiling. For a 128x128
                ; texture you'd use 127, for 512x512 use 511, etc. The mask
                ; must be (power_of_2 - 1).
                mov dword [BLT_MODE7_TEX_W], 255
                mov dword [BLT_MODE7_TEX_H], 255

                ; --- Load the 6 affine parameters computed by compute_frame ---

                ; Starting texture coordinates (top-left screen pixel maps here)
                mov eax, [var_u0]
                mov [BLT_MODE7_U0], eax

                mov eax, [var_v0]
                mov [BLT_MODE7_V0], eax

                ; Column deltas: how texture coords change per horizontal pixel
                mov eax, [var_ca]
                mov [BLT_MODE7_DU_COL], eax     ; du_col = CA

                mov eax, [var_sa]
                mov [BLT_MODE7_DV_COL], eax     ; dv_col = SA

                ; Row deltas: how texture coords change per vertical scanline
                ; du_row = -SA (negate SA for the rotation matrix's -sin term)
                neg eax                          ; -SA
                mov [BLT_MODE7_DU_ROW], eax     ; du_row = -SA

                mov eax, [var_ca]
                mov [BLT_MODE7_DV_ROW], eax     ; dv_row = CA

                ; --- Trigger the blit and wait for completion ---
                ; Writing 1 to BLT_CTRL starts the operation. The blitter runs
                ; asynchronously; we must poll BLT_STATUS until it signals done.
                mov dword [BLT_CTRL], 1

                ; WHY POLL BLT_STATUS BIT 1 (mask value 2):
                ; BLT_STATUS bit 1 = busy flag. While this bit is set, the
                ; blitter is still processing. We spin-wait here because we
                ; need the result before we can blit to front.
.wait:          mov eax, [BLT_STATUS]
                test eax, 2
                jnz .wait

                ret

; ============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM)
; ============================================================================
; WHY DOUBLE BUFFERING: Mode7 renders to BACK_BUFFER (0x600000), and this
; routine copies the completed frame to VRAM_START (0x100000). The display
; hardware scans out from VRAM, so the viewer only ever sees complete frames.
;
; Without double buffering, the Mode7 blitter would write directly to VRAM
; while the display is reading from it, causing visible tearing -- the top
; portion would show the new frame while the bottom shows the old one.
;
; The copy uses BLT_OP_COPY which is a simple memcpy-like block transfer.
; LINE_BYTES (2560 = 640*4) is the stride for both source and destination
; since both buffers have the same 640-pixel-wide layout.
; ============================================================================

blit_to_front:
                mov dword [BLT_OP], BLT_OP_COPY
                mov dword [BLT_SRC], BACK_BUFFER
                mov dword [BLT_DST], VRAM_START
                mov dword [BLT_WIDTH], RENDER_W
                mov dword [BLT_HEIGHT], RENDER_H
                mov dword [BLT_SRC_STRIDE], LINE_BYTES
                mov dword [BLT_DST_STRIDE], LINE_BYTES
                mov dword [BLT_CTRL], 1

.wait:          mov eax, [BLT_STATUS]
                test eax, 2
                jnz .wait

                ret

; ============================================================================
; ADVANCE ANIMATION
; ============================================================================
; Increments the two animation accumulators and wraps them to 16 bits.
;
; The accumulators are 8.8 fixed-point counters:
;   - Bits 15-8 (high byte): Integer part = table index (0-255)
;   - Bits 7-0 (low byte):   Fractional part = sub-step precision
;
; WHY AND 0xFFFF: Masking to 16 bits ensures the accumulator wraps around
; after the high byte passes 255 (one full revolution / one full zoom cycle).
; Without this mask, the accumulator would grow indefinitely, and after
; reaching 0x10000 the high byte would enter a second "revolution" at
; index 256+ which is outside our 256-entry tables.
;
; WHY SEPARATE INCREMENTS (NOT IN MAIN LOOP):
; Keeping animation advancement in its own function makes the main loop
; structure clearer and allows easy modification (e.g., changing speeds,
; adding acceleration, or pausing animation).
; ============================================================================

advance_animation:
                mov eax, [angle_accum]
                add eax, ANGLE_INC
                and eax, 0xFFFF
                mov [angle_accum], eax

                mov eax, [scale_accum]
                add eax, SCALE_INC
                and eax, 0xFFFF
                mov [scale_accum], eax

                ret

; ============================================================================
; VARIABLES (Runtime State)
; ============================================================================
; These are modified every frame by the compute and animation routines.
;
; WHY DD (DWORD) FOR EVERYTHING: All values are 32-bit to match x86's
; natural register width. Using smaller sizes would require sign/zero
; extension on every access, adding unnecessary instructions.
;
; angle_accum: 8.8 FP rotation angle accumulator.
;   High byte cycles 0-255 = one full rotation. At ANGLE_INC=313 per frame
;   and 60fps, one full rotation takes 256*256/313/60 = ~3.5 seconds.
;
; scale_accum: 8.8 FP zoom oscillation accumulator.
;   Indexes into recip_table for zoom factor. At SCALE_INC=104 per frame,
;   one full oscillation takes 256*256/104/60 = ~10.5 seconds.
;
; var_ca, var_sa: Combined rotation+zoom factors (16.16 FP).
;   These are the raw column deltas passed to Mode7. Recomputed every frame.
;
; var_u0, var_v0: Texture starting coordinates (16.16 FP).
;   Where the top-left screen pixel maps to in texture space. Includes
;   centreing offsets. Recomputed every frame.
; ============================================================================

angle_accum:    dd 0
scale_accum:    dd 0
var_ca:         dd 0
var_sa:         dd 0
var_u0:         dd 0
var_v0:         dd 0

; ============================================================================
; SINE TABLE - 256 Entries, Signed 16-bit (8.8 Fixed-Point)
; ============================================================================
; Precomputed sine values for angles 0-255 representing 0 to 360 degrees.
; Each entry is round(sin(i * 2*pi / 256) * 256).
; Values range from -256 to +256, representing -1.0 to +1.0 in 8.8 FP.
;
; WHY A LOOKUP TABLE (NOT RUNTIME CALCULATION):
; Computing sine requires a Taylor series (5+ multiplies and adds per call)
; or CORDIC algorithm (16+ iterations). A 512-byte table (256 entries x
; 2 bytes each) gives instant O(1) results. This was the universal approach
; on every 8/16/32-bit platform in the demoscene.
;
; WHY 256 ENTRIES:
; 256 = 2^8, so a full revolution maps to exactly one byte of index range.
; This means angle wrapping is a simple AND 255 (or implicit byte overflow).
; It gives ~1.4-degree resolution, which is more than sufficient for smooth
; animation at 60 fps. Higher-resolution tables (512 or 1024 entries) would
; give marginally smoother motion but waste cache/memory for negligible gain.
;
; HOW TO USE:
;   sin(angle) = sine_table[angle]         (index 0-255, value -256..+256)
;   cos(angle) = sine_table[(angle+64)&255] (cosine = sine phase-shifted 90)
;
; TABLE LAYOUT (4 quadrants of 64 entries each):
;   Indices   0-63:   sin rises from 0 to +256  (0 to 90 degrees)
;   Indices  64-127:  sin falls from +256 to 0   (90 to 180 degrees)
;   Indices 128-191:  sin falls from 0 to -256   (180 to 270 degrees)
;   Indices 192-255:  sin rises from -256 to 0   (270 to 360 degrees)
;
; GENERATION FORMULA (Python):
;   for i in range(256):
;       print(int(round(math.sin(i * 2 * math.pi / 256) * 256)), end=',')
; ============================================================================

sine_table:
                ; Quadrant 1: 0 to 90 degrees (indices 0-63)
                ; Values rise smoothly from 0 to 256 (0.0 to 1.0)
                dw 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
                dw 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
                dw 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
                dw 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256

                ; Quadrant 2: 90 to 180 degrees (indices 64-127)
                ; Mirror of quadrant 1, values fall from 256 back to 0
                dw 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
                dw 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
                dw 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
                dw 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6

                ; Quadrant 3: 180 to 270 degrees (indices 128-191)
                ; Negative mirror of quadrant 1, values fall from 0 to -256
                dw 0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
                dw -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
                dw -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
                dw -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256

                ; Quadrant 4: 270 to 360 degrees (indices 192-255)
                ; Negative mirror of quadrant 2, values rise from -256 back to 0
                dw -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
                dw -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
                dw -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
                dw -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

; ============================================================================
; RECIPROCAL TABLE - 256 Entries, Unsigned 16-bit (Zoom Factor)
; ============================================================================
; Precomputed zoom values: round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
;
; WHY A RECIPROCAL TABLE (NOT RUNTIME DIVISION):
; Division is the most expensive arithmetic operation on nearly every CPU.
; Even x86's IDIV takes 20-40 cycles versus 3-4 for IMUL. Pre-computing
; the reciprocal eliminates all runtime division from the animation loop.
;
; WHY THIS SPECIFIC FORMULA:
;   base = 0.5 + sin(i) * 0.3
;   recip = 256 / base
;
; The 0.5 base ensures the denominator is always positive (range 0.2 to 0.8),
; avoiding division by zero or negative zoom (which would flip the texture).
; The 0.3 amplitude gives a gentle oscillation: when sin=+1, base=0.8,
; recip=320 (1.25x zoom, slightly zoomed in); when sin=-1, base=0.2,
; recip=1280 (5x zoom, heavily zoomed in). This creates a smooth,
; hypnotic breathing effect as the texture alternately shrinks and expands.
;
; VALUE RANGE AND FIXED-POINT INTERPRETATION:
;   Minimum: 320 (at sin=+1, denominator=0.8) -> zoom factor ~1.25x
;   Maximum: 1280 (at sin=-1, denominator=0.2) -> zoom factor ~5.0x
;   Midpoint: 512 (at sin=0, denominator=0.5) -> zoom factor 2.0x
;
; When multiplied by sine/cosine values (range -256..+256) in compute_frame,
; the products range from -327,680 to +327,680, well within 32-bit signed
; range. The result is a 16.16 fixed-point Mode7 parameter.
;
; TABLE LAYOUT:
;   Index 0: sin(0)=0, base=0.5, recip=512
;   Index 64: sin(90)=1, base=0.8, recip=320  (minimum zoom)
;   Index 128: sin(180)=0, base=0.5, recip=512
;   Index 192: sin(270)=-1, base=0.2, recip=1280 (maximum zoom)
;
; GENERATION FORMULA (Python):
;   for i in range(256):
;       base = 0.5 + math.sin(i * 2 * math.pi / 256) * 0.3
;       print(int(round(256 / base)), end=',')
; ============================================================================

recip_table:
                ; Indices 0-63: zoom decreasing (recip falling from 512 to 320)
                ; Sine is rising 0 -> 1, so denominator rises 0.5 -> 0.8
                dw 512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
                dw 416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
                dw 359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
                dw 329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320

                ; Indices 64-127: zoom increasing (recip rising from 320 to 512)
                ; Sine is falling 1 -> 0, so denominator falls 0.8 -> 0.5
                dw 320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
                dw 329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
                dw 359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
                dw 416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505

                ; Indices 128-191: zoom increasing further (recip rising 512 to 1280)
                ; Sine is falling 0 -> -1, so denominator falls 0.5 -> 0.2
                dw 512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
                dw 665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
                dw 889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
                dw 1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279

                ; Indices 192-255: zoom decreasing (recip falling from 1280 to 512)
                ; Sine is rising -1 -> 0, so denominator rises 0.2 -> 0.5
                dw 1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
                dw 1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
                dw 889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
                dw 665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; ============================================================================
; MUSIC DATA - PSG (AY-3-8910 / YM2149) FORMAT
; ============================================================================
; WHY PSG MUSIC:
; Each IntuitionEngine demo showcases a different audio chip from computing
; history. The x86 rotozoomer uses the PSG -- the AY-3-8910 / YM2149 sound
; chip found in:
;   - Atari ST (YM2149)
;   - Amstrad CPC (AY-3-8912)
;   - ZX Spectrum 128K (AY-3-8912)
;   - MSX computers (AY-3-8910)
;   - Intellivision (AY-3-8914)
;
; The .ym file format stores register dumps of the PSG chip at 50Hz (PAL)
; or 60Hz (NTSC). Each frame contains the values for all 14 PSG registers.
; The data is typically LHA-compressed; the IntuitionEngine decompresses it
; transparently when loading.
;
; HOW PSG PLAYBACK WORKS:
; Unlike SID playback (which executes real 6502 code), PSG playback is
; simpler: the audio subsystem reads register values from the .ym data
; and writes them directly to the PSG emulation registers at the correct
; rate. No CPU emulation is needed -- it's a register-value replay.
;
; The PSG has 3 square-wave tone channels, 1 noise channel, and a hardware
; envelope generator. Despite its simplicity, skilled composers created
; remarkable music within these constraints, as this demo demonstrates.
;
; INCBIN loads the raw file contents at this position in the binary.
; psg_data_end - psg_data gives the file size for PSG_PLAY_LEN.
; ============================================================================

psg_data:
                incbin "../assets/OverscanScreen.ym"
psg_data_end:
