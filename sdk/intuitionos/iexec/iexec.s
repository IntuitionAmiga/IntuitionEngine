; ============================================================================
; IExec.library - IE64 Microkernel Nucleus (Standalone)
; ============================================================================
;
; Amiga Exec-inspired protected microkernel for the IE64 CPU.
; Milestone 1: Self-sufficient boot with MMU, 2-task preemptive scheduler.
;
; Build:    sdk/bin/ie64asm -I sdk/include sdk/intuitionos/iexec/iexec.s
; Run:      bin/IntuitionEngine -ie64 iexec.ie64
;
; This kernel is fully standalone: it builds its own page tables, creates
; two user tasks, and runs a preemptive scheduler. No host-side setup needed.
;
; All initialization (steps 1-7) runs with MMU OFF so the kernel has direct
; physical memory access to all addresses including user pages at 0x600000+.
;
; Interrupt model:
;   - trapEntry() atomically disables interrupts on entry
;   - ERET atomically re-enables interrupts when returning to user mode
;
; ============================================================================

include "iexec.inc"

; ============================================================================
; Entry Point ($1000)
; ============================================================================

iexec_start:
    ; ---------------------------------------------------------------
    ; 1. Set trap and interrupt vectors
    ; ---------------------------------------------------------------
    move.l  r1, #trap_handler
    mtcr    cr4, r1                 ; CR_TRAP_VEC

    move.l  r1, #intr_handler
    mtcr    cr7, r1                 ; CR_INTR_VEC

    ; ---------------------------------------------------------------
    ; 2. Set kernel stack pointer
    ; ---------------------------------------------------------------
    move.l  r1, #KERN_STACK_TOP
    mtcr    cr8, r1                 ; CR_KSP

    ; ---------------------------------------------------------------
    ; 3. Build kernel page table
    ;    Identity-map pages 0-383 with P|R|W|X (supervisor only)
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_PAGE_TABLE
    move.l  r4, #0                  ; page counter

.kern_pt_loop:
    lsl     r3, r4, #13            ; PTE = page << 13
    or      r3, r3, #0x0F          ; | P|R|W|X (no U)
    lsl     r5, r4, #3             ; offset = page * 8
    add     r5, r5, r2
    store.q r3, (r5)
    add     r4, r4, #1
    move.l  r6, #KERN_PAGES
    blt     r4, r6, .kern_pt_loop

    ; ---------------------------------------------------------------
    ; 4. Build user page table 0 (copy kernel PTEs + add user pages)
    ;    MMU is still OFF — direct physical memory access.
    ; ---------------------------------------------------------------
    move.l  r7, #USER_PT0_BASE     ; destination PT

    ; Copy kernel PTEs (pages 0-383)
    move.l  r2, #KERN_PAGE_TABLE   ; source PT
    move.l  r4, #0
.copy_pt0_loop:
    lsl     r5, r4, #3             ; offset = page * 8
    add     r8, r5, r2             ; src addr
    load.q  r3, (r8)               ; read kernel PTE
    add     r9, r5, r7             ; dst addr
    store.q r3, (r9)               ; write to user PT
    add     r4, r4, #1
    move.l  r6, #KERN_PAGES
    blt     r4, r6, .copy_pt0_loop

    ; Map task 0 user pages (identity-mapped with U bit)
    ; Code page: VPN=0x600, PPN=0x600, P|R|X|U = 0x19
    move.l  r3, #USER_TASK0_CODE_VPN
    lsl     r3, r3, #13            ; PPN << 13
    or      r3, r3, #0x19          ; P|R|X|U
    move.l  r5, #USER_TASK0_CODE_VPN
    lsl     r5, r5, #3             ; PTE offset
    add     r5, r5, r7
    store.q r3, (r5)

    ; Stack page: VPN=0x601, PPN=0x601, P|R|W|U = 0x17
    move.l  r3, #USER_TASK0_STACK_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x17          ; P|R|W|U
    move.l  r5, #USER_TASK0_STACK_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    ; Data page: VPN=0x602, PPN=0x602, P|R|W|U = 0x17
    move.l  r3, #USER_TASK0_DATA_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x17
    move.l  r5, #USER_TASK0_DATA_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    ; ---------------------------------------------------------------
    ; 5. Build user page table 1 (same pattern)
    ; ---------------------------------------------------------------
    move.l  r7, #USER_PT1_BASE

    ; Copy kernel PTEs
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

    ; Map task 1 user pages
    move.l  r3, #USER_TASK1_CODE_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x19          ; P|R|X|U
    move.l  r5, #USER_TASK1_CODE_VPN
    lsl     r5, r5, #3
    add     r5, r5, r7
    store.q r3, (r5)

    move.l  r3, #USER_TASK1_STACK_VPN
    lsl     r3, r3, #13
    or      r3, r3, #0x17          ; P|R|W|U
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
    ; 6. Copy user task code to physical pages (MMU still off)
    ; ---------------------------------------------------------------

    ; Copy task 0 template to 0x600000
    move.l  r1, #user_task0_template    ; source (in kernel image)
    move.l  r2, #USER_TASK0_CODE        ; destination
    move.l  r3, #USER_CODE_SIZE         ; bytes to copy
    move.l  r4, #0                      ; offset
.copy_task0:
    add     r5, r1, r4
    load.q  r6, (r5)
    add     r5, r2, r4
    store.q r6, (r5)
    add     r4, r4, #8
    blt     r4, r3, .copy_task0

    ; Copy task 1 template to 0x610000
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
    ; 7. Initialize task state at KERN_DATA_BASE (MMU still off)
    ; ---------------------------------------------------------------
    move.l  r12, #KERN_DATA_BASE

    ; current_task = 0
    store.q r0, (r12)                   ; R0 = 0

    ; tick_count = 0
    store.q r0, KD_TICK_COUNT(r12)

    ; num_tasks = 2
    move.q  r1, #2
    store.q r1, KD_NUM_TASKS(r12)

    ; Task 0: PC, USP
    move.l  r1, #USER_TASK0_CODE
    store.q r1, KD_TASK0_PC(r12)            ; task 0 PC
    move.l  r1, #USER_TASK0_STACK
    add     r1, r1, #MMU_PAGE_SIZE          ; stack top = base + 4096
    store.q r1, KD_TASK0_USP(r12)           ; task 0 USP

    ; Task 1: PC, USP
    move.l  r1, #USER_TASK1_CODE
    store.q r1, KD_TASK1_PC(r12)            ; task 1 PC
    move.l  r1, #USER_TASK1_STACK
    add     r1, r1, #MMU_PAGE_SIZE
    store.q r1, KD_TASK1_USP(r12)           ; task 1 USP

    ; PTBR array
    move.l  r1, #USER_PT0_BASE
    store.q r1, KD_PTBR0(r12)              ; task 0 PTBR
    move.l  r1, #USER_PT1_BASE
    store.q r1, KD_PTBR1(r12)              ; task 1 PTBR

    ; ---------------------------------------------------------------
    ; 8. Enable MMU
    ; ---------------------------------------------------------------
    move.l  r1, #KERN_PAGE_TABLE
    mtcr    cr0, r1                 ; CR_PTBR
    move.q  r1, #1
    mtcr    cr5, r1                 ; CR_MMU_CTRL = enable

    ; ---------------------------------------------------------------
    ; 9. Program timer (10000 instructions per quantum)
    ; ---------------------------------------------------------------
    move.l  r1, #10000
    mtcr    cr9, r1                 ; CR_TIMER_PERIOD
    move.l  r1, #10000
    mtcr    cr10, r1                ; CR_TIMER_COUNT
    move.q  r1, #3                  ; timer enable + interrupt enable
    mtcr    cr11, r1                ; CR_TIMER_CTRL

    ; ---------------------------------------------------------------
    ; 10. Enter first user task
    ; ---------------------------------------------------------------
    ; Load task 0 state from KERN_DATA_BASE
    move.l  r12, #KERN_DATA_BASE

    load.q  r1, KD_TASK0_USP(r12)      ; task 0 USP
    mtcr    cr12, r1                     ; CR_USP

    load.q  r1, KD_TASK_BASE(r12)       ; task 0 PC
    mtcr    cr3, r1                      ; CR_FAULT_PC

    load.q  r1, KD_PTBR_BASE(r12)       ; task 0 PTBR
    mtcr    cr0, r1                      ; switch page table
    tlbflush

    ; ERET to user mode (atomically re-enables interrupts)
    eret

; ============================================================================
; Trap Handler (SYSCALL + faults)
; ============================================================================

trap_handler:
    mfcr    r10, cr2                ; FAULT_CAUSE
    move.l  r11, #FAULT_SYSCALL
    beq     r10, r11, .syscall_dispatch
    halt                            ; fault → halt

.syscall_dispatch:
    mfcr    r10, cr1                ; syscall number
    move.l  r11, #SYS_YIELD
    beq     r10, r11, .do_yield
    move.l  r11, #SYS_GET_SYS_INFO
    beq     r10, r11, .do_getsysinfo
    move.q  r2, #ERR_BADARG
    eret

.do_yield:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task

    mfcr    r14, cr3
    lsl     r15, r13, #4
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    store.q r14, (r15)              ; save PC

    mfcr    r14, cr12
    store.q r14, 8(r15)             ; save USP

    move.q  r16, #1
    sub     r13, r16, r13           ; next = 1 - current
    store.q r13, (r12)              ; update current_task

    lsl     r15, r13, #4
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    load.q  r14, (r15)
    mtcr    cr3, r14                ; next PC
    load.q  r14, 8(r15)
    mtcr    cr12, r14               ; next USP

    lsl     r15, r13, #3
    add     r15, r15, #KD_PTBR_BASE
    add     r15, r15, r12
    load.q  r14, (r15)
    mtcr    cr0, r14                ; next PTBR
    tlbflush
    eret

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
    load.q  r10, KD_TICK_COUNT(r12)
    add     r10, r10, #1
    store.q r10, KD_TICK_COUNT(r12)

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

user_task0_template:
    move.l  r1, #USER_TASK0_DATA
.t0_loop:
    load.q  r2, (r1)
    add     r2, r2, #1
    store.q r2, (r1)
    bra     .t0_loop
user_task0_template_end:

user_task1_template:
    move.l  r1, #USER_TASK1_DATA
.t1_loop:
    load.q  r2, (r1)
    add     r2, r2, #1
    store.q r2, (r1)
    bra     .t1_loop
user_task1_template_end:

; ============================================================================
; End of IExec Nucleus
; ============================================================================
