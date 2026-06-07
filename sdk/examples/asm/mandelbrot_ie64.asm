; ============================================================================
; Mandelbrot (IE64) - double-buffered screen renderer benchmark
; ============================================================================
;
; Ported from:
;   /home/zayn/GolandProjects/exvm/source/junk/mandelbrot.c
;
; Changes from C version:
; - Renders to alternating VideoChip mode 0 framebuffers (640x480 BGRA)
; - Runs continuously in a loop (benchmark workload)
; - Uses signed 16.16 fixed-point math for fractal core
; - Mirrors the real-axis-symmetric default view to halve fractal work
;
; Build:
;   sdk/bin/ie64asm -I sdk/include sdk/examples/asm/mandelbrot_ie64.asm
;
; Run:
;   ./bin/IntuitionEngine -ie64 mandelbrot_ie64.ie64 -perf
;
; ============================================================================

include "ie64.inc"

; -----------------------------------------------------------------------------
; Constants
; -----------------------------------------------------------------------------

WIDTH           equ 640
HEIGHT          equ 480
HALF_HEIGHT     equ 240
MANDEL_LINE     equ 2560
MANDEL_2_LINES  equ 5120
FRAME_BYTES     equ 1228800

FRONT_BUFFER    equ VRAM_START
BACK_BUFFER     equ 0x900000
PALETTE_BASE    equ 0x800000

MAX_ITERS       equ 256

; 16.16 fixed-point domain
; y: [-1.25,  1.25]
; x: [X_MIN,  0.75]
;
; Keep square-ish sampling by deriving X_MIN from y step and WIDTH:
;   step = (2.5 / HEIGHT) ~= 341 / 65536
;   X_MIN = X_MAX - step * WIDTH
;
X_MAX_FP        equ 49152                   ; 0.75 * 65536
Y_MAX_FP        equ 81920                   ; 1.25 * 65536
STEP_FP         equ 341                     ; approx (2.5 / 480) * 65536
X_MIN_FP        equ -169088                 ; 49152 - (341 * 640)
X_MIN_ABS       equ 169088

BAILOUT_FP      equ 262144                  ; 4.0 * 65536 (canonical Mandelbrot bailout)

org 0x1000

start:
    la      r31, STACK_TOP

    ; Enable VideoChip mode 0 (640x480x32 BGRA)
    la      r1, VIDEO_CTRL
    move.q  r2, #1
    store.l r2, (r1)

    la      r1, VIDEO_MODE
    store.l r0, (r1)                        ; mode 0
    la      r1, VIDEO_FB_BASE
    move.q  r2, #FRONT_BUFFER
    store.l r2, (r1)

    jsr     build_palette

    la      r1, draw_buffer
    move.q  r2, #BACK_BUFFER
    store.l r2, (r1)

main_loop:
    jsr     render_frame
    jsr     wait_vsync

    ; Present the completed off-screen frame.
    la      r3, draw_buffer
    load.l  r2, (r3)
    la      r1, VIDEO_FB_BASE
    store.l r2, (r1)

    ; Draw the next frame into the other buffer.
    move.q  r4, #FRONT_BUFFER
    beq     r2, r4, .use_back_buffer
    store.l r4, (r3)
    bra     .advance_phase
.use_back_buffer:
    move.q  r4, #BACK_BUFFER
    store.l r4, (r3)

.advance_phase:
    la      r1, color_phase
    load.l  r2, (r1)
    add.q   r2, r2, #3
    and.q   r2, r2, #0xFF
    store.l r2, (r1)
    bra     main_loop

; -----------------------------------------------------------------------------
; wait_vsync
; Two-phase wait so presentation catches the next blanking edge.
; -----------------------------------------------------------------------------
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

; -----------------------------------------------------------------------------
; build_palette
; Builds a 256-entry BGRA gradient once. The render loop then uses one table
; load per pixel instead of per-pixel RGB multiplies and shifts.
; -----------------------------------------------------------------------------
build_palette:
    move.q  r1, #PALETTE_BASE
    move.q  r2, #0                          ; index
    move.q  r6, #256

.palette_loop:
    ; red rises quickly, green peaks through the middle, blue cools the edges
    mulu.q  r3, r2, #5
    and.q   r3, r3, #0xFF                   ; red

    mulu.q  r4, r2, #3
    add.q   r4, r4, #64
    and.q   r4, r4, #0xFF                   ; green

    mulu.q  r5, r2, #7
    add.q   r5, r5, #128
    and.q   r5, r5, #0xFF                   ; blue

    lsl.l   r3, r3, #16
    lsl.l   r4, r4, #8
    or.l    r5, r5, r4
    or.l    r5, r5, r3
    add.q   r5, r5, #0xFF000000
    store.l r5, (r1)

    add.q   r1, r1, #4
    add.q   r2, r2, #1
    blt     r2, r6, .palette_loop
    rts

; -----------------------------------------------------------------------------
; render_frame
; Computes Mandelbrot into the current off-screen buffer.
;
; Pixel mapping:
;   palette_index = (i*i + colour_phase) & 0xFF
;   out           = palette[palette_index]
; -----------------------------------------------------------------------------
render_frame:
    move.q  r12, #MAX_ITERS                 ; max iterations
    move.q  r13, #BAILOUT_FP                ; bailout threshold (16.16)
    move.q  r14, #WIDTH
    move.q  r15, #HALF_HEIGHT

    la      r1, draw_buffer
    load.l  r11, (r1)                       ; top-row framebuffer pointer

    move.q  r18, #FRAME_BYTES
    sub.q   r18, r18, #MANDEL_LINE
    add.q   r18, r18, r11                   ; mirrored bottom-row pointer

    move.q  r19, #PALETTE_BASE
    la      r16, color_phase
    load.l  r16, (r16)

    ; y = 0, cy = Y_MAX_FP
    move.q  r2, #0
    move.q  r5, #Y_MAX_FP

.y_loop:
    ; x = 0, cx = X_MIN_FP
    move.q  r3, #0
    move.q  r4, #X_MIN_ABS
    neg.q   r4, r4

.x_loop:
    ; Reject the period-2 bulb: (cx + 1)^2 + cy^2 <= 1/16.
    move.q  r20, r4
    add.q   r20, r20, #65536
    muls.q  r21, r20, r20
    asr.q   r21, r21, #16
    muls.q  r22, r5, r5
    asr.q   r22, r22, #16
    add.q   r21, r21, r22
    move.q  r23, #4096
    ble     r21, r23, .inside_set

    ; Reject the main cardioid:
    ; q = (cx - 0.25)^2 + cy^2
    ; q * (q + cx - 0.25) <= 0.25 * cy^2
    move.q  r20, r4
    sub.q   r20, r20, #16384
    muls.q  r21, r20, r20
    asr.q   r21, r21, #16
    add.q   r21, r21, r22                   ; q
    add.q   r23, r21, r20                   ; q + cx - 0.25
    muls.q  r23, r21, r23
    asr.q   r23, r23, #16
    move.q  r24, r22
    asr.q   r24, r24, #2
    ble     r23, r24, .inside_set

    ; i = 0, zx = 0, zy = 0 (canonical Mandelbrot)
    move.q  r8, #0
    move.q  r6, r0
    move.q  r7, r0

.iter_loop:
    ; zx2 = (zx*zx) >> 16
    muls.q  r9, r6, r6
    asr.q   r9, r9, #16

    ; zy2 = (zy*zy) >> 16
    muls.q  r10, r7, r7
    asr.q   r10, r10, #16

    ; test = zx2 + zy2
    add.q   r1, r9, r10
    bge     r1, r13, .iter_done

    ; newZX = zx2 - zy2 + cx
    sub.q   r9, r9, r10
    add.q   r9, r9, r4

    ; newZY = ((2*zx*zy) >> 16) + cy
    muls.q  r10, r6, r7
    asr.q   r10, r10, #15
    add.q   r10, r10, r5

    ; update z and i
    move.q  r6, r9
    move.q  r7, r10
    add.q   r8, r8, #1

    ; while (i < MAX_ITERS)
    blt     r8, r12, .iter_loop

.iter_done:
    bge     r8, r12, .inside_set

    ; Colour phase turns the benchmark frame into a palette cycle.
    mulu.q  r9, r8, r8
    add.q   r9, r9, r16
    and.q   r9, r9, #0xFF

    lsl.q   r9, r9, #2
    add.q   r9, r9, r19
    load.l  r10, (r9)
    bra     .write_pixel

.inside_set:
    move.q  r10, #0xFF000000

.write_pixel:
    store.l r10, (r11)
    store.l r10, (r18)

    ; next pixel
    add.q   r11, r11, #4
    add.q   r18, r18, #4
    add.q   r3, r3, #1
    add.q   r4, r4, #STEP_FP
    blt     r3, r14, .x_loop

    ; next row
    sub.q   r18, r18, #MANDEL_2_LINES
    add.q   r2, r2, #1
    sub.q   r5, r5, #STEP_FP
    blt     r2, r15, .y_loop

    rts

color_phase: dc.l 0
draw_buffer: dc.l 0
