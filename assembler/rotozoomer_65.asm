; ============================================================================
; MODE 7 HARDWARE BLITTER ROTOZOOMER - 6502 ASSEMBLY
; Intuition Engine SDK Reference Implementation
; ============================================================================
;
; === WHAT THIS DEMO DOES ===
;
; A real-time rotozoomer effect: a 256x256 checkerboard texture is rotated
; and zoomed smoothly using the hardware Mode 7 affine texture mapper. The
; CPU computes just 6 parameters per frame (u0, v0, du_col, dv_col,
; du_row, dv_row), and the blitter handles all 307,200 pixels (640x480).
; PSG music plays in the background via the audio subsystem.
;
; === WHY MODE 7 HARDWARE BLITTER ===
;
; The 6502 CPU runs at 1-2 MHz with no multiply instruction, no barrel
; shifter, and only three 8-bit registers (A, X, Y). Software-rendering
; a 640x480 rotozoomer at 60fps is impossible -- each pixel would need
; two multiplies and two adds for the affine transform, totaling over
; 1.2 million multiplies per frame. Even at one multiply per 100 cycles,
; that would require 120 MHz -- roughly 100x what we have.
;
; The Mode 7 blitter solves this by accepting 6 parameters that define
; the affine transformation matrix and rendering the entire frame in
; hardware. The CPU's only job is computing those 6 values, which
; involves a handful of 16x16 multiplies and 32-bit additions.
;
; This is the same principle behind the SNES Mode 7 (F-Zero, Mario Kart)
; and the GBA affine backgrounds: the hardware does the per-pixel work,
; the CPU just sets up the transform.
;
; === ARCHITECTURE OVERVIEW ===
;
;   ┌─────────────────────────────────────────────────────────────┐
;   │                    MAIN LOOP (60 FPS)                       │
;   │                                                             │
;   │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
;   │  │  COMPUTE    │───>│   RENDER    │───>│  BLIT TO    │     │
;   │  │  FRAME      │    │   MODE 7   │    │   FRONT     │     │
;   │  │ (6 params)  │    │  (blitter)  │    │  (blitter)  │     │
;   │  └─────────────┘    └─────────────┘    └─────────────┘     │
;   │        │                                      │             │
;   │        │                               ┌──────┘             │
;   │        │                               │                    │
;   │  ┌─────────────┐    ┌─────────────┐    │                    │
;   │  │  ADVANCE    │<───│  WAIT FOR   │<───┘                    │
;   │  │  ANIMATION  │    │   VSYNC     │                         │
;   │  └─────────────┘    └─────────────┘                         │
;   └─────────────────────────────────────────────────────────────┘
;
; === MODE 7 AFFINE TRANSFORMATION ===
;
; The Mode 7 blitter samples a texture using these per-pixel equations:
;
;   u(x,y) = u0 + x * du_col + y * du_row
;   v(x,y) = v0 + x * dv_col + y * dv_row
;
; For a pure rotation by angle A with zoom factor Z, the matrix is:
;
;   ┌ du_col  du_row ┐   ┌  cos(A)*Z   -sin(A)*Z ┐
;   │                │ = │                         │
;   └ dv_col  dv_row ┘   └  sin(A)*Z    cos(A)*Z  ┘
;
; And (u0, v0) centres the rotation on the texture:
;
;   u0 = texture_centre - du_col * (screen_w/2) + du_row * (screen_h/2)
;   v0 = texture_centre - dv_col * (screen_w/2) - dv_row * (screen_h/2)
;
; The sign pattern in u0/v0 (subtract du_col*320, ADD du_row*240 for u0;
; subtract BOTH for v0) comes from the rotation matrix being:
;   du_row = -SA (negative sine), dv_row = CA (positive cosine).
;
; === MEMORY MAP ===
;
;   $000000 - $0000FF : Zero page (fast-access working variables)
;   $000200 - $00EFFF : Program code + data (CODE and RODATA segments)
;   $100000 - $14BFFF : VRAM front buffer (640 x 480 x 4 bytes = 1,228,800)
;   $500000 - $5FFFFF : Texture data (256 x 256 x 4, stride 1024)
;   $600000 - $6FFFFF : Back buffer (Mode 7 renders here first)
;
;   ┌──────────────────────────────────────────────────────┐
;   │ $000000  Zero Page: angle_accum, scale_accum,        │
;   │          var_ca, var_sa, var_u0, var_v0,              │
;   │          mul_a, mul_b, mul_result, tmp32_*, sign_flag │
;   ├──────────────────────────────────────────────────────┤
;   │ $000200  CODE: main, compute_frame, mul16_signed,    │
;   │          render_mode7, blit_to_front, etc.            │
;   ├──────────────────────────────────────────────────────┤
;   │ $100000  VRAM (front buffer) -- final display output  │
;   ├──────────────────────────────────────────────────────┤
;   │ $500000  Texture (256x256 checkerboard, stride 1024)  │
;   ├──────────────────────────────────────────────────────┤
;   │ $600000  Back buffer (Mode 7 renders here)            │
;   └──────────────────────────────────────────────────────┘
;
; === WHY DOUBLE BUFFERING ===
;
; The Mode 7 blitter writes 307,200 pixels to the destination buffer.
; If we rendered directly to VRAM ($100000), the display would show
; a partially-rendered frame (tearing). Instead, we render to the back
; buffer ($600000), then BLIT COPY the completed frame to VRAM in one
; shot between vsyncs.
;
; === BUILD AND RUN ===
;
;   Build: make ie65asm SRC=assembler/rotozoomer_65.asm
;   Run:   ./bin/IntuitionEngine -m6502 assembler/rotozoomer_65.ie65
;
; === DEPENDENCIES ===
;
;   - ie65.inc : Hardware register definitions and utility macros
;   - WaksonsZak018.ay : PSG music file (AY format, embedded via .incbin)
;   - cc65 toolchain (ca65 assembler, ld65 linker)
;
; ============================================================================

.include "ie65.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Rendering Geometry ---
; The IE VideoChip defaults to 640x480 in mode 0 (32-bit RGBA per pixel).
; LINE_BYTES = 640 * 4 = 2560 bytes per scanline.
RENDER_W         = 640
RENDER_H         = 480

; --- Texture Memory ---
; The texture lives at $500000, well above VRAM ($100000-$14BFFF) and the
; back buffer ($600000-$6FFFFF). This avoids any memory overlap.
;
; Why split into bytes: the 6502 is an 8-bit CPU with 16-bit addresses.
; The STORE32 macro needs individual bytes of the 32-bit address.
; $500000 = byte 0: $00, byte 1: $00, byte 2: $50, byte 3: $00.
TEXTURE_BASE_LO  = $00
TEXTURE_BASE_MI  = $00
TEXTURE_BASE_HI  = $50
TEXTURE_BASE     = $500000

; Why $600000 for the back buffer: it must not overlap the texture
; ($500000) or VRAM ($100000). With 640x480x4 = 1,228,800 bytes
; (~1.17 MB), $600000 has plenty of room in the 32-bit address space.
BACK_BUFFER      = $600000

; --- Texture Dimensions ---
; TEX_STRIDE = 1024 bytes = 256 pixels * 4 bytes/pixel (RGBA).
; The width/height MASKS are 255 (0xFF), meaning the blitter wraps
; texture coordinates modulo 256 by ANDing with the mask. This gives
; us seamless tiling with no branch or modulo instruction.
TEX_STRIDE       = 1024
TEX_W_MASK       = 255
TEX_H_MASK       = 255

; --- Animation Increments (8.8 Fixed-Point Accumulators) ---
;
; The angle and scale accumulators are 16-bit values in 8.8 fixed-point
; format: the high byte is the integer part (table index), the low byte
; is the fractional part (sub-index precision).
;
; ANGLE_INC = 313 ($0139):
;   High byte = 1 (advance ~1 table entry per frame)
;   Low byte = $39 = 57/256 ~ 0.22
;   So angle advances by ~1.22 entries per frame.
;   At 256 entries per revolution and 60fps: ~3.5 seconds per full rotation.
;
; SCALE_INC = 104 ($0068):
;   High byte = 0 (no integer advance per frame)
;   Low byte = $68 = 104/256 ~ 0.41
;   So scale advances by ~0.41 entries per frame.
;   This creates a slow zoom oscillation that takes ~10 seconds per cycle.
;
; These values were chosen to match the BASIC version's A+=0.03, SI+=0.01
; ratio (approximately 3:1) while using 8.8 fixed-point representation.
; The ratio ensures the rotation and zoom never synchronize, creating
; an endlessly varying visual pattern.
;
; The `<` and `>` operators are cc65 syntax for extracting the low and
; high bytes of a 16-bit value, respectively.
ANGLE_INC_LO     = <313          ; 313 = $0139, low byte = $39
ANGLE_INC_HI     = >313          ; high byte = $01
SCALE_INC_LO     = <104          ; 104 = $0068, low byte = $68
SCALE_INC_HI     = >104          ; high byte = $00

; ============================================================================
; ZERO PAGE VARIABLES
; ============================================================================
;
; WHY ZERO PAGE?
;
; The 6502's zero page ($00-$FF) provides faster access than any other
; memory region. Zero-page instructions are one byte shorter (2 bytes
; instead of 3 for absolute addressing) and one cycle faster. On a
; 1-2 MHz CPU where every cycle matters, this is significant.
;
; For example:
;   LDA $0010    ; Zero page: 2 bytes, 3 cycles
;   LDA $0200    ; Absolute:  3 bytes, 4 cycles
;
; We place ALL working variables here because this code is
; arithmetic-heavy: the multiply routine and 32-bit additions
; continuously load and store from these locations.
;
; The `.segment "ZEROPAGE"` directive tells cc65 to allocate these
; variables starting at address $00. The `.res N` directive reserves
; N bytes without initializing them (they start as whatever the
; hardware provides, but we always write before we read).
;
; Variable layout in zero page:
;   $00-$01 : angle_accum (16-bit fractional rotation accumulator)
;   $02-$03 : scale_accum (16-bit fractional zoom accumulator)
;   $04-$07 : var_ca      (32-bit: cos(angle) * reciprocal(scale))
;   $08-$0B : var_sa      (32-bit: sin(angle) * reciprocal(scale))
;   $0C-$0F : var_u0      (32-bit: texture U origin)
;   $10-$13 : var_v0      (32-bit: texture V origin)
;   $14-$15 : mul_a       (16-bit multiply operand A)
;   $16-$17 : mul_b       (16-bit multiply operand B)
;   $18-$1B : mul_result  (32-bit multiply result)
;   $1C-$1F : tmp32_a     (32-bit temporary for CA*320, SA*320)
;   $20-$23 : tmp32_b     (32-bit temporary for SA*240, CA*240)
;   $24-$27 : tmp32_c     (32-bit temporary for shifted values)
;   $28     : sign_flag   (tracks sign for signed multiply)
; ---------------------------------------------------------------------------
.segment "ZEROPAGE"
angle_accum:     .res 2          ; 16-bit 8.8 FP: high byte = sine table index
scale_accum:     .res 2          ; 16-bit 8.8 FP: high byte = reciprocal table index
var_ca:          .res 4          ; 32-bit signed: cos(angle) * reciprocal = du_col, dv_row
var_sa:          .res 4          ; 32-bit signed: sin(angle) * reciprocal = dv_col
var_u0:          .res 4          ; 32-bit signed: texture U start position
var_v0:          .res 4          ; 32-bit signed: texture V start position
mul_a:           .res 2          ; 16-bit multiply input A (consumed by mul16_signed)
mul_b:           .res 2          ; 16-bit multiply input B (consumed by mul16_signed)
mul_result:      .res 4          ; 32-bit multiply output (written by mul16u)
tmp32_a:         .res 4          ; 32-bit scratch: holds CA*320 or SA*320 result
tmp32_b:         .res 4          ; 32-bit scratch: holds SA*240 or CA*240 result
tmp32_c:         .res 4          ; 32-bit scratch: holds intermediate shifted value
sign_flag:       .res 1          ; Signed multiply: counts negative operands

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

; ============================================================================
; ENTRY POINT / MAIN LOOP
; ============================================================================
; The program begins here. We initialize the video hardware, generate
; the texture, start background music, then enter the infinite main loop.
;
; The main loop is structured for maximum clarity:
;   1. compute_frame   - Calculate the 6 affine transform parameters
;   2. render_mode7    - Program the blitter and render to back buffer
;   3. blit_to_front   - Copy completed frame to VRAM (prevents tearing)
;   4. wait_vsync      - Synchronize to display refresh (60 Hz)
;   5. advance_animation - Increment rotation and zoom accumulators
; ============================================================================
.proc main
    ; --- Enable the IE VideoChip ---
    ; VIDEO_CTRL = 1 enables the VideoChip display engine.
    ; VIDEO_MODE = 0 selects the default 640x480 32-bit RGBA mode.
    ; This must happen before any VRAM writes or blitter operations.
    lda #1
    sta VIDEO_CTRL
    lda #0
    sta VIDEO_MODE

    ; --- Generate the checkerboard texture ---
    ; Creates a 256x256 pixel checkerboard pattern at $500000 using
    ; 4 hardware blitter FILL operations (one per quadrant).
    jsr generate_texture

    ; --- Initialize animation accumulators to zero ---
    ; Both angle and scale start at 0. The high byte (table index) = 0
    ; means we start at sine[0]=0 (angle=0) and recip[0]=512 (max zoom).
    lda #0
    sta angle_accum
    sta angle_accum+1
    sta scale_accum
    sta scale_accum+1

    ; --- Start PSG music playback ---
    ; WHY PSG MUSIC?
    ; The 6502 version uses PSG (AY-3-8910 / YM2149) because it is the
    ; native sound chip associated with 8-bit Z80/6502 platforms (ZX
    ; Spectrum, Amstrad CPC, MSX). The AY format (.ay) contains embedded
    ; Z80 code that the audio subsystem executes on its own Z80 core.
    ;
    ; ENABLE_PSG_PLUS activates enhanced PSG mode (better mixing).
    ; SET_PSG_PTR sets the 32-bit pointer to the music data.
    ; SET_PSG_LEN sets the 32-bit data length.
    ; START_PSG_LOOP writes 5 ($05) to PSG_PLAY_CTRL:
    ;   bit 0 = start playback, bit 2 = enable looping.
    ;   This makes the music repeat endlessly.
    ENABLE_PSG_PLUS
    SET_PSG_PTR psg_data
    SET_PSG_LEN psg_data_end - psg_data
    START_PSG_LOOP

    ; === MAIN LOOP ===
    ; This runs forever at 60fps (synchronized by wait_vsync).
loop:
    jsr compute_frame
    jsr render_mode7
    jsr blit_to_front
    jsr wait_vsync
    jsr advance_animation
    jmp loop
.endproc

; ============================================================================
; WAIT FOR VSYNC (two-phase edge detection)
; ============================================================================
; Synchronizes to the vertical blanking interval to prevent tearing
; and ensure consistent 60fps timing.
;
; WHY TWO PHASES?
; A single "wait until vblank" could return immediately if we're already
; IN vblank (from the previous frame). The two-phase approach guarantees
; we wait for exactly one full frame:
;
;   Phase 1 (@wait_end): If we're currently in vblank, wait until it ends.
;                        This handles the case where we finished rendering
;                        during the vblank period.
;
;   Phase 2 (@wait_start): Wait until vblank BEGINS again.
;                          This catches the rising edge of the next vblank.
;
; The result is exactly one frame of delay, regardless of when we arrive.
;
; STATUS_VBLANK (= 2) is bit 1 of VIDEO_STATUS. When set, the display
; is in the vertical blanking interval (the beam is retracing from
; bottom to top, and no pixels are being drawn).
; ============================================================================
.proc wait_vsync
@wait_end:
    lda VIDEO_STATUS
    and #STATUS_VBLANK
    bne @wait_end           ; Loop while vblank is active (wait for it to end)
@wait_start:
    lda VIDEO_STATUS
    and #STATUS_VBLANK
    beq @wait_start         ; Loop while vblank is inactive (wait for it to start)
    rts
.endproc

; ============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; ============================================================================
; Creates a classic demoscene checkerboard texture using the hardware
; blitter's FILL operation (BLT_OP = 1). Each quadrant is filled
; separately:
;
;   ┌─────────────┬─────────────┐
;   │   WHITE     │   BLACK     │  Row 0-127
;   │  128x128    │  128x128    │
;   │ $500000     │ $500200     │
;   ├─────────────┼─────────────┤
;   │   BLACK     │   WHITE     │  Row 128-255
;   │  128x128    │  128x128    │
;   │ $520000     │ $520200     │
;   └─────────────┴─────────────┘
;
; Address calculations:
;   Top-left:     TEXTURE_BASE + 0 = $500000
;   Top-right:    TEXTURE_BASE + 128*4 = $500000 + 512 = $500200
;   Bottom-left:  TEXTURE_BASE + 128*1024 = $500000 + 131072 = $520000
;   Bottom-right: TEXTURE_BASE + 128*1024 + 128*4 = $500000 + 131584 = $520200
;
; Each pixel is 4 bytes (RGBA). Stride = 256 * 4 = 1024 bytes.
; The top-right offset is 128 pixels * 4 bytes = 512 bytes.
; The bottom-left offset is 128 rows * 1024 bytes/row = 131072 bytes.
;
; Colors:
;   White = $FFFFFFFF (RGBA: fully opaque white)
;   Black = $FF000000 (RGBA: fully opaque black)
;
; WHY USE THE BLITTER FOR TEXTURE GENERATION?
; The 6502 would need to write 256*256*4 = 262,144 bytes to create the
; texture. At ~5 cycles per STA, that's over 1.3 million cycles just
; for the stores. The blitter does it in hardware, essentially for free.
; ============================================================================
.proc generate_texture
    ; --- Top-left quadrant: 128x128 white ---
    STORE32 BLT_OP, 1              ; BLT_OP = 1 = FILL operation
    STORE32 BLT_DST_0, TEXTURE_BASE
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FFFFFFFF ; Opaque white (ABGR: $FF,$FF,$FF,$FF)
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT                      ; Trigger the blitter
    WAIT_BLIT                       ; Spin until blitter finishes

    ; --- Top-right quadrant: 128x128 black ---
    ; Offset = 128 pixels * 4 bytes = 512 bytes from texture base
    STORE32 BLT_OP, 1
    STORE32 BLT_DST_0, TEXTURE_BASE+512
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FF000000 ; Opaque black
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; --- Bottom-left quadrant: 128x128 black ---
    ; Offset = 128 rows * 1024 bytes/row = 131072 bytes from texture base
    STORE32 BLT_OP, 1
    STORE32 BLT_DST_0, TEXTURE_BASE+131072
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FF000000
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    ; --- Bottom-right quadrant: 128x128 white ---
    ; Offset = 131072 + 512 = 131584 bytes from texture base
    STORE32 BLT_OP, 1
    STORE32 BLT_DST_0, TEXTURE_BASE+131584
    STORE32 BLT_WIDTH_LO, 128
    STORE32 BLT_HEIGHT_LO, 128
    STORE32 BLT_COLOR_0, $FFFFFFFF
    STORE32 BLT_DST_STRIDE_LO, TEX_STRIDE
    START_BLIT
    WAIT_BLIT

    rts
.endproc

; ============================================================================
; COMPUTE FRAME - Calculate the 6 Mode 7 affine transform parameters
; ============================================================================
; This is the mathematical core of the rotozoomer. We compute:
;
;   CA = cos(angle) * reciprocal(scale)    (combined rotation + zoom)
;   SA = sin(angle) * reciprocal(scale)    (combined rotation + zoom)
;   u0 = 0x800000 - CA*320 + SA*240        (texture U origin)
;   v0 = 0x800000 - SA*320 - CA*240        (texture V origin)
;
; These give us the 6 blitter parameters:
;   du_col =  CA       (U step per column = rightward in texture)
;   dv_col =  SA       (V step per column)
;   du_row = -SA       (U step per row = downward in texture)
;   dv_row =  CA       (V step per row)
;   u0, v0             (starting texture coordinates)
;
; This is the standard 2D rotation matrix [[CA, -SA], [SA, CA]]
; combined with a zoom factor (the reciprocal table entry).
;
; === WHY PROPER SINE TABLES ===
;
; The sine table contains 256 entries computed as:
;   round(sin(i * 2*pi / 256) * 256)
;
; This gives true circular rotation with ~1.4 degree resolution
; (360/256 = 1.40625 degrees per step). The values range from -256 to
; +256 in 8.8 fixed-point, where 256 represents 1.0.
;
; === WHY A RECIPROCAL TABLE ===
;
; The 6502 has NO divide instruction. To compute the zoom factor, we
; would need to divide: zoom = base_scale / (0.5 + sin(scale_idx)*0.3).
; Instead, we precompute all 256 possible results into a lookup table:
;   recip_table[i] = round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
;
; The denominator oscillates between 0.2 and 0.8, giving zoom values
; from 320 to 1280 -- a 4:1 zoom range that creates the pulsing effect.
;
; === WHY 0x800000 FOR TEXTURE CENTER ===
;
; The Mode 7 blitter uses 16.16 fixed-point for texture coordinates.
; The texture is 256x256 pixels, so the centre is at pixel (128, 128).
; In 16.16 fixed-point: 128 * 65536 = 0x800000.
;
; We compute u0 = centre - CA*half_width + SA*half_height, which
; ensures the rotation pivots around the texture's centre point.
; ============================================================================
.proc compute_frame
    ; --- Extract table indices from 8.8 accumulators ---
    ; The high byte of each accumulator IS the table index (0-255).
    ; The low byte is the fractional part, providing smooth sub-step
    ; interpolation between frames (the fraction accumulates until
    ; it overflows into the integer byte, advancing the index).
    ldx angle_accum+1      ; X = angle index (0-255) for sine/cosine lookup
    ldy scale_accum+1      ; Y = scale index (0-255) for reciprocal lookup

    ; -----------------------------------------------------------------------
    ; STEP 1: Look up cos(angle) from the sine table
    ; -----------------------------------------------------------------------
    ; cos(angle) = sin(angle + 64) because 64 = 256/4 = 90 degrees.
    ; This identity lets us use a single sine table for both sin and cos.
    ;
    ; WHY SPLIT TABLE ACCESS WITH BCS:
    ; The 6502 can only index up to 255 bytes from a base address with
    ; the LDA table,X addressing mode. Our sine table is 512 bytes
    ; (256 entries * 2 bytes each). After ASL A (multiply by 2 for word
    ; index), if the carry flag is set, the original index was >= 128,
    ; meaning the byte offset exceeds 255. We branch to access the
    ; upper half of the table at sine_table+256.
    ;
    ; This is the standard 6502 idiom for tables larger than 256 bytes.
    txa
    clc
    adc #64                 ; A = (angle_idx + 64) & 0xFF (natural 8-bit wrap)
    asl a                   ; *2 for word index; carry set if index >= 128
    tax
    bcs @cos_hi
    lda sine_table,x        ; Load cos low byte from table lower half
    sta mul_a
    lda sine_table+1,x      ; Load cos high byte
    sta mul_a+1
    jmp @cos_done
@cos_hi:
    lda sine_table+256,x    ; Load cos low byte from table upper half
    sta mul_a
    lda sine_table+257,x    ; Load cos high byte
    sta mul_a+1
@cos_done:

    ; -----------------------------------------------------------------------
    ; STEP 2: Look up reciprocal(scale) from the reciprocal table
    ; -----------------------------------------------------------------------
    ; The reciprocal table provides the zoom factor. Same split-table
    ; access pattern as the sine table (512 bytes = 256 word entries).
    tya                      ; A = scale_idx
    asl a                    ; *2 for word index
    tax
    bcs @rcp1_hi
    lda recip_table,x
    sta mul_b
    lda recip_table+1,x
    sta mul_b+1
    jmp @rcp1_done
@rcp1_hi:
    lda recip_table+256,x
    sta mul_b
    lda recip_table+257,x
    sta mul_b+1
@rcp1_done:

    ; -----------------------------------------------------------------------
    ; STEP 3: CA = cos(angle) * reciprocal(scale)
    ; -----------------------------------------------------------------------
    ; This 16x16 signed multiply produces a 32-bit result that combines
    ; rotation and zoom into a single value. CA represents how much the
    ; texture U coordinate changes per column step (du_col) and also
    ; the V change per row step (dv_row).
    jsr mul16_signed
    lda mul_result
    sta var_ca
    lda mul_result+1
    sta var_ca+1
    lda mul_result+2
    sta var_ca+2
    lda mul_result+3
    sta var_ca+3

    ; -----------------------------------------------------------------------
    ; STEP 4: Look up sin(angle) from the sine table
    ; -----------------------------------------------------------------------
    ; Same split-table pattern. Note we reload angle_accum+1 because
    ; the multiply may have used X for something else.
    lda angle_accum+1
    asl a                    ; *2 for word index
    tax
    bcs @sin_hi
    lda sine_table,x
    sta mul_a
    lda sine_table+1,x
    sta mul_a+1
    jmp @sin_done
@sin_hi:
    lda sine_table+256,x
    sta mul_a
    lda sine_table+257,x
    sta mul_a+1
@sin_done:

    ; -----------------------------------------------------------------------
    ; STEP 5: Look up reciprocal(scale) again for SA
    ; -----------------------------------------------------------------------
    ; We re-read the reciprocal because mul16_signed consumed mul_b.
    lda scale_accum+1
    asl a
    tax
    bcs @rcp2_hi
    lda recip_table,x
    sta mul_b
    lda recip_table+1,x
    sta mul_b+1
    jmp @rcp2_done
@rcp2_hi:
    lda recip_table+256,x
    sta mul_b
    lda recip_table+257,x
    sta mul_b+1
@rcp2_done:

    ; -----------------------------------------------------------------------
    ; STEP 6: SA = sin(angle) * reciprocal(scale)
    ; -----------------------------------------------------------------------
    ; SA represents how much the texture V coordinate changes per column
    ; step (dv_col). Its negation (-SA) is du_row.
    jsr mul16_signed
    lda mul_result
    sta var_sa
    lda mul_result+1
    sta var_sa+1
    lda mul_result+2
    sta var_sa+2
    lda mul_result+3
    sta var_sa+3

    ; -----------------------------------------------------------------------
    ; STEP 7: Compute u0 = 0x800000 - CA*320 + SA*240
    ; -----------------------------------------------------------------------
    ; This positions the texture so that rotation is centreed on the
    ; middle of both the screen (320x240 half-dimensions) and the
    ; texture (0x800000 = 128.0 in 16.16 fixed-point).
    ;
    ; The formula derives from: for the centre pixel (x=320, y=240),
    ; we want u = 128.0 (texture centre). Working backwards:
    ;   u(320,240) = u0 + 320*du_col + 240*du_row
    ;   128.0      = u0 + 320*CA + 240*(-SA)
    ;   u0         = 128.0 - 320*CA + 240*SA
    ;              = 0x800000 - CA*320 + SA*240
    ;
    ; WHY SHIFT LOOPS FOR *320 AND *240:
    ; The 6502 has no barrel shifter and no multiply instruction.
    ; We decompose the multiplications using power-of-two shifts:
    ;   320 = 256 + 64 = (val << 8) + (val << 6)
    ;   240 = 256 - 16 = (val << 8) - (val << 4)
    ;
    ; The <<8 is a "free" byte-shift (just copy bytes up by one
    ; position and zero the bottom byte). The <<6 requires 6
    ; iterations of ASL/ROL across the 32-bit value (one bit per
    ; iteration because there is no barrel shifter). Similarly,
    ; <<4 requires 4 iterations.
    jsr compute_ca_320      ; tmp32_a = CA * 320
    jsr compute_sa_240      ; tmp32_b = SA * 240

    ; u0 = 0x800000 (texture centre in 16.16 fixed-point)
    lda #$00
    sta var_u0
    sta var_u0+1
    lda #$80                ; 0x800000 = 128.0 in 16.16 FP
    sta var_u0+2
    lda #$00
    sta var_u0+3

    ; u0 -= CA*320 (subtract rotation contribution from columns)
    ; WHY SEC/SBC FOR SUBTRACTION:
    ; The 6502 has no SUB instruction. Subtraction is done via SBC
    ; (Subtract with Carry). SEC sets the carry flag first, which
    ; acts as "no borrow" for the first byte. Subsequent SBC
    ; instructions automatically propagate the borrow through carry.
    sec
    lda var_u0
    sbc tmp32_a
    sta var_u0
    lda var_u0+1
    sbc tmp32_a+1
    sta var_u0+1
    lda var_u0+2
    sbc tmp32_a+2
    sta var_u0+2
    lda var_u0+3
    sbc tmp32_a+3
    sta var_u0+3

    ; u0 += SA*240 (add rotation contribution from rows)
    ; Note: du_row = -SA, so moving down a row SUBTRACTS SA from U.
    ; The SA*240 term here is ADDED because u0 needs to compensate
    ; for the 240 rows of -SA that will be accumulated.
    clc
    lda var_u0
    adc tmp32_b
    sta var_u0
    lda var_u0+1
    adc tmp32_b+1
    sta var_u0+1
    lda var_u0+2
    adc tmp32_b+2
    sta var_u0+2
    lda var_u0+3
    adc tmp32_b+3
    sta var_u0+3

    ; -----------------------------------------------------------------------
    ; STEP 8: Compute v0 = 0x800000 - SA*320 - CA*240
    ; -----------------------------------------------------------------------
    ; Same centreing logic for the V coordinate:
    ;   v(320,240) = v0 + 320*dv_col + 240*dv_row
    ;   128.0      = v0 + 320*SA + 240*CA
    ;   v0         = 128.0 - 320*SA - 240*CA
    ;              = 0x800000 - SA*320 - CA*240
    ;
    ; Note both terms are SUBTRACTED here (unlike u0 where SA*240
    ; is added). This is because dv_row = +CA (not negated).
    jsr compute_sa_320      ; tmp32_a = SA * 320
    jsr compute_ca_240      ; tmp32_b = CA * 240

    ; v0 = 0x800000
    lda #$00
    sta var_v0
    sta var_v0+1
    lda #$80
    sta var_v0+2
    lda #$00
    sta var_v0+3

    ; v0 -= SA*320
    sec
    lda var_v0
    sbc tmp32_a
    sta var_v0
    lda var_v0+1
    sbc tmp32_a+1
    sta var_v0+1
    lda var_v0+2
    sbc tmp32_a+2
    sta var_v0+2
    lda var_v0+3
    sbc tmp32_a+3
    sta var_v0+3

    ; v0 -= CA*240
    sec
    lda var_v0
    sbc tmp32_b
    sta var_v0
    lda var_v0+1
    sbc tmp32_b+1
    sta var_v0+1
    lda var_v0+2
    sbc tmp32_b+2
    sta var_v0+2
    lda var_v0+3
    sbc tmp32_b+3
    sta var_v0+3

    rts
.endproc

; ============================================================================
; MULTIPLY BY 320: CA*320 -> tmp32_a
; ============================================================================
; Computes a 32-bit value * 320 using shift-and-add decomposition:
;   val * 320 = val * 256 + val * 64 = (val << 8) + (val << 6)
;
; WHY THIS DECOMPOSITION:
; 320 = 0x140 = 256 + 64. Both 256 and 64 are powers of two, so
; each "multiply" is just a left shift. The 6502 has no barrel
; shifter, so each shift is done one bit at a time via ASL/ROL.
;
; The <<8 shift is essentially free: we just copy the bytes up by
; one position and zero the bottom byte. For <<6, we need 6
; iterations of the 4-byte ASL/ROL chain.
;
; Alternative decompositions like 320 = 5 * 64 would require a
; multiply by 5, which itself needs shifts and adds. The 256+64
; decomposition minimizes total shift iterations (0 + 6 = 6 shifts
; vs. the 64*5 approach which needs 6 + 2 + 1 = 9).
; ============================================================================
.proc compute_ca_320
    ; --- CA * 256 via byte shift (instant, no loop needed) ---
    ; Shifting left by 8 bits = moving each byte up one position.
    ; Original: [ca+0, ca+1, ca+2, ca+3] (32-bit little-endian)
    ; Result:   [0, ca+0, ca+1, ca+2]
    ; (ca+3 is shifted out / lost -- this is fine because the result
    ; fits in 32 bits for our value range)
    lda #0
    sta tmp32_c
    lda var_ca
    sta tmp32_c+1
    lda var_ca+1
    sta tmp32_c+2
    lda var_ca+2
    sta tmp32_c+3

    ; --- CA * 64 via 6 iterations of shift-left-1 ---
    ; Start with a copy of CA in tmp32_a, then shift left 6 times.
    ; Each iteration: ASL shifts byte 0 left (bit 7 -> carry),
    ; ROL shifts byte 1 left (carry -> bit 0, bit 7 -> carry),
    ; ROL shifts byte 2, ROL shifts byte 3.
    ; This propagates bits across the 32-bit value, one position per loop.
    lda var_ca
    sta tmp32_a
    lda var_ca+1
    sta tmp32_a+1
    lda var_ca+2
    sta tmp32_a+2
    lda var_ca+3
    sta tmp32_a+3
    ldx #6                  ; 6 iterations for <<6
:   asl tmp32_a             ; Shift 32-bit value left by 1
    rol tmp32_a+1
    rol tmp32_a+2
    rol tmp32_a+3
    dex
    bne :-                  ; Loop until X = 0

    ; --- CA * 320 = (CA << 8) + (CA << 6) ---
    ; Add the two partial products. Result stored in tmp32_a.
    clc
    lda tmp32_c
    adc tmp32_a
    sta tmp32_a
    lda tmp32_c+1
    adc tmp32_a+1
    sta tmp32_a+1
    lda tmp32_c+2
    adc tmp32_a+2
    sta tmp32_a+2
    lda tmp32_c+3
    adc tmp32_a+3
    sta tmp32_a+3
    rts
.endproc

; ============================================================================
; MULTIPLY BY 320: SA*320 -> tmp32_a
; ============================================================================
; Identical algorithm to compute_ca_320 but operates on var_sa.
; SA * 320 = (SA << 8) + (SA << 6).
; ============================================================================
.proc compute_sa_320
    ; SA << 8 (byte shift)
    lda #0
    sta tmp32_c
    lda var_sa
    sta tmp32_c+1
    lda var_sa+1
    sta tmp32_c+2
    lda var_sa+2
    sta tmp32_c+3

    ; SA << 6 (6 iterations of shift-left-1)
    lda var_sa
    sta tmp32_a
    lda var_sa+1
    sta tmp32_a+1
    lda var_sa+2
    sta tmp32_a+2
    lda var_sa+3
    sta tmp32_a+3
    ldx #6
:   asl tmp32_a
    rol tmp32_a+1
    rol tmp32_a+2
    rol tmp32_a+3
    dex
    bne :-

    ; SA*320 = (SA<<8) + (SA<<6)
    clc
    lda tmp32_c
    adc tmp32_a
    sta tmp32_a
    lda tmp32_c+1
    adc tmp32_a+1
    sta tmp32_a+1
    lda tmp32_c+2
    adc tmp32_a+2
    sta tmp32_a+2
    lda tmp32_c+3
    adc tmp32_a+3
    sta tmp32_a+3
    rts
.endproc

; ============================================================================
; MULTIPLY BY 240: SA*240 -> tmp32_b
; ============================================================================
; Computes a 32-bit value * 240 using shift-and-subtract:
;   val * 240 = val * 256 - val * 16 = (val << 8) - (val << 4)
;
; WHY SUBTRACT INSTEAD OF ADD:
; 240 = 256 - 16. Both are powers of two, so we shift and subtract.
; An additive decomposition would be 240 = 128 + 64 + 32 + 16 =
; (val << 7) + (val << 6) + (val << 5) + (val << 4), requiring
; 7 + 6 + 5 + 4 = 22 shift iterations plus 3 additions.
; The subtractive approach needs only 0 + 4 = 4 shift iterations
; plus 1 subtraction. Much faster.
; ============================================================================
.proc compute_sa_240
    ; SA << 8 (byte shift -- zero iterations, just byte copy)
    lda #0
    sta tmp32_c
    lda var_sa
    sta tmp32_c+1
    lda var_sa+1
    sta tmp32_c+2
    lda var_sa+2
    sta tmp32_c+3

    ; SA << 4 (4 iterations of shift-left-1)
    lda var_sa
    sta tmp32_b
    lda var_sa+1
    sta tmp32_b+1
    lda var_sa+2
    sta tmp32_b+2
    lda var_sa+3
    sta tmp32_b+3
    ldx #4                  ; 4 iterations for <<4
:   asl tmp32_b
    rol tmp32_b+1
    rol tmp32_b+2
    rol tmp32_b+3
    dex
    bne :-

    ; SA*240 = (SA<<8) - (SA<<4)
    ; WHY SEC/SBC: The 6502 subtraction pattern. SEC clears the
    ; borrow flag (carry=1 means "no borrow"). Each subsequent SBC
    ; propagates borrow automatically through carry.
    sec
    lda tmp32_c
    sbc tmp32_b
    sta tmp32_b
    lda tmp32_c+1
    sbc tmp32_b+1
    sta tmp32_b+1
    lda tmp32_c+2
    sbc tmp32_b+2
    sta tmp32_b+2
    lda tmp32_c+3
    sbc tmp32_b+3
    sta tmp32_b+3
    rts
.endproc

; ============================================================================
; MULTIPLY BY 240: CA*240 -> tmp32_b
; ============================================================================
; Identical algorithm to compute_sa_240 but operates on var_ca.
; CA * 240 = (CA << 8) - (CA << 4).
; ============================================================================
.proc compute_ca_240
    ; CA << 8 (byte shift)
    lda #0
    sta tmp32_c
    lda var_ca
    sta tmp32_c+1
    lda var_ca+1
    sta tmp32_c+2
    lda var_ca+2
    sta tmp32_c+3

    ; CA << 4 (4 iterations)
    lda var_ca
    sta tmp32_b
    lda var_ca+1
    sta tmp32_b+1
    lda var_ca+2
    sta tmp32_b+2
    lda var_ca+3
    sta tmp32_b+3
    ldx #4
:   asl tmp32_b
    rol tmp32_b+1
    rol tmp32_b+2
    rol tmp32_b+3
    dex
    bne :-

    ; CA*240 = (CA<<8) - (CA<<4)
    sec
    lda tmp32_c
    sbc tmp32_b
    sta tmp32_b
    lda tmp32_c+1
    sbc tmp32_b+1
    sta tmp32_b+1
    lda tmp32_c+2
    sbc tmp32_b+2
    sta tmp32_b+2
    lda tmp32_c+3
    sbc tmp32_b+3
    sta tmp32_b+3
    rts
.endproc

; ============================================================================
; MUL16_SIGNED: mul_result (32-bit) = mul_a * mul_b (signed 16 x 16)
; ============================================================================
; The 6502 has NO multiply instruction. This routine implements signed
; 16x16 -> 32-bit multiplication using the sign-magnitude approach:
;
;   1. Record the sign of each operand (check bit 7 of high byte)
;   2. Negate any negative operands to make them positive
;   3. Perform unsigned multiplication (mul16u)
;   4. If the signs differed (one negative, one positive), negate result
;
; WHY SIGN-MAGNITUDE INSTEAD OF TWO'S COMPLEMENT MULTIPLY?
; A two's complement multiply would need to handle sign extension at
; every partial product addition. The sign-magnitude approach is
; simpler: convert to positive, do unsigned math, fix the sign at the
; end. The overhead of up to 2 negations is small compared to the
; 16-iteration multiply loop.
;
; WHY `sign_flag` VARIABLE INSTEAD OF A REGISTER?
; The 6502 has only three registers: A (accumulator), X, and Y. All
; three are needed during the negation and multiply operations. There
; is literally no spare register to hold the sign. We use a zero-page
; byte instead: increment it for each negative operand, then test bit 0.
; Odd = one negative operand = negate result. Even = zero or two
; negatives = keep result positive.
;
; WHY SEC; LDA #0; SBC FOR NEGATION?
; The 6502 has no NEG instruction. Two's complement negation is
; computed as: -value = 0 - value. In 6502 idiom:
;   SEC          ; Set carry (no borrow)
;   LDA #0       ; Load zero
;   SBC value    ; Subtract value: A = 0 - value - !carry = 0 - value
; For multi-byte negation, the carry propagates automatically through
; subsequent SBC instructions, handling the borrow chain.
; ============================================================================
.proc mul16_signed
    lda #0
    sta sign_flag           ; Start with sign count = 0

    ; --- Check and normalize operand A ---
    lda mul_a+1             ; High byte of mul_a
    bpl @a_pos              ; BPL: branch if bit 7 clear (positive)
    ; mul_a is negative -- negate to positive
    sec
    lda #0
    sbc mul_a               ; Low byte: 0 - mul_a
    sta mul_a
    lda #0
    sbc mul_a+1             ; High byte: 0 - mul_a+1 - borrow
    sta mul_a+1
    inc sign_flag           ; Record that one operand was negative
@a_pos:
    ; --- Check and normalize operand B ---
    lda mul_b+1
    bpl @b_pos
    sec
    lda #0
    sbc mul_b
    sta mul_b
    lda #0
    sbc mul_b+1
    sta mul_b+1
    inc sign_flag           ; Now sign_flag = 0, 1, or 2
@b_pos:
    ; --- Perform unsigned multiply with both operands now positive ---
    jsr mul16u

    ; --- Fix sign of result ---
    ; If sign_flag is odd (exactly one operand was negative), negate
    ; the 32-bit result. If even (0 or 2 negatives), result is correct.
    lda sign_flag
    and #1                  ; Test bit 0: odd or even?
    beq @done               ; Even = both same sign = result is positive
    ; Negate 32-bit result using SEC/LDA #0/SBC chain
    ; Each SBC propagates borrow to the next byte automatically.
    sec
    lda #0
    sbc mul_result
    sta mul_result
    lda #0
    sbc mul_result+1
    sta mul_result+1
    lda #0
    sbc mul_result+2
    sta mul_result+2
    lda #0
    sbc mul_result+3
    sta mul_result+3
@done:
    rts
.endproc

; ============================================================================
; MUL16U: mul_result (32-bit) = mul_a * mul_b (unsigned 16x16 -> 32)
; ============================================================================
; Classic shift-and-add binary multiplication algorithm.
;
; WHY SHIFT-AND-ADD:
; Binary multiplication works exactly like long multiplication in
; decimal, but simpler: each "digit" is 0 or 1, so each partial
; product is either 0 (skip) or the multiplicand (add).
;
; Algorithm:
;   result = 0
;   for each bit in mul_a (MSB to LSB):
;     result <<= 1            ; Shift result left (make room)
;     if bit was 1:
;       result += mul_b       ; Add multiplicand
;
; We process 16 bits (one for each bit of mul_a), so the loop runs
; exactly 16 times. The result can be up to 32 bits wide because
; the maximum product is 65535 * 65535 = 4,294,836,225.
;
; WHY MSB-FIRST:
; We shift mul_a left and check the carry flag. The MSB (bit 15)
; comes out first into carry via the ROL instruction. The result
; is also shifted left each iteration, building up from the MSB.
; This avoids the need for a separate bit counter -- after 16
; shifts, all bits of mul_a have been consumed.
;
; PERFORMANCE: 16 iterations * ~25 cycles = ~400 cycles per multiply.
; On a 1 MHz 6502, that's 0.4ms -- acceptable for 2 multiplies per frame.
; ============================================================================
.proc mul16u
    ; Clear 32-bit result
    lda #0
    sta mul_result
    sta mul_result+1
    sta mul_result+2
    sta mul_result+3

    ldx #16                 ; 16 bits to process
@loop:
    ; --- Shift result left by 1 bit (32-bit) ---
    ; ASL shifts byte 0 left, putting bit 7 into carry.
    ; ROL shifts byte 1 left, carry in from bit 0, bit 7 to carry.
    ; ROL shifts byte 2, ROL shifts byte 3.
    asl mul_result
    rol mul_result+1
    rol mul_result+2
    rol mul_result+3

    ; --- Shift mul_a left, extracting MSB into carry ---
    ; After this, carry = the current bit of mul_a.
    ; mul_a is consumed (shifted to zero) after 16 iterations.
    asl mul_a
    rol mul_a+1
    bcc @no_add             ; If carry = 0, this bit is 0 -- skip addition

    ; --- Add mul_b to the low 16 bits of result ---
    ; If carry was set, this bit of mul_a is 1, so we add mul_b.
    ; We only add to the low 16 bits; the carry chain propagates
    ; into the high bytes via INC.
    clc
    lda mul_result
    adc mul_b
    sta mul_result
    lda mul_result+1
    adc mul_b+1
    sta mul_result+1
    bcc @no_add             ; No carry into byte 2 -- done
    inc mul_result+2        ; Propagate carry into byte 2
    bne @no_add             ; If byte 2 didn't wrap to 0, done
    inc mul_result+3        ; Propagate carry into byte 3
@no_add:
    dex
    bne @loop               ; Loop for all 16 bits
    rts
.endproc

; ============================================================================
; RENDER MODE 7 - Program the blitter for affine texture mapping
; ============================================================================
; This procedure sets up all Mode 7 blitter registers and triggers the
; hardware affine texture transformation. The blitter reads the texture
; from $500000, applies the affine transform defined by the 6 parameters,
; and writes the result to the back buffer at $600000.
;
; MODE 7 BLITTER REGISTERS:
;   BLT_OP          = 5 (Mode 7 affine texture mapping)
;   BLT_SRC         = texture base address
;   BLT_DST         = output buffer address
;   BLT_WIDTH/HEIGHT = output dimensions (640x480)
;   BLT_SRC_STRIDE  = texture stride (1024 = 256 pixels * 4 bytes)
;   BLT_DST_STRIDE  = output stride (2560 = 640 pixels * 4 bytes)
;   BLT_MODE7_TEX_W = texture width mask (255 for 256-pixel wrap)
;   BLT_MODE7_TEX_H = texture height mask (255 for 256-pixel wrap)
;   BLT_MODE7_U0    = starting U coordinate (32-bit)
;   BLT_MODE7_V0    = starting V coordinate (32-bit)
;   BLT_MODE7_DU_COL = U step per column = CA (32-bit)
;   BLT_MODE7_DV_COL = V step per column = SA (32-bit)
;   BLT_MODE7_DU_ROW = U step per row = -SA (32-bit)
;   BLT_MODE7_DV_ROW = V step per row = CA (32-bit)
;
; WHY BYTE-BY-BYTE MODE 7 REGISTER WRITES:
; The 6502 has 8-bit registers and 16-bit addresses, but the Mode 7
; registers are 32 bits wide. Each register is exposed as 4 consecutive
; byte-level MMIO addresses (e.g., BLT_MODE7_U0_0 through
; BLT_MODE7_U0_3). The STORE32 macro handles this decomposition for
; compile-time constants, but for runtime variables (u0, v0, CA, SA),
; we must manually load each byte from zero page and store it to the
; corresponding MMIO address.
;
; WHY THE ROTATION MATRIX IS [[CA, -SA], [SA, CA]]:
; This is the standard 2D rotation matrix. CA = cos(angle)*zoom,
; SA = sin(angle)*zoom. The Mode 7 parameters map directly:
;   du_col = CA   (moving right in screen = moving in +cos direction in texture)
;   dv_col = SA   (moving right in screen = moving in +sin direction in texture)
;   du_row = -SA  (moving down in screen = moving in -sin direction in texture)
;   dv_row = CA   (moving down in screen = moving in +cos direction in texture)
; ============================================================================
.proc render_mode7
    ; --- Set up constant blitter parameters ---
    ; These don't change between frames but must be set each time
    ; because other blitter operations (generate_texture, blit_to_front)
    ; overwrite these registers.
    STORE32 BLT_OP, BLT_OP_MODE7_OP
    STORE32 BLT_SRC_0, TEXTURE_BASE
    STORE32 BLT_DST_0, BACK_BUFFER
    STORE32 BLT_WIDTH_LO, RENDER_W
    STORE32 BLT_HEIGHT_LO, RENDER_H
    STORE32 BLT_SRC_STRIDE_LO, TEX_STRIDE
    STORE32 BLT_DST_STRIDE_LO, LINE_BYTES
    STORE32 BLT_MODE7_TEX_W_0, TEX_W_MASK
    STORE32 BLT_MODE7_TEX_H_0, TEX_H_MASK

    ; --- Write u0 (starting U texture coordinate) ---
    ; 4 bytes, little-endian, from zero page to MMIO registers
    lda var_u0
    sta BLT_MODE7_U0_0
    lda var_u0+1
    sta BLT_MODE7_U0_1
    lda var_u0+2
    sta BLT_MODE7_U0_2
    lda var_u0+3
    sta BLT_MODE7_U0_3

    ; --- Write v0 (starting V texture coordinate) ---
    lda var_v0
    sta BLT_MODE7_V0_0
    lda var_v0+1
    sta BLT_MODE7_V0_1
    lda var_v0+2
    sta BLT_MODE7_V0_2
    lda var_v0+3
    sta BLT_MODE7_V0_3

    ; --- Write du_col = CA (U change per pixel moving right) ---
    lda var_ca
    sta BLT_MODE7_DU_COL_0
    lda var_ca+1
    sta BLT_MODE7_DU_COL_1
    lda var_ca+2
    sta BLT_MODE7_DU_COL_2
    lda var_ca+3
    sta BLT_MODE7_DU_COL_3

    ; --- Write dv_col = SA (V change per pixel moving right) ---
    lda var_sa
    sta BLT_MODE7_DV_COL_0
    lda var_sa+1
    sta BLT_MODE7_DV_COL_1
    lda var_sa+2
    sta BLT_MODE7_DV_COL_2
    lda var_sa+3
    sta BLT_MODE7_DV_COL_3

    ; --- Write du_row = -SA (U change per pixel moving down) ---
    ; WHY SEC; LDA #0; SBC FOR NEGATION:
    ; The 6502 has no NEG instruction. Two's complement negate is
    ; computed as 0 - value. SEC sets carry (no borrow), then each
    ; SBC subtracts the corresponding byte, propagating borrow
    ; automatically through the carry flag to the next byte.
    ; This 4-byte chain produces the 32-bit negation of SA.
    sec
    lda #0
    sbc var_sa
    sta BLT_MODE7_DU_ROW_0
    lda #0
    sbc var_sa+1
    sta BLT_MODE7_DU_ROW_1
    lda #0
    sbc var_sa+2
    sta BLT_MODE7_DU_ROW_2
    lda #0
    sbc var_sa+3
    sta BLT_MODE7_DU_ROW_3

    ; --- Write dv_row = CA (V change per pixel moving down) ---
    lda var_ca
    sta BLT_MODE7_DV_ROW_0
    lda var_ca+1
    sta BLT_MODE7_DV_ROW_1
    lda var_ca+2
    sta BLT_MODE7_DV_ROW_2
    lda var_ca+3
    sta BLT_MODE7_DV_ROW_3

    ; --- Trigger the blitter and wait for completion ---
    ; START_BLIT writes 1 to BLT_CTRL, starting the hardware operation.
    ; WAIT_BLIT spins on BLT_CTRL bit 1 (busy flag) until the blitter
    ; finishes rendering all 307,200 pixels. During this time the CPU
    ; is idle, but the blitter is working at hardware speed.
    START_BLIT
    WAIT_BLIT
    rts
.endproc

; ============================================================================
; BLIT BACK BUFFER TO FRONT BUFFER
; ============================================================================
; Copies the completed Mode 7 render from the back buffer ($600000)
; to VRAM ($100000) using a hardware block copy (BLT_OP = 0).
;
; WHY NOT RENDER DIRECTLY TO VRAM?
; If the blitter wrote directly to VRAM, the display would show a
; partially-rendered frame during the blit operation. This manifests
; as "tearing" -- the top half shows the new frame while the bottom
; half still shows the previous frame. Double buffering eliminates
; this artifact: we render to an off-screen buffer, then copy the
; complete frame to VRAM between display refreshes.
;
; The copy itself is a simple source-to-destination block transfer:
;   - Source: BACK_BUFFER ($600000)
;   - Destination: VRAM_START ($100000)
;   - Both use LINE_BYTES (2560) stride
;   - Full screen dimensions (640x480)
; ============================================================================
.proc blit_to_front
    STORE32 BLT_OP, 0               ; BLT_OP = 0 = block copy
    STORE32 BLT_SRC_0, BACK_BUFFER
    STORE32 BLT_DST_0, VRAM_START
    STORE32 BLT_WIDTH_LO, RENDER_W
    STORE32 BLT_HEIGHT_LO, RENDER_H
    STORE32 BLT_SRC_STRIDE_LO, LINE_BYTES
    STORE32 BLT_DST_STRIDE_LO, LINE_BYTES
    START_BLIT
    WAIT_BLIT
    rts
.endproc

; ============================================================================
; ADVANCE ANIMATION - Increment rotation and zoom accumulators
; ============================================================================
; Updates the 16-bit 8.8 fixed-point accumulators that drive the
; rotation angle and zoom scale.
;
; WHY 8.8 FIXED-POINT ACCUMULATORS:
; Using a 16-bit accumulator with an 8-bit fractional part gives us
; sub-index precision. Each frame, we add the increment value. The
; fractional part accumulates until it overflows into the integer byte,
; which IS the table index. This creates smooth, non-integer stepping
; through the tables.
;
; For example, with ANGLE_INC = 313 ($0139):
;   Frame 0: accum = $0000, index = $00 (entry 0)
;   Frame 1: accum = $0139, index = $01 (entry 1)
;   Frame 2: accum = $0272, index = $02 (entry 2)
;   Frame 3: accum = $03AB, index = $03 (entry 3)
;   ...continuing with fractional accumulation...
;
; WHY NATURAL 16-BIT WRAP:
; When the 16-bit accumulator overflows past $FFFF, it wraps to $0000
; automatically. The ADC instruction adds the low byte; if it carries,
; the next ADC picks up that carry into the high byte. If the high
; byte overflows past $FF, the carry is simply lost. This gives us
; free modulo-65536 behavior. Since our tables have 256 entries and
; the index is the high byte, this effectively wraps modulo 256
; (because the high byte wraps from $FF to $00 on overflow).
; No AND mask is needed.
; ============================================================================
.proc advance_animation
    ; --- Advance rotation angle ---
    ; angle_accum += ANGLE_INC (313 = $0139)
    clc
    lda angle_accum
    adc #ANGLE_INC_LO       ; Add low byte ($39)
    sta angle_accum
    lda angle_accum+1
    adc #ANGLE_INC_HI       ; Add high byte ($01), plus carry from low byte
    sta angle_accum+1       ; Natural 16-bit wrap on overflow

    ; --- Advance zoom scale ---
    ; scale_accum += SCALE_INC (104 = $0068)
    clc
    lda scale_accum
    adc #SCALE_INC_LO       ; Add low byte ($68)
    sta scale_accum
    lda scale_accum+1
    adc #SCALE_INC_HI       ; Add high byte ($00), plus carry
    sta scale_accum+1       ; Natural 16-bit wrap on overflow

    rts
.endproc

; ============================================================================
; DATA TABLES
; ============================================================================
; All lookup tables are placed in the RODATA segment, which cc65
; allocates in read-only program memory. These tables are generated
; at assembly time and never modified at runtime.
; ============================================================================
.segment "RODATA"

; ============================================================================
; SINE TABLE - 256 entries, signed 16-bit little-endian
; ============================================================================
; Precomputed sine values for angles 0 through 255 (0 to 360 degrees).
; Each entry is a 16-bit signed word: round(sin(i * 2*pi / 256) * 256).
; Values range from -256 to +256, representing -1.0 to +1.0 in 8.8 FP.
;
; WHY PROPER SINE TABLES:
; Using round(sin(i*2*pi/256)*256) gives true circular rotation.
; 256 entries = 360/256 = ~1.4 degree resolution, which is smooth
; enough that individual angle steps are invisible at 60fps.
;
; The table is indexed by: angle_index * 2 (because each entry is
; 2 bytes). Cosine is obtained by looking up sin(angle + 64), since
; 64 = 256/4 = 90 degrees.
;
; WHY UNSIGNED REPRESENTATION IN NEGATIVE ENTRIES:
; cc65's .word directive interprets values as unsigned 16-bit. Negative
; values like -6 are stored as 65530 (0xFFFA) -- the two's complement
; unsigned equivalent. When loaded into registers and used in arithmetic,
; the bit pattern is identical to signed -6. The signed multiply routine
; (mul16_signed) handles this correctly via sign-magnitude conversion:
; it checks bit 15 (the sign bit), negates if needed, does unsigned
; math, then fixes the sign.
;
; TABLE STRUCTURE:
;   Entries   0- 63: sin(0) to sin(90)   =   0 to +256 (ascending)
;   Entries  64-127: sin(90) to sin(180)  = +256 to   0 (descending)
;   Entries 128-191: sin(180) to sin(270) =   0 to -256 (descending)
;   Entries 192-255: sin(270) to sin(360) = -256 to   0 (ascending)
;
; TOTAL SIZE: 256 entries * 2 bytes = 512 bytes.
; ============================================================================
sine_table:
    ;         0    1    2    3    4    5    6    7    8    9   10   11   12   13   14   15
    .word 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
    .word 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
    .word 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
    .word 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
    ;        64   65   66   67   68   69   70   71   72   73   74   75   76   77   78   79
    .word 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
    .word 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
    .word 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
    .word 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
    ;       128  129  130  131  (negative half: stored as unsigned two's complement)
    ;       -6 stored as 65530 ($FFFA), -13 as 65523 ($FFE3), etc.
    .word 0,65530,65523,65517,65511,65505,65498,65492,65486,65480,65474,65468,65462,65456,65450,65444
    .word 65438,65432,65427,65421,65415,65410,65404,65399,65394,65389,65384,65379,65374,65369,65364,65359
    .word 65355,65351,65346,65342,65338,65334,65330,65327,65323,65320,65316,65313,65310,65307,65305,65302
    .word 65299,65297,65295,65293,65291,65289,65288,65286,65285,65284,65283,65282,65281,65281,65280,65280
    ;       192  193  194  195  (ascending back toward 0)
    .word 65280,65280,65280,65281,65281,65282,65283,65284,65285,65286,65288,65289,65291,65293,65295,65297
    .word 65299,65302,65305,65307,65310,65313,65316,65320,65323,65327,65330,65334,65338,65342,65346,65351
    .word 65355,65359,65364,65369,65374,65379,65384,65389,65394,65399,65404,65410,65415,65421,65427,65432
    .word 65438,65444,65450,65456,65462,65468,65474,65480,65486,65492,65498,65505,65511,65517,65523,65530

; ============================================================================
; RECIPROCAL TABLE - 256 entries, unsigned 16-bit little-endian
; ============================================================================
; Precomputed zoom factors: round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
;
; WHY A RECIPROCAL TABLE:
; The 6502 has no divide instruction. Computing zoom = base / denominator
; would require a multi-byte software division routine (~500+ cycles).
; By precomputing all 256 possible zoom values, we replace the division
; with a single table lookup (~10 cycles).
;
; The denominator (0.5 + sin(i)*0.3) oscillates between 0.2 and 0.8:
;   - At sin(i) = +1.0: denom = 0.5 + 0.3 = 0.8, zoom = 256/0.8 = 320
;   - At sin(i) =  0.0: denom = 0.5 + 0.0 = 0.5, zoom = 256/0.5 = 512
;   - At sin(i) = -1.0: denom = 0.5 - 0.3 = 0.2, zoom = 256/0.2 = 1280
;
; This gives a 4:1 zoom range (320 to 1280), creating the pulsing
; zoom-in / zoom-out effect. The 0.5 base prevents division by zero
; and ensures the minimum denominator (0.2) is safely positive.
;
; TABLE STRUCTURE:
;   Entry 0:   512 (sin(0)=0, denominator=0.5)
;   Entry 64:  320 (sin(64)=1.0, denominator=0.8, closest zoom)
;   Entry 128: 512 (sin(128)=0, denominator=0.5)
;   Entry 192: 1280 (sin(192)=-1.0, denominator=0.2, farthest zoom)
;
; The result is multiplied with sin/cos values to produce CA and SA,
; which encode both rotation and zoom in a single value.
;
; TOTAL SIZE: 256 entries * 2 bytes = 512 bytes.
; ============================================================================
recip_table:
    .word 512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
    .word 416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
    .word 359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
    .word 329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
    .word 320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
    .word 329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
    .word 359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
    .word 416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
    .word 512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
    .word 665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
    .word 889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
    .word 1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
    .word 1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
    .word 1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
    .word 889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
    .word 665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; ============================================================================
; MUSIC DATA - PSG (AY-3-8910) Format
; ============================================================================
; The .ay file format contains embedded Z80 code that drives the PSG
; registers. The Intuition Engine's audio subsystem runs this code on
; its internal Z80 core, just like the SID player runs 6502 code.
;
; The PSG_PLAY_PTR and PSG_PLAY_LEN registers tell the audio subsystem
; where to find the data and how large it is. START_PSG_LOOP writes 5
; to PSG_PLAY_CTRL (bit 0 = start, bit 2 = loop), causing continuous
; playback.
;
; WHY PSG AND NOT SID:
; The PSG (AY-3-8910 / YM2149) is the chip historically associated
; with 8-bit Z80 and 6502 platforms: ZX Spectrum 128, Amstrad CPC,
; MSX, and Atari ST. While the SID is the C64's chip (also 6502),
; using PSG here demonstrates the variety of audio hardware the
; Intuition Engine supports. Either chip works fine with any CPU.
; ============================================================================
psg_data:
    .incbin "WaksonsZak018.ay"
psg_data_end:
