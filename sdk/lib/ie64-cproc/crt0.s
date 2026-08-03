; IE64 bare-metal C runtime entry point.
;
; The linker places `_start` at canonical PROG_START.  This unit is
; intentionally relocatable and therefore has no org.
; It does not include ie64.inc: the bare-metal C ABI owns this layout.

.section .text,"ax"
.global _start
.global __ie64_start
.global __libc_init_array
.global exit
.type _start,@function
_start:
__ie64_start:
    ; The ABI reserves [0x8F000, 0x9F000) for the full-descending stack.
    li      r31, #0x0009F000

    ; Refuse a machine on which the reservation is not wholly visible.
    mfcr    r1, cr15
    li      r2, #0x0009F000
    blt     r1, r2, __ie64_startup_failure

    ; The linker owns the BSS bounds. Clear the exact linked interval.
    move.l  r2, #lo32(__bss_start)
    movt    r2, #hi32(__bss_start)
    move.l  r3, #lo32(__bss_end)
    movt    r3, #hi32(__bss_end)
__ie64_clear_bss:
    beq     r2, r3, __ie64_bss_cleared
    store.b r0, (r2)
    add.q   r2, r2, #1
    bra     __ie64_clear_bss

__ie64_bss_cleared:
    ; A default V2 link always includes Picolibc initialisation and exit.
    move.l  r2, #lo32(__libc_init_array)
    movt    r2, #hi32(__libc_init_array)
    jsr     (r2)

__ie64_call_main:
    jsr     main

    ; main's result is already in R1, the first argument register.
    move.l  r2, #lo32(exit)
    movt    r2, #hi32(exit)
    jsr     (r2)
__ie64_exit_returned:
    halt

__ie64_startup_failure:
    ; R1 distinguishes startup failure from a normal main return.  There is
    ; deliberately no output device or operating-system dependency.
    li      r1, #1
    halt
.size _start,.-_start
