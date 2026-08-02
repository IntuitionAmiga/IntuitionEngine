; IE64 bare-metal C runtime entry point.
;
; The ie64-cproc driver owns the image origin and emits `org 0x1000` before
; this file.  This unit is intentionally linkable and therefore has no org.
; It does not include ie64.inc: the bare-metal C ABI owns this layout.

__ie64_start:
    ; The ABI reserves [0x8F000, 0x9F000) for the full-descending stack.
    li      r31, #0x0009F000

    ; Refuse a machine on which the reservation is not wholly visible.
    mfcr    r1, cr15
    li      r2, #0x0009F000
    blt     r1, r2, __ie64_startup_failure

    jsr     main
    halt

__ie64_startup_failure:
    ; R1 distinguishes startup failure from a normal main return.  There is
    ; deliberately no output device or operating-system dependency.
    li      r1, #1
    halt
