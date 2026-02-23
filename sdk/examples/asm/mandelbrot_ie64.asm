; ============================================================================
; Mandelbrot (IE64) - screen renderer benchmark
; ============================================================================
;
; Ported from:
;   /home/zayn/GolandProjects/exvm/source/junk/mandelbrot.c
;
; Changes from C version:
; - Renders directly to VideoChip VRAM (mode 0, 640x480 BGRA)
; - Runs continuously in a loop (benchmark workload)
; - Uses signed 16.16 fixed-point math for fractal core
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
LINE_BYTES      set 2560                    ; 640 * 4

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

main_loop:
    jsr     render_frame
    bra     main_loop

; -----------------------------------------------------------------------------
; render_frame
; Computes Mandelbrot and writes BGRA pixels directly to VRAM_START.
;
; Pixel mapping:
;   color = (i*i) & 0xFF
;   out   = 0xFF000000 | (color<<16) | (color<<8) | color
; -----------------------------------------------------------------------------
render_frame:
    move.q  r12, #MAX_ITERS                 ; max iterations
    move.q  r13, #BAILOUT_FP                ; bailout threshold (16.16)
    move.q  r14, #WIDTH
    move.q  r15, #HEIGHT

    la      r11, VRAM_START                 ; framebuffer pointer

    ; y = 0, cy = Y_MAX_FP
    move.q  r2, #0
    move.q  r5, #Y_MAX_FP

.y_loop:
    ; x = 0, cx = X_MIN_FP
    move.q  r3, #0
    move.q  r4, #X_MIN_ABS
    neg.q   r4, r4

.x_loop:
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
    lsl.q   r10, r10, #1
    asr.q   r10, r10, #16
    add.q   r10, r10, r5

    ; update z and i
    move.q  r6, r9
    move.q  r7, r10
    add.q   r8, r8, #1

    ; while (i < MAX_ITERS)
    blt     r8, r12, .iter_loop

.iter_done:
    ; gray = (i*i) & 0xFF
    mulu.q  r9, r8, r8
    and.q   r9, r9, #0xFF

    ; rgb = gray | (gray << 8) | (gray << 16)
    move.q  r10, r9
    lsl.q   r10, r10, #8
    add.q   r10, r10, r9                   ; 0x0000GGGG

    move.q  r1, r10
    lsl.q   r1, r1, #16
    add.q   r10, r10, r1                   ; 0x00GGGGGG

    ; color = 0xFF000000 | rgb
    move.q  r1, #255
    lsl.q   r1, r1, #24                    ; 0xFF000000
    add.q   r10, r10, r1

    store.l r10, (r11)

    ; next pixel
    add.q   r11, r11, #4
    add.q   r3, r3, #1
    add.q   r4, r4, #STEP_FP
    blt     r3, r14, .x_loop

    ; next row
    add.q   r2, r2, #1
    sub.q   r5, r5, #STEP_FP
    blt     r2, r15, .y_loop

    rts
