; ============================================================================
; MODE7 HARDWARE BLITTER ROTOZOOMER - Z80 SDK REFERENCE IMPLEMENTATION
; Zilog Z80 Assembly for IntuitionEngine - VideoChip Mode (640x480x32bpp)
; ============================================================================
;
; REFERENCE IMPLEMENTATION FOR DEMOSCENE TECHNIQUES ON AN 8-BIT CPU
; This file is heavily commented to teach Mode7 blitter programming,
; Z80-specific workarounds, and fixed-point math on constrained hardware.
;
; === WHAT THIS DEMO DOES ===
; 1. Generates a 256x256 checkerboard texture in off-screen memory
; 2. Computes a per-frame affine transformation matrix (rotation + zoom)
; 3. Delegates the full 640x480 pixel transform to the Mode7 hardware blitter
; 4. Double-buffers through an off-screen back buffer to prevent tearing
; 5. Plays SID music through the 6502 audio subsystem in the background
;
; === WHY MODE7 HARDWARE BLITTER ===
; The "Mode7" blitter performs affine texture mapping: it reads a source
; texture and writes transformed pixels to a destination buffer, applying
; rotation, scaling, and translation per-scanline. This is inspired by the
; SNES Mode 7 chip, which enabled effects like the rotating floors in
; Super Mario Kart and F-Zero.
;
; The critical insight is DELEGATION. A 4MHz Z80 running software pixel
; loops could never transform 307,200 pixels (640x480) at 60fps. The CPU
; would need to compute U/V texture coordinates per pixel, fetch texels,
; and write RGBA values -- roughly 20 instructions per pixel minimum.
; That is 6,144,000 instructions per frame, or 368,640,000 per second at
; 60fps. A Z80 at 4MHz executes ~1,000,000 instructions/second. The CPU
; is literally 368x too slow.
;
; Instead, the Z80 computes just 6 parameters per frame (U0, V0, dU/col,
; dV/col, dU/row, dV/row) and hands them to the Mode7 blitter, which
; processes all 307,200 pixels in hardware. The CPU does ~500 instructions
; of math per frame. This is the fundamental pattern for 8-bit demoscene
; programming: use the CPU for control, use hardware for throughput.
;
; === WHY A ROTOZOOMER (HISTORICAL CONTEXT) ===
; The rotozoomer was a signature demoscene effect from the early 1990s.
; It demonstrates mastery of affine texture mapping -- rotating and zooming
; a texture in real-time. On the Amiga and Atari ST, this was typically
; done in software with heavy inner-loop optimization. On the SNES, Mode 7
; provided it in hardware. This demo recreates the effect using the
; IntuitionEngine's blitter, showing how an 8-bit Z80 can drive visuals
; that would normally require a 16-bit or 32-bit CPU.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +------------------------------------------------------------------+
;   |                    MAIN LOOP (60 FPS)                            |
;   |                                                                  |
;   |  +--------------+   +--------------+   +--------------+          |
;   |  | compute_frame|-->| render_mode7 |-->|blit_to_front |          |
;   |  | (6 params)   |   | (HW blitter) |   | (HW copy)    |          |
;   |  +--------------+   +--------------+   +--------------+          |
;   |        |                                      |                  |
;   |        v                                      v                  |
;   |  +--------------+                      +--------------+          |
;   |  |   advance    |<---------------------| WAIT_VBLANK  |          |
;   |  |  animation   |                      | (two-phase)  |          |
;   |  +--------------+                      +--------------+          |
;   +------------------------------------------------------------------+
;
;   +------------------------------------------------------------------+
;   |              SID AUDIO SUBSYSTEM (runs in parallel)              |
;   |                                                                  |
;   |  A real 6502 CPU core executes the SID player code from the     |
;   |  embedded .sid file. Register writes to $D400-$D418 are         |
;   |  intercepted and remapped to the native synthesizer engine.     |
;   |  The Z80 main loop is completely unaware of audio playback.     |
;   +------------------------------------------------------------------+
;
; === MEMORY MAP ===
;
;   Address       Size     Purpose
;   -----------   ------   ----------------------------------------
;   0x000000+     ~2KB     Z80 program code + tables (this file)
;   0x100000      1.2MB    VRAM front buffer (640x480x32bpp)
;   0x500000      256KB    Texture (256x256x32bpp, 1024-byte stride)
;   0x600000      1.2MB    Back buffer for Mode7 rendering
;   0xF000-0xFFFF          MMIO registers (mapped from 0xF0000+)
;
;   Note: Z80 has a 16-bit address bus (0x0000-0xFFFF). The bus adapter
;   maps Z80 addresses 0xF000+ to physical 0xF0000+ via the formula:
;     physical = z80_addr - 0xF000 + 0xF0000
;   This lets us access 20-bit MMIO addresses from 16-bit Z80 code.
;   Addresses above 0xFFFF (VRAM, texture, back buffer) are accessed
;   indirectly through blitter registers, never by Z80 load/store.
;
; === BUILD AND RUN ===
;
;   Assemble:
;     vasmz80_std -Fbin -I. -o assembler/rotozoomer_z80.ie80 assembler/rotozoomer_z80.asm
;
;   Run:
;     ./bin/IntuitionEngine -z80 assembler/rotozoomer_z80.ie80
;
; ============================================================================

    .include "ie80.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Texture Configuration ---
; The texture lives in bus memory above VRAM, at address 0x500000.
; We use bus memory (not VRAM) because VRAM at 0x100000 is the front buffer
; being displayed. The texture must be in a separate region so the blitter
; can read it while the display hardware reads VRAM.
.set TEXTURE_BASE,0x500000

; --- Double Buffer ---
; Mode7 renders to this off-screen buffer at 0x600000, NOT directly to VRAM.
; After rendering completes, we BLIT COPY the result to VRAM (0x100000).
; This prevents tearing: the display always shows a complete frame, never
; a partially-rendered one. Without double buffering, the top half of the
; screen might show the new frame while the bottom half shows the old one.
.set BACK_BUFFER,0x600000

; --- Screen Dimensions ---
; 640x480 is the VideoChip's native resolution in mode 0 (32-bit RGBA).
; Each pixel is 4 bytes (RGBA), so each scanline is 640*4 = 2560 bytes.
; The total framebuffer is 640*480*4 = 1,228,800 bytes (~1.2MB).
.set RENDER_W,640
.set RENDER_H,480
.set LINE_BYTES,2560           ; 640 pixels * 4 bytes/pixel (RGBA)

; --- Texture Geometry ---
; The texture is 256x256 pixels but stored with a 1024-byte stride.
; WHY 1024? Each pixel is 4 bytes (RGBA), so 256 pixels = 1024 bytes.
; The stride must match the actual byte width so the blitter can advance
; scanline-by-scanline through the texture correctly.
.set TEX_STRIDE,1024

; --- VRAM Address ---
; The front buffer starts at 0x100000 (defined by VideoChip hardware).
; This is where the display controller reads pixels from each frame.
.set VRAM_START,0x100000

; --- Blitter Operation Codes ---
; These select which operation the blitter performs when triggered.
; BLT_OP_FILL (1):  Fills a rectangle with a solid color
; BLT_OP_COPY (0):  Copies a rectangle from source to destination
; BLT_OP_MODE7 (5): Performs affine texture mapping (rotation + zoom)
.set BLT_OP_FILL,1
.set BLT_OP_COPY,0
.set BLT_OP_MODE7,5

; --- Animation Accumulator Increments ---
; These control the speed of rotation and zoom oscillation.
; Both accumulators are 16-bit values interpreted as 8.8 fixed-point:
; the high byte is the table index (0-255), the low byte is the fraction.
;
; ANGLE_INC = 313: Each frame adds 313 to the 16-bit accumulator.
;   313/256 ~ 1.22 table indices per frame. A full rotation (256 indices)
;   takes 256/(313/256) ~ 210 frames ~ 3.5 seconds at 60fps.
;
; SCALE_INC = 104: Each frame adds 104 to the 16-bit accumulator.
;   104/256 ~ 0.41 table indices per frame. A full zoom cycle (256 indices)
;   takes 256/(104/256) ~ 630 frames ~ 10.5 seconds at 60fps.
;
; These values were chosen to match the BASIC reference implementation's
; rates of A+=0.03 and SI+=0.01 respectively (approximately a 3:1 ratio).
; The 3:1 ratio ensures rotation and zoom don't sync up, creating an
; ever-changing visual pattern that doesn't repeat for a long time.
.set ANGLE_INC,313
.set SCALE_INC,104

; ============================================================================
; ENTRY POINT
; ============================================================================
; The Z80 CPU begins execution at address 0x0000.
; We must initialize the stack, enable video, generate the texture,
; set up animation state, and start audio before entering the main loop.
; ============================================================================

    .org 0x0000

start:
    ; --- Initialize Stack Pointer ---
    ; Z80 uses a descending stack (SP decrements on PUSH, increments on POP).
    ; STACK_TOP is defined in ie80.inc as 0xF000, which is the highest RAM
    ; address before MMIO space begins. This gives us the full 0x0000-0xEFFF
    ; range for code, data, and stack combined.
    ld sp,STACK_TOP

    ; --- Enable VideoChip ---
    ; The VideoChip is the IntuitionEngine's native display controller.
    ; Writing 1 to VIDEO_CTRL enables it. Writing 0 to VIDEO_MODE selects
    ; mode 0 (640x480, 32-bit RGBA). The blitter and Mode7 engine are
    ; part of the VideoChip, so it MUST be enabled before any blit operations.
    ld a,1
    ld (VIDEO_CTRL),a
    xor a
    ld (VIDEO_MODE),a

    ; --- Generate Checkerboard Texture ---
    ; Creates a 256x256 checkerboard pattern using 4 BLIT FILL operations.
    ; This texture will be read by the Mode7 blitter every frame as the
    ; source for the affine transformation. The checkerboard is the classic
    ; rotozoomer texture because it makes rotation and scaling clearly visible.
    call generate_texture

    ; --- Initialize Animation Accumulators ---
    ; Both accumulators start at zero. They are 16-bit values that wrap
    ; naturally at 65536 due to Z80's 16-bit arithmetic overflow.
    ; The high byte of each accumulator is used as the table index (0-255),
    ; giving smooth sub-index-level animation progression.
    ld hl,0
    ld (angle_accum),hl
    ld (scale_accum),hl

    ; --- Start SID Music Playback ---
    ; The SID audio subsystem expects a pointer and length to the .sid file
    ; data, then a control byte to begin playback. The .sid file contains
    ; actual 6502 machine code that is executed by the IntuitionEngine's
    ; real 6502 CPU core. Register writes to $D400-$D418 (SID chip) are
    ; intercepted and remapped to the native synthesizer.
    ;
    ; WHY BYTE-BY-BYTE MMIO WRITES: Z80's address bus is only 16 bits
    ; (0x0000-0xFFFF), but IntuitionEngine MMIO registers live at 20-bit
    ; physical addresses (0xF0000+). The bus adapter maps Z80 addresses
    ; 0xF000+ to physical 0xF0000+ via:
    ;   physical_addr = z80_addr - 0xF000 + 0xF0000
    ;
    ; SID player registers are at physical 0xF0E20+, which maps to Z80
    ; address 0xFE20 (0xF0E20 - 0xF0000 + 0xF000 = 0xFE20).
    ;
    ; We CANNOT use the SET_SID_PTR/SET_SID_LEN/SET_SID_LOOP macros from
    ; ie80.inc because they embed 20-bit physical addresses (e.g., 0xF0E20)
    ; directly. vasm truncates these to 16-bit, producing wrong addresses
    ; (0x0E20 instead of 0xFE20), causing writes to hit RAM instead of MMIO.
    ; Writing individual bytes to 0xFE20..0xFE28 works correctly because
    ; the bus adapter maps each byte write independently.
    ;
    ; Byte layout (little-endian 32-bit pointer):
    ;   0xFE20 = SID data pointer byte 0 (LSB)
    ;   0xFE21 = SID data pointer byte 1
    ;   0xFE22 = SID data pointer byte 2
    ;   0xFE23 = SID data pointer byte 3 (MSB)
    ;   0xFE24 = SID data length byte 0 (LSB)
    ;   0xFE25 = SID data length byte 1
    ;   0xFE26 = SID data length byte 2
    ;   0xFE27 = SID data length byte 3 (MSB)
    ;   0xFE28 = SID play control (5 = start + loop)
    ld a,sid_data & 0xFF
    ld (0xFE20),a
    ld a,(sid_data >> 8) & 0xFF
    ld (0xFE21),a
    ld a,(sid_data >> 16) & 0xFF
    ld (0xFE22),a
    ld a,(sid_data >> 24) & 0xFF
    ld (0xFE23),a
    ld a,(sid_data_end-sid_data) & 0xFF
    ld (0xFE24),a
    ld a,((sid_data_end-sid_data) >> 8) & 0xFF
    ld (0xFE25),a
    ld a,((sid_data_end-sid_data) >> 16) & 0xFF
    ld (0xFE26),a
    ld a,((sid_data_end-sid_data) >> 24) & 0xFF
    ld (0xFE27),a
    ld a,5                     ; Control: bit 0 = start, bit 2 = loop
    ld (0xFE28),a

; ============================================================================
; MAIN LOOP
; ============================================================================
; This loop runs once per frame (60fps when vsync-locked).
; The order of operations is critical for tear-free rendering:
;
; 1. compute_frame:    CPU math -- derive 6 Mode7 parameters from angle/scale
; 2. render_mode7:     Program blitter with those params, trigger HW render
; 3. blit_to_front:    Copy completed back buffer to VRAM (display buffer)
; 4. WAIT_VBLANK:      Synchronize to vertical blank (prevents tearing)
; 5. advance_animation: Increment accumulators for next frame's parameters
;
; WHY THIS ORDER: We do all rendering BEFORE waiting for vblank. This means
; rendering overlaps with the display of the previous frame. The vblank wait
; ensures we don't swap buffers mid-scanout. The animation advance happens
; AFTER vblank so the next frame's compute starts with fresh values.
; ============================================================================
main_loop:
    call compute_frame
    call render_mode7
    call blit_to_front
    ; --- Two-Phase Vertical Blank Synchronization ---
    ; WAIT_VBLANK (defined in ie80.inc) implements the standard two-phase wait:
    ;   Phase 1: Wait while already IN vblank (in case we're mid-vblank)
    ;   Phase 2: Wait until vblank BEGINS (catch the rising edge)
    ; This guarantees exactly one frame per loop iteration, regardless of
    ; how long rendering takes. Without the first phase, we might see the
    ; SAME vblank twice and run at 30fps instead of 60fps.
    WAIT_VBLANK
    call advance_animation
    jp main_loop

; ============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; ============================================================================
; Creates a 256x256 checkerboard pattern in off-screen memory at 0x500000.
; The checkerboard has 128x128 quadrants alternating white and black:
;
;   +--------+--------+
;   | WHITE  | BLACK  |   <- Top half (rows 0-127)
;   | 128x128| 128x128|
;   +--------+--------+
;   | BLACK  | WHITE  |   <- Bottom half (rows 128-255)
;   | 128x128| 128x128|
;   +--------+--------+
;
; Each quadrant is a separate BLIT FILL operation. We can't fill the entire
; texture in one pass because the two colors alternate per quadrant.
;
; WHY BLIT FILL INSTEAD OF CPU LOOPS: The Z80 would need to write
; 256*256*4 = 262,144 bytes through MMIO one byte at a time. Even at
; ~10 cycles per byte, that's ~2.6 million cycles (~0.65 seconds at 4MHz).
; Four BLIT FILL operations complete in hardware almost instantly.
;
; ADDRESS CALCULATIONS:
;   Texture stride = 1024 bytes/row (256 pixels * 4 bytes/pixel)
;   Top-left quadrant:     TEXTURE_BASE + 0         = 0x500000
;   Top-right quadrant:    TEXTURE_BASE + 128*4     = 0x500000 + 512
;   Bottom-left quadrant:  TEXTURE_BASE + 128*1024  = 0x500000 + 131072
;   Bottom-right quadrant: TEXTURE_BASE + 128*1024 + 128*4 = 0x500000 + 131584
; ============================================================================
generate_texture:
    ; --- Top-left 128x128: White (0xFFFFFFFF = opaque white RGBA) ---
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFFFFFFFF
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; --- Top-right 128x128: Black (0xFF000000 = opaque black RGBA) ---
    ; Offset by 512 bytes = 128 pixels * 4 bytes/pixel to start at column 128.
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE+512
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFF000000
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; --- Bottom-left 128x128: Black ---
    ; Offset by 131072 bytes = 128 rows * 1024 bytes/row to start at row 128.
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE+131072
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFF000000
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; --- Bottom-right 128x128: White ---
    ; Offset = 128*1024 + 128*4 = 131072 + 512 = 131584 (row 128, column 128).
    SET_BLT_OP BLT_OP_FILL
    SET_BLT_DST TEXTURE_BASE+131584
    SET_BLT_WIDTH 128
    SET_BLT_HEIGHT 128
    SET_BLT_COLOR 0xFFFFFFFF
    SET_DST_STRIDE TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ret

; ============================================================================
; COMPUTE FRAME - Derive Mode7 Parameters from Animation State
; ============================================================================
; This is the CPU-intensive part of the demo. We compute the 6 parameters
; that define the affine transformation for this frame:
;
;   U0, V0       : Texture origin (where does pixel (0,0) map in texture space?)
;   dU/col, dV/col: Texture step per column (horizontal gradient)
;   dU/row, dV/row: Texture step per row (vertical gradient)
;
; === THE AFFINE TRANSFORMATION MATRIX ===
;
; The Mode7 blitter implements the mapping:
;   U(x,y) = U0 + x * dU_col + y * dU_row
;   V(x,y) = V0 + x * dV_col + y * dV_row
;
; For a rotozoomer, the matrix is a rotation scaled by a zoom factor:
;
;   | dU_col  dU_row |   | CA  -SA |
;   | dV_col  dV_row | = | SA   CA |
;
; Where CA = cos(angle) * zoom, SA = sin(angle) * zoom.
;
; The origin (U0, V0) is chosen so the CENTER of the screen maps to the
; CENTER of the texture (128, 128 in a 256x256 texture, scaled to
; fixed-point as 8388608 = 128 << 16 = 0x800000).
;
;   U0 = centre_tex - CA * half_width + SA * half_height
;   V0 = centre_tex - SA * half_width - CA * half_height
;
; Where half_width = 320 (640/2) and half_height = 240 (480/2).
;
; === COMPUTATION PIPELINE ===
;
; 1. Read angle_idx and scale_idx from 16-bit accumulators (high byte = index)
; 2. Look up cos(angle), sin(angle) from sine_table
; 3. Look up zoom = recip_table[scale_idx]
; 4. CA = cos * zoom  (signed 16 x unsigned 16 -> signed 32)
; 5. SA = sin * zoom  (signed 16 x unsigned 16 -> signed 32)
; 6. U0 = 0x800000 - CA*320 + SA*240
; 7. V0 = 0x800000 - SA*320 - CA*240
;
; All intermediate values are 32-bit to preserve precision through the
; multiply-accumulate chain. The blitter reads 32-bit parameters.
; ============================================================================
compute_frame:
    ; --- Extract Table Indices from 16-bit Accumulators ---
    ; The accumulators are 16-bit values in 8.8 fixed-point format.
    ; The high byte IS the integer table index (0-255). We read it directly
    ; using little-endian byte addressing: (angle_accum+1) is the high byte.
    ;
    ; WHY THIS WORKS: In a 16-bit value stored little-endian at address N,
    ; byte at N is the low byte (fractional part) and byte at N+1 is the
    ; high byte (integer part). No shift or mask needed -- just read the
    ; right byte. This is a Z80-specific optimization that exploits the
    ; little-endian byte ordering of the Z80's memory layout.
    ld a,(angle_accum+1)
    ld (var_angle_idx),a

    ld a,(scale_accum+1)
    ld (var_scale_idx),a

    ; --- Look Up Cosine ---
    ; cos(angle) = sin(angle + 64) because our sine table uses 256 entries
    ; for a full circle, so 90 degrees = 256/4 = 64 entries.
    ; The ADD wraps naturally in 8 bits (A register), so angle 200 + 64 = 8
    ; (wraps past 255), which is correct modular arithmetic.
    ld a,(var_angle_idx)
    add a,64                ; cos = sin(angle + 90deg), wraps naturally in 8 bits
    call lookup_sine        ; HL = cos_val (signed 16-bit, range -256..+256)
    ld (var_cos),hl

    ; --- Look Up Sine ---
    ld a,(var_angle_idx)
    call lookup_sine        ; HL = sin_val (signed 16-bit, range -256..+256)
    ld (var_sin),hl

    ; --- Look Up Reciprocal (Zoom Factor) ---
    ; WHY A RECIPROCAL TABLE: The zoom effect oscillates smoothly between
    ; zooming in and zooming out. The formula is:
    ;   recip[i] = round(256 / (0.5 + sin(i * 2*pi/256) * 0.3))
    ;
    ; This produces values from ~320 (zoomed in) to ~1280 (zoomed out).
    ; The sine oscillation creates smooth zoom pulsation.
    ;
    ; WHY NOT JUST DIVIDE AT RUNTIME: The Z80 has NO divide instruction
    ; at all. Implementing 16-bit division in software would take hundreds
    ; of cycles. A 512-byte lookup table gives instant results.
    ld a,(var_scale_idx)
    call lookup_recip       ; HL = recip (unsigned 16-bit)
    ld (var_recip),hl

    ; --- Compute CA = cos_val * recip ---
    ; This is a signed 16-bit by unsigned 16-bit multiply, producing a
    ; signed 32-bit result. We need the full 32 bits because:
    ;   cos_val range: -256..+256
    ;   recip range:   320..1280
    ;   Product range: -327,680..+327,680 (exceeds 16-bit!)
    ; The result is in fixed-point: it represents the actual cosine*zoom
    ; value scaled by 256 (from the sine table) * 256 (inherent in recip).
    ld hl,(var_cos)
    ld de,(var_recip)
    call mul16_signed       ; DEHL = cos * recip (signed 32-bit)
    ld (var_ca),hl
    ld (var_ca+2),de

    ; --- Compute SA = sin_val * recip ---
    ld hl,(var_sin)
    ld de,(var_recip)
    call mul16_signed
    ld (var_sa),hl
    ld (var_sa+2),de

    ; --- Compute U0 = 0x800000 - CA*320 + SA*240 ---
    ; 0x800000 = 8388608 = 128 << 16 = texture centre in 16.16 fixed-point.
    ; CA*320 shifts the origin by half the screen width (scaled).
    ; SA*240 compensates for the rotation's vertical component.
    ;
    ; WHY 320 AND 240: The screen is 640x480, so half-width=320, half-height=240.
    ; The affine transform maps pixel (x,y) to texture coordinate:
    ;   U = U0 + x*CA + y*(-SA)
    ; At the screen centre (320, 240):
    ;   U = U0 + 320*CA - 240*SA
    ; We want this to equal 0x800000 (texture centre), so:
    ;   U0 = 0x800000 - 320*CA + 240*SA
    call compute_ca_320     ; 32-bit result -> var_tmp1
    call compute_sa_240     ; 32-bit result -> var_tmp2

    ; U0 = 0x800000 - var_tmp1 + var_tmp2
    ; Step 1: 0x800000 - CA*320
    ; 0x800000 as two 16-bit words: low = 0x0000, high = 0x0080
    ld hl,0x0000           ; low word of 0x800000
    ld de,(var_tmp1)
    or a                   ; clear carry flag (SBC always subtracts carry too)
    sbc hl,de
    ld (var_u0),hl
    ld hl,0x0080           ; high word of 0x800000
    ld de,(var_tmp1+2)
    sbc hl,de              ; propagate borrow from low-word subtraction
    ld (var_u0+2),hl
    ; Step 2: + SA*240
    ld hl,(var_u0)
    ld de,(var_tmp2)
    add hl,de
    ld (var_u0),hl
    ld hl,(var_u0+2)
    ld de,(var_tmp2+2)
    adc hl,de              ; propagate carry from low-word addition
    ld (var_u0+2),hl

    ; --- Compute V0 = 0x800000 - SA*320 - CA*240 ---
    ; Similar derivation: at screen centre (320, 240):
    ;   V = V0 + 320*SA + 240*CA
    ; We want V at centre = 0x800000, so:
    ;   V0 = 0x800000 - 320*SA - 240*CA
    call compute_sa_320     ; 32-bit result -> var_tmp1
    call compute_ca_240     ; 32-bit result -> var_tmp2

    ; V0 = 0x800000 - var_tmp1 - var_tmp2
    ld hl,0x0000
    ld de,(var_tmp1)
    or a
    sbc hl,de
    ld (var_v0),hl
    ld hl,0x0080
    ld de,(var_tmp1+2)
    sbc hl,de
    ld (var_v0+2),hl
    ; - CA*240
    ld hl,(var_v0)
    ld de,(var_tmp2)
    or a
    sbc hl,de
    ld (var_v0),hl
    ld hl,(var_v0+2)
    ld de,(var_tmp2+2)
    sbc hl,de
    ld (var_v0+2),hl

    ret

; ============================================================================
; MULTIPLICATION HELPERS: val * 320 and val * 240
; ============================================================================
; These helpers compute 32-bit products of CA or SA with screen half-dimensions.
;
; WHY DECOMPOSE INTO SHIFTS AND ADDS:
; The Z80 has no multiply instruction at all (unlike the 8086's MUL or
; the M68K's MULU). Software multiply via shift-and-add would work but is
; slow for arbitrary values. Instead, we decompose the constant multipliers
; into sums of powers of two, which only need shifts:
;
;   320 = 256 + 64  => val*320 = val*256 + val*64 = (val<<8) + (val<<6)
;   240 = 256 - 16  => val*240 = val*256 - val*16 = (val<<8) - (val<<4)
;
; This reduces each multiplication to 2 shifts and 1 add/subtract, which
; is much faster than a generic 32x16-bit multiply loop.
;
; HOW val*256 WORKS (byte shift):
; Shifting left by 8 bits is the same as moving each byte up one position.
; In a 32-bit value [byte3, byte2, byte1, byte0]:
;   byte0 of result = 0
;   byte1 of result = byte0 of source
;   byte2 of result = byte1 of source
;   byte3 of result = byte2 of source
; We lose byte3 of the source, but our values (CA, SA) fit in ~20 bits,
; so byte3 is just sign extension and the shift stays within 32 bits.
; ============================================================================

; --- Helper: CA*320 -> var_tmp1 ---
compute_ca_320:
    ; CA*256 = byte shift left by 8 (copy bytes up one position)
    ld a,(var_ca)
    ld (var_tmp3+1),a
    ld a,(var_ca+1)
    ld (var_tmp3+2),a
    ld a,(var_ca+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a
    ; Sign extend byte 3 from bit 7 of var_ca+2
    ld a,(var_ca+3)
    ; Actually CA is 32-bit, CA*256 could overflow. But CA<<8:
    ; byte 0 = 0, byte 1 = CA[0], byte 2 = CA[1], byte 3 = CA[2]
    ; We lose CA[3], but CA fits in ~20 bits so this is fine.

    ; CA*64 = shift left by 6 bits (using shift_left_6 on DEHL)
    ld hl,(var_ca)
    ld de,(var_ca+2)
    call shift_left_6

    ; CA*320 = CA*256 + CA*64
    ld bc,(var_tmp3)
    add hl,bc
    ld (var_tmp1),hl
    ex de,hl
    ld bc,(var_tmp3+2)
    adc hl,bc              ; propagate carry from low-word addition
    ld (var_tmp1+2),hl
    ret

; --- Helper: SA*320 -> var_tmp1 ---
; Same decomposition as CA*320 but operating on var_sa instead.
compute_sa_320:
    ld a,(var_sa)
    ld (var_tmp3+1),a
    ld a,(var_sa+1)
    ld (var_tmp3+2),a
    ld a,(var_sa+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a

    ld hl,(var_sa)
    ld de,(var_sa+2)
    call shift_left_6

    ld bc,(var_tmp3)
    add hl,bc
    ld (var_tmp1),hl
    ex de,hl
    ld bc,(var_tmp3+2)
    adc hl,bc
    ld (var_tmp1+2),hl
    ret

; --- Helper: SA*240 -> var_tmp2 ---
; 240 = 256 - 16, so SA*240 = SA*256 - SA*16 = (SA<<8) - (SA<<4)
compute_sa_240:
    ; SA*256 (byte shift)
    ld a,(var_sa)
    ld (var_tmp3+1),a
    ld a,(var_sa+1)
    ld (var_tmp3+2),a
    ld a,(var_sa+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a

    ; SA*16 (shift left 4)
    ld hl,(var_sa)
    ld de,(var_sa+2)
    call shift_left_4

    ; SA*240 = SA*256 - SA*16
    ; At this point: DEHL = SA<<4, var_tmp3 = SA<<8
    ; We want var_tmp3 - DEHL, so save DEHL on stack and subtract
    push de
    push hl
    ld hl,(var_tmp3)
    pop de
    or a
    sbc hl,de
    ld (var_tmp2),hl
    ld hl,(var_tmp3+2)
    pop de
    sbc hl,de              ; propagate borrow
    ld (var_tmp2+2),hl
    ret

; --- Helper: CA*240 -> var_tmp2 ---
; Same decomposition as SA*240 but operating on var_ca.
compute_ca_240:
    ; CA*256
    ld a,(var_ca)
    ld (var_tmp3+1),a
    ld a,(var_ca+1)
    ld (var_tmp3+2),a
    ld a,(var_ca+2)
    ld (var_tmp3+3),a
    xor a
    ld (var_tmp3),a

    ; CA*16
    ld hl,(var_ca)
    ld de,(var_ca+2)
    call shift_left_4

    ; CA*240 = CA*256 - CA*16
    push de
    push hl
    ld hl,(var_tmp3)
    pop de
    or a
    sbc hl,de
    ld (var_tmp2),hl
    ld hl,(var_tmp3+2)
    pop de
    sbc hl,de
    ld (var_tmp2+2),hl
    ret

; ============================================================================
; LOOKUP SINE: A = table index -> HL = signed 16-bit value
; ============================================================================
; Reads a 16-bit signed value from the sine table at the given index.
;
; WHY PROPER SINE TABLES:
; Each entry is round(sin(i * 2*pi/256) * 256), giving true circular rotation.
; 256 entries cover a full circle with ~1.4 degree resolution (360/256).
; Values range from -256 to +256, representing -1.0 to +1.0 in 8.8 format.
;
; The table is stored as 16-bit little-endian words, so we multiply the
; index by 2 (ADD HL,HL) for byte-level addressing.
;
; Input:  A = table index (0-255)
; Output: HL = signed 16-bit sine value (-256..+256)
; Clobbers: DE
; ============================================================================
lookup_sine:
    ld l,a
    ld h,0
    add hl,hl              ; *2: each entry is 2 bytes (16-bit word)
    ld de,sine_table
    add hl,de              ; HL = address of sine_table[index]
    ld e,(hl)              ; load low byte
    inc hl
    ld d,(hl)              ; load high byte
    ex de,hl               ; HL = value (DE was just a temp)
    ret

; ============================================================================
; LOOKUP RECIP: A = table index -> HL = unsigned 16-bit value
; ============================================================================
; Reads a 16-bit unsigned value from the reciprocal/zoom table.
;
; WHY A RECIPROCAL TABLE:
; The table stores pre-computed values of:
;   recip[i] = round(256 / (0.5 + sin(i * 2*pi/256) * 0.3))
;
; This creates a smoothly oscillating zoom factor. At index 0 (sin=0),
; the value is 256/0.5 = 512. At index 64 (sin=1), it is 256/0.8 = 320
; (zoomed in). At index 192 (sin=-1), it is 256/0.2 = 1280 (zoomed out).
;
; Pre-computing avoids runtime division, which the Z80 simply cannot do
; (it has no DIV instruction at all, not even 8-bit division).
;
; Input:  A = table index (0-255)
; Output: HL = unsigned 16-bit reciprocal value
; Clobbers: DE
; ============================================================================
lookup_recip:
    ld l,a
    ld h,0
    add hl,hl              ; *2 for word access
    ld de,recip_table
    add hl,de
    ld e,(hl)
    inc hl
    ld d,(hl)
    ex de,hl
    ret

; ============================================================================
; MUL16_SIGNED: DEHL = HL * DE (signed 16-bit -> signed 32-bit)
; ============================================================================
; WHY SOFTWARE MULTIPLY: The Z80 has NO multiply instruction of any kind.
; Not 8-bit, not 16-bit. Every multiplication must be done in software.
; This is one of the most significant limitations of the Z80 compared to
; later CPUs (the 8086 has MUL, the M68K has MULU/MULS).
;
; ALGORITHM: Sign-magnitude approach.
; 1. Check if HL is negative (test bit 7 of H)
; 2. If negative: negate HL to make it positive, multiply unsigned,
;    then negate the 32-bit result
; 3. If positive: multiply unsigned directly
;
; We only check HL's sign because in our usage, DE (the reciprocal/zoom
; value) is always positive. If both operands could be negative, we'd
; need to XOR the signs and conditionally negate the result.
;
; Input:  HL = signed 16-bit multiplicand (sine/cosine value)
;         DE = unsigned 16-bit multiplier (reciprocal/zoom value)
; Output: DEHL = signed 32-bit product
; ============================================================================
mul16_signed:
    ; Check sign of HL (the sine/cosine value)
    bit 7,h
    jr z,.hl_pos
    ; HL is negative: negate to positive, multiply, negate result
    push de
    ; Two's complement negation: complement then add 1
    xor a
    sub l
    ld l,a
    ld a,0                 ; can't use XOR A here (would clear carry flag)
    sbc a,h
    ld h,a                 ; HL = |HL| (absolute value)
    pop de
    call mul16u
    ; Negate the 32-bit result DEHL using two's complement:
    ; Complement all 4 bytes, then add 1 with carry propagation
    ld a,l
    cpl
    ld l,a
    ld a,h
    cpl
    ld h,a
    ld a,e
    cpl
    ld e,a
    ld a,d
    cpl
    ld d,a
    ; Add 1 (two's complement = complement + 1)
    inc hl
    ; If HL wrapped from 0xFFFF to 0x0000, carry into DE
    ld a,h
    or l
    ret nz                 ; no wrap, done
    inc de                 ; propagate carry to high word
    ret
.hl_pos:
    jp mul16u              ; positive: just do unsigned multiply

; ============================================================================
; MUL16U: DEHL = HL * DE (unsigned 16x16 -> unsigned 32-bit)
; ============================================================================
; Shift-and-add multiplication, the fundamental algorithm for CPUs without
; hardware multiply.
;
; ALGORITHM:
; Think of it like long multiplication in decimal, but in binary.
; For each bit of the multiplicand (BC), if the bit is 1, add the
; multiplier (DE) to the accumulator at the corresponding bit position.
;
; Implementation:
; - BC holds the multiplicand, shifted left each iteration (MSB first)
; - DE holds the multiplier (constant, added when BC's top bit is set)
; - IX:HL is the 32-bit accumulator (IX = high word, HL = low word)
;
; WHY IX FOR THE HIGH WORD:
; The Z80 has only three general 16-bit register pairs: HL, DE, BC.
; HL is the accumulator low word (needed for ADD HL,DE).
; DE is the multiplier (added on set bits).
; BC is the multiplicand (shifted to extract bits).
; The ONLY register left for the high word is IX.
;
; IX has limited operations: you can INC IX but NOT do ADC IX,DE.
; So carry propagation from HL to IX requires push/pop to move IX
; through HL temporarily for the ADC instruction. This is ugly but
; unavoidable on the Z80's asymmetric register architecture.
;
; LOOP COUNT: 16 iterations for 16-bit multiplicand (one per bit).
;
; Input:  HL = unsigned 16-bit multiplicand
;         DE = unsigned 16-bit multiplier
; Output: DEHL = unsigned 32-bit product (DE=high, HL=low)
; Preserves: BC, IX (saved/restored)
; ============================================================================
mul16u:
    push bc
    ld b,h
    ld c,l                 ; BC = multiplicand (will shift out MSB-first)
    ; DE = multiplier (added to accumulator when BC bit is set)
    push ix
    ld ix,0                ; IX = high word of 32-bit accumulator
    ld hl,0                ; HL = low word of 32-bit accumulator

    ld a,16                ; 16 iterations (one per bit of multiplicand)
.mul_loop:
    ; --- Shift 32-bit accumulator left by 1 ---
    ; First shift HL left (low word)
    add hl,hl              ; HL <<= 1, carry out goes to C flag
    ; Now shift IX left, bringing in carry from HL
    ; Z80 has no "ADC IX,IX" so we move IX through HL temporarily
    push hl                ; save low word
    push ix
    pop hl                 ; HL = IX (high word)
    adc hl,hl              ; HL <<= 1 with carry from low word shift
    push hl
    pop ix                 ; IX = shifted high word
    pop hl                 ; HL = shifted low word (restore)

    ; --- Test top bit of multiplicand ---
    ; Shift BC left; if the bit shifted out was 1, add multiplier
    sla c
    rl b                   ; BC <<= 1, old MSB now in carry flag
    jr nc,.no_add          ; bit was 0: skip addition

    ; --- Add multiplier (DE) to accumulator low word ---
    add hl,de
    jr nc,.no_add          ; no carry: skip high word increment
    ; Carry from low word addition: propagate to high word
    inc ix
.no_add:
    dec a
    jr nz,.mul_loop

    ; --- Move result to DEHL ---
    push ix
    pop de                 ; DE = high word of result

    pop ix                 ; restore caller's IX
    pop bc                 ; restore caller's BC
    ret

; ============================================================================
; MUL_A_L: HL = A * L (8x8 -> 16-bit unsigned multiply)
; ============================================================================
; A simpler multiply for 8-bit operands. Same shift-and-add algorithm
; but only 8 iterations since the multiplicand is 8 bits.
;
; Not used in the main rotozoomer math but included as a utility.
;
; Input:  A = unsigned 8-bit multiplicand
;         L = unsigned 8-bit multiplier
; Output: HL = unsigned 16-bit product
; Clobbers: DE, B
; ============================================================================
mul_a_l:
    ld h,0
    ld d,0
    ld e,l                 ; DE = L (multiplier)
    ld l,h                 ; HL = 0 (accumulator)
    ld b,8                 ; 8 iterations for 8-bit multiplicand
.loop:
    add hl,hl              ; shift accumulator left
    rla                    ; shift A left, old MSB goes to carry
    jr nc,.no_add2         ; if carry=0, this bit was 0
    add hl,de              ; bit was 1: add multiplier
.no_add2:
    djnz .loop             ; decrement B, loop if not zero
    ret

; ============================================================================
; 32-BIT SHIFT LEFT HELPERS: DEHL <<= N
; ============================================================================
; These routines shift a 32-bit value held across two register pairs
; (DE = high word, HL = low word) left by a fixed number of bit positions.
;
; WHY UNROLLED LOOPS: The Z80 has no barrel shifter (unlike ARM or x86's
; SHL with a count operand). Each bit position requires a separate shift
; instruction. Unrolling the loop avoids the overhead of a counter and
; branch, which matters when these routines are called multiple times
; per frame.
;
; HOW EACH SHIFT STEP WORKS:
;   ADD HL,HL:  Left-shift HL by 1, MSB goes to carry flag
;   RL E:       Rotate E left through carry (carry in from HL's MSB)
;   RL D:       Rotate D left through carry (carry in from E's MSB)
;
; This chains the carry bit through all 32 bits, implementing a proper
; 32-bit left shift across two 16-bit register pairs.
; ============================================================================

; --- Shift DEHL left by 4 bits ---
shift_left_4:
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    ret

; --- Shift DEHL left by 6 bits ---
; Reuses shift_left_4 for the first 4 bits, then does 2 more manually.
shift_left_6:
    call shift_left_4
    add hl,hl
    rl e
    rl d
    add hl,hl
    rl e
    rl d
    ret

; ============================================================================
; WRITE 32-BIT VALUE TO MMIO: copy 4 bytes from (HL) to (DE)
; ============================================================================
; WHY BYTE-BY-BYTE MMIO WRITES:
; Z80's address bus is only 16 bits (0x0000-0xFFFF), but IntuitionEngine's
; MMIO registers are at 20-bit physical addresses (0xF0000+). The bus
; adapter maps Z80 addresses 0xF000+ to physical 0xF0000+.
;
; For 32-bit MMIO registers (like BLT_MODE7_U0), we must write 4 bytes
; sequentially. Each individual byte write triggers the bus adapter
; independently, correctly routing the write to the physical MMIO address.
;
; There is no 16-bit or 32-bit write instruction on the Z80 that would
; work with MMIO (LD (nn),HL writes to RAM, not through the I/O adapter).
; So byte-by-byte is the only correct approach.
;
; Input:  HL = pointer to 4-byte source value in RAM
;         DE = MMIO register base address (byte 0 of the 32-bit register)
; Clobbers: A, HL, DE (both advanced by 4)
; ============================================================================
write_mode7_32:
    ld a,(hl)
    ld (de),a
    inc hl
    inc de
    ld a,(hl)
    ld (de),a
    inc hl
    inc de
    ld a,(hl)
    ld (de),a
    inc hl
    inc de
    ld a,(hl)
    ld (de),a
    ret

; ============================================================================
; NEG32: Negate a 32-bit value at (HL) in-place
; ============================================================================
; Two's complement negation of a 32-bit value stored in memory.
;
; WHY THIS IS COMPLEX ON Z80:
; The Z80's NEG instruction only works on the A register (8-bit). There is
; no 16-bit or 32-bit negate. We must complement all 4 bytes manually using
; CPL (complement accumulator = bitwise NOT) and then add 1 with carry
; propagation across all 4 bytes.
;
; Algorithm: result = ~value + 1  (two's complement definition)
;   1. Load all 4 bytes into registers
;   2. Complement each byte (CPL = bitwise NOT)
;   3. Add 1 to the low word (BC), propagate carry to high word (HL)
;   4. Store all 4 bytes back to memory
;
; Input:  HL = pointer to 32-bit value in memory (4 bytes, little-endian)
; Output: The 4 bytes at (HL) are replaced with their two's complement
; Preserves: BC (saved/restored)
; ============================================================================
neg32:
    push bc
    ; Load 4 bytes into registers: BC = low word, HL = high word
    ld c,(hl)
    inc hl
    ld b,(hl)
    inc hl
    push hl                ; save pointer to byte 2 (we need it for store-back)
    ld a,(hl)
    inc hl
    ld h,(hl)
    ld l,a                 ; HL = high word (byte3:byte2), BC = low word (byte1:byte0)

    ; Complement all 4 bytes (bitwise NOT)
    ld a,c
    cpl
    ld c,a
    ld a,b
    cpl
    ld b,a
    ld a,l
    cpl
    ld l,a
    ld a,h
    cpl
    ld h,a

    ; Add 1 with carry propagation (completing two's complement)
    inc bc                 ; BC = ~low_word + 1
    ld a,b
    or c
    jr nz,.no_carry        ; if BC didn't wrap to 0, no carry to high word
    inc hl                 ; BC wrapped 0xFFFF -> 0x0000: carry into high word
.no_carry:
    ; Store the negated value back to memory
    pop de                 ; DE = pointer to byte 2 (saved earlier)
    push de
    ; Navigate back to byte 0
    dec de
    dec de                 ; DE = original pointer (byte 0)
    ex de,hl               ; HL = pointer, DE = high word value
    ld (hl),c
    inc hl
    ld (hl),b
    inc hl
    ld (hl),e
    inc hl
    ld (hl),d
    pop de                 ; clean up the push from earlier
    pop bc
    ret

; ============================================================================
; RENDER MODE7 - Program the Blitter and Execute Affine Transform
; ============================================================================
; This routine programs all Mode7 blitter registers with the parameters
; computed by compute_frame, then triggers the hardware to render the
; entire 640x480 frame.
;
; === MODE7 BLITTER PARAMETERS ===
;
; The Mode7 blitter implements the affine texture mapping:
;
;   For each output pixel at screen position (x, y):
;     U = U0 + x * dU_col + y * dU_row
;     V = V0 + x * dV_col + y * dV_row
;     output[x,y] = texture[(U >> 16) & tex_w, (V >> 16) & tex_h]
;
; The texture coordinates wrap using bitwise AND with tex_w/tex_h,
; creating the infinite-tiling effect characteristic of rotozoomers.
;
; === MODE7 MATRIX AND ITS MEANING ===
;
; For a rotation by angle A with zoom factor Z:
;
;   CA = cos(A) * Z
;   SA = sin(A) * Z
;
;   | dU_col  dU_row |   |  CA  -SA |
;   | dV_col  dV_row | = |  SA   CA |
;
; dU_col (= CA):  Moving one pixel RIGHT on screen advances U by CA
; dV_col (= SA):  Moving one pixel RIGHT on screen advances V by SA
; dU_row (= -SA): Moving one scanline DOWN on screen advances U by -SA
; dV_row (= CA):  Moving one scanline DOWN on screen advances V by CA
;
; This is a standard 2D rotation matrix scaled by the zoom factor.
; The blitter applies it per-pixel in hardware, producing the rotated
; and zoomed texture output.
;
; === WHY DOUBLE BUFFERING ===
; The blitter writes to BACK_BUFFER (0x600000), not directly to VRAM
; (0x100000). After rendering completes, blit_to_front copies the
; result to VRAM. This prevents the display from showing a partially-
; rendered frame (tearing). The cost is an extra copy, but the blitter
; handles it in hardware so it is essentially free.
; ============================================================================
render_mode7:
    ; Configure blitter for Mode7 affine transform operation
    SET_BLT_OP BLT_OP_MODE7
    SET_BLT_SRC TEXTURE_BASE      ; Source: 256x256 checkerboard texture
    SET_BLT_DST BACK_BUFFER       ; Destination: off-screen back buffer
    SET_BLT_WIDTH RENDER_W         ; Output width: 640 pixels
    SET_BLT_HEIGHT RENDER_H        ; Output height: 480 pixels
    SET_SRC_STRIDE TEX_STRIDE      ; Source stride: 1024 bytes (256 px * 4 bpp)
    SET_DST_STRIDE LINE_BYTES      ; Dest stride: 2560 bytes (640 px * 4 bpp)

    ; Set texture wrap dimensions (0-indexed: 255 means wrap at 256)
    ; The blitter uses (U & tex_w, V & tex_h) for wrapping, so tex_w=255
    ; gives modulo-256 wrap. This creates the infinite tiling effect.
    STORE32 BLT_MODE7_TEX_W_0, 255
    STORE32 BLT_MODE7_TEX_H_0, 255

    ; --- Write the 6 Mode7 affine parameters ---
    ; U0: texture U coordinate at screen origin (0,0)
    ld hl,var_u0
    ld de,BLT_MODE7_U0_0
    call write_mode7_32

    ; V0: texture V coordinate at screen origin (0,0)
    ld hl,var_v0
    ld de,BLT_MODE7_V0_0
    call write_mode7_32

    ; dU_col = CA: U increment per column (per pixel horizontally)
    ld hl,var_ca
    ld de,BLT_MODE7_DU_COL_0
    call write_mode7_32         ; du_col = CA

    ; dV_col = SA: V increment per column (per pixel horizontally)
    ld hl,var_sa
    ld de,BLT_MODE7_DV_COL_0
    call write_mode7_32         ; dv_col = SA

    ; dU_row = -SA: U increment per row (per scanline vertically)
    ; WHY var_neg_sa SCRATCH SPACE: To write -SA to the blitter register,
    ; we need to negate a 32-bit value. The neg32 routine works IN-PLACE
    ; on memory (it reads, complements, and writes back to the same address).
    ; If we negated var_sa directly, we would corrupt it -- but var_sa is
    ; still needed for dV_col (already written above) and potentially for
    ; future frames. So we copy SA to a scratch variable (var_neg_sa),
    ; negate the copy, and write the negated copy to the blitter register.
    ld hl,(var_sa)
    ld (var_neg_sa),hl
    ld hl,(var_sa+2)
    ld (var_neg_sa+2),hl
    ld hl,var_neg_sa
    call neg32
    ld hl,var_neg_sa
    ld de,BLT_MODE7_DU_ROW_0
    call write_mode7_32

    ; dV_row = CA: V increment per row (per scanline vertically)
    ld hl,var_ca
    ld de,BLT_MODE7_DV_ROW_0
    call write_mode7_32         ; dv_row = CA

    ; --- Trigger the blitter and wait for completion ---
    ; START_BLIT writes 1 to BLT_CTRL, which starts the hardware operation.
    ; WAIT_BLIT polls BLT_CTRL until the busy bit clears.
    ; The blitter processes all 640*480 = 307,200 pixels in hardware.
    START_BLIT
    WAIT_BLIT

    ret

; ============================================================================
; BLIT BACK BUFFER TO FRONT (Double Buffer Swap)
; ============================================================================
; Copies the completed Mode7 rendering from the back buffer (0x600000)
; to VRAM (0x100000) using a hardware block copy.
;
; This is the "swap" phase of double buffering. The blitter copies
; 640*480*4 = 1,228,800 bytes from back buffer to front buffer.
; During this copy, the display shows the previous frame's data (or the
; copy completes fast enough to be invisible).
; ============================================================================
blit_to_front:
    SET_BLT_OP BLT_OP_COPY
    SET_BLT_SRC BACK_BUFFER
    SET_BLT_DST VRAM_START
    SET_BLT_WIDTH RENDER_W
    SET_BLT_HEIGHT RENDER_H
    SET_SRC_STRIDE LINE_BYTES
    SET_DST_STRIDE LINE_BYTES
    START_BLIT
    WAIT_BLIT
    ret

; ============================================================================
; ADVANCE ANIMATION - Increment Accumulator State for Next Frame
; ============================================================================
; Adds the fixed increments to the 16-bit angle and scale accumulators.
;
; WHY ACCUMULATORS DON'T NEED MASKING:
; The accumulators are 16-bit values stored in 16-bit memory locations.
; Adding ANGLE_INC (313) or SCALE_INC (104) to a 16-bit value wraps
; naturally at 65536 due to Z80's 16-bit arithmetic overflow. No AND
; mask is needed -- the hardware wraps for free. When we later read the
; high byte as the table index, it is inherently in the range 0-255.
;
; This is a common 8/16-bit microprocessor trick: use the natural word
; size overflow as a free modulo operation.
; ============================================================================
advance_animation:
    ld hl,(angle_accum)
    ld de,ANGLE_INC
    add hl,de              ; wraps naturally at 65536 (16-bit overflow)
    ld (angle_accum),hl

    ld hl,(scale_accum)
    ld de,SCALE_INC
    add hl,de              ; wraps naturally at 65536
    ld (scale_accum),hl

    ret

; ============================================================================
; VARIABLES (RAM Working Storage)
; ============================================================================
; All mutable state used by compute_frame and render_mode7.
; These variables are in the code segment because the Z80's address space
; is flat -- there is no separate data segment. They are initialized to
; zero and overwritten every frame.
;
; MEMORY LAYOUT (byte offsets from angle_accum):
;
;   Offset  Size  Name           Purpose
;   ------  ----  -------------- ----------------------------------------
;   +0      2     angle_accum    16-bit rotation accumulator (8.8 fixed)
;   +2      2     scale_accum    16-bit zoom accumulator (8.8 fixed)
;   +4      1     var_angle_idx  High byte of angle_accum (table index)
;   +5      1     var_scale_idx  High byte of scale_accum (table index)
;   +6      2     var_cos        cos(angle) from sine table (signed 16)
;   +8      2     var_sin        sin(angle) from sine table (signed 16)
;   +10     2     var_recip      Zoom factor from reciprocal table (unsigned 16)
;   +12     4     var_ca         cos * zoom (signed 32-bit)
;   +16     4     var_sa         sin * zoom (signed 32-bit)
;   +20     4     var_neg_sa     Scratch: negated SA for dU_row parameter
;   +24     4     var_u0         Texture U origin (signed 32-bit)
;   +28     4     var_v0         Texture V origin (signed 32-bit)
;   +32     4     var_tmp1       Scratch: intermediate multiply result
;   +36     4     var_tmp2       Scratch: intermediate multiply result
;   +40     4     var_tmp3       Scratch: intermediate multiply result
; ============================================================================
angle_accum:    .word 0
scale_accum:    .word 0
var_angle_idx:  .byte 0
var_scale_idx:  .byte 0
var_cos:        .word 0
var_sin:        .word 0
var_recip:      .word 0
var_ca:         .byte 0,0,0,0
var_sa:         .byte 0,0,0,0
var_neg_sa:     .byte 0,0,0,0
var_u0:         .byte 0,0,0,0
var_v0:         .byte 0,0,0,0
var_tmp1:       .byte 0,0,0,0
var_tmp2:       .byte 0,0,0,0
var_tmp3:       .byte 0,0,0,0

; ============================================================================
; SINE TABLE - 256 entries, signed 16-bit little-endian
; ============================================================================
; Pre-computed sine values for angles 0 through 255 (representing 0 to 360
; degrees). Each entry is round(sin(i * 2*pi/256) * 256).
;
; WHY PROPER SINE TABLES (not approximations):
; Using the exact formula round(sin(i * 2*pi/256) * 256) gives true
; circular rotation. Cheaper approximations (like triangle waves or
; piecewise linear) would produce visible distortion: the rotation would
; speed up and slow down non-uniformly, and the zoom would wobble instead
; of pulsing smoothly.
;
; 256 entries gives ~1.4 degree resolution (360/256), which is sufficient
; for smooth animation at 60fps (the eye can't distinguish sub-degree
; rotation steps at this frame rate).
;
; VALUES:
;   Index 0:   sin(0)     = 0      (zero crossing, ascending)
;   Index 64:  sin(pi/2)  = 256    (positive peak = +1.0 in 8.8)
;   Index 128: sin(pi)    = 0      (zero crossing, descending)
;   Index 192: sin(3pi/2) = -256   (negative peak = -1.0 in 8.8)
;
; USAGE WITH COSINE:
;   cos(angle) = sin(angle + 64)  because cos(x) = sin(x + pi/2)
;   and pi/2 = 256/4 = 64 entries.
;
; TABLE SIZE: 256 entries * 2 bytes = 512 bytes.
; On a Z80 with ~61KB usable RAM, 512 bytes is a trivial cost for
; eliminating all runtime trigonometry.
; ============================================================================
sine_table:
    ; Quadrant 1: indices 0-63, angles 0 to ~90 degrees, values 0 to 256
    .word 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
    .word 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
    .word 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
    .word 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
    ; Quadrant 2: indices 64-127, angles ~90 to ~180 degrees, values 256 to 0
    .word 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
    .word 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
    .word 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
    .word 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
    ; Quadrant 3: indices 128-191, angles ~180 to ~270 degrees, values 0 to -256
    .word 0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
    .word -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
    .word -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
    .word -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256
    ; Quadrant 4: indices 192-255, angles ~270 to ~360 degrees, values -256 to 0
    .word -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
    .word -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
    .word -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
    .word -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

; ============================================================================
; RECIPROCAL TABLE - 256 entries, unsigned 16-bit little-endian
; ============================================================================
; Pre-computed zoom oscillation values. Each entry is:
;   recip[i] = round(256 / (0.5 + sin(i * 2*pi/256) * 0.3))
;
; WHY THIS FORMULA:
; The denominator (0.5 + sin(x)*0.3) oscillates between 0.2 and 0.8.
; Taking the reciprocal (256 / denom) gives a value that oscillates
; between 320 (zoomed in, when denom=0.8) and 1280 (zoomed out, when
; denom=0.2). The factor of 256 keeps values in a useful 16-bit range.
;
; The sine-based oscillation creates a smooth, organic zoom pulsation.
; Linear or triangle-wave zoom would look mechanical and unnatural.
;
; RANGE OF VALUES:
;   Minimum (index ~64):  recip = 320  (zoomed in, texture appears large)
;   Midpoint (index 0):   recip = 512  (neutral zoom)
;   Maximum (index ~192): recip = 1280 (zoomed out, texture appears small)
;
; These values get multiplied by sine/cosine (-256 to +256) to produce CA/SA.
; The resulting products range from about -327,680 to +327,680, fitting
; comfortably in 32 bits.
;
; TABLE SIZE: 256 entries * 2 bytes = 512 bytes.
; Combined with the sine table (512 bytes), the total lookup table cost
; is 1024 bytes -- a small price for eliminating both division and
; trigonometry at runtime.
; ============================================================================
recip_table:
    ; Entries 0-63: zoom from neutral (512) toward zoomed-in (320)
    .word 512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
    .word 416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
    .word 359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
    .word 329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
    ; Entries 64-127: zoom from zoomed-in (320) back toward neutral (512)
    .word 320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
    .word 329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
    .word 359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
    .word 416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
    ; Entries 128-191: zoom from neutral (512) toward zoomed-out (1280)
    .word 512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
    .word 665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
    .word 889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
    .word 1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
    ; Entries 192-255: zoom from zoomed-out (1280) back toward neutral (512)
    .word 1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
    .word 1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
    .word 889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
    .word 665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; ============================================================================
; SID MUSIC DATA - Embedded .sid File
; ============================================================================
; The .sid file contains actual 6502 machine code and music data.
; IntuitionEngine's real 6502 CPU core executes the player code, which
; writes to SID registers ($D400-$D418). Those writes are intercepted
; and remapped to the native synthesizer engine.
;
; The Z80 main loop is completely unaware of audio -- it runs in parallel
; on a separate CPU core. This is the power of the IntuitionEngine's
; multi-CPU architecture: each processor does what it is best at.
;
; "Circus Attractions" is a SID tune composed for the Commodore 64.
; The player code is highly optimized 6502 assembly, called ~50 times
; per second by the audio subsystem to update SID register state.
; ============================================================================
sid_data:
    .incbin "Circus_Attractions.sid"
sid_data_end:
