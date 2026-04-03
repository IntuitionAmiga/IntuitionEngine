; ============================================================================
; IExec.library - IE64 Microkernel Nucleus (M2: Observable Kernel)
; ============================================================================
;
; Amiga Exec-inspired protected microkernel for the IE64 CPU.
; M2: Boot banner, DebugPutChar syscall, visible demo tasks, fault reporting.
;
; Build:    sdk/bin/ie64asm -I sdk/include sdk/intuitionos/iexec/iexec.s
; Run:      bin/IntuitionEngine -ie64 iexec.ie64
;
; ============================================================================

include "iexec.inc"
include "ie64.inc"

; ============================================================================
; Entry Point ($1000)
; ============================================================================

iexec_start:
    ; ---------------------------------------------------------------
    ; 1. Set trap and interrupt vectors
    ; ---------------------------------------------------------------
    move.l  r1, #trap_handler
    mtcr    cr4, r1
    move.l  r1, #intr_handler
    mtcr    cr7, r1

    ; ---------------------------------------------------------------
    ; 2. Set kernel stack pointer
    ; ---------------------------------------------------------------
    move.l  r1, #KERN_STACK_TOP
    mtcr    cr8, r1

    ; ---------------------------------------------------------------
    ; 3. Build kernel page table (pages 0-383, supervisor only)
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.kern_pt_loop:
    lsl     r3, r4, #13
    or      r3, r3, #0x0F
    lsl     r5, r4, #3
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    move.l  r6, #KERN_PAGES
    blt     r4, r6, .kern_pt_loop

    ; ---------------------------------------------------------------
    ; 4. Build user page tables (copy kernel + add user pages)
    ; ---------------------------------------------------------------

    ; --- User PT 0 ---
    move.l  r7, #USER_PT0_BASE
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.copy_pt0_loop:
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    move.l  r6, #KERN_PAGES
    blt     r4, r6, .copy_pt0_loop

    ; Task 0 user pages (identity-mapped with U)
    move.l  r3, #USER_TASK0_CODE_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x19
    move.l  r5, #USER_TASK0_CODE_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    move.l  r3, #USER_TASK0_STACK_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x17
    move.l  r5, #USER_TASK0_STACK_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    move.l  r3, #USER_TASK0_DATA_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x17
    move.l  r5, #USER_TASK0_DATA_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    ; --- User PT 1 ---
    move.l  r7, #USER_PT1_BASE
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0
.copy_pt1_loop:
    lsl     r5, r4, #3
    add     r8, r5, r2
    load.q  r3, (r8)
    add     r9, r5, r7
    store.q r3, (r9)
    add     r4, r4, #1
    move.l  r6, #KERN_PAGES
    blt     r4, r6, .copy_pt1_loop

    move.l  r3, #USER_TASK1_CODE_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x19
    move.l  r5, #USER_TASK1_CODE_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    move.l  r3, #USER_TASK1_STACK_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x17
    move.l  r5, #USER_TASK1_STACK_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    move.l  r3, #USER_TASK1_DATA_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x17
    move.l  r5, #USER_TASK1_DATA_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    ; ---------------------------------------------------------------
    ; 5. Copy user task code to physical pages (MMU still off)
    ; ---------------------------------------------------------------
    move.l  r1, #user_task0_template
    move.l  r2, #USER_TASK0_CODE
    move.l  r3, #USER_CODE_SIZE
    move.l  r4, #0
.copy_task0:
    add     r5, r1, r4
    load.q  r6, (r5)
    add     r5, r2, r4
    store.q r6, (r5)
    add     r4, r4, #8
    blt     r4, r3, .copy_task0

    move.l  r1, #user_task1_template
    move.l  r2, #USER_TASK1_CODE
    move.l  r4, #0
.copy_task1:
    add     r5, r1, r4
    load.q  r6, (r5)
    add     r5, r2, r4
    store.q r6, (r5)
    add     r4, r4, #8
    blt     r4, r3, .copy_task1

    ; ---------------------------------------------------------------
    ; 6. Initialize task state at KERN_DATA_BASE (MMU still off)
    ; ---------------------------------------------------------------
    move.l  r12, #KERN_DATA_BASE
    store.q r0, (r12)
    store.q r0, KD_TICK_COUNT(r12)
    move.q  r1, #2
    store.q r1, KD_NUM_TASKS(r12)

    move.l  r1, #USER_TASK0_CODE
    store.q r1, KD_TASK0_PC(r12)
    move.l  r1, #USER_TASK0_STACK
    add     r1, r1, #MMU_PAGE_SIZE
    store.q r1, KD_TASK0_USP(r12)

    move.l  r1, #USER_TASK1_CODE
    store.q r1, KD_TASK1_PC(r12)
    move.l  r1, #USER_TASK1_STACK
    add     r1, r1, #MMU_PAGE_SIZE
    store.q r1, KD_TASK1_USP(r12)

    move.l  r1, #USER_PT0_BASE
    store.q r1, KD_PTBR0(r12)
    move.l  r1, #USER_PT1_BASE
    store.q r1, KD_PTBR1(r12)

    ; ---------------------------------------------------------------
    ; 7. Enable MMU
    ; ---------------------------------------------------------------
    move.l  r1, #KERN_PAGE_TABLE
    mtcr    cr0, r1
    move.q  r1, #1
    mtcr    cr5, r1

    ; ---------------------------------------------------------------
    ; 8. Print boot banner (kernel is privileged, TERM_OUT is mapped)
    ; ---------------------------------------------------------------
    la      r8, boot_banner
    jsr     kern_puts

    ; ---------------------------------------------------------------
    ; 9. Program timer
    ; ---------------------------------------------------------------
    move.l  r1, #10000
    mtcr    cr9, r1
    move.l  r1, #10000
    mtcr    cr10, r1
    move.q  r1, #3
    mtcr    cr11, r1

    ; ---------------------------------------------------------------
    ; 10. Enter first user task
    ; ---------------------------------------------------------------
    move.l  r12, #KERN_DATA_BASE
    load.q  r1, KD_TASK0_USP(r12)
    mtcr    cr12, r1
    load.q  r1, KD_TASK0_PC(r12)
    mtcr    cr3, r1
    load.q  r1, KD_PTBR0(r12)
    mtcr    cr0, r1
    tlbflush
    eret

; ============================================================================
; Kernel Output Helpers
; ============================================================================

; kern_put_char: print byte in R8 to TERM_OUT
kern_put_char:
    la      r28, TERM_OUT
    store.b r8, (r28)
    rts

; kern_puts: print null-terminated string at R8 to TERM_OUT
kern_puts:
    la      r28, TERM_OUT
.puts_loop:
    load.b  r29, (r8)
    beqz    r29, .puts_done
    store.b r29, (r28)
    add     r8, r8, #1
    bra     .puts_loop
.puts_done:
    rts

; kern_put_hex: print 64-bit value in R8 as 16-digit hex to TERM_OUT
kern_put_hex:
    push    r20
    push    r21
    push    r22
    la      r28, TERM_OUT
    move.q  r20, r8             ; value to print
    move.q  r21, #60            ; shift amount (start from top nibble)
.hex_loop:
    lsr     r22, r20, r21       ; shift right
    and     r22, r22, #0x0F     ; mask nibble
    ; Convert to ASCII: 0-9 -> '0'-'9', 10-15 -> 'A'-'F'
    move.q  r29, #10
    blt     r22, r29, .hex_digit
    add     r22, r22, #55       ; 'A' - 10 = 55
    bra     .hex_emit
.hex_digit:
    add     r22, r22, #48       ; '0' = 48
.hex_emit:
    store.b r22, (r28)
    sub     r21, r21, #4
    move.q  r29, #0
    bge     r21, r29, .hex_loop
    pop     r22
    pop     r21
    pop     r20
    rts

; ============================================================================
; Trap Handler
; ============================================================================

trap_handler:
    mfcr    r10, cr2

    move.l  r11, #FAULT_SYSCALL
    beq     r10, r11, .syscall_dispatch

    ; --- Fault reporting ---
    ; Print: FAULT cause=N PC=$XXXX ADDR=$XXXX\n
    la      r8, fault_msg_prefix    ; "FAULT cause="
    jsr     kern_puts
    move.q  r8, r10                 ; cause code
    jsr     kern_put_hex
    la      r8, fault_msg_pc        ; " PC="
    jsr     kern_puts
    mfcr    r8, cr3                 ; FAULT_PC
    jsr     kern_put_hex
    la      r8, fault_msg_addr      ; " ADDR="
    jsr     kern_puts
    mfcr    r8, cr1                 ; FAULT_ADDR
    jsr     kern_put_hex
    move.q  r8, #0x0A              ; newline
    jsr     kern_put_char
    halt

.syscall_dispatch:
    mfcr    r10, cr1

    move.l  r11, #SYS_YIELD
    beq     r10, r11, .do_yield

    move.l  r11, #SYS_GET_SYS_INFO
    beq     r10, r11, .do_getsysinfo

    move.l  r11, #SYS_DEBUG_PUTCHAR
    beq     r10, r11, .do_debug_putchar

    move.q  r2, #ERR_BADARG
    eret

; --- DebugPutChar ---
.do_debug_putchar:
    ; R1 = character (preserved from user context, not clobbered by dispatch)
    la      r28, TERM_OUT
    store.b r1, (r28)
    move.q  r2, #ERR_OK
    eret

; --- Yield ---
.do_yield:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)

    mfcr    r14, cr3
    lsl     r15, r13, #4
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    store.q r14, (r15)

    mfcr    r14, cr12
    store.q r14, 8(r15)

    move.q  r16, #1
    sub     r13, r16, r13
    store.q r13, (r12)

    lsl     r15, r13, #4
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    load.q  r14, (r15)
    mtcr    cr3, r14
    load.q  r14, 8(r15)
    mtcr    cr12, r14

    lsl     r15, r13, #3
    add     r15, r15, #KD_PTBR_BASE
    add     r15, r15, r12
    load.q  r14, (r15)
    mtcr    cr0, r14
    tlbflush
    eret

; --- GetSysInfo ---
.do_getsysinfo:
    move.l  r11, #SYSINFO_TICK_COUNT
    beq     r1, r11, .info_ticks
    move.q  r1, #0
    move.q  r2, #ERR_OK
    eret
.info_ticks:
    move.l  r11, #KERN_DATA_BASE
    load.q  r1, KD_TICK_COUNT(r11)
    move.q  r2, #ERR_OK
    eret

; ============================================================================
; Interrupt Handler (Timer Preemption)
; ============================================================================

intr_handler:
    move.l  r12, #KERN_DATA_BASE

    ; Increment tick count
    load.q  r10, KD_TICK_COUNT(r12)
    add     r10, r10, #1
    store.q r10, KD_TICK_COUNT(r12)

    ; Heartbeat: print '.' every IEXEC_HEARTBEAT_INTERVAL ticks
    and     r11, r10, #IEXEC_HEARTBEAT_INTERVAL-1
    bnez    r11, .no_heartbeat
    move.q  r8, #0x2E              ; '.'
    jsr     kern_put_char
.no_heartbeat:

    ; Context switch
    load.q  r13, (r12)

    mfcr    r14, cr3
    lsl     r15, r13, #4
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    store.q r14, (r15)

    mfcr    r14, cr12
    store.q r14, 8(r15)

    move.q  r16, #1
    sub     r13, r16, r13
    store.q r13, (r12)

    lsl     r15, r13, #4
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    load.q  r14, (r15)
    mtcr    cr3, r14
    load.q  r14, 8(r15)
    mtcr    cr12, r14

    lsl     r15, r13, #3
    add     r15, r15, #KD_PTBR_BASE
    add     r15, r15, r12
    load.q  r14, (r15)
    mtcr    cr0, r14
    tlbflush
    eret

; ============================================================================
; User Task Templates (copied to user code pages during init)
; ============================================================================

; Task 0: print 'A', delay, yield, loop
user_task0_template:
.t0_loop:
    move.l  r1, #0x41              ; 'A'
    syscall #SYS_DEBUG_PUTCHAR
    ; delay loop
    move.l  r2, #0
    move.l  r3, #5000
.t0_delay:
    add     r2, r2, #1
    blt     r2, r3, .t0_delay
    syscall #SYS_YIELD
    bra     .t0_loop
user_task0_template_end:

; Task 1: print 'B', delay, yield, loop
user_task1_template:
.t1_loop:
    move.l  r1, #0x42              ; 'B'
    syscall #SYS_DEBUG_PUTCHAR
    ; delay loop
    move.l  r2, #0
    move.l  r3, #5000
.t1_delay:
    add     r2, r2, #1
    blt     r2, r3, .t1_delay
    syscall #SYS_YIELD
    bra     .t1_loop
user_task1_template_end:

; ============================================================================
; Data: Strings
; ============================================================================

boot_banner:
    dc.b    "IExec M2 boot", 0x0A, 0
    align   4

fault_msg_prefix:
    dc.b    "FAULT cause=", 0
    align   4

fault_msg_pc:
    dc.b    " PC=", 0
    align   4

fault_msg_addr:
    dc.b    " ADDR=", 0
    align   4

; ============================================================================
; End of IExec M2
; ============================================================================
