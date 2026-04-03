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

    ; Task 0: PC, USP, signals, state
    move.l  r1, #USER_TASK0_CODE
    store.q r1, KD_TASK0_PC(r12)
    move.l  r1, #USER_TASK0_STACK
    add     r1, r1, #MMU_PAGE_SIZE
    store.q r1, KD_TASK0_USP(r12)
    ; Signal fields: alloc=0xFFFF (system bits pre-allocated), wait=0, recv=0
    move.l  r1, #SIG_SYSTEM_MASK
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_TASK_BASE
    store.l r1, KD_TASK_SIG_ALLOC(r2)      ; task 0 sig_alloc
    store.l r0, KD_TASK_SIG_WAIT(r2)       ; task 0 sig_wait = 0
    store.l r0, KD_TASK_SIG_RECV(r2)       ; task 0 sig_recv = 0
    store.b r0, KD_TASK_STATE(r2)          ; task 0 state = READY (0)
    move.b  r1, #WAITPORT_NONE
    store.b r1, KD_TASK_WAITPORT(r2)       ; task 0 waitport_id = NONE

    ; Task 1: PC, USP, signals, state
    move.l  r1, #USER_TASK1_CODE
    store.q r1, KD_TASK1_PC(r12)
    move.l  r1, #USER_TASK1_STACK
    add     r1, r1, #MMU_PAGE_SIZE
    store.q r1, KD_TASK1_USP(r12)
    move.l  r1, #SIG_SYSTEM_MASK
    add     r2, r2, #KD_TASK_STRIDE        ; advance to task 1
    store.l r1, KD_TASK_SIG_ALLOC(r2)
    store.l r0, KD_TASK_SIG_WAIT(r2)
    store.l r0, KD_TASK_SIG_RECV(r2)
    store.b r0, KD_TASK_STATE(r2)
    move.b  r1, #WAITPORT_NONE
    store.b r1, KD_TASK_WAITPORT(r2)       ; task 1 waitport_id = NONE

    ; PTBR array
    move.l  r1, #USER_PT0_BASE
    store.q r1, KD_PTBR0(r12)
    move.l  r1, #USER_PT1_BASE
    store.q r1, KD_PTBR1(r12)

    ; ---------------------------------------------------------------
    ; 6b. Initialize port slots (all invalid)
    ; ---------------------------------------------------------------
    move.l  r2, #KERN_DATA_BASE
    add     r2, r2, #KD_PORT_BASE      ; R2 = &port[0]
    move.l  r4, #0                      ; port counter
.port_init_loop:
    store.b r0, KD_PORT_VALID(r2)      ; valid = 0
    add     r2, r2, #KD_PORT_STRIDE
    add     r4, r4, #1
    move.l  r5, #KD_PORT_MAX
    blt     r4, r5, .port_init_loop

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

    move.l  r11, #SYS_ALLOC_SIGNAL
    beq     r10, r11, .do_alloc_signal

    move.l  r11, #SYS_FREE_SIGNAL
    beq     r10, r11, .do_free_signal

    move.l  r11, #SYS_SIGNAL
    beq     r10, r11, .do_signal

    move.l  r11, #SYS_WAIT
    beq     r10, r11, .do_wait

    move.l  r11, #SYS_CREATE_PORT
    beq     r10, r11, .do_create_port

    move.l  r11, #SYS_PUT_MSG
    beq     r10, r11, .do_put_msg

    move.l  r11, #SYS_GET_MSG
    beq     r10, r11, .do_get_msg

    move.l  r11, #SYS_WAIT_PORT
    beq     r10, r11, .do_wait_port

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
    load.q  r13, (r12)              ; current_task

    ; Save current task's state
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)

    ; Find next task
    move.q  r16, #1
    sub     r13, r16, r13           ; next = 1 - current

    ; Check if next is READY (skip if WAITING — stay on current)
    lsl     r17, r13, #5
    add     r17, r17, #KD_TASK_BASE
    add     r17, r17, r12
    load.b  r18, KD_TASK_STATE(r17)
    move.l  r19, #TASK_WAITING
    beq     r18, r19, .yield_stay   ; next is WAITING → don't switch

    ; Switch to next
    store.q r13, (r12)
    bra     restore_task

.yield_stay:
    ; Other task is waiting — stay on current task, just return
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

; --- AllocSignal ---
; R1 = requested bit (-1 for auto), returns R1 = bit number, R2 = err
.do_alloc_signal:
    ; Get current task's TCB address
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12           ; R15 = &TCB[current]

    ; Load current sig_alloc
    load.l  r20, KD_TASK_SIG_ALLOC(r15)

    ; If R1 == -1 (0xFFFFFFFF as unsigned), auto-allocate
    move.l  r21, #0xFFFFFFFF
    bne     r1, r21, .alloc_specific

    ; Auto-allocate: scan bits 16-31 for a free bit (bit=0 in sig_alloc)
    move.l  r22, #16               ; start from bit 16
.alloc_scan:
    move.l  r23, #32
    bge     r22, r23, .alloc_exhausted
    ; Check if bit r22 is free in sig_alloc
    move.q  r23, #1
    lsl     r23, r23, r22          ; mask = 1 << bit
    and     r24, r20, r23
    bnez    r24, .alloc_next       ; bit already allocated
    ; Found free bit — allocate it
    or      r20, r20, r23          ; set bit in sig_alloc
    store.l r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r1, r22                ; return bit number
    move.q  r2, #ERR_OK
    eret
.alloc_next:
    add     r22, r22, #1
    bra     .alloc_scan
.alloc_exhausted:
    move.q  r1, #0xFFFFFFFF        ; -1 = failure
    move.q  r2, #ERR_NOMEM
    eret

.alloc_specific:
    ; R1 = specific bit number requested
    ; Check range 16-31
    move.l  r22, #16
    blt     r1, r22, .alloc_badarg
    move.l  r22, #32
    bge     r1, r22, .alloc_badarg
    ; Check if already allocated
    move.q  r23, #1
    lsl     r23, r23, r1
    and     r24, r20, r23
    bnez    r24, .alloc_badarg     ; already allocated
    ; Allocate
    or      r20, r20, r23
    store.l r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r2, #ERR_OK
    eret
.alloc_badarg:
    move.q  r2, #ERR_BADARG
    eret

; --- FreeSignal ---
; R1 = bit number to free, returns R2 = err
.do_free_signal:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12

    ; Check range 16-31
    move.l  r22, #16
    blt     r1, r22, .free_badarg
    move.l  r22, #32
    bge     r1, r22, .free_badarg

    ; Check bit is actually allocated
    load.l  r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r23, #1
    lsl     r23, r23, r1
    and     r24, r20, r23
    beqz    r24, .free_badarg      ; not allocated → ERR_BADARG

    ; Clear bit from sig_alloc
    not     r23, r23
    and     r20, r20, r23
    store.l r20, KD_TASK_SIG_ALLOC(r15)
    move.q  r2, #ERR_OK
    eret
.free_badarg:
    move.q  r2, #ERR_BADARG
    eret

; --- Signal ---
; R1 = target taskID, R2 = signal mask, returns R2 = err
.do_signal:
    ; Validate taskID (must be 0 or 1)
    move.l  r22, #2
    bge     r1, r22, .signal_badarg

    ; Get target TCB
    move.l  r12, #KERN_DATA_BASE
    lsl     r15, r1, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12           ; R15 = &TCB[target]

    ; Set bits in target's signal_recv
    load.l  r20, KD_TASK_SIG_RECV(r15)
    or      r20, r20, r2            ; recv |= mask
    store.l r20, KD_TASK_SIG_RECV(r15)

    ; If target is WAITING and (recv & wait) != 0, set to READY
    load.b  r21, KD_TASK_STATE(r15)
    move.l  r22, #TASK_WAITING
    bne     r21, r22, .signal_done

    load.l  r23, KD_TASK_SIG_WAIT(r15)
    and     r24, r20, r23           ; recv & wait
    beqz    r24, .signal_done

    ; Wake the target: set state to READY
    move.b  r21, #TASK_READY
    store.b r21, KD_TASK_STATE(r15)

.signal_done:
    move.q  r2, #ERR_OK
    eret
.signal_badarg:
    move.q  r2, #ERR_BADARG
    eret

; --- Wait ---
; R1 = signal mask to wait for, returns R1 = received signals
.do_wait:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12           ; R15 = &TCB[current]

    ; Check if any waited signals are already pending
    load.l  r20, KD_TASK_SIG_RECV(r15)
    and     r21, r20, r1            ; matched = recv & mask
    beqz    r21, .wait_block

    ; Immediate return: clear consumed signals
    not     r22, r21
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)
    move.q  r1, r21                 ; R1 = matched signals
    move.q  r2, #ERR_OK
    eret

.wait_block:
    ; No signals ready — block this task
    store.l r1, KD_TASK_SIG_WAIT(r15)   ; save wait mask
    move.b  r22, #TASK_WAITING
    store.b r22, KD_TASK_STATE(r15)     ; state = WAITING

    ; Fall through to context switch (same as yield but skips WAITING tasks)
    ; Save current task's PC and USP
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)

    ; Find next READY task
    move.q  r16, #1
    sub     r13, r16, r13           ; next = 1 - current

    ; Check if next task is runnable (not WAITING)
    lsl     r17, r13, #5
    add     r17, r17, #KD_TASK_BASE
    add     r17, r17, r12
    load.b  r18, KD_TASK_STATE(r17)
    move.l  r19, #TASK_WAITING
    beq     r18, r19, .wait_deadlock

    ; Switch to next task
    store.q r13, (r12)              ; current_task = next
    bra     restore_task           ; shared restore path

.wait_deadlock:
    la      r8, deadlock_msg
    jsr     kern_puts
    halt

; ============================================================================
; Message Port Syscalls
; ============================================================================

; --- CreatePort ---
; Returns R1=portID (0-3), R2=err
.do_create_port:
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)              ; current_task
    ; Scan for unused port slot
    move.l  r20, #0                 ; port index
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE ; R21 = &port[0]
.create_scan:
    move.l  r22, #KD_PORT_MAX
    bge     r20, r22, .create_full
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .create_found
    add     r21, r21, #KD_PORT_STRIDE
    add     r20, r20, #1
    bra     .create_scan
.create_found:
    move.b  r23, #1
    store.b r23, KD_PORT_VALID(r21)     ; valid = 1
    store.b r13, KD_PORT_OWNER(r21)     ; owner = current_task
    store.b r0, KD_PORT_COUNT(r21)      ; count = 0
    store.b r0, KD_PORT_HEAD(r21)       ; head = 0
    store.b r0, KD_PORT_TAIL(r21)       ; tail = 0
    move.q  r1, r20                      ; R1 = portID
    move.q  r2, #ERR_OK
    eret
.create_full:
    move.q  r2, #ERR_NOMEM
    eret

; --- PutMsg ---
; R1=portID, R2=msg_type, R3=msg_data → R2=err
.do_put_msg:
    ; Validate portID
    move.l  r22, #KD_PORT_MAX
    bge     r1, r22, .putmsg_badarg
    ; Compute port address: R21 = KERN_DATA_BASE + KD_PORT_BASE + portID * KD_PORT_STRIDE
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)                  ; current_task (for src_task)
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r1, r20
    add     r21, r21, r20               ; R21 = &port[portID]
    ; Check valid
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .putmsg_badarg
    ; Check FIFO not full (count < 4)
    load.b  r24, KD_PORT_COUNT(r21)
    move.l  r25, #KD_PORT_FIFO_SIZE
    bge     r24, r25, .putmsg_full
    ; Enqueue: compute message slot address
    ; slot = port_base + KD_PORT_MSGS + tail * KD_MSG_SIZE
    load.b  r25, KD_PORT_TAIL(r21)
    move.l  r26, #KD_MSG_SIZE
    mulu    r26, r25, r26               ; tail * 16
    add     r26, r26, r21
    add     r26, r26, #KD_PORT_MSGS     ; R26 = &msg_slot
    ; Write message fields
    store.l r2, KD_MSG_TYPE(r26)        ; mn_Type = R2
    store.l r13, KD_MSG_SRC(r26)        ; mn_SrcTask = current_task
    store.q r3, KD_MSG_DATA(r26)        ; mn_Data = R3
    ; Advance tail (mod 4)
    add     r25, r25, #1
    and     r25, r25, #3               ; tail = (tail + 1) & 3
    store.b r25, KD_PORT_TAIL(r21)
    ; Increment count
    add     r24, r24, #1
    store.b r24, KD_PORT_COUNT(r21)
    ; Signal port owner with SIGF_PORT
    load.b  r27, KD_PORT_OWNER(r21)    ; owner task ID
    lsl     r28, r27, #5
    add     r28, r28, #KD_TASK_BASE
    add     r28, r28, r12              ; R28 = &TCB[owner]
    load.l  r29, KD_TASK_SIG_RECV(r28)
    or      r29, r29, #SIGF_PORT
    store.l r29, KD_TASK_SIG_RECV(r28)
    ; If owner is WAITING on SIGF_PORT, set READY
    load.b  r30, KD_TASK_STATE(r28)
    move.l  r20, #TASK_WAITING
    bne     r30, r20, .putmsg_done
    load.l  r20, KD_TASK_SIG_WAIT(r28)
    and     r20, r20, #SIGF_PORT
    beqz    r20, .putmsg_done
    move.b  r30, #TASK_READY
    store.b r30, KD_TASK_STATE(r28)
.putmsg_done:
    move.q  r2, #ERR_OK
    eret
.putmsg_badarg:
    move.q  r2, #ERR_BADARG
    eret
.putmsg_full:
    move.q  r2, #ERR_AGAIN
    eret

; --- GetMsg ---
; R1=portID → R1=msg_type, R2=msg_data, R3=err
.do_get_msg:
    move.l  r22, #KD_PORT_MAX
    bge     r1, r22, .getmsg_badarg
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
    ; Compute port address
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r1, r20
    add     r21, r21, r20
    ; Check valid
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .getmsg_badarg
    ; Check ownership
    load.b  r24, KD_PORT_OWNER(r21)
    bne     r13, r24, .getmsg_perm
    ; Check count > 0
    load.b  r25, KD_PORT_COUNT(r21)
    beqz    r25, .getmsg_empty
    ; Dequeue: read message at head
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS     ; R27 = &msg_slot
    load.l  r1, KD_MSG_TYPE(r27)        ; R1 = msg_type
    load.q  r2, KD_MSG_DATA(r27)        ; R2 = msg_data
    ; Advance head (mod 4)
    add     r26, r26, #1
    and     r26, r26, #3
    store.b r26, KD_PORT_HEAD(r21)
    ; Decrement count
    sub     r25, r25, #1
    store.b r25, KD_PORT_COUNT(r21)
    move.q  r3, #ERR_OK
    eret
.getmsg_badarg:
    move.q  r3, #ERR_BADARG
    eret
.getmsg_perm:
    move.q  r3, #ERR_PERM
    eret
.getmsg_empty:
    move.q  r3, #ERR_AGAIN
    eret

; --- WaitPort ---
; R1=portID → R1=msg_type, R2=msg_data, R3=err
; Loop: check port, if empty Wait(SIGF_PORT), recheck.
.do_wait_port:
    ; Save portID in R5 (not clobbered by Wait internals)
    move.q  r5, r1
.waitport_loop:
    ; Compute port address
    move.l  r22, #KD_PORT_MAX
    bge     r5, r22, .waitport_badarg
    move.l  r12, #KERN_DATA_BASE
    load.q  r13, (r12)
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r5, r20
    add     r21, r21, r20
    ; Check valid + ownership
    load.b  r23, KD_PORT_VALID(r21)
    beqz    r23, .waitport_badarg
    load.b  r24, KD_PORT_OWNER(r21)
    bne     r13, r24, .waitport_perm
    ; Check count > 0
    load.b  r25, KD_PORT_COUNT(r21)
    bnez    r25, .waitport_dequeue
    ; Empty — block on SIGF_PORT with WaitPort flag
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    move.l  r22, #SIGF_PORT
    store.l r22, KD_TASK_SIG_WAIT(r15)
    store.b r5, KD_TASK_WAITPORT(r15)      ; mark as WaitPort (portID in R5)
    move.b  r22, #TASK_WAITING
    store.b r22, KD_TASK_STATE(r15)
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)
    ; Find next READY task
    move.q  r16, #1
    sub     r13, r16, r13
    lsl     r17, r13, #5
    add     r17, r17, #KD_TASK_BASE
    add     r17, r17, r12
    load.b  r18, KD_TASK_STATE(r17)
    move.l  r19, #TASK_WAITING
    beq     r18, r19, .waitport_deadlock
    store.q r13, (r12)
    bra     restore_task
.waitport_dequeue:
    ; Dequeue (same as GetMsg)
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS
    load.l  r1, KD_MSG_TYPE(r27)
    load.q  r2, KD_MSG_DATA(r27)
    add     r26, r26, #1
    and     r26, r26, #3
    store.b r26, KD_PORT_HEAD(r21)
    load.b  r25, KD_PORT_COUNT(r21)
    sub     r25, r25, #1
    store.b r25, KD_PORT_COUNT(r21)
    move.q  r3, #ERR_OK
    eret
.waitport_badarg:
    move.q  r3, #ERR_BADARG
    eret
.waitport_perm:
    move.q  r3, #ERR_PERM
    eret
.waitport_deadlock:
    la      r8, deadlock_msg
    jsr     kern_puts
    halt

; ============================================================================
; Shared Task Restore (used by yield, wait, and interrupt handlers)
; ============================================================================
; Entry: R13 = next task index, R12 = KERN_DATA_BASE
; Loads PC, USP, PTBR from TCB. Checks signal_wait for Wait/WaitPort delivery.

restore_task:
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12

    ; Load PC
    load.q  r14, KD_TASK_PC(r15)
    mtcr    cr3, r14

    ; Load USP
    load.q  r14, KD_TASK_USP(r15)
    mtcr    cr12, r14

    ; Load PTBR
    lsl     r17, r13, #3
    add     r17, r17, #KD_PTBR_BASE
    add     r17, r17, r12
    load.q  r14, (r17)
    mtcr    cr0, r14
    tlbflush

    ; Check if this task was blocked in Wait (signal_wait != 0)
    load.l  r18, KD_TASK_SIG_WAIT(r15)
    beqz    r18, .restore_normal

    ; Check if this was a WaitPort block (waitport_id != WAITPORT_NONE)
    load.b  r23, KD_TASK_WAITPORT(r15)
    move.l  r24, #WAITPORT_NONE
    bne     r23, r24, .restore_waitport

    ; Plain Wait delivery: compute matched signals
    load.l  r20, KD_TASK_SIG_RECV(r15)
    and     r21, r20, r18           ; matched = recv & wait

    ; Clear consumed signals
    not     r22, r21
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)

    ; Clear wait mask
    store.l r0, KD_TASK_SIG_WAIT(r15)

    ; Set state to RUNNING
    move.b  r22, #TASK_RUNNING
    store.b r22, KD_TASK_STATE(r15)

    ; Deliver result in R1
    move.q  r1, r21
    move.q  r2, #ERR_OK

.restore_normal:
    eret

; --- WaitPort resume: dequeue message from port r23 ---
.restore_waitport:
    ; Compute port address
    move.l  r21, #KERN_DATA_BASE
    add     r21, r21, #KD_PORT_BASE
    move.l  r20, #KD_PORT_STRIDE
    mulu    r20, r23, r20
    add     r21, r21, r20               ; R21 = &port[waitport_id]

    ; Check count > 0 (spurious wake if another port got SIGF_PORT)
    load.b  r25, KD_PORT_COUNT(r21)
    beqz    r25, .restore_waitport_spurious

    ; Message available — dequeue
    load.b  r26, KD_PORT_HEAD(r21)
    move.l  r27, #KD_MSG_SIZE
    mulu    r27, r26, r27
    add     r27, r27, r21
    add     r27, r27, #KD_PORT_MSGS
    load.l  r1, KD_MSG_TYPE(r27)
    load.q  r2, KD_MSG_DATA(r27)
    add     r26, r26, #1
    and     r26, r26, #3
    store.b r26, KD_PORT_HEAD(r21)
    sub     r25, r25, #1
    store.b r25, KD_PORT_COUNT(r21)

    ; Clear wait state
    store.l r0, KD_TASK_SIG_WAIT(r15)
    move.b  r24, #WAITPORT_NONE
    store.b r24, KD_TASK_WAITPORT(r15)
    move.b  r22, #TASK_RUNNING
    store.b r22, KD_TASK_STATE(r15)

    ; Clear consumed SIGF_PORT from recv
    load.l  r20, KD_TASK_SIG_RECV(r15)
    move.l  r24, #SIGF_PORT
    not     r22, r24
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)

    move.q  r3, #ERR_OK
    eret

.restore_waitport_spurious:
    ; Port empty — spurious wake from different port sharing SIGF_PORT.
    ; Clear consumed signal, re-block, switch to other task.
    load.l  r20, KD_TASK_SIG_RECV(r15)
    move.l  r24, #SIGF_PORT
    not     r22, r24
    and     r20, r20, r22
    store.l r20, KD_TASK_SIG_RECV(r15)

    ; Re-block (state=WAITING; signal_wait and waitport_id remain set)
    move.b  r22, #TASK_WAITING
    store.b r22, KD_TASK_STATE(r15)

    ; Find other task
    move.q  r16, #1
    sub     r13, r16, r13
    lsl     r17, r13, #5
    add     r17, r17, #KD_TASK_BASE
    add     r17, r17, r12
    load.b  r18, KD_TASK_STATE(r17)
    move.l  r19, #TASK_WAITING
    beq     r18, r19, .restore_waitport_deadlock
    store.q r13, (r12)
    bra     restore_task

.restore_waitport_deadlock:
    la      r8, deadlock_msg
    jsr     kern_puts
    halt

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
    load.q  r13, (r12)              ; current_task

    ; Save current task
    lsl     r15, r13, #5
    add     r15, r15, #KD_TASK_BASE
    add     r15, r15, r12
    mfcr    r14, cr3
    store.q r14, KD_TASK_PC(r15)
    mfcr    r14, cr12
    store.q r14, KD_TASK_USP(r15)

    ; Find next task
    move.q  r16, #1
    sub     r13, r16, r13

    ; Check if next is READY
    lsl     r17, r13, #5
    add     r17, r17, #KD_TASK_BASE
    add     r17, r17, r12
    load.b  r18, KD_TASK_STATE(r17)
    move.l  r19, #TASK_WAITING
    beq     r18, r19, .intr_stay    ; next is WAITING → don't switch

    ; Switch to next
    store.q r13, (r12)
    bra     restore_task

.intr_stay:
    ; Other task is waiting — stay on current, just return
    eret

; ============================================================================
; User Task Templates (copied to user code pages during init)
; ============================================================================

; Task 0: Create port, yield (let task 1 create its port), then IPC loop
; Port-based request/reply: Exec-style IPC demonstration.
; Task 0 gets portID=0, task 1 gets portID=1 (allocated sequentially).
; Port IDs are immediate constants — no register save across syscalls needed.
user_task0_template:
    syscall #SYS_CREATE_PORT        ; R1 = portID (0)
    move.q  r1, r1                  ; NOP (keeps template at 12 instructions)
    syscall #SYS_YIELD              ; let task 1 create its port
.t0_loop:
    move.l  r1, #0x41              ; 'A'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #1                 ; target port 1
    move.l  r2, #1                 ; msg_type = request
    move.l  r3, #0                 ; msg_data
    syscall #SYS_PUT_MSG
    move.l  r1, #0                 ; own portID (immediate, ABI-safe)
    syscall #SYS_WAIT_PORT
    bra     .t0_loop
user_task0_template_end:

; Task 1: Create port, yield (sync), then WaitPort/print/reply loop
user_task1_template:
    syscall #SYS_CREATE_PORT        ; R1 = portID (1)
    move.q  r1, r1                  ; NOP (keeps template at 12 instructions)
    syscall #SYS_YIELD              ; let task 0 start sending
.t1_loop:
    move.l  r1, #1                 ; own portID (immediate, ABI-safe)
    syscall #SYS_WAIT_PORT
    move.l  r1, #0x42              ; 'B'
    syscall #SYS_DEBUG_PUTCHAR
    move.l  r1, #0                 ; target port 0
    move.l  r2, #2                 ; msg_type = reply
    move.l  r3, #0                 ; msg_data
    syscall #SYS_PUT_MSG
    bra     .t1_loop
user_task1_template_end:

; ============================================================================
; Data: Strings
; ============================================================================

boot_banner:
    dc.b    "IExec M4 boot", 0x0A, 0
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

deadlock_msg:
    dc.b    "DEADLOCK: no runnable tasks", 0x0A, 0
    align   4

; ============================================================================
; End of IExec M2
; ============================================================================
